// 真实评测冒烟测试：直接驱动 Executor（本机工具链）。
// Windows/开发后端与 Linux nsjail 后端共用同一套语义断言。
package judgeserver

import (
	"context"
	"strings"
	"testing"
	"time"

	"orangerepo/internal/judge"
)

func newTestExecutor(t *testing.T) *Executor {
	t.Helper()
	ex, err := NewExecutor(t.TempDir(), 10*time.Second)
	if err != nil {
		t.Fatalf("NewExecutor: %v", err)
	}
	return ex
}

func pyTask(code string, cases []judge.JudgeCase, check bool) judge.JudgeTask {
	return judge.JudgeTask{
		SubmissionID:    1,
		Language:        "python",
		SourceCode:      code,
		TimeLimitMS:     2000,
		MemoryLimitMiB:  256,
		CheckAnswer:     check,
		CompileTimeoutS: 10,
		Cases:           cases,
	}
}

// TestPythonAC python 两数之和 AC。
func TestPythonAC(t *testing.T) {
	ex := newTestExecutor(t)
	if ex.ToolchainMissing("python") {
		t.Skip("python 不可用")
	}
	res, err := ex.Execute(context.Background(), pyTask(`a, b = map(int, input().split())
print(a + b)`, []judge.JudgeCase{
		{Input: "1 2\n", Expected: "3\n"},
		{Input: "10 -3\n", Expected: "7\n"},
	}, true))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Verdict != judge.VerdictAC {
		t.Fatalf("verdict = %s, want AC; stderr=%s stdout=%s", res.Verdict, res.Stderr, res.Stdout)
	}
	if len(res.CaseResults) != 2 || res.CaseResults[0].Verdict != judge.VerdictAC {
		t.Fatalf("caseResults wrong: %+v", res.CaseResults)
	}
	if res.TimeMS <= 0 {
		t.Fatalf("timeMs not measured: %d", res.TimeMS)
	}
}

// TestPythonWA 输出比对（NormalizeOutput：空白容忍 + 内容错误 → WA）。
func TestPythonWA(t *testing.T) {
	ex := newTestExecutor(t)
	if ex.ToolchainMissing("python") {
		t.Skip("python 不可用")
	}
	res, err := ex.Execute(context.Background(), pyTask(`print("wrong")`, []judge.JudgeCase{
		{Input: "", Expected: "right"},
	}, true))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Verdict != judge.VerdictWA {
		t.Fatalf("verdict = %s, want WA", res.Verdict)
	}
}

// TestPythonOutputNormalize 行尾空白/换行差异不应判 WA。
func TestPythonOutputNormalize(t *testing.T) {
	ex := newTestExecutor(t)
	if ex.ToolchainMissing("python") {
		t.Skip("python 不可用")
	}
	res, err := ex.Execute(context.Background(), pyTask(`print("  hello  ")`, []judge.JudgeCase{
		{Input: "", Expected: "hello"},
	}, true))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Verdict != judge.VerdictAC {
		t.Fatalf("verdict = %s, want AC (行尾空白应容忍): %s", res.Verdict, res.Stderr)
	}
}

// TestPythonRE 运行期异常 → RE。
func TestPythonRE(t *testing.T) {
	ex := newTestExecutor(t)
	if ex.ToolchainMissing("python") {
		t.Skip("python 不可用")
	}
	res, err := ex.Execute(context.Background(), pyTask(`raise RuntimeError("boom")`, []judge.JudgeCase{
		{Input: "", Expected: ""},
	}, true))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Verdict != judge.VerdictRE {
		t.Fatalf("verdict = %s, want RE", res.Verdict)
	}
	if !strings.Contains(res.Stderr, "boom") {
		t.Fatalf("stderr 应含异常信息: %s", res.Stderr)
	}
}

// TestPythonCE python 语法错误 → 直接以运行期 SyntaxError 呈现（解释型无编译阶段）。
// 上游对解释型语言同样把语法错误作为 RE/启动失败处理——这里不做 CE 断言，仅验证不 panic。
func TestPythonSyntaxError(t *testing.T) {
	ex := newTestExecutor(t)
	if ex.ToolchainMissing("python") {
		t.Skip("python 不可用")
	}
	res, err := ex.Execute(context.Background(), pyTask(`def broken(:`, []judge.JudgeCase{
		{Input: "", Expected: ""},
	}, true))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Verdict == judge.VerdictAC || res.Verdict == judge.VerdictOK {
		t.Fatalf("语法错误不应判过: %s", res.Verdict)
	}
}

// TestPythonTLE 死循环 → TLE。
func TestPythonTLE(t *testing.T) {
	ex := newTestExecutor(t)
	if ex.ToolchainMissing("python") {
		t.Skip("python 不可用")
	}
	res, err := ex.Execute(context.Background(), pyTask(`while True:
    pass`, []judge.JudgeCase{{Input: "", Expected: ""}}, true))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Verdict != judge.VerdictTLE {
		t.Fatalf("verdict = %s, want TLE", res.Verdict)
	}
	if res.CaseResults[0].Verdict != judge.VerdictTLE {
		t.Fatalf("case verdict = %s, want TLE", res.CaseResults[0].Verdict)
	}
}

// TestRunModeNoCheckAnswer run 模式：不比对输出，全 OK。
func TestRunModeNoCheckAnswer(t *testing.T) {
	ex := newTestExecutor(t)
	if ex.ToolchainMissing("python") {
		t.Skip("python 不可用")
	}
	res, err := ex.Execute(context.Background(), pyTask(`import sys
print("hello " + sys.stdin.read().strip())`, []judge.JudgeCase{
		{Input: "world"},
	}, false))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Verdict != judge.VerdictOK {
		t.Fatalf("verdict = %s, want OK", res.Verdict)
	}
	if !strings.Contains(res.Stdout, "hello world") {
		t.Fatalf("stdout 应含运行输出: %q", res.Stdout)
	}
}

// ---------- C++ ----------

func cppTask(code string, cases []judge.JudgeCase, check bool) judge.JudgeTask {
	return judge.JudgeTask{
		SubmissionID:    2,
		Language:        "cpp",
		SourceCode:      code,
		TimeLimitMS:     2000,
		MemoryLimitMiB:  256,
		CheckAnswer:     check,
		CompileTimeoutS: 20,
		Cases:           cases,
	}
}

// TestCppAC g++ 真实编译 + 两数之和 AC。
func TestCppAC(t *testing.T) {
	ex := newTestExecutor(t)
	if ex.ToolchainMissing("g++") {
		t.Skip("g++ 不可用")
	}
	code := `#include <iostream>
int main() { long long a, b; std::cin >> a >> b; std::cout << a + b << std::endl; return 0; }`
	res, err := ex.Execute(context.Background(), cppTask(code, []judge.JudgeCase{
		{Input: "1 2\n", Expected: "3\n"},
		{Input: "10000000000 1\n", Expected: "10000000001\n"},
	}, true))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Verdict != judge.VerdictAC {
		t.Fatalf("verdict = %s, want AC; stderr=%s", res.Verdict, res.Stderr)
	}
}

// TestCppCE 编译错误 → CE。
func TestCppCE(t *testing.T) {
	ex := newTestExecutor(t)
	if ex.ToolchainMissing("g++") {
		t.Skip("g++ 不可用")
	}
	res, err := ex.Execute(context.Background(), cppTask(`int main() { this is not c++ }`, []judge.JudgeCase{
		{Input: "", Expected: ""},
	}, true))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Verdict != judge.VerdictCE {
		t.Fatalf("verdict = %s, want CE", res.Verdict)
	}
}

// TestCppRE 运行期崩溃（除零/非法内存）→ RE。
func TestCppRE(t *testing.T) {
	ex := newTestExecutor(t)
	if ex.ToolchainMissing("g++") {
		t.Skip("g++ 不可用")
	}
	code := `#include <vector>
int main() { std::vector<int> v; return v[100]; }`
	res, err := ex.Execute(context.Background(), cppTask(code, []judge.JudgeCase{{Input: "", Expected: ""}}, true))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Verdict != judge.VerdictRE {
		t.Fatalf("verdict = %s, want RE", res.Verdict)
	}
}

// TestCppTLE C++ 死循环 → TLE（编译产物被超时终止）。
func TestCppTLE(t *testing.T) {
	ex := newTestExecutor(t)
	if ex.ToolchainMissing("g++") {
		t.Skip("g++ 不可用")
	}
	code := `int main() { for (;;) {} return 0; }`
	res, err := ex.Execute(context.Background(), cppTask(code, []judge.JudgeCase{{Input: "", Expected: ""}}, true))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Verdict != judge.VerdictTLE {
		t.Fatalf("verdict = %s, want TLE; stderr=%s", res.Verdict, res.Stderr)
	}
}

// TestUnsupportedLanguage go/turtle 等一律拒绝。
func TestUnsupportedLanguage(t *testing.T) {
	ex := newTestExecutor(t)
	res, err := ex.Execute(context.Background(), judge.JudgeTask{
		SubmissionID: 3, Language: "go", SourceCode: "package main", TimeLimitMS: 1000,
		MemoryLimitMiB: 256, CheckAnswer: true, Cases: []judge.JudgeCase{{Input: "", Expected: ""}},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Verdict != judge.VerdictRE {
		t.Fatalf("verdict = %s, want RE（不支持语言）", res.Verdict)
	}
}
