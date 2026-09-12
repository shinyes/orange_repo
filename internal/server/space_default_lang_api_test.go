// 空间默认编程语言（spaces.default_lang）管理端接口：
// PATCH /api/admin/spaces/:id 支持 name/defaultLang 部分更新（非法取值 400），
// GET /api/admin/spaces 回传 defaultLang（''=未设置）。
package server

import (
	"strconv"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// spaceEntry 从管理端空间列表中取指定空间条目（global_admin 须带 domainId）。
func spaceEntry(t *testing.T, app *fiber.App, cookie string, domainID, spaceID int64) map[string]any {
	t.Helper()
	resp, out := doJSON(t, app, "GET", "/api/admin/spaces?domainId="+strconv.FormatInt(domainID, 10), cookie, nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("GET /api/admin/spaces = %d %v", resp.StatusCode, out)
	}
	raw, ok := out["spaces"].([]any)
	if !ok {
		t.Fatalf("spaces 字段类型异常: %v", out)
	}
	for _, v := range raw {
		m, ok := v.(map[string]any)
		if !ok {
			t.Fatalf("spaces 元素类型异常: %T (%v)", v, v)
		}
		if id, ok := m["id"].(float64); ok && int64(id) == spaceID {
			return m
		}
	}
	t.Fatalf("空间列表未含空间 %d: %v", spaceID, raw)
	return nil
}

// defaultLangOf 取空间条目的 defaultLang（键缺失即失败：前端依赖该键）。
func defaultLangOf(t *testing.T, entry map[string]any) string {
	t.Helper()
	v, ok := entry["defaultLang"]
	if !ok {
		t.Fatalf("空间条目缺少 defaultLang 键: %v", entry)
	}
	s, ok := v.(string)
	if !ok {
		t.Fatalf("defaultLang 类型 = %T (%v), want string", v, v)
	}
	return s
}

// TestSpaceDefaultLangAPI 空间默认编程语言：部分更新、校验与列表下发。
func TestSpaceDefaultLangAPI(t *testing.T) {
	app, _ := newTestApp(t)
	gc := sessionCookie(t, app) // global_admin

	_, dOut := doJSON(t, app, "POST", "/api/admin/domains", gc, map[string]any{"name": "语言域"})
	domainID := int64(dOut["id"].(float64))
	_, spOut := doJSON(t, app, "POST", "/api/admin/spaces?domainId="+strconv.FormatInt(domainID, 10), gc,
		map[string]string{"name": "语言空间"})
	spaceID := int64(spOut["id"].(float64))
	if spaceID == 0 {
		t.Fatal("space id = 0")
	}
	patch := "/api/admin/spaces/" + strconv.FormatInt(spaceID, 10)

	// 1) 新建空间：未设置（""；键存在，前端可读）
	if got := defaultLangOf(t, spaceEntry(t, app, gc, domainID, spaceID)); got != "" {
		t.Fatalf("新建空间 defaultLang = %q, want ''", got)
	}

	// 2) 仅 defaultLang：204；列表刷新后生效，名称不受影响
	resp, _ := doJSON(t, app, "PATCH", patch, gc, map[string]string{"defaultLang": "cpp"})
	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("PATCH defaultLang=cpp = %d, want 204", resp.StatusCode)
	}
	entry := spaceEntry(t, app, gc, domainID, spaceID)
	if got := defaultLangOf(t, entry); got != "cpp" {
		t.Fatalf("PATCH 后 defaultLang = %q, want cpp", got)
	}
	if entry["name"] != "语言空间" {
		t.Fatalf("仅改 defaultLang 时名称被改: %v", entry["name"])
	}

	// 3) 非法取值：400 且库中值不变
	for _, bad := range []string{"java", "Python", "c++", "py"} {
		resp, _ = doJSON(t, app, "PATCH", patch, gc, map[string]string{"defaultLang": bad})
		if resp.StatusCode != fiber.StatusBadRequest {
			t.Fatalf("PATCH defaultLang=%q = %d, want 400", bad, resp.StatusCode)
		}
	}
	if got := defaultLangOf(t, spaceEntry(t, app, gc, domainID, spaceID)); got != "cpp" {
		t.Fatalf("非法更新后 defaultLang = %q, want cpp（保持原值）", got)
	}

	// 4) 旧调用兼容（只传 name）：改名成功且 defaultLang 保持
	resp, _ = doJSON(t, app, "PATCH", patch, gc, map[string]string{"name": "语言空间II"})
	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("PATCH name only = %d, want 204", resp.StatusCode)
	}
	entry = spaceEntry(t, app, gc, domainID, spaceID)
	if entry["name"] != "语言空间II" || defaultLangOf(t, entry) != "cpp" {
		t.Fatalf("仅改名后条目 = %v, want name=语言空间II defaultLang=cpp", entry)
	}

	// 5) name + defaultLang 同时更新
	resp, _ = doJSON(t, app, "PATCH", patch, gc, map[string]string{"name": "语言空间III", "defaultLang": "python"})
	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("PATCH name+defaultLang = %d, want 204", resp.StatusCode)
	}
	entry = spaceEntry(t, app, gc, domainID, spaceID)
	if entry["name"] != "语言空间III" || defaultLangOf(t, entry) != "python" {
		t.Fatalf("双改后条目 = %v, want name=语言空间III defaultLang=python", entry)
	}

	// 6) 恢复未设置：'' 合法（沿用 python）
	resp, _ = doJSON(t, app, "PATCH", patch, gc, map[string]string{"defaultLang": ""})
	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("PATCH defaultLang='' = %d, want 204", resp.StatusCode)
	}
	if got := defaultLangOf(t, spaceEntry(t, app, gc, domainID, spaceID)); got != "" {
		t.Fatalf("恢复未设置后 defaultLang = %q, want ''", got)
	}

	// 7) 空请求体 / 空名称：400
	if resp, _ = doJSON(t, app, "PATCH", patch, gc, map[string]string{}); resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("PATCH {} = %d, want 400", resp.StatusCode)
	}
	if resp, _ = doJSON(t, app, "PATCH", patch, gc, map[string]string{"name": "  "}); resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("PATCH name 空白 = %d, want 400", resp.StatusCode)
	}

	// 8) 空间不存在：404
	resp, _ = doJSON(t, app, "PATCH", "/api/admin/spaces/"+strconv.FormatInt(spaceID+999, 10), gc,
		map[string]string{"defaultLang": "cpp"})
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("PATCH 不存在空间 = %d, want 404", resp.StatusCode)
	}
}
