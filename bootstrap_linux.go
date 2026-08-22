//go:build linux

package main

import (
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"syscall"
)

// 容器内的目标运行身份（distroless nonroot 用户）。
const (
	containerUID = 65532
	containerGID = 65532
)

// bootstrapDataDir 解决「宿主机目录绑定挂载到 /app/data 时 uid 65532 无权写入」的问题：
//
// 镜像默认以 root 启动 → 这里把数据目录整体属主修正为 65532:65532，
// 然后立刻降权（setgroups/setgid/setuid），保证真正跑服务的进程永远不是 root。
//
// 非 root 启动（自定义 --user 或非 Linux）时是空操作；任何一步失败都直接报错退出，绝不以 root 继续运行。
func bootstrapDataDir(dataDir string) error {
	if os.Geteuid() != 0 {
		return nil
	}

	abs, err := filepath.Abs(dataDir)
	if err != nil {
		return fmt.Errorf("解析数据目录 %q 失败: %w", dataDir, err)
	}
	log.Printf("[BOOTSTRAP] 检测到以 root 启动：修正数据目录 %s 属主为 %d:%d 后降权运行", abs, containerUID, containerGID)

	if err := os.MkdirAll(abs, 0o755); err != nil {
		return fmt.Errorf("创建数据目录失败（挂载是否为只读？）: %w", err)
	}
	// Lchown：不跟随符号链接，避免越界改到挂载点之外的文件。
	err = filepath.WalkDir(abs, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return os.Lchown(path, containerUID, containerGID)
	})
	if err != nil {
		return fmt.Errorf("修正数据目录属主失败（宿主机目录是否允许 chown？可改用命名卷或手动 sudo chown -R %d:%d）: %w",
			containerUID, containerGID, err)
	}

	if err := syscall.Setgroups([]int{containerGID}); err != nil {
		return fmt.Errorf("清除附加组失败: %w", err)
	}
	if err := syscall.Setgid(containerGID); err != nil {
		return fmt.Errorf("设置 gid %d 失败: %w", containerGID, err)
	}
	if err := syscall.Setuid(containerUID); err != nil {
		return fmt.Errorf("降权到 uid %d 失败: %w", containerUID, err)
	}
	if os.Geteuid() != containerUID || os.Getegid() != containerGID {
		return fmt.Errorf("降权校验失败：当前 euid=%d egid=%d", os.Geteuid(), os.Getegid())
	}
	log.Printf("[BOOTSTRAP] 已降权至 %d:%d，继续启动。", containerUID, containerGID)
	return nil
}
