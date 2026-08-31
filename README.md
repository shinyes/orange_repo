# 🍊 OrangeRepo 题库

兼容 [OrangeOJ](https://github.com/shinyes/OrangeOJ) 格式的**纯题库管理 Web 应用**：
斜杠嵌套标签树（父标签前缀筛选）、题面+答案同屏查看编辑、勾选题目编制训练与练习、ZIP 双向导入导出。
不做判题、提交等任何 OJ 运行时功能。

配套**刷题服务**（独立端口，另见下文）：学生按科目/分类随机抽题作答，答错进入错题集（答对自动移除），
管理员在系统管理中维护科目、分类（映射题目标签与题型）与学生账号。

## 技术栈

| 层 | 技术 |
|---|---|
| 后端 | Go + Fiber v2 + SQLite（modernc.org/sqlite，无 CGO） |
| 前端 | React 19 + Vite + TypeScript + Tailwind CSS v4 + shadcn/ui |
| 渲染 | marked + DOMPurify + KaTeX（与 OrangeOJ 一致） |

## 快速开始

```powershell
# 方式一：开发模式（主站前端 :5173 热更新，后端 :8080）
.\scripts\dev.ps1

# 方式二：开发模式（主站 + 刷题服务一起：:5173/:8080 与 :5174/:8081）
.\scripts\dev-quiz.ps1

# 方式三：生产模式（Go 直接托管前端构建产物）
cd web ; npm install ; npm run build ; cd ..
go run . -seed          # 首次可加 -seed 导入示例包

# 刷题服务（生产模式；需先初始化题库，即主站至少启动过一次）
cd web-quiz ; npm install ; npm run build ; cd ..
go run ./cmd/quiz -addr :8081
```

访问 `http://localhost:5173`（主站开发）或 `http://localhost:8080`（主站生产）；
刷题服务见 `http://localhost:5174`（开发）或 `http://localhost:8081`（生产）。
刷题服务初始管理员 `admin/123456`，学生账号由管理员在「我的 → 系统管理」中创建。

## Docker 部署

推送 `v*` 版本标签时，GitHub Actions 自动构建镜像发布到 GHCR，并创建对应 Release。

```bash
docker compose -f deploy/docker-compose.yml up -d
# 或手动：
docker run -d -p 8080:8080 -v ./data:/app/data ghcr.io/shinyes/orange_repo:1.9.5
```

镜像基于 distroless/static。容器启动时若为 root，会自动把数据目录属主修正为运行用户 65532 并**立即降权**后再对外服务——因此 `./data:/app/data` 绑定挂载在 Linux 宿主机上开箱即用，无需手动 chown；命名卷（`-v orangerepo-data:/app/data`）同样免配置。

每个 Release 附件附带离线镜像包 `orangerepo-<版本>-linux-amd64-image.tar.gz`，在无法访问 GHCR 的机器上 `docker load -i <包名>` 导入即可使用。

**首次启动**自动创建单用户密码，默认 `123456`，请登录后在左上角「⚙」中修改。
数据存储在 `data/` 目录（SQLite + 上传图片），删除该目录即可重置。

## 功能

- **三栏布局**：左栏 = 标签筛选（搜索、类型过滤、标签树）；中栏 = 题目查看（新建/导入导出、题目列表、批量操作）；右栏 = 详情（题面 Markdown+KaTeX 与答案同屏、按题型渲染、题解列表）；题册栏可展开在题目栏右侧
- **多标签筛选**：标签树常驻展示（每个标签显示它自己实际命中的题目数，不受已选标签影响）；父标签前缀包含全部子孙；支持多选 AND 组合、已选置顶移除、一键清空、树内查找与数量/名称排序；节点菜单支持子树整体重命名与删除，涉及题目自动同步
- **题目编辑**：三种题型（编程 / 单选 / 判断），字段结构与 OrangeOJ 完全一致；编程题支持输入输出格式、样例、测试点、时限内存；题解支持多语言代码 + Markdown 解读；题面支持图片上传插入
- **训练与练习**：左侧勾选题目 → 加入训练（章节结构）/ 练习（平铺）→ 一键导出 OrangeOJ 兼容 ZIP；题册栏支持可嵌套目录管理与拖拽（改层级/顺序），训练/练习内均支持鼠标拖拽调整题目顺序
- **OrangeOJ 兼容**：`problems.json` + `trainingPlan.json` + `images/` 的双向转换，图片引用路径自动重写，导出包可直接导入 OrangeOJ
- **刷题服务（独立端口）**：学生选科目/分类后随机抽题（每轮题数可全局配置），点选项即自动判题并提示对错与正确答案，有解析的题可展开「查看解析」；答错自动进错题集（按分类展示，学生可单独或全部练习，**任何模式下答对即自动移除**）；管理员「我的 → 系统管理」维护科目、分类（显示顺序 + 题目标签/题型映射 + 实时题目数）、学生账号与全局设置

## API 概览

认证为单用户会话 Cookie。完整契约见 `docs/aegis/specs/2026-08-22-orangerepo-design.md` §5；**面向 AI/程序化调用的端点级参考见 `docs/api-reference.md`**（含请求/响应示例与工作流）。

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

刷题服务契约见 `docs/aegis/specs/2026-08-29-quiz-service-design.md` §5（独立进程 :8081，多用户会话）：

```
POST /api/auth/login|logout   GET /api/auth/me      PUT /api/auth/password
GET /api/quiz/subjects        POST /api/quiz/round | submit | wrong-round      GET /api/quiz/wrong-summary
GET/POST /api/admin/subjects · PUT /api/admin/subjects/order · PATCH/DELETE /api/admin/subjects/:id
POST/PATCH/DELETE /api/admin/categories · PUT /api/admin/subjects/:id/categories/order · GET /api/admin/problems-count
GET/POST /api/admin/students · PUT /api/admin/students/:id/password · DELETE /api/admin/students/:id
GET/PUT /api/admin/settings
```

## 项目结构

```
main.go                  入口（-addr / -data / -seed）
cmd/quiz/                刷题服务入口（独立端口，默认 :8081）
internal/model           数据模型与 JSON 形状
internal/store           SQLite 迁移与查询
internal/quizstore       刷题数据层：quiz.db（用户/科目/分类/错题）+ orangerepo.db 只读 reader
internal/quizserver      刷题 Fiber 路由与会话认证（多用户：管理员/学生）
internal/zipio           OrangeOJ ZIP 兼容层（唯一权威实现，含单测）
internal/server          Fiber 路由与会话认证（含 httptest 冒烟测试）
web/                     React 前端（Vite + shadcn/ui）
web-quiz/                刷题前端（Vite + shadcn/ui，dev :5174）
samples/orangeoj-sample.zip  示例题包（配合 -seed）
docs/aegis/              设计规格、实施计划与治理文档
```

## 开发验证

```powershell
go vet ./... ; go test ./...     # 后端：zipio 往返一致性 + 主站/刷题服务端到端冒烟
cd web ; npm run build           # 主站前端构建
cd web-quiz ; npm run build      # 刷题前端构建
```
