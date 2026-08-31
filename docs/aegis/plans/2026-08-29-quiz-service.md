# 刷题服务（quiz-service）实施计划

日期：2026-08-29 · 依据规格：`docs/aegis/specs/2026-08-29-quiz-service-design.md`（已确认）

## 1. 目标

在 orange_repo 仓库新增独立刷题服务：Go 服务 `cmd/quiz`（默认 :8081）+ 独立前端 `web-quiz/`（dev :5174），实现刷题页、错题页、我的页（管理员含系统管理）、学生账号、科目/分类管理（标签+题型映射、显示顺序）、全局每轮题数设置。题库数据只读共享 `data/orangerepo.db`，自有数据写入 `data/quiz.db`。

## 2. 架构与文件映射

```
cmd/quiz/main.go                入口（-addr :8081 / -data ./data / -web ./web-quiz/dist / -repo-db）
internal/quizstore/quizstore.go quiz.db：迁移 + users/sessions/subjects/categories/wrong_answers/settings
internal/quizstore/problems.go  orangerepo.db 只读 reader（mode=ro）+ 筛选/抽题/判题取数
internal/quizstore/*_test.go    单测（造临时 orangerepo.db 验证筛选与抽题）
internal/quizserver/server.go   Fiber 组装：路由 / 会话 / 静态 /api/uploads + web-quiz/dist
internal/quizserver/auth.go     bootstrap 管理员 + login/logout/me/password
internal/quizserver/quiz.go     学生端点（subjects/round/submit/wrong-round/wrong-summary）
internal/quizserver/admin.go    管理员端点（科目/分类/学生/设置/problems-count）
internal/quizserver/server_test.go  httptest 冒烟
internal/store/store.go         仅：tagMatchesSelected → TagMatchesSelected 导出（语义不变）
web-quiz/                       独立 Vite 应用（复制 web/ 的 UI 组件与渲染管线）
scripts/dev-quiz.ps1            一键起 4 进程（主站 :8080/:5173 + 刷题 :8081/:5174）
README.md                       功能与新服务快速开始
docs/aegis/INDEX.md             登记本计划
```

技术栈：Go 1.25 + Fiber v2 + modernc.org/sqlite + bcrypt；React 19 + Vite + TS + Tailwind v4 + shadcn 组件 + marked/DOMPurify/KaTeX + TanStack Query。

## 3. 权威基线 / 兼容边界

- 唯一权威规格见上；API 契约以其 §5 为准，字段一律 camelCase。
- **兼容边界（硬）**：不写 orangerepo.db（`mode=ro`）；不迁移主库；主站零行为变更（store 仅导出改名）；`data/quiz.db` 与主库物理隔离；前端 API 路径 `/api/*` 与主站形态一致（Vite 代理、上传图片同 `/api/uploads` 约定）。

## 4. TDD Route

```text
TDD Route:
- Mode: off
- Decision: skipped
- Strict authority: not applicable
- Test posture: post-change regression + httptest 冒烟（仓库既有姿势）
- Reason: 无 strict 请求；沿用仓库「实现 + 回归/冒烟」惯例
- Verification: go vet ./... ; go test ./... ; cd web ; npm run build ; cd ../web-quiz ; npm run build
```

## 5. 任务切片

### Slice A — 数据层（store 改名 + quizstore）

**A1. internal/store 导出 TagMatchesSelected**
- Files: modify `internal/store/store.go`
- Why: quizstore 复用主库前缀 AND 标签匹配语义，避免复制实现（单一 owner）
- Change Necessity: code-change —— 复用需要导出；最小边界 = 改名 + 更新两处调用
- 步骤：
  1. `tagMatchesSelected` 改名为 `TagMatchesSelected`，加 doc 注释「匹配选中标签集（前缀包含 + AND），供刷题服务只读复用」
  2. `ListProblems`、`ListTagFacets` 内调用点同步改名
- Verification: `go build ./... && go test ./internal/store`

**A2. internal/quizstore/quizstore.go**
- Files: create（约 400 行）
- 内容要点（完整契约）：
  - `Store struct { DB *sql.DB; Repo *RepoReader }`；`Open(dataDir, repoPath string) (*Store, error)`：mkdir dataDir；quiz.db DSN `file:<quiz.db>?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)`；`SetMaxOpenConns(1)`；迁移（规格 §3 五张表 + settings 补 `round_size` 默认 10，幂等 CREATE TABLE IF NOT EXISTS）；随后 `OpenRepoReader(repoPath)`
  - users：`CreateUser(username, password, role)`（校验：trim 非空、1–32 字符、无控制字符、大小写不敏感唯一；bcrypt；role ∈ admin|student）、`GetUserByUsername`、`GetUserByID`、`ListStudents() ([]Student{ID, Username, CreatedAt, WrongCount})`（LEFT JOIN 计数）、`DeleteUser(id)`（级联 sessions/wrong_answers）、`ResetPassword(id, password)`
  - sessions：`CreateSession(userID) (token, error)`（32B hex）、`GetUserByToken(token) (*User, bool)`（JOIN users，会话失效即删除）、`DeleteSession(token)`
  - settings：`GetRoundSize() int`（默认 10）、`SetRoundSize(n)`（1–100 校验）
  - subjects：`ListSubjects() ([]Subject{ID, Name, OrderNo, Categories []Category})`、`CreateSubject(name) (id, error)`（orderNo = max+1）、`RenameSubject(id, name)`、`DeleteSubject(id)`、`SetSubjectOrder(ids []int64)`（事务内按序写 order_no=idx+1，校验齐全性）
  - categories：`CreateCategory(subjectID, name, orderNo, tags, types) (id, error)`（types 校验仅 single_choice|true_false）、`UpdateCategory(id, name?, orderNo?, tags?, types?)`、`DeleteCategory(id)`、`SetCategoryOrder(subjectID, ids)`、`GetCategoriesBySubject`；tags 逐条 `store.ValidateTagPath`
- Verification: `go vet ./internal/quizstore && go build ./...`（暂未测试主体，A4 补）

**A3. internal/quizstore/problems.go（只读 reader）**
- Files: create（约 180 行）
- 内容要点：
  - `OpenRepoReader(path string) (*RepoReader, error)`：DSN `file:<path>?mode=ro&_pragma=busy_timeout(5000)`；`SetMaxOpenConns(1)`；打开失败返回含「请先运行主站初始化 data/orangerepo.db」的错误；不做迁移
  - `QueryProblems(tags []string, types []string) ([]ProblemBrief, error)`：`SELECT id,type,title,statement_md,body_json,solutions_json FROM problems WHERE type IN (?)`（types 空 → IN('single_choice','true_false')）；Go 侧 `store.TagMatchesSelected(p.Tags, tags)` 过滤（先 `SELECT id,tags_json` 全表扫得到 tags 再做 IN 过滤太绕 → 直接全表取行再 Go 过滤，与主库 facets 同量级）
  - `CountProblems(tags, types) (int, error)`（复用 QueryProblems 计数即可，阶段一量级可接受）
  - `GetQuizProblem(id) (*QuizProblem, error)`：`{ID, Type, Title, StatementMD, BodyJSON(json.RawMessage), HasExplanation bool}`；HasExplanation = solutions_json 解码后存在 markdown 非空的条目（解码失败或空列表 = false）
  - `GetAnswer(id) (*AnswerEnvelope, error)`：`{Type, AnswerIndex *int, Answer *bool}`（single_choice 读 answer_json.answerIndex；true_false 读 answer 布尔；缺失/类型不符 → 错误「题目答案缺失或格式异常」）
  - `GetExplanation(id) (string, bool)`：solutions 中第一条 markdown 非空的文本
- Verification: `go build ./...`

**A4. quizstore 测试**
- Files: create `internal/quizstore/quizstore_test.go`、`internal/quizstore/problems_test.go`
- 内容要点：
  - 辅助：`t.TempDir()`；用 `store.Open(tmp)` 造主库并 `CreateProblem` 若干（单选+判断+编程；标签含前缀关系：`数学`、`数学/几何`、`物理/力学`、无标签题）；关闭后由 `OpenRepoReader` 只读打开验证
  - 用例：标签前缀 AND 命中数与抽题全集；types 过滤；HasExplanation（有 markdown 题解 true / 空题解 false / 无题解 false）；GetAnswer 两题型；quiz.db：创建用户/重复用户名冲突/学生列表计数、科目分类 CRUD 与排序、分类级联、错题 upsert 保留首分类/答对移除、roundSize 边界校验
- Verification: `go test ./internal/quizstore ./internal/store`

### Slice B — 服务端（quizserver + 入口）

**B1. internal/quizserver/server.go**
- Files: create
- 内容要点：
  - `New(qs *quizstore.Store, uploadsDir, webDist string) *fiber.App`：与主站同款 ErrorHandler/BodyLimit/logger/recover
  - Cookie 名 `quiz_session`；`requireSession`（查 token → Locals("user")，无效 401）、`requireAdmin`（role != admin → 403）
  - 路由：`/api/auth/*`（login 公开；logout/me/password 需会话）；`/api/quiz/*`（会话）；`/api/admin/*`（admin）；`/api/uploads` 静态（uploadsDir 存在时）；webDist 存在时 `app.Static("/", webDist)` + SPA 回退
  - `/api/health` 公开
- Verification: `go build ./...`

**B2. internal/quizserver/auth.go**
- Files: create
- 内容要点：`EnsureBootstrap()`（无 admin 用户时建 `admin/123456`，返回 bool 并日志）；`handleLogin`（bcrypt 比对 → CreateSession → SetCookie MaxAge 30d HTTPOnly SameSite Lax）；`handleLogout`；`handleMe`（返回 `{authenticated, user}`）；`handleChangePassword`（旧密码校验 → 更新哈希 → 轮换当前会话 token）
- Verification: `go build ./...`

**B3. internal/quizserver/quiz.go（学生端点）**
- Files: create
- 内容要点（契约来自规格 §5，逐条实现）：
  - `handleListSubjects`：`ListSubjects` + 每分类 `CountProblems` → `{subjects:[{id,name,orderNo,categories:[{id,name,orderNo,questionCount}]}]}`
  - `handleStartRound`：body `{categoryId}` → 校验分类存在 → `QueryProblems(cat.Tags, cat.Types)` → `rand.Shuffle` → 取前 roundSize → 逐题 `GetQuizProblem`（含题目缺失跳过）→ `{categoryId, total, problems: [...]}`
  - `handleSubmit`：body `{problemId, categoryId, optionIndex?, answer?}` → `GetAnswer` 判题：single_choice 需 `optionIndex` 为有效整数且落在选项区间；true_false 需 `answer` 为布尔；判定结果 → correct: `RemoveWrong(userID, problemID)`；wrong: `AddWrong(userID, problemID, categoryID)`（分类不存在时容错跳过写入）→ 响应 `{correct, correctAnswer, hasExplanation, explanation}`（GetExplanation）
  - `handleStartWrongRound`：body `{categoryId?}` → `ListWrongProblems(userID, categoryID?)`（JOIN 主库存在性，随机全量）→ 每题附记录 `categoryId` → `{scope, categoryId?, problems}`
  - `handleWrongSummary`：`{total, groups:[{categoryId, categoryName, subjectName, count}]}`
- Verification: `go build ./...`

**B4. internal/quizserver/admin.go**
- Files: create
- 内容要点：规格 §5 管理员端点逐条；`problems-count`（GET，query tags/types → CountProblems）；排序端点校验 id 集合与归属（防跨科目乱序）；students `POST` 冲突返回 409「用户名已存在」
- Verification: `go build ./...`

**B5. cmd/quiz/main.go**
- Files: create
- 内容要点：flags `-addr`(:8081) `-data`(./data) `-web`(./web-quiz/dist) `-repo-db`(默认 dataDir/orangerepo.db)；`quizstore.Open`；`EnsureBootstrap`；`quizserver.New`；Listen；启动日志含默认管理员提示；bootstrapDataDir 复用（照抄 main.go 中逻辑，注意 bootstrap 属主修正函数在当前包不可见 → 在 cmd/quiz 内复制小函数或提取）

  > 修正：bootstrapping 逻辑位于 root main.go 包（`bootstrapDataDir`），cmd/quiz 需独立复制该小函数（约 20 行，见 main.go 当前实现）
- Verification: `go run ./cmd/quiz -addr :0` 冒烟（启动即退出 via kill）→ 后续 B6 覆盖

**B6. server_test.go 冒烟**
- Files: create `internal/quizserver/server_test.go`
- 内容要点（httptest + fiber app.Test，仿现有 server_test.go 风格）：临时目录同时开主库（store.Open + 造题）与 quiz；测试流：EnsureBootstrap → admin 登录 → 建学生 → 学生登录 → 建科目/分类（带 tags/types）→ 学生 subjects 列表含题目数 → round 抽题（断言题目集合与随机顺序）→ submit 答对（断言错题集无记录）→ submit 答错（断言记录入集）→ wrong-summary 分组 → wrong-round 练习 → 再答对该题 → 移除（summary 归零）；越权断言（学生访问 /api/admin/* → 403；未登录 → 401）；分类 types 非法 → 400；roundSize 越界 → 400
- Verification: `go test ./internal/quizserver ./internal/quizstore ./internal/store`

### Slice C — 前端 web-quiz

**C1. 脚手架与 UI 基础**
- Files: create `web-quiz/{package.json, vite.config.ts, tsconfig.json, tsconfig.app.json, tsconfig.node.json, index.html, src/main.tsx, src/index.css, src/App.css, src/vite-env.d.ts}`；复制 `web/src/components/ui/*`（button/input/label/dialog/select/switch/badge/tabs/sonner/checkbox/textarea/scroll-area/separator/alert-dialog/tooltip/dropdown-menu/popover 全部，保持原样）；复制 `web/src/lib/{utils.ts, markdown.tsx}` 与 `web/src/types/katex-auto-render.d.ts`（改相应别名 '@' 指向）
- vite.config.ts：插件 react + tailwindcss；`server.proxy: {'/api': 'http://127.0.0.1:8081'}`；`resolve.alias '@' → ./src`
- package.json 依赖与 web/ 完全一致（含 @tanstack/react-query、marked、dompurify、katex、lucide-react、sonner、tailwind v4、shadcn 组件运行库）
- Verification: `cd web-quiz && npm install && npm run build`（此时 src 最小占位）

**C2. 数据层**
- Files: create `web-quiz/src/lib/{types.ts, api.ts}`
- types.ts：`User{id,username,role}`、`Subject{id,name,orderNo,categories:CategoryBrief[]}`、`CategoryBrief{id,name,orderNo,questionCount}`、`QuizProblem{id,type,title,statementMd,bodyJson,hasExplanation}`、`Round{categoryId,total,problems}`、`SubmitResult{correct,correctAnswer,hasExplanation,explanation}`、`WrongGroup{categoryId,categoryName,subjectName,count}`、`AdminCategory{id,name,orderNo,tags,types}`、`Student{id,username,createdAt,wrongCount}`、`Settings{roundSize}`
- api.ts：按规格 §5 全端点；`req` 仿 web/src/lib/api.ts（401 → 派发 `quiz:unauthorized` 事件）；ApiError 类
- Verification: `cd web-quiz && npm run build`

**C3. 应用壳与登录**
- Files: create `src/components/Login.tsx`、`src/App.tsx`
- App.tsx：`api.me()` 门控（仿 web App.tsx）；登录后 `AppStateProvider`（nav: 'quiz'|'wrong'|'mine'|'admin' + 当前科目/分类/轮次上下文）；底部导航栏（固定底栏三个 tab + 管理员第四 tab「管理」仅 role=admin 可见）；Toaster
- Login.tsx：用户名+密码表单，错误 toast
- Verification: `npm run build`

**C4. QuestionCard + 刷题页**
- Files: create `src/components/QuestionCard.tsx`、`src/pages/QuizPage.tsx`
- QuestionCard（刷题/错题共用）：props `{problem, onSubmit(optionIndex|answer) → Promise<SubmitResult>, correct?: boolean}`；渲染题面 markdown、单选 A–D 按钮（提交后锁定、正确项绿色高亮、错误项红色、未答项置灰）、判断题「正确/错误」两按钮同理；反馈横幅 ✓/✗ + 正确答案提示；`hasExplanation` →「查看解析」按钮展开下方 markdown；onSubmit 进行中禁用
- QuizPage：状态机 `chooseSubject → chooseCategory → playing(round) → done(stats)`；choose: 科目列表按钮 + 该科目分类卡片（含题目数，0 题禁用并提示）；playing: 进度「第 x / N 题」+ QuestionCard +「下一题」；done: 本轮答对/答错统计 + 「再练一轮」（重新抽题）与「返回分类」；错误处理：抽题 404/空 → toast
- Verification: `npm run build`

**C5. 错题页**
- Files: create `src/pages/WrongPage.tsx`
- 顶部「全部错题（N）」卡片 + 各分类分组卡片（count）→ 点击 → wrong-round 练习（复用 QuestionCard；错题练习答对时 toast「已从错题集移除」，答完返回后 refetch summary）；未进入练习时展示空态引导
- Verification: `npm run build`

**C6. 我的页 + 系统管理页**
- Files: create `src/pages/MyPage.tsx`、`src/pages/AdminPage.tsx`、`src/components/PasswordDialog.tsx`
- MyPage：用户名/角色展示、错题总数（wrong-summary）、修改密码对话框（old/new 校验、成功后登出重登）、退出登录；管理员显示「系统管理」入口 → nav='admin'
- AdminPage（仅 admin 可达，非 admin 重定向）：Tabs 科目/分类/学生/设置
  - 科目：列表（上下移排序按钮调 subjects/order；改名/删除 AlertDialog）；新增对话框
  - 分类：按科目分组展示（名称/顺序/标签 chips/题型/题目数）；编辑对话框：名称、显示顺序、标签文本输入（逗号/空格分隔）、单选题/判断题 checkbox、实时题目数预览（防抖调 problems-count）；新增/删除/排序
  - 学生：列表（用户名/创建时间/错题数）+ 新增（用户名/密码）+ 重置密码 + 删除
  - 设置：每轮题数 Input（1–100，保存调 settings PUT）
- Verification: `npm run build`

### Slice D — 脚本、文档与全量验证

**D1. scripts/dev-quiz.ps1**
- Files: create（仿 dev.ps1，ASCII-only）：主站 go run .（:8080）+ 主站 vite（:5173）+ `go run ./cmd/quiz`（:8081）+ web-quiz vite（:5174）；Ctrl+C 树杀
- Verification: 手动启动观察日志

**D2. README 与 INDEX 更新**
- Files: modify `README.md`、`docs/aegis/INDEX.md`
- README：功能小节加刷题服务条目；快速开始加 `scripts/dev-quiz.ps1` 或 `go run ./cmd/quiz -addr :8081`；端口表更新（:8080/:5173 主站，:8081/:5174 刷题）；项目结构小节加 cmd/quiz、internal/quizstore、internal/quizserver、web-quiz
- INDEX：追加本计划条目
- Verification: 无（文档）

**D3. 全量验证**
- 命令：`go vet ./...`；`go test ./...`；`cd web ; npm run build`；`cd web-quiz ; npm run build`
- 手动冒烟（可选，若环境允许起服务）：dev-quiz.ps1 起服务 → 主站登录建题（`-seed` 示例题）→ 刷题服务 admin/123456 登录 → 系统管理建科目/分类（映射 示例标签+单选）→ 建学生 → 学生登录刷题 → 答错进错题集 → 答对移除

## 6. 风险与回滚

- **共享 SQLite 只读**：mode=ro 不写主库；风险 = 主库被持有写锁时读超时 → busy_timeout(5000) 兜底；验证 = 冒烟覆盖
- **store 导出改名**：语义零变化，`go test ./internal/store` 兜底；改名为纯重命名不涉及行为
- **前端复制组件**：web-quiz 与 web 组件重复 —— 接受（两应用独立构建树，避免耦合主站构建）
- **回滚面**：全部新增路径不触碰主站既有文件（唯一修改 store.go 为一行改名 + README/INDEX 文档）；有问题可直接删除新增目录/文件

## 7. 退役与基线同步信号

- 无旧逻辑退役；ADR 信号已在规格 §9 记录（共享 SQLite 只读依赖方向、quiz.db 归属）；实施完成后按仓库惯例无需单独 ADR（规格即基线条目）