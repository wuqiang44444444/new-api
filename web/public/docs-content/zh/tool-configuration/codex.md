---
page-id: tool-codex
kind: guide
last-verified: 2026-08-27
operations: []
---

# Codex

按下面两份文件配置。API Key 保存在本机，不需要设置环境变量。

## 1. 配置 Codex

打开 `~/.codex/config.toml`。如果文件已经存在，只添加或修改下面四项，不要删除其他配置；修改前建议先备份。

```toml
model_provider = "openai"
model = "{{MODEL_ID_PLACEHOLDER}}"
openai_base_url = "{{OPENAI_BASE_URL}}"
cli_auth_credentials_store = "file"
```

## 2. 保存 API Key

打开 `~/.codex/auth.json`，写入：

```json
{
  "auth_mode": "apikey",
  "OPENAI_API_KEY": "{{API_KEY_PLACEHOLDER}}"
}
```

如果文件已经存在，修改前先备份。写入这份内容会把当前登录切换为网关 API Key，不要把旧的 ChatGPT 登录字段混合进来。

macOS、Linux 或 WSL 再执行：

```bash
chmod 600 ~/.codex/auth.json
```

Windows 的目录是 `%USERPROFILE%\.codex`。

## 3. 重新启动

完全退出 Codex，重新打开后新建任务测试。不要继续使用配置修改前的旧任务。

不要添加 `disable_response_storage`，也不要自行填写 `model_context_window` 或 `model_auto_compact_token_limit`。这些值必须由实际模型能力决定。

参考：[OpenAI Codex 高级配置](https://learn.chatgpt.com/docs/config-file/config-advanced)和[认证说明](https://learn.chatgpt.com/docs/auth)。
