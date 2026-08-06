---
status: accepted
owner: Dev Team
last-reviewed: 2026-08-06
---

# FunCloud 国内 Seedance 2.0 模型接入设计

## 1. 目的、范围与状态

本文定义 FunCloud 国内 `seedance2.0` 与 `seedance2.0-fast` 如何作为 Link 服务合同的 Provider
实现接入 new-api。设计覆盖客户模型发布、SKU 能力、渠道配置、请求转换、异步 Task、Link 资源、
计费、错误、安全与发布门禁，不把 FunCloud 私有协议暴露给客户。

本文以 `docs/70-research/funcloud/` 下三份 FunCloud 官方资料快照为 Provider 证据，并服从下列上位
架构：

- [Link 服务合同概念与协作关系](../../Link服务合同概念与协作关系.md)；
- [Link 服务合同注册与履约架构](../../Link服务合同注册与履约架构.md)；
- [Link 资源合同与解析架构](../../Link资源合同与解析架构.md)。

`status: accepted` 表示合同边界和接入方案已经确定，不表示生产渠道已经开放。当前代码已登记
`funcloud.seedance-json@v1`、两个 Link SKU、FunCloud v2 adapter 和 `source_url` 素材解析。生产凭据
黑盒已闭合 Standard 三档文本、Fast 两档文本、Fast 参考视频、图片加音频和 Link Asset 图片的创建、
终态、本站内容代理与 Range；Standard 媒体稳定性、Provider 账单、确定拒绝和真人语义仍未闭合。

## 2. Provider 证据与保守边界

### 2.1 已确认事实

| 事实 | Standard | Fast |
| --- | --- | --- |
| Base URL | `https://mm-internal-cn.leonecloud.com` | 同左 |
| 创建路径 | `/api/v2/open/aigc/seedance2-0` | `/api/v2/open/aigc/seedance2-0-fast` |
| 查询路径 | `/api/v2/open/aigc/{taskId}` | 同左 |
| 鉴权 | `Authorization: Bearer <token>` | 同左 |
| 分辨率 | `480p`、`720p`、`1080p` | `480p`、`720p`，明确不支持 `1080p` |
| 时长 | 4—15 秒，默认 5 秒 | 4—15 秒，默认 5 秒 |
| 画幅 | `16:9`、`9:16`、`1:1`、`4:3`、`3:4`、`21:9`、`adaptive` | 同左 |
| 输入 | 文本、图片、视频、音频 | 同左 |
| 创建成功 | `code=0`、`data.taskId`、`data.status=processing` | 同左 |
| 查询活态 | `processing`；嵌套 output 还可为 `submitted` / `running` | `processing` |
| 查询成功态 | 顶层 `success`，嵌套 output 为 `succeeded` | 顶层 `completed` |
| 查询失败态 | `failed` | `failed` |

### 2.2 资料缺口与冲突

以下内容不能仅凭现有资料进入公开合同：

1. 文档展示了多图、多模态示例，但没有给出各媒体类型的数量、文件大小、格式、分辨率、时长和总
   请求大小上限；
2. 两个创建接口都声明 `returnLastFrame`，但 Fast 查询合同没有尾帧字段，Standard 也只在嵌套
   `output` 中描述该字段；
3. Standard 描述了批量查询与回调，但没有给出回调签名算法、密钥轮换、重放窗口和失败重试合同；
   Fast 文档也没有证明批量查询和回调与 Standard 完全等价；
4. 两份文档都引用独立“素材库 API 对接文档”，但该资料不在本次官方文档集合中，无法证明素材
   创建、查询、删除、作用域和凭据合同；
5. `realPersonMode` 一方面被描述为无需预认证即可处理普通人物图片，另一方面其合规文字又要求素材
   不得与自然人形象雷同，无法证明它满足平台真人授权、撤回和审计合同；
6. 返回 `result` 是数组，而当前 Link 视频客户合同只交付一个确定视频；没有证据说明多结果的顺序、
   计费和部分失败语义；
7. Standard 的数值错误码表没有按创建/查询端点拆分，也没有承诺任一错误必然发生在任务创建前；
   Fast 则在查询失败响应中使用字符串 `data.errorCode`，没有发布可证明“确定未创建”的创建错误
   结构或数值码表；
8. 资料没有提供供应商价格、任务级 usage、失败扣费、退款、结果 URL 有效期和 CDN 跳转规则。

因此初始公开合同采用“有证据才开放”的窄能力：资料缺失项拒绝或保持未发布，不能由 converter
猜测、静默丢弃或转成 Provider 默认行为。

## 3. Link 服务合同映射

### 3.1 五类权威事实

| 层次 | FunCloud 接入值 | 权威职责 |
| --- | --- | --- |
| 客户接入合同 | `modelark.contents.generations.v3` | 客户路径、DTO、响应、错误和 Task 投影 |
| 客户模型 publication | `(link, modelark_video, customer_model) -> Link SKU + version` | 客户模型稳定代表哪个 SKU |
| Link SKU capability | `seedance-2.0-standard`、`seedance-2.0-fast` | 字段、值域、媒体、资源和生命周期能力 |
| Link implementation | `funcloud.seedance-json@v1` | FunCloud 路径、profile、adapter、解析方式与执行绑定 |
| Channel / Ability | 客户模型、`model_mapping`、Key、分组、价格、优先级、权重 | 当前能否履约，不定义合同身份 |

三种模型身份保持分离：

```text
customer_model
  -> channel.model_mapping
  -> provider execution model
  -> funcloud.seedance-json@v1 execution binding
  -> Link SKU
```

FunCloud 创建请求本身没有 `model` 字段。这里的 provider execution model 是 execution binding 的
内部判别值：必须精确为 `seedance-2.0-standard` 或 `seedance-2.0-fast`，但不会发送给 FunCloud。

### 3.2 实现注册

以下内容是当前代码注册投影，不是从 Provider 资料或上位架构反推的通用术语。运行时权威来源为
`model/link_implementation.go` 与 `relay/common/video_adapter_version.go`；Provider 专属 adapter
版本和实现参数不提升为客户合同标识。

```text
implementation:        funcloud.seedance-json@v1
contract_id:           modelark.contents.generations.v3
route_family:          modelark_video
channel_type:          DoubaoVideo
video profile:         third_party_funcloud_seedance_v2
adapter version:       54:third_party_funcloud_seedance_v2:v2
query path:            /api/v2/open/aigc/{task_id}
asset profile:         none
asset resolution mode: source_url
task contract:         shared_video_task
billing contract:      newapi_quota
```

execution binding 为：

| Provider 执行模型 | Link SKU | 创建路径 |
| --- | --- | --- |
| `seedance-2.0-standard` | `seedance-2.0-standard` | `/api/v2/open/aigc/seedance2-0` |
| `seedance-2.0-fast` | `seedance-2.0-fast` | `/api/v2/open/aigc/seedance2-0-fast` |

Standard 与 Fast 必须使用不同渠道实例或至少不同的不可变执行配置。创建路径是 SKU 身份的一部分，
不能让同一条渠道根据请求临时切换路径，也不能把 Fast 当作 Standard 的重试降级。

## 4. 客户能力合同

### 4.1 SKU 能力矩阵

| 能力 | `seedance-2.0-standard` | `seedance-2.0-fast` |
| --- | --- | --- |
| 分辨率 | `480p` / `720p` / `1080p` | `480p` / `720p` |
| 默认分辨率 | `720p` | `720p` |
| 时长 | 4—15 秒，默认 5 秒 | 4—15 秒，默认 5 秒 |
| 画幅 | 官方列出的 7 个值 | 官方列出的 7 个值 |
| 文本 | 至少一个非空 `text` | 同左 |
| 图片 | 最多 3 个 | 最多 3 个 |
| 视频 | 最多 1 个 | 最多 1 个 |
| 音频 | 最多 1 个，且请求还须有图片或视频 | 同左 |
| 直接媒体 | 仅绝对 HTTPS URL；Provider 抓取风险由渠道准入和域名策略治理 | 同左 |
| Link 资源 | `general`；通过 `source_url` 解析 | 同左 |
| 混合媒体路径 | 同一请求不得混用直接 URL 与 `asset://ast_*` | 同左 |
| 生成音频 | 支持 `generate_audio` | 支持 `generate_audio` |
| 内容查询 | 支持 | 支持 |
| 取消、删除 | 不发布 | 不发布 |
| 尾帧返回 | 初始不发布 | 初始不发布 |
| Provider 回调、批量查询 | 不进入客户合同 | 不进入客户合同 |
| 真人能力 | 不发布 | 不发布 |

媒体数量 3 / 1 / 1 是当前公开 capability 的保守上限，不是对 FunCloud 极限能力的推断。扩大上限
必须先取得精确 Provider 证据，随后提升 capability 与 implementation/adapter 版本并增加计费和
回归测试。

### 4.2 content 规则

初始合同只接受：

- `text`：至少一个非空文本项；多个文本项按原顺序保留；
- `image_url`：`role` 只能为 `reference_image`、`first_frame` 或 `last_frame`；
- `video_url`：`role` 必须为 `reference_video`；
- `audio_url`：`role` 必须为 `reference_audio`；
- `last_frame` 必须与 `first_frame` 同时出现，首帧和尾帧各最多一个；
- 所有媒体项保持客户数组顺序，确保提示词中的“图片1 / 视频1 / 音频1”与 Provider 看到的顺序
  一致。

未识别的 content type、错误 role、空文本、空 URL、明文 HTTP、带 userinfo 的 URL 和未解析的
`asset://` 都在发送前返回稳定 4xx。

### 4.3 客户字段与 Provider 出站字段

客户初始可提交字段为：

```text
model, end_user_subject, content, duration, resolution, ratio,
generate_audio, watermark, seed, camera_fixed
```

其中可选标量必须使用指针与 `omitempty`：缺失表示使用已冻结 capability 默认值，显式 `0` 或
`false` 必须保留。`end_user_subject` 是客户接入合同中的平台安全字段：网关按 App 作用域计算不可逆
哈希并写入请求上下文，随后从类型化 Provider 请求中移除；它不发送 FunCloud、不建立真人身份、
不授予真人能力，也不能与客户提供的 `safety_identifier` 同时出现。

FunCloud 实际出站字段只有：

```text
content, ratio, duration, resolution,
generateAudio, watermark, seed, cameraFixed
```

`callback_url`、`service_tier`、`return_last_frame`、`execution_expires_after`、`draft`、`tools`、
`safety_identifier`、`priority`、`frames` 以及任何 FunCloud 私有扩展初始均拒绝。

## 5. 请求转换设计

### 5.1 字段映射

| ModelArk 客户字段 | FunCloud 字段 | 规则 |
| --- | --- | --- |
| publication 后的客户模型 | 无 | 只选择 execution binding 和创建路径，不进入请求体 |
| `content` | `content` | 只保留通过 capability 校验的类型、role 和媒体 URL |
| `ratio` | `ratio` | 缺失时规范化为 `16:9` |
| `duration` | `duration` | 缺失时规范化为 5；只发送 4—15 的整数 |
| `resolution` | `resolution` | 缺失时规范化为 `720p`；Fast 拒绝 `1080p` |
| `generate_audio` | `generateAudio` | 保留显式 `false` |
| `watermark` | `watermark` | 保留显式 `false` |
| `seed` | `seed` | 保留显式 `0` |
| `camera_fixed` | `cameraFixed` | 保留显式 `false` |

converter 不读取数据库、不选择渠道、不解密 AssetSource、不推断 Link SKU，也不接收已经由客户
接入层消费的 `end_user_subject`；它不能从 `metadata`、`extra` 或未知 JSON 字段接受
`realPersonMode`、`callbackUrl`、`returnLastFrame` 等旁路参数。

### 5.2 默认值与路径锁定

默认值在 Link capability 冻结后由网关显式写入上游请求，不能依赖 Provider 未来改变默认行为。
发送前必须同时复检：

```text
publication Link SKU
= implementation execution binding Link SKU
= model_mapping 后的 provider execution model
= 渠道固定创建路径对应的 Link SKU
```

任一不一致都在 Provider POST 前失败关闭。

## 6. Link 资源与多媒体输入

### 6.1 两种 asset URI 不得混淆

| URI | 所属合同 | 客户是否可直接使用 |
| --- | --- | --- |
| `asset://ast_*` | new-api Link 资源合同 | 是 |
| `asset://<FunCloud assetId>` | FunCloud 私有素材合同 | 否 |

现有资料没有包含 FunCloud 素材 API 的完整生命周期合同，因此 `funcloud.seedance-json@v1` 不创建、
查询或删除 FunCloud 托管素材，也不把 Provider asset ID 保存为 `AssetBinding`。客户提交
`asset://ast_*` 后，由唯一 Resolver 在选渠前验证资源，并在每次 Provider 尝试前解析为受保护
的 HTTPS `source_url`；converter 只接收解析结果。

### 6.2 解析约束

FunCloud 实现的 Link 资源能力固定为：

```text
asset_kind:             general
media_type:             image / video / audio
resolution_mode:        source_url
source_min_ttl_seconds: 300
max_images/videos/audio: 3 / 1 / 1
```

执行顺序为：

```text
冻结 publication 与 SKU
  -> 校验 Asset 所有权、App、状态、类型和 publication 快照
  -> 计算所有 Asset 都可由同一 FunCloud 候选解析的交集
  -> 校验已声明 TTL 至少剩余 300 秒；未知 TTL 按 best-effort
  -> NEWAPI 选渠
  -> 发送前再次校验并解密 source URL
  -> 将 asset://ast_* 改写为 HTTPS
  -> FunCloud converter
```

同一请求一旦出现 Link 资源，其它媒体也必须使用 Link 资源，不能混入请求级 URL。请求级 HTTPS URL
只属于当前调用，不进入 Asset CRUD、授权、迁移或长期保存。AssetSource 过期、解密失败或多素材
候选交集为空时 fail closed，不把裸 `asset://ast_*` 发给 Provider。

### 6.3 真人边界

`real_person` Asset 和 `realPersonMode=true` 均不属于初始 FunCloud 实现。网关不能从普通 URL 自动
判断是否包含真人，也不能以 FunCloud 合规声明替代平台的授权、撤回、任务 reservation 和结果回源
检查。在取得 Provider 书面合同并完成法律、产品、授权、计费和黑盒验收前：

- capability 不登记 `real_person`；
- converter 不发送 `realPersonMode`；
- 普通客户字段、metadata 和渠道覆盖不能开启该能力；
- 对外文档不宣称已支持 FunCloud 真人模式。

## 7. 异步创建与任务履约

### 7.1 创建安全边界

所有 FunCloud 创建请求进入共享异步安全链：

```text
publication 与 capability 校验
  -> 候选过滤、选渠、model_mapping 和 execution binding 复检
  -> 冻结计费 probe、implementation、渠道、单 Key 和媒体解析事实
  -> TaskCreateAttempt(prepared)
  -> 预扣 + billing hold + sending 原子提交
  -> POST FunCloud
  -> 可信 taskId：创建 Task、转移 hold、完成 attempt
  -> 创建结果模糊：unknown，对账期间不自动重发或退款
```

`Idempotency-Key` 只幂等平台 attempt；不能假设 FunCloud 已提供等价幂等键。发送后断连、超时、
无效 JSON、未知应用错误或 `code=0` 但缺少可信 `taskId` 时，不得换渠道再次 POST。

### 7.2 创建响应

可信成功必须同时满足：

- HTTP 响应可解析为预期 JSON；
- `code=0`；
- `data.taskId` 非空、长度受控且不含空白或控制字符；
- `data.status=processing`。

现有资料不足以登记任何自动“确定未创建”组合：

- Standard 资料把 `10002`、`10005`、`10006` 分别定义为参数错误、API Key 错误和余额不足，但没有
  绑定 HTTP 状态、创建端点或“任务绝未创建”保证；
- `90003` 已明确定义为服务器内部错误，业务结果不可判定，必须进入创建 `unknown`；
- `30003` 被定义为任务不存在，但资料没有按端点限定其出现位置；一次查询不存在不能倒推创建一定
  失败；
- Fast 查询失败使用 `INVALID_RESOLUTION`、`INSUFFICIENT_BALANCE`、`INVALID_CONTENT`、
  `INVALID_URL`、`TASK_NOT_FOUND` 等字符串 `data.errorCode`，但这不能证明 Fast 创建失败也使用相同
  结构，更不能把 Standard 数值码套用到 Fast 创建路径。

因此，任何已经发送请求字节后的非零应用 code、非 2xx、协议冲突、解析失败或网络错误都先进入
`unknown`，保留 hold 且不自动重发。未来只有取得书面且可重复验证的证据后，才能按下列完整键登记
确定拒绝：

```text
implementation ID/version
+ Link SKU/create path
+ HTTP status
+ application error code
-> terminal rejection
```

分类器必须实际接收并匹配 HTTP status；不能只按响应 body 中的数值或字符串 code 释放 hold。

### 7.3 查询与状态归一化

查询只使用 Task 冻结的 Base URL、查询模板、单 Key、adapter 版本和 upstream task ID，不重新选渠：

| FunCloud 状态 | Link Task 状态 |
| --- | --- |
| `processing` / `running` / `submitted` | `running` |
| `success` / `completed` / `succeeded` | `succeeded` |
| `failed` | `failed` |

查询响应必须满足：

- `code=0`；
- 顶层 `data.taskId` 与冻结 task ID 精确相等；
- 同时存在顶层与 `output` 状态时，两者归一化后必须一致；
- 成功时只能归一出一个绝对 HTTPS 视频 URL；`result[0]` 与
  `output.content.video_url` 同时存在时必须完全相等；
- 失败错误码和消息经过长度限制与脱敏，不能携带 URL、Token、Cookie 或 Authorization 信息。

无效 JSON、未知状态、ID 不匹配、状态冲突、多结果、结果冲突、成功无 URL 或非法 URL 都是轮询
合同违例，进入 `RECONCILIATION_REQUIRED`；它们不能直接把业务任务标为失败或退款。可信失败按
冻结合同结算，确定不可交付终态另记 Provider exposure。

### 7.4 不采用 Provider 回调和批量查询

任务状态仍由共享 Task 调度器按冻结单任务查询合同拉取。初始实现不使用 FunCloud `callbackUrl`，
也不调用 `/api/v2/open/aigc/batch`：

- 回调签名算法和重放边界未闭合，开放它会新增公网入站安全面；
- 批量响应合同未在资料中给出，且没有证明 Fast 共用相同语义；
- 单任务查询已经满足客户合同，不需要建立第二套状态权威。

## 8. 计费、退款与 Provider exposure

### 8.1 客户计费

FunCloud 资料没有提供可直接用于客户计费的价格或 usage，因此客户费用继续以 NEWAPI 冻结价格和
结算日志为唯一权威。计费表达式只读取已经过 capability 校验的受信 probe：

```text
duration_seconds
resolution
has_video_input
generate_audio（仅在已审批价格确实使用时）
```

Standard 与 Fast 使用独立客户模型价格；不能用一个 SKU 的价格覆盖另一个 SKU，也不能把 FunCloud
价格写到官方、TokenSave、飞彩或 NEWAPI 原生视频模型。数值价格属于配置与审批事实，不从本文三份
Provider 资料推导。

### 8.2 预扣与终态

- 预扣、Task 和终态结算使用同一冻结表达式、probe、客户模型与分组倍率；
- 时长必须先限制在 4—15，再成为乘数；Fast 的 `1080p` 在计费前拒绝；
- quota 换算使用 checked 饱和函数，饱和请求必须因额度不足失败，不能溢出为负数；
- Provider 未返回可信 usage 时，结算只能使用冻结且已审批的请求维度，不能解析错误文本猜测用量；
- 创建 unknown 保持 hold；到期仍无法恢复时释放客户资金并幂等记录 Provider cost exposure；
- 客户退款和 Provider 可能已发生的成本分账，不得以不退客户额度隐藏平台损失。

Provider exposure 至少按 `channel + Link SKU + funcloud.seedance-json@v1 + profile + reason + window`
隔离。策略缺失、过期或预算耗尽时 FunCloud 候选 fail closed。

## 9. 渠道配置与发布

### 9.1 固定配置

FunCloud 渠道保存时必须验证：

- Base URL 精确为 `https://mm-internal-cn.leonecloud.com`，无 userinfo、query 和 fragment；
- profile 精确为 `third_party_funcloud_seedance_v2`；
- query path 精确为 `/api/v2/open/aigc/{task_id}`；
- Standard 只使用 `/api/v2/open/aigc/seedance2-0` 和 Standard execution model；
- Fast 只使用 `/api/v2/open/aigc/seedance2-0-fast` 和 Fast execution model；
- `asset_upstream_profile=none`，素材方式只能由 implementation 登记的 `source_url` 获得；
- 精确选择 `funcloud.seedance-json@v1`，且 exposure、价格和单 Key 配置完整。

### 9.2 发布流程

```text
管理员选择 FunCloud Link 接入方案
  -> 系统锁定 implementation/profile/adapter/path/素材方式
  -> 配置 customer_model -> provider execution model
  -> execution binding 唯一解析 Link SKU
  -> 校验 capability、价格、exposure 和素材覆盖
  -> 启用时原子创建或核对 publication
  -> 发布以 customer_model 为键的 Ability
```

已有 publication 不能由普通保存改绑。候选耗尽时返回“已发布但暂不可履约”，不得降级到另一个
FunCloud SKU、其它 Provider 私有 SKU、普通 DoubaoVideo 候选或 NEWAPI 原生 `/v1/videos`。

## 10. 安全与可观测性

- FunCloud Bearer Key 只保存在渠道和 Task 私有快照，不进入客户响应、普通日志或错误；
- 完整 AssetSource URL 只在发送前短暂解密，不进入 Task 快照、日志、指标标签和管理员普通页面；
- 请求级直接媒体由 FunCloud 抓取，网关只强制绝对 HTTPS、无 userinfo 等结构规则；由于网关不下载
  媒体，不能宣称已经通过本地 DNS/IP 检查证明其为公网地址，剩余风险由渠道准入和域名策略治理；
- Provider 真实 task ID、嵌套 output ID 和结果 URL 不作为客户任务 ID；
- 结果 URL 至少验证绝对 HTTPS、无 userinfo 和长度上限，并通过本站内容代理交付；生产开放前还须
  固化允许的 CDN origin、重定向和凭据转发策略；
- Provider 错误只记录受控 code 与脱敏摘要；未知 HTML、请求头、响应头和完整 body 不回显客户；
- 审计关联 customer model、publication version、Link SKU、implementation、channel、profile、
  attempt、Task、计费和 exposure，但普通客户只看到客户模型和 ModelArk Task 合同。

## 11. 当前实现差异与发布门禁

### 11.1 已有代码事实

当前代码已经具备：

- 两个 FunCloud SKU capability 和 `funcloud.seedance-json@v1` 显式注册；
- Base URL、Standard/Fast 路径和 query path 的渠道校验；
- ModelArk 到 FunCloud camelCase JSON 的类型化 converter，并保留显式 `false` / `0`；
- 创建响应、Standard/Fast 查询状态和结果的统一归一化；
- durable create attempt、共享 Task、计费 hold、unknown 对账和 exposure 接线；
- `asset://ast_* -> source_url` Resolver，且 converter 拒绝未解析 Asset；
- Fast `1080p` 拒绝、媒体数量上限和基础合同测试。

### 11.2 已闭合的结构差异

当前代码已经闭合原设计中的三项结构差异：

1. capability 显式登记图片、视频和音频 role 白名单，内部调用与公开入口使用同一约束；
2. `end_user_subject` 按 App 哈希后从类型化 Provider 请求移除，不进入 FunCloud body，也不授予真人能力；
3. 已删除仅按 FunCloud body 数值 code 判断确定拒绝的跨 SKU 规则。未登记的
   `implementation/SKU/path + HTTP status + application code` 组合一律保持 create unknown。

### 11.3 真实证据边界与待闭合项

2026-08-05 至 2026-08-06 的生产凭据黑盒与当前数据库形成以下事实：

- Standard 的 480p、720p、1080p 四秒文本任务成功；Fast 的 480p、720p 四秒文本任务成功；
- Fast 的 720p 参考视频、480p 中性图片加音频和 480p `asset://` 图片成功，内容代理与 Range 均可用；
- Standard 的图片、图片加音频和参考视频在四秒与五秒样本中多次进入可信 Provider 失败；同一素材在
  Fast 存在成功证据，因此不能把失败统一解释为平台 URL 转换问题，也不能横向推断 Standard 已通过；
- 当前数据库冻结 21 个真实 Task：13 个成功、8 个失败。21 个 create attempt 均为
  `complete + transferred`；成功任务 `SUCCESS + settled`，失败任务 `FAILURE + settled + quota=0`
  且存在退款日志；
- 该批客户账单毛消费 4,762,140 quota、退款 2,082,824 quota、净额 2,679,316 quota，与成功 Task
  最终 quota 合计一致。原实施报告冻结的是更早的 20-Task 快照；其后新增 1 个 Fast 成功 Task，增加
  293,000 quota 且未产生新退款。该勾稽只证明平台按冻结价格正确执行，不证明供应商实际成本或生产售价已批准。

生产开放前仍须完成：

1. 用供应商确认的中性媒体规格使 Standard 图片、图片加音频和参考视频获得稳定成功证据，并继续验证
   准备发布的其它画幅、默认值和 3 / 1 / 1 媒体边界；
2. 验证 `generateAudio`、`watermark`、`seed=0`、`cameraFixed=false` 的真实可观察行为；
3. 固化 Standard/Fast 错误结构、结果 URL origin/有效期/CDN 跳转和连续 Range 合同；
4. 用故障注入验证断连、坏 JSON、未知 code、ID/状态/结果冲突进入正确的 unknown 或 reconciliation；
5. 逐笔取得成功、内容拒绝和生成失败的 Provider 账单，验证币种、精度、时长口径、价格维度与失败扣费；
6. 完成 SQLite、MySQL、PostgreSQL 的 publication、attempt、Task、结算和 exposure 回归；
7. 保持 `realPersonMode`、Provider 托管素材、回调、批量查询和尾帧能力关闭，直到各自合同完整取证。

## 12. 验证矩阵

| 类别 | 必测场景 |
| --- | --- |
| 发布 | Standard/Fast 路径交叉配置、错误 model mapping、普通候选冲突、零候选 fail closed |
| 标量 | 缺省、最小、最大、越界 duration；每个 resolution/ratio；显式 `false` 与 `seed=0` |
| 媒体 | 0/3/4 图，0/1/2 视频，0/1/2 音频；入站与 capability 两层 role 错误；音频无视觉参考 |
| 资源 | 绝对 HTTPS、HTTP/Data URL 拒绝、Link 资源、混合路径、TTL 充足/不足/未知、过期、所有权与 App 不匹配 |
| 创建 | 正常 taskId、空/超长/控制字符 ID、Standard 数值码、Fast 真实失败结构、`90003`、未知 code、不同 HTTP 状态、断连、坏 JSON；未登记组合全部保持 unknown |
| 查询 | Standard `success`、Fast `completed`、嵌套 `succeeded`、`30003`/`TASK_NOT_FOUND` 的真实端点语义、未知状态、ID/状态/URL 冲突 |
| 计费 | Standard/Fast、三种 Standard 分辨率、两种 Fast 分辨率、视频输入、预扣/结算一致、饱和 |
| 安全 | `end_user_subject` 哈希存在且不进入上游 body；Key、源 URL、Provider task ID、错误正文不泄露；非法结果 URL 和重定向拒绝 |
| 数据 | publication/Task/attempt/exposure 幂等与三数据库行为一致 |

## 13. 架构不变量

1. FunCloud 只履约 `seedance-2.0-standard` 与 `seedance-2.0-fast`，不能加入其它 Seedance SKU 的候选集。
2. Standard 与 Fast 是两个独立 Link SKU；分辨率差异、创建路径和价格不能在运行时静默降级。
3. 客户模型、Link SKU 和 Provider 执行模型保持分离，publication 是已发布身份权威。
4. FunCloud 私有路径、Bearer Key、业务 code、asset ID、task ID 和状态外壳不进入客户合同。
5. Provider 资料未闭合的字段、媒体上限、回调、批量、尾帧和真人能力默认关闭。
6. `asset://ast_*` 只能由 Resolver 解析；FunCloud converter 不读取数据库或处理裸 Asset URI。
7. FunCloud 当前只实现 `general + source_url`，不把 Provider 私有素材协议冒充平台 Link 资源合同。
8. 所有 Provider POST 均先建立 durable attempt，并在资金 hold 与 `sending` 提交后发送。
9. 创建未知不自动重发，轮询合同违例不直接判失败，退款与 Provider exposure 分账。
10. 自动确定拒绝必须匹配完整的 implementation、SKU/path、HTTP status 和应用错误码注册；未登记
    组合一律保持 unknown。
11. `end_user_subject` 只形成平台安全哈希，不进入 FunCloud 请求，也不表示真人能力。
12. 客户费用只来自 NEWAPI 冻结计费与结算日志，不从 Provider 当前报价或错误信息回算。
13. Link 候选耗尽时 fail closed，不修改或包装 NEWAPI 原生 OpenAI Videos 合同。
14. implementation、capability 或 adapter 的不兼容变化必须提升版本，不能原地改变 content hash 语义。

## 14. 资料来源

- [FunCloud 多模态模型对接汇总](<../../../70-research/funcloud/国内-fun%20cloud%20对接文档.md>)
- [FunCloud 国内 Seedance 2.0 API](<../../../70-research/funcloud/国内-open-api-seedance%202.md>)
- [FunCloud 国内 Seedance 2.0 Fast API](../../../70-research/funcloud/国内-open-api-seedance2-0-fast.md)
- [Link 视频服务合同与异步任务架构](../../Link视频服务合同与异步任务架构.md)
- [API Key 用量账单架构](../../API-Key用量账单架构.md)
