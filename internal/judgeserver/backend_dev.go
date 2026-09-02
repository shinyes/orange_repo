//go:build !linux

// 非 Linux 开发沙箱后端（Windows 本地调试用）：
// 无 nsjail/cgroup，进程级受限运行——工作目录隔离、逐用例超时杀进程树、
// 精简环境变量、stdout/stderr 截断。
//
// ⚠ 安全声明：本后端不做安全隔离承诺，仅供开发联调；
// 生产评测必须运行在 Linux judge-runtime 容器（nsjail 后端）。
package judgeserver

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"orangerepo/internal/judge"
)

// devBackend 开发受限运行后端。
type devBackend struct {
	workRoot   string
	toolchains map[string]string
	// extraPathDirs 工具链所在目录（MinGW exe 运行需其 bin 下 DLL）。
	extraPathDirs string
}

// newBackend 非 Linux 平台构造开发后端。
func newBackend(workRoot string) (sandboxBackend, error) {
	b := &devBackend{workRoot: workRoot}
	b.toolchains = b.resolveToolchains()
	dirs := map[string]bool{}
	for _, p := range b.toolchains {
		if p == "" {
			continue
		}
		dirs[filepath.Dir(p)] = true
	}
	parts := make([]string, 0, len(dirs))
	for d := range dirs {
		parts = append(parts, d)
	}
	b.extraPathDirs = strings.Join(parts, string(os.PathListSeparator))
	return b, nil
}

func (b *devBackend) Describe() string { return "dev-process (no nsjail; dev only)" }

func (b *devBackend) Toolchains() map[string]string { return b.toolchains }

// resolveToolchains 解析 g++/python：PATH + 常见安装位点。
func (b *devBackend) resolveToolchains() map[string]string {
	// 注意：Windows 的 python3.exe 常指向 WindowsApps 商店占位 stub（运行即 9009），
	// 一律过滤；优先真解释器 python。
	py := ""
	if runtime.GOOS == "windows" {
		py = resolveRealExecutable("python", "C:\\Python314\\python.exe", "C:\\Python313\\python.exe",
			"C:\\Python312\\python.exe", "C:\\Python311\\python.exe", "C:\\Python310\\python.exe")
	} else {
		py = resolveExecutablePath("python3", "python")
	}
	gpp := resolveExecutablePath("g++", "c++", "gcc")
	if gpp == "" && runtime.GOOS == "windows" {
		gpp = resolveRealExecutable(
			`D:\tools\mingw\mingw64\bin\g++.exe`, // 本地开发常用安装点
			`C:\msys64\mingw64\bin\g++.exe`,
			`C:\mingw64\bin\g++.exe`,
			`C:\Program Files\mingw-w64\mingw64\bin\g++.exe`,
			`C:\Program Files\LLVM\bin\clang++.exe`,
		)
	}
	return map[string]string{"g++": gpp, "python": py}
}

// resolveRealExecutable 依次解析候选：LookPath 命中但位于 WindowsApps（商店 stub）则跳过。
func resolveRealExecutable(candidates ...string) string {
	for _, candidate := range candidates {
		resolved := ""
		if strings.ContainsAny(candidate, `/\`) {
			if _, err := os.Stat(candidate); err == nil {
				resolved = candidate
			}
		} else if p, err := exec.LookPath(candidate); err == nil {
			resolved = p
		}
		if resolved == "" {
			continue
		}
		if strings.Contains(strings.ToLower(resolved), `windowsapps`) {
			continue // 商店占位符，运行即失败
		}
		return resolved
	}
	return ""
}

// Run 进程级受限运行：cmd.Dir=jobDir、精简 PATH、超时杀整棵进程树。
func (b *devBackend) Run(ctx context.Context, jobDir string, argv []string, stdin string, memoryLimitMiB, timeLimitMS int) (sandboxResult, error) {
	result := sandboxResult{}
	if timeLimitMS <= 0 {
		timeLimitMS = 1000
	}

	// argv[0] 解析：绝对路径直接用；相对 ./xxx 或裸名（如 main.out）先按 jobDir 解析；
	// Windows 下同时尝试补 .exe。
	exe := argv[0]
	isBare := !strings.ContainsAny(exe, `/\`) && !filepath.IsAbs(exe)
	if strings.HasPrefix(exe, "./") || strings.HasPrefix(exe, ".\\") || isBare {
		base := strings.TrimLeft(exe, "./\\")
		candidates := []string{base}
		if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(base), ".exe") {
			candidates = append(candidates, base+".exe")
		}
		for _, cand := range candidates {
			full := filepath.Join(jobDir, cand)
			if _, err := os.Stat(full); err == nil {
				exe = full
				break
			}
		}
		argv = append([]string{exe}, argv[1:]...)
	}

	cmd := exec.CommandContext(ctx, exe, argv[1:]...)
	cmd.Dir = jobDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	// 精简环境：PATH（含工具链目录）+ 语言运行时常用变量（不继承用户环境）
	pathEnv := os.Getenv("PATH")
	if b.extraPathDirs != "" {
		pathEnv = b.extraPathDirs + string(os.PathListSeparator) + pathEnv
	}
	minimalEnv := []string{
		"PATH=" + pathEnv,
		"SystemRoot=" + os.Getenv("SystemRoot"),
		"TEMP=" + os.Getenv("TEMP"),
		"TMP=" + os.Getenv("TMP"),
		"LANG=C",
		"PYTHONIOENCODING=utf-8",
		"PYTHONDONTWRITEBYTECODE=1",
	}
	if runtime.GOOS == "windows" {
		minimalEnv = append(minimalEnv, "PATHEXT="+os.Getenv("PATHEXT"), "ProgramFiles="+os.Getenv("ProgramFiles"))
	}
	cmd.Env = minimalEnv

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
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.exitCode = exitErr.ExitCode()
			return result, nil
		}
		// 启动失败（如 exe 不存在）→ 以 RE 结果返回，避免整次评测中断
		result.exitCode = 127
		result.stderr = judge.TrimTo(err.Error(), 2000)
		return result, nil
	}
	result.exitCode = 0
	return result, nil
}
