// RenameUser（管理员修改用户名）accounts 层回归测试。
//
// 覆盖：改名后新旧用户名的可登录性、大小写不敏感唯一（COLLATE NOCASE）冲突、
// 非法名校验（空/全空格/超长/控制字符）与「失败不改库」、不存在 id → ErrNotFound、
// 同名幂等（含仅大小写变化）、角色无关（member/global_admin/domain_admin 均可改）、
// 会话按 user_id 关联故改名不失效、TrimSpace 落库。
package accounts_test

import (
	"errors"
	"strings"
	"testing"

	"orangeoj/internal/accounts"
)

// renameFixture 建 alice/bob 两名成员，返回 Store 与各自 id。
func renameFixture(t *testing.T) (*accounts.Store, int64, int64) {
	t.Helper()
	s := newTestAccounts(t)
	aliceID, err := s.CreateUser("alice", "pw-alice", accounts.RoleMember)
	if err != nil {
		t.Fatalf("创建 alice: %v", err)
	}
	bobID, err := s.CreateUser("bob", "pw-bob", accounts.RoleMember)
	if err != nil {
		t.Fatalf("创建 bob: %v", err)
	}
	return s, aliceID, bobID
}

// usernameOf 读库中该账号当前用户名（断言真实 DB 状态，而非只看返回值）。
func usernameOf(t *testing.T, s *accounts.Store, id int64) string {
	t.Helper()
	var name string
	if err := s.DB.QueryRow(`SELECT username FROM users WHERE id=?`, id).Scan(&name); err != nil {
		t.Fatalf("读取用户 %d 用户名: %v", id, err)
	}
	return name
}

// TestRenameUserBasic 改名成功：新名可查/可登录，旧名查询 ErrNotFound 且不可登录；
// 列表接口同步；入参前后空白被裁剪；仅大小写变化也真正落库。
func TestRenameUserBasic(t *testing.T) {
	s, aliceID, _ := renameFixture(t)

	if err := s.RenameUser(aliceID, "alice2"); err != nil {
		t.Fatalf("RenameUser(alice → alice2) = %v, want nil", err)
	}
	// 新名命中且 id/角色不变
	u, err := s.GetUserByUsername("alice2")
	if err != nil {
		t.Fatalf("按新用户名查询失败: %v", err)
	}
	if u.ID != aliceID || u.Username != "alice2" || u.Role != accounts.RoleMember {
		t.Fatalf("新名账号 = %+v, want id=%d username=alice2 role=member", u, aliceID)
	}
	// 旧名不再命中（具体错误值，不是「任意错误」）
	if _, err := s.GetUserByUsername("alice"); !errors.Is(err, accounts.ErrNotFound) {
		t.Fatalf("旧用户名查询 err = %v, want ErrNotFound", err)
	}
	// 关键用户可见行为：新用户名 + 原密码可登录、旧用户名不可登录
	if _, err := s.CheckPassword("alice2", "pw-alice"); err != nil {
		t.Fatalf("新用户名 + 原密码登录失败: %v", err)
	}
	if _, err := s.CheckPassword("alice", "pw-alice"); err == nil {
		t.Fatal("旧用户名仍可登录（改名后旧名应失效）")
	}
	// 大小写不敏感查询命中同一账号
	if u2, err := s.GetUserByUsername("ALICE2"); err != nil || u2.ID != aliceID {
		t.Fatalf("大小写不敏感查询 = %+v err=%v, want id=%d", u2, err, aliceID)
	}
	// GetUserByID 同步
	if byID, err := s.GetUserByID(aliceID); err != nil || byID.Username != "alice2" {
		t.Fatalf("GetUserByID = %+v err=%v, want username=alice2", byID, err)
	}
	// 列表同步（集中用户管理页/成员下拉数据源）
	all, err := s.ListAllUsers()
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, x := range all {
		names[x.Username] = true
	}
	if !names["alice2"] || names["alice"] {
		t.Fatalf("ListAllUsers 用户名集合 = %v, want 含 alice2 且不含 alice", names)
	}
	students, err := s.ListStudents()
	if err != nil {
		t.Fatal(err)
	}
	if len(students) != 2 || students[0].ID != aliceID || students[0].Username != "alice2" {
		t.Fatalf("ListStudents = %+v, want 首条 id=%d username=alice2", students, aliceID)
	}

	// 前后空白裁剪后落库（不保留空格）
	if err := s.RenameUser(aliceID, "  alice3  "); err != nil {
		t.Fatalf("带空白改名失败: %v", err)
	}
	if got := usernameOf(t, s, aliceID); got != "alice3" {
		t.Fatalf("带空白改名后 DB 用户名 = %q, want alice3（应 TrimSpace）", got)
	}
	// 仅大小写变化也应真正落库（显示名以最新写入为准）
	if err := s.RenameUser(aliceID, "Alice3"); err != nil {
		t.Fatalf("大小写改名失败: %v", err)
	}
	if got := usernameOf(t, s, aliceID); got != "Alice3" {
		t.Fatalf("大小写改名后 DB 用户名 = %q, want Alice3", got)
	}
	if _, err := s.CheckPassword("ALICE3", "pw-alice"); err != nil {
		t.Fatalf("大小写不敏感登录失败: %v", err)
	}
}

// TestRenameUserConflict 重名冲突：命中他人用户名（含仅大小写不同）→ ErrConflict，
// 且失败不修改任何账号。
func TestRenameUserConflict(t *testing.T) {
	s, aliceID, bobID := renameFixture(t)

	if err := s.RenameUser(aliceID, "bob"); !errors.Is(err, accounts.ErrConflict) {
		t.Fatalf("改名到已存在用户名 = %v, want ErrConflict", err)
	}
	// COLLATE NOCASE：大小写不同同样冲突（双向验证）
	if err := s.RenameUser(aliceID, "BOB"); !errors.Is(err, accounts.ErrConflict) {
		t.Fatalf("改名到 BOB（大小写不同）= %v, want ErrConflict", err)
	}
	if err := s.RenameUser(bobID, "ALICE"); !errors.Is(err, accounts.ErrConflict) {
		t.Fatalf("bob 改名到 ALICE = %v, want ErrConflict", err)
	}
	// 冲突失败后两个账号的用户名均未被改动
	if got := usernameOf(t, s, aliceID); got != "alice" {
		t.Fatalf("冲突后 alice 用户名 = %q, want alice", got)
	}
	if got := usernameOf(t, s, bobID); got != "bob" {
		t.Fatalf("冲突后 bob 用户名 = %q, want bob", got)
	}
	// 两个账号仍可各自登录（冲突未破坏密码/账号）
	if _, err := s.CheckPassword("alice", "pw-alice"); err != nil {
		t.Fatalf("冲突后 alice 登录失败: %v", err)
	}
	if _, err := s.CheckPassword("bob", "pw-bob"); err != nil {
		t.Fatalf("冲突后 bob 登录失败: %v", err)
	}
}

// TestRenameUserInvalidName 非法用户名被拒（返回校验错误文案）且不修改 DB；
// 边界 32 字符合法。
func TestRenameUserInvalidName(t *testing.T) {
	s, aliceID, _ := renameFixture(t)

	cases := []struct {
		name string
		in   string
	}{
		{"空串", ""},
		{"全空格", "   "},
		{"33 个字符", strings.Repeat("a", 33)},
		{"含控制字符", "bad\x01name"},
		{"含换行", "bad\nname"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := s.RenameUser(aliceID, tc.in)
			if err == nil {
				t.Fatalf("RenameUser(%q) = nil, want 校验错误", tc.in)
			}
			if !strings.Contains(err.Error(), "用户名") {
				t.Fatalf("校验错误文案 = %q, want 含「用户名」", err.Error())
			}
			if got := usernameOf(t, s, aliceID); got != "alice" {
				t.Fatalf("非法名改名后 DB 用户名 = %q, want alice（拒绝时不得改动）", got)
			}
		})
	}

	// 边界：32 字符（1–32 的上界）应成功
	ok32 := strings.Repeat("b", 32)
	if err := s.RenameUser(aliceID, ok32); err != nil {
		t.Fatalf("32 字符用户名应合法, got %v", err)
	}
	if got := usernameOf(t, s, aliceID); got != ok32 {
		t.Fatalf("32 字符改名后 DB 用户名 = %q, want %q", got, ok32)
	}
	// 33 字符（上界 +1）失败且不改库
	if err := s.RenameUser(aliceID, strings.Repeat("b", 33)); err == nil {
		t.Fatal("33 字符用户名应被拒")
	}
	if got := usernameOf(t, s, aliceID); got != ok32 {
		t.Fatalf("33 字符改名后 DB 用户名 = %q, want %q（应保持不变）", got, ok32)
	}
}

// TestRenameUserNotFound 不存在的 id → ErrNotFound，且不会创建任何账号。
func TestRenameUserNotFound(t *testing.T) {
	s, aliceID, _ := renameFixture(t)

	if err := s.RenameUser(999999, "ghost"); !errors.Is(err, accounts.ErrNotFound) {
		t.Fatalf("不存在 id 改名 = %v, want ErrNotFound", err)
	}
	if _, err := s.GetUserByUsername("ghost"); !errors.Is(err, accounts.ErrNotFound) {
		t.Fatalf("不存在 id 改名后查询 ghost err = %v, want ErrNotFound（不应创建账号）", err)
	}
	if got := usernameOf(t, s, aliceID); got != "alice" {
		t.Fatalf("不存在 id 改名后 alice 用户名 = %q, want alice", got)
	}
	// 负数/0 id 同样按不存在处理
	for _, id := range []int64{0, -1} {
		if err := s.RenameUser(id, "nobody"); !errors.Is(err, accounts.ErrNotFound) {
			t.Fatalf("id=%d 改名 = %v, want ErrNotFound", id, err)
		}
	}
}

// TestRenameUserSameNameIdempotent 同名改名幂等返回 nil（不视为冲突/未找到）。
func TestRenameUserSameNameIdempotent(t *testing.T) {
	s, aliceID, _ := renameFixture(t)

	if err := s.RenameUser(aliceID, "alice"); err != nil {
		t.Fatalf("同名改名 = %v, want nil（幂等）", err)
	}
	if got := usernameOf(t, s, aliceID); got != "alice" {
		t.Fatalf("同名改名后 DB 用户名 = %q, want alice", got)
	}
	if _, err := s.CheckPassword("alice", "pw-alice"); err != nil {
		t.Fatalf("同名改名后登录失败: %v", err)
	}
	// 带空白但裁剪后同名：同样视为成功
	if err := s.RenameUser(aliceID, "  alice  "); err != nil {
		t.Fatalf("空白包裹同名改名 = %v, want nil", err)
	}
	if got := usernameOf(t, s, aliceID); got != "alice" {
		t.Fatalf("空白包裹同名改名后 DB 用户名 = %q, want alice", got)
	}
}

// TestRenameUserRoleAgnostic RenameUser 不限角色：系统管理员/域管理员用户名同样可改，
// 且改名不改变角色与归属域。
func TestRenameUserRoleAgnostic(t *testing.T) {
	s := newTestAccounts(t)
	adminID, err := s.CreateUser("root", "pw-root", accounts.RoleGlobalAdmin)
	if err != nil {
		t.Fatal(err)
	}
	const domain = 7
	dadminID, err := s.CreateUser("dadmin", "pw-dadmin", accounts.RoleDomainAdmin, domain)
	if err != nil {
		t.Fatal(err)
	}

	if err := s.RenameUser(adminID, "root2"); err != nil {
		t.Fatalf("改系统管理员用户名 = %v, want nil", err)
	}
	admin, err := s.GetUserByID(adminID)
	if err != nil {
		t.Fatal(err)
	}
	if admin.Username != "root2" || admin.Role != accounts.RoleGlobalAdmin {
		t.Fatalf("改名后系统管理员 = %+v, want username=root2 role=global_admin", admin)
	}
	if admin.DomainID != nil {
		t.Fatalf("global_admin 归属域应为 nil, got %v", *admin.DomainID)
	}
	if _, err := s.CheckPassword("root2", "pw-root"); err != nil {
		t.Fatalf("改名后系统管理员登录失败: %v", err)
	}
	if has, err := s.HasAdmin(); err != nil || !has {
		t.Fatalf("HasAdmin = %v %v, want true（改名不应影响管理员存在性判定）", has, err)
	}

	if err := s.RenameUser(dadminID, "dadmin2"); err != nil {
		t.Fatalf("改域管理员用户名 = %v, want nil", err)
	}
	da, err := s.GetUserByID(dadminID)
	if err != nil {
		t.Fatal(err)
	}
	if da.Username != "dadmin2" || da.Role != accounts.RoleDomainAdmin {
		t.Fatalf("改名后域管理员 = %+v, want username=dadmin2 role=domain_admin", da)
	}
	if da.DomainID == nil || *da.DomainID != domain {
		t.Fatalf("改名后域管理员归属域 = %v, want %d（改名不应动 domain_id）", da.DomainID, domain)
	}
	// 域管理员列表按 domain_id 查询仍能取到（角色/域未受影响）
	admins, err := s.ListDomainAdmins(domain)
	if err != nil {
		t.Fatal(err)
	}
	if len(admins) != 1 || admins[0].ID != dadminID || admins[0].Username != "dadmin2" {
		t.Fatalf("ListDomainAdmins = %+v, want 单条 id=%d username=dadmin2", admins, dadminID)
	}
}

// TestRenameUserKeepsSessions 改名不清会话：既有 token 仍解析到同一 user_id 且返回新用户名；
// 与改密（清空全部会话）语义不同。
func TestRenameUserKeepsSessions(t *testing.T) {
	s, aliceID, _ := renameFixture(t)
	token, err := s.CreateSession(aliceID)
	if err != nil {
		t.Fatal(err)
	}
	if u, ok := s.GetUserByToken(token); !ok || u.Username != "alice" {
		t.Fatalf("改名前会话 = %+v ok=%v, want alice", u, ok)
	}

	if err := s.RenameUser(aliceID, "alice2"); err != nil {
		t.Fatalf("改名失败: %v", err)
	}
	u, ok := s.GetUserByToken(token)
	if !ok {
		t.Fatal("改名后旧会话应仍有效（会话按 user_id 关联，不存用户名）")
	}
	if u.ID != aliceID || u.Username != "alice2" {
		t.Fatalf("改名后会话用户 = %+v, want id=%d username=alice2", u, aliceID)
	}
	var n int
	if err := s.DB.QueryRow(`SELECT COUNT(1) FROM sessions WHERE user_id=?`, aliceID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("改名后会话行数 = %d, want 1（改名不得清理会话）", n)
	}

	// 对照：改密码会清空全部会话（现有语义，改名路径不应误用）
	if err := s.SetPassword(aliceID, "pw-new"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.GetUserByToken(token); ok {
		t.Fatal("改密后旧会话应失效（与改名区分）")
	}
}
