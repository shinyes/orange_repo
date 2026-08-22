package server

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"
)

var allowedImageExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".svg": true,
}

// handleUploadImage 接收 multipart(file)，存为 32 位随机十六进制文件名，返回 /api/uploads/ URL。
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

	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return err
	}
	name := hex.EncodeToString(buf) + ext
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
