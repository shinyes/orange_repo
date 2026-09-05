# OrangeOJ 重构 —— 证据记录

## 提交与验证证据（全部在本地 main，未推送）
| Commit | 内容 | 验证 |
|---|---|---|
| ddf33fd | 题目 UUIDv7 | go test ./... 绿；TestProblemUUID |
| 23498d4 | 域/空间数据层+三角色 | TestMigrateOldRoles/TestUsers 绿 |
| 4618fe9 | 域/空间/用户管理 API+respondError 修复 | TestDomainSpaceAPI（跨域 403）绿 |
| 0aa55f9 | 空间内容 store 层+规格追加 | TestSpaceTrainingFlow 等绿 |
| ace66ea | 空间内容管理 API+作答迁 quiz.db | 全测试绿 |
| 26ba675 | quizserver 门户 API | 全测试绿 |
| b826d92 | 门户端到端集成测试 | TestPortalMemberFlow PASS（限次409/标绿/交卷/刷题/排行去重2题） |
| 17e697e | 题目域隔离全面收紧 | 全测试绿 |
| 8c195ab | web 仓库页域化+后端缺口补齐 | web tsc/build 绿；go 全测试绿 |

## 关键修复证据
- respondError 语义：原写响应返回 nil → Fiber 中间件/handler 不中断（鉴权形同虚设）；
  改为返回 *fiber.Error 由 ErrorHandler 统一 JSON（server + quizserver）——TestDomainSpaceAPI 403 生效为证

## 运行中
- 阶段 6：web-quiz 门户空间化改造（子代理 a28f0d71）
