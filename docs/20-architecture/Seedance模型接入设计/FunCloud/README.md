---
status: accepted
owner: Dev Team
last-reviewed: 2026-08-14
---

# FunCloud Seedance 全模型接入设计索引

FunCloud Seedance 通过 `ChannelTypeSeedanceLink` 接入，北向固定使用统一 ModelArk V3 视频合同与
`/v1/assets`、`/v1/asset-groups` 素材合同，南向使用代码化 `funcloud_seedance` 和
`funcloud_material` adapter。平台不为 FunCloud 建立第二套 Router、Task、素材 API 或账本。

当前登记 Standard、Fast、Mini、2.5 四个独立客户模型和四个独立 Channel。Standard、Fast、Mini
可以配置 `funcloud_material`；2.5 固定使用 `asset_upstream_protocol=none`。统一北向不发布素材组
更新接口，因此 Provider 的 `/material/group/update` 不属于当前平台合同，代码也不保留不可达实现。

代码已经实现四模型精确路径、字段校验、异步 Task、虚拟素材上传，以及查询响应
`completionTokens` 作为实际 Token 用量、`pointConsume` 作为 Provider 成本证据的计费合同；真实
Mini/2.5 生成、全量素材信封、账单和生产灰度仍需按模型独立验收，代码存在不等于生产发布。

## 阅读顺序

1. [全模型与素材计费](FunCloud全模型与素材计费设计.md)
2. [素材协议与边界](FunCloud素材协议与边界设计.md)

旧的[国内 Seedance 2.0 模型接入设计](FunCloud国内Seedance-2.0模型接入设计.md)只保留历史链接，
不再承担当前事实。

## 上位架构

- [Seedance 专用渠道与 Link 架构](../../Seedance专用渠道与Link架构.md)
- [Seedance 官方素材库与素材引用设计](../../Seedance官方素材库与素材引用设计.md)
- [异步任务与计费事实架构](../../异步任务与计费事实架构.md)
