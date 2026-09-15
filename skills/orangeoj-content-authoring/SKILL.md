---
name: orangeoj-content-authoring
description: Use when 为 OrangeOJ（橙子OJ）生成或校订内容文件——题目、训练（章节）、练习的 JSON，或 OrangeOJ ZIP 交换包（problems.json / trainingPlan.json / images/）的目录结构与格式规范；包括 AI 批量生成题目与判断题解析、编写 trainingPlan.json 下标引用、确认 bodyJson/answerJson 形状、判题类型枚举、时限单位与标签层级规则时。
---

# OrangeOJ 内容创作（题目 / 训练 / 练习 / ZIP 包输出）

## 0. 这个 skill 解决什么

按 OrangeOJ **真实的数据格式**生成教学内容**文件**：

- 题目 JSON（三种 `type` 的 `bodyJson` / `answerJson` 形状）
- 训练 JSON（`trainingPlan.json`：章节 + 章节内条目）
- 练习 JSON（平铺条目）
- OrangeOJ ZIP 交换包目录（`problems.json` + `trainingPlan.json` + `images/`）

**本 skill 只教生成与自检，不含任何登录、上传、调接口的步骤**：AI 的产物是文件（JSON + 图片 + 包结构），
**导入由用户（管理员）自行完成**。

**权威来源与优先级**：本 skill 全部字段来自 `internal/model/model.go`、
`internal/zipio/zipio.go`、`internal/server/io_handlers.go`，并与 `docs/api-reference.md` 交叉核对。
**文档与代码不一致时以代码为准**（偏差见第 9 节）。

**不在范围内**：`/api/space/*`（空间训练/练习/刷题，学生侧）与 `internal/quizstore`（作答数据）。
不要改 `internal/`、`app/`、`docs/`。

---

## 1. 核心约定（先读，能挡掉 80% 的错误）

### 1.1 题型枚举（只有 3 个值，必须小写）

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

### 1.2 id 与 uuid：两套标识，用途完全不同

| 标识 | 类型 | 用途 | 何时出现 |
| --- | --- | --- | --- |
| `id` | 整数（AUTOINCREMENT） | 库内引用（`problemId`） | 入库后由数据库分配，**包内写不出来** |
| `uuid` | UUIDv7 字符串 | **跨库稳定标识**：导入去重 | 空则入库时自动生成；包内可带 |

- **包内引用一律不写整数 `id`**：`id` 入库后才有，手写包时不存在（`internal/store/store.go:479-489`）。
- `uuid` 只在 ZIP 包与跨库场景有意义（`internal/server/io_handlers.go:148-202`）。
- `trainingPlan.json` 里的 `problemIds` **既不是 id 也不是 uuid**，是 `problems.json`
  **数组下标**，见 5.3。

### 1.3 标签：不需要预先创建，斜杠即层级

标签**不是独立实体**，没有标签表、没有先建后用的约束：

- 存法：题目的 `tags_json` 列直接存一个字符串数组（`internal/store/store.go:533-539`）。
- **不校验标签是否存在**，写什么就是什么（`zipio.NormalizeProblemPayload`
  根本不动 `Tags`，`internal/zipio/zipio.go:349-391`）。
- 标签树是**读时聚合**出来的：= 现存字面标签 ∪ 虚拟祖先前缀
  （`数学/几何/圆` 让 `数学`、`数学/几何` 也出现在标签树里，`internal/store/store.go:943-950`）。
- 层级分隔符是 `/`，**用 `/` 表达子级，不要用 `.`、`-`、`>`**。路径校验规则（仅用于
  重命名/删除接口）：非空、首尾不得是 `/`、不得有空层级（`internal/store/store.go:799-814`）。
- **一个数组元素 = 一个完整路径**。想要"入门"下的"数组"，要写 `["入门/数组"]`；
  写成 `["入门","数组"]` 会得到**两个并列的顶层标签**（标签树里也是两个平级节点），
  不会产生父子关系（依据：`ListTagFacets` 只从字符串里的 `/` 推导祖先，`store.go:943-950`）。
- 过滤是**前缀命中**：选中 `数学` 会命中标签为 `数学` 或 `数学/...` 的题；多标签之间是 **AND**
  （`internal/store/store.go:616-641`）。
- 标签文本里**可以含逗号**：`tags` 查询参数按"重复参数"解析（`?tags=a&tags=b`），不要用逗号拼
  （`internal/server/problems.go:18-25`；这点与 `docs/api-reference.md:29` 不同，以代码为准）。
- `__none__` 是保留哨兵（表示"无任何标签"），不要给题目打这个标签
  （`internal/store/store.go:614`、`internal/server/tags.go:23-24`、`39-41`）。

### 1.4 图片：文件要随包，引用写成 `/api/uploads/<file>`

| 写在哪 | 正确写法 | 原因 |
| --- | --- | --- |
| 题面四个文本字段 | `/api/uploads/<file>`（推荐） | 导出包的真实形态（`internal/zipio/zipio.go:26`） |
| 同上（等价写法） | `(images/<file>)` | 导入时会被重写成 `/api/uploads/<file>`（`internal/zipio/zipio.go:99-115`） |
| 包内实际文件位置 | `images/<file>` | **只写引用不打包 = 图片丢失** |

- 可打包的图片扩展名只有 **png / jpg / jpeg / gif / webp**；引用正则见
  `internal/zipio/zipio.go:26`。**SVG 不在内**（服务器也不收，`internal/server/images.go:14-16`），
  所以 SVG 图片既不能随包也不被识别。
- 图片"被引用"的判定范围 = 题目四个文本字段：`statementMd`、`bodyJson`、`answerJson`、`solutions`
  （`internal/server/images.go:54-74`）。
- **包内图片名只用 `[A-Za-z0-9_-]` + 扩展名**；中文名、空格、`%` 会被静默忽略（见 5.5）。
- 导入后图片会被改成 16 位 nano 随机名并重写题面引用
  （`internal/server/io_handlers.go:133-162`）。**不要依赖包内图片名在导入后保持不变**。

### 1.5 时限与内存单位

- `timeLimitMs`：**毫秒**（整数），默认 `1000`。
- `memoryLimitMiB`：**MiB**（整数），默认 `256`。
- 只在 `type=programming` 时有意义，且只有编程题会被兜默认值与校验上限
  （`internal/zipio/zipio.go:358-372`）。
- 硬上限：`timeLimitMs ≤ 15000`、`memoryLimitMiB ≤ 2048`，超出报错
  （`internal/zipio/zipio.go:365-371`）。
- 客观题不写这两个字段（写了也不会被使用，
  `app/src/pages/admin/ProblemPane.tsx:179-180`）。
- 单位陷阱：把 1000ms 写成 `1`（当成"1 秒"）会变成 1 毫秒；把 256 写成 `268435456`（当成字节）
  会直接被上限拒绝。

### 1.6 题解语言枚举

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

### 1.7 域（domain）

题目归属域，库内按域隔离。对**内容生成**的唯一影响：新题最终会落到某个域——系统管理员可用
`?domainId=<ID>` 指定，域管理员强制写入自己所属域，缺省落默认域（`internal/server/domain_admin.go:45-76`、
`internal/server/problems.go:90-94`）。**你生成的字段里没有域信息**：包格式也没有 `domainId` 字段，不要发明它。

---

## 2. 题目格式

### 2.1 载荷字段全集

所有创建/导入路径共用**同一形状**（`internal/zipio/zipio.go:66-79`，服务端做归一化）：

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
| `uuid` | string | 可选 | 留空则入库时生成 UUIDv7 |

服务端归一化保证：`bodyJson` 缺省 `{}`、`answerJson` 缺省 `{}`、`solutions` 缺省 `[]`
（`internal/zipio/zipio.go:373-381`）。入库后的 `Problem` 还会多出 `id`、`domainId`、`createdAt`
（`internal/model/model.go:39-55`）——这些**不是**你要写的字段。

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

- `answerIndex` 是 **0-based 下标**（`internal/zipio/zipio.go:481-486`；界面显示时再加 A/B/C/D，
  `app/src/pages/admin/ProblemPane.tsx:582`、`602`）。
- `options` 里**只写选项正文，不要自带 `A.` `B.` 前缀**：界面自动编号
  （`app/src/pages/admin/ProblemPane.tsx:582`）；官方示例包也是纯文本
  （`samples/orangeoj-sample.zip` 内 `problems.json`）。
  注意 `docs/api-reference.md:60` 的示例写了 `"A. …"`，那是文档笔误——以代码与示例包为准。
- 兼容写法：`answerJson` 也可传 `{"answer":"<选项原文>"}`，会按
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
`examples/problem-true-false.json`（判断，含解析写法）。以下是最小可用载荷（可直接复制）：

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

章节/条目入库后的形状（`trainingPlan.json` 里只写 5.3 的子集）：

```jsonc
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

### 3.2 排序与层级规则

- 章节顺序 = `training_chapters.order_no`，新建章节自动排末尾
  （`internal/store/store_trainings.go:144-157`）。
- 条目顺序 = `training_items.order_no`，新建条目自动 `MAX+1`
  （`internal/store/store_trainings.go:187-221`）。
- 读取一律 `ORDER BY order_no, id`（`store_trainings.go:366`、`398`）。
- 条目引用题目的整数 `id`（`problemId`），不是 uuid；包内改用下标，见 5.3。
- 训练的 `tags`（`trainings.tags_json`）与题目标签是**同一套字符串数组格式**，但**互相独立**：
  训练的 `tags` 不影响题目标签树，也不会自动继承题目的标签
  （`internal/store/store_trainings.go:84-95`）。
- 删除训练会级联删除章节与条目，并且**把"仅被该训练引用"的题目一起删掉**；
  被其它训练/练习也引用的题目会保留（`internal/store/store_trainings.go:109-141`）。

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
| 分值 | 无分值语义 | **无分值语义**（`score` 列已退役，`internal/store/store.go:149-152`） |

### 4.2 练习在包里的写法

练习条目**没有** `uuid` 字段（`internal/model/model.go:121-129`），只有题目引用。
生成练习包时：

- `problems.json` 数组顺序 = 练习条目顺序（平铺顺序就是数组顺序，`io_handlers.go:284-288`）。
- `trainingPlan.json` 的 `chapters` **留 `[]`**：这样"平铺"语义最明确，也避免被当成训练（见 5.4）。
- 训练/练习的**整体 `uuid`** 存在（`trainings.uuid` / `practices.uuid`，
  `internal/store/store.go:180-187`），用于 `trainingPlan.json` 的 `uuid` 字段。

---

## 5. ZIP 包输出格式（OrangeOJ ZIP）

> 本节是**产出规范**：AI 负责把内容写成这样的目录与文件，导入动作由用户（管理员）执行。

### 5.0 两套格式别混用

仓库里存在**两套互不通用**的 ZIP 内容格式：

| 格式 | 清单文件 | 用途 |
| --- | --- | --- |
| **OrangeOJ 交换格式**（本 skill 主讲） | `trainingPlan.json` | 题目/训练/练习；与上游 OrangeOJ 双向兼容 |
| 全库备份格式 | `orangerepo-backup.json` | 整库/整域迁移（`internal/server/backup.go:24-68`） |

**内容创作一律用 `trainingPlan.json` 格式。** 不要手写 `orangerepo-backup.json`：那是全库备份清单
（`version` 必须为 `1`，`internal/server/backup.go:264-267`），语义是"恢复整库"。
训练与练习**分开出两个包**（一个包只对应一个题册）。

### 5.1 目录结构

```
<包根>/
├── problems.json          # 必填，题目数组
├── trainingPlan.json      # 可选，题册（训练/练习）元数据 + 章节下标
└── images/                # 可选，被题面引用的图片文件本体
    ├── example-sum.png
    └── figure-2.jpg
```

真实示例见 `examples/import-package/`（可直接打包的完整包）。

解析规则（`internal/zipio/zipio.go:241-344`）：

- `problems.json` / `trainingPlan.json` 用**文件名**在 ZIP 内**任意层级**查找
  （`findZipFileByNames`，`zipio.go:242-262`）——放子目录里也能找到。**推荐仍放包根**，便于人工核对。
- `images/` 同理：目录名是 `images` 或以 `/images` 结尾即可（`zipio.go:328-336`）。
- 包根下**其它文件**会被当"扩展文件"忽略（`zipio.go:337-341`）——不要往里塞别的东西当 manifest。
- **没有 manifest 文件**：不要发明 `manifest.json` / `meta.json` / `index.json`，系统不认。
- `problems.json` 缺失 → 解析直接失败：`400 missing problems.json`（`zipio.go:277-280`）。

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
- 图片引用写在题目**四个文本字段**里，用 `/api/uploads/<file>` 形态（见 1.4）。
- `uuid` 的语义：包内题目**带 `uuid`** 且同域内已存在 → 导入时**复用已有题**（跳过新建）；
  **不带 `uuid`** → 每次导入都新建（`internal/server/io_handlers.go:148-202`）。
  批量生成的新内容不要沿用别处抄来的 uuid；要么不写，要么用新的唯一 UUIDv7。
- `starterCpp` / `starterPy` 在 ZIP 导入路径**会正常落库**（构造题目时已拷贝，
  `internal/server/io_handlers.go:184-185`；导出侧同样带出，同文件 `32-33`）。
  空串 = 走前端通用模板（`internal/model/model.go:38`），照 2.1 正常填写即可。

### 5.3 `trainingPlan.json`

```jsonc
{
  "uuid": "0199…a1",            // 可选：题册自身的稳定标识
  "title": "第 1 周训练",        // 可选：题册名称（优先级最高）
  "description": "…",           // 可选
  "tags": ["第1周"],            // 可选：题册标签
  "chapters": [                 // 关键：非空 => 被识别为「训练」
    { "title": "热身", "orderNo": 1, "problemIds": [0, 1] },
    { "title": "巩固", "orderNo": 2, "problemIds": [] }
  ]
}
```

结构依据：`internal/zipio/zipio.go:48-63`（`PlanChapter` / `PlanMeta`）。

**`problemIds` 是 `problems.json` 数组的 0-based 下标**（注释在 `zipio.go:48`，
消费逻辑在 `internal/server/io_handlers.go:233-237`）。

- **不是**数据库 id，**不是** uuid。写错 → 章节变空且**不报错**，必须自检。
- 越界下标（负数或 ≥ 数组长度）被**静默丢弃**（`io_handlers.go:234-237`）。
- `orderNo` 在导入时**并未被使用**：章节按 `chapters` 数组顺序依次创建，
  新建章节自动排末尾（`io_handlers.go:226-244` + `store_trainings.go:144-157`）。
  所以**数组顺序才是真实顺序**，`orderNo` 请按 1..N 顺序写以免误导。
- 章节 `title` 不能为空（会原样建出空标题章节）；练习包写 `chapters: []`（见 5.4）。
- 题册名称优先级：`trainingPlan.json.title` → **ZIP 文件名**（去扩展名）→ 默认名
  （`导入的训练`/`导入的练习`，`io_handlers.go:207-221`、`262-276`）。
  所以**要显式写 `title`**，同时把包文件起个有意义的名字（如 `第3周训练.zip`）以防兜底。
- `trainingPlan.json` 的 `uuid` 导出时会写出（`zipio.go:166-167`）但导入时**不被消费**，
  所以重复导入训练/练习包**总是新建一个题册**（只有题目按 uuid 去重）——包内不必依赖它做幂等。

### 5.4 `chapters` 空与非空 = 训练 / 练习的分水岭

| `chapters` | 包语义 |
| --- | --- |
| 非空数组 | **训练**（按章节建组；章节顺序 = 数组顺序） |
| `[]` 或 `trainingPlan.json` 缺失 | **练习**（`problems.json` 全部按数组顺序平铺） |

配套的管理员导入约定（**供写包时对齐语义，不由 AI 执行**，见 6.4）：
`mode=auto` 判据是 `meta != nil && len(meta.Chapters) > 0`（`io_handlers.go:126-132`）；
`mode=training` 时若章节为空，会自动建一个名为 **`未分组`** 的章节收纳全部题目
（`io_handlers.go:245-262`）。

> **⚠️ 练习包的往返陷阱**：导出的**练习**包其 `trainingPlan.json` 里有**一个**章节
> （`io_handlers.go:452-458`，标题 = 练习名）。把这种包再导入 → 非空 `chapters` 命中 →
> 会建成**训练**而不是练习！**手写练习包时 `chapters` 留 `[]`** 更不容易误判。

### 5.5 图片随包规则

1. 题面写引用：`![图](/api/uploads/example-sum.png)`（推荐）或 `![图](images/example-sum.png)`；
   包内放真实文件 `images/example-sum.png`。**只写引用不打包 = 图片丢失**。
2. 文件名必须匹配 `/api/uploads/([a-zA-Z0-9_-]+\.(png|jpe?g|gif|webp))`——**只允许字母数字下划线连字符 + 点 + 扩展名**
   （`zipio.go:26`）。中文名、空格、`%` 等会被**直接忽略**（图片丢失，不报错，`zipio.go:329-334`）。
3. 扩展名限 png/jpg/jpeg/gif/webp；单图 > 12MiB 报错（`zipio.go:296-299`、`330-332`）。
4. 图片引用只在**题目的四个文本字段**里被识别（`zipio.go:84-96`、`io_handlers.go:154-162`）。

### 5.6 ZIP 大小限额（产出时不要超）

`internal/zipio/zipio.go:293-314` + `io_handlers.go:58-60`：

| 项 | 上限 | 说明 |
| --- | --- | --- |
| ZIP 文件大小 | 100 MiB | 压缩后 |
| ZIP 条目数 | 20000 | 含目录条目 |
| 单文件展开 | 64 MiB | 任意条目 |
| 总展开 | 800 MiB | 全部条目之和 |
| 单张图片 | 12 MiB | `images/` 下 |

超出任一项，包会被整体拒绝；**图片过多时优先压缩/换格式，而不是拆成多个包**（一个包只对应一个题册）。

### 5.7 交付前打包自检

在包目录内打包（保证根层级正确）：

```bash
cd my-package
zip -r ../week01.zip problems.json trainingPlan.json images
unzip -l ../week01.zip      # 核对图片真的在包里、problems.json 在包根
```

PowerShell（Windows 上 `Compress-Archive` 会写入反斜杠条目名；解析器做了 `\`→`/` 归一化，仍可用）：

```powershell
Compress-Archive -Path problems.json, trainingPlan.json, images -DestinationPath ..\week01.zip -Force
```

---

## 6. AI 工作流：理解需求 → 生成文件 → 自检 → 交付

**AI 的边界**：只产出文件。不登录、不调写接口、不上传图片、不导入。

### 6.1 交付物清单

| 交付物 | 必填 | 说明 |
| --- | --- | --- |
| 题目 JSON | ✅ | 题目数组；单独交付时可为 `problems.json`，也可嵌在包内 |
| `trainingPlan.json` | 出训练/练习时必填 | 题册元数据 + 章节下标；练习包写 `chapters: []` |
| `images/*` | 题面引用了图片时必填 | 文件名只用 `[A-Za-z0-9_-]` + 允许的扩展名 |
| 说明文字 | 建议 | 包内题目数、训练/练习语义（`chapters` 是否为空）、以及导入建议模式 |

**导入由用户（管理员）自行完成**：把交付物交给用户即可，不要代为执行。

### 6.2 生成步骤

1. **理解需求**：要出几道题、什么题型、哪个主题的标签、是训练（分章节）还是练习（平铺）、
   题面是否需要配图。需求含糊时先确认"训练 vs 练习"，因为它决定 `chapters` 写法。
2. **设计标签**：用 `/` 分层的完整路径，每个路径是**一个**数组元素（`["入门/数组"]`）；
   同类内容复用同一前缀，避免 `Python` 与 `python` 并存。
3. **写题目 JSON**：按第 2 节选 `bodyJson` / `answerJson` 形状；编程题必填 `testCases`；
   判断题解析写进 `solutions[].markdown`；客观题自检下标范围。
4. **写 `trainingPlan.json`**：训练按章节分组、`problemIds` 用 0-based 下标；
   练习写 `chapters: []`。每个下标都要指向真实存在的题目元素。
5. **准备图片**：文件名改成合法名（`[A-Za-z0-9_-]` + 扩展名），放进 `images/`，
   题面用 `/api/uploads/<file>` 引用。
6. **自检**：走 6.3 清单，逐条核对。
7. **交付**：给出包目录/文件清单 + 一句话导入说明（由用户执行）。

### 6.3 交付前自检清单（必须逐条过）

**下面这些错误全都不会被拒绝，只会静默出错**，所以自检不能省：

- [ ] `type` 是三个合法枚举之一（小写）。
- [ ] `title` 非空、不含首尾空格。
- [ ] 编程题：`bodyJson.testCases` 非空，元素键名是 `input` / `output`。
- [ ] 编程题：`timeLimitMs` 是毫秒且 ≤15000；`memoryLimitMiB` ≤2048。
- [ ] 编程题：`answerJson` 是 `{}`。
- [ ] 单选题：`options` 是**纯文本**数组（无 `A.` 前缀），`answerIndex` 是 0-based 且
      `0 <= answerIndex < options.length`。
- [ ] 判断题：`answerJson.answer` 是 JSON 布尔；`bodyJson` 为空（不写 `options`）；解析已写进 `solutions[].markdown`。
- [ ] `solutions` 是数组；`language` 用 `cpp` / `python`；没有空 `language` 项。
- [ ] 标签用 `/` 分层；父子层级写成了**一个**数组元素（`["入门/数组"]`），而不是两个平级元素。
- [ ] `problems.json` 顶层是数组，不是 `{"problems":[…]}`。
- [ ] `trainingPlan.json` 的每个 `problemIds` 下标都在 `[0, len(problems.json))` 内。
- [ ] `trainingPlan.json.chapters` 顺序 = 期望的章节顺序；`orderNo` 按 1..N 递增。
- [ ] 要训练就保证 `chapters` 非空；要练习就写 `chapters: []`。
- [ ] 想幂等重导就写唯一 `uuid`；不想复用旧题就别抄别处的 uuid。
- [ ] 题面引用的每个图片文件都真的在包内 `images/` 下，且文件名只用 `[A-Za-z0-9_-]`+扩展名。
- [ ] 打包后 `unzip -l` 能看到 `problems.json`、`trainingPlan.json`、全部 `images/*`。
- [ ] 编程题的 `starterCpp` / `starterPy` 是**题目顶层字段**，不在 `bodyJson` 里（写了就会落库）。

### 6.4 供管理员参考：导入参数（**AI 不执行**）

以下端点与参数**只是让写包时对齐语义**（例如为什么练习包要写 `chapters: []`）。
**这些是管理员在管理界面/后台执行的操作，AI 不得自行调用、不得生成调用脚本。**

| 端点 | 说明 |
| --- | --- |
| `POST /api/import?mode=problems\|training\|practice\|auto[&folderId=<目录ID>]` | 导入包；`mode` 缺省 `problems`（`io_handlers.go:70-75`） |
| `GET /api/export/problems`、`/api/export/trainings/:id`、`/api/export/practices/:id` | 导出标准包，可作为格式参照（`io_handlers.go:315-465`） |

要点（便于判断包的语义是否写对）：

- `mode=auto`：包内**非空 `chapters`** → 训练；否则 → 练习（`io_handlers.go:126-132`）。
  这正是"练习包必须写 `chapters: []`"的原因。
- `mode=practice` **忽略 `chapters`**，把 `problems.json` 全部按数组顺序平铺（`io_handlers.go:266-288`）。
- `folderId` 指题册目录；目录不存在会被拒绝（`io_handlers.go:76-90`）。
- 非法 `mode` → `400 invalid mode: 仅支持 problems|training|practice|auto`（`io_handlers.go:74`）。

---

## 7. 常见错误与自检

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
| 包被判成练习 | 写了 `trainingPlan.json` 但 `chapters: []` | 至少一个章节（要训练的话） |
| 包被判成训练 | 练习包也写了非空 `chapters` | 练习包写 `chapters: []`（5.4） |
| 导入后题册名不对 | 没写 `title`，用了文件名兜底 | 显式写 `trainingPlan.json.title` |
| 图片 404 / 不显示 | 只写引用没随包；或文件名含中文/空格 | 包内放 `images/<合法名>`，见 5.5 |
| 图片引用点了 404 但仍能导入 | 文件名合法但文件没打进包 | 打包后 `unzip -l` 核对 |
| 重复导入出现重复题 | 没写 `uuid` | 写唯一 `uuid` 实现题目层幂等 |
| 重复导入没新增题但内容旧 | 同 uuid 复用旧题，**不会更新**已有题内容 | 用户需在管理后台覆盖该题内容 |
| 重复导入出现多个同名题册 | `trainingPlan.json` 的 `uuid` 导入时不生效 | 预期行为；题册层没有幂等去重（5.3） |
| 批量打标签后标签树很乱 | 造了近似标签（`Python` vs `python/基础`） | 复用同一前缀写法 |
| `tags=a,b` 过滤不准 | 逗号不是分隔符 | 用重复参数 `tags=a&tags=b`（代码为准） |
| 用 UUID 当 `problemIds` | 混用两套标识 | 条目/路由用整数 id；包内用数组下标 |
| 标签树出现一堆平级标签 | 把父与子写成两个数组元素 `["入门","数组"]` | 写完整路径一个元素 `["入门/数组"]` |
| 用 `orangerepo-backup.json` 当内容包 | 混用两套 ZIP 格式 | 内容创作改用 `trainingPlan.json`（5.0） |

---

## 8. 交付物与示例文件

| 文件 | 内容 |
| --- | --- |
| `examples/problem-programming.json` | 编程题完整载荷（含 samples/testCases/starter/solutions） |
| `examples/problem-objective.json` | 单选题完整载荷（`options` + `answerIndex` + 题解） |
| `examples/problem-true-false.json` | 判断题完整载荷（含 `solutions[].markdown` 解析写法） |
| `examples/training-plan.json` | `trainingPlan.json`（多章节，下标引用） |
| `examples/practice-plan.json` | 平铺练习的 `trainingPlan.json`（`chapters: []`） |
| `examples/import-package/` | 完整的训练包目录（`problems.json` + `trainingPlan.json` + `images/`），可直接打包 |
| `examples/zip-layout.md` | 目录树 / manifest / 校验规则速查 |

生成新包时最省事的做法：**复制 `examples/import-package/` 目录，改内容，再按 5.7 打包**。

---

## 9. 文档与代码不一致处（以代码为准）

生成内容时若你参考了 `docs/api-reference.md`，注意这些已核对过的偏差：

| 位置 | 文档说法 | 代码事实 | 依据 |
| --- | --- | --- | --- |
| `docs/api-reference.md:105` | 支持扩展名含 `svg` | 服务器不收 svg；ZIP 图片引用正则也**不含** svg | `internal/server/images.go:14-16`、`internal/zipio/zipio.go:26` |
| `docs/api-reference.md:29` | `tags=数学,算法` 逗号分隔多标签 | 按**重复参数**解析（`?tags=a&tags=b`）；含逗号的标签本身合法 | `internal/server/problems.go:18-25` |
| `docs/api-reference.md:60` | 单选题 `options: ["A. …","B. …"]` | 选项应为**纯文本**，字母前缀由界面生成 | `app/src/pages/admin/ProblemPane.tsx:582`、`samples/orangeoj-sample.zip` |
| `docs/api-reference.md:33-39` | `ProblemSummary` 未列 `uuid`/`domainId` | 实际含 `uuid`、`domainId` | `internal/model/model.go:58-68` |
| `docs/api-reference.md:189` | `problems.json` + `trainingPlan.json` + `images/` | 一致；补充：`?folderId=` 参数与 uuid 去重语义 | `internal/server/io_handlers.go:76-90`、`148-202` |

`docs/api-reference.md` 中**未提到但确实存在**的关键点（本 skill 已覆盖）：
`?folderId=` 参数、uuid 去重、`mode=problems` 缺省值、
`trainingPlan.json` 章节顺序按数组而非 `orderNo`、`problemIds` 为数组下标。

**任何本 skill 未覆盖的行为，请直接读代码**：
`internal/zipio/zipio.go`（交换格式）、`internal/store/store.go`（题目/标签）、
`internal/store/store_trainings.go` 与 `store_practices.go`（训练/练习）、
`internal/server/io_handlers.go`（导入导出格式）、`internal/server/server.go:106-223`（路由全集）。
