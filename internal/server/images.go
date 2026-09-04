package server

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gofiber/fiber/v2"
)

var allowedImageExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".svg": true,
}

// uploadRefPattern 匹配题面文本中的 /api/uploads/<file> 图片引用（与 zipio 一致）。
var uploadRefPattern = regexp.MustCompile(`/api/uploads/([a-zA-Z0-9_-]+\.(?:png|jpe?g|gif|webp|svg))`)

// handleUploadImage 接收 multipart(file)，存为 16 位 URL-safe 随机文件名（nano 命名避免重名），返回 /api/uploads/ URL。
func (s *Server) handleUploadImage(c *fiber.Ctx) error {
	file, err := c.FormFile("file")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "missing file")
	}
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if !allowedImageExts[ext] {
		return respondError(c, fiber.StatusBadRequest, "unsupported image type: "+ext)
	}
	src, err := file.Open()
	if err != nil {
		return err
	}
	defer src.Close()

	name, err := NanoName(16)
	if err != nil {
		return err
	}
	name += ext
	dst, err := s.SaveUpload(name, src)
	if err != nil {
		return err
	}
	return respondData(c, fiber.StatusCreated, fiber.Map{"url": "/api/uploads/" + name, "path": dst})
}

// SaveUpload 将内容写入上传目录（导入 ZIP 图片也走这里）。
func (s *Server) SaveUpload(name string, src io.Reader) (string, error) {
	return saveUploadTo(s.UploadsDir, name, src)
}

// referencedUploads 收集全库题目四文本字段中被引用的图片文件名。
func (s *Server) referencedUploads() (map[string]bool, error) {
	rows, err := s.Store.DB.Query(`SELECT statement_md, body_json, answer_json, solutions_json FROM problems`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	refs := map[string]bool{}
	for rows.Next() {
		var stmt, body, answer, solutions string
		if err := rows.Scan(&stmt, &body, &answer, &solutions); err != nil {
			return nil, err
		}
		for _, field := range []string{stmt, body, answer, solutions} {
			for _, m := range uploadRefPattern.FindAllStringSubmatch(field, -1) {
				refs[m[1]] = true
			}
		}
	}
	return refs, rows.Err()
}

// orphanUploads 列出上传目录中未被任何题目引用的图片文件（按规则命名才计入）。
func (s *Server) orphanUploads(refs map[string]bool) ([]string, error) {
	entries, err := os.ReadDir(s.UploadsDir)
	if err != nil {
		return nil, err
	}
	var orphans []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !uploadRefPattern.MatchString("/api/uploads/" + name) {
			continue // 只处理规则命名的图片文件
		}
		if !refs[name] {
			orphans = append(orphans, name)
		}
	}
	return orphans, nil
}

// handleCleanupImages GET /api/uploads/cleanup?dryRun=true → {orphaned,total}（仅统计）；
// POST /api/uploads/cleanup → {removed}（删除未关联图片）。
func (s *Server) handleCleanupImages(c *fiber.Ctx) error {
	refs, err := s.referencedUploads()
	if err != nil {
		return err
	}
	orphans, err := s.orphanUploads(refs)
	if err != nil {
		return err
	}
	if c.Method() == fiber.MethodGet && c.Query("dryRun") == "true" {
		entries, err := os.ReadDir(s.UploadsDir)
		if err != nil {
			return err
		}
		total := 0
		for _, e := range entries {
			if !e.IsDir() && uploadRefPattern.MatchString("/api/uploads/"+e.Name()) {
				total++
			}
		}
		return respondData(c, fiber.StatusOK, fiber.Map{"orphaned": len(orphans), "total": total})
	}
	removed := 0
	for _, name := range orphans {
		if err := os.Remove(filepath.Join(s.UploadsDir, name)); err != nil {
			continue
		}
		removed++
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"removed": removed})
}
