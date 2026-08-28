package store

import (
	"testing"

	"orangerepo/internal/model"
)

// 构造：题目集合 + 训练/练习引用关系，删除题册时应级联删除"仅被该题册引用"的题目。
func cascadeFixture(t *testing.T) (*Store, map[string]int64, map[string]int64) {
	t.Helper()
	s := newTestStore(t)
	mk := func(title string) int64 {
		pid, err := s.CreateProblem(model.Problem{Type: model.TypeProgramming, Title: title,
			BodyJSON: []byte(`{}`), AnswerJSON: []byte(`{}`), Solutions: []byte(`[]`),
			TimeLimitMS: 1000, MemoryLimitMiB: 256})
		if err != nil {
			t.Fatal(err)
		}
		return pid
	}
	mkT := func(title string, pids []int64) int64 {
		tid, err := s.CreateTraining(title, "", nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		cid, err := s.CreateChapter(tid, "章")
		if err != nil {
			t.Fatal(err)
		}
		if len(pids) > 0 {
			if _, err := s.AddChapterItems(cid, pids); err != nil {
				t.Fatal(err)
			}
		}
		return tid
	}
	mkP := func(title string, pids []int64) int64 {
		pid, err := s.CreatePractice(title, "", nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(pids) > 0 {
			if _, err := s.AddPracticeItems(pid, pids); err != nil {
				t.Fatal(err)
			}
		}
		return pid
	}
	problems := map[string]int64{
		"P1": mk("P1"), "P2": mk("P2"), "P3": mk("P3"), "P4": mk("P4"), "P5": mk("P5"),
	}
	trainings := map[string]int64{
		"A": mkT("训练A", []int64{problems["P1"], problems["P3"], problems["P5"]}),
		"B": mkT("训练B", []int64{problems["P2"], problems["P3"]}),
	}
	// 练习 X 含 P4、P5（P5 与训练 A 共有）
	practices := map[string]int64{"X": mkP("练习X", []int64{problems["P4"], problems["P5"]})}
	return s, problems, mergeMaps(trainings, practices)
}

func mergeMaps[K comparable, V any](a, b map[K]V) map[K]V {
	out := make(map[K]V, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

func existCheck(t *testing.T, s *Store, pids map[string]int64, want map[string]bool) {
	t.Helper()
	list, err := s.ListProblems(ProblemFilter{})
	if err != nil {
		t.Fatal(err)
	}
	alive := map[int64]bool{}
	for _, p := range list {
		alive[p.ID] = true
	}
	for name, id := range pids {
		if alive[id] != want[name] {
			t.Errorf("题目 %s 存活=%v, want %v", name, alive[id], want[name])
		}
	}
}

func TestDeleteTrainingCascadesProblems(t *testing.T) {
	s, pids, groups := cascadeFixture(t)

	// 删除训练 A：P1 仅 A 引用 → 删除；P3 还在 B → 保留；P5 还在练习 X → 保留
	if err := s.DeleteTraining(groups["A"]); err != nil {
		t.Fatal(err)
	}
	existCheck(t, s, pids, map[string]bool{
		"P1": false, "P2": true, "P3": true, "P4": true, "P5": true,
	})
	// 训练 A 本身消失
	if _, err := s.GetTraining(groups["A"]); err == nil {
		t.Error("训练 A 应已删除")
	}
}

func TestDeletePracticeCascadesProblems(t *testing.T) {
	s, pids, groups := cascadeFixture(t)

	// 删除练习 X：P4 仅 X 引用 → 删除；P5 还在训练 A → 保留
	if err := s.DeletePractice(groups["X"]); err != nil {
		t.Fatal(err)
	}
	existCheck(t, s, pids, map[string]bool{
		"P1": true, "P2": true, "P3": true, "P4": false, "P5": true,
	})
}

// 交叉引用耗尽：先删 A，P5 只剩 X；再删 X，P5 无引用 → 最终删除
func TestCascadeWhenLastReferenceRemoved(t *testing.T) {
	s, pids, groups := cascadeFixture(t)

	if err := s.DeleteTraining(groups["A"]); err != nil {
		t.Fatal(err)
	}
	if err := s.DeletePractice(groups["X"]); err != nil {
		t.Fatal(err)
	}
	existCheck(t, s, pids, map[string]bool{
		"P1": false, "P2": true, "P3": true, "P4": false, "P5": false,
	})
}

// 目录「连同题册删除」同样应用级联规则
func TestDeleteDirectoryCascadesProblems(t *testing.T) {
	s, pids, groups := cascadeFixture(t)
	// 把训练 B 和练习 X 移入一个新目录
	dir, err := s.CreateBookletDirectory("待删目录", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetTrainingFolder(groups["B"], &dir); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPracticeFolder(groups["X"], &dir); err != nil {
		t.Fatal(err)
	}

	// 连同题册删除：B 的 P2 删、P3 保留（A 还在）；X 的 P4 删、P5 保留（A 还在）
	if err := s.DeleteBookletDirectory(dir, true); err != nil {
		t.Fatal(err)
	}
	existCheck(t, s, pids, map[string]bool{
		"P1": true, "P2": false, "P3": true, "P4": false, "P5": true,
	})
}

// 不删题册：题册与题目都保留
func TestDeleteDirectoryWithoutBookletsKeepsProblems(t *testing.T) {
	s, pids, groups := cascadeFixture(t)
	dir, err := s.CreateBookletDirectory("保留目录", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetTrainingFolder(groups["B"], &dir); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteBookletDirectory(dir, false); err != nil {
		t.Fatal(err)
	}
	existCheck(t, s, pids, map[string]bool{
		"P1": true, "P2": true, "P3": true, "P4": true, "P5": true,
	})
}