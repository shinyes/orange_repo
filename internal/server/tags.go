package server

import (
	"encoding/json"

	"github.com/gofiber/fiber/v2"

	"orangerepo/internal/store"
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
	if req.From == store.NoneTag || req.To == store.NoneTag {
		return respondError(c, fiber.StatusBadRequest, "不能重命名无标签标记")
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
	if tag == store.NoneTag {
		return respondError(c, fiber.StatusBadRequest, "不能删除无标签标记")
	}
	updated, err := s.Store.DeleteTag(tag)
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"updated": updated})
}

// ---------- 标签手动排序（settings 持久化） ----------

// tagOrderKey settings 键；值为 {"<父路径>": ["子标签",...]} 的 JSON，父路径 "" 表示顶层。
const tagOrderKey = "tag_order"

// handleGetTagOrder GET /api/tag-order → {order}。
func (s *Server) handleGetTagOrder(c *fiber.Ctx) error {
	if v, ok := s.Store.GetSetting(tagOrderKey); ok && v != "" {
		return respondData(c, fiber.StatusOK, fiber.Map{"order": json.RawMessage(v)})
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"order": fiber.Map{}})
}

// handleSetTagOrder PUT /api/tag-order {order} → 204。
func (s *Server) handleSetTagOrder(c *fiber.Ctx) error {
	var req struct {
		Order map[string][]string `json:"order"`
	}
	if err := c.BodyParser(&req); err != nil || req.Order == nil {
		return respondError(c, fiber.StatusBadRequest, "invalid order")
	}
	b, err := json.Marshal(req.Order)
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid order")
	}
	if len(b) > 128*1024 {
		return respondError(c, fiber.StatusBadRequest, "order too large")
	}
	if err := s.Store.SetSetting(tagOrderKey, string(b)); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}
