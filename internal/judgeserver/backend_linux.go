//go:build linux

// Linux 生产沙箱后端：nsjail + cgroup v2。
// 完整复刻上游 OrangeOJ backend/internal/judgeserver/executor.go 的 runInSandbox 参数
// （来源: https://github.com/shinyes/OrangeOJ）。
package judgeserver

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"orangerepo/internal/judge"
)

const sandboxDefaultPATH = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

// linuxBackend nsjail 后端。
type linuxBackend struct {
	workRoot string
}

// newBackend Linux 下构造 nsjail 后端（自检通过才返回；生产环境缺 nsjail 视为致命）。
func newBackend(workRoot string) (sandboxBackend, error) {
	if err := checkPrerequisites(); err != nil {
		return nil, err
	}
	return &linuxBackend{workRoot: workRoot}, nil
}

func (b *linuxBackend) Describe() string { return "nsjail (linux)" }

// Toolchains 解析 g++ 与 python3（PATH 优先，其次固定系统路径）。
func (b *linuxBackend) Toolchains() map[string]string {
	return map[string]string{
		"g++":    resolveExecutablePath("g++", "/usr/bin/g++", "c++", "/usr/bin/c++"),
		"python": resolveExecutablePath("python3", "/usr/bin/python3", "python", "/usr/bin/python"),
	}
}

// Run 以 nsjail 执行命令（argv[0] 为绝对路径或 PATH 可解析名称）。
func (b *linuxBackend) Run(ctx context.Context, jobDir string, argv []string, stdin string, memoryLimitMiB, timeLimitMS int) (sandboxResult, error) {
	result := sandboxResult{}
	if memoryLimitMiB <= 0 {
		memoryLimitMiB = 256
	}
	if memoryLimitMiB < 32 {
		memoryLimitMiB = 32
	}
	if timeLimitMS <= 0 {
		timeLimitMS = 1000
	}
	timeLimitSec := int(math.Ceil(float64(timeLimitMS) / 1000.0))
	if timeLimitSec < 1 {
		timeLimitSec = 1
	}
	// nsjail --time_limit 为整秒，到点杀进程的时间点（上游注释原样保留）
	nsjailLimitMS := timeLimitSec * 1000

	// cgroup 内存上限 = 题目限制 + 32MiB 容忍编译器/解释器开销（上游语义）
	cgroupMemoryMiB := memoryLimitMiB + 32
	memoryBytes := int64(cgroupMemoryMiB) * 1024 * 1024

	shell := "/bin/sh"
	if _, err := os.Stat(shell); err != nil {
		if _, bashErr := os.Stat("/bin/bash"); bashErr == nil {
			shell = "/bin/bash"
		}
	}

	cmdLine := strings.Join(argv, " ")
	args := []string{
		"--really_quiet",
		"--mode", "o",
		"--time_limit", strconv.Itoa(timeLimitSec),
		"--disable_proc",
		"--iface_no_lo",
		"--user", "65534",
		"--group", "65534",
		"--chroot", "/",
		"--cwd", "/sandbox",
		"--bindmount", fmt.Sprintf("%s:/sandbox", jobDir),
		"--env", "PATH=" + sandboxDefaultPATH,
		"--use_cgroupv2",
		"--cgroup_mem_max", strconv.FormatInt(memoryBytes, 10),
		"--cgroup_pids_max", "128",
		"--",
		shell, "-lc", cmdLine,
	}

	cmd := exec.CommandContext(ctx, "nsjail", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}

	start := time.Now()
	err := cmd.Run()
	result.durationMS = int(time.Since(start).Milliseconds())
	result.stdout = judge.TrimTo(stdout.String(), 8000)
	result.stderr = judge.TrimTo(stderr.String(), 8000)

	if ctx.Err() == context.DeadlineExceeded {
		result.timedOut = true
		result.durationMS = timeLimitMS
		return result, nil
	}

	// nsjail --time_limit 到点同样 SIGKILL（137 / stderr Killed），与 OOM 被杀无法仅凭
	// 退出码区分；超时被杀必然跑满整个时间上限，OOM 则在此之前（上游注释原样保留）。
	if isTimeoutKill(result, nsjailLimitMS) {
		result.timedOut = true
		result.durationMS = timeLimitMS
		return result, nil
	}

	if err != nil {
		result.exitCode = parseExitCode(err)
		if result.exitCode < 0 {
			return result, fmt.Errorf("start nsjail failed: %w", err)
		}
		return result, nil
	}
	result.exitCode = 0
	return result, nil
}

// isTimeoutKill 超时被杀 vs OOM 被杀：SIGKILL(137)/stderr Killed + 已跑满 nsjailLimitMS。
func isTimeoutKill(result sandboxResult, nsjailLimitMS int) bool {
	if result.exitCode != 137 && !strings.Contains(strings.ToLower(result.stderr), "killed") {
		return false
	}
	return result.durationMS >= nsjailLimitMS
}

// checkPrerequisites 生产前置检查：nsjail + cgroup v2（上游 judgeserver/server.go）。
func checkPrerequisites() error {
	if _, err := exec.LookPath("nsjail"); err != nil {
		return fmt.Errorf("nsjail not found in PATH: %w", err)
	}
	if _, err := os.Stat("/sys/fs/cgroup"); err != nil {
		return fmt.Errorf("cgroup mount missing: %w", err)
	}
	if _, err := os.Stat("/sys/fs/cgroup/cgroup.controllers"); err != nil {
		return fmt.Errorf("cgroup v2 required (/sys/fs/cgroup/cgroup.controllers): %w", err)
	}
	return nil
}
