---
page-id: images-edits
kind: api-reference
last-verified: 2026-08-28
operations:
  - createImageEdit
---

# 图片编辑

`POST /v1/images/edits` · Bearer 鉴权 · `multipart/form-data`

该接口上传一张或多张源图片，并默认在本次 HTTP 请求内返回编辑结果。模型目录声明异步能力时，
可加 `Prefer: respond-async` 请求头显式异步受理（`202` + 任务 ID，与 `stream=true` 互斥），
结果经[图片任务查询](images/tasks)获取。不要手工设置 multipart boundary；
让 HTTP 客户端根据表单自动生成 `Content-Type`。

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

## 请求参数

| 表单字段 | 类型 | 必填 | 取值与说明 |
| --- | --- | --- | --- |
| `model` | string | 是 | 当前 Key 可访问且明确支持图片编辑的模型 |
| `prompt` | string | 是 | 编辑指令 |
| `image` | file，可重复 | 是 | 一张或多张待编辑图片；文件数量、格式和大小由模型合同决定 |
| `mask` | file | 否 | 编辑区域遮罩；只有模型公开遮罩能力时可用 |
| `n` | integer string | 否 | 输出数量，`1`～`128`，默认 `1`；模型可以有更低上限 |
| `size` | string | 否 | 输出尺寸；只发送模型公开的值 |
| `response_format` | string | 否 | `url` 或 `b64_json`；实际支持范围由模型决定 |
| `quality` | string | 否 | 输出质量档位 |
| `input_fidelity` | string | 否 | 输入保真设置；仅公开该字段的模型可用 |
| `stream` | boolean string | 否 | 表单值必须是 `true` 或 `false`；仅支持流式编辑的模型可用 |
| `watermark` | boolean string | 否 | 表单值为 `true` 或 `false`；仅公开该字段的模型可用 |

`api.image.creation` 描述的是图片生成入口，不能单独证明同一模型支持编辑、遮罩或多图。编辑调用应以
本页字段、当前部署公开的模型说明和管理员确认的服务能力为准；不支持的遮罩、多图、尺寸、质量或字段
组合会以 `400` 拒绝，不会被静默删除或改写。

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

图片编辑不会返回图片任务 ID，也没有后续查询接口。multipart 上传和生成可能耗时较长，应设置合理的
上传与响应超时。网络中断后，只有确认服务端未受理时才重试；否则重复提交可能产生重复结果和费用。

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
