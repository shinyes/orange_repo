# 刷题服务（quiz-service）设计规格

日期：2026-08-29 · 状态：已确认（用户逐项拍板：共享 SQLite 只读、管理员建学生账号、解析=题解 markdown、每轮固定题数全局配置、任何模式答对即移出错题集、答后展示正确答案、错题页提供全部错题入口）

## 1. 目标与边界

在 orange_repo 仓库内新增**独立刷题 Web 服务**（另一端口），面向学生与管理员：

- **刷题页**：学生选科目 → 选分类 → 随机顺序做题；点选项即自动提交，正确提示 ✓、错误提示 ✗ 并展示正确答案；题目有解析（题解 markdown 非空）时显示「查看解析」，点击后在下方展开解析
- **错题页**：答错的题自动进入错题集（按学生维度），按分类分组展示数量；提供「全部错题」与逐分类练习入口；**任何模式下答对该题即自动从错题集移除**
- **我的页**：用户信息、修改密码、退出；管理员额外显示「系统管理」入口
- **系统管理页**（仅管理员）：科目管理、分类管理（名称/显示顺序/标签映射/题型映射/实时题目数）、学生账号管理、全局每轮题数设置

**非目标（第一阶段）**：编程题、多选/不定项、历史练习统计报表、题库本身编辑（仍在主站完成）、Docker/CI 集成（仅加开发脚本与文档，不动现有 Docker 流程）。

## 2. 架构

```
Go 1.25 + Fiber v2 + modernc.org/sqlite（纯 Go 无 CGO）+ bcrypt
React 19 + Vite + TypeScript + Tailwind v4 + shadcn 组件 + marked + KaTeX + DOMPurify

主站（现有）   :8080  /api + web/dist     写 data/orangerepo.db（权威题库）
刷题服务（新增） :8081  /api + web-quiz/dist
数据访问：
  - 只读打开 data/orangerepo.db（mode=ro，不迁移、不写、不加锁 DDL）
  - 自己的数据写 data/quiz.db（独立 WAL）
图片：刷题服务以 /api/uploads 静态服务 data/uploads（与主站同路径约定，题面图片正常显示）
开发期：web-quiz Vite :5174 代理 /api → 127.0.0.1:8081；scripts/dev-quiz.ps1 一键起 4 进程
```

代码布局（沿用现有风格）：

```
cmd/quiz/main.go            入口（-addr/-data/-web/-repo-db 参数）
internal/quizstore/         数据层：quiz.db 迁移与 CRUD + orangerepo.db 只读查询
internal/quizserver/        Fiber 路由与会话认证（含 httptest 冒烟）
web-quiz/                   刷题前端（独立 Vite 应用，复用 web/ 的 UI 组件与渲染管线）
```

复用 `internal/model`（Problem 类型）与 `internal/store` 的导出工具：`TagMatchesSelected`（本次由私有改名导出，前缀包含 + 多选 AND 语义）、`ValidateTagPath`、`NoneTag`。

## 3. 数据模型（quiz.db）

```sql
users(id PK, username TEXT NOT NULL UNIQUE COLLATE NOCASE,
      password_hash TEXT NOT NULL, role TEXT NOT NULL CHECK(role IN ('admin','student')),
      created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)
sessions(token TEXT PRIMARY KEY, user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
         created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)
subjects(id PK, name TEXT NOT NULL, order_no INTEGER NOT NULL DEFAULT 0,
         created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)
categories(id PK, subject_id INTEGER NOT NULL REFERENCES subjects(id) ON DELETE CASCADE,
           name TEXT NOT NULL, order_no INTEGER NOT NULL DEFAULT 0,
           tags_json TEXT NOT NULL DEFAULT '[]', types_json TEXT NOT NULL DEFAULT '[]',
           created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)
wrong_answers(id PK, user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
              problem_id INTEGER NOT NULL, category_id INTEGER NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
              created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
              UNIQUE(user_id, problem_id))
settings(key TEXT PRIMARY KEY, value TEXT NOT NULL)   -- round_size（每轮题数，默认 10）
```

- 首次启动（users 表无 admin 时）自动创建管理员 `admin/123456`，日志提示修改（与主站惯例一致）
- 各表迁移用 `CREATE TABLE IF NOT EXISTS` + 幂等补列（沿用 store.migrate 模式）

## 4. 分类筛选语义（映射 orange_repo）

- 分类 tags_json：标签路径列表（如 `["数学","物理/力学"]`），命中规则与主站一致：对每个选中标签，题目有该标签或前缀子孙（`t==sel || HasPrefix(t, sel+"/")`），多标签之间 AND
- 分类 types_json：仅允许 `single_choice` / `true_false`（第一阶段），空数组 = 两种都算；服务端校验
- 分类实时题目数 = 主库中满足「标签 AND + 题型 IN」的题目数；读不到主库题目（已删除）的错题记录自动忽略（JOIN 过滤）

## 5. API 契约（刷题服务 /api，JSON camelCase）

### 认证

```
POST /api/auth/login  {username, password} → 204 + 会话 Cookie quiz_session（HTTPOnly, SameSite=Lax, 30 天）
POST /api/auth/logout → 204
GET  /api/auth/me     → {authenticated, user?: {id, username, role}}
PUT  /api/auth/password {oldPassword, newPassword} → 204（轮换当前会话 token）
```

### 学生端（requireSession）

```
GET /api/quiz/subjects
  → {subjects: [{id, name, orderNo, categories: [{id, name, orderNo, questionCount}]}]}

POST /api/quiz/round {categoryId}
  → 服务端按分类筛选随机抽题（随机顺序，至多 round_size 道；不足则全量）
  → {categoryId, total, problems: [{id, type, title, statementMd, bodyJson, hasExplanation}]}
  注：bodyJson 仅含题面形态（single_choice → {options}，true_false → {}），绝不含答案/题解

POST /api/quiz/submit {problemId, categoryId, optionIndex?, answer?}
  → 服务端判题（与 answerJson 比对，单选 optionIndex、判断 answer 布尔）
  → {correct, correctAnswer, hasExplanation, explanation}
  - correct → 删除该生该题的错题记录（任何模式答对即移除）
  - wrong  → UPSERT 错题记录（UNIQUE(user_id, problem_id)，保留首次分类） 
  - explanation 仅在 hasExplanation 时为非空 markdown

POST /api/quiz/wrong-round {categoryId?}
  → 该生错题（JOIN 主库存在性过滤）随机顺序全量
  → {scope: 'all'|'category', categoryId?, problems: [{id, type, title, statementMd, bodyJson, hasExplanation, categoryId}]}
  注：problem.categoryId 为错题记录所属分类，提交时回传以便答错时归集

GET /api/quiz/wrong-summary
  → {total, groups: [{categoryId, categoryName, subjectName, count}]}
```

### 管理员（requireAdmin，前缀 /api/admin）

```
科目：GET /api/admin/subjects（含分类全配置与 questionCount）
     POST /api/admin/subjects {name} → {id}
     PATCH /api/admin/subjects/:id {name}
     DELETE /api/admin/subjects/:id（级联分类与错题记录）
     PUT /api/admin/subjects/order {ids: []}
分类：POST /api/admin/categories {subjectId, name, orderNo?, tags, types} → {id}
     PATCH /api/admin/categories/:id {name?, orderNo?, tags?, types?}
     DELETE /api/admin/categories/:id（级联错题记录）
     PUT /api/admin/subjects/:id/categories/order {ids: []}
     GET /api/admin/problems-count?tags=a,b&types=single_choice → {count}（编辑时实时预览题目数）
学生：GET /api/admin/students → {students: [{id, username, createdAt, wrongCount}]}
     POST /api/admin/students {username, password} → {id}
     PUT /api/admin/students/:id/password {password}
     DELETE /api/admin/students/:id（级联错题记录）
设置：GET  /api/admin/settings → {roundSize}
     PUT  /api/admin/settings {roundSize}（校验 1–100，默认 10）
```

校验规则：username 去空格后 1–32 字符、无控制字符、大小写不敏感唯一；password 非空；tags 逐条过 `ValidateTagPath`；types 仅限两种题型。

## 6. 前端 UX（web-quiz）

- 登录页（用户名 + 密码）→ 主界面，底部导航：**刷题 / 错题 / 我的**
- **刷题页**：科目列表 → 分类卡片（显示题目数）→ 做题页：进度「第 x / N 题」、题面（Markdown/KaTeX）、选项（单选 A–D）或 正确/错误 大按钮；点选项即自动提交 → 反馈横幅（✓ 回答正确 / ✗ 回答错误）、正确选项高亮、选错项标红；`hasExplanation` 时显示「查看解析」按钮，点击展开下方解析；「下一题」推进；完成页展示本轮统计（答对/答错），可再练一轮或返回分类
- **错题页**：「全部错题」（总数）+ 按分类分组列表（每类数量）→ 点击进入错题练习（同做题交互）；错题练习中答对时提示「已从错题集移除」并即时刷新列表
- **我的页**：头像区（用户名/角色）、修改密码对话框、退出登录；管理员额外「系统管理」入口（角色可见性由 /api/auth/me 驱动）
- **系统管理页**（仅管理员）：Tab = 科目 / 分类 / 学生 / 设置
  - 科目：增删改、上下移排序
  - 分类（按科目分组）：增删改；编辑对话框含名称、显示顺序、标签（文本输入，服务端校验）、题型复选框（单选题/判断题）、实时题目数预览（调 problems-count）
  - 学生：增删、重置密码、错题数展示
  - 设置：每轮题数（1–100）
- 空态：分类无符合题目时提示「该分类暂无符合条件的题目」；错题集为空时提示并引导去刷题

## 7. 安全与并发

- 只读打开 orangerepo.db：DSN `file:<path>?mode=ro` + `_pragma=busy_timeout(5000)`；打开失败给出明确日志（提示先运行主站初始化题库）；读连接 `SetMaxOpenConns(1)`（沿用主站惯例）
- 判题一律服务端完成，前端不接触 answerJson/solutions（仅提交后由服务端返回判定与解析）
- 会话 token 为 32 字节随机 hex；bcrypt 存密码；管理员接口二次校验 role
- 删除科目/分类/学生均级联清理错题记录，无孤儿引用

## 8. 测试策略

- `internal/quizstore`：quiz.db CRUD 单测（科目/分类/排序/级联、错题 upsert/移除、筛选计数与抽题语义）；测试内用 `store.Open` 造临时 orangerepo.db 供只读查询验证
- `internal/quizserver`：httptest 冒烟（管理员引导登录 → 建学生 → 学生登录 → 建科目分类 → 抽题 → 判题对/错 → 错题汇总/练习 → 答对移除；角色越权 403）
- 前端：`tsc -b && vite build` 通过（与现有 web/ 一致，无 FE 单测）
- 全量：`go vet ./... ; go test ./...`；两个前端各自 `npm run build`

## 9. 兼容边界与文档

- 主站零行为变更：仅 `internal/store` 将 `tagMatchesSelected` 更名为导出 `TagMatchesSelected`（语义不变）
- quiz.db 与 orangerepo.db 物理隔离，刷题服务不持有主库写路径
- 更新 README（功能与新服务快速开始）与 docs/aegis/INDEX.md（登记本规格）
- ADR 信号：共享 SQLite 只读访问（跨进程数据依赖方向）、独立 quiz.db 归属 —— 均为本次规格已明确的架构决策，形成基线条目后无需单独 ADR