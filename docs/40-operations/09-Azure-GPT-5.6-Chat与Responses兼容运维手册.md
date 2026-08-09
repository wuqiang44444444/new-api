---
status: current
owner: Dev Team
last-reviewed: 2026-08-06
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

## 4. 参考

- [OpenAI GPT-5.6 指南](https://developers.openai.com/api/docs/guides/latest-model)
- [Azure Responses API 文档](https://learn.microsoft.com/en-us/azure/foundry/openai/how-to/responses)
- [Azure reasoning 文档](https://learn.microsoft.com/en-us/azure/foundry/openai/how-to/reasoning)
- 策略配置定义：`setting/model_setting/global.go`
- Chat → Responses 选择逻辑：`service/openai_chat_responses_mode.go`
- Chat → Responses 转换入口：`relay/chat_completions_via_responses.go`
