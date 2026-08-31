# 刷题服务（quiz-service）— 证据

## 验证记录（全部通过）

- `go vet ./...` ✅；`go test ./... -count=1` ✅（store/quizstore/quizserver/server/zipio 全 ok）
- `web: npm run build` ✅（✓ built in 1.16s）
- `web-quiz: npm run build` ✅（TS + Vite 构建成功，15.1s）
- 实时 E2E（main :8086 + quiz :8087 共享同一临时 data 目录，主站写、刷题服务只读）12/12 PASS：
  启动 ×2 / 主站建题（带题解解析）/ 科目分类学生创建 / 题目数预览=1 / 学生分类题目数=1 /
  抽题(1/1 + hasExplanation) / 答错判定（返回 correct=false + correctAnswer + explanation）/ 错题入集(1) /
  错题练习(1 题) / 答对判定 / 答对后错题集清空(0)

## 提交历史

- 38dbd0a 规格（spec+INDEX）
- 26118b7 实施计划（plan+INDEX）
- 882c0a3 工作记录（intent/checkpoint）
- 629e393 Slice A：store 导出改名 + quizstore（quiz.db + 只读 reader）+ 测试
- 10c5b14 Slice B：quizserver（auth/quiz/admin）+ cmd/quiz + 冒烟测试
- a7b8e5a Slice C：web-quiz 前端
- （待）UpdateCategory 严格契约 + package-lock
- （待）Slice D：dev-quiz.ps1 + README + 最终记录

## 过程备注

- npm 环境泄漏 `npm_config_global=true`（宿主以 npm exec 运行），npm install 需先移除该变量；
  被中断的安装曾产生损坏的 rolldown 原生绑定（13MB 无效镜像），`--prefer-online` 重装后恢复正常（20.9MB）
- E2E 中发现主站 API 会丢弃空 language 的题解（zipio 归一化）—— 解析标记判定与主站归一化语义一致，非缺陷