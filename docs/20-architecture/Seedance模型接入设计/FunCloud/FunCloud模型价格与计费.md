---
status: current
owner: Dev Team
last-reviewed: 2026-08-18
---

# FunCloud 模型价格与计费

## 1. Provider 价格

单位为 USD/百万 Token；`has_video_input=true` 只在请求含视频时命中，图片和音频不算视频输入。

| 模型 | 无视频输入 | 含视频输入 | 官方 BytePlus 对照 |
| --- | ---: | ---: | --- |
| Standard 480/720p | 6.74 | 4.11 | 官方 7.00 / 4.30，约低 3.7% / 4.4% |
| Standard 1080p | 7.48 | 4.55 | 官方 7.70 / 4.70，约低 2.9% / 3.2% |
| Fast 480/720p | 5.43 | 3.23 | 官方 5.60 / 3.30，约低 3.0% / 2.1% |
| Mini 480/720p | 3.37 | 2.05 | 官方 3.50 / 2.10，约低 3.7% / 2.4% |
| 2.5 480/720p | 10.26 | 6.16 | 官方 10.70 / 6.40，约低 4.1% / 3.8% |

对照使用 [Seedance 国内火山与海外 BytePlus 官方价格基准](../Seedance国内火山与海外BytePlus官方价格基准.md)；FunCloud 报价资料的折扣/人民币列不覆盖平台客户账单。

## 2. 客户计费表达式

四个客户模型使用 `tiered_expr`，以 `completion_tokens` 结算：

```text
无视频：c * provider_price
有视频：c * provider_video_price
```

表达式实际通过 `param("_task.has_video_input")`、`param("_task.resolution")` 和 `tier()` 展开，单位是 USD/百万 Token；统一转换为 `quota = cost / 1,000,000 × QuotaPerUnit × group_ratio`。预扣上界：Standard 730,000，Fast/Mini 324,000，2.5 648,000（均为配置快照，需按当前合同复核）。

## 3. 结算与 Provider 成本分账

`completionTokens` 是客户结算依据；`pointConsume` 仅记录为 Provider exposure 证据，必须为有限正数且顶层/输出值一致。成功结算写入私有 `provider_billing_evidence`；失败按明确失败退款，unknown 保留 hold。所有 quota 转换使用 checked 饱和函数，异常进入管理员审计。

## 4. 与官方计价的解释

官方与 FunCloud 都按成功视频的 Token 量计费，但官方价格按地区与输入视频档分开；FunCloud 是第三方报价，不能因为数值接近就把线路映射为官方渠道，也不能把官方资源包/汇率带入客户表达式。价格变化只影响新 Task，历史任务使用冻结表达式。

## 5. 证据状态

Standard 曾取得 `completionTokens` 与 Provider 账单一致样本；Fast/Mini/2.5 的真实终态、失败账单和素材账单仍需逐模型验收，未完成前不标为生产发布。
