// Package judge 实现判题编排层（复刻上游 OrangeOJ backend/internal/judge：
// queue.go / runner.go 的语义与 SQL 结构，来源: https://github.com/shinyes/OrangeOJ）。
//
// 职责边界：
//   - QueueService：judge_jobs/submissions 的状态机（认领/写回/失败兜底）与进度 upsert，
//     仅操作调用方传入的 *sql.DB（quiz.db）——表结构见 internal/quizstore 迁移；
//   - Runner：向独立 judge-runtime 发起评测的 HTTP 客户端（Runner 接口便于测试注入 mock）。
package judge

import (
	"context"
	"strings"
)

// SubmitType 提交动作类型（与上游 model.SubmitType 一致）。
type SubmitType string

const (
	SubmitTypeRun       SubmitType = "run"
	SubmitTypeTest      SubmitType = "test"
	SubmitTypeSubmit    SubmitType = "submit"
	SubmitTypeObjective SubmitType = "objective"
)

// SubmissionStatus 提交状态（与上游一致）。
type SubmissionStatus string

const (
	SubmissionStatusQueued  SubmissionStatus = "queued"
	SubmissionStatusRunning SubmissionStatus = "running"
	SubmissionStatusDone    SubmissionStatus = "done"
	SubmissionStatusFailed  SubmissionStatus = "failed"
)

// Verdict 评测结论（与上游 model.Verdict 一致）。
type Verdict string

const (
	VerdictPending Verdict = "PENDING"
	VerdictOK      Verdict = "OK"
	VerdictAC      Verdict = "AC"
	VerdictWA      Verdict = "WA"
	VerdictCE      Verdict = "CE"
	VerdictRE      Verdict = "RE"
	VerdictTLE     Verdict = "TLE"
	VerdictMLE     Verdict = "MLE"
)

// IsAccepted 报告 verdict 是否属于“通过”类（AC/OK）。
func IsAccepted(v Verdict) bool { return v == VerdictAC || v == VerdictOK }

// JudgeCase 单个测试点：输入与期望输出。
type JudgeCase struct {
	Input    string `json:"input"`
	Expected string `json:"expected"`
}

// JudgeTask 判题任务（发送给 judge-runtime 的载荷，字段与上游一致）。
type JudgeTask struct {
	SubmissionID    int64       `json:"submissionId"`
	Language        string      `json:"language"`
	SourceCode      string      `json:"sourceCode"`
	TimeLimitMS     int         `json:"timeLimitMs"`
	MemoryLimitMiB  int         `json:"memoryLimitMiB"`
	CheckAnswer     bool        `json:"checkAnswer"`
	CompileTimeoutS int         `json:"compileTimeoutS"`
	Cases           []JudgeCase `json:"cases"`
}

// RunResult 单次评测结果。
type RunResult struct {
	Verdict     Verdict      `json:"verdict"`
	Stdout      string       `json:"stdout"`
	Stderr      string       `json:"stderr"`
	TimeMS      int          `json:"timeMs"`
	MemoryKiB   int          `json:"memoryKiB"`
	CaseResults []CaseResult `json:"caseResults,omitempty"`
}

// CaseResult 单个测试点结果。
type CaseResult struct {
	CaseNo         int     `json:"caseNo"`
	Verdict        Verdict `json:"verdict"`
	Input          string  `json:"input"`
	Output         string  `json:"output"`
	ExpectedOutput string  `json:"expectedOutput"`
	Error          string  `json:"error"`
	TimeMS         int     `json:"timeMs"`
	MemoryKiB      int     `json:"memoryKiB"`
}

// Runner 执行判题任务的抽象（HTTPRunner 为生产实现，测试可注入 fake）。
type Runner interface {
	Judge(ctx context.Context, task JudgeTask) (RunResult, error)
}

// RuntimeSubmission 队列处理所需的提交+题目运行时数据。
// 跨库组装（quiz.db submissions + 主库 problems）由存储层（SubmissionLoader 实现者）负责。
type RuntimeSubmission struct {
	ID             int64
	UserID         int64
	ProblemID      int64
	SubmitType     SubmitType
	Language       string
	SourceCode     string
	InputData      string
	TimeLimitMS    int
	MemoryLimitMiB int
	BodyJSON       string
}

// SubmissionLoader 装载运行时提交数据（由 quizstore 实现）。
type SubmissionLoader interface {
	LoadSubmission(ctx context.Context, submissionID int64) (*RuntimeSubmission, error)
}

// ProgrammingBody 编程题 bodyJson 形状（与上游 queue.go programmingBody 一致）。
type ProgrammingBody struct {
	InputFormat  string            `json:"inputFormat"`
	OutputFormat string            `json:"outputFormat"`
	Samples      []ProgrammingCase `json:"samples"`
	TestCases    []ProgrammingCase `json:"testCases"`
}

// ProgrammingCase 编程题测试点/样例。
type ProgrammingCase struct {
	Input  string `json:"input"`
	Output string `json:"output"`
}

// SelectCases 按 submitType 选择评测用例（语义照搬上游 queue.go processJob）：
//
//	run     → 仅 inputData 单样例（checkAnswer=false）
//	test    → body.testCases；空则回退 body.samples；再空则 inputData
//	submit  → body.testCases；空则回退 body.samples
//	空兜底   → 一个空输入样例
func SelectCases(submitType SubmitType, body ProgrammingBody, inputData string) []JudgeCase {
	var selected []ProgrammingCase
	switch submitType {
	case SubmitTypeRun:
		selected = []ProgrammingCase{{Input: inputData}}
	case SubmitTypeTest:
		selected = body.TestCases
		if len(selected) == 0 {
			selected = body.Samples
		}
		if len(selected) == 0 {
			selected = []ProgrammingCase{{Input: inputData}}
		}
	case SubmitTypeSubmit:
		selected = body.TestCases
		if len(selected) == 0 {
			selected = body.Samples
		}
	default:
		selected = []ProgrammingCase{{Input: inputData}}
	}
	if len(selected) == 0 {
		selected = []ProgrammingCase{{Input: "", Output: ""}}
	}
	cases := make([]JudgeCase, 0, len(selected))
	for _, item := range selected {
		cases = append(cases, JudgeCase{Input: item.Input, Expected: item.Output})
	}
	return cases
}

// NormalizeOutput 输出归一化：\r\n→\n、逐行去行尾空白、整体 TrimSpace
// （与上游 judge/runner.go NormalizeOutput 一致）。
func NormalizeOutput(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// 文本截断上限（与上游 executor.go/queue.go 一致）。
const (
	MaxStderrPerCase = 8000
	MaxTotalText     = 12000
	MaxFailStderr    = 4000
)

// TrimTo 超长截断（上游 trimTo 语义）。
func TrimTo(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n...[truncated]"
}
