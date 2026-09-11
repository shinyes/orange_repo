// 管理员修改用户名（PUT /api/admin/users/:id/username）HTTP 层回归测试。
//
// 覆盖：204 成功语义（DB 落库、新名可登录 / 旧名不可登录）、边界状态码
// （400 校验/非法 id、401 未登录或非管理员、404 目标不存在、409 重名 COLLATE NOCASE）、
// 域管理员作用域（本域成员 vs 他域成员 vs 管理员账号）、global_admin 全权、
// 以及「改名不清会话」（旧 cookie 仍可用且 /api/auth/me 返回新用户名）。
package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"orangeoj/internal/accounts"
	"orangeoj/internal/store"
)

// renameEnv 修改用户名测试环境：真实 sqlite（主库 + 账号库同目录）+ httptest 应用。
type renameEnv struct {
	app *fiber.App
	st  *store.Store
	acc *accounts.Store
}

func newRenameEnv(t *testing.T) *renameEnv {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	accDB, err := accounts.OpenDB(dir)
	if err != nil {
		t.Fatalf("open accounts: %v", err)
	}
	t.Cleanup(func() { _ = accDB.Close() })
	acc := accounts.New(accDB)
	srv := &Server{Store: st, Accounts: acc, UploadsDir: filepath.Join(dir, "uploads")}
	srv.EnsureBootstrap() // 引导默认系统管理员 admin/123456（global_admin）
	return &renameEnv{app: New(st, acc, srv.UploadsDir, ""), st: st, acc: acc}
}

// rawJSON 发送原始 body 的请求（不做 JSON 规范化，便于构造非法 body），
// 返回响应与解析后的 JSON 体（非 JSON/无 body 时为 nil）。
func rawJSON(t *testing.T, app *fiber.App, method, path, cookie, body string) (*http.Response, map[string]any) {
	t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	out := map[string]any{}
	raw, _ := io.ReadAll(resp.Body)
	if len(raw) > 0 && strings.Contains(resp.Header.Get("Content-Type"), "json") {
		_ = json.Unmarshal(raw, &out)
	}
	return resp, out
}

// adminCookie 系统管理员会话（引导账号）。
func (e *renameEnv) adminCookie(t *testing.T) string {
	t.Helper()
	return loginCookie(t, e.app, BootstrapAdmin, BootstrapPassword)
}

// username 读库中该账号当前用户名（断言真实 DB 状态，而非只看状态码）。
func (e *renameEnv) username(t *testing.T, id int64) string {
	t.Helper()
	var name string
	if err := e.acc.DB.QueryRow(`SELECT username FROM users WHERE id=?`, id).Scan(&name); err != nil {
		t.Fatalf("读取用户 %d 用户名: %v", id, err)
	}
	return name
}

// exec 直接改库（构造 handler 前置条件，例如给 member 写归属域）。
func (e *renameEnv) exec(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := e.acc.DB.Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

// sessionCount 该会话 token 在库中的行数（1=会话未被清理/轮换）。
func (e *renameEnv) sessionCount(t *testing.T, token string) int {
	t.Helper()
	var n int
	if err := e.acc.DB.QueryRow(`SELECT COUNT(1) FROM sessions WHERE token=?`, token).Scan(&n); err != nil {
		t.Fatalf("查询会话: %v", err)
	}
	return n
}

// renamePath 改名端点路径。
func renamePath(id int64) string { return fmt.Sprintf("/api/admin/users/%d/username", id) }

// rename 请求改用户名（任意字符串，含非法值）。
func (e *renameEnv) rename(t *testing.T, cookie string, id int64, username string) (*http.Response, map[string]any) {
	t.Helper()
	b, err := json.Marshal(map[string]string{"username": username})
	if err != nil {
		t.Fatal(err)
	}
	return rawJSON(t, e.app, "PUT", renamePath(id), cookie, string(b))
}

// renameOK 期望 204 的改名。
func (e *renameEnv) renameOK(t *testing.T, cookie string, id int64, username string) {
	t.Helper()
	resp, out := e.rename(t, cookie, id, username)
	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("改名 #%d → %q = %d %v, want 204", id, username, resp.StatusCode, out)
	}
}

// meUsername 取 /api/auth/me 响应中的 user.username。
func meUsername(t *testing.T, out map[string]any) string {
	t.Helper()
	u, ok := out["user"].(map[string]any)
	if !ok {
		t.Fatalf("/api/auth/me 响应缺 user 对象: %v", out)
	}
	name, _ := u["username"].(string)
	return name
}

// createMember 经产品入口（POST /api/admin/users）创建成员，返回账号 id。
func (e *renameEnv) createMember(t *testing.T, cookie, username, password string) int64 {
	t.Helper()
	resp, out := doJSON(t, e.app, "POST", "/api/admin/users", cookie,
		map[string]string{"username": username, "password": password})
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("创建成员 %s = %d %v, want 201", username, resp.StatusCode, out)
	}
	return int64(out["id"].(float64))
}

// joinDomainSpace 在指定域下建一个空间并把用户加入其中——
// 即「使该用户成为本域成员」（member 无 users.domain_id，域归属以空间归属表达）。
func (e *renameEnv) joinDomainSpace(t *testing.T, domainID, userID int64) {
	t.Helper()
	var count int
	if err := e.st.DB.QueryRow(`SELECT COUNT(1) FROM spaces WHERE domain_id=?`, domainID).Scan(&count); err != nil {
		t.Fatalf("查询域空间: %v", err)
	}
	var spaceID int64
	if count == 0 {
		id, err := e.st.CreateSpace(domainID, fmt.Sprintf("域%d空间", domainID))
		if err != nil {
			t.Fatalf("创建空间: %v", err)
		}
		spaceID = id
	} else if err := e.st.DB.QueryRow(`SELECT id FROM spaces WHERE domain_id=? ORDER BY id LIMIT 1`, domainID).Scan(&spaceID); err != nil {
		t.Fatalf("取域空间: %v", err)
	}
	if _, err := e.st.DB.Exec(`INSERT OR IGNORE INTO space_members(space_id,user_id) VALUES(?,?)`, spaceID, userID); err != nil {
		t.Fatalf("加入空间: %v", err)
	}
}

// loginStatus 登录并返回状态码（不改测试状态）。
func (e *renameEnv) loginStatus(t *testing.T, username, password string) int {
	t.Helper()
	resp, _ := doJSON(t, e.app, "POST", "/api/auth/login", "",
		map[string]string{"username": username, "password": password})
	return resp.StatusCode
}

// TestRenameUserHTTPGlobalAdmin global_admin 改名全流程：204 + DB 落库；
// 新用户名 + 原密码可登录、旧用户名不可登录（关键用户可见行为）；TrimSpace 与
// 仅大小写变化落库；同名改名幂等。
func TestRenameUserHTTPGlobalAdmin(t *testing.T) {
	env := newRenameEnv(t)
	admin := env.adminCookie(t)
	aliceID := env.createMember(t, admin, "alice", "pw-alice")

	// 前提：改名前旧名 + 密码可登录（否则后面的「旧名失败」断言无意义）
	if code := env.loginStatus(t, "alice", "pw-alice"); code != fiber.StatusNoContent {
		t.Fatalf("改名前 alice 登录 = %d, want 204", code)
	}

	// 改名 → 204
	resp, out := env.rename(t, admin, aliceID, "alice2")
	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("改用户名 = %d %v, want 204", resp.StatusCode, out)
	}
	if got := env.username(t, aliceID); got != "alice2" {
		t.Fatalf("改名后 DB 用户名 = %q, want alice2", got)
	}
	// 新用户名 + 原密码可登录
	if code := env.loginStatus(t, "alice2", "pw-alice"); code != fiber.StatusNoContent {
		t.Fatalf("新用户名 + 原密码登录 = %d, want 204", code)
	}
	// 旧用户名不可登录（具体状态码 + 文案）
	resp, out = doJSON(t, env.app, "POST", "/api/auth/login", "",
		map[string]string{"username": "alice", "password": "pw-alice"})
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("旧用户名登录 = %d, want 401", resp.StatusCode)
	}
	if out["error"] != "用户名或密码错误" {
		t.Fatalf("旧用户名登录 error = %v, want 用户名或密码错误", out["error"])
	}
	// 登录对大小写不敏感：ALICE2 亦可
	if code := env.loginStatus(t, "ALICE2", "pw-alice"); code != fiber.StatusNoContent {
		t.Fatalf("ALICE2 登录 = %d, want 204", code)
	}
	// 同名改名幂等 → 204，库名不变
	env.renameOK(t, admin, aliceID, "alice2")
	if got := env.username(t, aliceID); got != "alice2" {
		t.Fatalf("同名改名后 DB 用户名 = %q, want alice2", got)
	}
	// 前后空白裁剪后落库（客户端多打空格不应写进库）
	env.renameOK(t, admin, aliceID, "  alice3  ")
	if got := env.username(t, aliceID); got != "alice3" {
		t.Fatalf("带空白改名后 DB 用户名 = %q, want alice3", got)
	}
	// 仅大小写变化真正落库，且仍可登录
	env.renameOK(t, admin, aliceID, "Alice3")
	if got := env.username(t, aliceID); got != "Alice3" {
		t.Fatalf("大小写改名后 DB 用户名 = %q, want Alice3", got)
	}
	if code := env.loginStatus(t, "alice3", "pw-alice"); code != fiber.StatusNoContent {
		t.Fatalf("小写形式登录 = %d, want 204（用户名大小写不敏感）", code)
	}
}

// TestRenameUserHTTPValidation 状态码矩阵：400（校验/非法 id/非法 body）、
// 401（无会话 / member 非管理员）、404（目标不存在）、409（重名，含大小写不同）；
// 每个失败分支都断言 DB 未被改动。
func TestRenameUserHTTPValidation(t *testing.T) {
	env := newRenameEnv(t)
	admin := env.adminCookie(t)
	aliceID := env.createMember(t, admin, "alice", "pw-alice")
	bobID := env.createMember(t, admin, "bob", "pw-bob")
	_ = bobID

	cases := []struct {
		name   string
		id     int64 // 0 = 目标为 alice
		body   string
		want   int
		errMsg string
	}{
		{"重名", 0, `{"username":"bob"}`, fiber.StatusConflict, "用户名已存在"},
		{"重名（大小写不同，COLLATE NOCASE）", 0, `{"username":"BOB"}`, fiber.StatusConflict, "用户名已存在"},
		{"空用户名", 0, `{"username":""}`, fiber.StatusBadRequest, "用户名不能为空"},
		{"全空格用户名", 0, `{"username":"   "}`, fiber.StatusBadRequest, "用户名不能为空"},
		{"缺 username 字段", 0, `{}`, fiber.StatusBadRequest, "用户名不能为空"},
		{"33 字符用户名", 0, `{"username":"` + strings.Repeat("a", 33) + `"}`, fiber.StatusBadRequest, "用户名不能超过 32 个字符"},
		{"含控制字符", 0, `{"username":"bad\u0001name"}`, fiber.StatusBadRequest, "用户名含非法字符"},
		{"非法 JSON body", 0, `{"username":`, fiber.StatusBadRequest, "invalid request"},
		{"不存在的用户 id", 999999, `{"username":"ghost"}`, fiber.StatusNotFound, "用户不存在"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id := tc.id
			if id == 0 {
				id = aliceID
			}
			path := renamePath(id)
			resp, out := rawJSON(t, env.app, "PUT", path, admin, tc.body)
			if resp.StatusCode != tc.want {
				t.Fatalf("PUT %s body=%s = %d %v, want %d", path, tc.body, resp.StatusCode, out, tc.want)
			}
			if out["error"] != tc.errMsg {
				t.Fatalf("error = %v, want %q", out["error"], tc.errMsg)
			}
			// 失败一律不改库
			if got := env.username(t, aliceID); got != "alice" {
				t.Fatalf("失败后 alice 用户名 = %q, want alice（拒绝时不得改动）", got)
			}
			if got := env.username(t, bobID); got != "bob" {
				t.Fatalf("失败后 bob 用户名 = %q, want bob", got)
			}
		})
	}

	// 非法 id（非数字 / 0 / 负数）→ 400 invalid id
	for _, raw := range []string{"abc", "0", "-1"} {
		resp, out := rawJSON(t, env.app, "PUT", "/api/admin/users/"+raw+"/username", admin, `{"username":"x"}`)
		if resp.StatusCode != fiber.StatusBadRequest || out["error"] != "invalid id" {
			t.Fatalf("id=%s = %d %v, want 400 invalid id", raw, resp.StatusCode, out)
		}
	}

	// 鉴权：无 cookie → 401 unauthorized（requireAdmin：无有效会话）
	resp, out := rawJSON(t, env.app, "PUT", renamePath(aliceID), "", `{"username":"hacked"}`)
	if resp.StatusCode != fiber.StatusUnauthorized || out["error"] != "unauthorized" {
		t.Fatalf("未登录改名 = %d %v, want 401 unauthorized", resp.StatusCode, out)
	}
	// 鉴权：member 会话（非管理员）→ 401 unauthorized（与本项目 requireAdmin 语义一致）
	memberCookie := loginCookie(t, env.app, "alice", "pw-alice")
	resp, out = rawJSON(t, env.app, "PUT", renamePath(bobID), memberCookie, `{"username":"hacked"}`)
	if resp.StatusCode != fiber.StatusUnauthorized || out["error"] != "unauthorized" {
		t.Fatalf("member 改名 = %d %v, want 401 unauthorized", resp.StatusCode, out)
	}
	// 401 路径也不得改动任何账号
	if got := env.username(t, aliceID); got != "alice" {
		t.Fatalf("401 后 alice 用户名 = %q, want alice", got)
	}
	if got := env.username(t, bobID); got != "bob" {
		t.Fatalf("401 后 bob 用户名 = %q, want bob", got)
	}
}

// TestRenameUserHTTPDomainAdminScope 域管理员作用域：
// 本域成员 → 204；他域成员 / 同域其他管理员 / global_admin 账号 / 未关联域 → 403 且不改库；
// global_admin 改域管理员用户名 → 204（全权），且域管理员既有会话仍有效、返回新用户名。
func TestRenameUserHTTPDomainAdminScope(t *testing.T) {
	env := newRenameEnv(t)
	admin := env.adminCookie(t) // global_admin

	d1, err := env.st.CreateDomain("域一")
	if err != nil {
		t.Fatal(err)
	}
	d2, err := env.st.CreateDomain("域二")
	if err != nil {
		t.Fatal(err)
	}
	d1AdminID, err := env.acc.CreateUser("d1admin", "pw-d1", accounts.RoleDomainAdmin, d1)
	if err != nil {
		t.Fatal(err)
	}
	// 同域的另一名域管理员（用于验证「只允许 member」的角色门槛）
	d1PeerID, err := env.acc.CreateUser("d1peer", "pw-d1peer", accounts.RoleDomainAdmin, d1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.acc.CreateUser("d2admin", "pw-d2", accounts.RoleDomainAdmin, d2); err != nil {
		t.Fatal(err)
	}
	// 未关联任何域的域管理员（DomainID=nil）
	if _, err := env.acc.CreateUser("orphanadmin", "pw-orphan", accounts.RoleDomainAdmin); err != nil {
		t.Fatal(err)
	}
	d1Cookie := loginCookie(t, env.app, "d1admin", "pw-d1")
	orphanCookie := loginCookie(t, env.app, "orphanadmin", "pw-orphan")

	// 互为「本域/他域」的两名成员：member 不持久化 users.domain_id，
	// 本域判定按「是否属于本域任一空间的成员」——故通过加入域内空间构造。
	m1ID := env.createMember(t, admin, "m1", "pw-m1")
	env.joinDomainSpace(t, d1, m1ID)
	m2ID := env.createMember(t, admin, "m2", "pw-m2")
	env.joinDomainSpace(t, d2, m2ID)

	// 1) 本域成员 → 204，落库且新名可登录
	env.renameOK(t, d1Cookie, m1ID, "m1-renamed")
	if got := env.username(t, m1ID); got != "m1-renamed" {
		t.Fatalf("本域成员改名后 DB 用户名 = %q, want m1-renamed", got)
	}
	if code := env.loginStatus(t, "m1-renamed", "pw-m1"); code != fiber.StatusNoContent {
		t.Fatalf("本域成员新名登录 = %d, want 204", code)
	}

	// 2) 他域成员 → 403「域管理员仅可修改本域成员的用户名」且不改库
	resp, out := env.rename(t, d1Cookie, m2ID, "m2-hacked")
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("跨域改名 = %d %v, want 403", resp.StatusCode, out)
	}
	if out["error"] != "域管理员仅可修改本域成员的用户名" {
		t.Fatalf("跨域改名 error = %v, want 域管理员仅可修改本域成员的用户名", out["error"])
	}
	if got := env.username(t, m2ID); got != "m2" {
		t.Fatalf("跨域改名后他域成员用户名 = %q, want m2（拒绝时不得改动）", got)
	}
	if code := env.loginStatus(t, "m2-hacked", "pw-m2"); code != fiber.StatusUnauthorized {
		t.Fatalf("被他域管理员尝试改名后，旧密码 + 目标新名登录 = %d, want 401", code)
	}

	// 3) 同域内的其他域管理员 → 403（角色门槛：只允许 member）
	resp, out = env.rename(t, d1Cookie, d1PeerID, "d1peer-x")
	if resp.StatusCode != fiber.StatusForbidden || out["error"] != "域管理员仅可修改本域成员的用户名" {
		t.Fatalf("改同域域管理员 = %d %v, want 403（仅可改 member）", resp.StatusCode, out)
	}
	if got := env.username(t, d1PeerID); got != "d1peer" {
		t.Fatalf("403 后同域域管理员用户名 = %q, want d1peer", got)
	}

	// 4) global_admin 账号 → 403
	adminUser, err := env.acc.GetUserByUsername(BootstrapAdmin)
	if err != nil {
		t.Fatal(err)
	}
	resp, out = env.rename(t, d1Cookie, adminUser.ID, "root2")
	if resp.StatusCode != fiber.StatusForbidden || out["error"] != "域管理员仅可修改本域成员的用户名" {
		t.Fatalf("域管理员改系统管理员 = %d %v, want 403", resp.StatusCode, out)
	}
	if got := env.username(t, adminUser.ID); got != BootstrapAdmin {
		t.Fatalf("403 后系统管理员用户名 = %q, want %s", got, BootstrapAdmin)
	}

	// 5) 未关联域的域管理员 → 403（operator.DomainID == nil，无法界定本域范围）
	resp, out = env.rename(t, orphanCookie, m1ID, "m1-y")
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("未关联域的域管理员改名 = %d %v, want 403", resp.StatusCode, out)
	}
	if out["error"] != "当前账号未关联域，无法修改用户名" {
		t.Fatalf("未关联域 error = %v, want 当前账号未关联域，无法修改用户名", out["error"])
	}
	if got := env.username(t, m1ID); got != "m1-renamed" {
		t.Fatalf("403 后本域成员用户名 = %q, want m1-renamed", got)
	}

	// 5b) 未加入本域任何空间的成员 → 403（不属于本域）
	loneID := env.createMember(t, admin, "lone", "pw-lone")
	resp, out = env.rename(t, d1Cookie, loneID, "lone2")
	if resp.StatusCode != fiber.StatusForbidden || out["error"] != "域管理员仅可修改本域成员的用户名" {
		t.Fatalf("域管理员改未入域成员 = %d %v, want 403", resp.StatusCode, out)
	}
	if got := env.username(t, loneID); got != "lone" {
		t.Fatalf("403 后未入域成员用户名 = %q, want lone", got)
	}

	// 6) 目标不存在时先按 404 处理（域管理员也不例外）
	resp, out = env.rename(t, d1Cookie, 999999, "ghost")
	if resp.StatusCode != fiber.StatusNotFound || out["error"] != "用户不存在" {
		t.Fatalf("域管理员改不存在 id = %d %v, want 404 用户不存在", resp.StatusCode, out)
	}

	// 7) global_admin 改 D1 域管理员用户名 → 204（全权，含其他管理员）
	env.renameOK(t, admin, d1AdminID, "d1admin-renamed")
	if got := env.username(t, d1AdminID); got != "d1admin-renamed" {
		t.Fatalf("global 改域管理员后 DB 用户名 = %q, want d1admin-renamed", got)
	}
	// 该域管理员改名前建立的会话仍有效，且 /api/auth/me 读库返回新用户名
	resp, out = doJSON(t, env.app, "GET", "/api/auth/me", d1Cookie, nil)
	if resp.StatusCode != fiber.StatusOK || out["authenticated"] != true {
		t.Fatalf("被改名后域管理员 /api/auth/me = %d %v, want 200 authenticated=true", resp.StatusCode, out)
	}
	if name := meUsername(t, out); name != "d1admin-renamed" {
		t.Fatalf("/api/auth/me user.username = %q, want d1admin-renamed", name)
	}
	if code := env.loginStatus(t, "d1admin", "pw-d1"); code != fiber.StatusUnauthorized {
		t.Fatalf("域管理员旧名登录 = %d, want 401", code)
	}
	if code := env.loginStatus(t, "d1admin-renamed", "pw-d1"); code != fiber.StatusNoContent {
		t.Fatalf("域管理员新名登录 = %d, want 204", code)
	}

	// 8) global_admin 改自身用户名 → 204（放在最后：改后引导账号名不再可用）
	env.renameOK(t, admin, adminUser.ID, "root")
	if got := env.username(t, adminUser.ID); got != "root" {
		t.Fatalf("自我改名后 DB 用户名 = %q, want root", got)
	}
	resp, out = doJSON(t, env.app, "GET", "/api/auth/me", admin, nil)
	if resp.StatusCode != fiber.StatusOK || out["authenticated"] != true {
		t.Fatalf("自我改名后 /api/auth/me = %d %v, want 200 authenticated=true", resp.StatusCode, out)
	}
	if name := meUsername(t, out); name != "root" {
		t.Fatalf("自我改名后 /api/auth/me user.username = %q, want root", name)
	}
	if code := env.loginStatus(t, "root", BootstrapPassword); code != fiber.StatusNoContent {
		t.Fatalf("自我改名后新名登录 = %d, want 204", code)
	}
}

// TestRenameUserHTTPDomainAdminDomainMembership 域管理员改名范围以「空间归属」判定：
//   - 成员未加入本域任何空间 → 403（不属于本域）；
//   - 成员加入本域空间后 → 204（可改名），且新名可登录；
//   - 对照 global_admin 始终 204。
//
// 背景：member 账号不持久化 users.domain_id（该列语义为「域管理员归属域」），
// 因此本域成员判定按 space_members JOIN spaces 的空间归属，与 requireSpaceAccess 口径一致。
func TestRenameUserHTTPDomainAdminDomainMembership(t *testing.T) {
	env := newRenameEnv(t)
	admin := env.adminCookie(t)
	d1, err := env.st.CreateDomain("域一")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.acc.CreateUser("d1admin", "pw-d1", accounts.RoleDomainAdmin, d1); err != nil {
		t.Fatal(err)
	}
	d1Cookie := loginCookie(t, env.app, "d1admin", "pw-d1")

	// 域管理员自己创建成员（产品入口允许）
	mID := env.createMember(t, d1Cookie, "stu", "pw-stu")

	// 1) 尚未加入本域任何空间 → 不属于本域，拒绝且不改库
	resp, out := env.rename(t, d1Cookie, mID, "stu2")
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("未入域空间的成员改名 = %d %v, want 403", resp.StatusCode, out)
	}
	if out["error"] != "域管理员仅可修改本域成员的用户名" {
		t.Fatalf("error = %v, want 域管理员仅可修改本域成员的用户名", out["error"])
	}
	if got := env.username(t, mID); got != "stu" {
		t.Fatalf("403 后成员用户名 = %q, want stu（拒绝时不得改动）", got)
	}

	// 2) 加入本域空间后 → 204，落库且新名可登录
	env.joinDomainSpace(t, d1, mID)
	env.renameOK(t, d1Cookie, mID, "stu2")
	if got := env.username(t, mID); got != "stu2" {
		t.Fatalf("入域后改名 = %q, want stu2", got)
	}
	if code := env.loginStatus(t, "stu2", "pw-stu"); code != fiber.StatusNoContent {
		t.Fatalf("改名后成员新名登录 = %d, want 204", code)
	}

	// 3) 对照：global_admin 对未入域成员亦 204（全权）
	other := env.createMember(t, admin, "stu3", "pw-stu3")
	env.renameOK(t, admin, other, "stu4")
	if got := env.username(t, other); got != "stu4" {
		t.Fatalf("global_admin 改名 = %q, want stu4", got)
	}
}

// TestRenameUserHTTPSessionSurvives 改名不清会话（重要回归点）：
// 改动前登录的 cookie 在改名后仍可访问需要会话的接口，且 /api/auth/me 返回新用户名
// （该端点经 sessions JOIN users 读库）；会话行未增未减、token 未轮换。
func TestRenameUserHTTPSessionSurvives(t *testing.T) {
	env := newRenameEnv(t)
	admin := env.adminCookie(t)
	stuID := env.createMember(t, admin, "stu", "pw-stu")

	// 用旧名登录取得会话
	stuCookie := loginCookie(t, env.app, "stu", "pw-stu")
	token := strings.TrimPrefix(stuCookie, SessionCookie+"=")
	if token == "" || token == stuCookie {
		t.Fatalf("cookie 解析异常: %q", stuCookie)
	}

	// 前提：会话可用且当前返回旧名
	resp, out := doJSON(t, env.app, "GET", "/api/auth/me", stuCookie, nil)
	if resp.StatusCode != fiber.StatusOK || out["authenticated"] != true {
		t.Fatalf("改名前 /api/auth/me = %d %v, want 200 authenticated=true", resp.StatusCode, out)
	}
	if name := meUsername(t, out); name != "stu" {
		t.Fatalf("改名前 /api/auth/me user.username = %q, want stu", name)
	}
	if n := env.sessionCount(t, token); n != 1 {
		t.Fatalf("改名前会话行数 = %d, want 1", n)
	}

	// 管理员改名
	env.renameOK(t, admin, stuID, "stu-renamed")

	// 1) 改名前取得的 cookie 仍有效
	resp, out = doJSON(t, env.app, "GET", "/api/auth/me", stuCookie, nil)
	if resp.StatusCode != fiber.StatusOK || out["authenticated"] != true {
		t.Fatalf("改名后旧 cookie /api/auth/me = %d %v, want 200 authenticated=true（会话按 user_id 关联，不应失效）",
			resp.StatusCode, out)
	}
	// 2) 返回的用户名已是新名（/api/auth/me 从库读）
	if name := meUsername(t, out); name != "stu-renamed" {
		t.Fatalf("改名后 /api/auth/me user.username = %q, want stu-renamed", name)
	}
	// 3) 会话行未被清理、token 未轮换
	if n := env.sessionCount(t, token); n != 1 {
		t.Fatalf("改名后会话行数 = %d, want 1（改名不得清理/轮换会话）", n)
	}
	// 4) 需要会话的接口（requireAny 组）仍放行：改密端点带错误原密码 → 401「原密码错误」；
	//    若会话失效则中间件先短路为 401「unauthorized」，文案不同可区分
	resp, out = doJSON(t, env.app, "PUT", "/api/auth/password", stuCookie,
		map[string]string{"oldPassword": "not-the-password", "newPassword": "whatever"})
	if resp.StatusCode != fiber.StatusUnauthorized || out["error"] != "原密码错误" {
		t.Fatalf("改名后 requireAny 端点 = %d %v, want 401 原密码错误（证明会话仍被识别）", resp.StatusCode, out)
	}
	// 5) 旧名不可登录、新名 + 原密码可登录
	if code := env.loginStatus(t, "stu", "pw-stu"); code != fiber.StatusUnauthorized {
		t.Fatalf("旧名登录 = %d, want 401", code)
	}
	if code := env.loginStatus(t, "stu-renamed", "pw-stu"); code != fiber.StatusNoContent {
		t.Fatalf("新名登录 = %d, want 204", code)
	}
	// 6) 改名未额外建会话（仍只有原 token 那一行）
	if n := env.sessionCount(t, token); n != 1 {
		t.Fatalf("改名后原 token 会话行数 = %d, want 1", n)
	}
	// 7) 对照：无 cookie 时 /api/auth/me 返回 authenticated=false —— 说明上面 true 的断言非空转
	resp, out = doJSON(t, env.app, "GET", "/api/auth/me", "", nil)
	if resp.StatusCode != fiber.StatusOK || out["authenticated"] != false {
		t.Fatalf("匿名 /api/auth/me = %d %v, want 200 authenticated=false", resp.StatusCode, out)
	}
}
