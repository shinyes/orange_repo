package server

import (
	"errors"

	"github.com/gofiber/fiber/v2"

	"orangerepo/internal/model"
	"orangerepo/internal/store"
)

// ---------- 题册目录（可嵌套） ----------

type bookletDirectoryPayload struct {
	Name     string `json:"name"`
	ParentID *int64 `json:"parentId"`
}

// handleListBookletDirectories GET /api/booklet-directories → {directories}（扁平列表，同层按序）。
func (s *Server) handleListBookletDirectories(c *fiber.Ctx) error {
	list, err := s.Store.ListBookletDirectories()
	if err != nil {
		return err
	}
	if list == nil {
		list = []model.BookletDirectory{}
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"directories": list})
}

// handleCreateBookletDirectory POST /api/booklet-directories {name,parentId?} → {id}。
func (s *Server) handleCreateBookletDirectory(c *fiber.Ctx) error {
	var req bookletDirectoryPayload
	if err := c.BodyParser(&req); err != nil || req.Name == "" {
		return respondError(c, fiber.StatusBadRequest, "name is required")
	}
	id, err := s.Store.CreateBookletDirectory(req.Name, req.ParentID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return respondError(c, fiber.StatusNotFound, "parent directory not found")
		}
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	return respondData(c, fiber.StatusCreated, fiber.Map{"id": id})
}

// handleRenameBookletDirectory PATCH /api/booklet-directories/:id {name} → 204。
func (s *Server) handleRenameBookletDirectory(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	var req bookletDirectoryPayload
	if err := c.BodyParser(&req); err != nil || req.Name == "" {
		return respondError(c, fiber.StatusBadRequest, "name is required")
	}
	if err := s.Store.RenameBookletDirectory(id, req.Name); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return respondError(c, fiber.StatusNotFound, "directory not found")
		}
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// handleDeleteBookletDirectory DELETE /api/booklet-directories/:id[?deleteBooklets=true] → 204。
// deleteBooklets=true：直接归属的训练/练习一并删除；否则它们移到顶层。
// 子目录总是上移一层，不删除数据。
func (s *Server) handleDeleteBookletDirectory(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	deleteBooklets := c.Query("deleteBooklets") == "true" || c.Query("deleteBooklets") == "1"
	if err := s.Store.DeleteBookletDirectory(id, deleteBooklets); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return respondError(c, fiber.StatusNotFound, "directory not found")
		}
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// handleSetBookletDirectoryLayout PUT /api/booklet-directories/layout
// {directories:[{id,parentId,orderNo...}]} → 204。
// 前端拖拽改层级/顺序后一次性提交整个目录树布局。
func (s *Server) handleSetBookletDirectoryLayout(c *fiber.Ctx) error {
	var req struct {
		Directories []model.BookletDirectory `json:"directories"`
	}
	if err := c.BodyParser(&req); err != nil || req.Directories == nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	if err := s.Store.SetBookletDirectoryLayout(req.Directories); err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// ---------- 题册归属目录 ----------

// handleSetTrainingFolder PUT /api/trainings/:id/folder {folderId: number|null} → 204。
func (s *Server) handleSetTrainingFolder(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	var req struct {
		FolderID *int64 `json:"folderId"`
	}
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	if err := s.Store.SetTrainingFolder(id, req.FolderID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			// 训练或目录不存在时统一 404 语义
			return respondError(c, fiber.StatusNotFound, "training or directory not found")
		}
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// handleSetPracticeFolder PUT /api/practices/:id/folder {folderId: number|null} → 204。
func (s *Server) handleSetPracticeFolder(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	var req struct {
		FolderID *int64 `json:"folderId"`
	}
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	if err := s.Store.SetPracticeFolder(id, req.FolderID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return respondError(c, fiber.StatusNotFound, "practice or directory not found")
		}
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	return c.SendStatus(fiber.StatusNoContent)
}