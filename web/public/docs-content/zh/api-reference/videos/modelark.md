---
page-id: videos-modelark
kind: api-reference
last-verified: 2026-08-28
operations:
  - listModelArkVideoModels
  - retrieveModel
  - createModelArkVideoTask
  - listModelArkVideoTasks
  - getModelArkVideoTask
  - deleteModelArkVideoTask
  - getVideoContent
---

# ModelArk V3 Seedance 视频

所有 Seedance 客户模型统一使用 ModelArk V3 任务合同。`/v1/video/generations` 属于 NEWAPI 原生视频
合同，不是本接口的别名；ModelArk、Kling、即梦和 OpenAI Videos 的字段不能混用。

## 查询模型与参数合同

可使用以下任一入口读取模型：

```text
GET /v1/models/{customer_model}
GET /api/v3/contents/generations/models
```

ModelArk 模型列表返回：

```json
{
  "success": true,
  "object": "list",
  "data": [
    {
      "id": "customer-seedance-model",
      "object": "model",
      "owned_by": "new-api",
      "supported_endpoint_types": ["modelark-video"],
      "available": true,
      "availability": "available",
      "api": {
        "video": {
          "protocol": "modelark_v3",
          "documentation_path": "/docs/api-reference/videos/modelark",
          "operations": [
            {
              "operation": "create_video",
              "method": "POST",
              "path": "/api/v3/contents/generations/tasks",
              "supported": true
            }
          ],
          "creation": {
            "method": "POST",
            "path": "/api/v3/contents/generations/tasks",
            "content_type": "application/json",
            "required_fields": ["model", "content"],
            "model": "customer-seedance-model",
            "additional_properties": false,
            "parameters": [
              {"name": "model", "type": "string", "required": true},
              {"name": "content", "type": "array", "required": true, "min_items": 1},
              {"name": "duration", "type": "integer", "minimum": 4, "maximum": 15}
            ],
            "content_types": [
              {"type": "text", "required_fields": ["type", "text"]},
              {
                "type": "image_url",
                "roles": ["first_frame", "last_frame", "reference_image"],
                "required_fields": ["type", "role", "image_url.url"]
              }
            ]
          }
        }
      }
    }
  ]
}
```

每个模型的 `api.video` 是创建和任务操作的机器可读合同：

| 字段 | 用途 |
| --- | --- |
| `available` / `availability` | 当前 Key 是否能创建任务；不可用模型仍可能保留在目录中 |
| `api.video.creation.parameters` | 顶层字段的类型、必填性、固定值、默认值、枚举和上下限 |
| `api.video.creation.content_types` | `content` 允许的媒体类型、角色、子字段和数量边界 |
| `api.video.operations` | 创建、列表、查询、删除与内容下载的路径和支持状态 |
| `api.assets` | 素材类型、操作、创建限制、引用格式与匿名复用域 |

模型名由部署方定义。客户端只能发送目录中的客户模型名，不能从名称推断分辨率、时长、媒体组合、
上游模型或 Provider。

## 创建任务

`POST /api/v3/contents/generations/tasks` · Bearer 鉴权 · `application/json`

```bash
curl "{{SITE_BASE_URL}}/api/v3/contents/generations/tasks" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "{{MODEL_ID_PLACEHOLDER}}",
    "content": [
      {"type": "text", "text": "镜头缓慢掠过清晨的山谷"},
      {
        "type": "image_url",
        "image_url": {"url": "https://example.com/first-frame.png"},
        "role": "first_frame"
      }
    ],
    "duration": 5,
    "ratio": "16:9",
    "generate_audio": false
  }'
```

### 顶层请求参数

| 字段 | 类型 | 结构必填 | 公共结构约束 |
| --- | --- | --- | --- |
| `model` | string | 是 | 客户模型名；必须可用于当前 Key |
| `content` | array | 是 | 至少一项；每项只能表达一种文本或媒体内容 |
| `duration` | integer | 否 | `-1` 表示智能时长，或 `1`～`3600`；具体模型通常有更窄范围，`0` 无效 |
| `callback_url` | string | 否 | 回调 URI；只有模型合同公开该字段时可用 |
| `resolution` | string | 否 | 输出分辨率；枚举与必填性以模型合同为准 |
| `ratio` | string | 否 | 输出画幅；枚举、默认值和自适应支持以模型合同为准 |
| `output_format` | string | 否 | `mp4` 或 `mov`；只有明确发布该字段的模型接受 |
| `service_tier` | string | 否 | 服务档位；可选值由模型合同决定 |
| `generate_audio` | boolean | 否 | 是否生成音频；模型可以只允许固定 `true` 或固定 `false` |
| `watermark` | boolean | 否 | 是否添加水印；仅模型公开时可用 |
| `return_last_frame` | boolean | 否 | 是否返回末帧；成功任务可能据此提供 `last_frame_url` |
| `execution_expires_after` | integer | 否 | 执行有效期，范围 `3600`～`259200` 秒 |
| `draft` | boolean | 否 | 草稿模式；仅模型公开时可用 |
| `tools` | array | 否 | 工具数组；每项当前只允许 `type`，可用类型由模型合同决定 |
| `safety_identifier` | string | 否 | 调用方安全标识；仅模型公开时可用 |
| `priority` | integer | 否 | `0`～`9`；仅模型公开时可用 |
| `frames` | integer | 否 | `29`～`289` 且满足 `25 + 4n`；与 `duration` 同时存在时按模型合同处理 |
| `seed` | integer | 否 | `-1`～`2147483647` |
| `camera_fixed` | boolean | 否 | 是否固定相机；仅模型公开时可用 |

公共结构允许字段不等于所选模型允许字段。提交前必须再按
`api.video.creation.parameters` 过滤；`additional_properties=false` 时，列表外字段直接返回
`400 unsupported_parameter`。

### `content` 内容项

| `type` | 必填子字段 | `role` | 说明 |
| --- | --- | --- | --- |
| `text` | `text` | 不允许 | 文本不能为空；一项中不能同时带任何媒体字段 |
| `image_url` | `image_url.url` | `first_frame`、`last_frame` 或 `reference_image` | 图片首帧、末帧或参考图 |
| `video_url` | `video_url.url` | `reference_video` | 参考视频 |
| `audio_url` | `audio_url.url` | `reference_audio` | 参考音频 |

每个媒体项必须且只能带与 `type` 对应的 URL 对象，并且必须提供有效 `role`：

```json
{
  "type": "image_url",
  "image_url": {"url": "asset://provider-reference-id"},
  "role": "reference_image"
}
```

媒体 URL 支持：

- `http://` 或 `https://` 绝对 URL，不允许 URL 用户名或密码；
- 包含逗号分隔负载的 Data URL；
- 非空的 `asset://<opaque-id>` 引用。

具体模型允许哪些 `type + role`、每种内容的数量以及组合规则，以
`api.video.creation.content_types` 为准。不支持的组合会明确失败，不会静默删字段或降级为文生视频。

### 创建响应

创建成功返回 HTTP `200`，只返回平台任务 ID：

```json
{
  "id": "task-public-id"
}
```

客户端必须立即保存 `id`，随后通过任务查询接口读取状态。HTTP `200` 只表示任务已建立，不表示视频
已经生成完成。

## 查询单个任务

`GET /api/v3/contents/generations/tasks/{task_id}`

```bash
curl "{{SITE_BASE_URL}}/api/v3/contents/generations/tasks/task-public-id" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}"
```

成功任务示例：

```json
{
  "id": "task-public-id",
  "model": "customer-seedance-model",
  "status": "succeeded",
  "content": {
    "video_url": "{{SITE_BASE_URL}}/v1/videos/task-public-id/content",
    "last_frame_url": "{{SITE_BASE_URL}}/v1/videos/task-public-id/content?part=last_frame"
  },
  "seed": 12345,
  "resolution": "1080p",
  "duration": 5,
  "frames": 121,
  "ratio": "16:9",
  "generate_audio": false,
  "service_tier": "default",
  "usage": {
    "completion_tokens": 100,
    "total_tokens": 100
  },
  "created_at": 1760000000,
  "updated_at": 1760000120
}
```

### 任务响应字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | string | 平台任务 ID |
| `model` | string | 创建时冻结的客户模型名 |
| `status` | string | `queued`、`running`、`succeeded`、`failed`、`cancelled` 或 `expired` |
| `content.video_url` | string | 成功后的视频内容代理路径 |
| `content.last_frame_url` | string | 成功且存在末帧时返回的末帧代理路径 |
| `seed` | integer | 实际或上游返回的随机种子；不存在时省略 |
| `resolution` | string | 实际输出分辨率；不存在时省略 |
| `duration` | integer | 实际时长；不存在时省略 |
| `frames` | integer | 实际帧数；不存在时省略 |
| `framespersecond` | integer | 实际帧率；不存在时省略 |
| `ratio` | string | 实际画幅；不存在时省略 |
| `generate_audio` | boolean | 是否生成音频；保留显式 `false` |
| `draft` | boolean | 是否为草稿任务；保留显式 `false` |
| `draft_task_id` | string | 相关草稿任务 ID；模型返回时存在 |
| `safety_identifier` | string | 创建时的公开安全标识；存在时返回 |
| `priority` | integer | 创建时的优先级；保留显式 `0` |
| `service_tier` | string | 服务档位；未指定时通常为 `default` |
| `usage.completion_tokens` | integer | 可选完成用量 |
| `usage.total_tokens` | integer | 可选总用量 |
| `usage.tool_usage.web_search` | integer | 可选工具用量 |
| `error.code` | string | 失败、取消或过期时的公开错误码 |
| `error.message` | string | 脱敏错误说明 |
| `created_at` / `updated_at` | integer | Unix 秒时间戳 |

终态任务的公开错误码包括 `generation_failed`、`provider_contract_failure`、`cancelled` 和 `expired`。
一次上游查询异常不会立刻把已有任务判为业务失败；接口可能返回最后一次持久化状态，客户端应继续有界
轮询。

## 列出任务

`GET /api/v3/contents/generations/tasks`

```bash
curl "{{SITE_BASE_URL}}/api/v3/contents/generations/tasks?page_num=1&page_size=20&filter.status=running" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}"
```

| 查询参数 | 默认值 | 约束与说明 |
| --- | --- | --- |
| `page_num` | `1` | `1`～`500` |
| `page_size` | `10` | `1`～`500` |
| `filter.status` | 全部 | `queued`、`running`、`succeeded`、`failed`、`cancelled` 或 `expired` |
| `filter.task_ids` | 无 | 可重复查询参数，最多 `500` 个，例如 `?filter.task_ids=id1&filter.task_ids=id2` |
| `filter.model` | 无 | 精确匹配创建时的客户模型名 |
| `filter.service_tier` | `default` | 精确匹配服务档位 |

列表只返回当前 API Key 应用范围内、最近 7 天、尚未被客户端删除的 ModelArk 任务，按创建时间倒序排列：

```json
{
  "total": 1,
  "items": [
    {
      "id": "task-public-id",
      "model": "customer-seedance-model",
      "status": "running",
      "service_tier": "default",
      "created_at": 1760000000,
      "updated_at": 1760000030
    }
  ]
}
```

`total` 是过滤后的总数，`items` 是当前页的任务对象数组。

## 删除或取消任务

`DELETE /api/v3/contents/generations/tasks/{task_id}`

成功返回 HTTP `200` 和空对象：

```json
{}
```

删除语义取决于当前任务状态与模型公开的生命周期能力：

- 排队任务只有在模型支持取消时才能取消；
- `running` 任务不能删除；
- 已取消任务不能再次删除；
- 成功、失败或过期任务只有在模型支持终态删除时才能删除。

状态不允许或能力不支持时返回 HTTP `409`，常见错误码包括 `cancellation_unsupported`、
`task_running`、`task_cancelled`、`delete_unsupported`、`cancellation_in_progress`、
`cancellation_unknown` 和 `delete_rejected`。取消结果不明时先重新查询任务，不能换模型再次创建。

## 下载视频或末帧

任务为 `succeeded` 后，使用任务响应中返回的内容路径：

```bash
curl "{{OPENAI_BASE_URL}}/videos/task-public-id/content" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  --output result.mp4
```

如响应包含 `last_frame_url`，按该路径下载末帧。内容代理要求任务已成功，并按创建任务的鉴权主体隔离。
下载前先检查 HTTP 状态和 `Content-Type`；失败响应是 JSON，不是视频或图片字节。

| HTTP 状态 | 常见错误码 | 说明 |
| --- | --- | --- |
| `400` | `video_not_ready`、`invalid_content_part` | 任务尚未成功，或 `part` 不是 `last_frame` |
| `404` | `video_not_found`、`content_not_found` | 任务、视频或末帧不存在 |
| `410` | `video_content_expired` | 上游内容已过期 |
| `403` / `502` | `content_url_not_allowed`、`upstream_unavailable` | 内容来源被安全策略阻止或上游读取失败 |

## 素材引用

视频请求可直接使用请求级媒体，也可以使用素材 API 返回的 `asset://<opaque-id>`。平台不查询该 ID，
不验证所有权、ready 状态、创建模型或 Provider 作用域，也不尝试其它 Provider；当前模型选定的上游
最终判断素材存在性、权限、兼容性与内容审核。

跨模型复用前比较两个模型的 `api.assets.reuse_scope`。只有两个非空值完全相同时才可尝试复用；scope
不同或缺失时不得复用。素材创建、素材组和真人认证的详细参数见素材 API 页面。

控制台模型广场的模型卡片按同一规则显示“素材共享组”短标签：标签相同的模型位于同一素材复用域，
可以把同一个 `asset://` 引用交给同组其它模型尝试；没有标签的模型未发布素材库，也没有可比较的
复用域，不得把其它模型的素材引用交给它。标签只是复用提示，最终存在性、权限和兼容性由当前模型
选定的上游判断。

## 创建结果不明、计费与重试

ModelArk 创建请求最多发送一次上游 POST，当前不接受客户幂等键。发送后结果不明时返回
`create_outcome_unknown`，平台不会自动重发、换模型或退款。客户端必须停止自动创建，保存
`request_id` 并联系技术人员核查。

任务成功建立后，预扣、结算、差额和退款使用创建时冻结的模型、执行路径和计费事实。轮询 GET 可以有界
重试；创建 POST 不能因为客户端超时而盲目重放。

## 错误响应

ModelArk 错误信封：

```json
{
  "error": {
    "code": "invalid_request",
    "message": "model and content are required",
    "request_id": "req-placeholder"
  }
}
```

| HTTP 状态 | 常见错误码 | 说明 |
| --- | --- | --- |
| `400` | `invalid_request`、`unsupported_parameter`、`invalid_page_size`、`invalid_status` | 请求结构、字段、分页或过滤条件错误 |
| `401` / `403` | 鉴权或权限错误 | 检查 API Key、模型可用状态和分组 |
| `404` | `task_not_found` | 当前应用范围内没有该 ModelArk 任务 |
| `409` | 取消或删除冲突码 | 当前任务状态不允许该操作 |
| `429` | 限流或额度错误 | 根据错误码区分并退避 |
| `503` | `create_outcome_unknown`、`upstream_unavailable`、`cancellation_unknown` | 不要重发创建；查询或联系管理员核查 |
