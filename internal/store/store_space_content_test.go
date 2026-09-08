package store

import (
	"testing"

	"orangeoj/internal/model"
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

	trID, err := s.CreateSpaceTraining(spaceID, "第一训练", "描述", []string{"t1"}, 3, false)
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
