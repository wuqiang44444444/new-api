---
page-id: videos-modelark
kind: api-reference
last-verified: 2026-09-22
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

# ModelArk V3 Seedance 视频

所有 Seedance 客户模型统一使用 ModelArk V3 任务合同。`/v1/video/generations` 属于原生通用视频
协议，不是本接口的别名；ModelArk、Kling、即梦和 OpenAI Videos 的字段不能混用。

完整流程是：查询模型 → POST 创建 → 保存 `id` → GET 轮询 → 成功后鉴权下载。
可复制的轮询脚本见[图片与视频调用实战](guides/media-workflow)。所有步骤使用创建时的同一 API Key。

## 查询模型与参数合同

可使用以下任一入口读取模型：

```text
GET /v1/models/{customer_model}
GET /api/v3/contents/generations/models
```

查询当前 Key 可用的视频模型：

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
| `api.assets`                       | 素材类型、操作、创建限制、引用格式与匿名复用域               |

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
Seedance 2.5 能力的客户模型时，还需遵守以下上游规则：

- 首帧、首尾帧、视频延长仅支持 `ratio=adaptive`；这些场景不要照抄文生视频的 `16:9`。
- 首帧/首尾帧与多模态参考素材不能混用。
- 图片最多 30 项、视频最多 10 项、音频最多 10 项，总素材最多 50 项；参考视频、参考音频各自
  总时长不得超过 30 秒。支持纯音频参考，视频编辑尚未开放。
- 上游根据内容角色与提示词判定任务类型，部分场景错误可能在任务开始后异步返回。
  网关不根据提示词替调用方选择场景，也不改写画幅。

Fast/Mini 的音频参考应搭配图片或视频，文档不建议仅传音频；不能把接受 `audio_url` 视为纯音频
生成已经验证。2.0（0818）页面未禁止纯音频，也没有提供真实履约保证。

四款对应的本站合同缺省时长为 5 秒；2.5 上游自身默认 -1 不改变本站默认值。模型名称可由部署方
自定义，以上场景说明不能用于从客户模型名推断 Provider 或能力。

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

已发布字段按统一协议的条件生效，服务负责必要的转换；不要求调用方填写内部参数或改写请求结构。
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
分别受支持就认定其组合有效；失败不会静默删字段或降级为文生视频。

### 参考音频的 URL、Base64 与文件

`type=audio_url`、`role=reference_audio` 和 `audio_url.url` 保持不变：

- HTTPS 地址由网关原样传递，无需预先复制到本站 OSS。
- 音频 Data URL（例如 `data:audio/wav;base64,...`）或裸 Base64 由网关上传到本站 OSS，使用本站
  HTTPS URL 发送。未声明 MIME 时可根据文件头识别，但识别结果不会作为音频有效性的拒绝条件。
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
15 MiB（含上限，按文件实际字节数计算），全请求仍受站点请求体上限限制。本站 OSS 不可用、上传或签名失败时返回
`503 reference_audio_unavailable`，不提交生成任务、不预扣生成费用。HTTPS URL 无需本站 OSS。

网关不检查参考音频的实际时长、不转码；媒体是否有效以及是否满足模型限制由上游判断，错误信息经
脱敏后返回。音频上传不会创建图片素材，也不会把 `audio_url` 改成 `image_url`。

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

| 字段                          | 类型    | 说明                                                                 |
| ----------------------------- | ------- | -------------------------------------------------------------------- |
| `id`                          | string  | 平台任务 ID                                                          |
| `model`                       | string  | 创建时冻结的客户模型名                                               |
| `status`                      | string  | `queued`、`running`、`succeeded`、`failed`、`cancelled` 或 `expired` |
| `content.video_url`           | string  | 成功后的视频内容代理路径                                             |
| `content.last_frame_url`      | string  | 成功且存在末帧时返回的末帧代理路径                                   |
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
| `usage.completion_tokens`     | integer | 可选完成用量；当前响应省略零值，字段缺失不能区分零用量与未报告       |
| `usage.total_tokens`          | integer | 可选总用量；当前响应省略零值，不能用字段缺失推算费用                 |
| `usage.tool_usage.web_search` | integer | 可选工具用量                                                         |
| `error.code`                  | string  | 失败、取消或过期时的公开错误码                                       |
| `error.message`               | string  | 脱敏错误说明                                                         |
| `created_at` / `updated_at`   | integer | Unix 秒时间戳                                                        |

终态任务的公开错误码包括 `generation_failed`、`provider_contract_failure`、`cancelled` 和 `expired`。
一次上游查询异常不会立刻把已有任务判为业务失败；接口可能返回最后一次持久化状态，客户端应继续有界
轮询。
`running` 也可能表示执行结果仍在核实，不保证上游正在生成。等待超过客户端时限后保留原任务 ID，
稍后继续查询或联系管理员，不重新创建任务，也不据此认定失败或退款。

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
      "created_at": 1760000000,
      "updated_at": 1760000030
    }
  ]
}
```

`total` 是过滤后的总数，`items` 是当前页的任务对象数组。

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

任务为 `succeeded` 后，使用任务响应中返回的内容路径：

```bash
curl --fail "{{OPENAI_BASE_URL}}/videos/task-public-id/content" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  --output result.mp4
```

如响应包含 `last_frame_url`，按该路径下载末帧。例如：

```bash
curl --fail "{{OPENAI_BASE_URL}}/videos/task-public-id/content?part=last_frame" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  --output last-frame.jpg
```

末帧是否提供取决于模型和任务结果，扩展名应按实际媒体类型选择。
内容代理要求任务已成功，并按创建任务的鉴权主体隔离。
下载前先检查 HTTP 状态和 `Content-Type`；失败响应是 JSON，不是视频或图片字节。

视频仅临时可下载，生成成功后请及时下载保存；平台不提供长期视频存储。
可用时间由实际内容和下载地址决定，不保证完成后固定 24 小时可用。没有明确到期时间时，
不能按任务创建或完成时间推算。内容过期后不保证可以恢复，重新查询不会自动延长下载期限。
下载不可用不会把已成功的生成任务改为失败，也不会自动触发退款；有效期内读取失败请保留
本站任务 ID 与请求 ID 联系技术人员核查。

| HTTP 状态     | 常见错误码                                        | 说明                                      |
| ------------- | ------------------------------------------------- | ----------------------------------------- |
| `400`         | `video_not_ready`、`invalid_content_part`         | 任务尚未成功，或 `part` 不是 `last_frame` |
| `404`         | `video_not_found`、`content_not_found`            | 任务、视频或末帧不存在                    |
| `410`         | `video_content_expired`                           | 内容或下载签名已明确到期                  |
| `403` / `502` | `content_url_not_allowed`、`upstream_unavailable` | 内容来源被安全策略阻止或上游读取失败      |

## 素材引用

视频请求可直接使用请求级媒体，也可以使用素材 API 返回的 `reference`。读取当前模型的
`api.assets.management_mode` 区分两种流程：

- `caller_managed_stateless`：普通代理 ID 不在本地验证所有权、ready 状态或作用域，由素材服务判断
  存在性、权限、兼容性与审核；不能通过改模型探测 ID。
- `platform_hosted`：本站托管图片会在视频预扣前校验当前账号归属、删除状态和引用位置。
  只将图片 reference 放入 `image_url.url`，不能当作视频或音频。相同账号的 API Key 共享托管素材，
  但视频任务仍仅属于创建它的 API Key。删除素材只阻止新引用，已受理视频继续执行。

部分模型只能使用直接媒体 URL 或本站托管图片，不能消费普通代理素材 ID。不要手工构造或猜测引用，
以模型发布的素材能力和创建响应为准。

跨模型复用前比较两个模型的 `api.assets.reuse_scope`。只有两个非空值完全相同时才可尝试复用；scope
不同或缺失时不得复用。素材创建、素材组和真人认证的详细参数见[素材与素材组](api-reference/assets)。

控制台模型广场的模型卡片按同一规则显示“素材共享组”短标签：标签相同的模型位于同一素材复用域，
可以把同一个 `asset://` 引用交给同组其它模型尝试；程序应比较完整非空 `reuse_scope`，不能仅比较短标签。
没有标签的模型未发布素材库，也没有可比较的
复用域，不得把其它模型的素材引用交给它。标签只是复用提示，最终存在性、权限和兼容性由当前模型
选定的上游判断。

## 创建结果不明、计费与重试

ModelArk 创建请求最多发送一次上游 POST，当前不接受客户幂等键。发送后结果不明时返回
`create_outcome_unknown`，平台不会自动重发或换模型。客户端必须停止自动创建，保存
`request_id`。创建阶段钱包占款满 24 小时后自动退回，后台每 15 秒检查，退款失败每 30 秒重试；
服务停机、数据库故障或积压可能延迟实际到账。正式任务按任务计费合同处理，不因满 24 小时自动退款。

控制台用量日志的“视频退款进度”显示本人请求，包括没有成功返回任务 ID 的创建占款。程序可用原
API Key 请求 `GET /api/video-funds/token?p=1`，可选 `task_id` 按平台任务或占款请求 ID 精确筛选。
每页 20 条；只返回该 Key 所属用户及应用的数据。金额单位为 quota，`refund_amount_known=false`
表示历史退款额无法核实，零时间表示不适用或未知。退款不代表上游已取消；退款后不会自动补扣。

Funding progress: use the original API key with `GET /api/video-funds/token?p=1`. Creation wallet holds
are released after 24 hours; the worker checks every 15 seconds and retries failures after 30 seconds.
Do not automatically repeat an unknown creation. Successfully accepted tasks follow their normal billing
contract. The response is scoped to the authenticated user and application; amounts use quota units.

任务成功建立后，费用按创建时确定的模型、请求参数和价格规则处理。成功视频可以先交付，结算随后完成：

- 按实际用量计费时，缺少 `usage` 不代表零费用，预扣可能保留到用量补齐后再结算；
- 当前响应会省略零值用量字段，`usage` 也可能为空对象；字段缺失不能区分已确认零用量与尚未报告，
  不得补零作为计费依据，也不能仅凭 `usage` 是否存在判断结算完成；
- 最终费用可能高于或低于预扣，差额按实际规则补扣或退还；不要用预扣值当最终账单；
- 成功后的费用核实不要求重新生成视频，查询也不会重新触发生成。

轮询 GET 可以有界重试；创建 POST 不能因为客户端超时而盲目重放。下载应在内容有效期内完成，
`410 video_content_expired` 不能通过重新查询保证恢复。

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
| `401` / `403` | 鉴权或权限错误                                                                    | 检查 API Key、模型可用状态和分组             |
| `404`         | `task_not_found`                                                                  | 当前应用范围内没有该 ModelArk 任务           |
| `409`         | 取消或删除冲突码                                                                  | 当前任务状态不允许该操作                     |
| `413`         | `request_body_too_large`                                                          | 请求正文或内联媒体超过上限，减少大小后再提交 |
| `429`         | 限流或额度错误                                                                    | 根据错误码区分并退避                         |
| `503`         | `create_outcome_unknown`、`upstream_unavailable`、`cancellation_unknown`          | 不要重发创建；查询或联系管理员核查           |

请求记录相关的 `evidence_*` 错误及重试边界见[错误与重试](concepts/errors#请求记录相关错误)。
