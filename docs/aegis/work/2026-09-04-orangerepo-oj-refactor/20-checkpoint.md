# OrangeRepo OJ 重构 —— 检查点（更新）

规格：docs/aegis/specs/2026-09-04-orangerepo-oj-refactor-design.md（已确认）
工作目录：docs/aegis/work/2026-09-04-orangerepo-oj-refactor/

## TodoCheckpoint（当前进展）
已完成提交：
1. ddf33fd 阶段 1 UUIDv7（problems.uuid 列/backfill/导入去重/导出带 uuid）
2. 23498d4 阶段 2 数据层+账号角色（users.domain_id + 三角色迁移重建；domains/spaces/space_members
   表 + store_domains.go；problems.domain_id 存量归默认域；仓库登录放宽两类管理员）
3. 4618fe9 阶段 3 域/空间管理 API（domains CRUD/设域管理员/空间 CRUD+成员/users CRUD +
   respondError 语义修复——鉴权中断 bug 修复 + ProblemFilter.DomainID）

## 关键技术教训（重要，勿重复踩坑）
- ★ respondError 原来写响应后返回 nil → Fiber 中间件/组 require 后仍继续执行后续 handler，
  handler 内嵌 require 也失效。已修为返回 fiber.NewError（ErrorHandler 统一 JSON），
  两服务（server/quizserver）均改。**新增 require 类函数必须返回非 nil error**
- SQLite 改 CHECK 需重建表（accounts.Migrate migrateUsersSchema：检测 DDL 含 global_admin 且
  有 domain_id 列才跳过重建；INSERT SELECT CASE 映射角色）
- 测试 doJSON 返回 (*http.Response, map) 第一值是 resp 对象
- domain id 注意「默认域」backfill 只在有存量题时建（空库无默认域，域 id 从 1 起）

## 剩余工作（阶段 4-6 未动）
- [ ] 阶段 4 空间训练/练习 API 与存储：space_trainings/training_chapters/items 限次、
  space_practices 整卷交卷；判题提交空间化；从仓库模板拷贝结构
- [ ] 阶段 5 仓库页前端（web）：域切换 + 空间管理 UI + 题目按域隔离
- [ ] 阶段 6 门户前端（web-quiz）：空间切换 + 训练（限次标色）/练习（整卷交卷）做题页；
  移除随机刷题/错题；角色 UI 适配 global_admin/domain_admin/member
- [ ] 端到端验证（test-oj.ps1 适配）+ 规格文档最终核对 + compose/README

## 下一步建议
阶段 4 后端：space 训练/练习表迁移与 store API（含从仓库模板 import 结构、限次记录表、
练习提交表）；建议先做存储+核心 API 再用 agent 并行写前端。

## 风险
- quizstore 只读 RepoReader 目前读主库所有题（无域过滤）——空间化做题前必须按 domain 过滤
- 旧 assignments/subjects/categories/错题表最终退役（用户已确认移除随机刷题）但迁移期保留无害
