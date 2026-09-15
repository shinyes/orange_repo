# orangeoj-content-authoring

把这份 skill 交给 AI（或程序化调用方），让它按 OrangeOJ 的**真实格式**生成题目、训练、练习内容，
并组装成规范的 OrangeOJ ZIP 包目录。

**本 skill 只覆盖"生成内容"**：产出 JSON 与图片文件、给出包结构。**导入由用户（管理员）自行完成**——
skill 里不含登录、上传、调写接口的任何操作步骤。

## 内容

```
orangeoj-content-authoring/
├── SKILL.md                              # 主文档（字段/枚举/格式规范/工作流/自检清单）
├── README.md                             # 本文件
└── examples/
    ├── problem-programming.json          # 编程题完整载荷
    ├── problem-objective.json            # 单选题完整载荷
    ├── problem-true-false.json           # 判断题完整载荷（含 solutions[].markdown 解析写法）
    ├── training-plan.json                # trainingPlan.json（多章节，下标引用）
    ├── practice-plan.json                # 平铺练习的 trainingPlan.json（chapters: []）
    ├── zip-layout.md                     # ZIP 包输出规范速查（目录树 / 清单文件 / 校验规则）
    └── import-package/                   # 完整训练包目录（可直接打包）
        ├── problems.json
        ├── trainingPlan.json
        └── images/example-sum.png
```

## 用法一：作为 DSH / Aegis skill 安装

把整个 `orangeoj-content-authoring/` 目录放到 skill 搜索根下即可（目录名即 skill 名）：

- 项目级：本仓库的 `skills/orangeoj-content-authoring/`（已就位）
- 用户级（跨项目复用）：`~/.dsh/skills/orangeoj-content-authoring/`
  （或宿主对应的 `~/.claude/skills/`、`~/.agents/skills/`）

`SKILL.md` 的 YAML frontmatter 只有 `name` 与 `description` 两个字段，符合
[agentskills.io 规范](https://agentskills.io/specification)：宿主读取 `description`
判断何时加载，因此 description 里写的是触发条件（题目、训练、练习、判断题、ZIP、格式、AI 生成、OrangeOJ）
而不是流程摘要。

## 用法二：作为系统提示词直接贴给模型

`SKILL.md` 是自包含的单文件（约 640 行），可直接作为 system prompt 或 context 注入。
配合仓库一起用时，建议追加一句：

> 若本文件与 `internal/`、`app/` 下的代码不一致，以代码为准；不确定的字段请先读
> `internal/zipio/zipio.go` 与 `internal/model/model.go`。

## 用法三：给程序化调用方当格式规范

`examples/import-package/` 是一份**结构完整的最小包**，适合作为模板或回归测试夹具：
复制该目录、改内容、按 `examples/zip-layout.md` 的规则打包即可。

`examples/training-plan.json` 与 `examples/import-package/` 的字段结构经过
`internal/zipio` 的真实解析路径校验（`problems.json` 归一化、章节下标范围均通过）。

## 维护约定

- 本目录**只**放文档与示例，不放可执行代码；不要在此目录引入 Go/JS 源码。
- 本目录**不放**面向 AI 的导入/上传操作步骤；管理员导入说明只保留在 `SKILL.md` 第 6.4 节的
  独立小节里，并明确标注"AI 不执行"。
- 字段、枚举、格式若在代码里变更，需同步更新 `SKILL.md` 第 9 节（文档与代码不一致处）。
- 所有依据都写成 `文件:行`，便于复核；新增断言请附代码位置。
