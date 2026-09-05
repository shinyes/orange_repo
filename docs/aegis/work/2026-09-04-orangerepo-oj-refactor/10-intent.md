# OrangeOJ OJ 重构 —— 任务意图与检查点

任务：重构 OrangeOJ 系统为 域(domain)→仓库(仓库页=原主站)→空间(space) 三级模型，
空间训练（客观题限次标色）/练习（整卷交卷），系统管理员管域、域管理员管域内空间，
题目 UUIDv7。规格：docs/aegis/specs/2026-09-04-OrangeOJ-oj-refactor-design.md（已确认）。

## 基线（Backend/frontend 调研已完成，见 agent 报告）
- 双库双进程：orangerepo.db（题目/训练/练习/目录）+ quiz.db（账号/学生数据/判题）
- 主站 = 管理工具（仅 admin 登录）；web-quiz = 学生门户（admin/student）
- store 迁移模式：CREATE IF NOT EXISTS + ensureColumn/dropColumn + 一次性 legacy
- problems/trainings/chapters/items/practices 全为全局自增 id 单命名空间

## TodoCheckpoint（当前）
已完成：
- [x] 规格文档 + 决策全锁定（A–F + UUID + 域/空间模型）
- [x] 阶段 1 UUIDv7（commit ddf33fd，测试绿）

进行中：
- [ ] 阶段 2 数据层：domains/spaces/space_members 迁移 + problems.domain_id + 账号角色扩展
- [ ] 阶段 3 域管理 + 仓库域隔离 API
- [ ] 阶段 4 空间管理 + 空间训练/练习（限次/交卷）
- [ ] 阶段 5 判题数据空间化 + 旧模块清理
- [ ] 阶段 6 前端：仓库页改造（域切换）+ 门户（空间切换/训练/练习卷面）
- [ ] 端到端验证 + 文档 + 提交

## 下一步
阶段 2：store 迁移 domains/spaces/space_members + problems.domain_id + users 角色
（role 扩 global_admin/domain_admin/member + users.domain_id；accounts 迁移）。

## 风险
- 域化会把 problems 从"全局单命名空间"改为 domain 作用域：id 仍全局自增但查询/外键按 domain 过滤，
  quiz.db assignments.repo_id 跨库引用语义（将随空间化重构）
- quiz.db 学生表（submissions 等）未来加 space 归属；刷题端只读主库 RepoReader 的隔离
- 前端主站从管理工具变仓库页 = 角色/入口大改
