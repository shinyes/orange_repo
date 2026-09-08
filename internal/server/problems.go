package server

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"orangeoj/internal/model"
	"orangeoj/internal/store"
	"orangeoj/internal/zipio"
)

// parseProblemFilter 从查询参数解析题目过滤条件（列表 / 导出 / 标签 facets 共用）。
func parseProblemFilter(c *fiber.Ctx) (store.ProblemFilter, error) {
	f := store.ProblemFilter{Q: c.Query("q")}
	// tags 一律按“重复参数 = 多标签”解析（tags=a&tags=b），标签文本可含逗号，
	// 不再以逗号作多标签分隔（含逗号标签旧版无法选中，属缺陷修复）。
	for _, t := range multiQuery(c, "tags") {
		if t = strings.TrimSpace(t); t != "" {
			f.Tags = append(f.Tags, t)
		}
	}
	f.Type = c.Query("type")
	if idsParam := c.Query("ids"); idsParam != "" {
		for _, part := range strings.Split(idsParam, ",") {
			id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
			if err != nil || id <= 0 {
				return f, errors.New("invalid ids")
			}
			f.IDs = append(f.IDs, id)
		}
	}
	return f, nil
}

// multiQuery 取某 query 参数的全部值（支持 tags=a&tags=b 重复参数；值已百分号解码）。
func multiQuery(c *fiber.Ctx, key string) []string {
	args := c.Request().URI().QueryArgs()
	items := args.PeekMulti(key)
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, string(it))
	}
	return out
}

// ---------- 题目 ----------

func (s *Server) handleListProblems(c *fiber.Ctx) error {
	filter, err := parseProblemFilter(c)
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	// 域隔离：仓库页题目列表按当前域（domain_admin 本域/global query 或默认域）
	user := currentUser(c)
	scope, err := s.domainOrDefault(c, user)
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	filter.DomainID = scope
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
	// 域约束：domain_admin 强制本域；global_admin 优先带 domainId query，
	// 缺省归默认域（兼容既有调用/单域部署；多域时前端总会带）
	user := currentUser(c)
	scope, err := s.domainOrDefault(c, user)
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "缺少 domainId（请先选择域）")
	}
	p := model.Problem{
		DomainID:       scope,
		Type:           model.ProblemType(req.Type),
		Title:          req.Title,
		Tags:           req.Tags,
		StatementMD:    req.StatementMD,
		BodyJSON:       req.BodyJSON,
		AnswerJSON:     req.AnswerJSON,
		Solutions:      req.Solutions,
		StarterCpp:     req.StarterCpp,
		StarterPy:      req.StarterPy,
		TimeLimitMS:    req.TimeLimitMS,
		MemoryLimitMiB: req.MemoryLimitMiB,
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
	if !s.problemInDomain(c, p) {
		return respondError(c, fiber.StatusNotFound, "problem not found")
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"problem": p})
}

// problemInDomain 题目是否属于当前请求域（越权视为不存在）。
func (s *Server) problemInDomain(c *fiber.Ctx, p *model.Problem) bool {
	scope, err := s.domainOrDefault(c, currentUser(c))
	if err != nil || scope == nil {
		return false
	}
	if p.DomainID == nil {
		return false
	}
	return *p.DomainID == *scope
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
	if !s.problemInDomain(c, existing) {
		return respondError(c, fiber.StatusNotFound, "problem not found")
	}
	var req zipio.ProblemPayload
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	if err := zipio.NormalizeProblemPayload(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	p := model.Problem{
		ID:             id,
		DomainID:       existing.DomainID, // 域不可变（防跨域改写）
		Type:           model.ProblemType(req.Type),
		Title:          req.Title,
		Tags:           req.Tags,
		StatementMD:    req.StatementMD,
		BodyJSON:       req.BodyJSON,
		AnswerJSON:     req.AnswerJSON,
		Solutions:      req.Solutions,
		StarterCpp:     req.StarterCpp,
		StarterPy:      req.StarterPy,
		TimeLimitMS:    req.TimeLimitMS,
		MemoryLimitMiB: req.MemoryLimitMiB,
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
	existing, err := s.Store.GetProblem(id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return respondError(c, fiber.StatusNotFound, "problem not found")
		}
		return err
	}
	if !s.problemInDomain(c, existing) {
		return respondError(c, fiber.StatusNotFound, "problem not found")
	}
	if err := s.Store.DeleteProblem(id); err != nil {
		return err
	}
	// 刷题侧数据同步清理（判题历史/草稿/错题集/作答/通过记录引用被删题）
	if s.QuizStore != nil {
		u := existing.UUID
		if err := s.QuizStore.CleanupProblems([]int64{id}, []string{u}); err != nil {
			return err
		}
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

// ---------- 标签（动态 facet 计数） ----------

// handleListTags 返回当前过滤上下文下的候选标签命中数与总命中题数。
// 无过滤参数时等价于全局计数。
func (s *Server) handleListTags(c *fiber.Ctx) error {
	filter, err := parseProblemFilter(c)
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	tags, total, err := s.Store.ListTagFacets(filter)
	if err != nil {
		return err
	}
	if tags == nil {
		tags = []store.TagCount{}
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"tags": tags, "total": total})
}
