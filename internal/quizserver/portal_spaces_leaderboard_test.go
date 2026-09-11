// 空间列表排行榜可见性回归测试（改动 B）：
// GET /api/portal/spaces 为每个空间附带 canViewLeaderboard（管理员恒 true；
// 成员取决于所属域 domains.leaderboard_public），并与 GET /api/portal/rank 的
// 403/200 口径保持一致（前端隐藏 tab 后后端仍须拦截直连请求）。
package quizserver

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v2"

	"orangeoj/internal/quizstore"
	"orangeoj/internal/store"
)

// spacesLeaderboardEnv 空间列表可见性测试环境。
type spacesLeaderboardEnv struct {
	app        *fiber.App
	domain     int64
	space      int64
	main       *store.Store // 保持打开：直接 SQL 翻转 domains.leaderboard_public
	stuCookie  string
	gAdmCookie string
	dAdmCookie string
}

// newSpacesLeaderboardEnv 建库（域 + 空间 + 三类账号 + 空间成员）。
func newSpacesLeaderboardEnv(t *testing.T) *spacesLeaderboardEnv {
	t.Helper()
	dir := t.TempDir()

	// 阶段 1：主库建域 + 空间
	main, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open main store: %v", err)
	}
	domainID, err := main.CreateDomain("可见性域")
	if err != nil {
		t.Fatal(err)
	}
	spaceID, err := main.CreateSpace(domainID, "一班")
	if err != nil {
		t.Fatal(err)
	}
	if err := main.Close(); err != nil {
		t.Fatal(err)
	}

	// 阶段 2：quiz 建账号（member + 系统管理员 + 域管理员）
	qs, err := quizstore.Open(dir)
	if err != nil {
		t.Fatalf("open quiz store: %v", err)
	}
	t.Cleanup(func() { _ = qs.Close() })
	stuID, err := qs.Accounts.CreateUser("stu1", "pw", "member")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := qs.Accounts.CreateUser("gadmin", "pw", "global_admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := qs.Accounts.CreateUser("dadmin", "pw", "domain_admin", domainID); err != nil {
		t.Fatal(err)
	}

	// 阶段 3：回主库写空间成员（保持打开供翻转开关）
	main2, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = main2.Close() })
	if err := main2.SetSpaceMembers(spaceID, []int64{stuID}); err != nil {
		t.Fatal(err)
	}

	env := &spacesLeaderboardEnv{domain: domainID, space: spaceID, main: main2}
	env.app = New(&Server{QS: qs}, nil, 0)
	env.stuCookie = loginStudent(t, env.app, "stu1", "pw")
	env.gAdmCookie = loginStudent(t, env.app, "gadmin", "pw")
	env.dAdmCookie = loginStudent(t, env.app, "dadmin", "pw")
	return env
}

// setPublic 直接改域排行榜公开开关（模拟域管理 PATCH leaderboardPublic）。
func (e *spacesLeaderboardEnv) setPublic(t *testing.T, pub bool) {
	t.Helper()
	if _, err := e.main.DB.Exec(`UPDATE domains SET leaderboard_public=? WHERE id=?`, b2iTest(pub), e.domain); err != nil {
		t.Fatal(err)
	}
}

func b2iTest(b bool) int {
	if b {
		return 1
	}
	return 0
}

// spaces 请求空间列表并返回 spaces 数组。
func (e *spacesLeaderboardEnv) spaces(t *testing.T, cookie string) []map[string]any {
	t.Helper()
	resp, out := doJSON(t, e.app, "GET", "/api/portal/spaces", cookie, nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("GET /api/portal/spaces = %d %v", resp.StatusCode, out)
	}
	raw, ok := out["spaces"].([]any)
	if !ok {
		t.Fatalf("spaces 字段类型异常: %v", out)
	}
	list := make([]map[string]any, 0, len(raw))
	for _, v := range raw {
		m, ok := v.(map[string]any)
		if !ok {
			t.Fatalf("spaces 元素类型异常: %T (%v)", v, v)
		}
		list = append(list, m)
	}
	return list
}

// canViewOf 取指定空间条目的 canViewLeaderboard（字段缺失即失败：前端依赖该键）。
func canViewOf(t *testing.T, list []map[string]any, spaceID int64) bool {
	t.Helper()
	for _, sp := range list {
		if int64(sp["id"].(float64)) != spaceID {
			continue
		}
		v, ok := sp["canViewLeaderboard"]
		if !ok {
			t.Fatalf("空间 %d 响应缺少 canViewLeaderboard 键: %v", spaceID, sp)
		}
		b, ok := v.(bool)
		if !ok {
			t.Fatalf("canViewLeaderboard 类型 = %T (%v)，want bool", v, v)
		}
		return b
	}
	t.Fatalf("空间列表未含空间 %d: %v", spaceID, list)
	return false
}

// rank 请求排行榜，返回响应与响应体。
func (e *spacesLeaderboardEnv) rank(t *testing.T, cookie string) (*http.Response, map[string]any) {
	t.Helper()
	return doJSON(t, e.app, "GET", fmt.Sprintf("/api/portal/rank?domainId=%d", e.domain), cookie, nil)
}

// TestPortalSpacesLeaderboard 改动 B：空间列表 canViewLeaderboard 随域开关翻转，
// 管理员不受限；且与 /api/portal/rank 的 403 口径一致。
func TestPortalSpacesLeaderboard(t *testing.T) {
	env := newSpacesLeaderboardEnv(t)

	// ---- 1. 默认公开（leaderboard_public=1）：member 可见 ----
	if got := canViewOf(t, env.spaces(t, env.stuCookie), env.space); got != true {
		t.Fatalf("公开时 member canViewLeaderboard = %v（want true）", got)
	}
	if resp, out := env.rank(t, env.stuCookie); resp.StatusCode != fiber.StatusOK {
		t.Fatalf("公开时 member rank = %d %v（want 200）", resp.StatusCode, out)
	}

	// ---- 2. 域关闭公开 → member 空间列表 canViewLeaderboard=false ----
	env.setPublic(t, false)
	stuSpaces := env.spaces(t, env.stuCookie)
	if got := canViewOf(t, stuSpaces, env.space); got != false {
		t.Fatalf("不公开时 member canViewLeaderboard = %v（want false）", got)
	}
	// 空间本身仍可见（隐藏的是排行榜 tab，不是空间）
	if len(stuSpaces) != 1 || stuSpaces[0]["name"] != "一班" {
		t.Fatalf("不公开时 member 空间列表 = %v（空间条目应照常返回）", stuSpaces)
	}

	// ---- 3. 管理员在域不公开时恒 true（global_admin + domain_admin）----
	if got := canViewOf(t, env.spaces(t, env.gAdmCookie), env.space); got != true {
		t.Fatalf("不公开时 global_admin canViewLeaderboard = %v（want true）", got)
	}
	if got := canViewOf(t, env.spaces(t, env.dAdmCookie), env.space); got != true {
		t.Fatalf("不公开时 domain_admin canViewLeaderboard = %v（want true）", got)
	}

	// ---- 4. 交叉断言：同一 member 此刻 rank 403 「排行榜未公开」 ----
	resp, out := env.rank(t, env.stuCookie)
	if resp.StatusCode != fiber.StatusForbidden || out["error"] != "排行榜未公开" {
		t.Fatalf("不公开时 member rank = %d %v（want 403 排行榜未公开，须与 canViewLeaderboard=false 一致）", resp.StatusCode, out)
	}
	// 管理员 rank 不受限（与 canViewLeaderboard=true 一致）
	if resp, out := env.rank(t, env.gAdmCookie); resp.StatusCode != fiber.StatusOK {
		t.Fatalf("不公开时 global_admin rank = %d %v（want 200）", resp.StatusCode, out)
	}
	if resp, out := env.rank(t, env.dAdmCookie); resp.StatusCode != fiber.StatusOK {
		t.Fatalf("不公开时 domain_admin rank = %d %v（want 200）", resp.StatusCode, out)
	}

	// ---- 5. 重新公开 → member 列表与 rank 同步恢复 ----
	env.setPublic(t, true)
	if got := canViewOf(t, env.spaces(t, env.stuCookie), env.space); got != true {
		t.Fatalf("重开后 member canViewLeaderboard = %v（want true）", got)
	}
	if resp, out := env.rank(t, env.stuCookie); resp.StatusCode != fiber.StatusOK {
		t.Fatalf("重开后 member rank = %d %v（want 200）", resp.StatusCode, out)
	}
}
