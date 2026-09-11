// 云端代码草稿数据层回归测试（internal/quizstore/code_drafts.go）。
//
// 覆盖：无记录零值、保存/覆盖后 UpdatedAt 前进、存量两种 updated_at 格式
// （RFC3339 与 SQLite CURRENT_TIMESTAMP）统一按 UTC 解析、语言与上下文隔离。
//
// 关键回归点：UpdatedAt 必须是「非零 + UTC」的确定时刻。HTTP 层直接
// d.UpdatedAt.Format(time.RFC3339) 下发，前端按本机时区解析后与 localStorage
// 记录的时间戳比较；若这里退化成无时区/本地时区，跨端判断会差若干小时并反向，
// 用旧草稿覆盖新草稿。故此处逐个钉住「可解析 / 带时区 / 落在当前时间窗」。
package quizstore_test

import (
	"strings"
	"testing"
	"time"

	"orangeoj/internal/accounts"
	"orangeoj/internal/quizstore"
)

// draftProblemID 本环境已建的编程题 id（code_drafts.problem_id 无外键，仅作键使用）。
const draftProblemID = int64(5)

// newDraftUser 建一名学生：code_drafts.user_id 外键引用 users，必须是真实用户。
func newDraftUser(t *testing.T, qs *quizstore.Store, username string) int64 {
	t.Helper()
	uid, err := qs.Accounts.CreateUser(username, "pw", accounts.RoleMember)
	if err != nil {
		t.Fatalf("create user %q: %v", username, err)
	}
	return uid
}

// insertRawDraft 直接用 SQL 写一条「全局槽」（ctx_kind 为空串 / ctx_id=0）的存量行，
// 绕过 SaveDraft 以模拟老库数据；updated_at 原样写入（TEXT 存储，不被改写）。
// problem_id 无外键约束，未建题的 id 亦合法。
func insertRawDraft(t *testing.T, qs *quizstore.Store, userID, problemID int64, language, code, rawUpdated string) {
	t.Helper()
	if _, err := qs.DB.Exec(`INSERT INTO code_drafts(user_id,problem_id,language,ctx_kind,ctx_id,code,updated_at)
		VALUES(?,?,?,'',0,?,?)`, userID, problemID, language, code, rawUpdated); err != nil {
		t.Fatalf("insert raw draft(problem=%d lang=%s updated_at=%q): %v", problemID, language, rawUpdated, err)
	}
}

// TestDraftGetNoRecordZeroValue 无记录 → 返回零值 Draft（Code 空、UpdatedAt 零值）且 err 为 nil。
func TestDraftGetNoRecordZeroValue(t *testing.T) {
	qs := newTestEnvironment(t)
	uid := newDraftUser(t, qs, "drafter")

	d, err := qs.GetDraft(uid, draftProblemID, "python", "", 0)
	if err != nil {
		t.Fatalf("GetDraft 无记录应不报错: %v", err)
	}
	if d.Code != "" {
		t.Fatalf("Code = %q, want 空串", d.Code)
	}
	if !d.UpdatedAt.IsZero() {
		t.Fatalf("UpdatedAt = %v, want 零值（前端据此判定无云端草稿）", d.UpdatedAt)
	}
	// 训练/练习上下文未保存同样为零值
	for _, ctx := range []struct {
		kind string
		id   int64
	}{{"training", 7}, {"practice", 9}} {
		d2, err := qs.GetDraft(uid, draftProblemID, "python", ctx.kind, ctx.id)
		if err != nil {
			t.Fatalf("GetDraft(%s/%d) 无记录应不报错: %v", ctx.kind, ctx.id, err)
		}
		if d2.Code != "" || !d2.UpdatedAt.IsZero() {
			t.Fatalf("GetDraft(%s/%d) = %+v, want 零值", ctx.kind, ctx.id, d2)
		}
	}
}

// TestDraftSaveThenGetUpdatedAtNearNow 保存后读取：Code 一致、UpdatedAt 非零且为 UTC，
// 时刻落在本机当前时间窗内（SQLite CURRENT_TIMESTAMP 秒级截断，放宽 1 秒）。
func TestDraftSaveThenGetUpdatedAtNearNow(t *testing.T) {
	qs := newTestEnvironment(t)
	uid := newDraftUser(t, qs, "drafter")

	const code = "print('draft')\n"
	lower := time.Now().UTC().Add(-time.Second) // 秒级截断容差
	if err := qs.SaveDraft(uid, draftProblemID, "python", "", 0, code); err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}
	upper := time.Now().UTC().Add(time.Second)

	d, err := qs.GetDraft(uid, draftProblemID, "python", "", 0)
	if err != nil {
		t.Fatalf("GetDraft: %v", err)
	}
	if d.Code != code {
		t.Fatalf("Code = %q, want %q", d.Code, code)
	}
	if d.UpdatedAt.IsZero() {
		t.Fatal("UpdatedAt 为零值, want 保存时间")
	}
	if d.UpdatedAt.Location() != time.UTC {
		t.Fatalf("UpdatedAt 时区 = %v, want UTC（HTTP 层 Format(RFC3339) 依赖 UTC → Z 后缀）", d.UpdatedAt.Location())
	}
	if d.UpdatedAt.Before(lower) || d.UpdatedAt.After(upper) {
		t.Fatalf("UpdatedAt = %v, want 落在本机 UTC 保存窗口 [%v, %v]", d.UpdatedAt, lower, upper)
	}
	// 与当前时间相差 ≤5 分钟（若按本地时区误解析，此处会差数小时）
	if delta := time.Since(d.UpdatedAt); delta < -5*time.Minute || delta > 5*time.Minute {
		t.Fatalf("UpdatedAt 与当前时间相差 %v, want ≤5 分钟", delta)
	}
	// RFC3339 往返：UTC 时刻格式化后必带 Z（前端 Date.parse 据此判定时区）
	if got := d.UpdatedAt.Format(time.RFC3339); !strings.HasSuffix(got, "Z") {
		t.Fatalf("Format(RFC3339) = %q, want UTC 带 Z 后缀", got)
	}
}

// TestDraftOverwriteAdvancesUpdatedAt 覆盖保存：行数仍为 1（upsert），
// 且 UpdatedAt 严格前进（第二次前 sleep 越过 CURRENT_TIMESTAMP 秒级精度）。
func TestDraftOverwriteAdvancesUpdatedAt(t *testing.T) {
	qs := newTestEnvironment(t)
	uid := newDraftUser(t, qs, "drafter")

	if err := qs.SaveDraft(uid, draftProblemID, "python", "", 0, "first"); err != nil {
		t.Fatalf("SaveDraft first: %v", err)
	}
	first, err := qs.GetDraft(uid, draftProblemID, "python", "", 0)
	if err != nil {
		t.Fatalf("GetDraft first: %v", err)
	}
	if first.Code != "first" {
		t.Fatalf("first.Code = %q, want %q", first.Code, "first")
	}
	if first.UpdatedAt.IsZero() {
		t.Fatal("first.UpdatedAt 为零值, want 保存时间")
	}

	time.Sleep(1100 * time.Millisecond) // CURRENT_TIMESTAMP 精度到秒

	if err := qs.SaveDraft(uid, draftProblemID, "python", "", 0, "second"); err != nil {
		t.Fatalf("SaveDraft second: %v", err)
	}
	second, err := qs.GetDraft(uid, draftProblemID, "python", "", 0)
	if err != nil {
		t.Fatalf("GetDraft second: %v", err)
	}
	if second.Code != "second" {
		t.Fatalf("second.Code = %q, want %q", second.Code, "second")
	}
	if !second.UpdatedAt.After(first.UpdatedAt) {
		t.Fatalf("覆盖后 UpdatedAt = %v, want 严格晚于 %v（前端「仅云端严格更新才覆盖本地」依赖此单调性）",
			second.UpdatedAt, first.UpdatedAt)
	}
	if d := second.UpdatedAt.Sub(first.UpdatedAt); d < time.Second {
		t.Fatalf("两次保存 UpdatedAt 间隔 = %v, want ≥1s（sleep 已越过秒级精度）", d)
	}
	// upsert 语义：同键仍只有 1 行
	var n int
	if err := qs.DB.QueryRow(`SELECT COUNT(1) FROM code_drafts WHERE user_id=? AND problem_id=?`,
		uid, draftProblemID).Scan(&n); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if n != 1 {
		t.Fatalf("覆盖保存后行数 = %d, want 1（upsert）", n)
	}
}

// TestDraftLegacyUpdatedAtFormatsParsedAsUTC 存量两种 updated_at 格式均按 UTC 正确解析：
//   - RFC3339（"2026-01-02T03:04:05Z"）
//   - SQLite CURRENT_TIMESTAMP（"2026-01-02 03:04:05"，UTC 语义、无时区标记）
//
// 两者必须得到同一时刻；无法解析的值（"garbage"）返回零值而非报错。
func TestDraftLegacyUpdatedAtFormatsParsedAsUTC(t *testing.T) {
	qs := newTestEnvironment(t)
	uid := newDraftUser(t, qs, "drafter")

	want := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	cases := []struct {
		name      string
		language  string
		raw       string
		wantUTC   time.Time
		wantEmpty bool
	}{
		{name: "RFC3339 带 Z", language: "python", raw: "2026-01-02T03:04:05Z", wantUTC: want},
		{name: "SQLite CURRENT_TIMESTAMP", language: "cpp", raw: "2026-01-02 03:04:05", wantUTC: want},
		// 无法解析的存量值 → 零值（不报错）；用未建题的 id 隔离该行
		{name: "不可解析", language: "python", raw: "garbage", wantEmpty: true},
	}
	for i, tc := range cases {
		problemID := draftProblemID
		if tc.wantEmpty {
			problemID = draftProblemID + 1000 + int64(i) // 与上面两行不同键，互不影响
		}
		insertRawDraft(t, qs, uid, problemID, tc.language, "legacy-"+tc.name, tc.raw)

		d, err := qs.GetDraft(uid, problemID, tc.language, "", 0)
		if err != nil {
			t.Fatalf("%s: GetDraft 不应报错: %v", tc.name, err)
		}
		if d.Code != "legacy-"+tc.name {
			t.Fatalf("%s: Code = %q, want %q", tc.name, d.Code, "legacy-"+tc.name)
		}
		if tc.wantEmpty {
			if !d.UpdatedAt.IsZero() {
				t.Fatalf("%s: UpdatedAt = %v, want 零值（raw=%q）", tc.name, d.UpdatedAt, tc.raw)
			}
			continue
		}
		if d.UpdatedAt.IsZero() {
			t.Fatalf("%s: UpdatedAt 为零值（raw=%q 未解析）", tc.name, tc.raw)
		}
		if !d.UpdatedAt.Equal(tc.wantUTC) {
			t.Fatalf("%s: UpdatedAt = %v, want %v（按 UTC 解析）", tc.name, d.UpdatedAt, tc.wantUTC)
		}
		if d.UpdatedAt.Location() != time.UTC {
			t.Fatalf("%s: 时区 = %v, want UTC（SQLite 存量格式按本地时区解析会差整数小时）",
				tc.name, d.UpdatedAt.Location())
		}
		// 年月日时分秒逐项断言（防止只对时间戳、错墙上时钟）
		y, mo, day := d.UpdatedAt.Date()
		h, mi, sec := d.UpdatedAt.Clock()
		if y != 2026 || mo != time.January || day != 2 || h != 3 || mi != 4 || sec != 5 {
			t.Fatalf("%s: 墙上时钟 = %04d-%02d-%02d %02d:%02d:%02d, want 2026-01-02 03:04:05 UTC",
				tc.name, y, mo, day, h, mi, sec)
		}
		// 下发到 HTTP 层的字符串必须是带时区的 RFC3339
		if got := d.UpdatedAt.Format(time.RFC3339); got != "2026-01-02T03:04:05Z" {
			t.Fatalf("%s: Format(RFC3339) = %q, want %q", tc.name, got, "2026-01-02T03:04:05Z")
		}
	}
}

// TestDraftContextAndLanguageIsolation 同题不同 语言/上下文 各自独立：
// 覆盖某一槽不影响其它槽（前端按上下文恢复草稿，串味即丢代码）。
func TestDraftContextAndLanguageIsolation(t *testing.T) {
	qs := newTestEnvironment(t)
	uid := newDraftUser(t, qs, "drafter")
	other := newDraftUser(t, qs, "drafter2")

	type slot struct {
		language string
		ctxKind  string
		ctxID    int64
	}
	seed := []struct {
		slot slot
		code string
	}{
		{slot{"python", "", 0}, "global-py"},
		{slot{"cpp", "", 0}, "global-cpp"},
		{slot{"python", "training", 7}, "train-7-py"},
		{slot{"python", "training", 8}, "train-8-py"},
		{slot{"python", "practice", 7}, "practice-7-py"},
	}
	for _, row := range seed {
		if err := qs.SaveDraft(uid, draftProblemID, row.slot.language, row.slot.ctxKind, row.slot.ctxID, row.code); err != nil {
			t.Fatalf("SaveDraft %+v: %v", row.slot, err)
		}
	}
	for _, row := range seed {
		d, err := qs.GetDraft(uid, draftProblemID, row.slot.language, row.slot.ctxKind, row.slot.ctxID)
		if err != nil {
			t.Fatalf("GetDraft %+v: %v", row.slot, err)
		}
		if d.Code != row.code {
			t.Fatalf("GetDraft %+v = %q, want %q", row.slot, d.Code, row.code)
		}
		if d.UpdatedAt.IsZero() {
			t.Fatalf("GetDraft %+v: UpdatedAt 零值, want 保存时间", row.slot)
		}
	}

	// 覆盖训练 7 的草稿：其它槽（含全局与同 ctxKind 的其它 id）不受影响
	if err := qs.SaveDraft(uid, draftProblemID, "python", "training", 7, "train-7-py-v2"); err != nil {
		t.Fatalf("SaveDraft 覆盖训练草稿: %v", err)
	}
	after := []struct {
		slot slot
		code string
	}{
		{slot{"python", "", 0}, "global-py"},
		{slot{"cpp", "", 0}, "global-cpp"},
		{slot{"python", "training", 7}, "train-7-py-v2"},
		{slot{"python", "training", 8}, "train-8-py"},
		{slot{"python", "practice", 7}, "practice-7-py"},
	}
	for _, row := range after {
		d, err := qs.GetDraft(uid, draftProblemID, row.slot.language, row.slot.ctxKind, row.slot.ctxID)
		if err != nil {
			t.Fatalf("GetDraft %+v: %v", row.slot, err)
		}
		if d.Code != row.code {
			t.Fatalf("覆盖训练草稿后 GetDraft %+v = %q, want %q（上下文串味）", row.slot, d.Code, row.code)
		}
	}

	// 另一用户同槽无草稿（按 user_id 隔离）
	d, err := qs.GetDraft(other, draftProblemID, "python", "", 0)
	if err != nil {
		t.Fatalf("GetDraft 另一用户: %v", err)
	}
	if d.Code != "" || !d.UpdatedAt.IsZero() {
		t.Fatalf("另一用户 %+v = %+v, want 零值", slot{"python", "", 0}, d)
	}
}
