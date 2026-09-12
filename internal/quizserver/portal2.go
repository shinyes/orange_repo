// OrangeOJ 门户 API（续）：空间练习（整卷交卷）、空间刷题、排行榜。
package quizserver

import (
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"

	"orangeoj/internal/accounts"
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

// handlePortalPracticeDraftGet GET /api/portal/space/:id/practice/:pid/draft
// → 我的练习整卷作答草稿（云端续答：换设备后恢复已选答案）。
func (s *Server) handlePortalPracticeDraftGet(c *fiber.Ctx) error {
	spaceID, err := s.resolveSpace(c)
	if err != nil {
		return err
	}
	pid, err := paramID(c, "pid")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid practice id")
	}
	user := currentUser(c)
	p, _, err := s.QS.Repo.GetSpacePracticeBrief(pid, viewerID(user))
	if err != nil {
		return respondError(c, fiber.StatusNotFound, "练习不存在")
	}
	if p.SpaceID != spaceID {
		return respondError(c, fiber.StatusNotFound, "练习不存在")
	}
	answers, updated, ok, err := s.QS.LoadPracticeDraft(user.ID, pid)
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	if !ok {
		answers = "{}"
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"answers": json.RawMessage(answers), "updatedAt": updated})
}

// handlePortalPracticeDraftPut PUT /api/portal/space/:id/practice/:pid/draft {answers}
// → 保存整卷作答草稿（提交空对象/空表=清除草稿）。
func (s *Server) handlePortalPracticeDraftPut(c *fiber.Ctx) error {
	spaceID, err := s.resolveSpace(c)
	if err != nil {
		return err
	}
	pid, err := paramID(c, "pid")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid practice id")
	}
	user := currentUser(c)
	p, _, err := s.QS.Repo.GetSpacePracticeBrief(pid, viewerID(user))
	if err != nil {
		return respondError(c, fiber.StatusNotFound, "练习不存在")
	}
	if p.SpaceID != spaceID {
		return respondError(c, fiber.StatusNotFound, "练习不存在")
	}
	var req struct {
		Answers json.RawMessage `json:"answers"`
	}
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	// 校验 answers 为对象且字段值仅 number|boolean，防任意大/异常载荷
	var m map[string]json.RawMessage
	if err := json.Unmarshal(req.Answers, &m); err != nil {
		return respondError(c, fiber.StatusBadRequest, "answers 格式错误")
	}
	if len(m) > 1000 {
		return respondError(c, fiber.StatusBadRequest, "answers 过多")
	}
	for k, v := range m {
		if _, err := strconv.ParseInt(k, 10, 64); err != nil {
			return respondError(c, fiber.StatusBadRequest, "answers 键必须为题目 id")
		}
		var num json.Number
		var b bool
		if err := json.Unmarshal(v, &num); err != nil {
			if err2 := json.Unmarshal(v, &b); err2 != nil {
				return respondError(c, fiber.StatusBadRequest, "answers 值必须为序号或布尔")
			}
		}
	}
	if err := s.QS.SavePracticeDraft(user.ID, pid, string(req.Answers)); err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	return c.SendStatus(fiber.StatusNoContent)
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
			createdAt = sb.CreatedAt.Format(time.RFC3339)
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

// quizPickResult 抽题结果。
type quizPickResult struct {
	Problem    *quizstore.OJProblem `json:"problem"`
	Done       bool                 `json:"done"`
	EmptyRange bool                 `json:"emptyRange"` // 范围内无客观题可刷
	NewBatch   bool                 `json:"newBatch"`   // 开新一轮（错题复习优先）
	BatchNo    int                  `json:"batchNo"`
	WrongCnt   int                  `json:"wrongCnt"` // 会话中待纠正错题数
	// 本轮内的位置展示（前端显示 “第 x/y 题”；total=0 表示无限）
	Pos   int `json:"pos"`
	Total int `json:"total"`
}

// pickQuizProblem 按默认刷题规则抽一题：
//   - 范围 = 刷题项目题集（tags/repo 客观题）；批内不重复（drawn）
//   - 候选优先：本会话答错的 → 未 uuid 通过的 → 其余
//   - 本批覆盖完且仍有错题 → 自动开新批（错题下批优先复抽）
//   - 本批覆盖完且无错 → done（一轮完整刷完，用户可重开）
func (s *Server) pickQuizProblem(user *accounts.User, qid int64) (*quizPickResult, error) {
	// 会话锁：Get→Save 整体读改写需按 user×quiz 串行（防并发抽题丢 wrong/drawn）
	unlock := s.lockQuizSession(user.ID, qid)
	defer unlock()
	ids, err := s.quizProblemIDs(qid)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return &quizPickResult{Done: true, EmptyRange: true}, nil
	}
	solved, err := s.QS.SolvedUUIDs(user.ID)
	if err != nil {
		return nil, err
	}
	ss, err := s.QS.GetQuizSession(user.ID, qid)
	if err != nil {
		return nil, err
	}
	// 每轮题数（0=不限制：整范围为一轮）
	var roundSize int
	_ = s.QS.Repo.DB.QueryRow(`SELECT COALESCE(round_size,0) FROM space_quizzes WHERE id=?`, qid).Scan(&roundSize)
	if roundSize < 0 {
		roundSize = 0
	}
	// coveredOf = 跨批已抽出集合（历史 covered ∪ 本批 drawn）
	coveredOf := func() map[int64]bool {
		m := map[int64]bool{}
		for _, x := range ss.Covered {
			m[x] = true
		}
		for _, x := range ss.Drawn {
			m[x] = true
		}
		return m
	}
	allCovered := func() bool {
		m := coveredOf()
		for _, id := range ids {
			if !m[id] {
				return false
			}
		}
		return true
	}
	// 尝试开新批（把本批已抽并入 covered；保留错题袋），至多重试 3 次后兜底 done
	newBatch := false
	for attempt := 0; attempt < 3; attempt++ {
		drawn := ss.Drawn
		// 候选 = 范围中本批未抽的题
		candidates := make([]int64, 0, len(ids))
		for _, id := range ids {
			if !quizstore.ContainsInt64(drawn, id) {
				candidates = append(candidates, id)
			}
		}
		batchFull := roundSize > 0 && len(drawn) >= roundSize
		if len(candidates) == 0 || batchFull {
			// 本批结束（抽满每轮题数 / 本批池空）
			ss.Covered = append(ss.Covered, drawn...)
			ss.Drawn = []int64{}
			needMore := len(ss.Wrong) > 0 || !allCovered()
			if needMore {
				ss.BatchNo++
				newBatch = true
				continue
			}
			// 全范围已刷且无错 → 完成
			return &quizPickResult{Done: true, BatchNo: ss.BatchNo, WrongCnt: len(ss.Wrong)}, nil
		}
		// 分组加权：错题（本批未抽的）> 未通过 > 其余
		var wrongCand, freshCand, restCand []int64
		for _, id := range candidates {
			var u string
			_ = s.QS.Repo.DB.QueryRow(`SELECT uuid FROM problems WHERE id=?`, id).Scan(&u)
			switch {
			case quizstore.ContainsInt64(ss.Wrong, id):
				wrongCand = append(wrongCand, id)
			case u != "" && solved[u]:
				restCand = append(restCand, id)
			default:
				freshCand = append(freshCand, id)
			}
		}
		var pool []int64
		switch {
		case len(wrongCand) > 0:
			pool = wrongCand
		case len(freshCand) > 0:
			pool = freshCand
		default:
			pool = restCand
		}
		if len(pool) == 0 {
			// 理论不可达（candidates 非空），兜底全候选
			pool = candidates
		}
		pick := pool[randIntn(len(pool))]
		// 入批（本批已抽）
		ss.Drawn = append(drawn, pick)
		if err := s.QS.SaveQuizSession(user.ID, qid, ss); err != nil {
			return nil, err
		}
		problem, err := s.QS.Repo.GetOJProblem(pick)
		if err != nil {
			return nil, err
		}
		return &quizPickResult{
			Problem:  problem,
			BatchNo:  ss.BatchNo,
			NewBatch: newBatch,
			WrongCnt: len(ss.Wrong),
			// 本轮第几题：本批已抽计数（含本次）；每轮题数 0=无限
			Pos:   len(ss.Drawn),
			Total: roundSize,
		}, nil
	}
	return &quizPickResult{Done: true, BatchNo: ss.BatchNo}, nil
}

// handlePortalQuizProblem GET /api/portal/quiz/:qid/problem
// → 按默认刷题规则取一题（范围=项目题集；做过少做/错过多做/批内不重复）。
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
	// fresh=1（进入刷题页的首请求）：开新批——清空本批已抽（保留错题袋），
	// 避免上次会话残留的 drawn 导致一进入就判定“本轮已完成”
	if c.Query("fresh") == "1" {
		unlock := s.lockQuizSession(user.ID, qid)
		ss, serr := s.QS.GetQuizSession(user.ID, qid)
		if serr == nil && len(ss.Drawn) > 0 {
			ss.BatchNo++
			ss.Drawn = []int64{}
			serr = s.QS.SaveQuizSession(user.ID, qid, ss)
		}
		unlock()
		if serr != nil {
			return respondError(c, fiber.StatusInternalServerError, serr.Error())
		}
	}
	res, err := s.pickQuizProblem(user, qid)
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	return respondData(c, fiber.StatusOK, fiber.Map{
		"problem":    res.Problem,
		"done":       res.Done,
		"emptyRange": res.EmptyRange,
		"newBatch":   res.NewBatch,
		"batchNo":    res.BatchNo,
		"wrongCnt":   res.WrongCnt,
	})
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
	// 归属校验：题目必须属于该项目题集（防用任意域/任意题枚举答案密钥）
	// 归属校验：题目必须属于该项目题集（防用任意域/任意题枚举答案密钥）
	// 单条 EXISTS 判定（tags 源逐 tag 匹配同源逻辑，repo 源 JOIN 模板条目），避免全量拉题
	inScope, err := s.problemInQuizScope(spaceID, qid, req.ProblemID)
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	if !inScope {
		return respondError(c, fiber.StatusBadRequest, "题目不在该刷题项目范围内")
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
	// 同步刷题会话：答对→从错题袋移除；答错→加入错题袋（下批优先复抽）
	unlockSess := s.lockQuizSession(user.ID, qid)
	defer unlockSess() // 统一 defer：任一路径（含 panic 恢复转 500）都释放，防锁死
	ss, serr := s.QS.GetQuizSession(user.ID, qid)
	if serr != nil {
		return respondError(c, fiber.StatusInternalServerError, serr.Error())
	}
	if correct {
		if quizstore.ContainsInt64(ss.Wrong, req.ProblemID) {
			ss.Wrong = quizstore.RemoveInt64(ss.Wrong, req.ProblemID)
			if err := s.QS.SaveQuizSession(user.ID, qid, ss); err != nil {
				return respondError(c, fiber.StatusInternalServerError, err.Error())
			}
		}
		// 全局错题集同步：答对即移除
		if err := s.QS.RemoveWrongByProblem(user.ID, req.ProblemID); err != nil {
			return respondError(c, fiber.StatusInternalServerError, err.Error())
		}
	} else {
		// 会话错题袋（批内优先级）与全局错题集（幂等，独立于袋状态）
		if !quizstore.ContainsInt64(ss.Wrong, req.ProblemID) {
			ss.Wrong = append(ss.Wrong, req.ProblemID)
			if err := s.QS.SaveQuizSession(user.ID, qid, ss); err != nil {
				return respondError(c, fiber.StatusInternalServerError, err.Error())
			}
		}
		if err := s.QS.AddWrong(user.ID, req.ProblemID, qid); err != nil {
			return respondError(c, fiber.StatusInternalServerError, err.Error())
		}
	}
	return respondData(c, fiber.StatusOK, fiber.Map{
		"correct":       correct,
		"correctAnswer": correctAnswer,
		"firstTime":     correct && !already, // 首次通过（此前未记过）
		"wrongCnt":      len(ss.Wrong),
	})
}

// handlePortalQuizReset POST /api/portal/quiz/:qid/reset → 清空会话（重新开始一轮）。
func (s *Server) handlePortalQuizReset(c *fiber.Ctx) error {
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
	// 项目级可见性（开放 + 已分配）——与其他刷题入口一致，避免成员对不可见项目仍能重置会话
	vis, err := s.QS.Repo.QuizVisibleForUser(qid, viewerID(user))
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	if !vis {
		return respondError(c, fiber.StatusNotFound, "刷题项目不存在")
	}
	unlock := s.lockQuizSession(user.ID, qid)
	defer unlock()
	if err := s.QS.ResetQuizSession(user.ID, qid); err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// ---------- 排行榜 ----------

// handlePortalRank GET /api/portal/rank?domainId= → 按域学生通过数排行榜（管理员不参与）。
func (s *Server) handlePortalRank(c *fiber.Ctx) error {
	user := currentUser(c)
	domainID, err := s.rankDomainOf(c, user)
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	// 排行榜公开校验：域关闭公开（leaderboard_public=0）时，普通成员不可查看
	// （管理员始终可看；rankDomainOf 已把 member 限制到其加入空间所属域）
	if user.Role == accounts.RoleMember {
		var pub int
		if err := s.QS.Repo.DB.QueryRow(`SELECT COALESCE(leaderboard_public,1) FROM domains WHERE id=?`, domainID).Scan(&pub); err != nil {
			return respondError(c, fiber.StatusInternalServerError, err.Error())
		}
		if pub == 0 {
			return respondError(c, fiber.StatusForbidden, "排行榜未公开")
		}
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
