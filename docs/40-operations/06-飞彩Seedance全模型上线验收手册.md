---
status: accepted
owner: Dev Team
last-reviewed: 2026-08-10
---

# 06 飞彩 Seedance 全模型上线验收手册

## 1. 目的与边界

本手册用于在隔离分组中，以正式 HTTPS、目标生产凭据和 ModelArk V3 客户入口验收飞彩精确
Provider 协议版本。当前生产渠道使用代码协议 `media_arrays_v2`；无后缀模型若使用另一套 `/v3`
并从零取得独立证据，不能继承 v2 验收结果。任何当前版本都不保留或测试 v1 fallback。

所有证据必须脱敏。不得在本文或验收产物中写入真实 Key、Cookie、完整签名 URL、上游任务 ID、
真实渠道 ID或内部管理地址。

## 2. v1 移除前置审计

上线窗口开始前，逐环境确认：

- 飞彩 v1 Channel 与 Ability 已停流；
- v1 非终态 Task、create attempt、hold、结算、退款和 exposure 数量均为零；
- 没有仍需访问的 v1 历史内容；
- 客户模型、Channel、Model Mapping、价格和运维配置不引用 v1；
- 已保存旧版本二进制和配置快照，必要时可整体回滚；
- 新版本不会在运行时解析 v1 adapter。

任一项不满足则停止上线，不临时打开兼容分支。

## 3. 验收准备

1. 创建隔离测试分组、测试用户和专用 API Key。
2. 配置正式可验证 HTTPS Base URL、单一目标 Key 和 10 个精确 model mapping。
3. 导出客户模型、唯一 Channel、Provider 模型、代码协议、价格和 Provider exposure 快照。
4. 每个用例只发送一次创建请求；记录平台 `request_id`，不得用客户幂等键重放。
5. 准备可验证的 HTTPS 图片、音频和视频；记录 MIME、大小、有效期和所有权，不保存完整源 URL 到共享证据。
6. 准备余额前后快照、任务列表、usage/subscription 和 NEWAPI 结算日志的只读核对方式。
7. 记录飞彩 Ability 停流方式；回滚只阻止新流量，不能中断已创建 Task 按冻结 Channel 与协议的轮询和结算。

## 4. 10 模型黑盒矩阵

每一行必须独立创建、轮询、下载、核账；其它模型的成功不能代替。

| # | 客户模型 | 必验成功场景 | 必验拒绝/边界 | 账单重点 |
| ---: | --- | --- | --- | --- |
| 1 | `seedance-2.0-mini-720p` | 4 秒、15 秒；已批准 size；图片/音频允许组合 | 3/16 秒；未登记 ratio；10 图/4 音频/任意视频 | 按秒、模型独立单价 |
| 2 | `seedance-2.0-sd2-720p` | 1 图+11 秒、9 图+15 秒；已批准两画幅 | 无图；10 图；10/16 秒；任意音频/视频；省略 duration | 按秒；失败是否扣费 |
| 3 | `seedance-2.0-fast-720p` | 4/15 秒；已批准 size；图片/音频 | 越界时长、媒体和未登记 ratio | 按秒；记录时延但不宣称 SLA |
| 4 | `seedance-2.0-value-720p` | v2 下 16:9、9:16；4/15 秒；内容下载 | 未登记画幅、媒体越界 | 按秒；与 v1 研究结果分开 |
| 5 | `seedance-2.0-standard-720p` | 独立执行同类场景 | 不接受 value 模型证据代替 | 独立单价、独立 Provider 任务 |
| 6 | `seedance-2.0-value-1080p` | 每个批准的 1080p size；4/15 秒；Range | 720p/4K resolution、未登记 ratio | 按秒、高分辨率成本 |
| 7 | `seedance-2.0-standard-1080p` | 独立执行 1080p 场景 | 不接受 value size/账单代替 | 独立单价 |
| 8 | `seedance-2.0-value-4k` | 每个批准的 4K size；最大时长；Range | 非真实 4K、未登记 ratio、额度不足 | quota 上界、内容大小 |
| 9 | `seedance-2.0-standard-4k` | 独立执行 4K 场景 | 不接受 value 证据代替 | 独立单价、quota 饱和 |
| 10 | `seedance-2.0-pro-pi-720p` | 图片、音频、视频各自及批准组合；固定 15 秒 | 14/16 秒；4 视频；错误 role；未批准宽幅 | 固定按次，duration 不乘价 |

只有已经进入 v2 size registry 的组合才执行成功验收；研究资料列出的其它画幅保持拒绝，直到形成新证据并重新审批。

## 5. 单模型串行验收流程

对表中每个模型执行：

1. 通过客户模型发现或控制台确认该客户模型对测试 Key 可见。
2. 使用 ModelArk v3 客户入口发送请求，不直接调用飞彩，不注入 metadata/extra 私有字段。
3. 记录平台请求 ID、Task ID、HTTP 状态和脱敏请求摘要。
4. 只轮询平台 Task 查询端点，验证 queued/running/终态单调且平台 ID 不变。
5. 成功后通过平台 content endpoint 下载，检查非空、MIME、可播放、Range 和同源代理行为。
6. 制造明确失败，确认平台没有切换渠道或发送第二个 Provider POST。
7. 核对冻结 customer model、Channel、Provider model、`media_arrays_v2` 协议和 billing probe。
8. 核对客户预扣、终态结算/退款、Provider 任务 quota 和账户用量，保持三类金额分离。
9. 执行表中拒绝用例，确认错误发生在 Provider POST 前或符合已登记的确定拒绝合同。
10. 将结果标记为通过、失败或阻塞，并附脱敏证据摘要。

## 6. Task 身份、状态和 unknown 专项

- 创建响应顶层 `id` 是唯一上游任务身份；不使用创建或查询响应中的 `task_id` fallback。
- 主轮询确认小写 `queued/processing/completed/failed` 的完整状态链。
- 任务列表确认大写 `QUEUED/IN_PROGRESS/SUCCESS/FAILURE`，并验证它与冻结 id 的真实关联。
- 人为制造发送后断连、无效 JSON、缺少 id 或未登记非 2xx，确认进入 unknown、不重发、不换渠道、不退款。
- 只有证明任务列表存在稳定唯一关联时，才验收自动 CAS 恢复；否则保持人工/有界对账。
- v2 的任何 terminal rejection 必须独立验证精确 HTTP status 与 error.code；不能沿用 v1 证据。

## 7. 素材与内容专项

- 图片 HTTPS 与允许的图片 Data URL 分别验证；音频、视频只使用 HTTPS。
- 验证 HTTP、localhost、内网、本地路径、相对路径和过期 URL 均失败关闭。
- 验证直接媒体、平台素材和所选 Provider 支持的组合。
- 验证 Asset 所有权、App、客户模型、唯一 Channel、状态、TTL 和 `real_person` 拒绝。
- Pro PI 验证 1/3 个参考视频的顺序和抓取时间；其它九个模型验证任意视频输入拒绝。
- 内容代理验证冻结 Key、同源 HTTPS、拒绝重定向、结果 URL 过期和 Range。
- 普通响应、日志、CSV、指标和验收截图不得出现 Key、Provider 模型、上游任务 ID或完整媒体/结果 URL。

## 8. 计费与 Provider 对账专项

### 8.1 客户费用

- 九个按秒模型核对批准单价、duration、size multiplier 和 group/model factor；
- Pro PI 核对固定按次，15 秒不作为费用乘数；
- 预扣和终态使用同一 billing probe；
- 所有消费、差额、退款和净费用均非负且只发生一次。

### 8.2 Provider 证据

- `/v1/tasks[].quota` 按经批准的 ÷500000 CNY 口径记录，未验证前保持未知；
- `total_usage` 以分计，÷100 后才作为 CNY 趋势；
- `soft_limit_usd` 名称不能覆盖资料声明的实际 CNY 语义；
- Provider 实际金额不改写 NEWAPI 历史客户结算；
- 客户退款但 Provider 可能计费时，核对唯一 `ProviderCostExposure`。

## 9. 通过标准

单个模型只有同时满足以下条件才可发布：

- 精确 customer model、唯一 Channel、Provider model 和 `media_arrays_v2` 协议一致；
- 上线清单中的 duration、resolution、size、媒体 min/max/roles 与黑盒一致；
- 成功、可信失败、unknown、单次创建、轮询和内容下载均通过；
- 客户账单、Provider 证据和 exposure 可分别核对；
- 无敏感信息泄漏；
- 该模型的 Ability 可独立停流且回滚边界明确。

某一模型通过只授权该模型及已验证组合，不授权同族、同分辨率或同报价模型。

## 10. 停流与回滚

出现以下任一情况立即禁用对应客户模型的唯一飞彩 Channel：

- 客户模型/Provider 模型错配、错误 size 或媒体静默丢弃；
- 负费用、重复扣费/退款、预扣与结算 probe 不一致；
- unknown 被自动重发、换渠道或退款；
- Task ID 误用、错误状态被判成功、内容跨域回源；
- v1 路径或 adapter 被运行时重新启用；
- Key、Provider 模型、上游任务 ID或完整 URL 泄漏。

回滚只停止新流量。已创建的 Task、attempt、hold、exposure 和结算继续按冻结 Channel、Provider 模型和
协议闭合；不得跨 v2/v3 解释在途事实，也不得通过恢复 v1 渠道处理当前版本任务。

## 11. 证据归档

每次验收保留：矩阵、配置/hash 摘要、脱敏请求响应、轮询时序、媒体校验、客户账单、Provider 对账、
unknown/单次创建结果、停流演练和最终审批。证据中不得保存秘密或未经脱敏的 Provider 私有数据。
