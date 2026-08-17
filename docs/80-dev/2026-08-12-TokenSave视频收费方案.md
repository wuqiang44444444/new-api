---
status: in-progress
owner: Dev Team
last-reviewed: 2026-08-12
---

# TokenSave Seedance 2.0 视频收费方案

## 问题与目标

为客户模型 `doubao-seedance-2-0-260128-tokensave` 建立与 TokenSave 上游价格一致的按秒收费方案。
收费只使用创建时已经校验并冻结的请求参数，不依赖 TokenSave 可选的 `usage` 或结果时长；发送上游
请求前完成预扣，可信成功终态确认同一冻结金额，可信失败终态幂等全额退款。

本次只更新主数据库中的模型计费表达式和记录收费方案，不修改 TokenSave adapter 的媒体能力。完整
视频输入价格已经进入表达式，但当前 adapter 仍在发送前拒绝 `video_url`，因此不能把“有视频输入”
写成已发布能力。

## 当前实际情况

### 环境与配置变更

- 变更时间：2026-08-12（Asia/Shanghai）
- 环境：本地运行实例，SQLite 主数据库 `one-api.db`，后端监听 `8100`
- 客户模型：`doubao-seedance-2-0-260128-tokensave`
- Provider 模型：`doubao-seedance-2-0-260128`
- 计费模式：`tiered_expr`
- quota 换算：`QuotaPerUnit = 500000`
- 默认分组倍率：`1`
- 变更前数据库备份：`one-api-pre-tokensave-pricing-20260812.db`

数据库只更新 `billing_setting.billing_expr` 中该客户模型的一项；`ModelPrice` 和 `ModelRatio` 未为该模型
建立平行价格，`tiered_expr` 继续是唯一收费事实。配置更新不重算历史请求，也不改变已经创建 Task 中
冻结的旧表达式和旧金额。

### 上游基础价格

以下价格为分组倍率应用前的美元基础价：

| 生成方式 | 分辨率 | 无视频输入 | 有视频输入 |
| --- | --- | ---: | ---: |
| 文生视频 | 480p | $0.0679/秒 | $0.1653/秒 |
| 图生视频 | 480p | $0.0679/秒 | $0.1653/秒 |
| 参考生视频 | 480p | $0.0679/秒 | $0.1653/秒 |
| 文生视频 | 720p | $0.1462/秒 | $0.3559/秒 |
| 图生视频 | 720p | $0.1461/秒 | $0.3559/秒 |
| 参考生视频 | 720p | $0.1462/秒 | $0.3559/秒 |
| 文生/图生/参考生视频 | 1080p | $0.3647/秒 | $0.3647/秒 |

上游价格表没有把 `generate_audio` 或画面比例列为独立加价维度，因此当前表达式不因音频开关或
`ratio` 改变单价。

### 已写入数据库的表达式

```text
param("_task.resolution") == "1080p" ? tier("1080p", param("_task.duration_seconds") * 364700) : param("_task.has_video_input") == true ? (param("_task.resolution") == "720p" ? tier("720p_video_input", param("_task.duration_seconds") * 355900) : tier("480p_video_input", param("_task.duration_seconds") * 165300)) : param("_task.resolution") == "720p" ? (param("_task.input_mode") != "text" && param("_task.control_mode") != "reference" ? tier("720p_image_no_video_input", param("_task.duration_seconds") * 146100) : tier("720p_text_or_reference_no_video_input", param("_task.duration_seconds") * 146200)) : tier("480p_no_video_input", param("_task.duration_seconds") * 67900)
```

表达式系数使用 billingexpr v1 的美元/百万单位表达。以 `$0.0679/秒` 为例，每秒写为 `67900`，最终
quota 计算为：

```text
基础美元金额 = duration_seconds × 67900 / 1000000
客户 quota = 基础美元金额 × 500000 × group_ratio
```

### 请求参数到价格维度的映射

| 价格维度 | 冻结输入 | 规则 |
| --- | --- | --- |
| 时长 | `_task.duration_seconds` | 固定时长直接按请求秒数计算；合法范围由 Provider 模型规格先校验。 |
| 分辨率 | `_task.resolution` | TokenSave 当前允许 480p、720p、1080p。 |
| 视频输入 | `_task.has_video_input` | 只有非空 `video_url` 才为 `true`；当前 TokenSave adapter 仍拒绝该输入。 |
| 文生视频 | `_task.input_mode=text` | 720p 无视频输入使用 `$0.1462/秒`。 |
| 图生视频 | 非 text 且非 reference | 720p 无视频输入使用 `$0.1461/秒`。 |
| 参考生视频 | `_task.control_mode=reference` | 720p 无视频输入使用 `$0.1462/秒`。 |
| 客户分组 | 冻结的 `group_ratio` | 在基础美元价格换算为 quota 后应用；默认组为 1。 |

### 收费生命周期

1. **发送前预扣**：从与南向请求相同的类型化载荷生成计费探针，按当前表达式计算 quota；表达式、
   请求计费维度、分组倍率和预扣额随 create attempt / Task 冻结，资金 hold 成功后才发送 Provider POST。
2. **可信成功**：使用创建时冻结的请求参数和价格确认账单。TokenSave 当前终态没有 `usage` 和实际
   `duration_seconds`，因此成功时保持预扣额，不按当前数据库价格重算。
3. **可信失败**：`failed`、确认取消或过期等明确失败终态把目标 quota 设为 0，并通过幂等资金状态机
   全额退款；重复轮询或补偿不会重复退款。
4. **不可采信结果**：`unknown`、`RECONCILIATION_REQUIRED`、单次无效 JSON、未知状态或 ID 不匹配
   不等于业务失败，继续保留 hold，不自动退款、重发或换渠道。
5. **Provider 成本分账**：客户退款不表示 Provider 成本为零；未知 Provider 金额保持未知，不使用客户
   quota 冒充供应商成本。

### 智能时长边界

TokenSave 当前允许 `duration=-1`。系统会按 Provider 最大时长 15 秒预扣；由于本次真实终态返回没有
`usage` 或实际时长，成功后无法向下调整，只能保持 15 秒预扣。正式对外使用前必须明确选择：禁止该
模型使用智能时长，或把“智能时长按 15 秒收费”写入客户合同。

### 验证结果

- 主数据库更新前已完成在线备份，备份库和更新后主库 `PRAGMA integrity_check` 均返回 `ok`；
- `options` 中该模型仍为 `tiered_expr`，更新后的 option JSON 合法；
- 运行实例 `/api/pricing` 已在 2026-08-12 23:22:08 加载包含 `has_video_input` 的新表达式；
- 8 个一秒请求向量逐项执行表达式，分别命中 480p/720p/1080p、有/无视频输入、文生/图生/参考生档位，
  输出依次与 `0.0679`、`0.1653`、`0.1462`、`0.1461`、`0.1462`、`0.3559`、`0.3647`、`0.3647`
  美元/秒一致；
- 先前 4 秒、480p、无视频输入请求的冻结计算仍为 `$0.2716`，默认分组下等于 `135800 quota`；历史
  Task 不受本次配置更新影响；
- 新配置加载后于 23:49:29 发起的真实纯文本任务，冻结探针记录 `input_mode=text`、
  `has_video_input=false`，实际命中 `480p_no_video_input`；23:53:49 成功终态后账单由 `pending` 转为
  `settled`，最终仍为 `135800 quota`，只有一条消费日志且无退款、差额或重复扣费；
- 聚焦回归验证已确认：请求计费探针会校验时长和分辨率；成功且无上游 usage 时保持冻结预扣；明确
  失败退款可由补偿机制幂等完成。

## 优化方案

后续门禁如下：

1. 若要发布“视频输入：是”，先完成 TokenSave `video_url` 真实 Provider 验证，再修改 adapter 能力；
   不能因为价格分支已经存在就绕过当前 fail-closed 校验。
2. 对固定时长分别实测 480p、720p、1080p 的文生、图生和参考生请求，核对预扣、成功结算、失败退款
   及账单展示；有视频输入路径须等能力发布后再测。
3. 决定智能时长的客户合同。若没有可信终态时长或 usage，不得按猜测值退款或调整。
4. 修复 billing expression 通用 smoke test 缺少 `_task` 请求样本的问题，使管理员以后能通过标准设置
   入口安全编辑这类任务表达式，而不是依赖数据库直改。
5. 取得 TokenSave Provider 账单后，按渠道、Provider 模型和账期核对上游实际成本；客户 quota 与
   Provider 金额继续分账。
