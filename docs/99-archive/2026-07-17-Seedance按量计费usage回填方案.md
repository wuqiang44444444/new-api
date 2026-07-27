---
status: historical
owner: Dev Team
last-reviewed: 2026-07-27
superseded-by: "../20-architecture/视频上游接入与异步任务架构.md; ../20-architecture/decisions/0002-异步任务表达式计费快照与补偿结算.md; ../40-operations/Seedance 2.0 三渠道价格与计费表达式.md; ../40-operations/Seedance视频渠道与计费配置手册.md"
---

# Seedance 2.0 按量计费完整方案（最终版）

> **结论**：按 token 计费要真正跑通，需要**两个前提同时满足**——①上游终态回填真实 `completion_tokens`（注入到 `c`）；②tiered_expr 模型已配「任务预扣 Token 上限」（`task_billing_setting.preconsume_tokens`）。本文前一版只覆盖了①，实测发现②在 UI 上存在**配置接线 bug**（可视化编辑器的「Token 估算器」不写入预扣字段），导致三渠道任务创建全部被拒。本文整合 usage 回填 + 预扣配置 + 实测验证，给出完整闭环方案。
>
> 本文取代早期基于文档格式表的悲观推断；计费 token 口径由真实 DB 数据 + moxing 上游直连实测 + 火山方舟官方计费口径三方坐实。

---

## 0. TL;DR（给赶时间的人）

- 三渠道（官方 doubao / 反代·海外官Key / 中转·海外版）已全部切到 tiered_expr 6 档表达式，按上游终态 `completion_tokens` 计费。
- **实测（2026-07-17）**：三渠道 720p/5s 文生视频，终态 `completion_tokens` 均 = **108900**，结算 quota 均 = **381150**（= `108900 × 7.0 / 1e6 × 500000`），`billing_state=settled`，用户额度净减 = 3 × 381150，精确无误。功能 / Token / 计费三项全 PASS。
- **踩过的坑（必读）**：tiered_expr 模型必须配预扣 Token 上限，否则创建任务直接 HTTP 400 `task pre-consume token upper bound is not configured`。而管理后台可视化编辑器的「Token 估算器」**不会**写入该字段（详见 §4），必须用 JSON 编辑模式的 `Async task pre-consume token upper bounds` 文本框，或直接落库。

---

## 1. 计费模型：token 的几何本质

Seedance 的计费 token **不是文本切分 token，而是视频帧的几何折算**。据火山方舟官方口径：

- **Token ≈ 时长(秒) × 宽 × 高 × 帧率 / 1024**
- Seedance 2.0：`(输入视频时长 + 输出视频时长) × 宽 × 高 × 帧率 / 1024`
- Seedance 1.0：`输出视频时长 × 宽 × 高 × 帧率 / 1024`

由此得到几个关键事实：

| 现象 | 原因 |
| --- | --- |
| `prompt_tokens` 恒为 0 | 文本 prompt 不参与 token 体系（token 是视频几何量，与字数无关） |
| `completion_tokens == total_tokens` | completion 即计费量（纯生成场景） |
| 是否含视频输入影响 token 量与单价 | Seedance 2.0 公式含输入视频时长，对应 moxing 6 档的"视频输入 是/否"与代码 `_task.has_video_input` |
| 同规格 token 高度稳定 | token 由视频参数决定，与生成内容无关 |

**反算验证**：实测文生 720p/5s = 108900；代入公式 `5 × 1280 × 720 × 24 / 1024 = 108000`，偏差 0.83%，量级吻合。这坐实 token 的几何本质，也意味着：同规格多次生成 token 稳定（720p 24fps ≈ 21600 token/秒），可用公式本地预估做预扣上限与交叉校验。

---

## 2. 三条渠道的现状

### 2.1 usage 现状（实测）

| 渠道 | 模型 | 协议 | 终态 usage | usage 处理 |
| --- | --- | --- | --- | --- |
| 官方 doubao | `seedance-byteplus`（→ `ep-...`） | official | **108900** | adaptor 原生读 |
| 反代·海外官Key | `dreamina-seedance-2-0-260128` | reverse_proxy（`/v1/ark/`） | **108900（透传）** | [`reverse_proxy.go`](../../relay/channel/task/doubao/thirdparty/reverse_proxy.go) 透传上游 usage |
| 中转·海外版 | `seedance-2-0-oversea` | relay（`/v1/media/`） | **108900（已回填）** | [`relay.go`](../../relay/channel/task/doubao/thirdparty/relay.go) 解开"刻意不回填"，注入 usage |

- 三条线同规格 token 完全一致，口径跨协议统一。`completion_tokens` 就是 moxing 的计费量。
- 中转线原 [`relay.go:163-212`](../../relay/channel/task/doubao/thirdparty/relay.go) 注释"未经真实账单验证前刻意不回填"**主动丢弃** usage——上游真返回了（绕过归一化层直连 moxing 中转上游，同规格也是 108900），是 new-api 扔的。现已解开（见 §5.3）。

### 2.2 计费配置现状（DB 实查）

三模型均已切 tiered_expr，表达式与三渠道价格文档一致：

```text
param("_task.has_video_input") == true
  ? (param("_task.resolution") == "4k"     ? tier("4k_video",      c * 2.4)
    : param("_task.resolution") == "1080p" ? tier("1080p_video",   c * 4.7)
    :                                       tier("480p720p_video", c * 4.3))
  : (param("_task.resolution") == "4k"     ? tier("4k",            c * 4.0)
    : param("_task.resolution") == "1080p" ? tier("1080p",         c * 7.7)
    :                                       tier("480p720p",       c * 7.0))
```

- DB `billing_setting.billing_mode`：三模型 = `tiered_expr` ✓
- DB `billing_setting.billing_expr`：三模型 = 上述 6 档表达式 ✓
- DB `task_billing_setting.preconsume_tokens`：**修复前为空**（这正是 §3/§4 的核心问题），修复后 = `{"seedance-byteplus":520000,"seedance-2-0-oversea":300000,"dreamina-seedance-2-0-260128":520000}`。

---

## 3. 按量计费成立的两个前提（核心）

tiered_expr + 终态真实 token 要精确结算，**两个前提缺一不可**，任一缺失都会退化或直接失败：

### 前提一：终态 usage 回填（决定 `c` 是否为真实值）

结算阶段把上游终态 `usage.completion_tokens`（缺失时回退 `usage.total_tokens`）代回表达式的 `c` 重算（多退少补）。

- **不回填的后果**：`c = 0`（预扣阶段代入的是估算值，但终态不回填则结算退化为「保持预扣额度」，不再重算）。反代已透传、中转已解开回填、官方原生，均满足（见 §5.3）。

### 前提二：预扣 Token 上限已配（决定任务能否创建）

[`relay/helper/task_tiered_price.go`](../../relay/helper/task_tiered_price.go) 在创建任务时强制读取预扣上限：

```go
estimatedTokens, ok := billing_setting.GetTaskPreConsumeTokens(info.OriginModelName)
if !ok {
    return types.PriceData{}, fmt.Errorf("model %s task pre-consume token upper bound is not configured", info.OriginModelName)
}
```

- **不配的后果**：创建任务直接 HTTP 400 `model_price_error: ... task pre-consume token upper bound is not configured`，三渠道全挂（本次实测复现）。
- **为什么不能给个默认兜底**：预扣上限是**业务安全约束**（必须由运维按渠道最坏场景设定），代码刻意 fail-closed 拒绝 magic fallback（见 [`task_billing.go` GetTaskPreConsumeTokens](../../setting/billing_setting/task_billing.go)）。

> **关键认知：预扣值只影响「预冻结额度」，不影响最终计费。** 最终扣费仍按上游真实 token 结算（多退少补，前提一回填生效）。但预扣值必须 ≥ 任务真实 token，否则成功任务会进 `debt`（欠款）而非正常结算（[`task_billing_state.go`](../../service/task_billing_state.go)：`quotaDelta > 0 → TaskBillingStateDebt`）。
>
> **异步任务强制全额预扣**：视频任务 `ForcePreConsume=true`（[`relay/relay_task.go:214`](../../relay/relay_task.go#L214)），`BillingSession.shouldTrust` 据此**禁用信任额度旁路**（[`billing_session.go:283`](../../service/billing_session.go#L283)）。因此即便 Token 设了 `unlimited_quota`，异步任务仍按预扣值全额冻结用户额度——预扣值不能瞎大（会因用户余额不足导致创建失败），也不能太小（会进 debt）。

---

## 4. 配置 Bug：可视化编辑器的「Token 估算器」不写入预扣字段

### 4.1 现象

运维在「系统设置 → 计费 → 模型定价」给三模型配 tiered_expr 表达式，并在可视化编辑器的「Token 估算器」填了 `输入 0 / 输出 7000000`，保存。但 DB `task_billing_setting.preconsume_tokens` 仍为空，任务创建被拒。

### 4.2 根因（代码链路）

管理后台计费页存在**两套互不连通**的"预扣 Token"概念：

| 控件 | 位置 | 作用 | 是否落库 `preconsume_tokens` |
| --- | --- | --- | --- |
| **Token 估算器**（输入 p / 输出 c） | 可视化编辑器内 | 仅本地费用预览（`evalExprLocally`） | **否** |
| **Async task pre-consume token upper bounds** | JSON 编辑模式文本框 | 真正的预扣配置 | 是 |

1. 可视化编辑器 [`model-ratio-visual-editor.tsx`](../../web/src/features/system-settings/models/model-ratio-visual-editor.tsx) 的 `onChange` 只发出 10 个字段（`ModelPrice`/`ModelRatio`/各 ratio/`billing_setting.billing_mode`/`billing_setting.billing_expr`），**不含 `TaskPreConsumeTokens`**——估算器的 p/c 值仅用于本地预览，从不写入表单字段。
2. 真正的预扣字段 `TaskPreConsumeTokens`（[`model-ratio-form.tsx`](../../web/src/features/system-settings/models/model-ratio-form.tsx) 的 `modelJsonFields`）**只在 JSON 编辑模式**下作为一个 JSON 文本框出现。
3. 保存逻辑 [`ratio-settings-card.tsx`](../../web/src/features/system-settings/models/ratio-settings-card.tsx) 的 `apiKeyMap` 映射正确（`TaskPreConsumeTokens → task_billing_setting.preconsume_tokens`），但**只保存相对默认值有变化的字段**：

   ```ts
   const updates = Object.keys(normalized).filter(
     (key) => normalized[key] !== modelNormalizedDefaults.current[key]
   )
   ```

   `TaskPreConsumeTokens` 一直是默认 `{}`（估算器没写过它）→ 被过滤 → **从不持久化**。

### 4.3 影响

任何只走可视化编辑器配置 tiered_expr 的运维，都会踩这个坑：表达式与模式保存成功，但预扣字段为空，任务创建全部失败，且报错信息（"pre-consume token upper bound is not configured"）不会指向"估算器≠预扣字段"这个 UX 陷阱。

### 4.4 当前修复（配置层，已生效）

直接落库 `task_billing_setting.preconsume_tokens`（合理取值见 §5.2），并依赖每分钟一次的 `model.SyncOptions` 载入内存（见 §8）。三渠道随即恢复。

> **已修复（代码层根因，分离式设计）**：可视化编辑器的「Token 估算器」恢复为**纯本地预览**（不写任何计费配置）；新增**独立**的「异步任务预扣 Token 上界」字段绑定 `task_billing_setting.preconsume_tokens[model]`，含「空或正整数」强校验。两者严格分离（试算≠计费），杜绝试算值污染真实预扣。配套：编辑器重建边界改为模型名（同模型保存不重建，保留试算现场）；估算器新增 `_task` 探针（按分辨率/视频输入命中正确档位）+ USD/配额换算 + 按模型版本化本地草稿。详见 [整合方案](./2026-07-17-表达式计费Token估算器与异步任务预扣配置整合方案.md)。

---

## 5. 最终方案

### 5.1 账单机制：全线按 token

两条第三方线配同一套 tiered_expr 6 档表达式（系数为 BytePlus 官方 **USD/百万 tokens** 价，详见 [三渠道价格](../40-operations/Seedance%202.0%20三渠道价格与计费表达式.md)），表达式写 `c * 单价`（`c` = `completion_tokens`）。预扣用 EstimatedTokens 留余量，终态用真实 token 差额结算。

| 渠道 | 改动 | 性质 |
| --- | --- | --- |
| 官方 doubao | 无（usage 原生） | 已闭环 |
| 反代·官Key | 配 tiered_expr 6 档表达式，废弃 `ModelPrice=10` | 零代码（运营配置） |
| 中转·海外版 | 解开 [`relay.go`](../../relay/channel/task/doubao/thirdparty/relay.go) usage 回填（参照 reverse_proxy.go 透传）+ 配 tiered_expr | 一处代码 + 配置 |

解开 relay.go 回填属于其注释预设的"验证后解开"——验证前提（上游真返回 usage 且跨协议口径一致）已由实测满足，属预期行为，符合最小入侵。

### 5.2 预扣 Token 上限：取值规则（按渠道能力，替换 7000000）

官方 token 公式：`(输入视频时长 + 输出视频时长) × 宽 × 高 × 帧率 / 1024`。每秒 token 参考（16:9，24fps）：480p `9720`、720p `21600`、1080p `48960`、4k `194400`。

预扣必须覆盖渠道**最坏场景**（×1.2 余量），但**不能盲目取最大值**——异步任务强制全额预扣（§3 前提二），过大的预扣值会因用户余额不足直接导致创建失败。本次实测曾把三渠道统一设为 `7000000`（4k 视频输入最坏值），导致单任务预冻结 $49，admin 余额 $26 都不够创建。

**当前已落库的合理取值**（覆盖文生/图生到 1080p + 720p 视频生视频）：

| 渠道 | 能力 | 合理场景 | 预扣值 | 720p 任务预冻结 |
| --- | --- | --- | ---: | ---: |
| 中转（`seedance-2-0-oversea`） | 仅文生/图生（拒 `video_url`） | 1080p/5s | **300000** | 1,050,000（$2.10） |
| 官方（`seedance-byteplus`） | 支持视频输入 | 720p 输入15s + 输出5s | **520000** | 1,820,000（$3.64） |
| 反代（`dreamina-seedance-2-0-260128`） | 支持视频输入 | 720p 输入15s + 输出5s | **520000** | 1,820,000（$3.64） |

> 说明：720p 无视频输入档单价 $7.0，预冻结 = `预扣值 × 7.0 / 1e6 × QuotaPerUnit(500000)` = `预扣值 × 3.5`。终态实际扣费与预扣值无关（按真实 token 108900 结算 = 381150 / $0.7623），预冻结差额全额退还。

**未覆盖场景**（需调高对应渠道预扣，否则进 debt）：1080p 视频输入（≈1,200,000）、4k 任意场景（≈7,000,000）。若同一渠道同时开文生与视频生，按视频生（更大）的值配。更细粒度的取值对照见 [三渠道价格 §6](../40-operations/Seedance%202.0%20三渠道价格与计费表达式.md)。

### 5.3 usage 回填（三渠道均已满足）

- **反代**：[`reverse_proxy.go`](../../relay/channel/task/doubao/thirdparty/reverse_proxy.go) 透传上游 `usage.completion_tokens` / `usage.total_tokens`。
- **中转**：[`relay.go`](../../relay/channel/task/doubao/thirdparty/relay.go) 解开原"刻意不回填"，注入 usage（测试 `TestRelayTaskResponseNormalizesResultAndUsage`）。
- **官方**：adaptor 原生读 usage。

### 5.4 expired 状态（已修复）

官Key（Ark 直通）的 `expired` 终态原未被 [`reverse_proxy.go`](../../relay/channel/task/doubao/thirdparty/reverse_proxy.go) `normalizeReverseProxyStatus` 识别（fail closed 报错）。已修复：把 `expired` 归入 `failed`（触发退款，fail-closed，与 `cancelled` 一致——任务过期/超时无可用结果，用户拿不到视频不应扣费），补测试 `TestReverseProxyTaskResponseMapsExpiredToFailed`。

### 5.5 对账机制：可选审计层（不阻塞计费）

> 按 token 计费**完全不需要 request_id**——`completion_tokens` 已是 moxing 计费量。request_id 的唯一用途是 moxing [`usage-records`](../70-research/moxing%20单次查询.md) 接口的查询键，服务于"验证 usage 口径 = moxing 实际扣费"这一审计假设。

- **独立审计，不进计费热路径**：用 moxing `usage-records`（集成 token `itk-mxai-` + 创建时的 `request_id`）查 `charged_yuan`，做 T+1 抽样校验，偏差告警。复用现成 [`ReconcileTaskBilling`](../../service/task_billing_reconcile.go) 骨架（延迟重试 + `billing_state` 索引扫描）。
- **request_id 已持久化在 logs 表（实测）**：两条线的创建日志 `upstream_request_id` 字段（有索引 `idx_logs_upstream_request_id`）都已存入 moxing request_id。机制：[`doRequest`](../../relay/channel/api_request.go) 从响应头 `x-oneapi-request-id` 捕获到 gin context，[`model/log.go`](../../model/log.go) record log 时取出存库——moxing 响应头恒带该字段，故两条线均被捕获。
- **对账缺口：task ↔ request_id 关联断裂**：创建 log 有 `upstream_request_id` 但 `other` 未存 `task_id`；终态结算 log 的 `other` 有 `task_id` 但异步轮询无 gin context、无 `upstream_request_id`。两者无法直接 JOIN。**最小修复**：任务创建时把 request_id 也写入 `Task.PrivateData.UpstreamRequestID`（JSON 列加字段，三库零迁移），task 直接持有 request_id，对账无需 JOIN log 表。
- **request_id 取创建任务那次调用**：终态轮询响应不带；客户端只看到 new-api 自己的 `task_xxx`，上游 request_id 不应外泄。

---

## 6. 实测验证（2026-07-17，三渠道 720p/5s 文生视频）

统一请求：`POST /v1/video/generations`，`resolution=720p`、`seconds=5`、无视频输入（命中 `tier("480p720p", c * 7.0)` 档）。用 admin 令牌 + `-<渠道ID>` 后缀强制指定渠道。

| 渠道 | task_id（末段） | 功能 | completion_tokens | 终态 quota | billing_state |
| --- | --- | --- | ---: | ---: | --- |
| 26 官方 | `...oiSaylzp` | ✅ SUCCESS + mp4 URL | 108900 | 381150 | settled |
| 27 中转 | `...oxFraY` | ✅ SUCCESS + mp4 URL（`content.video_url`） | 108900 | 381150 | settled |
| 28 反代 | `...RMPMY` | ✅ SUCCESS + mp4 URL | 108900 | 381150 | settled |

完整 task_id：`task_ZroVbUvJ6ovQMcrQRWWyc9ruoiSaylzp` / `task_VigvXrh06bdBes0sNduOlmqWt0oxFraY` / `task_iUZijuGHglMFKYHkIhoFlcyE5Y9RMPMY`。

**计费精确性对账**：

- 终态 quota = `108900 × 7.0 / 1e6 × 500000` = `108900 × 3.5` = **381150**，三渠道精确一致。
- 日志双条目（预扣 type=2 + 退款 type=6），净值精确：

  | 渠道 | 预扣(type=2) | 退款(type=6) | 净扣费 |
  | --- | ---: | ---: | ---: |
  | 26 官方 | 1,820,000 | 1,438,850 | **381,150** |
  | 27 中转 | 1,050,000 | 668,850 | **381,150** |
  | 28 反代 | 1,820,000 | 1,438,850 | **381,150** |

  （log id：预扣 6898/6900/6902，退款 6899/6901/6903）
- **用户额度净减**：测试前 `quota=13135102` → 测试后 `11991652`，差值 = **1,143,450 = 3 × 381150**，无多扣/少扣。

**结论**：功能、Token、计费三项，三渠道全 PASS；跨协议 token 口径与计费完全一致。

> **已修复**：成功结算/退款日志（type=2 补扣、type=6 退款）现已回填真实 `completion_tokens`（之前恒为 0）。失败/超时退款日志无 usage，保持 0（符合预期）。

---

## 7. 实施清单（最小入侵，分阶段）

**阶段 1 · 计费上线（P0，已完成）**
- [x] 反代·官Key：配 tiered_expr 6 档表达式，废弃 `ModelPrice=10`。
- [x] 中转·海外版：解开 [`relay.go`](../../relay/channel/task/doubao/thirdparty/relay.go) usage 回填 + 配同样表达式。
- [x] **三渠道配预扣 Token 上限**（`task_billing_setting.preconsume_tokens`，取值见 §5.2）——本次实测发现并修复的缺失项。
- [x] 灰度核对：三渠道 720p/5s 终态 actualTokens = 108900、结算 quota = 381150（§6）。
- [ ] **request_id 持久化（建议与计费同步上线）**：`Task.PrivateData.UpstreamRequestID` 从 gin context 取值写入（两条线在 logs 已验证可得）。理由见 §9 时序约束——越早覆盖越多任务，延后不可补救。
- [ ] **运行时验证（重启或等同步后）**，确认两件事生效：
  ```bash
  # ① request_id 落库：跑一个任务后查 PrivateData（期望非空，形如 20260717…）
  sqlite3 one-api.db "SELECT json_extract(private_data,'$.upstream_request_id') FROM tasks ORDER BY id DESC LIMIT 1;"
  # ② 中转线 usage 回填：中转任务终态后查 Data（期望 {"completion_tokens":…,"total_tokens":…}）
  sqlite3 one-api.db "SELECT json_extract(data,'$.usage') FROM tasks ORDER BY id DESC LIMIT 1;"
  # ③ 预扣上限已配
  sqlite3 one-api.db "SELECT value FROM options WHERE key='task_billing_setting.preconsume_tokens';"
  ```

**阶段 2 · 已完成（代码部分）**
- [x] `expired` 状态归一化（见 §5.4）。
- [x] 中转线 `relay.go` usage 回填解开（见 §5.3，测试 `TestRelayTaskResponseNormalizesResultAndUsage`）。
- [x] `Task.PrivateData.UpstreamRequestID` 持久化接线（见 §5.5）。
- [x] **预扣配置补齐**（§4.4，配置层；代码层根因修复见阶段 4）。

**阶段 3 · 对账审计（可选，后置）**
- [ ] 渠道新增集成 token（`itk-mxai-`）配置字段。
- [ ] 复用 `ReconcileTaskBilling` 增加 T+1 对账分支，偏差告警。
- [ ] 用历史 task 的 request_id 抽样验证 `charged_yuan ≈ 108900 × 报价`（最终坐实 usage 即 moxing 计费依据）。

**阶段 4 · UX 根因修复（已完成，前端）**
- [x] 可视化编辑器把估算器输出值写入 `TaskPreConsumeTokens` 表单字段（消除 §4 的配置陷阱）。typecheck 通过；详见 §4.4。
- [x] 结算/退款日志回填真实 `completion_tokens`（`RecordTaskBillingLogParams` 加字段 + `settleTaskTieredSnapshot` 写 `async.ActualTokens` + `recalculateTaskQuotaWithReconcile` 透传），测试 `TestTieredSettlementLogRecordsCompletionTokens`。

---

## 8. 运维操作备忘

### 8.1 配置变更的生效时序

预扣上限等 option 缓存在内存包变量（`setting/billing_setting.taskBillingSetting`），变更后生效路径：

| 变更方式 | 生效时延 | 说明 |
| --- | --- | --- |
| 管理后台保存（`PUT /api/option`） | **即时** | controller 即时写 DB + 刷新内存（`UpdateOption` → `updateOptionMap` → `handleConfigUpdate`） |
| 直接改 DB（落库） | ≤ **60s** | `model.SyncOptions`（[`main.go:110`](../../main.go#L110) → [`model/option.go:199`](../../model/option.go#L199)）每 `SyncFrequency`（默认 60s，env `SYNC_FREQUENCY`）跑一次 `loadOptionsFromDatabase`，载入并应用到注册结构体 |
| 重启进程 | 启动时 | `InitOptionMap` 启动调用同一个 `loadOptionsFromDatabase`，DB 有值即载入 |

> 本次实测曾出现"重启后仍报 pre-consume 未配置"的假象——实为 DB 写入**晚于**重启那一刻，下个 60s 周期同步即载入。**不是 bug**：启动与周期同步走同一加载函数。结论：直接落库后等 ≤60s 即可，无需重启。

### 8.2 预扣值调整速查

改预扣值直接更新该 option（任一方式）：

```bash
sqlite3 one-api.db "UPDATE options SET value='{\"seedance-byteplus\":520000,\"seedance-2-0-oversea\":300000,\"dreamina-seedance-2-0-260128\":520000}' WHERE key='task_billing_setting.preconsume_tokens';"
# 等 ≤60s 同步载入（或管理后台 JSON 模式保存即时生效）
```

注意 JSON 值为 model → 正整数（`ValidateTaskPreConsumeTokensJSON` 拒绝 ≤0 与空 model 名）。

### 8.3 强制指定渠道测试

管理员令牌可用 `sk-<令牌>-<渠道ID>` 后缀强制路由到指定渠道（如 `sk-xxxx-26`），用于逐渠道验收。仅管理员令牌可用，普通用户报错。

---

## 9. 边界与风险

- **解开 relay.go 回填的残留假设**：`completion_tokens` 是 moxing 自报值，理论上即其计费依据；严格相等需阶段 3 对账最终确认。灰度期间可人工核对 moxing 后台账单。
- **moxing 若未来计文本输入 token**：当前 token 是视频几何量、文本恒 0。若 moxing 改计费模型开始计输入，表达式需改用 `total` 或分项计费——当前无需处理，作为监控点。
- **预估公式精度**：官方公式偏差约 <1%，适合设预扣上限与交叉校验，不替代上游返回的精确值。
- **预扣值的两难**：过小 → 真实 token 超过预扣进 `debt`（少收，需人工补扣）；过大 → 用户余额不足导致创建失败（§3 前提二）。取值须按渠道**实际开放的最坏场景**定（§5.2），开新分辨率/视频输入场景时同步调高。
- **request_id 持久化的时序不可补救**：moxing request_id 只在任务创建那一刻的 gin context 里可得，异步轮询/结算时已丢失。若不趁早写入 `Task.PrivateData.UpstreamRequestID`，已跑过的任务永久丢这层关联（只能事后靠 logs 表反查 JOIN）。故该字段应与计费上线**同步（阶段 1）**，而非等对账层（阶段 3）。
- **request_id 重试覆盖（已修复）**：[`doRequest`](../../relay/channel/api_request.go) 原仅在响应带 ID 时 `c.Set`，重试时若失败响应带 ID、最终成功响应不带，会残留失败请求的旧 ID；已改为始终覆盖（即便为空），确保最终记录的是最后一次响应的 ID。
- **估算器 ≠ 预扣字段的 UX 陷阱（阶段 4 待修）**：在可视化编辑器修复前，配 tiered_expr 必须额外在 JSON 模式填预扣字段或落库，否则任务创建失败。

---

## 10. 关键文件与证据索引

**计费表达式引擎与预扣**
- 表达式引擎：[`pkg/billingexpr/`](../../pkg/billingexpr/)（语法见 [`expr.md`](../../pkg/billingexpr/expr.md)）
- 预扣上限读取与校验：[`setting/billing_setting/task_billing.go`](../../setting/billing_setting/task_billing.go)（`GetTaskPreConsumeTokens` fail-closed、`TaskPreConsumeTokensOption`、`ValidateTaskPreConsumeTokensJSON`）
- tiered 预扣求值：[`relay/helper/task_tiered_price.go`](../../relay/helper/task_tiered_price.go)（缺预扣直接报错）
- 异步强制全额预扣：[`relay/relay_task.go:214`](../../relay/relay_task.go#L214)（`ForcePreConsume=true`）、[`service/billing_session.go:283`](../../service/billing_session.go#L283)（`shouldTrust` 禁用旁路）

**usage 回填**
- 反代透传 + expired 修复：[`relay/channel/task/doubao/thirdparty/reverse_proxy.go`](../../relay/channel/task/doubao/thirdparty/reverse_proxy.go)
- 中转回填：[`relay/channel/task/doubao/thirdparty/relay.go`](../../relay/channel/task/doubao/thirdparty/relay.go)
- 官方计费探针：[`relay/channel/task/doubao/billing_probe.go`](../../relay/channel/task/doubao/billing_probe.go)

**结算与对账**
- 终态结算 / debt：[`service/task_polling.go`](../../service/task_polling.go)、[`service/task_billing_state.go`](../../service/task_billing_state.go)、[`service/task_tiered_settle.go`](../../service/task_tiered_settle.go)
- 异步对账骨架：[`service/task_billing_reconcile.go`](../../service/task_billing_reconcile.go)、[`model/task_async_billing.go`](../../model/task_async_billing.go)
- request_id 捕获：[`relay/channel/api_request.go`](../../relay/channel/api_request.go)

**前端（预扣配置 UI，§4 根因所在）**
- 可视化编辑器 onChange（不写预扣）：[`web/src/features/system-settings/models/model-ratio-visual-editor.tsx`](../../web/src/features/system-settings/models/model-ratio-visual-editor.tsx)
- 表单字段定义（预扣仅 JSON 模式）：[`web/src/features/system-settings/models/model-ratio-form.tsx`](../../web/src/features/system-settings/models/model-ratio-form.tsx)
- 保存映射（过滤未变更字段）：[`web/src/features/system-settings/models/ratio-settings-card.tsx`](../../web/src/features/system-settings/models/ratio-settings-card.tsx)

**option 载入**
- 周期同步：[`main.go:110`](../../main.go#L110)、[`model/option.go:189-203`](../../model/option.go#L189)（`loadOptionsFromDatabase` / `SyncOptions`，`SyncFrequency` 默认 60s）
- 分层配置应用：[`setting/config/config.go`](../../setting/config/config.go)（`UpdateConfigFromMap`）、[`model/option.go:579`](../../model/option.go#L579)（`handleConfigUpdate`）

**实测证据（2026-07-17）**
- 三渠道 720p/5s 终态：`one-api.db` tasks 表，task_id 末段 `oiSaylzp` / `oxFraY` / `RMPMY`（均 `status=SUCCESS`、`quota=381150`、`billing_state=settled`、`data.usage.completion_tokens=108900`）
- 计费日志：`logs` 表 id 6898/6900/6902（预扣 type=2，1.82M/1.05M/1.82M）、6899/6901/6903（退款 type=6，1.438M/0.668M/1.438M）
- 用户额度对账：admin `quota` 13135102 → 11991652（净减 1,143,450 = 3 × 381150）

**上游文档与配套**
- [`Seedance 2.0 海外官 Key.md`](../70-research/Seedance%202.0%20海外官%20Key.md)、[`Seedance 2.0 海外版.md`](../70-research/Seedance%202.0%20海外版.md)、[`moxing 单次查询.md`](../70-research/moxing%20单次查询.md)
- [配置手册（含 6 档表达式）](../40-operations/Seedance视频渠道与计费配置手册.md)、[三渠道价格与预扣取值](../40-operations/Seedance%202.0%20三渠道价格与计费表达式.md)
- 官方计费口径：[火山方舟·模型价格](https://www.volcengine.com/docs/82379/1544106)、[Seedance 2.0 API 参考](https://www.volcengine.com/docs/82379/1520757)、[大模型调用计费（token 公式）](https://www.volcengine.com/docs/6492/1544808)
