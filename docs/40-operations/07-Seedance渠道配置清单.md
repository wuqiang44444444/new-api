---
status: current
owner: Dev Team
last-reviewed: 2026-09-14
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
| `moxing_modelark_media_v1` | 墨行 2.0、Fast、Mini、2.5 | `/v1/media/generations`、`/v1/media/tasks/{task_id}` |
| `ark_media_v1` | Ark 反代 | `/v1/ark/media/generations`、`/v1/ark/media/tasks/{task_id}` |
| `feicai_videos_v1` | 飞彩；仅 URL，不支持素材库 | `/v1/videos`、`/v1/videos/{task_id}` |
| `funcloud_modelark_v3` | FunCloud Standard/Fast/Mini/2.5 | 共用 `/api/v3/contents/generations/tasks` |
| `synlink_video_v1` | Synlink 海外火山中转 | `/v1/video/generate`、`/v1/video/tasks/{task_id}` |

这些路径不出现在管理员 JSON 配置中。不要把第三方协议挂回 `DoubaoVideo`，也不要让客户端调用第三方
私有路径。

墨行统一为 `moxing_modelark_media_v1` 单协议，四个精确 Provider 模型
（`doubao-seedance-2-0-260128-0818`、`...-fast-260128`、`...-mini-260615`、`doubao-seedance-2-5-260628`）
登记于 `relaykit/dto/moxing_video_models.go` 唯一注册表。墨行未登记的新 Provider ID 会在保存和请求
校验被拒绝；映射目标改名时由技术人员更新该登记表，通常无需改 adapter。旧 `moxing_media_task_v1` 与
`moxing_joycreator_assets_v1` 不再接受保存或创建；存量渠道直接在同一渠道原地改协议即可，
无需专门的素材租户迁移、盘点或默认组处理。缺省时长由 adapter 显式南向落实为 5 秒，显式 `-1`
原样发送并按登记预算上限估算。已创建的旧协议任务继续按创建时冻结事实查询与结算，不重发、不迁移。

FunCloud 仅允许 `funcloud_modelark_v3` 新配置和新提交，四模型共用
`/api/v3/contents/generations/tasks`，查询使用该路径加 `/{task_id}`。

| 客户模型示例 | Provider 模型 |
| --- | --- |
| `seedance-2-funcloud` | `seedance-2-0` |
| `seedance-2-fast-funcloud` | `seedance-2-0-fast` |
| `seedance-2-mini-funcloud` | `seedance-2-0-mini` |
| `seedance-2-5-funcloud` | `seedance-2-5` |

四模型可配置在同一 Channel 并配对 `funcloud_material`。旧 `funcloud_seedance` 不再接受保存或创建；
管理员必须显式核对 V3 模型映射、连接、素材租户与价格后切换，不能只替换协议字符串。
已创建 V2 任务继续按冻结 V2 事实查询和结算；unknown 创建不得重发到 V3。平台不自动修改存量渠道。

## 3. 素材协议清单

| 协议 | 必须配对的视频协议 | 管理字段 |
| --- | --- | --- |
| `none` | 任意 | 无素材库 |
| `volcengine_assets_action_v2024_01_01` | `modelark_v3_volcengine` | 素材 AK/SK、Project、固定 Region `cn-beijing`、TTL |
| `byteplus_assets_action_v2024_01_01` | `modelark_v3_byteplus` | 素材 AK/SK、Project、Region、TTL |
| `ark_assets_v1` | `ark_media_v1` | Base URL、同一单 Key、TTL |
| `tokensave_assets_v1` | `tokensave_media_task_v1` | Base URL、同一单 Key、TTL |
| `moxing_volc_assets_v1` | `moxing_modelark_media_v1` | Base URL、同一单 Key、TTL |
| `funcloud_material` | `funcloud_modelark_v3` | Base URL、同一单 Key、TTL；四模型均可用 |
| `funcloud_material_hosted` | `funcloud_modelark_v3`、`synlink_video_v1` | 本站托管图片库；配对后启用平台托管素材能力 |

国内火山与 BytePlus 使用不同协议标识和账号作用域，不得互换 Host、Region 或素材 ID。使用同一墨行
连接、`moxing_modelark_media_v1` 和 `moxing_volc_assets_v1` 的 Fast、Mini、2.5 可以配置在同一个
Channel；不同连接或协议仍须分开。飞彩选择 `none`；FunCloud V3 四模型均可配对 `funcloud_material`。

启用素材协议时，一个 Channel 就代表一个上游素材租户。需要共享素材的所有客户模型必须放在同一个
Channel；不同租户或无法确认同租户时必须分开。首次保存会生成内部随机 identity；建立后 Channel Type
不可修改。Base URL、视频/素材协议、Project 或 Region 变化时，管理员直接在当前 Channel 编辑界面填写
新值：页面列出精确差异，点击保存后必须勾选并确认“替换素材租户”。成功后 Channel ID 和客户模型不变，
identity/`reuse_scope` 更新（改为 `none` 时移除）；旧素材 ID/引用可能不可用，平台不迁移、探测或删除。
操作前应确认在途 Task 无需改线，操作后重新查询模型目录并按新 scope 建立复用关系。轮换 Key 或素材
AK/SK 且边界不变时，仍须确认“素材租户未变化”；平台记录声明但不验证 Provider 账号等价。

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

飞彩固定使用 VIP 五模型与性价比五模型两个 Channel（渠道 66、71）。飞彩协议当前只提供完成状态和
产物，没有已接通的实测 Token 计量合同；**两个飞彩渠道现已停用**，等待可信实测 Token 计费接通并
补齐真实账单证据后再逐模型灰度，不得把按秒/按次旧表达式强行切换为实测 Token 收费。除 SD2 只接受
`16:9`、`9:16` 外，其余九个模型接受代码登记的六种画幅；南向只发送 `ratio`。

## 5. 价格与预扣预算

- 当前 Seedance 客户模型统一按火山官方人民币刊例价 × 系数 `0.147 USD/CNY` 换算的美元单价配置
  表达式（见[计费与分组运维手册](01-计费与分组运维手册.md#23-seedance-当前统一标价)）；
  表达式输出美元，使用 NEWAPI 任务用量字段（`u("tokens")` 等），不再接受旧 `c`/`param("_task.*")`
  语法保存或用于新受理。
- 依赖 `u("tokens")` 的表达式**必须配置预扣 Token 预算**；FunCloud V3 / Synlink 的纯固定或冻结
  条件表达式可以不配置。预算是平台预扣估算的资金授信上界，不是官网承诺上限或最终扣费封顶；
  结算仍按可信实测用量多退少补，缺实测进入等待补查。
- 价格保存在客户模型上；启用中的同一客户模型只能归属一个渠道。管理保存会校验表达式编译、字段
  合法性、预算完整性与跨类型同名冲突，冲突时拒绝保存。
