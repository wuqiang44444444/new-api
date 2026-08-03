---
status: current
owner: Dev Team
last-reviewed: 2026-08-02
---

# TokenSave doubao Seedance 2.0

> 核对来源：[TokenSave 模型页](https://tokensave.pro/docs/models/doubao-seedance-2-0-260128)，
> 核对日期：2026-08-02。本文记录 Provider 资料；平台当前执行规范见
> [视频模型 API 用户调用指南](../30-engineering/视频模型API用户调用指南.md)和
> [视频与素材渠道运维手册](../40-operations/02-视频与素材渠道运维手册.md)。

## 已验证合同

| 字段 | 当前值 |
| --- | --- |
| 模型 ID | `doubao-seedance-2-0-260128` |
| 创建接口 | `POST /v1/media/generations` |
| 查询接口 | `GET /v1/media/tasks/{task_id}` |
| 生成场景 | 文生视频、图生视频、参考生视频 |
| 分辨率 | `480p`、`720p`、`1080p` |
| 时长 | 4～15 秒整数，或 `-1` |

当前 V2 创建字段包括 `model`、`prompt`、`capability=video_generation`、`input_mode`、
`duration_seconds`、`resolution`、`aspect_ratio`、`control_mode`、`with_audio`；参考图片使用
`reference_images` 传递素材引用。平台客户端 `/api/v3/contents/generations/tasks` 会把统一字段适配为
这些 V2 字段，公开模型名与上游模型名一致。

Provider 的模型介绍提到图像、视频、音频等多模态参考能力，但当前公开参数表只明确给出了
`reference_images`，没有定义视频或音频参考输入字段。因此在获得可验证的具体字段前，不应把
营销能力描述直接写成可调用合同。

## 模型页列价

单位均为美元/秒；“视频输入”是模型页的计价维度，不代表当前公开参数已经定义了视频引用字段。

| 生成方式 | 分辨率 | 视频输入：是 | 视频输入：否 |
| --- | --- | ---: | ---: |
| 文生视频 | 480p | $0.1653 | $0.0679 |
| 文生视频 | 720p | $0.3559 | $0.1462 |
| 文生视频 | 1080p | $0.3647 | $0.3647 |
| 图生视频 | 480p | $0.1653 | $0.0679 |
| 图生视频 | 720p | $0.3559 | $0.1461 |
| 图生视频 | 1080p | $0.3647 | $0.3647 |
| 参考生视频 | 480p | $0.1653 | $0.0679 |
| 参考生视频 | 720p | $0.3559 | $0.1462 |
| 参考生视频 | 1080p | $0.3647 | $0.3647 |

价格是 Provider 页面事实，不自动等于本站最终售价或结算表达式。生产启用前仍需用真实任务与
Provider 账单验证计费维度、用量返回和失败退款语义。

## 与历史 Ark 线路的边界

`doubao-seedance-2-0-260128` 不是历史 `dreamina-seedance-2-0-260128` 的别名。当前线路不得：

- 使用 `/v1/ark/media/generations`；
- 选择 `third_party_reverse_proxy`；
- 将 doubao 模型映射为 dreamina 模型。

上述历史配置描述见 [Seedance 2.0 海外官 Key](Seedance%202.0%20海外官%20Key.md)，仅供追溯。
