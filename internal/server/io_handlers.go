package server

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"orangerepo/internal/model"
	"orangerepo/internal/store"
	"orangerepo/internal/zipio"
)

// problemToExport 将存储实体转为导出条目。
func problemToExport(p *model.Problem) zipio.ExportProblem {
	return zipio.ExportProblem{
		Type:           string(p.Type),
		Title:          p.Title,
		Tags:           p.Tags,
		StatementMD:    p.StatementMD,
		BodyJSON:       p.BodyJSON,
		AnswerJSON:     p.AnswerJSON,
		Solutions:      p.Solutions,
		TimeLimitMS:    p.TimeLimitMS,
		MemoryLimitMiB: p.MemoryLimitMiB,
	}
}

// uploadResolver 从上传目录读取图片。
func (s *Server) uploadResolver(name string) ([]byte, error) {
	name = filepath.Base(filepath.ToSlash(name))
	if strings.Contains(name, "..") {
		return nil, fmt.Errorf("invalid image name")
	}
	return os.ReadFile(filepath.Join(s.UploadsDir, name))
}

// ---------- 导入 ----------

// handleImport 导入 OrangeOJ ZIP 包。mode=problems|training|practice|auto（默认 problems）。
// auto：按包内容自动识别——trainingPlan.json 含章节 → 训练，否则 → 练习。
func (s *Server) handleImport(c *fiber.Ctx) error {
	file, err := c.FormFile("zip")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "missing zip file")
	}
	if file.Size > 100<<20 {
		return respondError(c, fiber.StatusBadRequest, "ZIP 文件不能超过 100MB")
	}
	src, err := file.Open()
	if err != nil {
		return err
	}
	defer src.Close()
	data, err := io.ReadAll(src)
	if err != nil {
		return err
	}
	mode := strings.ToLower(strings.TrimSpace(c.Query("mode", "problems")))
	switch mode {
	case "problems", "training", "practice", "auto":
	default:
		return respondError(c, fiber.StatusBadRequest, "invalid mode: 仅支持 problems|training|practice|auto")
	}
	// 文件名作为题册名称兜底（去掉扩展名）
	nameHint := strings.TrimSuffix(filepath.Base(file.Filename), filepath.Ext(file.Filename))
	resp, err := s.ImportZipData(data, mode, nameHint)
	if err != nil {
		if ferr, ok := err.(*fiber.Error); ok {
			return respondError(c, ferr.Code, ferr.Message)
		}
		return err
	}
	return respondData(c, fiber.StatusCreated, resp)
}

// ImportZipData 导入核心：图片落盘 → 归一化插入题目 → 按模式建组。
// mode = problems | training | practice | auto；nameHint 为文件名（去扩展名），
// 作为题册名称兜底（元数据无标题时使用）。
func (s *Server) ImportZipData(data []byte, mode, nameHint string) (fiber.Map, error) {
	problems, meta, images, err := zipio.ParseZip(data)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	// auto：含章节结构 → 训练；否则 → 练习（平铺）
	if mode == "auto" {
		if meta != nil && len(meta.Chapters) > 0 {
			mode = "training"
		} else {
			mode = "practice"
		}
	}
	// Step 1: 落盘图片
	for name, content := range images {
		if _, err := s.SaveUpload(name, strings.NewReader(string(content))); err != nil {
			return nil, err
		}
	}
	// Step 2: 归一化并插入题目
	createdIDs := make([]int64, 0, len(problems))
	createdTitles := make([]fiber.Map, 0, len(problems))
	for i := range problems {
		p := problems[i]
		zipio.ApplyImportRewrite(&p)
		payload := zipio.ProblemPayload{
			Type: p.Type, Title: p.Title, Tags: p.Tags, StatementMD: p.StatementMD,
			BodyJSON: p.BodyJSON, AnswerJSON: p.AnswerJSON, Solutions: p.Solutions,
			TimeLimitMS: p.TimeLimitMS, MemoryLimitMiB: p.MemoryLimitMiB,
		}
		if err := zipio.NormalizeProblemPayload(&payload); err != nil {
			return nil, fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("题目 %q: %v", payload.Title, err))
		}
		id, err := s.Store.CreateProblem(model.Problem{
			Type:           model.ProblemType(payload.Type),
			Title:          payload.Title,
			Tags:           payload.Tags,
			StatementMD:    payload.StatementMD,
			BodyJSON:       payload.BodyJSON,
			AnswerJSON:     payload.AnswerJSON,
			Solutions:      payload.Solutions,
			TimeLimitMS:    payload.TimeLimitMS,
			MemoryLimitMiB: payload.MemoryLimitMiB,
		})
		if err != nil {
			return nil, err
		}
		createdIDs = append(createdIDs, id)
		createdTitles = append(createdTitles, fiber.Map{"id": id, "title": payload.Title})
	}

	resp := fiber.Map{"imported": createdTitles}

	// Step 3: 按模式建组
	if mode == "training" {
		title := "导入的训练"
		if nameHint != "" {
			title = nameHint
		}
		description := ""
		var tags []string
		if meta != nil {
			if meta.Title != "" {
				title = meta.Title
			}
			description = meta.Description
			tags = meta.Tags
		}
		trainingID, err := s.Store.CreateTraining(title, description, tags, nil)
		if err != nil {
			return nil, err
		}
		chapterCount := 0
		if meta != nil && len(meta.Chapters) > 0 {
			for _, ch := range meta.Chapters {
				cid, err := s.Store.CreateChapter(trainingID, ch.Title)
				if err != nil {
					return nil, err
				}
				var pids []int64
				for _, idx := range ch.ProblemIDs {
					if idx >= 0 && int(idx) < len(createdIDs) {
						pids = append(pids, createdIDs[idx])
					}
				}
				if len(pids) > 0 {
					if _, err := s.Store.AddChapterItems(cid, pids); err != nil {
						return nil, err
					}
				}
				chapterCount++
			}
		} else {
			// ZIP 未提供章节结构（缺 trainingPlan.json 或 chapters 为空）：
			// 自动创建「未分组」章节并收纳全部题目，避免题目被单独遗弃在题库中
			cid, err := s.Store.CreateChapter(trainingID, "未分组")
			if err != nil {
				return nil, err
			}
			if len(createdIDs) > 0 {
				if _, err := s.Store.AddChapterItems(cid, createdIDs); err != nil {
					return nil, err
				}
			}
			chapterCount = 1
		}
		resp["trainingId"] = trainingID
		resp["chapters"] = chapterCount
		resp["title"] = title
	} else if mode == "practice" {
		title := "导入的练习"
		if nameHint != "" {
			title = nameHint
		}
		description := ""
		var tags []string
		if meta != nil {
			if meta.Title != "" {
				title = meta.Title
			}
			description = meta.Description
			tags = meta.Tags
		}
		practiceID, err := s.Store.CreatePractice(title, description, tags, nil)
		if err != nil {
			return nil, err
		}
		if len(createdIDs) > 0 {
			if _, err := s.Store.AddPracticeItems(practiceID, createdIDs); err != nil {
				return nil, err
			}
		}
		resp["practiceId"] = practiceID
		resp["title"] = title
	}
	return resp, nil
}

// ---------- 导出 ----------

func sendZip(c *fiber.Ctx, data []byte, filename string) error {
	c.Set("Content-Type", "application/zip")
	c.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	return c.Send(data)
}

func exportFilename(prefix, name string) string {
	stamp := time.Now().Format("20060102-150405")
	base := prefix + "_" + stamp
	if n := strings.TrimSpace(name); n != "" {
		safe := strings.Map(func(r rune) rune {
			switch r {
			case '\\', '/', ':', '*', '?', '"', '<', '>', '|':
				return '_'
			}
			return r
		}, n)
		base = safe
	}
	return base + ".zip"
}

// handleExportProblems 导出题目：?ids=1,2 或按过滤参数，无参导出全部。
func (s *Server) handleExportProblems(c *fiber.Ctx) error {
	filter, err := parseProblemFilter(c)
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	list, err := s.Store.ListProblems(filter)
	if err != nil {
		return err
	}
	if len(list) == 0 {
		return respondError(c, fiber.StatusBadRequest, "no problems match the filter")
	}
	var entries []zipio.ExportProblem
	for _, sum := range list {
		p, err := s.Store.GetProblem(sum.ID)
		if err != nil {
			return err
		}
		entries = append(entries, problemToExport(p))
	}
	data, err := zipio.BuildZip(entries, nil, s.uploadResolver)
	if err != nil {
		return err
	}
	return sendZip(c, data, exportFilename("problems", c.Query("name")))
}

// handleExportTraining 导出训练：problems.json + trainingPlan.json（章节按下标引用）。
func (s *Server) handleExportTraining(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	t, err := s.Store.GetTraining(id)
	if err != nil {
		if err == store.ErrNotFound {
			return respondError(c, fiber.StatusNotFound, "training not found")
		}
		return err
	}
	chapters, err := s.Store.ListChapters(id)
	if err != nil {
		return err
	}
	var entries []zipio.ExportProblem
	var planChapters []zipio.PlanChapter
	for _, ch := range chapters {
		var indexes []int
		for _, it := range ch.Items {
			p, err := s.Store.GetProblem(it.ProblemID)
			if err != nil {
				if err == store.ErrNotFound {
					continue // 题目已被删除的悬空条目直接跳过
				}
				return err
			}
			indexes = append(indexes, len(entries))
			entries = append(entries, problemToExport(p))
		}
		planChapters = append(planChapters, zipio.PlanChapter{
			Title: ch.Title, OrderNo: ch.OrderNo, ProblemIDs: indexes,
		})
	}
	meta := &zipio.PlanMeta{Title: t.Title, Description: t.Description, Tags: t.Tags, Chapters: planChapters}
	data, err := zipio.BuildZip(entries, meta, s.uploadResolver)
	if err != nil {
		return err
	}
	return sendZip(c, data, exportFilename("training_"+strconv.FormatInt(id, 10), t.Title))
}

// handleExportPractice 导出练习：problems.json（平铺顺序）+ trainingPlan.json 单章结构。
func (s *Server) handleExportPractice(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	p, err := s.Store.GetPractice(id)
	if err != nil {
		if err == store.ErrNotFound {
			return respondError(c, fiber.StatusNotFound, "practice not found")
		}
		return err
	}
	items, err := s.Store.ListPracticeItems(id)
	if err != nil {
		return err
	}
	var entries []zipio.ExportProblem
	indexes := make([]int, 0, len(items))
	for _, it := range items {
		full, err := s.Store.GetProblem(it.ProblemID)
		if err != nil {
			if err == store.ErrNotFound {
				continue
			}
			return err
		}
		indexes = append(indexes, len(entries))
		entries = append(entries, problemToExport(full))
	}
	meta := &zipio.PlanMeta{
		Title: p.Title, Description: p.Description, Tags: p.Tags,
		Chapters: []zipio.PlanChapter{{Title: p.Title, OrderNo: 1, ProblemIDs: indexes}},
	}
	data, err := zipio.BuildZip(entries, meta, s.uploadResolver)
	if err != nil {
		return err
	}
	return sendZip(c, data, exportFilename("practice_"+strconv.FormatInt(id, 10), p.Title))
}
