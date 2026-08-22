# OrangeRepo 构建任务 — 意图与基线

- 目标 outcome：OrangeOJ 兼容的纯题库管理 Web 应用（Go+Fiber+SQLite / React+shadcn），含嵌套目录、标签筛选、题目编辑、训练练习编制、ZIP 双向导入导出、单用户登录。
- 非目标：判题/提交/成员/进度等 OJ 运行时功能。
- 基线 refs：上游 OrangeOJ export_handlers.go / db.go / objective_answers.go / queue.go（已读，格式事实记录于 specs/2026-08-22-orangerepo-design.md §2）。
- 计划：docs/aegis/plans/2026-08-22-orangerepo.md（TDD Route: off/skipped，post-change regression）。
