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

// handleImport 导入 OrangeOJ ZIP。mode=problems|training|practice（默认 problems）。
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
	resp, err := s.ImportZipData(data, c.Query("mode", "problems"))
	if err != nil {
		if ferr, ok := err.(*fiber.Error); ok {
			return respondError(c, ferr.Code, ferr.Message)
		}
		return err
	}
	return respondData(c, fiber.StatusCreated, resp)
}

// ImportZipData 导入核心：图片落盘 → 归一化插入题目 → 按模式建组。
// mode = problems | training | practice。
func (s *Server) ImportZipData(data []byte, mode string) (fiber.Map, error) {
	problems, meta, images, err := zipio.ParseZip(data)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, err.Error())
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
		description := ""
		var tags []string
		if meta != nil {
			if meta.Title != "" {
				title = meta.Title
			}
			description = meta.Description
			tags = meta.Tags
		}
		trainingID, err := s.Store.CreateTraining(title, description, tags)
		if err != nil {
			return nil, err
		}
		chapterCount := 0
		if meta != nil {
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
		}
		resp["trainingId"] = trainingID
		resp["chapters"] = chapterCount
	} else if mode == "practice" {
		title := "导入的练习"
		description := ""
		var tags []string
		if meta != nil {
			if meta.Title != "" {
				title = meta.Title
			}
			description = meta.Description
			tags = meta.Tags
		}
		practiceID, err := s.Store.CreatePractice(title, description, tags)
		if err != nil {
			return nil, err
		}
		if len(createdIDs) > 0 {
			if _, err := s.Store.AddPracticeItems(practiceID, createdIDs, 100); err != nil {
				return nil, err
			}
		}
		resp["practiceId"] = practiceID
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
	filter := store.ProblemFilter{Q: c.Query("q")}
	if tags := c.Query("tags"); tags != "" {
		for _, t := range strings.Split(tags, ",") {
			if t = strings.TrimSpace(t); t != "" {
				filter.Tags = append(filter.Tags, t)
			}
		}
	}
	if t := c.Query("type"); t != "" {
		filter.Type = t
	}
	if v := c.Query("dirId"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return respondError(c, fiber.StatusBadRequest, "invalid dirId")
		}
		filter.DirID = &id
		filter.Recursive = c.Query("recursive") == "1"
	}
	if idsParam := c.Query("ids"); idsParam != "" {
		for _, part := range strings.Split(idsParam, ",") {
			id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
			if err != nil || id <= 0 {
				return respondError(c, fiber.StatusBadRequest, "invalid ids")
			}
			filter.IDs = append(filter.IDs, id)
		}
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
