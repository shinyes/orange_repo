// 练习交卷记录「管理员可看全部成员」HTTP 层回归（真实临时 sqlite 库 + httptest）：
//
//	GET /api/portal/space/:id/practice/:pid/submissions[?scope=all]
//	GET /api/portal/space/:id/practice/:pid/submissions/:sid   （答题卡回看）
//
// 契约：
//   - ?scope=all 仅对 isAdminRole（系统/域管理员）生效：返回该练习全部成员的交卷记录
//     （id 倒序、每行带 userName），scope="all"；管理员不带 scope / scope=self → 只回本人且
//     scope="self"；
//   - 成员即便传 ?scope=all 也只返回本人（不报错、不越权），原始报文不含他人 userName；
//   - :sid 答题卡回看：成员只能读自己的（同练习内他人记录 → 404「提交记录不存在」），
//     管理员按「记录属于本练习」校验 → 可读任意成员答卷；别练习的记录 → 404「提交记录不存在」，
//     别空间的练习 id 走本空间路径 → 404「练习不存在」；
//   - 前置校验不因 scope 放宽：未登录 401；练习不可见（未开放 / 未分配）成员一律 404
//     「练习不存在」（管理员恒可见）；无记录时 submissions 必须是 []（不是 null）。
//
// 交卷记录经真实 POST .../submit 产生（快照格式与线上一致，答题卡回看才有逐题内容）；
// 复用 server_test.go / oj_submissions_scope_test.go / space_visibility_e2e_test.go 的
// doJSON、doJSONBody、subsOf、wantSubIDs、subOf、wantScope、fieldStr、fieldNum、
// rawSubmissions、loginStudent、jsonRaw 辅助，不另建同名函数。
package quizserver

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"orangeoj/internal/accounts"
	"orangeoj/internal/model"
	"orangeoj/internal/quizstore"
	"orangeoj/internal/store"
)

// psEnv 练习交卷记录回归环境：1 域 2 空间（主空间 6 个练习 + 另一空间 1 个），覆盖
// 「成员可见 / 未开放 / 未分配 / 无记录 / 跨练习 / 跨空间」各边界。
type psEnv struct {
	app *fiber.App
	qs  *quizstore.Store

	space      int64 // 主空间
	spaceOther int64 // 另一空间（跨空间 pid/sid 边界）
	problem    int64 // 主空间所在域的客观题（单选，正确项 index=1）

	practice      int64 // 开放 + 分配给 stu/mate/nostu：主用例（stu/mate/gAdmin 各交一卷）
	practiceOther int64 // 开放 + 分配给 stu：跨练习 sid 归属校验
	practiceEmpty int64 // 开放 + 分配给 stu：无人交卷（空数组边界）
	practiceOpen  int64 // 只开放（未分配）→ 成员不可见
	practiceGiven int64 // 只分配（未开放）→ 成员不可见
	practiceHide  int64 // 未开放 + 未分配 → 成员不可见（管理员仍可在其中交卷）
	practiceFar   int64 // 另一空间的练习 → 跨空间 pid 404

	stuID, mateID, gAdminID, dAdminID int64
	nostuID                           int64 // 可见该练习但从未交卷的成员（scope=all 必须为空）
	stuCook, mateCook, gAdminCook     string
	nostuCook, dAdminCook             string

	stuSub, mateSub, gAdminSub int64 // 主练习三条交卷（按此顺序写入 → id 递增）
	otherSub, hideSub, farSub  int64 // practiceOther / practiceHide / practiceFar 各一条
}

// psCreateObjectiveProblem 在指定域建一道单选客观题（正确项 index=1）。
func psCreateObjectiveProblem(t *testing.T, main *store.Store, domainID int64, title string) int64 {
	t.Helper()
	id, err := main.CreateProblem(model.Problem{
		Type: model.TypeSingleChoice, Title: title, StatementMD: "1+1=?",
		BodyJSON:   jsonRaw(`{"options":["1","2"]}`),
		AnswerJSON: jsonRaw(`{"answerIndex":1}`), Solutions: jsonRaw(`[]`),
	})
	if err != nil {
		t.Fatalf("create problem %s: %v", title, err)
	}
	if _, err := main.DB.Exec(`UPDATE problems SET domain_id=? WHERE id=?`, domainID, id); err != nil {
		t.Fatalf("set domain of problem %d: %v", id, err)
	}
	return id
}

// psCreatePractice 建练习并挂题（isPublic 即「开放」开关；「分配」名单由调用方另行设置）。
func psCreatePractice(t *testing.T, main *store.Store, spaceID int64, title string, isPublic bool, problemIDs ...int64) int64 {
	t.Helper()
	id, err := main.CreateSpacePractice(spaceID, title, "", nil, isPublic)
	if err != nil {
		t.Fatalf("create practice %s: %v", title, err)
	}
	if err := main.AddSpacePracticeItems(id, problemIDs); err != nil {
		t.Fatalf("add items to practice %d: %v", id, err)
	}
	return id
}

// psSetPracticeVisible 覆盖式设置练习可见名单（v2.5.0：可见 = 已开放 + 在名单内）。
func psSetPracticeVisible(t *testing.T, main *store.Store, practiceID int64, userIDs []int64) {
	t.Helper()
	if err := main.SetVisibleUsers("space_practice_visible", practiceID, userIDs); err != nil {
		t.Fatalf("set practice %d visible users: %v", practiceID, err)
	}
}

// newPSEnv 组装环境（三阶段建库范式同 games_test.go / space_visibility_e2e_test.go）：
// 主库建域/空间/题/练习 → quiz 库建账号 → 回主库写空间成员与练习可见名单 → 登录 + 造交卷。
func newPSEnv(t *testing.T) *psEnv {
	t.Helper()
	dir := t.TempDir()
	e := &psEnv{}

	// ---- 阶段 1：主库建域 / 两空间 / 题 / 练习 ----
	main, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open main store: %v", err)
	}
	domainID, err := main.CreateDomain("练习交卷域")
	if err != nil {
		t.Fatal(err)
	}
	if e.space, err = main.CreateSpace(domainID, "练习交卷班"); err != nil {
		t.Fatal(err)
	}
	if e.spaceOther, err = main.CreateSpace(domainID, "练习交卷二班"); err != nil {
		t.Fatal(err)
	}
	e.problem = psCreateObjectiveProblem(t, main, domainID, "练习交卷题")

	e.practice = psCreatePractice(t, main, e.space, "主练习", true, e.problem)
	e.practiceOther = psCreatePractice(t, main, e.space, "另一练习", true, e.problem)
	e.practiceEmpty = psCreatePractice(t, main, e.space, "空练习", true, e.problem)
	e.practiceOpen = psCreatePractice(t, main, e.space, "只开放练习", true, e.problem)
	e.practiceGiven = psCreatePractice(t, main, e.space, "只分配练习", false, e.problem)
	e.practiceHide = psCreatePractice(t, main, e.space, "未开放未分配练习", false, e.problem)
	e.practiceFar = psCreatePractice(t, main, e.spaceOther, "他班练习", true, e.problem)
	if err := main.Close(); err != nil {
		t.Fatal(err)
	}

	// ---- 阶段 2：quiz 库建账号（3 名成员 + 域管理员 + 系统管理员）----
	qs, err := quizstore.Open(dir)
	if err != nil {
		t.Fatalf("open quiz store: %v", err)
	}
	t.Cleanup(func() { _ = qs.Close() })
	e.qs = qs
	if e.stuID, err = qs.Accounts.CreateUser("psStu", "pw", accounts.RoleMember); err != nil {
		t.Fatal(err)
	}
	if e.mateID, err = qs.Accounts.CreateUser("psMate", "pw", accounts.RoleMember); err != nil {
		t.Fatal(err)
	}
	if e.nostuID, err = qs.Accounts.CreateUser("psNostu", "pw", accounts.RoleMember); err != nil {
		t.Fatal(err)
	}
	if e.gAdminID, err = qs.Accounts.CreateUser("psGAdmin", "pw", accounts.RoleGlobalAdmin); err != nil {
		t.Fatal(err)
	}
	if e.dAdminID, err = qs.Accounts.CreateUser("psDAdmin", "pw", accounts.RoleDomainAdmin, domainID); err != nil {
		t.Fatal(err)
	}

	// ---- 阶段 3：回主库写空间成员 + 练习可见名单（「开放 + 分配」两条件缺一不可）----
	main2, err := store.Open(dir)
	if err != nil {
		t.Fatalf("reopen main store: %v", err)
	}
	if err := main2.SetSpaceMembers(e.space, []int64{e.stuID, e.mateID, e.nostuID}); err != nil {
		t.Fatal(err)
	}
	psSetPracticeVisible(t, main2, e.practice, []int64{e.stuID, e.mateID, e.nostuID})
	psSetPracticeVisible(t, main2, e.practiceOther, []int64{e.stuID})
	psSetPracticeVisible(t, main2, e.practiceEmpty, []int64{e.stuID})
	psSetPracticeVisible(t, main2, e.practiceGiven, []int64{e.stuID}) // 只分配：未开放
	// practiceOpen 保持名单为空（只开放）；practiceHide / practiceFar 保持名单为空
	if err := main2.Close(); err != nil {
		t.Fatal(err)
	}

	// ---- 组装应用 + 登录 ----
	srv := &Server{QS: qs}
	e.app = New(srv, nil, 0)
	t.Cleanup(srv.StopQueue)
	e.stuCook = loginStudent(t, e.app, "psStu", "pw")
	e.mateCook = loginStudent(t, e.app, "psMate", "pw")
	e.nostuCook = loginStudent(t, e.app, "psNostu", "pw")
	e.gAdminCook = loginStudent(t, e.app, "psGAdmin", "pw")
	e.dAdminCook = loginStudent(t, e.app, "psDAdmin", "pw")

	// ---- 交卷夹具：主练习三条（stu → mate → gAdmin，id 递增，倒序断言有确定期望）----
	e.stuSub = e.submit(t, e.stuCook, e.space, e.practice)
	e.mateSub = e.submit(t, e.mateCook, e.space, e.practice)
	e.gAdminSub = e.submit(t, e.gAdminCook, e.space, e.practice)
	if !(e.stuSub < e.mateSub && e.mateSub < e.gAdminSub) {
		t.Fatalf("夹具异常：主练习交卷 id 未按写入顺序自增（%d,%d,%d）", e.stuSub, e.mateSub, e.gAdminSub)
	}
	e.otherSub = e.submit(t, e.stuCook, e.space, e.practiceOther)
	e.hideSub = e.submit(t, e.gAdminCook, e.space, e.practiceHide) // 管理员不受练习可见性限制
	e.farSub = e.submit(t, e.gAdminCook, e.spaceOther, e.practiceFar)
	return e
}

// submit 交卷一次（真实 POST .../submit），返回提交 id。
func (e *psEnv) submit(t *testing.T, cookie string, spaceID, pid int64) int64 {
	t.Helper()
	path := fmt.Sprintf("/api/portal/space/%d/practice/%d/submit", spaceID, pid)
	resp, out := doJSON(t, e.app, "POST", path, cookie,
		map[string]any{"answers": []map[string]any{{"problemId": e.problem, "answer": 1}}})
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("POST %s = %d %v，want 200", path, resp.StatusCode, out)
	}
	if correct := fieldNum(t, "交卷 "+path, out, "objectiveCorrect"); correct != 1 {
		t.Fatalf("交卷 %s objectiveCorrect = %d，want 1（夹具要求答对，答题卡断言才确定）", path, correct)
	}
	sid := fieldNum(t, "交卷 "+path, out, "submissionId")
	if sid <= 0 {
		t.Fatalf("交卷 %s submissionId = %d，want >0", path, sid)
	}
	return sid
}

// listPathIn 指定空间下的练习交卷记录列表路径（scope 非空时附 ?scope=）。
func (e *psEnv) listPathIn(spaceID, pid int64, scope string) string {
	p := fmt.Sprintf("/api/portal/space/%d/practice/%d/submissions", spaceID, pid)
	if scope != "" {
		p += "?scope=" + scope
	}
	return p
}

// listPath 本空间练习交卷记录列表路径。
func (e *psEnv) listPath(pid int64, scope string) string { return e.listPathIn(e.space, pid, scope) }

// getErr 任意路径 GET 并断言状态码（wantErr 非空时同时断言错误文案）。
func (e *psEnv) getErr(t *testing.T, cookie, path string, want int, wantErr string) map[string]any {
	t.Helper()
	resp, out := doJSON(t, e.app, "GET", path, cookie, nil)
	if resp.StatusCode != want {
		t.Fatalf("GET %s = %d %v，want %d %s", path, resp.StatusCode, out, want, wantErr)
	}
	if wantErr != "" {
		if got, _ := out["error"].(string); got != wantErr {
			t.Fatalf("GET %s error = %q，want %q（完整响应 %v）", path, got, wantErr, out)
		}
	}
	return out
}

// list 请求交卷记录列表并要求 200，返回解析体与原始报文（原始报文用于区分 [] 与 null、查泄露）。
func (e *psEnv) list(t *testing.T, cookie string, pid int64, scope string) (map[string]any, string) {
	t.Helper()
	path := e.listPath(pid, scope)
	resp, out, body := doJSONBody(t, e.app, "GET", path, cookie, nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("GET %s = %d %v，want 200", path, resp.StatusCode, out)
	}
	return out, body
}

// listErr 请求本空间练习交卷记录列表并要求指定错误码与文案（可见性 / 空间归属 404 用）。
func (e *psEnv) listErr(t *testing.T, cookie string, pid int64, scope string, want int, wantErr string) {
	t.Helper()
	e.getErr(t, cookie, e.listPath(pid, scope), want, wantErr)
}

// detail 请求答题卡回看并要求指定状态码（wantErr 非空时同时断言错误文案）。
func (e *psEnv) detail(t *testing.T, cookie string, spaceID, pid, sid int64, want int, wantErr string) map[string]any {
	t.Helper()
	return e.getErr(t, cookie, fmt.Sprintf("/api/portal/space/%d/practice/%d/submissions/%d", spaceID, pid, sid), want, wantErr)
}

// psFirstItem 取答题卡回看的首个条目（练习详情的 items 是扁平数组）。
func psFirstItem(t *testing.T, label string, detail map[string]any) map[string]any {
	t.Helper()
	items, ok := detail["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("%s items = %v（%T），want 至少 1 条", label, detail["items"], detail["items"])
	}
	item, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("%s items[0] 不是对象: %v", label, items[0])
	}
	return item
}

// TestPracticeSubmissionsUnauthorized 未登录：列表与答题卡回看均 401（会话中间件先于一切）。
func TestPracticeSubmissionsUnauthorized(t *testing.T) {
	e := newPSEnv(t)
	base := fmt.Sprintf("/api/portal/space/%d/practice/%d/submissions", e.space, e.practice)
	for _, path := range []string{base, base + fmt.Sprintf("/%d", e.stuSub)} {
		resp, out := doJSON(t, e.app, "GET", path, "", nil)
		if resp.StatusCode != fiber.StatusUnauthorized {
			t.Fatalf("未登录 GET %s = %d %v，want 401", path, resp.StatusCode, out)
		}
	}
}

// TestPracticeSubmissionsAdminScopeAllAcrossUsers 管理员 ?scope=all：
// 返回该练习全部成员的交卷记录（id 倒序），逐行 userId/userName 与提交者一致，scope="all"；
// 域管理员同口径；不带 scope（或 scope=self）只回本人那条且 scope="self"、不回带 userName。
func TestPracticeSubmissionsAdminScopeAllAcrossUsers(t *testing.T) {
	e := newPSEnv(t)
	wantAll := []int64{e.gAdminSub, e.mateSub, e.stuSub} // id 倒序
	wantRows := []struct {
		sid  int64
		uid  int64
		name string
	}{
		{e.gAdminSub, e.gAdminID, "psGAdmin"},
		{e.mateSub, e.mateID, "psMate"},
		{e.stuSub, e.stuID, "psStu"},
	}

	// ---- 系统管理员 scope=all ----
	const label = "系统管理员 scope=all"
	out, _ := e.list(t, e.gAdminCook, e.practice, "all")
	wantScope(t, label, out, "all")
	subs := subsOf(t, label, out)
	wantSubIDs(t, label+"（倒序、跨用户）", subs, wantAll...)
	for _, w := range wantRows {
		row := subOf(t, label, subs, w.sid)
		if got := fieldNum(t, label, row, "userId"); got != w.uid {
			t.Fatalf("%s 记录 %d userId = %d，want %d", label, w.sid, got, w.uid)
		}
		if got := fieldStr(t, label, row, "userName"); got != w.name {
			t.Fatalf("%s 记录 %d userName = %q，want %q", label, w.sid, got, w.name)
		}
		if got := fieldNum(t, label, row, "practiceId"); got != e.practice {
			t.Fatalf("%s 记录 %d practiceId = %d，want %d", label, w.sid, got, e.practice)
		}
	}

	// ---- 域管理员（本域）同口径 ----
	const dLabel = "域管理员 scope=all"
	outD, _ := e.list(t, e.dAdminCook, e.practice, "all")
	wantScope(t, dLabel, outD, "all")
	wantSubIDs(t, dLabel, subsOf(t, dLabel, outD), wantAll...)

	// ---- 大小写不敏感（handler 用 EqualFold）：ALL 对管理员等价 all ----
	const upLabel = "系统管理员 scope=ALL"
	outUp, _ := e.list(t, e.gAdminCook, e.practice, "ALL")
	wantScope(t, upLabel, outUp, "all")
	wantSubIDs(t, upLabel, subsOf(t, upLabel, outUp), wantAll...)

	// ---- 不带 scope / scope=self：只回本人 ----
	const selfLabel = "系统管理员 "
	for _, tc := range []struct{ name, scope string }{
		{"不带 scope", ""},
		{"scope=self", "self"},
	} {
		outSelf, _ := e.list(t, e.gAdminCook, e.practice, tc.scope)
		wantScope(t, selfLabel+tc.name, outSelf, "self")
		selfSubs := subsOf(t, selfLabel+tc.name, outSelf)
		wantSubIDs(t, selfLabel+tc.name+"（仅本人）", selfSubs, e.gAdminSub)
		row := subOf(t, selfLabel+tc.name, selfSubs, e.gAdminSub)
		if got := fieldNum(t, selfLabel+tc.name, row, "userId"); got != e.gAdminID {
			t.Fatalf("%s userId = %d，want %d（管理员默认视图也是本人）", selfLabel+tc.name, got, e.gAdminID)
		}
		if _, has := row["userName"]; has {
			t.Fatalf("%s 不得回带 userName: %v（本人视图无提交者字段）", selfLabel+tc.name, row)
		}
	}
}

// TestPracticeSubmissionsMemberScopeAllStaysSelf 成员传 ?scope=all 不越权：
// 只返回本人记录（同练习内其他成员的记录不出现），scope="self"，原始报文不含他人 userName；
// 大小写变体 ALL 同样不放大权限。
func TestPracticeSubmissionsMemberScopeAllStaysSelf(t *testing.T) {
	e := newPSEnv(t)
	for _, scope := range []string{"all", "ALL", "self"} {
		label := "成员 scope=" + scope
		out, body := e.list(t, e.stuCook, e.practice, scope)
		wantScope(t, label, out, "self")
		subs := subsOf(t, label, out)
		wantSubIDs(t, label+"（仅本人记录）", subs, e.stuSub)
		row := subOf(t, label, subs, e.stuSub)
		if got := fieldNum(t, label, row, "userId"); got != e.stuID {
			t.Fatalf("%s userId = %d，want %d", label, got, e.stuID)
		}
		if _, has := row["userName"]; has {
			t.Fatalf("%s 不得回带 userName: %v", label, row)
		}
		// 原始报文不得出现其他提交者的用户名（越权泄露的最直接证据）
		for _, name := range []string{"psMate", "psGAdmin", "psDAdmin", "psNostu"} {
			if strings.Contains(body, name) {
				t.Fatalf("%s 报文泄露了其他成员 %q: %s", label, name, body)
			}
		}
	}

	// 对照组：同样可见该练习但从未交卷的成员 —— scope=all 必须为空（不能因「本人无记录」
	// 而落到全部成员），且原始报文不含任何他人记录/用户名。
	const noneLabel = "无记录成员 scope=all"
	out, body := e.list(t, e.nostuCook, e.practice, "all")
	wantScope(t, noneLabel, out, "self")
	wantSubIDs(t, noneLabel+"（本人无记录）", subsOf(t, noneLabel, out))
	if raw := rawSubmissions(t, noneLabel, body); raw != "[]" {
		t.Fatalf("%s submissions 原文 = %s，want []（他人有记录时更须为空）", noneLabel, raw)
	}
	for _, name := range []string{"psStu", "psMate", "psGAdmin", "psDAdmin"} {
		if strings.Contains(body, name) {
			t.Fatalf("%s 报文泄露了其他成员 %q: %s", noneLabel, name, body)
		}
	}
}

// TestPracticeSubmissionDetailOwnershipAndAdmin 答题卡回看归属：
// 管理员按「记录属于本练习」可读任意成员答卷（200 + 逐题内容 + RFC3339 时间）；
// 成员读同练习内他人答卷 → 404「提交记录不存在」，读自己的 → 200。
func TestPracticeSubmissionDetailOwnershipAndAdmin(t *testing.T) {
	e := newPSEnv(t)

	// ---- 管理员读他人（mate）答卷 ----
	const label = "管理员读他人答卷"
	d := e.detail(t, e.gAdminCook, e.space, e.practice, e.mateSub, fiber.StatusOK, "")
	if got := fieldNum(t, label, d, "submissionId"); got != e.mateSub {
		t.Fatalf("%s submissionId = %d，want %d", label, got, e.mateSub)
	}
	if got := fieldNum(t, label, d, "practiceId"); got != e.practice {
		t.Fatalf("%s practiceId = %d，want %d", label, got, e.practice)
	}
	if got := fieldNum(t, label, d, "objectiveCorrect"); got != 1 {
		t.Fatalf("%s objectiveCorrect = %d，want 1（快照判定结果须随答卷回放）", label, got)
	}
	createdAt := fieldStr(t, label, d, "createdAt")
	ts, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		t.Fatalf("%s createdAt = %q 不是 RFC3339: %v", label, createdAt, err)
	}
	if delta := time.Since(ts); delta < -10*time.Minute || delta > 10*time.Minute {
		t.Fatalf("%s createdAt = %v 距当前 %v，want ≤10 分钟（时间格式/时区解析错位会差整数小时）", label, ts, delta)
	}
	item := psFirstItem(t, label, d)
	if got := fieldNum(t, label, item, "problemId"); got != e.problem {
		t.Fatalf("%s items[0].problemId = %d，want %d", label, got, e.problem)
	}
	if item["answered"] != true || item["correct"] != true {
		t.Fatalf("%s items[0] answered/correct = %v/%v，want true/true（答卷内容须正常回放）",
			label, item["answered"], item["correct"])
	}
	if got := fieldNum(t, label, item, "no"); got != 1 {
		t.Fatalf("%s items[0].no = %d，want 1", label, got)
	}

	// ---- 域管理员同口径（本域练习内的他人答卷）----
	if got := fieldNum(t, "域管理员读他人答卷", e.detail(t, e.dAdminCook, e.space, e.practice, e.mateSub,
		fiber.StatusOK, ""), "submissionId"); got != e.mateSub {
		t.Fatalf("域管理员读他人答卷 submissionId = %d，want %d", got, e.mateSub)
	}

	// ---- 成员读他人（mate / 管理员）答卷 → 404 ----
	for _, sid := range []int64{e.mateSub, e.gAdminSub} {
		e.detail(t, e.stuCook, e.space, e.practice, sid, fiber.StatusNotFound, "提交记录不存在")
	}
	// ---- 成员读自己的 → 200（答题卡内容正常）----
	dOwn := e.detail(t, e.stuCook, e.space, e.practice, e.stuSub, fiber.StatusOK, "")
	if got := fieldNum(t, "成员读自己答卷", dOwn, "submissionId"); got != e.stuSub {
		t.Fatalf("成员读自己答卷 submissionId = %d，want %d", got, e.stuSub)
	}
	if item := psFirstItem(t, "成员读自己答卷", dOwn); item["answered"] != true || item["correct"] != true {
		t.Fatalf("成员读自己答卷 items[0] = %v，want answered/correct 均 true", item)
	}
}

// TestPracticeSubmissionDetailRejectsForeignRecords 归属校验边界：
// 别练习的 sid 在本练习路径下 → 404「提交记录不存在」（同一 sid 在其所属练习下 200，
// 证明 404 来自归属校验而非记录不存在）；别空间的练习 id 走本空间路径 → 404「练习不存在」；
// 不存在的 sid → 404「提交记录不存在」。
func TestPracticeSubmissionDetailRejectsForeignRecords(t *testing.T) {
	e := newPSEnv(t)

	// ---- 同空间另一练习的 sid：本练习路径下管理员 404 ----
	const label = "管理员用别练习的 sid"
	e.detail(t, e.gAdminCook, e.space, e.practice, e.otherSub, fiber.StatusNotFound, "提交记录不存在")
	// 对照：同一 sid 在其所属练习路径下 200（管理员可读该练习内任意成员答卷）
	d := e.detail(t, e.gAdminCook, e.space, e.practiceOther, e.otherSub, fiber.StatusOK, "")
	if got := fieldNum(t, label+"（所属练习）", d, "practiceId"); got != e.practiceOther {
		t.Fatalf("%s practiceId = %d，want %d", label, got, e.practiceOther)
	}
	// 成员同样不能把别练习（即便是自己交的）的记录拿到本练习下读
	e.detail(t, e.stuCook, e.space, e.practice, e.otherSub, fiber.StatusNotFound, "提交记录不存在")
	if got := fieldNum(t, "成员读自己在另一练习的记录", e.detail(t, e.stuCook, e.space, e.practiceOther,
		e.otherSub, fiber.StatusOK, ""), "submissionId"); got != e.otherSub {
		t.Fatalf("成员读自己另一练习的记录 submissionId = %d，want %d", got, e.otherSub)
	}

	// ---- 别空间的练习 id 走本空间路径：前置空间归属先拦（404「练习不存在」）----
	const farLabel = "别空间练习 id 走本空间路径"
	e.detail(t, e.gAdminCook, e.space, e.practiceFar, e.farSub, fiber.StatusNotFound, "练习不存在")
	// 对照：在其所属空间 + 所属练习路径下 200
	dFar := e.detail(t, e.gAdminCook, e.spaceOther, e.practiceFar, e.farSub, fiber.StatusOK, "")
	if got := fieldNum(t, farLabel+"（所属空间）", dFar, "submissionId"); got != e.farSub {
		t.Fatalf("%s submissionId = %d，want %d", farLabel, got, e.farSub)
	}

	// ---- 不存在的 sid → 404 ----
	e.detail(t, e.gAdminCook, e.space, e.practice, 999999, fiber.StatusNotFound, "提交记录不存在")
}

// TestPracticeSubmissionsInvisiblePracticeStillHidden 练习不可见时成员一律 404
// 「练习不存在」（列表含 scope=all 试图绕过、以及答题卡回看都不过），管理员不受影响，
// 且管理员可读不可见练习里的他人答卷。
func TestPracticeSubmissionsInvisiblePracticeStillHidden(t *testing.T) {
	e := newPSEnv(t)
	for _, tc := range []struct {
		name string
		pid  int64
		want []int64 // 管理员 scope=all 期望记录（空 = 该练习无人交卷）
	}{
		{"未开放未分配", e.practiceHide, []int64{e.hideSub}}, // 管理员在此练习交了一卷
		{"只开放未分配", e.practiceOpen, nil},
		{"只分配未开放", e.practiceGiven, nil},
	} {
		// 成员：列表（含 scope=all）与回看均 404「练习不存在」
		e.listErr(t, e.stuCook, tc.pid, "", fiber.StatusNotFound, "练习不存在")
		e.listErr(t, e.stuCook, tc.pid, "all", fiber.StatusNotFound, "练习不存在")
		e.detail(t, e.stuCook, e.space, tc.pid, e.stuSub, fiber.StatusNotFound, "练习不存在")

		// 管理员：不受练习可见性影响（记录照常返回）
		out, _ := e.list(t, e.gAdminCook, tc.pid, "all")
		wantScope(t, tc.name+" 管理员 scope=all", out, "all")
		wantSubIDs(t, tc.name+" 管理员 scope=all", subsOf(t, tc.name+" 管理员 scope=all", out), tc.want...)
	}

	// 不可见练习内的他人答卷：管理员 200 可读；成员连练习都看不到（404「练习不存在」）
	d := e.detail(t, e.gAdminCook, e.space, e.practiceHide, e.hideSub, fiber.StatusOK, "")
	if got := fieldNum(t, "管理员读不可见练习的答卷", d, "submissionId"); got != e.hideSub {
		t.Fatalf("管理员读不可见练习的答卷 submissionId = %d，want %d", got, e.hideSub)
	}
	e.detail(t, e.stuCook, e.space, e.practiceHide, e.hideSub, fiber.StatusNotFound, "练习不存在")

	// 别空间的练习 id 走本空间的列表路径：scope=all 也不放宽前置空间归属（404「练习不存在」）
	e.getErr(t, e.gAdminCook, e.listPathIn(e.space, e.practiceFar, "all"), fiber.StatusNotFound, "练习不存在")
}

// TestPracticeSubmissionsEmptyIsArray 无交卷的练习：submissions 原始报文必须是 []
// （null 会让前端 map/filter 崩溃），管理员（scope=all / 默认）与成员（默认 / scope=all）同口径。
func TestPracticeSubmissionsEmptyIsArray(t *testing.T) {
	e := newPSEnv(t)
	for _, tc := range []struct {
		name, cookie, scope, wantScope string
	}{
		{"管理员 scope=all", e.gAdminCook, "all", "all"},
		{"管理员默认", e.gAdminCook, "", "self"},
		{"成员默认", e.stuCook, "", "self"},
		{"成员 scope=all", e.stuCook, "all", "self"},
	} {
		out, body := e.list(t, tc.cookie, e.practiceEmpty, tc.scope)
		wantScope(t, tc.name, out, tc.wantScope)
		if raw := rawSubmissions(t, tc.name, body); raw != "[]" {
			t.Fatalf("%s submissions 原文 = %s，want []（null 会让前端 map/filter 崩溃）", tc.name, raw)
		}
		if subs := subsOf(t, tc.name, out); len(subs) != 0 {
			t.Fatalf("%s submissions = %v，want 空数组", tc.name, subs)
		}
	}
}
