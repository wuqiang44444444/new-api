---
status: current
owner: Dev Team
last-reviewed: 2026-08-10
---

# 07 Seedance 渠道配置清单

## 1. 管理原则

所有 Seedance 官方和第三方模型均使用 `ChannelTypeSeedanceLink`。不同 Provider、协议、账号、地区或
价格必须使用不同客户模型名和不同 Channel。一个客户模型只启用一个 Channel；Priority、Weight、
Affinity、随机选渠、失败重选和 fallback 均不参与。

管理员只选择代码协议、填写单 Key、Base URL、Models、Model Mapping、Group 和价格，不填写请求路径
或 JSON 映射。新模型能完全兼容已有协议时可仅配置上线；不兼容时由技术人员新增代码协议后再交付。

## 2. 视频协议清单

| 协议 | 典型线路 | 代码内置创建/查询路径 |
| --- | --- | --- |
| `modelark_v3_volcengine` | 火山方舟国内官方 | 官方 `/api/v3/contents/generations/tasks` |
| `modelark_v3_byteplus` | BytePlus 官方 | 官方 `/api/v3/contents/generations/tasks` |
| `media_task_v1` | Moxing、TokenSave | `/v1/media/generations`、`/v1/media/tasks/{task_id}` |
| `ark_media_v1` | Ark 反代 | `/v1/ark/media/generations`、`/v1/ark/media/tasks/{task_id}` |
| `url_media_arrays_v1` | 飞彩；仅 URL，不支持素材库 | `/v1/videos`、`/v1/videos/{task_id}` |
| `funcloud_seedance_v2` | FunCloud | Standard/Fast 路径由 Provider 模型名在代码中选择 |

这些路径不出现在管理员 JSON 配置中。不要把第三方协议挂回 `DoubaoVideo`，也不要让客户端调用第三方
私有路径。

## 3. 素材协议清单

| 协议 | 必须配对的视频协议 | 管理字段 |
| --- | --- | --- |
| `none` | 任意 | 无素材库 |
| `volcengine_assets_action_v2024_01_01` | `modelark_v3_volcengine` | 素材 AK/SK、Project、固定 Region `cn-beijing`、TTL |
| `byteplus_assets_action_v2024_01_01` | `modelark_v3_byteplus` | 素材 AK/SK、Project、Region、TTL |
| `ark_assets_v1` | `ark_media_v1` | Base URL、同一单 Key、TTL |
| `relay_assets_v1` | `media_task_v1` | Base URL、同一单 Key、TTL |

国内火山与 BytePlus 使用不同协议标识和账号作用域，不得互换 Host、Region 或素材 ID。飞彩、
FunCloud 等没有已验证素材协议时选择 `none`。

## 4. 保存与上线检查

保存或启用前确认：客户模型名没有出现在其它已启用 Seedance Channel；只有一个 Key；协议及配对正确；
Model Mapping 指向经技术审核的 Provider 模型；价格和 Group 已审批。后台保存成功只代表配置结构合法，
不代表 Provider 能力已经生产验收。

上线必须逐客户模型完成真实创建、查询、删除、内容、素材和账单验证。失败或结果不明时不自动换渠；
管理员禁用该唯一 Channel 后，已有 Task 仍按创建时冻结的协议和连接继续处理。
