# 检查点 — 完成态

- 计划 11 个任务全部完成并逐任务提交（master：556ce7b → 379492c，共 5 commit，工作树干净）。
- 偏差记录：
  1. 示例包生成改用 PowerShell（原计划 Go 临时程序），并修复 PS5.1 UTF-8 BOM 问题；
  2. main.go 提前至 server 任务实现（便于实机验证）；
  3. shadcn 新版为 Base UI「base-nova」风格，组件 API 与旧版略有差异，已按实际导出适配。
- 教训（防复发）：禁止用 PowerShell Get-Content/Set-Content 回写 UTF-8 源码文件（两次造成中文乱码与结构损伤，均以 write/edit 工具重写修复）。
- DriftCheck: continue —— 全程未偏离规格 §2 兼容契约；无新增 fallback/重复 owner；zipio 保持格式唯一权威。
