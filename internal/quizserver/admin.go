package quizserver

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"

	"orangerepo/internal/quizstore"
)

// ---------- 科目 ----------

type adminCategory struct {
	ID            int64    `json:"id"`
	SubjectID     int64    `json:"subjectId"`
	Name          string   `json:"name"`
	OrderNo       int      `json:"orderNo"`
	Tags          []string `json:"tags"`
	Types         []string `json:"types"`
	QuestionCount int      `json:"questionCount"`
}

type adminSubject struct {
	ID         int64           `json:"id"`
	Name       string          `json:"name"`
	OrderNo    int             `json:"orderNo"`
	Categories []adminCategory `json:"categories"`
}

// handleAdminListSubjects 管理员视角：科目 + 分类全配置 + 实时题目数。
func (s *Server) handleAdminListSubjects(c *fiber.Ctx) error {
	subjects, err := s.QS.ListSubjects()
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	out := make([]adminSubject, 0, len(subjects))
	for _, sub := range subjects {
		as := adminSubject{ID: sub.ID, Name: sub.Name, OrderNo: sub.OrderNo, Categories: []adminCategory{}}
		for _, cat := range sub.Categories {
			n, err := s.QS.Repo.CountProblems(cat.Tags, cat.Types)
			if err != nil {
				return respondError(c, fiber.StatusInternalServerError, err.Error())
			}
			as.Categories = append(as.Categories, adminCategory{
				ID: cat.ID, SubjectID: cat.SubjectID, Name: cat.Name, OrderNo: cat.OrderNo,
				Tags: cat.Tags, Types: cat.Types, QuestionCount: n,
			})
		}
		out = append(out, as)
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"subjects": out})
}

func (s *Server) handleAdminCreateSubject(c *fiber.Ctx) error {
	var req struct {
		Name string `json:"name"`
	}
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	id, err := s.QS.CreateSubject(req.Name)
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	return respondData(c, fiber.StatusCreated, fiber.Map{"id": id})
}

func (s *Server) handleAdminRenameSubject(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid id")
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	if err := s.QS.RenameSubject(id, req.Name); err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleAdminDeleteSubject(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid id")
	}
	if err := s.QS.DeleteSubject(id); err != nil {
		return respondError(c, fiber.StatusNotFound, "科目不存在")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleAdminSetSubjectOrder(c *fiber.Ctx) error {
	var req struct {
		IDs []int64 `json:"ids"`
	}
	if err := c.BodyParser(&req); err != nil || len(req.IDs) == 0 {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	if err := s.QS.SetSubjectOrder(req.IDs); err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// ---------- 分类 ----------

func (s *Server) handleAdminCreateCategory(c *fiber.Ctx) error {
	var req struct {
		SubjectID int64    `json:"subjectId"`
		Name      string   `json:"name"`
		OrderNo   int      `json:"orderNo"`
		Tags      []string `json:"tags"`
		Types     []string `json:"types"`
	}
	if err := c.BodyParser(&req); err != nil || req.SubjectID <= 0 {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	id, err := s.QS.CreateCategory(req.SubjectID, req.Name, req.OrderNo, req.Tags, req.Types)
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	return respondData(c, fiber.StatusCreated, fiber.Map{"id": id})
}

func (s *Server) handleAdminUpdateCategory(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid id")
	}
	var req struct {
		Name    *string  `json:"name"`
		OrderNo *int     `json:"orderNo"`
		Tags    []string `json:"tags"`
		Types   []string `json:"types"`
	}
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	cat, err := s.QS.GetCategory(id)
	if err != nil {
		return respondError(c, fiber.StatusNotFound, "分类不存在")
	}
	if req.Name != nil {
		cat.Name = *req.Name
	}
	if req.OrderNo != nil {
		cat.OrderNo = *req.OrderNo
	}
	if req.Tags != nil {
		cat.Tags = req.Tags
	}
	if req.Types != nil {
		cat.Types = req.Types
	}
	if err := s.QS.UpdateCategory(*cat); err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleAdminDeleteCategory(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid id")
	}
	if err := s.QS.DeleteCategory(id); err != nil {
		return respondError(c, fiber.StatusNotFound, "分类不存在")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleAdminSetCategoryOrder(c *fiber.Ctx) error {
	subjectID, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid id")
	}
	var req struct {
		IDs []int64 `json:"ids"`
	}
	if err := c.BodyParser(&req); err != nil || len(req.IDs) == 0 {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	if err := s.QS.SetCategoryOrder(subjectID, req.IDs); err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// handleAdminProblemsCount 分类编辑时的实时题目数预览（query: tags=a,b&types=single_choice）。
func (s *Server) handleAdminProblemsCount(c *fiber.Ctx) error {
	tags := splitCSV(c.Query("tags"))
	types := splitCSV(c.Query("types"))
	if _, err := quizstore.NormalizeTypes(types); err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	if err := quizstore.NormalizeTags(tags); err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	n, err := s.QS.Repo.CountProblems(tags, types)
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"count": n})
}

func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ---------- 学生 ----------

func (s *Server) handleAdminListStudents(c *fiber.Ctx) error {
	students, err := s.QS.ListStudents()
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	if students == nil {
		students = []quizstore.Student{}
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"students": students})
}

func (s *Server) handleAdminCreateStudent(c *fiber.Ctx) error {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	id, err := s.QS.CreateUser(req.Username, req.Password, quizstore.RoleStudent)
	if err != nil {
		if errors.Is(err, quizstore.ErrConflict) {
			return respondError(c, fiber.StatusConflict, "用户名已存在")
		}
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	return respondData(c, fiber.StatusCreated, fiber.Map{"id": id})
}

func (s *Server) handleAdminResetStudentPassword(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid id")
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	if err := s.QS.SetStudentPassword(id, req.Password); err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleAdminDeleteStudent(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid id")
	}
	if err := s.QS.DeleteStudent(id); err != nil {
		return respondError(c, fiber.StatusNotFound, "学生不存在")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// ---------- 全局设置 ----------

func (s *Server) handleAdminGetSettings(c *fiber.Ctx) error {
	return respondData(c, fiber.StatusOK, fiber.Map{"roundSize": s.QS.GetRoundSize()})
}

func (s *Server) handleAdminPutSettings(c *fiber.Ctx) error {
	var req struct {
		RoundSize int `json:"roundSize"`
	}
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	if err := s.QS.SetRoundSize(req.RoundSize); err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	return c.SendStatus(fiber.StatusNoContent)
}