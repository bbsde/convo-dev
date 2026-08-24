//go:build windows

package main

import (
	"golang.org/x/sys/windows/registry"
	"net/url"
	"strings"
)

// sysProxyURL 读 Windows 系统代理（HKCU Internet Settings；Clash 等的"系统代理"模式写这里）。
// 返回代理 URL（如 http://127.0.0.1:7890）；未启用/读不到返回空。
func sysProxyURL() string {
	k, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Internet Settings`, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	enable, _, err := k.GetIntegerValue("ProxyEnable")
	if err != nil || enable != 1 {
		return ""
	}
	srv, _, err := k.GetStringValue("ProxyServer")
	if err != nil || srv == "" {
		return ""
	}
	// 分号格式（http=…;https=…;ftp=…）：取 https 项，缺则第一项；否则整体即 host[:port]。
	if strings.Contains(srv, "=") {
		parts := strings.Split(srv, ";")
		srv = strings.TrimPrefix(parts[0], "http=")
		for _, p := range parts {
			if strings.HasPrefix(p, "https=") {
				srv = strings.TrimPrefix(p, "https=")
				break
			}
		}
	}
	if !strings.Contains(srv, "://") {
		srv = "http://" + srv
	}
	if u, err := url.Parse(srv); err != nil || u.Host == "" {
		return ""
	}
	return srv
}
