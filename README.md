# 🍊 OrangeOJ

面向学校/机构的轻量 **在线判题与练习系统**：域级题库仓库 + 空间化训练/练习/刷题 + Python/C++ 沙箱判题。

- **管理端（域仓库）**：域管理、题库（题目/标签/题册模板）、空间与成员管理
- **学生门户**：空间切换、训练（限次作答）、练习（整卷交卷）、刷题、排行榜
- **判题内核**：独立沙箱进程（Linux: nsjail + cgroup v2），支持 Python 3 / C++

---

## 目录

- [概念模型](#概念模型)
- [功能一览](#功能一览)
- [快速开始（Docker Compose）](#快速开始docker-compose)
- [本地开发](#本地开发)
- [测试](#测试)
- [项目结构](#项目结构)
- [技术栈](#技术栈)

---

## 概念模型

```
系统管理员（域管理）
 └─ 域 Domain（题库逻辑隔离：题目/标签按域独立）
     ├─ 仓库 Repo = 该域题目管理页（域管理员维护：题目/标签/题册模板/导入导出）
     └─ 空间 Space（做题组织单位，域内多个，互相隔离）
         ├─ 空间成员（由管理员拉入）
         ├─ 空间训练：章节化题单；客观题选择次数上限（答对标绿、达限标红）
         ├─ 空间练习：整卷试卷；一次性作答后交卷才见结果（可重做、逐次记录）
         ├─ 空间刷题：按标签或仓库题单生成的客观题刷题项目
         └─ 排行榜：域级总榜，按题目 UUID 去重计通过数（管理员不参与）
```

角色：

| 角色 | 权限 |
|---|---|
| `global_admin` 系统管理员 | 建域/改名/删除、设域管理员、进任意域仓库与空间 |
| `domain_admin` 域管理员 | 管理所属域仓库（题目/模板）与该域全部空间（空间/成员/内容） |
| `member` 空间成员（学生） | 进入被加入的空间做题；多空间可切换 |

> 题目带有 **UUIDv7 稳定标识**：跨库迁移/备份导入按 UUID 去重，空间通过记录按 UUID 统计，ID 变化不影响连续性。

---

## 功能一览

### 管理端（主站 · 域仓库）

- **题库**：三栏管理（标签树前缀筛选 / 题目列表 / 题面·答案·题解同屏编辑）
  - 三种题型：编程（样例/测试点/时限内存）、单选、判断
  - 标签支持斜杠层级（`数学/几何`），前缀多选 AND
  - 图片上传、孤儿图片清理
- **域管理**（系统管理员）：域 CRUD、题目/空间计数、设置/移除域管理员
- **空间管理**（域管理员 / 系统管理员）：
  - 空间 CRUD、成员拉入/移除、成员账号创建
  - 空间内容编排：训练（章节+条目）、练习（题单）、刷题项目（标签/仓库题单两种来源）
  - 从**仓库题册模板**一键拷贝，或从域题库逐题勾选
- **导入导出**：
  - 单题册 / 筛选导出（OrangeOJ 兼容 ZIP）
  - **全库备份 / 恢复**（单文件整库迁移；导入时题目无 UUID 自动补、有 UUID 去重）

### 学生门户

- 登录后进入空间（单空间直达 / 多空间选择切换），空间内四页签：**训练 / 练习 / 刷题 / 排行榜**
- **训练**：章节化题单；客观题**限次作答**（训练级统一上限）——答对即绿锁定、答错计数、达上限标红禁选；
  编程题不限次
- **练习**：整卷作答（客观题可改选、编程题跳转做题页），**交卷后统一判定展示**；可重做且每次作答留档
- **刷题**：随机抽题即时反馈；答对记一次通过（UUID 去重），答错不限重答
- **排行榜**：所在域总榜（通过题数降序），高亮自己；管理员不参与
- **做题页**：编程题 = Monaco 编辑器（Python/C++，本地草稿）+ **运行**（自定义输入）/ **测试**（样例与测试点）/ **提交**（评测）+ 逐条测评记录；客观题即点即判并提示正确答案
- 判题结论：AC / WA / CE / RE / TLE / MLE，逐测试点明细与耗时

### 判题沙箱

- 独立 `judge-runtime` 进程，评测 Python 3 / C++（nsjail 隔离）
- 队列化评测（judge_jobs），按用例独立进程运行

---

## 快速开始（Docker Compose）

### 前置要求

- Docker Engine（含 Compose v2）
- Linux 宿主机支持 **cgroup v2**（判题沙箱 nsjail 需要；容器以 privileged 运行）
- 判题沙箱仅支持 **Linux**（Windows/macOS 仅适合开发，判题为受限模式）

### 部署步骤

```bash
# 1. 克隆
git clone https://github.com/shinyes/orange_repo.git
cd orange_repo

# 2. 设置判题共享 token（生产务必随机长字符串；三容器自动注入同一份）
export ORANGEOJ_JUDGE_SHARED_TOKEN='换成你的随机token'

# 3. 一键启动（自动从 GHCR 拉取主镜像与判题沙箱镜像）
docker compose -f deploy/docker-compose.yml up -d
```

访问：

- 管理端（域仓库）：http://localhost:8080
- 学生门户：http://localhost:8081
- judge-runtime :9090 仅供容器内网，不对外

首次启动自动创建管理员 `admin / 123456`（两端同一账号库），登录后请立即修改；随后：

1. 系统管理员在管理端「域管理」新建域（可同时创建域管理员账号）；
2. 域管理员进入仓库页维护题目（或 **导入 ZIP / 全量备份**，新库导入后即可用）；
3. 在「空间管理」新建空间、拉入成员账号；
4. 在空间内编排训练/练习/刷题项目（自建或从仓库题册拷贝）；
5. 学生登录门户进入空间做题。

### 说明

- **数据**：全部保存在 compose 文件同目录的 `./data`（唯一数据库 `orangeoj.db` + 上传图片）。删除目录即重置。
- **镜像**：主镜像 `ghcr.io/shinyes/orangeoj:<版本>` 由 GitHub Actions 随版本发布；判题镜像 `ghcr.io/shinyes/orangeoj-judge` 仅在含判题相关变更的版本构建并刷新 `:latest`（可用 `ORANGEOJ_JUDGE_IMAGE` 覆盖版本）。
- **升级**：`docker compose pull && docker compose up -d`；数据在卷内保留。需要跨大版本迁移时用管理端「全量备份/恢复」。
- 判题要求 `ORANGEOJ_JUDGE_SHARED_TOKEN` 非默认值且三容器一致，否则做题页的运行/测试/提交返回 503。

---

## 本地开发

三个可执行组件（Go 1.25+，无需 CGO）：

| 组件 | 入口 | 默认端口 | 前端 |
|---|---|---|---|
| 管理端（主站） | `go run . -data ./data -web ./web/dist` | 8080 | `web/`（Vite，代理 /api → :8080） |
| 学生门户（刷题服务） | `go run ./cmd/quiz -data ./data -web ./web-quiz/dist` | 8081 | `web-quiz/`（Vite，代理 /api → :8081） |
| 判题沙箱 | `go run ./cmd/judge-runtime`（见环境变量） | 9090 | — |

开发机直接运行：

```powershell
# Windows 一键脚本（后端 + 前端 + 判题 dev 模式）
scripts/dev.ps1          # 管理端
scripts/dev-quiz.ps1     # 门户 + 判题（dev token，无 nsjail 受限评测）

# 或手动：先起后端，再起各自前端
go run . -data ./data -seed          # 空库时 -seed 灌入示例题册
go run ./cmd/quiz -data ./data -judge-token dev-token -judge-endpoint http://127.0.0.1:9090
```

> 单库说明：所有数据（题库/账号/判题/作答）同处 `./data/orangeoj.db`；两个后端进程各自连接该文件（WAL 并发），需共享同一 `-data` 目录。

---

## 测试

```powershell
# 全量静态检查 + 单测
go vet ./...
go test ./...

# 前端
cd web && npm run build          # 或 node node_modules/typescript/bin/tsc -b
cd web-quiz && npm run build

# 端到端（真实判题，需本地 g++ / Python）
scripts/test-oj.ps1
```

`scripts/test-oj.ps1` 使用独立端口（18090/18091/19090）与 `%TEMP%` 临时数据，结束后自动清理；无本地工具链的机器对应断言自动 SKIP。

---

## 项目结构

```
cmd/
  quiz/            学生门户服务（:8081，托管 web-quiz）
  judge-runtime/   判题沙箱（:9090）
internal/
  accounts/        users/sessions（单库内账号权威）
  store/           orangeoj.db：题目/标签/域/空间/空间内容 + 迁移
  server/          管理端 HTTP API（域/空间/题库/导入导出/备份）
  quizstore/       判题与作答数据层（submissions/judge_jobs/空间作答）
  quizserver/      门户 HTTP API（/api/portal/*、/api/oj/*）
  judge/           判题队列编排
  judgeserver/     nsjail 沙箱执行器
  model/           共享类型
  zipio/           OrangeOJ ZIP 导入导出格式
web/               管理端前端（React + TS）
web-quiz/          学生门户前端（React Router + TS）
deploy/            Docker Compose 部署示例
scripts/           dev / 端到端测试脚本
samples/           示例题册（-seed）
docs/              设计文档
```

## 技术栈

| 层 | 技术 |
|---|---|
| 后端 | Go + Fiber v2 + SQLite（modernc.org/sqlite，无 CGO，单库文件） |
| 前端 | React 19 + Vite + TypeScript + Tailwind CSS v4 + base-ui |
| 编辑器 | Monaco（本地资源，无 CDN） |
| 渲染 | marked + DOMPurify + KaTeX + Shiki |
| 判题 | 独立 judge-runtime（Linux: nsjail + cgroup v2；Python 3 / C++） |
