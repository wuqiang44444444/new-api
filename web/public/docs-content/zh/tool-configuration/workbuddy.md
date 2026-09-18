---
page-id: tool-workbuddy
kind: guide
last-verified: 2026-09-09
operations: []
---

# WorkBuddy

本文按官方配置说明核对，未声明某个客户端版本已经实机验收。设置名称和界面位置可能随版本变化；
升级后先用一条最小会话验证连接，并确认所选客户模型支持该工具需要的协议与工具调用。

API Key 直接保存在 WorkBuddy 的本地模型设置中，不需要环境变量。

## 添加自定义模型

1. 打开对话输入框下方的模型选择器，选择“配置自定义模型”。
2. 若所用版本将此入口放在设置页，进入模型设置并找到添加自定义模型。
3. 按下面填写：

| 项目       | 填写内容                               |
| ---------- | -------------------------------------- |
| Provider   | `Custom`                               |
| Endpoint   | `{{OPENAI_BASE_URL}}/chat/completions` |
| API Key    | `{{API_KEY_PLACEHOLDER}}`              |
| Model Name | `{{MODEL_ID_PLACEHOLDER}}`             |

4. 只有模型明确支持时，才在高级选项中开启 `Tool Calling`、`Image Input` 或 `Reasoning`。
5. 保存，选择刚添加的模型，然后新建会话测试。

Endpoint 必须是完整的 Chat Completions 地址，不要只填写站点根地址，也不要改成 `/responses`。

参考：[腾讯云 WorkBuddy 自定义模型配置](https://intl.cloud.tencent.com/zh/document/product/1300/81046)。
