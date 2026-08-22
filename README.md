# 🍊 OrangeRepo 题库

兼容 [OrangeOJ](https://github.com/shinyes/OrangeOJ) 格式的**纯题库管理 Web 应用**：
嵌套目录树、标签搜索筛选、题面/答案/题解查看编辑、勾选题目编制训练与练习、ZIP 双向导入导出。
不做判题、提交等任何 OJ 运行时功能。

## 技术栈

| 层 | 技术 |
|---|---|
| 后端 | Go + Fiber v2 + SQLite（modernc.org/sqlite，无 CGO） |
| 前端 | React 19 + Vite + TypeScript + Tailwind CSS v4 + shadcn/ui |
| 渲染 | marked + DOMPurify + KaTeX（与 OrangeOJ 一致） |

## 快速开始

```powershell
# 方式一：开发模式（前端 :5173 热更新，后端 :8080）
.\scripts\dev.ps1

# 方式二：生产模式（Go 直接托管前端构建产物）
cd web ; npm install ; npm run build ; cd ..
go run . -seed          # 首次可加 -seed 导入示例包
```

访问 `http://localhost:5173`（开发）或 `http://localhost:8080`（生产）。

## Docker 部署

推送 `v*` 版本标签时，GitHub Actions 自动构建镜像发布到 GHCR，并创建对应 Release。

```bash
docker compose -f deploy/docker-compose.yml up -d
# 或手动：
docker run -d -p 8080:8080 -v ./data:/app/data ghcr.io/shinyes/orange_repo:1.0.2
```

镜像基于 distroless/static。容器启动时若为 root，会自动把数据目录属主修正为运行用户 65532 并**立即降权**后再对外服务——因此 `./data:/app/data` 绑定挂载在 Linux 宿主机上开箱即用，无需手动 chown；命名卷（`-v orangerepo-data:/app/data`）同样免配置。

每个 Release 附件附带离线镜像包 `orangerepo-<版本>-linux-amd64-image.tar.gz`，在无法访问 GHCR 的机器上 `docker load -i <包名>` 导入即可使用。

**首次启动**自动创建单用户密码，默认 `123456`，请登录后在左上角「⚙」中修改。
数据存储在 `data/` 目录（SQLite + 上传图片），删除该目录即可重置。

## 功能

- **两栏布局**：左栏 = 管理（新建题目/目录、导入导出 ZIP、搜索、类型过滤、嵌套目录树、题目列表）；右栏 = 浏览（题面 Markdown+KaTeX、按题型渲染的答案、题解列表）
- **多标签筛选**：标签面板常驻展示全部标签（动态 facet 计数随搜索/目录/已选联动），支持多选 AND 组合、已选置顶移除、一键清空、标签内查找与数量/名称排序
- **题目编辑**：三种题型（编程 / 单选 / 判断），字段结构与 OrangeOJ 完全一致；编程题支持输入输出格式、样例、测试点、时限内存；题解支持多语言代码 + Markdown 解读；题面支持图片上传插入
- **训练与练习**：左侧勾选题目 → 加入训练（章节结构）/ 练习（平铺+分值）→ 一键导出 OrangeOJ 兼容 ZIP
- **OrangeOJ 兼容**：`problems.json` + `trainingPlan.json` + `images/` 的双向转换，图片引用路径自动重写，导出包可直接导入 OrangeOJ

## API 概览

认证为单用户会话 Cookie。完整契约见 `docs/aegis/specs/2026-08-22-orangerepo-design.md` §5。

```
POST /api/auth/login|logout   GET /api/auth/me      PUT /api/auth/password
GET/POST /api/directories     PUT/DELETE /api/directories/:id
GET/POST /api/problems        GET/PUT/DELETE /api/problems/:id
PUT  /api/problems/:id/solutions | /directory
GET  /api/tags                POST /api/images      GET /api/uploads/*
POST /api/import?mode=…       GET  /api/export/problems | trainings/:id | practices/:id
CRUD /api/trainings · chapters · items ； CRUD /api/practices · practice-items
```

## 项目结构

```
main.go                  入口（-addr / -data / -seed）
internal/model           数据模型与 JSON 形状
internal/store           SQLite 迁移与查询
internal/zipio           OrangeOJ ZIP 兼容层（唯一权威实现，含单测）
internal/server          Fiber 路由与会话认证（含 httptest 冒烟测试）
web/                     React 前端（Vite + shadcn/ui）
samples/orangeoj-sample.zip  示例题包（配合 -seed）
docs/aegis/              设计规格、实施计划与治理文档
```

## 开发验证

```powershell
go vet ./... ; go test ./...     # 后端：zipio 往返一致性 + 服务端到端冒烟
cd web ; npm run build           # 前端构建
```
