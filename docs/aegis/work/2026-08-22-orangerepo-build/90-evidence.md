# 证据包

1. `go vet ./...` 通过；`go test ./...`：
   - internal/zipio：BuildZip↔ParseZip 往返语义等价、trainingPlan 下标映射、任意层级文件定位、(images/ 引用重写、三种题型归一化（answerIndex/answer bool/默认时限/题解语言别名）、非数组题解报错 —— ok
   - internal/server：认证流（401/登录/改密后旧密码失效）、目录树计数、三题型创建归一化断言、标签筛选与搜索、题目移动/删除、训练导出→新库导入→章节条目与图片落盘比对 —— ok
2. `cd web && npm run build`（tsc -b + vite build）成功。
3. 生产模式实机冒烟（orangerepo.exe -seed，临时数据目录）：
   - GET /api/health=200；GET / 返回前端 index.html（root 挂载点存在）
   - 未认证 /api/problems=401；login=204；种子导入 4 题；训练「示例训练计划」3 题
   - 标签汇总：入门/语法基础/循环/模拟
   - 导出训练 ZIP 结构：problems.json + trainingPlan.json + images/sample-figure.png
   - 该 ZIP 以 practice 模式重导入=201（3 题 + 练习建立）；图片经 /api/uploads 服务 200
4. 控制台中文显示乱码为 PowerShell 终端编码所致，存储与 HTTP 内容均为 UTF-8 正常。
