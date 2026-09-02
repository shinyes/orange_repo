// judge-runtime — OrangeOJ 判题沙箱运行器（独立进程 :9090）。
//
// 真正执行用户代码的唯一进程：仅支持 Python 与 C++。
// 生产（Linux）经 nsjail + cgroup v2 隔离；Windows/开发机降级为进程级受限运行（无安全承诺）。
// 配置全部走环境变量（与上游 cmd/judge-runtime/main.go 同构）。
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"orangerepo/internal/judgeserver"
)

func main() {
	cfg := judgeserver.Config{
		Port:              envOr("ORANGEOJ_JUDGE_RUNTIME_PORT", "9090"),
		SharedToken:       strings.TrimSpace(os.Getenv("ORANGEOJ_JUDGE_SHARED_TOKEN")),
		WorkDir:           envOr("ORANGEOJ_JUDGE_WORKDIR", "/work/jobs"),
		CompileTimeoutSec: envIntOr("ORANGEOJ_JUDGE_COMPILE_TIMEOUT_SEC", 10),
		ReadTimeoutSec:    envIntOr("ORANGEOJ_JUDGE_READ_TIMEOUT_SEC", 15),
		WriteTimeoutSec:   envIntOr("ORANGEOJ_JUDGE_WRITE_TIMEOUT_SEC", 300),
	}

	if cfg.SharedToken == "" {
		log.Fatal("ORANGEOJ_JUDGE_SHARED_TOKEN is required")
	}

	server, err := judgeserver.NewServer(cfg)
	if err != nil {
		log.Fatalf("init judge runtime failed: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Start()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		if err != nil {
			log.Fatalf("judge runtime stopped: %v", err)
		}
	case <-sigCh:
		log.Println("judge runtime shutting down...")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("judge runtime shutdown error: %v", err)
	}
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envIntOr(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return parsed
}
