// 空间内容管理 API（主站仓库/空间管理侧）：空间训练/练习/刷题项目 的结构 CRUD
// 与「从仓库模板拷贝」。作答与进度见刷题服务（quizserver）。
package server

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"orangeoj/internal/store"
)

// requireSpaceAdminSession 当前用户可管理目标空间（从 URL spaceId 或 body 校验）——
// 空间内容管理入口统一挂 /api/space/:id/...（:id=空间 id），此处按 requireSpaceAccess 校验。
func (s *Server) spaceParam(c *fiber.Ctx) (int64, error) {
	return paramID(c, "id")
}

// requireResourceInSpace 校验空间子资源确属 URL :id 空间（防跨空间越权写；
// 资源 id 由调用方自选，须与 URL 空间一致，不一致视为不存在）。
func (s *Server) requireResourceInSpace(c *fiber.Ctx, urlSpaceID, actualSpaceID int64) error {
	if actualSpaceID != urlSpaceID {
		return respondError(c, fiber.StatusNotFound, "资源不存在或不属于该空间")
	}
	return nil
}

// ---------- 空间训练管理 ----------

// handleListSpaceTrainings GET /api/space/:id/trainings
func (s *Server) handleListSpaceTrainings(c *fiber.Ctx) error {
	spaceID, err := s.spaceParam(c)
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid space id")
	}
	user := currentUser(c)
	if err := s.requireSpaceAccess(c, user, spaceID); err != nil {
		return err
	}
	list, err := s.Store.ListSpaceTrainings(spaceID)
	if err != nil {
		return err
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"trainings": list})
}

// handleCreateSpaceTraining POST /api/space/:id/trainings
// {title, description?, tags?, maxAttempts?, fromRepo?{kind:'training'|'practice', id}?}
// fromRepo 提供时从仓库模板拷贝结构（章节/条目）。
func (s *Server) handleCreateSpaceTraining(c *fiber.Ctx) error {
	spaceID, err := s.spaceParam(c)
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid space id")
	}
	user := currentUser(c)
	if err := s.requireSpaceAccess(c, user, spaceID); err != nil {
		return err
	}
	var req struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Tags        []string `json:"tags"`
		MaxAttempts int    `json:"maxAttempts"`
		FromRepo    *struct {
			Kind string `json:"kind"` // training | practice
			ID   int64  `json:"id"`
		} `json:"fromRepo"`
	}
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	if strings.TrimSpace(req.Title) == "" {
		return respondError(c, fiber.StatusBadRequest, "标题不能为空")
	}
	domainID, err := s.Store.SpaceDomain(spaceID)
	if err != nil {
		return err
	}
	id, err := s.Store.CreateSpaceTraining(spaceID, req.Title, req.Description, req.Tags, req.MaxAttempts)
	if err != nil {
		return err
	}
	// 从仓库模板拷贝（模板须同域；客观题条目直接引用题目，结构复制章节）
	if req.FromRepo != nil && req.FromRepo.ID > 0 {
		if err := s.copyRepoIntoSpaceTraining(id, domainID, req.FromRepo.Kind, req.FromRepo.ID); err != nil {
			_ = s.Store.DeleteSpaceTraining(id)
			return respondError(c, fiber.StatusBadRequest, err.Error())
		}
	}
	return respondData(c, fiber.StatusCreated, fiber.Map{"id": id})
}

// copyRepoIntoSpaceTraining 从仓库（域模板库）训练/练习拷贝章节结构到空间训练。
// 模板须属于目标域（模板条目题目域判定）。
func (s *Server) copyRepoIntoSpaceTraining(spaceTrainingID, domainID int64, kind string, repoID int64) error {
	if kind == "training" {
		ok, err := s.Store.TrainingInDomain(repoID, domainID)
		if err != nil {
			return err
		}
		if !ok {
			return fiber.NewError(fiber.StatusBadRequest, "仓库训练不存在或不属于该域")
		}
		chapters, err := s.Store.ListChapters(repoID)
		if err != nil {
			return err
		}
		for _, ch := range chapters {
			chID, err := s.Store.CreateSpaceChapter(spaceTrainingID, ch.Title)
			if err != nil {
				return err
			}
			var pids []int64
			for _, it := range ch.Items {
				pids = append(pids, it.ProblemID)
			}
			if len(pids) > 0 {
				if _, err := s.Store.AddSpaceChapterItems(chID, pids); err != nil {
					return err
				}
			}
		}
		return nil
	}
	// practice 模板：平铺条目 → 单章「练习题目」
	ok, err := s.Store.PracticeInDomain(repoID, domainID)
	if err != nil {
		return err
	}
	if !ok {
		return fiber.NewError(fiber.StatusBadRequest, "仓库练习不存在或不属于该域")
	}
	items, err := s.Store.ListPracticeItems(repoID)
	if err != nil {
		return err
	}
	chID, err := s.Store.CreateSpaceChapter(spaceTrainingID, "练习题目")
	if err != nil {
		return err
	}
	var pids []int64
	for _, it := range items {
		pids = append(pids, it.ProblemID)
	}
	if len(pids) > 0 {
		if _, err := s.Store.AddSpaceChapterItems(chID, pids); err != nil {
			return err
		}
	}
	return nil
}

// handleGetSpaceTraining GET /api/space/:id/trainings/:tid（含章节；成员可读）
func (s *Server) handleGetSpaceTraining(c *fiber.Ctx) error {
	spaceID, err := s.spaceParam(c)
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid space id")
	}
	tid, err := paramID(c, "tid")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid training id")
	}
	tr, chapters, err := s.Store.GetSpaceTraining(tid)
	if err != nil {
		if err == store.ErrNotFound {
			return respondError(c, fiber.StatusNotFound, "训练不存在")
		}
		return err
	}
	if tr.SpaceID != spaceID {
		return respondError(c, fiber.StatusNotFound, "训练不存在")
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"training": tr, "chapters": chapters})
}

// handleUpdateSpaceTrainingMeta PUT /api/space/:id/trainings/:tid
func (s *Server) handleUpdateSpaceTrainingMeta(c *fiber.Ctx) error {
	spaceID, err := s.spaceParam(c)
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid space id")
	}
	tid, err := paramID(c, "tid")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid training id")
	}
	user := currentUser(c)
	if err := s.requireSpaceAccess(c, user, spaceID); err != nil {
		return err
	}
	// 资源归属校验（防跨空间改写）
	as, err := s.Store.SpaceIDOfTraining(tid)
	if err != nil {
		if err == store.ErrNotFound {
			return respondError(c, fiber.StatusNotFound, "训练不存在")
		}
		return err
	}
	if err := s.requireResourceInSpace(c, spaceID, as); err != nil {
		return err
	}
	var req struct {
		Title       *string  `json:"title"`
		Description *string  `json:"description"`
		Tags        []string `json:"tags"`
		MaxAttempts *int     `json:"maxAttempts"`
	}
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	// title 显式提供时不得为空
	if req.Title != nil && strings.TrimSpace(*req.Title) == "" {
		return respondError(c, fiber.StatusBadRequest, "标题不能为空")
	}
	var tagsPtr []string
	if req.Tags != nil {
		tagsPtr = req.Tags
	}
	if err := s.Store.UpdateSpaceTrainingMeta(tid, req.Title, req.Description, tagsPtr, req.MaxAttempts); err != nil {
		if err == store.ErrNotFound {
			return respondError(c, fiber.StatusNotFound, "训练不存在")
		}
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// handleDeleteSpaceTraining DELETE /api/space/:id/trainings/:tid
func (s *Server) handleDeleteSpaceTraining(c *fiber.Ctx) error {
	spaceID, err := s.spaceParam(c)
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid space id")
	}
	tid, err := paramID(c, "tid")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid training id")
	}
	user := currentUser(c)
	if err := s.requireSpaceAccess(c, user, spaceID); err != nil {
		return err
	}
	as, err := s.Store.SpaceIDOfTraining(tid)
	if err != nil {
		if err == store.ErrNotFound {
			return respondError(c, fiber.StatusNotFound, "训练不存在")
		}
		return err
	}
	if err := s.requireResourceInSpace(c, spaceID, as); err != nil {
		return err
	}
	if err := s.Store.DeleteSpaceTraining(tid); err != nil {
		if err == store.ErrNotFound {
			return respondError(c, fiber.StatusNotFound, "训练不存在")
		}
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// handleCreateSpaceChapter POST /api/space/:id/trainings/:tid/chapters {title}
func (s *Server) handleCreateSpaceChapter(c *fiber.Ctx) error {
	spaceID, err := s.spaceParam(c)
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid space id")
	}
	tid, err := paramID(c, "tid")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid training id")
	}
	user := currentUser(c)
	if err := s.requireSpaceAccess(c, user, spaceID); err != nil {
		return err
	}
	as, err := s.Store.SpaceIDOfTraining(tid)
	if err != nil {
		if err == store.ErrNotFound {
			return respondError(c, fiber.StatusNotFound, "训练不存在")
		}
		return err
	}
	if err := s.requireResourceInSpace(c, spaceID, as); err != nil {
		return err
	}
	var req struct {
		Title string `json:"title"`
	}
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	chID, err := s.Store.CreateSpaceChapter(tid, req.Title)
	if err != nil {
		return err
	}
	return respondData(c, fiber.StatusCreated, fiber.Map{"id": chID})
}

// handleAddSpaceChapterItems POST /api/space/:id/chapters/:cid/items {problemIds}
func (s *Server) handleAddSpaceChapterItems(c *fiber.Ctx) error {
	spaceID, err := s.spaceParam(c)
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid space id")
	}
	cid, err := paramID(c, "cid")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid chapter id")
	}
	user := currentUser(c)
	if err := s.requireSpaceAccess(c, user, spaceID); err != nil {
		return err
	}
	as, err := s.Store.SpaceIDOfChapter(cid)
	if err != nil {
		if err == store.ErrNotFound {
			return respondError(c, fiber.StatusNotFound, "章节不存在")
		}
		return err
	}
	if err := s.requireResourceInSpace(c, spaceID, as); err != nil {
		return err
	}
	var req struct {
		ProblemIDs []int64 `json:"problemIds"`
	}
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	// 域门禁：仅放行属于该空间域的题目（防跨域塞题）
	domainID, err := s.Store.SpaceDomain(spaceID)
	if err != nil {
		return err
	}
	allowed, allOK, err := s.Store.FilterProblemsInDomain(req.ProblemIDs, domainID)
	if err != nil {
		return err
	}
	if !allOK {
		return respondError(c, fiber.StatusBadRequest, "包含不属于该域的题目，已拒绝添加")
	}
	ids, err := s.Store.AddSpaceChapterItems(cid, allowed)
	if err != nil {
		return err
	}
	return respondData(c, fiber.StatusCreated, fiber.Map{"itemIds": ids})
}

// chapterGuard 解析 :cid 章节并做归属校验，返回空间 id（共用：重命名/删除/条目重排）。
func (s *Server) chapterGuard(c *fiber.Ctx) (spaceID, cid int64, err error) {
	cid, perr := paramID(c, "cid")
	if perr != nil {
		return 0, 0, respondError(c, fiber.StatusBadRequest, "invalid chapter id")
	}
	user := currentUser(c)
	as, serr := s.Store.SpaceIDOfChapter(cid)
	if serr != nil {
		if serr == store.ErrNotFound {
			return 0, 0, respondError(c, fiber.StatusNotFound, "章节不存在")
		}
		return 0, 0, serr
	}
	if aerr := s.requireSpaceAccess(c, user, as); aerr != nil {
		return 0, 0, aerr
	}
	return as, cid, nil
}

// handleRenameSpaceChapter PUT /api/space/chapters/:cid {title} → 章节重命名。
func (s *Server) handleRenameSpaceChapter(c *fiber.Ctx) error {
	_, cid, err := s.chapterGuard(c)
	if err != nil {
		return err
	}
	var req struct {
		Title string `json:"title"`
	}
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	if strings.TrimSpace(req.Title) == "" {
		return respondError(c, fiber.StatusBadRequest, "章节名称不能为空")
	}
	if err := s.Store.RenameSpaceChapter(cid, strings.TrimSpace(req.Title)); err != nil {
		if err == store.ErrNotFound {
			return respondError(c, fiber.StatusNotFound, "章节不存在")
		}
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// handleDeleteSpaceChapter DELETE /api/space/chapters/:cid → 删除章节（级联条目）。
func (s *Server) handleDeleteSpaceChapter(c *fiber.Ctx) error {
	_, cid, err := s.chapterGuard(c)
	if err != nil {
		return err
	}
	if err := s.Store.DeleteSpaceChapter(cid); err != nil {
		if err == store.ErrNotFound {
			return respondError(c, fiber.StatusNotFound, "章节不存在")
		}
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// handleReorderSpaceChapters PUT /api/space/trainings/:tid/chapters/order {chapterIds}
// → 按给定顺序重排章节。
func (s *Server) handleReorderSpaceChapters(c *fiber.Ctx) error {
	tid, err := paramID(c, "tid")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid training id")
	}
	user := currentUser(c)
	as, err := s.Store.SpaceIDOfTraining(tid)
	if err != nil {
		if err == store.ErrNotFound {
			return respondError(c, fiber.StatusNotFound, "训练不存在")
		}
		return err
	}
	if err := s.requireSpaceAccess(c, user, as); err != nil {
		return err
	}
	var req struct {
		ChapterIDs []int64 `json:"chapterIds"`
	}
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	if err := s.Store.ReorderSpaceChapters(tid, req.ChapterIDs); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// handleReorderSpaceChapterItems PUT /api/space/chapters/:cid/items/order {itemIds}
// → 按给定顺序重排章节内题目。
func (s *Server) handleReorderSpaceChapterItems(c *fiber.Ctx) error {
	_, cid, err := s.chapterGuard(c)
	if err != nil {
		return err
	}
	var req struct {
		ItemIDs []int64 `json:"itemIds"`
	}
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	if err := s.Store.ReorderSpaceChapterItems(cid, req.ItemIDs); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// handleDeleteSpaceItem DELETE /api/space/space-items/:itemId（空间条目删除：训练/练习通用，
// 按条目归属表自动路由）。
func (s *Server) handleDeleteSpaceItem(c *fiber.Ctx) error {
	itemID, err := paramID(c, "itemId")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid item id")
	}
	user := currentUser(c)
	// 归属校验（先判训练条目，再判练习条目），防跨空间越权删除
	spaceID, err := s.Store.SpaceIDOfTrainingItem(itemID)
	if err == nil {
		if err := s.requireSpaceAccess(c, user, spaceID); err != nil {
			return err
		}
		if err := s.Store.RemoveSpaceChapterItem(itemID); err != nil {
			return err
		}
		return c.SendStatus(fiber.StatusNoContent)
	}
	if err != store.ErrNotFound {
		return err
	}
	spaceID, err = s.Store.SpaceIDOfPracticeItem(itemID)
	if err == nil {
		if err := s.requireSpaceAccess(c, user, spaceID); err != nil {
			return err
		}
		if err := s.Store.RemoveSpacePracticeItem(itemID); err != nil {
			return err
		}
		return c.SendStatus(fiber.StatusNoContent)
	}
	if err != store.ErrNotFound {
		return err
	}
	return respondError(c, fiber.StatusNotFound, "条目不存在")
}

// ---------- 空间练习管理 ----------

// handleListSpacePractices GET /api/space/:id/practices
func (s *Server) handleListSpacePractices(c *fiber.Ctx) error {
	spaceID, err := s.spaceParam(c)
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid space id")
	}
	user := currentUser(c)
	if err := s.requireSpaceAccess(c, user, spaceID); err != nil {
		return err
	}
	list, err := s.Store.ListSpacePractices(spaceID)
	if err != nil {
		return err
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"practices": list})
}

// handleCreateSpacePractice POST /api/space/:id/practices
// {title, description?, tags?, fromRepo?{kind,id}?}
func (s *Server) handleCreateSpacePractice(c *fiber.Ctx) error {
	spaceID, err := s.spaceParam(c)
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid space id")
	}
	user := currentUser(c)
	if err := s.requireSpaceAccess(c, user, spaceID); err != nil {
		return err
	}
	var req struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Tags        []string `json:"tags"`
		FromRepo    *struct {
			Kind string `json:"kind"`
			ID   int64  `json:"id"`
		} `json:"fromRepo"`
	}
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	if strings.TrimSpace(req.Title) == "" {
		return respondError(c, fiber.StatusBadRequest, "标题不能为空")
	}
	id, err := s.Store.CreateSpacePractice(spaceID, req.Title, req.Description, req.Tags)
	if err != nil {
		return err
	}
	if req.FromRepo != nil && req.FromRepo.ID > 0 {
		// 模板须同域
		domainID, err := s.Store.SpaceDomain(spaceID)
		if err != nil {
			_ = s.Store.DeleteSpacePractice(id)
			return err
		}
		var pids []int64
		if req.FromRepo.Kind == "training" {
			ok, err := s.Store.TrainingInDomain(req.FromRepo.ID, domainID)
			if err != nil {
				_ = s.Store.DeleteSpacePractice(id)
				return err
			}
			if !ok {
				_ = s.Store.DeleteSpacePractice(id)
				return respondError(c, fiber.StatusBadRequest, "仓库训练不存在或不属于该域")
			}
			chapters, err := s.Store.ListChapters(req.FromRepo.ID)
			if err != nil {
				return err
			}
			for _, ch := range chapters {
				for _, it := range ch.Items {
					pids = append(pids, it.ProblemID)
				}
			}
		} else {
			ok, err := s.Store.PracticeInDomain(req.FromRepo.ID, domainID)
			if err != nil {
				_ = s.Store.DeleteSpacePractice(id)
				return err
			}
			if !ok {
				_ = s.Store.DeleteSpacePractice(id)
				return respondError(c, fiber.StatusBadRequest, "仓库练习不存在或不属于该域")
			}
			items, err := s.Store.ListPracticeItems(req.FromRepo.ID)
			if err != nil {
				return err
			}
			for _, it := range items {
				pids = append(pids, it.ProblemID)
			}
		}
		if len(pids) > 0 {
			if err := s.Store.AddSpacePracticeItems(id, pids); err != nil {
				_ = s.Store.DeleteSpacePractice(id)
				return err
			}
		}
	}
	return respondData(c, fiber.StatusCreated, fiber.Map{"id": id})
}

// handleGetSpacePractice GET /api/space/:id/practices/:pid
func (s *Server) handleGetSpacePractice(c *fiber.Ctx) error {
	spaceID, err := s.spaceParam(c)
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid space id")
	}
	pid, err := paramID(c, "pid")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid practice id")
	}
	p, items, err := s.Store.GetSpacePractice(pid)
	if err != nil {
		if err == store.ErrNotFound {
			return respondError(c, fiber.StatusNotFound, "练习不存在")
		}
		return err
	}
	if p.SpaceID != spaceID {
		return respondError(c, fiber.StatusNotFound, "练习不存在")
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"practice": p, "items": items})
}

// handleAddSpacePracticeItems POST /api/space/:id/practices/:pid/items {problemIds}
func (s *Server) handleAddSpacePracticeItems(c *fiber.Ctx) error {
	spaceID, err := s.spaceParam(c)
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid space id")
	}
	pid, err := paramID(c, "pid")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid practice id")
	}
	user := currentUser(c)
	if err := s.requireSpaceAccess(c, user, spaceID); err != nil {
		return err
	}
	as, err := s.Store.SpaceIDOfPractice(pid)
	if err != nil {
		if err == store.ErrNotFound {
			return respondError(c, fiber.StatusNotFound, "练习不存在")
		}
		return err
	}
	if err := s.requireResourceInSpace(c, spaceID, as); err != nil {
		return err
	}
	var req struct {
		ProblemIDs []int64 `json:"problemIds"`
	}
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	// 域门禁：仅放行属于该空间域的题目（防跨域塞题）
	domainID, err := s.Store.SpaceDomain(spaceID)
	if err != nil {
		return err
	}
	allowed, allOK, err := s.Store.FilterProblemsInDomain(req.ProblemIDs, domainID)
	if err != nil {
		return err
	}
	if !allOK {
		return respondError(c, fiber.StatusBadRequest, "包含不属于该域的题目，已拒绝添加")
	}
	if err := s.Store.AddSpacePracticeItems(pid, allowed); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// handleUpdateSpacePractice PUT /api/space/:id/practices/:pid
func (s *Server) handleUpdateSpacePractice(c *fiber.Ctx) error {
	spaceID, err := s.spaceParam(c)
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid space id")
	}
	pid, err := paramID(c, "pid")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid practice id")
	}
	user := currentUser(c)
	if err := s.requireSpaceAccess(c, user, spaceID); err != nil {
		return err
	}
	as, err := s.Store.SpaceIDOfPractice(pid)
	if err != nil {
		if err == store.ErrNotFound {
			return respondError(c, fiber.StatusNotFound, "练习不存在")
		}
		return err
	}
	if err := s.requireResourceInSpace(c, spaceID, as); err != nil {
		return err
	}
	var req struct {
		Title       string   `json:"title"`
		Description string   `json:"description"`
		Tags        []string `json:"tags"`
	}
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	if err := s.Store.UpdateSpacePracticeMeta(pid, req.Title, req.Description, req.Tags); err != nil {
		if err == store.ErrNotFound {
			return respondError(c, fiber.StatusNotFound, "练习不存在")
		}
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// handleDeleteSpacePractice DELETE /api/space/:id/practices/:pid
func (s *Server) handleDeleteSpacePractice(c *fiber.Ctx) error {
	spaceID, err := s.spaceParam(c)
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid space id")
	}
	pid, err := paramID(c, "pid")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid practice id")
	}
	user := currentUser(c)
	if err := s.requireSpaceAccess(c, user, spaceID); err != nil {
		return err
	}
	as, err := s.Store.SpaceIDOfPractice(pid)
	if err != nil {
		if err == store.ErrNotFound {
			return respondError(c, fiber.StatusNotFound, "练习不存在")
		}
		return err
	}
	if err := s.requireResourceInSpace(c, spaceID, as); err != nil {
		return err
	}
	if err := s.Store.DeleteSpacePractice(pid); err != nil {
		if err == store.ErrNotFound {
			return respondError(c, fiber.StatusNotFound, "练习不存在")
		}
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// ---------- 空间刷题项目管理 ----------

// handleListSpaceQuizzes GET /api/space/:id/quizzes
func (s *Server) handleListSpaceQuizzes(c *fiber.Ctx) error {
	spaceID, err := s.spaceParam(c)
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid space id")
	}
	user := currentUser(c)
	if err := s.requireSpaceAccess(c, user, spaceID); err != nil {
		return err
	}
	list, err := s.Store.ListSpaceQuizzes(spaceID)
	if err != nil {
		return err
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"quizzes": list})
}

// handleCreateSpaceQuiz POST /api/space/:id/quizzes
// {title, tags?[], sourceType:'tags'|'repo', repoKind?, repoId?}
func (s *Server) handleCreateSpaceQuiz(c *fiber.Ctx) error {
	spaceID, err := s.spaceParam(c)
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid space id")
	}
	user := currentUser(c)
	if err := s.requireSpaceAccess(c, user, spaceID); err != nil {
		return err
	}
	var req struct {
		Title      string   `json:"title"`
		Tags       []string `json:"tags"`
		SourceType string   `json:"sourceType"`
		RepoKind   string   `json:"repoKind"`
		RepoID     int64    `json:"repoId"`
	}
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	if strings.TrimSpace(req.Title) == "" {
		return respondError(c, fiber.StatusBadRequest, "标题不能为空")
	}
	if req.SourceType != "tags" && req.SourceType != "repo" {
		return respondError(c, fiber.StatusBadRequest, "sourceType 须为 tags 或 repo")
	}
	id, err := s.Store.CreateSpaceQuiz(spaceID, req.Title, req.Tags, req.SourceType, req.RepoKind, req.RepoID)
	if err != nil {
		return err
	}
	return respondData(c, fiber.StatusCreated, fiber.Map{"id": id})
}

// handleDeleteSpaceQuiz DELETE /api/space/:id/quizzes/:qid
func (s *Server) handleDeleteSpaceQuiz(c *fiber.Ctx) error {
	spaceID, err := s.spaceParam(c)
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid space id")
	}
	qid, err := paramID(c, "qid")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid quiz id")
	}
	user := currentUser(c)
	if err := s.requireSpaceAccess(c, user, spaceID); err != nil {
		return err
	}
	as, err := s.Store.SpaceIDOfQuiz(qid)
	if err != nil {
		if err == store.ErrNotFound {
			return respondError(c, fiber.StatusNotFound, "刷题项目不存在")
		}
		return err
	}
	if err := s.requireResourceInSpace(c, spaceID, as); err != nil {
		return err
	}
	if err := s.Store.DeleteSpaceQuiz(qid); err != nil {
		if err == store.ErrNotFound {
			return respondError(c, fiber.StatusNotFound, "刷题项目不存在")
		}
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}
