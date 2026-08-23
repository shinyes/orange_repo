package server

import (
	"github.com/gofiber/fiber/v2"
)

// handleRenameTag 重命名标签（子树整体搬家）：PATCH /api/tags {from,to} → {updated}。
func (s *Server) handleRenameTag(c *fiber.Ctx) error {
	var req struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	if req.From == "" || req.To == "" {
		return respondError(c, fiber.StatusBadRequest, "from 和 to 不能为空")
	}
	updated, err := s.Store.RenameTag(req.From, req.To)
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"updated": updated})
}

// handleDeleteTag 删除标签及其全部子孙：DELETE /api/tags?tag=… → {updated}。
func (s *Server) handleDeleteTag(c *fiber.Ctx) error {
	tag := c.Query("tag")
	if tag == "" {
		return respondError(c, fiber.StatusBadRequest, "缺少 tag 参数")
	}
	updated, err := s.Store.DeleteTag(tag)
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"updated": updated})
}
