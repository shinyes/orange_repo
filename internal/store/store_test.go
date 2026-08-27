package store

import (
	"database/sql"
	"path/filepath"
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

	// 选中 {a}：每个标签仍显示自己实际命中的题数（a=2/b=1/c=1），total=当前筛出 =2
	tags, total, err = s.ListTagFacets(ProblemFilter{Tags: []string{"a"}})
	if err != nil {
		t.Fatalf("facets(a): %v", err)
	}
	m = facetMap(tags)
	if m["a"] != 2 || m["b"] != 1 || m["c"] != 1 || total != 2 {
		t.Errorf("selected {a}: got a=%d b=%d c=%d total=%d, want 2/1/1/2", m["a"], m["b"], m["c"], total)
	}

	// 选中 {a,c}：计数不受选中集影响；total=0（无题同时命中两者）
	tags, total, err = s.ListTagFacets(ProblemFilter{Tags: []string{"a", "c"}})
	if err != nil {
		t.Fatalf("facets(a,c): %v", err)
	}
	m = facetMap(tags)
	if total != 0 || m["a"] != 2 || m["c"] != 1 {
		t.Errorf("selected {a,c}: total=%d a=%d c=%d, want 0/2/1", total, m["a"], m["c"])
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

// 层级分面固定数据集：
//
//	P1: [数学/几何/圆]  P2: [数学/几何]  P3: [数学/代数, 算法]
func hierarchicalFixture(t *testing.T) *Store {
	s := newTestStore(t)
	mustAddProblem(t, s, "P1", []string{"数学/几何/圆"})
	mustAddProblem(t, s, "P2", []string{"数学/几何"})
	mustAddProblem(t, s, "P3", []string{"数学/代数", "算法"})
	return s
}

func TestTagPrefixFiltering(t *testing.T) {
	s := hierarchicalFixture(t)

	cases := []struct {
		selected []string
		want     []string // 期望命中的题目标题（升序）
	}{
		{nil, []string{"P1", "P2", "P3"}},
		{[]string{"数学"}, []string{"P1", "P2", "P3"}},
		{[]string{"数学/几何"}, []string{"P1", "P2"}},
		{[]string{"数学/几何/圆"}, []string{"P1"}},
		{[]string{"数学/代数"}, []string{"P3"}},
		{[]string{"算法"}, []string{"P3"}},
		{[]string{"数学", "算法"}, []string{"P3"}}, // AND：同时命中两个子树
		{[]string{"数学/几何", "算法"}, nil},         // 无交集
	}
	for _, tc := range cases {
		list, err := s.ListProblems(ProblemFilter{Tags: tc.selected})
		if err != nil {
			t.Fatalf("list(%v): %v", tc.selected, err)
		}
		got := map[string]bool{}
		for _, p := range list {
			got[p.Title] = true
		}
		if len(got) != len(tc.want) {
			t.Errorf("filter %v = %v, want %v", tc.selected, got, tc.want)
			continue
		}
		for _, w := range tc.want {
			if !got[w] {
				t.Errorf("filter %v missing %q (got %v)", tc.selected, w, got)
			}
		}
	}
}

func TestListTagFacetsHierarchical(t *testing.T) {
	s := hierarchicalFixture(t)

	// 空过滤：字面标签 + 虚拟祖先均出现，前缀计数
	tags, total, err := s.ListTagFacets(ProblemFilter{})
	if err != nil {
		t.Fatalf("facets: %v", err)
	}
	m := facetMap(tags)
	for tag, want := range map[string]int{
		"数学": 3, "数学/几何": 2, "数学/几何/圆": 1, "数学/代数": 1, "算法": 1,
	} {
		if m[tag] != want {
			t.Errorf("empty-filter count[%s] = %d, want %d", tag, m[tag], want)
		}
	}
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}

	// 选中 {数学}：total=3；计数均为各标签自己实际命中的题数（数学=3 几何=2 算法=1）
	tags, total, err = s.ListTagFacets(ProblemFilter{Tags: []string{"数学"}})
	if err != nil {
		t.Fatalf("facets(数学): %v", err)
	}
	m = facetMap(tags)
	if total != 3 || m["数学"] != 3 || m["数学/几何"] != 2 || m["算法"] != 1 {
		t.Errorf("selected {数学}: total=%d 数学=%d 几何=%d 算法=%d, want 3/3/2/1",
			total, m["数学"], m["数学/几何"], m["算法"])
	}

	// 选中 {数学/几何}：total=2；数学 仍显示含孙子的总数=3；数学/代数 显示自己的 1
	tags, total, err = s.ListTagFacets(ProblemFilter{Tags: []string{"数学/几何"}})
	if err != nil {
		t.Fatalf("facets(数学/几何): %v", err)
	}
	m = facetMap(tags)
	if total != 2 || m["数学"] != 3 || m["数学/代数"] != 1 {
		t.Errorf("selected {数学/几何}: total=%d 数学=%d 代数=%d, want 2/3/1",
			total, m["数学"], m["数学/代数"])
	}

	// q 缩小基底后候选与计数联动
	tags, total, err = s.ListTagFacets(ProblemFilter{Q: "P3"})
	if err != nil {
		t.Fatalf("facets(q): %v", err)
	}
	m = facetMap(tags)
	if total != 1 || m["数学"] != 1 || m["数学/代数"] != 1 || m["算法"] != 1 {
		if _, ok := m["数学/几何"]; ok && m["数学/几何"] > 0 {
			t.Errorf("q=P3 不应命中几何子树: %v", m)
		}
		t.Errorf("q=P3: total=%d m=%v", total, m)
	}
}

func TestNoneTagFiltering(t *testing.T) {
	s := newTestStore(t)
	mustAddProblem(t, s, "有标签", []string{"a"})
	mustAddProblem(t, s, "无标签题", []string{})
	mustAddProblem(t, s, "也是无标签", []string{})

	// 选中 __none__：命中 2 道无标签题
	list, err := s.ListProblems(ProblemFilter{Tags: []string{NoneTag}})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("none filter = %d, want 2", len(list))
	}

	// 无标签 + 有标签 AND 组合 → 0
	list, err = s.ListProblems(ProblemFilter{Tags: []string{NoneTag, "a"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("none+tag filter = %d, want 0", len(list))
	}

	// 分面：__none__ 计数 = 2
	tags, total, err := s.ListTagFacets(ProblemFilter{})
	if err != nil {
		t.Fatal(err)
	}
	m := facetMap(tags)
	if m[NoneTag] != 2 {
		t.Errorf("__none__ count = %d, want 2", m[NoneTag])
	}
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	// 选中 a → __none__ 计数不变（无标签题数），total=1
	tags, total, err = s.ListTagFacets(ProblemFilter{Tags: []string{"a"}})
	if err != nil {
		t.Fatal(err)
	}
	m = facetMap(tags)
	if m[NoneTag] != 2 {
		t.Errorf("none with a selected: count = %d, want 2", m[NoneTag])
	}
	if total != 1 {
		t.Errorf("total with a = %d, want 1", total)
	}
}

func TestValidateTagPath(t *testing.T) {
	valid := map[string]string{" 数学 ": "数学", "a/b/c": "a/b/c", "中文/标签": "中文/标签"}
	for in, want := range valid {
		got, err := ValidateTagPath(in)
		if err != nil || got != want {
			t.Errorf("ValidateTagPath(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "   ", "/a", "a/", "a//b"} {
		if _, err := ValidateTagPath(bad); err == nil {
			t.Errorf("ValidateTagPath(%q) 应报错", bad)
		}
	}
}

func TestRenameTagSubtree(t *testing.T) {
	s := hierarchicalFixture(t)

	// 整树搬家：数学 → Math
	n, err := s.RenameTag("数学", "Math")
	if err != nil || n != 3 {
		t.Fatalf("rename 数学→Math: n=%d err=%v, want 3", n, err)
	}
	p1, _ := s.GetProblem(mustFind(t, s, "P1"))
	p3, _ := s.GetProblem(mustFind(t, s, "P3"))
	if p1.Tags[0] != "Math/几何/圆" || p3.Tags[0] != "Math/代数" || p3.Tags[1] != "算法" {
		t.Errorf("after rename: p1=%v p3=%v", p1.Tags, p3.Tags)
	}

	// 子树搬家：Math/几何 → Math/图形
	n, err = s.RenameTag("Math/几何", "Math/图形")
	if err != nil || n != 2 {
		t.Fatalf("rename subtree: n=%d err=%v, want 2", n, err)
	}
	p2, _ := s.GetProblem(mustFind(t, s, "P2"))
	if p2.Tags[0] != "Math/图形" {
		t.Errorf("p2 after subtree rename: %v", p2.Tags)
	}
	p1, _ = s.GetProblem(mustFind(t, s, "P1"))
	if p1.Tags[0] != "Math/图形/圆" {
		t.Errorf("p1 after subtree rename: %v", p1.Tags)
	}

	// 重命名到已有标签时保序去重合并：算法 → Math/代数（P3 变成单标签）
	n, err = s.RenameTag("算法", "Math/代数")
	if err != nil || n != 1 {
		t.Fatalf("rename merge: n=%d err=%v, want 1", n, err)
	}
	p3, _ = s.GetProblem(mustFind(t, s, "P3"))
	if len(p3.Tags) != 1 || p3.Tags[0] != "Math/代数" {
		t.Errorf("merge dedup: %v, want [Math/代数]", p3.Tags)
	}

	if _, err := s.RenameTag("a", "/bad/"); err == nil {
		t.Error("非法路径应报错")
	}
	// 同名重命名是空操作
	n, err = s.RenameTag("Math/代数", "Math/代数")
	if err != nil || n != 0 {
		t.Fatalf("rename same: n=%d err=%v, want 0/nil", n, err)
	}
}

func TestDeleteTagSubtree(t *testing.T) {
	s := hierarchicalFixture(t)

	n, err := s.DeleteTag("数学/几何")
	if err != nil || n != 2 {
		t.Fatalf("delete subtree: n=%d err=%v, want 2", n, err)
	}
	p1, _ := s.GetProblem(mustFind(t, s, "P1"))
	p3, _ := s.GetProblem(mustFind(t, s, "P3"))
	if len(p1.Tags) != 0 {
		t.Errorf("p1 tags after delete: %v, want 空", p1.Tags)
	}
	if len(p3.Tags) != 2 || p3.Tags[0] != "数学/代数" || p3.Tags[1] != "算法" {
		t.Errorf("p3 tags after delete: %v, want 不受影响", p3.Tags)
	}

	// 分面不再出现被删子树的任何节点
	tags, _, err := s.ListTagFacets(ProblemFilter{})
	if err != nil {
		t.Fatal(err)
	}
	m := facetMap(tags)
	for _, gone := range []string{"数学/几何", "数学/几何/圆"} {
		if c, ok := m[gone]; ok && c > 0 {
			t.Errorf("%s 计数应为 0，得到 %d", gone, c)
		}
	}
}

func mustFind(t *testing.T, s *Store, title string) int64 {
	t.Helper()
	list, err := s.ListProblems(ProblemFilter{})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range list {
		if p.Title == title {
			return p.ID
		}
	}
	t.Fatalf("problem %q not found", title)
	return 0
}

// TestMigrateLegacyDirectories 旧 v1.0 库（含 directories 表与 directory_id 列）升级：
// 数据完好、目录结构移除。
func TestMigrateLegacyDirectories(t *testing.T) {
	dir := t.TempDir()
	dbPath := "file:" + filepath.ToSlash(filepath.Join(dir, "orangerepo.db"))
	legacy, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	oldSchema := []string{
		`CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`CREATE TABLE directories (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			parent_id INTEGER REFERENCES directories(id) ON DELETE SET NULL,
			order_no INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE TABLE problems (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			type TEXT NOT NULL,
			title TEXT NOT NULL,
			tags_json TEXT NOT NULL DEFAULT '[]',
			statement_md TEXT NOT NULL DEFAULT '',
			body_json TEXT NOT NULL DEFAULT '{}',
			answer_json TEXT NOT NULL DEFAULT '{}',
			solutions_json TEXT NOT NULL DEFAULT '[]',
			time_limit_ms INTEGER NOT NULL DEFAULT 1000,
			memory_limit_mib INTEGER NOT NULL DEFAULT 256,
			directory_id INTEGER REFERENCES directories(id) ON DELETE SET NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
		`INSERT INTO directories(id,name) VALUES(1,'旧目录')`,
		`INSERT INTO problems(type,title,tags_json,directory_id) VALUES('programming','旧题','["旧标签"]',1)`,
		`INSERT INTO problems(type,title,tags_json) VALUES('programming','无目录题','[]')`,
	}
	for _, q := range oldSchema {
		if _, err := legacy.Exec(q); err != nil {
			t.Fatalf("legacy schema: %v; stmt: %s", err, q)
		}
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := Open(dir)
	if err != nil {
		t.Fatalf("migrate open: %v", err)
	}
	defer s.Close()

	var dirTables int
	if err := s.DB.QueryRow(`SELECT COUNT(1) FROM sqlite_master WHERE name='directories'`).Scan(&dirTables); err != nil {
		t.Fatal(err)
	}
	if dirTables != 0 {
		t.Fatal("directories 表应已删除")
	}
	var dirCols int
	if err := s.DB.QueryRow(`SELECT COUNT(1) FROM pragma_table_info('problems') WHERE name='directory_id'`).Scan(&dirCols); err != nil {
		t.Fatal(err)
	}
	if dirCols != 0 {
		t.Fatal("directory_id 列应已删除")
	}
	list, err := s.ListProblems(ProblemFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("迁移后题目数 = %d, want 2", len(list))
	}
	p, err := s.GetProblem(list[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if p.Title == "旧题" && (len(p.Tags) != 1 || p.Tags[0] != "旧标签") {
		t.Errorf("迁移丢数据: %+v", p)
	}
	// 迁移后的库仍可正常写入（外键状态恢复）
	mustAddProblem(t, s, "新题", []string{"新标签"})
	if n, _ := s.CountProblems(); n != 3 {
		t.Errorf("count after insert = %d, want 3", n)
	}
}
