# 刷题服务（quiz-service）— 检查点

## 当前切片
Slice A（数据层）：A1 store 导出改名 + A2 quizstore + A3 problems reader + A4 测试

## 已完成
- 规格已提交（38dbd0a）+ 用户批准
- 计划已保存并登记 INDEX

## 下一步
A1: internal/store 改名导出 TagMatchesSelected

## 阻塞
无

## 证据
- spec: 38dbd0a；docs/aegis/plans/2026-08-29-quiz-service.md
- 工作树快照：main @ 38dbd0a；预先存在改动（README/api-reference/.commandcode）不触碰