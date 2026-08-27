package store

import (
	"math/rand"
	"testing"

	"orangerepo/internal/model"
)

// 标签计数的暴力一致性校验：每个候选的 count 应等于「基底过滤 + 仅该标签」的题目列表长度；
// total 应等于完整过滤（含选中集）的题目列表长度。计数不得受选中集影响。
func TestTagCountsMatchList(t *testing.T) {
	s := newTestStore(t)
	rng := rand.New(rand.NewSource(42))
	allTags := []string{"数学", "数学/几何", "数学/几何/圆", "数学/代数", "物理", "算法", "入门/基础", "入门"}
	for i := 0; i < 6; i++ {
		n := rng.Intn(3)
		tags := make([]string, 0, n)
		for j := 0; j < n; j++ {
			tags = append(tags, allTags[rng.Intn(len(allTags))])
		}
		typ := model.ProblemType("programming")
		if i%2 == 0 {
			typ = model.TypeSingleChoice
		}
		if _, err := s.CreateProblem(model.Problem{Type: typ, Title: "T", Tags: tags,
			BodyJSON: []byte(`{}`), AnswerJSON: []byte(`{}`), Solutions: []byte(`[]`),
			TimeLimitMS: 1000, MemoryLimitMiB: 256}); err != nil {
			t.Fatal(err)
		}
	}

	type filterCase struct {
		name string
		f    ProblemFilter
	}
	cases := []filterCase{
		{"empty", ProblemFilter{}},
		{"q", ProblemFilter{Q: "T"}},
		{"type", ProblemFilter{Type: "programming"}},
		{"sel-a", ProblemFilter{Tags: []string{"数学"}}},
		{"sel-b", ProblemFilter{Tags: []string{"数学/几何"}}},
		{"sel-c", ProblemFilter{Tags: []string{"数学", "算法"}}},
		{"sel-d", ProblemFilter{Tags: []string{"入门/基础"}}},
		{"sel-e", ProblemFilter{Tags: []string{NoneTag}}},
		{"multi", ProblemFilter{Tags: []string{"数学", "数学/几何"}}},
		{"all", ProblemFilter{Q: "T", Tags: []string{"数学"}, Type: "single_choice"}},
	}
	for _, tc := range cases {
		tags, total, err := s.ListTagFacets(tc.f)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		list, err := s.ListProblems(tc.f)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if total != len(list) {
			t.Errorf("%s: total=%d, ListProblems=%d", tc.name, total, len(list))
		}
		base := tc.f
		base.Tags = nil
		for _, tg := range tags {
			single := base
			single.Tags = []string{tg.Tag}
			l2, err := s.ListProblems(single)
			if err != nil {
				t.Fatal(err)
			}
			if tg.Count != len(l2) {
				t.Errorf("%s: count[%s]=%d, ListProblems(tags=[%s])=%d", tc.name, tg.Tag, tg.Count, tg.Tag, len(l2))
			}
		}
	}
}