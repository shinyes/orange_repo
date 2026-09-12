// 空间默认编程语言（spaces.default_lang）：
//  1. 存量库补列（旧 spaces 表无该列 → Open 时 ensureColumn，取值 ''=未设置）；
//  2. UpdateSpaceMeta 部分更新语义（name/defaultLang 各自可选）与取值校验。
package store

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// TestMigrateSpaceDefaultLangColumn 存量库补列：升级后列存在、存量空间取值为 ''（未设置）。
func TestMigrateSpaceDefaultLangColumn(t *testing.T) {
	dir := t.TempDir()
	dbPath := "file:" + filepath.ToSlash(filepath.Join(dir, "orangeoj.db"))
	legacy, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	// 旧 schema：spaces 无 default_lang 列（引入该设置前的库）
	oldSchema := []string{
		`CREATE TABLE domains (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL UNIQUE, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE TABLE spaces (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			domain_id INTEGER NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
			name TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
		`INSERT INTO domains(id,name) VALUES(1,'旧域')`,
		`INSERT INTO spaces(id,domain_id,name) VALUES(1,1,'旧空间')`,
	}
	for _, q := range oldSchema {
		if _, err := legacy.Exec(q); err != nil {
			t.Fatalf("legacy schema: %v; stmt: %s", err, q)
		}
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := Open(dir)
	if err != nil {
		t.Fatalf("migrate open: %v", err)
	}
	defer s.Close()

	var cols int
	if err := s.DB.QueryRow(`SELECT COUNT(1) FROM pragma_table_info('spaces') WHERE name='default_lang'`).Scan(&cols); err != nil {
		t.Fatal(err)
	}
	if cols != 1 {
		t.Fatal("spaces.default_lang 列应已补齐")
	}

	// 存量空间：未设置 → ''（前端据此沿用 python）
	sp, err := s.GetSpace(1)
	if err != nil {
		t.Fatalf("GetSpace: %v", err)
	}
	if sp.DefaultLang != "" {
		t.Fatalf("存量空间 default_lang = %q, want ''（未设置）", sp.DefaultLang)
	}
	list, err := s.ListSpaces(1)
	if err != nil {
		t.Fatalf("ListSpaces: %v", err)
	}
	if len(list) != 1 || list[0].DefaultLang != "" {
		t.Fatalf("ListSpaces = %+v, want 单空间且 defaultLang 为空", list)
	}
}

// TestUpdateSpaceMetaDefaultLang 部分更新：仅更新提供的字段；非法取值被拒。
func TestUpdateSpaceMetaDefaultLang(t *testing.T) {
	s := newTestStore(t)
	domainID, err := s.CreateDomain("语言域")
	if err != nil {
		t.Fatal(err)
	}
	spaceID, err := s.CreateSpace(domainID, "语言空间")
	if err != nil {
		t.Fatal(err)
	}

	// 仅 defaultLang：名称不变
	cpp := SpaceDefaultLangCpp
	if err := s.UpdateSpaceMeta(spaceID, nil, &cpp); err != nil {
		t.Fatalf("UpdateSpaceMeta(defaultLang=cpp): %v", err)
	}
	sp, err := s.GetSpace(spaceID)
	if err != nil {
		t.Fatal(err)
	}
	if sp.DefaultLang != "cpp" || sp.Name != "语言空间" {
		t.Fatalf("更新后 space = %+v, want name=语言空间 defaultLang=cpp", sp)
	}

	// 非法取值：报错且库中不变
	bad := "java"
	if err := s.UpdateSpaceMeta(spaceID, nil, &bad); err == nil {
		t.Fatal("非法 defaultLang 应报错")
	}
	if sp, _ = s.GetSpace(spaceID); sp.DefaultLang != "cpp" {
		t.Fatalf("非法更新后 defaultLang = %q, want cpp（保持原值）", sp.DefaultLang)
	}

	// 仅 name（旧调用语义：RenameSpace 等价于只传 name）：defaultLang 保持
	if err := s.RenameSpace(spaceID, " 新名字 "); err != nil {
		t.Fatalf("RenameSpace: %v", err)
	}
	if sp, _ = s.GetSpace(spaceID); sp.Name != "新名字" || sp.DefaultLang != "cpp" {
		t.Fatalf("改名后 space = %+v, want name=新名字 defaultLang=cpp", sp)
	}

	// 两者同时更新
	name := "双改"
	py := SpaceDefaultLangPython
	if err := s.UpdateSpaceMeta(spaceID, &name, &py); err != nil {
		t.Fatalf("UpdateSpaceMeta(name+defaultLang): %v", err)
	}
	if sp, _ = s.GetSpace(spaceID); sp.Name != "双改" || sp.DefaultLang != "python" {
		t.Fatalf("双改后 space = %+v, want name=双改 defaultLang=python", sp)
	}

	// 恢复未设置（'' 合法）
	unset := SpaceDefaultLangUnset
	if err := s.UpdateSpaceMeta(spaceID, nil, &unset); err != nil {
		t.Fatalf("UpdateSpaceMeta(defaultLang=''): %v", err)
	}
	if sp, _ = s.GetSpace(spaceID); sp.DefaultLang != "" {
		t.Fatalf("恢复未设置后 defaultLang = %q, want ''", sp.DefaultLang)
	}

	// 空字段 / 空名称 / 不存在的空间
	if err := s.UpdateSpaceMeta(spaceID, nil, nil); err == nil {
		t.Fatal("无更新字段应报错")
	}
	empty := "   "
	if err := s.UpdateSpaceMeta(spaceID, &empty, nil); err == nil {
		t.Fatal("空名称应报错")
	}
	if err := s.UpdateSpaceMeta(spaceID+999, nil, &cpp); err != ErrNotFound {
		t.Fatalf("不存在空间错误 = %v, want ErrNotFound", err)
	}

	// 取值白名单
	for _, v := range []string{"", "python", "cpp"} {
		if !ValidSpaceDefaultLang(v) {
			t.Fatalf("ValidSpaceDefaultLang(%q) = false, want true", v)
		}
	}
	for _, v := range []string{"java", "Python", "c++", "py"} {
		if ValidSpaceDefaultLang(v) {
			t.Fatalf("ValidSpaceDefaultLang(%q) = true, want false", v)
		}
	}
}
