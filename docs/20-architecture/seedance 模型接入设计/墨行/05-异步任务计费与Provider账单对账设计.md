---
status: accepted
owner: Dev Team
last-reviewed: 2026-08-05
---

# 墨行异步任务计费与 Provider 账单对账设计

## 1. 三类费用事实

| 事实 | 权威来源 | 用途 |
| --- | --- | --- |
| 客户费用 | NEWAPI 客户模型价格、分组倍率、Task billing 与结算日志 | 用户/Token 额度、客户账单 |
| Provider 预期成本 | 经审批的墨行价格快照和冻结请求维度 | 毛利预估、预扣和异常检测 |
| Provider 实际费用证据 | 终态 usage、`/v1/account/usage-records` 或正式账单 | 管理员对账与 exposure |

三类事实不得互相覆盖。客户账单继续从 NEWAPI 结算日志投影；墨行 `quota`、`charged_quota` 和
`*_yuan` 不成为第二套客户余额，也不重算历史客户费用。

## 2. 客户计费主链

```text
已验证请求
  -> 冻结 billing probe / 客户价格 / 分组倍率
  -> TaskCreateAttempt 资金 hold
  -> 墨行创建请求
  -> 可信 Task：hold 原子转移
  -> 成功：按可信 usage 结算；usage 不可信时遵守冻结预扣合同
  -> 失败：退款或结算
  -> unknown 到期释放：另记 ProviderCostExposure
```

异步任务预扣 Token 上界按客户模型配置，必须覆盖已发布能力的最大可能成本。它不能由进程默认值、
Provider 当前余额或无限额度旁路。quota 换算只使用统一 checked 饱和函数；溢出、NaN 和钳制必须
失败安全并进入管理员审计。

对当前 `seedance-2-0-oversea` SKU，`duration=-1` 表示智能时长，预扣阶段必须按已发布能力的最大档
15 秒计算 hold，不能按未知值、默认 5 秒或最小 4 秒预扣。模型合同先校验显式时长 `4..15` 或 `-1`；
全局 `MaxTaskDurationSeconds=3600` 只是所有任务共享的防溢出安全上限，不能替代更严格的模型边界。

## 3. Provider 价格与 usage

墨行当前 V2 资料按“元/百万 token”报价，并同时列出输出分辨率和是否包含视频输入。初始公开合同
只支持 480p/720p 且不支持视频输入，因此不能把历史 Ark 的 1080p/4K 或“含视频输入”低价档应用到
当前任务。

Provider 价格只用于成本审批，不直接成为客户价格。若客户采用表达式计费，表达式必须自包含，并
使用冻结任务探针；不能在运行时读取墨行当前报价。异步表达式不得依赖未冻结 header 或时间函数。

官方 V2 文档把任务 `usage` 描述为字符串，当前 adapter 则可接收结构化
`completion_tokens/total_tokens`。只有目标生产黑盒证明字段类型、单位、终态稳定性和退款口径后，
这些 token 才能用于实际结算。adapter 透传能力、单元测试 fixture 或代码注释只能证明解析器能够
处理该对象，不能替代可追溯的目标生产证据；未验证 usage 不得伪装为官方 Ark usage。

## 4. 单次账单查询

墨行账户接口：

```text
GET /v1/account/usage-records?request_id=<X-Oneapi-Request-Id>
Authorization: Bearer itk-mxai-...
```

模型调用使用 `sk-mxai-*`，账户查询使用单独的 `itk-mxai-*` 集成 Token。网关已捕获上游
`X-Oneapi-Request-Id` 并可冻结到 Task/create attempt 私有事实；不能使用
`X-Oneapi-Client-Request-Id` 代替。

### 4.1 凭据隔离

- 模型 Channel Key 只发往视频/素材数据面；
- 集成 Token 只由受限 Provider 对账控制面读取，不写入 Channel Key、Task、attempt、普通配置导出
  或客户日志；
- 缺少集成 Token 只关闭自动 Provider 对账，不阻断已验证的视频履约；
- 账户接口 Base URL、Token 和账号身份必须显式绑定，不能从模型域名或 Key 前缀猜测。

### 4.2 对账状态

| 墨行状态 | 对账处理 |
| --- | --- |
| `pending` | 延迟重试，不改变客户资金状态 |
| `success` | 记录归一化费用与 token 证据，比较但不覆盖客户结算 |
| `failed` | 记录 Provider 未扣费证据；客户退款仍按 Task 合同执行 |
| 404 | 先按最终一致性窗口重试；超过窗口标记 missing evidence |
| 401/403 | 停止该账号自动对账并告警，不回退模型 Key |

对账记录以 Provider 账号作用域与 `request_id` 幂等，保存状态、模型产品名、token 数、
`charged_quota`、`charged_yuan`、币种、查询时间和证据版本；不保存响应 body、Token 名称、集成 Token
或请求内容。`charged_quota` 是墨行 Provider 单位，不能与 NEWAPI quota 直接相减；`*_yuan` 也只能在
币种和换算口径明确时用于 Provider 成本。

## 5. 对账时机与 create unknown

单次查询是异步、只读的管理员流程，不进入客户计费热路径：

1. Task 可信终态后按冻结 `upstream_request_id` 查询；
2. create unknown 有请求 ID 时可用它寻找费用证据，但费用记录本身不能证明唯一任务 ID；
3. `pending` 在有界窗口内退避重试；
4. 差异只生成管理员告警、对账事实或 exposure，不改写已完成的客户 Log；
5. 正式供应商账单到达后可关联和冲销 Provider 侧证据，但仍不污染客户 quota 账本。

仅凭同一模型、相近时间或金额不能恢复 create unknown。自动恢复仍要求唯一、可重复验证的 Provider
任务关联键和 CAS。

## 6. exposure

客户退款但 Provider 可能已经产生费用时写入幂等 `ProviderCostExposure`。主运营指标继续使用
`customer_quota_released`，并按 channel、Link SKU、implementation、profile、reason 和时间窗口
聚合。查询到的 `charged_yuan` 可作为独立 Provider 金额证据，但不能用客户 quota 冒充或把不同币种
未换算相加。

exposure 策略缺失、失效或预算耗尽时，墨行 implementation 候选 fail closed。真正的并发硬上限
必须在 Provider POST 前原子预留预算，事后对账和熔断不能宣称为零超调。

## 7. 不变量

1. 客户费用永远由 NEWAPI 冻结价格与结算日志决定。
2. Provider 价格和账单只作成本证据，不覆盖客户 quota。
3. 模型 Key 与集成 Token 必须物理和权限隔离。
4. 未验证的 `usage` 不参与实际 token 结算。
5. 对账延迟或失败不阻断正常 Task 终态，但必须可观测。
6. create unknown 不因查到相似费用记录而自动认领任务。
7. Provider 金额未知时保持未知，不使用估算值伪装实际成本。
