// 书包（Scratch 工程库）HTTP 层：文件夹与工程的增删改查 + .sb3 落盘读写。
// 权限：登录用户只能操作自己的书包（数据层所有查询都带 user_id 条件）。
// 文件布局：<ScratchDir>/<user_id>/<uuid>.sb3；ScratchDir 为空时回退到 <UploadsDir>/../scratch。
package quizserver

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"orangeoj/internal/quizstore"
)

// scratchDir 书包文件根目录（惰性创建）。
func (s *Server) scratchDir() string {
	base := s.ScratchDir
	if strings.TrimSpace(base) == "" {
		if strings.TrimSpace(s.UploadsDir) != "" {
			base = filepath.Join(filepath.Dir(s.UploadsDir), "scratch")
		} else {
			base = filepath.Join("data", "scratch")
		}
	}
	return base
}

// scratchFilePath 某工程的磁盘路径。
func (s *Server) scratchFilePath(userID int64, projectUUID string) string {
	return filepath.Join(s.scratchDir(), strconv.FormatInt(userID, 10), projectUUID+".sb3")
}

// ---------- 文件夹 ----------

// handleScratchFoldersList GET /api/portal/scratch/folders
func (s *Server) handleScratchFoldersList(c *fiber.Ctx) error {
	user := currentUser(c)
	folders, err := s.QS.ListScratchFolders(user.ID)
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	used, err := s.QS.ScratchUsage(user.ID)
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	return respondData(c, fiber.StatusOK, fiber.Map{
		"folders": folders,
		"usage": fiber.Map{
			"usedBytes":       used,
			"quotaBytes":      quizstore.ScratchUserQuotaBytes,
			"maxProjectBytes": quizstore.MaxScratchProjectBytes,
			"maxFolders":      quizstore.MaxScratchFolders,
		},
	})
}

// handleScratchFolderCreate POST /api/portal/scratch/folders {name, parentId?}
func (s *Server) handleScratchFolderCreate(c *fiber.Ctx) error {
	user := currentUser(c)
	var req struct {
		Name     string `json:"name"`
		ParentID *int64 `json:"parentId"`
	}
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "参数不合法")
	}
	id, err := s.QS.CreateScratchFolder(user.ID, req.Name, req.ParentID)
	if err != nil {
		return scratchErr(c, err)
	}
	return respondData(c, fiber.StatusCreated, fiber.Map{"id": id})
}

// handleScratchFolderUpdate PATCH /api/portal/scratch/folders/:id {name?, parentId?}
func (s *Server) handleScratchFolderUpdate(c *fiber.Ctx) error {
	user := currentUser(c)
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid folder id")
	}
	var req struct {
		Name     *string `json:"name"`
		ParentID *int64  `json:"parentId"`
	}
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "参数不合法")
	}
	// parentId 传 0 表示移到根目录（数据层约定）
	if err := s.QS.UpdateScratchFolder(user.ID, id, req.Name, req.ParentID); err != nil {
		return scratchErr(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// handleScratchFolderDelete DELETE /api/portal/scratch/folders/:id
// 文件夹内的工程回到根目录（不删作品）。
func (s *Server) handleScratchFolderDelete(c *fiber.Ctx) error {
	user := currentUser(c)
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid folder id")
	}
	if err := s.QS.DeleteScratchFolder(user.ID, id); err != nil {
		return scratchErr(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// ---------- 工程 ----------

// handleScratchProjectsList GET /api/portal/scratch/projects?folderId=（省略=全部；0=根目录）
func (s *Server) handleScratchProjectsList(c *fiber.Ctx) error {
	user := currentUser(c)
	var folderID *int64
	if raw := strings.TrimSpace(c.Query("folderId")); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n < 0 {
			return respondError(c, fiber.StatusBadRequest, "invalid folderId")
		}
		folderID = &n
	}
	list, err := s.QS.ListScratchProjects(user.ID, folderID)
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"projects": list})
}

// handleScratchProjectUpload POST /api/portal/scratch/projects?name=&folderId=
// 请求体为 .sb3 原始字节（application/octet-stream）。
func (s *Server) handleScratchProjectUpload(c *fiber.Ctx) error {
	user := currentUser(c)
	name := strings.TrimSpace(c.Query("name"))
	var folderID *int64
	if raw := strings.TrimSpace(c.Query("folderId")); raw != "" {
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil && n > 0 {
			folderID = &n
		}
	}
	body := c.Body()
	if len(body) == 0 {
		return respondError(c, fiber.StatusBadRequest, "工程内容为空")
	}
	if len(body) > quizstore.MaxScratchProjectBytes {
		return respondError(c, fiber.StatusBadRequest,
			fmt.Sprintf("工程过大（单个上限 %d MB）", quizstore.MaxScratchProjectBytes>>20))
	}
	// .sb3 是 zip：校验魔数，避免把任意文件塞进书包
	if len(body) < 4 || body[0] != 'P' || body[1] != 'K' {
		return respondError(c, fiber.StatusBadRequest, "不是有效的 Scratch 工程文件（.sb3）")
	}
	// 进一步校验 zip 结构完整且含 project.json：截断/损坏的文件若被存下，
	// 之后"从书包打开"会在编辑器里报奇怪的解析错误（如 Non-ascii character in FixedAsciiString）。
	if err := validateSb3(body); err != nil {
		return respondError(c, fiber.StatusBadRequest, "工程文件不完整或已损坏："+err.Error())
	}
	// 配额预检（文件写完再校验会留下垃圾文件）
	used, err := s.QS.ScratchUsage(user.ID)
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	if used+int64(len(body)) > quizstore.ScratchUserQuotaBytes {
		return respondError(c, fiber.StatusBadRequest,
			fmt.Sprintf("书包空间不足（上限 %d MB）", quizstore.ScratchUserQuotaBytes>>20))
	}

	id := uuid.NewString()
	dir := filepath.Join(s.scratchDir(), strconv.FormatInt(user.ID, 10))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return respondError(c, fiber.StatusInternalServerError, "无法创建书包目录")
	}
	path := s.scratchFilePath(user.ID, id)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return respondError(c, fiber.StatusInternalServerError, "写入工程文件失败")
	}
	sum := sha256.Sum256(body)
	projectID, err := s.QS.CreateScratchProject(user.ID, id, name, folderID, int64(len(body)), hex.EncodeToString(sum[:]))
	if err != nil {
		_ = os.Remove(path) // 元数据失败则回收文件
		return scratchErr(c, err)
	}
	return respondData(c, fiber.StatusCreated, fiber.Map{"id": projectID, "uuid": id, "size": len(body)})
}

// handleScratchProjectFile GET /api/portal/scratch/projects/:id/file → .sb3 下载
func (s *Server) handleScratchProjectFile(c *fiber.Ctx) error {
	user := currentUser(c)
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid project id")
	}
	p, err := s.QS.GetScratchProject(user.ID, id)
	if err != nil {
		return scratchErr(c, err)
	}
	path := s.scratchFilePath(user.ID, p.UUID)
	f, err := os.Open(path)
	if err != nil {
		return respondError(c, fiber.StatusNotFound, "工程文件不存在")
	}
	defer f.Close()
	c.Set(fiber.HeaderContentType, "application/octet-stream")
	c.Set(fiber.HeaderContentDisposition,
		fmt.Sprintf(`attachment; filename="%s.sb3"`, sanitizeFilename(p.Name)))
	return c.SendStream(f, scratchSendSize(p.UUID, p.Size, f))
}

// handleScratchProjectContentPut PUT /api/portal/scratch/projects/:id/content
// 覆盖工程内容（实时暂存）：与原文件同路径写入，不新增作品记录。
func (s *Server) handleScratchProjectContentPut(c *fiber.Ctx) error {
	user := currentUser(c)
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid project id")
	}
	p, err := s.QS.GetScratchProject(user.ID, id)
	if err != nil {
		return scratchErr(c, err)
	}
	body := c.Body()
	if len(body) == 0 {
		return respondError(c, fiber.StatusBadRequest, "工程内容为空")
	}
	if len(body) > quizstore.MaxScratchProjectBytes {
		return respondError(c, fiber.StatusBadRequest,
			fmt.Sprintf("工程过大（单个上限 %d MB）", quizstore.MaxScratchProjectBytes>>20))
	}
	if len(body) < 4 || body[0] != 'P' || body[1] != 'K' {
		return respondError(c, fiber.StatusBadRequest, "不是有效的 Scratch 工程文件（.sb3）")
	}
	if err := validateSb3(body); err != nil {
		return respondError(c, fiber.StatusBadRequest, "工程文件不完整或已损坏："+err.Error())
	}
	sum := sha256.Sum256(body)
	if err := s.QS.UpdateScratchProjectContent(user.ID, id, int64(len(body)), hex.EncodeToString(sum[:])); err != nil {
		return scratchErr(c, err)
	}
	// 内容先落新文件再原子替换：避免写一半造成文件损坏（那会导致"打开作品失败"）
	path := s.scratchFilePath(user.ID, p.UUID)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return respondError(c, fiber.StatusInternalServerError, "写入工程文件失败")
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return respondError(c, fiber.StatusInternalServerError, "替换工程文件失败")
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"id": id, "size": len(body)})
}

// handleScratchProjectUpdate PATCH /api/portal/scratch/projects/:id {name?, folderId?}
func (s *Server) handleScratchProjectUpdate(c *fiber.Ctx) error {
	user := currentUser(c)
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid project id")
	}
	var req struct {
		Name     *string `json:"name"`
		FolderID *int64  `json:"folderId"`
	}
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "参数不合法")
	}
	if err := s.QS.UpdateScratchProject(user.ID, id, req.Name, req.FolderID); err != nil {
		return scratchErr(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// handleScratchProjectDelete DELETE /api/portal/scratch/projects/:id
func (s *Server) handleScratchProjectDelete(c *fiber.Ctx) error {
	user := currentUser(c)
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid project id")
	}
	projectUUID, err := s.QS.DeleteScratchProject(user.ID, id)
	if err != nil {
		return scratchErr(c, err)
	}
	_ = os.Remove(s.scratchFilePath(user.ID, projectUUID))
	return c.SendStatus(fiber.StatusNoContent)
}

// handleScratchProjectRaw GET /api/portal/scratch/projects/:id/raw → 原始字节（供编辑器 iframe 同源取用）
func (s *Server) handleScratchProjectRaw(c *fiber.Ctx) error {
	user := currentUser(c)
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid project id")
	}
	p, err := s.QS.GetScratchProject(user.ID, id)
	if err != nil {
		return scratchErr(c, err)
	}
	f, err := os.Open(s.scratchFilePath(user.ID, p.UUID))
	if err != nil {
		return respondError(c, fiber.StatusNotFound, "工程文件不存在")
	}
	defer f.Close()
	// 编辑器靠这段字节直接解析 zip：长度必须与真实文件一致，否则会被截断，
	// Scratch VM 会抛 "Non-ascii character in FixedAsciiString" 这类解析错误。
	c.Set(fiber.HeaderContentType, "application/x-scratch.sb3")
	return c.SendStream(f, scratchSendSize(p.UUID, p.Size, f))
}

// scratchSendSize 返回应当发送的字节数：以**文件实际大小**为准。
// 历史数据/异常写入可能让库中记录的 size 与磁盘文件不一致；此时若仍按记录值设置
// Content-Length，浏览器会报 ERR_CONTENT_LENGTH_MISMATCH 并截断数据，
// 表现为"打开书包里的作品失败"（VM 解析 zip 报 Non-ascii character in FixedAsciiString）。
func scratchSendSize(uuid string, recorded int64, f *os.File) int {
	if info, err := f.Stat(); err == nil {
		if info.Size() != recorded {
			log.Printf("[scratch] 工程 %s 记录大小 %d 与文件实际 %d 不一致，按实际大小发送",
				uuid, recorded, info.Size())
		}
		return int(info.Size())
	}
	return int(recorded)
}

// scratchErr 数据层错误 → HTTP：不存在/非本人 → 404；其余按 400（容量/命名等业务错误）。
func scratchErr(c *fiber.Ctx, err error) error {
	if errors.Is(err, quizstore.ErrNotFound) {
		return respondError(c, fiber.StatusNotFound, "不存在或无权访问")
	}
	return respondError(c, fiber.StatusBadRequest, err.Error())
}

// sanitizeFilename 下载文件名净化（去路径分隔与控制字符）。
func sanitizeFilename(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "project"
	}
	repl := strings.NewReplacer("/", "_", "\\", "_", "\"", "_", "\r", "", "\n", "", "\x00", "")
	name = repl.Replace(name)
	if len([]rune(name)) > 80 {
		name = string([]rune(name)[:80])
	}
	return name
}

// validateSb3 校验 .sb3（zip）结构完整且包含 project.json。
// 只做"能否安全打开"的检查：条目可读、project.json 存在且非空。
func validateSb3(body []byte) error {
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return errors.New("zip 结构无法解析")
	}
	for _, f := range zr.File {
		if f.Name != "project.json" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return errors.New("project.json 无法读取")
		}
		defer rc.Close()
		n, err := io.Copy(io.Discard, io.LimitReader(rc, 1<<20))
		if err != nil || n == 0 {
			return errors.New("project.json 为空或损坏")
		}
		return nil
	}
	return errors.New("缺少 project.json")
}
