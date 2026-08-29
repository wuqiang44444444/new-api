---
page-id: assets-lifecycle
kind: api-reference
last-verified: 2026-08-28
operations:
  - createAsset
  - getAsset
  - updateAsset
  - deleteAsset
  - createAssetGroup
  - getAssetGroup
---

# 素材与素材组

素材 API 是调用方自管的无状态单资源代理。应用必须保存创建响应中的客户 `model`、opaque `id` 和
`reference`；中转站不提供素材或素材组列表，也不保存 `Asset` / `AssetGroup` 映射。

所有操作使用 Bearer 鉴权。路径中的素材 ID 和素材组 ID 都是上游返回的不透明字符串，客户端不能解析、
改写或根据前缀判断来源。

> **AIGC 普通素材的推荐流程：直接创建素材，不要默认先创建素材组。** 调用
> `POST /v1/assets` 时优先省略 `asset_group_id`（发送空字符串也按未填写处理），这样不同模型可以保持
> 一致的接入流程。普通素材由所选 Channel 的系统默认组完成南向履约；只有业务确实需要自定义分组管理时，
> 才先调用素材组接口。平台继续保留素材组创建与单项查询能力。

## 调用前读取模型能力

先调用 `GET /v1/models/{customer_model}` 并读取 `api.assets`：

| 字段 | 用途 |
| --- | --- |
| `supported` | `false` 表示该客户模型不能使用素材 API |
| `operations[]` | 每个创建、查询、更新、删除、素材组和认证操作是否支持，以及对应方法与路径 |
| `media[]` | 支持的 `asset_kind`、`media_type` 组合及 `asset_group_requirement` |
| `creation.required_fields` | 当前模型创建素材时必须提交的字段 |
| `creation.name_max_characters` | `name` 最大字符数，当前公共上限为 `64` |
| `creation.source` | URL 协议、端口、最大长度、最短剩余有效期、MIME、大小和重定向限制 |
| `reuse_scope` | 匿名素材复用域；仅两个非空值完全相同时才可尝试跨模型复用 |

`reuse_scope` 相同不证明素材所有权、ready 状态或永久兼容，只表示可以把同一个 opaque ID 交给另一个
模型尝试。不同或缺失时不得跨模型发送素材 ID。

### 模型广场的“素材共享组”提示

控制台模型广场的模型卡片会显示“素材共享组”短标签，它是 `reuse_scope` 的界面缩写，用于在调用前
快速判断素材库归属：

- 卡片显示标签（例如 `素材共享组 4AFE`）：该模型已发布素材库，且标签相同的模型位于同一个素材
  复用域。可以把其中一个模型创建的素材 `asset://` 引用交给同组其它模型尝试使用；悬停标签可直接
  查看同组模型清单。
- 卡片没有标签：该模型未发布素材库（`api.assets.supported` 不为 `true` 或模型当前不可用），不能
  调用素材 API，也没有可比较的复用域；不得把其它模型的素材引用交给它。
- 标签文本（如 `4AFE`）是匿名复用域的短哈希，只用于界面区分，不代表上游身份或渠道信息。
- 相同标签只表示“可以尝试复用”，不保证素材一定被接受；最终存在性、权限、状态和兼容性仍由当前
  模型选定的上游判断，规则与逐模型比较 `reuse_scope` 完全一致。

## 创建素材

`POST /v1/assets` · `application/json`

```bash
curl "{{OPENAI_BASE_URL}}/assets" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "品牌片头",
    "asset_kind": "general",
    "media_type": "video",
    "model": "{{MODEL_ID_PLACEHOLDER}}",
    "source": {
      "type": "url",
      "url": "https://example.com/brand-intro.mp4",
      "expires_at": 1800000000
    }
  }'
```

### 创建请求参数

| 字段 | 类型 | 必填 | 取值与说明 |
| --- | --- | --- | --- |
| `name` | string | 是 | 素材名称；去除首尾空白后不能为空，最多 `64` 个字符 |
| `asset_kind` | string | 是 | `general` 或 `real_person` |
| `media_type` | string | 是 | `image`、`video` 或 `audio`；必须出现在当前模型的 `api.assets.media` 中 |
| `model` | string | 是 | 客户模型名；用于选择唯一素材执行路径，必须使用模型目录中的原值 |
| `asset_group_id` | string | 条件使用 | AIGC 普通素材优先省略、传空字符串或空白，由网关使用 Channel 默认组；裁剪后非空时原值交给 Provider。`real_person` 必须传认证产生的专用组 ID |
| `source` | object | 是 | 本次创建使用的源对象 |
| `source.type` | string | 是 | 当前固定为 `url` |
| `source.url` | string | 是 | 上游可访问的公网 HTTPS 绝对 URL |
| `source.expires_at` | integer | 否 | URL 过期时间，Unix 秒；填写时必须满足模型公开的最短剩余有效期 |

`source.url` 还必须满足以下公共边界：

- 只能使用 HTTPS 和 443 端口，不能包含 URL 用户名、密码或 fragment；
- 域名或 IP 必须通过公网与 SSRF 安全校验；
- URL 长度不能超过 `creation.source.max_url_length`；
- URL 只用于这一次调用，不会被持久化、写入普通日志或返回给客户端；
- 如模型目录公开 MIME、文件大小或重定向限制，源文件也必须满足这些限制。

### 创建成功响应

HTTP `201`：

```json
{
  "object": "asset",
  "id": "provider-resource-id",
  "model": "customer-model-name",
  "reference": "asset://provider-reference-id",
  "status": "ready"
}
```

| 字段 | 类型 | 是否总是存在 | 说明 |
| --- | --- | --- | --- |
| `object` | string | 是 | 固定为 `asset` |
| `id` | string | 是 | 上游素材资源 opaque ID；用于查询、更新和删除 |
| `model` | string | 是 | 本次请求使用的客户模型名 |
| `reference` | string | 否 | 可用于视频生成的 `asset://<opaque-id>` 引用；未 ready 时可能暂不返回 |
| `status` | string | 是 | `processing`、`ready` 或 `failed` |
| `error_code` | string | 否 | 素材失败时的脱敏错误码，当前公开值为 `upstream_asset_failed` |
| `error` | string | 否 | 脱敏错误说明；不会返回原始上游错误 |

`id` 与 `reference` 中的 opaque ID 可能不同，必须分别保存。`processing` 时使用 `model + id` 查询；
不要自行拼接 `asset://` 代替尚未返回的 `reference`。

## 查询素材

`GET /v1/assets/{asset_id}?model={customer_model}`

```bash
curl "{{OPENAI_BASE_URL}}/assets/provider-resource-id?model={{MODEL_ID_PLACEHOLDER}}" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}"
```

| 参数 | 位置 | 必填 | 说明 |
| --- | --- | --- | --- |
| `asset_id` | path | 是 | 创建响应的 `id`，原样 URL 编码后放入路径 |
| `model` | query | 是 | 创建该素材时保存的客户模型名 |

HTTP `200` 返回与创建相同的 `Asset` 对象。查询只请求当前客户模型选定的上游，不会探测 ID 的真实
来源，也不会换模型或 fallback。

## 更新素材名称

`PATCH /v1/assets/{asset_id}` · `application/json`

```bash
curl -X PATCH "{{OPENAI_BASE_URL}}/assets/provider-resource-id" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "{{MODEL_ID_PLACEHOLDER}}",
    "name": "品牌片头最终版"
  }'
```

| 参数 | 位置 | 必填 | 说明 |
| --- | --- | --- | --- |
| `asset_id` | path | 是 | 要更新的素材 opaque ID |
| `model` | body | 是 | 创建时使用的客户模型名 |
| `name` | body | 是 | 新名称；去除首尾空白后不能为空，最多 `64` 个字符 |

HTTP `200` 返回更新后的 `Asset` 对象。只有 `operations` 中 `update_asset.supported=true` 时才能调用；
不支持更新的模型返回 `422 unsupported_asset_operation`。

## 删除素材

`DELETE /v1/assets/{asset_id}?model={customer_model}`

```bash
curl -X DELETE "{{OPENAI_BASE_URL}}/assets/provider-resource-id?model={{MODEL_ID_PLACEHOLDER}}" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}"
```

成功返回 HTTP `204 No Content`，没有 JSON 响应体。删除失败不会建立本地 `unknown` 状态、自动重试、
孤儿扫描或跨上游清理任务；结果不明确时调用方应停止自动重发并保留公开请求 ID。

## 可选：创建普通素材组

AIGC 普通素材无需为了完成基础创建而预先建组。只有模型能力明确要求素材组，或调用方需要使用上游的
分组语义时，才执行本节；不需要分组时直接跳过，按前文创建素材即可。

`POST /v1/asset-groups` · `application/json`

```bash
curl "{{OPENAI_BASE_URL}}/asset-groups" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "产品素材",
    "description": "用于产品演示视频",
    "group_kind": "general",
    "model": "{{MODEL_ID_PLACEHOLDER}}"
  }'
```

### 素材组请求参数

| 字段 | 类型 | 必填 | 取值与说明 |
| --- | --- | --- | --- |
| `name` | string | 是 | 素材组名称；最多 `64` 个字符；`aigctokenaigeneral` 为系统保留名称，调用方不得创建 |
| `description` | string | 否 | 素材组说明；最多 `300` 个字符 |
| `group_kind` | string | 是 | 普通组使用 `general`；真人认证流程使用 `real_person` |
| `model` | string | 是 | 客户模型名 |
| `redirect_url` | string | 条件使用 | 真人认证完成后的客户端 HTTPS 跳转地址；普通组通常省略 |

### 普通素材组响应

HTTP `201`：

```json
{
  "object": "asset_group",
  "id": "provider-group-id",
  "model": "customer-model-name",
  "status": "ready"
}
```

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `object` | string | 普通组固定为 `asset_group` |
| `id` | string | 上游素材组 opaque ID；业务需要自定义分组时可把它放入普通素材的 `asset_group_id` |
| `model` | string | 创建素材组时使用的客户模型名 |
| `status` | string | `processing`、`ready` 或 `failed` |

## 创建真人认证会话

真人认证仍调用同一个 `POST /v1/asset-groups`，但将 `group_kind` 设为 `real_person`：

```json
{
  "name": "演员认证",
  "group_kind": "real_person",
  "model": "customer-model-name",
  "redirect_url": "https://client.example.com/verified"
}
```

HTTP `201` 示例：

```json
{
  "object": "asset_group_verification",
  "id": "provider-session-id",
  "model": "customer-model-name",
  "status": "processing",
  "verification_url": "https://verification.example.com/session-placeholder",
  "expires_at": 1800000000
}
```

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `object` | string | 真人流程固定为 `asset_group_verification` |
| `id` | string | 此阶段是上游认证会话 ID，不是最终素材组 ID |
| `model` | string | 请求使用的客户模型名 |
| `group_id` | string | 上游已同步返回实际素材组时可能存在；通常在认证完成查询后出现 |
| `status` | string | `processing`、`ready` 或 `failed` |
| `verification_url` | string | 交给真人本人打开的上游 HTTPS 认证地址；只在有效期内使用 |
| `expires_at` | integer | 认证会话或地址过期时间，Unix 秒 |

中转站不采集人脸、证件、活体或授权表单。调用方只能把 `verification_url` 交给本人，并自行保存
`model + session id` 以便查询。

## 查询素材组或认证结果

普通素材组：

```text
GET /v1/asset-groups/{group_id}?model={customer_model}
```

真人认证会话：

```text
GET /v1/asset-groups/{session_id}?model={customer_model}&verification_session=true
```

| 参数 | 位置 | 必填 | 说明 |
| --- | --- | --- | --- |
| `group_id` / `session_id` | path | 是 | 普通组 ID 或认证会话 ID |
| `model` | query | 是 | 创建组或会话时使用的客户模型名 |
| `verification_session` | query | 真人查询是 | 传 `true` 表示按认证会话查询；省略表示查询普通素材组 |

HTTP `200` 返回 `AssetGroup` 对象。真人认证完成后，响应中的 `group_id` 才是创建真人素材时应填写的
`asset_group_id`。不要把会话 `id` 当作最终素材组 ID。

平台没有注册 `GET /v1/assets`、`GET /v1/asset-groups` 列表，也没有素材组更新或删除接口。素材组可能
包含多项资源并产生级联影响，确需清理由上游管理员确认归属后执行。

## 在视频请求中引用素材

素材 ready 且返回 `reference` 后，可把该值放入 ModelArk V3 对应媒体项：

```json
{
  "type": "image_url",
  "image_url": {"url": "asset://provider-reference-id"},
  "role": "reference_image"
}
```

平台不会在视频生成前查询素材，也不会校验所有权、ready 状态、创建模型或上游作用域。最终存在性、
权限、兼容性和内容审核由当前客户模型选定的上游判断；素材单项查询成功也不保证视频一定生成成功。

## 状态处理

| 状态 | 客户端行为 |
| --- | --- |
| `processing` | 保存 `model + id`，有界轮询对应单项查询接口 |
| `ready` | 保存 `reference`；需要跨模型复用时仍先比较非空 `reuse_scope` |
| `failed` | 停止轮询，记录公开 `error_code` 和请求 ID，不解析原始上游身份 |

素材 POST 不支持客户幂等键。超时不代表创建一定失败，自动重复 POST 可能产生多个上游素材；不要自动
换模型、换路径或 fallback。

## 错误响应

素材错误使用以下信封：

```json
{
  "error": {
    "message": "asset operation is not supported by this model",
    "type": "asset_error",
    "code": "unsupported_asset_operation",
    "request_id": "req-placeholder"
  }
}
```

| HTTP 状态 | `error.code` | 含义与处理 |
| --- | --- | --- |
| `400` | `invalid_request` | JSON、必填字段、名称、URL、有效期或参数组合无效；修正后再请求 |
| `400` | `reserved_asset_group_name` | 普通调用方试图创建系统保留素材组名称；改用其它业务名称 |
| `400` | `asset_url_ttl_insufficient` | URL 剩余有效期不足；读取 `error.details.required_min_ttl_seconds` 后换用更长有效期 URL |
| `404` | `model_not_found` | 客户模型不存在；重新读取模型目录 |
| `404` | `asset_not_found` | 当前模型选定的上游未找到该素材或素材组 |
| `409` | `default_asset_group_not_configured` | 所选 Channel 尚未配置系统默认组；由管理员在渠道编辑页创建或复用后重试 |
| `422` | `unsupported_asset_type` | 当前模型不支持该 `asset_kind + media_type` 组合 |
| `422` | `unsupported_asset_operation` | 当前模型未发布该素材或素材组操作 |
| `502` | `asset_upstream_error` | 上游拒绝或返回无效结果；不要改成其它 Provider ID 探测 |
| `503` | `asset_upstream_unavailable` | 素材服务或凭据暂不可用；保留请求 ID 后联系管理员 |
| `500` | `internal_error` | 平台内部错误；不要依赖错误文本判断素材是否已创建 |
