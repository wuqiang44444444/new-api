---
status: current
owner: Dev Team
last-reviewed: 2026-08-03
---

# FunCloud Seedance 2.0 中转接入分析方案

## 1. 文档目的与最终方向

本文以 2026-08-02 收到的 FunCloud 最新文档为唯一 Provider 事实源，对照当前代码，给出**移除旧协议、直接应用新协议**的具体实施方案。整体产品和架构设计以 [Link 资源虚拟素材库架构](../20-architecture/Link资源虚拟素材库架构.md) 为准，代码侧共同改造以 [Link 资源虚拟素材库实施方案](./2026-08-02-Link资源虚拟素材库实施方案.md) 为准；本文不另行定义一套 FunCloud 客户素材产品。

前置架构语义是：

- Link 是平台面向客户的统一虚拟素材库；
- FunCloud、Moxing、官方等都是可替换的 Provider；
- FunCloud 没有已验证的上游托管素材生命周期，因此 `SupportsManagedAssets=false`；
- 但 FunCloud 视频协议能消费 HTTPS URL，所以可通过 Link Resolver 的 `source_url` 模式支持客户 `asset://ast_*`，目标为 `SupportsLinkAssets=true`；
- FunCloud 不创建 `AssetBinding`，不再需要 H5、`bytedToken`、material list、multipart 或本地暂存。

本次不再保留以下双轨或兼容路径：

- 不保留 FunCloud video adapter v1 与 v2 并存；
- 不保留旧查询信封的 fallback parser；
- 不保留旧 FunCloud H5 + `bytedToken` + multipart 真人素材实现；
- 不把旧素材数据迁移为新 `realPersonMode` 数据；
- 不继续接受旧 `mm-accelerate` 渠道配置；
- 不通过 `metadata`、`extra` 或供应商私有字段让客户端选择新模式。

最终只保留一套当前实现：FunCloud 最新 JSON `content` 视频协议和最新查询信封。`realPersonMode` 只记录为 Provider 文档中语义冲突、待书面澄清的字段；本阶段不注册对应公开 SKU，不向上游发送 `realPersonMode=true`，也不对客户暴露该私有字段。

## 2. 当前权威资料

本轮 FunCloud 渠道适配协议只采用：

1. `docs/70-research/funcloud/fun cloud 对接文档.md`
2. `docs/70-research/funcloud/open-api-seedance 2.md`
3. `docs/70-research/funcloud/open-api-seedance2-0-fast.md`

权威顺序：

```text
平台已发布 Link 合同与硬约束
  > FunCloud 最新 Standard/Fast 详细 API 文档
  > FunCloud 模型可用性目录
  > 旧 FunCloud 研究资料和旧实现
```

第一份文档只说明 `seedance2.0` 和 `seedance2.0-fast` 当前被 FunCloud 标记为可用；后两份才定义创建、查询和 `realPersonMode`。旧版素材库资料不再用于新实现。

## 3. 旧实现的代码归属与替换边界

### 3.1 代码归属决定处理方式

旧 FunCloud 素材链和旧 FunCloud 视频协议属于本地新增、尚未发布的预接入实现，不是 NEWAPI 上游原有合同。`funcloud_real_person_stream` profile、原始图片上传路由、multipart adapter、上传 attempt model、清理 service 以及相关前端字段均可直接删除；不为这些本地实现设计 API 退役期、数据迁移、零存量门禁、兼容 parser、feature flag 或 fallback。

删除时必须同时保护未来上游同步边界：

- 本地新增的独立文件和独立注册直接完整删除；
- `model/main.go`、`relaykit/dto/channel_settings.go`、`web/src/features/channels/lib/channel-form.ts`、`web/src/features/channels/types.ts` 等 NEWAPI 上游文件，只移除本地新增的窄接线，不移动、不重排、不重构其余代码；
- 通用素材、真人授权、multipart 和其它 Provider 实现均不因 FunCloud 退役而改变；
- 若工作区或开发数据库残留旧表或旧配置，由开发者按本地环境自行清理，不把它们升级成产品迁移合同。

这一边界的依据是代码来源，而不是某个环境当前是否存在记录。对于从未发布的本地实现，保留防御性兼容只会增加上游合并冲突面和第二套权威。

### 3.2 当前代码仍是旧实现

“允许直接删除”不等于“代码已经切换”。截至本文核对时，仓库实际状态仍是：

| 当前代码/配置 | 实际状态 |
| --- | --- |
| FunCloud adapter revision | 仍为 `54:third_party_funcloud_seedance_v2:v1` |
| Standard query | 仍错误要求 `output.id == data.taskId` |
| Fast failure | 仍未读取顶层 `data.errorCode` |
| `realPersonMode` | DTO、capability、converter 均未实现 |
| 普通 SKU 媒体上限 | 仍为 9 图/3 视频/3 音频 |
| Standard managed assets | 仍为 `SupportsManagedAssets=true` |
| 旧素材实现 | H5、`bytedToken`、multipart、material list、raw upload route 仍在代码中 |
| 渠道 Base URL | 仍为 `mm-accelerate.leonecloud.com` |
| `realPersonMode` 公开能力 | 未注册对应 SKU，Provider 语义澄清前不得预留或发布 |

因此本文后续内容是已经确认的替换目标和实施门禁，不得在代码完成前把它写成运行事实。当前渠道与 Ability 必须继续禁用。

## 4. 最新 FunCloud 渠道适配协议

### 4.1 Base URL 与认证

两份详细文档统一声明：

```text
Base URL: https://mm-internal-cn.leonecloud.com
Authorization: Bearer <token>
Content-Type: application/json
```

本地渠道 38/39 当前仍配置 `https://mm-accelerate.leonecloud.com`。直接应用新协议时应把两个禁用渠道都改为 `https://mm-internal-cn.leonecloud.com`，不保留旧域名 fallback。修改后仍保持渠道和 Ability 禁用，先做真实凭据 smoke test。

如果 Provider 说明两个域名属于不同正式环境，应分别建立明确的环境配置，而不是让 adapter 猜测或失败后跨域重试。

### 4.2 创建路径

| 产品 | 创建路径 | 查询路径 | 分辨率 |
| --- | --- | --- | --- |
| Standard | `/api/v2/open/aigc/seedance2-0` | `/api/v2/open/aigc/{taskId}` | 480p/720p/1080p |
| Fast | `/api/v2/open/aigc/seedance2-0-fast` | `/api/v2/open/aigc/{taskId}` | 480p/720p |

代码内部继续使用 `{task_id}` 作为渠道模板占位符，发送前 path escape 后替换；这只是内部配置语法，不改变上游实际 URL。

### 4.3 创建字段

| FunCloud 字段 | 类型 | 最新文档 | 新实现 |
| --- | --- | --- | --- |
| `content` | array | 必填，至少一个 text | typed 转换，拒绝任意 map 透传 |
| `ratio` | string | 默认 `16:9` | 省略时确定性注入 `16:9` |
| `duration` | int | 4～15，默认 5 | capability 校验后注入 |
| `resolution` | string | Standard 480p/720p/1080p；Fast 480p/720p | 省略时注入 720p |
| `generateAudio` | bool | 默认 false | `*bool`，保留显式 false |
| `watermark` | bool | 默认 false | `*bool`，保留显式 false |
| `seed` | int | 可选，未给范围 | `*int`，继续使用平台共享范围校验 |
| `cameraFixed` | bool | 可选 | `*bool`，保留显式 false |
| `returnLastFrame` | bool | 可选 | Standard 待补安全结果字段；未完成前 capability 明确拒绝 |
| `realPersonMode` | bool | 默认 false | 语义未决；本阶段不发送，客户也不能直接传供应商字段 |
| `callbackUrl` | string | Standard 可选 | 不接入；平台继续使用持久化轮询 |

创建请求不发送 `model`。Standard/Fast 由不同 endpoint 和已冻结的公开 SKU 决定，不再使用旧资料中的 `model + prompt + mode`。

### 4.4 content 项

最新文档定义：

| type | 载荷 | role |
| --- | --- | --- |
| `text` | `text` | 无 |
| `image_url` | `image_url.url` | `reference_image` / `first_frame` / `last_frame` |
| `video_url` | `video_url.url` | `reference_video` |
| `audio_url` | `audio_url.url` | `reference_audio` |

FunCloud 私有 converter 只接受文档明确展示的绝对 HTTPS URL。HTTP、相对 URL、userinfo、Data URL、裸 FunCloud `asset://asset-*` 和未经解析的平台 `asset://ast_*` 均拒绝。客户可以提交平台 `asset://ast_*`，但必须在进入 FunCloud converter 之前由权威 Link Resolver 完成所有权、授权、过期和能力校验，并替换为 HTTPS 源 URL。

最新文档明确展示的最低可发布能力为：

- 纯文本；
- 单张参考图；
- 3 张参考图；
- 1 图 + 1 视频 + 1 音频；
- 1 视频 + 1 图的视频编辑；
- 1 视频 + 1 音频的视频延长；
- first/last/reference image 三种图片 role。

文档没有给出正式最大数量和所有组合矩阵。因此旧 capability 的 9 图/3 视频/3 音频不能继续作为已证明事实。首发 capability 收窄为：

```text
MaxImages = 3
MaxVideos = 1
MaxAudio  = 1
RequiresText = true
SupportsMixedMediaPath = false
```

现有 `VideoSKUCapability` 如无法精确表达“已验证组合”，应增加稳定的组合规则，而不是在 converter 中隐藏补丁。真实验收通过更多组合后再提升 capability 和 implementation hash。

## 5. 公开 SKU 与未决 realPersonMode

### 5.1 普通视频 SKU

继续保留：

| 公开 SKU | endpoint | 首发能力 |
| --- | --- | --- |
| `seedance-2.0-standard` | Standard | 480p/720p/1080p，普通文本/多模态，不注入 `realPersonMode` |
| `seedance-2.0-fast` | Fast | 480p/720p，普通文本/多模态，不注入 `realPersonMode` |

两个普通 SKU 的公开客户能力是：

```text
SupportsLinkAssets = true
```

FunCloud channel/profile 的内部解析声明是：

```text
SupportsManagedAssets = false
AssetResolutionModes = [source_url]
```

公开 `SupportsLinkAssets` 参与 SKU capability hash 和候选渠道对 SKU 能力合同的等价实现判定；内部 managed-assets/解析模式只进入独立的 channel/profile 实现版本或快照，不参与公开等价 hash。原因不是 FunCloud 永久不支持平台素材库，而是本轮最新资料没有给出创建、认证、查询、删除和撤回的上游托管合同。旧素材 adapter 已决定删除，不能继续声明已不存在的 managed-assets implementation；但 FunCloud 能在视频创建请求中抓取 HTTPS URL，所以平台 Link 资源可以通过 `source_url` 解析模式统一接入。

### 5.2 realPersonMode 暂不形成平台产品能力

`realPersonMode` 是 FunCloud 私有 Provider 字段，不属于平台 ModelArk v3 客户端。FunCloud 文档同时使用“让图中真人开口/动起来”和“素材不得与任何自然人肖像或形象雷同”两种不一致表述。在 Provider 给出书面澄清前，平台既不能断言它是“AI 写实/AI 仿真人”的虚构人物生成，也不能将它映射为已完成授权的 `real_person` 能力。

本阶段因此采用 fail closed：

- 不注册、不预留 `seedance-2.0-*-ai-realistic` 公开 SKU；
- 不在 FunCloud 私有 DTO 或 converter 中实现 `realPersonMode=true` 注入；
- 不对外发布“AI 写实”、“虚构人物”或“授权真人驱动”的正向产品断言；
- 客户不能通过公共 DTO、`metadata`、`extra` 或未知字段注入该供应商参数。

书面澄清后只能进入两条明确分支：Provider 确认只允许不对应现实自然人的虚构人物时，可另行评审 AI 写实/AI 仿真人 SKU 及 `general` Link；Provider 确认会驱动可识别自然人肖像时，必须纳入平台 `real_person` 授权、subject、撤回和审计合同，FunCloud 在提供足够合同前不支持。

### 5.3 合规边界

FunCloud 文档同时称该能力为“真人模式”，又要求输入素材不得与任何自然人肖像或形象雷同。平台不能把它描述为已完成自然人本人授权的托管真人素材能力。

因此，当前合规边界是“功能整体不发布”，而不是先选一个有利的产品解释。平台不声称能自动识别人脸、验证肖像权或判定输入是否对应自然人。如果未来证实该模式会驱动真人形象，必须先取得真人授权合同并完成真实验收，不能恢复本次删除的旧实现。

### 5.4 与 Link 虚拟素材库的边界

Link 是平台供应商中立的客户产品，不等于 `AssetBinding` 集合，也不要求每个视频 Provider 都实现托管素材。FunCloud 可以作为 Link 的视频执行上游，具体方式是：

- 客户继续提交平台 `asset://ast_*`；
- 选渠层先选择 active binding；没有适用 binding 时，才判断该 Asset 是否存在可用的受保护源 URL；
- 客户声明 `expires_at > 0` 时校验剩余 TTL；`expires_at = 0` 只表示有效期未知，按 best-effort 使用，不解释为永久 URL，也不猜测 OSS 签名参数；
- 权威 Link Resolver 完成所有权、授权和状态校验后，将 Link 替换为 HTTPS URL；
- FunCloud converter 只接收替换后的 URL，不查库、不解密、不创建 `AssetBinding`；
- 直接请求级 URL 仍可按 SKU 能力使用，但它不自动进入 Link 生命周期；
- 不下载客户 URL，不调用旧 multipart 接口。

源 URL 只是 `Asset` 的短期执行引用，不是资源身份，也不能据此判断两个 URL 是否指向相同媒体。平台身份只由 `ast_*` 表达；创建幂等继续使用现有完整请求作用域 HMAC。为支撑 FunCloud 的 URL-only 执行路径，源记录只需要 `asset_id`、按 Asset scope 认证加密的 URL 和客户声明的 `expires_at`，不建立 URL HMAC、公开 source ID、独立状态机、密钥版本、后台清理或保留系统。

因此 FunCloud 的准确能力语义是：

```text
SupportsManagedAssets = false
SupportsLinkAssets = true
AssetResolutionModes = [source_url]
```

`realPersonMode` 本阶段不映射到 `general` 或 `real_person` Link，不创建对应 SKU，也不发送字段。平台 `real_person` Link 在 Provider 书面语义澄清、授权合同和真实验收完成前必须排除 FunCloud。

未来即使 FunCloud 提供新的托管素材 API，也必须通过独立渠道 adapter、真实生命周期验收和 ADR 后才能开启 `managed-assets`；不得恢复本次删除的 H5、`bytedToken` 或 multipart 实现作为兼容路径。

## 6. 创建响应

最新创建成功信封：

```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "taskId": "task_xxx",
    "status": "processing",
    "createdAt": "2026-04-29 15:00:00"
  }
}
```

唯一新 normalizer 必须要求：

- JSON 可解析；
- `code == 0`；
- `data.taskId` 非空、长度受控且无空白/控制字符；
- `data.status == processing`。

数字 `10002/10005/10006` 可继续作为候选“明确未创建”错误，但只有真实 API 证明相应响应绝不创建 task 后，才能保留 terminal rejection 分类。Fast 文档列出的字符串错误码没有完整创建失败信封，不能推断它们可安全重试。

## 7. 查询响应：直接替换旧解析

### 7.1 Standard 双任务 ID

Standard 文档明确区分：

```text
data.taskId   = FunCloud wrapper task ID，例如 task_*
data.output.id = 内部上游任务 ID，例如 cgt-*
```

当前旧代码要求两者相等，会把正常 Standard 响应判为合同违例。新解析必须：

- 只要求 `data.taskId == expectedTaskID`；
- `output.id` 独立做长度和控制字符校验；
- 不用 `output.id` 替换平台 task ID；
- 如需成本对账，把 `output.id` 写入私密执行/管理员审计字段；
- wrapper ID 与 output ID 冲突不再构成错误，因为它们本来就是不同命名空间。

### 7.2 Fast errorCode

Fast 失败信封使用顶层：

```json
{
  "data": {
    "status": "failed",
    "errorCode": "INVALID_RESOLUTION",
    "errorMsg": "..."
  }
}
```

新解析同时支持：

- Standard：`output.error.code/message`，回退 `data.errorMsg`；
- Fast：`data.errorCode/errorMsg`；
- 两套错误同时存在且冲突时 fail closed；
- Provider 错误文本继续做 URL、Token、Authorization、Cookie 等敏感信息清洗。

### 7.3 状态和结果

唯一状态映射：

| FunCloud | 内部状态 |
| --- | --- |
| `processing` / `running` / `submitted` | `running` |
| `success` / `completed` / `succeeded` | `succeeded` |
| `failed` | `failed` |

成功结果从 `data.result` 和 Standard `data.output.content.video_url` 读取：

- 两处同时存在必须一致；
- 只允许绝对 HTTPS；
- 当前客户端一个任务只交付一个视频，因此 `result` 必须恰好一个元素，不能静默取第一个；
- 成功但无 URL、多个 URL、URL 冲突或不安全 URL 均为上游合同违例；
- 合同违例进入有界对账，不能直接判业务失败或自动退款重发。

### 7.4 adapter revision 只保留新版

当前代码把 FunCloud 新任务冻结为：

```text
54:third_party_funcloud_seedance_v2:v1
```

直接替换后唯一允许的 revision 为：

```text
54:third_party_funcloud_seedance_v2:v2
```

实现要求：

- `CurrentVideoSouthboundAdapterVersion` 与 `ResolveVideoSouthboundAdapterVersion` 必须共用同一个以 `(channel_type, video_upstream_profile)` 为 key 的 revision 权威表，不再分别维护 if/switch 特例；
- 该权威表对 FunCloud 当前版本返回 v2，且解析时只接受 v2；
- 删除 FunCloud v1 normalizer 和 dispatch；
- FunCloud 空 snapshot 和显式 v1 都 fail closed，不自动解释为 v2 或提供 v1 fallback；
- 不为未发布的本地 v1 实现保留任务解释、数据扫描或在线兼容分支。

使用 v2 是为了让快照名称与新合同一致，不代表运行时保留两个版本。

## 8. 删除旧真人素材实现

本节的“删除”是完整移除本地新增、尚未发布的 FunCloud 旧上传垂直链路，不是只删除 `multipart.Writer`。公开 API、controller、service、adapter、数据模型、迁移注册、渠道配置、管理端和 OpenAPI 必须在同一实施批次中一起收敛，不得留下可达路由、可选 profile 或会被 AutoMigrate 重建的空壳；也不为这些空壳编写退役或迁移代码。

### 8.1 必须移除的 API 与上游协议

| 层级 | 必须移除 | 原因 |
| --- | --- | --- |
| 客户 API 合同 | `POST /v1/real-person-authorizations/{authorization_id}/asset` | 该原始图片请求体只服务 FunCloud 旧流式上传，新协议无调用方 |
| OpenAPI | `uploadRealPersonAuthorizationAsset` operation 及原始 JPEG/PNG request body | 避免文档继续宣称存在二进制入口 |
| Provider API | `/api/v2/open/material/person/validate/session` | 不属于本轮最新视频合同 |
| Provider API | `/api/v2/open/material/upload` | 旧 multipart 素材上传协议已失去合同依据 |
| Provider API | `/api/v2/open/material/list` | 不再用于确认视频请求级图片 |
| 私有字段/状态 | `bytedToken`、`upload_confirmed`、`funcloud_real_person_stream` | 新视频协议不经过素材授权和 Link binding 生命周期 |

旧链路删除与 `realPersonMode` 是否在未来发布无关：当前既不经过 H5、multipart、素材列表或平台 `AssetBinding`，也不向视频创建 JSON 发送 `realPersonMode=true`。

### 8.2 删除的独立文件

实施时删除：

```text
relay/channel/task/doubao/assets/funcloud_real_person_stream.go
relay/channel/task/doubao/assets/funcloud_real_person_stream_test.go
service/real_person_stream_upload.go
service/real_person_stream_upload_test.go
service/funcloud_real_person_cleanup.go
model/real_person_asset_upload_attempt.go
model/real_person_asset_upload_attempt_test.go
```

### 8.3 移除的接线

从现有文件删除 FunCloud 专属窄分支：

```text
router/asset-router.go
  - POST /v1/real-person-authorizations/:authorization_id/asset

controller/asset_consent.go
  - UploadRealPersonAuthorizationAsset

relaykit/dto/upstream_profile.go
dto/asset_upstream_profile.go
  - AssetUpstreamProfileFunCloud / funcloud_real_person_stream

relaykit/dto/channel_settings.go
  - AssetVerificationCompletion 本地新增字段

service/asset_binding_service.go
service/asset_consent_service.go
model/channel_asset_validation.go
middleware/asset_route_constraint.go
model/main.go
  - FunCloud asset adapter、上传完成、清理、迁移和路由分支

web/src/features/channels/**
  - FunCloud 素材 profile 与 upload_confirmed 管理项

docs/openapi/relay.json
  - 授权专用原始图片上传 endpoint
```

此外，必须同步删除或改写以下会继续保留旧能力的注册点：

- `model/main.go` 中 `RealPersonAssetUploadAttempt` 的 AutoMigrate 和 migration list 注册；
- `model/task_cas_test.go`、`model/video_asset_dialect_contract_test.go` 等测试夹具中的上传 attempt 表依赖；
- `middleware/asset_route_constraint.go`、`model/channel_asset_validation.go`、`service/asset_binding_service.go`、`service/asset_consent_service.go` 中的 FunCloud 特殊分支；
- `web/src/features/channels/types.ts`、`channel-form.ts`、`asset-upstream-validation.ts` 及相关表单/测试中的旧 profile 和 `upload_confirmed`。

### 8.4 明确保留的共享能力

只删除 FunCloud 旧素材扩展，不得全局删除 multipart、H5 或真人授权能力。以下实现继续保留：

- `POST /v1/assets` 的 JSON + HTTPS URL 创建合同；
- `/v1/real-person-authorizations` 创建、查询、重试和撤回接口；
- `RealPersonAuthorization`、`RealPersonVerificationSession`、授权 reservation 与撤回线性化；
- 官方、Moxing 等已验证上游 profile 的 H5、URL CreateAsset、查询、引用和删除流程；
- OpenAI 图片编辑、Dify 文件上传等与 Link/FunCloud 无关的 multipart 协议实现。

### 8.5 数据与上游结构边界

旧 FunCloud 素材链未形成已发布的数据合同，因此实施不新增跨数据库 drop migration、零存量扫描、部署阻断、历史数据转换或旧 profile 配置迁移：

- 从 `model/main.go` 精确移除本地 `RealPersonAssetUploadAttempt` AutoMigrate 接线，不改写 NEWAPI 原有迁移结构；
- 不把旧 authorization、asset 或 binding 转成 `realPersonMode`，因为两者语义和生命周期不同；
- 不在运行时代码中识别、保留或解释 `funcloud_real_person_stream`；
- 开发环境若残留旧表或旧配置，由开发者按环境直接清理，不将其纳入产品发布路径；
- 已有通用 `Asset`、`AssetBinding`、真人授权和其它 Provider 数据保持原样。

目标能力直接落为：普通 Standard/Fast 公开 `SupportsLinkAssets=true`；FunCloud channel/profile 内部 `SupportsManagedAssets=false`、`AssetResolutionModes=[source_url]`。不保留新旧字段并行期。

### 8.6 文档和 ADR

[ADR-0012](../20-architecture/decisions/0012-真人素材单图同步流式边界.md) 只记录即将删除的 FunCloud 流式例外；新 Link 设计还改变了 [ADR-0004](../20-architecture/decisions/0004-中转型素材代理与上游绑定.md) 的“不保存完整源 URL”、“创建必须绑定 model/target”和“素材只能在原 binding/模型中使用”三项核心决策。

已接受的 [ADR-0015：Link 公开 SKU 与实现身份版本绑定](../20-architecture/decisions/0015-Link公开SKU与实现身份版本绑定.md) 已取代 ADR-0014，并继续承接 ADR-0014 对 ADR-0004 和 ADR-0012 的替代关系；当前只保留必要决策：

- Link 资源可保存最小化的短期加密源引用，并按渠道解析为 binding 或 `source_url`；源 URL 不是资源身份，也不提供相同媒体判定；
- 源记录只有 `asset_id`、按 Asset scope 认证加密的 URL 和 `expires_at`；不新增 URL HMAC、公开 source ID、独立状态、密钥版本、迁移注册表或后台保留系统；
- `expires_at > 0` 用于剩余 TTL 校验，`expires_at = 0` 表示未知并按 best-effort 执行，不表示永久有效；
- FunCloud 不再接收平台二进制上传，不创建 `AssetBinding`；
- 平台仍不下载、不保存二进制、不引入 multipart/本地暂存/对象存储；
- `realPersonMode` 语义未决，书面澄清前不形成公开 SKU、不发送字段；
- 平台 `real_person` Link 在 FunCloud 具备明确授权合同前不路由到 FunCloud。

同时更新素材架构、视频架构、运维手册、用户文档、路线图、[Link 资源虚拟素材库架构](../20-architecture/Link资源虚拟素材库架构.md)、[Link 资源虚拟素材库实施方案](./2026-08-02-Link资源虚拟素材库实施方案.md) 和 `docs/80-dev/README.md`，删除“FunCloud 单图同步流式直通”和“FunCloud 通过本地暂存 multipart 接入 Link”的旧事实。

## 9. callback、batch 与 last frame

### 9.1 callback

Standard 文档提供 callback 字段和请求头，但缺少签名算法、密钥派生、重放窗口和重试合同。平台已有持久化轮询，因此新版仍不发送 `callbackUrl`，客户端 `callback_url` 对 FunCloud SKU 返回明确 4xx。

### 9.2 batch query

Standard 的 `/api/v2/open/aigc/batch` 不进入本次实现。Task worker 必须按冻结的 channel/key/profile/adapter snapshot 轮询，批量接口会引入跨快照合并，且 Fast 文档没有相同合同。

### 9.3 returnLastFrame

Standard 文档明确查询会返回 `output.content.last_frame_url`；Fast 文档只声明请求字段，没有定义查询返回位置。

本次核心替换先保持 `return_last_frame` fail closed。后续支持时：

- Standard 增加独立私密 last-frame URL；
- 增加平台内容代理 `part=last_frame` 或等价稳定合同；
- 补 URL 脱敏、Range、过期和鉴权测试；
- Fast 取得真实返回信封后再开放；
- 不把 last frame URL 写入普通 task data 或日志。

这属于能力收窄，不是旧协议兼容。

## 10. 计费

### 10.1 普通 SKU

本地数据库和测试已经为两个普通 SKU 配置 `tiered_expr` 和预扣 Token `1`，当前价格为：

| 输入类型 | 480p | 720p | 1080p |
| --- | ---: | ---: | ---: |
| 文本/图片/音频参考 | $0.0679/秒 | $0.1465/秒 | $0.3651/秒 |
| 含视频输入 | $0.1465/秒 | $0.1600/秒 | $0.3989/秒 |

Fast 只保留 480p/720p 分支。表达式输入继续来自受信 `_task.duration_seconds/_task.resolution/_task.has_video_input`，不从 metadata 任意取值。

三份最新资料没有价格或最终 usage，因此上表只是当前平台配置。启用前必须重新取得价格、失败任务是否收费、按请求还是实际输出时长结算，以及按 task ID 对账方式。

### 10.2 未决 realPersonMode

Provider 文档没有给出该模式的独立价格、排队/SLA 和失败结算语义，产品分类本身也未澄清。因此当前不注册 SKU、capability、价格或 Ability，也不使用普通 SKU 的默认价。书面澄清后如决定接入，必须另行评审产品合同和计费边界。

## 11. 代码改动范围

### 11.1 保留并改写的视频文件

```text
relay/channel/task/doubao/thirdparty/funcloud/create_request.go
relay/channel/task/doubao/thirdparty/funcloud/create_response.go
relay/channel/task/doubao/thirdparty/funcloud/task_response.go
relay/channel/task/doubao/thirdparty/funcloud/funcloud_test.go
relay/channel/task/doubao/funcloud_video_capability.go
model/video_funcloud_sku_capability.go
model/video_funcloud_profile_validation.go
model/video_sku_implementation.go
relay/common/video_adapter_version.go
```

不得修改公共 `dto.ModelArkVideoCreateRequest`，也不得增加任何客户可提交的 `real_person_mode`、metadata 或 extra 字段。本阶段同样不在 FunCloud 私有 `createRequest` 中新增或发送 `RealPersonMode`，`buildFunCloudVideoCreateRequest` 只构建已确认的普通 Standard/Fast 合同。如果后续经独立评审启用，该字段也只能存在于 FunCloud 私有 Provider DTO，不能由客户未知字段、`metadata` 或 `extra` 注入。

### 11.2 必要共享接线

共享文件只保留极窄修改：

- adapter version 当前值从 FunCloud v1 改为 v2；
- 删除旧 asset profile 枚举和分派 case；
- 删除旧 raw upload route/controller/service/model 注册；
- 复用 Link 统一 Resolver 在 FunCloud converter 前将 `general` Link 替换为受保护 HTTPS 源 URL；
- 将客户 SKU 能力合同与上游 managed-assets 能力拆分：公开 `SupportsLinkAssets` 参与 `VideoSKUCapabilitiesEquivalent` hash；内部 `SupportsManagedAssets` / `AssetResolutionModes` / TTL 只进入独立的 channel/profile 实现版本或快照，不参与公开等价 hash；
- 更新前端渠道表单、OpenAPI、用户和运维文档；
- 更新两个禁用渠道的 Base URL 和 settings；

不得借本次删除重构 official、第三方 relay、飞彩 JSON adapter、共享 Task 或其它真人素材 Provider。

## 12. 实施顺序

1. [x] [ADR-0015](../20-architecture/decisions/0015-Link公开SKU与实现身份版本绑定.md) 已接受并取代 ADR-0014，继续保留最小 AssetSource 与双模式 Resolver。
2. [ ] 直接删除本地旧 H5/multipart 素材文件、路由、profile、配置、前端字段和迁移注册；共享 NEWAPI 文件只撤销本地窄接线。
3. [ ] 实现最小 `AssetSource`、公开 `SupportsLinkAssets` 与内部解析能力拆分，完成双模式 Resolver。
4. [ ] 将普通 Standard/Fast 的公开能力直接设为 `SupportsLinkAssets=true`，内部渠道解析能力设为 `SupportsManagedAssets=false`、`AssetResolutionModes=[source_url]`，不保留字段并行期。
5. [ ] 将 FunCloud Base URL 改为最新文档地址，并收窄普通 SKU 为 3 图/1 视频/1 音频及已证明组合。
6. [ ] 用唯一新版 normalizer 修复 Standard 双 ID、Fast errorCode 和单结果约束。
7. [ ] 用 `(channel_type, video_upstream_profile)` 权威表将 FunCloud 当前 revision 直接切换到 v2，删除 v1 支持；空 snapshot 和 v1 都 fail closed。
8. [ ] 确保 `general` Link 在 FunCloud converter 前解析为 HTTPS URL，`real_person` Link 在选渠时排除 FunCloud，FunCloud converter 显式拒绝裸 `asset://ast_*`。
9. [ ] 不注册 `realPersonMode` 对应 SKU，不实现或发送 `realPersonMode=true`。
10. [ ] 更新公开 capability hash、内部实现版本/快照、OpenAPI、管理端和测试。
11. [ ] 运行三数据库模型验证、相关 Go 测试、relaykit 独立构建和前端构建。
12. [ ] 使用真实 Key 串行验收，先启用一个普通 SKU 的内部 Ability。
13. [ ] 取得 `realPersonMode` 书面语义澄清、价格与合规确认后，再根据虚构人物或真人授权分支另行评审。
14. [ ] 更新事实文档和路线图，实施验证完成后将两份 80-dev 方案收敛归档。

## 13. 测试矩阵

### 13.1 删除旧版本

- 删除后路由表不再包含 `POST /v1/real-person-authorizations/:authorization_id/asset`；
- profile 验证拒绝 `funcloud_real_person_stream` 和 `upload_confirmed`；
- `model/main.go` 不再注册 `RealPersonAssetUploadAttempt`，但其它 NEWAPI migration 结构没有被重排或重构；
- `relaykit/dto/channel_settings.go`、`channel-form.ts`、`types.ts` 只移除本地 FunCloud 窄接线；
- 代码中不存在旧表扫描、旧 profile fallback、API 退役响应、feature flag 或数据迁移分支；
- OpenAPI、前端表单和 i18n 不再出现旧入口；
- 官方/Moxing 真人授权与素材回归不受影响。
- FunCloud 普通 SKU 可以消费由 Link Resolver 转换的 `general` Link，但不会创建 `AssetBinding` 或本地临时文件。
- 未经 Link Resolver 的裸 `asset://ast_*` 不得进入 FunCloud converter，`real_person` Link 在选渠时稳定排除 FunCloud。
- `expires_at > 0` 校验最小 TTL；`expires_at = 0` 按未知有效期 best-effort 执行，不建立永久 URL 推断。

### 13.2 新视频协议

- Standard `taskId=task_*`、`output.id=cgt-*` 正常处理；
- expected ID 只比较 `data.taskId`；
- Fast 顶层 `errorCode/errorMsg` 被保留；
- result 恰好一个；0 个、多个、冲突或非 HTTPS 均拒绝；
- processing/success/completed/succeeded/failed 映射；
- 未知状态、矛盾错误字段、无效 JSON进入合同违例；
- 新任务 snapshot 固定为 FunCloud v2；
- FunCloud 空 snapshot 明确拒绝，不自动当作 v2；
- FunCloud v1 snapshot 明确拒绝，不 fallback；
- Standard/Fast 路径和分辨率严格隔离；
- 省略默认值、显式 false、seed=0 保留。

### 13.3 普通 SKU 与 realPersonMode 隔离

- 普通 SKU 不发送 `realPersonMode`；
- 公共 `dto.ModelArkVideoCreateRequest` 不包含 `realPersonMode`，客户 JSON、metadata 和 extra 都不能注入或覆盖该字段；
- 客户提交同名未知字段返回 4xx；
- 不存在 `seedance-2.0-*-ai-realistic` 公开 SKU、capability、价格或 Ability 注册；
- 内部 converter 不提供任何将 `realPersonMode` 设为 true 的可达分支；
- Provider 书面澄清前，模型名、描述、错误和文档都不对该字段做“虚构人物”或“授权真人”的正向断言。

### 13.4 真实串行验收

| 类别 | 用例 |
| --- | --- |
| 环境 | 最新 Base URL 鉴权和最小请求 |
| Standard | 480p/720p/1080p 文生视频 |
| Fast | 480p/720p；1080p 明确拒绝 |
| 时长 | 4、5、15 秒和省略默认值 |
| 比例 | 文档全部七种 ratio |
| 多模态 | 3 图、1 图+1视频+1音频、编辑、延长 |
| 状态 | Standard 双 ID、Fast completed、两类失败信封 |
| 结果 | 单 URL、内容代理 200/206、过期和跨域 |
| 创建错误 | 参数、Key、余额、HTTP 200 application error、截断和超时 |
| 计费 | 普通请求逐 task ID 对账 |
| `realPersonMode` 后续评审 | Provider 书面澄清后，先确认属于虚构人物还是真人授权分支，再单独设计验收用例 |

## 14. 上线门禁

以下条件全部满足后，才允许启用普通 FunCloud Ability：

- [ADR-0015](../20-architecture/decisions/0015-Link公开SKU与实现身份版本绑定.md) 已接受；
- 旧 v1 parser、旧素材 profile、旧 raw upload route 和旧数据库模型已移除；
- NEWAPI 上游原有代码未因删除本地 FunCloud 扩展而被移动、重排或重构，共享文件只保留必要窄接线；
- 新任务只生成 FunCloud v2 snapshot；
- Base URL 已替换并用真实凭据验证；
- Standard 双 ID、Fast errorCode 和单结果合同通过；
- capability 已收窄为有文档/实测证据的范围；
- Link `source_url` 解析已完成加密存储、TTL、授权、错误脱敏和不下载验证；
- `SupportsManagedAssets=false`、`SupportsLinkAssets=true` 与运行时行为一致；
- 当前价格和失败结算方式已确认；
- 内容代理、创建未知、轮询违例和退款/对账通过；
- docs、Go、relaykit、前端和三数据库验证通过。

当前不存在可启用的 `realPersonMode` 专用 SKU。Provider 书面澄清合同冲突后，必须先选定虚构人物或真人授权分支，再另行完成产品合同、价格、排队/SLA、合规文案、授权生命周期（如适用）和真实验收；不得在当前实施中预留可达的 true 注入分支。

## 15. 当前决策

FunCloud 接入不再维护旧版本。旧 `54:third_party_funcloud_seedance_v2:v1` 解析、旧 H5/multipart 真人素材 profile、旧上传 endpoint、旧 attempt model 和旧渠道 asset settings 全部进入删除范围；没有在线兼容、数据转换或 fallback。

最新 FunCloud 视频合同作为唯一渠道实现：Base URL 使用文档声明的 `mm-internal-cn.leonecloud.com`，请求使用 JSON `content`，时长 4～15 秒，Standard/Fast 使用各自 endpoint；查询正确区分 wrapper task ID 与内部 output ID，并归一 Fast 顶层错误字段。

普通 `seedance-2.0-standard` / `seedance-2.0-fast` 不发送 `realPersonMode`，也不声明上游 managed-assets。它们作为 Link 的渠道执行上游，通过 `source_url` 模式消费平台 `general` Link，不创建 FunCloud `AssetBinding`。`realPersonMode` 在 Provider 书面澄清前不是平台产品能力：不注册对应 SKU，不实现或发送 true，也不将它预先归类为虚构 AI 写实或授权真人驱动。

公共 `dto.ModelArkVideoCreateRequest` 保持不变。FunCloud 是 Link 合同的视频执行 Provider，但不是 managed-assets Provider：请求级 HTTPS URL 不自动创建 `Asset`，已有 `general` Link 资源由 Resolver 转换为 HTTPS 源 URL，FunCloud 不创建 `AssetBinding`，平台不建立本地下载或 multipart 转换桥。`real_person` Link 资源当前仍在选渠前排除 FunCloud。

旧 FunCloud 素材链和旧视频协议属于本地新增、尚未发布的预接入实现，因此直接删除，不建立零存量门禁、数据迁移或兼容层。删除范围以代码来源为界：本地独立实现彻底退出，NEWAPI 上游共享文件只移除本地窄接线，避免扩大以后同步上游代码的冲突面。
