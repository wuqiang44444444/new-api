---
status: current
owner: Dev Team
last-reviewed: 2026-07-23
---

# Seedance 2.0 三渠道价格与计费表达式

> 给运维/商务：Seedance 2.0 在 new-api 里通过三条 DoubaoVideo 渠道接入（BytePlus 官方直连、Moxing 反代·海外官Key、Moxing 中转·海外版）。本文给出三条渠道的**官方权威价格**、**计费 token 公式**，以及**可直接配置的 tiered_expr 表达式**。
>
> 配置入口与操作步骤见 [Seedance 视频渠道与计费配置手册](./Seedance视频渠道与计费配置手册.md)；表达式完整语法见 [表达式计费系统设计](../../pkg/billingexpr/expr.md)；任务快照、usage 回填与结算边界见 [视频上游接入与异步任务架构](../20-architecture/视频上游接入与异步任务架构.md)。

## 1. 核心结论

1. **三条渠道计费口径完全一致**：都按视频几何 token 计费，公式相同（见 §3），`completion_tokens` 即计费量。
2. **Moxing 就是 BytePlus 官方代理**：Moxing 的 ¥ 报价 = BytePlus 官方 USD 价 × **¥7/$**（逐档验证，见 §2），无额外加价。
3. **三条渠道的 tiered_expr 表达式系数相同**：因为 new-api 内部额度按美元计（`QuotaPerUnit`），直接填 BytePlus 官方 **USD 价**最准，无需从 ¥ 反算（避免汇率取值误差）。三条渠道配在各自公开模型名上即可，表达式可复用同一套。
4. **唯一结构差异**：中转·海外版实际只到 1080p（无 4k），官方/反代到 4k——表达式里的 4k 分支对中转线无害（不会触发）。

## 2. 三渠道价格表

**BytePlus ModelArk 官方（USD / 百万 tokens，权威）**——[官方定价页](https://docs.byteplus.com/en/docs/ModelArk/1544106)：

| 分辨率 | 含视频输入（视频生） | 不含视频输入（文生/图生） |
| --- | --- | --- |
| 480p / 720p | $4.3 | $7.0 |
| 1080p | $4.7 | $7.7 |
| 4k | $2.4 | $4.0 |

**Moxing 反代·海外官Key（¥ / 百万 tokens）**——[`dreamina-seedance-2-0-260128`](../70-research/Seedance%202.0%20海外官%20Key.md)：

| 分辨率 | 含视频输入 | 不含视频输入 | 折算 USD（÷7） |
| --- | --- | --- | --- |
| 480p / 720p | ¥30.1 | ¥49 | $4.3 / $7.0 |
| 1080p | ¥32.9 | ¥53.9 | $4.7 / $7.7 |
| 4k | ¥16.8 | ¥28 | $2.4 / $4.0 |

**Moxing 中转·海外版（¥ / 百万 tokens）**——[`seedance-2-0-oversea`](../70-research/Seedance%202.0%20海外版.md)：价格与反代相同（480p/720p/1080p 三档；文档限制实际只开 480p/720p，价格表列了 1080p）。

> **汇率验证**：官方每档 USD 价 × 7 = Moxing ¥ 价（$7.0→¥49、$4.0→¥28、$4.3→¥30.1、$7.7→¥53.9…逐档吻合）。故三条渠道在 new-api（美元计价）里**系数完全相同**。

## 3. 计费 token 公式（官方）

```
token ≈ (输入视频时长 + 输出视频时长) × 输出宽 × 输出高 × 帧率 / 1024
```

- 文生/图生（无输入视频）：输入时长 = 0，只算输出。
- `completion_tokens`（上游终态 `usage` 返回）即为计费量；`prompt_tokens` 恒 0（文本不参与）。
- 同规格 token 稳定（几何量，与内容无关）。

**实测验证**：文生 720p/5s 实测 `completion_tokens=108900`；按公式 `5 × 1280 × 720 × 24 / 1024 = 108000`，偏差 <1%。官方 1080p/5s 无视频报价 $1.87/视频，按 `$7.7/M × 244800 token = $1.88` 吻合。

## 4. 计费表达式（三渠道通用，6 档）

> 系数均为 **BytePlus 官方 USD / 百万 tokens**。`c` = 上游返回的 `completion_tokens`。每个分支必须被 `tier()` 包裹（校验要求）。

```text
param("_task.has_video_input") == true
  ? (param("_task.resolution") == "4k"     ? tier("4k_video",      c * 2.4)
    : param("_task.resolution") == "1080p" ? tier("1080p_video",   c * 4.7)
    :                                       tier("480p720p_video", c * 4.3))
  : (param("_task.resolution") == "4k"     ? tier("4k",            c * 4.0)
    : param("_task.resolution") == "1080p" ? tier("1080p",         c * 7.7)
    :                                       tier("480p720p",       c * 7.0))
```

要点：
- 末尾 `else` 同时覆盖 `480p`/`720p`（同价）并兜住任何未匹配值，保证总有一个 `tier()` 命中。
- `param("_task.resolution")` 中 **4k 是小写 `"4k"`**；`param("_task.has_video_input")` 仅视频生视频为 `true`（文生/图生/参考图均 `false`）。字段由 DoubaoVideo 适配器在创建任务时冻结，预扣与结算读到同一份。
- **视频 token 只走 `c`，不走 `p`**（`p` 恒 0）。写成 `p * 单价` 会算出 0。

若商务决定不区分视频输入（统一按"不含视频输入"价），可简化为 3 档：

```text
param("_task.resolution") == "4k"
  ? tier("4k", c * 4.0)
  : param("_task.resolution") == "1080p"
    ? tier("1080p", c * 7.7)
    : tier("480p720p", c * 7.0)
```

### 4.1 变体模型（fast / mini，仅官方直连场景）

BytePlus 官方还有两个轻量变体（**仅 BytePlus 直连渠道**，Moxing 两份文档未提供），不支持 1080p，单价只按「是否含视频输入」分 2 档（不分分辨率）：

| 模型 | 含视频输入 | 不含视频输入 |
| --- | --- | --- |
| `dreamina-seedance-2-0-fast-260128` | $3.3 | $5.6 |
| `dreamina-seedance-2-0-mini-260615` | $2.1 | $3.5 |

fast 表达式：

```text
param("_task.has_video_input") == true
  ? tier("fast_video", c * 3.3)
  : tier("fast", c * 5.6)
```

mini 表达式：

```text
param("_task.has_video_input") == true
  ? tier("mini_video", c * 2.1)
  : tier("mini", c * 3.5)
```

> fast/mini 不支持 1080p（请求传 1080p 会被上游拒绝），故表达式无需 1080p/4k 分支。token 用量仍由分辨率×时长决定，只是单价不分档。

## 5. 三渠道配置差异

三条渠道用**同一套表达式**，差别只在渠道与公开模型名：

| 渠道 | 视频上游方案 | 公开模型名 | 支持分辨率 | 表达式 |
| --- | --- | --- | --- | --- |
| BytePlus 官方直连 | 官方协议（official） | `seedance-byteplus`（自定） | 480p/720p/1080p/4k | §4 6 档 |
| Moxing 反代·海外官Key | 第三方反代协议 | `dreamina-seedance-2-0-260128` | 480p/720p/1080p/4k | §4 6 档 |
| Moxing 中转·海外版 | 第三方中转协议 | `seedance-2-0-oversea` | 480p/720p（文档开）；价格表含 1080p | §4 6 档（4k 分支不触发） |

渠道接入（地址、Key、路径后缀）见配置手册 [§2.2/§2.3/§2.4](./Seedance视频渠道与计费配置手册.md)。表达式配在**公开模型名**上（不是映射后的上游名），计费类型选「表达式计费」，把上面的表达式贴入。

## 6. 预扣 Token 上限（必须配，且必须覆盖最坏情况）

按 token 计费的模型，必须同时配「异步任务预扣 Token 上界」。预扣阶段把它代入 `c` 估算冻结额度；终态实际费用较低时退还差额，超过预扣上界时进入 `debt`，不会自动突破上界补扣。

> **⚠️ 预扣必须按"最坏情况"设，不能只按典型值。** tiered_expr 任务结算时，若实际 token 算出的费用**超过预扣**，系统不会补扣，而是把任务标记为 `debt`（欠款）待人工处理（[`task_billing_state.go:104`](../../service/task_billing_state.go#L104)：`quotaDelta > 0 → TaskBillingStateDebt`）。所以预扣上限必须覆盖该渠道可能出现的**最大 token 用量**。

官方 token 公式：`(输入视频时长 + 输出视频时长) × 宽 × 高 × 帧率 / 1024`。

- **文生 / 图生**（无输入视频）：只算输出时长。
- **视频生视频**（有输入视频）：输入视频时长（官方上限 15 秒）+ 输出时长。

> 第三方中转（`seedance-2-0-oversea`）拒绝 `video_url`（[`relay.go`](../../relay/channel/task/doubao/thirdparty/relay.go)），只有文生/图生，按输出时长算即可；**官方与反代支持视频输入，必须按"输入+输出"算**。

**每秒 token 参考**（官方公式，16:9，24fps）：480p `9 720`、720p `21 600`、1080p `48 960`、4k `194 400`。

**预扣上限推荐**（×1.2 余量，按渠道能力选最大场景）：

| 渠道能力 | 最大场景 | 预扣下限 |
| --- | --- | --- |
| 仅文生/图生（中转） | 720p 输出 5s | ~130 000 |
| 仅文生/图生（中转） | 1080p 输出 5s | ~300 000 |
| 支持视频输入（官方/反代） | 720p 输入 15s + 输出 5s | **~520 000** |
| 支持视频输入（官方/反代） | 1080p 输入 15s + 输出 5s | **~1 200 000** |
| 支持视频输入（官方/反代） | 4k 输入 15s + 输出 15s | **~7 000 000** |

> 若同一渠道同时开文生与视频生，按**视频生**（更大）的值配预扣。

### 6.1 「Token 估算器」输入值

系统 → 模型价格里的「Token 估算器」需要填写 `p`（输入 Token）和 `c`（输出 Token）。Seedance 的文本、参考图和输入视频都**不计入 `p`**；即使任务包含输入视频，上游仍把「输入视频时长 + 输出视频时长」对应的全部几何 token 记在 `completion_tokens`。因此：

- **输入 Token（`p`）：统一填 `0`**。
- **输出 Token（`c`）：填写对应场景的预扣 token 上限**。

| 渠道能力 | 最大场景 | ×1.2 后的计算值 | 输入 Token（`p`） | 输出 Token（`c`，建议填写） |
| --- | --- | ---: | ---: | ---: |
| 仅文生/图生（中转） | 720p 输出 5s | 129 600 | **0** | **130 000** |
| 仅文生/图生（中转） | 1080p 输出 5s | 293 760 | **0** | **300 000** |
| 支持视频输入（官方/反代） | 720p 输入 15s + 输出 5s | 518 400 | **0** | **520 000** |
| 支持视频输入（官方/反代） | 1080p 输入 15s + 输出 5s | 1 175 040 | **0** | **1 200 000** |
| 支持视频输入（官方/反代） | 4k 输入 15s + 输出 15s | 6 998 400 | **0** | **7 000 000** |

这里的「输入 Token」是表达式变量 `p`，不是公式中的「输入视频时长」，两者不要混淆。真正控制异步任务预扣的是同页「异步任务预扣 Token 上限」里的模型配置；Token 估算器只做费用预览，不能代替该配置。

试算器提供 `_task.has_video_input` 和 `_task.resolution` 请求探针，可预览 480p/720p、1080p、4k 与有无视频输入的分支。试算值按模型保存在浏览器本地，只用于显示命中档位、USD 费用和不含用户组倍率的配额；不会写入服务器计费配置。真实预扣只读取同页独立保存的「异步任务预扣 Token 上界」。

## 7. 上线前必验

1. **usage 回填**：三条渠道终态必须返回 `usage.completion_tokens`（反代透传、中转归一化回填、官方原生）；缺失时回退 `total_tokens`，两者都不可用时保持原预扣。实现边界见 [视频上游接入与异步任务架构 §11](../20-architecture/视频上游接入与异步任务架构.md#11-计费边界)。
2. **小流量核对**：跑一个 720p/5s 文生任务，确认终态 `completion_tokens ≈ 108 900`、结算扣费 ≈ `108900 × 7.0 / 1e6 × QuotaPerUnit`。
3. **expired 状态**：官Key 的 `expired` 终态已归一化为 `failed`（退款），无需额外处理。

## 8. 来源

- BytePlus ModelArk 官方定价：[Pricing – ModelArk](https://docs.byteplus.com/en/docs/ModelArk/1544106)（USD/百万 tokens，含 token 公式与每视频价示例）
- 火山方舟（中国区，¥）：[模型价格](https://www.volcengine.com/docs/82379/1544106)、[Seedance 2.0 API 参考](https://www.volcengine.com/docs/82379/1520757)
- Moxing 反代：[`Seedance 2.0 海外官 Key.md`](../70-research/Seedance%202.0%20海外官%20Key.md)
- Moxing 中转：[`Seedance 2.0 海外版.md`](../70-research/Seedance%202.0%20海外版.md)
