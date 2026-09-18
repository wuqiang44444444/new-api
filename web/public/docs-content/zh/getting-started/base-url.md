---
page-id: base-url
kind: guide
last-verified: 2026-09-09
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

直接复制本文显示的地址。部署在路径前缀下时，应保留该前缀；不要仅凭域名重新拼接地址。

| 使用位置                                  | 应填写的地址                           |
| ----------------------------------------- | -------------------------------------- |
| OpenAI SDK Base URL                       | `{{OPENAI_BASE_URL}}`                  |
| Anthropic SDK Base URL                    | `{{ANTHROPIC_BASE_URL}}`               |
| 需要完整 Chat Completions Endpoint 的工具 | `{{OPENAI_BASE_URL}}/chat/completions` |
| 图片任务返回的 `/v1/tasks/...` 查询路径   | 与 `{{SITE_BASE_URL}}` 拼接            |
| ModelArk V3 任务路径                      | 与 `{{SITE_BASE_URL}}` 拼接            |

遇到 `404` 或 HTML 响应，先检查是否重复添加 `/v1`、漏掉部署前缀，或将 Base URL 填进了要求完整
Endpoint 的字段。本文地址是 API 服务地址，不是控制台登录页地址。
