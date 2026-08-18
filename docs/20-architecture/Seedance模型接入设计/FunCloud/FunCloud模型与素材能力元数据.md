---
status: current
owner: Dev Team
last-reviewed: 2026-08-18
---

# FunCloud 模型与素材能力元数据

## 1. 公开投影

| 字段 | Standard/Fast/Mini | 2.5 |
| --- | --- | --- |
| `video.protocol` | `modelark_v3` | `modelark_v3` |
| `assets.supported` | `true`（需配置 `funcloud_material`） | `false` |
| `assets.management_mode` | `caller_managed_stateless` | `caller_managed_stateless` |
| `assets.requires_model` | `true` | `true` |
| `assets.reference_format` | `asset://{opaque_upstream_asset_id}` | 同左（仅引用，不创建） |
| `assets.reuse_scope` | 由协议/连接/项目指纹生成 | 空字符串 |
| `asset_group_requirement` | `required`（上传虚拟素材） | `unsupported` |

公开操作为创建/查询虚拟组、删除空组、上传/查询素材；列表操作永远 `supported=false`。真人素材、真人认证、单素材更新/删除不公开。

## 2. 逐模型视频元数据

| Provider 模型 | 分辨率 | 时长 | 图片/视频/音频上限 | 素材 CRUD |
| --- | --- | --- | --- | --- |
| `seedance-2` | 480/720/1080p | 4–15s | 3/1/1 | 可选 |
| `seedance-2-fast` | 480/720p | 4–15s | 3/1/1 | 可选 |
| `seedance-2-mini` | 480/720p | 4–15s | 3/1/1 | 可选 |
| `seedance-2-5` | 480/720p | 4–30s 或 -1 | 9/3/3 | 不支持 |

所有公开模型使用 `modelark_v3` 创建、查询、列表和删除任务投影；Provider 模型名、路径、账号、Project、原始 task ID 不返回。

## 3. 生成与安全规则

能力来自 `funcloud_seedance + 精确 Provider 模型` 的代码表，不从客户模型名称推断。`asset://` 只是一段 opaque 引用；平台不宣称所有权、ready 状态、兼容性或跨 Provider 复用。公开元数据由 `model/channel_seedance_public_catalog.go` 的 `seedancePublicAssetAPI` 生成。

