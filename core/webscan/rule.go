package webscan

import (
	"container/list"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slack-wails/lib/gologger"
	"slack-wails/lib/util"
	"strings"

	"gopkg.in/yaml.v2"
)

var fps map[string]interface{}

type FingerPEntity struct {
	ProductName      string
	AllString        string
	Rule             []RuleData
	IsExposureDetect bool
}

type RuleData struct {
	Start int
	End   int
	Op    int16  // 0= 1!= 2== 3>= 4<= 5~=
	Key   string // body="123"中的body
	Value string // body="123"中的123
	All   string // body="123"
}

var FingerprintDB []FingerPEntity
var ActiveFingerprintDB []FingerPEntity

func InitFingprintDB(fingerprintFile string) error {
	data, err := os.ReadFile(fingerprintFile)
	if err != nil {
		return err
	}
	fps = make(map[string]interface{})
	m := make(map[string][]string)
	err = yaml.Unmarshal(data, &fps)
	if err == nil {
		for productName, rulesInterface := range fps {
			for _, ruleInterface := range rulesInterface.([]interface{}) {
				ruleL := ruleInterface.(string)
				_, ok := m[productName]
				if ok {
					f := m[productName]
					if util.GetItemInArray(f, ruleL) == -1 {
						f = append(f, ruleL)
					}
					m[productName] = f
				} else {
					m[productName] = []string{ruleL}
				}
			}
		}
	}
	for productName, ruleLs := range m {
		for _, ruleL := range ruleLs {
			FingerprintDB = append(FingerprintDB, FingerPEntity{ProductName: productName, Rule: ParseRule(ruleL), AllString: ruleL})
		}
	}
	return nil
}

var Sensitive map[string][]string

type SensitivePath struct {
	Name string
	Path []string
}

func InitActiveScanPath(activefingerFile string) error {
	data, err := os.ReadFile(activefingerFile)
	if err != nil {
		return err
	}
	Sensitive = make(map[string][]string)
	err = yaml.Unmarshal(data, &Sensitive)
	if err != nil {
		return err
	}
	for _, fpe := range FingerprintDB {
		for name := range Sensitive {
			if fpe.ProductName == name {
				ActiveFingerprintDB = append(ActiveFingerprintDB, fpe)
				break
			}
		}
	}
	return nil
}

type FingerPoc struct {
	URL      string
	PocFiles []string
}

func ALLPoc() []string {
	var files []string
	root := util.HomeDir() + "/slack/config/pocs/"
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 检查文件后缀名
		if filepath.Ext(path) == ".yaml" {
			fileName := strings.TrimSuffix(filepath.Base(path), ".yaml")
			files = append(files, fileName)
		}
		return nil
	})
	return files
}

func FullPocName(pocs []string) []string {
	var news []string
	for _, poc := range pocs {
		if !strings.HasSuffix(poc, ".yaml") {
			poc = poc + ".yaml"
		}
		poc = util.HomeDir() + "/slack/config/pocs/" + poc
		news = append(news, poc)
	}
	return news
}

func ParseRule(rule string) []RuleData {
	var result []RuleData
	empty := RuleData{}

	for {
		data := getRuleData(rule)
		if data == empty {
			break
		}
		result = append(result, data)
		rule = rule[:data.Start] + "T" + rule[data.End:]
	}
	return result
}

func getRuleData(rule string) RuleData {
	if !strings.Contains(rule, "=\"") {
		return RuleData{}
	}
	pos := strings.Index(rule, "=\"")
	op := 0
	if rule[pos-1] == 33 {
		op = 1
	} else if rule[pos-1] == 61 {
		op = 2
	} else if rule[pos-1] == 62 {
		op = 3
	} else if rule[pos-1] == 60 {
		op = 4
	} else if rule[pos-1] == 126 {
		op = 5
	}

	start := 0
	ti := 0
	if op > 0 {
		ti = 1
	}
	for i := pos - 1 - ti; i >= 0; i-- {
		if (rule[i] > 122 || rule[i] < 97) && rule[i] != 95 {
			start = i + 1
			break
		}

	}
	key := rule[start : pos-ti]

	end := pos + 2
	for i := pos + 2; i < len(rule)-1; i++ {
		if rule[i] != 92 && rule[i+1] == 34 {
			end = i + 2
			break
		}
	}
	value := rule[pos+2 : end-1]
	all := rule[start:end]

	return RuleData{Start: start, End: end, Op: int16(op), Key: key, Value: value, All: all}
}

// 计算纯bool表达式，支持 ! && & || | ( )
func boolEval(ctx context.Context, expression string) bool {
	// 左右括号相等
	if strings.Count(expression, "(") != strings.Count(expression, ")") {
		gologger.Warning(ctx, fmt.Sprintf("纯布尔表达式 [%s] 左右括号不匹配", expression))
	}
	// 去除空格
	for strings.Contains(expression, " ") {
		expression = strings.ReplaceAll(expression, " ", "")
	}
	// 去除空表达式
	for strings.Contains(expression, "()") {
		expression = strings.ReplaceAll(expression, "()", "")
	}
	for strings.Contains(expression, "&&") {
		expression = strings.ReplaceAll(expression, "&&", "&")
	}
	for strings.Contains(expression, "||") {
		expression = strings.ReplaceAll(expression, "||", "|")
	}
	if !strings.Contains(expression, "T") && !strings.Contains(expression, "F") {
		return false
		// panic("纯布尔表达式错误，没有包含T/F")
	}

	expr := list.New()
	operator_stack := list.New()
	for _, ch := range expression {
		// ch 为 T或者F
		if ch == 84 || ch == 70 {
			expr.PushBack(int(ch))
		} else if advance(int(ch)) > 0 {
			if operator_stack.Len() == 0 {
				operator_stack.PushBack(int(ch))
				continue
			}
			// 两个!抵消
			if ch == 33 && operator_stack.Back().Value.(int) == 33 {
				operator_stack.Remove(operator_stack.Back())
				continue
			}
			for operator_stack.Len() != 0 && operator_stack.Back().Value.(int) != 40 && advance(operator_stack.Back().Value.(int)) >= advance(int(ch)) {
				e := operator_stack.Back()
				expr.PushBack(e.Value.(int))
				operator_stack.Remove(e)
			}
			operator_stack.PushBack(int(ch))

		} else if ch == 40 {
			operator_stack.PushBack(int(ch))
		} else if ch == 41 {
			for operator_stack.Back().Value.(int) != 40 {
				e := operator_stack.Back()
				expr.PushBack(e.Value.(int))
				operator_stack.Remove(e)
			}
			operator_stack.Remove(operator_stack.Back())
		}
	}
	for operator_stack.Len() != 0 {
		e := operator_stack.Back()
		expr.PushBack(e.Value.(int))
		operator_stack.Remove(e)
	}

	tf_stack := list.New()
	for expr.Len() != 0 {
		e := expr.Front()
		ch := e.Value.(int)
		expr.Remove(e)
		if ch == 84 || ch == 70 {
			tf_stack.PushBack(int(ch))
		}
		if ch == 38 { // &
			em := tf_stack.Back()
			a := em.Value.(int)
			tf_stack.Remove(em)
			em = tf_stack.Back()
			b := em.Value.(int)
			tf_stack.Remove(em)
			if a == 84 && b == 84 {
				tf_stack.PushBack(84)
			} else {
				tf_stack.PushBack(70)
			}
		}
		if ch == 124 { // |
			em := tf_stack.Back()
			a := em.Value.(int)
			tf_stack.Remove(em)
			em = tf_stack.Back()
			b := em.Value.(int)
			tf_stack.Remove(em)
			if a == 70 && b == 70 {
				tf_stack.PushBack(70)
			} else {
				tf_stack.PushBack(84)
			}
		}
		if ch == 33 { // !
			em := tf_stack.Back()
			a := em.Value.(int)
			tf_stack.Remove(em)
			if a == 70 {
				tf_stack.PushBack(84)
			} else if a == 84 {
				tf_stack.PushBack(70)
			}
		}
	}
	if tf_stack.Front().Value.(int) == 84 {
		return true
	} else {
		return false
	}

}

// 判断优先级 非运算符返回0
func advance(ch int) int {
	// !
	if ch == 33 {
		return 3
	}
	// &
	if ch == 38 {
		return 2
	}
	// |
	if ch == 124 {
		return 1
	}
	return 0
}

func regexMatch(pattern string, s string) (bool, error) {
	matched, err := regexp.MatchString(pattern, s)
	if err != nil {
		return false, err
	}
	return matched, nil
}

// body="123"  op=0  dataSource为http.body dataRule=123
func dataCheckString(op int16, dataSource string, dataRule string) bool {
	dataSource = strings.ToLower(dataSource)

	dataRule = strings.ToLower(dataRule)
	dataRule = strings.ReplaceAll(dataRule, "\\\"", "\"")
	if op == 0 {
		if strings.Contains(dataSource, dataRule) {
			return true
		}
	} else if op == 1 {
		if !strings.Contains(dataSource, dataRule) {
			return true
		}
	} else if op == 2 {
		if dataSource == dataRule {
			return true
		}
	} else if op == 5 {
		rs, err := regexMatch(dataRule, dataSource)
		if err == nil && rs {
			return true
		}
	}
	return false
}

func dataCheckInt(op int16, dataSource int, dataRule int) bool {
	if op == 0 { // 数字相等
		if dataSource == dataRule {
			return true
		}
	} else if op == 1 { // 数字不相等
		if dataSource != dataRule {
			return true
		}
	} else if op == 3 { // 大于等于
		if dataSource >= dataRule {
			return true
		}
	} else if op == 4 {
		if dataSource <= dataRule {
			return true
		}
	}
	return false
}

var WorkFlowDB map[string][]string

func InitAll(webfinger, activefinger string) bool {
	FingerprintDB = nil
	if err := InitFingprintDB(webfinger); err != nil {
		return false
	}
	if err := InitActiveScanPath(activefinger); err != nil {
		return false
	}
	return true
}
func InitWorkflow(workflowFile string) error {
	WorkFlowDB = make(map[string][]string)
	data, err := os.ReadFile(workflowFile)
	if err != nil {
		return err
	}
	err = yaml.Unmarshal(data, &WorkFlowDB)
	if err != nil {
		return err
	}
	return nil
}

// type TemplateInfo struct {
// 	Tags string `yaml:"tags"`
// }

// type Template struct {
// 	Info TemplateInfo `yaml:"info"`
// }

// func GetTagsList(templateDir string) map[string][]string {
// 	// 初始化字典
// 	tagsDict := make(map[string][]string)
// 	// 遍历所有模板文件
// 	filepath.WalkDir(templateDir, func(path string, d fs.DirEntry, err error) error {
// 		if err != nil {
// 			return err
// 		}
// 		if !d.IsDir() && strings.HasSuffix(d.Name(), ".yaml") {
// 			file, err := os.ReadFile(path)
// 			if err != nil {
// 				fmt.Printf("Error reading file %s: %v\n", path, err)
// 				return nil
// 			}

// 			var template Template
// 			err = yaml.Unmarshal(file, &template)
// 			if err != nil {
// 				fmt.Printf("Error parsing file %s: %v\n", path, err)
// 				return nil
// 			}

// 			if template.Info.Tags != "" {
// 				tags := strings.Split(template.Info.Tags, ",")
// 				tagsDict[d.Name()] = tags
// 			}
// 		}
// 		return nil
// 	})
// 	return tagsDict
// }
