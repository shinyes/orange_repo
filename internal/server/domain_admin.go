// 域管理 + 空间管理 API（OJ 重构）。
// 权限：域 CRUD/设域管理员 → 仅系统管理员（global_admin）；
//       空间 CRUD/成员管理 → 系统管理员或域管理员（后者限本域）。
package server

import (
	"errors"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"orangeoj/internal/accounts"
	"orangeoj/internal/model"
	"orangeoj/internal/store"
)

// domainScope 解析请求的域作用域：
//   - domain_admin：强制其归属域
//   - global_admin：优先 query/body 的 domainId，缺省报错
// 返回 nil 表示无有效域作用域（调用方按 400 处理）。
func (s *Server) domainScope(c *fiber.Ctx, user *accounts.User) (*int64, error) {
	if user.Role == accounts.RoleDomainAdmin {
		if user.DomainID == nil {
			return nil, errors.New("域管理员未关联域")
		}
		return user.DomainID, nil
	}
	if user.Role == accounts.RoleGlobalAdmin {
		raw := strings.TrimSpace(c.Query("domainId"))
		if raw == "" {
			return nil, errors.New("missing domainId")
		}
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			return nil, errors.New("invalid domainId")
		}
		return &id, nil
	}
	return nil, errors.New("无权访问")
}

// domainOrDefault 解析请求的域作用域（同 domainScope），global_admin 未显式带 domainId 时
// 回退到「默认域」（承接存量单域部署/旧调用；前端多域总会显式带）。
func (s *Server) domainOrDefault(c *fiber.Ctx, user *accounts.User) (*int64, error) {
	if user.Role == accounts.RoleDomainAdmin {
		if user.DomainID == nil {
			return nil, errors.New("域管理员未关联域")
		}
		return user.DomainID, nil
	}
	if user.Role == accounts.RoleGlobalAdmin {
		raw := strings.TrimSpace(c.Query("domainId"))
		if raw != "" {
			id, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || id <= 0 {
				return nil, errors.New("invalid domainId")
			}
			return &id, nil
		}
		// 回退默认域（不存在则自动创建——空库首题场景）
		var id int64
		err := s.Store.DB.QueryRow(`SELECT id FROM domains WHERE name=? ORDER BY id LIMIT 1`, store.DefaultDomainName).Scan(&id)
		if err != nil {
			nid, cerr := s.Store.CreateDomain(store.DefaultDomainName)
			if cerr != nil {
				return nil, errors.New("尚未创建任何域")
			}
			id = nid
		}
		return &id, nil
	}
	return nil, errors.New("无权访问")
}

// ---------- 域管理（global_admin） ----------

// handleListDomains GET /api/admin/domains → 域列表（global_admin 全部；
// domain_admin 仅返回其归属域——供管理页显示当前域名）。
func (s *Server) handleListDomains(c *fiber.Ctx) error {
	var domains []model.Domain
	var err error
	user := currentUser(c)
	if user.Role == accounts.RoleDomainAdmin {
		if user.DomainID == nil {
			return respondError(c, fiber.StatusForbidden, "域管理员未关联域")
		}
		var d *model.Domain
		d, err = s.Store.GetDomain(*user.DomainID)
		if err == nil {
			domains = []model.Domain{*d}
		}
	} else {
		domains, err = s.Store.ListDomains()
	}
	if err != nil {
		return err
	}
	type view struct {
		model.Domain
		SpaceCount   int `json:"spaceCount"`
		ProblemCount int `json:"problemCount"`
	}
	out := make([]view, 0, len(domains))
	for _, d := range domains {
		v := view{Domain: d}
		_ = s.Store.DB.QueryRow(`SELECT COUNT(1) FROM spaces WHERE domain_id=?`, d.ID).Scan(&v.SpaceCount)
		_ = s.Store.DB.QueryRow(`SELECT COUNT(1) FROM problems WHERE domain_id=?`, d.ID).Scan(&v.ProblemCount)
		out = append(out, v)
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"domains": out})
}

// handleCreateDomain POST /api/admin/domains {name, adminUsername?, adminPassword?}
// 建域并可同时创建初始域管理员账号。
func (s *Server) handleCreateDomain(c *fiber.Ctx) error {
	var req struct {
		Name          string `json:"name"`
		AdminUsername string `json:"adminUsername"`
		AdminPassword string `json:"adminPassword"`
	}
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	id, err := s.Store.CreateDomain(req.Name)
	if err != nil {
		if err.Error() == "域名称已存在" {
			return respondError(c, fiber.StatusConflict, err.Error())
		}
		return err
	}
	// 可选：建初始域管理员
	if req.AdminUsername != "" {
		if req.AdminPassword == "" {
			_ = s.Store.DeleteDomain(id)
			return respondError(c, fiber.StatusBadRequest, "域管理员密码不能为空")
		}
		if _, err := s.Accounts.CreateUser(req.AdminUsername, req.AdminPassword, accounts.RoleDomainAdmin, id); err != nil {
			_ = s.Store.DeleteDomain(id)
			if strings.Contains(err.Error(), "已存在") || strings.Contains(err.Error(), "UNIQUE") {
				return respondError(c, fiber.StatusConflict, "用户名已存在")
			}
			return err
		}
	}
	return respondData(c, fiber.StatusCreated, fiber.Map{"id": id})
}

// handleRenameDomain PATCH /api/admin/domains/:id 域元信息部分更新：
// body {name?, leaderboardPublic?}——仅更新请求中出现的字段（旧调用只传 name 保持兼容）。
func (s *Server) handleRenameDomain(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid id")
	}
	var req struct {
		Name              *string `json:"name"`
		LeaderboardPublic *bool   `json:"leaderboardPublic"`
	}
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	if req.Name == nil && req.LeaderboardPublic == nil {
		return respondError(c, fiber.StatusBadRequest, "缺少更新字段（name 或 leaderboardPublic）")
	}
	if req.Name != nil && strings.TrimSpace(*req.Name) == "" {
		return respondError(c, fiber.StatusBadRequest, "域名称不能为空")
	}
	if err := s.Store.UpdateDomainMeta(id, req.Name, req.LeaderboardPublic); err != nil {
		if err == store.ErrNotFound {
			return respondError(c, fiber.StatusNotFound, "域不存在")
		}
		if err.Error() == "域名称已存在" {
			return respondError(c, fiber.StatusConflict, err.Error())
		}
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// handleDeleteDomain DELETE /api/admin/domains/:id?deleteProblems=true
// 删域：默认仅当域内无题目/空间；deleteProblems=true 级联删除域内题目。
func (s *Server) handleDeleteDomain(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid id")
	}
	// 收集域内题目 id/uuid（供刷题侧数据清理；无论是否强制删除都需要）
	rows, err := s.Store.DB.Query(`SELECT id, COALESCE(uuid,'') FROM problems WHERE domain_id=?`, id)
	if err != nil {
		return err
	}
	var pids []int64
	var uuids []string
	for rows.Next() {
		var pid int64
		var u string
		if err := rows.Scan(&pid, &u); err != nil {
			rows.Close()
			return err
		}
		pids = append(pids, pid)
		if u != "" {
			uuids = append(uuids, u)
		}
	}
	rows.Close()
	if c.Query("deleteProblems") == "true" {
		// 删除前收集「仅含该域题」的仓库模板（删题后变空壳，应随之删除；不碰他域/空模板）
		trainingIDs, practiceIDs, err := s.Store.DomainOnlyBookletIDs(id)
		if err != nil {
			return err
		}
		// 级联删除域内题目（空间/条目引用一并清理）
		if err := s.Store.DeleteDomainProblems(id); err != nil {
			return err
		}
		if err := s.Store.DeleteBookletIDs(trainingIDs, practiceIDs); err != nil {
			return err
		}
	} else if len(pids) > 0 {
		return respondError(c, fiber.StatusConflict, "域内仍有题目，无法删除（可加 ?deleteProblems=true 强制）")
	}
	if err := s.Store.DeleteDomain(id); err != nil {
		if err == store.ErrNotFound {
			return respondError(c, fiber.StatusNotFound, "域不存在")
		}
		return err
	}
	// 域管理员的 domain_id 置空并降级为 member（账号保留）
	if err := s.Accounts.ClearDomainAdmins(id); err != nil {
		return err
	}
	// 刷题侧数据全清（域与空间已删，孤儿会话/练习快照在此阶段一并清理）：
	// 判题历史/进度/草稿/错题集/通过记录/会话/交卷快照 中引用该域题的数据；失败仅告警
	if s.QuizStore != nil {
		if err := s.QuizStore.CleanupDomainProblems(pids, uuids); err != nil {
			warnCleanup("domain-delete", err)
		}
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// handleListDomainAdmins GET /api/admin/domains/:id/admins → 该域全部域管理员。
func (s *Server) handleListDomainAdmins(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid id")
	}
	admins, err := s.Accounts.ListDomainAdmins(id)
	if err != nil {
		return err
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"admins": admins})
}

// handleRemoveDomainAdmin DELETE /api/admin/domains/:id/admins/:uid → 降级为普通成员。
func (s *Server) handleRemoveDomainAdmin(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid id")
	}
	uid, err := paramID(c, "uid")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid user id")
	}
	if _, err := s.Store.GetDomain(id); err != nil {
		return respondError(c, fiber.StatusNotFound, "域不存在")
	}
	// 归属校验：只能降级属于该域的域管理员（防跨域降级他人管理员）
	u, err := s.Accounts.GetUserByID(uid)
	if err != nil || u.Role != accounts.RoleDomainAdmin {
		return respondError(c, fiber.StatusNotFound, "该用户不是域管理员")
	}
	if u.DomainID == nil || *u.DomainID != id {
		return respondError(c, fiber.StatusNotFound, "该用户不是此域的域管理员")
	}
	if err := s.Accounts.RemoveDomainAdmin(uid); err != nil {
		if err == accounts.ErrNotFound {
			return respondError(c, fiber.StatusNotFound, "该用户不是域管理员")
		}
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// handleSetDomainAdmin PUT /api/admin/domains/:id/admin {username, password?}
// 指定/更换域管理员：用户已存在则升级为域管理员（原 member/其他域角色校验）；
// 用户不存在且提供 password 则直接创建。
func (s *Server) handleSetDomainAdmin(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid id")
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" {
		return respondError(c, fiber.StatusBadRequest, "缺少用户名")
	}
	if _, err := s.Store.GetDomain(id); err != nil {
		return respondError(c, fiber.StatusNotFound, "域不存在")
	}
	_, getErr := s.Accounts.GetUserByUsername(req.Username)
	if getErr != nil {
		if !errors.Is(getErr, accounts.ErrNotFound) {
			return getErr
		}
		// 用户不存在：需密码创建为域管理员
		if req.Password == "" {
			return respondError(c, fiber.StatusNotFound, "用户不存在，请提供密码以创建域管理员")
		}
		if _, err := s.Accounts.CreateUser(req.Username, req.Password, accounts.RoleDomainAdmin, id); err != nil {
			if errors.Is(err, accounts.ErrConflict) {
				return respondError(c, fiber.StatusConflict, "用户名已存在")
			}
			return err
		}
		return c.SendStatus(fiber.StatusNoContent)
	}
	if err := s.SetUserDomain(req.Username, id); err != nil {
		if errors.Is(err, accounts.ErrConflict) {
			return respondError(c, fiber.StatusConflict, "该用户已是其他域管理员或系统管理员")
		}
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// SetUserDomain 将用户设为某域的域管理员（创建或角色变更）：
//   - 用户不存在 → 由调用方先建（此处返回 ErrNotFound）
//   - 已是该域 domain_admin → 幂等
//   - 已是其他域 domain_admin 或 global_admin → ErrConflict（提示先移除）
func (s *Server) SetUserDomain(username string, domainID int64) error {
	u, err := s.Accounts.GetUserByUsername(username)
	if err != nil {
		return err
	}
	switch u.Role {
	case accounts.RoleDomainAdmin:
		if u.DomainID != nil && *u.DomainID == domainID {
			return nil // 幂等
		}
		return accounts.ErrConflict
	case accounts.RoleGlobalAdmin:
		return accounts.ErrConflict
	}
	// member 或其他 → 升级为域管理员
	_, err = s.Accounts.SetUserRole(username, accounts.RoleDomainAdmin, &domainID)
	return err
}

// ---------- 空间管理（global_admin / domain_admin） ----------

// handleListSpaces GET /api/admin/spaces?domainId= → 域内空间（含成员数/题量概览由前端另取）。
func (s *Server) handleListSpaces(c *fiber.Ctx) error {
	user := currentUser(c)
	scope, err := s.domainScope(c, user)
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	spaces, err := s.Store.ListSpaces(*scope)
	if err != nil {
		return err
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"spaces": spaces})
}

// handleCreateSpace POST /api/admin/spaces?domainId= {name}。
func (s *Server) handleCreateSpace(c *fiber.Ctx) error {
	user := currentUser(c)
	scope, err := s.domainScope(c, user)
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	id, err := s.Store.CreateSpace(*scope, req.Name)
	if err != nil {
		return err
	}
	return respondData(c, fiber.StatusCreated, fiber.Map{"id": id})
}

// handleRenameSpace PATCH /api/admin/spaces/:id {name}。
func (s *Server) handleRenameSpace(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid id")
	}
	user := currentUser(c)
	if err := s.requireSpaceAccess(c, user, id); err != nil {
		return err
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	if err := s.Store.RenameSpace(id, req.Name); err != nil {
		if err == store.ErrNotFound {
			return respondError(c, fiber.StatusNotFound, "空间不存在")
		}
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// handleDeleteSpace DELETE /api/admin/spaces/:id（级联删空间数据）。
func (s *Server) handleDeleteSpace(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid id")
	}
	user := currentUser(c)
	if err := s.requireSpaceAccess(c, user, id); err != nil {
		return err
	}
	// 删除前收集空间下内容 id（删除级联后无法再查），供作答侧数据清理
	quizIDs, trainingIDs, practiceIDs, err := s.Store.SpaceContentIDs(id)
	if err != nil {
		return err
	}
	if err := s.Store.DeleteSpace(id); err != nil {
		if err == store.ErrNotFound {
			return respondError(c, fiber.StatusNotFound, "空间不存在")
		}
		return err
	}
	// 作答侧数据清理（错题集/会话/尝试/交卷记录，均无 FK 级联）；失败仅告警不阻塞删除
	if s.QuizStore != nil {
		for _, qz := range quizIDs {
			if err := s.QuizStore.RemoveQuizData(qz); err != nil {
				warnCleanup("space-delete-quiz", err)
			}
		}
		for _, tr := range trainingIDs {
			if err := s.QuizStore.RemoveTrainingData(tr); err != nil {
				warnCleanup("space-delete-training", err)
			}
		}
		for _, pr := range practiceIDs {
			if err := s.QuizStore.RemovePracticeData(pr); err != nil {
				warnCleanup("space-delete-practice", err)
			}
		}
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// requireSpaceAccess 校验当前用户可管理该空间（global_admin 任意；domain_admin 限本域）。
// 失败返回 *fiber.Error（ErrorHandler 统一 JSON），确保调用方 err!=nil 即中断。
func (s *Server) requireSpaceAccess(c *fiber.Ctx, user *accounts.User, spaceID int64) error {
	if user == nil {
		return fiber.NewError(fiber.StatusForbidden, "需要管理员权限")
	}
	if user.Role == accounts.RoleDomainAdmin {
		if user.DomainID == nil {
			return fiber.NewError(fiber.StatusForbidden, "域管理员未关联域")
		}
		ok, err := s.Store.SpaceOfDomain(spaceID, *user.DomainID)
		if err != nil {
			return err
		}
		if !ok {
			return fiber.NewError(fiber.StatusForbidden, "无权访问该空间")
		}
		return nil
	}
	if user.Role == accounts.RoleGlobalAdmin {
		return nil
	}
	return fiber.NewError(fiber.StatusForbidden, "需要管理员权限")
}

// handleListSpaceMembers GET /api/admin/spaces/:id/members → 成员列表（user_id + username）。
func (s *Server) handleListSpaceMembers(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid id")
	}
	user := currentUser(c)
	if err := s.requireSpaceAccess(c, user, id); err != nil {
		return err
	}
	ids, err := s.Store.SpaceMemberIDs(id)
	if err != nil {
		return err
	}
	members := make([]model.SpaceMemberView, 0, len(ids))
	for _, uid := range ids {
		u, err := s.Accounts.GetUserByID(uid)
		if err != nil {
			continue // 账号已删的悬挂成员跳过
		}
		members = append(members, model.SpaceMemberView{UserID: uid, Username: u.Username})
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"members": members})
}

// handleSetSpaceMembers PUT /api/admin/spaces/:id/members {userIds: []int64} → 覆盖设置。
func (s *Server) handleSetSpaceMembers(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid id")
	}
	user := currentUser(c)
	if err := s.requireSpaceAccess(c, user, id); err != nil {
		return err
	}
	var req struct {
		UserIDs []int64 `json:"userIds"`
	}
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	// 成员须为 member 角色账号
	for _, uid := range req.UserIDs {
		u, err := s.Accounts.GetUserByID(uid)
		if err != nil || u.Role != accounts.RoleMember {
			return respondError(c, fiber.StatusBadRequest, "成员须为普通用户账号")
		}
	}
	if err := s.Store.SetSpaceMembers(id, req.UserIDs); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}
