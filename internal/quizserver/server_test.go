package quizserver

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// doJSON 发送 JSON 请求（cookie 可空），返回响应与解析后的响应体。
func doJSON(t *testing.T, app *fiber.App, method, path, cookie string, body any) (*http.Response, map[string]any) {
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
	var out map[string]any
	if resp.Body != nil {
		raw, _ := io.ReadAll(resp.Body)
		if len(raw) > 0 && strings.Contains(resp.Header.Get("Content-Type"), "json") {
			_ = json.Unmarshal(raw, &out)
		}
	}
	return resp, out
}

// cookieOf 从登录响应提取会话 cookie 片段。
func cookieOf(resp *http.Response) string {
	sc := resp.Header.Get("Set-Cookie")
	return strings.SplitN(sc, ";", 2)[0]
}

// nested 读取嵌套响应字段（如 "problems.0.id"）。
func nested(m map[string]any, path string) any {
	cur := any(m)
	for _, part := range strings.Split(path, ".") {
		if part == "" {
			continue
		}
		switch v := cur.(type) {
		case map[string]any:
			cur = v[part]
		case []any:
			idx := 0
			for i := 0; i < len(part); i++ {
				if part[i] < '0' || part[i] > '9' {
					return nil
				}
				idx = idx*10 + int(part[i]-'0')
			}
			if idx >= len(v) {
				return nil
			}
			cur = v[idx]
		default:
			return nil
		}
	}
	return cur
}
