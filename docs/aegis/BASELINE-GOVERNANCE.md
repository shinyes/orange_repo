# Baseline Governance — OrangeRepo

## Baseline Roles
- Product/Requirement Baseline：`docs/aegis/specs/2026-08-22-orangerepo-design.md` §1/§6/§7（范围、信息架构、验收）
- Architecture/Runtime Boundary Baseline：spec §2–§5（OrangeOJ 兼容格式、数据模型、API 契约）

## 不变式
1. zipio 包是 OrangeOJ 格式的唯一权威实现；其他代码不得内联格式知识。
2. API/JSON 字段一律 camelCase，与上游一致。
3. 非 OJ 功能（判题/提交/成员/进度）永不进入范围。
4. 兼容性破坏必须先改 spec 并记录，再改代码。

## Drift 处理
- 发现实现偏离 spec → Implementation Drift：回归 spec 或经用户确认修订 spec。
- 发现 spec 与上游格式事实冲突 → 以源码事实为准修 spec，标注来源文件与行为。
