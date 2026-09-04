// OrangeRepo 全库备份/迁移（backup）：
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

	"orangerepo/internal/model"
	"orangerepo/internal/store"
	"orangerepo/internal/zipio"
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
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Tags        []string        `json:"tags"`
	Folder      string          `json:"folder,omitempty"` // 目录名路径（/ 分隔），空=根
	Chapters    []backupChapter `json:"chapters"`
}

// backupPractice 练习条目。
type backupPractice struct {
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
		bt := backupTraining{Title: t.Title, Description: t.Description, Tags: t.Tags, Folder: pathOf(t.FolderID)}
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
		bp := backupPractice{Title: p.Title, Description: p.Description, Tags: p.Tags, Folder: pathOf(p.FolderID)}
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
	return sendZip(c, data, exportFilename("orangerepo_full_backup", ""))
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

// importBackup 全库恢复：题目全部新建 → 目录树 → 训练/练习（引用新题目 id）。
func (s *Server) importBackup(manifest *backupManifest, problems []zipio.ExportProblem) error {
	if manifest.Version != 1 {
		return errors.New("不支持的备份版本")
	}

	// 1) 题目全部新建（可能重复——按备份"全部新建副本"语义）
	createdIDs := make([]int64, 0, len(problems))
	for i := range problems {
		p := problems[i]
		zipio.ApplyImportRewrite(&p)
		payload := zipio.ProblemPayload{
			Type: p.Type, Title: p.Title, Tags: p.Tags, StatementMD: p.StatementMD,
			BodyJSON: p.BodyJSON, AnswerJSON: p.AnswerJSON, Solutions: p.Solutions,
			TimeLimitMS: p.TimeLimitMS, MemoryLimitMiB: p.MemoryLimitMiB,
		}
		if err := zipio.NormalizeProblemPayload(&payload); err != nil {
			return fmt.Errorf("题目 %q: %v", payload.Title, err)
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
			return err
		}
		createdIDs = append(createdIDs, id)
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
	for _, bt := range manifest.Trainings {
		folder, err := s.folderIDByPath(dirs, bt.Folder)
		if err != nil {
			return err
		}
		trID, err := s.Store.CreateTraining(bt.Title, bt.Description, bt.Tags, folder)
		if err != nil {
			return err
		}
		for _, bc := range bt.Chapters {
			chID, err := s.Store.CreateChapter(trID, bc.Title)
			if err != nil {
				return err
			}
			var pids []int64
			for _, idx := range bc.ProblemIDs {
				if idx >= 0 && idx < len(createdIDs) {
					pids = append(pids, createdIDs[idx])
				}
			}
			if len(pids) > 0 {
				if _, err := s.Store.AddChapterItems(chID, pids); err != nil {
					return err
				}
			}
		}
	}

	// 4) 练习
	for _, bp := range manifest.Practices {
		folder, err := s.folderIDByPath(dirs, bp.Folder)
		if err != nil {
			return err
		}
		prID, err := s.Store.CreatePractice(bp.Title, bp.Description, bp.Tags, folder)
		if err != nil {
			return err
		}
		var pids []int64
		for _, idx := range bp.ProblemIDs {
			if idx >= 0 && idx < len(createdIDs) {
				pids = append(pids, createdIDs[idx])
			}
		}
		if len(pids) > 0 {
			if _, err := s.Store.AddPracticeItems(prID, pids); err != nil {
				return err
			}
		}
	}
	return nil
}

// handleImportBackup POST /api/import/backup（multipart zip）→ 全库恢复。
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
	data, err := io.ReadAll(src)
	if err != nil {
		return err
	}

	problems, _, images, extra, err := zipio.ParseZipWithExtra(data)
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	raw, ok := extra[BackupJSONName]
	if !ok {
		return respondError(c, fiber.StatusBadRequest, "不是 OrangeRepo 全库备份包（缺少 "+BackupJSONName+"）")
	}
	manifest := &backupManifest{}
	if err := json.Unmarshal(raw, manifest); err != nil {
		return respondError(c, fiber.StatusBadRequest, "备份清单解析失败: "+err.Error())
	}

	// 落盘图片（nano 命名 + 引用重写，与 ImportZipData 一致）
	imageRename := map[string]string{}
	for name, content := range images {
		ext := extOf(name)
		newName, err := NanoName(16)
		if err != nil {
			return respondError(c, fiber.StatusInternalServerError, err.Error())
		}
		newName += ext
		if _, err := s.SaveUpload(newName, strings.NewReader(string(content))); err != nil {
			return respondError(c, fiber.StatusInternalServerError, err.Error())
		}
		imageRename[name] = newName
	}
	if len(imageRename) > 0 {
		for i := range problems {
			problems[i].StatementMD = rewriteUploadRefs(problems[i].StatementMD, imageRename)
			problems[i].BodyJSON = json.RawMessage(rewriteUploadRefs(string(problems[i].BodyJSON), imageRename))
			problems[i].AnswerJSON = json.RawMessage(rewriteUploadRefs(string(problems[i].AnswerJSON), imageRename))
			problems[i].Solutions = json.RawMessage(rewriteUploadRefs(string(problems[i].Solutions), imageRename))
		}
	}

	if err := s.importBackup(manifest, problems); err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	return respondData(c, fiber.StatusCreated, fiber.Map{
		"imported":  len(problems),
		"trainings": len(manifest.Trainings),
		"practices": len(manifest.Practices),
	})
}

func extOf(name string) string {
	i := strings.LastIndexByte(name, '.')
	if i < 0 {
		return ""
	}
	return name[i:]
}
