// 休息时间小游戏成绩/榜单数据层回归测试（orangeoj.db，单库）。
// 覆盖：首次提交 / 低分不覆盖但累计 plays / 高分刷新 / 越界不写库 /
// 提交限频与 ResetGameScoreRateLimit / 非法 game 标识 / 本域与全域过滤 /
// 名次并列与顺序 / 我的排名（前 limit 内外的第二个返回值）/ MyGameScore。
//
// game_scores.domain_id 无外键约束（只记录归属，域合法性由 HTTP 层按空间解析），
// 因此本文件可直接用任意域 id 构造多域场景，无需建 domains/spaces。
package quizstore_test

import (
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"testing"

	"orangeoj/internal/accounts"
	"orangeoj/internal/quizstore"
)

// gameSnake 测试用 game 标识（合法：小写字母）。
const gameSnake = "snake"

// newGameStudent 建一名空间成员账号（game_scores.user_id 外键引用 users）。
func newGameStudent(t *testing.T, qs *quizstore.Store, name string) int64 {
	t.Helper()
	uid, err := qs.Accounts.CreateUser(name, "pw", accounts.RoleMember)
	if err != nil {
		t.Fatalf("create user %s: %v", name, err)
	}
	return uid
}

// resetGameRateLimit 清空进程内限频表并登记用例结束清理：
// SubmitGameScore 按 userID 限频（3s），同一用例内多次提交前必须重置。
func resetGameRateLimit(t *testing.T) {
	t.Helper()
	quizstore.ResetGameScoreRateLimit()
	t.Cleanup(quizstore.ResetGameScoreRateLimit)
}

// gameRow 直读库内该用户该游戏的行（ok=false 表示无记录）。
func gameRow(t *testing.T, qs *quizstore.Store, game string, uid int64) (best, plays int, domain sql.NullInt64, ok bool) {
	t.Helper()
	err := qs.DB.QueryRow(`SELECT best_score, plays, domain_id FROM game_scores WHERE game=? AND user_id=?`, game, uid).
		Scan(&best, &plays, &domain)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, sql.NullInt64{}, false
	}
	if err != nil {
		t.Fatalf("读取 game_scores(%q,%d): %v", game, uid, err)
	}
	return best, plays, domain, true
}

// gameScoreTotal 全表行数（非法/越界提交不得写库）。
func gameScoreTotal(t *testing.T, qs *quizstore.Store) int {
	t.Helper()
	var n int
	if err := qs.DB.QueryRow(`SELECT COUNT(1) FROM game_scores`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// submitOK 提交一次成绩并要求成功（不重置限频，由调用方控制）。
func submitOK(t *testing.T, qs *quizstore.Store, game string, uid, domainID int64, score int) (int, bool) {
	t.Helper()
	best, isNew, err := qs.SubmitGameScore(game, uid, domainID, score)
	if err != nil {
		t.Fatalf("SubmitGameScore(%q,%d,domain=%d,score=%d): %v", game, uid, domainID, score, err)
	}
	return best, isNew
}

// gameSeed 造榜数据：(用户, 域, 分数)。
type gameSeed struct {
	userID   int64
	domainID int64
	score    int
}

// seedGame 逐个用户提交一次成绩（每条前重置限频，保证只受用例自身影响）。
func seedGame(t *testing.T, qs *quizstore.Store, game string, seeds []gameSeed) {
	t.Helper()
	for _, s := range seeds {
		resetGameRateLimit(t)
		submitOK(t, qs, game, s.userID, s.domainID, s.score)
	}
}

// rankBrief 便于失败信息阅读：用户 + 分数 + 名次。
func rankBrief(rows []quizstore.GameScore) string {
	parts := make([]string, 0, len(rows))
	for _, r := range rows {
		parts = append(parts, r.UserName+"("+strconv.Itoa(r.BestScore)+")/rank"+strconv.Itoa(r.Rank))
	}
	return strings.Join(parts, ", ")
}

// ---------- 1. 首次提交 ----------

// TestSubmitGameScoreFirstInsert 首次提交：插入成功，best=score、plays=1、isNewBest=true。
func TestSubmitGameScoreFirstInsert(t *testing.T) {
	qs := newTestEnvironment(t)
	resetGameRateLimit(t)
	uid := newGameStudent(t, qs, "gs-first")

	best, isNew := submitOK(t, qs, gameSnake, uid, 7, 70)
	if best != 70 || !isNew {
		t.Fatalf("首次提交返回 (best=%d, isNewBest=%v), want (70, true)", best, isNew)
	}
	gotBest, plays, domain, ok := gameRow(t, qs, gameSnake, uid)
	if !ok || gotBest != 70 || plays != 1 {
		t.Fatalf("库内行 (best=%d, plays=%d, ok=%v), want (70, 1, true)", gotBest, plays, ok)
	}
	if !domain.Valid || domain.Int64 != 7 {
		t.Fatalf("库内 domain_id = %v, want 7", domain)
	}
}

// ---------- 2. 低分提交 ----------

// TestSubmitGameScoreLowScoreKeepsBest 低分提交：best 不变、plays+1、isNewBest=false。
func TestSubmitGameScoreLowScoreKeepsBest(t *testing.T) {
	qs := newTestEnvironment(t)
	resetGameRateLimit(t)
	uid := newGameStudent(t, qs, "gs-low")

	submitOK(t, qs, gameSnake, uid, 3, 50)

	resetGameRateLimit(t)
	best, isNew := submitOK(t, qs, gameSnake, uid, 3, 30)
	if best != 50 || isNew {
		t.Fatalf("低分提交返回 (best=%d, isNewBest=%v), want (50, false)", best, isNew)
	}
	gotBest, plays, _, _ := gameRow(t, qs, gameSnake, uid)
	if gotBest != 50 || plays != 2 {
		t.Fatalf("低分提交后库内 (best=%d, plays=%d), want (50, 2)", gotBest, plays)
	}
}

// ---------- 3. 高分提交 ----------

// TestSubmitGameScoreHighScoreUpdatesBest 高分提交：best 更新、isNewBest=true、plays+1。
func TestSubmitGameScoreHighScoreUpdatesBest(t *testing.T) {
	qs := newTestEnvironment(t)
	resetGameRateLimit(t)
	uid := newGameStudent(t, qs, "gs-high")

	submitOK(t, qs, gameSnake, uid, 3, 50)

	resetGameRateLimit(t)
	best, isNew := submitOK(t, qs, gameSnake, uid, 3, 80)
	if best != 80 || !isNew {
		t.Fatalf("高分提交返回 (best=%d, isNewBest=%v), want (80, true)", best, isNew)
	}
	gotBest, plays, _, _ := gameRow(t, qs, gameSnake, uid)
	if gotBest != 80 || plays != 2 {
		t.Fatalf("高分提交后库内 (best=%d, plays=%d), want (80, 2)", gotBest, plays)
	}
}

// ---------- 4. 越界（含边界合法值） ----------

// TestSubmitGameScoreOutOfRangeNotWritten 越界分数 -1 / MaxGameScore+1 报错且不写库。
func TestSubmitGameScoreOutOfRangeNotWritten(t *testing.T) {
	qs := newTestEnvironment(t)
	resetGameRateLimit(t)
	uid := newGameStudent(t, qs, "gs-range")

	for _, bad := range []int{-1, quizstore.MaxGameScore + 1} {
		// 越界在限频判定之前返回，未污染限频表，可连续提交
		best, isNew, err := qs.SubmitGameScore(gameSnake, uid, 1, bad)
		if err == nil {
			t.Fatalf("score=%d 应被拒绝，实际返回 (best=%d, isNewBest=%v, nil)", bad, best, isNew)
		}
		if !strings.Contains(err.Error(), "分数超出允许范围") {
			t.Fatalf("score=%d 错误 = %v, want 含「分数超出允许范围」", bad, err)
		}
	}
	if n := gameScoreTotal(t, qs); n != 0 {
		t.Fatalf("越界提交后 game_scores 行数 = %d, want 0（不得写库）", n)
	}
	if best, plays, err := qs.MyGameScore(gameSnake, uid); err != nil || best != 0 || plays != 0 {
		t.Fatalf("越界提交后 MyGameScore = (%d, %d, %v), want (0, 0, nil)", best, plays, err)
	}
}

// TestSubmitGameScoreBoundaries 区间端点合法：0 与 MaxGameScore 都可写入（闭区间）。
func TestSubmitGameScoreBoundaries(t *testing.T) {
	qs := newTestEnvironment(t)
	resetGameRateLimit(t)
	uid := newGameStudent(t, qs, "gs-bound")

	if best, isNew := submitOK(t, qs, gameSnake, uid, 1, 0); best != 0 || !isNew {
		t.Fatalf("score=0 返回 (best=%d, isNewBest=%v), want (0, true)", best, isNew)
	}
	resetGameRateLimit(t)
	if best, isNew := submitOK(t, qs, gameSnake, uid, 1, quizstore.MaxGameScore); best != quizstore.MaxGameScore || !isNew {
		t.Fatalf("score=MaxGameScore 返回 (best=%d, isNewBest=%v), want (%d, true)", best, isNew, quizstore.MaxGameScore)
	}
	gotBest, plays, _, _ := gameRow(t, qs, gameSnake, uid)
	if gotBest != quizstore.MaxGameScore || plays != 2 {
		t.Fatalf("端点提交后库内 (best=%d, plays=%d), want (%d, 2)", gotBest, plays, quizstore.MaxGameScore)
	}
}

// ---------- 5. 限频 ----------

// TestSubmitGameScoreRateLimit 3 秒内同用户第二次提交 → ErrGameScoreTooFrequent，
// 被拒提交不写库；限频按 userID 分键（不牵连其他用户）；重置后可再次提交。
func TestSubmitGameScoreRateLimit(t *testing.T) {
	qs := newTestEnvironment(t)
	resetGameRateLimit(t)
	uid := newGameStudent(t, qs, "gs-rate")

	submitOK(t, qs, gameSnake, uid, 1, 10)

	// 同用户、同 game：被限频
	if _, _, err := qs.SubmitGameScore(gameSnake, uid, 1, 20); !errors.Is(err, quizstore.ErrGameScoreTooFrequent) {
		t.Fatalf("限频内第二次提交 err = %v, want ErrGameScoreTooFrequent", err)
	}
	// 同用户、另一个 game 也被限频（限频键是 userID，与 game 无关）
	if _, _, err := qs.SubmitGameScore("tetris", uid, 1, 99); !errors.Is(err, quizstore.ErrGameScoreTooFrequent) {
		t.Fatalf("限频内换 game 提交 err = %v, want ErrGameScoreTooFrequent", err)
	}
	// 被拒提交不留痕：best/plays 仍是首次结果，tetris 无行
	best, plays, _, _ := gameRow(t, qs, gameSnake, uid)
	if best != 10 || plays != 1 {
		t.Fatalf("限频后库内 (best=%d, plays=%d), want (10, 1)", best, plays)
	}
	if _, _, _, ok := gameRow(t, qs, "tetris", uid); ok {
		t.Fatal("被限频的 tetris 提交不应写库")
	}
	// 限频按 userID 分键：另一个用户不受影响
	other := newGameStudent(t, qs, "gs-rate-other")
	submitOK(t, qs, gameSnake, other, 1, 5)

	// 重置限频表后可再次提交
	quizstore.ResetGameScoreRateLimit()
	if best, isNew := submitOK(t, qs, gameSnake, uid, 1, 20); best != 20 || !isNew {
		t.Fatalf("重置限频后提交返回 (best=%d, isNewBest=%v), want (20, true)", best, isNew)
	}
	if _, plays, _, _ := gameRow(t, qs, gameSnake, uid); plays != 2 {
		t.Fatalf("重置限频后 plays = %d, want 2", plays)
	}
}

// ---------- 6. 非法 game 标识 ----------

// TestValidateGameID 合法/非法标识判定（^[a-z0-9_-]{1,32}$）。
func TestValidateGameID(t *testing.T) {
	for _, ok := range []string{"snake", "2048", "snake-2", "a_b-c9", strings.Repeat("a", 32)} {
		if !quizstore.ValidateGameID(ok) {
			t.Fatalf("ValidateGameID(%q) = false, want true", ok)
		}
	}
	bad := []struct {
		name string
		game string
	}{
		{"空串", ""},
		{"超 32 字符", strings.Repeat("a", 33)},
		{"大写字母", "Snake"},
		{"中文", "贪吃蛇"},
		{"含空格", "my game"},
		{"含点", "snake.v2"},
		{"含斜杠", "a/b"},
	}
	for _, tc := range bad {
		if quizstore.ValidateGameID(tc.game) {
			t.Fatalf("%s: ValidateGameID(%q) = true, want false", tc.name, tc.game)
		}
	}
}

// TestInvalidGameIDRejected 非法 game 标识下 SubmitGameScore / ListGameRank 均报错且不写库。
func TestInvalidGameIDRejected(t *testing.T) {
	qs := newTestEnvironment(t)
	resetGameRateLimit(t)
	uid := newGameStudent(t, qs, "gs-badid")

	// 合法 game 先写一行，便于断言「非法提交不新增行」
	resetGameRateLimit(t)
	submitOK(t, qs, gameSnake, uid, 1, 10)

	for _, bad := range []string{"", strings.Repeat("a", 33), "Snake", "贪吃蛇", "my game"} {
		if _, _, err := qs.SubmitGameScore(bad, uid, 1, 10); err == nil || !strings.Contains(err.Error(), "非法游戏标识") {
			t.Fatalf("SubmitGameScore(%q) err = %v, want 含「非法游戏标识」", bad, err)
		}
		if _, _, err := qs.ListGameRank(bad, 0, uid, 10); err == nil || !strings.Contains(err.Error(), "非法游戏标识") {
			t.Fatalf("ListGameRank(%q) err = %v, want 含「非法游戏标识」", bad, err)
		}
	}
	if n := gameScoreTotal(t, qs); n != 1 {
		t.Fatalf("非法标识提交后 game_scores 行数 = %d, want 1（仅先前合法那行）", n)
	}
}

// TestNormalizeGameID 规范化：去首尾空白 + 小写（HTTP 层入口用）。
func TestNormalizeGameID(t *testing.T) {
	cases := map[string]string{
		"  Snake  ": "snake",
		"SNAKE":     "snake",
		"snake":     "snake",
		"\t2048\n":  "2048",
		"":          "",
	}
	for in, want := range cases {
		if got := quizstore.NormalizeGameID(in); got != want {
			t.Fatalf("NormalizeGameID(%q) = %q, want %q", in, got, want)
		}
	}
}

// ---------- 7. 本域 / 全域过滤 ----------

// TestListGameRankDomainFilter domainID>0 只取该域；domainID=0 取全部域。
func TestListGameRankDomainFilter(t *testing.T) {
	qs := newTestEnvironment(t)
	resetGameRateLimit(t)
	const domainA, domainB = int64(11), int64(22)
	uA1 := newGameStudent(t, qs, "gs-a1")
	uA2 := newGameStudent(t, qs, "gs-a2")
	uB1 := newGameStudent(t, qs, "gs-b1")
	seedGame(t, qs, gameSnake, []gameSeed{
		{uA1, domainA, 10}, {uA2, domainA, 20}, {uB1, domainB, 30},
	})

	domainRows, mineDomain, err := qs.ListGameRank(gameSnake, domainA, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(domainRows) != 2 {
		t.Fatalf("本域榜单 = %d 行 (%s), want 2 行（仅 domainA）", len(domainRows), rankBrief(domainRows))
	}
	for _, r := range domainRows {
		if r.DomainID == nil || *r.DomainID != domainA {
			t.Fatalf("本域榜单混入其他域: %s domainId=%v", r.UserName, r.DomainID)
		}
	}
	// 分数降序：uA2(20) 在 uA1(10) 之前
	if domainRows[0].UserID != uA2 || domainRows[1].UserID != uA1 {
		t.Fatalf("本域榜单顺序 = %s, want gs-a2(20) → gs-a1(10)", rankBrief(domainRows))
	}
	if mineDomain != nil {
		t.Fatalf("forUserID=0 时第二个返回值 = %+v, want nil", mineDomain)
	}

	allRows, mineAll, err := qs.ListGameRank(gameSnake, 0, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(allRows) != 3 {
		t.Fatalf("全域榜单 = %d 行 (%s), want 3 行", len(allRows), rankBrief(allRows))
	}
	if allRows[0].UserID != uB1 || allRows[1].UserID != uA2 || allRows[2].UserID != uA1 {
		t.Fatalf("全域榜单顺序 = %s, want gs-b1(30) → gs-a2(20) → gs-a1(10)", rankBrief(allRows))
	}
	if mineAll != nil {
		t.Fatalf("forUserID=0 时第二个返回值 = %+v, want nil", mineAll)
	}
}

// ---------- 8. 名次并列与顺序 ----------

// TestListGameRankTiesShareRank 同分并列同名次：RANK() 给出 1,1,3（不是 1,2,3），
// 且整体按分数降序。同分并列要求 RANK() 的排序键完全一致，故显式对齐 updated_at
// （提交间隔为秒级时间戳，天然并列；此处对齐以保证确定性）。
func TestListGameRankTiesShareRank(t *testing.T) {
	qs := newTestEnvironment(t)
	resetGameRateLimit(t)
	u1 := newGameStudent(t, qs, "gs-tie1")
	u2 := newGameStudent(t, qs, "gs-tie2")
	u3 := newGameStudent(t, qs, "gs-tie3")
	seedGame(t, qs, gameSnake, []gameSeed{{u1, 1, 100}, {u2, 1, 100}, {u3, 1, 50}})

	if _, err := qs.DB.Exec(`UPDATE game_scores SET updated_at='2024-01-01 00:00:00' WHERE game=?`, gameSnake); err != nil {
		t.Fatal(err)
	}
	rows, _, err := qs.ListGameRank(gameSnake, 0, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("榜单 = %d 行 (%s), want 3 行", len(rows), rankBrief(rows))
	}
	// 分数降序
	for i := 1; i < len(rows); i++ {
		if rows[i-1].BestScore < rows[i].BestScore {
			t.Fatalf("榜单未按分数降序: %s", rankBrief(rows))
		}
	}
	// 并列两人同为 rank 1（顺序不定，按集合断言）
	tied := map[int64]bool{rows[0].UserID: true, rows[1].UserID: true}
	if !tied[u1] || !tied[u2] {
		t.Fatalf("前两行 = (%d, %d), want {%d, %d}", rows[0].UserID, rows[1].UserID, u1, u2)
	}
	if rows[0].Rank != 1 || rows[1].Rank != 1 {
		t.Fatalf("并列名次 = (%d, %d), want (1, 1)", rows[0].Rank, rows[1].Rank)
	}
	if rows[0].BestScore != 100 || rows[1].BestScore != 100 {
		t.Fatalf("并列分数 = (%d, %d), want (100, 100)", rows[0].BestScore, rows[1].BestScore)
	}
	// 第三名：RANK() 跳号为 3
	if rows[2].UserID != u3 || rows[2].Rank != 3 || rows[2].BestScore != 50 {
		t.Fatalf("第三行 = (%d, score=%d, rank=%d), want (%d, 50, 3)", rows[2].UserID, rows[2].BestScore, rows[2].Rank, u3)
	}
}

// TestListGameRankTieDependsOnUpdatedAt 同分即并列：分数相同、updated_at 不同时
// 两人都应为 1 名（RANK() 只按 best_score）；显示次序仍按「更早达到者靠前」。
func TestListGameRankTieDependsOnUpdatedAt(t *testing.T) {
	qs := newTestEnvironment(t)
	resetGameRateLimit(t)
	early := newGameStudent(t, qs, "gs-early")
	late := newGameStudent(t, qs, "gs-late")
	seedGame(t, qs, gameSnake, []gameSeed{{early, 1, 100}, {late, 1, 100}})

	if _, err := qs.DB.Exec(`UPDATE game_scores SET updated_at='2024-01-01 00:00:00' WHERE game=? AND user_id=?`, gameSnake, early); err != nil {
		t.Fatal(err)
	}
	if _, err := qs.DB.Exec(`UPDATE game_scores SET updated_at='2024-02-01 00:00:00' WHERE game=? AND user_id=?`, gameSnake, late); err != nil {
		t.Fatal(err)
	}
	rows, _, err := qs.ListGameRank(gameSnake, 0, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("榜单 = %d 行 (%s), want 2 行", len(rows), rankBrief(rows))
	}
	if rows[0].UserID != early || rows[0].Rank != 1 || rows[0].BestScore != 100 {
		t.Fatalf("首行 = (%d, score=%d, rank=%d), want (%d, 100, 1)（更早达到者靠前）",
			rows[0].UserID, rows[0].BestScore, rows[0].Rank, early)
	}
	if rows[1].UserID != late || rows[1].Rank != 1 || rows[1].BestScore != 100 {
		t.Fatalf("次行 = (%d, score=%d, rank=%d), want (%d, 100, 1)（同分即并列，不受达成时间影响）",
			rows[1].UserID, rows[1].BestScore, rows[1].Rank, late)
	}
}

// ---------- 9. 我的排名（前 limit 内外） ----------

// TestListGameRankMyRowOutsideLimit 我不在前 limit 时：第二个返回值给出我的行（含最高分、
// 次数、域、IsMe）与**真实名次**；我在前 limit 内时第二个返回值为 nil（我的行已在列表中，IsMe=true）。
//
// 回归点：名次必须在全量榜内算出来再筛出我这一行——窗口函数不能与 user_id 过滤写在同一层，
// 否则窗口内只剩一行、RANK() 恒为 1（曾经如此）。
func TestListGameRankMyRowOutsideLimit(t *testing.T) {
	qs := newTestEnvironment(t)
	resetGameRateLimit(t)
	leader := newGameStudent(t, qs, "gs-lead")
	me := newGameStudent(t, qs, "gs-me")
	seedGame(t, qs, gameSnake, []gameSeed{{leader, 5, 100}, {me, 5, 50}})

	rows, mine, err := qs.ListGameRank(gameSnake, 5, me, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].UserID != leader || rows[0].IsMe {
		t.Fatalf("limit=1 榜单 = %s, want 仅 gs-lead 且 IsMe=false", rankBrief(rows))
	}
	if mine == nil {
		t.Fatal("我不在前 limit 内时第二个返回值 = nil, want 我的行")
	}
	if mine.UserID != me || mine.BestScore != 50 || mine.Plays != 1 || !mine.IsMe {
		t.Fatalf("我的行 = %+v, want (user=%d, best=50, plays=1, isMe=true)", mine, me)
	}
	if mine.UserName != "gs-me" {
		t.Fatalf("我的行 userName = %q, want gs-me", mine.UserName)
	}
	if mine.DomainID == nil || *mine.DomainID != 5 {
		t.Fatalf("我的行 domainId = %v, want 5", mine.DomainID)
	}
	// 名次：领先者 100 分、我 50 分 → 我应为第 2 名（断言具体数值，防止退回「恒为 1」）
	if mine.Rank != 2 {
		t.Fatalf("我的名次 = %d, want 2（窗口排名须在全量榜内计算后再筛出我这一行）", mine.Rank)
	}

	// 我在前 limit 内 → 第二个返回值为 nil，列表中我的行 IsMe=true
	rows, mine, err = qs.ListGameRank(gameSnake, 5, leader, 1)
	if err != nil {
		t.Fatal(err)
	}
	if mine != nil {
		t.Fatalf("我在前 limit 内时第二个返回值 = %+v, want nil", mine)
	}
	if len(rows) != 1 || rows[0].UserID != leader || !rows[0].IsMe || rows[0].Rank != 1 {
		t.Fatalf("榜单 = %s, want 仅 gs-lead IsMe=true rank=1", rankBrief(rows))
	}

	// 无成绩用户：不返回「我的行」（没有记录就没有名次）
	stranger := newGameStudent(t, qs, "gs-stranger")
	_, mine, err = qs.ListGameRank(gameSnake, 5, stranger, 1)
	if err != nil {
		t.Fatal(err)
	}
	if mine != nil {
		t.Fatalf("无成绩用户的第二个返回值 = %+v, want nil", mine)
	}
}

// ---------- 10. MyGameScore ----------

// TestMyGameScore 无记录返回 (0,0,nil)；有记录返回当前 best/plays（低分不覆盖但累计）。
func TestMyGameScore(t *testing.T) {
	qs := newTestEnvironment(t)
	resetGameRateLimit(t)
	uid := newGameStudent(t, qs, "gs-mine")

	// 无记录
	if best, plays, err := qs.MyGameScore(gameSnake, uid); err != nil || best != 0 || plays != 0 {
		t.Fatalf("无记录 MyGameScore = (%d, %d, %v), want (0, 0, nil)", best, plays, err)
	}
	submitOK(t, qs, gameSnake, uid, 1, 30)
	resetGameRateLimit(t)
	submitOK(t, qs, gameSnake, uid, 1, 10)

	if best, plays, err := qs.MyGameScore(gameSnake, uid); err != nil || best != 30 || plays != 2 {
		t.Fatalf("有记录 MyGameScore = (%d, %d, %v), want (30, 2, nil)", best, plays, err)
	}
	// 另一个 game 仍无记录
	if best, plays, err := qs.MyGameScore("tetris", uid); err != nil || best != 0 || plays != 0 {
		t.Fatalf("另一 game MyGameScore = (%d, %d, %v), want (0, 0, nil)", best, plays, err)
	}
}
