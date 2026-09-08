---
page-id: text-api
kind: api-reference
last-verified: 2026-09-09
operations:
  - listModels
  - retrieveModel
  - createChatCompletion
  - createResponse
  - createMessage
---

# 文本与模型

文本 API 提供原生 OpenAI Chat Completions、OpenAI Responses 和 Anthropic Messages 三种协议。选择与客户端一致、且目标模型支持的入口；视频与素材的扩展协议不改变这些文本接口的请求和响应。

## 查询模型

`GET /v1/models` · Bearer 鉴权

```bash
curl "{{OPENAI_BASE_URL}}/models" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}"
```

默认返回 OpenAI 格式。带 Anthropic 或 Gemini 协议头时响应格式可能相应变化；本页后续示例均使用默认 OpenAI 视图。

图片和视频模型还会返回机器可读合同：

- `supported_endpoint_types` 指明可调用的北向入口；
- `api.image.creation` 或 `api.video.creation` 指明方法、路径、内容类型和必填字段；
- `parameters` 是该客户模型允许的完整字段列表，包含类型、固定值、默认值、枚举和上下限；
- `additional_properties=false` 表示列表外字段不受支持；视频的 `content_types` 另列媒体类型、角色与数量。

`GET /v1/models/{model}` 与列表中的同名条目使用同一合同。不要从模型后缀猜测参数，也不要发送目录
没有登记的字段。完整的模型可用性、参数默认值和互选输入说明见[模型与参数](concepts/model-parameters)。

## Chat Completions

`POST /v1/chat/completions` · `application/json`

```bash
curl "{{OPENAI_BASE_URL}}/chat/completions" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "{{MODEL_ID_PLACEHOLDER}}",
    "messages": [{"role": "user", "content": "你好"}],
    "stream": false
  }'
```

`model` 和 `messages` 必填。仅使用该模型与所选文本协议支持的参数；`0`、`false` 与未填写含义不同，不要在客户端序列化时丢弃显式值。

## Responses

`POST /v1/responses` · `application/json`

```bash
curl "{{OPENAI_BASE_URL}}/responses" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "{{MODEL_ID_PLACEHOLDER}}",
    "input": "解释什么是幂等"
  }'
```

Responses 的 `input` 和输出条目与 Chat Completions 的 `messages`、`choices` 不同。不要跨接口复用响应类型。

## Anthropic Messages

`POST /v1/messages` · `application/json`

```bash
curl "{{ANTHROPIC_BASE_URL}}/v1/messages" \
  -H "x-api-key: {{API_KEY_PLACEHOLDER}}" \
  -H "anthropic-version: 2023-06-01" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "{{MODEL_ID_PLACEHOLDER}}",
    "max_tokens": 256,
    "messages": [{"role": "user", "content": "你好"}]
  }'
```

`max_tokens` 必填且必须为正整数。流式事件也遵循 Messages 协议，不应按 OpenAI SSE 数据结构解析。

## 如何读取结果与流式输出

| 协议 | 非流式正文 | 流式处理 |
| --- | --- | --- |
| Chat Completions | 读取 `choices[].message`；文本通常在 `content`，工具调用在 `tool_calls` | 设置 `stream=true`，按 SSE 的 `choices[].delta` 累积文本或工具参数 |
| Responses | 按 `output[]` 条目的 `type` 处理消息和工具调用；文本在消息的 `content[]` 中 | 设置 `stream=true`，按事件类型处理文本增量、工具调用和完成事件 |
| Messages | 按 `content[]` 块的 `type` 区分 `text` 与 `tool_use`，同时检查 `stop_reason` | 设置 `stream=true`，处理消息、内容块增量与结束事件 |

工具调用结果不等于最终文本；应用执行工具后，需按原协议提交对应的工具结果，再继续读取回答。
没有文本时先检查工具调用和结束原因，不要立即重发完整请求。

curl 查看流式输出时使用 `-N` 禁用客户端缓冲，并在原 JSON 中增加 `"stream": true`。
网络分块不等于完整 SSE 事件，应按空行分隔事件后解析；连接中断不代表已正常完成，也不保证没有产生费用。
通用客户端不应把三个协议的终止事件和用量字段混为一套格式。

## 计费与错误

实际计费由模型、输入输出用量、缓存和工具调用等因素决定。生产调用应记录协议、模型、请求 ID 与公开 usage，不要根据文本长度自行推断最终费用。
