# OrangeRepo OJ 重构（域/仓库/空间）设计规格

日期：2026-09-04 · 状态：已确认（用户逐项拍板，§10 清单 A–F 全部定案）
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

## 10. 待用户确认清单（写代码前必须锁定）

A. 现主库 trainings/practices（含样例训练）与 booklet_directories 目录：删除？还是保留为域级「仓库模板」供空间从仓库选？（用户说训练页可"从仓库中选择训练或练习创建训练"——仓库里的训练/练习模板从哪来？若模板=仓库页可建的训练/练习，则仓库页仍需保留训练/练习的简单管理（无作答））
B. 现 web-quiz 的「刷题（科目随机抽题）/错题集」功能去留（空间化后保留 or 移除）
C. 训练中每道客观题的选择次数上限如何配置（全局统一默认？每题单独可配？默认值多少）
D. 练习交卷后能否重做（一次机会 or 可重复交卷取最好）
E. domain_admin 是否可管理其域内任意空间（不需要单独 space admin）
F. 空间成员角色是否分 教师/学生（影响界面权限：谁能建训练）或一律成员、由 domain_admin 统一在空间内管理内容
