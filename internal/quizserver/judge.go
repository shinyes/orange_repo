// OrangeOJ 学生端做题 API（/api/oj）：
// 布置可见的任务 → 题目正文 → run/test/submit/objective-submit → 轮询 → 历史。
// 判题密钥（answerJson/testCases/题解）永不下发学生；判题一律服务端完成。
package quizserver

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"

	"orangerepo/internal/judge"
	"orangerepo/internal/quizstore"
)

// ---------- 可见性辅助 ----------

// problemVisibleToUser 题目是否属于该学生可见的某布置任务。
func (s *Server) problemVisibleToUser(userID, problemID int64) (bool, error) {
	assigns, err := s.QS.ListStudentAssignments(userID, "training")
	if err != nil {
		return false, err
	}
	practices, err := s.QS.ListStudentAssignments(userID, "practice")
	if err != nil {
		return false, err
	}
	assigns = append(assigns, practices...)
	for _, a := range assigns {
		switch a.Kind {
		case "training":
			chapters, err := s.QS.Repo.TrainingProblemIDs(a.RepoID)
			if err != nil {
				if errors.Is(err, quizstore.ErrNotFound) {
					continue
				}
				return false, err
			}
			for _, ids := range chapters {
				for _, pid := range ids {
					if pid == problemID {
						return true, nil
					}
				}
			}
		case "practice":
			ids, err := s.QS.Repo.PracticeProblemIDs(a.RepoID)
			if err != nil {
				if errors.Is(err, quizstore.ErrNotFound) {
					continue
				}
				return false, err
			}
			for _, pid := range ids {
				if pid == problemID {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

// ---------- 任务列表与详情（学生视图） ----------

// ojTrainingBrief 学生训练任务视图。
type ojTrainingBrief struct {
	ID           int64    `json:"id"`
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	Tags         []string `json:"tags"`
	ProblemCount int      `json:"problemCount"`
	Accepted     int      `json:"accepted"`
	ChapterCount int      `json:"chapterCount"`
}

type ojPracticeBrief struct {
	ID           int64    `json:"id"`
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	Tags         []string `json:"tags"`
	ProblemCount int      `json:"problemCount"`
	Accepted     int      `json:"accepted"`
}

// handleOJAssigned 学生视角任务列表（训练/练习分开，仅可见项）。
func (s *Server) handleOJAssigned(c *fiber.Ctx) error {
	user := currentUser(c)
	trainings := []ojTrainingBrief{}
	trainingAssigns, err := s.QS.ListStudentAssignments(user.ID, "training")
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	accepted, err := s.QS.AcceptedProblemIDs(user.ID)
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	for _, a := range trainingAssigns {
		brief := ojTrainingBrief{ID: a.ID, Title: a.Title, Description: a.Description, Tags: a.Tags}
		chapters, err := s.QS.Repo.TrainingProblemIDs(a.RepoID)
		if err != nil {
			if errors.Is(err, quizstore.ErrNotFound) {
				continue // 主库训练已删
			}
			return respondError(c, fiber.StatusInternalServerError, err.Error())
		}
		brief.ChapterCount = len(chapters)
		counted := map[int64]bool{}
		for _, ids := range chapters {
			for _, pid := range ids {
				if !counted[pid] {
					counted[pid] = true
					brief.ProblemCount++
					if accepted[pid] {
						brief.Accepted++
					}
				}
			}
		}
		trainings = append(trainings, brief)
	}

	practices := []ojPracticeBrief{}
	practiceAssigns, err := s.QS.ListStudentAssignments(user.ID, "practice")
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	for _, a := range practiceAssigns {
		brief := ojPracticeBrief{ID: a.ID, Title: a.Title, Description: a.Description, Tags: a.Tags}
		ids, err := s.QS.Repo.PracticeProblemIDs(a.RepoID)
		if err != nil {
			if errors.Is(err, quizstore.ErrNotFound) {
				continue
			}
			return respondError(c, fiber.StatusInternalServerError, err.Error())
		}
		seen := map[int64]bool{}
		for _, pid := range ids {
			if !seen[pid] {
				seen[pid] = true
				brief.ProblemCount++
				if accepted[pid] {
					brief.Accepted++
				}
			}
		}
		practices = append(practices, brief)
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"trainings": trainings, "practices": practices})
}

// ojItem 列表项（训练/练习共用）。
type ojItem struct {
	ProblemID int64  `json:"problemId"`
	OrderNo   int    `json:"orderNo"`
	Title     string `json:"title"`
	Type      string `json:"type"`
	Completed bool   `json:"completed"`
}

// ojTrainingDetail 训练详情（章节化）。
type ojTrainingDetail struct {
	ID          int64             `json:"id"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Tags        []string          `json:"tags"`
	Chapters    []ojChapter       `json:"chapters"`
	Accepted    int               `json:"accepted"`
	Total       int               `json:"total"`
}

type ojChapter struct {
	ID      int64    `json:"id"`
	Title   string   `json:"title"`
	OrderNo int      `json:"orderNo"`
	Items   []ojItem `json:"items"`
}

// loadVisibleAssignment 学生读取布置详情（校验可见性后返回 assignment）。
func (s *Server) loadVisibleAssignment(c *fiber.Ctx, userID, id int64, kind string) (*quizstore.Assignment, error) {
	a, err := s.QS.GetAssignment(id)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusNotFound, "任务不存在")
	}
	if a.Kind != kind {
		return nil, fiber.NewError(fiber.StatusNotFound, "任务不存在")
	}
	visible, err := s.QS.StudentCanAccess(id, userID)
	if err != nil {
		return nil, err
	}
	if !visible {
		return nil, fiber.NewError(fiber.StatusNotFound, "任务不存在或未发布")
	}
	return a, nil
}

// fillItemsBrief 批量填充题目标题/题型（缺失题目跳过）。
func (s *Server) fillItemsBrief(items []ojItem) []ojItem {
	ids := make([]int64, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.ProblemID)
	}
	briefs, err := s.QS.Repo.GetProblemsBrief(ids)
	if err != nil {
		return items
	}
	out := items[:0]
	for _, it := range items {
		b, ok := briefs[it.ProblemID]
		if !ok {
			continue // 主库题目已删 → 条目隐藏
		}
		it.Title = b.Title
		it.Type = b.Type
		out = append(out, it)
	}
	return out
}

// handleOJTraining GET /api/oj/training/:id
func (s *Server) handleOJTraining(c *fiber.Ctx) error {
	user := currentUser(c)
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid id")
	}
	a, err := s.loadVisibleAssignment(c, user.ID, id, "training")
	if err != nil {
		if fiberErr, ok := err.(*fiber.Error); ok {
			return respondError(c, fiberErr.Code, fiberErr.Message)
		}
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	repo, err := s.QS.Repo.GetRepoTraining(a.RepoID)
	if err != nil {
		if errors.Is(err, quizstore.ErrNotFound) {
			return respondData(c, fiber.StatusOK, fiber.Map{"id": a.ID, "title": a.Title, "description": a.Description, "chapters": []ojChapter{}, "total": 0, "stale": true})
		}
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	accepted, err := s.QS.AcceptedProblemIDs(user.ID)
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	detail := ojTrainingDetail{ID: a.ID, Title: repo.Title, Description: repo.Description, Tags: repo.Tags, Chapters: []ojChapter{}}
	seen := map[int64]bool{}
	for _, ch := range repo.Chapters {
		oc := ojChapter{ID: ch.ID, Title: ch.Title, OrderNo: ch.OrderNo, Items: []ojItem{}}
		for i, pid := range ch.Items {
			oc.Items = append(oc.Items, ojItem{ProblemID: pid, OrderNo: i + 1, Completed: accepted[pid]})
		}
		oc.Items = s.fillItemsBrief(oc.Items)
		detail.Chapters = append(detail.Chapters, oc)
		for _, it := range oc.Items {
			if !seen[it.ProblemID] {
				seen[it.ProblemID] = true
				detail.Total++
				if accepted[it.ProblemID] {
					detail.Accepted++
				}
			}
		}
	}
	return respondData(c, fiber.StatusOK, detail)
}

// ojPracticeDetail 练习详情（平铺）。
type ojPracticeDetail struct {
	ID          int64    `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Items       []ojItem `json:"items"`
	Accepted    int      `json:"accepted"`
	Total       int      `json:"total"`
}

// handleOJPractice GET /api/oj/practice/:id
func (s *Server) handleOJPractice(c *fiber.Ctx) error {
	user := currentUser(c)
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid id")
	}
	a, err := s.loadVisibleAssignment(c, user.ID, id, "practice")
	if err != nil {
		if fiberErr, ok := err.(*fiber.Error); ok {
			return respondError(c, fiberErr.Code, fiberErr.Message)
		}
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	repo, err := s.QS.Repo.GetRepoPractice(a.RepoID)
	if err != nil {
		if errors.Is(err, quizstore.ErrNotFound) {
			return respondData(c, fiber.StatusOK, fiber.Map{"id": a.ID, "title": a.Title, "description": a.Description, "items": []ojItem{}, "total": 0, "stale": true})
		}
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	accepted, err := s.QS.AcceptedProblemIDs(user.ID)
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	detail := ojPracticeDetail{ID: a.ID, Title: repo.Title, Description: repo.Description, Tags: repo.Tags, Items: []ojItem{}}
	seen := map[int64]bool{}
	for i, pid := range repo.Items {
		detail.Items = append(detail.Items, ojItem{ProblemID: pid, OrderNo: i + 1, Completed: accepted[pid]})
	}
	detail.Items = s.fillItemsBrief(detail.Items)
	for _, it := range detail.Items {
		if !seen[it.ProblemID] {
			seen[it.ProblemID] = true
			detail.Total++
			if accepted[it.ProblemID] {
				detail.Accepted++
			}
		}
	}
	return respondData(c, fiber.StatusOK, detail)
}

// ---------- 题目正文与判题动作 ----------

// ojProblemView 下发题目正文（隐藏判题密钥）。
type ojProblemView struct {
	ID             int64           `json:"id"`
	Type           string          `json:"type"`
	Title          string          `json:"title"`
	StatementMD    string          `json:"statementMd"`
	BodyJSON       json.RawMessage `json:"bodyJson"`
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
	visible, err := s.problemVisibleToUser(user.ID, problemID)
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
	visible, err := s.problemVisibleToUser(user.ID, problemID)
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
		BodyJSON: sanitizeOJBody(p), TimeLimitMS: p.TimeLimitMS, MemoryLimitMiB: p.MemoryLimitMiB,
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
	submissionID, err := s.QS.CreateProgrammingSubmission(user.ID, p.ID, p.Type, lang, req.SourceCode, req.InputData, submitType)
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	return respondData(c, fiber.StatusCreated, fiber.Map{"submissionId": submissionID, "status": "queued"})
}

func (s *Server) handleOJRun(c *fiber.Ctx) error    { return s.handleOJCodeAction(c, judge.SubmitTypeRun) }
func (s *Server) handleOJTest(c *fiber.Ctx) error   { return s.handleOJCodeAction(c, judge.SubmitTypeTest) }
func (s *Server) handleOJSubmit(c *fiber.Ctx) error { return s.handleOJCodeAction(c, judge.SubmitTypeSubmit) }

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
	visible, err := s.problemVisibleToUser(user.ID, problemID)
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
	return respondData(c, fiber.StatusOK, fiber.Map{"submissionId": submissionID, "verdict": verdict, "score": score, "correct": correct})
}

// gradeObjective 客观题判定（单选 answerIndex / 判断 answer 布尔）。
func (s *Server) gradeObjective(problemType string, problemID int64, answer json.RawMessage) (bool, error) {
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
	visible, err := s.problemVisibleToUser(user.ID, problemID)
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	if !visible {
		return respondError(c, fiber.StatusNotFound, "题目不存在或不可见")
	}
	list, err := s.QS.ListSubmissions(user.ID, problemID)
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
