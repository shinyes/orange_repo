package quizstore_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"orangerepo/internal/accounts"
	"orangerepo/internal/model"
	"orangerepo/internal/quizstore"
	"orangerepo/internal/store"
)

// newTestEnvironment 建立临时目录：先用主站 store.Open 造题库（含样例题目），
// 关闭后以只读方式打开供刷题侧验证。
func newTestEnvironment(t *testing.T) *quizstore.Store {
	t.Helper()
	dir := t.TempDir()
	main, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open main store: %v", err)
	}
	sampleProblems := []model.Problem{
		{ // 1：单选，仅数学
			Type: model.TypeSingleChoice, Title: "单选A", Tags: []string{"数学"},
			StatementMD: "1+1=?", BodyJSON: json.RawMessage(`{"options":["1","2","3","4"]}`),
			AnswerJSON: json.RawMessage(`{"answerIndex":1}`), Solutions: json.RawMessage(`[]`),
		},
		{ // 2：单选，数学 + 物理/力学
			Type: model.TypeSingleChoice, Title: "单选B", Tags: []string{"数学", "物理/力学"},
			StatementMD: "重力方向?", BodyJSON: json.RawMessage(`{"options":["向上","向下"]}`),
			AnswerJSON: json.RawMessage(`{"answerIndex":1}`), Solutions: json.RawMessage(`[]`),
		},
		{ // 3：判断，物理/力学，带解析
			Type: model.TypeTrueFalse, Title: "判断A", Tags: []string{"物理/力学"},
			StatementMD: "自由落体加速度为 g", BodyJSON: json.RawMessage(`{}`),
			AnswerJSON: json.RawMessage(`{"answer":true}`),
			Solutions:   json.RawMessage(`[{"language":"","code":"","markdown":"解析：自由落体加速度约为 9.8 m/s²"}]`),
		},
		{ // 4：判断，无标签
			Type: model.TypeTrueFalse, Title: "判断B", Tags: []string{},
			StatementMD: "1 是质数", BodyJSON: json.RawMessage(`{}`),
			AnswerJSON: json.RawMessage(`{"answer":false}`), Solutions: json.RawMessage(`[]`),
		},
		{ // 5：编程（默认题型排除）
			Type: model.TypeProgramming, Title: "编程题", Tags: []string{"数学"},
			StatementMD: "求两数之和", BodyJSON: json.RawMessage(`{}`),
			AnswerJSON: json.RawMessage(`{}`), Solutions: json.RawMessage(`[]`),
		},
	}
	for _, p := range sampleProblems {
		if _, err := main.CreateProblem(p); err != nil {
			t.Fatalf("seed problem: %v", err)
		}
	}
	if err := main.Close(); err != nil {
		t.Fatalf("close main store: %v", err)
	}
	qs, err := quizstore.Open(dir, filepath.Join(dir, "orangerepo.db"))
	if err != nil {
		t.Fatalf("open quiz store: %v", err)
	}
	t.Cleanup(func() { _ = qs.Close() })
	return qs
}

// ids 提取题目 id 列表（顺序敏感）。
func ids(ps []quizstore.QuizProblem) []int64 {
	out := make([]int64, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.ID)
	}
	return out
}

func containsAll(have []int64, want ...int64) bool {
	set := map[int64]bool{}
	for _, id := range have {
		set[id] = true
	}
	for _, id := range want {
		if !set[id] {
			return false
		}
	}
	return true
}

// ---------- problems_test ----------

func TestMatchingProblems_FilterSemantics(t *testing.T) {
	qs := newTestEnvironment(t)
	all, err := qs.Repo.MatchingProblems(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 4 { // 编程题被默认题型排除
		t.Fatalf("默认题型命中数 = %d, want 4", len(all))
	}
	mathOnly, err := qs.Repo.MatchingProblems([]string{"数学"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(mathOnly) != 2 { // 单选A + 单选B；编程题类型排除
		t.Fatalf("数学命中数 = %d, want 2", len(mathOnly))
	}
	virtualParent, err := qs.Repo.MatchingProblems([]string{"物理"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(virtualParent) != 2 { // 前缀子孙：单选B + 判断A
		t.Fatalf("物理(虚拟父标签)命中数 = %d, want 2", len(virtualParent))
	}
	tfOnly, err := qs.Repo.MatchingProblems(nil, []string{"true_false"})
	if err != nil {
		t.Fatal(err)
	}
	if len(tfOnly) != 2 {
		t.Fatalf("判断题命中数 = %d, want 2", len(tfOnly))
	}
	noHits, err := qs.Repo.MatchingProblems([]string{"不存在的标签"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(noHits) != 0 {
		t.Fatalf("不存在标签命中数 = %d, want 0", len(noHits))
	}
	// 题目 2 标签 [数学, 物理/力学]：AND 两个标签应同时命中
	andHit, err := qs.Repo.MatchingProblems([]string{"数学", "物理/力学"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(andHit) != 1 || andHit[0].Title != "单选B" {
		t.Fatalf("AND 组合命中 = %v, want 单选B", ids(andHit))
	}
}

func TestCountProblems(t *testing.T) {
	qs := newTestEnvironment(t)
	n, err := qs.Repo.CountProblems([]string{"数学"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("count = %d, want 2", n)
	}
}

func TestGetQuizProblem_HasExplanation(t *testing.T) {
	qs := newTestEnvironment(t)
	all, _ := qs.Repo.MatchingProblems(nil, nil)
	if !containsAll(ids(all), 1, 2, 3, 4) {
		t.Fatalf("样例题目缺失: %v", ids(all))
	}
	p3, err := qs.Repo.GetQuizProblem(3)
	if err != nil {
		t.Fatal(err)
	}
	if !p3.HasExplanation {
		t.Fatal("判断A 应标记有解析")
	}
	p1, err := qs.Repo.GetQuizProblem(1)
	if err != nil {
		t.Fatal(err)
	}
	if p1.HasExplanation {
		t.Fatal("单选A 无题解，不应标记有解析")
	}
	if _, err := qs.Repo.GetQuizProblem(9999); err == nil {
		t.Fatal("不存在的题目应返回错误")
	}
}

func TestGetAnswer(t *testing.T) {
	qs := newTestEnvironment(t)
	env, err := qs.Repo.GetAnswer(1)
	if err != nil {
		t.Fatal(err)
	}
	if env.Type != "single_choice" || env.AnswerIndex == nil || *env.AnswerIndex != 1 {
		t.Fatalf("题目1 答案 = %+v, want answerIndex=1", env)
	}
	env3, err := qs.Repo.GetAnswer(3)
	if err != nil {
		t.Fatal(err)
	}
	if env3.Answer == nil || !*env3.Answer {
		t.Fatalf("题目3 答案 = %+v, want true", env3)
	}
	if _, err := qs.Repo.GetAnswer(5); err == nil {
		t.Fatal("编程题应拒绝判题")
	}
}

func TestGetExplanation(t *testing.T) {
	qs := newTestEnvironment(t)
	md, ok := qs.Repo.GetExplanation(3)
	if !ok || md == "" {
		t.Fatalf("题目3 应返回解析, got ok=%v md=%q", ok, md)
	}
	if _, ok := qs.Repo.GetExplanation(1); ok {
		t.Fatal("题目1 无解析")
	}
}

// ---------- quizstore_test ----------

func TestSubjectsAndCategories(t *testing.T) {
	qs := newTestEnvironment(t)
	s1, err := qs.CreateSubject("数学")
	if err != nil {
		t.Fatal(err)
	}
	s2, err := qs.CreateSubject("物理")
	if err != nil {
		t.Fatal(err)
	}
	c1, err := qs.CreateCategory(s1, "代数", 0, []string{"数学"}, []string{"single_choice"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := qs.CreateCategory(s1, "几何", 0, []string{"数学/几何"}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := qs.CreateCategory(s1, "非法题型", 0, nil, []string{"multi_choice"}); err == nil {
		t.Fatal("非法题型应被拒绝")
	}
	if _, err := qs.CreateCategory(s1, "非法标签", 0, []string{"/bad"}, nil); err == nil {
		t.Fatal("非法标签应被拒绝")
	}
	subjects, err := qs.ListSubjects()
	if err != nil {
		t.Fatal(err)
	}
	if len(subjects) != 2 || len(subjects[0].Categories) != 2 {
		t.Fatalf("科目/分类 = %+v", subjects)
	}
	if subjects[0].Categories[0].ID != c1 {
		t.Fatalf("分类顺序错乱: %+v", subjects[0].Categories)
	}
	// 科目排序
	if err := qs.SetSubjectOrder([]int64{s2, s1}); err != nil {
		t.Fatal(err)
	}
	subjects, _ = qs.ListSubjects()
	if subjects[0].ID != s2 {
		t.Fatal("科目顺序未生效")
	}
	if err := qs.SetSubjectOrder([]int64{s2}); err == nil {
		t.Fatal("不完整的顺序列表应报错")
	}
	// 分类排序（含跨科目防御）
	if err := qs.SetCategoryOrder(s1, []int64{c1}); err != nil {
		t.Fatal(err)
	}
	// 删除科目级联
	if err := qs.DeleteSubject(s2); err != nil {
		t.Fatal(err)
	}
	if err := qs.SetCategoryOrder(s2, []int64{c1}); err == nil {
		t.Fatal("已删科目的排序应失败")
	}
}

func TestWrongAnswersLifecycle(t *testing.T) {
	qs := newTestEnvironment(t)
	uid, err := qs.Accounts.CreateUser("carol", "pw", accounts.RoleStudent)
	if err != nil {
		t.Fatal(err)
	}
	s1, _ := qs.CreateSubject("数学")
	c1, _ := qs.CreateCategory(s1, "代数", 0, nil, nil)
	c2, _ := qs.CreateCategory(s1, "几何", 0, nil, nil)
	// 答错入集：重复记录保留首次分类
	if err := qs.AddWrong(uid, 101, c1); err != nil {
		t.Fatal(err)
	}
	if err := qs.AddWrong(uid, 101, c2); err != nil {
		t.Fatal(err)
	}
	if err := qs.AddWrong(uid, 102, c2); err != nil {
		t.Fatal(err)
	}
	total, err := qs.WrongTotal(uid)
	if err != nil || total != 2 {
		t.Fatalf("错题总数 = %d, want 2", total)
	}
	groups, err := qs.WrongGroups(uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 || groups[0].CategoryID != c1 || groups[0].Count != 1 || groups[1].CategoryID != c2 || groups[1].Count != 1 {
		t.Fatalf("分组 = %+v", groups)
	}
	// 按分类过滤错题
	cat2 := c2
	only2, err := qs.ListWrongProblems(uid, &cat2)
	if err != nil || len(only2) != 1 || only2[0].ProblemID != 102 {
		t.Fatalf("分类2错题 = %+v, err=%v", only2, err)
	}
	all, err := qs.ListWrongProblems(uid, nil)
	if err != nil || len(all) != 2 {
		t.Fatalf("全部错题 = %+v, err=%v", all, err)
	}
	// 答对移除
	if err := qs.RemoveWrong(uid, 101); err != nil {
		t.Fatal(err)
	}
	total, _ = qs.WrongTotal(uid)
	if total != 1 {
		t.Fatalf("移除后总数 = %d, want 1", total)
	}
	// 学生删除级联
	_ = qs.Accounts.DeleteStudent(uid)
	total, _ = qs.WrongTotal(uid)
	if total != 0 {
		t.Fatalf("删除学生后错题 = %d, want 0", total)
	}
}

func TestRoundSize(t *testing.T) {
	qs := newTestEnvironment(t)
	if qs.GetRoundSize() != 10 {
		t.Fatalf("默认每轮题数 = %d, want 10", qs.GetRoundSize())
	}
	if err := qs.SetRoundSize(5); err != nil {
		t.Fatal(err)
	}
	if qs.GetRoundSize() != 5 {
		t.Fatal("每轮题数未生效")
	}
	if err := qs.SetRoundSize(0); err == nil {
		t.Fatal("0 应被拒绝")
	}
	if err := qs.SetRoundSize(101); err == nil {
		t.Fatal("101 应被拒绝")
	}
}