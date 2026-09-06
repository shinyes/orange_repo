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
1. 后端合服：main.go 单入口挂全部路由（server 包管理 handler + quizserver 门户 handler 同一 app）；
   cmd/quiz 删除；会话统一（同一 cookie/中间件/账号）
2. 前端合并：以 web-quiz（router）为基座并入 web 管理页；统一登录/api/域名；
   删除 web 独立入口与重复基建
3. 清理：双 dist 部署/scripts/compose 单服务化；无用代码删除
4. 验证：全测试绿 + 单进程真实 E2E
