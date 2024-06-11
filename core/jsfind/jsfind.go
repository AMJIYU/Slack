package jsfind

import (
	"context"
	"regexp"
	"slack-wails/lib/clients"
	"slack-wails/lib/gologger"
	"slack-wails/lib/util"
	"strings"
	"sync"
)

var (
	regJS     []*regexp.Regexp
	regFilter []*regexp.Regexp
	JsLink    = []string{
		"(https{0,1}:[-a-zA-Z0-9（）@:%_\\+.~#?&//=]{2,250}?[-a-zA-Z0-9（）@:%_\\+.~#?&//=]{3}[.]js)",
		"[\"'‘“`]\\s{0,6}(/{0,1}[-a-zA-Z0-9（）@:%_\\+.~#?&//=]{2,250}?[-a-zA-Z0-9（）@:%_\\+.~#?&//=]{3}[.]js)",
		"=\\s{0,6}[\",',’,”]{0,1}\\s{0,6}(/{0,1}[-a-zA-Z0-9（）@:%_\\+.~#?&//=]{2,250}?[-a-zA-Z0-9（）@:%_\\+.~#?&//=]{3}[.]js)",
	}
	// UrlFind = []string{
	// 	"[\"'‘“`]\\s{0,6}(https{0,1}:[-a-zA-Z0-9()@:%_\\+.~#?&//={}]{2,250}?)\\s{0,6}[\"'‘“`]",
	// 	"=\\s{0,6}(https{0,1}:[-a-zA-Z0-9()@:%_\\+.~#?&//={}]{2,250})",
	// 	"[\"'‘“`]\\s{0,6}([#,.]{0,2}/[-a-zA-Z0-9()@:%_\\+.~#?&//={}]{2,250}?)\\s{0,6}[\"'‘“`]",
	// 	"\"([-a-zA-Z0-9()@:%_\\+.~#?&//={}]+?[/]{1}[-a-zA-Z0-9()@:%_\\+.~#?&//={}]+?)\"",
	// 	"href\\s{0,6}=\\s{0,6}[\"'‘“`]{0,1}\\s{0,6}([-a-zA-Z0-9()@:%_\\+.~#?&//={}]{2,250})|action\\s{0,6}=\\s{0,6}[\"'‘“`]{0,1}\\s{0,6}([-a-zA-Z0-9()@:%_\\+.~#?&//={}]{2,250})",
	// }
	SensitiveField = []string{
		// sensitive-filed
		`((\[)?('|")?([\w]{0,10})((key)|(secret)|(token)|(config)|(auth)|(access)|(admin))([\w]{0,10})('|")?(\])?( |)(:|=)( |)('|")(.*?)('|")(|,))`,
		// username-filed
		`((|'|")(|[\w]{1,10})(([u](ser|name|sername))|(account)|((creat|updat)(|ed|or|er)(|by|on|at)))(|[\w]{1,10})(|'|")(:|=)( |)('|")(.*?)('|")(|,))`,
		// password-filed
		`((|'|")(|[\w]{1,10})([p](ass|wd|asswd|assword))(|[\w]{1,10})(|'|")(:|=)( |)('|")(.*?)('|")(|,))`,
	}
	Filter = []string{".vue", ".jpeg", ".png", ".jpg", ".ts", ".gif", ".css", ".svg", ".scss"}
)

type InfoSource struct {
	Filed  string
	Source string
}

type FindSomething struct {
	JS             []InfoSource
	APIRoute       []InfoSource
	IP_URL         []InfoSource
	ChineseIDCard  []InfoSource
	ChinesePhone   []InfoSource
	SensitiveField []InfoSource
}

func init() {
	for _, reg := range JsLink {
		regJS = append(regJS, regexp.MustCompile(reg))
	}
	for _, f := range Filter {
		regFilter = append(regFilter, regexp.MustCompile(f))
	}
}

func ExtractJS(ctx context.Context, url string) (allJS []string) {
	_, body, err := clients.NewSimpleGetRequest(url, clients.DefaultClient())
	if err != nil || body == nil {
		gologger.Debug(ctx, err)
		return
	}
	content := string(body)
	for _, reg := range regJS {
		for _, item := range reg.FindAllString(content, -1) {
			item = strings.TrimLeft(item, "=")
			item = strings.Trim(item, "\"")
			item = strings.TrimLeft(item, ".")
			if item[0:4] != "http" {
				allJS = append(allJS, item)
			}
		}
	}
	return util.RemoveDuplicates(allJS)
}

// setp 0 first need deep js
func FindInfo(ctx context.Context, url string, limiter chan bool, wg *sync.WaitGroup) *FindSomething {
	defer wg.Done()
	var fs FindSomething
	_, body, err := clients.NewSimpleGetRequest(url, clients.DefaultClient())
	if err != nil || body == nil {
		gologger.Debug(ctx, err)
		return &fs
	} else {
		content := string(body)
		// 先匹配其他信息
		urls, apis, js := urlInfoSeparate(util.RegLink.FindAllString(content, -1))
		fs.JS = *AppendSource(url, js)
		fs.APIRoute = *AppendSource(url, apis)
		fs.IP_URL = *AppendSource(url, append(util.RegIP_PORT.FindAllString(content, -1), urls...))
		fs.ChineseIDCard = *AppendSource(url, util.RegIDCard.FindAllString(content, -1))
		fs.ChinesePhone = *AppendSource(url, util.RegPhone.FindAllString(content, -1))
		for _, reg := range SensitiveField {
			regSen := regexp.MustCompile(reg)
			for _, item := range regSen.FindAllString(content, -1) {
				fs.SensitiveField = append(fs.SensitiveField, InfoSource{Filed: item, Source: url})
			}
		}
	}
	<-limiter
	return &fs
}

func AppendSource(source string, filed []string) *[]InfoSource {
	is := []InfoSource{}
	for _, f := range filed {
		is = append(is, InfoSource{Filed: f, Source: source})
	}
	return &is
}

func RemoveDuplicatesInfoSource(iss []InfoSource) []InfoSource {
	encountered := map[string]bool{}
	result := []InfoSource{}
	for _, is := range iss {
		if !encountered[is.Filed] {
			encountered[is.Filed] = true
			result = append(result, is)
		}
	}
	return result
}

func urlInfoSeparate(links []string) (urls, apis, js []string) {
	for _, link := range links {
		link = strings.Trim(link, "\"")
		link = strings.Trim(link, "'")
		if strings.HasPrefix(link, "http") || strings.HasPrefix(link, "ws") {
			urls = append(urls, link)
		} else {
			matched := false
			for _, r := range regFilter {
				if strings.Contains(link, ".js") {
					js = append(js, link)
					matched = true
					break
				}
				if r.MatchString(link) {
					matched = true // 匹配到过滤器后缀需要屏蔽
					break
				}
			}
			if !matched {
				apis = append(apis, link)
			}
		}
	}
	return urls, apis, js
}

func FilterExt(iss []InfoSource) (news []InfoSource) {
	for _, link := range iss {
		matched := false
		for _, r := range regFilter {
			if r.MatchString(link.Filed) {
				matched = true
				break
			}
		}
		if !matched {
			news = append(news, link)
		}

	}
	return news
}
