# OrangeRepo OJ 重构 —— 检查点（阶段 1-4b）

规格：docs/aegis/specs/2026-09-04-orangerepo-oj-refactor-design.md（已确认，含 11/12 节新增
排行榜与刷题项目需求）

## 已完成提交（本地 main 领先 origin 6）
1. ddf33fd UUIDv7（uuid 列/backfill/导入去重/导出带 uuid）
2. 23498d4 域/空间数据层 + 账号角色（users.domain_id + 三角色重建迁移；domains/spaces/
   space_members + problems.domain_id 存量归默认域）
3. 4618fe9 域/空间/用户管理 API + respondError 语义修复（鉴权正确中断关键 bug）
4. 0aa55f9 空间内容数据层（store）+ 规格追加排行榜/刷题需求
5. db88bb5 checkpoint
6. ace66ea 作答数据迁 quiz.db（quizstore space_progress）+ 空间内容管理 API（主站）
   + 从仓库模板拷贝 fromRepo

## 架构最终形态
- 主库 store：domains/spaces/space_members/problems(domain_id+uuid)/
  空间训练/练习/刷题【结构】/ 仓库模板（旧 trainings/practices）
- quiz.db quizstore：users/sessions/判题 + 作答三表（space_training_attempts/
  space_practice_submissions/student_solved）
- 主站 server：域/空间/用户管理 + 空间内容结构管理（管理员）
- quizserver：学生门户（作答/判题/排行榜/刷题）——【尚待实现】

## 剩余工作
- [ ] 4c: quizserver 门户 API：空间列表/切换（member 多空间）、训练/练习/刷题结构只读
      （RepoReader 加空间表读法——quizserver 只读主库）、训练客观题作答（限次/标色判定）、
      练习整卷作答+交卷、刷题随机轮、排行榜查询
- [ ] 5: 仓库页前端（web）：登录角色分流、域切换、空间管理、空间内容管理 UI
- [ ] 6: 门户前端（web-quiz）：空间切换、训练/练习/刷题/排行榜页
- [ ] 端到端 + 文档 + 旧模块退役
