# OrangeOJ 判题扩展 — 任务意图

## Requested Outcome
用户：「模仿 https://github.com/shinyes/OrangeOJ 的实现方式，扩展 Orange 刷题，成为新的 OrangeOJ，目前只需要保留对 Python 和 C++ 的支持即可」。
按已确认规格 `docs/aegis/specs/2026-09-03-orangeroj-design.md`（用户逐项拍板：独立 judge-runtime / 无题库浏览仅布置驱动 / run+test+submit 全做 / 接受 Docker 三服务改造 / 布置模仿 OrangeOJ 训练+练习语义 / 整份布置含客观题 / 学生历史+管理端汇总）实现。

## Scope
- 新增：cmd/judge-runtime、internal/judge（队列/HTTPRunner）、internal/judgeserver（executor + Windows/Linux 沙箱后端 + HTTP server）、quiz.db 判题与布置迁移、/api/oj 学生端、/api/admin/assignments 管理端、web-quiz 训练/练习/做题页/管理布置 Tab、Dockerfile.judge、compose 三服务、docs 全套
- 修改：cmd/quiz（队列启动+flags）、internal/quizstore、internal/quizserver、scripts/dev-quiz.ps1、README、INDEX、BASELINE-GOVERNANCE
- 语言：仅 Python + C++（不做 go/turtle）

## Non-Goals
题库浏览、管理端看代码、Special Judge、比赛、布置后题目结构变更跟随（快照）、题面用例编辑、多空间。

## Baseline ReadSet Hint
- 上游 OrangeOJ main 源码快照（已下载 $TEMP\OrangeOJ-src，已读：queue.go/runner.go/executor.go/judgeserver/server.go/judge-runtime main/db.go 判题表/training_handlers/practice_handlers/submission_handlers/CodingPage.jsx/api.js/routes.go/Dockerfile.judge/compose）
- 本仓库：quiz-service spec+plan+代码（已读）、orangerepo spec（已读）、store/server/accounts/quizstore/quizserver/web-quiz（已读）

## Impact Statement
- 影响层：新增 3 个 Go 包 + 1 个 cmd + web-quiz 大改 + 部署文件 + 文档；主站（orangerepo 主库/主进程）零改动
- 不变式：不写主库（mode=ro 维持）；一期路由零改动；判题安全隔离只在 judge-runtime（生产 Linux nsjail）；Windows 后端仅供开发
- 风险：Windows 无 nsjail → 执行器双后端（Windows 受限运行仅开发）；本机无 g++ → 需先装 mingw（choco）；快照 vs 动态读取边界需测试锁定；web-quiz 前端较大（做题页复刻上游 CodingPage 子集）

## 关键决策记录（本工作流）
- 布置对象：训练=参与学生（可全员）、练习=目标学生（可全员）——模仿上游 participants/targets 语义，一期合并为 assignments.assigned_all / assigned_students 通用模型
- 布置内容：快照章节结构/题目 id（不复制题目正文），做题时实时读主库
- 客观题也在做题页内联（objective-submit 写 submissions+progress）
- 统计不含代码；学生端可见 assigned_all 或 in assigned_students 且 published=1 的任务
