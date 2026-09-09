// 排行榜公开开关（域设置 leaderboard_public）：关闭后普通成员 403，
// 系统/域管理员不受影响；重新公开后恢复。
package quizserver

import (
	"fmt"
	"testing"

	"github.com/gofiber/fiber/v2"

	"orangeoj/internal/quizstore"
	"orangeoj/internal/store"
)

func TestPortalRankPublicity(t *testing.T) {
	dir := t.TempDir()
	// 阶段 1：主库建域 + 空间
	main, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open main store: %v", err)
	}
	domainID, err := main.CreateDomain("排行域")
	if err != nil {
		t.Fatal(err)
	}
	spaceID, err := main.CreateSpace(domainID, "一班")
	if err != nil {
		t.Fatal(err)
	}
	if err := main.Close(); err != nil {
		t.Fatal(err)
	}

	// 阶段 2：quiz 建账号（member + 系统管理员 + 域管理员）
	qs, err := quizstore.Open(dir)
	if err != nil {
		t.Fatalf("open quiz store: %v", err)
	}
	t.Cleanup(func() { _ = qs.Close() })
	stuID, err := qs.Accounts.CreateUser("stu1", "pw", "member")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := qs.Accounts.CreateUser("gadmin", "pw", "global_admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := qs.Accounts.CreateUser("dadmin", "pw", "domain_admin", domainID); err != nil {
		t.Fatal(err)
	}

	// 阶段 3：回主库写空间成员（main2 保持打开供翻转开关）
	main2, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = main2.Close() })
	if err := main2.SetSpaceMembers(spaceID, []int64{stuID}); err != nil {
		t.Fatal(err)
	}

	srv := &Server{QS: qs}
	app := New(srv, nil, 0)
	stuCookie := loginStudent(t, app, "stu1", "pw")
	gCookie := loginStudent(t, app, "gadmin", "pw")
	dCookie := loginStudent(t, app, "dadmin", "pw")
	rankPath := fmt.Sprintf("/api/portal/rank?domainId=%d", domainID)

	// 默认公开：member 可看（200）
	if resp, out := doJSON(t, app, "GET", rankPath, stuCookie, nil); resp.StatusCode != fiber.StatusOK {
		t.Fatalf("公开时 member rank = %d %v", resp.StatusCode, out)
	}

	// 域设置关闭公开（模拟域管理 PATCH leaderboardPublic=false）
	if _, err := main2.DB.Exec(`UPDATE domains SET leaderboard_public=0 WHERE id=?`, domainID); err != nil {
		t.Fatal(err)
	}
	// member → 403 排行榜未公开
	resp, out := doJSON(t, app, "GET", rankPath, stuCookie, nil)
	if resp.StatusCode != fiber.StatusForbidden || out["error"] != "排行榜未公开" {
		t.Fatalf("关闭后 member rank = %d %v, want 403 排行榜未公开", resp.StatusCode, out)
	}
	// 管理员始终可看（系统管理员 + 域管理员）
	if resp, _ := doJSON(t, app, "GET", rankPath, gCookie, nil); resp.StatusCode != fiber.StatusOK {
		t.Fatalf("关闭后 global_admin rank = %d, want 200", resp.StatusCode)
	}
	if resp, _ := doJSON(t, app, "GET", rankPath, dCookie, nil); resp.StatusCode != fiber.StatusOK {
		t.Fatalf("关闭后 domain_admin rank = %d, want 200", resp.StatusCode)
	}

	// 重新公开 → member 恢复可见
	if _, err := main2.DB.Exec(`UPDATE domains SET leaderboard_public=1 WHERE id=?`, domainID); err != nil {
		t.Fatal(err)
	}
	if resp, out := doJSON(t, app, "GET", rankPath, stuCookie, nil); resp.StatusCode != fiber.StatusOK {
		t.Fatalf("重开后 member rank = %d %v", resp.StatusCode, out)
	}
}
