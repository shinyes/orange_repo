# Aegis Workspace — OrangeRepo

本目录存放 OrangeRepo（题库管理应用）的治理文档：规格（specs/）、实施计划（plans/）、基线快照（baseline/）。

- 产品定位：纯题库管理工具（目录树 / 标签 / 搜索 / 题面·答案·题解编辑 / OrangeOJ ZIP 兼容导入导出 / 训练练习编组）。非目标：判题、提交、多用户空间等一切 OJ 运行时功能。
- 权威格式基线：上游 https://github.com/shinyes/OrangeOJ 的 `backend/internal/api/export_handlers.go`、`internal/db/db.go`、`internal/api/objective_answers.go`、`internal/judge/queue.go`。
