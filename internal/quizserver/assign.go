// OrangeOJ 管理员布置 API（/api/admin/assignments + 主库目录浏览）。
// 模仿上游 OrangeOJ：管理员从（空间内）训练/练习布置给学生；这里布置源 = 主库训练/练习。
package quizserver

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"

	"orangerepo/internal/quizstore"
)

// ---------- 主库目录浏览（只读） ----------

func (s *Server) handleAdminRepoTrainings(c *fiber.Ctx) error {
	list, err := s.QS.Repo.ListRepoTrainings()
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	if list == nil {
		list = []quizstore.RepoTrainings{}
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"trainings": list})
}

func (s *Server) handleAdminRepoPractices(c *fiber.Ctx) error {
	list, err := s.QS.Repo.ListRepoPractices()
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	if list == nil {
		list = []quizstore.RepoPractice{}
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"practices": list})
}

func (s *Server) handleAdminRepoTraining(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid id")
	}
	t, err := s.QS.Repo.GetRepoTraining(id)
	if err != nil {
		if errors.Is(err, quizstore.ErrNotFound) {
			return respondError(c, fiber.StatusNotFound, "训练不存在")
		}
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	return respondData(c, fiber.StatusOK, t)
}

func (s *Server) handleAdminRepoPractice(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid id")
	}
	p, err := s.QS.Repo.GetRepoPractice(id)
	if err != nil {
		if errors.Is(err, quizstore.ErrNotFound) {
			return respondError(c, fiber.StatusNotFound, "练习不存在")
		}
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	return respondData(c, fiber.StatusOK, p)
}

// ---------- 布置 CRUD ----------

// adminAssignmentView 管理端列表视图（problemCount 实时主库）。
type adminAssignmentView struct {
	quizstore.Assignment
}

func (s *Server) fillAssignmentCounts(a *quizstore.Assignment) error {
	switch a.Kind {
	case "training":
		n, err := s.QS.Repo.TrainingProblemCount(a.RepoID)
		if err != nil && !errors.Is(err, quizstore.ErrNotFound) {
			return err
		}
		a.ProblemCount = n
	case "practice":
		n, err := s.QS.Repo.PracticeProblemCount(a.RepoID)
		if err != nil && !errors.Is(err, quizstore.ErrNotFound) {
			return err
		}
		a.ProblemCount = n
	}
	return nil
}

// handleAdminListAssignments GET /api/admin/assignments
func (s *Server) handleAdminListAssignments(c *fiber.Ctx) error {
	filter := quizstore.AssignmentFilter{Kind: c.Query("kind")}
	if filter.Kind != "" && filter.Kind != "training" && filter.Kind != "practice" {
		return respondError(c, fiber.StatusBadRequest, "kind 非法")
	}
	list, err := s.QS.ListAssignments(filter)
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	for i := range list {
		if err := s.fillAssignmentCounts(&list[i]); err != nil {
			return respondError(c, fiber.StatusInternalServerError, err.Error())
		}
		if list[i].AssignedAll {
			ids, err := s.QS.AllStudentIDs()
			if err != nil {
				return respondError(c, fiber.StatusInternalServerError, err.Error())
			}
			list[i].StudentCount = len(ids)
		}
	}
	if list == nil {
		list = []quizstore.Assignment{}
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"assignments": list})
}

type createAssignmentRequest struct {
	Kind        string  `json:"kind"`
	RepoID      int64   `json:"repoId"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	AssignedAll bool    `json:"assignedAll"`
	Published   *bool   `json:"published"`
	StudentIDs  []int64 `json:"studentIds"`
}

// handleAdminCreateAssignment POST /api/admin/assignments
func (s *Server) handleAdminCreateAssignment(c *fiber.Ctx) error {
	var req createAssignmentRequest
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	if req.Kind != "training" && req.Kind != "practice" {
		return respondError(c, fiber.StatusBadRequest, "kind 必须为 training 或 practice")
	}
	if req.RepoID <= 0 {
		return respondError(c, fiber.StatusBadRequest, "repoId 非法")
	}
	title := strings.TrimSpace(req.Title)
	description := strings.TrimSpace(req.Description)
	var tags []string
	switch req.Kind {
	case "training":
		t, err := s.QS.Repo.GetRepoTraining(req.RepoID)
		if err != nil {
			if errors.Is(err, quizstore.ErrNotFound) {
				return respondError(c, fiber.StatusNotFound, "主库训练不存在")
			}
			return respondError(c, fiber.StatusInternalServerError, err.Error())
		}
		if title == "" {
			title = t.Title
		}
		if description == "" {
			description = t.Description
		}
		tags = t.Tags
	case "practice":
		p, err := s.QS.Repo.GetRepoPractice(req.RepoID)
		if err != nil {
			if errors.Is(err, quizstore.ErrNotFound) {
				return respondError(c, fiber.StatusNotFound, "主库练习不存在")
			}
			return respondError(c, fiber.StatusInternalServerError, err.Error())
		}
		if title == "" {
			title = p.Title
		}
		if description == "" {
			description = p.Description
		}
		tags = p.Tags
	}
	if err := s.validateStudentIDs(req.StudentIDs); err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	id, err := s.QS.CreateAssignment(req.Kind, req.RepoID, title, description, tags, req.AssignedAll, req.StudentIDs)
	if err != nil {
		if errors.Is(err, quizstore.ErrConflict) {
			return respondError(c, fiber.StatusConflict, "该训练/练习已布置过")
		}
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	if req.Published != nil && !*req.Published {
		_ = s.QS.UpdateAssignmentMeta(id, title, description, false)
	}
	return respondData(c, fiber.StatusCreated, fiber.Map{"id": id})
}

// validateStudentIDs 校验定向学生 id 全部存在且为 student 角色。
func (s *Server) validateStudentIDs(ids []int64) error {
	for _, id := range ids {
		u, err := s.QS.Accounts.GetUserByID(id)
		if err != nil || u.Role != "student" {
			return errors.New("定向学生不存在或非学生角色")
		}
	}
	return nil
}

// handleAdminUpdateAssignment PATCH /api/admin/assignments/:id {title?, description?, published?}
func (s *Server) handleAdminUpdateAssignment(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid id")
	}
	var req struct {
		Title       *string `json:"title"`
		Description *string `json:"description"`
		Published   *bool   `json:"published"`
	}
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	a, err := s.QS.GetAssignment(id)
	if err != nil {
		return respondError(c, fiber.StatusNotFound, "布置不存在")
	}
	title := a.Title
	if req.Title != nil {
		title = strings.TrimSpace(*req.Title)
	}
	description := a.Description
	if req.Description != nil {
		description = strings.TrimSpace(*req.Description)
	}
	published := a.Published
	if req.Published != nil {
		published = *req.Published
	}
	if err := s.QS.UpdateAssignmentMeta(id, title, description, published); err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// handleAdminSetAssignmentStudents PUT /api/admin/assignments/:id/students
func (s *Server) handleAdminSetAssignmentStudents(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid id")
	}
	var req struct {
		AssignedAll bool    `json:"assignedAll"`
		StudentIDs  []int64 `json:"studentIds"`
	}
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	if !req.AssignedAll {
		if err := s.validateStudentIDs(req.StudentIDs); err != nil {
			return respondError(c, fiber.StatusBadRequest, err.Error())
		}
	}
	if err := s.QS.SetAssignedStudents(id, req.AssignedAll, req.StudentIDs); err != nil {
		if errors.Is(err, quizstore.ErrNotFound) {
			return respondError(c, fiber.StatusNotFound, "布置不存在")
		}
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// handleAdminAssignmentStudents GET /api/admin/assignments/:id/students
func (s *Server) handleAdminAssignmentStudents(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid id")
	}
	a, err := s.QS.GetAssignment(id)
	if err != nil {
		return respondError(c, fiber.StatusNotFound, "布置不存在")
	}
	students, err := s.QS.ListAssignedStudents(id)
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	if students == nil {
		students = []quizstore.AssignedStudent{}
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"assignedAll": a.AssignedAll, "students": students})
}

// handleAdminDeleteAssignment DELETE /api/admin/assignments/:id
func (s *Server) handleAdminDeleteAssignment(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid id")
	}
	if err := s.QS.DeleteAssignment(id); err != nil {
		if errors.Is(err, quizstore.ErrNotFound) {
			return respondError(c, fiber.StatusNotFound, "布置不存在")
		}
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// ---------- 每题统计 ----------

// handleAdminAssignmentStats GET /api/admin/assignments/:id/stats
func (s *Server) handleAdminAssignmentStats(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid id")
	}
	a, err := s.QS.GetAssignment(id)
	if err != nil {
		return respondError(c, fiber.StatusNotFound, "布置不存在")
	}
	students, err := s.QS.AssignmentStudentSet(id)
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	var problemIDs []int64
	switch a.Kind {
	case "training":
		chapters, err := s.QS.Repo.TrainingProblemIDs(a.RepoID)
		if err != nil {
			return respondError(c, fiber.StatusInternalServerError, err.Error())
		}
		seen := map[int64]bool{}
		for _, ids := range chapters {
			for _, pid := range ids {
				if !seen[pid] {
					seen[pid] = true
					problemIDs = append(problemIDs, pid)
				}
			}
		}
	case "practice":
		problemIDs, err = s.QS.Repo.PracticeProblemIDs(a.RepoID)
		if err != nil {
			return respondError(c, fiber.StatusInternalServerError, err.Error())
		}
	}
	briefs, err := s.QS.Repo.GetProblemsBrief(problemIDs)
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	stats, err := s.QS.ProblemStatsOf(students, problemIDs)
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	// 组装视图：所有题目按序（含 0 统计），补标题/题型
	view := make([]fiber.Map, 0, len(problemIDs))
	for _, pid := range problemIDs {
		st := stats[pid]
		b := briefs[pid]
		view = append(view, fiber.Map{
			"problemId":   pid,
			"title":       b.Title,
			"type":        b.Type,
			"accepted":    st.AcceptedUsers,
			"submissions": st.SubmissionCount,
		})
	}
	if view == nil {
		view = []fiber.Map{}
	}
	return respondData(c, fiber.StatusOK, fiber.Map{
		"title":         a.Title,
		"kind":          a.Kind,
		"totalStudents": len(students),
		"problems":      view,
	})
}
