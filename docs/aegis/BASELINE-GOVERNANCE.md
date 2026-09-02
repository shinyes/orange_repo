# Baseline Governance — OrangeRepo

## Baseline Roles
- Product/Requirement Baseline：`docs/aegis/specs/2026-08-22-orangerepo-design.md` §1/§6/§7（范围、信息架构、验收）
- Architecture/Runtime Boundary Baseline：spec §2–§5（OrangeOJ 兼容格式、数据模型、API 契约）
- **OJ 判题边界 Baseline（2026-09-03 修订）**：`docs/aegis/specs/2026-09-03-orangeroj-design.md` ——
  用户决策将 OrangeRepo 扩展为支持判题的 OrangeOJ：独立 judge-runtime（仅 Python+C++）、
  布置体系（训练/练习布置给学生）、提交/进度/统计。**原不变式 3「非 OJ 功能（判题/提交/成员/进度）永不进入范围」已被本次用户决策推翻并退役**，
  判题类功能的准入边界改由本基线条目 + orangeroj spec §1 非目标约束。

## 不变式
1. zipio 包是 OrangeOJ 格式的唯一权威实现；其他代码不得内联格式知识。
2. API/JSON 字段一律 camelCase，与上游一致。
3. 判题/提交/进度的表结构与编排语义以 `docs/aegis/specs/2026-09-03-orangeroj-design.md` §0/§3/§4 为准
   （复刻上游 OrangeOJ queue.go/db.go），quiz.db 是唯一物理落点，主库（orangerepo.db）只读。
4. 真正执行用户代码的唯一进程是 judge-runtime（Linux 生产经 nsjail 隔离）；Windows/开发后端无安全隔离承诺。
5. 兼容性破坏必须先改 spec 并记录，再改代码。

## Drift 处理
- 发现实现偏离 spec → Implementation Drift：回归 spec 或经用户确认修订 spec。
- 发现 spec 与上游格式事实冲突 → 以源码事实为准修 spec，标注来源文件与行为。
- 判题内核（internal/judge、internal/judgeserver）与上游差异仅限：去掉 go/turtle 语言、
  space_id 维度、Windows 开发后端；发现偏差先回查上游源码再动手。
