// 小游戏提交响应的名次一致性：POST score 回带的 rankDomain/rankAll 必须与 GET rank 的 myRank 同值。
// 回归背景：旧实现直接取 ListGameRank 的第二个返回值，而它有「只在我未进前 limit 时给出」的语义，
// 用户通常都在前 50 内 → 提交响应里的名次恒为 0（前端据此静默不显示名次）。
package quizserver

import (
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestGameScoreSubmitRankMatchesRankAPI(t *testing.T) {
	e := newGameTestEnv(t)
	e.resetLimit()

	// 造出「我不是第一」的局面：A2 先拿到更高分，然后我（A1）提交中等分数
	if resp, out := e.submit(t, e.cookieA2, "dino", map[string]any{"score": 500, "spaceId": 0}); resp.StatusCode != fiber.StatusOK {
		t.Fatalf("A2 提交 = %d %v", resp.StatusCode, out)
	}
	e.resetLimit()

	resp, out := e.submit(t, e.cookieA1, "dino", map[string]any{"score": 120, "spaceId": 0})
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("A1 提交 = %d %v", resp.StatusCode, out)
	}
	rankAll, ok := out["rankAll"].(float64)
	if !ok {
		t.Fatalf("提交响应缺少 rankAll：%v", out)
	}
	if rankAll != 2 {
		t.Fatalf("提交响应 rankAll = %v, want 2（在前 50 内也必须回带真实名次，不能是 0）", out["rankAll"])
	}

	// 与全域榜单接口对照：myRank 必须同值
	_, rankOut := e.rank(t, e.cookieA1, "dino", "all", 0)
	if got, _ := rankOut["myRank"].(float64); got != rankAll {
		t.Fatalf("全域榜 myRank = %v, 提交响应 rankAll = %v（必须一致）", rankOut["myRank"], rankAll)
	}

	// 本域榜同样一致（A1 与 A2 同域）
	rankDomain, ok := out["rankDomain"].(float64)
	if !ok {
		t.Fatalf("提交响应缺少 rankDomain：%v", out)
	}
	_, rankDomOut := e.rank(t, e.cookieA1, "dino", "domain", 0)
	if got, _ := rankDomOut["myRank"].(float64); got != rankDomain {
		t.Fatalf("本域榜 myRank = %v, 提交响应 rankDomain = %v（必须一致）", rankDomOut["myRank"], rankDomain)
	}
	if rankDomain != 2 {
		t.Fatalf("本域榜应为第 2 名（A2 500 > A1 120），得到 rankDomain = %v", rankDomain)
	}
}

// 提交后榜单里应能看到自己（IsMe）与正确名次——与提交响应同源。
func TestGameScoreSubmitReflectedInRankRows(t *testing.T) {
	e := newGameTestEnv(t)
	e.resetLimit()
	if resp, out := e.submit(t, e.cookieA1, "dino", map[string]any{"score": 77, "spaceId": 0}); resp.StatusCode != fiber.StatusOK {
		t.Fatalf("提交 = %d %v", resp.StatusCode, out)
	}
	_, rankOut := e.rank(t, e.cookieA1, "dino", "all", 0)
	rows := rowsOf(t, rankOut)
	if len(rows) == 0 {
		t.Fatalf("提交后榜单为空：%v", rankOut)
	}
	if rows[0]["bestScore"].(float64) != 77 {
		t.Fatalf("榜首分数 = %v, want 77", rows[0]["bestScore"])
	}
	if isMe, _ := rows[0]["isMe"].(bool); !isMe {
		t.Fatalf("榜首应标记 isMe：%v", rows[0])
	}
}