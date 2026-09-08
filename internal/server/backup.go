// OrangeOJ 全库备份/迁移（backup）：
//   - 导出：全部题目 + 目录树 + 训练（含章节）/练习 打包为单 ZIP。
//     包内 problems.json 为全部题目（OrangeOJ 兼容），根另附 orangerepo-backup.json
//     记录目录/训练/练习结构与题目下标引用（OrangeOJ/旧版导入自然忽略该文件）。
//   - 导入：识别含 orangerepo-backup.json 的包 → 全库恢复；恢复一律新建，
//     不覆盖已有数据（同名也重建为副本）。无该文件则走既有 OrangeOJ 导入。
package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/gofiber/fiber/v2"

	"orangeoj/internal/model"
	"orangeoj/internal/store"
	"orangeoj/internal/zipio"
)

// BackupJSONName 全库备份清单文件名（位于包根，problems.json 之外）。
const BackupJSONName = "orangerepo-backup.json"

// BackupProblemIndex 训练/练习中题目以 problems.json 数组下标引用。
type backupProblem = zipio.ExportProblem

// backupChapter 训练章节（题目按下标引用）。
type backupChapter struct {
	Title      string `json:"title"`
	OrderNo    int    `json:"orderNo"`
	ProblemIDs []int  `json:"problemIds"`
}

// backupTraining 训练条目。
type backupTraining struct {
	UUID        string          `json:"uuid,omitempty"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Tags        []string        `json:"tags"`
	Folder      string          `json:"folder,omitempty"` // 目录名路径（/ 分隔），空=根
	Chapters    []backupChapter `json:"chapters"`
}

// backupPractice 练习条目。
type backupPractice struct {
	UUID        string   `json:"uuid,omitempty"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Folder      string   `json:"folder,omitempty"`
	ProblemIDs  []int    `json:"problemIds"`
}

// backupDirectory 目录条目：以 parent 名路径表达层级（根目录 parent=""）。
type backupDirectory struct {
	Name   string `json:"name"`
	Parent string `json:"parent,omitempty"`
}

// backupManifest 全库备份清单。
type backupManifest struct {
	Version     int               `json:"version"`
	Directories []backupDirectory `json:"directories,omitempty"`
	Trainings   []backupTraining  `json:"trainings,omitempty"`
	Practices   []backupPractice  `json:"practices,omitempty"`
}

// ---------- 导出 ----------

// dirPathOf 递归求目录名路径（根返回 ""）。
func (s *Server) dirPathOf(dirs []model.BookletDirectory, id int64) string {
	var find func(int64) []string
	find = func(cur int64) []string {
		for _, d := range dirs {
			if d.ID == cur {
				if d.ParentID != nil {
					return append(find(*d.ParentID), d.Name)
				}
				return []string{d.Name}
			}
		}
		return nil
	}
	return strings.Join(find(id), "/")
}

// buildBackup 组装全库清单与题目数组（problems.json 顺序即下标）。
func (s *Server) buildBackup() (*backupManifest, []zipio.ExportProblem, error) {
	manifest := &backupManifest{Version: 1}

	// 题目：全量导出（含被训练/练习引用与未被引用的）
	all, err := s.Store.ListProblems(store.ProblemFilter{})
	if err != nil {
		return nil, nil, err
	}
	indexOf := map[int64]int{}
	entries := make([]zipio.ExportProblem, 0, len(all))
	for _, sum := range all {
		p, err := s.Store.GetProblem(sum.ID)
		if err != nil {
			return nil, nil, err
		}
		indexOf[p.ID] = len(entries)
		entries = append(entries, problemToExport(p))
	}

	// 目录树
	dirs, err := s.Store.ListBookletDirectories()
	if err != nil {
		return nil, nil, err
	}
	pathOf := func(id *int64) string {
		if id == nil {
			return ""
		}
		return s.dirPathOf(dirs, *id)
	}
	for _, d := range dirs {
		parent := ""
		if d.ParentID != nil {
			parent = pathOf(d.ParentID)
		}
		manifest.Directories = append(manifest.Directories, backupDirectory{Name: d.Name, Parent: parent})
	}

	// 训练
	trainings, err := s.Store.ListTrainings()
	if err != nil {
		return nil, nil, err
	}
	for _, t := range trainings {
		bt := backupTraining{UUID: t.UUID, Title: t.Title, Description: t.Description, Tags: t.Tags, Folder: pathOf(t.FolderID)}
		chapters, err := s.Store.ListChapters(t.ID)
		if err != nil {
			return nil, nil, err
		}
		for _, ch := range chapters {
			bc := backupChapter{Title: ch.Title, OrderNo: ch.OrderNo}
			for _, it := range ch.Items {
				idx, ok := indexOf[it.ProblemID]
				if !ok {
					continue // 悬空引用跳过
				}
				bc.ProblemIDs = append(bc.ProblemIDs, idx)
			}
			bt.Chapters = append(bt.Chapters, bc)
		}
		manifest.Trainings = append(manifest.Trainings, bt)
	}

	// 练习
	practices, err := s.Store.ListPractices()
	if err != nil {
		return nil, nil, err
	}
	for _, p := range practices {
		bp := backupPractice{UUID: p.UUID, Title: p.Title, Description: p.Description, Tags: p.Tags, Folder: pathOf(p.FolderID)}
		items, err := s.Store.ListPracticeItems(p.ID)
		if err != nil {
			return nil, nil, err
		}
		for _, it := range items {
			if idx, ok := indexOf[it.ProblemID]; ok {
				bp.ProblemIDs = append(bp.ProblemIDs, idx)
			}
		}
		manifest.Practices = append(manifest.Practices, bp)
	}
	return manifest, entries, nil
}

// handleExportBackup GET /api/export/backup → 全库 ZIP。
func (s *Server) handleExportBackup(c *fiber.Ctx) error {
	manifest, entries, err := s.buildBackup()
	if err != nil {
		return err
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	data, err := zipio.BuildZipWithFiles(entries, nil, s.uploadResolver, map[string][]byte{BackupJSONName: manifestJSON})
	if err != nil {
		return err
	}
	return sendZip(c, data, exportFilename("OrangeOJ_full_backup", ""))
}

// ---------- 导入 ----------

// folderIDByPath 依名称路径逐级查找/创建目录，返回叶子目录 id。
func (s *Server) folderIDByPath(dirs []model.BookletDirectory, path string) (*int64, error) {
	path = strings.Trim(path, "/")
	if path == "" {
		return nil, nil
	}
	segments := strings.Split(path, "/")
	var parentID *int64
	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		var found *int64
		for _, d := range dirs {
			var dParent *int64
			if d.ParentID != nil {
				p := *d.ParentID
				dParent = &p
			}
			if d.Name == seg && (parentID == nil && dParent == nil || parentID != nil && dParent != nil && *parentID == *dParent) {
				id := d.ID
				found = &id
				break
			}
		}
		if found == nil {
			id, err := s.Store.CreateBookletDirectory(seg, parentID)
			if err != nil {
				return nil, err
			}
			found = &id
			dirs = append(dirs, model.BookletDirectory{ID: id, Name: seg, ParentID: parentID})
		}
		parentID = found
	}
	return parentID, nil
}

// backupScope 备份恢复的目标域：query domainId → 默认域自动（域管理员强制其域）。
func (s *Server) backupScope(c *fiber.Ctx) *int64 {
	scope, err := s.domainOrDefault(c, currentUser(c))
	if err != nil {
		return nil
	}
	return scope
}

// importBackup 全库恢复（严格模式）：预校验全部数据 → 逐段写入并记录创建资源；
// 任何一步失败立即报错并补偿回滚（删除本次已建题目/目录/训练/练习与落盘图片），
// 不留半导入状态。
//
// prog（可 nil）为进度回调：各阶段内以 prog(phase, cur, total) 上报当前进度，cur 从 1 起。
// phase 取值：题目/训练/练习（import_task.go 常量）。回调仅为上报，绝不改变导入语义。
func (s *Server) importBackup(manifest *backupManifest, problems []zipio.ExportProblem, domainID *int64, prog func(phase string, cur, total int)) (err error) {
	if manifest.Version != 1 {
		return errors.New("不支持的备份版本")
	}

	// ---------- 阶段 0：严格预校验（失败不写任何数据） ----------
	for i := range problems {
		p := problems[i]
		payload := zipio.ProblemPayload{
			UUID: p.UUID, Type: p.Type, Title: p.Title, Tags: p.Tags, StatementMD: p.StatementMD,
			BodyJSON: p.BodyJSON, AnswerJSON: p.AnswerJSON, Solutions: p.Solutions,
			StarterCpp: p.StarterCpp, StarterPy: p.StarterPy,
			TimeLimitMS: p.TimeLimitMS, MemoryLimitMiB: p.MemoryLimitMiB,
		}
		if err := zipio.NormalizeProblemPayload(&payload); err != nil {
			return fmt.Errorf("题目 %d（%q）不符合要求: %v", i+1, payload.Title, err)
		}
	}
	referRange := func(owner string, ids []int) error {
		for _, idx := range ids {
			if idx < 0 || idx >= len(problems) {
				return fmt.Errorf("%s 引用不存在的题目下标 %d（共 %d 题）", owner, idx, len(problems))
			}
		}
		return nil
	}
	for _, bt := range manifest.Trainings {
		if strings.TrimSpace(bt.Title) == "" {
			return fmt.Errorf("训练 %q 标题为空", bt.Title)
		}
		for _, bc := range bt.Chapters {
			if err := referRange(fmt.Sprintf("训练 %q 的章节 %q", bt.Title, bc.Title), bc.ProblemIDs); err != nil {
				return err
			}
		}
	}
	for _, bp := range manifest.Practices {
		if strings.TrimSpace(bp.Title) == "" {
			return fmt.Errorf("练习 %q 标题为空", bp.Title)
		}
		if err := referRange(fmt.Sprintf("练习 %q", bp.Title), bp.ProblemIDs); err != nil {
			return err
		}
	}
	for _, d := range manifest.Directories {
		if strings.TrimSpace(d.Name) == "" || strings.Contains(d.Name, "/") {
			return fmt.Errorf("题册目录名非法: %q", d.Name)
		}
	}

	// 回滚补偿：记录本次创建的资源（图片回删由调用方 runImportBackupTask 负责）
	type rollback struct {
		problemIDs  []int64
		trainingIDs []int64
		practiceIDs []int64
	}
	rb := &rollback{}
	// 导入前目录 id 集合（回滚时清理本次新建的空目录）
	initialDirs, err := s.Store.ListBookletDirectories()
	if err != nil {
		return err
	}
	initialDirIDs := map[int64]bool{}
	for _, d := range initialDirs {
		initialDirIDs[d.ID] = true
	}
	commit := false
	defer func() {
		if commit || err == nil {
			return
		}
		// 补偿回滚：题目（级联模板条目）→ 训练 → 练习 → 新建目录（尽力删空目录）
		for _, id := range rb.problemIDs {
			_ = s.Store.DeleteProblem(id)
		}
		for _, id := range rb.trainingIDs {
			_ = s.Store.DeleteTraining(id)
		}
		for _, id := range rb.practiceIDs {
			_ = s.Store.DeletePractice(id)
		}
		if dirs, lerr := s.Store.ListBookletDirectories(); lerr == nil {
			for i := len(dirs) - 1; i >= 0; i-- {
				if !initialDirIDs[dirs[i].ID] {
					_ = s.Store.DeleteBookletDirectory(dirs[i].ID, false) // 非空目录删除失败则跳过
				}
			}
		}
	}()

	// ---------- 1) 题目 ----------
	createdIDs := make([]int64, len(problems))
	for i := range problems {
		if prog != nil {
			prog(importPhaseProblems, i+1, len(problems)) // 含 uuid 去重命中/新建
		}
		p := problems[i]
		zipio.ApplyImportRewrite(&p)
		payload := zipio.ProblemPayload{
			UUID: p.UUID, Type: p.Type, Title: p.Title, Tags: p.Tags, StatementMD: p.StatementMD,
			BodyJSON: p.BodyJSON, AnswerJSON: p.AnswerJSON, Solutions: p.Solutions,
			StarterCpp: p.StarterCpp, StarterPy: p.StarterPy,
			TimeLimitMS: p.TimeLimitMS, MemoryLimitMiB: p.MemoryLimitMiB,
		}
		prob := model.Problem{
			UUID:           payload.UUID,
			DomainID:       domainID,
			Type:           model.ProblemType(payload.Type),
			Title:          payload.Title,
			Tags:           payload.Tags,
			StatementMD:    payload.StatementMD,
			BodyJSON:       payload.BodyJSON,
			AnswerJSON:     payload.AnswerJSON,
			Solutions:      payload.Solutions,
			StarterCpp:     payload.StarterCpp,
			StarterPy:      payload.StarterPy,
			TimeLimitMS:    payload.TimeLimitMS,
			MemoryLimitMiB: payload.MemoryLimitMiB,
		}
		if prob.UUID != "" && domainID != nil {
			id, err := s.Store.ProblemIDByUUIDInDomain(prob.UUID, domainID)
			if err == nil {
				createdIDs[i] = id
				continue
			}
			if err != store.ErrNotFound {
				return err
			}
		}
		id, err := s.Store.CreateProblem(prob)
		if err != nil {
			return err
		}
		createdIDs[i] = id
		rb.problemIDs = append(rb.problemIDs, id)
	}

	// 2) 目录树（manifest.Directories 顺序即创建序——父先于子）
	dirs, err := s.Store.ListBookletDirectories()
	if err != nil {
		return err
	}
	for _, d := range manifest.Directories {
		if _, err := s.folderIDByPath(dirs, strings.Trim(d.Parent+"/"+d.Name, "/")); err != nil {
			return err
		}
		// 刷新目录缓存以便兄弟/后续引用
		dirs, err = s.Store.ListBookletDirectories()
		if err != nil {
			return err
		}
	}

	// 3) 训练
	for i, bt := range manifest.Trainings {
		if prog != nil {
			prog(importPhaseTrainings, i+1, len(manifest.Trainings))
		}
		folder, err := s.folderIDByPath(dirs, bt.Folder)
		if err != nil {
			return err
		}
		trID, err := s.Store.CreateTraining(bt.Title, bt.Description, bt.Tags, folder)
		if err != nil {
			return err
		}
		rb.trainingIDs = append(rb.trainingIDs, trID)
		for _, bc := range bt.Chapters {
			chID, err := s.Store.CreateChapter(trID, bc.Title)
			if err != nil {
				return err
			}
			var pids []int64
			for _, idx := range bc.ProblemIDs {
				pids = append(pids, createdIDs[idx])
			}
			if len(pids) > 0 {
				if _, err := s.Store.AddChapterItems(chID, pids); err != nil {
					return err
				}
			}
		}
	}

	// 4) 练习
	for i, bp := range manifest.Practices {
		if prog != nil {
			prog(importPhasePractices, i+1, len(manifest.Practices))
		}
		folder, err := s.folderIDByPath(dirs, bp.Folder)
		if err != nil {
			return err
		}
		prID, err := s.Store.CreatePractice(bp.Title, bp.Description, bp.Tags, folder)
		if err != nil {
			return err
		}
		rb.practiceIDs = append(rb.practiceIDs, prID)
		var pids []int64
		for _, idx := range bp.ProblemIDs {
			pids = append(pids, createdIDs[idx])
		}
		if len(pids) > 0 {
			if _, err := s.Store.AddPracticeItems(prID, pids); err != nil {
				return err
			}
		}
	}
	commit = true
	return nil
}

// handleImportBackup POST /api/import/backup（multipart zip）→ 登记全库恢复任务。
// 接收 zip 后立即返回 201 {"taskId"}；解析/图片落盘/写库在后台 goroutine 顺序执行，
// 进度与结果经 GET /api/import/backup/task/:taskId 轮询（见 import_task.go）。
// 失败文本、严格预校验与补偿回滚语义与同步版本一致，仅错误上报改经任务表。
func (s *Server) handleImportBackup(c *fiber.Ctx) error {
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
	// 内存捕获（100MB 上限内可行）；data 由 goroutine 闭包持有，之后不再使用 src
	data, err := io.ReadAll(src)
	if err != nil {
		return err
	}

	task, err := s.createImportTask()
	if err != nil {
		return respondError(c, fiber.StatusServiceUnavailable, err.Error())
	}
	// fiber.Ctx 只能在 handler 内使用：域作用域先在主协程解析好，再交给后台任务
	scope := s.backupScope(c)
	go s.runImportBackupTask(task, data, scope)
	return respondData(c, fiber.StatusCreated, fiber.Map{"taskId": task.ID})
}

func extOf(name string) string {
	i := strings.LastIndexByte(name, '.')
	if i < 0 {
		return ""
	}
	return name[i:]
}
