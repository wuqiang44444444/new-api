---
page-id: tool-claude-code
kind: guide
last-verified: 2026-09-09
operations: []
---

# Claude Code

本文按官方配置说明核对，未声明某个客户端版本已经实机验收。设置名称和界面位置可能随版本变化；
升级后先用一条最小会话验证连接，并确认所选客户模型支持该工具需要的协议与工具调用。

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
    "ANTHROPIC_DEFAULT_HAIKU_MODEL": "{{MODEL_ID_PLACEHOLDER}}"
  }
}
```

- Base URL 直接使用本文给出的地址，不要追加 `/v1` 或 `/v1/messages`；Claude Code 会请求 `/v1/messages`。`ANTHROPIC_AUTH_TOKEN` 使用 Bearer 鉴权。
- `ANTHROPIC_MODEL` 指定默认主模型，四个 `ANTHROPIC_DEFAULT_*_MODEL` 配置相应档位。提供多个客户模型时可分别填写；这些设置不能保证覆盖命令行、项目设置或显式 `fallbackModel` 指定的其他模型。发生模型不存在错误时，检查实际请求的模型 ID。

以下为可选设置，按需加入同一个 `env` 对象，不是连通网关的前提：

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
