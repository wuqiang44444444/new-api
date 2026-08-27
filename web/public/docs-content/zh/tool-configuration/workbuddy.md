---
page-id: tool-workbuddy
kind: guide
last-verified: 2026-08-26
operations: []
---

# WorkBuddy

API Key 直接保存在 WorkBuddy 的本地模型设置中，不需要环境变量。

## 添加自定义模型

1. 打开左下角账号菜单，进入 `Settings`。
2. 选择 `Models` → `Custom Models` → `Add Model`。
3. 按下面填写：

| 项目 | 填写内容 |
| --- | --- |
| Provider | `Custom` |
| Endpoint | `{{OPENAI_BASE_URL}}/chat/completions` |
| API Key | `{{API_KEY_PLACEHOLDER}}` |
| Model Name | `{{MODEL_ID_PLACEHOLDER}}` |

4. 只有模型明确支持时，才在高级选项中开启 `Tool Calling`、`Image Input` 或 `Reasoning`。
5. 保存，选择刚添加的模型，然后新建会话测试。

Endpoint 必须是完整的 Chat Completions 地址，不要只填写站点根地址，也不要改成 `/responses`。

参考：[腾讯云 WorkBuddy 自定义模型配置](https://intl.cloud.tencent.com/document/product/1300/80640)。
