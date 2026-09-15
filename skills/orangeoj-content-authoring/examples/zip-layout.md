# OrangeOJ ZIP 包结构速查

> 权威实现：`internal/zipio/zipio.go`；导入流程：`internal/server/io_handlers.go`。
> 本文件只讲**内容创作**用的 OrangeOJ 交换格式（`trainingPlan.json`）。
> 全库备份格式（`orangerepo-backup.json`，走 `POST /api/import/backup`）是另一套，不要混用。

## 1. 目录树

```
<包根>/
├── problems.json          # 必填：题目数组（顶层就是 JSON array）
├── trainingPlan.json      # 可选：题册元数据 + 章节下标；决定 auto 模式判为训练还是练习
└── images/                # 可选：被题面引用的图片文件本体
    ├── example-sum.png
    └── figure-2.jpg
```

最小包（只导入题目）：

```
my-package/
└── problems.json
```

训练包（本目录 `import-package/` 就是这一种，可直接打包导入）：

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
- `problems.json` 缺失 → 导入直接失败：`400 missing problems.json`（`zipio.go:277-280`）。

## 3. 文件名查找规则（容忍层级）

| 内容 | 查找方式 | 说明 |
| --- | --- | --- |
| `problems.json` | 按**文件名**在 ZIP 内**任意层级**查找 | 放在 `sub/dir/problems.json` 也能找到（`zipio.go:242-262`、测试 `zipio_test.go:186-202`） |
| `trainingPlan.json` | 同上 | 同上 |
| 图片 | 文件所在目录名为 `images` 或以 `/images` 结尾 | `zipio.go:328-336`；文件名还必须匹配引用正则 |

**推荐仍放在包根**：与 `GET /api/export/*` 的产出保持一致，便于人工核对。

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

导入时 `(images/…)` 会被改写为 `(/api/uploads/…)`（`zipio.go:99-115`），
随后所有图片一律改名为 16 位 nano 随机名并重写引用（`io_handlers.go:133-162`）。
**所以不要依赖包内图片名在导入后保持不变。**

## 5. trainingPlan.json（= 本格式的 manifest）

```jsonc
{
  "uuid": "0199a1f0-0001-7000-8000-0000000000a1",  // 可选：题册自身稳定标识
  "title": "第 1 周训练：输入输出与列表",            // 可选：题册名（优先级最高）
  "description": "…",                              // 可选
  "tags": ["训练计划", "第1周"],                    // 可选：题册标签
  "chapters": [                                    // 非空 → auto 模式判为「训练」
    { "title": "热身", "orderNo": 1, "problemIds": [0] },
    { "title": "巩固", "orderNo": 2, "problemIds": [1] }
  ]
}
```

### 关键校验点

| 规则 | 依据 |
| --- | --- |
| `problemIds` 是 `problems.json` 的 **0-based 数组下标**，不是数据库 id、不是 uuid | `zipio.go:48`、`io_handlers.go:233-237` |
| 越界下标（负数或 ≥ 数组长度）被**静默丢弃**，章节可能变空且**不报错** | `io_handlers.go:234-237` |
| 章节真实顺序 = `chapters` **数组顺序**；`orderNo` 在导入路径中**未被使用** | `io_handlers.go:226-244`、`store_trainings.go:144-157` |
| `chapters` 为空/缺失 + `mode=training` → 自动建名为 `未分组` 的章节收纳全部题目 | `io_handlers.go:245-258` |
| `chapters` 为空/缺失 + `mode=auto` → 判为**练习**（平铺） | `io_handlers.go:126-132` |
| `trainingPlan.json` 缺失 → 只导题目（`mode=problems`），不建训练/练习 | `io_handlers.go:284-287` |
| 题册名称优先级：`title` → 上传文件名（去扩展名）→ `导入的训练`/`导入的练习` | `io_handlers.go:207-221`、`262-276` |

## 6. 导入模式与限额

```
POST /api/import?mode=problems|training|practice|auto[&folderId=<目录ID>]
Content-Type: multipart/form-data;  字段名 zip
```

| mode | 行为 |
| --- | --- |
| `problems` | 只入库题目（**缺省值**，`io_handlers.go:70`） |
| `training` | 建训练（按章节） |
| `practice` | 建练习（按 `problems.json` 顺序平铺） |
| `auto` | 有非空 `chapters` → 训练；否则 → 练习（推荐） |

| 限额 | 值 | 依据 |
| --- | --- | --- |
| 上传 ZIP 大小 | 100 MiB | `io_handlers.go:58-60` |
| ZIP 条目数 | 20000 | `zipio.go:295`、`300-302` |
| 单文件展开 | 64 MiB | `zipio.go:296`、`308-310` |
| 总展开 | 800 MiB | `zipio.go:297`、`311-314` |
| 单张图片 | 12 MiB | `zipio.go:298`、`330-332` |

## 7. 打包与导入

```bash
# 打包（在包目录内执行，保证根层级正确）
cd my-package
zip -r ../week01.zip problems.json trainingPlan.json images
cd ..

# 导入
curl -sS -b cookies.txt -X POST 'http://localhost:8080/api/import?mode=auto' \
  -F 'zip=@week01.zip'
# → 201 {"imported":[{"id":42,"title":"两数之和"},…],
#        "trainingId":7,"chapters":2,"title":"第 1 周训练：输入输出与列表"}

# 导入到指定题册目录（目录需已存在；不存在 → 400 目录不存在）
curl -sS -b cookies.txt -X POST 'http://localhost:8080/api/import?mode=auto&folderId=3' \
  -F 'zip=@week01.zip'
```

PowerShell 打包（Windows 上 `Compress-Archive` 会写入反斜杠条目名；
解析器做了 `\`→`/` 归一化，仍可用）：

```powershell
Compress-Archive -Path problems.json, trainingPlan.json, images -DestinationPath ..\week01.zip -Force
```

打包自检（确认图片真的在包里）：

```bash
unzip -l week01.zip
```

## 8. 导入后必查

```bash
# 题目数量与关键字段
curl -sS -b cookies.txt 'http://localhost:8080/api/problems?ids=<ID1>,<ID2>'

# 训练：章节数、每章条目数、条目 problemTitle 非空
curl -sS -b cookies.txt http://localhost:8080/api/trainings/<TRAINING_ID>

# 练习：条目数
curl -sS -b cookies.txt http://localhost:8080/api/practices/<PRACTICE_ID>

# 图片：引用是否已改为 nano 名且可访问
curl -sS -b cookies.txt http://localhost:8080/api/problems/<ID1> | grep -o '/api/uploads/[A-Za-z0-9_-]*\.png'
```

`problemTitle` 为空字符串说明条目指向的题目不存在（条目悬挂）；章节条目数少于预期
说明 `problemIds` 下标越界被丢弃。
