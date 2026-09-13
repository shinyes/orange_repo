// 孤儿域管理员（role=domain_admin 且未关联域）的越权回归。
// 该状态只能由脏数据/历史数据产生，但一旦存在，旧实现的 resolveSpaceCtx 因
// 「DomainID != nil 才校验本域」而**跳过校验**，等于放行任意空间——叠加 ?scope=all
// 就能跨域读到别域成员的姓名与答卷。修复后与排行榜入口口径一致：403「域管理员未关联域」。
package quizserver

import (
	"strconv"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestOrphanDomainAdminForbiddenInSpaceEndpoints(t *testing.T) {
	e := newPSEnv(t)

	// 不传 domainID 的 domain_admin = 未关联域（脏数据形态）
	if _, err := e.qs.Accounts.CreateUser("psOrphanAdmin", "pw", "domain_admin"); err != nil {
		t.Fatal(err)
	}
	cookie := loginStudent(t, e.app, "psOrphanAdmin", "pw")

	paths := []string{
		"/api/portal/space/" + psID(e.space) + "/home",
		"/api/portal/space/" + psID(e.space) + "/quizzes",
		"/api/portal/space/" + psID(e.space) + "/practice/" + psID(e.practice) + "/submissions",
		"/api/portal/space/" + psID(e.space) + "/practice/" + psID(e.practice) + "/submissions?scope=all",
		"/api/portal/space/" + psID(e.spaceOther) + "/practice/" + psID(e.practiceFar) + "/submissions?scope=all",
	}
	for _, p := range paths {
		resp, out := doJSON(t, e.app, "GET", p, cookie, nil)
		if resp.StatusCode != fiber.StatusForbidden {
			t.Fatalf("%s = %d %v, want 403（孤儿域管理员不得访问任意空间）", p, resp.StatusCode, out)
		}
		if msg, _ := out["error"].(string); msg != "域管理员未关联域" {
			t.Fatalf("%s 错误文案 = %v, want 域管理员未关联域", p, out["error"])
		}
	}

	// 系统管理员与「已关联域的域管理员」不受影响（不得误伤）
	if resp, out := doJSON(t, e.app, "GET",
		"/api/portal/space/"+psID(e.space)+"/practice/"+psID(e.practice)+"/submissions?scope=all",
		e.gAdminCook, nil); resp.StatusCode != fiber.StatusOK {
		t.Fatalf("系统管理员 = %d %v, want 200", resp.StatusCode, out)
	}
	if resp, out := doJSON(t, e.app, "GET",
		"/api/portal/space/"+psID(e.space)+"/practice/"+psID(e.practice)+"/submissions?scope=all",
		e.dAdminCook, nil); resp.StatusCode != fiber.StatusOK {
		t.Fatalf("本域域管理员 = %d %v, want 200", resp.StatusCode, out)
	}
}

// psID 数字 → 字符串（测试内拼路径用）。
func psID(v int64) string { return strconv.FormatInt(v, 10) }
