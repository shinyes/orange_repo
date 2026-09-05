// 空间训练/练习学生作答数据层测试（quiz.db）。
// 结构表在主库（本测试不建空间/训练/练习）；仅验证作答记录、限次幂等、
// 通过记录去重与交卷快照。user_id 引用 accounts 建的真实用户（FK 级联）。
package quizstore_test

import (
	"testing"

	"orangerepo/internal/accounts"
	"orangerepo/internal/quizstore"
)

// newProgressStudent 建一名学生，返回其 user_id。
func newProgressStudent(t *testing.T, qs *quizstore.Store) int64 {
	t.Helper()
	uid, err := qs.Accounts.CreateUser("prog", "pw", accounts.RoleMember)
	if err != nil {
		t.Fatal(err)
	}
	return uid
}

// TestRecordTrainingAttempt_限次幂等与通过去重
// 记录尝试逐次 +1；答对写 solved 与通过记录；已 solved 后再调幂等不再 +1，
// 通过记录按 uuid 去重不重复插入。
func TestRecordTrainingAttemptIdempotent(t *testing.T) {
	qs := newTestEnvironment(t)
	uid := newProgressStudent(t, qs)

	const trainingID, problemID = int64(5), int64(101)
	const uuidA = "uuid-attempt-a"

	// 两次答错 → attempts 递增、solved=false
	for i := 1; i <= 2; i++ {
		attempts, solved, err := qs.RecordTrainingAttempt(trainingID, uid, problemID, uuidA, false)
		if err != nil {
			t.Fatal(err)
		}
		if attempts != i || solved {
			t.Fatalf("第 %d 次答错: attempts=%d solved=%v, want attempts=%d solved=false", i, attempts, solved, i)
		}
	}
	// 第三次答对 → solved=true（attempts 继续 +1）
	attempts, solved, err := qs.RecordTrainingAttempt(trainingID, uid, problemID, uuidA, true)
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 3 || !solved {
		t.Fatalf("答对后 attempts=%d solved=%v, want 3/true", attempts, solved)
	}
	// 已 solved 后再调：幂等返回现状，不重复计数
	attempts, solved, err = qs.RecordTrainingAttempt(trainingID, uid, problemID, uuidA, true)
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 3 || !solved {
		t.Fatalf("已 solved 后再答: attempts=%d solved=%v, want 3/true（幂等）", attempts, solved)
	}
	// GetTrainingAttempt 读数一致
	st, err := qs.GetTrainingAttempt(trainingID, uid, problemID)
	if err != nil || st.Attempts != 3 || !st.Solved {
		t.Fatalf("state = %+v %v, want attempts=3 solved=true", st, err)
	}
	// 通过记录恰 1 条且为 uuidA
	uu, err := qs.SolvedUUIDs(uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(uu) != 1 || !uu[uuidA] {
		t.Fatalf("solved = %v, want 仅 %s", uu, uuidA)
	}
	// 未作答的题：GetTrainingAttempt 返回 0 次不报错
	st0, err := qs.GetTrainingAttempt(trainingID, uid, 999)
	if err != nil || st0.Attempts != 0 || st0.Solved {
		t.Fatalf("无记录 state = %+v %v, want 0/false", st0, err)
	}
}

// TestSavePracticeSubmission_交卷写库与通过去重
// 答对且带 uuid 的题写通过记录；无 uuid 或答错不写；提交记录可回读。
func TestSavePracticeSubmissionWritesSolved(t *testing.T) {
	qs := newTestEnvironment(t)
	uid := newProgressStudent(t, qs)

	const practiceID = int64(9)
	answers := `[{"problemId":101,"correct":true,"uuid":"uuid-p-101"},` +
		`{"problemId":102,"correct":false,"uuid":"uuid-p-102"},` +
		`{"problemId":103,"correct":true}]` // 无 uuid → 不写通过

	subID, err := qs.SavePracticeSubmission(practiceID, uid, answers, 1)
	if err != nil {
		t.Fatal(err)
	}
	if subID == 0 {
		t.Fatal("submission id = 0")
	}
	// 提交记录可回读（快照一致）
	raw, err := qs.GetPracticeSubmission(subID)
	if err != nil || raw != answers {
		t.Fatalf("snapshot = %q %v", raw, err)
	}
	// 列表恰 1 条
	subs, err := qs.ListPracticeSubmissions(practiceID, uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 1 || subs[0].ID != subID || subs[0].PracticeID != practiceID ||
		subs[0].UserID != uid || subs[0].ObjectiveCorrect != 1 || subs[0].CreatedAt == "" {
		t.Fatalf("subs = %+v", subs)
	}
	// 通过记录仅答对且带 uuid 的那题
	uu, err := qs.SolvedUUIDs(uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(uu) != 1 || !uu["uuid-p-101"] {
		t.Fatalf("solved = %v, want 仅 uuid-p-101", uu)
	}
	// 再次交卷（答对同题同 uuid）：通过记录去重仍 1 条；提交则新增
	if _, err := qs.SavePracticeSubmission(practiceID, uid, answers, 1); err != nil {
		t.Fatal(err)
	}
	uu, _ = qs.SolvedUUIDs(uid)
	if len(uu) != 1 {
		t.Fatalf("重复交卷后 solved = %v, want 仍 1 条（去重）", uu)
	}
	subs, _ = qs.ListPracticeSubmissions(practiceID, uid)
	if len(subs) != 2 {
		t.Fatalf("重复交卷后提交数 = %d, want 2", len(subs))
	}
}

// TestRecordSolvedDirect 直接按 uuid 记通过（去重；空 uuid 忽略）。
func TestRecordSolvedDirect(t *testing.T) {
	qs := newTestEnvironment(t)
	uid := newProgressStudent(t, qs)

	if err := qs.RecordSolved(uid, "uuid-x"); err != nil {
		t.Fatal(err)
	}
	if err := qs.RecordSolved(uid, "uuid-x"); err != nil {
		t.Fatal(err)
	}
	if err := qs.RecordSolved(uid, ""); err != nil {
		t.Fatal(err)
	}
	uu, err := qs.SolvedUUIDs(uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(uu) != 1 || !uu["uuid-x"] {
		t.Fatalf("solved = %v, want 仅 uuid-x", uu)
	}
}
