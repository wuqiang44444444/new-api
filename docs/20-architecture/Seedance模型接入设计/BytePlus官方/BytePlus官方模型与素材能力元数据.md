---
status: current
owner: Dev Team
last-reviewed: 2026-08-18
---

# BytePlus 官方模型与素材能力元数据

## 1. 公开投影

| 字段 | 值 |
| --- | --- |
| `video.protocol` | `modelark_v3` |
| `assets.supported` | `true`（需配置 BytePlus Action） |
| `assets.management_mode` | `caller_managed_stateless` |
| `assets.requires_model` | `true` |
| `assets.reference_format` | `asset://{opaque_upstream_asset_id}` |
| `assets.media` | general image/video/audio；real_person image |
| `asset_group_requirement` | optional |
| `reuse_scope` | `asset_scope_<credential/project/region fingerprint>` |

列表操作仍为 `supported=false`；真人认证仅代理会话与结果，不返回 AK/SK、Provider、Region/Project 或原始 ID。

## 2. 逐模型能力

| 模型 | 分辨率 | 时长/输入 | 公开素材能力 |
| --- | --- | --- | --- |
| Standard | 480/720/1080p/4K | 4–15s；含视频与否影响价格 | Action CRUD + opaque 引用 |
| Fast/Mini | 480/720p | 4–15s | Action CRUD + opaque 引用 |
| 2.5 | 480/720p | 4–30s 或 -1；多模态上限以代码/官方账号为准 | Action CRUD + opaque 引用 |

## 3. 来源与不变量

元数据由 `modelark_v3_byteplus + model_mapping + byteplus_assets_action_v2024_01_01` 生成；不根据 `dreamina` 名称、价格或域名推断。代码权威：`model/channel_seedance_public_catalog.go`、`relay/channel/task/seedance/assets/official_action.go`、`model/channel_asset_credential.go`。

