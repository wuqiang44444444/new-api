---
page-id: images-edits
kind: api-reference
last-verified: 2026-09-16
operations:
  - createImageEdit
---

# 图片编辑

`POST /v1/images/edits` · Bearer 鉴权 · `multipart/form-data` 或模型声明的 `application/json`

该接口上传一张或多张源图片，并默认在本次 HTTP 请求内返回编辑结果。模型目录声明异步能力时，
可加 `Prefer: respond-async` 请求头显式异步受理（`202` + 任务 ID），
结果经[图片任务查询](images/tasks)获取。不要手工设置 multipart boundary；
让 HTTP 客户端根据表单自动生成 `Content-Type`。

先确认 `api.image.operations` 中 `edit_image.supported=true`，再读取 `api.image.edit` 的输入字段、
数量和格式。JSON／multipart、源图顺序及模型支持的 mask 以该编辑合同为准。
`api.image.async.stream_priority=false` 表示流式不优先于异步偏好，不代表模型支持 `stream` 参数。
OpenAI／Azure 原生图片入口同时传 `stream=true` 时，通过受理检查后返回 `202`，由后台接收结果；
Gemini／Vertex／图片中转入口仍拒绝这一组合。受理后的幂等键提供平台任务幂等保证。
异步受理需要平台私有对象存储；存储不可用返回 `503`，
不扣费也不发送上游。

尚未提供平台异步执行的模型会忽略该偏好，继续在本次
请求内返回编辑结果，不因携带此头返回 `400`。此时一并携带的 `Idempotency-Key` 不提供平台任务幂等保证。

## 最小请求

```bash
curl "{{OPENAI_BASE_URL}}/images/edits" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  -F "model={{MODEL_ID_PLACEHOLDER}}" \
  -F "prompt=把天空改成日落" \
  -F "image=@input.png" \
  -F "n=1"
```

多图编辑时重复发送同名 `image` 字段：

```bash
curl "{{OPENAI_BASE_URL}}/images/edits" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  -F "model={{MODEL_ID_PLACEHOLDER}}" \
  -F "prompt=把第二张图中的商品放到第一张图的桌面上" \
  -F "image=@scene.png" \
  -F "image=@product.png"
```

## JSON 参考图输入

当 `api.image.edit` 声明支持 JSON `image` / `images` 时，单图使用 `image` 字符串，多图使用
`images` 数组，两者互斥。以下为多图的完整请求；输入顺序与提示词中的“第一张”“第二张”对应：

```bash
curl "{{OPENAI_BASE_URL}}/images/edits" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "{{MODEL_ID_PLACEHOLDER}}",
    "prompt": "把第二张图中的杯子放到第一张图的桌面上，保持杯子外观",
    "images": ["https://example.com/scene.png", "https://example.com/cup.png"]
  }'
```

单图也可用一个元素的 `images` 数组：

```json
{
  "model": "{{MODEL_ID_PLACEHOLDER}}",
  "prompt": "将杯子改为红色，保持构图",
  "images": ["https://example.com/reference.png"],
  "n": 1,
  "response_format": "url"
}
```

上述统一参考图输入接受 HTTPS URL 或 JPEG/PNG/WebP Data URL。Data URL 的形状为
`data:image/png;base64,<完整图片的Base64>`，不要只传没有 MIME 前缀的 Base64 字符串。
文件表单可使用重复的 `image` 或 `image[]`，不要在同一请求混用多套字段。

输出数量与尺寸读取模型合同，不能把参考图数量当作输出 `n`。统一参考图输入最多 14 张，模型可限制到
10 张或更少，以 `api.image.edit.parameters` 为准。字节输入每张最多 20 MiB，
总计最多 50 MiB；部分模型单张限制更小，为 10,000,000 字节。这些参考图 URL 不由网关主动下载校验，
调用方须保证格式、尺寸和处理期间的可访问性。未发布的遮罩、流式及其他字段不支持；只有编辑合同明确发布 `mask` 时才能上传遮罩。

默认同步返回 `data[]`，按模型和请求返回 URL 或 Base64；支持异步时加 `Prefer: respond-async` 后返回 202，再查询任务获得结果。异步结果存入
平台对象存储，签名有效期为 300 秒，到期可重新查询。需要 URL 输入的服务会将字节参考图暂存私有
对象存储，在实际发送时签发至少两小时的输入 URL；排队不会消耗该有效期。对象存储不可用时，
需要上传的同步编辑及所有异步受理失败。同步结果 URL 可能来自上游，并不表示已由平台保存。

参考图数量可能影响费用；多图请求因价格尚未配置被拒绝时，联系管理员确认服务可用性。
只使用当前部署已经开放并验证的模型规格。

## 异步编辑完整示例

先确认模型存在 `api.image.async`。与同步编辑使用相同正文，只增加异步偏好和可选幂等键：

```bash
curl -i "{{OPENAI_BASE_URL}}/images/edits" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  -H "Prefer: respond-async" \
  -H "Idempotency-Key: edit-order-example-001" \
  -F "model={{MODEL_ID_PLACEHOLDER}}" \
  -F "prompt=把天空改成日落，保留建筑外观" \
  -F "image=@input.png"
```

成功为 HTTP `202`，正文包含 `id`、`object=image_task`、`status=queued` 和
`query_url=/v1/tasks/{task_id}`。保存 ID，使用同一 API Key 调用 `GET /v1/tasks/{task_id}`。
不支持异步的模型仍可能返回同步 `200`，客户端必须实际检查状态码和正文。

重放同一次编辑必须保持 API Key、接口、提示词、参数、文件内容及顺序不变。表单自动生成的 boundary
变化不影响幂等；修改图片或提示词属于新请求，同键提交会返回 `409`。
仅传 `Idempotency-Key` 而没有异步偏好会返回 `400`。更多规则见
[图片生成](api-reference/images/generations)和[图片任务查询](images/tasks)。

## 请求参数

| 表单字段 | 类型 | 必填 | 取值与说明 |
| --- | --- | --- | --- |
| `model` | string | 是 | 当前 Key 可访问且明确支持图片编辑的模型 |
| `prompt` | string | 是 | 编辑指令 |
| `image` | file，可重复 | 是 | 一张或多张待编辑图片；文件数量、格式和大小由模型合同决定 |
| `mask` | file | 否 | 编辑区域遮罩；只有模型公开遮罩能力时可用 |
| `n` | integer string | 否 | 输出数量，按模型的 `minimum` / `maximum` / `fixed_value`；公共安全上限 `128` 不代表模型可生成这么多图片 |
| `size` | string | 否 | 输出尺寸；只发送模型公开的值 |
| `response_format` | string | 否 | `url` 或 `b64_json`；实际支持范围由模型决定 |
| `quality` | string | 否 | 输出质量档位 |
| `input_fidelity` | string | 否 | 输入保真设置；仅公开该字段的模型可用 |
| `background` / `moderation` | string | 否 | 背景和内容审核档位，按模型发布值填写 |
| `output_format` | string | 否 | 输出文件格式，按模型发布值填写 |
| `output_compression` | integer string | 否 | 输出压缩参数 `0`～`100`，仅支持时使用 |
| `partial_images` | integer string | 否 | 流式部分图片数量 `0`～`3`，仅支持时使用 |
| `user` | string | 否 | 调用方最终用户标识，仅模型发布时使用 |
| `extra_fields` | object / JSON string | 否 | JSON 使用对象，表单使用 JSON 文本；只发送模型发布的子字段 |
| `stream` | boolean string | 否 | 表单值必须是 `true` 或 `false`；仅支持流式编辑的模型可用 |
| `watermark` | boolean string | 否 | 表单值为 `true` 或 `false`；仅公开该字段的模型可用 |

`api.image.creation` 描述的是图片生成入口，不能单独证明编辑能力；`api.image.edit` 描述编辑输入。编辑调用应以
本页字段、当前部署公开的模型说明和管理员确认的服务能力为准；不支持的遮罩、多图、尺寸、质量或字段
组合在统一参考图合同中以 `400` 拒绝；原生编辑按其公开协议处理，不能依赖未发布的行为。

JSON 的 `image` 为字符串，`images` 为字符串数组；表单的 `image` 为文件。JSON 中的 `n` 用整数、
`stream` 用布尔值，表单中对应值为文本。`required_one_of` 描述互选必填项，不能将 `image` 和 `images`
同时提交。生成入口的参数表不能代替编辑入口的参数表。

## 文件要求

- 文件 MIME、扩展名、单文件大小、总请求大小、透明通道和图片数量由所选模型决定；
- 遮罩图的尺寸和透明规则由模型决定，不能假定所有编辑模型都兼容 OpenAI 的同一遮罩语义；
- 多图必须保持业务顺序，客户端不要依赖服务端重新排序；
- 不要在日志中记录图片二进制、Base64、完整敏感提示词或临时下载地址。

## 非流式响应

HTTP `200` 与图片生成使用相同的 JSON 结构：

```json
{
  "created": 1760000000,
  "data": [
    {
      "b64_json": "...",
      "revised_prompt": "Place the product on the table at sunset"
    }
  ]
}
```

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `created` | integer | 响应创建时间，Unix 秒 |
| `data` | array | 编辑结果数组 |
| `data[].url` | string | 临时图片地址；存在时应及时下载或转存 |
| `data[].b64_json` | string | Base64 图片内容 |
| `data[].revised_prompt` | string | 可选的模型改写提示词 |
| `metadata` | object | 可选公开扩展元数据 |
| `usage` | object | 部分模型返回的可选用量信息，常见子字段为 `input_tokens`、`output_tokens` 和 `total_tokens` |

单项通常返回 `url` 或 `b64_json` 之一。客户端必须先检查 HTTP 状态和 `Content-Type`，错误响应是 JSON，
不能当作图片保存或解码。

## 流式响应

当模型支持且表单发送 `stream=true` 时，响应为 `text/event-stream`。编辑模型可能直接返回
`image_edit.completed` 事件；对于只能返回普通 JSON 的兼容实现，中转站会将每个最终结果转换为
`image_generation.completed` 事件。客户端应读取 JSON 的 `type`，同时兼容这两种终态事件：

```text
event: image_edit.completed
data: {"type":"image_edit.completed","b64_json":"..."}

data: [DONE]
```

部分结果可能使用 `image_generation.partial_image`。每个事件可包含 `url`、`b64_json`、
`revised_prompt`、`created_at` 和 `usage`；字段是否存在由模型响应决定。只有读到 `[DONE]` 或明确终态
后，才能认为流已正常结束。

## 等待、费用与重试

同步图片编辑不创建平台任务；multipart 上传和生成可能耗时较长，应设置合理的上传与响应超时。
显式异步受理返回 `202` 后，客户端断开不会取消任务，使用已保存 ID 继续查询。

网络中断后不要盲目重复编辑。实际异步模式下可以用原幂等键及相同内容确认；同步、流式或未携带幂等键
的创建没有这项保证。任务进入 `unknown` 时保留原任务并联系管理员核实。

## 错误处理

错误响应通常使用 OpenAI 兼容信封：

```json
{
  "error": {
    "message": "request parameter is invalid",
    "type": "invalid_request_error",
    "param": "image",
    "code": "invalid_request",
    "request_id": "req-placeholder"
  }
}
```

`param`、`code` 和 `request_id` 可能省略；客户端应先检查 HTTP 状态，再读取存在的字段。

| HTTP 状态 | 常见原因 | 处理建议 |
| --- | --- | --- |
| `400` | multipart 无效、缺文件、字段类型错误、数量超限或模型不支持该组合 | 修正表单后再提交 |
| `401` / `403` | API Key、模型权限或分组不允许 | 修复鉴权或权限 |
| `413` | 请求体或文件超过部署限制 | 压缩文件或减少数量，不要原样重试 |
| `429` | 频率、并发或额度限制 | 根据错误码判断并退避 |
| `5xx` | 服务暂时不可用或上游异常 | 保存公开请求 ID；评估重复编辑风险后再决定是否重试 |
