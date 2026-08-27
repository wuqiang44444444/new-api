---
page-id: tool-claude-code
kind: guide
last-verified: 2026-08-26
operations: []
---

# Claude Code

只需要修改一个本地文件，不需要在终端设置环境变量。

## 1. 写入设置

打开 `~/.claude/settings.json`，写入：

```json
{
  "$schema": "https://json.schemastore.org/claude-code-settings.json",
  "model": "{{MODEL_ID_PLACEHOLDER}}",
  "env": {
    "ANTHROPIC_BASE_URL": "{{ANTHROPIC_BASE_URL}}",
    "ANTHROPIC_AUTH_TOKEN": "{{API_KEY_PLACEHOLDER}}"
  }
}
```

这里的 Base URL 不要追加 `/v1` 或 `/v1/messages`；Claude Code 会请求 `/v1/messages`。`ANTHROPIC_AUTH_TOKEN` 会使用 Bearer 鉴权。

macOS、Linux 或 WSL 建议执行：

```bash
chmod 600 ~/.claude/settings.json
```

Windows 的目录是 `%USERPROFILE%\.claude`。

## 2. 重新启动

完全退出 Claude Code，重新打开后新建会话测试。

参考：[Claude Code 环境变量](https://code.claude.com/docs/en/env-vars)、[设置文件](https://code.claude.com/docs/en/settings)和[模型配置](https://code.claude.com/docs/en/model-config)。
