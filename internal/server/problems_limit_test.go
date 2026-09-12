package server

import (
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// TestListProblemsLimit 题库列表可选限量：limit 截断返回条数、total 保留截断前总数。
// 供选题弹窗在题库很大时只取前 N 条，避免一次下发数千题导致加载慢与渲染卡顿。
func TestListProblemsLimit(t *testing.T) {
	app, _ := newTestApp(t)
	cookie := sessionCookie(t, app)

	// 建 5 道题
	for i := 1; i <= 5; i++ {
		resp, out := doJSON(t, app, "POST", "/api/problems", cookie, map[string]any{
			"type": "programming", "title": fmt.Sprintf("限量题%d", i),
		})
		if resp.StatusCode != fiber.StatusCreated {
			t.Fatalf("create #%d = %d %v", i, resp.StatusCode, out)
		}
	}

	// 不带 limit：全量 5 条，total 也是 5
	resp, out := doJSON(t, app, "GET", "/api/problems", cookie, nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("list = %d", resp.StatusCode)
	}
	all := out["problems"].([]any)
	if len(all) != 5 {
		t.Fatalf("全量条数 = %d, want 5", len(all))
	}
	if got := int(out["total"].(float64)); got != 5 {
		t.Fatalf("全量 total = %d, want 5", got)
	}

	// limit=2：只回 2 条，total 仍为 5（前端据此提示“共 N 条，仅显示前 M 条”）
	resp, out = doJSON(t, app, "GET", "/api/problems?limit=2", cookie, nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("limit list = %d", resp.StatusCode)
	}
	if got := len(out["problems"].([]any)); got != 2 {
		t.Fatalf("limit=2 返回 %d 条, want 2", got)
	}
	if got := int(out["total"].(float64)); got != 5 {
		t.Fatalf("limit=2 total = %d, want 5（总数不受限量影响）", got)
	}

	// limit 大于总数：回全部，不报错
	resp, out = doJSON(t, app, "GET", "/api/problems?limit=999", cookie, nil)
	if resp.StatusCode != fiber.StatusOK || len(out["problems"].([]any)) != 5 {
		t.Fatalf("limit=999 = %d %v, want 200 且 5 条", resp.StatusCode, out)
	}

	// limit 与搜索组合：服务端按 q 过滤后再限量
	resp, out = doJSON(t, app, "GET", "/api/problems?q=%E9%99%90%E9%87%8F%E9%A2%983&limit=1", cookie, nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("q+limit = %d", resp.StatusCode)
	}
	list := out["problems"].([]any)
	if len(list) != 1 {
		t.Fatalf("q+limit 返回 %d 条, want 1", len(list))
	}
	if got := int(out["total"].(float64)); got != 1 {
		t.Fatalf("q+limit total = %d, want 1（q 命中 1 条）", got)
	}
	if title := list[0].(map[string]any)["title"]; title != "限量题3" {
		t.Fatalf("q+limit 命中 = %v, want 限量题3", title)
	}

	// limit=0/负数视为不限量（保持既有调用语义）
	for _, raw := range []string{"0", "-3"} {
		req := httptest.NewRequest("GET", "/api/problems?limit="+raw, nil)
		req.Header.Set("Cookie", cookie)
		r, err := app.Test(req, -1)
		if err != nil {
			t.Fatalf("limit=%s: %v", raw, err)
		}
		if r.StatusCode != fiber.StatusOK {
			t.Fatalf("limit=%s = %d, want 200", raw, r.StatusCode)
		}
	}
}
