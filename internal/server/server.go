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

	"orangeoj/internal/accounts"
	"orangeoj/internal/store"
)

// SessionCookie 会话 Cookie 名。
const SessionCookie = "orange_session"

// Server 持有存储、共享账号库与上传目录。
type Server struct {
	Store      *store.Store
	Accounts   *accounts.Store
	UploadsDir string
	WebDist    string
}

// New 创建 Fiber 应用（含路由与中间件）。
func New(s *store.Store, acc *accounts.Store, uploadsDir, webDist string) *fiber.App {
	srv := &Server{Store: s, Accounts: acc, UploadsDir: uploadsDir, WebDist: webDist}
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
	api.Get("/uploads/cleanup", srv.handleCleanupImages)
	api.Post("/uploads/cleanup", srv.handleCleanupImages)
	// 注意：挂载在 /api 组内，前缀只需 /uploads（组前缀合成 /api/uploads）
	api.Static("/uploads", uploadsDir)

	api.Get("/booklet-directories", srv.handleListBookletDirectories)
	api.Post("/booklet-directories", srv.handleCreateBookletDirectory)
	api.Put("/booklet-directories/layout", srv.handleSetBookletDirectoryLayout)
	api.Patch("/booklet-directories/:id", srv.handleRenameBookletDirectory)
	api.Delete("/booklet-directories/:id", srv.handleDeleteBookletDirectory)

	api.Post("/import", srv.handleImport)
	api.Get("/export/problems", srv.handleExportProblems)
	api.Get("/export/trainings/:id", srv.handleExportTraining)
	api.Get("/export/practices/:id", srv.handleExportPractice)
	// 全库备份/迁移：导出单包 / 导入恢复（见 backup.go）
	api.Get("/export/backup", srv.handleExportBackup)
	api.Post("/import/backup", srv.handleImportBackup)

	// 域管理（系统管理员）：域 CRUD + 域管理员
	domainAdmin := api.Group("/admin/domains", srv.requireGlobalAdmin)
	domainAdmin.Get("/", srv.handleListDomains)
	domainAdmin.Post("/", srv.handleCreateDomain)
	domainAdmin.Patch("/:id", srv.handleRenameDomain)
	domainAdmin.Delete("/:id", srv.handleDeleteDomain)
	domainAdmin.Put("/:id/admin", srv.handleSetDomainAdmin)
	domainAdmin.Get("/:id/admins", srv.handleListDomainAdmins)
	domainAdmin.Delete("/:id/admins/:uid", srv.handleRemoveDomainAdmin)

	// 空间管理（系统/域管理员；/api 组已限管理员，handler 内再按域校验）
	spaceAdmin := api.Group("/admin/spaces")
	spaceAdmin.Get("/", srv.handleListSpaces)
	spaceAdmin.Post("/", srv.handleCreateSpace)
	spaceAdmin.Patch("/:id", srv.handleRenameSpace)
	spaceAdmin.Delete("/:id", srv.handleDeleteSpace)
	spaceAdmin.Get("/:id/members", srv.handleListSpaceMembers)
	spaceAdmin.Put("/:id/members", srv.handleSetSpaceMembers)

	// 空间成员账号管理（系统/域管理员）：member 账号 CRUD
	userAdmin := api.Group("/admin/users")
	userAdmin.Post("/", srv.handleCreateUser)
	userAdmin.Get("/", srv.handleListUsers)
	userAdmin.Delete("/:id", srv.handleDeleteUser)
	userAdmin.Put("/:id/password", srv.handleResetUserPassword)

	// 空间内容管理（系统/域管理员；成员不可达主站）：空间训练/练习/刷题 结构 CRUD
	sp := api.Group("/space")
	sp.Get("/:id/trainings", srv.handleListSpaceTrainings)
	sp.Post("/:id/trainings", srv.handleCreateSpaceTraining)
	sp.Get("/:id/trainings/:tid", srv.handleGetSpaceTraining)
	sp.Put("/:id/trainings/:tid", srv.handleUpdateSpaceTrainingMeta)
	sp.Delete("/:id/trainings/:tid", srv.handleDeleteSpaceTraining)
	sp.Post("/:id/trainings/:tid/chapters", srv.handleCreateSpaceChapter)
	sp.Post("/:id/chapters/:cid/items", srv.handleAddSpaceChapterItems)
	sp.Get("/:id/practices", srv.handleListSpacePractices)
	sp.Post("/:id/practices", srv.handleCreateSpacePractice)
	sp.Get("/:id/practices/:pid", srv.handleGetSpacePractice)
	sp.Put("/:id/practices/:pid", srv.handleUpdateSpacePractice)
	sp.Delete("/:id/practices/:pid", srv.handleDeleteSpacePractice)
	sp.Post("/:id/practices/:pid/items", srv.handleAddSpacePracticeItems)
	sp.Delete("/space-items/:itemId", srv.handleDeleteSpaceItem)
	sp.Get("/:id/quizzes", srv.handleListSpaceQuizzes)
	sp.Post("/:id/quizzes", srv.handleCreateSpaceQuiz)
	sp.Delete("/:id/quizzes/:qid", srv.handleDeleteSpaceQuiz)

	api.Get("/trainings", srv.handleListTrainings)
	api.Post("/trainings", srv.handleCreateTraining)
	api.Get("/trainings/:id", srv.handleGetTraining)
	api.Put("/trainings/:id", srv.handleUpdateTraining)
	api.Delete("/trainings/:id", srv.handleDeleteTraining)
	api.Post("/trainings/:id/chapters", srv.handleCreateChapter)
	api.Put("/trainings/:id/folder", srv.handleSetTrainingFolder)
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
	api.Put("/practices/:id/folder", srv.handleSetPracticeFolder)
	api.Put("/practices/:id/items", srv.handleReorderPracticeItems)
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

// respondError 返回 *fiber.Error（不写响应）：由全局 ErrorHandler 统一输出 JSON。
// 返回非 nil 保证中间件/调用链正确中断（nil 会让 Fiber 继续执行后续 handler）。
func respondError(c *fiber.Ctx, status int, msg string) error {
	return fiber.NewError(status, msg)
}

// paramID 解析路径参数中的正整数 id。
func paramID(c *fiber.Ctx, name string) (int64, error) {
	id, err := strconv.ParseInt(c.Params(name), 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid id")
	}
	return id, nil
}
