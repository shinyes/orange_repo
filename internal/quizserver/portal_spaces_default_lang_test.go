// GET /api/portal/spaces 下发空间默认编程语言（spaces.default_lang）：
// 成员（UserDomainSpaceIDs）、域管理员（spacesOfDomain）、系统管理员（spacesOfAllDomains）
// 三条查询路径都必须带 defaultLang（前端做题页据此选默认语言；''=未设置）。
package quizserver

import "testing"

// defaultLangOfSpace 取空间条目的 defaultLang（字段缺失即失败：前端依赖该键）。
func defaultLangOfSpace(t *testing.T, list []map[string]any, spaceID int64) string {
	t.Helper()
	for _, sp := range list {
		if int64(sp["id"].(float64)) != spaceID {
			continue
		}
		v, ok := sp["defaultLang"]
		if !ok {
			t.Fatalf("空间 %d 响应缺少 defaultLang 键: %v", spaceID, sp)
		}
		s, ok := v.(string)
		if !ok {
			t.Fatalf("defaultLang 类型 = %T (%v)，want string", v, v)
		}
		return s
	}
	t.Fatalf("空间列表未含空间 %d: %v", spaceID, list)
	return ""
}

// setSpaceDefaultLang 直接改主库空间默认语言（模拟管理员 PATCH defaultLang）。
func (e *spacesLeaderboardEnv) setSpaceDefaultLang(t *testing.T, lang string) {
	t.Helper()
	if _, err := e.main.DB.Exec(`UPDATE spaces SET default_lang=? WHERE id=?`, lang, e.space); err != nil {
		t.Fatal(err)
	}
}

// TestPortalSpacesDefaultLang 空间列表三条查询路径均下发 defaultLang，且随设置变化。
func TestPortalSpacesDefaultLang(t *testing.T) {
	env := newSpacesLeaderboardEnv(t)

	// ---- 1. 默认未设置：''（前端沿用 python）----
	for _, c := range []struct {
		name   string
		cookie string
	}{{"member", env.stuCookie}, {"domain_admin", env.dAdmCookie}, {"global_admin", env.gAdmCookie}} {
		if got := defaultLangOfSpace(t, env.spaces(t, c.cookie), env.space); got != "" {
			t.Fatalf("%s 未设置时 defaultLang = %q，want ''", c.name, got)
		}
	}

	// ---- 2. 设为 cpp：三类账号均可见 ----
	env.setSpaceDefaultLang(t, "cpp")
	for _, c := range []struct {
		name   string
		cookie string
	}{{"member", env.stuCookie}, {"domain_admin", env.dAdmCookie}, {"global_admin", env.gAdmCookie}} {
		if got := defaultLangOfSpace(t, env.spaces(t, c.cookie), env.space); got != "cpp" {
			t.Fatalf("%s 设置后 defaultLang = %q，want cpp", c.name, got)
		}
	}

	// ---- 3. 改回未设置（''）----
	env.setSpaceDefaultLang(t, "")
	if got := defaultLangOfSpace(t, env.spaces(t, env.stuCookie), env.space); got != "" {
		t.Fatalf("恢复未设置后 defaultLang = %q，want ''", got)
	}
}
