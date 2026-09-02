# OrangeOJ 判题扩展（orangeroj-build）— 证据

## 验证记录（全部通过）

- `go vet ./...` ✅；`go test ./...` 全 ok ✅
  - internal/judgeserver：**真实 Python 与 C++ 评测冒烟** —— Python AC/WA/空白归一/RE/TLE/run-OK/语法错误；
    C++（本机 g++ 16.2 mingw）AC/CE/RE；不支持语言 → RE
  - internal/judge：队列单测（用例选择 run/test/submit/回退、写回 done+score、progress upsert、runner 失败兜底 RE）
  - internal/quizserver `TestOJFullFlow`：httptest 全链路（布置定向/全体、可见性、撤回、客观题 WA→AC、
    编程题真实评测 test/submit/run/WA、提交历史、进度完成态、管理统计、越权 403、删除布置）
- `GOOS=linux CGO_ENABLED=0 go build ./...` ✅（含 cmd/judge-runtime 与 Linux nsjail 后端交叉编译）
- 双前端 `npm run build` ✅（web 1.8s / web-quiz 29s；oxlint 0 error）
- **实时 E2E（三进程，真实评测）** 全部符合预期：
  - 主站 :8090 造 A+B 编程题（3 测试点）+ 判断题 → 训练章节
  - 刷题/OJ :8091：admin 建学生 stu1 → 布置训练（定向）→ stu1 任务列表/训练详情正确（testCases 未泄漏）
  - Python submit → **AC score=100（3 用例）**；Python run(自定义输入 8 9) → **OK stdout=17**
  - C++ submit（g++ 编译运行）→ **AC score=100**；错误代码 submit → **WA**
  - 判断题 objective-submit → **AC**；训练完成态 **2/2**
  - 管理端统计：A+B 通过 1 人/提交 3 次；判断题通过 1 人/提交 1 次
  - 提交历史 4 条齐全（submit:AC → run:OK → submit:AC → submit:WA）
- 数据层快照：quiz.db 新表全部 `CREATE TABLE IF NOT EXISTS` 幂等（旧库平滑升级）；主库仅只读

## 提交历史（本工作流）

1. `4e77ea1` feat(judge): OrangeOJ 判题内核 —— judge-runtime 沙箱评测 + 判题队列 + 提交/布置数据层（docs + cmd/judge-runtime + internal/judge + internal/judgeserver + quiz.db 迁移 + quizstore + quizserver 路由）
2. `44120d6` feat(web-quiz): OrangeOJ 训练/练习/做题页与布置管理 UI（学生端导航/列表/详情/做题页 run-test-submit/测评记录；管理端布置 Tab）
3. （本次提交）feat(deploy,docs): Dockerfile.judge + compose 三服务 + dev-quiz.ps1 + README/INDEX/BASELINE + 证据

## 过程备注
- 上游 OrangeOJ 源码快照下载自 codeload.github.com（github.com 直连被墙但 codeload/raw 可达）：
  `$TEMP\OrangeOJ-src`，判题相关 Go 后端全部以之为基线逐行核对
- Windows 本机安装 MinGW 16.2（winlibs，D:\tools\mingw）后 C++ 真实评测可用；Python 解析注意过滤
  WindowsApps 商店 stub（python3.exe → 9009），工具链解析已内置过滤与常见位点探测
- 判题表结构与上游一致（去 space_id）；布置模型 = 上游 training_participants/practice_targets 语义统一为
  assignments.assigned_all/assigned_students；训练/练习结构实时读主库（与上游同库同构），不落快照
- Windows 开发沙箱无 nsjail：仅限时/隔离目录，README 与代码注释均明确「无安全隔离承诺，仅联调用」
- compose 中 orangejudge 需 privileged + cgroup host（nsjail），沿用上游 docker-compose.build.yml 配置；
  镜像 `orangeoj-judge:local` 本地构建，主镜像 GHCR 流程不变
