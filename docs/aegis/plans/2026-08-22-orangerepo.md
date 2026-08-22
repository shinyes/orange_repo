# OrangeRepo 实施计划

日期：2026-08-22 · 规格：`docs/aegis/specs/2026-08-22-orangerepo-design.md`（本计划引用其全部契约，不重复正文）

```text
TDD Route:
- Mode: off
- Decision: skipped（无显式 strict 请求）
- Test posture: post-change regression（zipio 往返单测 + server httptest 冒烟）
- Reason: 绿色field新应用，兼容层风险用往返测试覆盖即可
- Verification: go test ./... ；web npm run build；端到端冒烟脚本
```

## 文件地图

```
main.go                      入口：flags(-addr :8080 -data ./data -seed)、静态托管 web/dist、BOOTSTRAP 日志
internal/model/model.go      全部类型与 JSON 形状（spec §2/§4）
internal/store/store.go      Open/Migrate/目录树装配/查询过滤辅助
internal/zipio/zipio.go      兼容层唯一权威：BuildZip/ParseZip/图片收集与重写/normalizeProblem/solutions 归一化
internal/zipio/zipio_test.go 往返一致性 + 题型归一化表驱动测试
internal/server/*.go         fiber 装配与会话中间件；auth/directories/problems/tags/images/trainings/practices/io_handlers
internal/server/server_test.go httptest 冒烟（登录→CRUD→导出→清库→导入→比对）
web/                         Vite+TS+Tailwind v4+shadcn；src/lib(api,types,markdown) src/components/{ui,layout,sidebar,problem,trainings,dialogs}
samples/orangeoj-sample.zip  三题型+章节+图片的示例包（构建期生成，提交入库）
scripts/dev.ps1              并发启动 go run . 与 vite dev
```

## 任务序列（每任务一提交）

1. **Go 骨架**：go.mod(fiber v2, modernc.org/sqlite, x/crypto)；model.go；store.go 迁移。验证 `go build ./...`
2. **zipio 兼容层 + 单测**（spec §2 全量规则）。验证 `go test ./internal/zipio/`
3. **store 查询/树装配 + server 认证与目录/题目/标签/图片路由**。验证 `go build && go vet`
4. **训练/练习路由 + 导入导出 io_handlers**
5. **server httptest 冒烟 + main.go + -seed**。验证 `go test ./...`
6. **示例包生成**（Go 临时程序产 samples/orangeoj-sample.zip 后删除）
7. **前端脚手架**：Vite TS + Tailwind v4 + shadcn init + 组件集；lib/types.ts 与 api.ts 对齐 spec §5
8. **两栏布局 + 目录栏**（工具栏/搜索/标签/目录树/题目列表/批量条）
9. **浏览栏**（题面 KaTeX 渲染/答案渲染/题解卡片/编辑表单三题型）
10. **训练练习界面 + 导入导出对话框 + 登录页 + 改密**
11. **端到端验证**：双构建、后台起服、HTTP 会话冒烟、浏览器路径核对

## 验证命令

```powershell
go vet ./... ; go build ./... ; go test ./...
cd web ; npm install ; npm run build
# 冒烟：start server → login → CRUD → export zip → re-import → counts equal
```

## 风险与回滚

- modernc.org/sqlite 在 Go 1.25 的兼容性 → 任务1即验证，失败降级 mattn(go-sqlite3)+CGO
- shadcn init 网络交互失败 → 手写组件文件（按钮/输入等样式自包含）
- 回滚面：每任务独立提交；data/ 目录可整体删除重置
