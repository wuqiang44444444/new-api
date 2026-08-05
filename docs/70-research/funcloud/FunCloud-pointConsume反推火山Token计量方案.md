---
status: research
owner: Dev Team
last-reviewed: 2026-08-06
---

# FunCloud `pointConsume` 反推火山 Token 计量方案

## 1. 结论

在精确模型、是否包含视频输入、当次生效的火山价格版本均已知时，可以通过 FunCloud 任务终态返回的 `pointConsume` 反推当次火山 Token 用量。当前样本中唯一需要额外维护的参数，是 FunCloud 消费点与人民币成本之间的换算系数 `K`。

反推公式为：

```text
inferred_tokens = round(pointConsume × K × 1,000,000 / official_price_cny_per_million)
```

本样本反推出：

```text
K = 6.819995692768 ≈ 6.82
inferred_tokens = round(0.473622 × 6.82 × 1,000,000 / 37)
                = 87,300
```

结果与 FunCloud 后台显示的实际用量 `87,300 Token` 完全一致。

但 `pointConsume` 本身不是 Token 数。它更符合“按实际 Token 成本折算后的 FunCloud 总消费点”这一语义。`K = 6.82` 目前只是由单个真实样本精确反推得到的待验证计费事实，不能直接视为 FunCloud 全局、永久不变的公开合同。

## 2. 适用范围与边界

本方案用于：

- 在上游任务终态没有直接返回 `usage` 时，补充推断 Provider 侧 Token 用量；
- 对账 FunCloud `pointConsume`、火山官方价格、FunCloud 余额流水和后台 Token 用量；
- 以影子计算方式发现上游价格、换算系数或结算语义变化。

本方案暂不用于：

- 创建任务时的预扣费，因为此时尚无最终 `pointConsume`；
- 覆盖上游直接返回的官方 `usage`；
- 自动改变现有客户计费、客户账单或公开 API 合同；
- 把 Provider 成本直接解释成客户售价；
- 在失败、取消、退款、优惠语义尚未验证时生成确定 Token 用量。

客户计费与 Provider 成本必须保持两个独立事实面。即使成功反推出 Provider Token，也不能据此自动重算客户消费。

## 3. 实际请求与返回证据

### 3.1 任务轮询请求

对同一真实任务重新执行了 FunCloud 任务查询：

```http
GET https://mm-internal-cn.leonecloud.com/api/v2/open/aigc/{task_id}
```

脱敏后的实际响应为：

```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "taskId": "<redacted-provider-task-id>",
    "status": "success",
    "result": ["<redacted-result-url>"],
    "createdAt": "2026-08-06 00:20:04",
    "updatedAt": "2026-08-06 00:21:45",
    "createTime": 1785946804,
    "updateTime": 1785946905,
    "completeTime": 1785946905,
    "costTime": 101,
    "progress": 100,
    "pointConsume": "0.473622"
  }
}
```

该响应没有 `usage`、`total_tokens`、`completion_tokens` 等直接 Token 字段。可用于成本反推的核心字段只有终态 `pointConsume`。

### 3.2 FunCloud 后台同任务记录

同一任务在 FunCloud 后台显示：

| 字段 | 值 |
| --- | ---: |
| 模型 | `seedance2-fast` |
| 类型 / 意图 | 视频 / `t2v` |
| 分辨率 | 720p |
| 时长 | 4s |
| 计费方式 | 按 Token |
| 预估用量 | 86,400 Token |
| 实际用量 | 87,300 Token |
| 任务状态 | 成功 |
| 终态余额扣减流水 | 0.004883 |

对应余额从 `994.879246` 变为 `994.874363`，差额正好是：

```text
994.879246 - 994.874363 = 0.004883
```

### 3.3 成品视频元数据

对同一任务的成品视频进行只读探测，得到：

| 字段 | 值 |
| --- | ---: |
| 编码 | H.264 |
| 分辨率 | 1280 × 720 |
| 帧率 | 24 fps |
| 视频流时长 | 4.041667s |
| 容器时长 | 4.096s |
| 实际帧数 | 97 |

完整签名 URL、Provider 私有任务 ID 和原始响应不应写入文档、日志或审计字段。

## 4. Token 数与视频帧的核对

本样本显示 720p 每帧对应 900 Token：

```text
1280 × 720 / 1024 = 900 Token/帧
```

预估阶段按请求时长和帧率计算：

```text
4s × 24fps = 96 帧
96 × 900 = 86,400 Token
```

实际成品包含 97 帧：

```text
97 × 900 = 87,300 Token
```

因此预估与实际相差一帧，即：

```text
87,300 - 86,400 = 900 Token
```

这能完整解释后台显示的预估和实际 Token 差异。

注意：`像素数 / 1024` 目前只是从一个 720p 样本中得到的精确推断，尚未验证其是否是 480p、1080p、含音频或所有 Seedance 型号的通用官方公式。成品下载和 `ffprobe` 只适合作为研究交叉验证，不应进入计费热路径；签名 URL 会过期，媒体文件也不是持久计费权威。

## 5. 火山官方价格与成本计算

按 2026-08-06 查询到的火山方舟价格，Doubao-Seedance-2.0-fast 为：

| 输入类型 | 价格 |
| --- | ---: |
| 包含视频输入 | ¥22 / 百万 Token |
| 不包含视频输入 | ¥37 / 百万 Token |

本任务为文生视频，不包含视频输入，因此适用 ¥37 / 百万 Token。

参考来源：

- [火山方舟产品与定价](https://www.volcengine.com/product/ark)
- [火山方舟查询视频生成任务 API](https://api.volcengine.com/api-docs/view?action=GetContentsGenerationsTask&serviceCode=ark&version=2024-01-01)

火山官方查询任务合同可返回 `usage.completion_tokens`、`usage.total_tokens`、帧数和 FPS。若 FunCloud 后续能够透传这些字段，应优先采用直接用量，成本反推只作为对账和降级证据。

本任务的火山官方实际成本为：

```text
87,300 / 1,000,000 × 37 = ¥3.2301
```

预估成本为：

```text
86,400 / 1,000,000 × 37 = ¥3.1968
```

一帧差额对应：

```text
900 / 1,000,000 × 37 = ¥0.0333
```

## 6. `pointConsume` 与换算系数

假设 `pointConsume` 是实际火山人民币成本经 FunCloud 系数换算后的总消费点，则：

```text
pointConsume = official_cost_cny / K
K = official_cost_cny / pointConsume
```

代入实际数据：

```text
K = (87,300 × 37 / 1,000,000) / 0.473622
  = 6.819995692768
  ≈ 6.82
```

再用 `K = 6.82` 正向核对：

```text
¥3.2301 / 6.82 = 0.4736217 ≈ 0.473622
```

预估消费点为：

```text
¥3.1968 / 6.82 = 0.468739299
```

实际较预估增加一帧，对应补扣：

```text
¥0.0333 / 6.82 = 0.004882698 ≈ 0.004883
```

该结果与 FunCloud 终态扣费流水和余额差额完全一致。因此，本样本最合理的解释是：

- `pointConsume = 0.473622` 是基于 `87,300` 个实际 Token 计算出的总消费点；
- 创建时可能已经按 `86,400` Token 预扣；
- 终态流水 `0.004883` 是实际多出一帧、即 900 Token 的差额补扣；
- `pointConsume` 不是这笔终态差额，也不是 Token 数本身。

## 7. 可实施的反推模型

### 7.1 必要输入

每次请求需要冻结以下事实：

| 字段 | 来源 | 要求 |
| --- | --- | --- |
| 精确模型 / SKU | 我方请求与执行快照 | 不能只记录模糊产品名 |
| 是否包含视频输入 | 我方规范化请求 | 必须在发送前确定 |
| 官方单价 | 版本化价格表 | 按请求生效时间冻结 |
| FunCloud `pointConsume` | 上游任务终态响应 | 保留原始十进制字符串 |
| 换算系数 `K` | 经验证的 Provider 计费配置 | 按实现、账号和时间版本化 |

在这些条件中，前四项均可由我方请求、版本化价格表和上游终态响应确定，只有 `K` 需要通过真实账单样本验证并维护。

### 7.2 计算公式

```text
official_unit_price = 根据模型、has_video_input、价格版本选出的人民币/百万 Token 单价

inferred_tokens = round(
  pointConsume × conversion_rate × 1,000,000 / official_unit_price
)
```

必须使用十进制定点运算，不应先把 `pointConsume` 或价格转换为二进制浮点数。

### 7.3 建议的换算系数记录

`K` 不应作为全局常量写死。建议至少按以下维度保存和审计：

```yaml
provider: funcloud
implementation: funcloud.seedance-json/v1
account_scope: <channel-or-provider-account-scope>
conversion_rate: "6.82"
effective_from: <timestamp>
effective_to: null
status: provisional # provisional | verified | suspended
evidence:
  sample_count: 1
  price_version: <version>
```

后续样本应计算观测值：

```text
K_observed = actual_tokens × official_price / 1,000,000 / pointConsume
```

并与当前生效的 `K` 比较。若偏差超过约定阈值，应暂停推断并告警，不能静默改用新的系数。

### 7.4 推断结果的事实等级

推断结果必须与官方用量分开保存，并明确来源。例如：

```yaml
provider_usage:
  total_tokens: 87300
  usage_source: provider_cost_inferred
  inference_version: funcloud-volcengine-v1
  conversion_rate: "6.82"
  official_unit_price: "37"
  point_consume: "0.473622"
  confidence: provisional
```

不得用推断值覆盖后续取得的 Provider 官方 `usage`，也不得在没有单独架构决策和合同变更的情况下，把它直接作为客户计费权威。

## 8. 失败关闭条件

出现以下任一情况时，不生成确定 Token 用量：

- 没有当前生效且适用范围匹配的 `K`；
- 无法确定精确模型、是否有视频输入或官方价格版本；
- `pointConsume` 缺失、非数字、为负数或任务尚未进入可信终态；
- 任务失败、取消、退款、优惠券或折扣后的 `pointConsume` 语义尚未验证；
- 上游价格已经变化，但本地价格版本没有完成切换；
- `K_observed` 与当前系数发生超阈值漂移；
- 同一次任务出现多次互相冲突的终态消费值；
- Provider 直接用量与反推结果不一致且无法解释。

`pointConsume` 有 6 位小数。按 ¥37/百万 Token、`K = 6.82` 估算，其半个最小计量单位造成的误差约为：

```text
0.0000005 × 6.82 × 1,000,000 / 37 ≈ 0.092 Token
```

因此该样本中，小数位精度不是主要误差来源；价格选择、系数适用范围和消费语义才是主要风险。

## 9. 验证矩阵

在把 `K = 6.82` 从 `provisional` 提升为 `verified` 前，至少覆盖：

| 维度 | 样本 |
| --- | --- |
| 模型 | Seedance 2.0、Fast、Mini（以实际可用 SKU 为准） |
| 输入 | 文生视频、包含视频输入 |
| 分辨率 | 480p、720p、1080p（按模型能力） |
| 时长 | 4s、15s、默认值和其他受支持值 |
| 音频 | 开启、关闭 |
| 状态 | 成功、失败、取消、退款 |
| 结算条件 | 原价、折扣、优惠券、活动价 |
| 账号范围 | 多个 FunCloud 账号、密钥或渠道 |
| 价格变化 | 新旧官方价格版本切换前后 |

每个有效组合应采集多个样本，并同时核对：

- FunCloud 任务终态 `pointConsume`；
- FunCloud 后台实际 Token；
- 火山官方 `usage`，如果能够取得；
- 创建预扣、终态差额和余额总变化；
- 成品帧数，仅作为视频 Token 公式的辅助证据。

建议验收标准：

- 反推 Token 与后台或官方实际 Token 完全一致，或误差符合事先记录的舍入规则；
- 相同作用域和版本内的 `K_observed` 稳定；
- 总消费点、预估消费点和终态差额能闭环对账；
- 失败、取消和退款任务不会错误产生正向确定用量；
- 官方价格变化后不会继续使用旧价格反推；
- 在未正式采纳前，推断结果不会进入客户 API 或客户计费。

## 10. 分阶段落地建议

### 阶段一：研究与影子计算

- 只在任务成功终态计算推断值；
- 不改变客户计费、不修改公开响应；
- 保存受控的计算输入、公式版本和结果；
- 对照后台实际 Token，扩大样本矩阵。

### 阶段二：管理员对账与漂移告警

- 在管理员审计信息中展示 `pointConsume`、价格版本、`K` 和推断 Token；
- 增加 `K_observed` 漂移告警；
- 发现价格、折扣或账户差异时自动暂停该作用域的推断。

### 阶段三：确认 Provider 合同

- 向 FunCloud 确认 `pointConsume` 的正式单位、总额/净额语义和小数规则；
- 确认 `K` 是全局、账号、密钥、模型还是活动级参数；
- 优先要求 FunCloud 直接返回火山官方 `usage`；
- 确认失败、取消、退款和优惠场景的结算语义。

### 阶段四：正式采用决策

只有完成多样本验证和 Provider 合同确认后，才评估是否把推断用量提升为持久化 Provider 计量事实。任何客户可见 API 或客户计费用途，都需要另行更新产品合同、架构事实和必要的 ADR，不能由本研究方案自动生效。

## 11. 待确认问题

1. `pointConsume` 的正式单位是什么，是否始终代表任务总实际消费？
2. `pointConsume` 是折扣前总额、折扣后净额，还是余额体系中的内部点数？
3. `K = 6.82` 的适用范围是全局、账号、密钥、模型、活动还是时间段？
4. 创建预扣是否固定基于预估 Token，终态流水是否始终只记录差额？
5. 失败、取消、超时、退款和重试任务分别如何返回 `pointConsume`？
6. FunCloud 是否能直接透传火山 `usage.total_tokens`、帧数和 FPS？
7. 官方价格变更时，FunCloud 的价格和 `K` 是同步切换还是分别生效？
8. 优惠券、充值赠送、阶梯折扣是否会改变 `pointConsume` 或仅改变余额流水？

## 12. 当前判断

当前证据足以证明：在这个 `seedance2-fast`、无视频输入、720p、4s 的成功任务中，`pointConsume = 0.473622` 与火山实际 `87,300 Token`、¥37/百万 Token 的官方价格以及 `K ≈ 6.82` 构成精确闭环；终态扣减 `0.004883` 则对应实际比预估多出的一帧 900 Token。

当前证据尚不足以证明：`6.82` 对全部 FunCloud 账号、全部 Seedance 型号、全部价格版本和所有折扣/失败场景永久适用。因此，本方案应先作为版本化、失败关闭的 Provider 成本推断与对账机制验证，不能把单样本结论直接升级为生产计费合同。
