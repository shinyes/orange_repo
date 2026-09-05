package store

import (
	"strconv"
	"testing"

	"orangerepo/internal/model"
)

// setupSpaceContentEnv 建域/空间并造 3 道题（含 uuid），返回 spaceID 与题目。
func setupSpaceContentEnv(t *testing.T) (*Store, int64, []int64) {
	t.Helper()
	s := newTestStore(t)
	domainID, err := s.CreateDomain("域A")
	if err != nil {
		t.Fatal(err)
	}
	spaceID, err := s.CreateSpace(domainID, "空间A")
	if err != nil {
		t.Fatal(err)
	}
	var pids []int64
	for _, spec := range []struct {
		typ  string
		body string
	}{
		{"single_choice", `{"options":["a","b"],"answerIndex":0}`},
		{"true_false", `{"answer":true}`},
		{"single_choice", `{"options":["x","y"],"answerIndex":1}`},
	} {
		id, err := s.CreateProblem(model.Problem{Type: model.ProblemType(spec.typ), Title: "题", BodyJSON: jsonRaw(spec.body)})
		if err != nil {
			t.Fatal(err)
		}
		pids = append(pids, id)
	}
	return s, spaceID, pids
}

func jsonRaw(v string) []byte { return []byte(v) }

// TestSpaceTrainingFlow 空间训练：建/章节/条目/元信息/列表。
func TestSpaceTrainingFlow(t *testing.T) {
	s, spaceID, pids := setupSpaceContentEnv(t)

	trID, err := s.CreateSpaceTraining(spaceID, "第一训练", "描述", []string{"t1"}, 3)
	if err != nil {
		t.Fatal(err)
	}
	chID, err := s.CreateSpaceChapter(trID, "章一")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddSpaceChapterItems(chID, pids); err != nil {
		t.Fatal(err)
	}
	tr, chapters, err := s.GetSpaceTraining(trID)
	if err != nil {
		t.Fatal(err)
	}
	if tr.Title != "第一训练" || tr.MaxAttempts != 3 || tr.ProblemCount != 3 {
		t.Fatalf("training = %+v", tr)
	}
	if len(chapters) != 1 || len(chapters[0].Items) != 3 {
		t.Fatalf("chapters = %+v", chapters)
	}
	if chapters[0].Items[0].ProblemUUID == "" {
		t.Fatal("条目应带题目 uuid")
	}
	list, err := s.ListSpaceTrainings(spaceID)
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %v %v", list, err)
	}
}

// TestSpaceTrainingAttempts 限次与标色：答错 3 次达上限锁定；答对 solved。
func TestSpaceTrainingAttempts(t *testing.T) {
	s, spaceID, pids := setupSpaceContentEnv(t)
	trID, err := s.CreateSpaceTraining(spaceID, "限次训练", "", nil, 3)
	if err != nil {
		t.Fatal(err)
	}

	// 连续答错 3 次 → 达上限
	for i := 0; i < 3; i++ {
		a, solved, max, err := s.RecordSpaceTrainingAttempt(trID, 1, pids[0], false)
		if err != nil {
			t.Fatal(err)
		}
		if max != 3 || solved {
			t.Fatalf("attempt %d: a=%d solved=%v max=%d", i, a, solved, max)
		}
	}
	// 第 4 次（达上限后）幂等拒绝
	a, _, _, err := s.RecordSpaceTrainingAttempt(trID, 1, pids[0], false)
	if err != nil {
		t.Fatal(err)
	}
	if a != 3 {
		t.Fatalf("超过上限后 attempts=%d, want 3", a)
	}
	st, err := s.GetSpaceTrainingAttempt(trID, 1, pids[0])
	if err != nil || st.Attempts != 3 || st.Solved {
		t.Fatalf("state = %+v %v", st, err)
	}

	// 另一题一次答对 → solved
	_, solved, _, err := s.RecordSpaceTrainingAttempt(trID, 1, pids[1], true)
	if err != nil || !solved {
		t.Fatalf("correct attempt solved=%v err=%v", solved, err)
	}
	// 通过记录（uuid 去重）已写入
	solvedUUIDs, err := s.SolvedUUIDs(1)
	if err != nil || len(solvedUUIDs) != 1 {
		t.Fatalf("solved = %v %v", solvedUUIDs, err)
	}
	// 重复答对不再新增
	_, _, _, err = s.RecordSpaceTrainingAttempt(trID, 1, pids[1], true)
	if err != nil {
		t.Fatal(err)
	}
	// solved 后幂等不再 +1（上层禁选）——此处已 solved 直接返回原态
	st2, _ := s.GetSpaceTrainingAttempt(trID, 1, pids[1])
	if st2.Attempts != 1 {
		t.Fatalf("solved 后 attempts=%d want 1", st2.Attempts)
	}
}

// TestSpacePracticeSubmission 练习交卷：保存记录、答对写通过（去重）。
func TestSpacePracticeSubmission(t *testing.T) {
	s, spaceID, pids := setupSpaceContentEnv(t)
	prID, err := s.CreateSpacePractice(spaceID, "模拟考", "整卷", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddSpacePracticeItems(prID, pids); err != nil {
		t.Fatal(err)
	}
	// 拿题目 uuid
	var u0 string
	if err := s.DB.QueryRow(`SELECT uuid FROM problems WHERE id=?`, pids[0]).Scan(&u0); err != nil {
		t.Fatal(err)
	}
	answers := `[{"problemId":` + strconv.FormatInt(pids[0], 10) + `,"correct":true,"uuid":"` + u0 + `"},{"problemId":` + strconv.FormatInt(pids[1], 10) + `,"correct":false}]`
	subID, err := s.SaveSpacePracticeSubmission(prID, 7, answers, 1)
	if err != nil {
		t.Fatal(err)
	}
	if subID == 0 {
		t.Fatal("submission id = 0")
	}
	subs, err := s.ListSpacePracticeSubmissions(prID, 7)
	if err != nil || len(subs) != 1 || subs[0].ObjectiveCorrect != 1 {
		t.Fatalf("subs = %+v %v", subs, err)
	}
	// 通过记录 1 条
	uu, err := s.SolvedUUIDs(7)
	if err != nil || len(uu) != 1 {
		t.Fatalf("solved = %v %v", uu, err)
	}
	// 快照可取回
	raw, err := s.GetSpacePracticeSubmission(subID)
	if err != nil || raw == "" {
		t.Fatalf("snapshot = %q %v", raw, err)
	}
}
