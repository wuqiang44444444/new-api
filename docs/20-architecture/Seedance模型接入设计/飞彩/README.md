---
status: accepted
owner: Dev Team
last-reviewed: 2026-08-15
---

# 飞彩 Seedance 全模型接入设计索引

飞彩 Seedance 通过 `ChannelTypeSeedanceLink` 接入，北向固定 ModelArk V3，南向使用代码化
`feicai_videos_v1` adapter。该协议支持请求级 URL/Data URL，并将非空 `asset://<opaque-id>` 原样交给
Provider；素材 CRUD 协议为 `none`，不支持素材管理、`ast_*`、`pubref_*` 或
Provider 素材库；Channel 必须配置 `asset_upstream_protocol=none`。

飞彩资料覆盖 10 个模型档位。代码以精确 Provider 模型表登记固定输出分辨率、时长、媒体边界和计费模式。
Provider 的合法画幅不改变模型价格。北向 `ratio` 按精确 Provider 模型支持范围校验后直接转换为飞彩比例字段，
不再推导或发送像素 `size`：SD2 只允许 `16:9`、`9:16`，其余九个已登记 Provider 模型允许
`21:9`、`16:9`、`4:3`、`1:1`、`3:4`、`9:16`。每个准备上线的 Provider 模型仍由技术人员独立验证字段、媒体、
Task、内容和账单，再给管理员独立客户模型、模型映射、价格和协议配置。系统不建立 publication、
Link SKU、implementation 或 execution binding 门禁。

## 阅读顺序

1. [URL 媒体协议与素材边界](飞彩URL媒体协议与素材边界设计.md)
2. [全模型与计费](飞彩全模型与计费设计.md)
3. [火山八月折扣前价格对比](飞彩火山八月折扣前价格对比.md)

## 上位架构

- [Seedance 专用渠道与 Link 架构](../../Seedance专用渠道与Link架构.md)
- [异步任务与计费事实架构](../../异步任务与计费事实架构.md)
