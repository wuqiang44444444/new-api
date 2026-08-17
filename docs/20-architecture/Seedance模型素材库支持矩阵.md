---
status: current
owner: Dev Team
last-reviewed: 2026-08-17
---

# Seedance 模型素材库支持矩阵

本文只描述下游可见的通用合同。客户模型由部署方命名；Provider、上游原始模型和 Channel 不进入公开
模型目录。素材能力由 Channel 的已验证素材协议决定，不根据客户模型名推断。

| 通用类别 | `api.assets.supported` | 素材组 | 使用规则 |
| --- | --- | --- | --- |
| 固定分辨率系列（当前名称通常含 `720p`、`1080p`、`4k`） | `false` | 不适用 | 只使用该模型允许的请求级 URL/Data URL；素材操作返回 422 |
| 已配置无素材组协议的模型 | `true` | 不支持 | 直接创建普通图片、视频或音频素材 |
| 已配置可选素材组协议的模型 | `true` | 普通素材可选；真人素材按 `media` 要求 | 逐项读取 `operations` |
| 已配置必需素材组协议的模型 | `true` | 普通素材必填 | 先创建组，再创建素材 |
| 未配置素材协议的其它模型 | `false` | 不适用 | 不得因为名称不含固定分辨率就推断支持 |

固定分辨率系列当前不支持素材库已经由接入文档、已注册协议和实际配置共同确认：现有上游合同只有视频
创建/查询及计费，没有可验证的素材 CRUD、素材组或真人认证接口。该结论是当前接入事实；将来只有在
取得正式接口证据并实现 adapter 后才能改为支持。

外部调用方应以 `GET /v1/models/{customer_model}` 返回的以下字段为运行时权威：

- `api.assets.supported`：模型是否支持素材管理；
- `api.assets.operations`：每个素材和素材组操作是否支持；
- `api.assets.media`、`creation`：素材组、URL、MIME、大小和重定向限制；
- `api.assets.reuse_scope`：匿名复用域。两个模型只有非空 scope 完全相同时才可尝试复用，最终结果仍
  由上游裁决。

部署方可以为相同 `reuse_scope` 的客户模型使用相同业务后缀，但后缀不是运行时能力依据。Provider 失败
仍返回脱敏上游错误，不改判为 `unsupported_asset_operation`。素材列表固定不发布。
