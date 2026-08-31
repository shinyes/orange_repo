package quizserver

import (
	"math/rand"

	"github.com/gofiber/fiber/v2"

	"orangerepo/internal/quizstore"
)

// ---------- 选题列表 ----------

type categoryBrief struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	OrderNo       int    `json:"orderNo"`
	QuestionCount int    `json:"questionCount"`
}

type subjectBrief struct {
	ID         int64           `json:"id"`
	Name       string          `json:"name"`
	OrderNo    int             `json:"orderNo"`
	Categories []categoryBrief `json:"categories"`
}

// handleListSubjects 学生视角：科目 + 分类 + 实时题目数。
func (s *Server) handleListSubjects(c *fiber.Ctx) error {
	subjects, err := s.QS.ListSubjects()
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	out := make([]subjectBrief, 0, len(subjects))
	for _, sub := range subjects {
		sb := subjectBrief{ID: sub.ID, Name: sub.Name, OrderNo: sub.OrderNo, Categories: []categoryBrief{}}
		for _, cat := range sub.Categories {
			n, err := s.QS.Repo.CountProblems(cat.Tags, cat.Types)
			if err != nil {
				return respondError(c, fiber.StatusInternalServerError, err.Error())
			}
			sb.Categories = append(sb.Categories, categoryBrief{ID: cat.ID, Name: cat.Name, OrderNo: cat.OrderNo, QuestionCount: n})
		}
		out = append(out, sb)
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"subjects": out})
}

// ---------- 抽题 ----------

type startRoundRequest struct {
	CategoryID int64 `json:"categoryId"`
}

type roundResponse struct {
	CategoryID int64                  `json:"categoryId"`
	Total      int                     `json:"total"`
	Problems   []quizstore.QuizProblem `json:"problems"`
}

// handleStartRound 服务端按分类筛选随机抽题（随机顺序，至多每轮题数）。
func (s *Server) handleStartRound(c *fiber.Ctx) error {
	var req startRoundRequest
	if err := c.BodyParser(&req); err != nil || req.CategoryID <= 0 {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	cat, err := s.QS.GetCategory(req.CategoryID)
	if err != nil {
		return respondError(c, fiber.StatusNotFound, "分类不存在")
	}
	problems, err := s.QS.Repo.MatchingProblems(cat.Tags, cat.Types)
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	total := len(problems)
	rand.Shuffle(len(problems), func(i, j int) { problems[i], problems[j] = problems[j], problems[i] })
	roundSize := s.QS.GetRoundSize()
	if len(problems) > roundSize {
		problems = problems[:roundSize]
	}
	return respondData(c, fiber.StatusOK, roundResponse{CategoryID: cat.ID, Total: total, Problems: problems})
}

// ---------- 判题 ----------

type submitRequest struct {
	ProblemID   int64 `json:"problemId"`
	CategoryID  int64 `json:"categoryId"`
	OptionIndex *int  `json:"optionIndex"`
	Answer      *bool `json:"answer"`
}

// handleSubmit 服务端判题：正确 → 移出错题集；错误 → 记入错题集。
func (s *Server) handleSubmit(c *fiber.Ctx) error {
	var req submitRequest
	if err := c.BodyParser(&req); err != nil || req.ProblemID <= 0 {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	env, err := s.QS.Repo.GetAnswer(req.ProblemID)
	if err != nil {
		return respondError(c, fiber.StatusNotFound, "题目不存在或暂不支持")
	}
	correct := false
	switch env.Type {
	case "single_choice":
		if req.OptionIndex != nil && *req.OptionIndex == *env.AnswerIndex {
			correct = true
		}
	case "true_false":
		if req.Answer != nil && *req.Answer == *env.Answer {
			correct = true
		}
	}
	user := currentUser(c)
	if correct {
		_ = s.QS.RemoveWrong(user.ID, req.ProblemID)
	} else if req.CategoryID > 0 {
		// 分类已被删除等容错：写入失败不影响判题结果
		_ = s.QS.AddWrong(user.ID, req.ProblemID, req.CategoryID)
	}
	correctAnswer := fiber.Map{}
	switch env.Type {
	case "single_choice":
		correctAnswer["answerIndex"] = *env.AnswerIndex
	case "true_false":
		correctAnswer["answer"] = *env.Answer
	}
	explanation, has := s.QS.Repo.GetExplanation(req.ProblemID)
	return respondData(c, fiber.StatusOK, fiber.Map{
		"correct":        correct,
		"correctAnswer":  correctAnswer,
		"hasExplanation": has,
		"explanation":    explanation,
	})
}

// ---------- 错题集 ----------

type wrongRoundProblem struct {
	quizstore.QuizProblem
	CategoryID int64 `json:"categoryId"`
}

type startWrongRoundRequest struct {
	CategoryID *int64 `json:"categoryId"`
}

// handleStartWrongRound 错题集练习：该生错题随机顺序全量（可按分类过滤）。
func (s *Server) handleStartWrongRound(c *fiber.Ctx) error {
	var req startWrongRoundRequest
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	user := currentUser(c)
	records, err := s.QS.ListWrongProblems(user.ID, req.CategoryID)
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	rand.Shuffle(len(records), func(i, j int) { records[i], records[j] = records[j], records[i] })
	problems := []wrongRoundProblem{}
	for _, rec := range records {
		p, err := s.QS.Repo.GetQuizProblem(rec.ProblemID)
		if err != nil {
			continue // 主库中已删除的题目自动跳过
		}
		problems = append(problems, wrongRoundProblem{QuizProblem: *p, CategoryID: rec.CategoryID})
	}
	scope := "all"
	var catID *int64
	if req.CategoryID != nil {
		scope = "category"
		catID = req.CategoryID
	}
	return respondData(c, fiber.StatusOK, fiber.Map{
		"scope":      scope,
		"categoryId": catID,
		"problems":   problems,
	})
}

// handleWrongSummary 错题按分类统计。
func (s *Server) handleWrongSummary(c *fiber.Ctx) error {
	user := currentUser(c)
	total, err := s.QS.WrongTotal(user.ID)
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	groups, err := s.QS.WrongGroups(user.ID)
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	if groups == nil {
		groups = []quizstore.WrongGroup{}
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"total": total, "groups": groups})
}