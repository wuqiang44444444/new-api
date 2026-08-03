---
page-id: base-url
kind: guide
last-verified: 2026-07-29
operations: []
---

# Base URL

当前部署的 OpenAI 兼容 Base URL 是：

```text
{{OPENAI_BASE_URL}}
```

Anthropic Messages Base URL 是：

```text
{{ANTHROPIC_BASE_URL}}
```

## OpenAI 兼容客户端

把 SDK 的 `baseURL` 或 `base_url` 设置为 `{{OPENAI_BASE_URL}}`。随后 SDK 会在其后追加 `/models`、`/chat/completions` 或 `/responses`。

## Anthropic 兼容客户端

把客户端的 Base URL 设置为 `{{ANTHROPIC_BASE_URL}}`，请求路径使用 `/v1/messages`。

## 平台原生入口

ModelArk 视频使用站点根地址下的 `/api/v3/...`，Kling 使用 `/kling/v1/...`，即梦使用 `/jimeng/`。这些路径不是 `/v1` 的子路径，示例会直接给出完整动态 URL。

## 反向代理与路径前缀

如果管理员配置了公开 `server_address`，文档优先使用该地址；否则使用当前页面 origin。地址末尾的 `/v1` 只会在生成协议 Base URL 时规范化一次，不会重复追加。
