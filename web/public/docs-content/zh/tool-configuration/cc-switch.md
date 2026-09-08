---
page-id: tool-cc-switch
kind: guide
last-verified: 2026-09-09
operations: []
---

# CC Switch

本文按官方配置说明核对，未声明某个客户端版本已经实机验收。设置名称和界面位置可能随版本变化；
升级后先用一条最小会话验证连接，并确认所选客户模型支持该工具需要的协议与工具调用。

在 CC Switch 中保存 API Key 即可，不需要环境变量。当前网关提供 Responses 和 Anthropic Messages 入口。先确认所选模型支持目标协议，再使用相应的原生直连配置。

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
4. 把默认或回退模型以及 Opus、Sonnet、Haiku 模型映射都设置为 `{{MODEL_ID_PLACEHOLDER}}`，保存并启用。
5. 完全退出 Claude Code 后重新打开，再新建会话测试。

选择支持目标协议的客户模型后，以上直连配置无需额外转换。若该版本提供 Fable 或其他默认、回退模型设置，也需填入可用的客户模型 ID。

切换完成后检查实际生效的 Base URL 和模型，新建会话发送一条简短请求。遇到 `401/403` 核对 Key；
模型不存在时核对模型映射；`404` 或 HTML 响应先核对地址是否重复添加协议路径。不要把含 Key 的配置导出文件用于公开排障。

参考：[CC Switch 添加供应商](https://github.com/farion1231/cc-switch/blob/main/docs/user-manual/zh/2-providers/2.1-add.md)。
