package store

import (
	"testing"

	"orangerepo/internal/model"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func mustAddProblem(t *testing.T, s *Store, title string, tags []string) {
	t.Helper()
	payload := model.Problem{Type: model.TypeProgramming, Title: title, Tags: tags,
		BodyJSON: []byte(`{}`), AnswerJSON: []byte(`{}`), Solutions: []byte(`[]`),
		TimeLimitMS: 1000, MemoryLimitMiB: 256}
	if _, err := s.CreateProblem(payload); err != nil {
		t.Fatalf("create problem %q: %v", title, err)
	}
}

func facetMap(tags []TagCount) map[string]int {
	m := map[string]int{}
	for _, t := range tags {
		m[t.Tag] = t.Count
	}
	return m
}

func TestListTagFacets(t *testing.T) {
	s := newTestStore(t)
	mustAddProblem(t, s, "P1-加法", []string{"a", "b"})
	mustAddProblem(t, s, "P2-减法", []string{"a"})
	mustAddProblem(t, s, "P3-乘法", []string{"c"})

	// 空过滤：退化为全局计数
	tags, total, err := s.ListTagFacets(ProblemFilter{})
	if err != nil {
		t.Fatalf("facets: %v", err)
	}
	m := facetMap(tags)
	for tag, want := range map[string]int{"a": 2, "b": 1, "c": 1} {
		if m[tag] != want {
			t.Errorf("empty-filter count[%s] = %d, want %d", tag, m[tag], want)
		}
	}
	if total != 3 {
		t.Errorf("empty-filter total = %d, want 3", total)
	}
	// 排序：数量降序，同数量按名称
	if tags[0].Count < tags[len(tags)-1].Count && tags[0].Tag != "a" {
		t.Errorf("sort order wrong: %+v", tags)
	}

	// 选中 {a}：a 预览取消勾选后=全部 3；b 需同时含 {a,b}=1；c 需 {a,c}=0；total=2
	tags, total, err = s.ListTagFacets(ProblemFilter{Tags: []string{"a"}})
	if err != nil {
		t.Fatalf("facets(a): %v", err)
	}
	m = facetMap(tags)
	if m["a"] != 3 || m["b"] != 1 || m["c"] != 0 || total != 2 {
		t.Errorf("selected {a}: got a=%d b=%d c=%d total=%d, want 3/1/0/2", m["a"], m["b"], m["c"], total)
	}

	// 选中 {a,c}：total=0；a 预览去掉自身剩 {c}=1；c 预览去掉自身剩 {a}=2
	tags, total, err = s.ListTagFacets(ProblemFilter{Tags: []string{"a", "c"}})
	if err != nil {
		t.Fatalf("facets(a,c): %v", err)
	}
	m = facetMap(tags)
	if total != 0 || m["a"] != 1 || m["c"] != 2 {
		t.Errorf("selected {a,c}: total=%d a=%d c=%d, want 0/1/2", total, m["a"], m["c"])
	}

	// 搜索词参与基底过滤
	tags, total, err = s.ListTagFacets(ProblemFilter{Q: "加法"})
	if err != nil {
		t.Fatalf("facets(q): %v", err)
	}
	m = facetMap(tags)
	if m["a"] != 1 || m["b"] != 1 || m["c"] != 0 || total != 1 {
		t.Errorf("q=加法: a=%d b=%d c=%d total=%d, want 1/1/0/1", m["a"], m["b"], m["c"], total)
	}
}

func TestProblemFilterByTagsAndDir(t *testing.T) {
	s := newTestStore(t)
	dirA, _ := s.CreateDirectory("目录A", nil)
	dirB, _ := s.CreateDirectory("目录B", nil)
	mustAddProblem(t, s, "X1", []string{"t"})
	mustAddProblem(t, s, "X2", []string{"t"})
	p := model.Problem{Type: model.TypeProgramming, Title: "X3", Tags: []string{"t"},
		BodyJSON: []byte(`{}`), AnswerJSON: []byte(`{}`), Solutions: []byte(`[]`),
		TimeLimitMS: 1000, MemoryLimitMiB: 256, DirectoryID: &dirB}
	if _, err := s.CreateProblem(p); err != nil {
		t.Fatal(err)
	}
	_ = dirA

	list, err := s.ListProblems(ProblemFilter{Tags: []string{"t"}, DirID: &dirB})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Title != "X3" {
		t.Fatalf("dir+tag filter = %+v, want X3 only", list)
	}
}
