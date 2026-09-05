# OrangeRepo OJ 重构（域/仓库/空间）设计规格

日期：2026-09-04（更新 2026-09-05：全系统统一命名 **OrangeOJ**——仓库页与门户均为 OrangeOJ 一部分；原"OrangeRepo"仅指代码仓库名） · 状态：已确认（用户逐项拍板，§10 清单 A–F 全部定案）

## 0.1 重构总原则（用户 2026-09-05 补充，最高优先级）

**除「全仓导入/导出（backup）」这一交互点外，重构过程不考虑向后兼容**：
- 旧数据表/旧接口/旧页面模块（subjects/categories/wrong_answers/assignments/assigned_students、
  旧 /api/quiz /api/oj 布置流、随机刷题/错题集 UI、旧角色 admin/student 读写路径等）在重构落位后
  **直接退役删除**，不留兼容层、不做双轨迁移
- 代码以**可维护性优先**：消灭兼容别名/迁移 shim（如角色值映射表、旧表并行）、结构清晰、
  命名一致；重构可自由重写旧实现
- 唯一例外：ZIP 全仓备份包导出/导入格式保持（含 problems.json + orangerepo-backup.json 扩展，
  双兼容设计不变）——这是跨实例迁移的唯一持久契约
用户需求原文：「重构 OrangeRepo 的 OJ 系统 —— 增加域功能（每域数据完全分开）；每域对应一个仓库（仓库页 = 现 OrangeRepo 主站页面）；每域含多个空间（空间间训练/练习隔离）；系统管理员有域管理页（新建/改名/删除/设域管理员）；登录后页面与现在 OJ 页一样，多空间用户可切换空间、系统管理员可切换域；训练页可自建或从仓库选训练/练习创建，可编辑名称/章节/题目/顺序，客观题选择次数上限（到限标红），通过标绿；练习页可自建或从仓库创建，练习=整份作答后统一交卷才看结果；去掉主站独立入口（每域一个仓库）；题目增加 UUIDv7（导入缺则补）」

## 0. 已确认决策（用户逐项拍板，含 §10 清单 A–F）

| 决策点 | 结论 |
|---|---|
| 题目数据隔离 | **题目域级共享**：problems 唯一（域级），仓库页 = 域题目全集管理；空间训练/练习/作答隔离；空间训练引用题目 id |
| 域隔离 | **逻辑隔离 domain_id**（单库多域，所有业务表带 domain_id 或经父表归属） |
| 训练选择限次 | 仅客观题（单选/判断）限次；编程题不限。**次数只存在于训练中**：每训练统一上限（训练级配置，默认 3），计数 = 该用户在该训练内对该客观题的作答次数，重进训练不清零；达限未对 → 标红禁选；答对 → 标绿禁选 |
| 练习模式 | **整份交卷**：一次性作答全部（客观选答案/编程提交），交卷后才显示结果；**可重做，每次作答结果都记录** |
| 用户进空间 | 管理员拉人（空间成员表）；登录后按所在空间切换 |
| 域管理员 | 域管理员 = 域内全部空间管理员（建空间/拉人/空间内建训练练习）；不另设空间管理员 |
| 空间成员 | **成员一律学生**（做题）；训练/练习创建管理由域管理员做（清单 F） |
| 仓库/空间题目 | 题目域级共享，仓库页管题目；**仓库保留训练/练习模板**（简单结构管理），空间从仓库选时拷贝结构（清单 A） |
| 主站去向 | 去掉独立主站：现主站页改造为「域仓库」页；OJ 统一门户按域/空间切换 |
| 题目 UUID | problems 加 uuid 列（UUIDv7，导入缺则生成），保留自增 id 主键；作跨库稳定标识与导入去重依据 |
| 随机刷题/错题 | **移除**（清单 B：现科目/分类随机抽题 + 错题集退役） |
| 练习重做 | 可重复交卷，**每次作答结果都记录**（清单 D） |
| 域管理 | 系统管理员域管理页：新建/改名/删除/设（移除）域管理员；域管理员 = role=domain_admin 且 users.domain_id 指向该域，可多名（清单 E） |

## 1. 概念模型

```
系统管理员（global admin）—— 域管理页
  └─ 域 Domain（数据逻辑隔离 domain_id）
       ├─ 域管理员 DomainAdmin（可设多人？初始建域时指定；域设置页更换）
       ├─ 仓库 Repo（= 域内题目全集；现主站 UI 改造为仓库页，仅系统/域管理员可进）
       │    题目 problems（domain_id）+ 标签 + 导入导出 + 目录（booklet_directories 变域级？见 §4）
       └─ 空间 Space（域内多个）
             ├─ 空间成员 space_members（管理员拉入；成员=学生/教师角色？先统一为成员）
             ├─ 空间训练（自建 或 从仓库选训练/练习导入结构）→ 客观题限次作答
             ├─ 空间练习（自建 或 从仓库选训练/练习导入结构）→ 整份交卷
             └─ 提交/进度/错题（现有 quiz.db 学生数据模型迁至空间维度）
```

## 2. 用户与角色

- users 扩展角色：`global_admin`（系统管理员）/ `domain_admin`（域管理员，带 domain_id 归属）/ `member`（空间成员/学生）
  - 兼容迁移：现 admin → global_admin；现 student → member（无域归属）
- 域管理员归属存 user.domain_id；空间成员关系存 space_members(space_id, user_id)
- 登录后：global_admin 可切域/管理域；domain_admin 默认其域；member 可见其加入的全部空间（可切换）
- 系统管理员仍可建普通管理员？——域管理员由系统管理员在域管理页指定（可多域？一个域管理员管一域，先 1:1，存 domain_admins 关系表更灵活——**决定：域管理员即 role=domain_admin 且 users.domain_id=域**，可多个）

## 3. 仓库页（现主站改造）

- 入口：登录后 global_admin 选域进入仓库 / domain_admin 直达其域仓库
- 页面 = 现主站三栏 UI（标签树/题目列表/题目详情+题解/训练练习编制**去除**——训练/练习移到空间，仓库保留题目管理 + 目录结构？原 booklet_directories 属于主库编排训练练习用，训练/练习移走后目录模型去留？——**决定**：仓库页保留题目管理（列表/标签/编辑/导入导出/图片）；原训练/练习/题册目录功能从仓库移除（它们归空间），现库中已有训练/练习作为「仓库模板」供空间选择？——需要用户确认：现主库 trainings/practices 是否保留为"仓库模板库"供空间从仓库选题创建）
- UUIDv7 展示/复制按钮

## 4. 数据模型（新增 domain/space 后主库 orangerepo.db）

```sql
domains(id PK, name UNIQUE, created_at)
problems(+ domain_id NOT NULL REFERENCES domains, + uuid TEXT UNIQUE)  -- 迁移补列
-- 训练/练习迁移为空间实体（原 trainings/practices 保留为域级仓库模板 or 移除？）
spaces(id PK, domain_id NOT NULL REFERENCES domains ON DELETE CASCADE, name, created_at)
space_members(space_id, user_id, role TEXT('admin'|'member'), PK(space_id,user_id))
space_trainings(id PK, space_id, title, description, tags_json, created_at)   -- 由仓库模板拷贝或自建
space_training_chapters / space_training_items(problem_id)
space_practices(id PK, space_id, title, description, tags_json, ...)
space_practice_items(problem_id)
-- 客观题限次：space_training_limits(user_id, space_training_item_id 或 problem_id+training_id, attempts, max_attempts)
-- 练习交卷：space_practice_submissions(practice_id, user_id, submitted_at) + answers 快照
```
- 原 quiz.db（科目/分类/错题/刷题轮/布置/提交/进度）学生数据在空间模型下的落位需重设计：
  - subjects/categories/wrong_answers/round：学生随机刷题体系与空间训练/练习并存？用户没说移除随机刷题，但重构核心是空间训练/练习。**决定（需确认）**：现 web-quiz 的「刷题(科目随机)/错题集」是否保留为空间内的可选项，还是随重构移除，学生端只剩 训练/练习？
  - submissions/judge_jobs/user_problem_progress：判题基础设施保留（加 space_id 或保留全局按 problem 维度——训练与练习都提交同一 problem，进度按 user+problem 全局）

## 5. 训练（空间内）功能细则

- 创建：自建（空，再添加题目）或「从仓库选择训练/练习」→ 拷贝结构到空间训练（复制章节/条目；题目不复制只引用）
- 编辑：名称、章节增删改名排序、题目增删排序（拖拽）
- 做题规则：章节/题目顺序浏览；客观题「选择次数上限」= 每训练题目配置 maxAttempts（默认？如 3 或无限？）——**每道题可选次数上限在训练创建/编辑时逐题或全局配置？需确认**；达上限后再选 → 标红禁用；答对 → 标绿；编程题不限制次数（提交评测）
- 进度视图按题状态（未做/做对绿/错达上限红）

## 6. 练习（空间内）功能细则

- 创建：自建或从仓库选训练/练习拷贝结构
- 作答模式：整份试卷一次进入；客观题作答可改；编程题编辑+测试（测试算不算作答？）；「交卷」后统一判定/展示结果
- 交卷后：客观对错+正确项、编程题最终 verdict；是否允许重做/再交？需确认（一次 or 多次覆盖）

## 7. 前端改造

- web（主站）→ 仓库页：登录（角色化）→ 域选择（global）→ 三栏题目管理（去训练练习编制 UI）
- web-quiz → 门户：登录后
  - 空间选择器（member 多空间 / domain_admin 其域内空间 / global_admin 选域+空间）
  - 空间内 Tab：训练 / 练习（+原随机刷题/错题视保留而定）
  - 训练页：列表（来自当前空间）→ 详情章节 → 做题（限次 UI/绿红状态）
  - 练习页：列表 → 试卷作答 → 交卷 → 结果
- 管理页：global_admin 域管理（CRUD/域管理员）；domain_admin 空间管理（建空间/改名/删/成员拉入/移除）；空间内训练练习管理

## 8. 题目 UUIDv7

- store.migrate：problems 加 `uuid TEXT`（补列）→ 存量行逐行生成 uuid（UUIDv7 或 v4？用户要 v7；Go 实现需库或手写（google/uuid 无 v7？有 v7 需第三方/自己编码）——查证）
- 导入 problems.json：解析到题目若无 uuid 字段 → 生成；有 uuid 且库中已存在 → 跳过该题（去重）
- 导出 problems.json 带 uuid 字段（新增可选字段，上游不认也无碍——export 结构加字段需与 zipio ExportProblem 兼容：加 `uuid,omitempty`，旧版导入忽略）
- 备份恢复亦按 uuid 去重

## 9. 分阶段（建议顺序）

1. 数据层：domains/spaces/space_members 迁移 + problems.domain_id/uuid；账号角色扩展（含迁移旧数据）
2. 域管理 + 仓库（题目域隔离）API；仓库前端改造（域切换 + 去训练练习 UI）
3. 空间管理 API + 成员
4. 空间训练（含限次）+ 空间练习（含交卷）API 与存储
5. 前端门户（空间切换/训练页/练习卷面/结果页）
6. 判题/提交数据空间化 + 清理旧模块（科目随机刷题等按决策）
7. 端到端验证 + 文档

## 10. 待用户确认清单（已全部定案，见第 0 节决策表）

A. 仓库保留训练/练习模板（结构管理，空间拷贝）✅
B. 移除随机刷题/错题集，刷题改为「空间刷题项目」新模型 ✅
C. 训练尝试次数 = 训练级统一上限（如默认 3），计数只存在于训练内（用户在该训练内对该题作答次数，重进不清零）✅
D. 练习可重做，每次作答结果都记录 ✅
E. 域管理员管域内全部空间，无独立空间管理员 ✅
F. 空间成员一律学生，训练/练习内容由域管理员管理 ✅

## 11. 追加需求（用户 2026-09-05 补充，已确认）

### 11.1 排行榜（按域、单一总榜、uuid 去重）
- 记录：学生每次答对客观题 / 编程题 AC，按 **problem.uuid 去重**记录一次通过
  （student_solved：user_id, problem_uuid, solved_at；每用户每 uuid 仅一行，UNIQUE(user_id, problem_uuid)）
- 通过计数来自全部来源：空间训练、空间练习、空间刷题 三处作答（客观答对 / 编程 AC）都计
- 排名：**按域排名**（学生通过总数降序；同分按先达成者靠前），**单一总榜**；
  管理员（global_admin/domain_admin）不参与排名
- 展示：学生端门户可见排行榜页；仅统计 role=member 且（在该域任一空间）的学生
- 依据 uuid 而非 problem_id：跨库迁移/导入去重后 id 变化不影响统计连续性

### 11.2 刷题（空间内第三类项目，替代原随机刷题）
- 定位：与训练/练习并列的独立页面（空间刷题列表）
- 生成来源（管理员布置，两种）：
  a) **标签筛选**：从域题库筛 单选/判断题（type IN single_choice,true_false）+ 标签条件
     → 动态题集（与现科目分类刷题同玩法：随机轮次即时反馈，答对移出/记录）
  b) **绑定仓库题单**：选择仓库中训练/练习模板 → 取其全部客观题生成刷题项目
- 规则：仅含客观题（选择/判断）；答对记通过（uuid 去重一次）；**答错不限次数**；
  通过数计入排行榜；项目可见性=布置给空间（空间成员在其空间刷题页可见）
- 表：space_quizzes(id, space_id, title, tags_json(来源 a 标签), source_type(‘tags’|‘repo’),
  repo_kind(‘training’|‘practice’)+repo_id(来源 b), created_at)；
  题集动态解析：tags 型=按标签查域题库客观题；repo 型=读仓库模板条目取客观题

## 12. 数据模型补充（第 4 节后追加表）

```sql
-- 排行榜（uuid 去重通过记录；域级——按 problem 所属域 + user 关联域查询）
student_solved(
  user_id INTEGER NOT NULL, problem_uuid TEXT NOT NULL,
  solved_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY(user_id, problem_uuid)
);
-- 空间刷题项目
space_quizzes(
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  space_id INTEGER NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  tags_json TEXT NOT NULL DEFAULT '[]',      -- 来源 a：标签筛选
  source_type TEXT NOT NULL DEFAULT 'tags',  -- tags | repo
  repo_kind TEXT NOT NULL DEFAULT '',        -- 来源 b：training | practice
  repo_id INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

## 13. 分阶段修订（含新增）

1. ✅ UUIDv7（commit ddf33fd）
2. ✅ 域/空间数据层 + 账号角色（commit 23498d4）
3. ✅ 域/空间/用户管理 API + respondError 修复（commit 4618fe9）
4. 空间训练/练习后端：表迁移 + CRUD + 从仓库模板拷贝 + 训练限次 + 练习整卷交卷
5. 刷题项目后端（space_quizzes + 两种来源解析）+ 排行榜后端（student_solved + 通过记录注入三处作答）
6. 判题提交空间化 + quizstore 按域/空间过滤 RepoReader
7. 仓库页前端（web）：域切换 + 空间管理 UI
8. 门户前端（web-quiz）：空间切换 + 训练页（限次标色）/练习卷面/刷题页/排行榜
9. 端到端验证 + 文档 + 旧模块退役
