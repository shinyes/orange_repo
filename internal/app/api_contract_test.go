package app

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// 前后端接口契约测试：把前端源码里出现的所有 /api/... 请求路径与方法，
// 与后端实际注册的路由表逐条比对，任何不匹配（路径拼错、方法写错）都让测试失败。
//
// 背景：曾出现 `DELETE /api/space/space-items/:id`（前端多写了一段 /space），
// 后端实际是 `/api/space-items/:id`。由于 SPA 兜底注册了 `GET *`，任意 GET 路径都
// “存在”，该错误请求被 Fiber 判为「路径存在但方法不允许」→ 405 Method Not Allowed，
// 排查成本高。本测试把这类契约漂移在构建期暴露。

// reqCallRe 匹配一次 req(...) / req<T>(...) 调用的起点。
var reqCallRe = regexp.MustCompile(`\breq(?:<[^(]*>)?\(`)

var (
	methodRe     = regexp.MustCompile(`method:\s*'([A-Za-z]+)'`)
	backtickRe   = regexp.MustCompile("`([^`]*)`")
	singleQuotRe = regexp.MustCompile(`'([^'\n]*)'`)
	doubleQuotRe = regexp.MustCompile(`"([^"\n]*)"`)
)

// routeSpec 归一化后的后端路由。
type routeSpec struct {
	method string
	segs   []string
	raw    string
}

// collectAPIRoutes 取后端已注册的 /api 路由（排除中间件 USE、静态与通配兜底路由——
// 它们会匹配任意路径，纳入比对会让契约检查失去意义）。
func collectAPIRoutes(app *fiber.App) []routeSpec {
	var out []routeSpec
	for _, r := range app.GetRoutes() {
		p := r.Path
		if !strings.HasPrefix(p, "/api") {
			continue
		}
		if r.Method == "USE" || r.Method == "HEAD" {
			continue
		}
		if strings.Contains(p, "*") { // 静态目录与 /api/* 兜底：非具体端点
			continue
		}
		out = append(out, routeSpec{method: r.Method, segs: splitSegs(p), raw: r.Method + " " + p})
	}
	return out
}

func splitSegs(p string) []string {
	parts := strings.Split(strings.Trim(p, "/"), "/")
	out := make([]string, 0, len(parts))
	for _, s := range parts {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// matchRoute 判断（方法, 路径段）能否命中已注册路由：
// 路由参数段（:id）可匹配任意单段；前端动态段（:p，由 ${...} 归一化而来）可匹配
// 路由的字面量段（其运行时取值可能就是该字面量）；两边都是字面量时必须相等。
func matchRoute(routes []routeSpec, method string, segs []string) bool {
	for _, r := range routes {
		if r.method != method || len(r.segs) != len(segs) {
			continue
		}
		ok := true
		for i := range segs {
			f, rs := segs[i], r.segs[i]
			fDyn := strings.HasPrefix(f, ":")
			rDyn := strings.HasPrefix(rs, ":")
			if !fDyn && !rDyn && f != rs {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// hasPrefixRoute 前缀式调用（字面量以 / 结尾，如 '/api/space/' + id）的宽松检查：
// 只要存在同一方法、且以该前缀开头的路由即视为合法。
func hasPrefixRoute(routes []routeSpec, method, prefix string) bool {
	pre := splitSegs(prefix)
	for _, r := range routes {
		if r.method != method || len(r.segs) < len(pre) {
			continue
		}
		ok := true
		for i := range pre {
			f, rs := pre[i], r.segs[i]
			fDyn := strings.HasPrefix(f, ":")
			rDyn := strings.HasPrefix(rs, ":")
			if !fDyn && !rDyn && f != rs {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// matchStaticPrefix 静态前缀匹配（用于 URL 拼接式调用，如 dq(`/api/export/backup${q}`)）：
// 前端静态段必须与路由对应段严格相等——不允许路由的动态段吸收前端字面量，
// 否则 `.../space/space-items/${id}` 会被 `/api/space/:id/trainings` 误判为合法。
// 方法不限（拼接式 URL 多用于下载，方法由具体用法决定）。
func matchStaticPrefix(routes []routeSpec, segs []string) bool {
	if len(segs) == 0 {
		return false
	}
	for _, r := range routes {
		if len(r.segs) < len(segs) {
			continue
		}
		ok := true
		for i := range segs {
			if r.segs[i] != segs[i] {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// matchRouteAny 与 matchRoute 同，但不限定方法（用于无法可靠推断方法的 URL 字面量）。
func matchRouteAny(routes []routeSpec, segs []string) bool {
	for _, r := range routes {
		if r.method == "USE" || len(r.segs) != len(segs) {
			continue
		}
		ok := true
		for i := range segs {
			fDyn := strings.HasPrefix(segs[i], ":")
			rDyn := strings.HasPrefix(r.segs[i], ":")
			if !fDyn && !rDyn && segs[i] != r.segs[i] {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// apiLiteralRe 匹配被引号/反引号包裹、以 /api/ 开头的路径字面量。
var apiLiteralRe = regexp.MustCompile("[\"'`](/api/[^\"'`]*)[\"'`]")

// walkSource 遍历前端源码目录下所有 .ts/.tsx，回调相对路径与文件内容。
func walkSource(t *testing.T, srcDir string, fn func(rel, src string)) {
	t.Helper()
	err := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if !strings.HasSuffix(path, ".ts") && !strings.HasSuffix(path, ".tsx") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(srcDir, path)
		fn(rel, string(raw))
		return nil
	})
	if err != nil {
		t.Fatalf("扫描前端源码: %v", err)
	}
}

// collectLooseLiterals 收集 req(...) 之外的 /api 字面量（URL 拼接、下载链接等），
// 用于路径级校验（方法不限）。跳过注释行，避免注释里的示例路径造成误报。
func collectLooseLiterals(t *testing.T, srcDir string) []frontendCall {
	t.Helper()
	var out []frontendCall
	walkSource(t, srcDir, func(rel, src string) {
		for i, line := range strings.Split(src, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "*") || strings.HasPrefix(trimmed, "/*") {
				continue
			}
			for _, m := range apiLiteralRe.FindAllStringSubmatch(line, -1) {
				out = append(out, frontendCall{file: rel, line: i + 1, method: "GET", raw: m[1]})
			}
		}
	})
	return out
}

// stripTemplates 把前端模板串里的 ${...} 归一化：
//   - 独立占位（前一个字符是 /）→ :p
//   - 紧跟在字面量后的占位（如 .../problem${cond ? '?fresh=1' : ”}）→ 删除
//     （这类占位是查询串等可选后缀，不代表路径段）
//
// 同时去掉查询串与末尾斜杠。
func stripTemplates(raw string) string {
	var b strings.Builder
	for i := 0; i < len(raw); {
		if strings.HasPrefix(raw[i:], "${") {
			depth := 1
			j := i + 2
			for j < len(raw) && depth > 0 {
				switch raw[j] {
				case '{':
					depth++
				case '}':
					depth--
				}
				j++
			}
			prev := byte('/')
			if b.Len() > 0 {
				prev = b.String()[b.Len()-1]
			}
			if prev == '/' {
				b.WriteString(":p")
			}
			i = j
			continue
		}
		b.WriteByte(raw[i])
		i++
	}
	s := b.String()
	if idx := strings.IndexByte(s, '?'); idx >= 0 {
		s = s[:idx]
	}
	s = strings.TrimSuffix(s, "/")
	return s
}

// frontendCall 前端一处 /api 调用。
type frontendCall struct {
	file   string
	line   int
	method string
	raw    string
	path   string
	prefix bool
}

// extractCallArgs 从 reqCallRe 命中处开始做括号配对，返回该次调用的参数片段。
func extractCallArgs(src string, open int) (string, bool) {
	depth := 0
	for i := open; i < len(src); i++ {
		switch src[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return src[open+1 : i], true
			}
		}
	}
	return "", false
}

// pickAPIPath 从一次调用的参数片段里取出 /api 开头的路径字面量
// （优先反引号模板串——其内部允许出现单引号，如 ${kind === 'training' ? ...}）。
func pickAPIPath(args string) string {
	for _, m := range backtickRe.FindAllStringSubmatch(args, -1) {
		if strings.HasPrefix(m[1], "/api/") {
			return m[1]
		}
	}
	for _, re := range []*regexp.Regexp{singleQuotRe, doubleQuotRe} {
		for _, m := range re.FindAllStringSubmatch(args, -1) {
			if strings.HasPrefix(m[1], "/api/") {
				return m[1]
			}
		}
	}
	return ""
}

// collectFrontendCalls 扫描前端源码，按 req(...) 调用切片收集所有 /api 调用。
func collectFrontendCalls(t *testing.T, srcDir string) []frontendCall {
	t.Helper()
	var calls []frontendCall
	err := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if !strings.HasSuffix(path, ".ts") && !strings.HasSuffix(path, ".tsx") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(srcDir, path)
		src := string(raw)
		for _, loc := range reqCallRe.FindAllStringIndex(src, -1) {
			args, ok := extractCallArgs(src, loc[1]-1)
			if !ok {
				continue
			}
			p := pickAPIPath(args)
			if p == "" {
				continue
			}
			method := "GET"
			if mm := methodRe.FindStringSubmatch(args); mm != nil {
				method = strings.ToUpper(mm[1])
			}
			line := strings.Count(src[:loc[0]], "\n") + 1
			call := frontendCall{file: rel, line: line, method: method, raw: p}
			call.prefix = strings.HasSuffix(p, "/") && !strings.Contains(p, "${")
			call.path = stripTemplates(p)
			calls = append(calls, call)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("扫描前端源码: %v", err)
	}
	return calls
}

// TestFrontendAPIContract 前端所有 /api 调用必须命中后端已注册路由（路径+方法）。
func TestFrontendAPIContract(t *testing.T) {
	a := newMergedApp(t)
	routes := collectAPIRoutes(a.Fiber)
	if len(routes) == 0 {
		t.Fatal("未取到任何 /api 路由（装配异常）")
	}

	srcDir := filepath.Join("..", "..", "app", "src")
	if _, err := os.Stat(srcDir); err != nil {
		t.Skipf("前端源码目录不可用（%v）", err)
	}
	calls := collectFrontendCalls(t, srcDir)
	if len(calls) < 50 {
		t.Fatalf("只解析到 %d 处 /api 调用，疑似解析规则失效", len(calls))
	}

	var bad []string
	covered := map[string]bool{}
	for _, c := range calls {
		covered[c.file+"|"+c.raw] = true
		if c.prefix {
			if !hasPrefixRoute(routes, c.method, c.path) {
				bad = append(bad, c.file+":"+itoa(c.line)+" "+c.method+" "+c.raw+"（前缀无匹配路由）")
			}
			continue
		}
		if !matchRoute(routes, c.method, splitSegs(c.path)) {
			bad = append(bad, c.file+":"+itoa(c.line)+" "+c.method+" "+c.raw)
		}
	}

	// 路径级检查：req 之外出现的 /api 字面量（URL 拼接、下载链接）必须存在（方法不限）
	looseChecked := 0
	for _, c := range collectLooseLiterals(t, srcDir) {
		if covered[c.file+"|"+c.raw] {
			continue
		}
		looseChecked++
		// 动态尾部（``/api/export/backup${q}``）与纯前缀常量（``/api/auth/`` + name）
		// 都只校验静态部分：其段必须与某路由严格对齐（不允许被路由动态段吸收）。
		if idx := strings.Index(c.raw, "${"); idx >= 0 || strings.HasSuffix(c.raw, "/") {
			static := c.raw
			if idx >= 0 {
				static = c.raw[:idx]
			}
			static = stripTemplates(static)
			if static == "" || !matchStaticPrefix(routes, splitSegs(static)) {
				bad = append(bad, c.file+":"+itoa(c.line)+" [拼接] "+c.raw+"（静态前缀无匹配路由）")
			}
			continue
		}
		if !matchRouteAny(routes, splitSegs(stripTemplates(c.raw))) {
			bad = append(bad, c.file+":"+itoa(c.line)+" [URL] "+c.raw)
		}
	}

	if len(bad) > 0 {
		sort.Strings(bad)
		t.Fatalf("前端有 %d 处 API 路径/方法与后端路由不匹配：\n  %s",
			len(bad), strings.Join(bad, "\n  "))
	}
	t.Logf("契约检查通过：req 调用 %d 处（方法+路径严格）· 其他 URL 字面量 %d 处（路径级）↔ 后端路由 %d 条",
		len(calls), looseChecked, len(routes))
}

// TestUnknownAPIRouteReturns404JSON /api 下未注册的路径/方法必须返回 JSON 404，
// 而不是被 SPA 兜底吞掉或变成费解的 405 Method Not Allowed。
func TestUnknownAPIRouteReturns404JSON(t *testing.T) {
	a := newMergedApp(t)
	cookie := loginCookie(t, a.Fiber, "admin", "123456")

	cases := []struct{ method, path string }{
		{"DELETE", "/api/space/space-items/1"}, // 历史错误路径（前端曾多写 /space）
		{"DELETE", "/api/space-items/1"},       // 正确路径但条目不存在 → handler 给 404
		{"GET", "/api/no-such-endpoint"},
		{"POST", "/api/no-such-endpoint"},
		{"DELETE", "/api/no-such-endpoint"},
		{"PUT", "/api/no-such-endpoint/1"},
	}
	for _, c := range cases {
		resp, out := doJSON(t, a.Fiber, c.method, c.path, cookie, nil)
		if resp.StatusCode != fiber.StatusNotFound {
			t.Errorf("%s %s = %d, want 404（不应出现 405/200）", c.method, c.path, resp.StatusCode)
			continue
		}
		if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "json") {
			t.Errorf("%s %s Content-Type = %q, want JSON", c.method, c.path, ct)
		}
		if msg, _ := out["error"].(string); msg == "" {
			t.Errorf("%s %s 响应缺 error 字段: %v", c.method, c.path, out)
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
