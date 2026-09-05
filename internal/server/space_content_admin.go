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
// 注：题目 domain 隔离全面落地前，模板仅校验存在性；后续收紧为同域校验。
func (s *Server) copyRepoIntoSpaceTraining(spaceTrainingID, domainID int64, kind string, repoID int64) error {
	if kind == "training" {
		if _, err := s.Store.GetTraining(repoID); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "仓库训练不存在")
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
	if _, err := s.Store.GetPractice(repoID); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "仓库练习不存在")
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
	var req struct {
		Title       string   `json:"title"`
		Description string   `json:"description"`
		Tags        []string `json:"tags"`
		MaxAttempts int      `json:"maxAttempts"`
	}
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	if err := s.Store.UpdateSpaceTrainingMeta(tid, req.Title, req.Description, req.Tags, req.MaxAttempts); err != nil {
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
	var req struct {
		ProblemIDs []int64 `json:"problemIds"`
	}
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	ids, err := s.Store.AddSpaceChapterItems(cid, req.ProblemIDs)
	if err != nil {
		return err
	}
	return respondData(c, fiber.StatusCreated, fiber.Map{"itemIds": ids})
}

// handleDeleteSpaceItem DELETE /api/space/items/:itemId（条目删除：训练章节或练习共用）
func (s *Server) handleDeleteSpaceItem(c *fiber.Ctx) error {
	itemID, err := paramID(c, "itemId")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid item id")
	}
	if err := s.Store.RemoveSpaceChapterItem(itemID); err != nil {
		if err == store.ErrNotFound {
			return respondError(c, fiber.StatusNotFound, "条目不存在")
		}
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
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
		var pids []int64
		if req.FromRepo.Kind == "training" {
			if _, err := s.Store.GetTraining(req.FromRepo.ID); err != nil {
				_ = s.Store.DeleteSpacePractice(id)
				return respondError(c, fiber.StatusBadRequest, "仓库训练不存在")
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
			if _, err := s.Store.GetPractice(req.FromRepo.ID); err != nil {
				_ = s.Store.DeleteSpacePractice(id)
				return respondError(c, fiber.StatusBadRequest, "仓库练习不存在")
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
	var req struct {
		ProblemIDs []int64 `json:"problemIds"`
	}
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	if err := s.Store.AddSpacePracticeItems(pid, req.ProblemIDs); err != nil {
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
	if err := s.Store.DeleteSpaceQuiz(qid); err != nil {
		if err == store.ErrNotFound {
			return respondError(c, fiber.StatusNotFound, "刷题项目不存在")
		}
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}
