---
page-id: quickstart
kind: guide
last-verified: 2026-07-29
operations: []
---

# 快速开始

下面的请求使用 OpenAI 兼容入口完成一次最小文本调用。

## 准备 API Key

在控制台创建 API Key，并只把它保存在服务端环境变量或密钥管理系统中。不要把 Key 提交到仓库、浏览器脚本、移动端安装包或日志。

## 查询可用模型

```bash
curl "{{OPENAI_BASE_URL}}/models" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}"
```

从响应的 `data[].id` 中选择当前 Key 可访问的模型。不要假设部署一定提供某个固定模型。

## 发起第一次请求

```bash
curl "{{OPENAI_BASE_URL}}/chat/completions" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "{{MODEL_ID_PLACEHOLDER}}",
    "messages": [
      {"role": "user", "content": "用一句话介绍你自己"}
    ]
  }'
```

成功响应通常包含 `choices` 和 `usage`。实际字段取决于你选择的公开协议，不应跨协议解析。

## 下一步

- 需要流式输出：在请求中加入 `"stream": true`，并按 SSE 读取。
- 使用 Anthropic SDK：改用 [文本与模型](api-reference/text) 中的 Messages 入口。
- 收到非 2xx：先阅读 [错误与重试](concepts/errors)。
- 使用媒体任务：先阅读 [异步任务](concepts/async-tasks)。
