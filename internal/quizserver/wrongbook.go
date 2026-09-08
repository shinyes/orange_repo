// 全局错题集 API（/api/portal/wrong-book）：
// 分组（按来源刷题项目）+ 逐题重刷（next 随机取一题；answer 判对即从错题集清除，错题保留）。
// 刷题（quiz）作答判错自动入集（保留首来源项目），判对自动清除——两处共用 wrong_book。
package quizserver

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"orangeoj/internal/quizstore"
)

// handleWrongBook GET /api/portal/wrong-book → 错题集分组（按来源刷题项目）。
func (s *Server) handleWrongBook(c *fiber.Ctx) error {
	user := currentUser(c)
	wrong, err := s.QS.ListWrongProblems(user.ID)
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	byQuiz := map[int64]int{}
	quizIDs := []int64{}
	for _, w := range wrong {
		if _, ok := byQuiz[w.QuizID]; !ok {
			quizIDs = append(quizIDs, w.QuizID)
		}
		byQuiz[w.QuizID]++
	}
	type group struct {
		QuizID    int64  `json:"quizId"`
		Title     string `json:"title"`
		SpaceID   int64  `json:"spaceId"`
		SpaceName string `json:"spaceName"`
		Count     int    `json:"count"`
	}
	groups := []group{}
	for _, qz := range quizIDs {
		var title, spaceName string
		var spaceID int64
		_ = s.QS.Repo.DB.QueryRow(`SELECT COALESCE(q.title,''), COALESCE(sp.name,''), COALESCE(q.space_id,0)
			FROM space_quizzes q LEFT JOIN spaces sp ON sp.id=q.space_id WHERE q.id=?`, qz).
			Scan(&title, &spaceName, &spaceID)
		if title == "" {
			title = "已删除的刷题项目"
		}
		groups = append(groups, group{QuizID: qz, Title: title, SpaceID: spaceID, SpaceName: spaceName, Count: byQuiz[qz]})
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"total": len(wrong), "groups": groups})
}

// handleWrongNext GET /api/portal/wrong-book/next?quizId=N（缺省=全部）→ 随机取一错题。
func (s *Server) handleWrongNext(c *fiber.Ctx) error {
	user := currentUser(c)
	var (
		wrong []quizstore.WrongProblem
		err   error
	)
	if raw := strings.TrimSpace(c.Query("quizId")); raw != "" {
		qid, perr := strconv.ParseInt(raw, 10, 64)
		if perr != nil || qid <= 0 {
			return respondError(c, fiber.StatusBadRequest, "invalid quizId")
		}
		wrong, err = s.QS.ListWrongProblemsOfQuiz(user.ID, qid)
	} else {
		wrong, err = s.QS.ListWrongProblems(user.ID)
	}
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	if len(wrong) == 0 {
		return respondData(c, fiber.StatusOK, fiber.Map{"problem": nil, "done": true})
	}
	pick := wrong[randIntn(len(wrong))]
	problem, err := s.QS.Repo.GetOJProblem(pick.ProblemID)
	if err != nil {
		// 题目已被删除：清掉错题记录继续抽下一题
		_ = s.QS.RemoveWrongByProblem(user.ID, pick.ProblemID)
		return s.handleWrongNext(c)
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"problem": problem, "done": false})
}

// handleWrongAnswer POST /api/portal/wrong-book/answer {problemId, answer} → 判分；
// 答对 → 从错题集清除（并记 uuid 通过）；答错 → 保留（可再试/换题）。
func (s *Server) handleWrongAnswer(c *fiber.Ctx) error {
	user := currentUser(c)
	var req struct {
		ProblemID int64           `json:"problemId"`
		Answer    json.RawMessage `json:"answer"`
	}
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	p, err := s.QS.Repo.GetOJProblem(req.ProblemID)
	if err != nil || p.Type == "programming" {
		return respondError(c, fiber.StatusBadRequest, "题目不可作答")
	}
	correct, err := s.gradeObjective(p.Type, req.ProblemID, req.Answer)
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	correctAnswer := fiber.Map{}
	if !correct {
		if env, err := s.QS.Repo.GetObjectiveAnswer(req.ProblemID); err == nil {
			if env.AnswerIndex != nil {
				correctAnswer["answerIndex"] = *env.AnswerIndex
			} else if env.Answer != nil {
				correctAnswer["answer"] = *env.Answer
			}
		}
	}
	if correct {
		if err := s.QS.RemoveWrongByProblem(user.ID, req.ProblemID); err != nil {
			return respondError(c, fiber.StatusInternalServerError, err.Error())
		}
		var uuid string
		_ = s.QS.Repo.DB.QueryRow(`SELECT uuid FROM problems WHERE id=?`, req.ProblemID).Scan(&uuid)
		if uuid != "" {
			_ = s.QS.RecordSolved(user.ID, uuid) // 重刷掌握即记通过（幂等）
		}
	}
	return respondData(c, fiber.StatusOK, fiber.Map{
		"correct":       correct,
		"correctAnswer": correctAnswer,
	})
}
