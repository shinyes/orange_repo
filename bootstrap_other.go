//go:build !linux

package main

// 非 Linux 平台（本地开发等）没有容器属主问题，无需引导。
func bootstrapDataDir(string) error { return nil }
