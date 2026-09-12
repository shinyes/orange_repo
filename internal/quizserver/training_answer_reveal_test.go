// 训练客观题「答案揭示时机」回归测试（改动 A）：
// 作答响应只在**次数用尽**（limited 训练达 max_attempts）时才下发 correctAnswer；
// 未用尽/不限次（maxAttempts=0）时只反馈对错，correctAnswer 必须为空对象。
// 全部走 HTTP 层真实请求（httptest + 真实 sqlite 临时库）。
package quizserver

import (
	"fmt"
	"testing"

	"github.com/gofiber/fiber/v2"

	"orangeoj/internal/model"
	"orangeoj/internal/quizstore"
	"orangeoj/internal/store"
)

// revealEnv 训练答案揭示测试环境（域/空间/两个训练/两题/一名空间成员）。
type revealEnv struct {
	app   *fiber.App
	cook  string
	space int64
	// limited：maxAttempts=3 的训练（单选 p1 答案 1；判断 p2 答案 true）
	limited int64
	// unlimited：maxAttempts=0 的训练（单选 p3 答案 0）
	unlimited  int64
	domain     int64
	p1, p2, p3 int64
}

// newRevealEnv 建库并组装 app（复用 rank_publicity_test.go 的三阶段建库范式）。
func newRevealEnv(t *testing.T) *revealEnv {
	t.Helper()
	dir := t.TempDir()

	// 阶段 1：主库建域/空间/题/训练（quizstore 打开时迁移，须先有域/空间结构）
	main, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open main store: %v", err)
	}
	domainID, err := main.CreateDomain("揭示域")
	if err != nil {
		t.Fatal(err)
	}
	spaceID, err := main.CreateSpace(domainID, "一班")
	if err != nil {
		t.Fatal(err)
	}
	// p1 单选（正确答案下标 1）；p2 判断（正确答案 true）；p3 单选（正确答案下标 0）
	newChoice := func(title string, answerIndex int) int64 {
		id, err := main.CreateProblem(model.Problem{
			Type: model.TypeSingleChoice, Title: title, StatementMD: title + "?",
			BodyJSON:   jsonRaw(`{"options":["A","B"]}`),
			AnswerJSON: jsonRaw(fmt.Sprintf(`{"answerIndex":%d}`, answerIndex)),
			Solutions:  jsonRaw(`[]`),
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := main.DB.Exec(`UPDATE problems SET domain_id=? WHERE id=?`, domainID, id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	newTF := func(title string, answer bool) int64 {
		id, err := main.CreateProblem(model.Problem{
			Type: model.TypeTrueFalse, Title: title, StatementMD: title + "?",
			BodyJSON:   jsonRaw(`{}`),
			AnswerJSON: jsonRaw(fmt.Sprintf(`{"answer":%t}`, answer)),
			Solutions:  jsonRaw(`[]`),
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := main.DB.Exec(`UPDATE problems SET domain_id=? WHERE id=?`, domainID, id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	env := &revealEnv{domain: domainID, space: spaceID}
	env.p1 = newChoice("单选1", 1)
	env.p2 = newTF("判断1", true)
	env.p3 = newChoice("单选3", 0)

	// 限答 3 次训练（单选 + 判断）
	env.limited, err = main.CreateSpaceTraining(spaceID, "限次训练", "", nil, 3, true)
	if err != nil {
		t.Fatal(err)
	}
	ch1, err := main.CreateSpaceChapter(env.limited, "客观题")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := main.AddSpaceChapterItems(ch1, []int64{env.p1, env.p2}); err != nil {
		t.Fatal(err)
	}
	// 不限次训练（maxAttempts=0）
	env.unlimited, err = main.CreateSpaceTraining(spaceID, "不限次训练", "", nil, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	ch2, err := main.CreateSpaceChapter(env.unlimited, "客观题")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := main.AddSpaceChapterItems(ch2, []int64{env.p3}); err != nil {
		t.Fatal(err)
	}
	if err := main.Close(); err != nil {
		t.Fatal(err)
	}

	// 阶段 2：quiz 库建学生
	qs, err := quizstore.Open(dir)
	if err != nil {
		t.Fatalf("open quiz store: %v", err)
	}
	t.Cleanup(func() { _ = qs.Close() })
	stuID, err := qs.Accounts.CreateUser("stu1", "pw", "member")
	if err != nil {
		t.Fatal(err)
	}

	// 阶段 3：回主库写空间成员 + 可见名单
	// 可见性语义已改为「已开放 AND 已分配到可见名单」（缺一不可）：两个训练虽 is_public=true，
	// 仍须把 stu1 写进 space_training_visible，否则成员连详情/作答入口都进不去（404），
	// 本文件考察的「答案揭示时机」就无从验证。授予名单只是让 fixture 满足新语义，不改判定预期。
	main2, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = main2.Close() })
	if err := main2.SetSpaceMembers(spaceID, []int64{stuID}); err != nil {
		t.Fatal(err)
	}
	if err := main2.SetVisibleUsers("space_training_visible", env.limited, []int64{stuID}); err != nil {
		t.Fatal(err)
	}
	if err := main2.SetVisibleUsers("space_training_visible", env.unlimited, []int64{stuID}); err != nil {
		t.Fatal(err)
	}

	env.app = New(&Server{QS: qs}, nil, 0)
	env.cook = loginStudent(t, env.app, "stu1", "pw")
	return env
}

// answer 提交一次客观题作答，返回响应体。
func (e *revealEnv) answer(t *testing.T, trainingID, problemID int64, answer any) map[string]any {
	t.Helper()
	resp, out := doJSON(t, e.app, "POST",
		fmt.Sprintf("/api/portal/space/%d/training/%d/answer", e.space, trainingID),
		e.cook, map[string]any{"problemId": problemID, "answer": answer})
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("answer(training=%d,problem=%d) = %d %v", trainingID, problemID, resp.StatusCode, out)
	}
	return out
}

// detail 取训练详情（reviewEnv 验证详情接口的回顾态）。
func (e *revealEnv) detail(t *testing.T, trainingID int64) map[string]any {
	t.Helper()
	resp, out := doJSON(t, e.app, "GET",
		fmt.Sprintf("/api/portal/space/%d/training/%d", e.space, trainingID), e.cook, nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("detail(training=%d) = %d %v", trainingID, resp.StatusCode, out)
	}
	return out
}

// correctAnswerOf 取响应中的 correctAnswer（缺省视为空对象）。
func correctAnswerOf(t *testing.T, out map[string]any) map[string]any {
	t.Helper()
	v, ok := out["correctAnswer"]
	if !ok || v == nil {
		return map[string]any{}
	}
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("correctAnswer 类型 = %T (%v)", v, v)
	}
	return m
}

// itemOf 取详情中指定题目的条目视图。
func itemOf(t *testing.T, detail map[string]any, problemID int64) map[string]any {
	t.Helper()
	chapters, ok := detail["chapters"].([]any)
	if !ok {
		t.Fatalf("chapters 类型异常: %v", detail)
	}
	for _, chAny := range chapters {
		ch, _ := chAny.(map[string]any)
		items, _ := ch["items"].([]any)
		for _, itAny := range items {
			it, _ := itAny.(map[string]any)
			if int64(it["problemId"].(float64)) == problemID {
				return it
			}
		}
	}
	t.Fatalf("训练详情未找到题 %d: %v", problemID, detail)
	return nil
}

// num 断言数值字段（JSON 数字解析为 float64）。
func num(t *testing.T, m map[string]any, key string) float64 {
	t.Helper()
	v, ok := m[key].(float64)
	if !ok {
		t.Fatalf("字段 %s = %v（%T），期望数字", key, m[key], m[key])
	}
	return v
}

// TestTrainingAnswerReveal 改动 A：限答 3 次训练——
// 第 1/2 次答错不揭示答案；第 3 次（次数用尽）才回带答案。
func TestTrainingAnswerReveal(t *testing.T) {
	env := newRevealEnv(t)
	tid := env.limited

	// ---- 单选：第 1 次答错（正确答案 1，提交 0）----
	a1 := env.answer(t, tid, env.p1, 0)
	if a1["correct"] != false || a1["solved"] != false || a1["locked"] != false {
		t.Fatalf("第1次答错标志位 = %v（want correct/solved/locked 全 false）", a1)
	}
	if num(t, a1, "attempts") != 1 || num(t, a1, "remaining") != 2 {
		t.Fatalf("第1次 attempts=%v remaining=%v（want 1 / 2）", a1["attempts"], a1["remaining"])
	}
	if ca := correctAnswerOf(t, a1); len(ca) != 0 {
		t.Fatalf("第1次答错下发了 correctAnswer=%v（应空对象：未用尽不得泄露答案）", ca)
	}
	if _, has := correctAnswerOf(t, a1)["answerIndex"]; has {
		t.Fatalf("第1次答错响应含 answerIndex: %v", a1)
	}

	// ---- 单选：第 2 次答错 ----
	a2 := env.answer(t, tid, env.p1, 0)
	if a2["correct"] != false || a2["locked"] != false {
		t.Fatalf("第2次答错标志位 = %v", a2)
	}
	if num(t, a2, "attempts") != 2 || num(t, a2, "remaining") != 1 {
		t.Fatalf("第2次 attempts=%v remaining=%v（want 2 / 1）", a2["attempts"], a2["remaining"])
	}
	if ca := correctAnswerOf(t, a2); len(ca) != 0 {
		t.Fatalf("第2次答错下发了 correctAnswer=%v（应空对象）", ca)
	}

	// ---- 单选：第 3 次答错 → 次数用尽，锁定并揭示答案 ----
	a3 := env.answer(t, tid, env.p1, 0)
	if a3["correct"] != false {
		t.Fatalf("第3次 correct = %v（want false）", a3["correct"])
	}
	if a3["locked"] != true || a3["solved"] != false {
		t.Fatalf("第3次 locked/solved = %v（want locked=true solved=false）", a3)
	}
	if num(t, a3, "attempts") != 3 || num(t, a3, "remaining") != 0 {
		t.Fatalf("第3次 attempts=%v remaining=%v（want 3 / 0）", a3["attempts"], a3["remaining"])
	}
	ca3 := correctAnswerOf(t, a3)
	if idx, ok := ca3["answerIndex"].(float64); !ok || int(idx) != 1 {
		t.Fatalf("第3次 correctAnswer=%v（want answerIndex=1 与题库答案一致）", ca3)
	}
	// 单选题不应携带判断题专用字段
	if _, has := ca3["answer"]; has {
		t.Fatalf("单选 correctAnswer 不应含 answer: %v", ca3)
	}

	// ---- 回顾态：详情接口在达上限后回带正确答案（实现未改动的既有预期）----
	item := itemOf(t, env.detail(t, tid), env.p1)
	if item["locked"] != true || item["solved"] != false || num(t, item, "attempts") != 3 {
		t.Fatalf("详情条目状态 = %v（want locked=true solved=false attempts=3）", item)
	}
	di, ok := item["correctAnswer"].(map[string]any)
	if !ok || int(di["answerIndex"].(float64)) != 1 {
		t.Fatalf("达上限后详情 correctAnswer = %v（want answerIndex=1）", item["correctAnswer"])
	}

	// ---- 判断题型：同样「用尽才揭示」，答案为布尔 ----
	// 第 1 次答错（正确答案 true，提交 false）
	b1 := env.answer(t, tid, env.p2, false)
	if b1["correct"] != false || b1["locked"] != false || num(t, b1, "remaining") != 2 {
		t.Fatalf("判断题第1次 = %v", b1)
	}
	if ca := correctAnswerOf(t, b1); len(ca) != 0 {
		t.Fatalf("判断题第1次下发了 correctAnswer=%v", ca)
	}
	// 第 2 次答错
	b2 := env.answer(t, tid, env.p2, false)
	if num(t, b2, "attempts") != 2 || b2["locked"] != false || num(t, b2, "remaining") != 1 {
		t.Fatalf("判断题第2次 = %v", b2)
	}
	if ca := correctAnswerOf(t, b2); len(ca) != 0 {
		t.Fatalf("判断题第2次下发了 correctAnswer=%v", ca)
	}
	// 第 3 次答错 → 揭示布尔答案
	b3 := env.answer(t, tid, env.p2, false)
	if b3["locked"] != true || num(t, b3, "attempts") != 3 || num(t, b3, "remaining") != 0 {
		t.Fatalf("判断题第3次 = %v", b3)
	}
	cb := correctAnswerOf(t, b3)
	if v, ok := cb["answer"].(bool); !ok || v != true {
		t.Fatalf("判断题第3次 correctAnswer=%v（want answer=true）", cb)
	}
	if _, has := cb["answerIndex"]; has {
		t.Fatalf("判断题 correctAnswer 不应含 answerIndex: %v", cb)
	}
}

// TestTrainingAnswerRevealUnlimited 不限次（maxAttempts=0）：答错不锁定、remaining=-1、不揭示答案；
// 详情在未用尽/未答对时也不得回带答案。
func TestTrainingAnswerRevealUnlimited(t *testing.T) {
	env := newRevealEnv(t)
	tid := env.unlimited

	// 连续答错 3 次（不限次：次数无上限，永远不 locked）
	for i := 1; i <= 3; i++ {
		a := env.answer(t, tid, env.p3, 1) // 正确答案下标 0，提交 1 → 错
		if a["correct"] != false || a["solved"] != false || a["locked"] != false {
			t.Fatalf("不限次第%d次标志位 = %v（want correct/solved/locked 全 false）", i, a)
		}
		if num(t, a, "attempts") != float64(i) {
			t.Fatalf("不限次第%d次 attempts = %v", i, a["attempts"])
		}
		if num(t, a, "remaining") != -1 {
			t.Fatalf("不限次第%d次 remaining = %v（want -1=不限次）", i, a["remaining"])
		}
		if ca := correctAnswerOf(t, a); len(ca) != 0 {
			t.Fatalf("不限次第%d次下发了 correctAnswer=%v（应空对象：不限次不在作答响应揭示）", i, ca)
		}
		// 详情：未答对且不限次 → 不 locked、不下发答案
		item := itemOf(t, env.detail(t, tid), env.p3)
		if item["locked"] != false || item["solved"] != false {
			t.Fatalf("不限次第%d次详情条目 = %v（want locked/solved false）", i, item)
		}
		if _, has := item["correctAnswer"]; has {
			t.Fatalf("不限次第%d次详情回带了 correctAnswer: %v", i, item["correctAnswer"])
		}
	}

	// 答对 → solved 且详情回带答案（回顾态；remaining 仍为 -1）
	a := env.answer(t, tid, env.p3, 0)
	if a["correct"] != true || a["solved"] != true || a["locked"] != true {
		t.Fatalf("不限次答对 = %v", a)
	}
	if num(t, a, "remaining") != -1 {
		t.Fatalf("不限次答对 remaining = %v（want -1）", a["remaining"])
	}
	// 答对时实现只揭示「答错且用尽」的场景，correctAnswer 保持空对象
	if ca := correctAnswerOf(t, a); len(ca) != 0 {
		t.Fatalf("答对响应 correctAnswer = %v（既有实现语义：仅答错且用尽才揭示）", ca)
	}
	// 已答对 → 详情回顾态回带答案
	item := itemOf(t, env.detail(t, tid), env.p3)
	di, ok := item["correctAnswer"].(map[string]any)
	if !ok || int(di["answerIndex"].(float64)) != 0 {
		t.Fatalf("答对后详情 correctAnswer = %v（want answerIndex=0）", item["correctAnswer"])
	}
}

// TestTrainingAnswerRevealLockIsStatic 已答对/次数用尽后再次提交 → 409「该题已锁定」。
func TestTrainingAnswerRevealLockIsStatic(t *testing.T) {
	env := newRevealEnv(t)
	tid := env.limited

	// p1：用尽 3 次（全错）
	for i := 0; i < 3; i++ {
		env.answer(t, tid, env.p1, 0)
	}
	resp, out := doJSON(t, env.app, "POST",
		fmt.Sprintf("/api/portal/space/%d/training/%d/answer", env.space, tid), env.cook,
		map[string]any{"problemId": env.p1, "answer": 1}) // 即使这次是正确答案
	if resp.StatusCode != fiber.StatusConflict || out["error"] != "该题已锁定（答对或次数用尽）" {
		t.Fatalf("用尽后再答 = %d %v（want 409 该题已锁定（答对或次数用尽））", resp.StatusCode, out)
	}
	// 状态未被这次提交污染
	item := itemOf(t, env.detail(t, tid), env.p1)
	if num(t, item, "attempts") != 3 || item["solved"] != false || item["locked"] != true {
		t.Fatalf("409 后状态 = %v（want attempts=3 solved=false locked=true）", item)
	}

	// p2：答对后再次提交 → 同样 409
	ok := env.answer(t, tid, env.p2, true)
	if ok["correct"] != true || ok["solved"] != true || ok["locked"] != true {
		t.Fatalf("判断题答对 = %v", ok)
	}
	resp2, out2 := doJSON(t, env.app, "POST",
		fmt.Sprintf("/api/portal/space/%d/training/%d/answer", env.space, tid), env.cook,
		map[string]any{"problemId": env.p2, "answer": true})
	if resp2.StatusCode != fiber.StatusConflict {
		t.Fatalf("已答对后再答 = %d %v（want 409）", resp2.StatusCode, out2)
	}
	// 已答对：attempts 不再自增
	item2 := itemOf(t, env.detail(t, tid), env.p2)
	if num(t, item2, "attempts") != 1 || item2["solved"] != true {
		t.Fatalf("答对后状态 = %v（want attempts=1 solved=true）", item2)
	}
}
