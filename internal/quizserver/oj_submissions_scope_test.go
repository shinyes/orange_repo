// GET /api/oj/problem/:id/submissions 的 HTTP 层回归（handler 权限与载荷）：
//
//   - 本人历史：只返回当前登录用户自己的提交，且**必须带 sourceCode**（列表查询曾漏选
//     source_code 列 → 测评记录详情「代码」页恒显示“无代码”），scope="self"；
//   - 管理端：?scope=all 仅对 isAdminRole（系统/域管理员）生效，返回全部成员提交并带
//     userId/userName，scope="all"；成员即便传 scope=all 也只返回本人（不报错、不越权）；
//   - 上下文过滤（trainingId/practiceId）在 self / all 两条路径上语义一致；
//   - 可见性门槛（problemVisibleToUser）先于 scope 判定：不可见题一律 404
//     「题目不存在或不可见」，不会被 scope=all 绕过；系统管理员的全库可见属既有豁免，
//     域管理员仍严格限本域；
//   - 无提交时必须是 []（不能是 null）。
//
// 全部走 HTTP 层真实请求（httptest + 真实临时 sqlite 库）；提交记录直接写库
// （quizstore.Store.CreateProgrammingSubmission），不依赖判题服务与工具链。
package quizserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"orangeoj/internal/accounts"
	"orangeoj/internal/judge"
	"orangeoj/internal/model"
	"orangeoj/internal/quizstore"
	"orangeoj/internal/store"
)

// ---------- 响应读取辅助（新增；不改动 server_test.go 既有 doJSON） ----------

// doJSONBody 同 doJSON，额外返回响应体原始 JSON 文本
// （用于严格区分 [] 与 null、以及断言某字段是否真的下发）。
func doJSONBody(t *testing.T, app *fiber.App, method, path, cookie string, body any) (*http.Response, map[string]any, string) {
	t.Helper()
	var req *http.Request
	if body != nil {
		b, _ := json.Marshal(body)
		req = httptest.NewRequest(method, path, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	raw := ""
	if resp.Body != nil {
		b, _ := io.ReadAll(resp.Body)
		raw = string(b)
	}
	var out map[string]any
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &out)
	}
	return resp, out, raw
}

// subsOf 取响应 submissions 字段（严格：必须是 JSON 数组——null 或缺失都在此失败）。
func subsOf(t *testing.T, label string, out map[string]any) []map[string]any {
	t.Helper()
	raw, ok := out["submissions"]
	if !ok {
		t.Fatalf("%s 响应缺 submissions 字段: %v", label, out)
	}
	arr, ok := raw.([]any)
	if !ok {
		t.Fatalf("%s submissions 不是 JSON 数组（null 或类型异常）= %v（%T）", label, raw, raw)
	}
	subs := make([]map[string]any, 0, len(arr))
	for _, it := range arr {
		m, ok := it.(map[string]any)
		if !ok {
			t.Fatalf("%s submissions 元素不是对象: %v", label, it)
		}
		subs = append(subs, m)
	}
	return subs
}

// wantSubIDs 断言提交 id 集合完全相等（顺序敏感：历史按 id 倒序）。
func wantSubIDs(t *testing.T, label string, subs []map[string]any, want ...int64) {
	t.Helper()
	got := make([]int64, 0, len(subs))
	for _, m := range subs {
		got = append(got, fieldNum(t, label, m, "id"))
	}
	if len(got) != len(want) {
		t.Fatalf("%s 提交条数 = %d %v，want %d %v", label, len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s 提交 id 序列 = %v，want %v（第 %d 项）", label, got, want, i)
		}
	}
}

// subOf 取指定 id 的提交条目（不存在即失败，避免"取到别的条目"掩盖越权）。
func subOf(t *testing.T, label string, subs []map[string]any, id int64) map[string]any {
	t.Helper()
	for _, m := range subs {
		if fieldNum(t, label, m, "id") == id {
			return m
		}
	}
	t.Fatalf("%s 未找到提交 %d（实得 %v）", label, id, subs)
	return nil
}

// wantScope 断言 scope 字段（"self" = 本人历史；"all" = 管理端全部成员）。
func wantScope(t *testing.T, label string, out map[string]any, want string) {
	t.Helper()
	if got := fieldStr(t, label, out, "scope"); got != want {
		t.Fatalf("%s scope = %q，want %q（完整响应 %v）", label, got, want, out)
	}
}

// fieldStr 严格读取字符串字段（缺失或类型不符即失败）。
func fieldStr(t *testing.T, label string, m map[string]any, key string) string {
	t.Helper()
	v, ok := m[key]
	if !ok {
		t.Fatalf("%s 缺字段 %s: %v", label, key, m)
	}
	s, ok := v.(string)
	if !ok {
		t.Fatalf("%s 字段 %s = %v（%T），期望字符串", label, key, v, v)
	}
	return s
}

// fieldNum 严格读取数字字段（JSON 数字解码为 float64）。
func fieldNum(t *testing.T, label string, m map[string]any, key string) int64 {
	t.Helper()
	v, ok := m[key]
	if !ok {
		t.Fatalf("%s 缺字段 %s: %v", label, key, m)
	}
	f, ok := v.(float64)
	if !ok {
		t.Fatalf("%s 字段 %s = %v（%T），期望数字", label, key, v, v)
	}
	return int64(f)
}

// rawSubmissions 从原始报文取 submissions 的字面 JSON（区分 [] 与 null）。
func rawSubmissions(t *testing.T, label, body string) string {
	t.Helper()
	var probe struct {
		Submissions json.RawMessage `json:"submissions"`
	}
	if err := json.Unmarshal([]byte(body), &probe); err != nil {
		t.Fatalf("%s 响应不是 JSON: %v（%s）", label, err, body)
	}
	if probe.Submissions == nil {
		t.Fatalf("%s 响应缺 submissions 字段: %s", label, body)
	}
	return string(probe.Submissions)
}

// ---------- 测试环境（域A空间A：两名成员；域B空间B：异域成员/异域题） ----------

// ojSubEnv 提交历史回归环境。
type ojSubEnv struct {
	app *fiber.App
	qs  *quizstore.Store

	problem      int64 // 域A 编程题：被测提交所在题
	emptyProblem int64 // 域A 编程题：无任何提交（空数组边界）
	foreignProb  int64 // 域B 编程题：域A 用户不可见
	training     int64 // 域A 空间A 训练（含 problem）
	practice     int64 // 域A 空间A 练习（含 problem）

	stuA, stuB int64 // 域A 空间A 成员
	stuF       int64 // 域B 空间B 成员（给异域题造提交）
	dAdminA    int64 // 域A 域管理员
	dAdminB    int64 // 域B 域管理员
	gAdmin     int64 // 系统管理员

	stuACook, stuBCook                   string
	dAdminACook, dAdminBCook, gAdminCook string
}

// ojSubProblem 在指定域建一道编程题（题面/判题密钥齐全；本文件只用其 id）。
func ojSubProblem(t *testing.T, main *store.Store, domainID int64, title string) int64 {
	t.Helper()
	id, err := main.CreateProblem(model.Problem{
		Type: model.TypeProgramming, Title: title, Tags: []string{"回归"},
		StatementMD: title + "：读入两个整数输出和。",
		BodyJSON:    jsonRaw(`{"inputFormat":"一行两个整数","outputFormat":"一个整数","samples":[{"input":"1 2","output":"3"}],"testCases":[{"input":"1 2","output":"3"}]}`),
		AnswerJSON:  jsonRaw(`{}`), Solutions: jsonRaw(`[]`),
		TimeLimitMS: 2000, MemoryLimitMiB: 256,
	})
	if err != nil {
		t.Fatalf("create problem %s: %v", title, err)
	}
	if _, err := main.DB.Exec(`UPDATE problems SET domain_id=? WHERE id=?`, domainID, id); err != nil {
		t.Fatalf("set domain of problem %d: %v", id, err)
	}
	return id
}

// newOJSubEnv 组装环境（复用 portal_integration_test.go 的三阶段建库范式）。
func newOJSubEnv(t *testing.T) *ojSubEnv {
	t.Helper()
	dir := t.TempDir()
	env := &ojSubEnv{}

	// ---- 阶段 1：主库建域/空间/题/训练/练习 ----
	main, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open main store: %v", err)
	}
	domainA, err := main.CreateDomain("提交回归域A")
	if err != nil {
		t.Fatal(err)
	}
	spaceA, err := main.CreateSpace(domainA, "提交回归班A")
	if err != nil {
		t.Fatal(err)
	}
	domainB, err := main.CreateDomain("提交回归域B")
	if err != nil {
		t.Fatal(err)
	}
	spaceB, err := main.CreateSpace(domainB, "提交回归班B")
	if err != nil {
		t.Fatal(err)
	}
	env.problem = ojSubProblem(t, main, domainA, "A+B")
	env.emptyProblem = ojSubProblem(t, main, domainA, "无人提交题")
	env.foreignProb = ojSubProblem(t, main, domainB, "异域题")

	// 训练/练习（含 problem）：trainingId/practiceId 用真实项目 id，贴近线上上下文
	env.training, err = main.CreateSpaceTraining(spaceA, "提交回归训练", "", nil, 0, true)
	if err != nil {
		t.Fatalf("create training: %v", err)
	}
	ch, err := main.CreateSpaceChapter(env.training, "第一章")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := main.AddSpaceChapterItems(ch, []int64{env.problem}); err != nil {
		t.Fatal(err)
	}
	env.practice, err = main.CreateSpacePractice(spaceA, "提交回归练习", "", nil, true)
	if err != nil {
		t.Fatalf("create practice: %v", err)
	}
	if err := main.AddSpacePracticeItems(env.practice, []int64{env.problem}); err != nil {
		t.Fatal(err)
	}
	if err := main.Close(); err != nil {
		t.Fatalf("close main store: %v", err)
	}

	// ---- 阶段 2：quiz 库（单库）建账号 ----
	qs, err := quizstore.Open(dir)
	if err != nil {
		t.Fatalf("open quiz store: %v", err)
	}
	t.Cleanup(func() { _ = qs.Close() })
	env.qs = qs
	if env.stuA, err = qs.Accounts.CreateUser("ojsubStuA", "pw", accounts.RoleMember); err != nil {
		t.Fatal(err)
	}
	if env.stuB, err = qs.Accounts.CreateUser("ojsubStuB", "pw", accounts.RoleMember); err != nil {
		t.Fatal(err)
	}
	if env.stuF, err = qs.Accounts.CreateUser("ojsubStuF", "pw", accounts.RoleMember); err != nil {
		t.Fatal(err)
	}
	if env.dAdminA, err = qs.Accounts.CreateUser("ojsubDAdminA", "pw", accounts.RoleDomainAdmin, domainA); err != nil {
		t.Fatal(err)
	}
	if env.dAdminB, err = qs.Accounts.CreateUser("ojsubDAdminB", "pw", accounts.RoleDomainAdmin, domainB); err != nil {
		t.Fatal(err)
	}
	if env.gAdmin, err = qs.Accounts.CreateUser("ojsubGAdmin", "pw", accounts.RoleGlobalAdmin); err != nil {
		t.Fatal(err)
	}

	// ---- 阶段 3：回主库写空间成员（成员可见性 = 加入空间所在域含该题） ----
	main2, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = main2.Close() })
	if err := main2.SetSpaceMembers(spaceA, []int64{env.stuA, env.stuB}); err != nil {
		t.Fatal(err)
	}
	if err := main2.SetSpaceMembers(spaceB, []int64{env.stuF}); err != nil {
		t.Fatal(err)
	}

	// 判题服务不参与本文件：提交记录直接写库（runner=nil）
	srv := &Server{QS: qs}
	env.app = New(srv, nil, 0)
	t.Cleanup(srv.StopQueue)
	env.stuACook = loginStudent(t, env.app, "ojsubStuA", "pw")
	env.stuBCook = loginStudent(t, env.app, "ojsubStuB", "pw")
	env.dAdminACook = loginStudent(t, env.app, "ojsubDAdminA", "pw")
	env.dAdminBCook = loginStudent(t, env.app, "ojsubDAdminB", "pw")
	env.gAdminCook = loginStudent(t, env.app, "ojsubGAdmin", "pw")
	return env
}

// submit 直接写一条编程提交记录（绕过判题服务），返回提交 id。
func (e *ojSubEnv) submit(t *testing.T, userID, problemID, trainingID, practiceID int64, code string) int64 {
	t.Helper()
	id, err := e.qs.CreateProgrammingSubmission(userID, problemID, trainingID, practiceID,
		"programming", "python", code, "", judge.SubmitTypeSubmit)
	if err != nil {
		t.Fatalf("写提交记录(user=%d problem=%d training=%d practice=%d): %v",
			userID, problemID, trainingID, practiceID, err)
	}
	return id
}

// get 发 GET 并断言状态码，返回响应体与原始文本。
func (e *ojSubEnv) get(t *testing.T, cookie, path string, want int) (map[string]any, string) {
	t.Helper()
	resp, out, body := doJSONBody(t, e.app, "GET", path, cookie, nil)
	if resp.StatusCode != want {
		t.Fatalf("GET %s = %d %v，want %d", path, resp.StatusCode, out, want)
	}
	return out, body
}

// historyGet 请求提交历史（query 可空，不带前导 ?）。
func (e *ojSubEnv) historyGet(t *testing.T, cookie string, problemID int64, query string, want int) (map[string]any, string) {
	t.Helper()
	path := fmt.Sprintf("/api/oj/problem/%d/submissions", problemID)
	if query != "" {
		path += "?" + query
	}
	return e.get(t, cookie, path, want)
}

// ---------- 1. 本人历史带源码 ----------

// TestOJSubmissionsSelfScopeCarriesSourceCode 成员本人历史：只返回自己的提交、
// 带 sourceCode、scope="self"，且不回带 userId/userName。
func TestOJSubmissionsSelfScopeCarriesSourceCode(t *testing.T) {
	e := newOJSubEnv(t)
	const codeA = "a, b = map(int, input().split())\nprint(a + b)  # self-scope\n"
	const codeB = "print('B 的代码不应出现在 A 的历史里')\n"
	mine := e.submit(t, e.stuA, e.problem, 0, 0, codeA)
	other := e.submit(t, e.stuB, e.problem, 0, 0, codeB)

	out, _ := e.historyGet(t, e.stuACook, e.problem, "", fiber.StatusOK)
	wantScope(t, "成员A本人历史", out, "self")
	subs := subsOf(t, "成员A本人历史", out)
	wantSubIDs(t, "成员A本人历史（仅自己、不含他人）", subs, mine)
	if other == mine {
		t.Fatalf("夹具异常：两次提交 id 相同(%d)", mine)
	}

	got := subOf(t, "成员A本人历史", subs, mine)
	if sc := fieldStr(t, "成员A本人历史", got, "sourceCode"); sc != codeA {
		t.Fatalf("sourceCode = %q，want %q（列表漏选 source_code 会让测评记录详情显示“无代码”）", sc, codeA)
	}
	if pid := fieldNum(t, "成员A本人历史", got, "problemId"); pid != e.problem {
		t.Fatalf("problemId = %d，want %d", pid, e.problem)
	}
	if lang := fieldStr(t, "成员A本人历史", got, "language"); lang != "python" {
		t.Fatalf("language = %q，want python", lang)
	}
	// 本人历史不得回带提交者身份字段（omitempty 应使其缺省）
	for _, key := range []string{"userId", "userName"} {
		if _, has := got[key]; has {
			t.Fatalf("本人历史不得回带 %s: %v", key, got)
		}
	}

	// 对照：B 的本人历史不含 A 的那条
	outB, _ := e.historyGet(t, e.stuBCook, e.problem, "", fiber.StatusOK)
	wantScope(t, "成员B本人历史", outB, "self")
	wantSubIDs(t, "成员B本人历史", subsOf(t, "成员B本人历史", outB), other)
}

// ---------- 2. 管理员 scope=all 可跨用户 ----------

// TestOJSubmissionsAdminScopeAllAcrossUsers 域管理员/系统管理员 ?scope=all&trainingId=N：
// 同时返回同域两名成员的提交，每条带 userName/userId 与 sourceCode，scope="all"。
func TestOJSubmissionsAdminScopeAllAcrossUsers(t *testing.T) {
	e := newOJSubEnv(t)
	const codeA = "print('admin-all: A')\n"
	const codeB = "print('admin-all: B')\n"
	subA := e.submit(t, e.stuA, e.problem, e.training, 0, codeA)
	subB := e.submit(t, e.stuB, e.problem, e.training, 0, codeB)
	if subB <= subA {
		t.Fatalf("夹具异常：提交 id 未按写入顺序自增 (A=%d, B=%d)", subA, subB)
	}

	type want struct {
		id       int64
		userID   int64
		userName string
		code     string
	}
	wants := []want{ // 顺序 = 历史返回顺序（id 倒序）
		{id: subB, userID: e.stuB, userName: "ojsubStuB", code: codeB},
		{id: subA, userID: e.stuA, userName: "ojsubStuA", code: codeA},
	}
	query := fmt.Sprintf("scope=all&trainingId=%d", e.training)

	admins := []struct{ label, cookie string }{
		{"域管理员(域A) ?scope=all", e.dAdminACook},
		{"系统管理员 ?scope=all", e.gAdminCook},
	}
	for _, a := range admins {
		out, _ := e.historyGet(t, a.cookie, e.problem, query, fiber.StatusOK)
		wantScope(t, a.label, out, "all")
		subs := subsOf(t, a.label, out)
		ids := make([]int64, 0, len(wants))
		for _, w := range wants {
			ids = append(ids, w.id)
		}
		wantSubIDs(t, a.label, subs, ids...)
		for _, w := range wants {
			got := subOf(t, a.label, subs, w.id)
			if name := fieldStr(t, a.label, got, "userName"); name != w.userName {
				t.Fatalf("%s 提交 %d userName = %q，want %q", a.label, w.id, name, w.userName)
			}
			if uid := fieldNum(t, a.label, got, "userId"); uid != w.userID {
				t.Fatalf("%s 提交 %d userId = %d，want %d", a.label, w.id, uid, w.userID)
			}
			if sc := fieldStr(t, a.label, got, "sourceCode"); sc != w.code {
				t.Fatalf("%s 提交 %d sourceCode = %q，want %q", a.label, w.id, sc, w.code)
			}
		}
	}

	// 大小写变体（handler 用 EqualFold 匹配 scope）对管理员同样放行
	label := "域管理员(域A) ?scope=All"
	out, _ := e.historyGet(t, e.dAdminACook, e.problem, fmt.Sprintf("scope=All&trainingId=%d", e.training), fiber.StatusOK)
	wantScope(t, label, out, "all")
	wantSubIDs(t, label, subsOf(t, label, out), subB, subA)
}

// ---------- 3. 成员传 scope=all 不越权 ----------

// TestOJSubmissionsMemberScopeAllStaysSelf 成员 A 传 scope=all（含带 trainingId 的形式、
// 以及大小写变体 scope=ALL——handler 用 EqualFold 匹配）仍只返回自己：
// scope="self"、不含 B 的提交、报文里不出现 userName/userId。
func TestOJSubmissionsMemberScopeAllStaysSelf(t *testing.T) {
	e := newOJSubEnv(t)
	const codeA = "print('member A only')\n"
	subA := e.submit(t, e.stuA, e.problem, e.training, 0, codeA)
	subB := e.submit(t, e.stuB, e.problem, e.training, 0, "print('member B only')\n")
	if subB <= subA {
		t.Fatalf("夹具异常：提交 id 未按写入顺序自增 (A=%d, B=%d)", subA, subB)
	}

	queries := []string{
		"scope=all",
		fmt.Sprintf("scope=all&trainingId=%d", e.training),
		fmt.Sprintf("scope=ALL&trainingId=%d", e.training), // 大小写变体（handler 用 EqualFold）同样不得越权
	}
	for _, q := range queries {
		label := "成员A ?" + q
		out, body := e.historyGet(t, e.stuACook, e.problem, q, fiber.StatusOK)
		wantScope(t, label, out, "self")
		subs := subsOf(t, label, out)
		wantSubIDs(t, label+"（只返回本人，不含 B）", subs, subA)
		if sc := fieldStr(t, label, subOf(t, label, subs, subA), "sourceCode"); sc != codeA {
			t.Fatalf("%s sourceCode = %q，want %q", label, sc, codeA)
		}
		// 不得以任何形式泄露他人提交者身份（self 路径不带 userId/userName）
		for _, key := range []string{`"userName"`, `"userId"`} {
			if strings.Contains(body, key) {
				t.Fatalf("%s 响应含 %s（成员传 scope=all 不得看到他人身份）: %s", label, key, body)
			}
		}
		for _, m := range subs {
			if fieldNum(t, label, m, "id") == subB {
				t.Fatalf("%s 返回了他人提交 %d: %v", label, subB, m)
			}
		}
	}
}

// ---------- 4. 上下文过滤仍然生效 ----------

// TestOJSubmissionsAllScopeTrainingContextFilter 管理员 ?scope=all&trainingId=N 只返回训练内提交；
// 不带 trainingId 时训练内与非上下文两条都在；self 路径过滤语义一致。
func TestOJSubmissionsAllScopeTrainingContextFilter(t *testing.T) {
	e := newOJSubEnv(t)
	const ctxCode = "print('in training')\n"
	const freeCode = "print('no context')\n"
	inTraining := e.submit(t, e.stuA, e.problem, e.training, 0, ctxCode)
	noCtx := e.submit(t, e.stuB, e.problem, 0, 0, freeCode)

	// 带训练上下文：只返回训练内那条（且仍跨用户）
	label := "管理员 ?scope=all&trainingId"
	out, _ := e.historyGet(t, e.dAdminACook, e.problem, fmt.Sprintf("scope=all&trainingId=%d", e.training), fiber.StatusOK)
	wantScope(t, label, out, "all")
	subs := subsOf(t, label, out)
	wantSubIDs(t, label, subs, inTraining)
	if sc := fieldStr(t, label, subOf(t, label, subs, inTraining), "sourceCode"); sc != ctxCode {
		t.Fatalf("%s sourceCode = %q，want %q", label, sc, ctxCode)
	}
	if name := fieldStr(t, label, subOf(t, label, subs, inTraining), "userName"); name != "ojsubStuA" {
		t.Fatalf("%s userName = %q，want ojsubStuA", label, name)
	}

	// 不带上下文：两条都在（倒序）
	label = "管理员 ?scope=all（无上下文）"
	out, _ = e.historyGet(t, e.dAdminACook, e.problem, "scope=all", fiber.StatusOK)
	wantScope(t, label, out, "all")
	wantSubIDs(t, label, subsOf(t, label, out), noCtx, inTraining)

	// 对照：成员 self 路径的上下文过滤未被改动
	label = "成员A ?trainingId（self）"
	out, _ = e.historyGet(t, e.stuACook, e.problem, fmt.Sprintf("trainingId=%d", e.training), fiber.StatusOK)
	wantScope(t, label, out, "self")
	wantSubIDs(t, label, subsOf(t, label, out), inTraining)
	// self 路径仍按用户过滤：B 的非上下文提交不会因"无上下文"而出现在 A 的历史里
	label = "成员A 无上下文（self，仅本人）"
	out, _ = e.historyGet(t, e.stuACook, e.problem, "", fiber.StatusOK)
	wantScope(t, label, out, "self")
	wantSubIDs(t, label, subsOf(t, label, out), inTraining)
}

// TestOJSubmissionsAllScopePracticeContextFilter 练习上下文（practiceId）在 all 路径同样生效。
func TestOJSubmissionsAllScopePracticeContextFilter(t *testing.T) {
	e := newOJSubEnv(t)
	inPractice := e.submit(t, e.stuA, e.problem, 0, e.practice, "print('in practice')\n")
	inTraining := e.submit(t, e.stuB, e.problem, e.training, 0, "print('in training')\n")

	label := "管理员 ?scope=all&practiceId"
	out, _ := e.historyGet(t, e.dAdminACook, e.problem, fmt.Sprintf("scope=all&practiceId=%d", e.practice), fiber.StatusOK)
	wantScope(t, label, out, "all")
	wantSubIDs(t, label, subsOf(t, label, out), inPractice)

	label = "管理员 ?scope=all（无上下文）"
	out, _ = e.historyGet(t, e.dAdminACook, e.problem, "scope=all", fiber.StatusOK)
	wantSubIDs(t, label, subsOf(t, label, out), inTraining, inPractice)
}

// ---------- 5. 可见性门槛未被绕过 ----------

// TestOJSubmissionsAllScopeRespectsVisibility 异域题目：成员与域管理员（非本域）无论是否
// 传 scope=all 都 404「题目不存在或不可见」，且不泄露他域提交；系统管理员的全库可见
// 是 problemVisibleToUser 的既有豁免（非 scope=all 引入），本域域管理员仍可见。
func TestOJSubmissionsAllScopeRespectsVisibility(t *testing.T) {
	e := newOJSubEnv(t)
	foreignSub := e.submit(t, e.stuF, e.foreignProb, 0, 0, "print('foreign domain code')\n")

	// 成员（域A空间成员）看域B题目：不带 scope 与带 scope=all 均 404
	for _, q := range []string{"", "scope=all"} {
		label := "成员A 异域题 ?" + q
		out, body := e.historyGet(t, e.stuACook, e.foreignProb, q, fiber.StatusNotFound)
		if msg, _ := out["error"].(string); msg != "题目不存在或不可见" {
			t.Fatalf("%s error = %v，want 题目不存在或不可见（完整响应 %v）", label, out["error"], out)
		}
		if _, has := out["submissions"]; has {
			t.Fatalf("%s 404 响应不得带 submissions（泄露他域提交）: %s", label, body)
		}
		if strings.Contains(body, "foreign domain code") {
			t.Fatalf("%s 404 响应泄露他域源码: %s", label, body)
		}
	}

	// 域管理员（域A）看域B题目：scope=all 不豁免域范围 → 404
	label := "域管理员(域A) 异域题 ?scope=all"
	out, body := e.historyGet(t, e.dAdminACook, e.foreignProb, "scope=all", fiber.StatusNotFound)
	if msg, _ := out["error"].(string); msg != "题目不存在或不可见" {
		t.Fatalf("%s error = %v，want 题目不存在或不可见（完整响应 %v）", label, out["error"], out)
	}
	if strings.Contains(body, "foreign domain code") {
		t.Fatalf("%s 404 响应泄露他域源码: %s", label, body)
	}

	// 对称：域管理员（域B）看域A题目 → 404（证明 404 来自按域判定，而非 scope=all 失效）
	e.submit(t, e.stuA, e.problem, 0, 0, "print('domain A code')\n")
	label = "域管理员(域B) 域A题 ?scope=all"
	out, _ = e.historyGet(t, e.dAdminBCook, e.problem, "scope=all", fiber.StatusNotFound)
	if msg, _ := out["error"].(string); msg != "题目不存在或不可见" {
		t.Fatalf("%s error = %v，want 题目不存在或不可见（完整响应 %v）", label, out["error"], out)
	}

	// 域管理员（域B）看本域题目 → 200 且 scope=all 能看到本域成员提交
	label = "域管理员(域B) 本域题 ?scope=all"
	out, _ = e.historyGet(t, e.dAdminBCook, e.foreignProb, "scope=all", fiber.StatusOK)
	wantScope(t, label, out, "all")
	subs := subsOf(t, label, out)
	wantSubIDs(t, label, subs, foreignSub)
	if name := fieldStr(t, label, subOf(t, label, subs, foreignSub), "userName"); name != "ojsubStuF" {
		t.Fatalf("%s userName = %q，want ojsubStuF", label, name)
	}

	// 系统管理员：problemVisibleToUser 的既有语义=全库题目可见（非 scope=all 带来的豁免）
	label = "系统管理员 异域题 ?scope=all"
	out, _ = e.historyGet(t, e.gAdminCook, e.foreignProb, "scope=all", fiber.StatusOK)
	wantScope(t, label, out, "all")
	wantSubIDs(t, label, subsOf(t, label, out), foreignSub)
}

// ---------- 6. 空历史必须是空数组 ----------

// TestOJSubmissionsAllScopeEmptyArray 无任何提交时：管理员 ?scope=all → 200、
// submissions 为 []（原始报文不得是 null）、scope="all"；成员本人空历史同样 []。
func TestOJSubmissionsAllScopeEmptyArray(t *testing.T) {
	e := newOJSubEnv(t)

	for _, tc := range []struct {
		label, cookie, query, scope string
	}{
		{"管理员 ?scope=all 空历史", e.dAdminACook, "scope=all", "all"},
		{"系统管理员 ?scope=all 空历史", e.gAdminCook, "scope=all", "all"},
		{"成员A 本人空历史", e.stuACook, "", "self"},
	} {
		out, body := e.historyGet(t, tc.cookie, e.emptyProblem, tc.query, fiber.StatusOK)
		wantScope(t, tc.label, out, tc.scope)
		if subs := subsOf(t, tc.label, out); len(subs) != 0 {
			t.Fatalf("%s 条数 = %d，want 0（%v）", tc.label, len(subs), subs)
		}
		if raw := rawSubmissions(t, tc.label, body); raw != "[]" {
			t.Fatalf("%s submissions 原始值 = %s，want []（空数组不能变 null）: %s", tc.label, raw, body)
		}
	}
}
