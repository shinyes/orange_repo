# OrangeRepo OJ 重构 —— 检查点

规格：docs/aegis/specs/2026-09-04-orangerepo-oj-refactor-design.md（已确认，决策 A–F + UUID + 角色模型全锁定）

## 基线（已完成调研，两份 agent 报告）
- 双库双进程；store 迁移模式；主站=管理工具仅 admin；web-quiz=学生门户
- 行数规模：store 1638 / server 2081 / quizserver 1827 / quizstore 1850 / accounts 368

## TodoCheckpoint（2026-09-04 晚）
已完成：
- [x] 规格 + 决策全锁定（含 A–F 自定义：次数只存在于训练内统一配置/可重做记录/域管域内全空间/成员=学生/仓库保留模板/移除随机刷题）
- [x] 阶段 1 UUIDv7 —— commit ddf33fd（创建生成/导入去重/export 带 uuid，测试绿）
- [x] 阶段 2 数据层 + 账号角色 —— commit 23498d4
  （users.domain_id + 三角色重建迁移；domains/spaces/space_members 表 + store_domains.go CRUD；
   problems.domain_id 补列存量归「默认域」；仓库页登录放宽 admin 两类；全测试绿）

进行中：
- [ ] 阶段 3 域/空间管理 API + 题目按 domain 隔离（server handlers）
- [ ] 阶段 4 空间训练（限次）/练习（交卷）API 与存储 + 空间化判题
- [ ] 阶段 5 仓库页前端改造（域切换 + 空间管理 UI）
- [ ] 阶段 6 门户前端（空间切换/训练/练习卷面）
- [ ] 端到端验证 + 文档

## 下一步（阶段 3）
server 层：
1. /api/admin/domains CRUD（global_admin）+ 域管理员设置（users.domain_id 更新）
2. /api/admin/spaces CRUD + 成员管理（域管理员或 global_admin；校验空间属域）
3. 题目 API 按域隔离：CreateProblem 带 domain_id（来自用户域上下文）、List/Get/Update/Delete 过滤 domain_id
4. me 接口返回 role/domainId/可切换域（global_admin 域列表；member 空间列表）
注意 quizstore 只读 RepoReader 读题也会读到跨域题目——域化后刷题端按空间查题需过滤（阶段 4 统一）

## 风险
- 题目 domain 过滤会使"无域旧数据/默认域"迁移后的老单库仍工作（单默认域=现状等价）
- 前端 web-quiz 按 role 'admin' 判断管理入口——现在 global_admin 需同步改（阶段 5/6）
- RepoReader（quiz.db 只读 orangerepo.db）后续需按空间/域过滤题目
