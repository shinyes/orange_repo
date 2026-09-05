# 🍊 OrangeOJ 题库与判题

兼容 [OrangeOJ](https://github.com/shinyes/OrangeOJ) 的题库管理与**判题** Web 应用，由「OrangeOJ 题库 + Orange 刷题」扩展而来：

- **主站**（:8080）：题库管理 —— 斜杠嵌套标签树、题面+答案同屏编辑、训练/练习编制、ZIP 双向导入导出；
- **刷题服务 / OJ**（:8081）：空间化做题门户（训练/练习/刷题/排行榜）+ 编程题判题；
- **判题沙箱 judge-runtime**（:9090）：真正执行学生代码的独立服务，**仅支持 Python 与 C++**（nsjail 隔离）。

实现方式模仿上游 OrangeOJ（判题队列/评测运行器以其源码为基线，差异仅保留 Python+C++）。

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
- 主镜像 `ghcr.io/shinyes/orangeoj:<版本>` 由 GitHub Actions 随每个版本自动发布；判题沙箱镜像
  `ghcr.io/shinyes/orangeoj-judge` **仅当版本含 judge 相关变更时构建**（并刷新 `:latest`——nsjail 编译耗时，
  避免无谓重建），compose 默认引用 `:latest` 自动沿用最新判题镜像；如需固定其他版本可设 `ORANGEOJ_JUDGE_IMAGE` 环境变量
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

### 域与仓库（主站，OrangeOJ 管理端）
- **域**：题库逻辑隔离单位（系统管理员建域/改名/删除/设域管理员，域间题目完全隔离）
- **仓库** = 每个域一页题目管理：三栏布局（标签树前缀筛选 / 题目列表 / 题面·答案·题解同屏编辑）
- 三种题型：编程（输入/输出格式、样例、测试点、时限内存）、单选、判断
- 题目带 **UUIDv7 稳定标识**（导入包缺 uuid 自动补、有 uuid 去重）
- 仓库题册（训练章节化/练习题单）保留作**域级模板**；OrangeOJ 兼容 ZIP 导入导出与**全量备份/恢复**
  （problems.json + orangerepo-backup.json，全库单包迁移）

### 空间（域内做题组织）
- 每域多个**空间**：成员由管理员拉入；空间内训练/练习/刷题互相隔离
- 空间内容由域管理员管理：自建或**从仓库模板拷贝**、从域题库选题编制

### 学生做题（门户）
- 登录后按加入的空间进入；多空间可切换；空间内 训练 / 练习 / 刷题 / 排行榜
- **训练**：章节结构；客观题**选择次数上限**（训练级配置，达限标红锁定禁选、答对标绿）；
  编程题不限次（Monaco 编辑器 + 运行/测试/提交评测）
- **练习**：整份试卷作答（客观题可改选 + 编程题跳转做题），**统一交卷后才见结果**，可重做且每次记录
- **刷题**：管理员按标签筛选题库或绑定仓库题单生成；客观题答对记一次通过、答错不限重答
- **排行榜**：按域总榜——学生按题目 uuid 去重计通过数，管理员不参与
- 客观题 = 单选/判断即点即判并高亮正确答案；判题内核照搬上游（队列/提交/进度/逐用例独立进程评测）
- 评测结论：AC / WA / CE / RE / TLE / MLE / OK（run），逐测试点明细与耗时

## API 概览

主站（管理员会话，完整契约见 `docs/aegis/specs/2026-08-22-OrangeOJ-design.md` §5 与 `docs/api-reference.md`）：

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

刷题服务（门户 + 判题，契约见 `docs/aegis/specs/2026-09-04-orangerepo-oj-refactor-design.md`）：

```
POST /api/auth/login|logout   GET /api/auth/me      PUT /api/auth/password
GET  /api/portal/spaces                                    我的空间（多空间切换）
GET  /api/portal/space/:id/home                            空间首页三区概览
GET  /api/portal/space/:id/training/:tid                   训练详情（客观题限次/锁定态）
POST /api/portal/space/:id/training/:tid/answer            训练客观题作答（限次）
GET  /api/portal/space/:id/practice/:pid                   练习详情
POST /api/portal/space/:id/practice/:pid/submit            练习整卷交卷
GET  /api/portal/space/:id/practice/:pid/submissions       交卷历史
GET  /api/portal/space/:id/quizzes                         空间刷题项目列表
GET  /api/portal/quiz/:qid/problem    POST /api/portal/quiz/:qid/answer   刷题抽题/作答
GET  /api/portal/rank?domainId=                           域排行榜（uuid 去重通过数）
GET  /api/oj/problem/:id                题目正文（测试点/答案/题解永不下发；可见性=空间域）
POST /api/oj/problem/:id/run|test|submit      {language, sourceCode[, inputData]} → submissionId
POST /api/oj/problem/:id/objective-submit     {answer} → 同步判定（客观题）
GET  /api/oj/submission/:id/poll             轮询结果     GET /api/oj/problem/:id/submissions 历史
judge-runtime：POST /internal/judge/execute（X-Judge-Token）  GET /healthz
```

题目可见性统一为**空间模型**：用户加入的空间所在域包含该题即可见（空间训练/练习/刷题引用域内题目）。
管理端（域管理员/空间管理、成员维护、训练/练习/刷题项目编制）位于主站 `internal/server`（`/api/admin/domains|spaces|users` 等）。

## 项目结构

```
main.go                  入口（-addr / -data / -seed）—— 主站
cmd/quiz/                刷题/OJ 服务入口（-addr / -judge-endpoint / -judge-token / -judge-workers）
cmd/judge-runtime/       judge-runtime 入口（环境变量配置，:9090）
internal/model           数据模型与 JSON 形状
internal/store           SQLite 迁移与查询（主库 orangeoj.db）
internal/accounts        共享账号库（users/sessions，主站与刷题服务统一账号唯一 owner）
internal/quizstore       刷题数据层：quiz.db（判题 submissions/judge_jobs/progress + 空间作答三表）+ 主库只读 reader
internal/quizserver      刷题 Fiber 路由（/api/auth /api/portal /api/oj）
internal/judge           判题编排（队列/HTTPRunner/类型），迁移自上游 queue.go/runner.go
internal/judgeserver     评测执行器（Python/C++）+ 沙箱后端（Linux nsjail / 开发受限运行）+ HTTP 服务
internal/zipio           OrangeOJ ZIP 兼容层
internal/server          主站 Fiber 路由
web/                     React 前端（主站）
web-quiz/                React 前端（刷题/OJ）
samples/                 示例题包
Dockerfile               主镜像（OrangeOJ + quiz 双二进制，distroless）
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
