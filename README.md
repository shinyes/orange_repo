# 🍊 OrangeOJ

面向学校/机构的轻量 **在线判题与练习系统**：域级题库仓库 + 空间化训练/练习/刷题 + Python/C++ 沙箱判题。

- **管理端（域仓库，应用内 /admin）**：域管理、题库（题目/标签/题册模板）、空间与成员管理
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

### 管理端（应用内 /admin · 域仓库）

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

访问（单服务、单前端，一个端口承载门户与管理端）：

- 学生门户：http://localhost:8080/
- 管理端（管理员）：http://localhost:8080/admin
- judge-runtime :9090 仅供容器内网，不对外

首次启动自动创建管理员 `admin / 123456`，登录后请立即修改；随后：

1. 系统管理员进入 **/admin**「域管理」新建域（可同时创建域管理员账号）；
2. 域管理员在「题目管理」维护题目（或 **导入 ZIP / 全量备份**，新库导入后即可用）；
3. 在「空间管理」新建空间、拉入成员账号；
4. 在空间内编排训练/练习/刷题项目（自建或从仓库题册拷贝）；
5. 学生登录门户（/）进入空间做题。

### 说明

- **数据**：全部保存在 compose 文件同目录的 `./data`（唯一数据库 `orangeoj.db` + 上传图片）。删除目录即重置。
- **架构**：单一 Go 进程托管全部前端与 API（管理端 + 门户合服）；仅判题沙箱为独立进程（nsjail 需特权容器）。
- **镜像**：主镜像 `ghcr.io/shinyes/orangeoj:<版本>` 由 GitHub Actions 随版本发布；判题镜像 `ghcr.io/shinyes/orangeoj-judge` 仅在含判题相关变更的版本构建并刷新 `:latest`（可用 `ORANGEOJ_JUDGE_IMAGE` 覆盖版本）。
- **升级**：`docker compose pull && docker compose up -d`；数据在卷内保留。需要跨大版本迁移时用管理端「全量备份/恢复」。
- 判题要求 `ORANGEOJ_JUDGE_SHARED_TOKEN` 非默认值且两容器一致，否则做题页的运行/测试/提交返回 503。

---

## 本地开发

两个可执行组件（Go 1.25+，无需 CGO）：

| 组件 | 入口 | 默认端口 | 前端 |
|---|---|---|---|
| 主服务（门户 + 管理端） | `go run . -data ./data -web ./app/dist` | 8080 | `app/`（单前端 Vite dev 代理 /api → :8080） |
| 判题沙箱 | `go run ./cmd/judge-runtime`（见环境变量） | 9090 | — |

开发机直接运行：

```powershell
# Windows 一键脚本（后端单进程 + 单前端 Vite dev + 判题 dev 模式）
scripts/dev.ps1

# 或手动：先起后端（空库 -seed 灌入示例题册），再起前端
go run . -data ./data -seed -judge-token dev-token -judge-endpoint http://127.0.0.1:9090
cd app && npm run dev        # http://localhost:5175（门户 / + 管理端 /admin）
```

> 单进程说明：全部 API/前端由同一 Go 进程承载（`.data/orangeoj.db` 单库）；生产用构建产物（`-web` 指向 `app/dist` 挂 `/`，管理区 `/admin` 由前端路由处理），开发用 Vite 代理。

---

## 测试

```powershell
# 全量静态检查 + 单测
go vet ./...
go test ./...

# 前端
cd app && npm run build          # 或 node node_modules/typescript/bin/tsc -b

# 端到端（真实判题，需本地 g++ / Python）
scripts/test-oj.ps1
```

`scripts/test-oj.ps1` 启动单主服务 + judge-runtime 于独立端口（18090/19090）与 `%TEMP%` 临时数据，结束后自动清理；无本地工具链的机器对应断言自动 SKIP。

---

## 项目结构

```
main.go              单进程入口（合服：管理 API + 门户 API + 静态托管）
cmd/
  judge-runtime/     判题沙箱（:9090，唯一独立进程）
internal/
  app/               单进程组装（共享连接/路由挂载/auth/静态）
  accounts/          users/sessions（单库内账号权威）
  store/             orangeoj.db：题目/标签/域/空间/空间内容 + 迁移
  server/            管理端 HTTP API（域/空间/题库/导入导出/备份）
  quizstore/         判题与作答数据层（submissions/judge_jobs/空间作答）
  quizserver/        门户 HTTP API（/api/portal/*、/api/oj/*）
  judge/             判题队列编排
  judgeserver/       nsjail 沙箱执行器
  model/             共享类型
  zipio/             OrangeOJ ZIP 导入导出格式
app/                 单前端（React + TS）：门户 / + 管理端 /admin（唯一前端）
deploy/              Docker Compose 部署示例
scripts/             dev / 端到端测试脚本
samples/             示例题册（-seed）
docs/                设计文档
```

## 技术栈

| 层 | 技术 |
|---|---|
| 后端 | Go + Fiber v2 + SQLite（modernc.org/sqlite，无 CGO，单库文件） |
| 前端 | React 19 + Vite + TypeScript + Tailwind CSS v4 + base-ui |
| 编辑器 | Monaco（本地资源，无 CDN） |
| 渲染 | marked + DOMPurify + KaTeX + Shiki |
| 判题 | 独立 judge-runtime（Linux: nsjail + cgroup v2；Python 3 / C++） |
