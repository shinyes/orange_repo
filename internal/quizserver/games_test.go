// 休息时间小游戏成绩/榜单 HTTP 层回归测试（真库 + httptest）：
//
//	POST /api/portal/game/:game/score
//	GET  /api/portal/game/:game/rank?scope=domain|all
//
// 覆盖：未登录 401 / 正常提交（只留最高分语义）/ score 缺失与越界 400 / game 标识非法 400 /
// 同用户连续提交 429 / scope=domain 与 scope=all 的内容与顺序 / 非法 scope 400 /
// 无成绩时 rows 为 `[]`（不是 null）/ 成员可看全域榜且不受 domains.leaderboard_public 影响 /
// 域归属按 spaceId 解析与回退。测试间用 quizstore.ResetGameScoreRateLimit 隔离限频。
package quizserver

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

	"orangeoj/internal/accounts"
	"orangeoj/internal/quizstore"
	"orangeoj/internal/store"
)

// gameTestEnv 小游戏测试环境：
// 甲域(domainA) 空间 spaceA 有成员 A1/A2；乙域(domainB) 空间 spaceB 有成员 B1；
// lone 是无空间归属的成员；admin 是系统管理员。
type gameTestEnv struct {
	app         *fiber.App
	qs          *quizstore.Store
	main        *store.Store // 保持打开：直接翻转 domains.leaderboard_public
	domainA     int64
	domainB     int64
	spaceA      int64
	spaceB      int64
	uidA1       int64
	uidA2       int64
	uidB1       int64
	uidLone     int64
	cookieA1    string
	cookieA2    string
	cookieB1    string
	cookieLone  string
	cookieAdmin string
}

// newGameTestEnv 建库（两域两空间 + 成员）并组装 app、登录全部账号。
func newGameTestEnv(t *testing.T) *gameTestEnv {
	t.Helper()
	dir := t.TempDir()

	// 阶段 1：主库建域 + 空间
	main, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open main store: %v", err)
	}
	env := &gameTestEnv{}
	if env.domainA, err = main.CreateDomain("甲域"); err != nil {
		t.Fatal(err)
	}
	if env.spaceA, err = main.CreateSpace(env.domainA, "甲域一班"); err != nil {
		t.Fatal(err)
	}
	if env.domainB, err = main.CreateDomain("乙域"); err != nil {
		t.Fatal(err)
	}
	if env.spaceB, err = main.CreateSpace(env.domainB, "乙域一班"); err != nil {
		t.Fatal(err)
	}
	if err := main.Close(); err != nil {
		t.Fatal(err)
	}

	// 阶段 2：quiz 建账号（4 名成员 + 1 名系统管理员）
	qs, err := quizstore.Open(dir)
	if err != nil {
		t.Fatalf("open quiz store: %v", err)
	}
	t.Cleanup(func() { _ = qs.Close() })
	env.qs = qs
	newUser := func(name string, role accounts.Role) int64 {
		t.Helper()
		id, err := qs.Accounts.CreateUser(name, "pw", role)
		if err != nil {
			t.Fatalf("create user %s: %v", name, err)
		}
		return id
	}
	env.uidA1 = newUser("gameStuA1", accounts.RoleMember)
	env.uidA2 = newUser("gameStuA2", accounts.RoleMember)
	env.uidB1 = newUser("gameStuB1", accounts.RoleMember)
	env.uidLone = newUser("gameLone", accounts.RoleMember)
	newUser("gameAdmin", accounts.RoleGlobalAdmin)

	// 阶段 3：回主库写空间成员（main 保持打开供翻转开关）
	main2, err := store.Open(dir)
	if err != nil {
		t.Fatalf("reopen main store: %v", err)
	}
	t.Cleanup(func() { _ = main2.Close() })
	env.main = main2
	if err := main2.SetSpaceMembers(env.spaceA, []int64{env.uidA1, env.uidA2}); err != nil {
		t.Fatal(err)
	}
	if err := main2.SetSpaceMembers(env.spaceB, []int64{env.uidB1}); err != nil {
		t.Fatal(err)
	}

	// 组装应用 + 登录
	env.app = New(&Server{QS: qs}, nil, 0)
	env.cookieA1 = loginStudent(t, env.app, "gameStuA1", "pw")
	env.cookieA2 = loginStudent(t, env.app, "gameStuA2", "pw")
	env.cookieB1 = loginStudent(t, env.app, "gameStuB1", "pw")
	env.cookieLone = loginStudent(t, env.app, "gameLone", "pw")
	env.cookieAdmin = loginStudent(t, env.app, "gameAdmin", "pw")

	// 限频表是进程内全局状态：用例开始清空，结束再清空（避免影响其他用例）
	quizstore.ResetGameScoreRateLimit()
	t.Cleanup(quizstore.ResetGameScoreRateLimit)
	return env
}

// submit 提交一次成绩。
func (e *gameTestEnv) submit(t *testing.T, cookie, game string, body any) (*http.Response, map[string]any) {
	t.Helper()
	return doJSON(t, e.app, "POST", "/api/portal/game/"+game+"/score", cookie, body)
}

// rank 查询榜单（scope 为空表示不带 scope 参数）。
func (e *gameTestEnv) rank(t *testing.T, cookie, game, scope string, spaceID int64) (*http.Response, map[string]any) {
	t.Helper()
	path := "/api/portal/game/" + game + "/rank"
	q := []string{}
	if scope != "" {
		q = append(q, "scope="+scope)
	}
	if spaceID > 0 {
		q = append(q, "spaceId="+strconv.FormatInt(spaceID, 10))
	}
	if len(q) > 0 {
		path += "?" + strings.Join(q, "&")
	}
	return doJSON(t, e.app, "GET", path, cookie, nil)
}

// resetLimit 清空进程内提交限频表。
func (e *gameTestEnv) resetLimit() { quizstore.ResetGameScoreRateLimit() }

// setLeaderboardPublic 直接改域「排行榜公开」开关（模拟域管理 PATCH leaderboardPublic）。
func (e *gameTestEnv) setLeaderboardPublic(t *testing.T, domainID int64, pub bool) {
	t.Helper()
	v := 0
	if pub {
		v = 1
	}
	if _, err := e.main.DB.Exec(`UPDATE domains SET leaderboard_public=? WHERE id=?`, v, domainID); err != nil {
		t.Fatal(err)
	}
}

// gameScoreSeed 造榜数据：(登录人, 空间, 分数)。
type gameScoreSeed struct {
	cookie  string
	spaceID int64
	score   int
}

// seed 逐个用户提交一次成绩（每条前重置限频，保证只受用例自身影响）。
func (e *gameTestEnv) seed(t *testing.T, game string, seeds []gameScoreSeed) {
	t.Helper()
	for _, s := range seeds {
		e.resetLimit()
		resp, out := e.submit(t, s.cookie, game, map[string]any{"score": s.score, "spaceId": s.spaceID})
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("seed 提交 score=%d 失败: %d %v", s.score, resp.StatusCode, out)
		}
	}
}

// rowsOf 取榜单 rows 并断言它是 JSON 数组。
func rowsOf(t *testing.T, out map[string]any) []map[string]any {
	t.Helper()
	raw, ok := out["rows"].([]any)
	if !ok {
		t.Fatalf("rows 字段类型 = %T (%v), want 数组", out["rows"], out["rows"])
	}
	rows := make([]map[string]any, 0, len(raw))
	for i, v := range raw {
		m, ok := v.(map[string]any)
		if !ok {
			t.Fatalf("rows[%d] 类型 = %T (%v), want 对象", i, v, v)
		}
		rows = append(rows, m)
	}
	return rows
}

// assertRow 校验榜单行的关键字段（用户名非空、id/分数/名次具体值）。
func assertRow(t *testing.T, rows []map[string]any, idx int, wantUser int64, wantName string, wantScore, wantRank int) {
	t.Helper()
	if idx >= len(rows) {
		t.Fatalf("rows 仅 %d 行，取不到第 %d 行: %v", len(rows), idx, rows)
	}
	row := rows[idx]
	name, _ := row["userName"].(string)
	if name == "" {
		t.Fatalf("rows[%d].userName 为空 (%v), want %q", idx, row, wantName)
	}
	if name != wantName {
		t.Fatalf("rows[%d].userName = %q, want %q", idx, name, wantName)
	}
	if got := row["userId"]; got != float64(wantUser) {
		t.Fatalf("rows[%d].userId = %v, want %d", idx, got, wantUser)
	}
	if got := row["bestScore"]; got != float64(wantScore) {
		t.Fatalf("rows[%d].bestScore = %v, want %d", idx, got, wantScore)
	}
	if got := row["rank"]; got != float64(wantRank) {
		t.Fatalf("rows[%d].rank = %v, want %d", idx, got, wantRank)
	}
}

// doRaw 发送请求并返回原始响应体（用于断言空数组在报文里是 [] 而非 null）。
func doRaw(t *testing.T, app *fiber.App, method, path, cookie string, body any) (*http.Response, string) {
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
	raw, _ := io.ReadAll(resp.Body)
	return resp, string(raw)
}

// ---------- 11. 未登录 ----------

// TestGameScoreUnauthorized 提交与榜单都要求登录：无 cookie → 401 unauthorized，且不写库。
func TestGameScoreUnauthorized(t *testing.T) {
	env := newGameTestEnv(t)

	resp, out := doJSON(t, env.app, "POST", "/api/portal/game/snake/score", "", map[string]any{"score": 10})
	if resp.StatusCode != fiber.StatusUnauthorized || out["error"] != "unauthorized" {
		t.Fatalf("未登录提交 = %d %v, want 401 unauthorized", resp.StatusCode, out)
	}
	for _, path := range []string{
		"/api/portal/game/snake/rank",
		"/api/portal/game/snake/rank?scope=all",
	} {
		resp, out := doJSON(t, env.app, "GET", path, "", nil)
		if resp.StatusCode != fiber.StatusUnauthorized || out["error"] != "unauthorized" {
			t.Fatalf("未登录 %s = %d %v, want 401 unauthorized", path, resp.StatusCode, out)
		}
	}
	// 未登录提交不得落库
	var n int
	if err := env.qs.DB.QueryRow(`SELECT COUNT(1) FROM game_scores`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("未登录提交后 game_scores 行数 = %d, want 0", n)
	}
}

// ---------- 12. 正常提交 ----------

// TestGameScoreSubmitSemantics 提交成功返回 bestScore/isNewBest/score/domainId，
// 且只保留最高分：低分不覆盖、高分刷新，plays 逐次累计。
func TestGameScoreSubmitSemantics(t *testing.T) {
	env := newGameTestEnv(t)

	// 首次提交 42：200 + isNewBest=true + 域归属=甲域
	env.resetLimit()
	resp, out := env.submit(t, env.cookieA1, "snake", map[string]any{"score": 42, "spaceId": env.spaceA})
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("首次提交 = %d %v, want 200", resp.StatusCode, out)
	}
	if out["bestScore"] != float64(42) || out["isNewBest"] != true || out["score"] != float64(42) {
		t.Fatalf("首次提交返回 = %v, want bestScore=42 isNewBest=true score=42", out)
	}
	if out["domainId"] != float64(env.domainA) {
		t.Fatalf("首次提交 domainId = %v, want %d", out["domainId"], env.domainA)
	}
	// rankDomain / rankAll 字段存在且为数值（取值问题见交付说明）
	for _, k := range []string{"rankDomain", "rankAll"} {
		v, ok := out[k]
		if !ok {
			t.Fatalf("提交响应缺少 %s 字段: %v", k, out)
		}
		if _, ok := v.(float64); !ok {
			t.Fatalf("%s 类型 = %T (%v), want 数值", k, v, v)
		}
	}

	// 低分 7：best 不变、isNewBest=false
	env.resetLimit()
	resp, out = env.submit(t, env.cookieA1, "snake", map[string]any{"score": 7, "spaceId": env.spaceA})
	if resp.StatusCode != fiber.StatusOK || out["bestScore"] != float64(42) || out["isNewBest"] != false || out["score"] != float64(7) {
		t.Fatalf("低分提交 = %d %v, want 200 bestScore=42 isNewBest=false score=7", resp.StatusCode, out)
	}

	// 高分 90：刷新最高分
	env.resetLimit()
	resp, out = env.submit(t, env.cookieA1, "snake", map[string]any{"score": 90, "spaceId": env.spaceA})
	if resp.StatusCode != fiber.StatusOK || out["bestScore"] != float64(90) || out["isNewBest"] != true {
		t.Fatalf("高分提交 = %d %v, want 200 bestScore=90 isNewBest=true", resp.StatusCode, out)
	}

	// 榜单回读：myBest=90、myPlays=3（三次提交都计次）、myRank=1
	resp, out = env.rank(t, env.cookieA1, "snake", "domain", env.spaceA)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("榜单 = %d %v", resp.StatusCode, out)
	}
	if out["myBest"] != float64(90) || out["myPlays"] != float64(3) {
		t.Fatalf("榜单 myBest/myPlays = %v/%v, want 90/3", out["myBest"], out["myPlays"])
	}
	if out["myRank"] != float64(1) {
		t.Fatalf("榜单 myRank = %v, want 1", out["myRank"])
	}
	if _, ok := out["myRow"].(map[string]any); !ok {
		t.Fatalf("榜单 myRow = %T (%v), want 对象", out["myRow"], out["myRow"])
	}
}

// ---------- 13. score 缺失 / 越界 ----------

// TestGameScoreSubmitBadScore score 缺失、非整数、负值与超上限都返回 400，且不写库。
func TestGameScoreSubmitBadScore(t *testing.T) {
	env := newGameTestEnv(t)
	const game = "scorerr"

	cases := []struct {
		name string
		body any
		want string
	}{
		{"缺 score 字段", map[string]any{}, "缺少分数"},
		{"score 为 null", map[string]any{"score": nil}, "缺少分数"},
		{"score 非整数", map[string]any{"score": "42"}, "缺少分数"},
		{"score 为负", map[string]any{"score": -1}, "分数超出允许范围"},
		{"score 超上限", map[string]any{"score": quizstore.MaxGameScore + 1}, "分数超出允许范围"},
	}
	for _, tc := range cases {
		env.resetLimit()
		resp, out := env.submit(t, env.cookieA2, game, tc.body)
		if resp.StatusCode != fiber.StatusBadRequest {
			t.Fatalf("%s: 状态码 = %d %v, want 400", tc.name, resp.StatusCode, out)
		}
		if out["error"] != tc.want {
			t.Fatalf("%s: error = %v, want %q", tc.name, out["error"], tc.want)
		}
	}
	// 全部被拒 → 无记录
	if best, plays, err := env.qs.MyGameScore(game, env.uidA2); err != nil || best != 0 || plays != 0 {
		t.Fatalf("非法分数提交后 MyGameScore = (%d, %d, %v), want (0, 0, nil)", best, plays, err)
	}
}

// ---------- 14. game 标识非法（含合法标识大小写规范化） ----------

// TestGameScoreInvalidGameID game 标识非法 → POST/GET 都 400「游戏标识不合法」；
// 大写标识经规范化后合法（落到小写 game 上）。
func TestGameScoreInvalidGameID(t *testing.T) {
	env := newGameTestEnv(t)

	bad := []struct {
		name string
		path string
	}{
		{"超 32 字符", strings.Repeat("a", 33)},
		{"含点", "snake.v2"},
		{"含空白", "my%20game"},
		{"含斜杠编码", "a%2Fb"},
	}
	for _, tc := range bad {
		resp, out := env.submit(t, env.cookieA1, tc.path, map[string]any{"score": 10, "spaceId": env.spaceA})
		if resp.StatusCode != fiber.StatusBadRequest || out["error"] != "游戏标识不合法" {
			t.Fatalf("%s: 提交 = %d %v, want 400 游戏标识不合法", tc.name, resp.StatusCode, out)
		}
		resp, out = env.rank(t, env.cookieA1, tc.path, "all", 0)
		if resp.StatusCode != fiber.StatusBadRequest || out["error"] != "游戏标识不合法" {
			t.Fatalf("%s: 榜单 = %d %v, want 400 游戏标识不合法", tc.name, resp.StatusCode, out)
		}
	}
	var n int
	if err := env.qs.DB.QueryRow(`SELECT COUNT(1) FROM game_scores`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("非法 game 标识后 game_scores 行数 = %d, want 0", n)
	}

	// 合法标识（大写经 NormalizeGameID 归一）可提交，落库为小写 game
	env.resetLimit()
	resp, out := env.submit(t, env.cookieA1, "Snake", map[string]any{"score": 15, "spaceId": env.spaceA})
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("大写标识提交 = %d %v, want 200", resp.StatusCode, out)
	}
	var stored string
	if err := env.qs.DB.QueryRow(`SELECT game FROM game_scores WHERE user_id=?`, env.uidA1).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != "snake" {
		t.Fatalf("落库 game = %q, want snake（规范化）", stored)
	}
	if best, plays, err := env.qs.MyGameScore("snake", env.uidA1); err != nil || best != 15 || plays != 1 {
		t.Fatalf("小写 game MyGameScore = (%d, %d, %v), want (15, 1, nil)", best, plays, err)
	}
}

// ---------- 15. 限频 → 429 ----------

// TestGameScoreSubmitRateLimited 同一用户 3 秒内第二次提交 → 429（文案为限频错误），
// 被拒提交不写库；ResetGameScoreRateLimit 后可再次提交。
func TestGameScoreSubmitRateLimited(t *testing.T) {
	env := newGameTestEnv(t)
	const game = "rate"

	env.resetLimit()
	resp, out := env.submit(t, env.cookieA2, game, map[string]any{"score": 10, "spaceId": env.spaceA})
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("首次提交 = %d %v, want 200", resp.StatusCode, out)
	}

	resp, out = env.submit(t, env.cookieA2, game, map[string]any{"score": 20, "spaceId": env.spaceA})
	if resp.StatusCode != fiber.StatusTooManyRequests {
		t.Fatalf("限频内二次提交 = %d %v, want 429", resp.StatusCode, out)
	}
	if out["error"] != quizstore.ErrGameScoreTooFrequent.Error() {
		t.Fatalf("限频错误文案 = %v, want %q", out["error"], quizstore.ErrGameScoreTooFrequent.Error())
	}
	// 被拒提交不写库
	if best, plays, err := env.qs.MyGameScore(game, env.uidA2); err != nil || best != 10 || plays != 1 {
		t.Fatalf("限频后 MyGameScore = (%d, %d, %v), want (10, 1, nil)", best, plays, err)
	}
	// 清空限频表后可再次提交并刷新最高分
	env.resetLimit()
	resp, out = env.submit(t, env.cookieA2, game, map[string]any{"score": 20, "spaceId": env.spaceA})
	if resp.StatusCode != fiber.StatusOK || out["bestScore"] != float64(20) || out["isNewBest"] != true {
		t.Fatalf("重置限频后提交 = %d %v, want 200 bestScore=20 isNewBest=true", resp.StatusCode, out)
	}
}

// ---------- 16. scope=domain / scope=all ----------

// TestGameRankScopes 本域榜只看本域、全域榜看所有域；行内 userName 非空、rows 为数组；
// 缺省 scope=domain；非法 scope → 400。
func TestGameRankScopes(t *testing.T) {
	env := newGameTestEnv(t)
	env.seed(t, "snake", []gameScoreSeed{
		{env.cookieA1, env.spaceA, 100},
		{env.cookieA2, env.spaceA, 30},
		{env.cookieB1, env.spaceB, 55},
	})

	// ---- 本域榜（甲域成员视角）----
	resp, out := env.rank(t, env.cookieA1, "snake", "domain", env.spaceA)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("本域榜 = %d %v, want 200", resp.StatusCode, out)
	}
	if out["scope"] != "domain" || out["game"] != "snake" {
		t.Fatalf("本域榜 scope/game = %v/%v, want domain/snake", out["scope"], out["game"])
	}
	if out["domainId"] != float64(env.domainA) || out["domainName"] != "甲域" {
		t.Fatalf("本域榜 domainId/domainName = %v/%v, want %d/甲域", out["domainId"], out["domainName"], env.domainA)
	}
	rows := rowsOf(t, out)
	if len(rows) != 2 {
		t.Fatalf("本域榜行数 = %d, want 2（只含甲域成员）: %v", len(rows), rows)
	}
	assertRow(t, rows, 0, env.uidA1, "gameStuA1", 100, 1)
	assertRow(t, rows, 1, env.uidA2, "gameStuA2", 30, 2)
	if out["myBest"] != float64(100) || out["myPlays"] != float64(1) || out["myRank"] != float64(1) {
		t.Fatalf("本域榜 myBest/myPlays/myRank = %v/%v/%v, want 100/1/1", out["myBest"], out["myPlays"], out["myRank"])
	}

	// ---- 全域榜（乙域成员视角，普通 member 即可看）----
	resp, out = env.rank(t, env.cookieB1, "snake", "all", 0)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("全域榜 = %d %v, want 200", resp.StatusCode, out)
	}
	if out["scope"] != "all" || out["domainId"] != float64(0) || out["domainName"] != "" {
		t.Fatalf("全域榜 scope/domainId/domainName = %v/%v/%v, want all/0/空", out["scope"], out["domainId"], out["domainName"])
	}
	rows = rowsOf(t, out)
	if len(rows) != 3 {
		t.Fatalf("全域榜行数 = %d, want 3（跨两个域）: %v", len(rows), rows)
	}
	assertRow(t, rows, 0, env.uidA1, "gameStuA1", 100, 1)
	assertRow(t, rows, 1, env.uidB1, "gameStuB1", 55, 2)
	assertRow(t, rows, 2, env.uidA2, "gameStuA2", 30, 3)
	if out["myBest"] != float64(55) || out["myPlays"] != float64(1) || out["myRank"] != float64(2) {
		t.Fatalf("全域榜 myBest/myPlays/myRank = %v/%v/%v, want 55/1/2", out["myBest"], out["myPlays"], out["myRank"])
	}

	// ---- 缺省 scope = domain ----
	resp, out = env.rank(t, env.cookieA1, "snake", "", env.spaceA)
	if resp.StatusCode != fiber.StatusOK || out["scope"] != "domain" {
		t.Fatalf("缺省 scope = %d %v, want 200 domain", resp.StatusCode, out)
	}
	if got := len(rowsOf(t, out)); got != 2 {
		t.Fatalf("缺省 scope 行数 = %d, want 2（等同本域榜）", got)
	}

	// ---- 非法 scope → 400 ----
	resp, out = env.rank(t, env.cookieA1, "snake", "team", env.spaceA)
	if resp.StatusCode != fiber.StatusBadRequest || out["error"] != "scope 仅支持 domain 或 all" {
		t.Fatalf("非法 scope = %d %v, want 400 scope 仅支持 domain 或 all", resp.StatusCode, out)
	}
}

// ---------- 17. 无成绩 → rows 为 [] ----------

// TestGameRankEmptyRowsIsArray 无任何成绩时榜单仍是 200，rows 在原始报文里是 `[]` 而非 null，
// 且不带 myRank/myRow 键。
func TestGameRankEmptyRowsIsArray(t *testing.T) {
	env := newGameTestEnv(t)

	for _, path := range []string{
		"/api/portal/game/nobody/rank?scope=all",
		"/api/portal/game/nobody/rank?scope=domain&spaceId=" + strconv.FormatInt(env.spaceA, 10),
	} {
		resp, raw := doRaw(t, env.app, "GET", path, env.cookieA1, nil)
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("%s = %d %s, want 200", path, resp.StatusCode, raw)
		}
		if !strings.Contains(raw, `"rows":[]`) {
			t.Fatalf("%s 原始报文 rows 不是 []: %s", path, raw)
		}
		if strings.Contains(raw, `"rows":null`) {
			t.Fatalf("%s 原始报文 rows 为 null（前端会渲染失败）: %s", path, raw)
		}
		resp, out := doJSON(t, env.app, "GET", path, env.cookieA1, nil)
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("%s = %d %v", path, resp.StatusCode, out)
		}
		if rows := rowsOf(t, out); len(rows) != 0 {
			t.Fatalf("%s 行数 = %d, want 0", path, len(rows))
		}
		if out["myBest"] != float64(0) || out["myPlays"] != float64(0) {
			t.Fatalf("%s myBest/myPlays = %v/%v, want 0/0", path, out["myBest"], out["myPlays"])
		}
		if _, ok := out["myRank"]; ok {
			t.Fatalf("%s 无成绩不应带 myRank: %v", path, out)
		}
		if _, ok := out["myRow"]; ok {
			t.Fatalf("%s 无成绩不应带 myRow: %v", path, out)
		}
	}
}

// ---------- 18. 成员可看全域榜，且不受「排行榜公开」开关影响 ----------

// TestGameRankMemberIgnoresLeaderboardPublic 普通成员可看两种游戏榜单；
// 关闭所属域 domains.leaderboard_public 后，传统排行榜 /api/portal/rank 对成员 403，
// 但游戏榜单（本域 + 全域）仍 200 可看。
func TestGameRankMemberIgnoresLeaderboardPublic(t *testing.T) {
	env := newGameTestEnv(t)
	env.seed(t, "snake", []gameScoreSeed{
		{env.cookieA1, env.spaceA, 100},
		{env.cookieB1, env.spaceB, 55},
	})

	// 前置：A1 确为普通成员（不是管理员，排除「管理员特权」解释）
	u, err := env.qs.Accounts.GetUserByID(env.uidA1)
	if err != nil {
		t.Fatal(err)
	}
	if string(u.Role) != "member" {
		t.Fatalf("A1 角色 = %q, want member", u.Role)
	}

	// 关闭甲域排行榜公开
	env.setLeaderboardPublic(t, env.domainA, false)

	// 交叉验证开关生效：传统排行榜对成员 403
	resp, out := doJSON(t, env.app, "GET", "/api/portal/rank?domainId="+strconv.FormatInt(env.domainA, 10), env.cookieA1, nil)
	if resp.StatusCode != fiber.StatusForbidden || out["error"] != "排行榜未公开" {
		t.Fatalf("关闭公开后 /api/portal/rank = %d %v, want 403 排行榜未公开", resp.StatusCode, out)
	}

	// 游戏榜单不受影响：本域榜与全域榜都 200
	resp, out = env.rank(t, env.cookieA1, "snake", "domain", env.spaceA)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("关闭公开后本域游戏榜 = %d %v, want 200", resp.StatusCode, out)
	}
	if rows := rowsOf(t, out); len(rows) != 1 {
		t.Fatalf("关闭公开后本域游戏榜行数 = %d, want 1", len(rows))
	}
	resp, out = env.rank(t, env.cookieA1, "snake", "all", 0)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("关闭公开后全域游戏榜 = %d %v, want 200", resp.StatusCode, out)
	}
	rows := rowsOf(t, out)
	if len(rows) != 2 {
		t.Fatalf("关闭公开后全域游戏榜行数 = %d, want 2（跨域可见）", len(rows))
	}
	assertRow(t, rows, 1, env.uidB1, "gameStuB1", 55, 2)
}

// ---------- 域归属解析与回退 ----------

// TestGameScoreDomainResolution 域归属由服务端解析：
//  1. 不带 spaceId → 回落到该用户所在空间的域；
//  2. spaceId 指向不可访问的空间 → 不采纳，仍回落本域（防止刷别域榜）；
//  3. 无空间归属的成员 → 取不到域（domainId=0，落 NULL），本域榜返回空榜 + noDomain 提示；
//  4. 系统管理员可指定任意空间 → 落在该空间所属域。
func TestGameScoreDomainResolution(t *testing.T) {
	env := newGameTestEnv(t)

	// 1. 不带 spaceId → 甲域
	env.resetLimit()
	resp, out := env.submit(t, env.cookieA1, "snake", map[string]any{"score": 11})
	if resp.StatusCode != fiber.StatusOK || out["domainId"] != float64(env.domainA) {
		t.Fatalf("无 spaceId 提交 = %d %v, want 200 domainId=%d", resp.StatusCode, out, env.domainA)
	}

	// 2. 传乙域空间（A1 非其成员）→ 回落到甲域
	env.resetLimit()
	resp, out = env.submit(t, env.cookieA1, "snake", map[string]any{"score": 12, "spaceId": env.spaceB})
	if resp.StatusCode != fiber.StatusOK || out["domainId"] != float64(env.domainA) {
		t.Fatalf("越权 spaceId 提交 = %d %v, want 200 domainId=%d（回落本域）", resp.StatusCode, out, env.domainA)
	}
	// 乙域榜（B1 视角）看不到 A1
	env.resetLimit()
	if resp, out := env.submit(t, env.cookieB1, "snake", map[string]any{"score": 55, "spaceId": env.spaceB}); resp.StatusCode != fiber.StatusOK {
		t.Fatalf("B1 提交 = %d %v", resp.StatusCode, out)
	}
	resp, out = env.rank(t, env.cookieB1, "snake", "domain", env.spaceB)
	if resp.StatusCode != fiber.StatusOK || out["domainId"] != float64(env.domainB) {
		t.Fatalf("乙域榜 = %d %v, want 200 domainId=%d", resp.StatusCode, out, env.domainB)
	}
	rows := rowsOf(t, out)
	if len(rows) != 1 {
		t.Fatalf("乙域榜行数 = %d, want 1（越权提交不进乙域榜）: %v", len(rows), rows)
	}
	assertRow(t, rows, 0, env.uidB1, "gameStuB1", 55, 1)

	// 3. 无空间成员：取不到域 → domainId=0（落 NULL，不属任何域）
	env.resetLimit()
	resp, out = env.submit(t, env.cookieLone, "snake", map[string]any{"score": 33})
	if resp.StatusCode != fiber.StatusOK || out["domainId"] != float64(0) {
		t.Fatalf("无空间成员提交 = %d %v, want 200 domainId=0", resp.StatusCode, out)
	}
	// 本域榜不能退化成全域：无空间归属时返回空榜 + noDomain 提示（domainID=0 是「全域榜单」的口径）
	env.resetLimit()
	resp, raw := doRaw(t, env.app, "GET", "/api/portal/game/snake/rank?scope=domain", env.cookieLone, nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("无空间成员本域榜 = %d %s, want 200", resp.StatusCode, raw)
	}
	if !strings.Contains(raw, `"rows":[]`) {
		t.Fatalf("无空间成员本域榜 rows 应为空数组（不得显示别域用户）: %s", raw)
	}
	_, out = doJSON(t, env.app, "GET", "/api/portal/game/snake/rank?scope=domain", env.cookieLone, nil)
	if out["domainId"] != float64(0) || out["domainName"] != "" {
		t.Fatalf("无空间成员本域榜 domainId/domainName = %v/%v, want 0/空", out["domainId"], out["domainName"])
	}
	if rows := rowsOf(t, out); len(rows) != 0 {
		t.Fatalf("无空间成员本域榜行数 = %d, want 0（不得退化为全域）: %v", len(rows), rows)
	}
	if out["noDomain"] != true {
		t.Fatalf("无空间成员本域榜 noDomain = %v, want true（前端据此提示「你还没有加入任何空间」）", out["noDomain"])
	}
	if hint, _ := out["scopeHint"].(string); hint == "" {
		t.Fatalf("无空间成员本域榜 scopeHint 为空: %v", out)
	}
	if out["myBest"] != float64(33) || out["myPlays"] != float64(1) {
		t.Fatalf("无空间成员本域榜 myBest/myPlays = %v/%v, want 33/1（自己的成绩仍可见）", out["myBest"], out["myPlays"])
	}
	// 全域榜仍可看（含自己的 33 分）
	resp, out = env.rank(t, env.cookieLone, "snake", "all", 0)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("无空间成员全域榜 = %d %v, want 200", resp.StatusCode, out)
	}
	rows = rowsOf(t, out)
	if len(rows) != 3 {
		t.Fatalf("无空间成员全域榜行数 = %d, want 3: %v", len(rows), rows)
	}
	if _, ok := out["noDomain"]; ok {
		t.Fatalf("全域榜不应带 noDomain: %v", out)
	}

	// 4. 系统管理员按 spaceId 指定空间 → 落在乙域
	env.resetLimit()
	resp, out = env.submit(t, env.cookieAdmin, "snake", map[string]any{"score": 44, "spaceId": env.spaceB})
	if resp.StatusCode != fiber.StatusOK || out["domainId"] != float64(env.domainB) {
		t.Fatalf("管理员指定 spaceId 提交 = %d %v, want 200 domainId=%d", resp.StatusCode, out, env.domainB)
	}
}
