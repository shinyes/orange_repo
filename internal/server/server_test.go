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
	"orangerepo/internal/zipio"
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
	// 标签 facet 端点：基底=全部题；选中「语法」后 total=2（与题目列表一致），
	// 各标签仍显示自己实际命中的题数（语法=2、入门=1），计数不受选中集影响
	resp, ft := doJSON(t, app, "GET", "/api/tags?tags=%E8%AF%AD%E6%B3%95", cookie, nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("tags facet = %d %v", resp.StatusCode, ft)
	}
	fm := map[string]float64{}
	for _, item := range ft["tags"].([]any) {
		tc := item.(map[string]any)
		fm[tc["tag"].(string)] = tc["count"].(float64)
	}
	if fm["语法"] != 2 || fm["入门"] != 1 || int(ft["total"].(float64)) != 2 {
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

// TestNoneTagGuard 无标签哨兵的重命名/删除守卫。
func TestNoneTagGuard(t *testing.T) {
	app, _ := newTestApp(t)
	cookie := sessionCookie(t, app)

	resp, _ := doJSON(t, app, "PATCH", "/api/tags", cookie, map[string]any{"from": "__none__", "to": "xxx"})
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("rename __none__ = %d, want 400", resp.StatusCode)
	}
	resp, _ = doJSON(t, app, "PATCH", "/api/tags", cookie, map[string]any{"from": "a", "to": "__none__"})
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("rename to __none__ = %d, want 400", resp.StatusCode)
	}
	resp, _ = doJSON(t, app, "DELETE", "/api/tags?tag=__none__", cookie, nil)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("delete __none__ = %d, want 400", resp.StatusCode)
	}
}

// TestTagOrderSettings 手动排序持久化往返。
func TestTagOrderSettings(t *testing.T) {
	app, _ := newTestApp(t)
	cookie := sessionCookie(t, app)

	resp, out := doJSON(t, app, "GET", "/api/tag-order", cookie, nil)
	if resp.StatusCode != fiber.StatusOK || len(out["order"].(map[string]any)) != 0 {
		t.Fatalf("initial tag-order = %d %v", resp.StatusCode, out)
	}

	resp, _ = doJSON(t, app, "PUT", "/api/tag-order", cookie, map[string]any{
		"order": map[string]any{"": []string{"算法", "数学"}, "数学": []string{"代数", "几何"}},
	})
	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("set tag-order = %d", resp.StatusCode)
	}

	resp, out = doJSON(t, app, "GET", "/api/tag-order", cookie, nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("get tag-order = %d", resp.StatusCode)
	}
	order := out["order"].(map[string]any)
	roots := order[""].([]any)
	if len(roots) != 2 || roots[0] != "算法" || roots[1] != "数学" {
		t.Fatalf("order round-trip wrong: %v", order)
	}

	// 非法请求体 → 400
	resp, _ = doJSON(t, app, "PUT", "/api/tag-order", cookie, map[string]any{"wrong": 1})
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("invalid order body = %d, want 400", resp.StatusCode)
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

// TestBookletDirectoriesAPI 题册目录路由：嵌套创建、重命名、题册移动、删除提升。
func TestBookletDirectoriesAPI(t *testing.T) {
	app, _ := newTestApp(t)
	cookie := sessionCookie(t, app)

	// 初始为空
	resp, out := doJSON(t, app, "GET", "/api/booklet-directories", cookie, nil)
	if resp.StatusCode != fiber.StatusOK || len(out["directories"].([]any)) != 0 {
		t.Fatalf("initial dirs = %d %v", resp.StatusCode, out)
	}

	// 创建根目录与子目录
	resp, out = doJSON(t, app, "POST", "/api/booklet-directories", cookie, map[string]any{"name": "数学"})
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("create root = %d", resp.StatusCode)
	}
	rootID := int64(out["id"].(float64))
	resp, out = doJSON(t, app, "POST", "/api/booklet-directories", cookie, map[string]any{"name": "几何", "parentId": rootID})
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("create sub = %d", resp.StatusCode)
	}
	subID := int64(out["id"].(float64))

	// 空名称/幽灵父目录 → 400/404
	resp, _ = doJSON(t, app, "POST", "/api/booklet-directories", cookie, map[string]any{"name": ""})
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("empty name = %d, want 400", resp.StatusCode)
	}
	resp, _ = doJSON(t, app, "POST", "/api/booklet-directories", cookie, map[string]any{"name": "x", "parentId": 99999})
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("ghost parent = %d, want 404", resp.StatusCode)
	}

	// 重命名
	resp, _ = doJSON(t, app, "PATCH", fmt.Sprintf("/api/booklet-directories/%d", rootID), cookie, map[string]string{"name": "数学A"})
	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("rename = %d", resp.StatusCode)
	}

	// 创建训练/练习并移入目录（创建时带 folderId / 单独移动接口）
	resp, tr := doJSON(t, app, "POST", "/api/trainings", cookie, map[string]any{"title": "训练X", "folderId": rootID})
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("create training = %d", resp.StatusCode)
	}
	trainingID := int64(tr["id"].(float64))
	resp, pr := doJSON(t, app, "POST", "/api/practices", cookie, map[string]any{"title": "练习Y"})
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("create practice = %d", resp.StatusCode)
	}
	practiceID := int64(pr["id"].(float64))
	resp, _ = doJSON(t, app, "PUT", fmt.Sprintf("/api/practices/%d/folder", practiceID), cookie, map[string]any{"folderId": subID})
	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("move practice = %d", resp.StatusCode)
	}
	resp, _ = doJSON(t, app, "PUT", fmt.Sprintf("/api/trainings/%d/folder", trainingID), cookie, map[string]any{"folderId": nil})
	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("move training to root = %d", resp.StatusCode)
	}
	// 移动回子目录
	resp, _ = doJSON(t, app, "PUT", fmt.Sprintf("/api/trainings/%d/folder", trainingID), cookie, map[string]any{"folderId": subID})
	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("move training into sub = %d", resp.StatusCode)
	}
	// 练习移入将被删除的根目录（验证删除时的上移一层）
	resp, _ = doJSON(t, app, "PUT", fmt.Sprintf("/api/practices/%d/folder", practiceID), cookie, map[string]any{"folderId": rootID})
	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("move practice into root = %d", resp.StatusCode)
	}
	// 幽灵目录 → 404
	resp, _ = doJSON(t, app, "PUT", fmt.Sprintf("/api/trainings/%d/folder", trainingID), cookie, map[string]any{"folderId": 99999})
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("ghost folder move = %d, want 404", resp.StatusCode)
	}

	// 列表返回 folderId
	_, tl := doJSON(t, app, "GET", "/api/trainings", cookie, nil)
	for _, item := range tl["trainings"].([]any) {
		tr1 := item.(map[string]any)
		if int64(tr1["id"].(float64)) == trainingID {
			if fid, ok := tr1["folderId"].(float64); !ok || int64(fid) != subID {
				t.Fatalf("training folderId = %v, want %d", tr1["folderId"], subID)
			}
		}
	}
	_, pl := doJSON(t, app, "GET", "/api/practices", cookie, nil)
	for _, item := range pl["practices"].([]any) {
		p1 := item.(map[string]any)
		if int64(p1["id"].(float64)) == practiceID {
			if fid, ok := p1["folderId"].(float64); !ok || int64(fid) != rootID {
				t.Fatalf("practice folderId = %v, want %d", p1["folderId"], rootID)
			}
		}
	}

	// 删除「数学A」：其直接子目录「几何」与其中的练习上移一层（父为空=根）；
	// 训练X 在「几何」下不受影响
	resp, _ = doJSON(t, app, "DELETE", fmt.Sprintf("/api/booklet-directories/%d", rootID), cookie, nil)
	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("delete root = %d", resp.StatusCode)
	}
	_, out = doJSON(t, app, "GET", "/api/booklet-directories", cookie, nil)
	dirs := out["directories"].([]any)
	if len(dirs) != 1 {
		t.Fatalf("dirs after delete = %d, want 1", len(dirs))
	}
	kept := dirs[0].(map[string]any)
	if fid, ok := kept["parentId"]; ok && fid != nil {
		t.Fatalf("promoted dir parentId = %v, want null", kept["parentId"])
	}
	_, pl = doJSON(t, app, "GET", "/api/practices", cookie, nil)
	for _, item := range pl["practices"].([]any) {
		p1 := item.(map[string]any)
		if int64(p1["id"].(float64)) == practiceID {
			if _, ok := p1["folderId"]; ok {
				t.Fatalf("practice folderId after delete = %v, want null", p1["folderId"])
			}
		}
	}
	_, tl = doJSON(t, app, "GET", "/api/trainings", cookie, nil)
	for _, item := range tl["trainings"].([]any) {
		tr1 := item.(map[string]any)
		if int64(tr1["id"].(float64)) == trainingID {
			if fid, ok := tr1["folderId"].(float64); !ok || int64(fid) != subID {
				t.Fatalf("training folderId after delete = %v, want %d", tr1["folderId"], subID)
			}
		}
	}

	// deleteBooklets=true：目录连同直接归属题册一起删除
	resp, d2 := doJSON(t, app, "POST", "/api/booklet-directories", cookie, map[string]any{"name": "临时代目录"})
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("create tmp dir = %d", resp.StatusCode)
	}
	tmpDir := int64(d2["id"].(float64))
	resp, pr2 := doJSON(t, app, "POST", "/api/practices", cookie, map[string]any{"title": "待删除练习"})
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("create tmp practice = %d", resp.StatusCode)
	}
	tmpPrac := int64(pr2["id"].(float64))
	resp, _ = doJSON(t, app, "PUT", fmt.Sprintf("/api/practices/%d/folder", tmpPrac), cookie, map[string]any{"folderId": tmpDir})
	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("move tmp practice = %d", resp.StatusCode)
	}
	resp, _ = doJSON(t, app, "DELETE", fmt.Sprintf("/api/booklet-directories/%d?deleteBooklets=true", tmpDir), cookie, nil)
	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("delete dir with booklets = %d", resp.StatusCode)
	}
	_, pl2 := doJSON(t, app, "GET", "/api/practices", cookie, nil)
	for _, item := range pl2["practices"].([]any) {
		if int64(item.(map[string]any)["id"].(float64)) == tmpPrac {
			t.Fatal("deleteBooklets=true 后练习应被删除")
		}
	}
}

// TestImportModeValidation 导入 mode 参数规范化与非法值防御。
func TestImportModeValidation(t *testing.T) {
	app, _ := newTestApp(t)
	cookie := sessionCookie(t, app)

	sendZip := func(mode string) int {
		body := &bytes.Buffer{}
		mw := multipart.NewWriter(body)
		fw, err := mw.CreateFormFile("zip", "p.zip")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write([]byte("not a zip")); err != nil {
			t.Fatal(err)
		}
		mw.Close()
		req := httptest.NewRequest("POST", "/api/import?mode="+mode, body)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req.Header.Set("Cookie", cookie)
		resp, err := app.Test(req, -1)
		if err != nil {
			t.Fatalf("import(%s): %v", mode, err)
		}
		return resp.StatusCode
	}

	// 非法/原始大小写 → 400（不复用 valid mode 静默降级）
	if code := sendZip("Training"); code != fiber.StatusBadRequest {
		t.Fatalf("mode=Training = %d, want 400", code)
	}
	if code := sendZip("badmode"); code != fiber.StatusBadRequest {
		t.Fatalf("mode=badmode = %d, want 400", code)
	}
	// 合法 mode 通过模式校验，随后因 ZIP 非法报 400（非 500）
	if code := sendZip("training"); code != fiber.StatusBadRequest {
		t.Fatalf("mode=training with bad zip = %d, want 400", code)
	}
}

// TestImportTrainingWithoutPlan 训练模式导入无章节结构的 ZIP：
// 缺 trainingPlan.json，或其 chapters 为空 → 自动建「未分组」章节收纳全部题目，题目不再被单独遗弃。
func TestImportTrainingWithoutPlan(t *testing.T) {
	mkEntries := func() []zipio.ExportProblem {
		srcApp, _ := newTestApp(t)
		cookie := sessionCookie(t, srcApp)
		doJSON(t, srcApp, "POST", "/api/problems", cookie, map[string]any{"type": "programming", "title": "题一"})
		doJSON(t, srcApp, "POST", "/api/problems", cookie, map[string]any{"type": "single_choice", "title": "题二"})
		zipData := getZip(t, srcApp, cookie, "/api/export/problems")
		problems, _, _, err := zipio.ParseZip(zipData)
		if err != nil {
			t.Fatalf("parse exported zip: %v", err)
		}
		return problems
	}
	entries := mkEntries()

	type caseSpec struct {
		name     string
		plan     *zipio.PlanMeta
		filename string // 上传文件名（无元数据标题时作为题册名称兜底）
		want     string // 期望的训练标题
		wantCh   int    // 期望章节数
		wantCap  string // 期望章节标题
	}
	cases := []caseSpec{
		{"无 trainingPlan.json", nil, "CIE 2203.zip", "CIE 2203", 1, "未分组"},
		// 用户实测场景：trainingPlan.json 存在但缺少 chapters 字段
		{"trainingPlan 无 chapters", &zipio.PlanMeta{Title: "CIE Python二级 2022年3月Python二级", Tags: []string{"CIE"}}, "ignored.zip", "CIE Python二级 2022年3月Python二级", 1, "未分组"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			zipData, err := zipio.BuildZip(entries, tc.plan, nil)
			if err != nil {
				t.Fatalf("build zip: %v", err)
			}
			dstApp, _ := newTestApp(t)
			dstCookie := sessionCookie(t, dstApp)
			importZipAs(t, dstApp, dstCookie, zipData, "training", tc.filename)

			_, tl := doJSON(t, dstApp, "GET", "/api/trainings", dstCookie, nil)
			trainings := tl["trainings"].([]any)
			if len(trainings) != 1 {
				t.Fatalf("trainings = %d, want 1", len(trainings))
			}
			tr := trainings[0].(map[string]any)
			if tr["title"] != tc.want {
				t.Fatalf("title = %v, want %q", tr["title"], tc.want)
			}
			if tr["problemCount"].(float64) != 2 {
				t.Fatalf("problemCount = %v, want 2", tr["problemCount"])
			}
			trainingID := int64(tr["id"].(float64))
			_, td := doJSON(t, dstApp, "GET", fmt.Sprintf("/api/trainings/%d", trainingID), dstCookie, nil)
			chapters := td["chapters"].([]any)
			if len(chapters) != tc.wantCh {
				t.Fatalf("chapters = %d, want %d", len(chapters), tc.wantCh)
			}
			first := chapters[0].(map[string]any)
			if first["title"] != tc.wantCap {
				t.Fatalf("chapter title = %v, want %q", first["title"], tc.wantCap)
			}
			if items := first["items"].([]any); len(items) != 2 {
				t.Fatalf("items = %d, want 2（全部题目应收录进训练）", len(items))
			}
		})
	}
}

// importZipAs 以指定文件名上传导入（文件名作为题册名称兜底）。
func importZipAs(t *testing.T, app *fiber.App, cookie string, zipData []byte, mode, filename string) map[string]any {
	t.Helper()
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	fw, err := mw.CreateFormFile("zip", filename)
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
		t.Fatalf("import(%s) = %d %s", mode, resp.StatusCode, raw)
	}
	return out
}

// TestImportAutoDetect auto 模式自动识别：含章节结构→训练；否则→练习（平铺），名称取文件名兜底。
func TestImportAutoDetect(t *testing.T) {
	srcApp, _ := newTestApp(t)
	cookie := sessionCookie(t, srcApp)
	doJSON(t, srcApp, "POST", "/api/problems", cookie, map[string]any{"type": "programming", "title": "题一"})
	doJSON(t, srcApp, "POST", "/api/problems", cookie, map[string]any{"type": "single_choice", "title": "题二"})
	zipData := getZip(t, srcApp, cookie, "/api/export/problems")
	entries, _, _, err := zipio.ParseZip(zipData)
	if err != nil {
		t.Fatalf("parse exported zip: %v", err)
	}

	// 场景 1：无 trainingPlan → auto 识别为练习，名称=文件名（去扩展名）
	plainZip, err := zipio.BuildZip(entries, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	dst1, _ := newTestApp(t)
	c1 := sessionCookie(t, dst1)
	out := importZipAs(t, dst1, c1, plainZip, "auto", "CIE 2203 真题.zip")
	if _, ok := out["practiceId"]; !ok {
		t.Fatalf("auto+无plan 应识别为练习: %v", out)
	}
	_, pl := doJSON(t, dst1, "GET", "/api/practices", c1, nil)
	practices := pl["practices"].([]any)
	if len(practices) != 1 {
		t.Fatalf("practices = %d, want 1", len(practices))
	}
	p := practices[0].(map[string]any)
	if p["title"] != "CIE 2203 真题" {
		t.Fatalf("practice title = %v, want 文件名兜底「CIE 2203 真题」", p["title"])
	}
	if p["problemCount"].(float64) != 2 {
		t.Fatalf("practice problemCount = %v, want 2", p["problemCount"])
	}

	// 场景 2：含 chapters → auto 识别为训练并按结构建章，名称=元数据标题
	planZip, err := zipio.BuildZip(entries, &zipio.PlanMeta{
		Title: "带章节的训练",
		Chapters: []zipio.PlanChapter{{Title: "热身", ProblemIDs: []int{0, 1}}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	dst2, _ := newTestApp(t)
	c2 := sessionCookie(t, dst2)
	out = importZipAs(t, dst2, c2, planZip, "auto", "ignored.zip")
	if _, ok := out["trainingId"]; !ok {
		t.Fatalf("auto+有章节 应识别为训练: %v", out)
	}
	_, tl := doJSON(t, dst2, "GET", "/api/trainings", c2, nil)
	trainings := tl["trainings"].([]any)
	if len(trainings) != 1 {
		t.Fatalf("trainings = %d, want 1", len(trainings))
	}
	tr := trainings[0].(map[string]any)
	if tr["title"] != "带章节的训练" {
		t.Fatalf("training title = %v, want 元数据标题", tr["title"])
	}
	trainingID := int64(tr["id"].(float64))
	_, td := doJSON(t, dst2, "GET", fmt.Sprintf("/api/trainings/%d", trainingID), c2, nil)
	chapters := td["chapters"].([]any)
	if len(chapters) != 1 || len(chapters[0].(map[string]any)["items"].([]any)) != 2 {
		t.Fatalf("training chapters wrong: %v", td["chapters"])
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
