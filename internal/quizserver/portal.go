// OrangeOJ 门户 API（空间化做题）：空间切换、空间训练（客观题限次作答标色）、
// 空间练习（整卷交卷）、空间刷题、排行榜。
// 结构只读自主库（RepoReader）；作答/进度/通过记录在 orangeoj.db（quizstore）。
package quizserver

import (
	"database/sql"
	"encoding/json"

	"github.com/gofiber/fiber/v2"

	"orangeoj/internal/accounts"
	"orangeoj/internal/quizstore"
)

// spaceLocals key。
const spaceLocals = "portal_space"

// resolveSpace 解析 URL :id 空间并校验当前用户成员/管理员身份，注入 Locals。
// domain_admin 限本域空间；global_admin 任意；member 须为空间成员。
func (s *Server) resolveSpace(c *fiber.Ctx) (int64, error) {
	id, err := paramID(c, "id")
	if err != nil {
		return 0, respondError(c, fiber.StatusBadRequest, "invalid space id")
	}
	return s.resolveSpaceCtx(c, id)
}

// handlePortalSpaces GET /api/portal/spaces → 我的空间（member 多空间；管理员列出其域空间）。
// 每个空间附带 canViewLeaderboard（管理员恒 true；成员取决于所属域排行榜是否公开），
// 供前端在「排行榜不公开」时直接隐藏该 tab。
func (s *Server) handlePortalSpaces(c *fiber.Ctx) error {
	user := currentUser(c)
	admin := isAdminRole(user.Role)
	if admin {
		// 管理员空间列表：domain_admin → 其域；global_admin → 全部（可再切域，此处列全部）
		spaces, err := s.QS.Repo.UserDomainSpaceIDs(user.ID) // member 关系可能为空
		if err != nil {
			return err
		}
		if user.Role == accounts.RoleDomainAdmin && user.DomainID != nil {
			spaces, err = s.spacesOfDomain(*user.DomainID)
			if err != nil {
				return err
			}
		} else if user.Role == accounts.RoleGlobalAdmin {
			spaces, err = s.spacesOfAllDomains()
			if err != nil {
				return err
			}
		}
		if err := s.annotateLeaderboardVisibility(spaces, true); err != nil {
			return err
		}
		return respondData(c, fiber.StatusOK, fiber.Map{"spaces": spaces})
	}
	spaces, err := s.QS.Repo.UserDomainSpaceIDs(user.ID)
	if err != nil {
		return err
	}
	if err := s.annotateLeaderboardVisibility(spaces, false); err != nil {
		return err
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"spaces": spaces})
}

// annotateLeaderboardVisibility 为空间列表补 canViewLeaderboard：
// 管理员恒可见；成员按所属域 domains.leaderboard_public（缺省视为公开）判定。
func (s *Server) annotateLeaderboardVisibility(spaces []quizstore.SpaceBrief, admin bool) error {
	if admin {
		for i := range spaces {
			spaces[i].CanViewLeaderboard = true
		}
		return nil
	}
	flags := map[int64]bool{}
	for i := range spaces {
		did := spaces[i].DomainID
		pub, ok := flags[did]
		if !ok {
			if err := s.QS.Repo.DB.QueryRow(
				`SELECT COALESCE(leaderboard_public,1) FROM domains WHERE id=?`, did).Scan(&pub); err != nil {
				// 域缺失/查询失败时保守放开（与既有 403 兜底一致，避免误隐藏）
				pub = true
			}
			flags[did] = pub
		}
		spaces[i].CanViewLeaderboard = pub
	}
	return nil
}

// spacesOfDomain 域内全部空间（含域名信息由前端另取；此处带 domainId）。
func (s *Server) spacesOfDomain(domainID int64) ([]quizstore.SpaceBrief, error) {
	ids, err := s.QS.Repo.DomainSpaceIDs(domainID)
	if err != nil {
		return nil, err
	}
	out := make([]quizstore.SpaceBrief, 0, len(ids))
	for _, id := range ids {
		sp, err := s.spaceBrief(id)
		if err != nil {
			return nil, err
		}
		out = append(out, sp)
	}
	return out, nil
}

func (s *Server) spaceBrief(id int64) (quizstore.SpaceBrief, error) {
	// 空间 + 域名一次查询
	var b quizstore.SpaceBrief
	var dName sql.NullString
	if err := s.QS.Repo.DB.QueryRow(`SELECT sp.id,sp.domain_id,d.name,sp.name FROM spaces sp
		LEFT JOIN domains d ON d.id=sp.domain_id WHERE sp.id=?`, id).
		Scan(&b.ID, &b.DomainID, &dName, &b.Name); err != nil {
		return quizstore.SpaceBrief{}, err
	}
	if dName.Valid {
		b.DomainName = dName.String
	}
	return b, nil
}

func (s *Server) spacesOfAllDomains() ([]quizstore.SpaceBrief, error) {
	rows, err := s.QS.Repo.DB.Query(`SELECT sp.id,sp.domain_id,d.name,sp.name FROM spaces sp
		LEFT JOIN domains d ON d.id=sp.domain_id ORDER BY sp.domain_id,sp.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []quizstore.SpaceBrief
	for rows.Next() {
		var b quizstore.SpaceBrief
		var dName sql.NullString
		if err := rows.Scan(&b.ID, &b.DomainID, &dName, &b.Name); err != nil {
			return nil, err
		}
		if dName.Valid {
			b.DomainName = dName.String
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// handlePortalSpaceHome GET /api/portal/space/:id/home → 三区概览（member 仅见已分配项目）。
func (s *Server) handlePortalSpaceHome(c *fiber.Ctx) error {
	spaceID, err := s.resolveSpace(c)
	if err != nil {
		return err
	}
	vid := viewerID(currentUser(c))
	trainings, err := s.QS.Repo.ListSpaceTrainingsBrief(spaceID, vid)
	if err != nil {
		return err
	}
	practices, err := s.QS.Repo.ListSpacePracticesBrief(spaceID, vid)
	if err != nil {
		return err
	}
	quizzes, err := s.QS.Repo.ListSpaceQuizzesBrief(spaceID, vid)
	if err != nil {
		return err
	}
	return respondData(c, fiber.StatusOK, fiber.Map{
		"trainings": trainings, "practices": practices, "quizzes": quizzes,
	})
}

// handlePortalTraining GET /api/portal/space/:id/training/:tid
// → 训练详情：章节+条目，附用户状态（题目类型客观：green/red/尝试次数/剩余）。
func (s *Server) handlePortalTraining(c *fiber.Ctx) error {
	spaceID, err := s.resolveSpace(c)
	if err != nil {
		return err
	}
	tid, err := paramID(c, "tid")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid training id")
	}
	user := currentUser(c)
	tr, chapters, err := s.QS.Repo.GetSpaceTrainingBrief(tid, viewerID(user))
	if err != nil {
		return respondError(c, fiber.StatusNotFound, "训练不存在")
	}
	if tr.SpaceID != spaceID {
		return respondError(c, fiber.StatusNotFound, "训练不存在")
	}
	// 用户作答状态（客观题；member 才有尝试记录；管理员无作答）
	// 回顾答案：仅当该题已通过或已达上限时下发（答完/锁定后展示正确答案是合理学习行为，
	// 未作答/还有次数时不下发，避免泄露判题密钥）。
	type answerView struct {
		AnswerIndex *int  `json:"answerIndex,omitempty"`
		Answer      *bool `json:"answer,omitempty"`
	}
	type itemView struct {
		quizstore.SpaceTrainingItem
		Solved        bool        `json:"solved"`
		Attempts      int         `json:"attempts"`
		Locked        bool        `json:"locked"` // 达上限未对=红；或已对=绿锁定
		CorrectAnswer *answerView `json:"correctAnswer,omitempty"`
	}
	type chapterView struct {
		ID     int64      `json:"id"`
		Title  string     `json:"title"`
		Items  []itemView `json:"items"`
	}
	out := make([]chapterView, 0, len(chapters))
	for _, ch := range chapters {
		cv := chapterView{ID: ch.ID, Title: ch.Title, Items: []itemView{}}
		for _, it := range ch.Items {
			iv := itemView{SpaceTrainingItem: it}
			if it.ProblemType == "" {
				cv.Items = append(cv.Items, iv)
				continue
			}
			// 客观题与编程题都读尝试状态：编程 AC 由 poll 写 attempts.solved=1，
			// detail 须回传 solved=true 供导航格子变绿/标题“已通过”
			st, err := s.QS.GetTrainingAttempt(tid, user.ID, it.ProblemID)
			if err == nil && st != nil {
				iv.Solved = st.Solved
				iv.Attempts = st.Attempts
				if it.ProblemType != "programming" {
					max := tr.MaxAttempts
					iv.Locked = st.Solved || (max > 0 && st.Attempts >= max)
					// 已通过/达限 → 附正确答案供回顾标色
					if iv.Solved || iv.Locked {
						if env, aerr := s.QS.Repo.GetAnswer(it.ProblemID); aerr == nil && env != nil {
							av := &answerView{AnswerIndex: env.AnswerIndex, Answer: env.Answer}
							iv.CorrectAnswer = av
						}
					}
				}
			}
			cv.Items = append(cv.Items, iv)
		}
		out = append(out, cv)
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"training": tr, "chapters": out})
}

// handlePortalTrainingAnswer POST /api/portal/space/:id/training/:tid/answer
// {problemId, answer} → {correct, attempts, solved, locked, correctAnswer?}
// 规则：客观题限次——达 max_attempts 或已 solved 时拒绝再答（403-ish 语义用 400 说明）。
func (s *Server) handlePortalTrainingAnswer(c *fiber.Ctx) error {
	spaceID, err := s.resolveSpace(c)
	if err != nil {
		return err
	}
	tid, err := paramID(c, "tid")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid training id")
	}
	user := currentUser(c)
	tr, chapters, err := s.QS.Repo.GetSpaceTrainingBrief(tid, viewerID(user))
	if err != nil {
		return respondError(c, fiber.StatusNotFound, "训练不存在")
	}
	if tr.SpaceID != spaceID {
		return respondError(c, fiber.StatusNotFound, "训练不存在")
	}
	var req struct {
		ProblemID int64           `json:"problemId"`
		Answer    json.RawMessage `json:"answer"`
	}
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	// 确认题目在该训练内且为客观题
	_, problemType, problemUUID, ok := s.findTrainingItem(chapters, req.ProblemID)
	if !ok {
		return respondError(c, fiber.StatusBadRequest, "题目不在该训练中")
	}
	if problemType == "programming" {
		return respondError(c, fiber.StatusBadRequest, "编程题不在此作答接口（请使用代码提交）")
	}
	// 限次/锁定判定
	st, err := s.QS.GetTrainingAttempt(tid, user.ID, req.ProblemID)
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	if st.Solved || (tr.MaxAttempts > 0 && st.Attempts >= tr.MaxAttempts) {
		return respondError(c, fiber.StatusConflict, "该题已锁定（答对或次数用尽）")
	}
	correct, err := s.gradeObjective(problemType, req.ProblemID, req.Answer)
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	attempts, solved, err := s.QS.RecordTrainingAttempt(tid, user.ID, req.ProblemID, problemUUID, correct)
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	// 次数用尽（限次且已达上限）才算「可回顾」，此时才揭示正确答案；
	// 未用尽时只反馈对错，避免学生答错一次就看到答案。
	expired := tr.MaxAttempts > 0 && attempts >= tr.MaxAttempts
	locked := solved || expired
	correctAnswer := fiber.Map{}
	if !correct && expired {
		if problemType == "single_choice" {
			if env, err := s.QS.Repo.GetObjectiveAnswer(req.ProblemID); err == nil && env.AnswerIndex != nil {
				correctAnswer["answerIndex"] = *env.AnswerIndex
			}
		} else if problemType == "true_false" {
			if env, err := s.QS.Repo.GetObjectiveAnswer(req.ProblemID); err == nil && env.Answer != nil {
				correctAnswer["answer"] = *env.Answer
			}
		}
	}
	// 剩余次数（-1=不限次），供前端提示「还可再答 N 次」
	remaining := -1
	if tr.MaxAttempts > 0 {
		remaining = tr.MaxAttempts - attempts
		if remaining < 0 {
			remaining = 0
		}
	}
	return respondData(c, fiber.StatusOK, fiber.Map{
		"correct":       correct,
		"attempts":      attempts,
		"solved":        solved,
		"locked":        locked,
		"remaining":     remaining,
		"correctAnswer": correctAnswer,
	})
}

// findTrainingItem 在章节中找条目（返回 problemType/uuid）。
func (s *Server) findTrainingItem(chapters []quizstore.SpaceTrainingChapter, problemID int64) (itemID int64, typ, uuid string, ok bool) {
	for _, ch := range chapters {
		for _, it := range ch.Items {
			if it.ProblemID == problemID {
				return it.ID, it.ProblemType, it.ProblemUUID, true
			}
		}
	}
	return 0, "", "", false
}
