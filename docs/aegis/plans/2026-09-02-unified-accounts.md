# 账号统一（unified-accounts）实施计划

日期：2026-09-02 · 依据：刷题服务规格 §7.1（2026-09-02b 修订）+ 主站规格修订行；用户已确认方案 A（账号数据统一）+ 主站用户名+密码登录

## 1. 目标

主站（OrangeRepo）与刷题服务共享同一账号库（users/sessions 位于 quiz.db）：一套账号、一个密码、一个管理入口；旧主站部署无感迁移。

## 2. 文件映射

```
internal/accounts/accounts.go   新建：quiz.db 连接 + users/sessions 迁移 + 全部用户/会话方法（唯一 owner）
internal/quizstore/quizstore.go 修改：用户/会话方法迁出 → Store.Accounts 委托；Open 内跑 accounts.Migrate
internal/quizserver/*.go        修改：调用点改为 s.QS.Accounts.*
internal/server/auth.go         重写：用户名+密码登录（限 admin）、会话表、me 返回 user、改密清会话
internal/server/server.go       修改：requireSession 校验 admin 角色
main.go                         修改：启动时 OpenAccounts + 旧哈希迁移 + 引导
cmd/quiz/main.go                修改：Open 走 accounts、EnsureBootstrap 容错冲突
web/src/components/Login.tsx    修改：加用户名输入框
web/src/lib/api.ts              修改：login(username,password)、me 形状
web-quiz/src/pages/AdminPage.tsx 修改：学生tab 增「管理员账号」区块（列出 + 重置密码）
internal/quizstore 校验测试、internal/server 测试、internal/accounts 新测试
docs/（README/规格/INDEX/work 记录）
```

## 3. 兼容边界与迁移

- 主站登录契约：`POST /api/auth/login {password}` → `{username,password}`；旧部署升级后 `admin` 账号沿用旧密码（settings 哈希迁移），会话自动失效需重新登录（一次性）
- quiz.db 物理位置与文件不变；部署结构不变（compose 共享卷）
- 刷题服务对外 API 不变（登录本来就带用户名）；越权规则不变（学生可登刷题、不可登主站——主站 requireSession 增加 admin 角色校验）

## 4. TDD Route

```text
TDD Route:
- Mode: off
- Decision: skipped
- Strict authority: not applicable
- Test posture: 迁移单测 + 既有冒烟更新 + 端到端回归（仓库既有姿势）
- Verification: go vet ./... ; go test ./... ; web & web-quiz 双 build
```

## 5. 任务切片

### Slice A — internal/accounts（新建）

**A1** `internal/accounts/accounts.go`
- Files: create
- 内容：`OpenDB(dataDir) (*sql.DB, error)`（quiz.db DSN 与主库一致：WAL/foreign_keys/busy_timeout，SetMaxOpenConns(1)）+ `Migrate(db)`（users/sessions 两表 DDL）+ `New(db) *Store`；方法从 quizstore 平移：`CreateUser/GetUserByUsername/GetUserByID/HasAdmin/ListStudents/DeleteStudent/SetStudentPassword/CheckPassword/SetPassword/CreateSession/GetUserByToken/DeleteSession/RotateSession/ListAdmins` + 新增 `CreateAdminFromHash(username, hash)`（旧哈希迁移）与 `TableHasUserByID` 用不到的不搬
- Verification: `go build ./...`

### Slice B — quizstore/quizserver 迁到 accounts

**B1** quizstore.go：删除用户/会话方法与其测试覆盖部分；`Store` 增 `Accounts *accounts.Store`；`Open` 中 `accounts.Migrate(db)` + `Accounts: accounts.New(db)`；settings 的 round_size 不动
**B2** quizserver 调用点：auth.go/admin.go 内 `s.QS.CreateUser(...)` 等 → `s.QS.Accounts.CreateUser(...)`（批量机械替换）
**B3** 测试更新：quizstore 测试里用户/会话用例移到 accounts 测试；quizserver 冒烟不变量
- Verification: `go test ./internal/accounts ./internal/quizstore ./internal/quizserver`

### Slice C — 主站认证改造

**C1** internal/server/auth.go 重写：
- loginRequest{username,password}；`accounts.CheckPassword` + `role==admin` 校验（否则 401/403）
- `CreateSession` → 会话表；logout 删 token；handleMe 返回 `{authenticated, user}`
- changePassword：旧密码校验 → SetPassword → 清该用户全部会话 → 新建会话
**C2** internal/server/server.go requireSession：GetUserByToken + role==admin
**C3** main.go：`OpenAccounts`（accounts.OpenDB + Migrate）→ `EnsureBootstrap` 改为：无 admin 时（a）旧 settings 哈希存在 → CreateAdminFromHash("admin") + 删 settings 键；（b）否则创建 admin/123456；日志提示
**C4** 测试：server_test.go 登录体改 {username:"admin",password}; 增迁移测试（造 settings 哈希 → 启动 → admin 可用旧密码登录 + settings 清理 + student 登录主站 403）
- Verification: `go test ./internal/server ./internal/accounts`

### Slice D — 前端与系统管理

**D1** web Login.tsx + api.ts：用户名输入、login(username,password)、me 用户信息
**D2** web-quiz AdminPage 学生 tab：顶部「管理员账号」区块（ListAdmins + 重置密码对话框复用）
**D3** 双前端 build + 手动冒烟（主站 admin 登录；刷题 student 登录；student 尝试主站登录被拒；改密后两边重登）
- Verification: 两 `npm run build`

### Slice E — 文档与全量验证

**E1** README：登录说明（主站用户名+密码、默认 admin/123456、账号统一说明）+ 规格 INDEX 登记计划
**E2** 全量：`go vet ./...; go test ./...;` 双前端 build；E2E 双服务同 data 目录（主站建题 → 刷题管理 → 统一账号互斥校验）
- Verification: 上述命令全绿

## 6. 风险

- **并发引导**：两容器同时首次启动创建 admin → 唯一约束容错（一方 ErrConflict 记日志跳过），行为幂等
- **老会话**：迁移后旧 orange_session 失效，用户重新登录一次（预期行为）
- **两端密码哈希一致**：迁移直接沿用旧 bcrypt 哈希 → 旧密码无缝可用
- 回滚：全部改动可逆（settings 清理前保留旧键直到迁移成功；失败不删键）

## 7. 退役

- settings 的 password_hash/session_token 键退役（迁移成功后删除）；无外部契约依赖（仅本仓库两个服务）
- quizstore 内旧的用户/会话方法退役，owner 收敛为 internal/accounts