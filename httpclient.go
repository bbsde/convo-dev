// httpclient.go — 共享 HTTP client：代理感知（环境变量优先，Windows 系统代理兜底）。
package main

import (
	"net/http"
	"net/url"
	"time"
)

// newHTTPClient 代理优先级：HTTPS_PROXY 等环境变量（Go 默认行为）> Windows 系统代理。
// Go net/http 不读 Windows 系统代理（注册表 Internet Settings）——Clash 等开"系统代理"
// 模式时环境变量并不存在，CLI 直连 GitHub 必超时（实案），此处补齐。
func newHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy: func(r *http.Request) (*url.URL, error) {
				if u, err := http.ProxyFromEnvironment(r); err == nil && u != nil {
					return u, nil
				}
				if sp := sysProxyURL(); sp != "" {
					return url.Parse(sp)
				}
				return nil, nil
			},
		},
	}
}
