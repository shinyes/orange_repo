// Package server 组装 Fiber 应用：路由、会话认证、静态资源。
package server

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"orangerepo/internal/store"
)

// SessionCookie 会话 Cookie 名。
const SessionCookie = "orange_session"

// Server 持有存储与上传目录。
type Server struct {
	Store      *store.Store
	UploadsDir string
	WebDist    string
}

// New 创建 Fiber 应用（含路由与中间件）。
func New(s *store.Store, uploadsDir, webDist string) *fiber.App {
	srv := &Server{Store: s, UploadsDir: uploadsDir, WebDist: webDist}
	app := fiber.New(fiber.Config{
		BodyLimit: 200 << 20,
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			if e, ok := err.(*fiber.Error); ok {
				return c.Status(e.Code).JSON(fiber.Map{"error": e.Message})
			}
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		},
	})
	app.Use(logger.New())
	app.Use(recover.New())

	app.Get("/api/health", func(c *fiber.Ctx) error { return c.JSON(fiber.Map{"ok": true}) })

	auth := app.Group("/api/auth")
	auth.Post("/login", srv.handleLogin)
	auth.Post("/logout", srv.handleLogout)
	auth.Get("/me", srv.handleMe)
	auth.Put("/password", srv.requireSession, srv.handleChangePassword)

	api := app.Group("/api", srv.requireSession)

	api.Get("/problems", srv.handleListProblems)
	api.Post("/problems", srv.handleCreateProblem)
	api.Get("/problems/:id", srv.handleGetProblem)
	api.Put("/problems/:id", srv.handleUpdateProblem)
	api.Delete("/problems/:id", srv.handleDeleteProblem)
	api.Put("/problems/:id/solutions", srv.handleUpdateSolutions)

	api.Get("/tags", srv.handleListTags)
	api.Patch("/tags", srv.handleRenameTag)
	api.Delete("/tags", srv.handleDeleteTag)
	api.Get("/tag-order", srv.handleGetTagOrder)
	api.Put("/tag-order", srv.handleSetTagOrder)

	api.Post("/images", srv.handleUploadImage)
	api.Static("/api/uploads", uploadsDir)

	api.Post("/import", srv.handleImport)
	api.Get("/export/problems", srv.handleExportProblems)
	api.Get("/export/trainings/:id", srv.handleExportTraining)
	api.Get("/export/practices/:id", srv.handleExportPractice)

	api.Get("/trainings", srv.handleListTrainings)
	api.Post("/trainings", srv.handleCreateTraining)
	api.Get("/trainings/:id", srv.handleGetTraining)
	api.Put("/trainings/:id", srv.handleUpdateTraining)
	api.Delete("/trainings/:id", srv.handleDeleteTraining)
	api.Post("/trainings/:id/chapters", srv.handleCreateChapter)
	api.Put("/chapters/:id", srv.handleUpdateChapter)
	api.Delete("/chapters/:id", srv.handleDeleteChapter)
	api.Post("/chapters/:id/items", srv.handleAddChapterItems)
	api.Put("/chapters/:id/items", srv.handleReorderChapterItems)
	api.Put("/trainings/:id/layout", srv.handleTrainingLayout)
	api.Delete("/items/:id", srv.handleDeleteItem)

	api.Get("/practices", srv.handleListPractices)
	api.Post("/practices", srv.handleCreatePractice)
	api.Get("/practices/:id", srv.handleGetPractice)
	api.Put("/practices/:id", srv.handleUpdatePractice)
	api.Delete("/practices/:id", srv.handleDeletePractice)
	api.Post("/practices/:id/items", srv.handleAddPracticeItems)
	api.Put("/practices/:id/items", srv.handleReorderPracticeItems)
	api.Put("/practice-items/:id", srv.handleUpdatePracticeItem)
	api.Delete("/practice-items/:id", srv.handleDeletePracticeItem)

	// 前端静态资源 + SPA 回退
	if webDist != "" {
		if _, err := os.Stat(filepath.Join(webDist, "index.html")); err == nil {
			app.Static("/", webDist)
			app.Get("*", func(c *fiber.Ctx) error {
				return c.SendFile(filepath.Join(webDist, "index.html"))
			})
		}
	}
	return app
}

// ---------- 响应辅助 ----------

func respondData(c *fiber.Ctx, status int, data fiber.Map) error {
	return c.Status(status).JSON(data)
}

func respondError(c *fiber.Ctx, status int, msg string) error {
	return c.Status(status).JSON(fiber.Map{"error": msg})
}

// paramID 解析路径参数中的正整数 id。
func paramID(c *fiber.Ctx, name string) (int64, error) {
	id, err := strconv.ParseInt(c.Params(name), 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid id")
	}
	return id, nil
}
