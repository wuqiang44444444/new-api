---
status: current
owner: Dev Team
last-reviewed: 2026-08-25
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
| `tokensave_media_task_v1` | TokenSave 2.0 | `/v1/media/generations`、`/v1/media/tasks/{task_id}` |
| `moxing_media_task_v1` | 墨行国内 2.0 | `/v1/media/generations`、`/v1/media/tasks/{task_id}` |
| `moxing_modelark_media_v1` | 墨行 Fast、Mini、2.5 | `/v1/media/generations`、`/v1/media/tasks/{task_id}` |
| `ark_media_v1` | Ark 反代 | `/v1/ark/media/generations`、`/v1/ark/media/tasks/{task_id}` |
| `feicai_videos_v1` | 飞彩；仅 URL，不支持素材库 | `/v1/videos`、`/v1/videos/{task_id}` |
| `funcloud_seedance` | FunCloud Standard/Fast/Mini/2.5 | 四个 Provider 模型均在代码中精确注册创建路径 |

这些路径不出现在管理员 JSON 配置中。不要把第三方协议挂回 `DoubaoVideo`，也不要让客户端调用第三方
私有路径。

FunCloud 四个客户模型和路径必须精确对应：

| 客户模型 | Provider 模型 | 创建路径 |
| --- | --- | --- |
| `seedance-2-funcloud` | `seedance-2` | `/api/v2/open/aigc/seedance2-0` |
| `seedance-2-fast-funcloud` | `seedance-2-fast` | `/api/v2/open/aigc/seedance2-0-fast` |
| `seedance-2-mini-funcloud` | `seedance-2-mini` | `/api/v2/open/aigc/seedance2-0-mini` |
| `seedance-2-5-funcloud` | `seedance-2-5` | `/api/v2/open/aigc/seedance2-5` |

一个 Channel 可以包含表中多个客户模型。Models、客户名称和 `model_mapping` 由管理员维护；保存时逐项
解析 Channel Models，并确认最终 Provider 模型属于表中登记范围。Standard/Fast/Mini 可以在启用
`funcloud_material` 的同一 Channel，2.5 只能位于素材协议为 `none` 的 Channel。查询路径统一为
`/api/v2/open/aigc/{task_id}`，创建路径按最终 Provider 模型由代码注册，管理员不填写。

## 3. 素材协议清单

| 协议 | 必须配对的视频协议 | 管理字段 |
| --- | --- | --- |
| `none` | 任意 | 无素材库 |
| `volcengine_assets_action_v2024_01_01` | `modelark_v3_volcengine` | 素材 AK/SK、Project、固定 Region `cn-beijing`、TTL |
| `byteplus_assets_action_v2024_01_01` | `modelark_v3_byteplus` | 素材 AK/SK、Project、Region、TTL |
| `ark_assets_v1` | `ark_media_v1` | Base URL、同一单 Key、TTL |
| `tokensave_assets_v1` | `tokensave_media_task_v1` | Base URL、同一单 Key、TTL |
| `moxing_joycreator_assets_v1` | `moxing_media_task_v1` | Base URL、同一单 Key、TTL |
| `moxing_volc_assets_v1` | `moxing_modelark_media_v1` | Base URL、同一单 Key、TTL |
| `funcloud_material` | `funcloud_seedance` | Base URL、同一单 Key、TTL；仅 Standard/Fast/Mini |

国内火山与 BytePlus 使用不同协议标识和账号作用域，不得互换 Host、Region 或素材 ID。使用同一墨行
连接、`moxing_modelark_media_v1` 和 `moxing_volc_assets_v1` 的 Fast、Mini、2.5 可以配置在同一个
Channel；不同连接或协议仍须分开。飞彩、FunCloud 2.5 选择 `none`，不得配置 `funcloud_material`。

启用素材协议时，一个 Channel 就代表一个上游素材租户。需要共享素材的所有客户模型必须放在同一个
Channel；不同租户或无法确认同租户时必须分开。首次保存会生成内部随机 identity；建立后不得原地修改
Channel Type、Base URL、视频/素材协议、Project 或 Region，也不得改回 `none`。此类变化应新建 Channel。
轮换 Key 或素材 AK/SK 时，管理端必须确认“素材租户未变化”；平台记录该声明但不验证 Provider 账号等价。

升级到统一 scope 公式后，全部旧 `reuse_scope`（包括移动云 CMCC）都会失效。发布前应通知 API 调用方
清理按旧 scope 缓存的跨模型复用关系，并在升级完成后重新查询模型目录；不得把旧 scope 与新 scope
判为同组。迁移合并 Channel 时先停用旧 Channel，再启用新 Channel，接受这两个管理动作之间的短暂不可用。

## 4. 保存与上线检查

保存或启用前确认：每个客户模型名没有出现在其它已启用 Seedance Channel；只有一个 Key；需要共享素材的
模型处于同一 Channel；协议及配对正确；Channel Models 逐项经管理员 mapping 解析到经技术审核的
Provider 模型；价格和 Group 已审批。
后台保存成功只代表配置结构合法，不代表 Provider 能力已经生产验收。

上线必须逐客户模型完成真实创建、查询、删除、内容、素材和账单验证。失败或结果不明时不自动换渠；
管理员禁用该唯一 Channel 后，已有 Task 仍按创建时冻结的协议和连接继续处理。

FunCloud 还需按[FunCloud 四模型 Token 价格](01-计费与分组运维手册.md#25-funcloud-seedance-四模型-token-价格)
配置美元表达式和预扣上界。任何旧按秒 Standard/Fast 配置都必须在开放前替换，不得与新客户模型混用。

飞彩固定使用 VIP 五模型与性价比五模型两个 Channel。除 SD2 只接受 `16:9`、`9:16` 外，其余九个
模型接受代码登记的六种画幅；南向只发送 `ratio`，模型的 720p/1080p/4K 档位来自精确 Provider 模型。
两个 Channel 的启停分别影响各自五个模型，不存在逐模型 Ability；一次性开放十模型前必须完成全部
十行验收，再同时启用两个 Channel。
