---
page-id: text-api
kind: api-reference
last-verified: 2026-07-29
operations:
  - listModels
  - retrieveModel
  - createChatCompletion
  - createResponse
  - createMessage
---

# 文本与模型

文本能力在 Link 合同中提供 OpenAI Chat Completions、OpenAI Responses 和 Anthropic Messages 三种明确的客户 API 协议。选择一种与你的 SDK 和响应解析逻辑一致的协议。

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
没有登记的字段。

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

`model` 和 `messages` 必填。可选标量会保留显式的 `0` 或 `false`；不要用未知字段传递渠道私有配置。

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

## 计费与错误

实际计费由模型、输入输出用量、缓存和工具调用等因素决定。生产调用应记录协议、模型、请求 ID 与公开 usage，不要根据文本长度自行推断最终费用。
