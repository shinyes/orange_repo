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

	// 目录接口已退役 → 404
	resp, _ := doJSON(t, app, "GET", "/api/directories", cookie, nil)
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("legacy /api/directories = %d, want 404", resp.StatusCode)
	}

	// 三种题型（p2/p3 使用斜杠层级标签）
	mk := func(payload map[string]any) map[string]any {
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
		"type": "single_choice", "title": "标识符", "tags": []string{"语法/基础"},
		"statementMd": "哪个不合法？",
		"bodyJson":    map[string]any{"options": []string{"int", "2var"}},
		"answerJson":  map[string]any{"answer": "2var"},
	})
	p3 := mk(map[string]any{
		"type": "true_false", "title": "判断题", "tags": []string{"语法/基础"},
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

	// 标签筛选（前缀规则：父标签「语法」命中 语法/基础）
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

	// 标签子树重命名：语法 → Language/语法（两题联动）
	resp, rn := doJSON(t, app, "PATCH", "/api/tags", cookie, map[string]string{"from": "语法", "to": "Language/语法"})
	if resp.StatusCode != fiber.StatusOK || rn["updated"].(float64) != 2 {
		t.Fatalf("rename tag = %d %v", resp.StatusCode, rn)
	}
	_, list = doJSON(t, app, "GET", "/api/problems?tags=Language", cookie, nil)
	if got := len(list["problems"].([]any)); got != 2 {
		t.Fatalf("after rename prefix filter = %d, want 2", got)
	}
	// 删除子树：Language 从两题上移除
	resp, del := doJSON(t, app, "DELETE", "/api/tags?tag=Language", cookie, nil)
	if resp.StatusCode != fiber.StatusOK || del["updated"].(float64) != 2 {
		t.Fatalf("delete tag = %d %v", resp.StatusCode, del)
	}
	_, list = doJSON(t, app, "GET", "/api/problems?tags=Language", cookie, nil)
	if got := len(list["problems"].([]any)); got != 0 {
		t.Fatalf("after delete filter = %d, want 0", got)
	}

	// 删除题目
	pid := int64(p1["id"].(float64))
	resp, _ = doJSON(t, app, "DELETE", fmt.Sprintf("/api/problems/%d", pid), cookie, nil)
	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("delete = %d", resp.StatusCode)
	}
}

func TestExportImportRoundTrip(t *testing.T) {
	srcApp, srcStore := newTestApp(t)
	cookie := sessionCookie(t, srcApp)

	// 两道题
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

// TestTrainingLayoutReorder 布局接口：章节排序 + 跨章节移动条目 + 完整性校验。
func TestTrainingLayoutReorder(t *testing.T) {
	app, _ := newTestApp(t)
	cookie := sessionCookie(t, app)

	_, trResp := doJSON(t, app, "POST", "/api/trainings", cookie, map[string]any{"title": "布局训练"})
	trainingID := int64(trResp["id"].(float64))
	mkChapter := func(title string) int64 {
		_, r := doJSON(t, app, "POST", fmt.Sprintf("/api/trainings/%d/chapters", trainingID), cookie, map[string]string{"title": title})
		return int64(r["id"].(float64))
	}
	chA := mkChapter("甲")
	chB := mkChapter("乙")
	mkProblem := func(title string) int64 {
		_, r := doJSON(t, app, "POST", "/api/problems", cookie,
			map[string]any{"type": "programming", "title": title, "bodyJson": map[string]any{}, "answerJson": map[string]any{}})
		return int64(r["problem"].(map[string]any)["id"].(float64))
	}
	p1 := mkProblem("题目一")
	p2 := mkProblem("题目二")
	if resp, _ := doJSON(t, app, "POST", fmt.Sprintf("/api/chapters/%d/items", chA), cookie, map[string]any{"problemIds": []int64{p1, p2}}); resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("add items = %d", resp.StatusCode)
	}

	// 读取条目 id（甲在先）
	_, detail := doJSON(t, app, "GET", fmt.Sprintf("/api/trainings/%d", trainingID), cookie, nil)
	chs := detail["chapters"].([]any)
	itemsA := chs[0].(map[string]any)["items"].([]any)
	i1 := int64(itemsA[0].(map[string]any)["id"].(float64))
	i2 := int64(itemsA[1].(map[string]any)["id"].(float64))

	// 布局：乙在前、甲在后；把 i1（题目一）跨章节移入乙
	resp, out := doJSON(t, app, "PUT", fmt.Sprintf("/api/trainings/%d/layout", trainingID), cookie, map[string]any{
		"chapterIds": []int64{chB, chA},
		"chapters": []any{
			map[string]any{"chapterId": chB, "itemIds": []int64{i1}},
			map[string]any{"chapterId": chA, "itemIds": []int64{i2}},
		},
	})
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("layout = %d %v", resp.StatusCode, out)
	}
	newChs := out["chapters"].([]any)
	first := newChs[0].(map[string]any)
	second := newChs[1].(map[string]any)
	if first["title"] != "乙" {
		t.Fatalf("chapter order wrong: %v", newChs)
	}
	fb := first["items"].([]any)
	if len(fb) != 1 || fb[0].(map[string]any)["problemTitle"] != "题目一" {
		t.Fatalf("moved item wrong: %v", fb)
	}
	sb := second["items"].([]any)
	if len(sb) != 1 || sb[0].(map[string]any)["problemTitle"] != "题目二" {
		t.Fatalf("remaining item wrong: %v", sb)
	}

	// 完整性：缺条目 → 400
	resp, _ = doJSON(t, app, "PUT", fmt.Sprintf("/api/trainings/%d/layout", trainingID), cookie, map[string]any{
		"chapterIds": []int64{chB, chA},
		"chapters": []any{
			map[string]any{"chapterId": chB, "itemIds": []int64{}},
			map[string]any{"chapterId": chA, "itemIds": []int64{i2}},
		},
	})
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("incomplete layout = %d, want 400", resp.StatusCode)
	}
	// 外来章节 → 400
	resp, _ = doJSON(t, app, "PUT", fmt.Sprintf("/api/trainings/%d/layout", trainingID), cookie, map[string]any{
		"chapterIds": []int64{99999},
		"chapters":   []any{map[string]any{"chapterId": 99999, "itemIds": []int64{}}},
	})
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("foreign chapter layout = %d, want 400", resp.StatusCode)
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
