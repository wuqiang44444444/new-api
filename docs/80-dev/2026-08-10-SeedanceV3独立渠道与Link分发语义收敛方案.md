---
status: draft
owner: Dev Team
last-reviewed: 2026-08-10
---

# Seedance 专用渠道、ModelArk V3 北向与素材库简化方案

> 本文是已完成业务评审的待实施方案，不是当前生产事实。实施后必须把长期边界收敛到
> `20-architecture/`、更新被本决策取代的硬约束与 ADR，并完成真实 Provider、计费、三数据库与
> 灰度验证。本文不得用于声称功能已经生产发布。

## 1. 最终结论

Seedance 不再作为 `DoubaoVideo` 渠道内的隐藏 Link 方案或 profile，而是建立管理人员可以
直接理解的独立业务渠道：

```text
技术类型：ChannelTypeSeedanceLink
管理名称：Seedance 专用渠道
北向视频合同：ModelArk V3
北向素材合同：平台 /v1/assets + /v1/asset-groups
南向实现：代码化 upstream_protocol
```

新渠道覆盖所有 Seedance 模型线路，包括：

- 国内火山方舟官方；
- 海外 BytePlus 官方；
- 飞彩、FunCloud、TokenSave 等第三方 Seedance 上游；
- 以后经技术人员线下确认兼容的新 Seedance 线路。

上述线路的南向协议不必都是 ModelArk V3。“Seedance 专用渠道”是业务分类，
`upstream_protocol` 才是技术协议分类。

## 2. 本次收敛的核心原则

### 2.1 管理语义优先

管理员只需知道“这是哪个 Seedance 模型、哪个渠道、使用哪种上游协议”。允许在
Seedance 专属代码中保留少量局部重复，不为追求通用抽象把管理概念再次混入
`DoubaoVideo`。

### 2.2 人与技术分工

- 技术人员线下确认某个模型和上游是否符合已有协议；
- 已经兼容现有协议的新模型，管理员通过模型和渠道配置即可上线；
- 不兼容现有协议时，由技术人员新增代码化 `upstream_protocol` adapter；
- 不建立管理员无法理解的 JSON 字段映射、状态脚本、自动认证或能力推断系统。

### 2.3 系统只保证高价值不变量

保留：

- 统一 ModelArk V3 请求结构校验；
- Token 和模型权限；
- 计费安全与防溢出；
- 视频 Task 的 durable create attempt、资金 hold 和冻结快照；
- 平台素材的 `user_id + app_id` 隔离；
- 素材与确定渠道、账号和 Project 的不变绑定；
- 敏感凭据和完整签名 URL 的保护。

删除或不再新增：

- 为极低概率异常建立的运行时补丁；
- 重复门禁、启动扫描、后台自动修复和管理员核查工作流；
- 通过渠道、Ability、模型名或 profile 反向推导复杂 Link 合同身份。

## 3. 模型与渠道的唯一关系

### 3.1 不同渠道绝不共用前端模型名

管理约定是：不同 Seedance 渠道的 Provider 模型即使品牌和版本相同，也必须使用不同的
前端客户模型名。

```text
客户模型 A -> 国内火山渠道 -> Provider 模型
客户模型 B -> 海外 BytePlus 渠道 -> Provider 模型
客户模型 C -> 飞彩渠道 -> Provider 模型
客户模型 D -> TokenSave 渠道 -> Provider 模型
```

唯一路由为：

```text
customer_model
  -> Group / 模型权限
  -> 唯一 ChannelTypeSeedanceLink
  -> model_mapping
  -> provider_model
```

### 3.2 Priority 和 Weight 不参与 Seedance 路由

Seedance 不使用：

- Priority；
- Weight；
- Affinity；
- 随机分发；
- 失败重选；
- 跨渠道重试；
- fallback。

`Group` 只决定客户是否有权使用该模型，`Ability` 只登记模型与渠道的关系。

管理端在 `ChannelTypeSeedanceLink` 下置灰 Priority 和 Weight，使用普通人可理解的提示：

> **Seedance 模型不使用优先级和权重**  
> 每个 Seedance 模型只能配置在一个 Seedance 专用渠道中。请求会直接发送到这个渠道，
> 不会自动切换到其他渠道。

字段置灰提示：

> 此设置对 Seedance 专用渠道无效。

### 3.3 只在保存和启用时防止重名

系统只在以下管理操作中检查前端模型名是否已被其他已启用的 Seedance 渠道使用：

- 新建并启用渠道；
- 修改已启用渠道的模型列表；
- 将停用渠道重新启用。

冲突时拒绝保存或启用：

> 模型“{model}”已经配置在渠道“{channel}”中。每个 Seedance 模型只能使用一个渠道，
> 请更换模型名称或先停用原渠道。

停用渠道可以暂时保留重复模型，但重新启用前必须解决冲突。请求时不重复检查，也不增加：

- 数据库唯一约束；
- 启动扫描；
- 并发补偿；
- 历史数据自动修复；
- 运行时重名防御分支。

通过非标准方式直接修改数据库造成的非法状态由管理员自行检查和修正。

## 4. 视频北向合同

### 4.1 完整四组行为

`ChannelTypeSeedanceLink` 北向提供完整 ModelArk V3 任务接口：

```text
POST   /api/v3/contents/generations/tasks
GET    /api/v3/contents/generations/tasks/{task_id}
GET    /api/v3/contents/generations/tasks
DELETE /api/v3/contents/generations/tasks/{task_id}
```

其中：

- create 通过前端模型直接确定唯一渠道；
- get 通过平台 Task ID 和创建时冻结的 adapter 查询；
- list 查询本地主数据库，在平台 `user_id + app_id` 内汇总不同 Seedance 模型的任务；
- delete 通过 Task 冻结 adapter 调用上游；上游不支持时返回诚实的不支持错误，
  不伪造取消或删除成功。

### 4.2 不接入历史 `/v1/video/generations`

`/v1/video/generations` 继续属于 NEWAPI 原生 `DoubaoVideo` 和已有渠道。Seedance 专用渠道不为了
复用历史 OpenAI 式视频 DTO 而接入该路由。

```text
/v1/video/generations       -> 原有渠道
/api/v3/contents/.../tasks  -> ChannelTypeSeedanceLink
```

### 4.3 请求校验边界

系统校验统一 ModelArk V3 请求结构、必填字段、媒体类型和计费安全上界。不再通过
publication、Link SKU capability、implementation 或 execution binding 决定一个管理员已配置模型是否
可以使用 Seedance 路由。

不支持字段不得被默默删除或改义。选定 adapter 不支持时返回明确错误。

## 5. 南向协议与管理配置

### 5.1 `upstream_protocol` 是代码，不是 JSON 脚本

南向协议以代码 adapter 实现，渠道只选择已经存在的协议类型。示例：

```text
video_upstream_protocol:
  modelark_v3_volcengine
  modelark_v3_byteplus
  media_task_v1
  media_arrays_v2
  funcloud_seedance_v2

asset_upstream_protocol:
  none
  volcengine_assets_action_v2024_01_01
  byteplus_assets_action_v2024_01_01
  ark_assets_v1
  relay_assets_v1
```

不允许管理员编写请求字段映射、响应状态脚本或任务生命周期 JSON。

### 5.2 字段根据协议按需显示

| 渠道方案 | 管理员配置 |
| --- | --- |
| 国内火山官方直连 | 视频 API Key；素材 AK/SK；Region；Project；模型映射 |
| 海外 BytePlus 官方直连 | 视频 API Key；素材 AK/SK；Region；Project；模型映射 |
| 飞彩、FunCloud、TokenSave 等第三方 | Base URL；平台 Key；视频协议；素材协议；模型映射 |
| 无素材库上游 | Base URL；Key；视频协议；`asset_upstream_protocol=none`；模型映射 |

第三方已经代理官方 AK/SK、Region、Project、真人认证或 Asset Group 时，管理端不再要求
这些字段。

NEWAPI 现有 `param_override` 和 `header_override` 继续显示，不为 Seedance 单独隐藏或新增规则。

### 5.3 凭据轮换和账号身份

素材只绑定稳定的 `channel_id` 与素材账号作用域，不使用包含 Secret 的 credential fingerprint 作为
素材资格。

- 同一上游账号内正常轮换 API Key 或 AK/SK，既有素材继续可用；
- 存在素材时，Region、Project、国内/海外类型和素材协议不允许直接修改；
- 切换云账号、Project、区域或素材协议时必须新建渠道；
- 密钥轮换可使用只读素材查询确认新凭据仍属于预期账号，但不建立通用自动账号认证系统。

## 6. 视频任务语义

### 6.1 创建后不切换

Seedance 是高成本、重任务。每次视频创建只允许一次 Provider POST：

```text
前端模型
  -> 唯一渠道
  -> 冻结协议、Provider 模型、凭据、素材和计费事实
  -> 一次 Provider POST
  -> 成功 / 明确失败 / unknown
```

禁止：

- 自动重发；
- 换渠道重建；
- 从火山切换到飞彩、FunCloud 或 TokenSave；
- 把网络结果不明当成确定失败并立即退款。

### 6.2 视频 `unknown` 仍必须保留

视频创建发送后无法确定 Provider 是否创建任务时，create attempt 保持 `unknown`：

- 不自动重试；
- 不自动切渠道；
- 不自动退款；
- 保留资金和 Provider exposure 事实，供后续对账。

这与低频、低成本的素材管理失败不同，不得混用状态策略。

### 6.3 不新增平台幂等合同

火山和 BytePlus 公开的视频创建请求未提供可依赖的 `Idempotency-Key` 或 `ClientToken`。
Seedance 北向不要求客户提供平台自定义幂等键，也不因缺少幂等键拒绝官方请求。

## 7. 中转平台素材库边界

### 7.1 官方素材库是独立控制面

官方素材库不是 URL 缓存。官方流程包含：

```text
可访问 URL
  -> CreateAsset
  -> Processing / 预处理 / 审核 / 一致性检测
  -> Active / Failed
  -> asset://<Provider Asset ID>
  -> Seedance 视频生成
```

真人素材还可以包含本人认证、授权、Asset Group 和同人一致性检测。因此平台必须保留
官方素材库能力，但不建设自己的审核、人脸表单或法律授权系统。

火山官方说明参见：

- [私域真人人像库使用指南](https://www.volcengine.com/docs/82379/2315856?lang=zh)；
- [Doubao Seedance 2.0 高级创作权益包](https://www.volcengine.com/docs/82379/2377608?lang=zh)；
- [火山方舟安全创作体系](https://www.volcengine.com/activity/Seedance2-0-security?infrom=100001.100.140)。

BytePlus 官方说明参见：

- [Private real-human asset library guide](https://docs.byteplus.com/en/docs/modelark/2333589)；
- [GetVisualValidateResult API](https://docs.byteplus.com/en/docs/ModelArk/2333588)；
- [DeleteAsset API](https://docs.byteplus.com/en/docs/ModelArk/2318278)。

### 7.2 北向是平台素材 API，南向是多种素材协议

中转平台不要求客户持有 Provider AK/SK，因此不复制官方 Action 签名路由作为客户合同。
客户统一使用：

```text
POST   /v1/asset-groups
GET    /v1/asset-groups/{group_id}
GET    /v1/asset-groups
DELETE /v1/asset-groups/{group_id}

POST   /v1/assets
GET    /v1/assets/{asset_id}
GET    /v1/assets
PATCH  /v1/assets/{asset_id}
DELETE /v1/assets/{asset_id}
```

客户资源身份为：

```text
Asset Group: astgrp_xxx
Asset:       ast_xxx
视频引用: asset://ast_xxx
```

平台发送上游时转换为真实 Provider Group/Asset ID。

### 7.3 一个平台素材只对应一个上游素材

目标是一对一绑定：

```text
ast_xxx
  -> user_id + app_id
  -> channel_id
  -> asset_upstream_protocol
  -> Provider 账号作用域
  -> Region + Project（如协议需要）
  -> Provider Asset ID
  -> Provider status + moderation strategy
```

删除通用 0..N `AssetBinding`、多渠道素材候选、自动物化、自动迁移与 source/binding fallback。

创建素材时请求可以携带一个前端模型用于找到唯一渠道，但素材本身不永久绑定该模型。
同一渠道、账号和 Project 下的其他兼容 Seedance 模型可以复用素材。

### 7.4 国内与海外分离，但不让客户选区域

国内火山与海外 BytePlus 的账号、Region、Project、权益和真人认证协议不互通。同一份媒体如果
需要在两个区域使用，必须分别创建两个平台素材、分别通过上游处理和审核。

客户不通过 Token/Group 或每次请求选择 `cn/global`。不同渠道本就使用不同前端模型名和模型
页面展示，客户在选择模型时已经选定对应线路。

### 7.5 裸 Provider ID 必须先导入

北向只接受：

```text
asset://ast_xxx
```

不接受客户直接提交：

```text
asset://asset-2026xxxx
```

因为裸 ID 不能证明所属用户、应用、账号、Project 和渠道。火山或 BytePlus 控制台已存在的
素材不自动同步，只能由管理员显式导入并分配到具体 `user_id + app_id`，然后生成
`ast_*` 或 `astgrp_*`。

Provider `ListAssets` 只用于管理员连通性测试和技术分析，不把账号级列表直接展示给普通客户。

## 8. Asset Group 与真人认证

### 8.1 删除平台真人授权系统

不实现以下平台概念：

- `RealPersonAuthorization`；
- `/v1/real-person-authorizations`；
- `APIServiceRuleAcceptance`；
- `end_user_subject`；
- 平台人脸、活体或授权表单；
- 平台 H5 中转页面；
- 自建授权撤回、Task reservation 和内容回源门禁。

真人认证是素材上游协议中的一种 Asset Group 创建方式，不是平台独立业务域。

### 8.2 直接返回官方或第三方链接

创建真人素材组时：

```text
客户 POST /v1/asset-groups
  -> 平台按模型找到唯一渠道
  -> adapter 请求上游创建认证/邀请
  -> 直接返回官方或第三方 verification_url / QR 链接
  -> 客户直接访问上游页面
  -> GET /v1/asset-groups/{id} 时查询上游结果
```

上游允许客户 `CallbackURL` 时，平台做必要的 URL 安全校验后直接传递，不建立自己的
callback 页面。只有某个上游协议强制回调平台时，才在该 adapter 内增加最窄的接线。

`AssetGroup` 只保存租户归属、冻结渠道、Provider Group ID、上游状态和必要的短期查询句柄。
平台不保存人脸媒体、身份证件、活体数据、人脸特征或授权表单正文。

### 8.3 删除和撤回跟随上游真实能力

- Provider 支持删除 Group 或取消授权：调用已实现 adapter；
- Provider 不支持：返回明确不支持；
- 平台不声称已撤回 Provider 授权、已删除 Provider 数据或已收回平台外副本。

## 9. 素材创建、查询和删除

### 9.1 客户请求

素材创建使用前端模型找到唯一渠道：

```json
{
  "model": "<customer-model>",
  "group_id": "astgrp_xxx",
  "url": "https://customer.example/media.png",
  "asset_type": "image",
  "moderation": {
    "strategy": "default"
  }
}
```

渠道 `asset_upstream_protocol=none` 时直接返回“该模型不支持素材库”。不扫描其他渠道，也不切换。

### 9.2 `Moderation` 保持上游语义

- `/v1/assets` 允许传递官方 `moderation` 语义；
- 平台不自行判断上游账号是否允许 `Skip`；
- 上游不支持时返回明确错误，不默默删除字段；
- 平台保存实际 moderation strategy；
- `Active` 只表示 Provider 按该策略接受素材，不表述为平台已完成全面合规审核；
- 管理员仍可使用 NEWAPI 现有 `param_override` 覆盖策略。

### 9.3 允许平台素材与 URL/Data URL 混用

ModelArk V3 请求可以同时包含 `asset://ast_*` 和普通 HTTP/Data URL。BytePlus 官方示例已展示
`asset://` 与 HTTPS URL 共存。

只要请求含有平台素材：

- 模型对应的唯一渠道必须与所有素材的冻结渠道相同；
- 多个素材必须属于相同渠道、账号、Region 和 Project；
- 普通 URL/Data URL 跟随这个唯一渠道发送；
- 不再使用 SKU capability 或全局“禁止混用”门禁；
- 具体上游不支持时返回真实错误。

### 9.4 源 URL 只保留到上游创建请求结束

平台在向 Provider 发送 `CreateAsset` 前可以作用域加密保存源 URL。Provider 返回可信 Asset ID 后立即删除
平台保存的源 URL，不等待 `Active`。创建请求失败时同样立即删除。

后续视频只使用 Provider Asset ID。素材失效时不回退源 URL，不自动重建，不自动复制到其他区域。

### 9.5 素材创建不建立平台幂等系统

- 删除素材创建 `Idempotency-Key`、请求 HMAC 和幂等记录；
- 每次 `POST /v1/assets` 都表示新建一个素材；
- 每次 `POST /v1/asset-groups` 都表示新建一个素材组或认证邀请；
- 平台不自动重试；
- 客户在网络失败后重新提交可能创建新资源，与直接调用上游的行为一致。

### 9.6 按需查询，不建立复杂后台任务

- `POST /v1/assets` 调用 Provider `CreateAsset`，取得 Asset ID 后返回 `processing`；
- `GET /v1/assets/{id}` 调用 Provider `GetAsset` 刷新 `processing/active/failed`；
- `GET /v1/asset-groups/{id}` 按需查询认证或 Group 状态；
- 普通列表以本地数据库为主，不对每一项自动调用上游；
- 不建立持续轮询、自动重试、自动迁移和复杂 `AssetOperationJob`。

素材管理不使用 `create_unknown/delete_unknown`：

- `CreateAsset` 未取得可信 Provider Asset ID：客户请求失败，本地记录为 `failed`；
- 上游可能已创建孤儿素材的风险只记录到系统日志或使用日志，由技术人员分析；
- `DeleteAsset` 未取得明确成功：返回删除失败，平台素材保持原状态；
- 后续 `GET` 明确确认上游不存在时，再更新为 `deleted`；
- 不建立管理员核查页、对账状态机或自动孤儿扫描。

日志可以保留 channel、protocol、operation、Provider request ID 和脱敏错误，不得保留源 URL、凭据、完整
Provider 响应或人脸数据。

## 10. 素材与视频的确定性路由

### 10.1 不含平台素材

```text
前端模型 -> 唯一 Seedance 渠道 -> Provider
```

### 10.2 含 `asset://ast_*`

```text
前端模型 -> 唯一 Seedance 渠道
asset://ast_* -> user/app + active + 冻结渠道/账号/Project
两者必须相同
  -> 改写为 asset://<Provider Asset ID>
  -> 一次 Provider POST
```

不相同时返回：

```text
asset_channel_mismatch
```

多素材请求的任一账号、Region 或 Project 不同时返回：

```text
asset_scope_conflict
```

这些是资源所有权和 Provider 作用域校验，不是 SKU capability 或 Link 发布门禁。

## 11. 目标数据与状态边界

### 11.1 保留的主要事实

```text
Channel
  -> ChannelTypeSeedanceLink
  -> video_upstream_protocol
  -> asset_upstream_protocol
  -> model_mapping
  -> 按协议需要的凭据 / Region / Project

AssetGroup
  -> astgrp_*
  -> user_id + app_id
  -> channel_id + Provider 账号作用域
  -> Provider Group ID
  -> type + status + 短期查询句柄

Asset
  -> ast_*
  -> user_id + app_id
  -> channel_id + Provider 账号作用域
  -> Provider Asset ID / Group ID
  -> media type + status + moderation strategy

TaskCreateAttempt / Task
  -> customer model
  -> channel + adapter/protocol + Provider model
  -> 素材引用快照
  -> 计费快照
```

### 11.2 素材状态

保持能表达上游真实结果的最小集合：

```text
creating
processing
active
failed
deleting
deleted
```

Asset Group 根据协议可以使用：

```text
creating
verifying
active
failed
expired
deleting
deleted
```

不建立 `create_unknown/delete_unknown`。

## 12. 整体 Link 简化范围

本次决策不只是 Seedance 局部例外，而是 Link 整体收敛方向。当前 Link 主要是 Seedance 模型，
不需要为少量其他模型保留高成本的通用合同体系。

目标删除：

- `LinkModelPublication`；
- Link SKU；
- 逐模型 SKU capability；
- `LinkImplementation` ID/version/hash；
- execution binding；
- capability/content hash 等价证明；
- Link Access Plan；
- publication 改绑版本和审计工作流；
- 由 publication、implementation 和 Asset binding 共同计算的候选交集；
- `RealPersonAuthorization` 和相关 reservation/撤回体系；
- 通用 0..N AssetBinding 和自动迁移体系。

简化后的主链是：

```text
customer model
  -> Group / Ability / price
  -> 唯一 Channel
  -> model_mapping
  -> code-backed upstream_protocol
  -> Provider model
```

平台不再尝试从模型、Channel、profile、价格或素材字段中自动认证 Link 合同。管理配置是
当前路由事实，技术兼容性由线下评审保证。

## 13. 实施边界

### 13.1 新增代码优先隔离

建议将 Seedance 专属代码放在新增目录，具体名称按仓库约定确定：

```text
relay/channel/task/seedance/
  adaptor.go
  request.go
  response.go
  task.go
  error.go
  protocols/
    modelark_v3.go
    media_task_v1.go
    media_arrays_v2.go
    funcloud_v2.go
  assets/
    volcengine_action.go
    byteplus_action.go
    relay_assets.go
```

只在 NEWAPI 原生热路中保留必需的 ChannelType/adapter 接线、路由入口和管理保存校验。
不借机重构原生 Distributor、DoubaoVideo 或其他渠道。

### 13.2 官方国内素材不得假设兼容 BytePlus

当前 `official_action_assets` 实现的 Host 固定为 `byteplusapi.com`，真人认证也使用 BytePlus
`CreateVisualValidateSession/GetVisualValidateResult`。这不能证明已兼容国内火山。

实施国内 `volcengine_assets_action_v2024_01_01` 前必须取得并验证国内正式合同、Host、
凭据、权益、Project 和真人邀请流程，不允许仅替换 Base URL 或 Region 即声称兼容。

当前实现只允许真人图片，而官方指南描述了图片、视频和音频素材。目标 adapter 必须以已验证官方合同
为准，不把当前本地限制误写成官方能力。

## 14. 迁移步骤

1. 新增 `ChannelTypeSeedanceLink` 和独立管理入口。
2. 新增 ModelArk V3 北向四组路由和 Seedance 专属 adapter 选择。
3. 实现代码化 `video_upstream_protocol` 和 `asset_upstream_protocol`。
4. 将已经验证的火山、BytePlus、飞彩、FunCloud、TokenSave 线路分别配置为独立前端模型名。
5. 在渠道保存、修改已启用渠道和启用动作中阻止 Seedance 前端模型重名。
6. 在 Seedance 管理表单中置灰 Priority/Weight，根据协议按需展示官方素材字段。
7. 实现一对一 `AssetGroup` / `Asset` 代理，删除平台真人授权域和通用 0..N binding。
8. 将视频 Task 迁入“唯一渠道、一次 Provider POST”语义，保留视频 unknown 和计费事实。
9. 删除素材幂等、unknown、后台轮询、自动迁移和管理核查机制。
10. 将存量控制台素材改为管理员显式导入，不向普通客户暴露账号级 `ListAssets`。
11. 移除 Link publication、SKU、implementation、execution binding、Link Access Plan 和相关门禁。
12. 更新硬约束、`20-architecture/`、被取代 ADR、工程指南、运维手册、OpenAPI 和管理交互。
13. 完成真实 Provider、素材权益、三数据库、计费和灰度验证。

迁移时不原地把已有 `DoubaoVideo` 生产渠道改成新类型。必须新建独立渠道和前端模型名，
完成验证后再停用旧配置。存量 Task 继续使用创建时冻结的旧 adapter 和连接事实。

## 15. 验收矩阵

### 15.1 模型与渠道

- [ ] 管理员可以直接新建 `ChannelTypeSeedanceLink`，不需要理解 `DoubaoVideo` 或 Link Access Plan。
- [ ] 不同 Seedance 渠道使用不同前端模型名。
- [ ] 保存或启用已被其他已启用 Seedance 渠道使用的模型名时，返回普通人可理解的冲突提示。
- [ ] 停用渠道可保留重名，重新启用时必须解决。
- [ ] 请求时不重复执行模型重名检查。
- [ ] Priority/Weight 在 Seedance 表单中置灰并显示普通人可理解的无效提示。
- [ ] 视频、素材和素材组不使用 Priority/Weight、Affinity、失败重选或 fallback。

### 15.2 视频北向与任务

- [ ] ModelArk V3 create/get/list/delete 四组接口行为完整。
- [ ] `/api/v3` 只使用 `ChannelTypeSeedanceLink`，不选择普通 `DoubaoVideo`。
- [ ] `/v1/video/generations` 不选择 `ChannelTypeSeedanceLink`。
- [ ] 一个视频创建只发送一次 Provider POST，不自动重试或换渠道。
- [ ] 视频结果不明保持 `unknown`，不自动退款。
- [ ] 视频创建不要求平台自定义幂等键。
- [ ] list 在本地 `user_id + app_id` 内返回任务，delete 使用冻结 adapter。

### 15.3 素材库

- [ ] 官方国内、官方海外、第三方和无素材库协议只显示必要管理字段。
- [ ] 客户只能使用 `ast_*` / `asset://ast_*`，裸 Provider ID 必须先由管理员导入。
- [ ] 控制台存量素材不自动同步。
- [ ] Asset 和 Asset Group 固定到单一渠道、账号和 Project，不建立 0..N binding。
- [ ] 真人 Asset Group 直接返回官方或第三方认证链接，不经平台表单。
- [ ] 平台不保存人脸媒体、活体数据、身份证件或授权表单。
- [ ] `Moderation` 保持上游语义，不静默删除。
- [ ] `asset://ast_*` 可与 HTTP/Data URL 混用，但所有平台素材必须属于相同上游作用域。
- [ ] Provider 返回可信 Asset ID 或创建失败后立即删除平台保存的源 URL。
- [ ] 素材创建不使用幂等键、unknown、自动重试或后台核查机制。
- [ ] 素材删除不确定时返回失败并保持原状态，后续查询明确不存在后再更新。
- [ ] 密钥轮换不使既有素材失效，更换账号/Project/区域/协议必须新建渠道。

### 15.4 Link 收敛和工程验证

- [ ] publication、Link SKU、implementation、execution binding、Link Access Plan 和相关运行时门禁已移除。
- [ ] `RealPersonAuthorization` 和通用 Asset binding/迁移体系已移除。
- [ ] 本地扩展主体位于新增 Seedance 专属文件，NEWAPI 原生热路仅保留最小接线。
- [ ] SQLite、MySQL、PostgreSQL 相关迁移和事务通过。
- [ ] 前端类型检查、测试、构建和全语言 i18n 校验通过。
- [ ] `task docs:check` 与 `task ai:check` 通过。
- [ ] 公开 API 变更时 `cd web && bun run docs:validate` 通过。
- [ ] 至少完成一条国内官方、一条海外官方和一条第三方的真实视频与素材验证。

## 16. 当前事实文档的后续收敛

当前 `docs/00-context/硬约束.md`、多份 `20-architecture/` 和 ADR 仍记录 publication、Link SKU、
implementation、execution binding、通用 Asset binding 和平台真人授权体系。这些是当前代码事实，
本文不在尚未实施时将其改写为已完成状态。

实施完成后必须统一收敛至少以下文档：

- `docs/00-context/硬约束.md`；
- `docs/00-context/项目简报.md`；
- `docs/20-architecture/架构概览.md`；
- `docs/20-architecture/Seedance统一北向合同架构.md`；
- `docs/20-architecture/Link服务合同注册与履约架构.md`；
- `docs/20-architecture/Link资源合同与解析架构.md`；
- `docs/20-architecture/真人素材授权与撤回架构.md`；
- `docs/30-engineering/素材库对接指南.md`；
- `docs/40-operations/` 中的 Seedance 和素材运维手册；
- 相关 OpenAPI、管理端交互和已被取代 ADR。

99 归档目录不得回写。实施前应单独评审需要被 supersede 的 ADR 链，不在本开发现场文档中
直接修改已归档历史。

## 17. 不建立的复杂度清单

为防止实施期再次扩张，明确不做：

1. 不让管理员编写协议 JSON。
2. 不建立模型自动兼容性认证。
3. 不让一个前端 Seedance 模型对应多个渠道。
4. 不在请求时重复检查管理配置唯一性。
5. 不为 Seedance 使用 Priority/Weight/Affinity/fallback。
6. 不为视频或素材自动重试和切换渠道。
7. 不建立逐模型 SKU capability 和 publication 履约门禁。
8. 不建立平台人脸表单、授权证据或法律合规系统。
9. 不自动同步官方账号存量素材。
10. 不把一个素材绑定到多个 Provider 账号或多个区域。
11. 不在素材失效时回退源 URL。
12. 不建立素材 unknown 对账、孤儿扫描或管理员核查系统。
13. 不为无结果的极端数据库或并发情况增加运行时补丁。

