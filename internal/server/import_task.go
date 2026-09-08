// 全量导入异步任务（import/backup 异步化）：
//
//   - POST /api/import/backup 只做 multipart 接收并登记后台任务，立即 201 {"taskId"}，
//     解析 / 图片落盘重写 / 写库全部在后台 goroutine 顺序执行；
//   - GET /api/import/backup/task/:taskId 轮询进度：
//     {done, ok, phase, message, current, total, error?, result?}；
//   - 单进程内存任务表（Server 私有字段）：上限 importTaskCapacity 个，
//     满时淘汰最旧已完成任务；无已完成可淘汰则新任务拒绝（503 忙碌）。
package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"orangeoj/internal/zipio"
)

// 任务阶段（phase）约定：前端按字符串展示/换算进度。
// importBackup 内部回调的阶段：题目/训练/练习（各阶段内 current/total 独立计数）。
const (
	importPhaseQueued    = "排队中"
	importPhaseParsing   = "解析中"
	importPhaseProblems  = "题目"
	importPhaseTrainings = "训练"
	importPhasePractices = "练习"
	importPhaseDone      = "完成"
)

// importTaskCapacity 任务表上限：保留最近的任务供轮询。
const importTaskCapacity = 100

// ImportTask 一次全量导入后台任务的进度快照。
// 写入一律经 Server.updateImportTask（持锁）；读取用 Server.importTaskSnapshot。
// Result 仅在任务完成（Done=true）后设置，此后不再变更。
type ImportTask struct {
	ID        string
	Done      bool
	Ok        bool
	Error     string
	Phase     string
	Message   string
	Current   int
	Total     int
	StartedAt time.Time
	Result    map[string]any // 成功时 {imported,trainings,practices}
}

// importProgressMessage 由阶段与计数值生成用户可读的进行中消息。
func importProgressMessage(phase string, cur, total int) string {
	switch phase {
	case importPhaseProblems:
		return fmt.Sprintf("导入题目 %d/%d", cur, total)
	case importPhaseTrainings:
		return fmt.Sprintf("创建训练 %d/%d", cur, total)
	case importPhasePractices:
		return fmt.Sprintf("创建练习 %d/%d", cur, total)
	}
	return phase
}

// createImportTask 登记新任务（id = NanoName(16) 随机串）。
// 容量满时先淘汰最旧已完成任务；若不存在已完成任务（说明并发任务数已达上限）则报错，
// 由调用方按 503 忙碌处理。
func (s *Server) createImportTask() (*ImportTask, error) {
	s.importTaskMu.Lock()
	defer s.importTaskMu.Unlock()
	if s.importTasks == nil {
		s.importTasks = map[string]*ImportTask{}
	}
	if len(s.importTasks) >= importTaskCapacity {
		var oldest *ImportTask
		for _, t := range s.importTasks {
			if t.Done && (oldest == nil || t.StartedAt.Before(oldest.StartedAt)) {
				oldest = t
			}
		}
		if oldest == nil {
			return nil, errors.New("导入任务队列已满，请稍后重试")
		}
		delete(s.importTasks, oldest.ID)
	}
	id, err := NanoName(16)
	if err != nil {
		return nil, err
	}
	t := &ImportTask{ID: id, Phase: importPhaseQueued, StartedAt: time.Now()}
	s.importTasks[id] = t
	return t, nil
}

// importTaskSnapshot 返回任务字段的浅拷贝；不存在返回 false。
func (s *Server) importTaskSnapshot(id string) (*ImportTask, bool) {
	s.importTaskMu.Lock()
	defer s.importTaskMu.Unlock()
	t, ok := s.importTasks[id]
	if !ok {
		return nil, false
	}
	c := *t
	return &c, true
}

// updateImportTask 持锁更新任务字段（后台 goroutine 写进度统一走这里）。
func (s *Server) updateImportTask(id string, fn func(*ImportTask)) {
	s.importTaskMu.Lock()
	defer s.importTaskMu.Unlock()
	if t, ok := s.importTasks[id]; ok {
		fn(t)
	}
}

// runImportBackupTask 后台执行一次全量导入：解析备份包 → 严格校验/图片落盘重写 →
// importBackup（分阶段进度回调）。错误不写响应，而是落到 task.Error：
// 库侧失败沿用「导入失败，已全部回滚: …」文本，并清理本次已落盘的图片。
// 注意：goroutine 内绝不使用 fiber.Ctx（原 handler 的上下文只留在 handler 中），
// 只操作 task 表 + store + 上传目录。
func (s *Server) runImportBackupTask(task *ImportTask, data []byte, domainID *int64) {
	update := func(fn func(*ImportTask)) {
		s.updateImportTask(task.ID, fn)
	}
	fail := func(msg string) {
		update(func(t *ImportTask) { t.Done, t.Ok, t.Error = true, false, msg })
	}

	// ---- 解析阶段（不在 importBackup 内） ----
	update(func(t *ImportTask) {
		t.Phase, t.Message = importPhaseParsing, "正在解析备份包…"
		t.Current, t.Total = 0, 0
	})
	problems, _, images, extra, err := zipio.ParseZipWithExtra(data)
	if err != nil {
		fail(err.Error())
		return
	}
	raw, ok := extra[BackupJSONName]
	if !ok {
		fail("不是 OrangeOJ 全库备份包（缺少 " + BackupJSONName + "）")
		return
	}
	manifest := &backupManifest{}
	if err := json.Unmarshal(raw, manifest); err != nil {
		fail("备份清单解析失败: " + err.Error())
		return
	}

	// 严格校验：题目文本引用的图片必须存在于包内（缺失=不完整备份，拒绝导入）
	available := map[string]bool{}
	for name := range images {
		available[name] = true
	}
	for i := range problems {
		refs := zipio.CollectImageRefs(
			problems[i].StatementMD, string(problems[i].BodyJSON),
			string(problems[i].AnswerJSON), string(problems[i].Solutions),
		)
		for _, r := range refs {
			if !available[r] {
				fail(fmt.Sprintf("题目 %d（%q）引用图片 %q 但备份包内缺失——备份不完整，已拒绝导入", i+1, problems[i].Title, r))
				return
			}
		}
	}

	// 落盘图片（nano 命名 + 引用重写，与同步导入一致）
	imageRename := map[string]string{}
	for name, content := range images {
		ext := extOf(name)
		newName, err := NanoName(16)
		if err != nil {
			fail(err.Error())
			return
		}
		newName += ext
		if _, err := s.SaveUpload(newName, strings.NewReader(string(content))); err != nil {
			fail(err.Error())
			return
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

	// ---- 分阶段进度回调 → 写任务表 ----
	prog := func(phase string, cur, total int) {
		update(func(t *ImportTask) {
			t.Phase, t.Message = phase, importProgressMessage(phase, cur, total)
			t.Current, t.Total = cur, total
		})
	}
	if err := s.importBackup(manifest, problems, domainID, prog); err != nil {
		// 失败：清理本次已落盘的图片（库侧资源已由 importBackup 补偿删除）
		for _, newName := range imageRename {
			_ = os.Remove(filepath.Join(s.UploadsDir, newName))
		}
		fail("导入失败，已全部回滚: " + err.Error())
		return
	}
	update(func(t *ImportTask) {
		t.Done, t.Ok = true, true
		t.Phase, t.Message = importPhaseDone, "导入完成"
		t.Result = map[string]any{
			"imported":  len(problems),
			"trainings": len(manifest.Trainings),
			"practices": len(manifest.Practices),
		}
	})
}

// handleImportTask GET /api/import/backup/task/:taskId → 轮询导入任务进度。
// 成功: {done:true, ok:true, phase, message, current, total, result:{imported,trainings,practices}}
// 失败: {done:true, ok:false, error, phase, message, current, total}
// 进行中: {done:false, ok:false, phase, message, current, total}
// 任务不存在/已过期（被容量淘汰）→ 404。
func (s *Server) handleImportTask(c *fiber.Ctx) error {
	t, ok := s.importTaskSnapshot(c.Params("taskId"))
	if !ok {
		return respondError(c, fiber.StatusNotFound, "任务不存在或已过期")
	}
	out := fiber.Map{
		"done":    t.Done,
		"ok":      t.Ok,
		"phase":   t.Phase,
		"message": t.Message,
		"current": t.Current,
		"total":   t.Total,
	}
	if t.Error != "" {
		out["error"] = t.Error
	}
	if t.Result != nil {
		out["result"] = t.Result
	}
	return respondData(c, fiber.StatusOK, out)
}
