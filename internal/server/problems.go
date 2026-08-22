package server

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"orangerepo/internal/model"
	"orangerepo/internal/store"
	"orangerepo/internal/zipio"
)

// ---------- 题目 ----------

func (s *Server) handleListProblems(c *fiber.Ctx) error {
	filter := store.ProblemFilter{Q: c.Query("q")}
	if tags := c.Query("tags"); tags != "" {
		for _, t := range strings.Split(tags, ",") {
			if t = strings.TrimSpace(t); t != "" {
				filter.Tags = append(filter.Tags, t)
			}
		}
	}
	if t := c.Query("type"); t != "" {
		filter.Type = t
	}
	if v := c.Query("dirId"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return respondError(c, fiber.StatusBadRequest, "invalid dirId")
		}
		filter.DirID = &id
		filter.Recursive = c.Query("recursive") == "1" || c.Query("recursive") == "true"
	}
	if idsParam := c.Query("ids"); idsParam != "" {
		for _, part := range strings.Split(idsParam, ",") {
			id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
			if err != nil {
				return respondError(c, fiber.StatusBadRequest, "invalid ids")
			}
			filter.IDs = append(filter.IDs, id)
		}
	}
	list, err := s.Store.ListProblems(filter)
	if err != nil {
		return err
	}
	if list == nil {
		list = []model.ProblemSummary{}
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"problems": list})
}

func (s *Server) handleCreateProblem(c *fiber.Ctx) error {
	var req zipio.ProblemPayload
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	if err := zipio.NormalizeProblemPayload(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	p := model.Problem{
		Type:           model.ProblemType(req.Type),
		Title:          req.Title,
		Tags:           req.Tags,
		StatementMD:    req.StatementMD,
		BodyJSON:       req.BodyJSON,
		AnswerJSON:     req.AnswerJSON,
		Solutions:      req.Solutions,
		TimeLimitMS:    req.TimeLimitMS,
		MemoryLimitMiB: req.MemoryLimitMiB,
		DirectoryID:    req.DirectoryID,
	}
	id, err := s.Store.CreateProblem(p)
	if err != nil {
		return err
	}
	full, err := s.Store.GetProblem(id)
	if err != nil {
		return err
	}
	return respondData(c, fiber.StatusCreated, fiber.Map{"problem": full})
}

func (s *Server) handleGetProblem(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	p, err := s.Store.GetProblem(id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return respondError(c, fiber.StatusNotFound, "problem not found")
		}
		return err
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"problem": p})
}

func (s *Server) handleUpdateProblem(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	existing, err := s.Store.GetProblem(id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return respondError(c, fiber.StatusNotFound, "problem not found")
		}
		return err
	}
	var req zipio.ProblemPayload
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	// 保留原目录归属（目录移动走独立端点），除非显式携带 directoryId
	req.DirectoryID = existing.DirectoryID
	if err := zipio.NormalizeProblemPayload(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	p := model.Problem{
		ID:             id,
		Type:           model.ProblemType(req.Type),
		Title:          req.Title,
		Tags:           req.Tags,
		StatementMD:    req.StatementMD,
		BodyJSON:       req.BodyJSON,
		AnswerJSON:     req.AnswerJSON,
		Solutions:      req.Solutions,
		TimeLimitMS:    req.TimeLimitMS,
		MemoryLimitMiB: req.MemoryLimitMiB,
		DirectoryID:    req.DirectoryID,
	}
	if err := s.Store.UpdateProblem(p); err != nil {
		return err
	}
	full, err := s.Store.GetProblem(id)
	if err != nil {
		return err
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"problem": full})
}

func (s *Server) handleDeleteProblem(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	if err := s.Store.DeleteProblem(id); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

type movePayload struct {
	DirectoryID *int64 `json:"directoryId"`
}

// handleMoveProblem 移动题目到目录（null 表示移到根）。
func (s *Server) handleMoveProblem(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	var req movePayload
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	p, err := s.Store.GetProblem(id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return respondError(c, fiber.StatusNotFound, "problem not found")
		}
		return err
	}
	p.DirectoryID = req.DirectoryID
	if err := s.Store.UpdateProblem(*p); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleUpdateSolutions(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	var req struct {
		Solutions json.RawMessage `json:"solutions"`
	}
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	normalized, err := zipio.NormalizeSolutions(req.Solutions)
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	if err := s.Store.UpdateProblemSolutions(id, normalized); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return respondError(c, fiber.StatusNotFound, "problem not found")
		}
		return err
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"solutions": json.RawMessage(normalized)})
}

// ---------- 标签 ----------

func (s *Server) handleListTags(c *fiber.Ctx) error {
	tags, err := s.Store.ListTags()
	if err != nil {
		return err
	}
	if tags == nil {
		tags = []store.TagCount{}
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"tags": tags})
}
