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

## 快速开始（Docker Compose 部署）

### 前置要求
- 安装 Docker Engine（含 Compose v2）
- Linux 宿主机需支持 **cgroup v2**（判题沙箱 nsjail 需要，容器会以 privileged 运行）
- 数据保存在 compose 文件所在目录的 `./data`（删除即可重置）

### 部署步骤

```bash
# 1. 准备 compose 文件（可来自仓库 deploy/ 目录或直接下载）
git clone https://github.com/shinyes/orange_repo.git
cd orange_repo

# 2. 设置判题共享 token（生产务必用随机长字符串；主站/刷题/判题三容器共用同一份 compose 自动注入）
export ORANGEOJ_JUDGE_SHARED_TOKEN='换成你的随机token'

# 3. 一键启动（自动从 GHCR 拉取主镜像与判题沙箱镜像）
docker compose -f deploy/docker-compose.yml up -d
```

访问：
- 主站（题库管理）：http://localhost:8080
- 刷题 / OJ（学生做题、判题）：http://localhost:8081
- 判题沙箱 :9090 仅容器内网使用，不对外

首次启动自动创建管理员 `admin / 123456`（主站与刷题服务共享账号库，改密两端联动），请登录后修改。

### 说明
- 主镜像 `ghcr.io/shinyes/orange_repo:<版本>` 与判题沙箱镜像 `ghcr.io/shinyes/orange_repo-judge:<版本>` 均由 GitHub Actions 随版本标签自动发布，无需本地构建；如需覆盖判题镜像可设 `ORANGEOJ_JUDGE_IMAGE` 环境变量
- 升级：拉取新版本后重新 `docker compose up -d`（题库与判题数据都在 `./data` 卷内原样保留）
- 判题功能要求 `ORANGEOJ_JUDGE_SHARED_TOKEN` 非默认值且三容器配置一致，否则刷题页面的运行/测试/提交返回 503

### 本地开发（可选）

仅面向开发者，普通部署请用上方 Compose 方式：

```powershell
.\scripts\dev.ps1        # 仅主站开发（:8080 + :5173 热更新）
.\scripts\dev-quiz.ps1   # 主站 + 刷题/OJ + 判题沙箱全部开发（:8080/:5173、:8081/:5174、:9090）
.\scripts\test-oj.ps1    # 全量测试（静态检查 + 真实 Python/C++ 评测 E2E）
```

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

### 一键测试（含真实评测端到端）

```powershell
.\scripts\test-oj.ps1                # 全量：静态检查 + 双前端构建 + 三进程真实 Python/C++ 评测 E2E
.\scripts\test-oj.ps1 -StaticOnly    # 仅 go vet/test + linux 交叉编译（+前端构建）
.\scripts\test-oj.ps1 -SkipFrontends # 跳过 npm build
# E2E 使用独立端口（默认 18090/18091/19090）与 %TEMP%\orangeoj-test-* 临时数据，
# 结束自动清理；无本地 g++/python 的机器对应断言自动 SKIP。
```

### 判题安全须知
- 学生代码只在 **judge-runtime** 内执行：Linux 生产为 nsjail（无网络、无 proc、降权 nobody、cgroup 内存/PID 限制）；
  Windows/本地开发为进程级受限运行（限时/隔离目录/精简环境），**无安全隔离承诺**，仅用于联调
- judge-runtime 与刷题服务之间以共享 token（`ORANGEOJ_JUDGE_SHARED_TOKEN`）认证；生产务必更换默认值
- 评测结果仅记录：逐测试点 verdict/耗时/输出/错误与提交历史；题面测试点与答案永不下发学生
- 判题队列空闲轮询 400ms/认领失败退避 800ms，worker 数由 `-judge-workers` 控制（默认 2）
