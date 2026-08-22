package server

import (
	"errors"

	"github.com/gofiber/fiber/v2"

	"orangerepo/internal/model"
	"orangerepo/internal/store"
)

func paramID(c *fiber.Ctx, name string) (int64, error) {
	id, err := c.ParamsInt(name)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid id")
	}
	return int64(id), nil
}

// ---------- 目录 ----------

func (s *Server) handleListDirectories(c *fiber.Ctx) error {
	tree, err := s.Store.DirectoryTree()
	if err != nil {
		return err
	}
	if tree == nil {
		tree = []model.DirectoryNode{}
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"directories": tree})
}

type directoryPayload struct {
	Name     string `json:"name"`
	ParentID *int64 `json:"parentId"`
	OrderNo  *int   `json:"orderNo"`
}

func (s *Server) handleCreateDirectory(c *fiber.Ctx) error {
	var req directoryPayload
	if err := c.BodyParser(&req); err != nil || req.Name == "" {
		return respondError(c, fiber.StatusBadRequest, "name is required")
	}
	id, err := s.Store.CreateDirectory(req.Name, req.ParentID)
	if err != nil {
		return err
	}
	return respondData(c, fiber.StatusCreated, fiber.Map{"id": id})
}

func (s *Server) handleUpdateDirectory(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	var req directoryPayload
	if err := c.BodyParser(&req); err != nil || req.Name == "" {
		return respondError(c, fiber.StatusBadRequest, "name is required")
	}
	orderNo := 0
	if req.OrderNo != nil {
		orderNo = *req.OrderNo
	}
	if err := s.Store.UpdateDirectory(id, req.Name, req.ParentID, orderNo); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return respondError(c, fiber.StatusNotFound, "directory not found")
		}
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleDeleteDirectory(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	if err := s.Store.DeleteDirectory(id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return respondError(c, fiber.StatusNotFound, "directory not found")
		}
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}
