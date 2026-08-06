---
status: accepted
owner: Dev Team
last-reviewed: 2026-08-06
---

# 飞彩全模型 SKU 与计费设计

## 1. 目的与权威边界

本文逐一覆盖飞彩资料中的 10 个 Seedance 模型，定义目标 Link SKU、capability、size 证据、媒体输入、
billing mode 和 Provider 对账边界。Mini、Fast、933、VIP、SD2 与 Pro PI 在质量档、输入媒体、时长、
分辨率或计费维度上存在客户可观察差异，因此必须分别注册，不能因字段相似或报价接近而合并。

客户模型名可自定义，Link SKU 是稳定产品身份，Provider 模型只用于 `model_mapping` 和 execution binding。
2026-08-03 研究报价只记录 Provider 证据，不是客户生产售价。

飞彩只采用单轨 v2。两个旧 v1 capability 已被 v2 全模型注册整体替换，不保留 v1 hash 或 v1 converter。

## 2. 10 模型完整合同矩阵

| # | Provider 模型 | 目标 Link SKU | 固定分辨率 | duration | 图片 min–max | 音频 max | 视频 max | billing mode | 当前代码 |
| ---: | --- | --- | --- | --- | ---: | ---: | ---: | --- | --- |
| 1 | `seedance-2.0-vip-720p-mini-azhw-feicai` | `seedance-2.0-mini-720p` | 720p | 4–15；初版必填 | 0–9 | 3 | 0 | per-second | v2 已登记，渠道 51 已发布 |
| 2 | `seedance2.0-sd2-feicai` | `seedance-2.0-sd2-720p` | 720p | 11–15；初版必填 | 1–9 | 0 | 0 | per-second | v2 已登记，未发布 |
| 3 | `seedance-2.0-vip-720p-fast-azhw-feicai` | `seedance-2.0-fast-720p` | 720p | 4–15；初版必填 | 0–9 | 3 | 0 | per-second | v2-r2 已登记，渠道 51 已发布 |
| 4 | `seedance-2.0-933-720p-azhw-feicai` | `seedance-2.0-value-720p` | 720p | 4–15；初版必填 | 0–9 | 3 | 0 | per-second | v2-r3 已登记，渠道 52 已收窄发布 |
| 5 | `seedance-2.0-vip-720p-azhw-feicai` | `seedance-2.0-standard-720p` | 720p | 4–15；初版必填 | 0–9 | 3 | 0 | per-second | v2 已登记，渠道 51 已发布 |
| 6 | `seedance-2.0-933-1080p-azhw-feicai` | `seedance-2.0-value-1080p` | 1080p | 4–15；初版必填 | 0–9 | 3 | 0 | per-second | v2 已登记，未发布 |
| 7 | `seedance-2.0-vip-1080p-azhw-feicai` | `seedance-2.0-standard-1080p` | 1080p | 4–15；初版必填 | 0–9 | 3 | 0 | per-second | v2 已登记，渠道 51 已发布 |
| 8 | `seedance-2.0-933-4k-azhw-feicai` | `seedance-2.0-value-4k` | 4K | 4–15；初版必填 | 0–9 | 3 | 0 | per-second | v2 已登记，未发布 |
| 9 | `seedance-2.0-vip-4k-azhw-feicai` | `seedance-2.0-standard-4k` | 4K | 4–15；初版必填 | 0–9 | 3 | 0 | per-second | v2-r2 已登记，渠道 51 已发布 |
| 10 | `seedance-933-pro-pi-feicai` | `seedance-2.0-pro-pi-720p` | 720p | 固定 15；省略归一为 15 | 0–9 | 3 | 3 | per-request | v2 已登记，未发布 |

前 9 个模型的 v2 合同不继承旧 v1 的“省略即 4 秒”，初版统一要求客户显式提交 duration。
这是发布前的失败关闭边界，只有逐模型黑盒证据能改变默认策略。

## 3. capability v2 数据模型

### 3.1 当前结构与稳定语义

`VideoSKUCapability` 已登记并将以下 v2 语义纳入 capability hash：

- `MinImages`：SD2 登记为 1，其余模型为 0；
- `BillingMode`：至少区分 `per-second` 与 `per-request`；
- `VideoRoles`：字段已存在，Pro PI 登记 `reference_video`，其它模型保持空；
- 10 个模型分别固定 hash，不以共享 struct 值替代独立合同身份；
- 所有 min/max、默认值、角色和 billing mode 在 Provider POST 与预扣前校验。

不得只在 converter 中对 SD2 图片必填或 Pro PI 视频角色做私有特判；否则 capability、选渠、素材解析、
billing probe 和公开合同可能产生分叉。

### 3.2 通用按秒模型

Mini、Fast、933 与 VIP 的 8 个按秒 SKU 使用同一协议形状，但分别注册：

- 固定一个分辨率，客户不能通过 `resolution` 跨 SKU 升降档；
- 文本至少一项非空；
- 图片 0–9，角色只允许 `reference_image`；
- 音频 0–3，角色只允许 `reference_audio`；
- 音频单独使用是否有效尚未闭合，初版要求视觉参考；
- 参考视频上限为 0；
- 初始不发布 `first_frame`、`last_frame`，不从图片位置猜语义；
- `supports_direct_media=true`、`supports_link_assets=true`、`supports_mixed_media_paths=false`；
- 生命周期只承诺创建、查询和内容，不承诺取消、删除或尾帧。

### 3.3 SD2 图生视频

`seedance-2.0-sd2-720p` 单独注册：

- 固定 720p，只允许 11–15 秒；
- 初版 `duration` 必填；
- 文本必填，`MinImages=1`、`MaxImages=9`；
- 图片角色只允许 `reference_image`；
- `MaxAudio=0`、`MaxVideos=0`；
- 初始只考虑 16:9、9:16，但仍需逐模型成功证据和精确像素值；
- 不登记 `real_person`。

资料同时声称全局默认 4 秒和 SD2 只允许 11–15 秒，因此不能把省略值猜成 11，也不能发送 4。
只有正式合同和黑盒共同证明模型专属默认后，才能提升 capability 开放省略。

### 3.4 Pro PI 按次与参考视频

`seedance-2.0-pro-pi-720p` 单独注册：

- 固定 720p、固定 15 秒；省略归一为 15，显式非 15 返回 4xx；
- 图片 0–9、音频 0–3、参考视频 0–3；
- `VideoRoles=[reference_video]`，所有媒体数组保序；
- 音频单独使用未证明时要求图片或视频参考；
- Link 资源只允许 `general` 图片、音频和视频；
- billing mode 固定 `per-request`，duration 不进入费用乘数；
- 在 Provider 澄清前不开放宽幅，不应用通用 1.667 倍率。

## 4. 模型族隔离规则

### 4.1 Mini 与 Fast

- Mini 的 `seedance-2.0-mini-720p` 不是 value/standard 的低价候选；
- Fast 的 `seedance-2.0-fast-720p` 不得复用 FunCloud 的 `seedance-2.0-fast`；
- 两者分别验证默认 duration、逐模型 size、媒体组合、排队时延和账单；
- “轻量”“快速”不自动产生 SLA。

### 4.2 value 与 standard

933/value 与 VIP/standard 即使分辨率相同，也分别冻结 Provider 模型、SKU、价格和 exposure。720p 的
现有两组 size 证据不能自动授权同族的 1080p、4K，也不能因为 v2 替换 v1 而开放六画幅、首尾帧或真人资源。

### 4.3 1080p 与 4K

- `value-1080p`、`standard-1080p`、`value-4k`、`standard-4k` 都已有 v2 capability 与飞彩 binding；
- `standard-1080p` 与 `standard-4k` 只有 `16:9` 的精确 size 证据；两个 value SKU 与两个 standard SKU
  的其它 ratio 均无 size 登记；
- 不得从 720p 或 standard 1080p 推导 value 1080p、`1080x1920` 或任何 4K 像素值；
- 资料中的 `1792x1024` 不是 4K，也不能自动对应 21:9；
- 高分辨率客户价格独立审批，不从 720p 临时乘倍率推导；
- 4K 必须验证最大时长额度、quota 饱和、内容大小、Range 和结果保留。

## 5. size registry 当前事实

### 5.1 当前代码事实

当前 registry 已使用四元键：

```text
(implementation ID/version, provider_model, resolution, ratio)
  -> provider size + billing size class + evidence version
```

registry 当前有六个精确条目；2026-08-06 新增的 Fast 720p、Standard 4K 与 Value 720p 来自隔离
Provider 创建、轮询、同源内容检查和账户 usage 窗口证据：

| Provider 模型 | resolution / ratio | Provider 请求 `size` | 实测产物 | evidence version |
| --- | --- | --- | --- | --- |
| `seedance-2.0-vip-720p-mini-azhw-feicai` | `720p / 16:9` | `1280x720` | 1280×720 | `feicai-prod-2026-08-05-r1` |
| `seedance-2.0-vip-720p-fast-azhw-feicai` | `720p / 16:9` | `1280x720` | 1280×720 | `feicai-prod-2026-08-06-r2` |
| `seedance-2.0-933-720p-azhw-feicai` | `720p / 16:9` | `1280x720` | 1256×720 | `feicai-prod-2026-08-06-r3` |
| `seedance-2.0-vip-720p-azhw-feicai` | `720p / 16:9` | `1280x720` | 1280×720 | `feicai-prod-2026-08-05-r1` |
| `seedance-2.0-vip-1080p-azhw-feicai` | `1080p / 16:9` | `1280x720` | 1920×1080 | `feicai-prod-2026-08-05-r1` |
| `seedance-2.0-vip-4k-azhw-feicai` | `4k / 16:9` | `1280x720` | 3840×2160 | `feicai-prod-2026-08-06-r2` |

Standard 1080p 的 Provider 请求 `size` 与最终编码像素不同，两个事实必须分开冻结。旧
`720p:16:9` 和 `720p:9:16` 两元记录未作为 fallback 保留。
create converter 与 billing probe 共用同一 resolver，并且都必须提供冻结 implementation 与映射后的
Provider 模型。未登记组合在预扣和 Provider POST 前失败关闭。

### 5.2 v2 结构不变量

v2 完整键为：

```text
(implementation ID/version, provider_model, resolution, ratio)
  -> provider size + billing size class + evidence version
```

resolver 必须始终同时接收冻结 `feicai.seedance-videos@v2` 和映射后的 Provider 模型。旧两元 resolver 与
全局 fallback 不得恢复；converter 和 billing probe 必须调用同一 resolver，确保发送像素值和费用维度来自同一证据。

### 5.3 10 模型 size 发布状态

| # | Link SKU | 研究资料画幅 | 可直接进入 v2 registry | 发布要求 |
| ---: | --- | --- | --- | --- |
| 1 | `seedance-2.0-mini-720p` | 六画幅 | `16:9` | 本系统创建/轮询/计费/内容/Range 已验证；逐任务 Provider 账单未闭合 |
| 2 | `seedance-2.0-sd2-720p` | 16:9、9:16 | 无 | 16:9 create 503 unknown，不自动重发 |
| 3 | `seedance-2.0-fast-720p` | 六画幅 | `16:9` | 两次隔离 4 秒成功均生成 1280×720，账户 usage 各增 180 分；当前客户价格继续使用已配置表达式 |
| 4 | `seedance-2.0-value-720p` | 六画幅 | `16:9` | 4 秒任务 completed；内容 4,032,300 B、1256×720、4.062993 秒；渠道 52 仅发布此 SKU |
| 5 | `seedance-2.0-standard-720p` | 六画幅 | `16:9` | 本系统创建/轮询/计费/内容/Range 已验证；逐任务 Provider 账单未闭合 |
| 6 | `seedance-2.0-value-1080p` | 六画幅 | 无 | 16:9 两次终态失败，不从标准 1080p 推导 |
| 7 | `seedance-2.0-standard-1080p` | 六画幅 | `16:9` | 本系统创建/轮询/计费/内容/Range 已验证；请求 size 与产物像素已分开冻结 |
| 8 | `seedance-2.0-value-4k` | 六画幅 | 无 | 16:9 两次终态失败，不从标准 4K 推导 |
| 9 | `seedance-2.0-standard-4k` | 六画幅 | `16:9` | 两次隔离 4 秒任务均 completed、账户 usage 各增 1120 分；完整内容验证为 3840×2160、4.016667 秒 |
| 10 | `seedance-2.0-pro-pi-720p` | 六画幅 | 无 | 主轮询出现 `in_progress`，复测终态失败；未闭合按次账单 |

“研究资料画幅”不是生产 capability。每个 SKU 初始只发布已经进入 v2 registry 的精确组合；未登记 ratio
在预扣前返回能力不支持。

## 6. 字段映射与请求边界

Provider 原生资料明确支持整数 `duration` 替代字符串 `seconds`，两者二选一。v2 只发送 `duration`，
避免字符串解析、双字段冲突和两个默认值来源。

| 客户字段 | 飞彩字段 | v2 规则 |
| --- | --- | --- |
| 客户模型 | `model` | 精确 `model_mapping` 后由 execution binding 复检 |
| 非空 text | `prompt` | 按顺序换行拼接 |
| `duration` | `duration` | 经 SKU 校验的整数，始终显式发送 |
| 固定分辨率 + ratio | `size` | 四元 registry 查表，同时冻结 billing size class |
| `reference_image` | `images[]` | 保序，受每 SKU min/max 约束 |
| `reference_audio` | `audios[]` | 保序，SD2 禁止 |
| `reference_video` | `videos[]` | 仅 Pro PI，保序，最多 3 |

所有 SKU 初版拒绝 `end_user_subject`、`callback_url`、`service_tier`、`generate_audio`、`watermark`、
`return_last_frame`、`execution_expires_after`、`draft`、`tools`、`safety_identifier`、`priority`、
`frames`、`seed` 和 `camera_fixed`。显式 `false` 不能因 `omitempty` 被静默丢弃后宣称支持。

## 7. 客户计费、Provider 成本与实际账单

### 7.1 三类金额事实

| 事实 | 权威来源 | 用途 |
| --- | --- | --- |
| 客户费用 | NEWAPI 冻结价格、billing probe、quota 结算和消费/退款日志 | 用户/Token 扣费与客户账单 |
| Provider 预期成本 | 已审批成本快照与受信任务维度 | 毛利预估与异常检测 |
| Provider 实际证据 | 飞彩任务列表、usage/subscription 或正式供应商账单 | 对账与 exposure，不改写历史客户费用 |

客户账单继续由 `/api/billing/self` 聚合结算日志。飞彩 billing 端点不转发为客户余额，也不用于按当前报价
回算历史 quota。

### 7.2 九个按秒 SKU

Mini、Fast、SD2、933 与 VIP 的 9 个 SKU：

```text
customer_quota = checked(
  approved_customer_unit_price
  × validated_duration_seconds
  × verified_size_multiplier
  × NEWAPI group/model factors
)
```

- duration 来自 capability 校验后的整数；
- size multiplier 与 converter 使用同一 v2 registry；
- 未登记 size 不生成 billing probe；
- 预扣和终态结算使用同一冻结 probe/expression；
- 换算使用 `common.QuotaFromFloatChecked`、`QuotaRoundChecked` 或 `QuotaFromDecimalChecked`；
- saturation 写入 `relayInfo.QuotaClamp` 并附加管理员日志标记；
- 客户单价必须独立审批，研究人民币报价不能直接成为生产配置。

### 7.3 Pro PI 按次 SKU

```text
customer_quota = checked(approved_customer_per_request_price × group/model factors)
```

固定 15 秒保存在请求和 Task 快照，但不是费用乘数。初版只允许倍率 1 的已验证 size；在按次与宽幅倍率
形成正式合同前，不开放宽幅，也不叠加 1.667。

### 7.4 预扣、结算、退款与 exposure

```text
validated request
  -> frozen billing probe
  -> quota estimate + sufficient-quota check
  -> TaskCreateAttempt held + sending
  -> trusted Task
  -> terminal settlement / refund
```

- 预扣上界覆盖最大时长和最大已发布 size multiplier；
- saturated oversized quota 必须因额度不足失败，不能溢出为负数；
- create unknown 保留 hold 并有界对账；
- 客户退款但 Provider 可能已计费时另写幂等 `ProviderCostExposure`；
- exposure 主指标使用 `customer_quota_released`，不能冒充供应商实际成本；
- exposure 按 channel、Link SKU、implementation v2、profile、reason 和滚动窗口隔离。

## 8. 飞彩任务、余额与用量对账

| 端点/字段 | 研究资料口径 | 正式凭据当前边界 |
| --- | --- | --- |
| `/v1/tasks` | 大写状态、时间、quota、结果与失败原因 | 目标生产 HTTPS Base URL 实测 404，当前不可作为对账源 |
| `/v1/tasks[].quota` | ÷500000 被描述为 CNY | 因列表接口不可用，当前无法取得任务级 Provider 成本 |
| `/v1/dashboard/billing/subscription` | `soft_limit_usd` 实际描述为 CNY 总额度 | 仅作 Provider 账户容量监控候选，需正式口径确认 |
| `/v1/dashboard/billing/usage` | `total_usage` 以分计，÷100 为 CNY | 正式接口可读，但只能作为账户累计趋势，不能拆成任务成本 |

研究任务列表状态 `QUEUED/IN_PROGRESS/SUCCESS/FAILURE` 与主轮询已观察到的
`queued/processing/in_progress/completed/failed` 是两套合同，必须独立归一化。任务列表恢复前不建立生产 parser
入口，也不能用主轮询 parser 猜测大写状态。

自动使用前必须验证：任务列表 `task_id` 与冻结顶层 `id` 的关系、quota 币种与换算、失败/退款/重试、
分页/过滤/延迟、Key 隔离和结果 URL 安全。不同单位必须显式记录来源、币种和换算版本；未知金额保持空值。

2026-08-05 受控黑盒中账户 usage 增量为 ¥56.88；Fast 隔离成功窗口增量为 180 分，与研究报价的 240 分
不一致。2026-08-06 渠道 52 五模型隔离窗口总增量为 144 分，且只有 Value 720p 成功；该账户聚合变化
仍不是任务级账单，不能反推单价或改写客户费用。

任务列表不覆盖 NEWAPI 客户结算，不用相似时间/模型猜测 create unknown，`fail_reason` 不原样回显。
只有能以冻结 request ID、幂等键或其它唯一键精确关联时，才可进入 CAS 幂等恢复。

## 9. SKU 与计费不变量

1. 10 个 Provider 模型分别绑定 10 个 Link SKU，不以相似参数、画幅或报价合并。
2. 固定分辨率属于 SKU 身份，不能请求期跨价格档。
3. SD2 `MinImages=1` 和 Pro PI 参考视频/按次计费存在于 capability/hash，不只存在于 converter。
4. capability 只发布已验证的 size、媒体角色、默认值和 billing mode。
5. v2 converter 与 billing probe 使用同一四元 size registry。
6. 飞彩端点是 Provider 对账源，不是客户计费账本。
7. 按秒和按次模式使用不同冻结 billing mode；乘数先校验后计费。
8. 客户账单读取结算日志，不按当前 Provider 报价回算。
9. Provider 实际金额未知时保持未知，不用客户 quota 冒充。
10. v1 capability、size resolver 和 adapter 不作为 v2 fallback 保留。
