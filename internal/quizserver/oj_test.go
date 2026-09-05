// OrangeOJ 判题链路 httptest 冒烟（空间模型）：
// 主库造域/空间/题目（域内编程题 + 客观题）→ 空间成员 → 学生三动作（真实本机 executor 注入）
// → 进度/历史。可见性 = 用户加入空间所在域含该题（域外题目 404）。
package quizserver

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"orangeoj/internal/judge"
	"orangeoj/internal/judgeserver"
	"orangeoj/internal/model"
	"orangeoj/internal/quizstore"
	"orangeoj/internal/store"
)

// ---------- 复用 server_test.go 的 doJSON/cookieOf/nested ----------

func loginStudent(t *testing.T, app *fiber.App, username, password string) string {
	t.Helper()
	resp, _ := doJSON(t, app, "POST", "/api/auth/login", "", map[string]string{"username": username, "password": password})
	if resp.StatusCode != 204 {
		t.Fatal("学生登录失败")
	}
	return cookieOf(resp)
}

func pollVerdict(t *testing.T, app *fiber.App, cookie string, subID int64) string {
	t.Helper()
	res := pollResult(t, app, cookie, subID)
	v, _ := res["verdict"].(string)
	return v
}

func pollResult(t *testing.T, app *fiber.App, cookie string, subID int64) map[string]any {
	t.Helper()
	for i := 0; i < 200; i++ {
		resp, out := doJSON(t, app, "GET", "/api/oj/submission/"+strconv.FormatInt(subID, 10)+"/poll", cookie, nil)
		if resp.StatusCode != 200 {
			t.Fatalf("poll = %d", resp.StatusCode)
		}
		if out["isFinal"] == true {
			return out
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("判题超时")
	return nil
}

type executorRunner struct {
	ex *judgeserver.Executor
}

func (e *executorRunner) Judge(ctx context.Context, task judge.JudgeTask) (judge.RunResult, error) {
	return e.ex.Execute(ctx, task)
}

// newTestOJSpaceApp 主库造域/空间/题目：域内 2 编程 + 1 单选 + 1 判断；另建异域 1 题（不可见性）。
// 空间成员（member）加入空间；刷题服务挂真实 executor runner。
// ids 返回 {p0..p3, foreign} 等主库题 id。
func newTestOJSpaceApp(t *testing.T) (*fiber.App, *Server, map[string]int64, bool) {
	t.Helper()
	dir := t.TempDir()
	main, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open main store: %v", err)
	}
	domainID, err := main.CreateDomain("OJ域")
	if err != nil {
		t.Fatalf("create domain: %v", err)
	}
	spaceID, err := main.CreateSpace(domainID, "OJ班")
	if err != nil {
		t.Fatalf("create space: %v", err)
	}
	// 异域（学生未加入任何该域空间）
	foreignDomain, err := main.CreateDomain("异域")
	if err != nil {
		t.Fatalf("create foreign domain: %v", err)
	}
	ids := map[string]int64{}
	probs := []struct {
		p    model.Problem
		dom  int64
		key  string
	}{
		{model.Problem{Type: model.TypeProgramming, Title: "A+B", Tags: []string{"入门"},
			StatementMD: "读入两个整数输出和。",
			BodyJSON:    json.RawMessage(`{"inputFormat":"一行两个整数","outputFormat":"一个整数","samples":[{"input":"1 2","output":"3"}],"testCases":[{"input":"1 2","output":"3"},{"input":"10 20","output":"30"}]}`),
			AnswerJSON:  json.RawMessage(`{}`), Solutions: json.RawMessage(`[]`), TimeLimitMS: 2000, MemoryLimitMiB: 256}, domainID, "p0"},
		{model.Problem{Type: model.TypeProgramming, Title: "回文判断", Tags: []string{"入门"},
			StatementMD: "判断字符串是否回文。",
			BodyJSON:    json.RawMessage(`{"inputFormat":"一个字符串","outputFormat":"YES 或 NO","samples":[{"input":"aba","output":"YES"}],"testCases":[{"input":"abc","output":"NO"},{"input":"abba","output":"YES"}]}`),
			AnswerJSON:  json.RawMessage(`{}`), Solutions: json.RawMessage(`[]`), TimeLimitMS: 2000, MemoryLimitMiB: 256}, domainID, "p1"},
		{model.Problem{Type: model.TypeSingleChoice, Title: "单选X", Tags: []string{"数学"},
			StatementMD: "1+1=?", BodyJSON: json.RawMessage(`{"options":["1","2","3"]}`),
			AnswerJSON: json.RawMessage(`{"answerIndex":1}`), Solutions: json.RawMessage(`[]`)}, domainID, "p2"},
		{model.Problem{Type: model.TypeTrueFalse, Title: "判断Y", Tags: []string{"数学"},
			StatementMD: "地球是圆的", BodyJSON: json.RawMessage(`{}`),
			AnswerJSON: json.RawMessage(`{"answer":true}`), Solutions: json.RawMessage(`[]`)}, domainID, "p3"},
		{model.Problem{Type: model.TypeSingleChoice, Title: "异域题", Tags: []string{"数学"},
			StatementMD: "异域 1+1=?", BodyJSON: json.RawMessage(`{"options":["1","2"]}`),
			AnswerJSON: json.RawMessage(`{"answerIndex":0}`), Solutions: json.RawMessage(`[]`)}, foreignDomain, "foreign"},
	}
	for _, row := range probs {
		id, err := main.CreateProblem(row.p)
		if err != nil {
			t.Fatalf("seed problem: %v", err)
		}
		if _, err := main.DB.Exec(`UPDATE problems SET domain_id=? WHERE id=?`, row.dom, id); err != nil {
			t.Fatal(err)
		}
		ids[row.key] = id
	}
	ids["domain"] = domainID
	ids["space"] = spaceID
	if err := main.Close(); err != nil {
		t.Fatalf("close main store: %v", err)
	}

	qs, err := quizstore.Open(dir, filepath.Join(dir, "orangeoj.db"))
	if err != nil {
		t.Fatalf("open quiz store: %v", err)
	}
	t.Cleanup(func() { _ = qs.Close() })
	memberID, err := qs.Accounts.CreateUser("ojstu", "pw", "member")
	if err != nil {
		t.Fatal(err)
	}
	ids["member"] = memberID

	// 回主库登记空间成员
	main2, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := main2.SetSpaceMembers(spaceID, []int64{memberID}); err != nil {
		t.Fatal(err)
	}
	if err := main2.Close(); err != nil {
		t.Fatal(err)
	}

	executor, err := judgeserver.NewExecutor(filepath.Join(dir, "jobs"), 20*time.Second)
	if err != nil {
		t.Fatalf("executor: %v", err)
	}
	pyOK := !executor.ToolchainMissing("python")

	srv := &Server{QS: qs, UploadsDir: filepath.Join(dir, "uploads")}
	if !srv.EnsureBootstrap() {
		t.Fatal("bootstrap 管理员失败")
	}
	app := New(srv, &executorRunner{ex: executor}, 2)
	t.Cleanup(srv.StopQueue)
	return app, srv, ids, pyOK
}

// TestOJSpaceFlow 空间成员三动作 + 进度 + 域外不可见（编程题真实评测）。
func TestOJSpaceFlow(t *testing.T) {
	app, _, ids, pyOK := newTestOJSpaceApp(t)
	stuCookie := loginStudent(t, app, "ojstu", "pw")

	// 题目正文：域内编程题可见且不下发 testCases
	programmingProblemID := ids["p0"]
	objectiveProblemID := ids["p2"]
	tfProblemID := ids["p3"]
	resp, out := doJSON(t, app, "GET", fmt.Sprintf("/api/oj/problem/%d", programmingProblemID), stuCookie, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("取编程题 = %d %v", resp.StatusCode, out)
	}
	body := nested(out, "bodyJson").(map[string]any)
	if _, has := body["testCases"]; has {
		t.Fatal("编程题不应下发 testCases")
	}
	if nested(out, "timeLimitMs").(float64) != 2000 {
		t.Fatalf("时限字段 = %v", out)
	}

	// 异域题目 404（学生未加入异域任何空间）
	if resp, _ := doJSON(t, app, "GET", fmt.Sprintf("/api/oj/problem/%d", ids["foreign"]), stuCookie, nil); resp.StatusCode != 404 {
		t.Fatalf("异域题应 404 = %d", resp.StatusCode)
	}
	// 未登录 401
	if resp, _ := doJSON(t, app, "GET", fmt.Sprintf("/api/oj/problem/%d", programmingProblemID), "", nil); resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("未登录 = %d, want 401", resp.StatusCode)
	}

	// 客观题：先答错 WA，再答对 AC（写 submissions + progress）
	resp, out = doJSON(t, app, "POST", fmt.Sprintf("/api/oj/problem/%d/objective-submit", objectiveProblemID), stuCookie, map[string]any{"answer": 0})
	if resp.StatusCode != 200 || nested(out, "verdict") != "WA" {
		t.Fatalf("客观答错 = %d %v", resp.StatusCode, out)
	}
	resp, out = doJSON(t, app, "POST", fmt.Sprintf("/api/oj/problem/%d/objective-submit", objectiveProblemID), stuCookie, map[string]any{"answer": 1})
	if resp.StatusCode != 200 || nested(out, "verdict") != "AC" || nested(out, "score").(float64) != 100 {
		t.Fatalf("客观答对 = %d %v", resp.StatusCode, out)
	}
	resp, out = doJSON(t, app, "POST", fmt.Sprintf("/api/oj/problem/%d/objective-submit", tfProblemID), stuCookie, map[string]any{"answer": true})
	if nested(out, "verdict") != "AC" {
		t.Fatalf("判断题 = %v", out)
	}

	if !pyOK {
		t.Log("python 不可用，编程题真实评测断言跳过")
	} else {
		// test：跑题面 testCases → AC
		code := "a, b = map(int, input().split())\nprint(a + b)\n"
		resp, out = doJSON(t, app, "POST", fmt.Sprintf("/api/oj/problem/%d/test", programmingProblemID), stuCookie, map[string]any{"language": "python", "sourceCode": code})
		if resp.StatusCode != 201 {
			t.Fatalf("test = %d %v", resp.StatusCode, out)
		}
		subID := int64(nested(out, "submissionId").(float64))
		if v := pollVerdict(t, app, stuCookie, subID); v != "AC" {
			t.Fatalf("test verdict = %s", v)
		}
		// submit → AC 且写进度
		resp, out = doJSON(t, app, "POST", fmt.Sprintf("/api/oj/problem/%d/submit", programmingProblemID), stuCookie, map[string]any{"language": "python", "sourceCode": code})
		subID = int64(nested(out, "submissionId").(float64))
		if v := pollVerdict(t, app, stuCookie, subID); v != "AC" {
			t.Fatalf("submit verdict = %s", v)
		}
		// run：自定义输入 → OK + stdout 含 12
		resp, out = doJSON(t, app, "POST", fmt.Sprintf("/api/oj/problem/%d/run", programmingProblemID), stuCookie, map[string]any{"language": "python", "sourceCode": code, "inputData": "5 7"})
		subID = int64(nested(out, "submissionId").(float64))
		runRes := pollResult(t, app, stuCookie, subID)
		if runRes["verdict"] != "OK" || !strings.Contains(runRes["stdout"].(string), "12") {
			t.Fatalf("run = %v", runRes)
		}
		// WA 提交
		waCode := "a, b = map(int, input().split())\nprint(a - b)\n"
		resp, out = doJSON(t, app, "POST", fmt.Sprintf("/api/oj/problem/%d/submit", programmingProblemID), stuCookie, map[string]any{"language": "python", "sourceCode": waCode})
		subID = int64(nested(out, "submissionId").(float64))
		if v := pollVerdict(t, app, stuCookie, subID); v != "WA" {
			t.Fatalf("WA verdict = %s", v)
		}
		// 历史 ≥ 4（test + submit + run + submit）
		_, out = doJSON(t, app, "GET", fmt.Sprintf("/api/oj/problem/%d/submissions", programmingProblemID), stuCookie, nil)
		if len(out["submissions"].([]any)) < 4 {
			t.Fatalf("提交历史 = %v", out)
		}
	}
}
