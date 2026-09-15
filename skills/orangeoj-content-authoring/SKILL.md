---
name: orangeoj-content-authoring
description: Use when generating, editing, or importing OrangeOJ (橙子OJ) 内容——题目、训练（章节）、练习、题册目录、标签或 OrangeOJ ZIP 导入包，包括 AI 批量生成题目 JSON、编写 trainingPlan.json、上传图片、用 curl 调 /api/problems 与 /api/import，或需要确认 bodyJson/answerJson 形状、判题类型枚举、时限单位与标签层级规则时。
---

# OrangeOJ 内容创作（题目 / 训练 / 练习 / ZIP 导入）

## 0. 这个 skill 解决什么

按 OrangeOJ **真实的数据格式**生成教学内容并导入：

- 题目 JSON（三种 `type` 的 `bodyJson` / `answerJson` 形状）
- 训练 JSON（`trainingPlan.json`：章节 + 章节内条目）
- 练习 JSON（平铺条目）
- OrangeOJ ZIP 交换包（`problems.json` + `trainingPlan.json` + `images/`）
- 通过 `curl` 调管理 API 落库并验证

**权威来源与优先级**：本 skill 全部字段来自 `internal/store/`、`internal/zipio/zipio.go`、
`internal/server/` 的代码，并与 `docs/api-reference.md` 交叉核对。
**文档与代码不一致时以代码为准**（`docs/api-reference.md` 未随重构更新的地方在本 skill 第 9 节列出）。

**不在范围内**：`/api/space/*`（空间训练/练习/刷题，学生侧）与 `internal/quizstore`（作答数据）。
本 skill 只涉及**仓库层**（repo 层）：`/api/problems`、`/api/trainings`、`/api/practices`、
`/api/booklet-directories`、`/api/tags`、`/api/import`、`/api/export/*`。
不要改 `internal/`、`app/`、`docs/`；内容创作只需产出 JSON + 调 API。

---

## 1. 核心约定（先读，能挡掉 80% 的错误）

### 1.1 认证

所有管理端点都要会话 Cookie（`POST /api/auth/login` 除外）。

| 项 | 值 | 依据 |
| --- | --- | --- |
| Cookie 名 | `orange_session` | `internal/server/server.go:21` |
| 默认管理员 | `admin` / `123456` | `internal/server/auth.go:18-21` |
| 本地地址 | `http://localhost:8080` | `main.go:23`（`-addr :8080`） |
| 未认证响应 | `401 {"error":"unauthorized"}` | `internal/server/auth.go:31-38` |

**登录体必须同时带 `username` 和 `password`**（不是只有 `password`）：

```go
// internal/server/auth.go:95-106
type loginRequest struct {
    Username string `json:"username"`
    Password string `json:"password"`
}
if ... || strings.TrimSpace(req.Username) == "" || req.Password == "" {
    return respondError(c, fiber.StatusBadRequest, "invalid request")
}
```

只传 `{"password":"123456"}` → **`400 invalid request`**（`docs/api-reference.md:11`
写的 `{password}` 已过时，以代码为准）。成功后返回 `204` + `Set-Cookie: orange_session=…`
（`internal/server/auth.go:111-123`，有效期 30 天）。用户名错误返回
`401 用户名或密码错误`（同文件 `107-110`）。

会话失效时机（**别再假设"重新登录会踢掉旧会话"**）：

- `CreateSession` 只做 `INSERT`，**不删除**旧会话（`internal/accounts/accounts.go:624-635`）。
  所以脚本登录一次不会把浏览器里的会话踢下线；但为了简单，仍建议**登录一次、全程复用
  `cookies.txt`**（并发登录本身无害，只是多几条会话行）。
- 真正会清空会话的是改密码、删除/重命名用户、重置密码等账号变更操作
  （`DELETE FROM sessions` 出现在 `accounts.go:487/508/548/570/610`）。
  `docs/api-reference.md:18` 与 `docs/api-reference.md:203` 的"单会话模型/重复登录踢掉旧会话"
  描述已过时，以代码为准。

权限门槛（`internal/server/auth.go:40-58`）：

- 本 skill 用到的管理端点都要**管理员角色**。登录成功但账号是 `member` 时，调管理 API 仍会得到
  `401 {"error":"unauthorized"}`——这**不是**会话失效，而是角色不够，别误判成要重新登录。
- `/api/admin/domains` 的写操作、`/api/admin/all-users` 额外要求**系统管理员**，
  域管理员访问返回 `403 {"error":"需要系统管理员权限"}`。

### 1.2 题型枚举（只有 3 个值，必须小写）

```go
// internal/model/model.go:13-17
TypeProgramming  ProblemType = "programming"
TypeSingleChoice ProblemType = "single_choice"
TypeTrueFalse    ProblemType = "true_false"
```

- `type` 会被 `strings.ToLower(strings.TrimSpace(...))` 归一化（`internal/zipio/zipio.go:351`），
  但**只接受上面三个值**，其它值报 `invalid problem type`（`internal/zipio/zipio.go:355-357`）。
- 常被写错的错误值：`objective`、`choice`、`singleChoice`、`single-choice`、`multiple_choice`、
  `judge`、`fill_blank`、`coding`、`program` —— 全部不存在。客观题只有 `single_choice` 与 `true_false`。

### 1.3 id 与 uuid：两套标识，用途完全不同

| 标识 | 类型 | 用途 | 何时出现 |
| --- | --- | --- | --- |
| `id` | 整数（AUTOINCREMENT） | **API 路由与条目引用**（`/api/problems/12`、`problemIds:[12]`） | 入库后由数据库分配 |
| `uuid` | UUIDv7 字符串 | **跨库稳定标识**：导入去重 | 建题时自动生成；包内可带 |

- 空 `uuid` 建题 → 服务端自动生成 UUIDv7（`internal/store/store.go:479-489`）。
- **库内引用一律用整数 `id`**；`uuid` 只在 ZIP 包与跨库场景有意义（`internal/server/io_handlers.go:148-202`）。
- 但 `trainingPlan.json` 里的 `problemIds` **既不是 id 也不是 uuid**，是 `problems.json`
  **数组下标**，见 4.2。

### 1.4 标签：不需要预先创建，斜杠即层级

标签**不是独立实体**，没有标签表、没有先建后用的约束：

- 存法：题目的 `tags_json` 列直接存一个字符串数组（`internal/store/store.go:533-539`）。
- 建题/改题**不校验标签是否存在**，写什么就是什么（`zipio.NormalizeProblemPayload`
  根本不动 `Tags`，`internal/zipio/zipio.go:349-391`）。
- 标签树是**读时聚合**出来的：= 现存字面标签 ∪ 虚拟祖先前缀
  （`数学/几何/圆` 让 `数学`、`数学/几何` 也出现在标签树里，`internal/store/store.go:943-950`）。
- 层级分隔符是 `/`，**用 `/` 表达子级，不要用 `.`、`-`、`>`**。路径校验规则（仅用于
  重命名/删除接口）：非空、首尾不得是 `/`、不得有空层级（`internal/store/store.go:799-814`）。
- **一个数组元素 = 一个完整路径**。想要"入门"下的"数组"，要写 `["入门/数组"]`；
  写成 `["入门","数组"]` 会得到**两个并列的顶层标签**（标签树里也是两个平级节点），
  不会产生父子关系（依据：`ListTagFacets` 只从字符串里的 `/` 推导祖先，`store.go:943-950`）。
- 命名空间里没有"先创建标签"这种操作：`/api/tags` 只有 `GET`（列表+计数）、
  `PATCH`（重命名）、`DELETE`（删除），**没有 POST**（`internal/server/server.go:113-117`）。
- 过滤是**前缀命中**：选中 `数学` 会命中标签为 `数学` 或 `数学/...` 的题；多标签之间是 **AND**
  （`internal/store/store.go:616-641`）。
- 标签文本里**可以含逗号**：`tags` 查询参数按"重复参数"解析（`?tags=a&tags=b`），不要用逗号拼
  （`internal/server/problems.go:18-25`；这点与 `docs/api-reference.md:29` 不同，以代码为准）。
- `__none__` 是保留哨兵（表示"无任何标签"），不能重命名/删除，不要真的给题目打这个标签
  （`internal/store/store.go:614`、`internal/server/tags.go:23-24`、`39-41`）。

### 1.5 图片引用

**两种写法，用错场景会导致图片丢失：**

| 写在哪 | 正确写法 | 原因 |
| --- | --- | --- |
| API 载荷（`POST /api/problems`）与库内题面 | `/api/uploads/<file>` | 上传接口返回的就是这个 URL（`internal/server/images.go:46`） |
| ZIP 包内 `problems.json`（可选） | `/api/uploads/<file>`（推荐）或 `(images/<file>)` | 前者是导出包的真实形态；后者导入时会被重写（`internal/zipio/zipio.go:99-115`） |

- 可打包的图片扩展名只有 **png / jpg / jpeg / gif / webp**；引用正则见
  `internal/zipio/zipio.go:26`。**SVG 不在内**（上传接口也不收，`internal/server/images.go:14-16`），
  所以 SVG 图片既不能随包也不能被 cleanup 识别。
- 图片"被引用"的判定范围 = 题目四个文本字段：`statementMd`、`bodyJson`、`answerJson`、`solutions`
  （`internal/server/images.go:54-74`）。
- `images/` 里的文件名 → 导入后会被**改名成 16 位 nano 随机名**并重写题面引用
  （`internal/server/io_handlers.go:133-162`；测试 `internal/server/server_test.go:316-331`）。
  所以不要依赖包内图片名在导入后保持不变。
- 上传：`POST /api/images`，multipart 字段名 `file`，返回 `{"url":"/api/uploads/xxxx.png","path":...}`
  （`internal/server/images.go:22-47`）。
- **图片不随包 = 图片丢失**。ZIP 里必须真的带 `images/<file>`，只写引用不打包是无效的。

### 1.6 时限与内存单位

- `timeLimitMs`：**毫秒**（整数），默认 `1000`。
- `memoryLimitMiB`：**MiB**（整数），默认 `256`。
- 只在 `type=programming` 时有意义，且只有编程题会被兜默认值与校验上限
  （`internal/zipio/zipio.go:358-372`）。
- 硬上限：`timeLimitMs ≤ 15000`、`memoryLimitMiB ≤ 2048`，超出报错
  （`internal/zipio/zipio.go:365-371`）。
- 客观题不写这两个字段（写了也不会被使用；前端保存客观题时传 `undefined`，
  `app/src/pages/admin/ProblemPane.tsx:179-180`）。
- 单位陷阱：把 1000ms 写成 `1`（当成"1 秒"）会变成 1 毫秒；把 256 写成 `268435456`（当成字节）
  会直接被上限拒绝。

### 1.7 题解语言枚举

`solutions[].language` 会被归一化（`internal/zipio/zipio.go:450-463`）：

| 输入别名 | 归一化结果 |
| --- | --- |
| `c++`, `cpp`, `c` | `cpp` |
| `python`, `python3`, `py`, `python 3` | `python` |
| `go`, `golang` | `go` |
| `turtle`, `python turtle`, `pythonturtle` | `turtle` |
| 其它 | 原样小写 |

**归一化后为空的项会被静默丢弃**（`internal/zipio/zipio.go:432-435`）；
`solutions` 不是数组会报 `solutions must be a JSON array`（同文件 `413-421`）。
写题解就用 `cpp` / `python`，不要写 `C++`（虽能归一化，但导出后统一成 `cpp`）。

### 1.8 域（domain）——建题必须落到某个域

题目归属于"域"，域之间互相隔离。对 AI 调用方的实际影响：

- `global_admin`（系统管理员）：可用 `?domainId=<ID>` 指定域；不带则落到**默认域**
  （不存在会自动创建，名称为 `默认域`，`internal/server/domain_admin.go:45-76`、
  `internal/store/store.go:197-221`）。
- `domain_admin`（域管理员）：**忽略** `domainId`，强制写入自己所属域
  （`internal/server/domain_admin.go:48-53`）。
- `domain_admin` 建题若未关联域会报 `缺少 domainId（请先选择域）`
  （`internal/server/problems.go:90-94`）。
- 读取/更新/删除别人域的题 → `404 problem not found`（不是 403，
  `internal/server/problems.go:132-148`）。

---

## 2. 题目格式

### 2.1 载荷字段全集

创建与更新用**同一形状**（`internal/zipio/zipio.go:66-79`，服务端做归一化）：

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `type` | string | ✅ | `programming` / `single_choice` / `true_false` |
| `title` | string | ✅ | trim 后不得为空（`zipio.go:350-354`） |
| `tags` | string[] | 建议 | 斜杠层级；可空数组 |
| `statementMd` | string | 建议 | 题干 Markdown，支持 KaTeX `$...$` |
| `bodyJson` | object | 建议 | 按题型，见下 |
| `answerJson` | object | 客观题必填 | 按题型，见下 |
| `solutions` | array | 可选 | `[{language, code, markdown}]`；缺省 `[]` |
| `starterCpp` | string | 可选 | 编程题 C++ 起始模板，空=用通用模板 |
| `starterPy` | string | 可选 | 编程题 Python 起始模板 |
| `timeLimitMs` | int | 编程题 | 毫秒，默认 1000，上限 15000 |
| `memoryLimitMiB` | int | 编程题 | MiB，默认 256，上限 2048 |
| `uuid` | string | 可选 | 留空则服务端生成 UUIDv7 |

服务端归一化保证：`bodyJson` 缺省 `{}`、`answerJson` 缺省 `{}`、`solutions` 缺省 `[]`
（`internal/zipio/zipio.go:373-381`）。

响应：`POST /api/problems` → `201 {"problem": Problem}`；完整 `Problem` 比载荷多
`id`、`domainId`、`createdAt`（`internal/model/model.go:39-55`）。

### 2.2 `programming`：`bodyJson` 形状

```jsonc
{
  "inputFormat": "一行两个整数 $a$ 与 $b$",     // string，可含 Markdown
  "outputFormat": "一行一个整数",              // string
  "samples":   [{ "input": "1 2", "output": "3" }],   // 展示用样例
  "testCases": [{ "input": "1 2", "output": "3" }]    // 判题用测试点
}
```

- 权威结构：`internal/judge/types.go:117-129`（`ProgrammingBody` / `ProgrammingCase`）。
- 判题取用规则（`internal/judge/types.go:131-166`）：
  - `submit`（提交评测）→ 用 `testCases`；为空则**回退 `samples`**；
  - `test`（自测）→ `testCases` → 回退 `samples` → 回退用户输入；
  - `run`（运行）→ 只用用户自定义输入。
  - **推论**：想让隐藏测试点真正生效，必须填 `testCases`；只填 `samples` 时学生提交会拿样例判分。
- 输出比较前会归一化：`\r\n`→`\n`、逐行去行尾空白、整体 trim（`internal/judge/types.go:168-177`），
  所以期望输出末尾的空行不影响判题。
- `answerJson` 编程题恒为 `{}`（前端也是这么存的，`app/src/pages/admin/ProblemPane.tsx:161-163`）。
- `samples` / `testCases` 是**数组**（不是单个对象），元素键名是 `input` / `output`（不是
  `expected`、`expectedOutput`、`in`、`out`）。
- 起始代码 `starterCpp` / `starterPy` 是**题目顶层字段**，不是 `bodyJson` 里的
  （`internal/model/model.go:50-51`；学生侧取用顺序见 `app/src/lib/use-programming-workspace.ts:25-31`）。

### 2.3 `single_choice`：`bodyJson` / `answerJson` 形状

```jsonc
{
  "bodyJson":   { "options": ["选项文本1", "选项文本2", "选项文本3", "选项文本4"] },
  "answerJson": { "answerIndex": 2 }        // 0-based，指向 options 下标
}
```

- `answerIndex` 是 **0-based 下标**（`internal/zipio/zipio.go:481-486`；前端显示时再加 A/B/C/D，
  `app/src/pages/admin/ProblemPane.tsx:582`、`602`）。
- `options` 里**只写选项正文，不要自带 `A.` `B.` 前缀**：界面自动编号
  （`app/src/pages/admin/ProblemPane.tsx:582`）；官方示例包也是纯文本
  （`samples/orangeoj-sample.zip` 内 `problems.json`）。
  注意 `docs/api-reference.md:60` 的示例写了 `"A. …"`，那是文档笔误——以代码与示例包为准。
- 兼容写法：`answerJson` 也可传 `{"answer":"<选项原文>"}`，服务端会按
  **忽略大小写的 trim 全等**匹配选项文本并改写成 `answerIndex`
  （`internal/zipio/zipio.go:487-498`）。**推荐直接用 `answerIndex`**，避免匹配失败。
- 匹配失败或下标越界**不会报错**，只是保持原值（`internal/zipio/zipio.go:481`）——
  也就是说写错答案不会被拒绝，会静默留下一个无法判对的题。生成后必须自检。
- `options` 为空数组时直接跳过归一化（`internal/zipio/zipio.go:478-480`）。

### 2.4 `true_false`：`bodyJson` / `answerJson` 形状

```jsonc
{
  "bodyJson":   {},
  "answerJson": { "answer": true }          // 布尔值
}
```

- 键名必须是 `answer`，值是 JSON 布尔（`internal/zipio/zipio.go:499-521`）。
- **不要写 `options`**：判断题的作答界面固定渲染「正确 / 错误」两个按钮
  （`app/src/components/portal/objective.tsx:91,121` 只在 `single_choice` 分支渲染 `options`），
  写了不会被使用，反而容易让题面重复。
- **解析写 `solutions[].markdown`**：说明为什么对/错、常见混淆点（见 `examples/problem-true-false.json`）。
  题干里不要塞答案，`answerJson` 只放布尔值。
- 容错键：`answer` / `correct` / `correctAnswer` / `value` 任一命中即取用，并统一写成 `answer`
  （`internal/zipio/zipio.go:504`）。
- 值容错（`internal/zipio/zipio.go:589-605`）：`true/1/"yes"/"对"/"正确"` → `true`；
  `false/0/"no"/"错"/"错误"` → `false`；数字非 0 → `true`。
  **推荐直接写 `true` / `false`**，不要写 `"是"`、`"T"`、`"A"`（这些不认，且不报错）。
- 其它键会被保留（`answer` 之外的键不会丢，见 `zipio.go:513-518`）。

### 2.5 最小可用题目示例

见 `examples/problem-programming.json`、`examples/problem-objective.json`（单选）与
`examples/problem-true-false.json`（判断，含解析写法）。以下是最小可提交载荷（可直接复制）：

> **判断题的"解释"写在哪**：写在 `solutions[].markdown`（题解/解析区），不要写进 `statementMd`
> 或 `answerJson`。`answerJson` 只放布尔答案（`{"answer": true|false}`）；`markdown` 里说明
> 为什么对/错、以及常见混淆点。见 `examples/problem-true-false.json`。

```json
{
  "type": "programming",
  "title": "两数之和",
  "tags": ["入门/顺序结构"],
  "statementMd": "给定两个整数 $a$ 与 $b$，输出 $a+b$。",
  "bodyJson": {
    "inputFormat": "一行两个整数",
    "outputFormat": "一个整数",
    "samples": [{ "input": "1 2", "output": "3" }],
    "testCases": [{ "input": "1 2", "output": "3" }]
  },
  "answerJson": {},
  "solutions": [{ "language": "cpp", "code": "int main(){}", "markdown": "略" }],
  "timeLimitMs": 1000,
  "memoryLimitMiB": 256
}
```

```json
{
  "type": "single_choice",
  "title": "下列哪个不是合法的 C++ 标识符？",
  "tags": ["语法基础"],
  "statementMd": "下列哪个不是合法的 C++ 标识符？",
  "bodyJson": { "options": ["_count", "2nd_value", "value2"] },
  "answerJson": { "answerIndex": 1 },
  "solutions": []
}
```

```json
{
  "type": "true_false",
  "title": "C++ 是编译型语言。",
  "tags": ["语法基础"],
  "statementMd": "C++ 是编译型语言。",
  "bodyJson": {},
  "answerJson": { "answer": true },
  "solutions": []
}
```

---

## 3. 训练格式（章节结构）

训练 = **章节（chapter）→ 条目（item）→ 题目**，最多两层。

### 3.1 数据结构

```jsonc
// GET /api/trainings/:id →
{
  "training": { "id": 1, "uuid": "0199…", "title": "第 1 周训练", "description": "…",
                "tags": ["第1周"], "folderId": 2, "problemCount": 3, "createdAt": "…" },
  "chapters": [
    { "id": 1, "trainingId": 1, "title": "热身", "orderNo": 1,
      "items": [
        { "id": 10, "chapterId": 1, "problemId": 5, "orderNo": 1,
          "problemTitle": "A+B", "problemType": "programming" }
      ] }
  ]
}
```

依据：`internal/model/model.go:90-107`（`Chapter` / `Item`）、
`internal/store/store_trainings.go:365-412`（列表与联表取题目标题/类型）。
条目**引用题目的整数 `id`**（`problemId`），不是 uuid。

### 3.2 训练相关路由

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/trainings` | 列表（含 `problemCount`）；可带 `?domainId=` |
| POST | `/api/trainings` | `{"title","description","tags","folderId"}` → `201 {"id"}` |
| GET | `/api/trainings/:id` | `{"training":…,"chapters":[…]}` |
| PUT | `/api/trainings/:id` | `{"title","description","tags"}` → `204`（`title` 必填） |
| DELETE | `/api/trainings/:id` | `204` |
| PUT | `/api/trainings/:id/folder` | `{"folderId":1}` 或 `{"folderId":null}` → `204` |
| POST | `/api/trainings/:id/chapters` | `{"title"}` → `201 {"id"}`（排到末尾） |
| PUT | `/api/chapters/:id` | `{"title","orderNo"}` → `204`（`title` 必填） |
| DELETE | `/api/chapters/:id` | `204` |
| POST | `/api/chapters/:id/items` | `{"problemIds":[5,6]}` → `201 {"itemIds":[10,11]}`（追加到末尾） |
| PUT | `/api/chapters/:id/items` | `{"itemIds":[11,10]}` → `204`（整章重排，须覆盖全部条目） |
| DELETE | `/api/items/:id` | `204`（删条目，不删题目） |
| PUT | `/api/trainings/:id/layout` | 拖拽原子提交，见 3.3 |

路由依据：`internal/server/server.go:201-213`；handler：`internal/server/trainings_practices.go`。

排序规则：

- 章节顺序 = `training_chapters.order_no`，新建章节自动排末尾
  （`internal/store/store_trainings.go:144-157`）。
- 条目顺序 = `training_items.order_no`，新建条目自动 `MAX+1`
  （`internal/store/store_trainings.go:187-221`）。
- 读取一律 `ORDER BY order_no, id`（`store_trainings.go:366`、`398`）。

### 3.3 `layout`（拖拽整体重排）的硬校验

`PUT /api/trainings/:id/layout`：

```json
{
  "chapterIds": [1, 2],
  "chapters": [
    { "chapterId": 1, "itemIds": [10, 11] },
    { "chapterId": 2, "itemIds": [] }
  ]
}
```

规则（`internal/store/store_trainings.go:258-350`，全部校验失败即整体回滚 → 400）：

1. `chapterIds` 必须是该训练**全部章节的一个排列**（无重复、无遗漏、无外来 id）；
2. `chapters` 必须**恰好覆盖全部章节**（长度相等且 chapterId 都属于本训练）；
3. 所有 `itemIds` 的**并集必须恰好等于该训练全部条目**（无重复、无遗漏、无外来 id）；
4. 支持跨章节移动条目（会改写条目的 `chapter_id`）。

> 只想改顺序时别用 `layout`，用 `PUT /api/chapters/:id/items`（单章）。

### 3.4 条目引用与"删题"陷阱

- 条目引用**整数 `problemId`**；`trainingPlan.json` 里的 `problemIds` 是**数组下标**，见 4.2。
- `POST /api/chapters/:id/items` 会**静默跳过不存在的题目**
  （`internal/store/store_trainings.go:200-207`）——传错 id 不会报错，只返回少几个 `itemIds`。
  **必须核对返回的 `itemIds` 长度**。
- 删除训练会级联删除章节与条目，并且**把"仅被该训练引用"的题目一起删掉**；
  被其它训练/练习也引用的题目会保留（`internal/store/store_trainings.go:109-141`、
  `internal/store/store.go:763-782`）。
- 删除单道题会从**所有**训练/练习里摘掉它的条目（条目消失，训练本体保留，
  `internal/store/store.go:742-761`）。

### 3.5 训练的 `tags`

`trainings.tags_json` 与题目标签是**同一套字符串数组格式**，但**互相独立**：
训练的 `tags` 不影响题目标签树，也不会自动继承题目的标签
（`internal/store/store_trainings.go:84-95`）。

---

## 4. 练习格式（平铺）

### 4.1 与训练的结构差异

| 维度 | 训练（training） | 练习（practice） |
| --- | --- | --- |
| 层级 | 训练 → 章节 → 条目（两层） | 练习 → 条目（**一层，平铺**） |
| 数据表 | `trainings` / `training_chapters` / `training_items` | `practices` / `practice_items` |
| 建表 SQL | `internal/store/store.go:96-115` | `internal/store/store.go:116-129` |
| 条目结构体 | `model.Item`（有 `chapterId`） | `model.PracticeItem`（**无** chapterId，有 `practiceId`） |
| Go 定义 | `internal/model/model.go:90-107` | `internal/model/model.go:121-129` |
| 排序接口 | 章节级 `PUT /api/chapters/:id/items` + 整体 `PUT /api/trainings/:id/layout` | `PUT /api/practices/:id/items`（整表重排） |
| 删条目路由 | `DELETE /api/items/:id` | `DELETE /api/practice-items/:id` |
| 分值 | 无分值语义 | **无分值语义**（`score` 列已退役，`internal/store/store.go:149-152`） |

### 4.2 练习路由

| 方法 | 路径 |
| --- | --- |
| GET | `/api/practices`（可带 `?domainId=`） |
| POST | `/api/practices` → `201 {"id"}` |
| GET | `/api/practices/:id` → `{"practice":…,"items":[…]}` |
| PUT | `/api/practices/:id` → `204`（`title` 必填） |
| DELETE | `/api/practices/:id` → `204` |
| PUT | `/api/practices/:id/folder` → `204` |
| POST | `/api/practices/:id/items` `{"problemIds":[…]}` → `201 {"itemIds":[…]}` |
| PUT | `/api/practices/:id/items` `{"itemIds":[…]}` → `204`（**必须覆盖全部条目**） |
| DELETE | `/api/practice-items/:id` → `204` |

路由依据：`internal/server/server.go:215-223`。
`PUT /api/practices/:id/items` 的完整性校验在 `internal/store/store_practices.go:158-177`
（`itemIds must cover all items of the practice`）。

练习条目**也没有** `uuid` 字段（`internal/model/model.go:121-129`），只有整数 `problemId`。 
训练/练习的**整体 `uuid`** 存在（`trainings.uuid` / `practices.uuid`，
`internal/store/store.go:180-187`），用于 `trainingPlan.json` 的 `uuid` 字段。

---

## 5. ZIP 导入 / 导出格式（OrangeOJ ZIP）

### 5.0 两套格式别混用（先分清用哪个入口）

仓库里存在**两套互不通用**的 ZIP 内容格式，按入口决定用哪套：

| 格式 | 清单文件 | 入口 | 用途 |
| --- | --- | --- | --- |
| **OrangeOJ 交换格式**（本 skill 主讲） | `trainingPlan.json` | `POST /api/import?mode=…` | 导入题目/训练/练习；与上游 OrangeOJ 双向兼容 |
| 全库备份格式 | `orangerepo-backup.json` | `POST /api/import/backup` | 整库/整域迁移，可一次带多个训练+练习+目录 |

**内容创作请用 `trainingPlan.json` 格式。** 不要为了拿"训练+练习"而去手写
`orangerepo-backup.json`：那是全库备份清单（`internal/server/backup.go:24-68`），
语义是"恢复整库"（`version` 必须为 `1`，`internal/server/backup.go:264-267`），
且一次 `/api/import` 调用只能产出**一个**训练**或**一个练习（见 5.4）。
训练与练习分开导两个包即可。

### 5.1 目录结构

```
<包根>/
├── problems.json          # 必填，题目数组
├── trainingPlan.json      # 可选，题册（训练/练习）元数据 + 章节下标
└── images/                # 可选，被引用的图片（tarball 内的真实文件）
    ├── example-sum.png
    └── figure-2.jpg
```

真实示例见 `examples/import-package/`（可直接打包导入）与 `samples/orangeoj-sample.zip`。

解析规则（`internal/zipio/zipio.go:241-344`）：

- `problems.json` / `trainingPlan.json` 用**文件名**在 ZIP 内**任意层级**查找
  （`findZipFileByNames`，`zipio.go:242-262`）——放子目录里也能找到。
- `images/` 同理：目录名是 `images` 或以 `/images` 结尾即可（`zipio.go:328-336`）。
- 包根下**其它文件**会被当"扩展文件"忽略（`zipio.go:337-341`）——不要往里塞别的东西当 manifest。
- **没有 manifest 文件**：不要发明 `manifest.json` / `meta.json` / `index.json`，系统不认。

### 5.2 `problems.json`

**顶层是数组**（不是 `{"problems":[…]}`），元素字段 = 2.1 的载荷字段（`ExportProblem`，
`internal/zipio/zipio.go:33-46`）：

```json
[
  {
    "uuid": "0199…-0001",          // 可选；导入去重用
    "type": "programming",
    "title": "两数之和",
    "tags": ["入门/顺序结构"],
    "statementMd": "…![图](/api/uploads/example-sum.png)",
    "bodyJson": { "inputFormat": "…", "outputFormat": "…", "samples": [], "testCases": [] },
    "answerJson": {},
    "solutions": [{ "language": "cpp", "code": "…", "markdown": "…" }],
    "starterCpp": "…",
    "starterPy": "…",
    "timeLimitMs": 1000,
    "memoryLimitMiB": 256
  }
]
```

- 导出侧格式：2 空格缩进、**不转义 HTML**（`zipio.go:119-129`）。手写时可以不管缩进，解析器只要求合法 JSON。
- `solutions` 缺失时按 `[]`，`bodyJson`/`answerJson` 缺失时按 `{}`（`zipio.go:373-381`）。
- 图片引用写在题目**四个文本字段**里，最稳妥用 `/api/uploads/<file>` 形态（见 1.5）。

> **⚠️ 已知缺陷：ZIP 导入会丢掉 `starterCpp` / `starterPy`。**
> 导入代码把它们解析进了 `payload`（`internal/server/io_handlers.go:166`），
> 但构造 `model.Problem` 时**没有拷贝**（`io_handlers.go:172-184`，对比同类代码
> `problems.go:104-105`、`182-183` 都是拷贝的），于是 `CreateProblem` 写入空串。
> 影响：**起始代码只能走 API 落库，ZIP 导入路径静默丢弃**（导出→再导入的往返同样丢）。
> 变通：ZIP 导入后补一次全量替换（`PUT` 会拷贝这两个字段，`problems.go:182-183`）：
>
> ```
> curl -sS -b cookies.txt -X PUT http://localhost:8080/api/problems/<PROBLEM_ID> \
>   -H 'Content-Type: application/json' --data-binary @problem-with-starter.json
> ```
>
> 或者干脆用 `POST /api/problems` 建带起始代码的题（该路径正常保留）。

### 5.3 `trainingPlan.json`

```jsonc
{
  "uuid": "0199…a1",            // 可选：题册自身的稳定标识
  "title": "第 1 周训练",        // 可选：题册名称（优先级最高）
  "description": "…",           // 可选
  "tags": ["第1周"],            // 可选：题册标签
  "chapters": [                 // 关键：非空 => auto 模式判为「训练」
    { "title": "热身", "orderNo": 1, "problemIds": [0, 1] },
    { "title": "巩固", "orderNo": 2, "problemIds": [] }
  ]
}
```

结构依据：`internal/zipio/zipio.go:48-63`（`PlanChapter` / `PlanMeta`）。

**`problemIds` 是 `problems.json` 数组的 0-based 下标**（注释在 `zipio.go:48`，
消费逻辑在 `internal/server/io_handlers.go:233-237`：`if idx >= 0 && int(idx) < len(createdIDs)`，
**越界下标被静默忽略**）。

- **不是**数据库 id，**不是** uuid。写错 → 章节变空且**不报错**，必须自检。
- 越界下标（负数或 ≥ 数组长度）被**静默丢弃**（`io_handlers.go:234-237`）。
- `orderNo` 在导入时**并未被使用**：章节按 `chapters` 数组顺序依次创建，
  新建章节自动排末尾（`io_handlers.go:226-244` + `store_trainings.go:144-157`）。
  所以**数组顺序才是真实顺序**，`orderNo` 请按 1..N 顺序写以免误导。

### 5.4 导入模式 `mode`

`POST /api/import?mode=…`（`internal/server/io_handlers.go:70-75`）：

| mode | 行为 |
| --- | --- |
| `problems` | 只把题目入库，**不建**训练/练习 |
| `training` | 建训练；包内无章节时自动建一个名为 **`未分组`** 的章节收纳全部题目 |
| `practice` | 建练习（平铺，按 `problems.json` 顺序） |
| `auto`（推荐） | 包内 **非空 `chapters`** → 训练；否则 → 练习 |

- 非法 `mode` → `400 invalid mode: 仅支持 problems|training|practice|auto`（`io_handlers.go:74`）；
  `mode` 缺省值是 `problems`（`io_handlers.go:70` 的 `c.Query("mode","problems")`）。
- `auto` 判据是 `meta != nil && len(meta.Chapters) > 0`（`io_handlers.go:126-132`）：写了
  `trainingPlan.json` 但 `chapters: []` → 判为**练习**。
- **`mode=practice` 会忽略 `chapters`，把 `problems.json` 全部按数组顺序平铺**
  （`io_handlers.go:262-287`），所以"练习包"里 `chapters` 写什么都行，
  但**要用 `mode=practice` 显式声明**——否则见 5.9 的往返陷阱。

### 5.5 题册名称优先级

`trainingPlan.json.title` → **上传文件名**（去扩展名）→ 默认名（`导入的训练`/`导入的练习`）
（`io_handlers.go:207-221`、`262-276`）。
所以 `第3周训练.zip` 不写 `title` 时题册就叫"第3周训练"；文件名带日期等噪声时请显式写 `title`。

### 5.6 导入去重（uuid）与 UUID 冲突

`internal/server/io_handlers.go:148-202`：

- 包内题目**带 `uuid`** 且**同域内已存在该 uuid** → **跳过新建，复用已有题 id**，
  并把该下标槽位指向已存在题目，保证章节/练习引用不错位。
- 包内题目**无 `uuid`** → 每次导入都**新建一道题**（重复导入会产生重复题）。
- **跨域**同 `uuid` → 视为新题（各域独立副本，`io_handlers.go:185` 注释）。
- 复用是"跳过"而非"覆盖"：同 uuid 重导**不会更新**已有题内容，改内容请用 `PUT /api/problems/:id`。

实践建议：批量生成的新内容不要沿用别处抄来的 uuid；要么不写（每次新建），要么用新的唯一 UUIDv7。

### 5.7 图片随包的完整规则

1. 题面写引用：`![图](/api/uploads/example-sum.png)`（推荐）或 `![图](images/example-sum.png)`；
   包内放真实文件 `images/example-sum.png`。只写引用不打包 = 图片丢失。
2. 文件名必须匹配 `/api/uploads/([a-zA-Z0-9_-]+\.(png|jpe?g|gif|webp))`——**只允许字母数字下划线连字符 + 点 + 扩展名**
   （`zipio.go:26`）。中文名、空格、`%` 等会被**直接忽略**（图片丢失，不报错，`zipio.go:329-334`）。
3. 扩展名限 png/jpg/jpeg/gif/webp；单图 > 12MiB 报错（`zipio.go:296-299`、`330-332`）。
4. 导入后图片被改名为 16 位 nano 随机名并重写题面引用（`io_handlers.go:133-162`）。
5. 图片只在**题目的四个文本字段**里被识别为引用（`zipio.go:84-96`）。

### 5.8 ZIP 限额（解压炸弹防护）

`internal/zipio/zipio.go:293-314` + `io_handlers.go:58-60`：

| 项 | 上限 |
| --- | --- |
| 上传 ZIP 文件大小 | 100 MiB |
| ZIP 条目数 | 20000 |
| 单文件展开 | 64 MiB |
| 总展开 | 800 MiB |
| 单张图片 | 12 MiB |

### 5.9 导出接口（用来拿"标准答案格式"）

| 方法 | 路径 | 产出 |
| --- | --- | --- |
| GET | `/api/export/problems?q=&tags=&type=&ids=1,2` | `problems.json`（+ `images/`），**无** `trainingPlan.json` |
| GET | `/api/export/trainings/:id` | `problems.json` + `trainingPlan.json`（章节按下标引用） |
| GET | `/api/export/practices/:id` | `problems.json` + `trainingPlan.json`（单个平铺章节） |

依据：`internal/server/io_handlers.go:315-461`。导出的 `trainingPlan.json` 里
`problemIds` 是导出时重建的下标（`io_handlers.go:383-401`），可放心作为格式参照。
无匹配题目时导出返回 `400 no problems match the filter`（`io_handlers.go:335-337`）。

> **⚠️ 练习包的往返陷阱**：导出的**练习**包其 `trainingPlan.json` 里有**一个**章节
> （`io_handlers.go:452-455`，标题 = 练习名）。把这种包用 `mode=auto` 再导入 → 非空
> `chapters` 命中 → 会建成**训练**而不是练习！重导练习包请显式用 **`mode=practice`**；
> 手写练习包时 `chapters` 留 `[]` 更不容易误判。

> **⚠️ `uuid` 只在包内有意义**：`POST /api/problems` **不会**采用你传的 `uuid`
> （`problems.go:95-108` 未拷贝 `req.UUID`），服务端总是自己生成 UUIDv7；
> `trainingPlan.json` 的 `uuid` 导出时会写出（`zipio.go:166-167`）但导入时**不被消费**，
> 所以重复导入训练/练习包**总是新建一个题册**（只有题目按 uuid 去重）。
>
> **不确定格式时最省事的办法**：先建一道题，`GET /api/export/problems?ids=<PROBLEM_ID>`
> 下载 ZIP，照抄它的结构。

---

## 6. 典型 AI 工作流

### 步骤 0–2：登录、确认域与标签、上传图片

```bash
curl -sS -c cookies.txt -X POST http://localhost:8080/api/auth/login \
  -H 'Content-Type: application/json' -d '{"username":"admin","password":"123456"}'
curl -sS -b cookies.txt http://localhost:8080/api/auth/me
curl -sS -b cookies.txt http://localhost:8080/api/admin/domains
curl -sS -b cookies.txt 'http://localhost:8080/api/tags'
curl -sS -b cookies.txt -X POST http://localhost:8080/api/images -F 'file=@./figure.png'
```

- 登录体**必须同时带 `username` 和 `password`**（只带 password → `400 invalid request`），
  成功 `204` + `Set-Cookie: orange_session=…`；`/api/auth/me` → `{"authenticated":true,"user":{…}}`。
- `401 {"error":"unauthorized"}`：先确认账号是管理员角色，再重新登录（见 1.1）。
- `/api/admin/domains` → `{"domains":[{"id":1,"name":"默认域",…}]}`；`global_admin` 用
  `?domainId=<ID>` 指定域（不指定则落默认域）。
- `/api/tags` → `{"tags":[{"tag":"Python/基础","count":2},…],"total":7}`。
  标签不是必须先建，但**复用已有前缀**能让标签树干净；发现近似标签时优先复用其写法。
- `/api/images` → `201 {"url":"/api/uploads/Ab3xY9_kLm2Pq7Rs.png",…}`。把该 URL 写进
  `statementMd`：`![图](/api/uploads/Ab3xY9_kLm2Pq7Rs.png)`。若改用 ZIP 导入并希望图片随包，
  要把文件放进包的 `images/` 且文件名只用 `[A-Za-z0-9_-]` + 扩展名。

### 步骤 3：生成题目并落库（API 方式，适合少量/需要即时 id）

```bash
curl -sS -b cookies.txt -X POST http://localhost:8080/api/problems \
  -H 'Content-Type: application/json' \
  -d '{
    "type": "programming",
    "title": "两数之和",
    "tags": ["入门/顺序结构"],
    "statementMd": "给定两个整数 $a$ 与 $b$，输出 $a+b$。",
    "bodyJson": {
      "inputFormat": "一行两个整数",
      "outputFormat": "一个整数",
      "samples": [{"input":"1 2","output":"3"}],
      "testCases": [{"input":"1 2","output":"3"},{"input":"1000000000 1000000000","output":"2000000000"}]
    },
    "answerJson": {},
    "solutions": [{"language":"cpp","code":"int main(){}","markdown":"直接相加"}],
    "timeLimitMs": 1000,
    "memoryLimitMiB": 256
  }'
echo "→ 201 {'problem':{'id':42,…}}"
```

`global_admin` 想指定域就加查询参数：`…/api/problems?domainId=1`。

### 步骤 4：建训练 + 章节 + 条目

```bash
# 4.1 建训练（folderId 可省略 = 放根目录）
TRAIN_ID=$(curl -sS -b cookies.txt -X POST http://localhost:8080/api/trainings \
  -H 'Content-Type: application/json' \
  -d '{"title":"第 1 周训练","description":"输入输出与列表","tags":["第1周"]}' \
  | sed -n 's/.*"id":\([0-9]*\).*/\1/p')

# 4.2 建章节（新建章节自动排到末尾）
CH1=$(curl -sS -b cookies.txt -X POST "http://localhost:8080/api/trainings/$TRAIN_ID/chapters" \
  -H 'Content-Type: application/json' -d '{"title":"热身"}' \
  | sed -n 's/.*"id":\([0-9]*\).*/\1/p')

# 4.3 章节内追加题目（用整数题目 id；返回的 itemIds 数量必须等于传入数量）
curl -sS -b cookies.txt -X POST "http://localhost:8080/api/chapters/$CH1/items" \
  -H 'Content-Type: application/json' -d '{"problemIds":[42,43]}'
echo "→ 201 {'itemIds':[7,8]}"
```

### 步骤 5：建练习

```bash
PRAC_ID=$(curl -sS -b cookies.txt -X POST http://localhost:8080/api/practices \
  -H 'Content-Type: application/json' \
  -d '{"title":"第 1 周练习","description":"平铺练习","tags":["第1周"]}' \
  | sed -n 's/.*"id":\([0-9]*\).*/\1/p')

curl -sS -b cookies.txt -X POST "http://localhost:8080/api/practices/$PRAC_ID/items" \
  -H 'Content-Type: application/json' -d '{"problemIds":[42,43]}'
echo "→ 201 {'itemIds':[9,10]}"
```

### 步骤 6：ZIP 方式导入（推荐批量 AI 生成走这条路）

```bash
# 6.1 组装包目录（examples/import-package 就是一份现成的）
#   my-package/
#   ├── problems.json
#   ├── trainingPlan.json
#   └── images/example-sum.png

# 6.2 打包（在包目录内打包，保证 problems.json 位于包根或其子目录）
cd my-package
zip -r ../week01.zip problems.json trainingPlan.json images
cd ..

# 6.3 导入（auto：有 chapters → 训练；无 → 练习）
curl -sS -b cookies.txt -X POST 'http://localhost:8080/api/import?mode=auto' \
  -F 'zip=@week01.zip'
# → 201 {"imported":[{"id":42,"title":"两数之和"},…],
#        "trainingId":7,"chapters":2,"title":"第 1 周训练：输入输出与列表"}

# 6.4 若还想把它放进某个题册目录
curl -sS -b cookies.txt -X PUT "http://localhost:8080/api/trainings/7/folder" \
  -H 'Content-Type: application/json' -d '{"folderId":3}'
```

导入到指定目录也可以直接用查询参数：`?mode=auto&folderId=3`
（`internal/server/io_handlers.go:76-90`；目录不存在报 `400 目录不存在`）。

### 步骤 7：验证（必须做，不能只看 201）

```bash
# 7.1 题目是否真的入库、类型/时限对不对
curl -sS -b cookies.txt 'http://localhost:8080/api/problems?q=两数之和'
# → {"problems":[{"id":42,"type":"programming","title":"两数之和","tags":["入门/顺序结构"],
#                 "timeLimitMs":1000,"memoryLimitMiB":256,…}],"total":1}

# 7.2 训练结构：章节数、每章条目数与 problemTitle（空标题 = 题目丢了）
curl -sS -b cookies.txt http://localhost:8080/api/trainings/7

# 7.3 练习结构
curl -sS -b cookies.txt http://localhost:8080/api/practices/5

# 7.4 客观题答案是否被接受（归一化后应等于你写的 answerIndex）
curl -sS -b cookies.txt http://localhost:8080/api/problems/43 | grep -o '"answerJson":{[^}]*}'

# 7.5 图片是否落盘（引用文件名应为 16 位 nano 名）
curl -sS -b cookies.txt http://localhost:8080/api/problems/42 | grep -o '/api/uploads/[A-Za-z0-9_-]*\.png'

# 7.6 往返校验：导出刚导入的训练，比对 problems.json/trainingPlan.json 是否符合预期
curl -sS -b cookies.txt -o roundtrip.zip http://localhost:8080/api/export/trainings/7
```

自检不通过时的排查顺序：① 条目 `problemId` 是否存在 → ② `problemIds` 下标是否越界
→ ③ `type` 是否拼错 → ④ 图片文件名是否合法。

---

## 7. 常见错误与自检清单

### 7.1 常见错误速查

| 症状 | 根因 | 修法 |
| --- | --- | --- |
| `400 invalid problem type` | `type` 写了 `objective` / `choice` / `singleChoice` | 只用 `programming`/`single_choice`/`true_false` |
| `400 type and title are required` | `title` 空或只有空格 | 填真实标题（`zipio.go:350-354`） |
| `400 time limit 过大（≤15000ms）` | 把秒写进 `timeLimitMs`（如 `60000`） | 单位是毫秒，≤15000 |
| `400 memory limit 过大（≤2048MiB）` | 把字节写进 `memoryLimitMiB` | 单位是 MiB，≤2048 |
| `400 solutions must be a JSON array` | `solutions` 写成对象 `{...}` | 写成数组 `[{...}]` |
| 客观题永远判不对 | `answerIndex` 越界/答案文本对不上 → **静默保留原值** | 用 0-based 整数下标，且 `0 ≤ idx < options.length` |
| 判断题答案不生效 | 写了 `"answer":"是"` / `"T"` | 用布尔 `true` / `false` |
| 判断题写了 `options` | 界面固定渲染「正确/错误」，`options` 不生效 | 判断题 `bodyJson` 留空 `{}`，答案放 `answerJson.answer` |
| 判断题没有解析 | 只填了答案，学生看不到为什么 | 解析写进 `solutions[].markdown`（见 `examples/problem-true-false.json`） |
| 提交后只用样例判分 | 只写了 `samples`，没写 `testCases` | 补 `testCases` |
| 章节为空 / 少了题目 | `trainingPlan.json` 的 `problemIds` 下标越界或写成了数据库 id | 用 0-based 数组下标 |
| `auto` 导成了练习 | 写了 `trainingPlan.json` 但 `chapters: []` | 至少一个章节 |
| 导入后题册名不对 | 没写 `title`，用了文件名兜底 | 显式写 `trainingPlan.json.title` |
| 图片 404 / 不显示 | 只写引用没随包；或文件名含中文/空格 | 包内放 `images/<合法名>`，见 5.7 |
| 图片引用点了 404 但仍能导入 | 文件名合法但文件没打进包 | 打包后 `unzip -l` 核对 |
| 重复导入出现重复题 | 没写 `uuid` | 写唯一 `uuid` 实现幂等 |
| 重复导入没新增题但章节内容旧 | 同 uuid 复用旧题，**不会更新**已有题内容 | 需要覆盖内容请走 `PUT /api/problems/:id` |
| `POST /api/chapters/:id/items` 返回的 itemIds 少了 | 传了不存在的 `problemId`，被静默跳过 | 校验数量 |
| 批量打标签后标签树很乱 | 造了近似标签（`Python` vs `python/基础`） | 先 `GET /api/tags` 复用已有前缀 |
| `tags=a,b` 过滤不准 | 逗号不是分隔符 | 用重复参数 `tags=a&tags=b`（代码为准） |
| 用 UUID 当 `problemIds` | 混用两套标识 | 条目/路由用整数 id；下标用数组下标 |
| `401 unauthorized` | Cookie 过期或被新登录踢掉 | 重新登录，串行执行 |
| `400 invalid request`（登录时） | 只传了 `password`，缺 `username` | 传 `{"username":"admin","password":"…"}` |
| 标签树出现一堆平级标签 | 把父与子写成两个数组元素 `["入门","数组"]` | 写完整路径一个元素 `["入门/数组"]` |
| 用 `orangerepo-backup.json` 调 `/api/import` | 混用两套 ZIP 格式 | 内容创作改用 `trainingPlan.json`（5.0） |
| ZIP 导入后 `starterCpp` 为空 | ZIP 导入路径**已知缺陷**：不拷贝起始代码字段 | 导入后用 `PUT /api/problems/:id` 补，或改用 `POST /api/problems`（5.2） |
| 重导练习包却建成了训练 | 导出的练习包带一个章节，`mode=auto` 判为训练 | 显式用 `mode=practice`（5.9） |
| 重复导入训练包出现多个同名题册 | `trainingPlan.json` 的 `uuid` 导入时不生效 | 预期行为；题册层没有幂等去重（5.9） |
| `POST /api/problems` 传了 `uuid` 但库里不是它 | 该接口不采用请求里的 `uuid` | 只有 `problems.json` 里的 `uuid` 参与去重（5.9） |

### 7.2 提交前自检清单

生成内容后逐条核对（**这些错误全都不会被 API 拒绝，只会静默出错**）：

- [ ] `type` 是三个合法枚举之一（小写）。
- [ ] `title` 非空、不含首尾空格。
- [ ] 编程题：`bodyJson.testCases` 非空，元素键名是 `input` / `output`。
- [ ] 编程题：`timeLimitMs` 是毫秒且 ≤15000；`memoryLimitMiB` ≤2048。
- [ ] 编程题：`answerJson` 是 `{}`。
- [ ] 单选题：`options` 是**纯文本**数组（无 `A.` 前缀），`answerIndex` 是 0-based 且
      `0 <= answerIndex < options.length`。
- [ ] 判断题：`answerJson.answer` 是 JSON 布尔；`bodyJson` 为空（不写 `options`）；解析已写进 `solutions[].markdown`。
- [ ] `solutions` 是数组；`language` 用 `cpp` / `python`。
- [ ] 标签用 `/` 分层，且与 `GET /api/tags` 里已有前缀一致（避免 `Python` 与 `python` 并存）。
- [ ] 父子层级写成了**一个**数组元素（`["入门/数组"]`），而不是两个平级元素。
- [ ] `trainingPlan.json` 的每个 `problemIds` 下标都在 `[0, len(problems.json))` 内。
- [ ] `trainingPlan.json.chapters` 顺序 = 期望的章节顺序；`orderNo` 按 1..N 递增。
- [ ] 需要训练就保证 `chapters` 非空（否则 `auto` 会导成练习）。
- [ ] 想幂等重导就写唯一 `uuid`；不想复用旧题就别抄别处的 uuid。
- [ ] 题面引用的每个图片文件都真的在包内 `images/` 下，且文件名只用 `[A-Za-z0-9_-]`+扩展名。
- [ ] 导入后调 `GET /api/trainings/:id`、`GET /api/practices/:id` 数条目数与预期一致。

---

## 8. 交付物与最小示例文件

| 文件 | 内容 |
| --- | --- |
| `examples/problem-programming.json` | 编程题完整载荷（含 samples/testCases/starter/solutions） |
| `examples/problem-objective.json` | 单选题完整载荷（`options` + `answerIndex` + 题解） |
| `examples/training-plan.json` | `trainingPlan.json`（两章节，下标引用） |
| `examples/practice-plan.json` | 平铺练习的 `trainingPlan.json`（`chapters: []`，配合 `mode=practice`） |
| `examples/import-package/` | 可直接打包导入的完整包（`problems.json` + `trainingPlan.json` + `images/`） |
| `examples/zip-layout.md` | 目录树 / manifest / 校验规则速查 |

打包并导入示例包：

```bash
cd examples/import-package && zip -r ../../week01.zip problems.json trainingPlan.json images && cd ../..
curl -sS -b cookies.txt -X POST 'http://localhost:8080/api/import?mode=auto' -F 'zip=@week01.zip'
```

---

## 9. 文档与代码不一致处（以代码为准）

生成内容时若你参考了 `docs/api-reference.md`，注意这些已核对过的偏差：

| 位置 | 文档说法 | 代码事实 | 依据 |
| --- | --- | --- | --- |
| `docs/api-reference.md:11` | 登录体 `{password}` | 必须 `{"username","password"}`，缺 `username` → `400 invalid request` | `internal/server/auth.go:95-106` |
| `docs/api-reference.md:105` | 支持扩展名含 `svg` | 上传**不收** svg；ZIP 图片引用正则也**不含** svg | `internal/server/images.go:14-16`、`internal/zipio/zipio.go:26` |
| `docs/api-reference.md:29` | `tags=数学,算法` 逗号分隔多标签 | 按**重复参数**解析（`?tags=a&tags=b`）；含逗号的标签本身合法 | `internal/server/problems.go:18-25` |
| `docs/api-reference.md:60` | 单选题 `options: ["A. …","B. …"]` | 选项应为**纯文本**，字母前缀由界面生成 | `app/src/pages/admin/ProblemPane.tsx:582`、`samples/orangeoj-sample.zip` |
| `docs/api-reference.md:33-39` | `ProblemSummary` 未列 `uuid`/`domainId` | 实际含 `uuid`、`domainId` | `internal/model/model.go:58-68` |
| `docs/api-reference.md:189` | `problems.json` + `trainingPlan.json` + `images/` | 一致；补充：另有 `?folderId=` 导入参数与 uuid 去重语义 | `internal/server/io_handlers.go:76-90`、`148-202` |

`docs/api-reference.md` 中**未提到但确实存在**的关键点（本 skill 已覆盖）：
`?folderId=` 导入参数、uuid 去重、图片导入改名、`mode=problems` 缺省值、
`trainingPlan.json` 章节顺序按数组而非 `orderNo`、`problemIds` 为数组下标。

**任何本 skill 未覆盖的行为，请直接读代码**：
`internal/zipio/zipio.go`（交换格式）、`internal/store/store.go`（题目/标签）、
`internal/store/store_trainings.go` 与 `store_practices.go`（训练/练习）、
`internal/server/io_handlers.go`（导入导出）、`internal/server/server.go:106-223`（路由全集）。
