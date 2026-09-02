# OrangeOJ 判题扩展（orangeroj-build）— 20-checkpoint

## 阶段核对（2026-09-03）

计划 10 个任务完成情况：

| # | 任务 | 状态 | 提交 |
|---|---|---|---|
| 1 | 规格/计划/意图文档 | ✅ | 4e77ea1 |
| 2 | judge-runtime + internal/judge + internal/judgeserver + 测试 | ✅ | 4e77ea1 |
| 3 | quiz.db 判题迁移 + submissions 数据层 + 单测 | ✅ | 4e77ea1 |
| 4 | 只读主库扩展（题目正文/训练练习结构） | ✅ | 4e77ea1 |
| 5 | assignments 布置数据层 + 单测 | ✅ | 4e77ea1 |
| 6 | quizserver 学生端 /api/oj + 队列接线 + httptest | ✅ | 4e77ea1 |
| 7 | quizserver 管理端布置 /api/admin/assignments + httptest | ✅ | 4e77ea1 |
| 8 | web-quiz 页面（列表/详情/做题页/管理布置 Tab） | ✅ | 44120d6 |
| 9 | 真实 E2E（三进程 Python/C++ 全链路） | ✅ | 见 90-evidence |
| 10 | Dockerfile.judge + compose 三服务 + dev-quiz.ps1 + README/INDEX/BASELINE/证据 | ✅ | 本提交 |

## 决策变更记录
- 布置内容从「布置时快照冻结」修订为「实时跟随主库」（与上游同库同构，训练/练习结构与题目在同一库）：
  spec §3/§4.2 已同步；assignment 仅存 (kind, repo_id) 引用与可见性；**practice_records/assignment_train_* 快照表已从迁移移除**
- 一期「错题集」与 OJ 客观题提交解耦：OJ objective-submit 只写 submissions+progress，不进错题集
  （错题集语义绑定刷题分类，OJ 题不在分类体系内）
- 前端编辑器用 textarea（无 Monaco 依赖），代码草稿 localStorage 按题目+语言分存

## 遗留/后续建议（非本期范围）
- GHCR 自动构建 judge 镜像的 CI workflow（本期 compose 本地构建）
- 代码行内高亮/编辑增强、Special Judge、多文件
