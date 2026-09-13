// 练习交卷记录「管理员可看全部成员」数据层回归（internal/quizstore/space_progress.go）：
//
//	ListPracticeSubmissionsAllUsers —— 跨用户聚合、按 id 倒序、上限 200、每行带 userName
//	PracticeSubmissionBelongsTo     —— 「记录是否属于该练习」（管理端读他人答卷的归属校验）
//	GetPracticeSubmissionMeta       —— created_at 两种存量格式统一解析为同一 UTC 时刻；
//	                                   记录不存在 → ErrNotFound
//
// 空间/练习结构在主库（本层不建）：space_practice_submissions.practice_id 是无外键的裸 id
// （见 quizstore.go 建表注释），故这里只建真实 users 行（user_id 外键级联）。
package quizstore_test

import (
	"errors"
	"testing"
	"time"

	"orangeoj/internal/quizstore"
)

// practiceTimeLayout 存量格式之一：SQLite CURRENT_TIMESTAMP（UTC 裸格式，无时区后缀）。
const practiceTimeLayout = "2006-01-02 15:04:05"

// psSaveSubmission 直写一条交卷记录（快照为空卷），返回提交 id（自增 = 写入顺序）。
func psSaveSubmission(t *testing.T, qs *quizstore.Store, practiceID, userID int64) int64 {
	t.Helper()
	id, err := qs.SavePracticeSubmission(practiceID, userID, "[]", 0)
	if err != nil {
		t.Fatalf("SavePracticeSubmission(practice=%d user=%d): %v", practiceID, userID, err)
	}
	return id
}

// psListAll 取该练习「全部成员」交卷记录。
func psListAll(t *testing.T, qs *quizstore.Store, practiceID int64) []quizstore.PracticeSubmission {
	t.Helper()
	subs, err := qs.ListPracticeSubmissionsAllUsers(practiceID)
	if err != nil {
		t.Fatalf("ListPracticeSubmissionsAllUsers(%d): %v", practiceID, err)
	}
	return subs
}

// psListSelf 取某用户的本人交卷记录。
func psListSelf(t *testing.T, qs *quizstore.Store, practiceID, userID int64) []quizstore.PracticeSubmission {
	t.Helper()
	subs, err := qs.ListPracticeSubmissions(practiceID, userID)
	if err != nil {
		t.Fatalf("ListPracticeSubmissions(%d,%d): %v", practiceID, userID, err)
	}
	return subs
}

// psOnDiskCreatedAt 取 SQLite 侧实际存储的字面量：CAST(created_at AS TEXT) 的结果列无 DATETIME
// 声明，故不经驱动的「DATETIME → time.Time → RFC3339 字符串」归一化（普通 SELECT 走该归一化）。
func psOnDiskCreatedAt(t *testing.T, qs *quizstore.Store, sid int64) string {
	t.Helper()
	var raw string
	if err := qs.DB.QueryRow(`SELECT CAST(created_at AS TEXT) FROM space_practice_submissions WHERE id=?`, sid).Scan(&raw); err != nil {
		t.Fatalf("读库内 created_at(id=%d): %v", sid, err)
	}
	return raw
}

// psRewriteCreatedAt 覆盖 created_at（模拟两种存量格式的老库行）。
func psRewriteCreatedAt(t *testing.T, qs *quizstore.Store, sid int64, raw string) {
	t.Helper()
	if _, err := qs.DB.Exec(`UPDATE space_practice_submissions SET created_at=? WHERE id=?`, raw, sid); err != nil {
		t.Fatalf("改写 created_at(id=%d, %q): %v", sid, raw, err)
	}
}

// TestListPracticeSubmissionsAllUsersAcrossUsers 三名用户各自/重复交卷：
// 「全部成员」视图返回该练习全部记录（id 倒序），逐行 userId/userName 与提交者一致，
// 不混入其它练习的记录；对照本人视图只返回自己那几条且不回带 userName。
func TestListPracticeSubmissionsAllUsersAcrossUsers(t *testing.T) {
	qs := newTestEnvironment(t)
	const practiceID, otherPracticeID = int64(101), int64(102)
	uidA := newGameStudent(t, qs, "psAllA")
	uidB := newGameStudent(t, qs, "psAllB")
	uidC := newGameStudent(t, qs, "psAllC")

	idA1 := psSaveSubmission(t, qs, practiceID, uidA)
	idB := psSaveSubmission(t, qs, practiceID, uidB)
	idC := psSaveSubmission(t, qs, practiceID, uidC)
	idA2 := psSaveSubmission(t, qs, practiceID, uidA) // 同一用户二次交卷
	psSaveSubmission(t, qs, otherPracticeID, uidA)    // 干扰项：别的练习的记录
	if !(idA1 < idB && idB < idC && idC < idA2) {
		t.Fatalf("夹具异常：提交 id 未按写入顺序自增（%d,%d,%d,%d）", idA1, idB, idC, idA2)
	}

	// ---- 全部成员视图 ----
	subs := psListAll(t, qs, practiceID)
	if len(subs) != 4 {
		t.Fatalf("全部成员记录数 = %d（%+v），want 4（只含本练习，不得混入其它练习）", len(subs), subs)
	}
	want := []struct {
		id   int64
		uid  int64
		name string
	}{
		{idA2, uidA, "psAllA"}, // 倒序：最新在前
		{idC, uidC, "psAllC"},
		{idB, uidB, "psAllB"},
		{idA1, uidA, "psAllA"},
	}
	for i, w := range want {
		got := subs[i]
		if got.ID != w.id || got.UserID != w.uid || got.UserName != w.name {
			t.Fatalf("全部成员视图第 %d 行 = {id:%d user:%d name:%q}，want {id:%d user:%d name:%q}（须 id 倒序且带对应用户名）",
				i, got.ID, got.UserID, got.UserName, w.id, w.uid, w.name)
		}
		if got.PracticeID != practiceID {
			t.Fatalf("全部成员视图第 %d 行 practiceId = %d，want %d", i, got.PracticeID, practiceID)
		}
		if got.CreatedAt.IsZero() {
			t.Fatalf("全部成员视图第 %d 行 createdAt 为零值（时间解析失败）: %+v", i, got)
		}
	}

	// ---- 对照：本人视图只返回自己，且不回带 userName ----
	mineA := psListSelf(t, qs, practiceID, uidA)
	if len(mineA) != 2 || mineA[0].ID != idA2 || mineA[1].ID != idA1 {
		t.Fatalf("用户A 本人记录 = %+v，want [%d %d]（倒序）", mineA, idA2, idA1)
	}
	for i, sub := range mineA {
		if sub.UserID != uidA {
			t.Fatalf("用户A 本人记录第 %d 行 userId = %d，want %d", i, sub.UserID, uidA)
		}
		if sub.UserName != "" {
			t.Fatalf("本人视图第 %d 行 userName = %q，want 空（成员视图不回带提交者名）", i, sub.UserName)
		}
	}
	mineB := psListSelf(t, qs, practiceID, uidB)
	if len(mineB) != 1 || mineB[0].ID != idB || mineB[0].UserID != uidB {
		t.Fatalf("用户B 本人记录 = %+v，want 仅 {id:%d user:%d}", mineB, idB, uidB)
	}
}

// TestListPracticeSubmissionsAllUsersCapsAt200 上限 200：
// 写 205 条 → 返回最新 200 条（id 倒序），最旧的 5 条被截断（漏 LIMIT 会把大练习整表拖回）。
func TestListPracticeSubmissionsAllUsersCapsAt200(t *testing.T) {
	qs := newTestEnvironment(t)
	uid := newGameStudent(t, qs, "psCap")
	const practiceID = int64(103)
	const total, limit = 205, 200

	ids := make([]int64, 0, total)
	for i := 0; i < total; i++ {
		ids = append(ids, psSaveSubmission(t, qs, practiceID, uid))
	}
	subs := psListAll(t, qs, practiceID)
	if len(subs) != limit {
		t.Fatalf("记录数 = %d，want %d（上限）", len(subs), limit)
	}
	for i := 0; i < limit; i++ {
		if wantID := ids[total-1-i]; subs[i].ID != wantID {
			t.Fatalf("第 %d 行 id = %d，want %d（倒序取最新 %d 条，最旧 5 条应被截断）", i, subs[i].ID, wantID, limit)
		}
	}
	if subs[0].UserName != "psCap" {
		t.Fatalf("第 0 行 userName = %q，want psCap", subs[0].UserName)
	}
}

// TestListPracticeSubmissionsAllUsersEmptyAndBelongsTo 空练习返回空切片（非 nil）；
// PracticeSubmissionBelongsTo 只在「记录属于该练习」时为 true。
func TestListPracticeSubmissionsAllUsersEmptyAndBelongsTo(t *testing.T) {
	qs := newTestEnvironment(t)
	const practiceID, otherPracticeID = int64(201), int64(202)
	uid := newGameStudent(t, qs, "psEmpty")

	empty := psListAll(t, qs, practiceID)
	if empty == nil {
		t.Fatal("无记录练习返回 nil 切片，want 空切片（nil 序列化为 JSON null，前端 map/filter 会崩）")
	}
	if len(empty) != 0 {
		t.Fatalf("无记录练习 = %+v，want 空切片", empty)
	}
	if self := psListSelf(t, qs, practiceID, uid); self == nil || len(self) != 0 {
		t.Fatalf("本人视图无记录 = %+v（nil=%v），want 空切片", self, self == nil)
	}

	sid := psSaveSubmission(t, qs, practiceID, uid)
	for _, tc := range []struct {
		name     string
		sid, pid int64
		want     bool
	}{
		{"本练习的记录", sid, practiceID, true},
		{"别的练习 id", sid, otherPracticeID, false},
		{"不存在的记录 id", 999999, practiceID, false},
	} {
		got, err := qs.PracticeSubmissionBelongsTo(tc.sid, tc.pid)
		if err != nil {
			t.Fatalf("%s: PracticeSubmissionBelongsTo(%d,%d): %v", tc.name, tc.sid, tc.pid, err)
		}
		if got != tc.want {
			t.Fatalf("PracticeSubmissionBelongsTo(sid=%d,pid=%d) [%s] = %v，want %v",
				tc.sid, tc.pid, tc.name, got, tc.want)
		}
	}
}

// TestGetPracticeSubmissionMetaTimeFormats 交卷时间：必须等于该记录的 created_at
// （库内实际存储的是 SQLite CURRENT_TIMESTAMP 裸 UTC 格式，见 psOnDiskCreatedAt），
// 且落在当前时间窗；库内两种存量格式（裸 UTC / 早期 ISO Z）与带偏移的 RFC3339
// 均解析为同一时刻；不存在 → ErrNotFound。
func TestGetPracticeSubmissionMetaTimeFormats(t *testing.T) {
	qs := newTestEnvironment(t)
	uid := newGameStudent(t, qs, "psMeta")
	const practiceID = int64(301)
	sid := psSaveSubmission(t, qs, practiceID, uid)

	// ---- 新写入行：库内格式必须是 SQLite CURRENT_TIMESTAMP（本用例覆盖的第一种存量格式）----
	onDisk := psOnDiskCreatedAt(t, qs, sid)
	written, err := time.ParseInLocation(practiceTimeLayout, onDisk, time.UTC)
	if err != nil {
		t.Fatalf("新写入行库内 created_at = %q，want SQLite CURRENT_TIMESTAMP 格式 %q: %v", onDisk, practiceTimeLayout, err)
	}
	got, err := qs.GetPracticeSubmissionMeta(sid)
	if err != nil {
		t.Fatalf("GetPracticeSubmissionMeta(%d): %v", sid, err)
	}
	if got.IsZero() {
		t.Fatalf("库内 created_at = %q 解析为零值（解析失败会下发出 0001-01-01）", onDisk)
	}
	if !got.Equal(written) {
		t.Fatalf("meta = %v，want 库内 created_at = %v（raw=%q）", got, written, onDisk)
	}
	if got.Location() != time.UTC {
		t.Fatalf("meta 时区 = %v，want UTC（裸格式误按本地时区解析会与判题时间轴差整数小时）", got.Location())
	}
	if delta := time.Since(got); delta < -10*time.Minute || delta > 10*time.Minute {
		t.Fatalf("meta = %v 距当前 %v（raw=%q），want ≤10 分钟", got, delta, onDisk)
	}
	// 实测：普通 SELECT（列声明 DATETIME）经驱动读回时已归一化为 RFC3339 字符串，故线上生效的
	// 主要是 parsePracticeTime 的 RFC3339 分支；裸格式分支是存量/异常值（驱动原样透传）兜底。
	// 此处只钉住对外契约：meta 与库内 created_at 同一时刻且为 UTC。

	// ---- 存量格式（含早期 ISO）与带偏移的 RFC3339 → 同一时刻 ----
	wantUTC := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	for _, tc := range []struct {
		name string
		raw  string
		// wantUTCLoc：无时区后缀（裸格式）与带 Z 的格式须解析为 UTC 时区；
		// 带偏移的 RFC3339 保留自身偏移（只要求时刻相同）。
		wantUTCLoc bool
	}{
		{"库内 SQLite CURRENT_TIMESTAMP（裸 UTC）", "2026-01-02 03:04:05", true},
		{"库内早期 ISO 带 Z", "2026-01-02T03:04:05Z", true},
		{"库内 RFC3339 带 +08:00 偏移（同一时刻）", "2026-01-02T11:04:05+08:00", false},
	} {
		psRewriteCreatedAt(t, qs, sid, tc.raw)
		got, err := qs.GetPracticeSubmissionMeta(sid)
		if err != nil {
			t.Fatalf("%s: GetPracticeSubmissionMeta: %v", tc.name, err)
		}
		if !got.Equal(wantUTC) {
			t.Fatalf("%s: 库内 created_at = %q → meta = %v，want %v（须为同一时刻）", tc.name, tc.raw, got, wantUTC)
		}
		if tc.wantUTCLoc && got.Location() != time.UTC {
			t.Fatalf("%s: 库内 created_at = %q → meta 时区 = %v，want UTC（按本地时区解析会差整数小时）",
				tc.name, tc.raw, got.Location())
		}
	}

	// 不可解析的存量值 → 零值（不报错；下发出 0001-01-01T00:00:00Z 由调用方兜底）
	psRewriteCreatedAt(t, qs, sid, "garbage")
	if got, err := qs.GetPracticeSubmissionMeta(sid); err != nil || !got.IsZero() {
		t.Fatalf("不可解析存量值 meta = %v err = %v，want 零值且不报错", got, err)
	}

	// 不存在的记录 → ErrNotFound
	if _, err := qs.GetPracticeSubmissionMeta(999999); !errors.Is(err, quizstore.ErrNotFound) {
		t.Fatalf("不存在记录 err = %v，want quizstore.ErrNotFound", err)
	}
}
