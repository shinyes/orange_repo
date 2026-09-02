// HTTPRunner 与 runner 接口（迁移自上游 OrangeOJ backend/internal/judge/runner.go）。
package judge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// HTTPRunner 通过 HTTP 调用独立 judge-runtime 执行评测。
type HTTPRunner struct {
	endpoint string
	token    string
	client   *http.Client
}

// NewHTTPRunner 构造 HTTPRunner；endpoint 空时默认 http://judge-runtime:9090。
func NewHTTPRunner(endpoint, token string, timeout time.Duration) *HTTPRunner {
	cleanEndpoint := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if cleanEndpoint == "" {
		cleanEndpoint = "http://judge-runtime:9090"
	}
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	return &HTTPRunner{
		endpoint: cleanEndpoint,
		token:    strings.TrimSpace(token),
		client:   &http.Client{Timeout: timeout},
	}
}

// Judge 执行一次评测任务并解析结果。
func (r *HTTPRunner) Judge(ctx context.Context, task JudgeTask) (RunResult, error) {
	if strings.TrimSpace(task.Language) == "" {
		return RunResult{}, fmt.Errorf("language is required")
	}
	if strings.TrimSpace(task.SourceCode) == "" {
		return RunResult{}, fmt.Errorf("sourceCode is required")
	}
	if len(task.Cases) == 0 {
		return RunResult{}, fmt.Errorf("judge cases are empty")
	}

	payload, err := json.Marshal(task)
	if err != nil {
		return RunResult{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint+"/internal/judge/execute", bytes.NewReader(payload))
	if err != nil {
		return RunResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if r.token != "" {
		req.Header.Set("X-Judge-Token", r.token)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return RunResult{}, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = resp.Status
		}
		return RunResult{}, fmt.Errorf("judge runtime error: %s", msg)
	}

	var result RunResult
	if err := json.Unmarshal(body, &result); err != nil {
		return RunResult{}, fmt.Errorf("invalid judge response: %w", err)
	}
	if result.Verdict == "" {
		return RunResult{}, fmt.Errorf("judge response missing verdict")
	}
	return result, nil
}
