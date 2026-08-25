---
status: accepted
owner: Dev Team
last-reviewed: 2026-08-25
---

# Seedance 模型接入设计索引

本目录按 Provider 收纳 Seedance 模型接入目标架构。所有线路使用 `ChannelTypeSeedanceLink` 和
ModelArk V3 北向；各 Provider 可以使用不同代码化上游协议、路径、鉴权、任务信封和素材方式。
不同线路必须使用不同客户模型名，每个客户模型只对应一个已启用 Seedance Channel。

下游通用合同见[公开模型调用与素材能力元数据设计](公开模型调用与素材能力元数据设计.md)：客户模型
可以使用部署方自己的名称，目录通过 `api.video`、`api.assets` 和匿名 `reuse_scope` 描述能力，不返回
上游身份。每个 Provider 子目录统一只保留四份文档：原始技术资料整理、模型与素材库对接设计、模型价格与计费、模型与素材能力元数据；README 不再作为第二套事实入口。

## Provider 设计

跨 Provider 的官方价格基准统一见
[Seedance 国内火山与海外 BytePlus 官方价格基准](Seedance国内火山与海外BytePlus官方价格基准.md)。
Provider 专题只记录自身客户价格、第三方报价和履约边界，不复制官方主价格表。

| Provider | 设计入口 | 当前状态 |
| --- | --- | --- |
| 火山官方 | [模型与素材库对接设计](火山官方/火山官方模型与素材库对接设计.md) | `modelark_v3_volcengine` + `volcengine_assets_action_v2024_01_01`；官方模型、价格与 Action 素材合同已代码化，账号开通和生产账单需逐模型验收 |
| BytePlus 官方 | [模型与素材库对接设计](BytePlus官方/BytePlus官方模型与素材库对接设计.md) | `modelark_v3_byteplus` + `byteplus_assets_action_v2024_01_01`；海外 `dreamina-*` 身份与国内 `doubao-*` 隔离，目标账号验收未完成 |
| 移动云 | [模型与素材库对接设计](移动云/移动云模型与素材库对接设计.md) | 目标 `modelark_v3_cmcc` + `cmcc_aicc_assets_v2`；Standard 的 raw REST 已实测文本、480p/720p/1080p 与 `reference_video`，代码、素材和账单验收未完成；Fast、Mini、条件 2.5 仍为无 Provider 证据候选 |
| FunCloud | [模型与素材库对接设计](FunCloud/FunCloud模型与素材库对接设计.md) | 四个独立客户模型；同一连接可在一个 Channel 配置多个映射，Standard/Fast/Mini 可选虚拟素材，2.5 无素材；真实模型、素材和账单仍需生产验收 |
| 墨行（含 TokenSave 海外转售） | [模型与素材库对接设计](墨行/墨行模型与素材库对接设计.md) | 五个客户模型、三个视频协议和三种素材协议；Fast 有真实终态 usage 与账单一致证据，Mini/2.5 未发布 |
| 飞彩 | [模型与素材库对接设计](飞彩/飞彩模型与素材库对接设计.md) | 专用 `feicai_videos_v1`；十个固定分辨率模型，`asset_upstream_protocol=none`，不提供素材 CRUD |

Seedance 各线路已于 2026-08-24 手动完成生产验收，验收口径见
[全渠道上线验收手册](../../40-operations/05-全渠道上线验收手册.md)。架构文档只保留稳定边界
和证据结论，不承载逐次调用流水。

`status: accepted` 表示设计边界已确定，不表示代码、渠道或生产分组已开放。配置权威是客户模型、
Channel、`model_mapping`、价格和代码协议；是否兼容与是否上线由技术人员线下验证后通知管理员。
系统不建立 publication、SKU、implementation 或 execution binding 自动门禁。

## 共同上位架构

- [Seedance 专用渠道与 Link 架构](../Seedance专用渠道与Link架构.md)
- [异步任务与计费事实架构](../账单计费-异步任务与计费事实架构.md)
- [API Key 用量账单架构](../账单计费-APIKEY用量账单架构.md)
