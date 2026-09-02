# OrangeOJ 判题扩展实施计划

日期：2026-09-03 · 规格：`docs/aegis/specs/2026-09-03-orangeroj-design.md`（本计划引用其全部契约，不重复正文）
工作流记录：`docs/aegis/work/2026-09-03-orangeroj-build/`（10-intent.md 已建）

```text
TDD Route:
- Mode: off（沿用仓库惯例）
- Test posture: 判题语义/队列/迁移用 Go 单测覆盖（含真实 Python/C++ 冒烟）；server 走 httptest；前端 tsc+vite build
- Reason: 大规模功能扩展 + 评测是高风险面 —— 用「真实工具链评测测试」做验证锚点
- Verification: go vet ./...；go test ./...；web/web-quiz npm run build；GOOS=linux 交叉编译；本地 E2E
```

## 文件地图（相对现有结构）

```
cmd/judge-runtime/main.go           judge-runtime 入口（env 配置，上游 cmd/judge-runtime 同构）
internal/judge/queue.go             QueueService：claim（RETURNING 原子认领）/process/写回/failJob/Enqueue
internal/judge/runner.go            Runner 接口 + HTTPRunner + JudgeTask/CaseResult 类型 + NormalizeOutput
internal/judge/queue_test.go        用例选择/写回/进度 upsert 语义单测（fake Runner）
internal/judgeserver/executor.go    Execute(task)（语言命令/逐用例/verdict 判定）+ selfCheck
internal/judgeserver/sandbox_windows.go  受限运行后端（无 nsjail：超时杀进程树/最小环境/截断）
internal/judgeserver/sandbox_linux.go    nsjail 后端（复刻上游 executor.go runInSandbox 全参数）
internal/judgeserver/server.go      HTTP：/healthz、/internal/judge/execute（token 校验、body 4MB）
internal/judgeserver/executor_test.go  真实评测冒烟（python AC/WA/RE/TLE/CE；g++ 缺失则 t.Skip cpp）
internal/quizstore/quizstore.go     migrate 追加判题+布置表与索引
internal/quizstore/problems.go      只读 reader：GetFullProblem/GetProgrammingCaseSet/GetRepoTrainingSnapshot/GetRepoPracticeSnapshot
internal/quizstore/submissions.go   提交/队列/进度 CRUD + practice_records
internal/quizstore/assignments.go   布置 CRUD/可见性/统计
internal/quizstore/*_test.go        迁移+语义单测（store.Open 造主库数据）
internal/quizserver/judge.go        /api/oj 学生端路由处理器
internal/quizserver/assign.go       /api/admin/assignments + 布置列表
internal/quizserver/server.go       路由挂载、队列启动、flags 透传
internal/quizserver/*_test.go       httptest 冒烟（布置可见性/三动作/进度/统计/越权）
web-quiz/src/pages/AssignedList.tsx     训练/练习两页列表
web-quiz/src/pages/TrainingPage.tsx     训练章节导航/做题上下文
web-quiz/src/pages/PracticePage.tsx     练习题单
web-quiz/src/pages/ProblemSolvePage.tsx 做题页（编程+客观合一，run/test/submit 轮询、测评记录抽屉）
web-quiz/src/components/CodeEditor.tsx  textarea 编辑器（localStorage 草稿、语言切换）
web-quiz/src/pages/AdminAssignmentsPage.tsx 管理布置 Tab
web-quiz/src/lib/types.ts / api.ts    扩展
web-quiz/src/App.tsx               导航加 训练/练习、路由
Dockerfile.judge                    新
deploy/docker-compose.yml           三服务
scripts/dev-quiz.ps1                加 judge-runtime
README.md / docs/aegis/INDEX.md / BASELINE-GOVERNANCE.md  文档
```

## 任务序列（每任务一提交，编号便于证据记录）

1. **计划与规格落地**：本 plan + specs/2026-09-03-orangeroj-design.md + work/10-intent.md（已完成）
2. **judge-runtime + internal/judge + internal/judgeserver**（先于任何调用方）：
   - internal/judge：queue.go/runner.go 从上游迁移（去 go/turtle、归一化语言白名单 python|cpp）
   - internal/judgeserver：executor.go + sandbox_windows.go（本机先行）+ sandbox_linux.go（nsjail 参数同上游）
   - cmd/judge-runtime/main.go
   - 测试：executor_test.go（真实评测）；queue_test.go（mock runner）；`go vet ./...` 前先 `go build ./...`
3. **quiz.db 判题迁移 + 数据层**：quizstore.go migrate 增量表；submissions.go；quizstore 单测
4. **只读主库扩展**：problems.go 加题目详情/用例集合/训练练习快照读取；单测
5. **布置数据层**：assignments.go（快照/定向/级联/统计查询）；单测
6. **quizserver 学生端 /api/oj**：题目可见性/详情/run/test/submit/objective-submit/poll/历史 + 队列启动接线（cmd/quiz flags）→ httptest（runner 注入 fake）
7. **quizserver 管理端布置 /api/admin/assignments**：repo 浏览/布置 CRUD/学生维护/统计 → httptest
8. **web-quiz 页面**：路由/导航/列表页/训练练习详情/做题页（客观 + 编程）/测评记录抽屉/管理布置 Tab；`npm run build`
9. **真实 E2E**：主站造题 → 布置 → 学生真实 Python/C++ run/test/submit → 核对 AC/WA/RE/TLE/CE 与进度/统计/历史
10. **部署与收尾**：Dockerfile.judge、compose 三服务、dev-quiz.ps1、README/INDEX/BASELINE、证据文档

## 验证命令

```powershell
go vet ./... ; go build ./... ; go test ./...
GOOS=linux CGO_ENABLED=0 go build ./...        # 交叉编译（含 judge-runtime）
cd web ; npm run build ; cd .. ; cd web-quiz ; npm run build
# E2E：三进程起服 → curl 全链路（见 90-evidence.md）
```

## 风险与回滚
- Windows 本机无 g++ → 先 `choco install mingw`（无管理员则仅测 Python + cpp 用例 t.Skip）；CI/生产不受影响
- nsjail 仅在 Linux 镜像内 → Windows 后端不做安全承诺（开发用），代码路径按 build tags 或 runtime 探测隔离
- 上游代码 mojibake 风险 → 只迁移 Go 后端（UTF-8 干净）；前端按交互重写（不复刻乱码文案）
- 快照读取边界：布置冻结 id 列表；做题动态读主库；主库删题 → 跳过并提示
- 回滚面：每任务独立提交；quiz.db 追加表幂等可平滑降级（新代码停止写新表即可，旧表不动）；一期功能零改动
