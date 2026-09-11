// Package server 组装 Fiber 应用：路由、会话认证、静态资源。
package server

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"orangeoj/internal/accounts"
	"orangeoj/internal/quizstore"
	"orangeoj/internal/store"
)

// SessionCookie 会话 Cookie 名（合服后管理端/门户共用同一会话）。
const SessionCookie = "orange_session"

// Server 持有存储、共享账号库与上传目录。
type Server struct {
	Store      *store.Store
	Accounts   *accounts.Store
	UploadsDir string
	WebDist    string
	// 刷题侧数据（submissions/草稿/错题集等）清理（组合层注入；nil=跳过清理）
	QuizStore *quizstore.Store

	// 全量导入异步任务表（单进程内存态，惰性初始化，见 import_task.go）
	importTaskMu sync.Mutex
	importTasks  map[string]*ImportTask
}

// New 创建管理端 Fiber 应用（含路由与中间件）——保留签名供测试与旧单进程模式。
// 合服组装请使用 RegisterAuth + RegisterRoutes 挂到既有 app 上。
func New(s *store.Store, acc *accounts.Store, uploadsDir, webDist string) *fiber.App {
	srv := &Server{Store: s, Accounts: acc, UploadsDir: uploadsDir, WebDist: webDist}
	app := NewApp()
	app.Use(logger.New())
	app.Use(recover.New())
	app.Get("/api/health", func(c *fiber.Ctx) error { return c.JSON(fiber.Map{"ok": true}) })
	srv.RegisterAuth(app)
	RegisterRoutes(s, acc, uploadsDir, app)

	// 前端静态资源 + SPA 回退
	if webDist != "" {
		mountSPA(app, webDist)
	}
	return app
}

// NewApp 创建带统一配置（BodyLimit/ErrorHandler）的空 Fiber 应用。
// 合服时由组装层创建单一 app 后分别挂载 RegisterAuth / RegisterRoutes。
func NewApp() *fiber.App {
	return fiber.New(fiber.Config{
		BodyLimit: 200 << 20,
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			if e, ok := err.(*fiber.Error); ok {
				return c.Status(e.Code).JSON(fiber.Map{"error": e.Message})
			}
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		},
	})
}

// RegisterAuth 在 app 上挂载一份统一认证端点（login 放行任意角色；
// logout/me 无门槛，password 需有效会话）。合服后全端共用，仅挂一次。
func (s *Server) RegisterAuth(app *fiber.App) {
	auth := app.Group("/api/auth")
	auth.Post("/login", s.handleLogin)
	auth.Post("/logout", s.handleLogout)
	auth.Get("/me", s.handleMe)
	auth.Put("/password", s.requireAny, s.handleChangePassword)
}

// RegisterRoutes 将仓库管理 API 挂载到既有 app（鉴权：requireAdmin 组级，
// 域管理子组 requireGlobalAdmin）。静态 /api/uploads 匿名（题面图片对门户也需可见）。
func RegisterRoutes(s *store.Store, acc *accounts.Store, uploadsDir string, app *fiber.App) {
	srv := &Server{Store: s, Accounts: acc, UploadsDir: uploadsDir}
	srv.registerManagement(app)
	// 题面/题解图片：匿名可读（门户做题页也要显示）。仅当目录存在时挂载。
	if uploadsDir != "" {
		if _, err := os.Stat(uploadsDir); err == nil {
			app.Group("/api").Static("/uploads", uploadsDir)
		}
	}
}

// registerManagement 挂载管理 API 的全部子路由（problems/tags/…/space）。
//
// 注意：不采用 app.Group("/api", requireAdmin) 的「前缀级中间件」——Fiber 的组中间件
// 注册为全局前缀 USE 路由，会同时拦截拼到同一 app 上的 /api/portal、/api/oj 与匿名
// /api/uploads 静态。故此处按「每条叶子路由挂 requireAdmin（组级语义）」实现：
// 仅精确覆盖管理端点，门户/刷题/图片静态不受前缀遮蔽。
func (s *Server) registerManagement(app *fiber.App) {
	api := app.Group("/api") // 纯前缀，不挂中间件

	// adminGet/… 给管理叶子路由逐个挂 requireAdmin（可再叠加 requireGlobalAdmin）。
	ga := func(method, path string, h fiber.Handler) {
		api.Add(method, path, s.requireAdmin, h)
	}

	ga("GET", "/problems", s.handleListProblems)
	ga("POST", "/problems", s.handleCreateProblem)
	ga("GET", "/problems/:id", s.handleGetProblem)
	ga("PUT", "/problems/:id", s.handleUpdateProblem)
	ga("DELETE", "/problems/:id", s.handleDeleteProblem)
	ga("PUT", "/problems/:id/solutions", s.handleUpdateSolutions)

	ga("GET", "/tags", s.handleListTags)
	ga("PATCH", "/tags", s.handleRenameTag)
	ga("DELETE", "/tags", s.handleDeleteTag)
	ga("GET", "/tag-order", s.handleGetTagOrder)
	ga("PUT", "/tag-order", s.handleSetTagOrder)

	ga("POST", "/images", s.handleUploadImage)
	ga("GET", "/uploads/cleanup", s.handleCleanupImages)
	ga("POST", "/uploads/cleanup", s.handleCleanupImages)

	ga("GET", "/booklet-directories", s.handleListBookletDirectories)
	ga("POST", "/booklet-directories", s.handleCreateBookletDirectory)
	ga("PUT", "/booklet-directories/layout", s.handleSetBookletDirectoryLayout)
	ga("PATCH", "/booklet-directories/:id", s.handleRenameBookletDirectory)
	ga("DELETE", "/booklet-directories/:id", s.handleDeleteBookletDirectory)

	ga("POST", "/import", s.handleImport)
	ga("GET", "/export/problems", s.handleExportProblems)
	ga("GET", "/export/trainings/:id", s.handleExportTraining)
	ga("GET", "/export/practices/:id", s.handleExportPractice)
	// 全库备份/迁移：导出单包 / 导入恢复（见 backup.go）
	ga("GET", "/export/backup", s.handleExportBackup)
	ga("POST", "/import/backup", s.handleImportBackup)
	// 全量导入异步任务进度轮询（任务登记于 POST /import/backup，见 import_task.go）
	ga("GET", "/import/backup/task/:taskId", s.handleImportTask)

	// 域管理：列表查询允许两类管理员（domain_admin 仅见其域，用于显示域名）；其余域管理仅系统管理员
	gag := func(method, path string, h fiber.Handler) {
		api.Add(method, path, s.requireAdmin, s.requireGlobalAdmin, h)
	}
	ga("GET", "/admin/domains", s.handleListDomains)
	gag("POST", "/admin/domains", s.handleCreateDomain)
	gag("PATCH", "/admin/domains/:id", s.handleRenameDomain)
	gag("DELETE", "/admin/domains/:id", s.handleDeleteDomain)
	gag("PUT", "/admin/domains/:id/admin", s.handleSetDomainAdmin)
	gag("GET", "/admin/domains/:id/admins", s.handleListDomainAdmins)
	gag("DELETE", "/admin/domains/:id/admins/:uid", s.handleRemoveDomainAdmin)

	// 空间管理（系统/域管理员；requireAdmin 已限管理员，handler 内再按域校验）
	ga("GET", "/admin/spaces", s.handleListSpaces)
	ga("POST", "/admin/spaces", s.handleCreateSpace)
	ga("PATCH", "/admin/spaces/:id", s.handleRenameSpace)
	ga("DELETE", "/admin/spaces/:id", s.handleDeleteSpace)
	ga("GET", "/admin/spaces/:id/members", s.handleListSpaceMembers)
	ga("PUT", "/admin/spaces/:id/members", s.handleSetSpaceMembers)

	// 空间成员账号管理（系统/域管理员）：member 账号 CRUD
	ga("POST", "/admin/users", s.handleCreateUser)
	ga("GET", "/admin/users", s.handleListUsers)
	ga("DELETE", "/admin/users/:id", s.handleDeleteUser)
	ga("PUT", "/admin/users/:id/password", s.handleResetUserPassword)
	// 修改用户名（系统管理员任意账号；域管理员限本域成员）
	ga("PUT", "/admin/users/:id/username", s.handleRenameUser)
	// 集中用户管理（仅系统管理员）：全账号列表（含角色/归属域）
	gag("GET", "/admin/all-users", s.handleListAllUsers)

	// 空间内容管理（系统/域管理员）：空间训练/练习/刷题 结构 CRUD
	ga("GET", "/space/:id/trainings", s.handleListSpaceTrainings)
	ga("POST", "/space/:id/trainings", s.handleCreateSpaceTraining)
	ga("GET", "/space/:id/trainings/:tid", s.handleGetSpaceTraining)
	ga("PUT", "/space/:id/trainings/:tid", s.handleUpdateSpaceTrainingMeta)
	ga("DELETE", "/space/:id/trainings/:tid", s.handleDeleteSpaceTraining)
	ga("POST", "/space/:id/trainings/:tid/chapters", s.handleCreateSpaceChapter)
	ga("POST", "/space/:id/chapters/:cid/items", s.handleAddSpaceChapterItems)
	ga("PUT", "/space/chapters/:cid", s.handleRenameSpaceChapter)
	ga("DELETE", "/space/chapters/:cid", s.handleDeleteSpaceChapter)
	ga("PUT", "/space/trainings/:tid/chapters/order", s.handleReorderSpaceChapters)
	ga("PUT", "/space/chapters/:cid/items/order", s.handleReorderSpaceChapterItems)
	ga("GET", "/space/:id/practices", s.handleListSpacePractices)
	ga("POST", "/space/:id/practices", s.handleCreateSpacePractice)
	ga("GET", "/space/:id/practices/:pid", s.handleGetSpacePractice)
	ga("PUT", "/space/:id/practices/:pid", s.handleUpdateSpacePractice)
	ga("DELETE", "/space/:id/practices/:pid", s.handleDeleteSpacePractice)
	ga("POST", "/space/:id/practices/:pid/items", s.handleAddSpacePracticeItems)
	ga("PUT", "/space/practices/:pid/items/order", s.handleReorderSpacePracticeItems)
	ga("DELETE", "/space-items/:itemId", s.handleDeleteSpaceItem)
	ga("GET", "/space/:id/quizzes", s.handleListSpaceQuizzes)
	ga("POST", "/space/:id/quizzes", s.handleCreateSpaceQuiz)
	ga("PUT", "/space/:id/quizzes/:qid", s.handleUpdateSpaceQuiz)
	ga("DELETE", "/space/:id/quizzes/:qid", s.handleDeleteSpaceQuiz)
	// 可见成员授权（训练/练习/刷题：默认无成员可见，管理员分配）
	ga("GET", "/space/:id/trainings/:tid/visible", s.handleGetVisibleUsers)
	ga("PUT", "/space/:id/trainings/:tid/visible", s.handleSetVisibleUsers)
	ga("GET", "/space/:id/practices/:pid/visible", s.handleGetVisibleUsers)
	ga("PUT", "/space/:id/practices/:pid/visible", s.handleSetVisibleUsers)
	ga("GET", "/space/:id/quizzes/:qid/visible", s.handleGetVisibleUsers)
	ga("PUT", "/space/:id/quizzes/:qid/visible", s.handleSetVisibleUsers)

	ga("GET", "/trainings", s.handleListTrainings)
	ga("POST", "/trainings", s.handleCreateTraining)
	ga("GET", "/trainings/:id", s.handleGetTraining)
	ga("PUT", "/trainings/:id", s.handleUpdateTraining)
	ga("DELETE", "/trainings/:id", s.handleDeleteTraining)
	ga("POST", "/trainings/:id/chapters", s.handleCreateChapter)
	ga("PUT", "/trainings/:id/folder", s.handleSetTrainingFolder)
	ga("PUT", "/chapters/:id", s.handleUpdateChapter)
	ga("DELETE", "/chapters/:id", s.handleDeleteChapter)
	ga("POST", "/chapters/:id/items", s.handleAddChapterItems)
	ga("PUT", "/chapters/:id/items", s.handleReorderChapterItems)
	ga("PUT", "/trainings/:id/layout", s.handleTrainingLayout)
	ga("DELETE", "/items/:id", s.handleDeleteItem)

	ga("GET", "/practices", s.handleListPractices)
	ga("POST", "/practices", s.handleCreatePractice)
	ga("GET", "/practices/:id", s.handleGetPractice)
	ga("PUT", "/practices/:id", s.handleUpdatePractice)
	ga("DELETE", "/practices/:id", s.handleDeletePractice)
	ga("POST", "/practices/:id/items", s.handleAddPracticeItems)
	ga("PUT", "/practices/:id/folder", s.handleSetPracticeFolder)
	ga("PUT", "/practices/:id/items", s.handleReorderPracticeItems)
	ga("DELETE", "/practice-items/:id", s.handleDeletePracticeItem)
}

// mountSPA 将单页前端目录挂到根路径并做 SPA 回退（目录含 index.html 时）。
func mountSPA(app *fiber.App, webDist string) {
	if _, err := os.Stat(filepath.Join(webDist, "index.html")); err != nil {
		return
	}
	app.Static("/", webDist)
	app.Get("*", func(c *fiber.Ctx) error {
		return c.SendFile(filepath.Join(webDist, "index.html"))
	})
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
