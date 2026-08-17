---
status: current
owner: Dev Team
last-reviewed: 2026-08-15
---

# TokenSave 与墨行 Seedance 接入设计索引

本目录保留三份当前事实文档：

1. [渠道对接设计](渠道对接设计.md)：五个客户模型、精确映射、三个视频协议、逐模型 typed contract、
   南向转换、终态 usage 归一、durable attempt、计费与发布边界。
2. [素材库对接设计](素材库对接设计.md)：TokenSave、JoyCreator、墨行火山三种素材协议、能力矩阵、
   Asset/AssetGroup 作用域、Resolver、真人验证和失败语义。
3. [全模型与计费设计](墨行全模型与计费设计.md)：面向商务人员说明五个客户模型分别按秒或按 Token
   计费、当前美元价格、Fast 真实 usage/账单证据、实际用量结算规则和发布状态。

当前代码已经具备上述协议和本地合同测试，但“代码已实现”“渠道可配置”和“生产已发布”是三个不同
状态。Fast 已取得一次真实成功查询和账单一致证据；代码已删除 `IncludeVerifiedUsage`，成功终态中的
明确 usage/Token 数值会直接进入归一，Fast、Mini、2.5 按实际 completion tokens 结算，墨行 2.0 仍
按秒结算。Fast、Mini、2.5 仍保持可配置但未发布；所有线路都必须按客户模型完成剩余真实视频、素材、
账单和灰度验收后，才能启用生产流量。

## 上位架构

- [Seedance 专用渠道与 Link 架构](../../Seedance专用渠道与Link架构.md)
- [Seedance 官方素材库与素材引用设计](../../Seedance官方素材库与素材引用设计.md)
- [异步任务与计费事实架构](../../异步任务与计费事实架构.md)
