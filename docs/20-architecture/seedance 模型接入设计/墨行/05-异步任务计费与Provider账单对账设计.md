---
status: accepted
owner: Dev Team
last-reviewed: 2026-08-10
---

# 墨行异步任务计费与 Provider 账单对账设计

## 1. 三类费用事实

| 事实 | 权威来源 | 用途 |
| --- | --- | --- |
| 客户费用 | 客户模型价格、分组倍率、Task billing、结算日志 | 客户额度和账单 |
| Provider 预期成本 | 审批价格与冻结请求维度 | 毛利估算和预扣 |
| Provider 实际证据 | usage-records 或正式账单 | 管理员对账和 exposure |

三类事实不得互相覆盖。Provider quota/金额不能成为第二套客户余额，也不重算历史客户费用。

## 2. 两条线路

- oversea 使用自己的按量表达式和预扣上界；当前 480p/720p 且无视频输入；
- doubao 使用 duration/resolution/input mode 的按秒表达式；
- 两条线路不共享客户模型或价格；`duration=-1` 均按 15 秒上界预扣；
- 未验证的字符串/缺失 usage 不参与结算；doubao 已验证样本没有 usage，因此按冻结请求规格结算。

quota 换算只使用 checked 饱和函数，NaN、溢出或异常大请求必须失败安全并进入管理员审计。

## 3. Provider 对账

oversea 资料提供：

```text
GET /v1/account/usage-records?request_id=<X-Oneapi-Request-Id>
Authorization: Bearer <integration token>
```

模型 Key 与集成 Token 必须物理和权限隔离。对账只读取冻结 Provider request ID，pending 有界重试，
401/403 停止该账号自动对账并告警，404 在一致性窗口后记录缺少证据。对账结果只形成管理员成本证据，
不覆盖客户 Log。

create unknown 不能凭相似模型、时间、金额或一条费用记录自动认领任务。Provider 金额未知时保持未知。

## 4. 不变量

1. 客户费用只由冻结 NEWAPI 计费事实决定。
2. 两条线路使用不同客户模型和价格表达式。
3. 未验证 usage 不参与结算。
4. 模型 Key 与对账 Token 隔离。
5. 客户退款和 Provider exposure 分账。
