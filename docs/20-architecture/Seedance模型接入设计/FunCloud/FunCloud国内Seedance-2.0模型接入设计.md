---
status: accepted
owner: Dev Team
last-reviewed: 2026-08-11
---

# FunCloud 国内 Seedance 2.0 模型接入设计

## 1. 定位与状态

FunCloud 的 Standard 与 Fast 作为两个独立 `ChannelTypeSeedanceLink` 渠道接入。两条渠道必须配置
不同客户模型名，北向均使用 ModelArk V3，南向使用代码协议 `funcloud_seedance_v2`。本文是已接受
设计，不表示生产渠道已开放。

FunCloud 不再通过 Link publication、SKU、implementation 或 execution binding 获得履约资格。技术
人员确认协议兼容后，管理员配置客户模型、Channel、模型映射、价格和协议；请求直接定位唯一渠道。

## 2. 已确认 Provider 事实

| 事实 | Standard | Fast |
| --- | --- | --- |
| Base URL | `https://mm-internal-cn.leonecloud.com` | 同左 |
| 创建路径 | `/api/v2/open/aigc/seedance2-0` | `/api/v2/open/aigc/seedance2-0-fast` |
| 查询路径 | `/api/v2/open/aigc/{taskId}` | 同左 |
| 鉴权 | `Authorization: Bearer <token>` | 同左 |
| 分辨率 | `480p`、`720p`、`1080p` | `480p`、`720p`，不支持 `1080p` |
| 时长 | 4—15 秒，默认 5 秒 | 同左 |
| 画幅 | `16:9`、`9:16`、`1:1`、`4:3`、`3:4`、`21:9`、`adaptive` | 同左 |
| 输入 | 文本、图片、视频、音频 | 同左 |
| 创建成功 | `code=0`、`data.taskId`、`data.status=processing` | 同左 |
| 查询活态 | `processing`；嵌套可为 `submitted` / `running` | `processing` |
| 查询成功 | 顶层 `success`，嵌套 `succeeded` | 顶层 `completed` |
| 查询失败 | `failed` | `failed` |

现有资料没有闭合素材库 CRUD、真人认证、Provider 价格、任务级 usage、失败扣费、回调签名、批量查询、
多结果、尾帧或结果 URL 生命周期。这些能力不能由 adapter 猜测。

## 3. 渠道设计

建议分别配置：

```text
FunCloud Standard customer model
  -> unique ChannelTypeSeedanceLink
  -> model_mapping: seedance-2.0-standard
  -> funcloud_seedance_v2 + standard path

FunCloud Fast customer model
  -> unique ChannelTypeSeedanceLink
  -> model_mapping: seedance-2.0-fast
  -> funcloud_seedance_v2 + fast path
```

Standard 与 Fast 不得共用客户模型名，也不得互相 fallback。两条渠道不使用 Priority、Weight、
Affinity 或失败重选。FunCloud 创建 body 没有模型字段；`model_mapping` 只确定固定路径和计价模型，
adapter 不把模型名发送给 FunCloud。

管理字段只有 Base URL、Bearer Key、视频协议、素材协议、模型映射和价格。当前没有完整 FunCloud
素材库合同，因此配置 `asset_upstream_protocol=none`，不要求官方 AK/SK、Region 或 Project。

## 4. 北向与字段转换

初始支持：

- 至少一个非空 text；
- 图片最多 3 个，视频最多 1 个，音频最多 1 个且必须有图片或视频；
- 图片 role：`reference_image`、`first_frame`、`last_frame`；
- 视频 role：`reference_video`；音频 role：`reference_audio`；
- `last_frame` 必须与 `first_frame` 同时出现且各最多一个；
- `duration` 4—15，缺失显式写 5；
- `resolution` 缺失显式写 `720p`，Fast 拒绝 `1080p`；
- `ratio` 缺失显式写 `16:9`；
- `generate_audio`、`watermark`、`camera_fixed` 保留显式 false，`seed` 保留显式 0。

出站字段为：

```text
content, ratio, duration, resolution,
generateAudio, watermark, seed, cameraFixed
```

`callback_url`、`return_last_frame`、`realPersonMode`、批量查询和未验证私有字段均明确拒绝。adapter 不
读取数据库、不选择渠道、不推断模型身份，也不从 metadata/extra 接受旁路字段。

## 5. 媒体与素材

当前 FunCloud 渠道只接受 Provider 已支持的请求级 HTTP/HTTPS URL 或 Data URL；是否允许 Data URL
以真实协议验证为准。由于没有完整素材库 CRUD 合同，该渠道不接受平台 `asset://ast_*`，也不把旧
AssetSource URL 当作素材 fallback。

以后取得 FunCloud 素材库合同后，应新增独立代码化 `asset_upstream_protocol`，让一个平台 Asset
一对一固定到该 FunCloud Channel/账号/Provider Asset。不能恢复通用 0..N binding 或自动迁移。

`realPersonMode` 不等于平台或 Provider 已完成真人认证；取得完整合同前不开放真人 AssetGroup。

## 6. 创建和 Task

所有 FunCloud 视频创建使用 Seedance 共享安全链：

```text
统一 ModelArk V3 校验
  -> 客户模型确定唯一 FunCloud Channel
  -> 冻结 model_mapping、adapter、连接、媒体和计费
  -> durable attempt + hold + sending
  -> 一次 Provider POST
  -> 可信 taskId 创建 Task；否则 unknown
```

可信创建成功要求 HTTP/JSON 可解析、`code=0`、`data.taskId` 非空受控且
`data.status=processing`。现有资料不足以证明任一应用 code 必然“未创建”；已经发送请求后的非零
code、非 2xx、坏 JSON、网络异常或缺少 taskId 均进入视频 `unknown`，不重试、换渠道或退款。

查询只使用 Task 冻结 Base URL、路径、Key、adapter 和 task ID。状态归一：

| FunCloud | 平台 |
| --- | --- |
| `processing` / `running` / `submitted` | running |
| `success` / `completed` / `succeeded` | succeeded |
| `failed` | failed |

未知状态、ID 冲突、成功无唯一 HTTPS URL或多结果冲突只表示观测不可采信，不直接判定业务失败。
不使用 Provider 回调或批量查询作为第二状态权威。

## 7. 计费

Standard 与 Fast 使用不同客户模型和独立价格。客户费用只来自 NEWAPI 冻结价格、分组倍率、计费表达式
和结算日志，不从 Provider 当前报价或错误文本重算。

计费 probe 只读取已校验的 duration、resolution、是否有视频输入和已经审批使用的音频开关。duration
先限制在 4—15；Fast 1080p 在计费前拒绝；quota 使用 checked 饱和函数。视频 unknown 保持 hold，
客户退款与 Provider 潜在成本分账。

## 8. 真实证据边界

2026-08-05 至 2026-08-06 的生产凭据黑盒事实：

- Standard 480p/720p/1080p 四秒文本成功；Fast 480p/720p 四秒文本成功；
- Fast 720p 参考视频、480p 中性图片加音频和 480p 平台素材转换样本成功，内容代理与 Range 可用；
- Standard 图片、图片加音频和参考视频在多次样本中可信失败，不能从 Fast 成功横向推断；
- 当时数据库 21 个 Task：13 成功、8 失败；21 个 attempt 均 complete + transferred；
- 客户账单毛消费 4,762,140 quota、退款 2,082,824 quota、净额 2,679,316 quota，与成功 Task 最终
  quota 合计一致；这只证明平台冻结价格结算，不证明 Provider 成本或生产售价已批准。

旧样本中的平台素材转换依赖待删除 source URL 路径，不代表新的一对一素材协议已经可用。生产开放
仍需闭合 Standard 媒体稳定性、Provider 账单、确定拒绝、结果 URL、故障注入和三数据库验证。

## 9. 不变量

1. Standard 与 Fast 使用不同客户模型和不同 Seedance Channel。
2. 两者不互相 fallback，不使用 Priority/Weight/Affinity。
3. FunCloud 私有路径、Key、task ID 和状态外壳不进入客户合同。
4. 创建只发送一次；结果不明保持视频 unknown。
5. 当前素材协议为 none，不通过 source URL 冒充素材库。
6. 客户费用只读取冻结 NEWAPI 计费事实。
7. 代码/文档存在不等于生产开放，未验证能力保持关闭。

## 10. 相关文档

- [Seedance 专用渠道与 Link 架构](../../Seedance专用渠道与Link架构.md)
- [异步任务与计费事实架构](../../异步任务与计费事实架构.md)
