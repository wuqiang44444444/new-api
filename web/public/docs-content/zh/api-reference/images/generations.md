---
page-id: images-generations
kind: api-reference
last-verified: 2026-08-28
operations:
  - createImageGeneration
---

# 图片生成

`POST /v1/images/generations` · Bearer 鉴权 · `application/json`

该接口使用 OpenAI 兼容的同步图片合同。请求只填写当前 Key 可访问模型公开的字段；普通响应会在本次
HTTP 请求内返回最终图片，不会返回可供稍后查询的图片任务 ID。

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
| `n` | integer | 否 | 生成数量，`1`～`128`，默认 `1`；模型可以有更低上限 |
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

上表是公共字段全集，不表示每个模型都支持所有字段。调用前读取模型详情中的
`api.image.creation.parameters`：

- `required=true` 表示该模型要求字段必填；
- `enum`、`minimum`、`maximum`、`default` 和固定值描述该模型的有效范围；
- `additional_properties=false` 表示未列出的字段必须省略。

不要根据模型名称猜测尺寸、质量、数量、参考输入或流式能力。不支持的字段会以 `400` 拒绝，不会被
静默删除、钳制或改义。显式的 `false`、`0` 和空字符串是否有意义由对应字段合同决定，不能用“省略”
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

非流式请求需要设置足够的 HTTP 超时。超时或网络中断不代表服务端一定未受理；该接口没有图片任务查询
和客户幂等键，盲目重复提交可能产生重复图片和费用。

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
