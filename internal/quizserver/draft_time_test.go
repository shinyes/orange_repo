// 云端代码草稿 HTTP 契约回归（internal/quizserver/judge.go 的 draft 端点）。
//
// 契约（前端多端合并依赖）：
//   - GET /api/oj/problem/:id/draft?lang=python|cpp[&ctxKind=&ctxId=]
//     → 200 {code, language, updatedAt}；无草稿时 code 与 updatedAt 均为空串（键必须存在）
//   - updatedAt 必须是「带时区的 RFC3339」（UTC → Z 后缀）。前端把它解析为毫秒时间戳，
//     与本机 localStorage 记录的草稿写入时间比较，仅当云端严格更新时才覆盖本地；
//     一旦退化成无时区的裸格式（如 SQLite "2006-01-02 15:04:05"），前端按本地时区解析
//     会差若干小时、判断反向 → 用旧草稿覆盖新草稿。这是本文件的核心回归点。
//   - PUT 保存 → 204；既有校验（非法 lang/上下文 400、不可见题 404）不回退。
package quizserver

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"orangeoj/internal/model"
	"orangeoj/internal/quizstore"
	"orangeoj/internal/store"
)

// rfc3339ZoneRe 显式时区后缀：Z 或 ±HH:MM。
var rfc3339ZoneRe = regexp.MustCompile(`(Z|[+-]\d{2}:\d{2})$`)

// newDraftTestApp 建最小环境：主库 1 域 1 空间（域内 1 编程题）+ 异域 1 题，
// 空间成员 drafter。草稿端点不判题，故不挂 runner（队列不启动）。
// ids：problem/foreign/space/user。
func newDraftTestApp(t *testing.T) (*fiber.App, *quizstore.Store, map[string]int64) {
	t.Helper()
	dir := t.TempDir()
	ids := map[string]int64{}

	main, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open main store: %v", err)
	}
	domainID, err := main.CreateDomain("草稿域")
	if err != nil {
		t.Fatal(err)
	}
	spaceID, err := main.CreateSpace(domainID, "草稿班")
	if err != nil {
		t.Fatal(err)
	}
	foreignDomainID, err := main.CreateDomain("异域")
	if err != nil {
		t.Fatal(err)
	}
	// 域内编程题（草稿端点要求题目对该用户可见）
	problemID, err := main.CreateProblem(model.Problem{
		Type: model.TypeProgramming, Title: "A+B", StatementMD: "读入两个整数输出和。",
		BodyJSON:   jsonRaw(`{"samples":[{"input":"1 2","output":"3"}]}`),
		AnswerJSON: jsonRaw(`{}`), Solutions: jsonRaw(`[]`), TimeLimitMS: 2000, MemoryLimitMiB: 256,
	})
	if err != nil {
		t.Fatalf("seed problem: %v", err)
	}
	if _, err := main.DB.Exec(`UPDATE problems SET domain_id=? WHERE id=?`, domainID, problemID); err != nil {
		t.Fatal(err)
	}
	foreignID, err := main.CreateProblem(model.Problem{
		Type: model.TypeProgramming, Title: "异域题", StatementMD: "异域：求和。",
		BodyJSON:   jsonRaw(`{"samples":[]}`),
		AnswerJSON: jsonRaw(`{}`), Solutions: jsonRaw(`[]`),
	})
	if err != nil {
		t.Fatalf("seed foreign problem: %v", err)
	}
	if _, err := main.DB.Exec(`UPDATE problems SET domain_id=? WHERE id=?`, foreignDomainID, foreignID); err != nil {
		t.Fatal(err)
	}
	ids["problem"] = problemID
	ids["foreign"] = foreignID
	ids["space"] = spaceID
	if err := main.Close(); err != nil {
		t.Fatal(err)
	}

	qs, err := quizstore.Open(dir)
	if err != nil {
		t.Fatalf("open quiz store: %v", err)
	}
	t.Cleanup(func() { _ = qs.Close() })
	userID, err := qs.Accounts.CreateUser("drafter", "pw", "member")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	ids["user"] = userID

	// 回主库登记空间成员（可见性 = 用户加入的空间所在域包含该题）
	main2, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := main2.SetSpaceMembers(spaceID, []int64{userID}); err != nil {
		t.Fatal(err)
	}
	if err := main2.Close(); err != nil {
		t.Fatal(err)
	}

	srv := &Server{QS: qs, UploadsDir: filepath.Join(dir, "uploads")}
	app := New(srv, nil, 0)
	t.Cleanup(srv.StopQueue)
	return app, qs, ids
}

// draftResp GET 草稿响应：三字段均为字符串（缺字段即契约破坏，故解析失败直接 Fatal）。
type draftResp struct {
	raw       map[string]any
	code      string
	language  string
	updatedAt string
}

// getDraft GET 草稿端点，要求 200 且 code/language/updatedAt 三字段齐备。
func getDraft(t *testing.T, app *fiber.App, cookie string, problemID int64, query string) draftResp {
	t.Helper()
	path := fmt.Sprintf("/api/oj/problem/%d/draft", problemID)
	if query != "" {
		path += "?" + query
	}
	resp, out := doJSON(t, app, "GET", path, cookie, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("GET %s = %d %v, want 200", path, resp.StatusCode, out)
	}
	r := draftResp{raw: out}
	var ok bool
	if r.code, ok = out["code"].(string); !ok {
		t.Fatalf("GET %s 缺 code 字符串字段: %v", path, out)
	}
	if r.language, ok = out["language"].(string); !ok {
		t.Fatalf("GET %s 缺 language 字符串字段: %v", path, out)
	}
	if r.updatedAt, ok = out["updatedAt"].(string); !ok {
		t.Fatalf("GET %s 缺 updatedAt 字符串字段（无草稿应为空串，不可省略键）: %v", path, out)
	}
	return r
}

// putDraft PUT 保存草稿，要求 204。
func putDraft(t *testing.T, app *fiber.App, cookie string, problemID int64, payload map[string]any) {
	t.Helper()
	resp, out := doJSON(t, app, "PUT", fmt.Sprintf("/api/oj/problem/%d/draft", problemID), cookie, payload)
	if resp.StatusCode != 204 {
		t.Fatalf("PUT 草稿 %v = %d %v, want 204", payload, resp.StatusCode, out)
	}
}

// TestDraftGetWithoutSaveReturnsEmptyUpdatedAt 未保存过 → 200，code 与 updatedAt 均为空串。
func TestDraftGetWithoutSaveReturnsEmptyUpdatedAt(t *testing.T) {
	app, _, ids := newDraftTestApp(t)
	cookie := loginStudent(t, app, "drafter", "pw")

	r := getDraft(t, app, cookie, ids["problem"], "lang=python")
	if _, present := r.raw["updatedAt"]; !present {
		t.Fatalf("响应必须显式含 updatedAt 键（前端据此判「无云端草稿」）: %v", r.raw)
	}
	if r.code != "" {
		t.Fatalf("code = %q, want 空串", r.code)
	}
	if r.language != "python" {
		t.Fatalf("language = %q, want python", r.language)
	}
	if r.updatedAt != "" {
		t.Fatalf("updatedAt = %q, want 空串（无草稿；非空会让前端用空气草覆盖本地）", r.updatedAt)
	}

	// 训练上下文未保存同样为空（上下文独立）
	r = getDraft(t, app, cookie, ids["problem"], "lang=python&ctxKind=training&ctxId=7")
	if r.code != "" || r.updatedAt != "" {
		t.Fatalf("训练上下文无草稿: code=%q updatedAt=%q, want 均空串", r.code, r.updatedAt)
	}
}

// TestDraftSaveThenGetUpdatedAtRFC3339WithZone PUT 204 → GET 回读：
// code 一致、updatedAt 可被 time.Parse(time.RFC3339) 解析、带显式时区且落在当前时间 ±5 分钟内。
func TestDraftSaveThenGetUpdatedAtRFC3339WithZone(t *testing.T) {
	app, qs, ids := newDraftTestApp(t)
	cookie := loginStudent(t, app, "drafter", "pw")
	problemID := ids["problem"]

	const code = "a, b = map(int, input().split())\nprint(a + b)\n"
	before := time.Now().UTC()
	putDraft(t, app, cookie, problemID, map[string]any{"language": "python", "code": code})
	after := time.Now().UTC()

	r := getDraft(t, app, cookie, problemID, "lang=python")
	if r.code != code {
		t.Fatalf("code = %q, want 保存的 %q", r.code, code)
	}
	if r.language != "python" {
		t.Fatalf("language = %q, want python", r.language)
	}
	if r.updatedAt == "" {
		t.Fatal("updatedAt = 空串, want 保存时间（已保存却为空 → 前端永不采用云端草稿）")
	}
	// 核心回归点 1：必须能被 RFC3339 解析（退化成 SQLite 裸格式 "2006-01-02 15:04:05" 会失败）
	parsed, err := time.Parse(time.RFC3339, r.updatedAt)
	if err != nil {
		t.Fatalf("updatedAt = %q 不是 RFC3339（前端 Date.parse 依赖时区）: %v", r.updatedAt, err)
	}
	// 核心回归点 2：必须带显式时区后缀（Z 或 ±HH:MM），否则前端按本地时区解析
	if !rfc3339ZoneRe.MatchString(r.updatedAt) {
		t.Fatalf("updatedAt = %q 不含显式时区（want 以 Z 或 ±HH:MM 结尾）", r.updatedAt)
	}
	// 契约：UTC 时刻 → Z 后缀
	if !strings.HasSuffix(r.updatedAt, "Z") {
		t.Fatalf("updatedAt = %q, want UTC 带 Z 后缀", r.updatedAt)
	}
	// 核心回归点 3：时刻正确（±5 分钟内）；若把 UTC 值当本地时间解析/输出，此处会差整数小时
	if delta := time.Since(parsed); delta < -5*time.Minute || delta > 5*time.Minute {
		t.Fatalf("updatedAt = %v 与本机当前时间相差 %v, want ≤5 分钟（时区错位会差数小时）", parsed, delta)
	}
	if parsed.Before(before.Add(-5*time.Minute)) || parsed.After(after.Add(5*time.Minute)) {
		t.Fatalf("updatedAt = %v 不在保存窗口 [%v, %v] 内", parsed, before, after)
	}

	// 老库存量行（SQLite 格式）经 HTTP 也必须升格为带 Z 的 RFC3339
	if _, err := qs.DB.Exec(`UPDATE code_drafts SET updated_at=? WHERE user_id=? AND problem_id=? AND language=?`,
		"2026-01-02 03:04:05", ids["user"], problemID, "python"); err != nil {
		t.Fatalf("rewrite legacy updated_at: %v", err)
	}
	r = getDraft(t, app, cookie, problemID, "lang=python")
	if r.code != code {
		t.Fatalf("存量格式行 code = %q, want %q", r.code, code)
	}
	if r.updatedAt != "2026-01-02T03:04:05Z" {
		t.Fatalf("存量 SQLite 格式下发 updatedAt = %q, want %q（须按 UTC 解析并带 Z）",
			r.updatedAt, "2026-01-02T03:04:05Z")
	}
}

// TestDraftHTTPContextIsolation 全局 / training / practice 与语言各自独立：
// 同题同语言不同上下文互不覆盖（PUT 用 body 的 ctxKind/ctxId，GET 用 query）。
func TestDraftHTTPContextIsolation(t *testing.T) {
	app, _, ids := newDraftTestApp(t)
	cookie := loginStudent(t, app, "drafter", "pw")
	problemID := ids["problem"]

	putDraft(t, app, cookie, problemID, map[string]any{"language": "python", "code": "global-py"})
	putDraft(t, app, cookie, problemID, map[string]any{
		"language": "python", "code": "train-7-py", "ctxKind": "training", "ctxId": 7})
	putDraft(t, app, cookie, problemID, map[string]any{
		"language": "python", "code": "train-8-py", "ctxKind": "training", "ctxId": 8})
	putDraft(t, app, cookie, problemID, map[string]any{
		"language": "python", "code": "practice-7-py", "ctxKind": "practice", "ctxId": 7})
	putDraft(t, app, cookie, problemID, map[string]any{"language": "cpp", "code": "global-cpp"})

	for _, tc := range []struct{ name, query, wantCode string }{
		{"全局 python", "lang=python", "global-py"},
		{"全局 cpp", "lang=cpp", "global-cpp"},
		{"训练 7", "lang=python&ctxKind=training&ctxId=7", "train-7-py"},
		{"训练 8", "lang=python&ctxKind=training&ctxId=8", "train-8-py"},
		{"练习 7", "lang=python&ctxKind=practice&ctxId=7", "practice-7-py"},
	} {
		r := getDraft(t, app, cookie, problemID, tc.query)
		if r.code != tc.wantCode {
			t.Fatalf("%s (%s): code = %q, want %q", tc.name, tc.query, r.code, tc.wantCode)
		}
		if r.updatedAt == "" {
			t.Fatalf("%s (%s): updatedAt 为空串（该槽已有草稿）", tc.name, tc.query)
		}
	}

	// 未保存过的槽：空草稿（不被其它上下文串味）
	for _, query := range []string{
		"lang=python&ctxKind=training&ctxId=99",
		"lang=python&ctxKind=practice&ctxId=99",
		"lang=cpp&ctxKind=training&ctxId=7",
	} {
		r := getDraft(t, app, cookie, problemID, query)
		if r.code != "" || r.updatedAt != "" {
			t.Fatalf("未保存的槽 %s: code=%q updatedAt=%q, want 均空串", query, r.code, r.updatedAt)
		}
	}

	// 覆盖训练草稿：全局与其它槽不变
	putDraft(t, app, cookie, problemID, map[string]any{
		"language": "python", "code": "train-7-py-v2", "ctxKind": "training", "ctxId": 7})
	for _, tc := range []struct{ query, wantCode string }{
		{"lang=python", "global-py"},
		{"lang=python&ctxKind=training&ctxId=7", "train-7-py-v2"},
		{"lang=python&ctxKind=training&ctxId=8", "train-8-py"},
		{"lang=python&ctxKind=practice&ctxId=7", "practice-7-py"},
	} {
		if r := getDraft(t, app, cookie, problemID, tc.query); r.code != tc.wantCode {
			t.Fatalf("覆盖训练草稿后 %s: code = %q, want %q（上下文串味）", tc.query, r.code, tc.wantCode)
		}
	}
}

// TestDraftEndpointValidation 既有限制不回退：非法 lang/上下文 400、不可见题 404，且被拒请求不写库。
func TestDraftEndpointValidation(t *testing.T) {
	app, qs, ids := newDraftTestApp(t)
	cookie := loginStudent(t, app, "drafter", "pw")
	base := fmt.Sprintf("/api/oj/problem/%d/draft", ids["problem"])
	foreignBase := fmt.Sprintf("/api/oj/problem/%d/draft", ids["foreign"])

	// 非法语言 → 400（GET/PUT 均拒）
	if resp, out := doJSON(t, app, "GET", base+"?lang=java", cookie, nil); resp.StatusCode != 400 {
		t.Fatalf("GET lang=java = %d %v, want 400", resp.StatusCode, out)
	}
	if resp, out := doJSON(t, app, "PUT", base, cookie, map[string]any{"language": "java", "code": "x"}); resp.StatusCode != 400 {
		t.Fatalf("PUT lang=java = %d %v, want 400", resp.StatusCode, out)
	}
	// 上下文非法 → 400：training 缺 ctxId（GET query / PUT body 两条路径）
	if resp, out := doJSON(t, app, "GET", base+"?lang=python&ctxKind=training", cookie, nil); resp.StatusCode != 400 {
		t.Fatalf("GET training 无 ctxId = %d %v, want 400", resp.StatusCode, out)
	}
	if resp, out := doJSON(t, app, "PUT", base, cookie,
		map[string]any{"language": "python", "code": "x", "ctxKind": "training"}); resp.StatusCode != 400 {
		t.Fatalf("PUT training 无 ctxId = %d %v, want 400", resp.StatusCode, out)
	}
	// 未知 ctxKind → 400
	if resp, out := doJSON(t, app, "GET", base+"?lang=python&ctxKind=bogus&ctxId=1", cookie, nil); resp.StatusCode != 400 {
		t.Fatalf("GET ctxKind=bogus = %d %v, want 400", resp.StatusCode, out)
	}
	if resp, out := doJSON(t, app, "PUT", base, cookie,
		map[string]any{"language": "python", "code": "x", "ctxKind": "bogus", "ctxId": 1}); resp.StatusCode != 400 {
		t.Fatalf("PUT ctxKind=bogus = %d %v, want 400", resp.StatusCode, out)
	}
	// 不可见题目（异域，学生未加入其域任何空间）→ 404
	if resp, out := doJSON(t, app, "GET", foreignBase+"?lang=python", cookie, nil); resp.StatusCode != 404 {
		t.Fatalf("GET 异域题草稿 = %d %v, want 404", resp.StatusCode, out)
	}
	if resp, out := doJSON(t, app, "PUT", foreignBase, cookie,
		map[string]any{"language": "python", "code": "x"}); resp.StatusCode != 404 {
		t.Fatalf("PUT 异域题草稿 = %d %v, want 404", resp.StatusCode, out)
	}
	// 被拒请求不得落库（校验/可见性早于写入）
	var n int
	if err := qs.DB.QueryRow(`SELECT COUNT(1) FROM code_drafts`).Scan(&n); err != nil {
		t.Fatalf("count code_drafts: %v", err)
	}
	if n != 0 {
		t.Fatalf("被拒请求写入了 %d 行草稿, want 0", n)
	}
}
