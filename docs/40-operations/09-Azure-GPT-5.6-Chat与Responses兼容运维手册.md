---
status: current
owner: Dev Team
last-reviewed: 2026-11-12
---

# 09 Azure GPT-5.6 Chat 与 Responses 兼容运维手册

## 1. 适用范围与故障信号

本手册用于 Azure 渠道的 Chat 请求携带推理等级与函数工具，且上游返回 HTTP 400、要求使用
Responses 的排查。用等价测试请求复现：

```text
POST /v1/chat/completions
model = gpt-5.6-sol
reasoning_effort = <推理等级>
tools = <function tools>
```

记录实际渠道、上游部署、API Version 和脱敏错误码。用相同参数复测，普通渠道探针无法覆盖推理与工具组合。

## 2. 原因

### 2.1 直接原因

客户端选择了 `/v1/chat/completions`，NEWAPI 原生链路默认保持客户端选择的协议，并把 Chat 请求发送到
Azure Chat Completions 地址。Azure 对当前模型的 `reasoning_effort + function tools` 组合执行了端点能力
限制，要求使用 Responses API。

Responses 协议中的推理等级不是 Chat 顶层字段，而是：

```json
{
  "reasoning": {
    "effort": "medium"
  }
}
```

因此，`reasoning_effort` 并非完全不受支持；错误来自所选端点和参数组合不匹配。

### 2.2 对照不同渠道

不同 Provider 对 Chat 与 Responses 的能力边界并不相同。其他渠道可能接受 Chat 请求、在上游内部转换，
或者没有命中完全相同的模型与参数组合。Azure 渠道对该组合进行了严格校验，所以只有它返回了明确的
`Please use /v1/responses instead`。

普通渠道测试未携带与真实请求相同的 `reasoning_effort + function tools`，测试成功不能证明这个组合在
Chat 端点可用。

### 2.3 核对近期变更

如需判断变更来源，对照故障前后网关版本、渠道配置、API Version、模型部署映射与请求参数。
只有同一组合的前后结果可用于判断端点行为变化。

## 3. 解决方案

### 3.1 推荐：按渠道和模型启用 Chat → Responses 兼容转换

无需修改代码。在管理后台进入：

```text
系统设置 → 模型设置 → 全局模型配置
```

执行以下配置：

1. 关闭“启用请求透传”，即保存：

   ```text
   global.pass_through_request_enabled = false
   ```

2. 在“ChatCompletions → Responses Compatibility”的“Policy JSON”中填写：

   ```json
   {
     "enabled": true,
     "all_channels": false,
     "channel_ids": [123],
     "model_patterns": ["^(?:gpt-5\\.5|gpt-5\\.6-(?:sol|terra|luna))$"]
   }
   ```

   将示例渠道 ID 和模型表达式替换为已复现问题的范围。

3. 确认目标渠道自身的请求体透传也处于关闭状态：

   ```text
   pass_through_body_enabled = false
   ```

配置生效后，客户端仍可调用 `/v1/chat/completions`；选中目标渠道且模型精确匹配
`gpt-5.6-sol` 时，网关会将请求转换为 Responses 协议发送给上游，再把响应转换回 Chat Completions
格式。全局或渠道级任一请求透传开关开启，都会绕过该兼容转换。

`global.pass_through_request_enabled` 的代码默认值就是 `false`。管理后台保存后，该配置以
`global.pass_through_request_enabled` 为键写入系统 `options` 配置；它不是必须通过环境变量设置的开关。

### 3.2 上线验证

先仅对目标渠道和 `gpt-5.6-sol` 灰度，并使用与故障请求相同的参数形状验证：

- 非流式请求能够返回正常 Chat Completions 响应；
- 流式请求可以正常结束；
- function tool call 的名称、参数和结束原因没有丢失；
- 上游实际请求路径为 Responses，而客户端入口仍为 Chat Completions；
- usage、计费和错误日志符合预期。

如果转换异常，先禁用该 Policy；如果还有其他确认支持该请求组合的渠道，可临时从目标渠道 移除
`gpt-5.6-sol` 能力，避免请求再次选中该渠道。

### 3.3 不推荐的长期方案

- 直接删除 `reasoning_effort`：会丢失客户要求的推理等级语义，而且 tools 组合仍可能受到端点限制。
- 依赖旧 Azure API Version：不能保证支持 GPT-5.6，也不能消除端点合同差异。
- 对所有模型、所有渠道全量启用转换：扩大兼容转换影响面，不适合作为本次问题的初始处置。

## 4. 排查 Chat 参数兼容

OpenAI/Azure Chat 参数兼容由 adapter 能力表（`GetOpenAIChatCapabilities`）
驱动，按**映射后的上游模型名**判定，与客户别名无关：

- 已登记模型（含 GPT-5 系列、GPT-6 Astra 及日期快照）自动启用 `max_completion_tokens`、
  `developer` 系统角色，并移除 `temperature`、`top_p`、`logprobs`、`top_logprobs`。
- 精确名称 `gpt-chat-latest` 已按当前 GPT-5.6 参数规则登记（滚动更新名称，不拼接版本日期）；
  客户别名通过 `model_mapping` 精确映射到该名称时同样生效。裸名 `chat-latest`、
  不透明部署名、`gpt-chat-latest-preview` 不自动纳入。

排查渠道测试 `max_tokens` 参数 400 时的已核边界：

| 现象 | 结论与处置 |
| --- | --- |
| 客户别名映射到已登记模型后仍带 `max_tokens` | 检查渠道/全局“请求体透传”是否开启；透传会绕过 adapter 转换与参数覆盖 |
| 参数覆盖又写回 `max_tokens` | 参数覆盖执行于 adapter 转换之后；两个 token 字段同时非零时旧字段不被清除（上游现状边界） |
| 渠道测试按钮失败但正式请求正常 | Chat→Responses 兼容策略只作用于正式 Chat 入口，渠道测试不调用该策略；不能以测试按钮作为策略验证手段 |
| 需要裸名或不透明部署名的兼容 | 升级不能解决识别缺口；供应商支持 Responses 时先显式选择 Responses 端点，或关闭透传后使用 Chat→Responses 策略 |

测试预算默认 16 token 含推理预算时可能不足以产生可见输出；出现空输出时先核对结束原因，调高
需要复现工具调用时使用第 3 节的正式请求与适用预算。

## 5. 参考

- [OpenAI GPT-5.6 指南](https://developers.openai.com/api/docs/guides/latest-model)
- [Azure Responses API 文档](https://learn.microsoft.com/en-us/azure/foundry/openai/how-to/responses)
- [Azure reasoning 文档](https://learn.microsoft.com/en-us/azure/foundry/openai/how-to/reasoning)
- 策略配置定义：`setting/model_setting/global.go`
- Chat → Responses 选择逻辑：`service/openai_chat_responses_mode.go`
- Chat → Responses 转换入口：`relay/chat_completions_via_responses.go`
