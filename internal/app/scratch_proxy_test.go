// Scratch 容器反代测试：容器作为**内部容器**时，主站必须在它的公开域名**根路径**上反代。
//
// 背景（真实故障）：上游 scratch-gui 的 standalone 构建把 webpack publicPath 硬编码为 "/"，
// 运行期以绝对路径取懒加载 chunk（/chunks/fetch-worker.*.js 等）、块素材（/static/blocks-media/...）
// 与教程图（/static/assets/...）。此前把容器挂在路径前缀 /scratch-app/ 下，这些请求会打到主站域名根、
// 被 SPA 兜底成 index.html（浏览器报 "Unexpected token '<'"），块素材也全部 404。
// 本测试锁定：① 公开域名的根路径请求被转发到容器（含 /chunks/*、/static/*）；
//
//	② 其它域名照常由主站处理（不受影响）；③ /api/config 下发公开地址。
package app_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"orangeoj/internal/app"
)

const scratchHost = "scratch.test.example"

func TestScratchContainerProxiedByHost(t *testing.T) {
	// 模拟 Scratch 容器（nginx 静态服务）：根路径 + chunk + 块素材
	container := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte("<html>scratch-editor-host</html>"))
		case "/chunks/fetch-worker.abc.js":
			w.Header().Set("Content-Type", "application/javascript")
			_, _ = w.Write([]byte("self.onmessage=()=>{}"))
		case "/static/blocks-media/default/repeat.svg":
			w.Header().Set("Content-Type", "image/svg+xml")
			_, _ = w.Write([]byte("<svg/>"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer container.Close()

	// 主站前端产物：只需一个 index.html 供 SPA 兜底。
	// 注意：不用 t.TempDir()——Windows 下 Fiber 的 SendFile 会短暂持有文件句柄，
	// TempDir 的 RemoveAll 清理会因此失败（报 "being used by another process"）；
	// 这里自建目录并容错清理。
	web, err := os.MkdirTemp("", "orangeoj-scratch-web-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(web) })
	if err := os.WriteFile(filepath.Join(web, "index.html"), []byte("<html>main-site-spa</html>"), 0o644); err != nil {
		t.Fatal(err)
	}

	a, err := app.Open(app.Config{
		DataDir:            t.TempDir(),
		WebDist:            web,
		ScratchURL:         "https://" + scratchHost,
		ScratchInternalURL: container.URL,
	})
	if err != nil {
		t.Fatalf("app.Open: %v", err)
	}
	defer a.Close()

	get := func(host, path string) (int, string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Host = host
		resp, err := a.Fiber.Test(req, 5000)
		if err != nil {
			t.Fatalf("request %s%s: %v", host, path, err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(body)
	}

	// ① 公开域名的根路径 → 容器（编辑器宿主页）
	if code, body := get(scratchHost, "/"); code != http.StatusOK || !strings.Contains(body, "scratch-editor-host") {
		t.Fatalf("根路径未反代到容器：code=%d body=%.60q", code, body)
	}
	// ② 懒加载 chunk（绝对路径 /chunks/...）→ 容器：这是此前致命的一处
	if code, body := get(scratchHost, "/chunks/fetch-worker.abc.js"); code != http.StatusOK || !strings.Contains(body, "onmessage") {
		t.Fatalf("chunk 未反代到容器：code=%d body=%.60q", code, body)
	}
	// ③ 块素材（绝对路径 /static/blocks-media/default/...）→ 容器
	if code, body := get(scratchHost, "/static/blocks-media/default/repeat.svg"); code != http.StatusOK || !strings.Contains(body, "<svg") {
		t.Fatalf("块素材未反代到容器：code=%d body=%.60q", code, body)
	}
	// ④ 其它域名不受影响（仍由主站处理）
	if code, body := get("orangeoj.test.example", "/"); code != http.StatusOK || !strings.Contains(body, "main-site-spa") {
		t.Fatalf("主站根路径受影响：code=%d body=%.60q", code, body)
	}
	// ⑤ /api/config 下发公开地址（前端据此加载 iframe）
	code, body := get("orangeoj.test.example", "/api/config")
	if code != http.StatusOK || !strings.Contains(body, "https://"+scratchHost) {
		t.Fatalf("/api/config 未下发 scratchUrl：code=%d body=%.120q", code, body)
	}

	// ⑥ 只配公开地址（子域直连容器）时：主站不做任何反代
	external, err := app.Open(app.Config{
		DataDir:    t.TempDir(),
		WebDist:    web,
		ScratchURL: "https://" + scratchHost,
	})
	if err != nil {
		t.Fatalf("app.Open(仅公开地址): %v", err)
	}
	defer external.Close()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = scratchHost
	resp, err := external.Fiber.Test(req, 5000)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(b), "main-site-spa") {
		t.Fatalf("仅配公开地址时不应反代，实际 body=%.60q", string(b))
	}

	// ⑦ 配了内部地址但没给公开地址 → **只告警不拦启动**：服务照常可用，Scratch 视为未部署。
	// （曾经把这种情况当硬错误，直接把线上服务拦死；可选功能配置不全不应导致整站起不来。）
	partial, err := app.Open(app.Config{
		DataDir:            t.TempDir(),
		WebDist:            web,
		ScratchInternalURL: container.URL,
	})
	if err != nil {
		t.Fatalf("只配内部地址时应正常启动（仅告警），实际报错：%v", err)
	}
	defer partial.Close()
	if code, body := getFrom(partial, "orangeoj.test.example", "/api/config"); code != http.StatusOK || strings.Contains(body, `"scratchUrl":"http`) {
		t.Fatalf("未配公开地址时不应下发 scratchUrl：code=%d body=%.120q", code, body)
	}
	if code, body := getFrom(partial, scratchHost, "/"); code != http.StatusOK || !strings.Contains(body, "main-site-spa") {
		t.Fatalf("未配公开地址时不应反代：code=%d body=%.60q", code, body)
	}

	// ⑧ 内部地址非法 → 同样只告警，不影响启动
	invalidTarget, err := app.Open(app.Config{
		DataDir:            t.TempDir(),
		WebDist:            web,
		ScratchURL:         "https://" + scratchHost,
		ScratchInternalURL: "://bad-url",
	})
	if err != nil {
		t.Fatalf("内部地址非法时应正常启动（仅告警），实际报错：%v", err)
	}
	defer invalidTarget.Close()
	if code, body := getFrom(invalidTarget, scratchHost, "/"); code != http.StatusOK || !strings.Contains(body, "main-site-spa") {
		t.Fatalf("内部地址非法时不应反代：code=%d body=%.60q", code, body)
	}
}

// getFrom 对指定 App 发一次带 Host 的请求（与上面的 get 同语义，便于对多个 App 断言）。
func getFrom(a *app.App, host, path string) (int, string) {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Host = host
	resp, err := a.Fiber.Test(req, 5000)
	if err != nil {
		return 0, err.Error()
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}
