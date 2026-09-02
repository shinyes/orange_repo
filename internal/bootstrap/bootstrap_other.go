//go:build !linux

package bootstrap

// 非 Linux 平台（本地开发等）没有容器属主问题，无需引导。
func DataDir(string) error { return nil }