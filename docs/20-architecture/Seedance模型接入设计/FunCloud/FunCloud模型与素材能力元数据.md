---
status: current
owner: Dev Team
last-reviewed: 2026-09-07
---

# FunCloud 模型与素材能力元数据

## 1. V2 公开投影

| 字段 | Standard/Fast/Mini | 2.5 |
| --- | --- | --- |
| `video.protocol` | `modelark_v3` | `modelark_v3` |
| `assets.supported` | `true`（需配置 `funcloud_material`） | `false` |
| `assets.management_mode` | `caller_managed_stateless` | `caller_managed_stateless` |
| `assets.requires_model` | `true` | `true` |
| `assets.reference_format` | `asset://{opaque_upstream_asset_id}` | 同左（仅引用，不创建） |
| `assets.reuse_scope` | 由 Channel 随机稳定 identity 与统一格式版本生成 | 空字符串 |
| `asset_group_requirement` | `required`（上传虚拟素材） | `unsupported` |

公开操作为创建/查询虚拟组、上传/查询素材；列表和素材组删除不进入公开操作元数据。真人素材、真人认证、单素材更新/删除不公开。

## 2. 当前 V2 实现的逐模型视频元数据

以下是现有代码限制，不是最新 V3 的能力表。V3 通用输入表为 9/3/3，且分辨率范围有变化，
详见[原始技术文档整理](FunCloud供应商原始技术文档整理.md)。V3 的新增登记见第 4 节。

| Provider 模型 | 分辨率 | 时长 | 图片/视频/音频上限 | 素材 CRUD |
| --- | --- | --- | --- | --- |
| `seedance-2` | 480/720/1080p | 4–15s | 3/1/1 | 可选 |
| `seedance-2-fast` | 480/720p | 4–15s | 3/1/1 | 可选 |
| `seedance-2-mini` | 480/720p | 4–15s | 3/1/1 | 可选 |
| `seedance-2-5` | 480/720p | 4–30s 或 -1 | 9/3/3 | 不支持 |

所有公开模型使用 `modelark_v3` 创建、查询、列表和删除任务投影；Provider 模型名、路径、账号、Project、原始 task ID 不返回。

## 3. 生成与安全规则

能力来自 `funcloud_seedance + 精确 Provider 模型` 的代码表，不从客户模型名称推断。`asset://` 只是一段 opaque 引用；平台不宣称所有权、ready 状态、兼容性或跨 Provider 复用。公开元数据由 `model/channel_seedance_public_catalog.go` 的 `seedancePublicAssetAPI` 生成。

## 4. V3 运行时与公开投影

`funcloud_modelark_v3` 的四模型共同读取 `relaykit/dto/funcloud_modelark_models.go`：

| Provider 模型 | 分辨率 | 时长 | 图/视频/音频 |
| --- | --- | --- | --- |
| `seedance-2-0` / `seedance-2-0-fast` / `seedance-2-0-mini` | 480/720p | 4–15 秒 | 9/3/3 |
| `seedance-2-5` | 480/720/1080p | 4–30 秒或 -1 | 9/3/3 |

四模型均可配对既有 `funcloud_material`，公开创建/查询组、创建/查询素材；更新/删除素材继续不支持。
同一 Channel 的四模型具有相同非空 `reuse_scope`，其含义仅为可尝试复用。视频协议替换确认后作用域会变化。
V3 公开视频创建、查询、本地列表和内容读取；删除操作标记不支持，与运行时能力一致。
代码与自动测试完成不代表真实 Provider、账单和生产发布均已验收；状态见 80-dev 实施记录。
