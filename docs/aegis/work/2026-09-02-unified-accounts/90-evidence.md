# 账号统一（unified-accounts）— 证据

## 验证记录（全部通过）

- `go vet ./...` ✅；`go test ./...` 全 ok ✅（accounts/quizserver/quizstore/server/store/zipio）
- `GOOS=linux CGO_ENABLED=0 go build ./...` ✅（容器侧交叉编译）
- 双前端 `npm run build` ✅（web 19.47s / web-quiz 16.09s）
- 实时 E2E（主站 :8086 + 刷题 :8087 共享临时数据目录）12/12 PASS：
  双服务启动 / 主站 admin 登录 / 刷题服务同账号登录（统一生效）/ 系统管理可见学生与管理员 /
  学生登录主站被拒(403) / 刷题端改密后主站用新密码登录成功 / 主站旧密码失效(401) /
  改密后旧会话字面 token 失效(401，curl 字面验证)
- 迁移单测 `TestLegacySettingsMigration`：旧 settings 哈希 → admin 账号（原密码可登录）+ settings 清理 + 幂等

## 提交历史（本工作流）

- db454be internal/accounts + quizstore/quizserver 迁移
- （待）主站认证改造 + 前端 + 管理员管理 + 文档

## 过程备注

- E2E 中一次 FAIL 为脚本假阴性：Invoke-RestMethod 自动更新 WebSession cookie，改用 curl 字面 token 复测通过
- 首页引导顺序：主站先启动则主站创建 admin/123456；刷题服务先启动则其引导创建；并发时唯一约束容错
- quiz 服务启动要求主库存在（OpenRepoReader 探活）；E2E 单起 quiz 前需先初始化主库