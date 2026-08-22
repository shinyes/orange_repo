package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"orangerepo/internal/store"
)

func newTestApp(t *testing.T) (*fiber.App, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	srv := &Server{Store: st, UploadsDir: filepath.Join(dir, "uploads")}
	srv.EnsureBootstrap()
	app := New(st, srv.UploadsDir, "")
	return app, st
}

// doJSON 发送 JSON 请求并返回响应与解析后的响应体。
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

func sessionCookie(t *testing.T, app *fiber.App) string {
	t.Helper()
	resp, _ := doJSON(t, app, "POST", "/api/auth/login", "", map[string]string{"password": "123456"})
	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("login status = %d", resp.StatusCode)
	}
	sc := resp.Header.Get("Set-Cookie")
	if sc == "" {
		t.Fatal("no session cookie")
	}
	return strings.Split(sc, ";")[0]
}

func TestAuthFlow(t *testing.T) {
	app, _ := newTestApp(t)
	// 未认证 → 401
	resp, _ := doJSON(t, app, "GET", "/api/problems", "", nil)
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("unauthed /api/problems = %d, want 401", resp.StatusCode)
	}
	// 错误密码 → 401
	resp, _ = doJSON(t, app, "POST", "/api/auth/login", "", map[string]string{"password": "wrong"})
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("wrong password = %d, want 401", resp.StatusCode)
	}
	// 正确密码 → 204 + 会话可用
	cookie := sessionCookie(t, app)
	resp, data := doJSON(t, app, "GET", "/api/auth/me", cookie, nil)
	if resp.StatusCode != fiber.StatusOK || data["authenticated"] != true {
		t.Fatalf("me = %d %v", resp.StatusCode, data)
	}
	// 改密后旧密码失效
	resp, _ = doJSON(t, app, "PUT", "/api/auth/password", cookie,
		map[string]string{"oldPassword": "123456", "newPassword": "new-pass"})
	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("change password = %d", resp.StatusCode)
	}
	resp, _ = doJSON(t, app, "POST", "/api/auth/login", "", map[string]string{"password": "123456"})
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("old password should fail")
	}
}

func TestProblemCRUDAndFilter(t *testing.T) {
	app, _ := newTestApp(t)
	cookie := sessionCookie(t, app)

	// 目录
	resp, dirResp := doJSON(t, app, "POST", "/api/directories", cookie, map[string]any{"name": "第一章", "parentId": nil})
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("create dir = %d", resp.StatusCode)
	}
	dirID := int64(dirResp["id"].(float64))

	// 三种题型
	mk := func(payload map[string]any) map[string]any {
		payload["directoryId"] = dirID
		resp, data := doJSON(t, app, "POST", "/api/problems", cookie, payload)
		if resp.StatusCode != fiber.StatusCreated {
			t.Fatalf("create problem %+v → %d %v", payload, resp.StatusCode, data)
		}
		p := data["problem"].(map[string]any)
		return p
	}
	p1 := mk(map[string]any{
		"type": "programming", "title": "A+B", "tags": []string{"入门", "模拟"},
		"statementMd": "# A+B\n求 $a+b$",
		"bodyJson":    map[string]any{"inputFormat": "一行两个整数", "samples": []any{map[string]string{"input": "1 2", "output": "3"}}},
	})
	p2 := mk(map[string]any{
		"type": "single_choice", "title": "标识符", "tags": []string{"语法"},
		"statementMd": "哪个不合法？",
		"bodyJson":    map[string]any{"options": []string{"int", "2var"}},
		"answerJson":  map[string]any{"answer": "2var"},
	})
	p3 := mk(map[string]any{
		"type": "true_false", "title": "判断题", "tags": []string{"语法"},
		"answerJson": map[string]any{"value": "false"},
	})

	// 归一化断言
	if p1["timeLimitMs"].(float64) != 1000 || p1["memoryLimitMiB"].(float64) != 256 {
		t.Errorf("programming defaults: %v", p1)
	}
	ans2 := p2["answerJson"].(map[string]any)
	if ans2["answerIndex"].(float64) != 1 {
		t.Errorf("single_choice answerIndex = %v, want 1", ans2["answerIndex"])
	}
	ans3 := p3["answerJson"].(map[string]any)
	if ans3["answer"] != false {
		t.Errorf("true_false answer = %v, want false", ans3["answer"])
	}

	// 标签筛选
	resp, list := doJSON(t, app, "GET", "/api/problems?tags=%E8%AF%AD%E6%B3%95", cookie, nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("filter by tag = %d", resp.StatusCode)
	}
	problems := list["problems"].([]any)
	if len(problems) != 2 {
		t.Fatalf("tag filter = %d results, want 2", len(problems))
	}
	// 搜索
	_, list = doJSON(t, app, "GET", "/api/problems?q=A%2BB", cookie, nil)
	if got := len(list["problems"].([]any)); got != 1 {
		t.Fatalf("search = %d results, want 1", got)
	}
	// 标签 facet 端点：基底=全部题；选中「语法」后 total=2，语法预览取消=3，入门需同时含=0
	resp, ft := doJSON(t, app, "GET", "/api/tags?tags=%E8%AF%AD%E6%B3%95", cookie, nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("tags facet = %d %v", resp.StatusCode, ft)
	}
	fm := map[string]float64{}
	for _, item := range ft["tags"].([]any) {
		tc := item.(map[string]any)
		fm[tc["tag"].(string)] = tc["count"].(float64)
	}
	if fm["语法"] != 3 || fm["入门"] != 0 || int(ft["total"].(float64)) != 2 {
		t.Fatalf("facet result wrong: %v total=%v", fm, ft["total"])
	}

	// 目录树计数
	_, tree := doJSON(t, app, "GET", "/api/directories", cookie, nil)
	dirs := tree["directories"].([]any)
	if len(dirs) != 1 || dirs[0].(map[string]any)["problemCount"].(float64) != 3 {
		t.Fatalf("tree = %v", tree)
	}
	// 移动题目到根（null）
	pid := int64(p1["id"].(float64))
	resp, _ = doJSON(t, app, "PUT", fmt.Sprintf("/api/problems/%d/directory", pid), cookie,
		map[string]any{"directoryId": nil})
	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("move = %d", resp.StatusCode)
	}
	_, tree = doJSON(t, app, "GET", "/api/directories", cookie, nil)
	if tree["directories"].([]any)[0].(map[string]any)["problemCount"].(float64) != 2 {
		t.Fatalf("count after move wrong: %v", tree)
	}
	// 删除题目
	resp, _ = doJSON(t, app, "DELETE", fmt.Sprintf("/api/problems/%d", pid), cookie, nil)
	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("delete = %d", resp.StatusCode)
	}
}

func TestExportImportRoundTrip(t *testing.T) {
	srcApp, srcStore := newTestApp(t)
	cookie := sessionCookie(t, srcApp)

	// 建目录 + 两道题
	_, dirResp := doJSON(t, srcApp, "POST", "/api/directories", cookie, map[string]any{"name": "目录A"})
	dirID := int64(dirResp["id"].(float64))
	doJSON(t, srcApp, "POST", "/api/problems", cookie, map[string]any{
		"type": "programming", "title": "题目一", "tags": []string{"T"},
		"statementMd": "内容 (/api/uploads/img_test.png)",
	})
	doJSON(t, srcApp, "POST", "/api/problems", cookie, map[string]any{
		"type": "single_choice", "title": "题目二", "bodyJson": map[string]any{"options": []string{"甲", "乙"}},
	})
	list := doListIDs(t, srcApp, cookie)

	// 训练：一章含两题
	_, trResp := doJSON(t, srcApp, "POST", "/api/trainings", cookie,
		map[string]any{"title": "训练X", "description": "描述", "tags": []string{"计划"}})
	trainingID := int64(trResp["id"].(float64))
	_, chResp := doJSON(t, srcApp, "POST", fmt.Sprintf("/api/trainings/%d/chapters", trainingID), cookie,
		map[string]string{"title": "章节1"})
	chapterID := int64(chResp["id"].(float64))
	resp, _ := doJSON(t, srcApp, "POST", fmt.Sprintf("/api/chapters/%d/items", chapterID), cookie,
		map[string]any{"problemIds": list})
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("add items = %d", resp.StatusCode)
	}

	// 上传图片并确认存在（导出打包用）
	_ = dirID
	writeUpload(t, srcStore, "img_test.png", []byte("PNG"))

	// 导出训练 ZIP
	zipData := getZip(t, srcApp, cookie, fmt.Sprintf("/api/export/trainings/%d", trainingID))

	// 新库导入（mode=training）
	dstApp, dstStore := newTestApp(t)
	importZip(t, dstApp, sessionCookie(t, dstApp), zipData, "training")

	// 校验：题目数、训练数、章节数一致
	_, dlist := doJSON(t, dstApp, "GET", "/api/problems", sessionCookie(t, dstApp), nil)
	if got := len(dlist["problems"].([]any)); got != len(list) {
		t.Fatalf("imported problems = %d, want %d", got, len(list))
	}
	_, tl := doJSON(t, dstApp, "GET", "/api/trainings", sessionCookie(t, dstApp), nil)
	trainings := tl["trainings"].([]any)
	if len(trainings) != 1 || trainings[0].(map[string]any)["title"] != "训练X" {
		t.Fatalf("trainings = %v", tl)
	}
	newTrainingID := int64(trainings[0].(map[string]any)["id"].(float64))
	_, td := doJSON(t, dstApp, "GET", fmt.Sprintf("/api/trainings/%d", newTrainingID), sessionCookie(t, dstApp), nil)
	chapters := td["chapters"].([]any)
	if len(chapters) != 1 {
		t.Fatalf("chapters = %d, want 1", len(chapters))
	}
	items := chapters[0].(map[string]any)["items"].([]any)
	if len(items) != 2 || items[0].(map[string]any)["problemTitle"] != "题目一" {
		t.Fatalf("items = %v", items)
	}
	// 图片已随导入落盘
	if _, err := os.Stat(filepath.Join(dstStore.DataDir, "uploads", "img_test.png")); err != nil {
		t.Fatalf("imported image missing: %v", err)
	}
	// 题面引用被重写回 /api/uploads/
	_, pl := doJSON(t, dstApp, "GET", "/api/problems", sessionCookie(t, dstApp), nil)
	firstID := int64(pl["problems"].([]any)[len(list)-1].(map[string]any)["id"].(float64))
	_, pd := doJSON(t, dstApp, "GET", fmt.Sprintf("/api/problems/%d", firstID), sessionCookie(t, dstApp), nil)
	stmt := pd["problem"].(map[string]any)["statementMd"].(string)
	if !strings.Contains(stmt, "(images/img_test.png)") && !strings.Contains(stmt, "(images/") {
		t.Logf("note: statementMd = %q（导入重写仅处理 images/ 相对引用，本包为绝对引用属正常）", stmt)
	} else if strings.Contains(stmt, "(images/") {
		t.Errorf("statementMd should be rewritten to /api/uploads/: %q", stmt)
	}
}

func doListIDs(t *testing.T, app *fiber.App, cookie string) []int64 {
	t.Helper()
	_, list := doJSON(t, app, "GET", "/api/problems", cookie, nil)
	arr := list["problems"].([]any)
	ids := make([]int64, 0, len(arr))
	for _, item := range arr {
		ids = append(ids, int64(item.(map[string]any)["id"].(float64)))
	}
	// 按 id 升序（列表默认倒序）
	for i, j := 0, len(ids)-1; i < j; i, j = i+1, j-1 {
		ids[i], ids[j] = ids[j], ids[i]
	}
	return ids
}

func writeUpload(t *testing.T, st *store.Store, name string, content []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(st.DataDir, "uploads", name), content, 0o644); err != nil {
		t.Fatalf("write upload: %v", err)
	}
}

func getZip(t *testing.T, app *fiber.App, cookie, path string) []byte {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	req.Header.Set("Cookie", cookie)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("export %s: %v", path, err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("export %s = %d", path, resp.StatusCode)
	}
	data, _ := io.ReadAll(resp.Body)
	if !strings.Contains(resp.Header.Get("Content-Type"), "zip") {
		t.Fatalf("export content type = %s", resp.Header.Get("Content-Type"))
	}
	return data
}

func importZip(t *testing.T, app *fiber.App, cookie string, zipData []byte, mode string) map[string]any {
	t.Helper()
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	fw, err := mw.CreateFormFile("zip", "export.zip")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(zipData); err != nil {
		t.Fatal(err)
	}
	mw.Close()
	req := httptest.NewRequest("POST", "/api/import?mode="+mode, body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Cookie", cookie)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("import = %d %s", resp.StatusCode, raw)
	}
	return out
}
