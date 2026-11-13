---
page-id: videos-modelark
kind: api-reference
last-verified: 2026-11-13
operations:
  - listModelArkVideoModels
  - retrieveModel
  - createModelArkVideoTask
  - listModelArkVideoTasks
  - getModelArkVideoTask
  - deleteModelArkVideoTask
  - getVideoContent
  - listVideoFundProgress
---

# ModelArk V3 标准视频

Seedance、H3 和 MiniMax 标准视频可使用 ModelArk V3。创建入口为 `POST /api/v3/contents/generations/tasks`。

完整流程是：查询模型 → POST 创建 → 保存 `id` → GET 轮询 → 成功后等待交付就绪，再通过结果签名地址下载。
可复制的轮询脚本见[图片与视频调用实战](guides/media-workflow)。创建、查询和访问内容入口使用创建时
的同一 API Key；下载本站 OSS 签名地址无需附带平台或 Provider Key。

按模型元数据中的 `api.video.documentation_path` 阅读对应说明：

- [H3 视频](api-reference/videos/h3)：参考图、视频、音频及首尾帧。
- [Seedance V 视频](api-reference/videos/seedance-v)

## 查询模型与参数合同

可使用以下任一入口读取模型：

```text
GET /v1/models/{customer_model}
GET /api/v3/contents/generations/models
```

列出视频模型，通过 `available` 判断当前 Key 能否调用：

```bash
curl "{{SITE_BASE_URL}}/api/v3/contents/generations/models" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}"
```

也可以通过 `{{OPENAI_BASE_URL}}/models/{{MODEL_ID_PLACEHOLDER}}` 读取单个模型；路径中的模型名
须先进行 URL 编码。ModelArk 创建路径从站点根 `/api/v3` 开始，不拼到 `/v1` 后面。

以下列表为结构示意，省略了部分操作和参数；时长与媒体上限不能直接套用到其他模型：

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
              { "name": "model", "type": "string", "required": true },
              {
                "name": "content",
                "type": "array",
                "required": true,
                "min_items": 1
              },
              {
                "name": "duration",
                "type": "integer",
                "minimum": 4,
                "maximum": 15
              },
              {
                "name": "resolution",
                "type": "string",
                "required": false,
                "default_value": "720p",
                "enum": ["480p", "720p"]
              }
            ],
            "content_types": [
              { "type": "text", "required_fields": ["type", "text"] },
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

| 字段                               | 用途                                                         |
| ---------------------------------- | ------------------------------------------------------------ |
| `available` / `availability`       | 当前 Key 是否能创建任务；不可用模型仍可能保留在目录中        |
| `api.video.creation.parameters`    | 顶层字段的类型、必填性、固定值、默认值、枚举、推荐值和上下限 |
| `api.video.creation.content_types` | `content` 允许的媒体类型、角色、子字段和数量边界             |
| `api.video.operations`             | 创建、列表、查询、删除与内容下载的路径和支持状态             |
| `api.assets` / `api.hosted_assets`   | 默认素材合同，以及配置上游代理时单独列出的账号级托管合同   |

模型名由部署方定义。客户端只能发送目录中的客户模型名，不能从名称推断分辨率、时长、媒体组合、
上游模型或 Provider。

### 分辨率：允许值与推荐值

读取 `parameters` 中 `name=resolution` 的项：

- 存在 `enum` 时，只发送其中的值。例如 `["480p", "720p"]` 不支持 `1080p` 或 `4k`。
- `suggested_values` 是非穷举推荐值。例如 `["480p", "720p"]` 可用于展示常用选项，但不能当作
  `enum`；没有 enum 也不表示任意分辨率都能成功，其它值仍由上游判定。
- `default_value` 表示省略字段后的本站默认值；推荐值不是默认值。按目录中的大小写发送，
  不要自行把 `4k` 改写成 `4K`。

推荐值的结构示意：

```json
{
  "name": "resolution",
  "type": "string",
  "required": false,
  "default_value": "720p",
  "suggested_values": ["480p", "720p"]
}
```

模型可以只支持两档分辨率，也可以支持四档；不能因为模型版本更新就假定分辨率更高。
调用方不应硬编码全部模型共享的分辨率列表。

### 输出格式与场景限制

仅当模型参数列表含 `output_format` 时才发送该字段，并遵守其枚举。不支持格式参数的模型应省略
整个字段，不能默认给所有视频请求添加 `output_format=mp4`；显式传入未发布字段会失败。

平面的参数枚举、媒体类型和数量上限不能完整表达场景间的组合限制。使用部署方确认对应
官方 Seedance 2.5 能力的客户模型时，还需遵守以下上游规则。
本段不适用于 [Seedance V](api-reference/videos/seedance-v) 或 [H3](api-reference/videos/h3)，
即使客户模型名称相似也应读取其专属合同：

- 首帧、首尾帧、视频延长仅支持 `ratio=adaptive`；这些场景不要照抄文生视频的 `16:9`。
- 首帧/首尾帧与多模态参考素材不能混用。
- 图片最多 30 项、视频最多 10 项、音频最多 10 项，总素材最多 50 项；参考视频、参考音频各自
  总时长不得超过 30 秒。支持纯音频参考，视频编辑尚未开放。
- 请使用与场景匹配的媒体角色、提示词和画幅。部分组合错误在任务查询时返回。

使用对应 Fast 或 Mini 的模型时，请将参考音频与图片或视频搭配。纯音频输入以所选模型的生成合同为准。

省略 `duration` 时以模型元数据的 `default_value` 为准。

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
    "ratio": "adaptive",
    "generate_audio": false
  }'
```

### 文生视频

将上例的 `content` 替换为仅含文本的数组即可。以下为完整请求体；发送到相同创建路径。
示例中的 5 秒、720p 和 16:9 仅用于演示，调用前核对模型参数，补齐模型声明的其他必填字段：

```json
{
  "model": "{{MODEL_ID_PLACEHOLDER}}",
  "content": [
    { "type": "text", "text": "清晨的海面上，一艘帆船缓缓驶过，镜头平稳推进" }
  ],
  "duration": 5,
  "resolution": "720p",
  "ratio": "16:9"
}
```

### 首尾帧与多参考素材

模型公开对应 `type + role` 时，可将 `content` 替换为下列数组。首帧与末帧控制起止画面，
`reference_image` 用作参考素材，不能把角色互换来绕过不支持的组合。

首尾帧示例：替换 `content` 时同时核对 `ratio`；2.5 必须使用 `adaptive`。

```json
[
  {
    "type": "text",
    "text": "镜头从空桌面平滑推进到摆好餐具的桌面"
  },
  {
    "type": "image_url",
    "role": "first_frame",
    "image_url": {
      "url": "https://example.com/start.png"
    }
  },
  {
    "type": "image_url",
    "role": "last_frame",
    "image_url": {
      "url": "https://example.com/end.png"
    }
  }
]
```

多模态参考示例，仅用于同时支持这些类型和组合的模型：

```json
[
  {
    "type": "text",
    "text": "保持参考图中的商品外观，参考视频的镜头运动和音频节奏"
  },
  {
    "type": "image_url",
    "role": "reference_image",
    "image_url": {
      "url": "https://example.com/product.png"
    }
  },
  {
    "type": "video_url",
    "role": "reference_video",
    "video_url": {
      "url": "https://example.com/motion.mp4"
    }
  },
  {
    "type": "audio_url",
    "role": "reference_audio",
    "audio_url": {
      "url": "https://example.com/rhythm.mp3"
    }
  }
]
```

每张参考图分别占一个内容项。不要同时照抄全部示例；组合、总数量及文件规格以模型公开能力为准。
媒体地址必须在处理期间可读取，不能使用本地文件路径。长期复用图片可先调用
[素材与素材组](api-reference/assets)，再原样使用返回的 `reference`。

### 顶层请求参数

| 字段                      | 类型    | 结构必填 | 公共结构约束                                                                                    |
| ------------------------- | ------- | -------- | ----------------------------------------------------------------------------------------------- |
| `model`                   | string  | 是       | 客户模型名；必须可用于当前 Key                                                                  |
| `content`                 | array   | 是       | 至少一项；每项只能表达一种文本或媒体内容                                                        |
| `duration`                | integer | 否       | `-1` 表示智能时长，或 `1`～`3600`；具体模型通常有更窄范围，`0` 无效                             |
| `callback_url`            | string  | 否       | 回调 URI；只有模型合同公开该字段时可用，未声明时使用 GET 轮询，不假设平台提供统一回调或重试保证 |
| `resolution`              | string  | 否       | 输出分辨率；枚举与必填性以模型合同为准                                                          |
| `ratio`                   | string  | 否       | 输出画幅；枚举、默认值和自适应支持以模型合同为准                                                |
| `output_format`           | string  | 否       | `mp4` 或 `mov`；只有明确发布该字段的模型接受                                                    |
| `service_tier`            | string  | 否       | 服务档位；可选值由模型合同决定                                                                  |
| `generate_audio`          | boolean | 否       | 是否生成音频；模型可以只允许固定 `true` 或固定 `false`                                          |
| `watermark`               | boolean | 否       | 是否添加水印；仅模型公开时可用                                                                  |
| `return_last_frame`       | boolean | 否       | 是否返回末帧；成功任务可能据此提供 `last_frame_url`                                             |
| `execution_expires_after` | integer | 否       | 执行有效期，范围 `3600`～`259200` 秒                                                            |
| `draft`                   | boolean | 否       | 草稿模式；仅模型公开时可用                                                                      |
| `tools`                   | array   | 否       | 工具数组；每项当前只允许 `type`，可用类型由模型合同决定                                         |
| `safety_identifier`       | string  | 否       | 调用方安全标识；仅模型公开时可用                                                                |
| `priority`                | integer | 否       | `0`～`9`；仅模型公开时可用                                                                      |
| `frames`                  | integer | 否       | `29`～`289` 且满足 `25 + 4n`；与 `duration` 同时存在时按模型合同处理                            |
| `seed`                    | integer | 否       | `-1`～`2147483647`                                                                              |
| `camera_fixed`            | boolean | 否       | 是否固定相机；仅模型公开时可用                                                                  |

公共结构允许字段不等于所选模型允许字段。提交前必须按
`api.video.creation.parameters` 组装请求；不要提交后再静默删字段重试。
其中 `fixed_value` 为固定值，`default_value` 为缺省值，`special_values` 表示额外允许的特殊值。
只有 `duration.special_values` 明确含 `-1` 时才能请求智能时长；需要固定输出时长时直接传合法正整数。
`additional_properties=false` 时，列表外字段直接返回
`400 unsupported_parameter`。

字段的生效条件见上表。
例如是否返回末帧仍需检查成功响应中是否实际存在 `last_frame_url`，不能仅凭提交了字段就假定结果存在。

### `content` 内容项

| `type`      | 必填子字段      | `role`                                           | 说明                                       |
| ----------- | --------------- | ------------------------------------------------ | ------------------------------------------ |
| `text`      | `text`          | 不允许                                           | 文本不能为空；一项中不能同时带任何媒体字段 |
| `image_url` | `image_url.url` | `first_frame`、`last_frame` 或 `reference_image` | 图片首帧、末帧或参考图                     |
| `video_url` | `video_url.url` | `reference_video`                                | 参考视频                                   |
| `audio_url` | `audio_url.url` | `reference_audio`                                | 参考音频                                   |

每个媒体项必须且只能带与 `type` 对应的 URL 对象，并且必须提供有效 `role`：

```json
{
  "type": "image_url",
  "image_url": { "url": "asset://example-reference-id" },
  "role": "reference_image"
}
```

媒体 URL 支持：

- `http://` 或 `https://` 绝对 URL，不允许 URL 用户名或密码；
- 包含逗号分隔负载的 Data URL；
- 非空的 `asset://<opaque-id>` 引用。

具体模型允许哪些 `type + role`、每种内容的数量以及组合规则，以
`api.video.creation.content_types` 与上述场景说明为准。部分组合由上游异步校验，不能仅凭每种媒体
分别受支持就认定其组合有效。

### 参考音频的 URL、Base64 与文件

`type=audio_url`、`role=reference_audio` 和 `audio_url.url` 保持不变：

- `audio_url.url` 可传 HTTPS URL、音频 Data URL（如 `data:audio/wav;base64,...`）或裸 Base64。
- 文件使用同一路径的 `multipart/form-data`：`request` 表单项包含完整 ModelArk JSON，音频附件通过
  `audio_url.url` 中的 `file://<表单项名称>` 引用。它只引用本次上传的附件，不是服务器文件路径。

文件上传示例（模型须支持参考音频与该组合）：

```bash
curl "{{SITE_BASE_URL}}/api/v3/contents/generations/tasks" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  -F 'request={"model":"{{MODEL_ID_PLACEHOLDER}}","content":[{"type":"text","text":"蓝色陶瓷杯，镜头缓慢推进"},{"type":"image_url","image_url":{"url":"https://example.com/cup.png"},"role":"reference_image"},{"type":"audio_url","audio_url":{"url":"file://audio"},"role":"reference_audio"}]};type=application/json' \
  -F 'audio=@reference.wav;type=audio/wav'
```

附件表单项名称必须唯一，所有附件都必须被音频内容项引用；每个上传文件或解码后的 Base64 音频最多
15 MiB，全请求仍受站点请求体上限限制。内联或文件音频无法读取用于生成时，可返回
`503 reference_audio_unavailable`，此时不会预扣生成费用。请确保媒体格式、实际时长和组合符合模型要求。
需要跨任务复用时，使用[托管素材](api-reference/assets)。

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
    "delivery_status": "ready",
    "video_url": "https://storage.example/video-object",
    "last_frame_url": "https://storage.example/last-frame-object"
  },
  "seed": 12345,
  "resolution": "1080p",
  "duration": 5,
  "frames": 121,
  "ratio": "16:9",
  "generate_audio": false,
  "service_tier": "default",
  "usage": {
    "total_seconds": 5,
    "input_seconds": 0,
    "output_seconds": 5,
    "input_audio_seconds": 0,
    "input_image_count": 1,
    "prompt_tokens": 0,
    "completion_tokens": 100,
    "total_tokens": 100,
    "tool_usage": {"web_search": 0}
  },
  "created_at": 1760000000,
  "updated_at": 1760000120
}
```

### 任务响应字段

| 字段                          | 类型    | 说明                                                                 |
| ----------------------------- | ------- | -------------------------------------------------------------------- |
| `id`                          | string  | 平台任务 ID                                                          |
| `model`                       | string  | 创建时使用的客户模型名                                               |
| `status`                      | string  | `queued`、`running`、`succeeded`、`failed`、`cancelled` 或 `expired` |
| `content.delivery_status`     | string  | 本站托管线路的主视频交付状态：`ready`、`preparing` 或 `unavailable`；范围外线路可省略 |
| `content.video_url`           | string  | 本站托管线路就绪后返回 24 小时预签名 GET 地址；范围外线路沿用鉴权内容代理路径 |
| `content.last_frame_url`      | string  | 末帧就绪时返回的签名 GET 地址                                   |
| `seed`                        | integer | 实际或上游返回的随机种子；不存在时省略                               |
| `resolution`                  | string  | 实际输出分辨率；不存在时省略                                         |
| `duration`                    | integer | 实际时长；不存在时省略                                               |
| `frames`                      | integer | 实际帧数；不存在时省略                                               |
| `framespersecond`             | integer | 实际帧率；不存在时省略                                               |
| `ratio`                       | string  | 实际画幅；不存在时省略                                               |
| `generate_audio`              | boolean | 是否生成音频；保留显式 `false`                                       |
| `draft`                       | boolean | 是否为草稿任务；保留显式 `false`                                     |
| `draft_task_id`               | string  | 相关草稿任务 ID；模型返回时存在                                      |
| `safety_identifier`           | string  | 创建时的公开安全标识；存在时返回                                     |
| `priority`                    | integer | 创建时的优先级；保留显式 `0`                                         |
| `service_tier`                | string  | 服务档位；未指定时通常为 `default`                                   |
| `usage.total_seconds`        | number  | 已确认的输入与输出视频秒数之和，或已确认同口径的上游总秒数。音频单列 |
| `usage.input_seconds`        | number  | 已确认的输入视频秒数 |
| `usage.output_seconds`       | number  | 已确认的输出视频秒数 |
| `usage.input_audio_seconds`  | number  | 已确认的输入音频秒数 |
| `usage.input_image_count`    | integer | 已确认的输入图片数 |
| `usage.prompt_tokens`        | integer | 来源明确的输入 Token 数 |
| `usage.completion_tokens`    | integer | 归一后的完成 Token 数 |
| `usage.total_tokens`         | integer | 归一后的总 Token 数 |
| `usage.tool_usage.web_search` | integer | 已确认的搜索调用次数 |
| `error.code`                  | string  | 失败、取消或过期时的公开错误码                                       |
| `error.message`               | string  | 脱敏错误说明                                                         |
| `created_at` / `updated_at`   | integer | Unix 秒时间戳                                                        |

单查与列表的每个任务都返回上述 `usage` 字段。秒数允许小数。字段值为 `0` 可能表示已确认为零，
也可能表示用量尚未确认。

终态任务的公开错误码包括 `generation_failed`、`provider_contract_failure`、`cancelled` 和 `expired`。
`queued` 或 `running` 时退避查询原任务。`running` 也可能表示结果仍在核实。客户端等待超时或
单次查询失败时，保留原任务 ID，稍后继续查询或联系管理员核查。

## 列出任务

`GET /api/v3/contents/generations/tasks`

```bash
curl "{{SITE_BASE_URL}}/api/v3/contents/generations/tasks?page_num=1&page_size=20&filter.status=running" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}"
```

| 查询参数              | 默认值    | 约束与说明                                                                     |
| --------------------- | --------- | ------------------------------------------------------------------------------ |
| `page_num`            | `1`       | `1`～`500`                                                                     |
| `page_size`           | `10`      | `1`～`500`                                                                     |
| `filter.status`       | 全部      | `queued`、`running`、`succeeded`、`failed`、`cancelled` 或 `expired`           |
| `filter.task_ids`     | 无        | 可重复查询参数，最多 `500` 个，例如 `?filter.task_ids=id1&filter.task_ids=id2` |
| `filter.model`        | 无        | 精确匹配创建时的客户模型名                                                     |
| `filter.service_tier` | `default` | 精确匹配服务档位                                                               |

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
      "usage": {
        "total_seconds": 0,
        "input_seconds": 0,
        "output_seconds": 0,
        "input_audio_seconds": 0,
        "input_image_count": 0,
        "prompt_tokens": 0,
        "completion_tokens": 0,
        "total_tokens": 0,
        "tool_usage": {"web_search": 0}
      },
      "created_at": 1760000000,
      "updated_at": 1760000030
    }
  ]
}
```

`total` 是过滤后的总数，`items` 是当前页的任务对象数组。运行中任务也返回完整的 `usage`；
上例中的零值不表示用量已确认或结算已完成。

## 删除或取消任务

`DELETE /api/v3/contents/generations/tasks/{task_id}`

请求示例：

```bash
curl -X DELETE "{{SITE_BASE_URL}}/api/v3/contents/generations/tasks/task-public-id" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}"
```

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

Seedance 与 MiniMax 标准视频返回 `status=succeeded` 后，按 `content.delivery_status` 处理下载。

| 交付状态 | 客户行为 |
| --- | --- |
| `preparing` | 结果正在准备，不返回下载地址；继续查询同一任务 |
| `ready` | 主视频就绪，按 `video_url` 下载；实际末帧就绪时另返回 `last_frame_url` |
| `unavailable` | 结果无法交付，不返回主视频地址；保留任务 ID 联系技术人员核查 |

签名 URL 自签发起有效 24 小时（86400 秒）。下载时使用完整 URL，无需附带 API Key。

```bash
curl --fail "$VIDEO_URL" --output result.mp4
```

`VIDEO_URL` 为授权任务查询返回的完整 `content.video_url`。末帧同理使用 `content.last_frame_url`，
文件扩展名按实际 MIME 选择。断点续传取决于下载地址是否支持 Range 请求。

`delivery_status=ready` 表示主视频就绪，不保证末帧已就绪；`last_frame_url` 缺失不能单独判定
未生成末帧。需要已请求的末帧时可继续有界查询，或使用下述 `?part=last_frame` 内容入口区分
准备中、交付不可用和部件不存在。

链接过期后，若文件仍保留且授权有效，重新查询原任务可续签。文件保留期由部署方设定。
完整签名 URL 带有下载权限，请妥善保存。

既有内容入口仍需平台鉴权，交付就绪后返回 `302` 跳转至本站签名地址：

```bash
curl --fail --location "{{OPENAI_BASE_URL}}/videos/task-public-id/content" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  --output result.mp4
```

不要使用 `--location-trusted` 将平台鉴权转发给对象存储。末帧使用相同入口并添加 `?part=last_frame`。
HEAD 内容请求返回针对 HEAD 方法的签名地址；响应中的 GET 下载地址用于 GET，不能替代 HEAD 签名。

| HTTP 状态 | 常见错误码 | 说明 |
| --- | --- | --- |
| `400` | `video_not_ready`、`invalid_content_part` | 任务尚未成功，或部件不合法 |
| `404` | `video_not_found`、`content_not_found` | 任务或所请求的末帧不存在 |
| `409` | `video_delivery_preparing` | 结果准备中，稍后重试 |
| `410` | `video_delivery_unavailable` | 结果交付不可用 |
| `503` | `video_delivery_unavailable` | 当前本站交付存储不可用 |

交付失败时请核查原任务。已确认的生成费用仍按账单结算。原生视频的 `410 video_content_expired`
表示内容已过期，重新查询原任务无法保证恢复下载。

## 素材引用

将[托管素材](api-reference/assets)返回的 `reference` 填入对应的 `image_url.url`、`audio_url.url` 或
`video_url.url`。直接 URL 可与托管引用混用。提交生成时，媒体类型、角色、数量、组合及规格须符合目标模型的要求。

## 创建结果不明、计费与重试

ModelArk V3 创建不支持客户幂等键。收到 `create_outcome_unknown` 时，保留 `request_id` 核查，
不要重发创建请求。结果待核实且尚未建立任务的钱包占款，满 24 小时后自动退回。已受理任务按实际任务结算。

控制台用量日志的“视频退款进度”显示本人请求，包括没有成功返回任务 ID 的创建占款。程序可用原
API Key 请求 `GET /api/video-funds/token?p=1`，可选 `task_id` 按平台任务或占款请求 ID 精确筛选。
每页 20 条；只返回该 Key 所属用户及应用的数据。金额单位为 quota，`refund_amount_known=false`
表示历史退款额无法核实，零时间表示不适用或未知。退款不代表上游已取消；退款后不会自动补扣。

任务成功建立后，费用按创建时确定的模型、请求参数和价格规则处理。成功视频可以先交付，结算随后完成：

- 按实际用量计费时，尚未确认所需用量可能继续保留预扣，直到用量补齐后再结算。
  `usage` 用于展示，零值和字段是否存在均不能作为结算凭证。
- 最终费用可能高于或低于预扣，差额按实际规则补扣或退还；不要用预扣值当最终账单；
- 成功后的费用核实不要求重新生成视频，查询也不会重新触发生成。

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

| HTTP 状态     | 常见错误码                                                                        | 说明                                         |
| ------------- | --------------------------------------------------------------------------------- | -------------------------------------------- |
| `400`         | `invalid_request`、`unsupported_parameter`、`invalid_page_size`、`invalid_status` | 请求结构、字段、分页或过滤条件错误           |
| `400`         | `invalid_video_parameter`                                                         | 参数或素材引用不符合当前模型合同             |
| `401` / `403` | 鉴权或权限错误                                                                    | 检查 API Key、模型可用状态和分组             |
| `404`         | `task_not_found`                                                                  | 当前应用范围内没有该 ModelArk 任务           |
| `409`         | 取消或删除冲突码                                                                  | 当前任务状态不允许该操作                     |
| `413`         | `request_body_too_large`                                                          | 请求正文或内联媒体超过上限，减少大小后再提交 |
| `429`         | 限流或额度错误                                                                    | 根据错误码区分并退避                         |
| `503`         | `internal_error`                                                                  | 也用于托管对象或签名不可用，此类错误发生在预扣和提交前       |
| `503`         | `create_outcome_unknown`、`upstream_unavailable`、`cancellation_unknown`          | 不要重发创建；查询或联系管理员核查           |

请求记录相关的 `evidence_*` 错误及重试边界见[错误与重试](concepts/errors#请求记录相关错误)。
