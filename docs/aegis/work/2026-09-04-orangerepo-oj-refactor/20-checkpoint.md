# OrangeOJ 重构 —— 检查点（全部阶段完成 2026-09-05）

规格：docs/aegis/specs/2026-09-04-orangerepo-oj-refactor-design.md（含排行榜/刷题，系统命名 OrangeOJ；
重构总原则：除全仓导入导出外不向后兼容、可维护性优先）

## ✅ 全部完成（本地 main 领先 origin 16 commits，全测试绿 + 三进程 E2E 通过）
1. ddf33fd 题目 UUIDv7
2. 23498d4 域/空间数据层 + 三角色账号迁移
3. 4618fe9 域/空间/用户管理 API + respondError 语义修复（鉴权中断 bug）
4. 0aa55f9 空间内容数据层 + 规格追加排行榜/刷题需求
5. ace66ea 空间内容管理 API + 作答数据迁 quiz.db
6. 26ba675 quizserver 门户 API（训练限次/练习交卷/刷题/排行榜）
7. b826d92 门户端到端集成测试
8. 17e697e 题目域隔离全面收紧（创建/列表/读改删/导入/备份归域）
9. 8c195ab web 仓库页域化（域切换/域管理/空间管理）+ 后端缺口补齐
10. af3269d 空间编制从题库选题弹窗 + 证据
11. 8e4694d 空间成员题目可见性修复（纯空间成员可经 /api/oj/problem 做题）
12. 9ebd09e web-quiz 门户空间化（SpacePicker/SpaceShell/训练限次/练习整卷/刷题/排行榜/移除旧模块）

## 架构（最终交付）
- orangerepo.db（主库）：domains/spaces/space_members + problems(domain_id,uuid) +
  空间训练/练习/刷题【结构】+ 仓库模板（题册 trainings/practices/booklet dirs 保留=域级模板管理）
- quiz.db：users/sessions + 作答三表（attempts/submissions/student_solved）+ 判题
- 主站 :8080（web）：域仓库页（登录分流 → global 域切换/域管理；domain_admin 空间管理/域内仓库；member 拒）
- 门户 :8081（web-quiz）：空间化（/ 空间选择 → /s/:sid 训练/练习/刷题/排行榜；/problem/:id 做题）
- 旧模型已退役：科目/分类/错题/布置/随机轮（表保留于库内未删，API 与前端已移除；按"不做兼容"原则可后续清理表）

## E2E 证据（真实双进程 HTTP）
建域→空间→member→域内题→空间训练→member 登录→作答 solved→排行榜 1人1题 全 OK（probe 输出留存于会话）

## 未决/可后续
- quiz.db 旧表（subjects/categories/wrong_answers/assignments/assigned_students）与主库旧目录表
  未 DROP（API/前端已断）；按原则可安全清理
- 练习交卷历史无每题明细回放（快照存库）——前端仅显示答对数，可后续增强
- 空间训练条目响应不带 problemTitle（前端兜底 #id）——可后续补
