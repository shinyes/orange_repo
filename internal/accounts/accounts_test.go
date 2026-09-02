package accounts_test

import (
	"testing"

	"golang.org/x/crypto/bcrypt"

	"orangerepo/internal/accounts"
)

func newTestAccounts(t *testing.T) *accounts.Store {
	t.Helper()
	db, err := accounts.OpenDB(t.TempDir())
	if err != nil {
		t.Fatalf("open accounts db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return accounts.New(db)
}

func TestUsers(t *testing.T) {
	s := newTestAccounts(t)
	adminID, err := s.CreateUser("Admin", "pw-admin", accounts.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser("alice", "pw-a", accounts.RoleStudent); err != nil {
		t.Fatal(err)
	}
	// 大小写不敏感唯一
	if _, err := s.CreateUser("ALICE", "pw-b", accounts.RoleStudent); err == nil {
		t.Fatal("重复用户名应冲突")
	}
	u, err := s.GetUserByUsername("aDmIn")
	if err != nil || u.ID != adminID {
		t.Fatalf("大小写不敏感查询失败: %v %+v", err, u)
	}
	// 登录校验
	lu, err := s.CheckPassword("alice", "pw-a")
	if err != nil || lu == nil || lu.Role != accounts.RoleStudent {
		t.Fatalf("alice 登录失败: %v", err)
	}
	if _, err := s.CheckPassword("alice", "wrong"); err == nil {
		t.Fatal("错误密码应失败")
	}
	// 管理员列表
	admins, err := s.ListAdmins()
	if err != nil || len(admins) != 1 || admins[0].Username != "Admin" {
		t.Fatalf("管理员列表 = %+v, err=%v", admins, err)
	}
	// 学生列表
	students, err := s.ListStudents()
	if err != nil {
		t.Fatal(err)
	}
	if len(students) != 1 || students[0].Username != "alice" {
		t.Fatalf("学生列表 = %+v", students)
	}
	// 重置学生密码
	if err := s.SetStudentPassword(students[0].ID, "pw-new"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CheckPassword("alice", "pw-new"); err != nil {
		t.Fatal("重置后新密码登录失败")
	}
	// 删除学生
	if err := s.DeleteStudent(students[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetUserByID(students[0].ID); err == nil {
		t.Fatal("删除后用户应不存在")
	}
	// 重置任意用户（管理员）密码 + 会话清空
	u2, _ := s.GetUserByUsername("admin")
	if err := s.SetUserPassword(u2.ID, "pw-admin2"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CheckPassword("admin", "pw-admin2"); err != nil {
		t.Fatal("管理员重置后登录失败")
	}
}

func TestSessions(t *testing.T) {
	s := newTestAccounts(t)
	id, err := s.CreateUser("bob", "pw", accounts.RoleStudent)
	if err != nil {
		t.Fatal(err)
	}
	token, err := s.CreateSession(id)
	if err != nil {
		t.Fatal(err)
	}
	u, ok := s.GetUserByToken(token)
	if !ok || u.ID != id {
		t.Fatalf("会话取用户失败: %v %v", u, ok)
	}
	if err := s.DeleteSession(token); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.GetUserByToken(token); ok {
		t.Fatal("已删除会话不应有效")
	}
	// 改密码清空全部会话
	t2, _ := s.CreateSession(id)
	if err := s.SetPassword(id, "pw2"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.GetUserByToken(t2); ok {
		t.Fatal("改密后旧会话应失效")
	}
}

// TestMigrateFromLegacyHash 旧版主站 settings 密码哈希迁移为 admin 账号后，原密码无缝可用。
func TestMigrateFromLegacyHash(t *testing.T) {
	s := newTestAccounts(t)
	legacyHash, err := bcrypt.GenerateFromPassword([]byte("old-password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateAdminFromHash("admin", string(legacyHash)); err != nil {
		t.Fatal(err)
	}
	if has, err := s.HasAdmin(); err != nil || !has {
		t.Fatalf("HasAdmin = %v %v", has, err)
	}
	u, err := s.CheckPassword("admin", "old-password")
	if err != nil || u == nil || u.Role != accounts.RoleAdmin {
		t.Fatalf("旧密码登录失败: %v", err)
	}
	// 重复引导容错
	if _, err := s.CreateAdminFromHash("admin", string(legacyHash)); err == nil {
		t.Fatal("重复创建 admin 应冲突")
	}
}