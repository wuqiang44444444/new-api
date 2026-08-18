---
status: current
owner: Dev Team
last-reviewed: 2026-08-18
---

# BytePlus 官方模型价格与计费

## 1. 官方在线价格

单位为 USD/百万 Token；括号为含视频输入。

| 模型 | 480/720p | 1080p | 4K |
| --- | ---: | ---: | ---: |
| `dreamina-seedance-2-0-260128` | $7.00 / $4.30 | $7.70 / $4.70 | $4.00 / $2.40 |
| `dreamina-seedance-2-0-fast-260128` | $5.60 / $3.30 | 不支持 | 不支持 |
| `dreamina-seedance-2-0-mini-260615` | $3.50 / $2.10 | 不支持 | 不支持 |
| `dreamina-seedance-2-5-260628` | $10.70 / $6.40 | 不支持 | 不支持 |

旧版 1.x 在线/离线价格仅作存量账单核对。资源包为 90 天预付 Token，需单独记录模型匹配和到期时间。

## 2. 平台结算

成功终态优先使用可信 `usage.completion_tokens`；官方 Token 估算公式仅用于预扣：

```text
estimated_tokens = (input_video_seconds + output_seconds) × width × height × fps / 1024
quota = actual_tokens / 1,000,000 × price × QuotaPerUnit × group_ratio
```

图片/音频输入不自动命中含视频价格；只有实际含视频输入才切换档位。表达式、模型和连接在 Task 创建时冻结，统一 checked quota conversion 防溢出。

## 3. 与火山官方对比

按固定汇率 1 USD=6.82 RMB，BytePlus 相对火山同家族价格略高：2.0 7.00 vs 国内折算 6.7449（+3.8%），Fast 5.60 vs 5.4252（+3.2%），Mini 3.50 vs 3.3724（+3.8%）；2.5 10.70 vs 国内 10.2639（+4.3%）。两地 Provider 身份、账号、资源包和 SLA 独立，不能合并计费。

## 4. 风险与证据

官方失败任务通常不收费的资料口径仍需目标账号账单验证；unknown 保留 hold，不以“官方失败不收费”推断平台可自动退款。价格变更只影响新 Task，Provider exposure 未知保持未知。

