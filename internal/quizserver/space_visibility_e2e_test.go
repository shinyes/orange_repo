// 空间项目可见性端到端回归（门户成员视角，HTTP 层真实请求 + 真实临时 sqlite 库）。
//
// 语义：成员可见某训练/练习/刷题项目须**同时**满足 ①「已开放」(is_public=1)
// 与 ②「已分配到该项目可见名单」；两者缺一即列表不出现、直达 404。管理员（域/系统）恒可见。
//
// 覆盖：四种组合的阶段推进（未开放未分配 → 仅分配 → 仅开放 → 开放且分配）、
// 三类项目的列表/详情/作答/交卷/草稿/记录入口、同空间「只开放」与「只分配」两个项目的
// 交叉组合（防把两个条件合并成空间级判断）、非空间成员 403 边界。
package quizserver

import (
	"fmt"
	"testing"

	"github.com/gofiber/fiber/v2"

	"orangeoj/internal/model"
	"orangeoj/internal/quizstore"
	"orangeoj/internal/store"
)

// svHTTPTables 项目类型 → 主库表 / 可见名单表。
var svHTTPTables = map[string]struct{ item, visible string }{
	"training": {"space_trainings", "space_training_visible"},
	"practice": {"space_practices", "space_practice_visible"},
	"quiz":     {"space_quizzes", "space_quiz_visible"},
}

// svHTTPEnv 端到端环境。
type svHTTPEnv struct {
	app  *fiber.App
	main *store.Store // 保持打开：测试中翻转 is_public / 改写可见名单

	spaceID int64
	problem int64

	// 四阶段驱动项目（初始：未开放 + 未分配）
	training int64
	practice int64
	quiz     int64

	// 交叉组合项目：同空间同类型的「只开放」与「只分配」
	trainOpenOnly     int64
	trainAssignedOnly int64
	quizOpenOnly      int64
	quizAssignedOnly  int64

	stuID int64

	stuCookie    string // 空间成员
	outsiderCook string // member，但不在该空间
	gAdminCookie string
	dAdminCookie string
}

// svHTTPCreateTraining 建训练 + 1 章 + 1 题。
func svHTTPCreateTraining(t *testing.T, main *store.Store, spaceID, problemID int64, title string, isPublic bool) int64 {
	t.Helper()
	id, err := main.CreateSpaceTraining(spaceID, title, "", nil, 3, isPublic)
	if err != nil {
		t.Fatalf("create training %s: %v", title, err)
	}
	ch, err := main.CreateSpaceChapter(id, "章")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := main.AddSpaceChapterItems(ch, []int64{problemID}); err != nil {
		t.Fatal(err)
	}
	return id
}

// newSVHTTPEnv 组装环境（复用 portal_integration_test.go 的三阶段建库范式）。
func newSVHTTPEnv(t *testing.T) *svHTTPEnv {
	t.Helper()
	dir := t.TempDir()
	env := &svHTTPEnv{}

	// ---- 阶段 1：主库建域/空间/题/项目 ----
	main, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open main store: %v", err)
	}
	env.main = main
	domainID, err := main.CreateDomain("可见性E2E域")
	if err != nil {
		t.Fatal(err)
	}
	spaceID, err := main.CreateSpace(domainID, "可见性E2E班")
	if err != nil {
		t.Fatal(err)
	}
	env.spaceID = spaceID
	pid, err := main.CreateProblem(model.Problem{
		Type: model.TypeSingleChoice, Title: "可见性E2E题", StatementMD: "1+1=?",
		BodyJSON:   jsonRaw(`{"options":["1","2"]}`),
		AnswerJSON: jsonRaw(`{"answerIndex":1}`), Solutions: jsonRaw(`[]`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := main.DB.Exec(`UPDATE problems SET domain_id=? WHERE id=?`, domainID, pid); err != nil {
		t.Fatal(err)
	}
	env.problem = pid

	// 四阶段驱动项目：初始「未开放 + 未分配」
	env.training = svHTTPCreateTraining(t, main, spaceID, pid, "训练T1", false)
	prID, err := main.CreateSpacePractice(spaceID, "练习P1", "", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	env.practice = prID
	if err := main.AddSpacePracticeItems(prID, []int64{pid}); err != nil {
		t.Fatal(err)
	}
	qzID, err := main.CreateSpaceQuiz(spaceID, "刷题Q1", nil, "tags", "", 0, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	env.quiz = qzID

	// 交叉组合：训练「只开放」/「只分配」；刷题「只开放」/「只分配」
	env.trainOpenOnly = svHTTPCreateTraining(t, main, spaceID, pid, "训练T2只开放", true)
	env.trainAssignedOnly = svHTTPCreateTraining(t, main, spaceID, pid, "训练T3只分配", false)
	env.quizOpenOnly, err = main.CreateSpaceQuiz(spaceID, "刷题Q2只开放", nil, "tags", "", 0, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	env.quizAssignedOnly, err = main.CreateSpaceQuiz(spaceID, "刷题Q3只分配", nil, "tags", "", 0, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := main.Close(); err != nil {
		t.Fatal(err)
	}

	// ---- 阶段 2：quiz 库建账号（member / 非本空间 member / 域管理员 / 系统管理员） ----
	qs, err := quizstore.Open(dir)
	if err != nil {
		t.Fatalf("open quiz store: %v", err)
	}
	t.Cleanup(func() { _ = qs.Close() })
	env.stuID, err = qs.Accounts.CreateUser("visStu", "pw", "member")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := qs.Accounts.CreateUser("visOutsider", "pw", "member"); err != nil {
		t.Fatal(err)
	}
	if _, err := qs.Accounts.CreateUser("visGAdmin", "pw", "global_admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := qs.Accounts.CreateUser("visDAdmin", "pw", "domain_admin", domainID); err != nil {
		t.Fatal(err)
	}

	// ---- 阶段 3：回主库写空间成员 + 交叉组合的初始名单 ----
	main2, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = main2.Close() })
	env.main = main2
	// 只有 visStu 是本空间成员（visOutsider 用于非成员 403 边界）
	if err := main2.SetSpaceMembers(spaceID, []int64{env.stuID}); err != nil {
		t.Fatal(err)
	}
	// 「只分配」项目：名单含本成员但未开放
	if err := main2.SetVisibleUsers("space_training_visible", env.trainAssignedOnly, []int64{env.stuID}); err != nil {
		t.Fatal(err)
	}
	if err := main2.SetVisibleUsers("space_quiz_visible", env.quizAssignedOnly, []int64{env.stuID}); err != nil {
		t.Fatal(err)
	}

	env.app = New(&Server{QS: qs}, nil, 0)
	env.stuCookie = loginStudent(t, env.app, "visStu", "pw")
	env.outsiderCook = loginStudent(t, env.app, "visOutsider", "pw")
	env.gAdminCookie = loginStudent(t, env.app, "visGAdmin", "pw")
	env.dAdminCookie = loginStudent(t, env.app, "visDAdmin", "pw")
	return env
}

// setPublic 翻转项目开放位（模拟门户「开放」开关）。
func (e *svHTTPEnv) setPublic(t *testing.T, kind string, itemID int64, pub bool) {
	t.Helper()
	tbl := svHTTPTables[kind].item
	v := 0
	if pub {
		v = 1
	}
	if _, err := e.main.DB.Exec(`UPDATE `+tbl+` SET is_public=? WHERE id=?`, v, itemID); err != nil {
		t.Fatalf("set is_public %s/%d: %v", kind, itemID, err)
	}
}

// setVisible 覆盖式改写项目可见名单（模拟门户「可见性」浮窗保存）。
func (e *svHTTPEnv) setVisible(t *testing.T, kind string, itemID int64, userIDs []int64) {
	t.Helper()
	if err := e.main.SetVisibleUsers(svHTTPTables[kind].visible, itemID, userIDs); err != nil {
		t.Fatalf("set visible %s/%d: %v", kind, itemID, err)
	}
}

// req 发一次请求并断言状态码（wantErr 非空时同时断言错误文案）。
func (e *svHTTPEnv) req(t *testing.T, method, path, cookie string, body any, want int, wantErr string) map[string]any {
	t.Helper()
	resp, out := doJSON(t, e.app, method, path, cookie, body)
	if resp.StatusCode != want {
		t.Fatalf("%s %s = %d %v，want %d %s", method, path, resp.StatusCode, out, want, wantErr)
	}
	if wantErr != "" {
		if got, _ := out["error"].(string); got != wantErr {
			t.Fatalf("%s %s error = %q，want %q（完整响应 %v）", method, path, got, wantErr, out)
		}
	}
	return out
}

// idsOf 取响应中某数组字段的 id 列表（按返回顺序）。
func idsOf(t *testing.T, out map[string]any, key string) []int64 {
	t.Helper()
	raw, ok := out[key].([]any)
	if !ok {
		t.Fatalf("字段 %s 不是数组: %v", key, out)
	}
	ids := make([]int64, 0, len(raw))
	for _, v := range raw {
		m, ok := v.(map[string]any)
		if !ok {
			t.Fatalf("字段 %s 元素不是对象: %v", key, v)
		}
		f, ok := m["id"].(float64)
		if !ok {
			t.Fatalf("字段 %s 元素缺 id: %v", key, m)
		}
		ids = append(ids, int64(f))
	}
	return ids
}

// wantIDs 断言 id 集合完全相等（顺序敏感：列表按 id 升序）。
func wantIDs(t *testing.T, label string, got, want []int64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %v（%d 项），want %v（%d 项）", label, got, len(got), want, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s = %v，want %v（第 %d 项）", label, got, want, i)
		}
	}
}

// home 取空间首页三区 id。
func (e *svHTTPEnv) home(t *testing.T, cookie string) (trainings, practices, quizzes []int64) {
	t.Helper()
	out := e.req(t, "GET", fmt.Sprintf("/api/portal/space/%d/home", e.spaceID), cookie, nil, fiber.StatusOK, "")
	return idsOf(t, out, "trainings"), idsOf(t, out, "practices"), idsOf(t, out, "quizzes")
}

// quizzesIn 取 /api/portal/space/:id/quizzes 的 id 列表。
func (e *svHTTPEnv) quizzesIn(t *testing.T, cookie string) []int64 {
	t.Helper()
	out := e.req(t, "GET", fmt.Sprintf("/api/portal/space/%d/quizzes", e.spaceID), cookie, nil, fiber.StatusOK, "")
	return idsOf(t, out, "quizzes")
}

// assertMemberCannotReach 断言成员对三类驱动项目的**全部入口**都拒绝（404 且错误文案一致），
// 且列表/首页不含这些项目（给定期望的列表内容由调用方传入）。
func (e *svHTTPEnv) assertMemberCannotReach(t *testing.T, stage string, homeTrainings, homePractices, homeQuizzes []int64) {
	t.Helper()
	sp, tr, pr, qz := e.spaceID, e.training, e.practice, e.quiz

	tr2, pr2, qz2 := e.home(t, e.stuCookie)
	wantIDs(t, stage+" 首页 trainings", tr2, homeTrainings)
	wantIDs(t, stage+" 首页 practices", pr2, homePractices)
	wantIDs(t, stage+" 首页 quizzes", qz2, homeQuizzes)
	wantIDs(t, stage+" 刷题列表", e.quizzesIn(t, e.stuCookie), homeQuizzes)

	// 列表不出现 → 直达详情/作答/交卷/草稿/记录一路 404（按 handler 实际返回码断言）
	e.req(t, "GET", fmt.Sprintf("/api/portal/space/%d/training/%d", sp, tr), e.stuCookie, nil,
		fiber.StatusNotFound, "训练不存在")
	e.req(t, "POST", fmt.Sprintf("/api/portal/space/%d/training/%d/answer", sp, tr), e.stuCookie,
		map[string]any{"problemId": e.problem, "answer": 1}, fiber.StatusNotFound, "训练不存在")
	e.req(t, "GET", fmt.Sprintf("/api/portal/space/%d/practice/%d", sp, pr), e.stuCookie, nil,
		fiber.StatusNotFound, "练习不存在")
	e.req(t, "POST", fmt.Sprintf("/api/portal/space/%d/practice/%d/submit", sp, pr), e.stuCookie,
		map[string]any{"answers": []map[string]any{{"problemId": e.problem, "answer": 1}}},
		fiber.StatusNotFound, "练习不存在")
	e.req(t, "GET", fmt.Sprintf("/api/portal/space/%d/practice/%d/draft", sp, pr), e.stuCookie, nil,
		fiber.StatusNotFound, "练习不存在")
	e.req(t, "PUT", fmt.Sprintf("/api/portal/space/%d/practice/%d/draft", sp, pr), e.stuCookie,
		map[string]any{"answers": map[string]any{}}, fiber.StatusNotFound, "练习不存在")
	e.req(t, "GET", fmt.Sprintf("/api/portal/space/%d/practice/%d/submissions", sp, pr), e.stuCookie, nil,
		fiber.StatusNotFound, "练习不存在")
	e.req(t, "GET", fmt.Sprintf("/api/portal/quiz/%d/problem", qz), e.stuCookie, nil,
		fiber.StatusNotFound, "刷题项目不存在")
	e.req(t, "POST", fmt.Sprintf("/api/portal/quiz/%d/answer", qz), e.stuCookie,
		map[string]any{"problemId": e.problem, "answer": 1}, fiber.StatusNotFound, "刷题项目不存在")
}

// assertAdminCanReach 断言管理员恒可见：三类项目均出现在列表/首页，详情可取。
func (e *svHTTPEnv) assertAdminCanReach(t *testing.T, stage, cookie, role string) {
	t.Helper()
	sp, tr, pr, qz := e.spaceID, e.training, e.practice, e.quiz
	tr2, pr2, qz2 := e.home(t, cookie)
	if !containsID(tr2, tr) || !containsID(pr2, pr) || !containsID(qz2, qz) {
		t.Fatalf("%s %s：未开放未分配项目应可见，实得 trainings=%v practices=%v quizzes=%v",
			stage, role, tr2, pr2, qz2)
	}
	e.req(t, "GET", fmt.Sprintf("/api/portal/space/%d/training/%d", sp, tr), cookie, nil, fiber.StatusOK, "")
	e.req(t, "GET", fmt.Sprintf("/api/portal/space/%d/practice/%d", sp, pr), cookie, nil, fiber.StatusOK, "")
	e.req(t, "GET", fmt.Sprintf("/api/portal/quiz/%d/problem", qz), cookie, nil, fiber.StatusOK, "")
}

func containsID(ids []int64, want int64) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

// TestSpaceVisibilityMemberStages 成员视角四阶段推进（未开放未分配 → 仅分配 → 仅开放 → 开放且分配）。
func TestSpaceVisibilityMemberStages(t *testing.T) {
	e := newSVHTTPEnv(t)

	// ---- 阶段 A：未开放 + 未分配 → 全部不可见 ----
	e.assertMemberCannotReach(t, "A(未开放未分配)", nil, nil, nil)
	// 管理员在任何阶段都可见可进入（此处为「未开放未分配」阶段）
	e.assertAdminCanReach(t, "A", e.gAdminCookie, "系统管理员")
	e.assertAdminCanReach(t, "A", e.dAdminCookie, "域管理员")

	// ---- 阶段 B：仅分配（未开放）→ 仍全部不可见（关键：仅分配不够） ----
	e.setVisible(t, "training", e.training, []int64{e.stuID})
	e.setVisible(t, "practice", e.practice, []int64{e.stuID})
	e.setVisible(t, "quiz", e.quiz, []int64{e.stuID})
	e.assertMemberCannotReach(t, "B(仅分配未开放)", nil, nil, nil)
	e.assertAdminCanReach(t, "B", e.gAdminCookie, "系统管理员")
	e.assertAdminCanReach(t, "B", e.dAdminCookie, "域管理员")

	// ---- 阶段 C：仅开放（未分配）→ 仍全部不可见（关键：仅开放不够） ----
	e.setVisible(t, "training", e.training, nil)
	e.setVisible(t, "practice", e.practice, nil)
	e.setVisible(t, "quiz", e.quiz, nil)
	e.setPublic(t, "training", e.training, true)
	e.setPublic(t, "practice", e.practice, true)
	e.setPublic(t, "quiz", e.quiz, true)
	e.assertMemberCannotReach(t, "C(仅开放未分配)", nil, nil, nil)
	e.assertAdminCanReach(t, "C", e.gAdminCookie, "系统管理员")
	e.assertAdminCanReach(t, "C", e.dAdminCookie, "域管理员")

	// ---- 阶段 D：开放 + 分配 → 列表可见且能进入 ----
	e.setVisible(t, "training", e.training, []int64{e.stuID})
	e.setVisible(t, "practice", e.practice, []int64{e.stuID})
	e.setVisible(t, "quiz", e.quiz, []int64{e.stuID})

	tr, pr, qz := e.home(t, e.stuCookie)
	wantIDs(t, "D 首页 trainings", tr, []int64{e.training})
	wantIDs(t, "D 首页 practices", pr, []int64{e.practice})
	// 刷题区含「开放且分配」的 Q1；交叉组合的 Q2 只开放、Q3 只分配都不该出现
	wantIDs(t, "D 首页 quizzes", qz, []int64{e.quiz})
	wantIDs(t, "D 刷题列表", e.quizzesIn(t, e.stuCookie), []int64{e.quiz})

	sp := e.spaceID
	// 训练详情 200 + 回传项目 id 一致
	detail := e.req(t, "GET", fmt.Sprintf("/api/portal/space/%d/training/%d", sp, e.training), e.stuCookie, nil,
		fiber.StatusOK, "")
	trObj, ok := detail["training"].(map[string]any)
	if !ok || int64(trObj["id"].(float64)) != e.training {
		t.Fatalf("D 训练详情 = %v，want id=%d", detail["training"], e.training)
	}
	// 作答 200：客观题按题库答案判对
	ans := e.req(t, "POST", fmt.Sprintf("/api/portal/space/%d/training/%d/answer", sp, e.training), e.stuCookie,
		map[string]any{"problemId": e.problem, "answer": 1}, fiber.StatusOK, "")
	if ans["correct"] != true {
		t.Fatalf("D 训练作答 = %v，want correct=true", ans)
	}
	// 练习详情/交卷/记录 200
	e.req(t, "GET", fmt.Sprintf("/api/portal/space/%d/practice/%d", sp, e.practice), e.stuCookie, nil,
		fiber.StatusOK, "")
	sub := e.req(t, "POST", fmt.Sprintf("/api/portal/space/%d/practice/%d/submit", sp, e.practice), e.stuCookie,
		map[string]any{"answers": []map[string]any{{"problemId": e.problem, "answer": 1}}}, fiber.StatusOK, "")
	if sub["objectiveTotal"].(float64) != 1 || sub["objectiveCorrect"].(float64) != 1 {
		t.Fatalf("D 练习交卷 = %v，want 1/1", sub)
	}
	subs := e.req(t, "GET", fmt.Sprintf("/api/portal/space/%d/practice/%d/submissions", sp, e.practice), e.stuCookie, nil,
		fiber.StatusOK, "")
	if len(subs["submissions"].([]any)) != 1 {
		t.Fatalf("D 交卷记录 = %v，want 1 条", subs)
	}
	// 刷题抽题 200
	qp := e.req(t, "GET", fmt.Sprintf("/api/portal/quiz/%d/problem", e.quiz), e.stuCookie, nil, fiber.StatusOK, "")
	if qp["done"] == true || qp["problem"] == nil {
		t.Fatalf("D 刷题抽题 = %v，want 有题可刷", qp)
	}
	// 管理员在阶段 D 同样可见
	e.assertAdminCanReach(t, "D", e.gAdminCookie, "系统管理员")
	e.assertAdminCanReach(t, "D", e.dAdminCookie, "域管理员")
}

// TestSpaceVisibilityCrossCombination 同空间交叉组合：
// 训练「只开放 T2」+「只分配 T3」、刷题「只开放 Q2」+「只分配 Q3」——成员一个都看不到；
// 逐个补齐条件时只有被补齐的那个出现（证明可见性按项目逐个判定，未合并到空间级）。
func TestSpaceVisibilityCrossCombination(t *testing.T) {
	e := newSVHTTPEnv(t)
	sp := e.spaceID

	// 初始：交叉组合项目均不可见（此时四阶段驱动项目也未开放未分配 → 三区应为空）
	e.assertMemberCannotReach(t, "交叉-初始", nil, nil, nil)

	// 只开放 / 只分配 两个半边并存 → 成员仍看不到任何一个
	tr, _, qz := e.home(t, e.stuCookie)
	wantIDs(t, "交叉-半边 trainings", tr, nil)
	wantIDs(t, "交叉-半边 quizzes", qz, nil)
	e.req(t, "GET", fmt.Sprintf("/api/portal/space/%d/training/%d", sp, e.trainOpenOnly), e.stuCookie, nil,
		fiber.StatusNotFound, "训练不存在")
	e.req(t, "GET", fmt.Sprintf("/api/portal/space/%d/training/%d", sp, e.trainAssignedOnly), e.stuCookie, nil,
		fiber.StatusNotFound, "训练不存在")
	e.req(t, "GET", fmt.Sprintf("/api/portal/quiz/%d/problem", e.quizOpenOnly), e.stuCookie, nil,
		fiber.StatusNotFound, "刷题项目不存在")
	e.req(t, "GET", fmt.Sprintf("/api/portal/quiz/%d/problem", e.quizAssignedOnly), e.stuCookie, nil,
		fiber.StatusNotFound, "刷题项目不存在")

	// 给「只分配」的训练补开放 → 只出现它；「只开放」的训练仍不可见
	e.setPublic(t, "training", e.trainAssignedOnly, true)
	tr, _, _ = e.home(t, e.stuCookie)
	wantIDs(t, "交叉-训练补开放后", tr, []int64{e.trainAssignedOnly})
	e.req(t, "GET", fmt.Sprintf("/api/portal/space/%d/training/%d", sp, e.trainOpenOnly), e.stuCookie, nil,
		fiber.StatusNotFound, "训练不存在")
	// 给「只分配」的刷题补开放 → 同理
	e.setPublic(t, "quiz", e.quizAssignedOnly, true)
	wantIDs(t, "交叉-刷题补开放后", e.quizzesIn(t, e.stuCookie), []int64{e.quizAssignedOnly})
	e.req(t, "GET", fmt.Sprintf("/api/portal/quiz/%d/problem", e.quizOpenOnly), e.stuCookie, nil,
		fiber.StatusNotFound, "刷题项目不存在")

	// 再给「只开放」的项目补名单 → 两个都可见（顺序按 id）
	e.setVisible(t, "training", e.trainOpenOnly, []int64{e.stuID})
	e.setVisible(t, "quiz", e.quizOpenOnly, []int64{e.stuID})
	tr, _, _ = e.home(t, e.stuCookie)
	want := []int64{e.trainOpenOnly, e.trainAssignedOnly}
	if e.trainAssignedOnly < e.trainOpenOnly {
		want = []int64{e.trainAssignedOnly, e.trainOpenOnly}
	}
	wantIDs(t, "交叉-训练两个条件都补齐后", tr, want)
	qwant := []int64{e.quizOpenOnly, e.quizAssignedOnly}
	if e.quizAssignedOnly < e.quizOpenOnly {
		qwant = []int64{e.quizAssignedOnly, e.quizOpenOnly}
	}
	wantIDs(t, "交叉-刷题两个条件都补齐后", e.quizzesIn(t, e.stuCookie), qwant)

	// 管理员：交叉组合项目全部可见（与是否开放/分配无关）
	e.assertAdminCanReach(t, "交叉", e.gAdminCookie, "系统管理员")
	e.assertAdminCanReach(t, "交叉", e.dAdminCookie, "域管理员")
	atr, _, aqz := e.home(t, e.gAdminCookie)
	for _, id := range []int64{e.trainOpenOnly, e.trainAssignedOnly} {
		if !containsID(atr, id) {
			t.Fatalf("管理员 trainings = %v，缺 %d", atr, id)
		}
	}
	for _, id := range []int64{e.quizOpenOnly, e.quizAssignedOnly} {
		if !containsID(aqz, id) {
			t.Fatalf("管理员 quizzes = %v，缺 %d", aqz, id)
		}
	}
}

// TestSpaceVisibilityNonMemberForbidden 非空间成员：即使项目「开放 + 分配」（给别人），
// 也在空间层先被拦（403）——两层模型（空间成员 + 项目双条件）都不放宽。
func TestSpaceVisibilityNonMemberForbidden(t *testing.T) {
	e := newSVHTTPEnv(t)
	sp := e.spaceID
	// 让驱动项目处于「开放 + 分配给 visStu」（visOutsider 不是空间成员）
	e.setPublic(t, "training", e.training, true)
	e.setVisible(t, "training", e.training, []int64{e.stuID})
	e.setPublic(t, "quiz", e.quiz, true)
	e.setVisible(t, "quiz", e.quiz, []int64{e.stuID})

	// 空间成员 visStu：可见可进入（对照）
	e.req(t, "GET", fmt.Sprintf("/api/portal/space/%d/training/%d", sp, e.training), e.stuCookie, nil,
		fiber.StatusOK, "")
	// 非成员：403「你不是该空间成员」（空间层拦截，先于项目可见性）
	e.req(t, "GET", fmt.Sprintf("/api/portal/space/%d/training/%d", sp, e.training), e.outsiderCook, nil,
		fiber.StatusForbidden, "你不是该空间成员")
	e.req(t, "GET", fmt.Sprintf("/api/portal/space/%d/home", sp), e.outsiderCook, nil,
		fiber.StatusForbidden, "你不是该空间成员")
	e.req(t, "GET", fmt.Sprintf("/api/portal/quiz/%d/problem", e.quiz), e.outsiderCook, nil,
		fiber.StatusForbidden, "你不是该空间成员")
}

// TestSpaceVisibilityQuizReset 刷题 reset 也须做项目级可见性校验（已修复）：
// POST /api/portal/quiz/:qid/reset 原先只走 resolveSpaceCtx（仅校验空间成员），
// 成员对不可见的刷题项目仍能重置会话（204）。修复后与其它刷题入口一致：不可见 → 404。
func TestSpaceVisibilityQuizReset(t *testing.T) {
	e := newSVEnvWithInvisibleQuiz(t)

	// 成员：不可见（未开放未分配）→ 404，且不泄露内容
	resp, out := doJSON(t, e.app, "POST", fmt.Sprintf("/api/portal/quiz/%d/reset", e.quiz), e.stuCookie, nil)
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("成员对不可见刷题项目 reset = %d %v, want 404", resp.StatusCode, out)
	}
	if msg, _ := out["error"].(string); msg != "刷题项目不存在" {
		t.Fatalf("reset 错误文案 = %v, want 刷题项目不存在", out["error"])
	}

	// 管理员：恒可见 → 204（与其它刷题入口口径一致）
	resp, out = doJSON(t, e.app, "POST", fmt.Sprintf("/api/portal/quiz/%d/reset", e.quiz), e.gAdminCookie, nil)
	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("管理员 reset = %d %v, want 204", resp.StatusCode, out)
	}

	// 补齐「开放 + 分配」后成员可重置 → 204
	e.setPublic(t, "quiz", e.quiz, true)
	e.setVisible(t, "quiz", e.quiz, []int64{e.stuID})
	resp, out = doJSON(t, e.app, "POST", fmt.Sprintf("/api/portal/quiz/%d/reset", e.quiz), e.stuCookie, nil)
	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("开放且分配后成员 reset = %d %v, want 204", resp.StatusCode, out)
	}
}

// newSVEnvWithInvisibleQuiz 轻量环境：只造一个「未开放未分配」的刷题项目（reset 缺口用）。
func newSVEnvWithInvisibleQuiz(t *testing.T) *svHTTPEnv {
	t.Helper()
	e := newSVHTTPEnv(t)
	e.setVisible(t, "quiz", e.quiz, nil)
	e.setPublic(t, "quiz", e.quiz, false)
	return e
}
