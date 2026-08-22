# OrangeRepo 设计规格

日期：2026-08-22 · 状态：已确认（用户选定 Go+Fiber 后端、单用户、纯题库管理）

## 1. 目标与边界

构建题库管理 Web 应用 **OrangeRepo**：

- 嵌套层级的目录树管理题目
- 标签搜索与筛选
- 两栏布局：左栏 = 管理（新建/上传/下载/搜索/目录树/题目列表）；右栏 = 浏览（题面/答案/题解 查看+编辑）
- 勾选题目编制「训练」（章节结构）与「练习」（平铺列表），可导出
- 与 OrangeOJ 的 ZIP 交换格式**双向兼容**（硬约束）
- 单用户登录

**非目标**：判题、提交记录、考试运行时、成员/进度/多空间、批量注册。训练/练习仅是题目编组，不含作答流程。

## 2. OrangeOJ 兼容格式（权威基线，源自上游源码）

ZIP 包结构：

```
problems.json      必有；题目对象数组（2 空格缩进、不转义 HTML）
trainingPlan.json  可选；仅当存在章节或标题/标签元数据时写出
images/<file>      可选；题面引用到的图片文件集合
```

题目对象字段（与上游 `problemExportEntry` 一致）：

```json
{
  "type": "programming | single_choice | true_false",
  "title": "…",
  "tags": ["…"],
  "statementMd": "Markdown，支持 KaTeX",
  "bodyJson": {…}, "answerJson": {…},
  "solutions": [{"language":"cpp|python|go|turtle","code":"…","markdown":"…"}],
  "timeLimitMs": 1000, "memoryLimitMiB": 256
}
```

题型约定（源自上游 `queue.go` / `objective_answers.go`）：

- programming：`bodyJson = {"inputFormat":"","outputFormat":"","samples":[{"input","output"}],"testCases":[…]}`；时限默认 1000ms / 256MiB
- single_choice：`bodyJson={"options":[…]}`；`answerJson={"answerIndex":n}`（导入兼容 `answer` 文本匹配选项）
- true_false：`answerJson={"answer":bool}`（导入兼容键 answer/correct/correctAnswer/value 强转布尔）

trainingPlan.json（与上游 `trainingPlanChapterJSON` 一致）：`{"title"?,"description"?,"tags"?:[],"chapters":[{"title","orderNo","problemIds":[…]}]}`，其中 `problemIds` 是指向 problems.json 数组**下标**的引用。

图片引用：导出时收集四个文本字段中的 `/api/uploads/<file>` 引用并把文件打包进 images/（不改写 markdown）；导入时把 `(images/` 重写为 `(/api/uploads/`。文件名规则 `[a-zA-Z0-9_-]+\.(png|jpe?g|gif|webp|svg)`。problems.json / trainingPlan.json 允许位于 ZIP 内任意目录层级（取第一个命中）。

solutions 语言归一化别名：c++/cpp/c→cpp；python/python3/py→python；go/golang→go；turtle 变体→turtle；空 language 项丢弃。

## 3. 架构与技术栈

```
Go 1.25 + Fiber v2 + modernc.org/sqlite（纯 Go 无 CGO）+ bcrypt 会话认证
React 18 + Vite + TypeScript + Tailwind v4 + shadcn/ui + marked + KaTeX + DOMPurify + TanStack Query
单进程部署：Go 服务 :8080 同时提供 /api 与 web/dist 静态资源；开发期 Vite :5173 代理 /api
```

存储：SQLite 文件 `data/orangerepo.db`；上传图片落盘 `data/uploads/<hex32>.<ext>`。

## 4. 数据模型（SQLite）

```sql
settings(key TEXT PRIMARY KEY, value TEXT NOT NULL)          -- session_token / password_hash
directories(id PK, name TEXT NOT NULL, parent_id INT NULL REFERENCES directories(id), order_no INT DEFAULT 0)
problems(id PK, type TEXT NOT NULL, title TEXT NOT NULL,
         tags_json TEXT DEFAULT '[]', statement_md TEXT DEFAULT '',
         body_json TEXT DEFAULT '{}', answer_json TEXT DEFAULT '{}',
         solutions_json TEXT DEFAULT '[]',
         time_limit_ms INT DEFAULT 1000, memory_limit_mib INT DEFAULT 256,
         directory_id INT NULL, created_at DATETIME DEFAULT CURRENT_TIMESTAMP)
trainings(id PK, title, description DEFAULT '', tags_json DEFAULT '[]', created_at)
training_chapters(id PK, training_id INT NOT NULL, title, order_no INT DEFAULT 0)
training_items(id PK, chapter_id INT NOT NULL, problem_id INT NOT NULL, order_no INT DEFAULT 0)
practices(id PK, title, description DEFAULT '', tags_json DEFAULT '[]', created_at)
practice_items(id PK, practice_id INT NOT NULL, problem_id INT NOT NULL, order_no INT DEFAULT 0, score INT DEFAULT 100)
```

删除语义：删除目录 → 其子目录与题目上移到被删目录的父级（父为空则到根）；删除题目 → 级联清理 training/practice 条目；删除训练/练习 → 级联章节与条目；不删题目本体。

## 5. API 契约

除 `/api/auth/login|me` 外全部要求会话 Cookie（`orange_session`）。

```
POST /api/auth/login {password} → 204 | 401     POST /api/auth/logout → 204
GET  /api/auth/me → {authenticated}             PUT  /api/auth/password {oldPassword,newPassword}

GET    /api/directories → [{id,name,parentId,orderNo,problemCount,children:[…]}]（树）
POST   /api/directories {name,parentId|null}    PUT /api/directories/:id {name?,parentId?,orderNo?}
DELETE /api/directories/:id

GET  /api/problems?q&tags=a,b&type&dirId&recursive → [{id,type,title,tags,directoryId,timeLimitMs,memoryLimitMiB}]
POST /api/problems（完整 payload，服务端归一化） GET/PUT/DELETE /api/problems/:id
PUT  /api/problems/:id/solutions {solutions}

GET  /api/tags → [{tag,count}]
POST /api/images multipart(file) → {url}        GET /api/uploads/* 静态

POST /api/import?mode=problems|training|practice  multipart(zip)
GET  /api/export/problems?ids=1,2 | ?dirId=&tags=&q=&type=
GET  /api/export/trainings/:id                  GET /api/export/practices/:id

CRUD /api/trainings[/:id]；POST /api/trainings/:id/chapters {title}
PUT/DELETE /api/chapters/:id；POST /api/chapters/:id/items {problemIds}（追加）
PUT  /api/chapters/:id/items {itemIds}（整表重排）；DELETE /api/items/:id
CRUD /api/practices[/:id]；POST /api/practices/:id/items {problemIds,score?}
PUT  /api/practices/:id/items {itemIds}；PUT/DELETE /api/practice-items/:id
```

导入语义：mode=training 时用 meta.title/tags（缺省「导入的训练」）建训练并按下标映射章节条目；mode=practice 时按数组顺序建平铺练习。

首启引导：密码默认 `123456`（bcrypt 入库），控制台打印 BOOTSTRAP 提示；`-seed` 标志在库为空时导入 `samples/orangeoj-sample.zip`。

## 6. 前端信息架构

两栏布局（左 340px 固定 + 右自适应，移动端左栏可折叠）：

- 左栏：操作区（新建题目/新建目录/导入/导出下拉）→ 搜索框 + 类型过滤 + 标签 chips 多选 → 「全部题目」根节点 + 目录树（展开折叠、重命名/新增子目录/删除）→ 当前范围题目列表（复选框 + 类型徽标 + 标签）→ 底部「训练」「练习」两个分区入口
- 右栏：空态统计页 / 题目详情（面包屑、标题、类型徽标、标签、时限；Tabs 题面|答案|题解|编辑）/ 训练详情（章节+条目排序增删）/ 练习详情（平铺条目+分值）
- 批量选择条：加入训练 / 加入练习 / 导出选中 / 删除
- Markdown 渲染链路与上游一致：marked → DOMPurify → KaTeX auto-render

## 7. 验收标准

1. `go vet ./... && go build ./...` 通过；`web` `npm run build` 通过
2. zipio 单测：BuildZip→ParseZip 往返保持 problems.json 语义等价、trainingPlan.json 下标映射正确、图片打包/重写正确
3. server httptest 冒烟：登录→建目录→建三种题型→按标签筛选→导出 ZIP→清库重导入→计数一致
4. 手工冒烟：dev 启动后浏览器可用两栏界面完成一次完整编辑与导入导出
