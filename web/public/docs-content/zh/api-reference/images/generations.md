---
page-id: images-generations
kind: api-reference
last-verified: 2026-09-09
operations:
  - createImageGeneration
---

# 图片生成

`POST /v1/images/generations` · Bearer 鉴权 · `application/json`

该接口使用 OpenAI 兼容的图片合同。请求只填写当前 Key 可访问模型公开的字段；默认在本次 HTTP
请求内返回最终图片。模型目录声明异步能力时，可加 `Prefer: respond-async` 请求头显式选择异步受理：
返回 `202` 与平台任务 ID，结果经
[图片任务查询](images/tasks) 获取。

尚未提供平台异步执行的模型会忽略该偏好，继续在本次
请求内返回图片，不因携带此头返回 `400`。此时一并携带的 `Idempotency-Key` 不提供平台任务幂等保证。

## 调用前确认模型

先用当前 API Key 查询模型详情：

```bash
curl "{{OPENAI_BASE_URL}}/models/{{MODEL_ID_PLACEHOLDER}}" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}"
```

模型名包含特殊字符时，须先进行 URL 路径编码。若返回 `available`，确认其为 `true`；同时确认
`api.image.operations` 中 `create_image.supported=true`。生成参数读取
`api.image.creation.parameters`，编辑参数读取 `api.image.edit.parameters`，异步支持读取
`api.image.async`。缺少异步声明时不要假定能得到任务 ID。

`{{OPENAI_BASE_URL}}` 已包含 `/v1`，下列示例不要再追加 `/v1`。
完整接入步骤见[图片与视频调用实战](guides/media-workflow)。

## 最小请求

```bash
curl "{{OPENAI_BASE_URL}}/images/generations" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "{{MODEL_ID_PLACEHOLDER}}",
    "prompt": "雾中灯塔的水彩插画"
  }'
```

## 请求参数

| 字段 | 类型 | 必填 | 取值与说明 |
| --- | --- | --- | --- |
| `model` | string | 是 | `GET /v1/models` 返回且当前 Key 可访问的图片模型 |
| `prompt` | string | 是 | 图片描述或生成指令 |
| `n` | integer | 否 | 输出数量；公共安全上限为 `128`，实际范围读取模型的 `minimum` / `maximum` / `fixed_value`；许多模型固定为 `1`，不能按公共上限批量请求 |
| `size` | string | 否 | 输出尺寸或分辨率；只发送模型目录列出的值，使用小写字母 `x`，不要使用乘号 `×` |
| `quality` | string | 否 | 质量档位；可选值和默认值由模型合同决定 |
| `style` | string | 否 | 风格；公共 OpenAPI 值为 `vivid` 或 `natural`，仅模型公开该字段时可用 |
| `response_format` | string | 否 | `url` 或 `b64_json`；默认行为由模型决定 |
| `user` | string | 否 | 调用方自定义的最终用户标识；仅模型公开该字段时使用 |
| `background` | string | 否 | 背景设置，例如透明背景能力；取值由模型合同决定 |
| `moderation` | string | 否 | 内容审核设置；取值由模型合同决定 |
| `output_format` | string | 否 | 输出文件格式；取值由模型合同决定 |
| `output_compression` | integer | 否 | 输出压缩参数；范围由模型合同决定 |
| `partial_images` | integer | 否 | 流式响应中希望接收的部分图片数量；仅支持流式图片的模型可用 |
| `stream` | boolean | 否 | `true` 返回 SSE；只有模型明确公开流式能力时才能使用 |
| `watermark` | boolean | 否 | 是否添加水印；显式 `false` 会被保留，只有模型公开该字段时可用 |
| `extra_fields` | object | 否 | 只接受模型参数表明确列出的子字段；例如 `extra_fields.aspect_ratio`，不得作为任意参数透传入口 |

### 尺寸、画幅和参考图

- `size` 可能是 `1024x1024` 这类像素尺寸、`1K` / `2K` 这类档位或 `auto`；它们不能相互替换。
  只有模型公开对应值时才发送。不要把 `16:9` 填入像素尺寸字段。
- 参数名 `extra_fields.aspect_ratio` 表示嵌套 JSON；仅当模型公开该字段和 `16:9` 时，才可发送
  `"extra_fields": {"aspect_ratio": "16:9"}`。不能发送名为 `"extra_fields.aspect_ratio"` 的顶层键。
- 图片生成使用文本提示词；参考图请使用[图片编辑](api-reference/images/edits)的 `image` / `images`
  或文件表单，不要在生成请求中添加未公开的参考图字段。

## 异步受理（可选）

已发布异步能力的模型可携带以下请求头：

| 请求头 | 说明 |
| --- | --- |
| `Prefer: respond-async` | 显式选择异步受理；`api.image.async.stream_priority=true` 时 `stream=true` 优先流式响应，不创建任务；其他异步图片模型不接受流式与异步同时使用 |
| `Idempotency-Key` | 可选幂等键，仅异步模式支持；同键等价请求重放原任务 ID，不同请求体返回 `409`，去除首尾空白后最多 191 字节 |

已发布此能力的图片生成与编辑沿用现有模型、参数和 Key。模型详情中的
`api.image.async` 声明请求头与查询路径；`stream_priority=true` 表示流式优先。原生流式请求同时
携带此偏好与幂等键时不创建平台任务，也不提供平台任务幂等保证。

异步受理需要平台已启用私有对象存储及后台图片任务执行。存储不可用返回 `503`，不扣费也不发送上游。

### 提交异步生成

```bash
curl -i "{{OPENAI_BASE_URL}}/images/generations" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  -H "Content-Type: application/json" \
  -H "Prefer: respond-async" \
  -H "Idempotency-Key: image-order-example-001" \
  -d '{
    "model": "{{MODEL_ID_PLACEHOLDER}}",
    "prompt": "雾中灯塔的水彩插画"
  }'
```

幂等键应来自调用方业务订单：新生成意图使用新键，确认同一次请求时使用原键、同一 API Key、
同一接口和相同请求内容。没有 `Prefer: respond-async` 而单独传幂等键会返回
`400 invalid_idempotency_key`。同键不同请求为 `409 idempotency_conflict`；
`409 idempotency_in_progress` 表示原受理尚未确认，稍后用原键确认，不要换键创建。
幂等绑定不是永久订单存档；取得任务 ID 后应保存并改用 GET 查询，不要长期依赖重放 POST 找回任务。

受理成功返回 HTTP `202`，`Location` 响应头和正文 `query_url` 均给出查询路径：

```json
{
  "created": 1785207890,
  "id": "task_xxxxxxxx",
  "object": "image_task",
  "status": "queued",
  "query_url": "/v1/tasks/task_xxxxxxxx"
}
```

保存 `id` 后，使用同一 API Key 查询：

```bash
curl "{{OPENAI_BASE_URL}}/tasks/task_xxxxxxxx" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}"
```

`query_url` 是从站点根开始的路径，应与 `{{SITE_BASE_URL}}` 拼接，不能再与已含 `/v1` 的 Base URL
直接拼接。幂等重放的 `202` 仍可能显示 `queued`，实际最新状态以 GET 为准。

客户端断开不会取消已受理任务。应用未完成任务过多返回 `429`，平台排队容量耗尽返回 `503`；
两者都未受理、未扣费，可稍后重试。

上表是公共字段全集，不表示每个模型都支持所有字段。调用前读取模型详情中的
`api.image.creation.parameters`：

- `required=true` 表示该模型要求字段必填；
- `enum`、`minimum`、`maximum`、`default_value` 和 `fixed_value` 描述该模型的有效范围；
- `additional_properties=false` 表示未列出的字段必须省略。

不要根据模型名称猜测尺寸、质量、数量、参考输入或流式能力。统一图片合同对不支持的字段返回 `400`；
原生兼容入口保留其协议行为，调用方不要依赖未发布字段被忽略或透传。显式的 `false`、`0` 和空字符串是否有意义由对应字段合同决定，不能用“省略”
代替显式零值。

## 非流式响应

HTTP `200` 返回 JSON：

```json
{
  "created": 1760000000,
  "data": [
    {
      "url": "https://example.com/generated-image.png",
      "revised_prompt": "A watercolor lighthouse in the fog"
    }
  ]
}
```

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `created` | integer | 响应创建时间，Unix 秒 |
| `data` | array | 图片结果数组；通常与实际生成数量一致 |
| `data[].url` | string | 临时图片地址；返回该字段时应及时下载或转存 |
| `data[].b64_json` | string | Base64 图片内容；通常在 `response_format=b64_json` 时返回 |
| `data[].revised_prompt` | string | 模型改写后的提示词；并非所有模型都会返回 |
| `metadata` | object | 可选的公开扩展元数据；不要依赖未在模型合同中说明的键 |
| `usage` | object | 部分模型返回的用量信息；不存在时不要自行推算为服务端结算值 |
| `usage.input_tokens` | integer | 可选输入 Token 数 |
| `usage.output_tokens` | integer | 可选输出 Token 数 |
| `usage.total_tokens` | integer | 可选总 Token 数 |
| `usage.input_tokens_details` | object | 可选输入明细，例如文本、图片或缓存 Token；按字段存在性读取 |

单个结果通常在 `url` 和 `b64_json` 中返回一种。客户端应按字段是否存在处理，不要假定某个模型始终返回
同一种格式。错误响应仍是 JSON，不能当作图片字节或 Base64 解码。

## 流式响应

当模型支持且请求发送 `"stream": true` 时，响应类型为 `text/event-stream`。每个 SSE 帧的 `event`
与 JSON 中的 `type` 对应，常见事件为：

```text
event: image_generation.partial_image
data: {"type":"image_generation.partial_image","partial_image_index":0,"b64_json":"..."}

event: image_generation.completed
data: {"type":"image_generation.completed","b64_json":"...","created_at":1760000000}

data: [DONE]
```

| 事件字段 | 说明 |
| --- | --- |
| `type` | `image_generation.partial_image`、`image_generation.completed` 或错误事件类型 |
| `partial_image_index` | 部分图片序号；仅部分事件可能返回 |
| `url` / `b64_json` | 当前图片结果，具体形式由模型决定 |
| `revised_prompt` | 可选的改写提示词 |
| `created_at` | 可选的事件创建时间 |
| `usage` | 可选用量；一般以最后一个有效用量对象为准 |

客户端必须持续读取到 `data: [DONE]` 或连接结束。收到错误事件、HTTP 非 `2xx` 或连接中断时，不要把
已经收到的部分图误认为全部结果。

## 等待、费用与重试

数量、尺寸、质量和模型均可能影响费用。`n` 超过公共或模型上限时会在计费前以 `400` 拒绝。

同步请求需要设置足够的 HTTP 超时；同步 `200` 没有平台任务 ID，也没有任务幂等保证。
超时或网络中断不代表服务端未受理，盲目重复提交可能产生重复图片和费用。

显式异步 `202` 已受理并预扣，之后只查询原任务；如创建响应丢失，只有事先确认支持异步且提交了幂等键，
才能用原键与原内容确认受理结果。`unknown` 表示结果待核实，不能改键重发或当作已退款。
具体状态和结果有效期见[图片任务查询](images/tasks)。

## 错误处理

错误响应通常使用 OpenAI 兼容信封：

```json
{
  "error": {
    "message": "request parameter is invalid",
    "type": "invalid_request_error",
    "param": "size",
    "code": "invalid_request",
    "request_id": "req-placeholder"
  }
}
```

`param`、`code` 和 `request_id` 可能省略；客户端应先检查 HTTP 状态，再读取存在的字段。

| HTTP 状态 | 常见原因 | 处理建议 |
| --- | --- | --- |
| `400` | 缺少字段、字段类型错误、取值超范围或模型不支持该参数 | 修正请求后再提交 |
| `401` / `403` | API Key 无效、模型权限或分组不允许 | 修复鉴权或权限，不重试原请求 |
| `429` | 频率、并发或额度限制 | 区分限流与余额问题；可重试时使用退避 |
| `5xx` | 服务暂时不可用或上游异常 | 保存公开请求 ID；只有能接受重复生成风险时才有限重试 |
