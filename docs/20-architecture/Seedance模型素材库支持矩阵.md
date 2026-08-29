---
status: current
owner: Dev Team
last-reviewed: 2026-08-29
---

# Seedance 模型素材库支持矩阵

本文是 Seedance 客户模型的**对外素材能力与调用合同**。它回答“某个客户模型能做什么”，不回答
Provider 如何签名、如何选路或如何保存平台内部快照。内部无状态代理边界见
[Seedance 无状态素材代理架构](Seedance无状态素材代理架构.md)，总体渠道关系见
[Seedance 专用渠道与 Link 架构](Seedance专用渠道与Link架构.md)。

## 1. 权威边界

客户模型由部署方命名；Provider、上游原始模型、Channel、Region/Project 和凭据不进入公开模型目录。
素材能力由当前客户模型对应 Channel 的已验证 `asset_upstream_protocol` 生成，不根据模型名、Provider
名称、URL 后缀、Key 或共享账号推断。

代码和公开字段的对应关系如下：

| 公开合同 | 代码权威 | 作用 |
| --- | --- | --- |
| `api.assets` 字段结构 | `relaykit/dto/public_model_api.go` 的 `PublicAssetAPI` | 固定公开元数据形状 |
| 每个模型的支持矩阵 | `model/channel_seedance_public_catalog.go` 的 `seedancePublicAssetAPI` | 按素材协议生成能力投影 |
| 单资源操作和错误 | `controller/asset.go`、素材 Service 和对应 adapter | 决定实际请求是否可执行 |

`GET /v1/models/{customer_model}` 返回的 `api.assets` 是调用方运行时唯一能力依据。代码已存在不等于
相应 Provider 已完成真实验收或已进入生产分组。

2026-08-29 已完成“统一北向协议优先 + Channel 默认普通 AIGC 素材组”的代码与公开元数据接线；真实
Provider 和生产灰度仍需逐线路验收。不得把代码合同写成全部线路已经生产开放。

## 2. 通用能力矩阵

| 通用类别 | `api.assets.supported` | 素材组 | 使用规则 |
| --- | --- | --- | --- |
| 固定分辨率系列（当前名称通常含 `720p`、`1080p`、`4k`） | `false` | 不适用 | 使用该模型允许的请求级 URL/Data URL；素材操作返回 422 |
| 已配置无素材组协议的模型 | `true` | 南向不使用 | 可直接创建普通素材；统一北向允许携带 `asset_group_id`，adapter 不发送该字段 |
| 已配置可选素材组协议的模型 | `true` | 普通素材北向可选；真人素材按 `media` 要求 | 调用方 ID 优先，省略或无效时使用 Channel 默认组 |
| 已配置南向必需素材组协议的模型 | `true` | 普通素材北向仍可选 | Channel 默认组满足南向必填，调用方无需先创建组 |
| 未配置素材协议的其它模型 | `false` | 不适用 | 不得因为名称不含固定分辨率就推断支持 |

固定分辨率系列当前不支持素材库，是接入文档、已注册协议和实际配置共同确认的事实：现有上游合同只有
视频创建/查询及计费，没有可验证的素材 CRUD、素材组或真人认证接口。只有取得正式接口证据并实现
adapter 后，矩阵才可以改变。

## 3. `api.assets` 公开字段

| 字段 | 含义 | 调用方规则 |
| --- | --- | --- |
| `supported` | 当前客户模型是否配置已验证素材协议 | `false` 时不得调用素材操作 |
| `management_mode` | 固定为 `caller_managed_stateless` | 调用方自行保存 Provider opaque ID；平台不提供素材目录 |
| `requires_model` | 固定为 `true` | 查询、删除和视频引用都必须保留客户模型名 |
| `reference_format` | 固定为 `asset://{opaque_upstream_asset_id}` | 只把 Provider 返回的 opaque 引用作为视频素材引用 |
| `reuse_scope` | 匿名素材复用域；不支持素材时省略或为空 | 仅非空且完全相同的 scope 表示“可以尝试复用” |
| `media` | 按 `kind` 与 `media_type` 描述素材组要求 | 普通 AIGC 在默认组落地后统一为北向可选；`real_person` 仍可为 `required`；`unsupported` 表示南向不使用，不表示统一北向字段非法 |
| `operations` | 每项操作的 HTTP 方法、路径和 `supported` | 不支持的操作必须停止，不得换 Provider 或 fallback |
| `creation` | URL TTL、长度、MIME、大小和重定向限制 | 只提交满足该模型返回限制的创建请求 |

相同非空 `reuse_scope` 表示模型位于同一个由管理员声明并由平台冻结的 Channel 素材边界；不同 Channel
不会发布相同 scope。它仍不是 Provider 真实租户、存在性、权限、所有权或兼容性证明，最终结果由选定
Provider 裁决。部署方可以为相同 scope 的客户模型使用相同业务后缀，但后缀不是运行时能力依据。

## 4. 操作矩阵与北向路径

平台只发布单资源操作，不发布客户素材列表：

| 操作 | 路径 | `model` 位置 | 说明 |
| --- | --- | --- | --- |
| 创建素材 | `POST /v1/assets` | JSON body | source URL 只用于本次调用，不持久化 |
| 查询素材 | `GET /v1/assets/{asset_id}?model=...` | query | `asset_id` 是 Provider opaque ID |
| 更新素材 | `PATCH /v1/assets/{asset_id}` | JSON body | 是否支持由 `operations` 决定 |
| 删除素材 | `DELETE /v1/assets/{asset_id}?model=...` | query | 只发送到该模型选定的 Provider |
| 创建素材组/认证会话 | `POST /v1/asset-groups` | JSON body | 真人流程由 `media` 和操作矩阵共同决定 |
| 查询素材组/认证会话 | `GET /v1/asset-groups/{id}?model=...` | query | 认证会话必须显式传 `verification_session=true` |

`GET /v1/assets`、`GET /v1/asset-groups` 和 `DELETE /v1/asset-groups/{group_id}` 均不注册，也不进入
公开操作元数据。平台不主动删除素材组；Provider adapter 即使内部需要分页查询，也不得把上游 List
发布为客户目录。

## 5. ID、引用与素材组规则

### 5.1 Channel 默认普通 AIGC 素材组

每个支持普通 AIGC 素材组的 Channel 可以保存一个固定名称为 `aigctokenaigeneral` 的内部默认 Provider
group ID。该名称是普通北向素材组创建的系统保留名称；默认组只由管理员在渠道页面手工创建或复用，
业务素材请求不得自动创建。

统一北向 `asset_group_id` 是合法可选字段：

- 策略为 `default_fallback` 且调用方提供裁剪后非空 ID 时保持原值并优先使用；
- 未传、空或空白时使用 Channel 默认组；
- 非空值违反已发布北向结构约束时返回 `invalid_request`，不得改用默认组；
- 默认组未配置时返回 `default_asset_group_not_configured`；
- Provider 拒绝显式 ID 后不回退默认组；
- 策略为 `none` 时忽略该字段并继续创建素材。

`GeneralAssetGroupPolicy` 是运行时和公开元数据的唯一权威；`GroupAdapter` 只决定管理端能否创建组，不能
作为运行时判据。当前全部已发布且能创建普通素材组的协议目标策略为 `default_fallback`，`none` 协议不
发送组字段。

这些规则仅针对普通 AIGC 素材。`real_person` 仍使用 Provider 认证流程生成的专用组 ID。当前代码已按
[实施方案](../80-dev/2026-08-28-Seedance渠道默认素材组实施方案.md)完成策略注册、元数据、Service 回退、
错误合同和管理端能力；真实 Provider 与部署灰度仍需单独验收。

### 5.2 Provider opaque ID 与引用

创建响应中的 `id` 和 `reference` 都由 Provider 返回并由 adapter 归一，二者可以不同：

```json
{
  "object": "asset",
  "id": "provider-opaque-resource-id",
  "model": "customer-model-name",
  "reference": "asset://provider-opaque-reference-id",
  "status": "ready"
}
```

调用方应保存 `model + id + reference`。没有可用引用时响应可以省略 `reference`，调用方先按 `model + id`
查询并保存最新结果。真人认证创建返回上游会话 ID；完成后单项查询返回实际 `group_id`，调用方再使用
该组 ID 创建真人素材。

视频请求中的 `asset://<opaque-id>` 只做非空字符串和请求结构校验，不查询本地数据库，不验证所有权、
应用、ready 状态、创建模型、Channel、账号或 Provider 作用域。引用原样进入当前客户模型的 video
adapter，由 Provider 最终判断存在性、权限、审核和模型兼容性。旧 `asset://ast_*`、`asset://pubref_*`
不是平台命名空间，也没有兼容 resolver。

## 6. 错误语义

| 情况 | HTTP/错误码 | 处理原则 |
| --- | --- | --- |
| 未配置素材协议或 adapter 未实现操作 | `422 unsupported_asset_operation` | 直接失败，不尝试其它 Provider |
| Provider 明确不存在 | `404 asset_not_found` | 只针对当前模型选定的 Provider |
| 请求字段、URL、TTL、MIME 或大小不合法 | `400 invalid_request`（或具体校验错误） | 预先满足 `creation` 约束 |
| Provider 业务拒绝 | 脱敏 `asset_upstream_error` | 不改判为平台“不支持” |
| Provider 暂时不可用 | `asset_upstream_unavailable` | 不切换 Channel、不 fallback |
| 普通素材需要默认组但 Channel 尚未配置 | `409 default_asset_group_not_configured` | 管理员先在渠道页面创建或复用默认组；业务请求不自动创建 |
| 普通调用方尝试创建保留名称 | `400 reserved_asset_group_name` | `aigctokenaigeneral` 只允许管理端默认组动作使用 |
| 客户直接请求素材组操作但南向无此能力 | `422 unsupported_asset_operation` | 沿用既有公开错误；素材创建本身仍按统一北向合同履约 |

平台不返回 Provider 名称、Channel ID、账号、Region/Project、协议细节、凭据或原始响应，也不记录
source URL、签名 URL、媒体二进制和原始 Provider 响应。共享 Provider 账号不提供跨应用的强所有权隔离；
需要该保证时必须使用独立账号/Project，或另行设计有状态所有权层。

## 7. 变更性质

| 变更面 | 性质 | 说明 |
| --- | --- | --- |
| `api.assets` 公开元数据和能力投影 | Link 新增能力 | 由 Seedance 专用 Channel 和代码协议注册生成，不改变 NEWAPI 原生模型目录语义 |
| `/v1/assets`、`/v1/asset-groups` 单资源代理 | Link 新增能力 | 平台不建立客户 Asset/AssetGroup、列表、resolver 或所有权事实；Channel 默认组只是一对一内部配置 |
| 原生鉴权、Token 模型白名单、错误响应和日志底座 | NEWAPI 原生能力复用 + 必要窄接线 | 只传递客户模型和请求事实，保持原生入口语义 |
| `asset://` 视频引用校验 | Link 专属窄接线 | 不修改 NEWAPI 原生视频入口，不把 Link 模型降级到原生渠道 |

## 8. 不承担的职责

本文不描述 Provider 签名、连接和 adapter 细节，不定义任务创建 attempt、Task 冻结、计费结算或迁移步骤。
这些内容分别由 [Seedance 无状态素材代理架构](Seedance无状态素材代理架构.md)、[Seedance 专用渠道与
Link 架构](Seedance专用渠道与Link架构.md)、[异步任务与计费事实架构](账单计费-异步任务与计费事实架构.md) 和
[ADR-0017：调用方自管无状态素材代理](decisions/0017-调用方自管无状态素材代理.md)、
[ADR-0018：Channel 默认素材组基础设施配置](decisions/0018-Channel默认素材组基础设施配置.md) 负责。
