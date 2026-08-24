//go:build !windows

package main

// 非 Windows 平台无系统代理注册表，仅环境变量代理（Go 默认行为已覆盖）。
func sysProxyURL() string { return "" }
