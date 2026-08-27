---
page-id: tool-claude-code
kind: guide
last-verified: 2026-08-28
operations: []
---

# Claude Code

只需要修改一个本地文件，不需要在终端设置环境变量。

## 1. 写入设置

打开 `~/.claude/settings.json`。如果文件已经存在，请把下面字段合并进去，不要覆盖原有的权限、插件或 Hook；下面是新文件的完整示例：

```json
{
  "$schema": "https://json.schemastore.org/claude-code-settings.json",
  "env": {
    "ANTHROPIC_BASE_URL": "{{ANTHROPIC_BASE_URL}}",
    "ANTHROPIC_AUTH_TOKEN": "{{API_KEY_PLACEHOLDER}}",
    "ANTHROPIC_MODEL": "{{MODEL_ID_PLACEHOLDER}}",
    "ANTHROPIC_DEFAULT_FABLE_MODEL": "{{MODEL_ID_PLACEHOLDER}}",
    "ANTHROPIC_DEFAULT_OPUS_MODEL": "{{MODEL_ID_PLACEHOLDER}}",
    "ANTHROPIC_DEFAULT_SONNET_MODEL": "{{MODEL_ID_PLACEHOLDER}}",
    "ANTHROPIC_DEFAULT_HAIKU_MODEL": "{{MODEL_ID_PLACEHOLDER}}",
    "API_TIMEOUT_MS": "3000000",
    "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1"
  }
}
```

- Base URL 直接使用本文给出的地址，不要追加 `/v1` 或 `/v1/messages`；Claude Code 会请求 `/v1/messages`。`ANTHROPIC_AUTH_TOKEN` 使用 Bearer 鉴权。
- `ANTHROPIC_MODEL` 与四个 `ANTHROPIC_DEFAULT_*_MODEL` 把主会话和 Opus、Sonnet、Haiku、Fable 档位都固定到网关上存在的模型，避免 Plan 模式、后台任务或模型自动回退请求网关中不存在的默认 Claude 模型；网关提供多个模型时可按档位分别填写。
- `API_TIMEOUT_MS` 默认 600000（10 分钟），走网关的长请求可以按需调大。
- `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC` 设为 `1` 关闭自动更新、遥测等非必要外联；它只判断变量是否非空，设 `0` 也不会恢复，需要删除该变量。
- 可选：`ANTHROPIC_DEFAULT_*_MODEL_NAME` 只改变 `/model` 选择器里的显示名称，不影响实际请求的模型。

macOS、Linux 或 WSL 建议执行：

```bash
chmod 600 ~/.claude/settings.json
```

Windows 的目录是 `%USERPROFILE%\.claude`。

## 2. 重新启动

完全退出 Claude Code，重新打开后新建会话测试。

参考：[Claude Code 环境变量](https://code.claude.com/docs/en/env-vars)、[设置文件](https://code.claude.com/docs/en/settings)和[模型配置](https://code.claude.com/docs/en/model-config)。
