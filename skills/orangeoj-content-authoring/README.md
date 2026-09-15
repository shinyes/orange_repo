# orangeoj-content-authoring

把这份 skill 交给 AI（或程序化调用方），让它按 OrangeOJ 的**真实格式**生成题目、训练、练习内容，
打包成 OrangeOJ ZIP 并导入、验证。

## 内容

```
orangeoj-content-authoring/
├── SKILL.md                              # 主文档（字段/枚举/路由/工作流/自检清单）
├── README.md                             # 本文件
└── examples/
    ├── problem-programming.json          # 编程题完整载荷
    ├── problem-objective.json            # 单选题完整载荷
    ├── training-plan.json                # trainingPlan.json（两章节，下标引用）
    ├── practice-plan.json                # 平铺练习的 trainingPlan.json
    ├── zip-layout.md                     # ZIP 目录树 / manifest / 校验规则速查
    └── import-package/                   # 可直接打包导入的完整包
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
判断何时加载，因此 description 里写的是触发条件（题目、训练、练习、导入、ZIP、AI 生成、OrangeOJ）
而不是流程摘要。

## 用法二：作为系统提示词直接贴给模型

`SKILL.md` 是自包含的单文件（约 890 行），可直接作为 system prompt 或 context 注入。
配合仓库一起用时，建议追加一句：

> 若本文件与 `internal/`、`app/` 下的代码不一致，以代码为准；不确定的字段请先读
> `internal/zipio/zipio.go` 与 `internal/server/io_handlers.go`。

## 用法三：给程序化调用方当格式规范

`examples/import-package/` 是一份**可直接导入**的最小包，适合作为回归测试夹具或模板：

```bash
cd examples/import-package
zip -r ../../week01.zip problems.json trainingPlan.json images
curl -sS -b cookies.txt -X POST 'http://localhost:8080/api/import?mode=auto' -F 'zip=@week01.zip'
```

`examples/training-plan.json` 与 `examples/import-package/` 的字段结构经过
`internal/zipio` 的真实解析路径校验（`problems.json` 归一化、章节下标范围均通过）。

## 维护约定

- 本目录**只**放文档与示例，不放可执行代码；不要在此目录引入 Go/JS 源码。
- 字段、枚举、路由若在代码里变更，需同步更新 `SKILL.md` 第 9 节（文档与代码不一致处）。
- 所有依据都写成 `文件:行`，便于复核；新增断言请附代码位置。
