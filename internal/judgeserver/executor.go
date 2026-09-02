// Package judgeserver 实现 judge-runtime：真正执行用户代码的评测服务。
//
// 复刻上游 OrangeOJ backend/internal/judgeserver/executor.go 的评测语义
// （来源: https://github.com/shinyes/OrangeOJ），仅保留 Python 与 C++ 两个语言。
//
// 沙箱后端按平台拆分（backend_windows.go / backend_linux.go）：
//   - Linux（生产）：nsjail + cgroup v2，完整复刻上游 runInSandbox 参数；
//   - Windows 等非 Linux（开发）：进程级受限运行（无安全隔离承诺，仅供本地调试）。
//
// 评测语义（与上游一致）：
//   - 编译/运行产物放在独立任务临时目录，结束即删；
//   - 每个测试点独立进程运行，首败即停（TLE/MLE/RE/WA 判定顺序与上游相同）；
//   - run（checkAnswer=false）→ 全用例 OK；test/submit 全过 → AC(score 100)。
package judgeserver

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"orangerepo/internal/judge"
)

// sandboxResult 单次进程运行结果。
type sandboxResult struct {
	stdout     string
	stderr     string
	durationMS int
	exitCode   int
	timedOut   bool
}

// sandboxBackend 平台沙箱后端。
type sandboxBackend interface {
	// Run 在 jobDir 中以受限方式执行 argv，输入 stdin，限时 timeLimitMS / 限内存 memoryLimitMiB。
	Run(ctx context.Context, jobDir string, argv []string, stdin string, memoryLimitMiB, timeLimitMS int) (sandboxResult, error)
	// Toolchains 返回按名称解析出的工具链绝对路径（找不到的条目为空串）。
	Toolchains() map[string]string
	// Describe 后端名称（日志用）。
	Describe() string
}

// Executor 评测执行器。
type Executor struct {
	workRoot       string
	compileTimeout time.Duration
	backend        sandboxBackend
	toolchains     map[string]string
}

// NewExecutor 构造执行器：workRoot 空时用默认，compileTimeout<=0 用 10s；
// 后端按平台自动选择（newBackend 由 backend_*.go 提供）。
func NewExecutor(workRoot string, compileTimeout time.Duration) (*Executor, error) {
	root := strings.TrimSpace(workRoot)
	if root == "" {
		root = "/work/jobs"
	}
	if compileTimeout <= 0 {
		compileTimeout = 10 * time.Second
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	backend, err := newBackend(root)
	if err != nil {
		return nil, err
	}
	return &Executor{
		workRoot:       root,
		compileTimeout: compileTimeout,
		backend:        backend,
		toolchains:     backend.Toolchains(),
	}, nil
}

// ToolchainMissing 报告名为 name 的工具链（g++/python3）是否缺失。
func (e *Executor) ToolchainMissing(name string) bool {
	return strings.TrimSpace(e.toolchains[name]) == ""
}

// ToolchainPaths 工具链解析结果（name → 路径），供健康检查/日志。
func (e *Executor) ToolchainPaths() map[string]string {
	out := make(map[string]string, len(e.toolchains))
	for k, v := range e.toolchains {
		out[k] = v
	}
	return out
}

// Execute 执行一次评测任务并给出 verdict 与用例明细。
func (e *Executor) Execute(ctx context.Context, task judge.JudgeTask) (judge.RunResult, error) {
	if strings.TrimSpace(task.Language) == "" {
		return judge.RunResult{}, fmt.Errorf("language is required")
	}
	if strings.TrimSpace(task.SourceCode) == "" {
		return judge.RunResult{}, fmt.Errorf("sourceCode is required")
	}
	if len(task.Cases) == 0 {
		return judge.RunResult{}, fmt.Errorf("cases are required")
	}
	if task.TimeLimitMS <= 0 {
		task.TimeLimitMS = 1000
	}
	if task.MemoryLimitMiB <= 0 {
		task.MemoryLimitMiB = 256
	}
	if task.MemoryLimitMiB < 32 {
		task.MemoryLimitMiB = 32
	}
	if task.CompileTimeoutS <= 0 {
		task.CompileTimeoutS = int(e.compileTimeout.Seconds())
	}
	if task.CompileTimeoutS <= 0 {
		task.CompileTimeoutS = 10
	}

	jobDir, err := os.MkdirTemp(e.workRoot, fmt.Sprintf("sub-%d-", task.SubmissionID))
	if err != nil {
		return judge.RunResult{}, err
	}
	defer os.RemoveAll(jobDir)

	sourceFile, compileCmd, runCmd, err := e.buildCommands(task.Language)
	if err != nil {
		return judge.RunResult{Verdict: judge.VerdictRE, Stderr: judge.TrimTo(err.Error(), 4000)}, nil
	}
	if err := os.WriteFile(filepath.Join(jobDir, sourceFile), []byte(task.SourceCode), 0o600); err != nil {
		return judge.RunResult{}, err
	}

	// ---------- 编译阶段（python 无编译） ----------
	if compileCmd != nil {
		compileCtx, cancel := context.WithTimeout(ctx, time.Duration(task.CompileTimeoutS)*time.Second)
		compileResult, runErr := e.backend.Run(compileCtx, jobDir, compileCmd, "", task.MemoryLimitMiB, task.CompileTimeoutS*1000)
		cancel()
		if runErr != nil {
			return judge.RunResult{}, runErr
		}
		if compileResult.timedOut {
			return judge.RunResult{
				Verdict:   judge.VerdictCE,
				Stderr:    judge.TrimTo(compileResult.stderr+"\nCompile timeout exceeded", 8000),
				MemoryKiB: task.MemoryLimitMiB * 1024,
			}, nil
		}
		if compileResult.exitCode != 0 {
			return judge.RunResult{
				Verdict:   judge.VerdictCE,
				Stdout:    judge.TrimTo(compileResult.stdout, 8000),
				Stderr:    judge.TrimTo(compileResult.stderr, 8000),
				MemoryKiB: task.MemoryLimitMiB * 1024,
			}, nil
		}
	}

	// ---------- 逐用例运行 ----------
	verdict := judge.VerdictAC
	if !task.CheckAnswer {
		verdict = judge.VerdictOK
	}
	maxTimeMS := 0
	stdoutBuilder := strings.Builder{}
	stderrBuilder := strings.Builder{}
	caseResults := make([]judge.CaseResult, 0, len(task.Cases))

	for i, tc := range task.Cases {
		caseCtx, cancel := context.WithTimeout(ctx, time.Duration(task.TimeLimitMS+250)*time.Millisecond)
		runResult, runErr := e.backend.Run(caseCtx, jobDir, runCmd, tc.Input, task.MemoryLimitMiB, task.TimeLimitMS)
		cancel()
		if runErr != nil {
			return judge.RunResult{}, runErr
		}
		if runResult.durationMS > maxTimeMS {
			maxTimeMS = runResult.durationMS
		}
		appendCaseOutput(&stdoutBuilder, &stderrBuilder, i, runResult)

		// 判定优先级：TLE → MLE → RE → WA（与上游完全一致；break = 首败即停）
		caseVerdict, caseError, done := classifyCase(task, tc, runResult, i, &stderrBuilder, &maxTimeMS)
		caseResults = append(caseResults, judge.CaseResult{
			CaseNo:         i + 1,
			Verdict:        caseVerdict,
			Input:          tc.Input,
			Output:         judge.TrimTo(runResult.stdout, 8000),
			ExpectedOutput: tc.Expected,
			Error:          judge.TrimTo(caseError, 8000),
			TimeMS:         runResult.durationMS,
			MemoryKiB:      task.MemoryLimitMiB * 1024,
		})
		if caseVerdict != judge.VerdictAC && caseVerdict != judge.VerdictOK {
			verdict = caseVerdict
		}
		if done {
			break
		}
	}

	return judge.RunResult{
		Verdict:     verdict,
		Stdout:      judge.TrimTo(stdoutBuilder.String(), judge.MaxTotalText),
		Stderr:      judge.TrimTo(stderrBuilder.String(), judge.MaxTotalText),
		TimeMS:      maxTimeMS,
		MemoryKiB:   task.MemoryLimitMiB * 1024,
		CaseResults: caseResults,
	}, nil
}

// classifyCase 判定单个用例结果；返回 caseVerdict、caseError、done（是否应停止后续用例）。
func classifyCase(task judge.JudgeTask, tc judge.JudgeCase, r sandboxResult, i int, stderrB *strings.Builder, maxTimeMS *int) (judge.Verdict, string, bool) {
	if !task.CheckAnswer {
		// run 模式只执行不比对
		return judge.VerdictOK, r.stderr, r.timedOut || r.exitCode != 0
	}
	switch {
	case r.timedOut:
		if task.TimeLimitMS > *maxTimeMS {
			*maxTimeMS = task.TimeLimitMS
		}
		stderrB.WriteString(fmt.Sprintf("[case %d stderr]\nTime limit exceeded\n", i+1))
		return judge.VerdictTLE, r.stderr + "\nTime limit exceeded", true
	case isMemoryExceeded(r.exitCode, r.stderr):
		return judge.VerdictMLE, r.stderr, true
	case r.exitCode != 0:
		return judge.VerdictRE, r.stderr, true
	case judge.NormalizeOutput(r.stdout) != judge.NormalizeOutput(tc.Expected):
		stderrB.WriteString(fmt.Sprintf("[case %d] expected:\n%s\n", i+1, tc.Expected))
		return judge.VerdictWA, r.stderr + "\nExpected output:\n" + tc.Expected, true
	default:
		return judge.VerdictAC, r.stderr, false
	}
}

// appendCaseOutput 汇总各用例 stdout/stderr 到总控制台文本。
func appendCaseOutput(stdoutB, stderrB *strings.Builder, i int, r sandboxResult) {
	stdoutB.WriteString(fmt.Sprintf("[case %d stdout]\n%s\n", i+1, judge.TrimTo(r.stdout, 8000)))
	if r.stderr != "" {
		stderrB.WriteString(fmt.Sprintf("[case %d stderr]\n%s\n", i+1, judge.TrimTo(r.stderr, 8000)))
	}
}

// buildCommands 按语言生成 源文件名/编译 argv/运行 argv。
func (e *Executor) buildCommands(language string) (sourceFile string, compileCmd, runCmd []string, err error) {
	switch language {
	case "cpp", "c++":
		gpp := e.toolchains["g++"]
		if gpp == "" {
			return "", nil, nil, fmt.Errorf("C++ 编译器 g++ 不可用（生产环境请使用 judge-runtime 容器）")
		}
		return "main.cpp",
			[]string{gpp, "-std=c++11", "-O2", "main.cpp", "-o", "main.out"},
			[]string{filepath.Join(".", "main.out")}, nil
	case "python", "python3", "py":
		py := e.toolchains["python"]
		if py == "" {
			return "", nil, nil, fmt.Errorf("Python 解释器不可用")
		}
		return "main.py", nil, []string{py, "main.py"}, nil
	default:
		return "", nil, nil, fmt.Errorf("unsupported language: %s", language)
	}
}

// isMemoryExceeded 兼容 nsjail/cgroup 的 OOM 信号（退出码 137 或 stderr 关键字）。
func isMemoryExceeded(exitCode int, stderr string) bool {
	normalizedErr := strings.ToLower(stderr)
	return exitCode == 137 || strings.Contains(normalizedErr, "out of memory") || strings.Contains(normalizedErr, "killed")
}

// resolveExecutablePath 依序解析候选可执行文件（绝对路径优先，其次 PATH）。
func resolveExecutablePath(candidates ...string) string {
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if strings.ContainsAny(candidate, `/\`) {
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
			continue
		}
		if resolved, err := exec.LookPath(candidate); err == nil {
			return resolved
		}
	}
	return ""
}

// parseExitCode 提取 exec 失败时的退出码（-1 表示无法启动）。
func parseExitCode(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}
