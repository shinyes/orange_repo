// OrangeOJ 判题链路 httptest 冒烟：
// 主库造训练/练习（含编程题与客观题）→ 布置 → 学生三动作（真实本机 executor 注入）→ 进度/历史/统计。
package quizserver

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"orangerepo/internal/judge"
	"orangerepo/internal/judgeserver"
	"orangerepo/internal/model"
	"orangerepo/internal/quizstore"
	"orangerepo/internal/store"
)

// ---------- 复用 server_test.go 的 doJSON/cookieOf/nested ----------

func loginAdmin(t *testing.T, app *fiber.App) string {
	t.Helper()
	resp, _ := doJSON(t, app, "POST", "/api/auth/login", "", map[string]string{"username": "admin", "password": "123456"})
	if resp.StatusCode != 204 {
		t.Fatal("admin 登录失败")
	}
	return cookieOf(resp)
}

func loginStudent(t *testing.T, app *fiber.App, username, password string) string {
	t.Helper()
	resp, _ := doJSON(t, app, "POST", "/api/auth/login", "", map[string]string{"username": username, "password": password})
	if resp.StatusCode != 204 {
		t.Fatal("学生登录失败")
	}
	return cookieOf(resp)
}

func createStudent(t *testing.T, app *fiber.App, adminCookie, username, password string) int64 {
	t.Helper()
	resp, out := doJSON(t, app, "POST", "/api/admin/students", adminCookie, map[string]string{"username": username, "password": password})
	if resp.StatusCode != 201 {
		t.Fatalf("建学生 = %d %v", resp.StatusCode, out)
	}
	return int64(nested(out, "id").(float64))
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

// newTestOJApp 主库造题：2 编程 + 1 单选 + 1 判断 → 训练（2 章）+ 练习；刷题服务挂真实 executor runner。
func newTestOJApp(t *testing.T) (*fiber.App, *Server, map[string]int64, bool) {
	t.Helper()
	dir := t.TempDir()
	main, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open main store: %v", err)
	}
	ids := map[string]int64{}
	probs := []model.Problem{
		{Type: model.TypeProgramming, Title: "A+B", Tags: []string{"入门"},
			StatementMD: "读入两个整数输出和。",
			BodyJSON:    json.RawMessage(`{"inputFormat":"一行两个整数","outputFormat":"一个整数","samples":[{"input":"1 2","output":"3"}],"testCases":[{"input":"1 2","output":"3"},{"input":"10 20","output":"30"}]}`),
			AnswerJSON:  json.RawMessage(`{}`), Solutions: json.RawMessage(`[]`), TimeLimitMS: 2000, MemoryLimitMiB: 256},
		{Type: model.TypeProgramming, Title: "回文判断", Tags: []string{"入门"},
			StatementMD: "判断字符串是否回文。",
			BodyJSON:    json.RawMessage(`{"inputFormat":"一个字符串","outputFormat":"YES 或 NO","samples":[{"input":"aba","output":"YES"}],"testCases":[{"input":"abc","output":"NO"},{"input":"abba","output":"YES"}]}`),
			AnswerJSON:  json.RawMessage(`{}`), Solutions: json.RawMessage(`[]`), TimeLimitMS: 2000, MemoryLimitMiB: 256},
		{Type: model.TypeSingleChoice, Title: "单选X", Tags: []string{"数学"},
			StatementMD: "1+1=?", BodyJSON: json.RawMessage(`{"options":["1","2","3"]}`),
			AnswerJSON: json.RawMessage(`{"answerIndex":1}`), Solutions: json.RawMessage(`[]`)},
		{Type: model.TypeTrueFalse, Title: "判断Y", Tags: []string{"数学"},
			StatementMD: "地球是圆的", BodyJSON: json.RawMessage(`{}`),
			AnswerJSON: json.RawMessage(`{"answer":true}`), Solutions: json.RawMessage(`[]`)},
	}
	for i, p := range probs {
		id, err := main.CreateProblem(p)
		if err != nil {
			t.Fatalf("seed problem: %v", err)
		}
		ids["p"+strconv.Itoa(i)] = id
	}
	trID, err := main.CreateTraining("入门训练", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	ch1, err := main.CreateChapter(trID, "基础")
	if err != nil {
		t.Fatal(err)
	}
	ch2, err := main.CreateChapter(trID, "进阶")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := main.AddChapterItems(ch1, []int64{ids["p0"], ids["p2"]}); err != nil {
		t.Fatal(err)
	}
	if _, err := main.AddChapterItems(ch2, []int64{ids["p3"]}); err != nil {
		t.Fatal(err)
	}
	prID, err := main.CreatePractice("综合练习", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := main.AddPracticeItems(prID, []int64{ids["p1"], ids["p2"], ids["p3"]}); err != nil {
		t.Fatal(err)
	}
	if err := main.Close(); err != nil {
		t.Fatal(err)
	}
	ids["training"] = trID
	ids["practice"] = prID

	qs, err := quizstore.Open(dir, filepath.Join(dir, "orangerepo.db"))
	if err != nil {
		t.Fatalf("open quiz store: %v", err)
	}
	t.Cleanup(func() { _ = qs.Close() })

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

// TestOJFullFlow 布置 + 三动作 + 进度 + 统计全链路（编程题真实评测）。
func TestOJFullFlow(t *testing.T) {
	app, _, ids, pyOK := newTestOJApp(t)
	adminCookie := loginAdmin(t, app)

	bobID := createStudent(t, app, adminCookie, "bob", "pw")
	createStudent(t, app, adminCookie, "alice", "pw")

	// 主库目录浏览
	resp, out := doJSON(t, app, "GET", "/api/admin/repo-trainings", adminCookie, nil)
	if resp.StatusCode != 200 || len(out["trainings"].([]any)) != 1 {
		t.Fatalf("repo-trainings = %d %v", resp.StatusCode, out)
	}
	if nested(out, "trainings.0.problemCount").(float64) != 3 {
		t.Fatalf("训练题数 = %v", out)
	}
	if _, out := doJSON(t, app, "GET", "/api/admin/repo-practices", adminCookie, nil); len(out["practices"].([]any)) != 1 {
		t.Fatalf("repo-practices = %v", out)
	}

	// 布置：训练 → 定向 bob；练习 → 全体
	resp, out = doJSON(t, app, "POST", "/api/admin/assignments", adminCookie, map[string]any{
		"kind": "training", "repoId": ids["training"], "assignedAll": false, "studentIds": []int64{bobID},
	})
	if resp.StatusCode != 201 {
		t.Fatalf("建训练布置 = %d %v", resp.StatusCode, out)
	}
	trainingAssignID := int64(nested(out, "id").(float64))

	resp, out = doJSON(t, app, "POST", "/api/admin/assignments", adminCookie, map[string]any{
		"kind": "practice", "repoId": ids["practice"], "assignedAll": true,
	})
	if resp.StatusCode != 201 {
		t.Fatalf("建练习布置 = %d %v", resp.StatusCode, out)
	}
	practiceAssignID := int64(nested(out, "id").(float64))
	if resp, _ := doJSON(t, app, "POST", "/api/admin/assignments", adminCookie, map[string]any{
		"kind": "training", "repoId": ids["training"], "assignedAll": true,
	}); resp.StatusCode != 409 {
		t.Fatal("重复布置应 409")
	}

	// bob：训练（定向）+ 练习（全体）都可见
	bobCookie := loginStudent(t, app, "bob", "pw")
	_, out = doJSON(t, app, "GET", "/api/oj/assigned", bobCookie, nil)
	if len(out["trainings"].([]any)) != 1 || len(out["practices"].([]any)) != 1 {
		t.Fatalf("bob 任务列表 = %v", out)
	}
	// alice：训练不可见，练习可见
	aliceCookie := loginStudent(t, app, "alice", "pw")
	_, out = doJSON(t, app, "GET", "/api/oj/assigned", aliceCookie, nil)
	if len(out["trainings"].([]any)) != 0 || len(out["practices"].([]any)) != 1 {
		t.Fatalf("alice 可见性 = %v", out)
	}

	// 撤回练习 → 学生不可见；恢复后可见
	if resp, _ := doJSON(t, app, "PATCH", "/api/admin/assignments/"+strconv.FormatInt(practiceAssignID, 10), adminCookie, map[string]any{"published": false}); resp.StatusCode != 204 {
		t.Fatal("撤回失败")
	}
	_, out = doJSON(t, app, "GET", "/api/oj/assigned", bobCookie, nil)
	if len(out["practices"].([]any)) != 0 {
		t.Fatalf("撤回后练习应不可见 = %v", out)
	}
	if resp, _ := doJSON(t, app, "PATCH", "/api/admin/assignments/"+strconv.FormatInt(practiceAssignID, 10), adminCookie, map[string]any{"published": true}); resp.StatusCode != 204 {
		t.Fatal("恢复发布失败")
	}

	// 训练详情（章节结构）+ 完成态
	resp, out = doJSON(t, app, "GET", "/api/oj/training/"+strconv.FormatInt(trainingAssignID, 10), bobCookie, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("训练详情 = %d %v", resp.StatusCode, out)
	}
	chapters := nested(out, "chapters").([]any)
	if len(chapters) != 2 {
		t.Fatalf("章节数 = %v", out)
	}
	items0 := nested(out, "chapters.0.items").([]any)
	items1 := nested(out, "chapters.1.items").([]any)
	if len(items0) != 2 || len(items1) != 1 {
		t.Fatalf("条目结构 = %v / %v", items0, items1)
	}
	programmingProblemID := int64(items0[0].(map[string]any)["problemId"].(float64))
	objectiveProblemID := int64(items0[1].(map[string]any)["problemId"].(float64))
	tfProblemID := int64(items1[0].(map[string]any)["problemId"].(float64))

	// 题目正文：编程题不含 testCases/answerJson；客观题不含 answerJson
	resp, out = doJSON(t, app, "GET", "/api/oj/problem/"+strconv.FormatInt(programmingProblemID, 10), bobCookie, nil)
	body := nested(out, "bodyJson").(map[string]any)
	if _, has := body["testCases"]; has {
		t.Fatal("编程题不应下发 testCases")
	}
	if nested(out, "timeLimitMs").(float64) != 2000 {
		t.Fatalf("时限字段 = %v", out)
	}

	// 客观题：先答错 WA，再答对 AC（写 submissions + progress）
	resp, out = doJSON(t, app, "POST", "/api/oj/problem/"+strconv.FormatInt(objectiveProblemID, 10)+"/objective-submit", bobCookie, map[string]any{"answer": 0})
	if resp.StatusCode != 200 || nested(out, "verdict") != "WA" {
		t.Fatalf("客观答错 = %d %v", resp.StatusCode, out)
	}
	resp, out = doJSON(t, app, "POST", "/api/oj/problem/"+strconv.FormatInt(objectiveProblemID, 10)+"/objective-submit", bobCookie, map[string]any{"answer": 1})
	if resp.StatusCode != 200 || nested(out, "verdict") != "AC" || nested(out, "score").(float64) != 100 {
		t.Fatalf("客观答对 = %d %v", resp.StatusCode, out)
	}
	// 判断题答对
	resp, out = doJSON(t, app, "POST", "/api/oj/problem/"+strconv.FormatInt(tfProblemID, 10)+"/objective-submit", bobCookie, map[string]any{"answer": true})
	if nested(out, "verdict") != "AC" {
		t.Fatalf("判断题 = %v", out)
	}

	if !pyOK {
		t.Log("python 不可用，编程题真实评测断言跳过")
	} else {
		// test：跑题面 testCases → AC
		code := "a, b = map(int, input().split())\nprint(a + b)\n"
		resp, out = doJSON(t, app, "POST", "/api/oj/problem/"+strconv.FormatInt(programmingProblemID, 10)+"/test", bobCookie, map[string]any{"language": "python", "sourceCode": code})
		if resp.StatusCode != 201 {
			t.Fatalf("test = %d %v", resp.StatusCode, out)
		}
		subID := int64(nested(out, "submissionId").(float64))
		if v := pollVerdict(t, app, bobCookie, subID); v != "AC" {
			t.Fatalf("test verdict = %s", v)
		}
		// submit → AC 且写进度
		resp, out = doJSON(t, app, "POST", "/api/oj/problem/"+strconv.FormatInt(programmingProblemID, 10)+"/submit", bobCookie, map[string]any{"language": "python", "sourceCode": code})
		subID = int64(nested(out, "submissionId").(float64))
		if v := pollVerdict(t, app, bobCookie, subID); v != "AC" {
			t.Fatalf("submit verdict = %s", v)
		}
		// run：自定义输入 → OK + stdout 含 12
		resp, out = doJSON(t, app, "POST", "/api/oj/problem/"+strconv.FormatInt(programmingProblemID, 10)+"/run", bobCookie, map[string]any{"language": "python", "sourceCode": code, "inputData": "5 7"})
		subID = int64(nested(out, "submissionId").(float64))
		runRes := pollResult(t, app, bobCookie, subID)
		if runRes["verdict"] != "OK" || !strings.Contains(runRes["stdout"].(string), "12") {
			t.Fatalf("run = %v", runRes)
		}
		// WA 提交
		waCode := "a, b = map(int, input().split())\nprint(a - b)\n"
		resp, out = doJSON(t, app, "POST", "/api/oj/problem/"+strconv.FormatInt(programmingProblemID, 10)+"/submit", bobCookie, map[string]any{"language": "python", "sourceCode": waCode})
		subID = int64(nested(out, "submissionId").(float64))
		if v := pollVerdict(t, app, bobCookie, subID); v != "WA" {
			t.Fatalf("WA verdict = %s", v)
		}
		// 历史 ≥ 4（test + submit + run + submit）
		_, out = doJSON(t, app, "GET", "/api/oj/problem/"+strconv.FormatInt(programmingProblemID, 10)+"/submissions", bobCookie, nil)
		if len(out["submissions"].([]any)) < 4 {
			t.Fatalf("提交历史 = %v", out)
		}
		// 完成态 = true（AC 后）
		_, out = doJSON(t, app, "GET", "/api/oj/training/"+strconv.FormatInt(trainingAssignID, 10), bobCookie, nil)
		if nested(out, "chapters.0.items.0.completed") != true {
			t.Fatalf("编程题完成态 = %v", out)
		}
		// 列表 accepted 计数
		_, out = doJSON(t, app, "GET", "/api/oj/assigned", bobCookie, nil)
		if nested(out, "trainings.0.accepted").(float64) != 3 {
			t.Fatalf("accepted 计数 = %v", out)
		}
	} // end pyOK

	// 管理统计
	resp, out = doJSON(t, app, "GET", "/api/admin/assignments/"+strconv.FormatInt(trainingAssignID, 10)+"/stats", adminCookie, nil)
	if resp.StatusCode != 200 || len(out["problems"].([]any)) != 3 {
		t.Fatalf("统计 = %d %v", resp.StatusCode, out)
	}
	if nested(out, "problems.1.accepted").(float64) != 1 { // 单选 X：bob AC
		t.Fatalf("单选统计 = %v", out)
	}
	if nested(out, "problems.0.submissions").(float64) != 2 { // A+B：test+submit+run+WA submit 中 submit 2 次
		t.Fatalf("编程题提交统计 = %v", out)
	}
	if nested(out, "totalStudents").(float64) != 1 { // 定向 bob
		t.Fatalf("学生集 = %v", out)
	}

	// 学生越权 403
	if resp, _ := doJSON(t, app, "GET", "/api/admin/assignments", bobCookie, nil); resp.StatusCode != 403 {
		t.Fatal("学生访问布置管理应 403")
	}
	// 学生不可见他人布置的题目（alice 不可见 bob 的训练题目）
	if resp, _ := doJSON(t, app, "GET", "/api/oj/problem/"+strconv.FormatInt(programmingProblemID, 10), aliceCookie, nil); resp.StatusCode != 404 {
		t.Fatal("alice 应不可见 bob 训练的题目")
	}
	// 删除布置 → 不可见
	if resp, _ := doJSON(t, app, "DELETE", "/api/admin/assignments/"+strconv.FormatInt(trainingAssignID, 10), adminCookie, nil); resp.StatusCode != 204 {
		t.Fatal("删除布置失败")
	}
	_, out = doJSON(t, app, "GET", "/api/oj/assigned", bobCookie, nil)
	if len(out["trainings"].([]any)) != 0 {
		t.Fatalf("删除后训练应不可见 = %v", out)
	}
}
