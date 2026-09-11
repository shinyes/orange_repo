// OrangeOJ 学生端做题 API（/api/oj）：
// 题目正文 → run/test/submit/objective-submit → 轮询 → 历史。
// 判题密钥（answerJson/testCases/题解）永不下发学生；判题一律服务端完成。
// 题目可见性 = 空间模型：用户加入的空间所在域包含该题。
package quizserver

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"orangeoj/internal/accounts"
	"orangeoj/internal/judge"
	"orangeoj/internal/quizstore"
)

// ---------- 可见性辅助 ----------

// problemVisibleToUser 题目是否对该用户可见。
// 管理员：domain_admin 其归属域内全部题可见；global_admin 全部可见（做题/预览/管理）。
// 成员：用户加入的空间所在域包含该题即可见（空间训练/练习/刷题引用域内题目）。
func (s *Server) problemVisibleToUser(user *accounts.User, problemID int64) (bool, error) {
	if isAdminRole(user.Role) {
		if user.Role == accounts.RoleGlobalAdmin {
			// 任意域题目（练习/预览）
			var n int
			err := s.QS.Repo.DB.QueryRow(`SELECT COUNT(1) FROM problems WHERE id=?`, problemID).Scan(&n)
			if err != nil {
				return false, err
			}
			return n > 0, nil
		}
		// domain_admin：其域题目可见
		if user.DomainID != nil {
			var n int
			err := s.QS.Repo.DB.QueryRow(`SELECT COUNT(1) FROM problems WHERE id=? AND domain_id=?`, problemID, *user.DomainID).Scan(&n)
			if err != nil {
				return false, err
			}
			return n > 0, nil
		}
		return false, nil
	}
	return s.problemVisibleViaSpaces(user.ID, problemID)
}

// problemVisibleViaSpaces 用户加入的空间所在域是否包含该题目。
func (s *Server) problemVisibleViaSpaces(userID, problemID int64) (bool, error) {
	spaces, err := s.QS.Repo.UserDomainSpaceIDs(userID)
	if err != nil {
		return false, err
	}
	if len(spaces) == 0 {
		return false, nil
	}
	var pdomain int64
	err = s.QS.Repo.DB.QueryRow(`SELECT domain_id FROM problems WHERE id=?`, problemID).Scan(&pdomain)
	if err != nil {
		return false, nil // 题目不存在/无域 → 不可见
	}
	for _, sp := range spaces {
		if sp.DomainID == pdomain {
			return true, nil
		}
	}
	return false, nil
}

// ---------- 题目正文与判题动作 ----------

// ojProblemView 下发题目正文（隐藏判题密钥；starter 为学生起始代码模板）。
type ojProblemView struct {
	ID             int64           `json:"id"`
	Type           string          `json:"type"`
	Title          string          `json:"title"`
	StatementMD    string          `json:"statementMd"`
	BodyJSON       json.RawMessage `json:"bodyJson"`
	StarterCpp     string          `json:"starterCpp,omitempty"`
	StarterPy      string          `json:"starterPy,omitempty"`
	TimeLimitMS    int             `json:"timeLimitMs"`
	MemoryLimitMiB int             `json:"memoryLimitMiB"`
}

// sanitizeOJBody 剥离编程题 testCases（判题密钥不下发学生）。
func sanitizeOJBody(p *quizstore.OJProblem) json.RawMessage {
	var body map[string]any
	if err := json.Unmarshal(p.BodyJSON, &body); err != nil || body == nil {
		return p.BodyJSON
	}
	delete(body, "testCases")
	delete(body, "testcases")
	b, _ := json.Marshal(body)
	return b
}

// requireVisibleProgramming 题目可见性校验 + 取编程题正文。
func (s *Server) requireVisibleProgramming(c *fiber.Ctx, problemID int64) (*quizstore.OJProblem, error) {
	user := currentUser(c)
	visible, err := s.problemVisibleToUser(user, problemID)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	if !visible {
		return nil, fiber.NewError(fiber.StatusNotFound, "题目不存在或不可见")
	}
	p, err := s.QS.Repo.GetOJProblem(problemID)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusNotFound, "题目不存在或不可见")
	}
	if p.Type != "programming" {
		return nil, fiber.NewError(fiber.StatusBadRequest, "该题目非编程题")
	}
	return p, nil
}

// handleOJProblem GET /api/oj/problem/:id
func (s *Server) handleOJProblem(c *fiber.Ctx) error {
	user := currentUser(c)
	problemID, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid id")
	}
	visible, err := s.problemVisibleToUser(user, problemID)
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	if !visible {
		return respondError(c, fiber.StatusNotFound, "题目不存在或不可见")
	}
	p, err := s.QS.Repo.GetOJProblem(problemID)
	if err != nil {
		return respondError(c, fiber.StatusNotFound, "题目不存在或不可见")
	}
	return respondData(c, fiber.StatusOK, ojProblemView{
		ID: p.ID, Type: p.Type, Title: p.Title, StatementMD: p.StatementMD,
		BodyJSON: sanitizeOJBody(p), StarterCpp: p.StarterCpp, StarterPy: p.StarterPy,
		TimeLimitMS: p.TimeLimitMS, MemoryLimitMiB: p.MemoryLimitMiB,
	})
}

// normalizeLanguage 语言归一化：仅 python/cpp。
func normalizeLanguage(lang string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(lang)) {
	case "python", "python3", "py":
		return "python", true
	case "cpp", "c++", "c":
		return "cpp", true
	}
	return "", false
}

type codeSubmitRequest struct {
	Language   string `json:"language"`
	SourceCode string `json:"sourceCode"`
	InputData  string `json:"inputData"`
	TrainingID int64  `json:"trainingId"` // >0=训练内提交（历史按训练×题隔离）
	PracticeID int64  `json:"practiceId"` // >0=练习内提交（历史按练习×题隔离）
}

// judgeEnabled 是否配置了 judge-runtime。
func (s *Server) judgeEnabled() bool {
	return s.Runner != nil && s.queue != nil
}

// handleOJCodeAction run/test/submit 共用。
func (s *Server) handleOJCodeAction(c *fiber.Ctx, submitType judge.SubmitType) error {
	if !s.judgeEnabled() {
		return respondError(c, fiber.StatusServiceUnavailable, "判题服务未配置（judge token）")
	}
	user := currentUser(c)
	problemID, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid id")
	}
	p, err := s.requireVisibleProgramming(c, problemID)
	if err != nil {
		if fiberErr, ok := err.(*fiber.Error); ok {
			return respondError(c, fiberErr.Code, fiberErr.Message)
		}
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	var req codeSubmitRequest
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	if len(req.SourceCode) > 512*1024 {
		return respondError(c, fiber.StatusBadRequest, "代码过长")
	}
	if len(req.InputData) > 256*1024 {
		return respondError(c, fiber.StatusBadRequest, "自定义输入过大")
	}
	lang, ok := normalizeLanguage(req.Language)
	if !ok {
		return respondError(c, fiber.StatusBadRequest, "仅支持 Python 与 C++")
	}
	if strings.TrimSpace(req.SourceCode) == "" {
		return respondError(c, fiber.StatusBadRequest, "代码不能为空")
	}
	if len(req.SourceCode) > 256*1024 {
		return respondError(c, fiber.StatusBadRequest, "代码过长")
	}
	submissionID, err := s.QS.CreateProgrammingSubmission(user.ID, p.ID, req.TrainingID, req.PracticeID, p.Type, lang, req.SourceCode, req.InputData, submitType)
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	return respondData(c, fiber.StatusCreated, fiber.Map{"submissionId": submissionID, "status": "queued"})
}

func (s *Server) handleOJRun(c *fiber.Ctx) error { return s.handleOJCodeAction(c, judge.SubmitTypeRun) }
func (s *Server) handleOJTest(c *fiber.Ctx) error {
	return s.handleOJCodeAction(c, judge.SubmitTypeTest)
}
func (s *Server) handleOJSubmit(c *fiber.Ctx) error {
	return s.handleOJCodeAction(c, judge.SubmitTypeSubmit)
}

// handleOJObjectiveSubmit 客观题同步判定（写 submissions + progress）。
func (s *Server) handleOJObjectiveSubmit(c *fiber.Ctx) error {
	user := currentUser(c)
	problemID, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid id")
	}
	var req struct {
		Answer json.RawMessage `json:"answer"`
	}
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	p, err := s.QS.Repo.GetOJProblem(problemID)
	if err != nil {
		return respondError(c, fiber.StatusNotFound, "题目不存在或不可见")
	}
	visible, err := s.problemVisibleToUser(user, problemID)
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	if !visible {
		return respondError(c, fiber.StatusNotFound, "题目不存在或不可见")
	}
	if p.Type == "programming" {
		return respondError(c, fiber.StatusBadRequest, "编程题请使用代码提交接口")
	}
	correct, err := s.gradeObjective(p.Type, problemID, req.Answer)
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	verdict := judge.VerdictWA
	score := 0
	if correct {
		verdict = judge.VerdictAC
		score = 100
	}
	submissionID, err := s.QS.CreateObjectiveSubmission(user.ID, problemID, p.Type, string(req.Answer), verdict, score)
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	if err := s.QS.UpsertProgress(user.ID, problemID, verdict, score, submissionID); err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	// 反馈正确答案（同期 objective 交互：答错时高亮正确项）
	correctAnswer := fiber.Map{}
	if p.Type == "single_choice" {
		if env, err := s.QS.Repo.GetObjectiveAnswer(problemID); err == nil && env.AnswerIndex != nil {
			correctAnswer["answerIndex"] = *env.AnswerIndex
		}
	} else if p.Type == "true_false" {
		if env, err := s.QS.Repo.GetObjectiveAnswer(problemID); err == nil && env.Answer != nil {
			correctAnswer["answer"] = *env.Answer
		}
	}
	return respondData(c, fiber.StatusOK, fiber.Map{
		"submissionId":  submissionID,
		"verdict":       verdict,
		"score":         score,
		"correct":       correct,
		"correctAnswer": correctAnswer,
	})
}

// gradeObjective 客观题判定（单选 answerIndex / 判断 answer 布尔）。
func (s *Server) gradeObjective(problemType string, problemID int64, answer json.RawMessage) (bool, error) {
	if len(answer) > 1024 {
		return false, errors.New("答案数据过大")
	}
	env, err := s.QS.Repo.GetObjectiveAnswer(problemID)
	if err != nil {
		return false, errors.New("题目答案缺失")
	}
	switch problemType {
	case "single_choice":
		if env.AnswerIndex == nil {
			return false, errors.New("题目答案缺失")
		}
		var got int
		if err := json.Unmarshal(answer, &got); err != nil {
			return false, errors.New("请提交选项序号")
		}
		return got == *env.AnswerIndex, nil
	case "true_false":
		if env.Answer == nil {
			return false, errors.New("题目答案缺失")
		}
		var got bool
		if err := json.Unmarshal(answer, &got); err != nil {
			return false, errors.New("请提交布尔答案")
		}
		return got == *env.Answer, nil
	}
	return false, errors.New("不支持的题型")
}

// handleOJSubmissions GET /api/oj/problem/:id/submissions（本人历史）。
func (s *Server) handleOJSubmissions(c *fiber.Ctx) error {
	user := currentUser(c)
	problemID, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid id")
	}
	visible, err := s.problemVisibleToUser(user, problemID)
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	if !visible {
		return respondError(c, fiber.StatusNotFound, "题目不存在或不可见")
	}
	// 训练/练习内历史隔离：?trainingId=N / ?practiceId=M 仅返回对应上下文内的提交
	var trainingID, practiceID int64
	if raw := strings.TrimSpace(c.Query("trainingId")); raw != "" {
		if tid, perr := strconv.ParseInt(raw, 10, 64); perr == nil && tid > 0 {
			trainingID = tid
		}
	}
	if raw := strings.TrimSpace(c.Query("practiceId")); raw != "" {
		if pid, perr := strconv.ParseInt(raw, 10, 64); perr == nil && pid > 0 {
			practiceID = pid
		}
	}
	list, err := s.QS.ListSubmissions(user.ID, problemID, trainingID, practiceID)
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	if list == nil {
		list = []quizstore.Submission{}
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"submissions": list})
}

// handleOJSubmissionPoll GET /api/oj/submission/:id/poll
func (s *Server) handleOJSubmissionPoll(c *fiber.Ctx) error {
	user := currentUser(c)
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid id")
	}
	sub, err := s.QS.GetSubmission(user.ID, id)
	if err != nil {
		if errors.Is(err, quizstore.ErrNotFound) {
			return respondError(c, fiber.StatusNotFound, "提交不存在")
		}
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	// 仅编程题轮询需要；客观题直接返回终态
	isFinal := sub.Status == "done" || sub.Status == "failed"
	verdict := sub.Verdict
	if sub.Status == "failed" {
		verdict = judge.VerdictRE
	}
	// 训练内编程题：AC 落定时标记该训练条目 solved（幂等；trainingId 由训练内嵌提交轮询携带）
	if isFinal && verdict == judge.VerdictAC && sub.SubmitType == judge.SubmitTypeSubmit {
		if tidRaw := strings.TrimSpace(c.Query("trainingId")); tidRaw != "" {
			if tid, perr := strconv.ParseInt(tidRaw, 10, 64); perr == nil && tid > 0 {
				// 可见性校验：该训练对当前用户可见（成员需在可见名单/空间成员，管理员豁免）
				_, _, verr := s.QS.Repo.GetSpaceTrainingBrief(tid, viewerID(user))
				if verr != nil {
					return respondError(c, fiber.StatusNotFound, "训练不存在或不可见")
				}
				// 校验该题确属该训练（防乱标）
				var ok bool
				_ = s.QS.Repo.DB.QueryRow(`SELECT COUNT(1)>0 FROM space_training_items i
					JOIN space_training_chapters c ON i.chapter_id=c.id
					WHERE c.training_id=? AND i.problem_id=?`, tid, sub.ProblemID).Scan(&ok)
				if ok {
					var uuid string
					_ = s.QS.Repo.DB.QueryRow(`SELECT uuid FROM problems WHERE id=?`, sub.ProblemID).Scan(&uuid)
					if err := s.QS.MarkTrainingProgrammingSolved(tid, user.ID, sub.ProblemID, uuid); err != nil {
						return respondError(c, fiber.StatusInternalServerError, err.Error())
					}
				}
			}
		}
	}
	return respondData(c, fiber.StatusOK, fiber.Map{
		"submissionId": sub.ID,
		"status":       sub.Status,
		"isFinal":      isFinal,
		"verdict":      verdict,
		"score":        sub.Score,
		"timeMs":       sub.TimeMS,
		"memoryKiB":    sub.MemoryKiB,
		"stdout":       sub.Stdout,
		"stderr":       sub.Stderr,
		"caseDetails":  sub.CaseDetails,
		"pollAfterMs":  1000,
	})
}

// draftCtx 解析草稿上下文（kind: training/practice/空=全局；id 对应项目）。
func draftCtx(kindRaw, idRaw string) (string, int64, bool) {
	kind := strings.TrimSpace(kindRaw)
	if kind != "" && kind != "training" && kind != "practice" {
		return "", 0, false
	}
	id := int64(0)
	if idRaw = strings.TrimSpace(idRaw); idRaw != "" {
		v, err := strconv.ParseInt(idRaw, 10, 64)
		if err != nil || v <= 0 {
			return "", 0, false
		}
		id = v
	}
	if kind != "" && id <= 0 {
		return "", 0, false
	}
	return kind, id, true
}

// handleOJGetDraft GET /api/oj/problem/:id/draft?lang=python|cpp[&ctxKind=&ctxId=] → 云端草稿。
// 返回 {code, language, updatedAt}：updatedAt 为最后保存时间（RFC3339，无草稿时为空），
// 供前端在多设备场景比较新旧、避免用旧草稿覆盖新草稿。
func (s *Server) handleOJGetDraft(c *fiber.Ctx) error {
	user := currentUser(c)
	problemID, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid id")
	}
	lang, ok := normalizeLanguage(c.Query("lang"))
	if !ok {
		return respondError(c, fiber.StatusBadRequest, "仅支持 Python 与 C++")
	}
	ctxKind, ctxID, okCtx := draftCtx(c.Query("ctxKind"), c.Query("ctxId"))
	if !okCtx {
		return respondError(c, fiber.StatusBadRequest, "无效的草稿上下文")
	}
	visible, err := s.problemVisibleToUser(user, problemID)
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	if !visible {
		return respondError(c, fiber.StatusNotFound, "题目不存在或不可见")
	}
	d, err := s.QS.GetDraft(user.ID, problemID, lang, ctxKind, ctxID)
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	updatedAt := ""
	if !d.UpdatedAt.IsZero() {
		updatedAt = d.UpdatedAt.Format(time.RFC3339)
	}
	return respondData(c, fiber.StatusOK, fiber.Map{
		"code": d.Code, "language": lang, "updatedAt": updatedAt,
	})
}

// handleOJSaveDraft PUT /api/oj/problem/:id/draft {language, code[, ctxKind, ctxId]} → 保存云端草稿。
func (s *Server) handleOJSaveDraft(c *fiber.Ctx) error {
	user := currentUser(c)
	problemID, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid id")
	}
	var req struct {
		Language string `json:"language"`
		Code     string `json:"code"`
		CtxKind  string `json:"ctxKind"`
		CtxID    *int64 `json:"ctxId"`
	}
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	lang, ok := normalizeLanguage(req.Language)
	if !ok {
		return respondError(c, fiber.StatusBadRequest, "仅支持 Python 与 C++")
	}
	// 上下文解析：kind 空=全局（忽略 id）；training/practice 必须带正 id；非法 kind 拒绝
	var ctxKind string
	var ctxID int64
	switch strings.TrimSpace(req.CtxKind) {
	case "":
	case "training", "practice":
		if req.CtxID == nil || *req.CtxID <= 0 {
			return respondError(c, fiber.StatusBadRequest, "无效的草稿上下文")
		}
		ctxKind = strings.TrimSpace(req.CtxKind)
		ctxID = *req.CtxID
	default:
		return respondError(c, fiber.StatusBadRequest, "无效的草稿上下文")
	}
	if len(req.Code) > 512*1024 {
		return respondError(c, fiber.StatusBadRequest, "草稿过大")
	}
	visible, err := s.problemVisibleToUser(user, problemID)
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	if !visible {
		return respondError(c, fiber.StatusNotFound, "题目不存在或不可见")
	}
	if err := s.QS.SaveDraft(user.ID, problemID, lang, ctxKind, ctxID, req.Code); err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	return c.SendStatus(fiber.StatusNoContent)
}
