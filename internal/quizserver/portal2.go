// OrangeOJ 门户 API（续）：空间练习（整卷交卷）、空间刷题、排行榜。
package quizserver

import (
	"encoding/json"
	"errors"

	"github.com/gofiber/fiber/v2"

	"orangeoj/internal/quizstore"
)

// ---------- 空间练习（整卷交卷） ----------

// handlePortalPractice GET /api/portal/space/:id/practice/:pid
// → 练习详情：题目集（含类型/uuid，作答整卷由前端拉取题目内容走题目接口）。
func (s *Server) handlePortalPractice(c *fiber.Ctx) error {
	spaceID, err := s.resolveSpace(c)
	if err != nil {
		return err
	}
	pid, err := paramID(c, "pid")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid practice id")
	}
	p, items, err := s.QS.Repo.GetSpacePracticeBrief(pid, viewerID(currentUser(c)))
	if err != nil {
		return respondError(c, fiber.StatusNotFound, "练习不存在")
	}
	if p.SpaceID != spaceID {
		return respondError(c, fiber.StatusNotFound, "练习不存在")
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"practice": p, "items": items})
}

// practiceAnswerItem 交卷快照元素。
type practiceAnswerItem struct {
	ProblemID int64  `json:"problemId"`
	Answer    json.RawMessage `json:"answer"`   // 客观题：index/bool
	UUID      string `json:"uuid,omitempty"`    // 题目 uuid（练习条目已带）
}

// handlePortalPracticeSubmit POST /api/portal/space/:id/practice/:pid/submit
// {answers:[{problemId, answer, uuid}]} → 整卷判定：逐题客观判对错，返回汇总
// （交卷后展示结果；每次作答都记录，可重做）。
func (s *Server) handlePortalPracticeSubmit(c *fiber.Ctx) error {
	spaceID, err := s.resolveSpace(c)
	if err != nil {
		return err
	}
	pid, err := paramID(c, "pid")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid practice id")
	}
	user := currentUser(c)
	p, items, err := s.QS.Repo.GetSpacePracticeBrief(pid, viewerID(currentUser(c)))
	if err != nil {
		return respondError(c, fiber.StatusNotFound, "练习不存在")
	}
	if p.SpaceID != spaceID {
		return respondError(c, fiber.StatusNotFound, "练习不存在")
	}
	var req struct {
		Answers []practiceAnswerItem `json:"answers"`
	}
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	// 建立题目信息索引
	byID := map[int64]struct {
		typ  string
		uuid string
	}{}
	for _, it := range items {
		byID[it.ProblemID] = struct {
			typ  string
			uuid string
		}{it.ProblemType, it.ProblemUUID}
	}
	type result struct {
		ProblemID     int64           `json:"problemId"`
		Correct       bool            `json:"correct"`
		Type          string          `json:"type"`
		CorrectAnswer json.RawMessage `json:"correctAnswer,omitempty"`
	}
	// 快照元素：逐题含 correct 与用户所选 answer（交卷记录据此写 student_solved 通过记录，
	// 并支持答题卡回看逐题作答）
	type snapshotItem struct {
		ProblemID int64           `json:"problemId"`
		Correct   bool            `json:"correct"`
		UUID      string          `json:"uuid,omitempty"`
		Answer    json.RawMessage `json:"answer,omitempty"`
	}
	results := make([]result, 0, len(req.Answers))
	snapItems := make([]snapshotItem, 0, len(req.Answers))
	correctCount := 0
	objectiveTotal := 0
	for _, a := range req.Answers {
		info, ok := byID[a.ProblemID]
		if !ok {
			continue
		}
		if info.typ == "programming" {
			continue // 编程题整卷评测走代码提交，交卷统计客观题（前端另行整合）
		}
		objectiveTotal++
		ok2, err := s.gradeObjective(info.typ, a.ProblemID, a.Answer)
		if err != nil {
			continue
		}
		r := result{ProblemID: a.ProblemID, Correct: ok2, Type: info.typ}
		if !ok2 {
			if env, err := s.QS.Repo.GetObjectiveAnswer(a.ProblemID); err == nil {
				if env.AnswerIndex != nil {
					b, _ := json.Marshal(*env.AnswerIndex)
					r.CorrectAnswer = b
				} else if env.Answer != nil {
					b, _ := json.Marshal(*env.Answer)
					r.CorrectAnswer = b
				}
			}
		}
		results = append(results, r)
		ansJSON, _ := json.Marshal(a.Answer)
		snapItems = append(snapItems, snapshotItem{ProblemID: a.ProblemID, Correct: ok2, UUID: info.uuid, Answer: ansJSON})
		if ok2 {
			correctCount++
		}
	}
	snapshot, _ := json.Marshal(snapItems)
	submissionID, err := s.QS.SavePracticeSubmission(pid, user.ID, string(snapshot), correctCount)
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	return respondData(c, fiber.StatusOK, fiber.Map{
		"submissionId":    submissionID,
		"results":         results,
		"objectiveCorrect": correctCount,
		"objectiveTotal":  objectiveTotal,
	})
}

// handlePortalPracticeSubmissions GET /api/portal/space/:id/practice/:pid/submissions
// → 我的交卷记录。
func (s *Server) handlePortalPracticeSubmissions(c *fiber.Ctx) error {
	spaceID, err := s.resolveSpace(c)
	if err != nil {
		return err
	}
	pid, err := paramID(c, "pid")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid practice id")
	}
	user := currentUser(c)
	p, _, err := s.QS.Repo.GetSpacePracticeBrief(pid, viewerID(currentUser(c)))
	if err != nil {
		return respondError(c, fiber.StatusNotFound, "练习不存在")
	}
	if p.SpaceID != spaceID {
		return respondError(c, fiber.StatusNotFound, "练习不存在")
	}
	subs, err := s.QS.ListPracticeSubmissions(pid, user.ID)
	if err != nil {
		return err
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"submissions": subs})
}

// handlePortalPracticeSubmissionDetail GET /api/portal/space/:id/practice/:pid/submissions/:sid
// → 答题卡回看：该次交卷快照（逐题对错/所选答案）与练习条目合并的逐题明细。
func (s *Server) handlePortalPracticeSubmissionDetail(c *fiber.Ctx) error {
	spaceID, err := s.resolveSpace(c)
	if err != nil {
		return err
	}
	pid, err := paramID(c, "pid")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid practice id")
	}
	sid, err := paramID(c, "sid")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid submission id")
	}
	user := currentUser(c)
	p, items, err := s.QS.Repo.GetSpacePracticeBrief(pid, viewerID(user))
	if err != nil {
		return respondError(c, fiber.StatusNotFound, "练习不存在")
	}
	if p.SpaceID != spaceID {
		return respondError(c, fiber.StatusNotFound, "练习不存在")
	}
	// 提交记录归属校验（只能看自己的）
	subs, err := s.QS.ListPracticeSubmissions(pid, user.ID)
	if err != nil {
		return err
	}
	var createdAt string
	owned := false
	for _, sb := range subs {
		if sb.ID == sid {
			createdAt = sb.CreatedAt
			owned = true
			break
		}
	}
	if !owned {
		return respondError(c, fiber.StatusNotFound, "提交记录不存在")
	}
	snapshot, err := s.QS.GetPracticeSubmission(sid)
	if err != nil {
		if errors.Is(err, quizstore.ErrNotFound) {
			return respondError(c, fiber.StatusNotFound, "提交记录不存在")
		}
		return err
	}
	// 快照 → map
	type snapItem struct {
		ProblemID int64           `json:"problemId"`
		Correct   bool            `json:"correct"`
		UUID      string          `json:"uuid,omitempty"`
		Answer    json.RawMessage `json:"answer,omitempty"`
	}
	var snaps []snapItem
	if err := json.Unmarshal([]byte(snapshot), &snaps); err != nil {
		return respondError(c, fiber.StatusInternalServerError, "快照解析失败")
	}
	byPid := map[int64]snapItem{}
	for _, s2 := range snaps {
		byPid[s2.ProblemID] = s2
	}
	type itemDetail struct {
		ProblemID     int64           `json:"problemId"`
		No            int             `json:"no"`
		Title         string          `json:"title"`
		Type          string          `json:"type"`
		Answered      bool            `json:"answered"` // 客观题该次是否作答
		Correct       bool            `json:"correct"`
		Answer        json.RawMessage `json:"answer,omitempty"`
		CorrectAnswer json.RawMessage `json:"correctAnswer,omitempty"`
	}
	out := []itemDetail{}
	objCorrect := 0
	no := 0
	for _, it := range items {
		no++
		d := itemDetail{
			ProblemID: it.ProblemID, No: no, Title: it.ProblemTitle, Type: it.ProblemType,
		}
		if it.ProblemType == "programming" {
			// 编程题：导航占位（answered=false）
			out = append(out, d)
			continue
		}
		snap, ok := byPid[it.ProblemID]
		if ok {
			d.Answered = true
			d.Correct = snap.Correct
			d.Answer = snap.Answer
			if snap.Correct {
				objCorrect++
			}
		}
		// 答错或未作答：附正确项（用户已交卷可见）
		if !d.Answered || !d.Correct {
			if env, err := s.QS.Repo.GetObjectiveAnswer(it.ProblemID); err == nil {
				if env.AnswerIndex != nil {
					b, _ := json.Marshal(*env.AnswerIndex)
					d.CorrectAnswer = b
				} else if env.Answer != nil {
					b, _ := json.Marshal(*env.Answer)
					d.CorrectAnswer = b
				}
			}
		}
		out = append(out, d)
	}
	return respondData(c, fiber.StatusOK, fiber.Map{
		"submissionId":    sid,
		"practiceId":      pid,
		"createdAt":       createdAt,
		"objectiveCorrect": objCorrect,
		"items":           out,
	})
}

// ---------- 空间刷题 ----------

// handlePortalSpaceQuizzes GET /api/portal/space/:id/quizzes → 空间刷题项目列表。
func (s *Server) handlePortalSpaceQuizzes(c *fiber.Ctx) error {
	spaceID, err := s.resolveSpace(c)
	if err != nil {
		return err
	}
	quizzes, err := s.QS.Repo.ListSpaceQuizzesBrief(spaceID, viewerID(currentUser(c)))
	if err != nil {
		return err
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"quizzes": quizzes})
}

// handlePortalQuizProblem GET /api/portal/quiz/:qid/problem
// → 从刷题项目取一题（tags 源：按标签筛域题库单选/判断随机；repo 源：模板题单客观题随机）。
// 返回题目内容（不含答案），附该题是否已通过（uuid 去重，已过跳过）。
func (s *Server) handlePortalQuizProblem(c *fiber.Ctx) error {
	user := currentUser(c)
	qid, err := paramID(c, "qid")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid quiz id")
	}
	// 取项目元信息 + 成员可见校验
	spaceID, err := s.quizSpaceOf(qid)
	if err != nil {
		return respondError(c, fiber.StatusNotFound, "刷题项目不存在")
	}
	if _, err := s.resolveSpaceCtx(c, spaceID); err != nil {
		return err
	}
	vis, err := s.QS.Repo.QuizVisibleForUser(qid, viewerID(user))
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	if !vis {
		return respondError(c, fiber.StatusNotFound, "刷题项目不存在")
	}
	ids, err := s.quizProblemIDs(qid)
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	if len(ids) == 0 {
		return respondData(c, fiber.StatusOK, fiber.Map{"problem": nil, "done": true})
	}
	// 已通过过滤
	solved, err := s.QS.SolvedUUIDs(user.ID)
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	pending := make([]int64, 0, len(ids))
	for _, id := range ids {
		var u string
		if err := s.QS.Repo.DB.QueryRow(`SELECT uuid FROM problems WHERE id=?`, id).Scan(&u); err != nil {
			continue
		}
		if u != "" && solved[u] {
			continue
		}
		pending = append(pending, id)
	}
	if len(pending) == 0 {
		return respondData(c, fiber.StatusOK, fiber.Map{"problem": nil, "done": true})
	}
	// 随机取一题
	idx := randIntn(len(pending))
	problem, err := s.QS.Repo.GetOJProblem(pending[idx])
	if err != nil {
		return respondError(c, fiber.StatusNotFound, "题目不存在")
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"problem": problem, "done": false})
}

// handlePortalQuizAnswer POST /api/portal/quiz/:qid/answer {problemId, answer}
// → 判对错：对 → 记通过（uuid 去重）；错 → 不限制重答。
func (s *Server) handlePortalQuizAnswer(c *fiber.Ctx) error {
	user := currentUser(c)
	qid, err := paramID(c, "qid")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid quiz id")
	}
	spaceID, err := s.quizSpaceOf(qid)
	if err != nil {
		return respondError(c, fiber.StatusNotFound, "刷题项目不存在")
	}
	if _, err := s.resolveSpaceCtx(c, spaceID); err != nil {
		return err
	}
	vis, err := s.QS.Repo.QuizVisibleForUser(qid, viewerID(user))
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	if !vis {
		return respondError(c, fiber.StatusNotFound, "刷题项目不存在")
	}
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
	var uuid string
	_ = s.QS.Repo.DB.QueryRow(`SELECT uuid FROM problems WHERE id=?`, req.ProblemID).Scan(&uuid)
	already := false
	if correct && uuid != "" {
		solvedMap, err := s.QS.SolvedUUIDs(user.ID)
		if err != nil {
			return respondError(c, fiber.StatusInternalServerError, err.Error())
		}
		already = solvedMap[uuid]
		if err := s.QS.RecordSolved(user.ID, uuid); err != nil {
			return respondError(c, fiber.StatusInternalServerError, err.Error())
		}
	}
	return respondData(c, fiber.StatusOK, fiber.Map{
		"correct":       correct,
		"correctAnswer": correctAnswer,
		"firstTime":     correct && !already, // 首次通过（此前未记过）
	})
}

// ---------- 排行榜 ----------

// handlePortalRank GET /api/portal/rank?domainId= → 按域学生通过数排行榜（管理员不参与）。
func (s *Server) handlePortalRank(c *fiber.Ctx) error {
	user := currentUser(c)
	domainID, err := s.rankDomainOf(c, user)
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	// 该域全部学生 = space_members 中 user 集合（排除管理员角色）
	type row struct {
		UserID   int64  `json:"userId"`
		Username string `json:"username"`
		Solved   int    `json:"solved"`
	}
	// 该域空间成员 user 集
	memberRows, err := s.QS.Repo.DB.Query(`SELECT DISTINCT m.user_id FROM space_members m
		JOIN spaces sp ON sp.id=m.space_id WHERE sp.domain_id=?`, domainID)
	if err != nil {
		return err
	}
	var memberIDs []int64
	for memberRows.Next() {
		var id int64
		if err := memberRows.Scan(&id); err != nil {
			memberRows.Close()
			return err
		}
		memberIDs = append(memberIDs, id)
	}
	memberRows.Close()
	out := make([]row, 0, len(memberIDs))
	for _, uid := range memberIDs {
		u, err := s.QS.Accounts.GetUserByID(uid)
		if err != nil {
			continue
		}
		// 管理员不参与排名（仅空间成员计）
		if !u.Role.ValidNew() || isAdminRole(u.Role) {
			continue
		}
		var n int
		if err := s.QS.DB.QueryRow(`SELECT COUNT(1) FROM student_solved WHERE user_id=?`, uid).Scan(&n); err != nil {
			return err
		}
		out = append(out, row{UserID: uid, Username: u.Username, Solved: n})
	}
	// 按通过数降序（同分按 user id）
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && (out[j].Solved > out[j-1].Solved || (out[j].Solved == out[j-1].Solved && out[j].UserID < out[j-1].UserID)); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"rank": out, "domainId": domainID})
}
