# OrangeRepo 设计规格

日期：2026-08-22 · 状态：已确认（用户选定 Go+Fiber 后端、单用户、纯题库管理）
修订：2026-08-22b（v1.1.0）——移除目录结构，改为斜杠嵌套标签 + 标签树侧边栏；目录数据丢弃，标签子树整体重命名/删除
修订：2026-08-22f（v1.6.0）——题册目录（可嵌套）+ 题册栏移位/重建入口 + 练习拖拽排序 + 标签计数改为与选中集无关
修订：2026-08-22g（v1.7.0）——目录可拖拽改层级/顺序（layout 原子接口）+ 练习移除分值语义 + 行尾菜单宽窄与锚点闪现修复

## 1. 目标与边界

构建题库管理 Web 应用 **OrangeRepo**：

- 斜杠嵌套标签（`数学/几何/圆`）+ 标签树管理题目（v1.1.0 起取代目录树；目录功能已移除）
- 标签搜索与筛选：选中父标签前缀包含全部子孙标签
- 两栏布局：左栏 = 管理（新建/上传/下载/搜索/标签树/题目列表）；右栏 = 浏览（题面/答案/题解 查看+编辑）
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
problems(id PK, type TEXT NOT NULL, title TEXT NOT NULL,
         tags_json TEXT DEFAULT '[]', statement_md TEXT DEFAULT '',
         body_json TEXT DEFAULT '{}', answer_json TEXT DEFAULT '{}',
         solutions_json TEXT DEFAULT '[]',
         time_limit_ms INT DEFAULT 1000, memory_limit_mib INT DEFAULT 256,
         created_at DATETIME DEFAULT CURRENT_TIMESTAMP)
trainings(id PK, title, description DEFAULT '', tags_json DEFAULT '[]', folder_id NULL REFERENCES booklet_directories(id) ON DELETE SET NULL, created_at)
training_chapters(id PK, training_id INT NOT NULL, title, order_no INT DEFAULT 0)
training_items(id PK, chapter_id INT NOT NULL, problem_id INT NOT NULL, order_no INT DEFAULT 0)
practices(id PK, title, description DEFAULT '', tags_json DEFAULT '[]', folder_id NULL REFERENCES booklet_directories(id) ON DELETE SET NULL, created_at)
practice_items(id PK, practice_id INT NOT NULL, problem_id INT NOT NULL, order_no INT DEFAULT 0)
booklet_directories(id PK, name TEXT NOT NULL, parent_id NULL REFERENCES booklet_directories(id) ON DELETE SET NULL, order_no INT DEFAULT 0, created_at)
```

**标签层级语义（v1.1.0）**：标签为任意字符串，`/` 分隔层级（`数学/几何/圆`）。校验：trim 后非空、无空段、首尾不得为 `/`。树节点 = 现存字面标签 ∪ 其虚拟祖先前缀。

**筛选命中规则**：选中集 S，题目匹配 ⇔ ∀s∈S，题目至少有一个标签 t 满足 `t==s` 或 `strings.HasPrefix(t, s+"/")`。

删除语义（v1.1.0 起）：~~删除目录~~（功能移除）；删除题目 → 级联清理 training/practice 条目；删除训练/练习 → 级联章节与条目；不删题目本体。

**v1.0→v1.1 迁移**：检测到旧库含 `directories` 表时，事务内重建 problems 表（去掉 `directory_id` 列）并 `DROP TABLE directories`；目录数据按用户决策直接丢弃。

## 5. API 契约

除 `/api/auth/login|me` 外全部要求会话 Cookie（`orange_session`）。

```
POST /api/auth/login {password} → 204 | 401     POST /api/auth/logout → 204
GET  /api/auth/me → {authenticated}             PUT  /api/auth/password {oldPassword,newPassword}

（v1.1.0 起目录 CRUD 与题目移动接口已移除）

GET  /api/problems?q&tags=a,b&type → [{id,type,title,tags,timeLimitMs,memoryLimitMiB}]
     tags 命中按前缀规则；tags 参数值为完整路径，逗号分隔 AND
POST /api/problems（完整 payload，服务端归一化） GET/PUT/DELETE /api/problems/:id
PUT  /api/problems/:id/solutions {solutions}

GET  /api/tags[?q&tags&type] → {tags:[{tag,count}], total}
     标签计数（v1.6.0 起）：count=在当前基底过滤（q/类型）下，按前缀规则命中该标签（含子孙；`__none__` 计无标签题）的题目数，
     与当前选中集无关——每个标签始终显示「点它之后能筛出的题目数」；total=当前完整过滤（含全部选中标签，AND + 前缀规则）的题目数。
PATCH  /api/tags {from,to} → {updated}   重命名 from→to：精确匹配重写 + `from+"/"` 前缀子树整体搬家；与现存标签重复时去重合并；返回受影响题数
DELETE /api/tags?tag=… → {updated}       删除该标签及其全部前缀子孙，从所有题目上移除；返回受影响题数
GET/PUT /api/tag-order →/← {order}       （v1.5.0）手动排序持久化：order 为 {"<父路径>":["子标签",...]}，"" 表示顶层

POST /api/images multipart(file) → {url}        GET /api/uploads/* 静态

（v1.6.0 题册目录）
GET  /api/booklet-directories → {directories:[{id,name,parentId,orderNo}]}   扁平列表，parentId=null 表示根
POST /api/booklet-directories {name,parentId?} → {id}
PUT  /api/booklet-directories/layout {directories:[{id,parentId,orderNo}]} → 204
     （v1.7.0）目录拖拽改层级/顺序的原子提交：须恰好覆盖全部目录、parentId 不得指向自身、
     父链无环（不能移入自己的子孙），任一校验失败整体回滚
PATCH/DELETE /api/booklet-directories/:id（重命名 {name}/删除）
     删除语义：直接子目录与归属题册（训练/练习）上移一层挂到被删目录的父级，不删除任何题册
PUT  /api/trainings/:id/folder {folderId:number|null} → 204   训练移入目录（null=根目录）
PUT  /api/practices/:id/folder {folderId:number|null} → 204   练习移入目录（null=根目录）
     训练/练习列表响应带 folderId；POST /api/trainings|practices 可带 folderId 直接落入目录

POST /api/import?mode=problems|training|practice  multipart(zip)
GET  /api/export/problems?ids=1,2 | ?tags=&q=&type=
GET  /api/export/trainings/:id                  GET /api/export/practices/:id

CRUD /api/trainings[/:id]；POST /api/trainings/:id/chapters {title}
PUT/DELETE /api/chapters/:id；POST /api/chapters/:id/items {problemIds}（追加）
PUT  /api/chapters/:id/items {itemIds}（整表重排）；DELETE /api/items/:id
PUT  /api/trainings/:id/layout {chapterIds:[…], chapters:[{chapterId,itemIds:[…]}]}
     → {chapters}（v1.3.0：拖拽布局原子提交；chapterIds 须为章节全排列，
       chapters 须覆盖全部章节且条目并集恰为该训练全部条目，否则 400）
CRUD /api/practices[/:id]；POST /api/practices/:id/items {problemIds}（追加）
PUT  /api/practices/:id/items {itemIds}；DELETE /api/practice-items/:id
     （v1.7.0：删除分值语义——practice_items 不再有 score，OrangeOJ 练习格式无分值）
```

导入语义：mode=training 时用 meta.title/tags（缺省「导入的训练」）建训练并按下标映射章节条目；mode=practice 时按数组顺序建平铺练习。

首启引导：密码默认 `123456`（bcrypt 入库），控制台打印 BOOTSTRAP 提示；`-seed` 标志在库为空时导入 `samples/orangeoj-sample.zip`。

## 6. 前端信息架构

两栏布局（左 340px 固定 + 右自适应，移动端左栏可折叠）：

- 左栏（v1.1.0）：操作区（新建题目/导入/导出下拉/退出登录）→ 搜索框 + 类型过滤 → **标签树面板**：标题行（命中 total 徽章、名称/数量排序切换、展开折叠全部）→ 已选标签 chips（完整路径，可单独移除与一键清空）→ 树体：层级缩进 + 虚拟父节点淡显、点击行切换选中（AND，前缀含子孙）、hover 行尾 ⋮ 菜单提供**重命名**（Dialog 预填完整路径）/ **删除**（确认框提示影响题数与子树规模）、标签多于阈值时提供树内查找过滤 → 当前范围题目列表（复选框 + 类型徽标 + 标签）
- 中栏：题目列表（操作区 = 「+题目」/ 导入 / 导出 / 题册栏开关）；「题册」按钮展开**题册栏**（v1.6.0 起位于题目栏右侧，紧邻详情区）
- **题册栏**（v1.6.0；目录拖拽 v1.7.0）：标题行 = 「题册」计数 + **「+题册」下拉**（新建训练/新建练习/新建目录，落入当前选中目录）+ 搜索；树体 = 可嵌套目录（点击行展开/收起并选中为新建落点，hover ⋮ 菜单：新建子目录/重命名/删除——删除时子目录与归属题册上移一层；**拖动目录行**：上/下边缘=调整同级顺序，中间=改为目标目录的子目录，底部投放区=移到顶层）+ 训练/练习题册行（拖拽可移入目录或根区域）
- 右栏：空态统计页 / 题目详情（标题、类型徽标、标签、时限；Tabs 题面|答案|题解|编辑）/ 训练详情（章节+条目排序增删）/ 练习详情（平铺条目，鼠标拖拽调整顺序）
- 编辑器标签输入：chip 式输入框，回车/逗号添加 token（允许 `/`），建议列表来自现存标签
- 批量选择条：加入训练 / 加入练习 / 导出选中 / 删除
- Markdown 渲染链路与上游一致：marked → DOMPurify → KaTeX auto-render

## 7. 验收标准

1. `go vet ./... && go build ./...` 通过；`web` `npm run build` 通过
2. zipio 单测：BuildZip→ParseZip 往返保持 problems.json 语义等价、trainingPlan.json 下标映射正确、图片打包/重写正确
3. store 单测：前缀筛选与分面计数精确断言（含虚拟父节点）、标签重命名合并去重、子树删除、旧库迁移（directories→无目录）数据完好
4. server httptest 冒烟：登录→建三种题型（含斜杠标签）→按标签筛选→重命名/删除标签联动→导出 ZIP→清库重导入→计数一致
5. 手工冒烟：dev 启动后浏览器可用两栏界面完成一次完整编辑与导入导出
