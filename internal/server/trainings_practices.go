package server

import (
	"errors"

	"github.com/gofiber/fiber/v2"

	"orangerepo/internal/model"
	"orangerepo/internal/store"
)

// ---------- 训练（章节化编组） ----------

type trainingPayload struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
}

func (s *Server) handleListTrainings(c *fiber.Ctx) error {
	list, err := s.Store.ListTrainings()
	if err != nil {
		return err
	}
	if list == nil {
		list = []model.Training{}
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"trainings": list})
}

func (s *Server) handleCreateTraining(c *fiber.Ctx) error {
	var req trainingPayload
	if err := c.BodyParser(&req); err != nil || req.Title == "" {
		return respondError(c, fiber.StatusBadRequest, "title is required")
	}
	id, err := s.Store.CreateTraining(req.Title, req.Description, req.Tags)
	if err != nil {
		return err
	}
	return respondData(c, fiber.StatusCreated, fiber.Map{"id": id})
}

func (s *Server) handleGetTraining(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	t, err := s.Store.GetTraining(id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return respondError(c, fiber.StatusNotFound, "training not found")
		}
		return err
	}
	chapters, err := s.Store.ListChapters(id)
	if err != nil {
		return err
	}
	if chapters == nil {
		chapters = []model.Chapter{}
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"training": t, "chapters": chapters})
}

func (s *Server) handleUpdateTraining(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	var req trainingPayload
	if err := c.BodyParser(&req); err != nil || req.Title == "" {
		return respondError(c, fiber.StatusBadRequest, "title is required")
	}
	if err := s.Store.UpdateTraining(id, req.Title, req.Description, req.Tags); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return respondError(c, fiber.StatusNotFound, "training not found")
		}
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleDeleteTraining(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	if err := s.Store.DeleteTraining(id); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

type chapterPayload struct {
	Title string `json:"title"`
}

func (s *Server) handleCreateChapter(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	var req chapterPayload
	if err := c.BodyParser(&req); err != nil || req.Title == "" {
		return respondError(c, fiber.StatusBadRequest, "title is required")
	}
	cid, err := s.Store.CreateChapter(id, req.Title)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return respondError(c, fiber.StatusNotFound, "training not found")
		}
		return err
	}
	return respondData(c, fiber.StatusCreated, fiber.Map{"id": cid})
}

func (s *Server) handleUpdateChapter(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	var req struct {
		Title   string `json:"title"`
		OrderNo int    `json:"orderNo"`
	}
	if err := c.BodyParser(&req); err != nil || req.Title == "" {
		return respondError(c, fiber.StatusBadRequest, "title is required")
	}
	if err := s.Store.UpdateChapter(id, req.Title, req.OrderNo); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return respondError(c, fiber.StatusNotFound, "chapter not found")
		}
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleDeleteChapter(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	if err := s.Store.DeleteChapter(id); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

type itemsPayload struct {
	ProblemIDs []int64 `json:"problemIds"`
	ItemIDs    []int64 `json:"itemIds"`
}

func (s *Server) handleAddChapterItems(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	var req itemsPayload
	if err := c.BodyParser(&req); err != nil || len(req.ProblemIDs) == 0 {
		return respondError(c, fiber.StatusBadRequest, "problemIds is required")
	}
	ids, err := s.Store.AddChapterItems(id, req.ProblemIDs)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return respondError(c, fiber.StatusNotFound, "chapter not found")
		}
		return err
	}
	return respondData(c, fiber.StatusCreated, fiber.Map{"itemIds": ids})
}

func (s *Server) handleReorderChapterItems(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	var req itemsPayload
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	if err := s.Store.ReorderChapterItems(id, req.ItemIDs); err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleDeleteItem(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	if err := s.Store.DeleteItem(id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return respondError(c, fiber.StatusNotFound, "item not found")
		}
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// ---------- 练习（平铺编组） ----------

type practicePayload struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
}

func (s *Server) handleListPractices(c *fiber.Ctx) error {
	list, err := s.Store.ListPractices()
	if err != nil {
		return err
	}
	if list == nil {
		list = []model.Practice{}
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"practices": list})
}

func (s *Server) handleCreatePractice(c *fiber.Ctx) error {
	var req practicePayload
	if err := c.BodyParser(&req); err != nil || req.Title == "" {
		return respondError(c, fiber.StatusBadRequest, "title is required")
	}
	id, err := s.Store.CreatePractice(req.Title, req.Description, req.Tags)
	if err != nil {
		return err
	}
	return respondData(c, fiber.StatusCreated, fiber.Map{"id": id})
}

func (s *Server) handleGetPractice(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	p, err := s.Store.GetPractice(id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return respondError(c, fiber.StatusNotFound, "practice not found")
		}
		return err
	}
	items, err := s.Store.ListPracticeItems(id)
	if err != nil {
		return err
	}
	if items == nil {
		items = []model.PracticeItem{}
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"practice": p, "items": items})
}

func (s *Server) handleUpdatePractice(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	var req practicePayload
	if err := c.BodyParser(&req); err != nil || req.Title == "" {
		return respondError(c, fiber.StatusBadRequest, "title is required")
	}
	if err := s.Store.UpdatePractice(id, req.Title, req.Description, req.Tags); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return respondError(c, fiber.StatusNotFound, "practice not found")
		}
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleDeletePractice(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	if err := s.Store.DeletePractice(id); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

type addPracticeItemsPayload struct {
	ProblemIDs []int64 `json:"problemIds"`
	Score      int     `json:"score"`
}

func (s *Server) handleAddPracticeItems(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	var req addPracticeItemsPayload
	if err := c.BodyParser(&req); err != nil || len(req.ProblemIDs) == 0 {
		return respondError(c, fiber.StatusBadRequest, "problemIds is required")
	}
	ids, err := s.Store.AddPracticeItems(id, req.ProblemIDs, req.Score)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return respondError(c, fiber.StatusNotFound, "practice not found")
		}
		return err
	}
	return respondData(c, fiber.StatusCreated, fiber.Map{"itemIds": ids})
}

func (s *Server) handleReorderPracticeItems(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	var req itemsPayload
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	if err := s.Store.ReorderPracticeItems(id, req.ItemIDs); err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	return c.SendStatus(fiber.StatusNoContent)
}

type updatePracticeItemPayload struct {
	Score int `json:"score"`
}

func (s *Server) handleUpdatePracticeItem(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	var req updatePracticeItemPayload
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	if err := s.Store.UpdatePracticeItem(id, req.Score); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return respondError(c, fiber.StatusNotFound, "item not found")
		}
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleDeletePracticeItem(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	if err := s.Store.DeletePracticeItem(id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return respondError(c, fiber.StatusNotFound, "item not found")
		}
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// ---------- 训练布局（拖拽排序的原子提交） ----------

type trainingLayoutPayload struct {
	ChapterIDs []int64 `json:"chapterIds"`
	Chapters   []struct {
		ChapterID int64   `json:"chapterId"`
		ItemIDs   []int64 `json:"itemIds"`
	} `json:"chapters"`
}

// handleTrainingLayout PUT /api/trainings/:id/layout
// chapterIds = 章节全排列；chapters = 每章条目的完整顺序（并集须覆盖全部条目）。
// 支持章内重排、跨章节移动与章节排序，单事务原子生效。
func (s *Server) handleTrainingLayout(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	var req trainingLayoutPayload
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	layout := make([]store.ChapterLayout, len(req.Chapters))
	for i, g := range req.Chapters {
		layout[i] = store.ChapterLayout{ChapterID: g.ChapterID, ItemIDs: g.ItemIDs}
	}
	if err := s.Store.SetTrainingLayout(id, req.ChapterIDs, layout); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return respondError(c, fiber.StatusNotFound, "training not found")
		}
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	chapters, err := s.Store.ListChapters(id)
	if err != nil {
		return err
	}
	if chapters == nil {
		chapters = []model.Chapter{}
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"chapters": chapters})
}
