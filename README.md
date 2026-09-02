# 🍊 OrangeOJ 题库与判题

兼容 [OrangeOJ](https://github.com/shinyes/OrangeOJ) 的题库管理与**判题** Web 应用，由「OrangeRepo 题库 + Orange 刷题」扩展而来：

- **主站**（:8080）：题库管理 —— 斜杠嵌套标签树、题面+答案同屏编辑、训练/练习编制、ZIP 双向导入导出；
- **刷题服务 / OJ**（:8081）：随机刷题 + 错题集（一期），以及**训练/练习布置**与**编程题判题**（本期新增）；
- **判题沙箱 judge-runtime**（:9090）：真正执行学生代码的独立服务，**仅支持 Python 与 C++**（nsjail 隔离）。

实现方式模仿上游 OrangeOJ（判题队列/评测运行器/布置语义均以其源码为基线，差异仅保留 Python+C++、去掉空间/多用户维度）。

## 技术栈

| 层 | 技术 |
|---|---|
| 后端 | Go + Fiber v2 + SQLite（modernc.org/sqlite，无 CGO） |
| 前端 | React 19 + Vite + TypeScript + Tailwind CSS v4 + shadcn/ui |
| 渲染 | marked + DOMPurify + KaTeX（与 OrangeOJ 一致） |
| 判题 | judge-runtime 独立进程（Linux: nsjail + cgroup v2 沙箱；Windows 开发机: 进程级受限运行，无安全承诺） |

## 快速开始

```powershell
# 方式一：开发模式（主站前端 :5173 热更新，后端 :8080）
.\scripts\dev.ps1

# 方式二：开发模式（主站 + 刷题/OJ + 判题沙箱：:5173/:8080、:5174/:8081、:9090）
.\scripts\dev-quiz.ps1

# 方式三：生产模式（Go 直接托管前端构建产物）
cd web ; npm install ; npm run build ; cd ..
go run . -seed          # 首次可加 -seed 导入示例包
```

判题三服务（生产模式）——两个进程 + 一个判题运行器：

```powershell
# 1. 主站先启动至少一次，初始化题库（含账号库）
cd web ; npm install ; npm run build ; cd ..
go run . -seed

# 2. 判题运行器（真实执行代码；本机为受限模式）
$env:ORANGEOJ_JUDGE_SHARED_TOKEN = 'dev-token'
go run ./cmd/judge-runtime

# 3. 刷题/OJ 服务（连接判题运行器）
cd web-quiz ; npm install ; npm run build ; cd ..
go run ./cmd/quiz -judge-endpoint http://127.0.0.1:9090 -judge-token dev-token -judge-workers 2
```

访问 `http://localhost:5173`（主站开发）或 `http://localhost:8080`（主站生产）；
刷题/OJ 见 `http://localhost:5174`（开发）或 `http://localhost:8081`（生产）。
**主站与刷题服务共享同一账号库**：首次启动自动创建管理员 `admin/123456`（主站仅管理员可登录）；
学生账号由管理员在刷题服务「我的 → 系统管理 → 学生」创建。

## Docker 部署

推送 `v*` 版本标签时，GitHub Actions 自动构建主镜像（orangerepo + quiz 双二进制）发布到 GHCR，并创建 Release。
**判题沙箱是独立镜像**（需 nsjail/g++/python3，非 distroless），先本地构建再 compose 一键起三服务：

```bash
docker build -f Dockerfile.judge -t orangeoj-judge:local .
ORANGEOJ_JUDGE_SHARED_TOKEN=换成你的随机token docker compose -f deploy/docker-compose.yml up -d --build
# 主站 http://localhost:8080 · 刷题/OJ http://localhost:8081 · 判题沙箱 :9090（不对外）
```

`orangejudge` 容器以 privileged + cgroup host 运行（nsjail 需要），仅暴露给内网 `orangequiz`；
`orangequiz` 必须配置与 `orangejudge` 一致的 `-judge-token`，否则判题接口返回 503。
主镜像保持 distroless/static；三容器共享 `./data` 卷（判题工作目录在命名卷 `judge-work`）。

## 功能

### 题库管理（主站，沿用一期）
- 三栏布局：标签树筛选（前缀包含 + 多选 AND）/ 题目列表 / 题面·答案·题解同屏编辑
- 三种题型：编程（输入/输出格式、样例、测试点、时限内存）、单选、判断
- 训练（章节化）与练习（题单）编制、题册目录、OrangeOJ 兼容 ZIP 导入导出
- 编程题的 `bodyJson.testCases` 即判题数据：**主站编辑测试点后，已布置任务实时生效**

### 刷题（一期）
- 学生选科目/分类随机抽题作答、答错进错题集、答对自动移除
- 科目/分类/学生账号/每轮题数由管理员在系统管理维护

### OJ 判题与布置（本期新增，模仿 OrangeOJ）
- **布置体系**：管理员在「系统管理 → 布置」中选择主库已有**训练/练习**，布置对象可为**全体学生或定向学生**，
  支持**发布/撤回/删除/编辑学生**；学生端新增**「训练」「练习」页签**（无题库浏览），只显示布置给本人且已发布的任务
- **训练详情**：章节结构 + 每章题目网格（AC 绿标/当前题高亮），训练/练习共用**做题页**（带返回上下文）
- **做题页**：编程题 = 题面（样例可复制/填入）+ 语言选择（Python 3 / C++11）+ 代码编辑（草稿本地保存）+
  **运行**（自定义输入，不比对）/ **测试**（跑题面样例与测试点）/ **提交**（正式评测）+ 轮询判题 + 控制台 +
  **测评记录**（逐条历史，展开看代码/输入/输出/期望/错误与每个测试点）；客观题 = 单选/判断即点即判并高亮正确答案
- **评测结论**：AC / WA / CE / RE / TLE / MLE / OK（run），逐测试点明细与耗时；空白容忍比对
- **进度与统计**：每题首次 AC 后标完成（个人）；管理端任务详情可看**每题通过人数与提交数**（不含代码）
- **判题内核**：队列（judge_jobs）+ 提交（submissions）+ 进度（user_problem_progress）表结构与
  编排语义照搬上游 OrangeOJ `internal/judge`；评测在 judge-runtime 内逐用例独立进程运行

## API 概览

主站（管理员会话，完整契约见 `docs/aegis/specs/2026-08-22-orangerepo-design.md` §5 与 `docs/api-reference.md`）：

```
POST /api/auth/login|logout   GET /api/auth/me      PUT /api/auth/password
GET/POST /api/problems        GET/PUT/DELETE /api/problems/:id
PUT  /api/problems/:id/solutions
GET  /api/tags[?q&tags&type]  PATCH/DELETE /api/tags   GET/PUT /api/tag-order
GET/POST /api/booklet-directories   PUT /api/booklet-directories/layout   PATCH/DELETE /api/booklet-directories/:id
POST /api/images              GET /api/uploads/*
POST /api/import?mode=…       GET  /api/export/problems | trainings/:id | practices/:id
CRUD /api/trainings · chapters · items · /folder ； CRUD /api/practices · practice-items · /folder
```

刷题服务（一期，`docs/aegis/specs/2026-08-29-quiz-service-design.md` §5）：

```
POST /api/auth/login|logout   GET /api/auth/me      PUT /api/auth/password
GET /api/quiz/subjects        POST /api/quiz/round | submit | wrong-round      GET /api/quiz/wrong-summary
GET/POST /api/admin/subjects · categories · students · settings …
```

**OJ 判题与布置（本期新增，契约见 `docs/aegis/specs/2026-09-03-orangeroj-design.md` §5）**：

```
学生端：GET  /api/oj/assigned           训练/练习任务列表（可见性 = 发布 + 全体/定向）
        GET  /api/oj/training|practice/:id      任务详情（章节/题单 + 完成态）
        GET  /api/oj/problem/:id                题目正文（测试点/答案/题解永不下发）
        POST /api/oj/problem/:id/run|test|submit      {language, sourceCode[, inputData]} → submissionId
        POST /api/oj/problem/:id/objective-submit     {answer} → 同步判定（客观题）
        GET  /api/oj/submission/:id/poll             轮询结果     GET /api/oj/problem/:id/submissions 历史
管理端：GET  /api/admin/repo-trainings|practices[/:id]  主库目录浏览
        CRUD /api/admin/assignments[/:id]               布置（发布/撤回/删除）
        PUT  /api/admin/assignments/:id/students        定向学生     GET …/students
        GET  /api/admin/assignments/:id/stats           每题通过人数/提交数
judge-runtime：POST /internal/judge/execute（X-Judge-Token）  GET /healthz
```

## 项目结构

```
main.go                  入口（-addr / -data / -seed）—— 主站
cmd/quiz/                刷题/OJ 服务入口（-addr / -judge-endpoint / -judge-token / -judge-workers）
cmd/judge-runtime/       judge-runtime 入口（环境变量配置，:9090）
internal/model           数据模型与 JSON 形状
internal/store           SQLite 迁移与查询（主库 orangerepo.db）
internal/accounts        共享账号库（users/sessions，主站与刷题服务统一账号唯一 owner）
internal/quizstore       刷题数据层：quiz.db（科目/错题/提交/队列/进度/布置）+ 主库只读 reader
internal/quizserver      刷题 Fiber 路由与 OJ API（/api/quiz /api/oj /api/admin/assignments）
internal/judge           判题编排（队列/HTTPRunner/类型），迁移自上游 queue.go/runner.go
internal/judgeserver     评测执行器（Python/C++）+ 沙箱后端（Linux nsjail / 开发受限运行）+ HTTP 服务
internal/zipio           OrangeOJ ZIP 兼容层
internal/server          主站 Fiber 路由
web/                     React 前端（主站）
web-quiz/                React 前端（刷题/OJ）
samples/                 示例题包
Dockerfile               主镜像（orangerepo + quiz 双二进制，distroless）
Dockerfile.judge         判题沙箱镜像（ubuntu + nsjail + g++ + python3）
deploy/docker-compose.yml  三服务部署
docs/aegis/              设计规格、实施计划与治理文档
```

## 开发验证

```powershell
go vet ./... ; go test ./...        # 后端全量（含 judge 真实 Python/C++ 评测冒烟）
GOOS=linux CGO_ENABLED=0 go build ./...   # 容器侧交叉编译（含 judge-runtime）
cd web ; npm run build              # 主站前端
cd web-quiz ; npm run build         # 刷题/OJ 前端
```

### 判题安全须知
- 学生代码只在 **judge-runtime** 内执行：Linux 生产为 nsjail（无网络、无 proc、降权 nobody、cgroup 内存/PID 限制）；
  Windows/本地开发为进程级受限运行（限时/隔离目录/精简环境），**无安全隔离承诺**，仅用于联调
- judge-runtime 与刷题服务之间以共享 token（`ORANGEOJ_JUDGE_SHARED_TOKEN`）认证；生产务必更换默认值
- 评测结果仅记录：逐测试点 verdict/耗时/输出/错误与提交历史；题面测试点与答案永不下发学生
- 判题队列空闲轮询 400ms/认领失败退避 800ms，worker 数由 `-judge-workers` 控制（默认 2）
