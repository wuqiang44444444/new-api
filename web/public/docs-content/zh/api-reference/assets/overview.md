---
page-id: assets-lifecycle
kind: api-reference
last-verified: 2026-11-13
operations:
  - createAsset
  - getAsset
  - updateAsset
  - deleteAsset
  - createAssetGroup
  - getAssetGroup
---

# 素材与素材组

托管素材库按账号共享，统一接收图片、MP3 和 MP4。创建时填入当前 API Key 有权使用的客户模型 ID
作为 `model`。托管入库按同一套文件规则校验，与所填模型无关。同账号下，不同 API Key 和新加入的
Seedance 模型都可复用已有托管素材，无需重新上传。

提交视频生成时，才按目标模型校验媒体类型、角色、数量、组合和规格。例如，MP3 可以先入库，
但只能在支持参考音频的模型请求中使用，并须满足该模型的组合规则。

已配置上游素材代理的模型仍可使用 `caller_managed_stateless`。创建本站托管素材或托管组时，
在 JSON 中传 `"management_mode":"platform_hosted"`。省略该字段时，沿用模型配置的代理；没有配置代理时
使用本站托管库。两种模式均提供单资源操作。

所有操作使用 Bearer 鉴权。路径中的素材 ID 和素材组 ID 都是服务端返回的不透明字符串，客户端不能解析、
改写或根据前缀判断来源。素材用于视频参考，不是图片生成或图片编辑的任务结果。

创建普通素材时可省略 `asset_group_id`，空字符串也按省略处理。托管素材默认不分组，代理素材按所选
合同使用默认组或忽略组。需要指定分组时，先取得可用的素材组 ID。

## 调用前读取模型能力

调用 `GET /v1/models/{customer_model}`。默认素材合同在 `api.assets`，同时配置了代理的模型在
`api.hosted_assets` 另列本站托管合同。两处使用相同结构。

| 字段                           | 用途                                                                                        |
| ------------------------------ | ------------------------------------------------------------------------------------------- |
| `management_mode`              | `caller_managed_stateless` 为代理模式，`platform_hosted` 为平台托管模式                     |
| `supported`                    | `false` 表示该素材合同不可用                                                      |
| `operations[]`                 | 每个创建、查询、更新、删除、素材组和认证操作是否支持，以及对应方法与路径                    |
| `media[]`                      | 支持的 `kind`、`media_type` 组合，对应创建请求的 `asset_kind`、`media_type` 及 `asset_group_requirement`                          |
| `creation.required_fields`     | 所选素材合同要求的创建字段                                                            |
| `creation.name_max_characters` | `name` 最大字符数，当前公共上限为 `64`                                                      |
| `creation.source.accepted_content_types` | 按媒体类型列出平台校验的源文件 MIME，字段存在时按该清单提交 |
| `creation.source`              | URL 协议、端口、最大长度、最短剩余有效期、MIME、编码大小、`max_pixels` 像素上限和重定向限制 |
| `reuse_scope`                  | 代理素材复用域；两个非空值完全相同时才可尝试跨模型复用                                    |

`asset_group_requirement=optional` 表示可省略组；`required` 表示该素材类型必须提供专用组；
`unsupported` 表示普通组不参与该模型履约，省略组即可创建，不能把这个值理解成素材创建也不支持。

## 平台托管素材

选择 `platform_hosted` 时，可直接创建素材，再使用返回的 `reference` 生成视频。
如需分组，先创建普通素材组，并在创建素材时传入当前账号下的组 ID。同一账号的 API Key 共享素材与素材组。

- 托管库统一接受 JPEG、PNG、WebP、BMP、TIFF、GIF、MP3 和 MP4。源地址须为当次可读的公开 HTTPS URL，
  可以带临时签名。创建返回 `ready` 后，源 URL 过期不影响托管素材。
- 创建校验媒体格式和 MIME，图片另验证解码内容。编码文件最大 100 MiB，图片最大 13,107,200 像素。
  具体限制以 `creation.source` 为准。格式或 MIME 校验失败时返回 `400 invalid_request`。
  当前素材合同不支持的 `asset_kind` 与 `media_type` 组合返回 `422 unsupported_asset_type`。
- 图片、音频和视频的 reference 分别放入 `image_url.url`、`audio_url.url` 和 `video_url.url`。无效、
  已删除、跨账号或类型与槽位不匹配的托管引用在预扣前返回 `400 invalid_video_parameter`。
  托管存储、对象或签名不可用时，视频创建返回 `503 internal_error`，此时尚未预扣。
- 删除后不能在新任务中使用该素材。已受理的任务可继续执行。
- 生成请求可按目标模型支持的组合混用直接 URL 和托管引用。

## 创建素材

`POST /v1/assets` · `application/json`

```bash
curl "{{OPENAI_BASE_URL}}/assets" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "商品参考图",
    "management_mode": "platform_hosted",
    "asset_kind": "general",
    "media_type": "image",
    "model": "{{MODEL_ID_PLACEHOLDER}}",
    "source": {
      "type": "url",
      "url": "https://example.com/product.png"
    }
  }'
```

创建托管音频或视频时，将示例中的 `media_type` 分别改为 `audio` 或 `video`，并换成对应的 MP3 或 MP4 源地址。

### 创建请求参数

| 字段                | 类型    | 必填     | 取值与说明                                                                                                                                                             |
| ------------------- | ------- | -------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `name`              | string  | 是       | 素材名称；去除首尾空白后不能为空，最多 `64` 个字符                                                                                                                     |
| `asset_kind`        | string  | 是       | `general` 或 `real_person`                                                                                                                                             |
| `management_mode`   | string  | 否       | `platform_hosted` 显式选择本站托管。省略时使用模型的默认素材合同 |
| `media_type`        | string  | 是       | `image`、`video` 或 `audio`，读取所选素材合同的 `media`                                                                                                |
| `model`             | string  | 是       | 模型目录中的客户模型名，用于访问校验。默认代理模式也用它选择素材服务                                                                                                         |
| `asset_group_id`    | string  | 条件使用 | AIGC 普通素材可省略；代理模式按模型策略使用默认组，裁剪后非空时原值交给上游；托管模式未填写时不分组，填写时校验当前账号组归属。`real_person` 必须传认证产生的专用组 ID |
| `source`            | object  | 是       | 本次创建使用的源对象                                                                                                                                                   |
| `source.type`       | string  | 是       | 当前固定为 `url`                                                                                                                                                       |
| `source.url`        | string  | 是       | 当次可访问的公网 HTTPS 绝对 URL。托管模式由平台读取，代理模式按素材服务要求提供                                                                                                                                        |
| `source.expires_at` | integer | 否       | URL 过期时间，Unix 秒。代理模式填写时须满足模型公开的最短剩余有效期；托管模式要求源地址在复制时有效                                                                                                          |

`source.url` 还必须满足以下公共边界：

- 只能使用 HTTPS 和 443 端口，不能包含 URL 用户名、密码或 fragment；
- 域名或 IP 必须能从公网访问；
- URL 长度不能超过 `creation.source.max_url_length`；
- 托管素材遵循上文的统一入库规则。代理素材按所选素材合同的 `creation.source` 提交。

### 创建成功响应

HTTP `201`：

```json
{
  "object": "asset",
  "id": "example-asset-id",
  "model": "customer-model-name",
  "reference": "asset://example-reference-id",
  "status": "ready"
}
```

| 字段         | 类型   | 是否总是存在 | 说明                                                                 |
| ------------ | ------ | ------------ | -------------------------------------------------------------------- |
| `object`     | string | 是           | 固定为 `asset`                                                       |
| `id`         | string | 是           | 素材资源的不透明 ID；用于查询、更新和删除                            |
| `model`      | string | 是           | 本次请求使用的客户模型名                                             |
| `reference`  | string | 否           | 可用于视频生成的 `asset://<opaque-id>` 引用；未 ready 时可能暂不返回 |
| `status`     | string | 是           | `processing`、`ready` 或 `failed`                                    |
| `error_code` | string | 否           | 素材失败时的脱敏错误码，当前公开值为 `upstream_asset_failed`         |
| `error`      | string | 否           | 脱敏错误说明；不会返回原始上游错误                                   |

`id` 与 `reference` 中的 opaque ID 可能不同，必须分别保存。`processing` 时使用 `model + id` 查询；
不要自行拼接 `asset://` 代替尚未返回的 `reference`。

## 查询素材

`GET /v1/assets/{asset_id}?model={customer_model}`

```bash
curl "{{OPENAI_BASE_URL}}/assets/example-asset-id?model={{MODEL_ID_PLACEHOLDER}}" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}"
```

| 参数       | 位置  | 必填 | 说明                                     |
| ---------- | ----- | ---- | ---------------------------------------- |
| `asset_id` | path  | 是   | 创建响应的 `id`，原样 URL 编码后放入路径 |
| `model`    | query | 是   | 托管素材使用当前 Key 可访问的模型 ID；代理素材使用创建时保存的模型 ID |

HTTP `200` 返回与创建相同的 `Asset` 对象。

## 更新素材名称

`PATCH /v1/assets/{asset_id}` · `application/json`

```bash
curl -X PATCH "{{OPENAI_BASE_URL}}/assets/example-asset-id" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "{{MODEL_ID_PLACEHOLDER}}",
    "name": "品牌片头最终版"
  }'
```

| 参数       | 位置 | 必填 | 说明                                             |
| ---------- | ---- | ---- | ------------------------------------------------ |
| `asset_id` | path | 是   | 要更新的素材 opaque ID                           |
| `model`    | body | 是   | 创建时使用的客户模型名                           |
| `name`     | body | 是   | 新名称；去除首尾空白后不能为空，最多 `64` 个字符 |

此示例适用于 `update_asset.supported=true` 的代理素材合同。成功返回 HTTP `200` 和更新后的素材；
托管素材改名返回 `422 unsupported_asset_operation`。

## 删除素材

`DELETE /v1/assets/{asset_id}?model={customer_model}`

```bash
curl -X DELETE "{{OPENAI_BASE_URL}}/assets/example-asset-id?model={{MODEL_ID_PLACEHOLDER}}" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}"
```

成功返回 HTTP `204 No Content`。删除结果不明确时，保留请求 ID 核查，暂停自动重试。

## 可选：创建普通素材组

需要自定义分组时，先创建组，再将返回的 ID 作为 `asset_group_id` 提交。

`POST /v1/asset-groups` · `application/json`

```bash
curl "{{OPENAI_BASE_URL}}/asset-groups" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "产品素材",
    "description": "用于产品演示视频",
    "group_kind": "general",
    "management_mode": "platform_hosted",
    "model": "{{MODEL_ID_PLACEHOLDER}}"
  }'
```

### 素材组请求参数

| 字段           | 类型   | 必填     | 取值与说明                                                                        |
| -------------- | ------ | -------- | --------------------------------------------------------------------------------- |
| `name`         | string | 是       | 素材组名称；最多 `64` 个字符；`aigctokenaigeneral` 为系统保留名称，调用方不得创建 |
| `description`  | string | 否       | 素材组说明；最多 `300` 个字符                                                     |
| `management_mode` | string | 否 | 普通组可传 `platform_hosted` 以显式选择托管库 |
| `group_kind`   | string | 是       | 普通组使用 `general`；真人认证流程使用 `real_person`                              |
| `model`        | string | 是       | 客户模型名                                                                        |
| `redirect_url` | string | 条件使用 | 真人认证完成后的客户端 HTTPS 跳转地址；普通组通常省略                             |

### 普通素材组响应

HTTP `201`：

```json
{
  "object": "asset_group",
  "id": "example-group-id",
  "model": "customer-model-name",
  "status": "ready"
}
```

| 字段     | 类型   | 说明                                                                         |
| -------- | ------ | ---------------------------------------------------------------------------- |
| `object` | string | 普通组固定为 `asset_group`                                                   |
| `id`     | string | 素材组的不透明 ID；业务需要自定义分组时可把它放入普通素材的 `asset_group_id` |
| `model`  | string | 创建素材组时使用的客户模型名                                                 |
| `status` | string | `processing`、`ready` 或 `failed`                                            |

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
  "id": "example-session-id",
  "model": "customer-model-name",
  "status": "processing",
  "verification_url": "https://verification.example.com/session-placeholder",
  "expires_at": 1800000000
}
```

| 字段               | 类型    | 说明                                                         |
| ------------------ | ------- | ------------------------------------------------------------ |
| `object`           | string  | 真人流程固定为 `asset_group_verification`                    |
| `id`               | string  | 此阶段是上游认证会话 ID，不是最终素材组 ID                   |
| `model`            | string  | 请求使用的客户模型名                                         |
| `group_id`         | string  | 上游已同步返回实际素材组时可能存在；通常在认证完成查询后出现 |
| `status`           | string  | `processing`、`ready` 或 `failed`                            |
| `verification_url` | string  | 交给真人本人打开的上游 HTTPS 认证地址；只在有效期内使用      |
| `expires_at`       | integer | 认证会话或地址过期时间，Unix 秒                              |

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

| 参数                      | 位置  | 必填       | 说明                                                 |
| ------------------------- | ----- | ---------- | ---------------------------------------------------- |
| `group_id` / `session_id` | path  | 是         | 普通组 ID 或认证会话 ID                              |
| `model`                   | query | 是         | 创建组或会话时使用的客户模型名                       |
| `verification_session`    | query | 真人查询是 | 传 `true` 表示按认证会话查询；省略表示查询普通素材组 |

HTTP `200` 返回 `AssetGroup` 对象。真人认证完成后，响应中的 `group_id` 才是创建真人素材时应填写的
`asset_group_id`。不要把会话 `id` 当作最终素材组 ID。

需要创建素材时，将组创建响应的 `id` 放入素材请求的 `asset_group_id`，其余字段与直接创建图片一致。
普通组的单项查询示例：

```bash
curl --get "{{OPENAI_BASE_URL}}/asset-groups/example-group-id" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  --data-urlencode "model={{MODEL_ID_PLACEHOLDER}}"
```

查询或删除素材时，`model` 同样应进行 URL 查询参数编码；可以用 `--get --data-urlencode` 构造查询。

素材组支持创建和单项查询。素材与素材组均无列表接口。

## 在视频请求中引用素材

素材 ready 且返回 `reference` 后，可把该值放入 ModelArk V3 对应媒体项：

```json
{
  "type": "image_url",
  "image_url": { "url": "asset://example-reference-id" },
  "role": "reference_image"
}
```

提交前按目标模型检查媒体类型、角色和组合。普通代理引用的有效性和权限由素材服务判定。

以下示例使用支持 `reference_image`、5 秒、`720p` 和 `16:9` 的 Seedance 模型。使用 H3 或其他模型时，
按该模型的 `api.video.creation` 调整参数。

```bash
curl "{{SITE_BASE_URL}}/api/v3/contents/generations/tasks" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "{{MODEL_ID_PLACEHOLDER}}",
    "content": [
      {"type": "text", "text": "保持商品外观，镜头缓慢环绕展示"},
      {
        "type": "image_url",
        "role": "reference_image",
        "image_url": {"url": "asset://example-reference-id"}
      }
    ],
    "duration": 5,
    "resolution": "720p",
    "ratio": "16:9"
  }'
```

将示例引用完整替换为创建或查询返回的 `reference`，不要从 `id` 自行拼接。时长、分辨率、画幅仍按模型
合同取值。创建成功保存视频任务 `id`，随后按[ModelArk V3 视频](api-reference/videos/modelark)查询与下载。

同账号 API Key 共享托管素材，但不共享视频或图片任务；不要把素材共享规则套用到任务查询。

## 状态处理

| 状态         | 客户端行为                                                    |
| ------------ | ------------------------------------------------------------- |
| `processing` | 保存 `model + id`，有界轮询对应单项查询接口                   |
| `ready`      | 保存 `reference`，用于视频生成  |
| `failed`     | 停止轮询，保留 `error_code` 和请求 ID |

素材创建不支持幂等键。超时后请先核查原请求，重复 POST 可能创建多个素材。

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

| HTTP 状态 | `error.code`                         | 含义与处理                                                                             |
| --------- | ------------------------------------ | -------------------------------------------------------------------------------------- |
| `400`     | `invalid_request`                    | JSON、必填字段、名称、URL、有效期或参数组合无效；修正后再请求                          |
| `400`     | `reserved_asset_group_name`          | 普通调用方试图创建系统保留素材组名称；改用其它业务名称                                 |
| `400`     | `asset_url_ttl_insufficient`         | URL 剩余有效期不足；读取 `error.details.required_min_ttl_seconds` 后换用更长有效期 URL |
| `404`     | `model_not_found`                    | 客户模型不存在；重新读取模型目录                                                       |
| `404`     | `asset_not_found`                    | 素材或素材组不存在，或当前账号无权访问该资源                                                 |
| `409`     | `default_asset_group_not_configured` | 当前模型的默认素材组未配置；联系管理员完成配置，或按业务需要使用已创建的合法组 ID      |
| `422`     | `unsupported_asset_type`             | 所选素材合同不支持该 `asset_kind + media_type` 组合                                        |
| `422`     | `unsupported_asset_operation`        | 当前模型未发布该素材或素材组操作                                                       |
| `502`     | `asset_upstream_error`               | 素材服务操作失败、返回无效结果，或托管存储操作失败                                  |
| `503`     | `asset_upstream_unavailable`         | 素材服务、凭据或托管存储暂不可用；保留请求 ID 后联系管理员                                       |
| `500`     | `internal_error`                     | 平台内部错误；不要依赖错误文本判断素材是否已创建                                       |
