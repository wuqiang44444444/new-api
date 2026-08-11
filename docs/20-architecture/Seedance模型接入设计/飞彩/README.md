---
status: accepted
owner: Dev Team
last-reviewed: 2026-08-11
---

# 飞彩 Seedance 全模型接入设计索引

飞彩 Seedance 通过 `ChannelTypeSeedanceLink` 接入，北向固定 ModelArk V3，南向使用代码化
`url_media_arrays_v1` adapter。该协议只接受请求级 URL/Data URL，不支持 `ast_*`、`pubref_*` 或
Provider 素材库；Channel 必须配置 `asset_upstream_protocol=none`。

飞彩资料覆盖 10 个模型档位。每个准备上线的 Provider 模型都由技术人员独立验证字段、size、媒体、
Task、内容和账单，再给管理员独立客户模型、模型映射、价格和协议配置。系统不建立 publication、
Link SKU、implementation 或 execution binding 门禁。

## 阅读顺序

1. [总体架构与履约](飞彩总体架构与履约设计.md)
2. [全模型与计费](飞彩全模型与计费设计.md)

## 上位架构

- [Seedance 专用渠道与 Link 架构](../../Seedance专用渠道与Link架构.md)
- [异步任务与计费事实架构](../../异步任务与计费事实架构.md)
