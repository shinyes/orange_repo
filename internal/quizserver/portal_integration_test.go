// 门户端到端集成测试：域/空间/训练（限次）/练习（交卷）/刷题/排行榜 HTTP 全链路。
package quizserver

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"orangeoj/internal/judgeserver"
	"orangeoj/internal/model"
	"orangeoj/internal/quizstore"
	"orangeoj/internal/store"
)

func jsonRaw(v string) json.RawMessage { return json.RawMessage(v) }

// newPortalEnv 建完整环境：
//  1. 主库：域/空间/题目（域内）/空间训练（2 章 3 题 限次2）/空间练习（2 题）/刷题项目
//  2. quiz：打开（先建学生取 id）→ 回主库写 space_members → quiz 复用
//
// 返回 (app, spaceID, stuID, adminApp 同 app)。测试用 member 空间流程。
func newPortalEnv(t *testing.T) (*fiber.App, map[string]int64, int64, string) {
	t.Helper()
	dir := t.TempDir()
	ids := map[string]int64{}

	// ---- 阶段 1：主库结构 ----
	main, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open main store: %v", err)
	}
	domainID, err := main.CreateDomain("数学域")
	if err != nil {
		t.Fatal(err)
	}
	spaceID, err := main.CreateSpace(domainID, "初一(1)班")
	if err != nil {
		t.Fatal(err)
	}
	probs := []model.Problem{
		{Type: model.TypeProgramming, Title: "求和", BodyJSON: jsonRaw(`{"testCases":[]}`),
			AnswerJSON: jsonRaw(`{}`), Solutions: jsonRaw(`[]`), TimeLimitMS: 2000, MemoryLimitMiB: 256},
		{Type: model.TypeSingleChoice, Title: "1+1", StatementMD: "1+1=?",
			BodyJSON: jsonRaw(`{"options":["1","2"]}`), AnswerJSON: jsonRaw(`{"answerIndex":1}`), Solutions: jsonRaw(`[]`)},
		{Type: model.TypeTrueFalse, Title: "地球是圆的", StatementMD: "地球是圆的？",
			BodyJSON: jsonRaw(`{}`), AnswerJSON: jsonRaw(`{"answer":true}`), Solutions: jsonRaw(`[]`)},
	}
	for i, p := range probs {
		id, err := main.CreateProblem(p)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := main.DB.Exec(`UPDATE problems SET domain_id=? WHERE id=?`, domainID, id); err != nil {
			t.Fatal(err)
		}
		ids[fmt.Sprintf("p%d", i)] = id
	}
	trID, err := main.CreateSpaceTraining(spaceID, "单元训练", "", nil, 2)
	if err != nil {
		t.Fatal(err)
	}
	ids["training"] = trID
	ch1, _ := main.CreateSpaceChapter(trID, "选择")
	_, _ = main.AddSpaceChapterItems(ch1, []int64{ids["p1"]})
	ch2, _ := main.CreateSpaceChapter(trID, "判断")
	_, _ = main.AddSpaceChapterItems(ch2, []int64{ids["p2"]})
	prID, err := main.CreateSpacePractice(spaceID, "期中卷", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	ids["practice"] = prID
	_ = main.AddSpacePracticeItems(prID, []int64{ids["p1"], ids["p2"]})
	qID, err := main.CreateSpaceQuiz(spaceID, "每日刷题", nil, "tags", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	ids["quiz"] = qID
	ids["domain"] = domainID
	ids["space"] = spaceID
	if err := main.Close(); err != nil {
		t.Fatal(err)
	}

	// ---- 阶段 2：quiz 建学生（users 在 quiz.db） ----
	qs, err := quizstore.Open(dir, filepath.Join(dir, "orangeoj.db"))
	if err != nil {
		t.Fatalf("open quiz store: %v", err)
	}
	t.Cleanup(func() { _ = qs.Close() })
	stuID, err := qs.Accounts.CreateUser("stu1", "pw", "member")
	if err != nil {
		t.Fatal(err)
	}

	// ---- 阶段 3：回主库写空间成员 ----
	main2, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := main2.SetSpaceMembers(spaceID, []int64{stuID}); err != nil {
		t.Fatal(err)
	}
	if err := main2.Close(); err != nil {
		t.Fatal(err)
	}

	executor, err := judgeserver.NewExecutor(filepath.Join(dir, "jobs"), 20*time.Second)
	if err != nil {
		t.Fatalf("executor: %v", err)
	}
	srv := &Server{QS: qs, UploadsDir: filepath.Join(dir, "uploads")}
	if !srv.EnsureBootstrap() {
		t.Fatal("bootstrap 失败")
	}
	app := New(srv, &executorRunner{ex: executor}, 2)
	t.Cleanup(srv.StopQueue)
	return app, ids, stuID, ""
}

// TestPortalMemberFlow 成员全链路。
func TestPortalMemberFlow(t *testing.T) {
	app, ids, _, _ := newPortalEnv(t)
	stuCookie := loginStudent(t, app, "stu1", "pw")

	// 我的空间
	resp, out := doJSON(t, app, "GET", "/api/portal/spaces", stuCookie, nil)
	if resp.StatusCode != 200 || len(out["spaces"].([]any)) != 1 {
		t.Fatalf("spaces = %d %v", resp.StatusCode, out)
	}
	spaceID := ids["space"]

	// 空间首页三区
	_, home := doJSON(t, app, "GET", fmt.Sprintf("/api/portal/space/%d/home", spaceID), stuCookie, nil)
	if len(home["trainings"].([]any)) != 1 || len(home["practices"].([]any)) != 1 || len(home["quizzes"].([]any)) != 1 {
		t.Fatalf("home = %v", home)
	}

	// 训练详情（题状态初始 0）
	trID := ids["training"]
	_, td := doJSON(t, app, "GET", fmt.Sprintf("/api/portal/space/%d/training/%d", spaceID, trID), stuCookie, nil)
	chapters := td["chapters"].([]any)
	if len(chapters) != 2 {
		t.Fatalf("training chapters = %v", td)
	}
	first := chapters[0].(map[string]any)["items"].([]any)[0].(map[string]any)
	if first["locked"] != false || first["attempts"].(float64) != 0 {
		t.Fatalf("初始状态 = %v", first)
	}

	// 限次作答：限次=2。答错 1 次 → attempts1 未锁；再答错 → 锁红；第三次被拒
	p1 := ids["p1"]
	resp, ans := doJSON(t, app, "POST", fmt.Sprintf("/api/portal/space/%d/training/%d/answer", spaceID, trID), stuCookie,
		map[string]any{"problemId": p1, "answer": 0}) // 错（答案 1）
	if resp.StatusCode != 200 || ans["correct"] != false || ans["attempts"].(float64) != 1 || ans["locked"] != false {
		t.Fatalf("answer1 = %d %v", resp.StatusCode, ans)
	}
	_, ans2 := doJSON(t, app, "POST", fmt.Sprintf("/api/portal/space/%d/training/%d/answer", spaceID, trID), stuCookie,
		map[string]any{"problemId": p1, "answer": 0})
	if ans2["attempts"].(float64) != 2 || ans2["locked"] != true {
		t.Fatalf("answer2 = %v", ans2)
	}
	resp3, _ := doJSON(t, app, "POST", fmt.Sprintf("/api/portal/space/%d/training/%d/answer", spaceID, trID), stuCookie,
		map[string]any{"problemId": p1, "answer": 1})
	if resp3.StatusCode != 409 {
		t.Fatalf("第三次应 409 = %d", resp3.StatusCode)
	}

	// 判断题一次答对 → solved 绿
	p2 := ids["p2"]
	_, ans3 := doJSON(t, app, "POST", fmt.Sprintf("/api/portal/space/%d/training/%d/answer", spaceID, trID), stuCookie,
		map[string]any{"problemId": p2, "answer": true})
	if ans3["correct"] != true || ans3["solved"] != true || ans3["locked"] != true {
		t.Fatalf("answer tf = %v", ans3)
	}

	// 刷题：训练后 p2 已通过 → 抽题必为 p1；答对记通过
	qID := ids["quiz"]
	_, qp := doJSON(t, app, "GET", fmt.Sprintf("/api/portal/quiz/%d/problem", qID), stuCookie, nil)
	if qp["done"] == true || qp["problem"] == nil {
		t.Fatalf("quiz problem = %v", qp)
	}
	prob := qp["problem"].(map[string]any)
	pid := int64(prob["id"].(float64))
	var correctAns any
	if prob["type"] == "single_choice" {
		correctAns = 1
	} else {
		correctAns = true
	}
	_, qa := doJSON(t, app, "POST", fmt.Sprintf("/api/portal/quiz/%d/answer", qID), stuCookie,
		map[string]any{"problemId": pid, "answer": correctAns})
	if qa["correct"] != true {
		t.Fatalf("quiz answer = %v", qa)
	}

	// 练习交卷：p1 答对 + p2 答对 → 2/2；交卷快照含 correct → 通过记录写入（去重）
	prID := ids["practice"]
	_, sub := doJSON(t, app, "POST", fmt.Sprintf("/api/portal/space/%d/practice/%d/submit", spaceID, prID), stuCookie,
		map[string]any{"answers": []map[string]any{
			{"problemId": p1, "answer": 1}, {"problemId": p2, "answer": true},
		}})
	if sub["objectiveCorrect"].(float64) != 2 || sub["objectiveTotal"].(float64) != 2 {
		t.Fatalf("submit = %v", sub)
	}
	_, subs := doJSON(t, app, "GET", fmt.Sprintf("/api/portal/space/%d/practice/%d/submissions", spaceID, prID), stuCookie, nil)
	if len(subs["submissions"].([]any)) != 1 {
		t.Fatalf("subs = %v", subs)
	}

	// 排行榜（域内成员：p1+p2 两题 uuid 去重 = 2）
	_, rank := doJSON(t, app, "GET", fmt.Sprintf("/api/portal/rank?domainId=%d", ids["domain"]), stuCookie, nil)
	rows := rank["rank"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["username"] != "stu1" {
		t.Fatalf("rank = %v", rank)
	}
	if rows[0].(map[string]any)["solved"].(float64) != 2 {
		t.Fatalf("rank solved = %v（应 2：uuid 去重）", rows[0])
	}

	// 空间成员可经 OJ 端点取题（空间训练/练习引用域内题；编程题跳转做题页前提）
	p0 := ids["p0"] // 编程题（域内，未在任何训练但属该域）
	respOJ, outOJ := doJSON(t, app, "GET", fmt.Sprintf("/api/oj/problem/%d", p0), stuCookie, nil)
	if respOJ.StatusCode != 200 {
		t.Fatalf("空间成员取域内题 = %d %v（应可见）", respOJ.StatusCode, outOJ)
	}
}
