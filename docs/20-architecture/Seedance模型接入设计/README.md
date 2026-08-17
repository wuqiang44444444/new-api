---
status: accepted
owner: Dev Team
last-reviewed: 2026-08-15
---

# Seedance 模型接入设计索引

本目录按 Provider 收纳 Seedance 模型接入目标架构。所有线路使用 `ChannelTypeSeedanceLink` 和
ModelArk V3 北向；各 Provider 可以使用不同代码化上游协议、路径、鉴权、任务信封和素材方式。
不同线路必须使用不同客户模型名，每个客户模型只对应一个已启用 Seedance Channel。

下游通用合同见[公开模型调用与素材能力元数据设计](公开模型调用与素材能力元数据设计.md)：客户模型
可以使用部署方自己的名称，目录通过 `api.video`、`api.assets` 和匿名 `reuse_scope` 描述能力，不返回
上游身份。

## Provider 设计

跨 Provider 的官方价格基准统一见
[Seedance 国内火山与海外 BytePlus 官方价格基准](Seedance国内火山与海外BytePlus官方价格基准.md)。
Provider 专题只记录自身客户价格、第三方报价和履约边界，不复制官方主价格表。

| Provider | 设计入口 | 当前状态 |
| --- | --- | --- |
| FunCloud | [Seedance 全模型接入设计](FunCloud/README.md) | 四个独立客户模型与 Channel 已实现；Standard/Fast/Mini 支持统一北向虚拟素材，2.5 无素材；真实模型、素材和账单仍需生产验收 |
| 墨行 | [TokenSave 与墨行 Seedance 接入设计](墨行/README.md) | 五个客户模型、三个视频协议和三种素材协议已实现为可配置合同；Fast 已取得真实终态 usage 与账单一致证据，实际用量结算已实施；Fast/Mini/2.5 未发布 |
| 飞彩 | [Seedance 全模型接入设计](飞彩/README.md) | 使用专用 `feicai_videos_v1`；SD2 开放 16:9/9:16，其余九个固定分辨率模型开放六种登记画幅 |

逐模型生产缺口和当前验收顺序集中记录在
[路线图的 Seedance 生产验收](../../50-planning/路线图.md#seedance-生产验收)。架构文档只保留稳定边界
和证据结论，不承载逐次调用流水。

`status: accepted` 表示设计边界已确定，不表示代码、渠道或生产分组已开放。配置权威是客户模型、
Channel、`model_mapping`、价格和代码协议；是否兼容与是否上线由技术人员线下验证后通知管理员。
系统不建立 publication、SKU、implementation 或 execution binding 自动门禁。

## 共同上位架构

- [Seedance 专用渠道与 Link 架构](../Seedance专用渠道与Link架构.md)
- [异步任务与计费事实架构](../异步任务与计费事实架构.md)
- [API Key 用量账单架构](../API-Key用量账单架构.md)
