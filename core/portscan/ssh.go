package portscan

import (
	"context"
	"fmt"
	"net"
	"slack-wails/lib/gologger"
	"slack-wails/lib/structs"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/crypto/ssh"
)

func SshScan(ctx context.Context, host string, usernames, passwords []string) {
	for _, user := range usernames {
		for _, pass := range passwords {
			if ExitBruteFunc {
				return
			}
			pass = strings.Replace(pass, "{user}", user, -1)
			flag, err := SshConn(host, user, pass)
			if flag && err == nil {
				runtime.EventsEmit(ctx, "nucleiResult", structs.VulnerabilityInfo{
					ID:       "ssh weak password",
					Name:     "ssh weak password",
					URL:      host,
					Type:     "SSH",
					Severity: "HIGH",
					Extract:  user + "/" + pass,
				})
			} else {
				gologger.Info(ctx, fmt.Sprintf("ssh://%s %s:%s is login failed", host, user, pass))
			}
		}
	}
}

func SshConn(host, user, pass string) (flag bool, err error) {
	flag = false
	config := &ssh.ClientConfig{
		User:    user,
		Auth:    []ssh.AuthMethod{ssh.Password(pass)},
		Timeout: 10 * time.Second,
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			return nil
		},
	}
	client, err := ssh.Dial("tcp", host, config)
	if err == nil {
		defer client.Close()
		session, err := client.NewSession()
		if err == nil {
			defer session.Close()
			flag = true
		}
	}
	return flag, err
}
