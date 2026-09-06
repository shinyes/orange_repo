package app

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func itob(v int64) string { return strconv.FormatInt(v, 10) }

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

// loginCookie 以指定账号登录（合服语义：任意角色均可登录）并返回 cookie。
func loginCookie(t *testing.T, app *fiber.App, username, password string) string {
	t.Helper()
	resp, _ := doJSON(t, app, "POST", "/api/auth/login", "", map[string]string{"username": username, "password": password})
	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("login(%s) status = %d", username, resp.StatusCode)
	}
	sc := resp.Header.Get("Set-Cookie")
	if sc == "" {
		t.Fatal("no session cookie")
	}
	return strings.Split(sc, ";")[0]
}

// newMergedApp 打开单进程合服 app（临时数据目录；无前端静态/判题）。
func newMergedApp(t *testing.T) *App {
	t.Helper()
	a, err := Open(Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatalf("app.Open: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })
	if !a.MainSrv.EnsureBootstrap() {
		t.Fatal("EnsureBootstrap 失败")
	}
	return a
}

// TestMergedAuthSplit 合服鉴权分流：
//   - admin（global_admin）可访问管理 API（/api/problems）与门户 API（/api/portal/*）；
//   - member 可登录（拿到同一 orange_session cookie）并访问门户 API（/api/portal/spaces），
//     但访问管理 API（/api/problems）被 requireAdmin 挡回 401。
func TestMergedAuthSplit(t *testing.T) {
	a := newMergedApp(t)
	app := a.Fiber

	// 管理端：health（匿名）
	resp, _ := doJSON(t, app, "GET", "/api/health", "", nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("health = %d", resp.StatusCode)
	}

	// 管理员登录（放行）→ /api/problems 200
	adminCookie := loginCookie(t, app, "admin", "123456")
	resp, out := doJSON(t, app, "GET", "/api/problems", adminCookie, nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("admin /api/problems = %d %v", resp.StatusCode, out)
	}
	if _, ok := out["problems"]; !ok {
		t.Fatalf("admin /api/problems 响应缺 problems: %v", out)
	}
	// me 返回 authenticated + admin 角色
	_, me := doJSON(t, app, "GET", "/api/auth/me", adminCookie, nil)
	if me["authenticated"] != true || me["user"].(map[string]any)["role"] != "global_admin" {
		t.Fatalf("admin me = %v", me)
	}

	// 建域（global_admin）→ 门户排行榜可按域访问
	_, dOut := doJSON(t, app, "POST", "/api/admin/domains", adminCookie, map[string]any{"name": "数学域"})
	domainID := int64(dOut["id"].(float64))
	if domainID == 0 {
		t.Fatalf("create domain = %v", dOut)
	}
	// admin 门户空间列表：管理员可列出（global_admin → 全部）
	resp, _ = doJSON(t, app, "GET", "/api/portal/spaces", adminCookie, nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("admin /api/portal/spaces = %d", resp.StatusCode)
	}
	// admin 门户排行榜（域内无成员 → 空榜 200）
	resp, _ = doJSON(t, app, "GET", "/api/portal/rank?domainId="+itob(domainID), adminCookie, nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("admin /api/portal/rank = %d", resp.StatusCode)
	}

	// 管理员建 member 账号
	resp, _ = doJSON(t, app, "POST", "/api/admin/users", adminCookie, map[string]any{"username": "stu1", "password": "pw"})
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("create member = %d", resp.StatusCode)
	}

	// member 登录 → 同一 cookie 语义放行
	memberCookie := loginCookie(t, app, "stu1", "pw")
	_, me = doJSON(t, app, "GET", "/api/auth/me", memberCookie, nil)
	if me["authenticated"] != true || me["user"].(map[string]any)["role"] != "member" {
		t.Fatalf("member me = %v", me)
	}

	// member 门户：/api/portal/spaces 200（未入任何空间 → 空列表/null）
	resp, ps := doJSON(t, app, "GET", "/api/portal/spaces", memberCookie, nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("member /api/portal/spaces = %d %v", resp.StatusCode, ps)
	}
	if arr, ok := ps["spaces"].([]any); ok && len(arr) != 0 {
		t.Fatalf("member 未入空间时 spaces 应为空: %v", ps)
	}

	// member 管理：/api/problems → 401（requireAdmin 挡）
	resp, _ = doJSON(t, app, "GET", "/api/problems", memberCookie, nil)
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("member /api/problems = %d, want 401", resp.StatusCode)
	}
	// member 域管理子组 → requireAdmin 401（未到 requireGlobalAdmin）
	resp, _ = doJSON(t, app, "GET", "/api/admin/domains", memberCookie, nil)
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("member /api/admin/domains = %d, want 401", resp.StatusCode)
	}
	// member 空间内容管理叶子 → requireAdmin 401
	resp, _ = doJSON(t, app, "GET", "/api/space/1/trainings", memberCookie, nil)
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("member /api/space/1/trainings = %d, want 401", resp.StatusCode)
	}
	// member 可改自己的密码（统一 requireAny）；改密后旧会话失效，响应轮换新 cookie
	respPw, _ := doJSON(t, app, "PUT", "/api/auth/password", memberCookie,
		map[string]string{"oldPassword": "pw", "newPassword": "pw2"})
	if respPw.StatusCode != fiber.StatusNoContent {
		t.Fatalf("member change password = %d", respPw.StatusCode)
	}
	newCookie := memberCookie
	if sc := respPw.Header.Get("Set-Cookie"); sc != "" {
		newCookie = strings.Split(sc, ";")[0]
	}
	// 旧 cookie 已失效；新 cookie 可继续访问门户
	resp, _ = doJSON(t, app, "GET", "/api/portal/spaces", memberCookie, nil)
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("member stale cookie after password change = %d, want 401", resp.StatusCode)
	}
	resp, _ = doJSON(t, app, "GET", "/api/portal/spaces", newCookie, nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("member with refreshed cookie = %d（会话应已轮换）", resp.StatusCode)
	}
	// member 无 token → 401
	resp, _ = doJSON(t, app, "GET", "/api/portal/spaces", "", nil)
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("anon /api/portal/spaces = %d, want 401", resp.StatusCode)
	}
}

// TestMergedAdminPortalFlow 合服后管理 API 与门户 API 在同一进程同一库协作：
// 建域/空间 → 空间内容 → member 入空间 → member 门户可见。
func TestMergedAdminPortalFlow(t *testing.T) {
	a := newMergedApp(t)
	app := a.Fiber
	adminCookie := loginCookie(t, app, "admin", "123456")

	// 建域 + 空间（管理 API）
	_, dOut := doJSON(t, app, "POST", "/api/admin/domains", adminCookie, map[string]any{"name": "OJ域"})
	domainID := int64(dOut["id"].(float64))
	_, sOut := doJSON(t, app, "POST", "/api/admin/spaces?domainId="+itob(domainID), adminCookie, map[string]string{"name": "初一班"})
	spaceID := int64(sOut["id"].(float64))
	if domainID == 0 || spaceID == 0 {
		t.Fatalf("domain/space = %d/%d", domainID, spaceID)
	}

	// 建 member 并入空间
	_, uOut := doJSON(t, app, "POST", "/api/admin/users", adminCookie, map[string]any{"username": "stu2", "password": "pw"})
	memberID := int64(uOut["id"].(float64))
	resp, _ := doJSON(t, app, "PUT", "/api/admin/spaces/"+itob(spaceID)+"/members", adminCookie, map[string]any{"userIds": []int64{memberID}})
	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("set members = %d", resp.StatusCode)
	}

	// member 登录 → 门户可见其空间
	memberCookie := loginCookie(t, app, "stu2", "pw")
	_, ps := doJSON(t, app, "GET", "/api/portal/spaces", memberCookie, nil)
	spaces := ps["spaces"].([]any)
	if len(spaces) != 1 || int64(spaces[0].(map[string]any)["id"].(float64)) != spaceID {
		t.Fatalf("member spaces = %v, want 1 (space %d)", spaces, spaceID)
	}

	// member 仍不能访问管理 API
	resp, _ = doJSON(t, app, "GET", "/api/admin/spaces", memberCookie, nil)
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("member /api/admin/spaces = %d, want 401", resp.StatusCode)
	}
}
