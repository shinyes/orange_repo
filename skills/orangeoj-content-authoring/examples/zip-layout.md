# OrangeOJ ZIP 包输出规范速查

> 权威实现：`internal/zipio/zipio.go`；包消费方：`internal/server/io_handlers.go`。
> 本文件只讲**内容创作**用的 OrangeOJ 交换格式（`trainingPlan.json`）。
> 全库备份格式（`orangerepo-backup.json`）是另一套，不要混用。
>
> **本文件描述"要产出什么"**：目录结构、清单文件、字段与校验限制。
> **导入由用户（管理员）自行完成**，这里不含、也不需要任何调用步骤。

## 1. 目录树（产出目标）

```
<包根>/
├── problems.json          # 必填：题目数组（顶层就是 JSON array）
├── trainingPlan.json      # 可选：题册元数据 + 章节下标；决定包被判为训练还是练习
└── images/                # 可选：被题面引用的图片文件本体
    ├── example-sum.png
    └── figure-2.jpg
```

最小包（只出题目）：

```
my-package/
└── problems.json
```

训练包（本目录 `import-package/` 就是这一种）：

```
import-package/
├── problems.json
├── trainingPlan.json
└── images/
    └── example-sum.png
```

## 2. 没有 manifest 文件

- **不存在** `manifest.json` / `meta.json` / `index.json`。不要发明这类文件。
- 包根下除 `problems.json`、`trainingPlan.json` 以外的文件会被当作"扩展文件"忽略
  （`zipio.go:337-341`）。
- `problems.json` 缺失 → 解析直接失败：`400 missing problems.json`（`zipio.go:277-280`）。

## 3. 文件名查找规则（容忍层级）

| 内容 | 查找方式 | 说明 |
| --- | --- | --- |
| `problems.json` | 按**文件名**在 ZIP 内**任意层级**查找 | 放在 `sub/dir/problems.json` 也能找到（`zipio.go:242-262`、测试 `zipio_test.go:186-202`） |
| `trainingPlan.json` | 同上 | 同上 |
| 图片 | 文件所在目录名为 `images` 或以 `/images` 结尾 | `zipio.go:328-336`；文件名还必须匹配引用正则 |

**推荐仍放在包根**：与标准导出包的产出一致，便于人工核对。

## 4. images/ 的命名与大小校验

图片**文件名**必须匹配（`zipio.go:26`）：

```
/api/uploads/([a-zA-Z0-9_-]+\.(png|jpe?g|gif|webp))
```

即文件名只能由 **字母、数字、下划线、连字符** + `.` + 扩展名组成，扩展名限
`png` / `jpg` / `jpeg` / `gif` / `webp`。

| 情况 | 结果 |
| --- | --- |
| `images/example-sum.png` | ✅ 被收集 |
| `images/题图.png` | ❌ 被**静默忽略**（图片丢失，不报错） |
| `images/my figure.png`（含空格） | ❌ 被静默忽略 |
| `images/logo.svg` | ❌ SVG 不在允许扩展名内 |
| 图片 > 12 MiB | ❌ `400 图片过大: <name>`（`zipio.go:330-332`） |

题面对应引用（两种写法都可，推荐前者，因为它是导出包的真实形态）：

```markdown
![示意图](/api/uploads/example-sum.png)
```

```markdown
![示意图](images/example-sum.png)
```

包内的 `(images/…)` 写法在解析时会被改写为 `(/api/uploads/…)`（`zipio.go:99-115`）。
**只写引用不打包 = 图片丢失**：`images/` 里必须有同名真实文件。

## 5. trainingPlan.json（= 本格式的清单文件）

```jsonc
{
  "uuid": "0199a1f0-0001-7000-8000-0000000000a1",  // 可选：题册自身稳定标识
  "title": "第 1 周训练：输入输出与列表",            // 可选：题册名（优先级最高）
  "description": "…",                              // 可选
  "tags": ["训练计划", "第1周"],                    // 可选：题册标签
  "chapters": [                                    // 非空 → 判为「训练」
    { "title": "热身", "orderNo": 1, "problemIds": [0] },
    { "title": "巩固", "orderNo": 2, "problemIds": [1] }
  ]
}
```

练习包则写 `"chapters": []`（见第 6 节）。

### 关键校验点

| 规则 | 依据 |
| --- | --- |
| `problemIds` 是 `problems.json` 的 **0-based 数组下标**，不是数据库 id、不是 uuid | `zipio.go:48`、`io_handlers.go:233-237` |
| 越界下标（负数或 ≥ 数组长度）被**静默丢弃**，章节可能变空且**不报错** | `io_handlers.go:234-237` |
| 章节真实顺序 = `chapters` **数组顺序**；`orderNo` 在解析路径中**未被使用** | `io_handlers.go:226-244`、`store_trainings.go:144-157` |
| `chapters` 为空/缺失 + 训练模式 → 自动建名为 `未分组` 的章节收纳全部题目 | `io_handlers.go:245-262` |
| `chapters` 为空/缺失 → 判为**练习**（平铺） | `io_handlers.go:126-132` |
| `trainingPlan.json` 缺失 → 只处理题目，不建训练/练习 | `io_handlers.go:284-288` |
| 题册名称优先级：`title` → 包文件名（去扩展名）→ `导入的训练`/`导入的练习` | `io_handlers.go:207-221`、`262-276` |

## 6. 包语义与限额

`chapters` 是训练 / 练习的分水岭：

| `chapters` | 包语义 |
| --- | --- |
| 非空数组 | **训练**（按章节建组，章节顺序 = 数组顺序） |
| `[]` 或 `trainingPlan.json` 缺失 | **练习**（`problems.json` 全部按数组顺序平铺） |

| 限额 | 值 | 依据 |
| --- | --- | --- |
| ZIP 文件大小 | 100 MiB | `io_handlers.go:58-60` |
| ZIP 条目数 | 20000 | `zipio.go:295`、`300-302` |
| 单文件展开 | 64 MiB | `zipio.go:296`、`308-310` |
| 总展开 | 800 MiB | `zipio.go:297`、`311-314` |
| 单张图片 | 12 MiB | `zipio.go:298`、`330-332` |

## 7. 打包与产出自检

```bash
# 在包目录内打包（保证根层级正确）
cd my-package
zip -r ../week01.zip problems.json trainingPlan.json images
cd ..

# 核对条目：problems.json 在包根、图片真的打进包里
unzip -l ../week01.zip
```

PowerShell（Windows 上 `Compress-Archive` 会写入反斜杠条目名；
解析器做了 `\`→`/` 归一化，仍可用）：

```powershell
Compress-Archive -Path problems.json, trainingPlan.json, images -DestinationPath ..\week01.zip -Force
```

产出前逐条核对：

- [ ] `problems.json` 顶层是数组，`type` 只用三个合法枚举。
- [ ] 编程题 `bodyJson.testCases` 非空，键名 `input` / `output`。
- [ ] 单选题 `answerIndex` 是 0-based 且落在 `options` 范围内。
- [ ] 判断题 `answerJson.answer` 是布尔，解析写在 `solutions[].markdown`。
- [ ] 每个 `problemIds` 下标都在 `[0, len(problems.json))` 内。
- [ ] 要训练则 `chapters` 非空；要练习则 `chapters: []`。
- [ ] 题面引用的每个图片文件都在 `images/` 下，文件名只用 `[A-Za-z0-9_-]` + 扩展名。
- [ ] `unzip -l` 能看到 `problems.json`、`trainingPlan.json`、全部 `images/*`。

## 8. 包交给用户之后（预期行为，供核对）

导入由管理员执行；以下是导入后**预期发生**的行为，便于判断包是否写对（**不是 AI 的操作步骤**）：

- 条目悬挂：章节条目数少于预期 → `problemIds` 下标越界被丢弃。
- 图片改名：`images/` 里的文件会被改成 16 位 nano 随机名，题面引用同步重写
  （`io_handlers.go:133-162`），**不要依赖包内图片名在导入后保持不变**。
- 题目去重：包内题目带 `uuid` 且同域已存在 → 复用旧题（不更新其内容）；
  不带 `uuid` → 每次导入都新建（`io_handlers.go:148-202`）。
- 题册不去重：`trainingPlan.json` 的 `uuid` 在导入路径不被消费，
  重复导入训练/练习包会新建题册。
