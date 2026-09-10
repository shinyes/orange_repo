package store

import (
	"testing"

	"orangeoj/internal/model"
)

// 域内模板列表（仓库页/管理端题册栏按域取数）必须带上题目数：
// 题册栏每行显示「N 题」直接读 problemCount，缺列时恒为 0（用户可见缺陷）。
func TestListInDomainIncludesProblemCount(t *testing.T) {
	s := newTestStore(t)

	domainA, err := s.CreateDomain("域A")
	if err != nil {
		t.Fatal(err)
	}
	domainB, err := s.CreateDomain("域B")
	if err != nil {
		t.Fatal(err)
	}
	mkProblem := func(title string, domainID int64) int64 {
		t.Helper()
		pid, err := s.CreateProblem(model.Problem{DomainID: &domainID, Type: model.TypeProgramming, Title: title,
			BodyJSON: []byte(`{}`), AnswerJSON: []byte(`{}`), Solutions: []byte(`[]`),
			TimeLimitMS: 1000, MemoryLimitMiB: 256})
		if err != nil {
			t.Fatal(err)
		}
		return pid
	}
	pA1 := mkProblem("A1", domainA)
	pA2 := mkProblem("A2", domainA)
	pA3 := mkProblem("A3", domainA)
	pB1 := mkProblem("B1", domainB)

	mkTraining := func(title string, pids []int64) int64 {
		t.Helper()
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
	mkPractice := func(title string, pids []int64) int64 {
		t.Helper()
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

	mkTraining("域A训练", []int64{pA1, pA2})
	mkTraining("混合训练", []int64{pA1, pB1})
	mkTraining("空训练", nil)
	mkPractice("域A练习", []int64{pA1, pA2, pA3})
	mkPractice("空练习", nil)

	// 域内训练列表：problemCount 必须等于题册内条数（全部条目，非仅本域条目）。
	trainings, err := s.ListTrainingsInDomain(domainA)
	if err != nil {
		t.Fatal(err)
	}
	gotCount := map[string]int{}
	for _, tr := range trainings {
		gotCount[tr.Title] = tr.ProblemCount
	}
	if gotCount["域A训练"] != 2 {
		t.Errorf("域A训练 problemCount = %d, want 2", gotCount["域A训练"])
	}
	if gotCount["混合训练"] != 2 {
		t.Errorf("混合训练 problemCount = %d, want 2（与题册内条数一致）", gotCount["混合训练"])
	}
	if _, ok := gotCount["空训练"]; ok {
		t.Errorf("空训练无本域题目，不应出现在域 A 列表中")
	}

	// 域内练习列表：同上。
	practices, err := s.ListPracticesInDomain(domainA)
	if err != nil {
		t.Fatal(err)
	}
	gotPracticeCount := map[string]int{}
	for _, p := range practices {
		gotPracticeCount[p.Title] = p.ProblemCount
	}
	if gotPracticeCount["域A练习"] != 3 {
		t.Errorf("域A练习 problemCount = %d, want 3", gotPracticeCount["域A练习"])
	}
	if _, ok := gotPracticeCount["空练习"]; ok {
		t.Errorf("空练习无本域题目，不应出现在域 A 列表中")
	}

	// 全量列表（未选域）：空题册仍显示 0，不得因修复而变成缺省值异常。
	all, err := s.ListTrainings()
	if err != nil {
		t.Fatal(err)
	}
	allCount := map[string]int{}
	for _, tr := range all {
		allCount[tr.Title] = tr.ProblemCount
	}
	for _, title := range []string{"域A训练", "混合训练", "空训练"} {
		if _, ok := allCount[title]; !ok {
			t.Fatalf("全量训练列表缺少 %q", title)
		}
	}
	if allCount["空训练"] != 0 {
		t.Errorf("空训练全量 problemCount = %d, want 0", allCount["空训练"])
	}
}
