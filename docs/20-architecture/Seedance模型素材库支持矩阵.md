---
status: current
owner: Dev Team
last-reviewed: 2026-11-13
---

# Seedance 模型素材库支持矩阵

本文是 Seedance 客户模型的**对外素材能力与调用合同**。它回答“某个客户模型能做什么”，不回答
Provider 如何签名、如何选路或如何保存平台内部快照。内部无状态代理边界见
[Seedance 无状态素材代理架构](Seedance无状态素材代理架构.md)，总体渠道关系见
[Seedance 专用渠道与 Link 架构](Seedance专用渠道与Link架构.md)。

## 1. 权威边界

客户模型由部署方命名并用于授权。本站托管素材库按账号共享，Seedance 与 MiniMax 标准视频均可使用。
素材库统一接受图片、MP3 与 MP4，媒体类型及 MIME 由 `relaykit/dto/seedance_hosted_media.go` 定义。
目标模型的媒体、角色、数量和组合由 `api.video.creation.content_types` 和该模型视频合同决定。

## 2. 素材合同选择

| 模型配置 | `api.assets` | `api.hosted_assets` |
| --- | --- | --- |
| 配置了上游素材代理 | 默认代理合同 | 账号级托管合同 |
| 无上游素材代理，或已有本站托管配置 | 账号级托管合同 | 省略 |

在创建素材或普通素材组时，传 `management_mode=platform_hosted` 可显式使用本站托管。读取与删除按
资源 ID 进入对应库。代理继续按已配置协议和素材租户边界运行。部署上线按真实供应商和账单验收。

## 3. `api.assets` 公开字段

| 字段 | 含义 | 调用方规则 |
| --- | --- | --- |
| `supported` | 当前客户模型是否配置已验证素材协议 | `false` 时不得调用素材操作 |
| `management_mode` | 无状态代理固定为 `caller_managed_stateless`；本站托管协议固定为 `platform_hosted` | 调用方自行保存 Provider opaque ID；托管素材由平台持久化并按用户共享 |
| `requires_model` | 固定为 `true` | 查询、删除和视频引用都必须保留客户模型名 |
| `reference_format` | 固定为 `asset://{opaque_upstream_asset_id}` | 只把 Provider 返回的 opaque 引用作为视频素材引用；托管协议返回平台 `fhas_*` ID |
| `reuse_scope` | 匿名素材复用域；不支持素材时省略或为空 | 仅非空且完全相同的 scope 表示“可以尝试复用”；托管协议发布固定托管复用域，不随上游账号轮换失效 |
| `media` | 按 `kind` 与 `media_type` 描述素材组要求 | 普通 AIGC 在默认组落地后统一为北向可选；`real_person` 仍可为 `required`；`unsupported` 表示南向不使用，不表示统一北向字段非法 |
| `operations` | 每项操作的 HTTP 方法、路径和 `supported` | 不支持的操作必须停止，不得换 Provider 或 fallback |
| `creation` | URL TTL、长度、MIME、大小和重定向限制 | 只提交满足该模型返回限制的创建请求 |

普通代理的相同非空 `reuse_scope` 表示模型位于同一冻结素材租户边界。本站托管使用固定复用域，素材由
账号共享。用于生成时，目标模型须支持对应的媒体输入。

## 4. 操作矩阵与北向路径

平台只发布单资源操作，不发布客户素材列表：

| 操作 | 路径 | `model` 位置 | 说明 |
| --- | --- | --- | --- |
| 创建素材 | `POST /v1/assets` | JSON body | source URL 只用于本次调用，不持久化 |
| 查询素材 | `GET /v1/assets/{asset_id}?model=...` | query | 使用创建响应返回的不透明素材 ID |
| 更新素材 | `PATCH /v1/assets/{asset_id}` | JSON body | 是否支持由 `operations` 决定 |
| 删除素材 | `DELETE /v1/assets/{asset_id}?model=...` | query | 代理模式调用选定的 Provider，托管模式停止新引用 |
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
作为运行时判据。已登记的远端普通组协议可使用 `default_fallback`；`none` 不发送组字段，`hosted` 按用户解析托管组。

这些规则仅针对普通 AIGC 素材。`real_person` 仍使用 Provider 认证流程生成的专用组 ID。管理端创建和上线步骤见[视频与素材渠道运维手册](../40-operations/02-视频与素材渠道运维手册.md)。

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

唯一例外是托管协议模型中的 `asset://fhas_*`：该前缀是平台托管素材命名空间，平台在资金 hold 之前
按 `user_id` 校验归属与可引用状态，发送时把引用替换为内部签发的临时 URL；跨用户、已删除或不存在的
托管引用明确拒绝，不回退为上游引用。托管素材删除只停止新引用，已受理任务继续。

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


### 托管素材的可用性边界

`platform_hosted` 创建校验媒体格式和 MIME，图片另外验证解码内容与像素数。所有托管类型的编码文件上限为
100 MiB，图片的 `max_pixels` 为 13,107,200。查询时已知对象缺失、存储不可用或位置配置不一致不会返回
虚假的 ready。托管引用必须匹配媒体类型和对应槽位；不符合条件返回 400，存储不可用返回 503，均在资金
hold 与 Provider POST 前拒绝。直接 URL 路径不建立素材或访问本站存储。
