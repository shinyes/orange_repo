# 刷题服务（quiz-service）— 任务意图

## Requested Outcome
按已确认规格 `docs/aegis/specs/2026-08-29-quiz-service-design.md` 与计划 `docs/aegis/plans/2026-08-29-quiz-service.md` 实现独立刷题服务（:8081 + web-quiz/:5174）。

## Scope
- 后端：cmd/quiz 入口、internal/quizstore（quiz.db + orangerepo.db 只读）、internal/quizserver（auth/quiz/admin）
- store.go 仅「tagMatchesSelected → TagMatchesSelected」导出改名
- 前端 web-quiz（登录/刷题/错题/我的/系统管理）
- scripts/dev-quiz.ps1、README、INDEX
- 验证：go vet/test 全量 + 双前端 build

## Non-Goals
编程题、多选、统计报表、Docker/CI、题库编辑。

## Baseline ReadSet Hint
- 必读：spec 2026-08-29-quiz-service-design.md（已读）、内部 store/server/model 现状（已读）
- 已确认决策：共享 SQLite 只读 / 管理员建学生账号 / 解析=题解 markdown / 每轮固定题数全局配置 / 任何模式答对即移除 / 答后展示正确答案 / 全部错题入口

## Impact Statement
- 影响层：新增 cmd/quiz、internal/quizstore、internal/quizserver、web-quiz、scripts/dev-quiz.ps1；修改 internal/store（改名）、README、INDEX
- 不变式：不写主库（mode=ro）；主站零行为变更；quiz.db 物理隔离
- 风险：跨进程共享 SQLite 只读并发（busy_timeout 兜底）；前端组件复制（接受）