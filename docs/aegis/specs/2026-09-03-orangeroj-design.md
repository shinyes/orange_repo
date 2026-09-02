# OrangeOJ（判题扩展）设计规格

日期：2026-09-03 · 状态：已确认（用户逐项拍板：独立 judge-runtime 进程、无题库浏览仅布置驱动、run/test/submit 全做、接受 Docker 三服务改造、布置模仿 OrangeOJ 空间语义 = 刷题服务内布置给全体学生/指定学生、整份训练/练习布置且做题页内联全部题型、可见性 = 学生历史 + 管理端汇总）
前置规格：`docs/aegis/specs/2026-08-29-quiz-service-design.md`（刷题服务一期）、`docs/aegis/specs/2026-08-22-orangerepo-design.md`（主站题库格式）
上游实现基线：https://github.com/shinyes/OrangeOJ main 分支（2026-09-03 抓取源码快照）`backend/internal/judge/queue.go`、`backend/internal/judge/runner.go`、`backend/internal/judgeserver/executor.go`、`backend/internal/judgeserver/server.go`、`backend/internal/api/submission_handlers.go`、`backend/internal/api/training_handlers.go`、`backend/internal/api/practice_handlers.go`、`backend/internal/db/db.go`、`backend/cmd/judge-runtime/main.go`、`Dockerfile.judge`、`frontend/src/pages/CodingPage.jsx`

## 0. 本规格如何“模仿 OrangeOJ”

OrangeOJ 的判题运行时事实（已读源码，逐条映射到本仓库）：

| 上游事实（源码位置） | 本仓库落地 |
|---|---|
| 队列表 `judge_jobs(status queued/running/done/failed, priority, available_at, worker_token)` + N 个 worker 轮询认领（queue.go） | quiz.db 同构迁移 + `internal/judge` 队列服务（worker 数 = 进程 flag） |
| 提交表 `submissions(user_id,space_id,problem_id,question_type,language,source_code,input_data,submit_type,status,verdict,time_ms,memory_kib,score,stdout,stderr,case_details_json,finished_at)`（db.go L251） | 同构迁移（去 space_id，problem_id 指向主库 id 不做 FK；`question_type` 保留） |
| 运行器 HTTPRunner → judge-runtime `POST /internal/judge/execute`（X-Judge-Token），任务体 `{submissionId,language,sourceCode,timeLimitMs,memoryLimitMiB,checkAnswer,compileTimeoutS,cases:[{input,expected}]}`（runner.go） | `internal/judge` 原样迁移 |
| submit_type = run/test/submit；run 用 `inputData` 单样例、test 优先 `testCases` 回退 `samples`、submit 同 test 且必须全 AC 才计 100（queue.go processJob） | 原样迁移；**取消 Python 之外语言的 go/turtle（本需求仅 Python+C++）**；时间/内存记录以 exec 实测与 OOM/TLE 判据为准 |
| 评测结果 verdict：PENDING/OK/AC/WA/CE/RE/TLE/MLE；`NormalizeOutput`（\r\n→\n、去行尾空白、整体 TrimSpace）比对（runner.go/executor.go） | 原样迁移（含 case_details） |
| 学生进度 `user_problem_progress(space_id,user_id,problem_id,best_verdict,best_score,last_submission_id)`（db.go L288） | 同构迁移（去 space_id）；best_verdict/best_score 以 best_score 优先更新（AC=100/WA=0） |
| 训练 `training_plans(space_id,title,description,tags_json,published_at)` + `training_participants(plan_id,user_id)`；题目正文在 space_problems（空间内独立问题库） | **内容源直接为主库训练/练习**（orange_repo.db 的 trainings/training_chapters/training_items/practices/practice_items）：布置时登记 (kind, repo_id)，**结构实时跟随主库**（与上游同库同构——上游训练/练习与题目在同一库、成员所见即当前结构）；题目正文仍动态读主库 |
| 练习 `practices(space_id,title,description,due_at,display_mode,published)` + `practice_targets(practice_id,user_id)`；targets 非空时仅目标学生可见 | `assignments` + `assigned_students`（assigned_all=1 全员 / 定向列表） |
| 权限：published+在 participants/targets 内的成员可见（training loadTrainingPlanAccess / practice loadPracticeAccess） | 学生端仅列出“已布置给自己且已发布”的任务；管理员全量管理 |
| 学生做题视图 CodingPage：run/test/submit 三动作 + 测评记录（轮询 submission/stream）+ 客观题内联作答（objective-submit 即答即判，AC=100/WA=0） | web-quiz 复刻（移动端优先，无 Monaco——用 textarea；无 turtle/go） |
| 客观题判定：单选 answerIndex、判断 answer 布尔（evaluateObjectiveAnswer/submission_handlers.go） | 复用一期 quizserver 判题逻辑，但记录 submission 与进度（best_score 参与 practice 汇总） |

## 1. 目标与边界

在 orange_repo 仓库内把 **Orange 刷题**（一期刷题服务 :8081 + web-quiz）扩展为支持编程题判题的 **OrangeOJ**：

- 新增**独立 judge-runtime 沙箱评测服务**（:9090，Linux 上 nsjail 隔离，仅支持 **Python 与 C++** 两个语言），经共享 token 只接受刷题服务的评测请求
- 刷题服务内新增判题队列（judge_jobs + worker）、提交存储（submissions）、**run（自定义输入运行）/ test（按题面测试用例自测）/ submit（正式提交评测）** 三动作 API（对题单中出现的编程题开放，模仿上游 spaceRead 只读权限）
- 学生进度（user_problem_progress）：每道题首次 AC 即标记完成
- **布置体系（模仿 OrangeOJ 训练/练习）**：管理员在刷题服务「系统管理」中从**主站已有训练（章节）/练习（题单）**做布置：
  - 布置对象可选全体学生或指定学生（目标学生维护，模仿 training_participants 全员可加 / practice_targets 定向）
  - 支持发布 / 撤回（隐藏不删记录）/ 删除布置
  - 学生端新增**「训练」与「练习」两个页签**（无题库浏览）：仅显示布置给本人且已发布的训练/练习；训练按章节、练习按题单顺序进入做题
  - 做题页对编程题提供 运行/测试/提交 + 测评记录，对单选/判断内联作答并即时反馈（AC=100/WA=0，与客观题 submit 一致）
- 管理端每题统计：被布置任务内每题通过人数/提交数（不展示学生代码）

**非目标（本期）**：题库浏览/自由刷编程题、学生代码在管理端可见、go/turtle 语言、题面测试用例增量编辑、训练多选作答、练习"整份提交/考试卷"模式（practice_records 结构留待二期）、多空间、比赛、Special Judge。

## 2. 架构

```
┌─────────────┐  :8080 主站（现有，不变）—— 题库编辑/训练/练习编制（写入 orangerepo.db）
└─────────────┘
┌─────────────┐  :8081 刷题服务（扩展）—— 一期刷题 + 布置/提交/评测队列（quiz.db 自有数据 + 只读 orangerepo.db）
│  quiz.db    │      submissions/judge_jobs/user_problem_progress/assignments/assigned_students
└─────────────┘
       │ HTTP POST /internal/judge/execute（X-Judge-Token）
┌─────────────┐  :9090 judge-runtime（新增，独立进程/容器）—— 唯一真正执行用户代码之处
│  nsjail     │      Linux: nsjail + g++/python3；Windows/开发机: 进程级受限运行（无 nsjail）
└─────────────┘
```

- 判题编排（队列/轮询/结果写回）位于刷题服务进程（模仿上游 main.go 内嵌 QueueService）
- judge-runtime 无状态、不落盘数据库；任务目录临时（`MkdirTemp` + defer RemoveAll，模仿 executor.go）
- 数据流完全照搬上游：提交事务写 submissions(queued) + judge_jobs(queued, priority=0) → worker `claimJob` 原子认领（RETURNING）→ runner 调 judge-runtime → 写回 submissions(done) + user_problem_progress + judge_jobs(done)；异常 → submissions(failed, RE)

### 代码布局（新增/修改，沿用现有风格）

```
cmd/judge-runtime/main.go       judge-runtime 入口（:9090，环境变量配置，见 §7.2）
cmd/quiz/main.go                [改] 增加 -judge-endpoint/-judge-token/-judge-workers；启动队列
internal/judge/queue.go         [新] 队列服务（claim/process/写回/失败兜底，迁移自上游）
internal/judge/runner.go        [新] Runner 接口 + HTTPRunner（迁移自上游）
internal/judge/queue_test.go    [新] 队列单测（用例选择/写回/progress upsert）
internal/judgeserver/executor.go [新] 评测执行器：语言命令装配 + 每用例运行 + verdict 判定
internal/judgeserver/sandbox_windows.go  [新] Windows/开发受限运行后端（无 nsjail）
internal/judgeserver/sandbox_linux.go    [新] Linux nsjail 后端（生产）
internal/judgeserver/server.go  [新] HTTP server（/healthz /internal/judge/execute，token 校验）
internal/judgeserver/executor_test.go    [新] 真实 Python/C++ 评测冒烟（本机可跑）
internal/quizstore/quizstore.go [改] migrate 追加 §3 新表
internal/quizstore/problems.go  [改] 只读 reader 增加题目详情/用例/答案读取与训练/练习结构读取（repo_oj.go）
internal/quizstore/submissions.go [新] submissions/judge_jobs/progress 写与查（quiz.db 侧）
internal/quizstore/assignments.go [新] 布置 CRUD/定向/统计
internal/quizserver/server.go   [改] 挂载新路由、注入 runner/queue
internal/quizserver/assign.go   [新] 管理员布置 API + 学生端任务列表/详情 API
internal/quizserver/judge.go    [新] 学生提交 API（run/test/submit/objective-submit/历史）
web-quiz/src/...                [改] 新页签/路由/页面（见 §6）
deploy/docker-compose.yml       [改] 三服务（orangerepo/orangequiz/orangejudge）
Dockerfile                      [改] 主镜像不变 + Dockerfile.judge（ubuntu24.04 + nsjail + g++ + python3）
scripts/dev-quiz.ps1            [改] 增加 judge-runtime :9090 进程（本机 nsjail 缺失时降级受限运行）
```

## 3. 数据模型（quiz.db 增量迁移，全部 `CREATE TABLE IF NOT EXISTS` 幂等）

沿用上游字段命名（snake_case 存储、JSON camelCase）。**problem_id 均为主库题目 id（int），不建外键**
（主库在另一文件且只读；主库删除的题目在读取时跳过）。

```sql
submissions(id PK, user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  problem_id INTEGER NOT NULL, question_type TEXT NOT NULL,      -- programming/single_choice/true_false
  language TEXT NOT NULL DEFAULT '', source_code TEXT NOT NULL DEFAULT '',
  input_data TEXT NOT NULL DEFAULT '', submit_type TEXT NOT NULL, -- run/test/submit/objective
  status TEXT NOT NULL DEFAULT 'queued', verdict TEXT NOT NULL DEFAULT 'PENDING',
  time_ms INTEGER NOT NULL DEFAULT 0, memory_kib INTEGER NOT NULL DEFAULT 0,
  score INTEGER NOT NULL DEFAULT 0, stdout TEXT NOT NULL DEFAULT '', stderr TEXT NOT NULL DEFAULT '',
  case_details_json TEXT NOT NULL DEFAULT '',                     -- [{"caseNo","verdict","input","output","expectedOutput","error","timeMs","memoryKiB"}]
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, finished_at DATETIME)
judge_jobs(id PK, submission_id INTEGER NOT NULL UNIQUE REFERENCES submissions(id) ON DELETE CASCADE,
  status TEXT NOT NULL, priority INTEGER NOT NULL DEFAULT 0,
  available_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, started_at DATETIME, finished_at DATETIME, worker_token TEXT)
-- index: judge_jobs(status, priority DESC, id ASC)
user_problem_progress(user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  problem_id INTEGER NOT NULL, best_verdict TEXT NOT NULL, best_score INTEGER NOT NULL DEFAULT 0,
  last_submission_id INTEGER NOT NULL, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY(user_id, problem_id), FOREIGN KEY(last_submission_id) REFERENCES submissions(id) ON DELETE CASCADE)

assignments(id PK, kind TEXT NOT NULL CHECK(kind IN ('training','practice')),
  repo_id INTEGER NOT NULL,          -- 主库 trainings.id / practices.id（引用，非快照）
  title TEXT NOT NULL, description TEXT NOT NULL DEFAULT '', tags_json TEXT NOT NULL DEFAULT '[]',
  published INTEGER NOT NULL DEFAULT 1,  -- 0 = 撤回（学生端隐藏，记录保留）
  assigned_all INTEGER NOT NULL DEFAULT 1, -- 1 = 全体学生；0 = 按 assigned_students 定向
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(kind, repo_id))
assigned_students(assignment_id INTEGER NOT NULL REFERENCES assignments(id) ON DELETE CASCADE,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, PRIMARY KEY(assignment_id, user_id))
```

> 注：训练/练习的章节与条目结构**不复制**——做题列表实时读主库
> （`trainings/training_chapters/training_items`、`practices/practice_items`，按 order_no,id 排序）。
> 与上游同构：上游训练/练习与题目同库，成员看到的即库内当前结构；主库结构/题目被改或删除时，
> 学生端列表与做题页实时反映（题目被删 → 条目自动消失）。每题的 completed 直接来自
> user_problem_progress JOIN 主库实时结构。**无 practice_records/assignment_train_*/assignment_practice_items
> 快照表**（实施期已按「实时跟随」决策删除）。

**客观题 submit 也写入 submissions**（submit_type='objective'，status='done'，score=100/0，finished_at 立即）与 user_problem_progress（模仿上游 objective-submit 的 handleObjectiveSubmit）。

## 4. 语义

### 4.1 布置可见性与发布
- `assignments.assigned_all=1` → 全部学生可见；`=0` → 仅 `assigned_students` 中的学生可见（模仿 practice_targets：has_targets=1 时仅目标可见）
- 学生端列表与详情均要求 `published=1` 且（assigned_all 或本人 in assigned_students）；管理员无视两者
- 删除布置 → 级联 assigned_students（ON DELETE CASCADE）；学生历史提交不受影响（submissions 独立）

### 4.2 题目集合与实时跟随
- 训练布置：主库 `trainings/:id` → 章节（training_chapters ORDER BY order_no,id）→ 条目题目（training_items ORDER BY order_no,id）；**实时读取**，不落快照
- 练习布置：主库 `practices/:id` 条目题目 id 有序列表；**实时读取**
- 主库训练/练习被删除：布置记录仍在但学生端列表显示「内容已失效」并给出空态提示（assignment 保留便于管理端处理；服务端读取失败/空章节视为 0 题）
- 题目正文永不复制：做题时按主库 problem id 实时读取（type/title/statementMd/bodyJson（编程题含 inputFormat/outputFormat/samples/testCases + starterCode 若存在）/answerJson（判题用，不下发）/timeLimitMs/memoryLimitMiB）

### 4.3 判题语义（run/test/submit）
- `run`：body `{language, sourceCode, inputData}`；单样例运行；**checkAnswer=false** → verdict OK（上游同）；stdout/stderr 记录（每条 <=8KB，总 <=12KB 截断——executor.go trimTo 语义）；不写 progress
- `test`：body `{language, sourceCode}`；用例 = bodyJson.testCases（无则 samples，再无则 inputData 空运行），checkAnswer=true；全过 → AC(score 100) 否则首个失败 verdict；**写 progress**（上游 queue.go L249：test/submit 都更新 progress；CodingPage 测试后刷新完成态）
- `submit`：同 test 用例来源；写 progress
- 判题以**首败即停**（上游 break 语义），但保留已跑用例的 caseDetails
- verdict 判定优先级：compile timeout → CE；进程 timeout → TLE；内存超限 → MLE；非零退出 → RE；输出比对（NormalizeOutput）不符 → WA；全过 → AC（checkAnswer=false 时为 OK）
- 编程题 language 白名单：`python`/`python3`/`py` → python（解释执行 main.py）；`cpp`/`c++`/`c` → cpp（`g++ -std=c++11 -O2 main.cpp -o main.out`）；语言归一化与主库题解语言约定一致；**不提供 go/turtle**
- 时限：题面 timeLimitMs（缺省 1000）；编译超时 10s；单用例运行超时 = timeLimitMS+250ms context 上限 + nsjail ceil 到整秒（Windows 后端用 10ms 精度定时杀）
- 内存：memoryLimitMiB（缺省 256，下限 32）；Linux nsjail `--cgroup_mem_max (limit+32)MiB`（+32 容忍编译器/解释器开销，上游语义）；Windows 后端无 cgroup → 按退出码/错误文本启发 + 尽力记录（非生产）
- **输出记录**：stdout 每用例独立捕获并即时判定（不拼接后判），console 用 stdout；run 模式 stdout 全文展示

### 4.4 结果可见性（管理端每题统计）
管理端任务详情提供每题统计（不含代码）：
```
GET /api/admin/assignments/:id/stats
→ {problems: [{problemId, title, type, accepted: 通过人数（best_verdict=AC 且属于本任务学生集）, submissions: 提交次数（submit 类型，含 run/test?——只统计 submit+objective）}]}
```
- 训练任务统计的“学生集” = assigned_all ? 全体学生（无 assigned_students 行时以 users role=student 现算） : assigned_students
- 编程题只计 submit；客观题计 objective-submit

### 4.5 进度与完成
- `user_problem_progress` 以 best_score 为权威（新 score >= 旧 score 才覆盖 best_verdict/best_score；客观题 AC=100 覆盖编程题 0 分前的记录等——上游 DO UPDATE CASE 语义）
- 学生端训练/练习列表展示每题完成态：客观题 = 曾 AC（best_verdict=AC）；编程题 = submit 曾 AC
- 学生提交历史：submissions WHERE user_id=? AND problem_id=? ORDER BY id DESC LIMIT 50（含 case_details 供逐点查看）

## 5. API 契约（刷题服务 /api 增量，全部 requireSession；管理员区 requireAdmin）

### 5.1 学生端（做题）

```
GET /api/oj/assigned          学生视角任务列表（训练/练习分组合并，仅 published 且可见）
  → {trainings:[{id,title,description,tags,publishedAt,problemCount,acceptedCount}], practices:[同练习]}
  - problemCount 来自主库实时结构（失效条目除外），acceptedCount = 本人 AC 数
GET /api/oj/training/:id      → 详情（学生可见性校验）
  → {id,title,description,tags,chapters:[{id,title,items:[{problemId,orderNo,title,type,completed}]}],progress:{accepted,total}}
GET /api/oj/practice/:id      → 详情
  → {id,title,description,tags,dueAt?,items:[{problemId,orderNo,title,type,completed}],...}

GET /api/oj/problem/:id       题目正文（可见性 = 属于本人可见任务；无任务引用则 404）
  → {id,type,title,statementMd,bodyJson:{inputFormat,outputFormat,samples,starterCode?|options?},timeLimitMs,memoryLimitMiB}
  - 编程题不带 testCases（判题在服务端）；客观题不带 answerJson（同上）；题解永不下发学生
POST /api/oj/problem/:id/run     {language,sourceCode,inputData} → {submissionId}
POST /api/oj/problem/:id/test    {language,sourceCode}           → {submissionId}
POST /api/oj/problem/:id/submit  {language,sourceCode}           → {submissionId}
POST /api/oj/problem/:id/objective-submit {answer: string|number|bool} → {submissionId,verdict,score}
  - objective 同步返回；编程题三动作异步返回 {submissionId,status:'queued'}
GET  /api/oj/submission/:id/poll → {submissionId,status,verdict,score,timeMs,memoryKiB,stdout,stderr}（非本人 404）
GET  /api/oj/problem/:id/submissions → 本人最近 50 条（含 caseDetails 精简字段）
```

### 5.2 管理端（布置；前缀 /api/admin/assignments）

```
GET /api/admin/repo-trainings     主库训练目录（只读，含章节题数与总题数）：[{id,title,tags,chapterCount,problemCount}]
GET /api/admin/repo-practices     主库练习目录：[{id,title,tags,problemCount}]
GET /api/admin/repo-trainings/:id 单训练结构预览（含章节，供布置确认）
GET /api/admin/repo-practices/:id
POST /api/admin/assignments {kind,repoId,title?,description?,assignedAll,published?,studentIds:[]} → {id}
     - title 缺省 = 主库原标题；kind+repoId 已布置 → 409
GET /api/admin/assignments        [{id,kind,repoId,title,assignedAll,published,problemCount,createdAt,studentCount}]
PATCH /api/admin/assignments/:id  {title?,description?,published?}（发布/撤回）
PUT  /api/admin/assignments/:id/students {studentIds:[...]}   // 全体模式置 assignedAll=1 并清空定向；定向模式置 0 并存列表
DELETE /api/admin/assignments/:id → 204
GET /api/admin/assignments/:id/students → {assignedAll, students:[{id,username}]}
GET /api/admin/assignments/:id/stats → §4.4
```

校验：kind ∈ {training,practice}；repoId 为正且对应主库实体存在；title trim 非空（缺省主库名）；studentIds 必须均为 student 角色。

## 6. 前端 UX（web-quiz）

- 底部导航 **刷题 / 训练 / 练习 / 错题 / 我的**（路由 /quiz、/training、/practice、/wrong、/mine；管理入口仍在「我的」→ /admin）
- **训练页 /training**：布置给我的训练卡片列表（标题/描述/进度 x/y、已发布）；空态引导联系管理员
- **练习页 /practice**：同上（练习卡片）
- **训练详情 /training/:id**：章节手风琴（展开当前/未全完成的章）＋ 章节网格导航（数字按钮、AC 绿标、当前题高亮）＋ 上一题/下一题
- **练习详情 /practice/:id**：题单列表（题号/标题/题型徽标/完成态）
- **做题页 /problem/:problemId?back=...**（训练与练习共用，带上下文返回）：
  - 客观题：单选/判断选项 +「提交答案」→ objective-submit 同步判定 ✓/✗、正确项高亮、展示解析按钮（沿用一期 QuestionCard 交互但独立页）
  - 编程题：左 = 题面（Markdown/KaTeX、输入/输出格式、样例输入输出带复制、时间/内存限制、`测试`按钮把样例填入自定义输入直接测？——首版仅展示 + 复制）；右 = 语言选择（C++/Python3）＋ 代码编辑（textarea + 行号感样式；草稿本地 localStorage 每语言分存）＋ 动作栏「运行 / 测试 / 提交」
  - 「运行」：弹自定义输入对话框 → 结果写控制台（stdout + 退出码/错误）
  - 「测试」：直接对题面用例评测 → 控制台显示 通过/未通过（失败展示首个失败用例输入/期望/输出）
  - 「提交」：正式评测 → 轮询 submission/poll 至 done；页面顶部 verdict 横幅 + 展开「提交记录」抽屉
  - 「测评记录」抽屉：列表（#id 时间 verdict 耗时/内存 测试点 x/y）→ 选中展开：代码/输入/输出/期望输出/错误 tab（复刻上游 Tabs 交互，测试点下拉）
  - 页面底栏训练/练习上下文导航
- **管理端（/admin 新增 Tab「布置」）**：分区 Tab（训练/练习），步骤式对话框：
  1. 选来源：GET repo-trainings|repo-practices 下拉 + 预览（章节/题数）
  2. 布置对象：全体学生单选 或 定向选择（搜索学生 → 候选 chips 添加/移除）
  3. 发布开关 + 创建 → 列表显示（标题/题型/对象/状态/题数/学生数/创建时间）＋ 操作：发布/撤回、编辑学生、查看统计、删除
  - 统计对话框：每题 通过人数 / 提交次数
- 移动端优先（沿用现有样式体系）；所有新页面在窄屏可操作（编辑器 min-h、横向滚动 pre）

## 7. 安全与部署

### 7.1 判题安全
- judge-runtime 与刷题服务之间共享 token（环境变量/flag），请求必须带 `X-Judge-Token`；token 不匹配 401
- 请求体限 4MB（上游 server.go）；源代码在服务端持久化前不限制（库内 TEXT，前端 maxlength 建议 64KB）；输出/错误截断（8KB/条、12KB/总——上游 trimTo）
- Linux 生产 = nsjail（`--mode o`、无网络接口、无 proc、降权 nobody 65534、bindmount 任务目录、cgroup v2 mem/pids、PATH 白名单）——完整复刻上游 executor.go 参数；仅 python3/g++ 工具链进镜像
- Windows/开发机（无 nsjail/cgroup）= 受限运行后端：仍逐用例限时（进程树 Kill）、工作目录隔离、环境变量最小化、stdout/stderr 截断；**不做安全隔离承诺**（开发便利），生产必须在 Linux judge 容器
- 编译/解释产物只读任务临时目录，defer RemoveAll；任务目录命名 `sub-<id>-*`
- judge-runtime 进程以非特权容器运行（compose 中 privileged 仅给该容器 + cgroup host + 必要 cap，同上游 docker-compose.build.yml）

### 7.2 judge-runtime 配置（环境变量，仿上游 cmd/judge-runtime）
```
ORANGEOJ_JUDGE_RUNTIME_PORT（默认 9090）
ORANGEOJ_JUDGE_SHARED_TOKEN（必填）
ORANGEOJ_JUDGE_WORKDIR（默认 /work/jobs）
ORANGEOJ_JUDGE_COMPILE_TIMEOUT_SEC（默认 10）
ORANGEOJ_JUDGE_READ_TIMEOUT_SEC（默认 15） / WRITE_TIMEOUT_SEC（默认 300）
```
刷题服务侧 flags：`-judge-endpoint http://judge-runtime:9090`、`-judge-token`（默认空=禁用队列，判题请求 503）、`-judge-workers`（默认 2）。

### 7.3 Docker/CI
- 新增 `Dockerfile.judge`：build stage golang:1.25-alpine 交叉编译 cmd/judge-runtime；runtime ubuntu:24.04 + build-essential(g++) + python3 + nsjail（编译自源码，复刻上游 Dockerfile.judge）＋ judge-runtime；EXPOSE 9090
- 主 Dockerfile 维持 distroless 单镜像（orangerepo + quiz）；release.yml 不变（judge 镜像为独立 Dockerfile，由新 workflow 或 compose 构建指引）——**本期 compose 引用本地构建镜像 orangeoj-judge:local（同上游 docker-compose.build.yml），不自动推送 GHCR**（保持 release 流程不扩面，文档写明）
- deploy/docker-compose.yml：加 orangejudge 服务（build: Dockerfile.judge、privileged、cgroup host、cap_add SYS_ADMIN/SYS_RESOURCE/SYS_PTRACE、security_opt unconfined、volumes /sys/fs/cgroup、tmpfs /tmp、healthcheck /healthz）；orangequiz 增 `ORANGEOJ_JUDGE_ENDPOINT`/token/workers 环境
- scripts/dev-quiz.ps1：增加 judge-runtime 进程（token=dev 固定；无 nsjail 时日志提示降级）；Dev 模式 Windows 后端不需要 docker

## 8. 测试策略

- `internal/judge`：单元测试（用例选择 run/test/submit、写回 verdict/score、progress upsert 语义、失败兜底 RE）
- `internal/judgeserver`：**真实评测**冒烟（Windows 受限后端可跑）：Python AC 程序、C++ AC 程序（g++ 不可用则 t.Skip）、WA（NormalizeOutput 比对）、RE（panic/exit 1）、TLE（sleep 超时）、CE（语法错误）；HTTP server token 校验单测
- `internal/quizstore`：迁移幂等；submissions/judge_jobs 事务写；assignments 级联/定向语义单测（fake 主库经 store.Open 造数据）
- `internal/quizserver`：httptest 冒烟（admin 登录 → 建学生 → 主库造训练/练习 → 布置全体/定向 → 学生登录列表可见性 → objective-submit 同步判定 → 编程 run/test/submit 队列（runner=mock 或本机 executor 注入）→ poll → 提交历史 → 进度 → 管理统计；越权 403/404）
- 前端：`tsc -b && vite build`
- 全量：`go vet ./... ; go test ./...`；双 npm build；`GOOS=linux CGO_ENABLED=0 go build ./...`（含 cmd/judge-runtime）
- E2E（本机）：三进程起服（judge-runtime Windows 受限后端 + 刷题 :8081 + 主站 :8080 造数据）→ 真实 C++/Python 提交 → 布置 → 学生答题 → 管理统计核对

## 9. 兼容边界与文档
- 主站零改动（题库编辑/导出不变；仅 data 目录同时被刷题服务继续只读）
- 刷题服务一期路由/行为零改动（/api/quiz、/api/admin 现有端点不动；新增 /api/oj 与 /api/admin/assignments）
- 主库表结构不变；quiz.db 仅追加表（`CREATE TABLE IF NOT EXISTS` + 索引），旧库平滑升级
- README（判题功能/三服务快速开始/限制说明：仅 Python+C++、判题在 judge 容器内执行）、docs/aegis/INDEX.md、BASELINE-GOVERNANCE.md 不变式 3 更新（原“非 OJ 功能永不进入范围”基线被本次用户决策推翻 → 修订基线条目并记录修订理由）
- 上游复刻点对照表（§0）与「格式基线」文档保留：internal/judge 与 internal/judgeserver 的注释头部注明来源文件

## 10. 验收
1. `go vet ./...`、`go test ./...` 全绿（含真实 Python/C++ 评测用例，本机 g++/python3 路径）
2. `GOOS=linux CGO_ENABLED=0 go build ./...` 通过（含 cmd/judge-runtime 交叉编译）
3. web + web-quiz `npm run build` 通过
4. 本地 E2E：主库造编程+客观题训练/练习 → 布置 → 学生三动作真实判题（AC/WA/RE/TLE/CE 可见）→ 训练章节完成态 → 管理统计与提交历史核对
5. Docker 相关文件语法/文档就绪（无 docker 环境则以交叉编译 + compose 语法核对替代）
