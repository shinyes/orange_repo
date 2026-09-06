# OrangeOJ 单进程单前端收敛（架构简化）设计

日期：2026-09-05 · 状态：已确认（用户拍板）

## 决策
- **单 Go 进程**：管理端（域仓库）+ 学生门户合并为一个服务（单一端口）
- **API 全合并一个 /api**（管理 + 门户统一前缀，同 cookie 同登录态）
- **单 React 前端**：一个应用内 /（学生门户）与 /admin（仓库管理区），共用登录/API/组件
- **judge-runtime 独立保留**（nsjail 沙箱无法同进程；仍三容器之一但主服务单一）
- 删除无用代码：双进程编排、cmd/quiz 独立入口、web-quiz 独立 Login/品牌/基建、重复 util

## 目标形态
```
orangeoj 单进程 (:8080)  ── 托管单前端 dist
  ├─ /           学生门户（空间切换/训练/练习/刷题/排行榜/做题）
  └─ /admin      仓库管理（域切换/域管理/空间管理/题目/标签/导入导出）
  /api/*         全部后端 API（统一会话 cookie、统一鉴权中间件）
orangeoj-judge (:9090)  ── 判题沙箱（唯一独立进程）
```

## 执行顺序
1. ✅ 后端合服：main.go 单入口挂全部路由（server 包管理 handler + quizserver 门户 handler 同一 app）；
   cmd/quiz 删除；会话统一（同一 cookie/中间件/账号）
2. ✅ 部署清理：compose 两容器、dev/test 脚本单进程化、Dockerfile 二进制 orangeoj、README 更新
3. ✅ 前端合并：新开 app/ 单前端——门户 / 全量迁入 + 管理 /admin 全量并入（api 统一层、AdminLayout、
   域切换/角色守卫、路由化）；web/ 与 web-quiz/ 删除；后端/部署/脚本单 dist
4. ✅ 清理：web/ 与 web-quiz/ 退役；构建/部署指向单 dist
5. ✅ 验证：全测试绿 + 单进程真实 E2E（test-oj.ps1 PASS=13）

## 最终形态（已达成）
```
app/                 单前端（React+TS+Vite）：/ 门户 + /admin 管理（单 dist）
main.go              单 Go 进程：管理 API + 门户 API + 静态托管（-web ./app/dist）
cmd/judge-runtime    判题沙箱（唯一独立进程 :9090）
orangeoj.db          唯一数据库（题库/域/空间/账号/判题/作答）
```

## 前端合并布局（新目录 app/ 或 web-app/，待定）
```
<新前端>/src/
  api/          统一 API 层（管理 + 门户；两套 api.ts/types.ts 合并）
  components/   共享（ui/*、markdown、code-highlight、monaco、登录）
    admin/      管理组件（Sidebar/DomainAdmin/SpaceAdmin/ProblemPane/…）
    portal/     门户组件（objective 等）
  pages/
    portal/     SpacePicker/SpaceShell/Training*/Practice*/Quiz*/Rank/MyPage/ProblemSolve
    admin/      DomainAdminPage/SpaceAdminPage/ProblemList 等
  App.tsx       路由：/ 门户区 + /admin 管理区；登录态统一
```
