# 实施计划：斜杠嵌套标签树侧边栏（v1.1.0）

日期：2026-08-22 · 上游规格：`docs/aegis/specs/2026-08-22-orangerepo-design.md`（已更新至修订 b）
执行路由：inline · TDD Route：Mode off / Decision skipped（无严格 TDD 授权；采用实现后回归验证姿态）

## Goal

移除目录结构（DB/API/UI 全链路退役），标签升级为 `/` 嵌套层级并在侧边栏以树展示；
支持标签子树整体重命名/删除并同步更新所有涉及题目。用户已拍板：父标签前缀包含子孙、
目录数据直接丢弃、子树联动编辑。

## Baseline / Authority Refs

- 规格 §1/§4/§5/§6/§7 修订 b（本次权威边界：前缀命中规则、迁移丢弃、PATCH/DELETE /api/tags）
- OrangeOJ 兼容基线 §2 不变（zipio 禁止触碰语义，仅允许删掉 payload 中无上游对应的 directoryId 字段）
- 训练/练习、认证、图片上传均为非目标，禁止改动行为

## Compatibility Boundary

- ZIP 双向兼容保持字节级语义等价（现有测试为准）
- 对内破坏性变更（已获批准）：`/api/directories*`、`PUT /api/problems/:id/directory`、
  查询参数 `dirId/recursive`、payload 字段 `directoryId` 全部删除
- 旧库升级：启动时自动一次性迁移，目录数据丢弃

## Files

| 动作 | 路径 |
|---|---|
| 改 | `internal/store/store.go`（迁移、过滤、分面、Rename/DeleteTag） |
| 删 | `internal/server/directories.go` |
| 改 | `internal/server/server.go`、`internal/server/problems.go`、新增 `internal/server/tags.go` |
| 改 | `internal/model/model.go`（删 DirectoryID×2、DirectoryNode）、`internal/zipio/zipio.go`（payload 删字段） |
| 改 | `internal/store/store_test.go`、`internal/server/server_test.go` |
| 改 | `web/src/lib/types.ts`、`web/src/lib/api.ts`、`web/src/app-context.tsx` |
| 重写 | `web/src/components/Sidebar.tsx`（标签树） |
| 改 | `web/src/components/dialogs.tsx`、`web/src/components/ProblemPane.tsx` |

## Tasks

### T1 存储层：退役目录 + 迁移
- migrate(): 移除 `directories` 建表语句；末尾追加旧库迁移——用 **pinned *sql.Conn** 执行：
  检测 `sqlite_master` 含 `directories` → `PRAGMA foreign_keys=off`（必须与后续语句同一连接，
  因 training_items 外键引用 problems，DROP 需临时关闭）→ 事务内建 `problems_new`（无 directory_id，
  显式列拷贝）→ `DROP TABLE problems` → `RENAME` → 提交 → `DROP TABLE IF EXISTS directories`
  → `PRAGMA foreign_keys=on`。
- 删除：`flatDir`~`ptrSlice` 全部目录函数；`ProblemFilter.DirID/Recursive`；
  `problemSummaryCols` 与各 Scan 去 `directory_id`；Create/Get/UpdateProblem 同。

### T2 存储层：标签前缀语义
- 新增：
  ```go
  func tagSetMatches(tags []string, sel string) bool // t==sel || HasPrefix(t, sel+"/")
  func tagMatchesSelected(tags []string, selected []string) bool // ∀s∈S
  ```
- `problemWhere` 只保留 q/type/ids；`ListProblems` 查询后用 `tagMatchesSelected` 内存过滤。
- `ListTagFacets` 计数改前缀规则；候选集 = 字面标签 ∪ 虚拟祖先（逐段累加）∪ selected；
  effective(S△T) 同样走前缀匹配；排序不变（count desc, tag asc）。空过滤退化全局。

### T3 存储层：RenameTag / DeleteTag
- `ValidateTagPath(s string)(string, error)`：trim 非空、首尾非 `/`、无空段。
- `RenameTag(from,to)`: 全量载入 (id,tags_json)，重写 `t==from→to`、`HasPrefix(t,from+"/")→to+后缀`，
  保序去重；事务内仅 UPDATE 变化行；返回受影响题数。to 校验同 ValidateTagPath。
- `DeleteTag(tag)`: 过滤 `t==tag || HasPrefix(t,tag+"/")`；同上事务模式。

### T4 server 层
- 删 `directories.go`、4 条目录路由、`PUT /problems/:id/directory` 与 `handleMoveProblem/movePayload`。
- `parseProblemFilter` 删 dirId/recursive 分支；create/update handler 删 DirectoryID 接线与保留注释。
- 新增 `internal/server/tags.go`：
  - `handleRenameTag`（Patch /api/tags，body `{from,to}`，400=校验失败）→ `{updated}`
  - `handleDeleteTag`（DELETE /api/tags?tag=…，400=缺失/校验失败）→ `{updated}`
- model.go / zipio.go 删字段；全仓 grep `DirectoryID|directory_id|dirId` 归零（迁移检测字符串除外）。

### T5 后端测试更新
- store_test.go：修掉 `&dirB` 引用；删目录相关用例；新增：
  前缀筛选（`数学` 命中 `数学/几何`）、层级分面精确计数断言（含虚拟父节点）、
  RenameTag 子树搬家+合并去重、DeleteTag 子树、旧库迁移（手工建旧 schema 库→Open→列消失数据完好）。
- server_test.go：删目录流程；新增 PATCH/DELETE /api/tags 往返与 `GET /api/directories`→404 断言。
- 命令：`go vet ./... && go test ./...`

### T6 前端契约层
- types.ts：删 Directory 类型与 directoryId/dirId/recursive；ProblemSummary/Problem 去 directoryId。
- api.ts：删目录方法与 moveProblem；加 `renameTag(from,to)`、`deleteTag(tag)`。
- app-context.tsx：Filter 状态去 dirId/recursive 及相关 patch 调用点。

### T7 Sidebar 标签树重写
- `buildTagTree(tags: TagCount[]): TagNode[]`（full/label/count/children；服务端已含虚拟祖先计数）。
- 行点击切换选中（patchFilter tags）；祖先被选时后代淡显（implied）；已选 chips 行（完整路径）+清空。
- ⋮ DropdownMenu：重命名 Dialog（预填完整路径）→ `renameTag` → invalidate `['tags']``['problems']`；
  删除 ConfirmDialog（提示影响题数）→ `deleteTag`。
- 排序名称（zh-Hans-CN localeCompare）/数量每层生效；展开折叠全部；节点数 >20 时显示树内过滤输入框
  （沿用 TAG_SEARCH_THRESHOLD 思路）。训练/练习分区与退出登录按钮原样保留。

### T8 编辑器与残留 UI
- dialogs.tsx 删新建/重命名目录对话框及调用点；ProblemPane 标签输入改 chip 式
  （Enter/逗号添加 token 允许 `/`，Backspace 删除末个，建议列表来自现存标签），题目头部去目录徽标。

### T9 构建 + 发布
- `go vet ./... ; go test ./...`；`cd web; npm.cmd run build`。
- 提交：①`feat(store,server)!:` 目录退役+标签子树 API（含迁移）②`feat(web):` 标签树侧边栏
  ③docs 规格与计划。推送 main → 标签 `v1.1.0` 触发 Release 工作流（GHCR + 附件 tar.gz）。

## Risks / Retirement

- 迁移不可逆（丢目录）：已在设计阶段由用户确认；迁移代码幂等（重复检测无害）。
- 退役核验：完成后全仓 grep 目录符号应为零残留。
- 回滚面：v1.1.0 镜像回 v1.0.3 即回到目录时代（但库已迁移不可逆，README 不需提级说明——单用户自用）。

## Verification

每个 Task 的命令如上；最终以 T9 全绿 + CI Release 成功 + （可选）浏览器手动走查收口。
