// 管理端（/admin）：第二阶段迁入的 web/ 管理功能（题目管理工作区/域管理/空间管理）。
// 页面路由树见 src/app/App.tsx：
//   /admin/problems —— 题目管理三栏工作区（ProblemsWorkspace）
//   /admin/domains  —— 域管理（DomainAdmin，global_admin）
//   /admin/spaces   —— 空间管理（SpaceAdmin）
// 共享状态/壳：AdminLayout（顶栏+域上下文）、app-context（工作区 view/filter/checked）、
// domain-context（当前管理域，含 localStorage 持久化与 api 同步）。
export {}
