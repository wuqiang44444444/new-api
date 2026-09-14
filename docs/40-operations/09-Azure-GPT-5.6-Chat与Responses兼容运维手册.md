---
status: current
owner: Dev Team
last-reviewed: 2026-09-14
---

# 09 Azure GPT-5.6 Chat 与 Responses 兼容运维手册

## 1. 问题

2026-08-05 起，真实用户通过 NEWAPI 原生入口请求：

```text
POST /v1/chat/completions
model = gpt-5.6-sol
reasoning_effort = <推理等级>
tools = <function tools>
```

当请求选中 Azure 渠道 `#14` 时，上游返回 HTTP 400，并明确要求改用 `/v1/responses`。同一渠道不带
`reasoning_effort` 的普通模型测试可以成功，因此该现象不表示渠道整体不可用，也不是通用网络故障。

本问题属于 NEWAPI 原生 Chat/Responses 转发范围，不属于 Link 服务合同。

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

### 2.2 为什么只在 Azure 渠道出现

不同 Provider 对 Chat 与 Responses 的能力边界并不相同。其他渠道可能接受 Chat 请求、在上游内部转换，
或者没有命中完全相同的模型与参数组合。Azure 渠道对该组合进行了严格校验，所以只有它返回了明确的
`Please use /v1/responses instead`。

普通渠道测试未携带与真实请求相同的 `reasoning_effort + function tools`，测试成功不能证明这个组合在
Chat 端点可用。

### 2.3 是否能判定为 Azure 在 2026-08-05 改动

当前仓库在该日期前后没有发现能够解释此行为的相关转发代码变化。如果线上同期也没有升级版本、修改
Azure API Version、模型部署映射或渠道配置，可以排除本地代码在当日改变行为。

但是，仅凭错误首次出现时间不能证明 Azure 当日改变了规则。现有日志同样符合“2026-08-05 首次出现
完全匹配的真实请求”的情况。只有找到该日期之前相同模型、端点、部署、API Version、
`reasoning_effort` 和 tools 请求成功的记录，才能进一步支持上游行为发生变化的判断。

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
     "channel_ids": [14],
     "model_patterns": ["^(?:gpt-5\\.5|gpt-5\\.6-(?:sol|terra|luna))$"]
   }
   ```

   "channel_ids": 渠道编号列表

3. 确认渠道 `#14` 自身的请求体透传也处于关闭状态：

   ```text
   pass_through_body_enabled = false
   ```

配置生效后，客户端仍可调用 `/v1/chat/completions`；选中渠道 `#14` 且模型精确匹配
`gpt-5.6-sol` 时，网关会将请求转换为 Responses 协议发送给上游，再把响应转换回 Chat Completions
格式。全局或渠道级任一请求透传开关开启，都会绕过该兼容转换。

`global.pass_through_request_enabled` 的代码默认值就是 `false`。管理后台保存后，该配置以
`global.pass_through_request_enabled` 为键写入系统 `options` 配置；它不是必须通过环境变量设置的开关。

### 3.2 上线验证

先仅对渠道 `#14` 和 `gpt-5.6-sol` 灰度，并使用与故障请求相同的参数形状验证：

- 非流式请求能够返回正常 Chat Completions 响应；
- 流式请求可以正常结束；
- function tool call 的名称、参数和结束原因没有丢失；
- 上游实际请求路径为 Responses，而客户端入口仍为 Chat Completions；
- usage、计费和错误日志符合预期。

如果转换异常，先禁用该 Policy；如果还有其他确认支持该请求组合的渠道，可临时从渠道 `#14` 移除
`gpt-5.6-sol` 能力，避免请求再次选中该渠道。

### 3.3 不推荐的长期方案

- 直接删除 `reasoning_effort`：会丢失客户要求的推理等级语义，而且 tools 组合仍可能受到端点限制。
- 依赖旧 Azure API Version：不能保证支持 GPT-5.6，也不能消除端点合同差异。
- 对所有模型、所有渠道全量启用转换：扩大兼容转换影响面，不适合作为本次问题的初始处置。

## 4. Chat 参数兼容现状（rc.36 同步后）

接取上游 rc.36 后，OpenAI/Azure Chat 参数兼容由 adapter 能力表（`GetOpenAIChatCapabilities`）
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
测试预算不属于参数兼容修复。原 2026-08-05 的 `reasoning_effort + tools` 端点限制与 §3 的
Chat→Responses 策略配置继续有效。

## 5. 参考

- [OpenAI GPT-5.6 指南](https://developers.openai.com/api/docs/guides/latest-model)
- [Azure Responses API 文档](https://learn.microsoft.com/en-us/azure/foundry/openai/how-to/responses)
- [Azure reasoning 文档](https://learn.microsoft.com/en-us/azure/foundry/openai/how-to/reasoning)
- 策略配置定义：`setting/model_setting/global.go`
- Chat → Responses 选择逻辑：`service/openai_chat_responses_mode.go`
- Chat → Responses 转换入口：`relay/chat_completions_via_responses.go`
