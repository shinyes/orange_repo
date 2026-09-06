package quizstore_test

import (
	"encoding/json"
	"testing"

	"orangeoj/internal/model"
	"orangeoj/internal/quizstore"
	"orangeoj/internal/store"
)

// newTestEnvironment 建立临时目录：先用主站 store.Open 造题库（含样例题目），
// 关闭后以只读方式打开供刷题侧验证。
func newTestEnvironment(t *testing.T) *quizstore.Store {
	t.Helper()
	dir := t.TempDir()
	main, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open main store: %v", err)
	}
	sampleProblems := []model.Problem{
		{ // 1：单选，仅数学
			Type: model.TypeSingleChoice, Title: "单选A", Tags: []string{"数学"},
			StatementMD: "1+1=?", BodyJSON: json.RawMessage(`{"options":["1","2","3","4"]}`),
			AnswerJSON: json.RawMessage(`{"answerIndex":1}`), Solutions: json.RawMessage(`[]`),
		},
		{ // 2：单选，数学 + 物理/力学
			Type: model.TypeSingleChoice, Title: "单选B", Tags: []string{"数学", "物理/力学"},
			StatementMD: "重力方向?", BodyJSON: json.RawMessage(`{"options":["向上","向下"]}`),
			AnswerJSON: json.RawMessage(`{"answerIndex":1}`), Solutions: json.RawMessage(`[]`),
		},
		{ // 3：判断，物理/力学
			Type: model.TypeTrueFalse, Title: "判断A", Tags: []string{"物理/力学"},
			StatementMD: "自由落体加速度为 g", BodyJSON: json.RawMessage(`{}`),
			AnswerJSON: json.RawMessage(`{"answer":true}`),
			Solutions:   json.RawMessage(`[{"language":"","code":"","markdown":"解析：自由落体加速度约为 9.8 m/s²"}]`),
		},
		{ // 5：编程（判题答案读取应拒绝）
			Type: model.TypeProgramming, Title: "编程题", Tags: []string{"数学"},
			StatementMD: "求两数之和", BodyJSON: json.RawMessage(`{}`),
			AnswerJSON: json.RawMessage(`{}`), Solutions: json.RawMessage(`[]`),
		},
	}
	for _, p := range sampleProblems {
		if _, err := main.CreateProblem(p); err != nil {
			t.Fatalf("seed problem: %v", err)
		}
	}
	if err := main.Close(); err != nil {
		t.Fatalf("close main store: %v", err)
	}
	qs, err := quizstore.Open(dir)
	if err != nil {
		t.Fatalf("open quiz store: %v", err)
	}
	t.Cleanup(func() { _ = qs.Close() })
	return qs
}

// ---------- problems_test ----------

func TestGetAnswer(t *testing.T) {
	qs := newTestEnvironment(t)
	env, err := qs.Repo.GetAnswer(1)
	if err != nil {
		t.Fatal(err)
	}
	if env.Type != "single_choice" || env.AnswerIndex == nil || *env.AnswerIndex != 1 {
		t.Fatalf("题目1 答案 = %+v, want answerIndex=1", env)
	}
	env3, err := qs.Repo.GetAnswer(3)
	if err != nil {
		t.Fatal(err)
	}
	if env3.Answer == nil || !*env3.Answer {
		t.Fatalf("题目3 答案 = %+v, want true", env3)
	}
	if _, err := qs.Repo.GetAnswer(5); err == nil {
		t.Fatal("编程题应拒绝判题")
	}
}
