---
page-id: tool-cc-switch
kind: guide
last-verified: 2026-08-26
operations: []
---

# CC Switch

在 CC Switch 中保存 API Key 即可，不需要环境变量。当前网关同时提供原生 Responses 和 Anthropic Messages，优先直连，不需要本地协议转换。

## 给 Codex 使用

1. 打开 `Codex` 页面，点击右上角 `+`，选择 `自定义`。
2. 填写：

| 项目 | 填写内容 |
| --- | --- |
| 供应商名称 | `{{SYSTEM_NAME}}` |
| API Key | `{{API_KEY_PLACEHOLDER}}` |
| API 请求地址 | `{{OPENAI_BASE_URL}}` |
| 默认模型 | `{{MODEL_ID_PLACEHOLDER}}` |

3. 保持原生 Responses 配置，不要开启“需要本地路由映射”。
4. 不要开启 1M 上下文，除非该模型的公开说明明确支持。
5. 保存并启用供应商，完全退出 Codex 后重新打开，再新建任务测试。

## 给 Claude Code 使用

1. 打开 `Claude Code` 页面，点击右上角 `+`，选择 `自定义`。
2. API Endpoint 填 `{{ANTHROPIC_BASE_URL}}`，API Key 填 `{{API_KEY_PLACEHOLDER}}`。
3. API 格式选择原生 `Anthropic Messages`，认证字段选择 `ANTHROPIC_AUTH_TOKEN`。
4. 把默认或回退模型设置为 `{{MODEL_ID_PLACEHOLDER}}`，保存并启用。
5. 完全退出 Claude Code 后重新打开，再新建会话测试。

只有上游协议与客户端协议不一致时才需要本地路由。当前网关已经提供两种原生协议，不要额外转换。

参考：[CC Switch 添加供应商](https://github.com/farion1231/cc-switch/blob/main/docs/user-manual/zh/2-providers/2.1-add.md)。
