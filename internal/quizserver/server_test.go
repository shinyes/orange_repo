package quizserver

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"orangerepo/internal/model"
	"orangerepo/internal/quizstore"
	"orangerepo/internal/store"
)

// newTestQuizApp 建立临时环境：主库（含样例题目）→ 刷题服务（bootstrap 管理员）。
func newTestQuizApp(t *testing.T) *fiber.App {
	t.Helper()
	dir := t.TempDir()
	main, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open main store: %v", err)
	}
	for _, p := range []model.Problem{
		{Type: model.TypeSingleChoice, Title: "单选A", Tags: []string{"数学"},
			StatementMD: "1+1=?", BodyJSON: json.RawMessage(`{"options":["1","2","3","4"]}`),
			AnswerJSON: json.RawMessage(`{"answerIndex":1}`), Solutions: json.RawMessage(`[]`)},
		{Type: model.TypeSingleChoice, Title: "单选B", Tags: []string{"数学"},
			StatementMD: "重力方向?", BodyJSON: json.RawMessage(`{"options":["向上","向下"]}`),
			AnswerJSON: json.RawMessage(`{"answerIndex":1}`), Solutions: json.RawMessage(`[]`)},
		{Type: model.TypeTrueFalse, Title: "判断A", Tags: []string{"数学"},
			StatementMD: "自由落体加速度为 g", BodyJSON: json.RawMessage(`{}`),
			AnswerJSON: json.RawMessage(`{"answer":true}`), Solutions: json.RawMessage(`[]`)},
	} {
		if _, err := main.CreateProblem(p); err != nil {
			t.Fatalf("seed problem: %v", err)
		}
	}
	if err := main.Close(); err != nil {
		t.Fatalf("close main store: %v", err)
	}
	qs, err := quizstore.Open(dir, filepath.Join(dir, "orangerepo.db"))
	if err != nil {
		t.Fatalf("open quiz store: %v", err)
	}
	t.Cleanup(func() { _ = qs.Close() })
	srv := &Server{QS: qs, UploadsDir: filepath.Join(dir, "uploads")}
	if !srv.EnsureBootstrap() {
		t.Fatal("bootstrap 管理员失败")
	}
	return New(qs, srv.UploadsDir, "")
}

// doJSON 发送 JSON 请求（cookie 可空），返回响应与解析后的响应体。
func doJSON(t *testing.T, app *fiber.App, method, path, cookie string, body any) (*http.Response, map[string]any) {
	t.Helper()
	var req *http.Request
	if body != nil {
		b, _ := json.Marshal(body)
		req = httptest.NewRequest(method, path, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	var out map[string]any
	if resp.Body != nil {
		raw, _ := io.ReadAll(resp.Body)
		if len(raw) > 0 && strings.Contains(resp.Header.Get("Content-Type"), "json") {
			_ = json.Unmarshal(raw, &out)
		}
	}
	return resp, out
}

// cookieOf 从登录响应提取会话 cookie 片段。
func cookieOf(resp *http.Response) string {
	sc := resp.Header.Get("Set-Cookie")
	return strings.SplitN(sc, ";", 2)[0]
}

// nested 读取嵌套响应字段（如 "subjects.0.categories.0.questionCount"）。
func nested(m map[string]any, path string) any {
	cur := any(m)
	for _, part := range strings.Split(path, ".") {
		if part == "" {
			continue
		}
		switch v := cur.(type) {
		case map[string]any:
			cur = v[part]
		case []any:
			idx := 0
			for i := 0; i < len(part); i++ {
				if part[i] < '0' || part[i] > '9' {
					return nil
				}
				idx = idx*10 + int(part[i]-'0')
			}
			if idx >= len(v) {
				return nil
			}
			cur = v[idx]
		default:
			return nil
		}
	}
	return cur
}

func TestQuizServiceFullFlow(t *testing.T) {
	app := newTestQuizApp(t)

	// 未登录：me 返回未认证；受保护接口 401
	_, out := doJSON(t, app, "GET", "/api/auth/me", "", nil)
	if nested(out, "authenticated") != false {
		t.Fatalf("未登录 me = %v", out)
	}
	if resp, _ := doJSON(t, app, "GET", "/api/quiz/subjects", "", nil); resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("未登录访问 subjects = %d, want 401", resp.StatusCode)
	}

	// 管理员登录（错误密码拒绝）
	if resp, _ := doJSON(t, app, "POST", "/api/auth/login", "", map[string]string{"username": "admin", "password": "wrong"}); resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatal("错误密码登录应 401")
	}
	loginResp, _ := doJSON(t, app, "POST", "/api/auth/login", "", map[string]string{"username": "admin", "password": "123456"})
	if loginResp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("管理员登录 = %d", loginResp.StatusCode)
	}
	adminCookie := cookieOf(loginResp)
	_, out = doJSON(t, app, "GET", "/api/auth/me", adminCookie, nil)
	if nested(out, "user.role") != "admin" {
		t.Fatalf("管理员 me = %v", out)
	}

	// 管理员：科目 + 分类（标签/题型映射）+ 参数校验
	resp, out := doJSON(t, app, "POST", "/api/admin/subjects", adminCookie, map[string]string{"name": "数学"})
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("建科目 = %d %v", resp.StatusCode, out)
	}
	subjectID := int64(nested(out, "id").(float64))
	resp, out = doJSON(t, app, "POST", "/api/admin/categories", adminCookie, map[string]any{
		"subjectId": subjectID, "name": "代数", "tags": []string{"数学"}, "types": []string{"single_choice"},
	})
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("建分类 = %d %v", resp.StatusCode, out)
	}
	categoryID := int64(nested(out, "id").(float64))
	if resp, _ := doJSON(t, app, "POST", "/api/admin/categories", adminCookie, map[string]any{
		"subjectId": subjectID, "name": "非法", "types": []string{"multi_choice"},
	}); resp.StatusCode != fiber.StatusBadRequest {
		t.Fatal("非法题型应 400")
	}
	if resp, _ := doJSON(t, app, "POST", "/api/admin/categories", adminCookie, map[string]any{
		"subjectId": subjectID, "name": "非法", "tags": []string{"/bad"},
	}); resp.StatusCode != fiber.StatusBadRequest {
		t.Fatal("非法标签应 400")
	}

	// 管理员：学生账号（重复用户名 409）
	resp, out = doJSON(t, app, "POST", "/api/admin/students", adminCookie, map[string]string{"username": "bob", "password": "pw"})
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("建学生 = %d %v", resp.StatusCode, out)
	}
	bobID := int64(nested(out, "id").(float64))
	if resp, _ := doJSON(t, app, "POST", "/api/admin/students", adminCookie, map[string]string{"username": "BOB", "password": "pw2"}); resp.StatusCode != fiber.StatusConflict {
		t.Fatal("重复用户名应 409")
	}
	if resp, _ := doJSON(t, app, "POST", "/api/admin/students", adminCookie, map[string]string{"username": "carol", "password": "pw"}); resp.StatusCode != fiber.StatusCreated {
		t.Fatal("建第二个学生失败")
	}

	// 管理员：全局每轮题数（1 道便于断言；越界拒绝）
	if resp, _ := doJSON(t, app, "PUT", "/api/admin/settings", adminCookie, map[string]int{"roundSize": 1}); resp.StatusCode != fiber.StatusNoContent {
		t.Fatal("设置每轮题数失败")
	}
	if resp, _ := doJSON(t, app, "PUT", "/api/admin/settings", adminCookie, map[string]int{"roundSize": 200}); resp.StatusCode != fiber.StatusBadRequest {
		t.Fatal("越界每轮题数应 400")
	}

	// 管理员视图：分类实时题目数 = 2（单选 ×2；判断题被题型过滤）
	_, out = doJSON(t, app, "GET", "/api/admin/subjects", adminCookie, nil)
	if nested(out, "subjects.0.categories.0.questionCount").(float64) != 2 {
		t.Fatalf("管理员题目数 = %v", out)
	}

	// 学生：登录、选题列表、抽题（随机 1 道 / total 2）
	loginResp, _ = doJSON(t, app, "POST", "/api/auth/login", "", map[string]string{"username": "bob", "password": "pw"})
	if loginResp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("学生登录 = %d", loginResp.StatusCode)
	}
	bobCookie := cookieOf(loginResp)
	_, out = doJSON(t, app, "GET", "/api/quiz/subjects", bobCookie, nil)
	if nested(out, "subjects.0.categories.0.questionCount").(float64) != 2 {
		t.Fatalf("学生题目数 = %v", out)
	}
	resp, out = doJSON(t, app, "POST", "/api/quiz/round", bobCookie, map[string]int64{"categoryId": categoryID})
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("抽题 = %d %v", resp.StatusCode, out)
	}
	if nested(out, "total").(float64) != 2 || len(nested(out, "problems").([]any)) != 1 {
		t.Fatalf("抽题结果 = %v", out)
	}
	problem := nested(out, "problems.0").(map[string]any)
	problemID := int64(problem["id"].(float64))
	if problem["hasExplanation"] != false {
		t.Fatal("无题解题目 hasExplanation 应为 false")
	}
	if problem["statementMd"] == "" {
		t.Fatal("抽题应含题面")
	}

	// 学生：答错 → 入错题集；答对 → 移除；两个学生互相隔离
	resp, out = doJSON(t, app, "POST", "/api/quiz/submit", bobCookie, map[string]any{
		"problemId": problemID, "categoryId": categoryID, "optionIndex": 0,
	})
	if resp.StatusCode != fiber.StatusOK || nested(out, "correct") != false {
		t.Fatalf("答错提交 = %d %v", resp.StatusCode, out)
	}
	_, out = doJSON(t, app, "GET", "/api/quiz/wrong-summary", bobCookie, nil)
	if nested(out, "total").(float64) != 1 {
		t.Fatalf("错题总数 = %v", out)
	}
	if nested(out, "groups.0.categoryName") != "代数" || nested(out, "groups.0.subjectName") != "数学" || nested(out, "groups.0.count").(float64) != 1 {
		t.Fatalf("错题分组 = %v", out)
	}
	// carol 隔离
	loginResp, _ = doJSON(t, app, "POST", "/api/auth/login", "", map[string]string{"username": "carol", "password": "pw"})
	carolCookie := cookieOf(loginResp)
	_, out = doJSON(t, app, "GET", "/api/quiz/wrong-summary", carolCookie, nil)
	if nested(out, "total").(float64) != 0 {
		t.Fatalf("carol 错题应隔离 = %v", out)
	}
	// bob 答对移除
	resp, out = doJSON(t, app, "POST", "/api/quiz/submit", bobCookie, map[string]any{
		"problemId": problemID, "categoryId": categoryID, "optionIndex": 1,
	})
	if nested(out, "correct") != true {
		t.Fatalf("答对提交 = %v", out)
	}
	_, out = doJSON(t, app, "GET", "/api/quiz/wrong-summary", bobCookie, nil)
	if nested(out, "total").(float64) != 0 {
		t.Fatalf("答对后错题总数 = %v", out)
	}
	// 错题练习为空集
	resp, out = doJSON(t, app, "POST", "/api/quiz/wrong-round", bobCookie, map[string]any{})
	if resp.StatusCode != fiber.StatusOK || len(nested(out, "problems").([]any)) != 0 {
		t.Fatalf("空错题练习 = %d %v", resp.StatusCode, out)
	}

	// 学生越权：管理员接口 403
	if resp, _ := doJSON(t, app, "GET", "/api/admin/students", bobCookie, nil); resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("学生访问管理接口 = %d, want 403", resp.StatusCode)
	}

	// 判断题判题路径（第二分类）
	resp, out = doJSON(t, app, "POST", "/api/admin/categories", adminCookie, map[string]any{
		"subjectId": subjectID, "name": "概念题", "tags": []string{"数学"}, "types": []string{"true_false"},
	})
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("建判断题分类 = %d %v", resp.StatusCode, out)
	}
	tfCategoryID := int64(nested(out, "id").(float64))
	_, out = doJSON(t, app, "POST", "/api/quiz/round", bobCookie, map[string]int64{"categoryId": tfCategoryID})
	tfProblem := nested(out, "problems.0").(map[string]any)
	tfID := int64(tfProblem["id"].(float64))
	resp, out = doJSON(t, app, "POST", "/api/quiz/submit", bobCookie, map[string]any{
		"problemId": tfID, "categoryId": tfCategoryID, "answer": false,
	})
	if nested(out, "correct") != false {
		t.Fatalf("判断题答错 = %v", out)
	}
	resp, out = doJSON(t, app, "POST", "/api/quiz/submit", bobCookie, map[string]any{
		"problemId": tfID, "categoryId": tfCategoryID, "answer": true,
	})
	if nested(out, "correct") != true || nested(out, "correctAnswer").(map[string]any)["answer"] != true {
		t.Fatalf("判断题答对 = %v", out)
	}

	// 学生重置密码与删除
	loginResp, out = doJSON(t, app, "PUT", "/api/admin/students/"+strconv.FormatInt(bobID, 10)+"/password", adminCookie, map[string]string{"password": "pw-new"})
	if loginResp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("重置密码 = %d %v", loginResp.StatusCode, out)
	}
	if resp, _ := doJSON(t, app, "POST", "/api/auth/login", "", map[string]string{"username": "bob", "password": "pw-new"}); resp.StatusCode != fiber.StatusNoContent {
		t.Fatal("新密码登录失败")
	}
	if resp, _ := doJSON(t, app, "DELETE", "/api/admin/students/"+strconv.FormatInt(bobID, 10), adminCookie, nil); resp.StatusCode != fiber.StatusNoContent {
		t.Fatal("删除学生失败")
	}
	if resp, _ := doJSON(t, app, "POST", "/api/auth/login", "", map[string]string{"username": "bob", "password": "pw-new"}); resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatal("已删除学生应无法登录")
	}

	// 学生改密码（自我修改）
	loginResp, _ = doJSON(t, app, "POST", "/api/auth/login", "", map[string]string{"username": "carol", "password": "pw"})
	carolCookie = cookieOf(loginResp)
	if resp, _ := doJSON(t, app, "PUT", "/api/auth/password", carolCookie, map[string]string{"oldPassword": "bad", "newPassword": "x"}); resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatal("原密码错误应 401")
	}
	resp, _ = doJSON(t, app, "PUT", "/api/auth/password", carolCookie, map[string]string{"oldPassword": "pw", "newPassword": "pw2"})
	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("改密码 = %d", resp.StatusCode)
	}
	if resp, _ := doJSON(t, app, "POST", "/api/auth/login", "", map[string]string{"username": "carol", "password": "pw2"}); resp.StatusCode != fiber.StatusNoContent {
		t.Fatal("新密码登录失败")
	}

	// problems-count 预览
	resp, out = doJSON(t, app, "GET", "/api/admin/problems-count?tags=%E6%95%B0%E5%AD%A6&types=single_choice", adminCookie, nil)
	if resp.StatusCode != fiber.StatusOK || nested(out, "count").(float64) != 2 {
		t.Fatalf("题目预览 = %d %v", resp.StatusCode, out)
	}
}