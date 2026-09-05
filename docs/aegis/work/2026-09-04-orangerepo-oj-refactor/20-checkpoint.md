# OrangeOJ 重构 —— 检查点（后端全部完成 2026-09-05）

规格：docs/aegis/specs/2026-09-04-orangerepo-oj-refactor-design.md（含 11 排行榜/12 刷题，全系统命名 OrangeOJ）

## ✅ 后端完成（本地 main 领先 origin 9 commits，全部测试绿）
1. ddf33fd 题目 UUIDv7
2. 23498d4 域/空间数据层 + 三角色账号迁移（users.domain_id）
3. 4618fe9 域/空间/用户管理 API + respondError 语义修复（★鉴权中断关键 bug）
4. 0aa55f9 空间训练/练习/刷题数据层（store）+ 规格追加排行榜/刷题
5. db88bb5 checkpoint
6. ace66ea 空间内容管理 API（主站）+ 作答数据迁 quiz.db（quizstore）
7. 8aca5fd checkpoint
8. 26ba675 quizserver 门户 API（portal/portal2/helpers + RepoReader repo_space）
9. b826d92 门户端到端集成测试 TestPortalMemberFlow（限次/锁定/交卷/刷题/排行全链路过）

## 架构（最终）
- orangerepo.db（主库 store）：domains/spaces/space_members + problems(domain_id,uuid) +
  空间训练/练习/刷题【结构】+ 仓库模板（旧 trainings/practices）
- quiz.db（quizstore/accounts）：users/sessions/判题 + 作答（space_training_attempts/
  space_practice_submissions/student_solved）
- 主站 server（管理）：域/空间/成员/用户 + 空间内容结构 CRUD（/api/space/:id/...）
- quizserver（门户）：/api/portal/**（spaces/home/training+answer/practice+submit/quizzes/quiz/rank）
  作答判定复用 gradeObjective；限次/锁由 handler 拦（读主库 max_attempts）

## 剩余：前端两大阶段（尚未开始）
- [ ] 阶段 5 仓库页前端 web：改造为「域仓库」——登录角色分流（global/domain admin）、
  域切换器（global）、去训练/练习编制 UI 的题目管理 + 空间管理页（域管理员：空间 CRUD/成员拉入/训练练习刷题布置）
  注意：现有 web 是纯 Context 状态机无路由——需加域/空间上下文
- [ ] 阶段 6 门户前端 web-quiz：登录后按角色：
  member → 空间切换（多空间 pill）+ 空间内 Tab（训练/练习/刷题/排行榜，替代原 刷题科目/错题/布置）；
  domain_admin → 域内空间选择 + 同 member 页面 + 管理入口；
  global_admin → 域选择
  做题页：训练（章节列表绿/红/次数徽标+禁选交互）、练习（整卷交卷流程）、刷题（随机轮即时反馈）
- [ ] 端到端三进程验证（scripts/test-oj.ps1 全面重写适配域流程）+ 文档 + 旧模块退役
  （subjects/categories/assignments/错题 API 与页面移除或保留兼容；web-quiz /quiz /wrong /admin 路由收敛）

## 关键约定（前端开发必读）
- 角色值：global_admin/domain_admin/member（旧 admin/student 已迁移）
- quiz 服务 me 返回 {authenticated,user{role,domainId}}；主站 me 仅 authenticated
- 题目 domain 隔离尚未完全收紧（CreateProblem 不带域→默认域；仓库模板拷贝未校验同域——
  前端仓库页建题时应带域参数；后续 API 需补）
- 排行榜/作答端点需 resolveSpace 成员校验；管理员列表空间：domain_admin 限域/global 全部
