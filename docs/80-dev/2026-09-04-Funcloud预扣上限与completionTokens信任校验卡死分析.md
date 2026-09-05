---
status: accepted
owner: Dev Team
last-reviewed: 2026-09-05
---

# Funcloud 预扣上限与 completionTokens 信任校验卡死任务分析

## 问题与目标

渠道 #72（FUNCLOUD-SD2-海外-素材共享，`https://mm-internal-cn.leonecloud.com`）上任务
`task_LwIzuFcoHjdb4rW6KOKZ5QW1KAqzjtxx`（上游真实 ID `task_20260904173433_pnor2id1`）于 2026-09-04
17:34:33 提交后长期停留本地状态 `RECONCILIATION_REQUIRED`，客户端持续轮询只看到 `running`，预扣
1,741,740 quota 一直挂起；而 Funcloud 上游该任务实际已成功产出视频。本次分析要回答：卡死的精确根因、
影响面、最快的止血配置，以及关联的渠道健康检查 403、素材代理 502 和 Provider 业务错误码为什么不能
作为视频失败证据，并给出对应修复方案。

## 当前实际情况

### 已验证事实

以下事实均通过渠道 #72 真实 key 查询 Funcloud 查询接口与本地数据库复核。

1. **上游任务真实成功**：`GET /api/v2/open/aigc/task_20260904173433_pnor2id1` 返回
   `status=success`、`output.status=succeeded`、模型 `doubao-seedance-2-0-260128`、1080p、15s、
   16:9、generate_audio=true、单一合法 HTTPS 视频 result URL、无 error 字段。
2. **卡死触发点是 completionTokens 超过冻结上限**：上游上报 `completionTokens=731025`；任务创建时
   冻结的 `AsyncBilling.EstimatedTokens=520000`。Funcloud adapter 在 succeeded 终态要求
   `completionTokens <= MaxTokens`（`relay/channel/task/seedance/thirdparty/funcloud/task_response.go`
   的 succeeded 分支），超限即返回 `UpstreamContractViolation`，`service/task_video_polling_link.go`
   的 `linkVideoContractViolationHandled` 随即把任务标记为 `RECONCILIATION_REQUIRED`。
3. **520000 来自管理端预扣配置**：选项 `task_billing_setting.preconsume_tokens["seedance-2-f"]=520000`，
   经 [task_tiered_price.go](../../relay/helper/task_tiered_price.go) 冻结进 `EstimatedTokens`。
   预扣数学完全吻合：`520000 × 7.7 / 1e6 × 500000 × 0.87 = 1,741,740`，正是该任务扣费金额，同时反推
   probe 为 `1080p / 无视频输入 / group_ratio 0.87`。
4. **RECONCILIATION_REQUIRED 非终态是设计行为**：该状态表示"当前观测不可采信，任务保持活动，计费
   pending"（`docs/20-architecture/账单计费-异步任务与计费事实架构.md`）。后台轮询与客户端每次 GET
   触发的 `RefreshVideoTask` 都会重复校验失败，任务无限保持活动；唯一自动出口是 24h 超时 sweep →
   `EXPIRED` + 全额退款（`TASK_TIMEOUT_MINUTES` 默认 1440）。
5. **实测量 token 速率**（本地库渠道 #72 成功任务实测，token 量与模型无关，与分辨率面积 × 时长成正比）：
   - 480p：40,594 / 4s ≈ 10,149 tokens/s
   - 720p：87,726 / 4s ≈ 21,931 tokens/s；108,900 / 5s ≈ 21,780 tokens/s
   - 1080p：731,025 / 15s ≈ 48,735 tokens/s（本次卡死任务实测）
   720p 与 1080p 速率比 ≈ 2.22，与分辨率面积比 2.25 一致，证明速率按分辨率面积缩放且跨模型一致
   （720p 样本来自 fast 模型，1080p 样本来自标准模型）。
6. **上游"任务不存在"是 HTTP 200 + `code=30003`**： adapter 会将其归为合同违规 → reconciliation，
   而不是确定性的 not-found → FAILURE + 退款。这是独立的次级缺陷。
7. **诊断盲点**：合同违规路径不落日志、不落具体 reason（`FailReason` 固定为
   `upstream_contract_violation`）、不持久化上游响应体，本次只能靠手工查上游定位。
8. **渠道健康检查 403 是假故障**：通用渠道测试把 `ChannelTypeSeedanceLink` 的 `seedance-2-f` 映射成
   `seedance-2` 后，错误地请求 `/v1/chat/completions`。Funcloud 返回“API Key 尚未开通 LLM 能力”只
   说明该 Key 没有 LLM Chat 权限，不能推出视频或素材权限损坏。代码证据是
   `controller/channel-test.go` 的不支持列表遗漏 Seedance Link，且默认路径是 Chat Completions；自动健康
   检查又直接复用了该通用测试。（“映射成 seedance-2”为推断：testModel 取渠道模型列表第一项，
   实际映射发生在 relay 层，403 文案本身不含模型信息。）
9. **渠道 #72 当前素材列表连通**：2026-09-04 在管理端执行只读 `GET /api/channel/test_asset/72` 返回
   HTTP 200，耗时约 557ms；同日复核用渠道 key 直接调用与 `CheckConnectivity` 相同的
   `GET /api/v2/open/material/list?page=1&pageSize=1`，返回 `code=0` 且列表含事故当日 17:34:30 上传的
   素材，当前连通性得到独立复核。这验证了当前 Base URL、素材凭据和列表接口连通性，但不等价于虚拟素材
   上传接口始终可用。
10. **18:03 附近的素材 502 不能证明视频不存在**：`POST /v1/assets` 是独立素材创建操作，不是视频任务
    查询。网关先读取调用方 source，再向素材 Provider 上传；素材 adapter 的 HTTP 4xx/5xx、业务拒绝、
    transport error 和响应解析错误当前均可被统一投影为北向
    `502 asset_upstream_error`。请求已经写出但等待响应失败，同样会得到 502。
11. **约 30 秒失败高概率是传输/响应边界**：相关请求耗时为 28.2、29.7、29.8、30.0 秒，与出站 HTTP 客户端
    整体超时 `Timeout: 30 * time.Second`（`service/http_client.go`）精确吻合。服务日志只有
    无 `status/provider_code` 的通用错误。这更符合连接、上传或等待响应阶段的超时/中断，而不是明确的
    Provider HTTP 拒绝；当前日志丢弃了 transport error 分类，无法再精确定位到某一层。另可收窄一层：
    调用方 source 拉取失败投影为北向 400 `invalid_request` 而非 502（`asset_funcloud_stream.go` 的
    source fetch 错误包装），因此观测到的 502 必然发生在向素材 Provider 上传或等待其响应的阶段，
    即"请求可能已写出、结果不明"。
12. **素材请求不能全部归因于渠道 #72**：三个 `POST /v1/assets` 的访问日志没有请求体中的客户模型，
    无法仅凭 URL 确认渠道；`GET /v1/assets/<opaque-id>?model=seedance-2-0-m` 明确属于 `-m` 模型对应的
    其他 Seedance Channel，不属于只承载 `-f` 模型的渠道 #72。

### 错误码与证据边界

| 观察到的状态/错误 | 归属 | 正确解释 | 禁止推导 |
| --- | --- | --- | --- |
| `RECONCILIATION_REQUIRED` | 本地 Task 状态 | 当前观测未通过信任校验，Task 和计费保持活动 | 上游返回了该状态、任务失败、没有视频 |
| `UpstreamContractViolation` | 冻结 adapter | Provider 响应与本地可信合同不一致 | Provider 业务任务一定失败 |
| 健康检查 `403 permission_error` | Funcloud LLM Chat | 错误的 Chat 探针已到达上游，Key 无 LLM 权限 | 视频/素材权限损坏或事故因果 |
| 北向 `502 asset_upstream_error` | 本地素材错误归一 | 当次素材操作没有取得可交付结果 | 请求未发送、视频不存在、重试一定安全 |
| 素材诊断 `status=400/401/403/5xx` | Provider HTTP 响应 | Provider 明确返回对应状态；北向仍可能统一为 502 | 北向状态等于 Provider 状态 |
| 素材诊断无 `status/provider_code` | 非结构化 adapter/transport error | 未取得可安全分类的上游诊断 | 精确超时层、精确 Provider 业务原因 |
| Funcloud HTTP 200 + `code=30003` | Provider 业务码 | 经协议验证的任务不存在 | 继续作为一般合同违规无限 reconciliation |

### 影响面（按当前配置推算）

`completionTokens` 信任校验只存在于 Funcloud 协议 adapter，受影响的是可路由到 Funcloud 渠道的
`-f` 系列客户模型。按实测速率 × 模型规格上限推算，以下配置在满规格请求下必然超限卡死：

| 客户模型 | 当前上限 | 规格上限场景 | 满规格实际 token | 结论 |
| --- | --- | --- | --- | --- |
| seedance-2-f | 520,000 | 1080p × 15s | ≈731,025（实测） | 超限，本次事故 |
| seedance-2-fast-f | 324,000 | 720p × 15s | ≈330,000 | 贴线必超 |
| seedance-2-mini-f | 324,000 | 720p × 15s | ≈330,000（按同速率推算） | 贴线必超 |
| seedance-2-5-f | 648,000 | 720p × 30s | ≈660,000 | 超限 |

`-m`（moxing 协议）模型不走该校验，不在本缺陷影响面内。

### 假设与未确认项

- seedance-2.5 与 mini 的单秒 token 速率未实测，按 720p ≈ 21,931 tokens/s 推算；上线建议值后应抽样
  复核。
- generate_audio / ratio / draft 对 token 速率的影响未单变量验证；当前建议值已含 25%–30% 余量。

## 优化方案

### 短期止血（仅配置，即时生效）

**结论：是的，最快的止血方案就是把 `task_billing_setting.preconsume_tokens` 上调到覆盖满规格真实
用量。** 它只影响新任务的创建时冻结值，无需改代码、无需发版。两个边界必须知道：

1. **救不了已卡死的任务**：`EstimatedTokens` 创建时冻结在 `private_data`，改配置不会解冻存量任务。
2. **生效方式**：通过管理台选项入口更新（走配置同步，即时生效）；直接改数据库需要重启进程。

建议保守值 = 实测速率上限 × 模型最大时长 × 1.25（未实测模型 ×1.3，取整到万位）：

| 客户模型 | 当前值 | 建议值 | 计算依据 | 预扣影响（最高价分支，估算） |
| --- | --- | --- | --- | --- |
| seedance-2-f | 520,000 | **920,000** | 49,000 × 15s × 1.25 | ≈1.74M → ≈3.08M |
| seedance-2-fast-f | 324,000 | **420,000** | 22,000 × 15s × 1.25 | ≈0.79M → ≈1.02M |
| seedance-2-mini-f | 324,000 | **420,000** | 22,000 × 15s × 1.25（速率未实测） | ≈0.49M → ≈0.64M |
| seedance-2-5-f | 648,000 | **860,000** | 22,000 × 30s × 1.3（速率未实测） | ≈2.89M → ≈3.84M |

预扣上调只是提高创建时暂挂资金。代码修复后，结算按真实用量双向调整：实际费用低于预扣则退差额，
实际费用高于预扣则从创建时冻结的同一资金来源原子补扣；只有补扣时钱包、订阅或 Token 额度不足，
计费才进入可重试 `debt`，但 Provider 已成功的任务仍保持 `SUCCESS`。创建时余额连预扣额都不足时，
请求会在发送上游前被拒绝，这是正常的授信边界，不是生成后失败。

### 为什么此前从界面看像“预扣不足导致生成失败”

预扣不足并没有使 Funcloud 生成失败；本次上游已经成功并返回视频。旧实现存在两层错误耦合：

1. 创建时的预扣 token 估算值同时被当成 Provider `completionTokens` 的可信上限。实际上报高于估算时，
   adapter 拒绝成功证据，任务被置为 `RECONCILIATION_REQUIRED`，界面因而长期只显示运行中。
2. 即使成功证据通过，阶梯表达式结算发现“实际费用 > 已预扣”时，旧代码也不尝试已有的原子差额扣款，
   而是直接写成 `debt`。这会把“估算偏低”错误地等同于“客户资金不足”。

修复后两者完全分离：协议合理性上限只校验 Provider 证据是否可能；预扣只负责创建时的资金暂挂；
真实费用高于预扣时先补扣，只有资金来源确实不足才记欠费。任务业务状态与计费状态独立，欠费不会把
已经生成成功的视频改成失败，也不会触发退款或重新调用上游。

### 本卡死任务处置

- 默认：不处理，2026-09-05 17:34 左右 sweep 自动 `EXPIRED` + 全额退款，视频对客户作废。
- 推荐（需按运维对账纪律）：管理员修正该任务冻结的 `private_data.async_billing.estimated_tokens`
  （520000 → ≥731025），下一次轮询即自然恢复 SUCCESS，表达式按实际 731,025 tokens 结算并补扣差额
  （≈2,448,644 quota），客户拿到视频、按真实用量付费。操作前先快照 private_data 证据，事务内修改，
  留审计说明。

### 关联 403/502 的修复

#### Seedance 渠道健康检查

1. 把 `ChannelTypeSeedanceLink` 从通用 Chat 健康检查中隔离，禁止再默认请求
   `/v1/chat/completions`。
2. 健康检查必须由代码登记的 `video_upstream_protocol` / `asset_upstream_protocol` 决定；仅执行只读、
   无计费副作用的协议探针。Funcloud 素材协议可继续使用分页大小为 1 的列表查询。
3. Funcloud 视频协议目前没有无副作用的视频探针时，应明确显示“不支持视频自动测试”，不得为了得到
   成功状态而创建收费视频，也不得退回 Chat 测试。
4. 素材探针的成功、鉴权拒绝或连通性失败都不得驱动视频 Channel 的自动启用/禁用，也不得写入通用
   `response_time/test_time`。CMCC 等协议的视频与素材凭据可分离，素材控制面不具备证明视频履约健康的充分性。

#### 素材代理 502

1. 在素材 adapter 边界保留脱敏错误分类和阶段：`upload_body`、`wait_response`、
   `decode_response`，以及 `timeout`、`connect`、`reset`、`upstream_http`、`application_error`、
   `invalid_response` 和其他 `transport`。`source_fetch` 发生在 Provider adapter 之前，是调用方
   source 拉取边界，继续投影为北向 400，不混入 Provider 502 诊断分类。
2. 日志保存请求关联 ID、Channel ID、客户模型、冻结素材协议、耗时和安全的 HTTP/provider code；不得
   保存凭据、source URL、完整签名 URL 或上游原始响应。
3. 北向暂时保持既有 `502 asset_upstream_error`，避免未经评审改变已发布合同；管理日志和指标必须能区分
   Provider 明确拒绝与“请求可能已发送、结果不明”。如未来要把 transport timeout 改为 504，必须单独
   评审客户兼容性和重试语义。
4. 对发送后结果不明的素材 POST 不自动重试、不换渠道、不回退默认素材组。素材代理继续遵守无状态边界，
   不建立视频级 create-attempt 或第二套 Task；可能的 Provider 孤儿素材由技术人员根据请求关联信息人工
   排查。
5. 增加按 `channel_id + protocol + operation + error_class` 聚合的指标，分别监控 Funcloud 列表检查、
   虚拟素材上传和查询，防止“列表连通”被误解为“上传链路完整健康”。

### 长期修复（代码，已实施）

1. **解耦两个语义**：`MaxTokens` 信任上限不应复用预扣预算。改为按协议合理性推导（模型最大时长 ×
   分辨率 token 律的硬上限），或新增独立的 Provider 证据上限配置。预扣预算只决定创建时暂挂金额，
   不充当成功证据上限或最终结算上限；实际费用高于预扣时必须先原子补扣。
2. **补诊断盲点**：合同违规路径增加带脱敏 reason 的 WARN 日志，并把 reason 追加进 `FailReason`，
   使 reconciliation 可自解释。
3. **修正 not-found 语义**：查询返回 `code=30003`（任务不存在）时应映射为确定性 not-found →
   FAILURE + 退款，而非 reconciliation。**前置条件**：官方协议的 HTTP 404 在轮询中一次即杀
   （`pollClassNotFound` → `failTasksFromPoll`），而 Funcloud `30003` 的创建后瞬时一致性语义未经验证；
   若创建后短时间窗口内查询可能返回 30003，一次即杀会误杀新任务并错误退款。因此实现必须带边界：
   要么先取得 Funcloud 对 30003 任意任务年龄下确定性的确认，要么复用既有连续失败上限
   （`TaskPollMaxFailures`，默认 20 次）作为宽限窗口后判 FAILURE + 退款。
4. **修正健康检查边界**：Seedance Link 使用协议感知探针或显式不支持，不再进入 NEWAPI 原生 Chat
   测试与由此触发的自动禁用判断。
5. **补齐素材错误可观测性**：保留安全的阶段、错误类别和请求关联信息；不得用北向 502 反推 Provider
   原始状态，也不得记录敏感上游响应。
6. **补齐预扣差额结算**：移除“实际费用超过预扣即直接欠费”的人工上限，统一调用持久化任务的原子
   结算入口。补扣成功时更新任务额度并记差额消费；钱包、订阅或 Token 额度确实不足时才进入可重试
   `debt`，保留冻结目标额度；数据库或状态异常仍进入 `failed`。无论哪类计费异常，Provider 成功任务
   都保持 `SUCCESS`，补偿任务只重试资金操作，不重发 Provider 请求。

### 验证方式

- 配置上调后，提交一个 1080p/15s 的 `seedance-2-f` 测试任务，确认走完 SUCCESS + 表达式结算 + 差额
  退款全链路；再使用低于真实费用的预扣值验证资金充足时原子补扣成功。
- 分别制造钱包、订阅和 Token 额度不足，确认原子事务不产生部分扣款，任务保持 `SUCCESS`、计费进入
  `debt` 且保存目标额度；补足额度后执行 reconciliation，确认只补扣一次并转为 `settled`。
- 抽样复核 seedance-2.5 / mini 的实际上报 completionTokens，验证推算速率。
- 代码回归：人工构造超上限 completionTokens 响应，确认只产生 WARN 与可解释 FailReason，
  不再无限 reconciliation。
- 对渠道 #72 运行自动健康检查，确认不会产生 `/v1/chat/completions` 请求；素材只读检查仍可成功，且不会
  创建收费视频或素材。
- 分别模拟素材 Provider 显式 400/401/403/500、连接失败、等待响应超时和无效响应，确认北向合同稳定、
  管理日志可区分错误类别且不包含凭据、source URL 或原始响应。
- 模拟素材 POST 在请求体已写出后超时，确认系统不自动重试、不换渠道、不创建本地素材事实，并产生可供
  人工核查 Provider 孤儿资源的脱敏关联日志。

## 实施记录（2026-09-05）

### 已完成（代码，本仓库工作树）

1. **信任上限解耦**（`relay/channel/task/seedance/funcloud_billing_context.go`）：`MaxTokens` 改为按
   冻结探针的 `resolution + duration_seconds` 推导的合理性上限（实测速率 × ≥2.4 倍余量：
   480p 30k/s、720p 60k/s、1080p 120k/s，时长缺省按 30s，下限 100k）。事故形状回归测试覆盖：
   1080p/15s 上限 1,920,000 > 实测 731,025，真实成功任务不再被拒；预扣预算仅决定预扣金额。
2. **合同违规可观测**（`service/task_video_polling_link.go`）：reconciliation 落库时输出 WARN 并把
   具体违规原因写入 `FailReason`（`upstream_contract_violation: <reason>`，截断至 200 字符）。
3. **not-found 语义**（`funcloud/task_response.go` + `relay/common/upstream_task_not_found.go` +
   轮询接线）：Funcloud `code=30003` 映射为 `UpstreamTaskNotFound`；轮询按“计入连续失败、达
   `TaskPollMaxFailures`（默认 20）后 FAILURE + 退款”处理，容忍 Provider 创建后可见性窗口，
   不再无限 reconciliation。识别点在 HTTP 200 响应的 `ParseTaskResult` 阶段，而非
   `FetchTask` 传输阶段；回归测试使用真实的“HTTP 200 响应→解析产生 30003”路径，覆盖
   “宽限期内保持活动 + 达限转 FAILURE”。
4. **健康检查边界**（`controller/channel-test.go` 窄分支接线 + 新文件
   `controller/channel_seedance_link_health.go`）：`ChannelTypeSeedanceLink` 加入不支持列表（手动与
   自动测试均不再发 `/v1/chat/completions`）；自动健康检查对 Link 渠道改为执行素材协议只读探针，
   成功只证明素材控制面可用，HTTP/业务码/transport 失败也只记录素材探针失败；两者都不自动启用或禁用
   视频渠道，不写入通用响应时间。`localErr` 正确计为失败；被禁用的 Seedance 不进入无法完成视频恢复证明的
   passive-recovery。无素材协议渠道（如 #75）落“探针不可用”分支。
5. **素材错误可观测与上传收敛**（`assets/diagnostic.go`、`assets/adapter.go`、`assets/funcloud.go`、
   `service/asset_service.go`）：transport 错误包装为带稳定阶段/类别的结构化错误，HTTP 拒绝、
   Provider 业务码和无效响应也使用同一诊断字段集。Funcloud `Do` 失败后不再无界等待
   可能阻塞在 `Source.Read` 的上传生产协程；请求体已完整写出后的失败归为
   `stage=wait_response`，未能确认上传完成的失败归为 `stage=upload_body`。素材失败 WARN
   日志含请求关联 ID、操作、客户模型、Channel ID、冻结素材协议、耗时与脱敏诊断。
   北向 502 `asset_upstream_error` 合同不变。
6. **预扣差额原子补扣**（`model/task_billing_atomic.go`、`service/task_billing_state.go`）：阶梯表达式
   实际费用超过预扣时不再直接进入欠费，而是复用持久化 Task 的原子结算入口，对创建时冻结的钱包、
   订阅及 Token 资金事实补扣差额。三个资金来源的额度不足统一包装为可判别的
   `ErrTaskBillingInsufficientFunding`，service 仅对此类错误写入 `debt`；其它结算错误写入 `failed`。
   原子事务保证钱包已经扣减但 Token 不足、或订阅额度不足时全部回滚。欠费保存 `TargetQuota` 并按既有
   reconciliation 重试；补足额度后转为 `settled`，重复扫描不会再次扣款。业务 Task 在整个过程中保持
   `SUCCESS`，不会因补扣失败改写成生成失败。
7. **失败终态原子退款**（`service/task_billing_state.go`、`service/task_polling.go`）：
   `FAILURE` / 取消 / 过期 / Provider 合同失败不再进入 tiered/token 结算并误保留预扣；
   轮询连续失败达限与失败任务补偿扫描统一调用 `refundTaskWithReconcile`，退款失败可幂等重试，不重发 Provider 请求。

验证：`go build ./...` 通过；`service`、`controller`、`relay/...`、`model` 全部测试通过；
`task docs:check`、`task ai:check` 通过。`relaykit/` 无改动。新增差额结算回归覆盖资金充足时补扣、
资金不足时 `SUCCESS + debt`、补足后幂等结清，以及钱包/订阅/Token 不足的事务回滚与错误分类。
新增回归覆盖：探针不可用计失败、素材探针不自动启用/禁用视频渠道、不改写通用响应时间，
以及上传源阻塞时的非阻塞返回、写体后等待响应分类和稳定诊断 taxonomy。

### 已完成（配置，本地数据库）

`task_billing_setting.preconsume_tokens` 已上调：`seedance-2-f` 520000→920000、
`seedance-2-fast-f` 324000→420000、`seedance-2-mini-f` 324000→420000、
`seedance-2-5-f` 648000→860000。生产实例需在管理台选项入口同步修改（走配置同步即时生效）。

### 剩余事项

- 事故任务 `task_LwIzu...` 的冻结值修正属生产库运维操作，需按上文"本卡死任务处置"执行（或等待
  24h sweep 自动 `EXPIRED` + 退款）。
- 素材失败日志已含 `channel_id + protocol + operation + error_class + elapsed_ms`；仓库尚无独立
  指标框架，当前以这组稳定字段聚合结构化日志，引入指标框架时直接沿用该维度。
- 30003 的创建后可见性窗口时长未经 Provider 确认；如确认其为任意任务年龄下的确定性信号，可将
  宽限窗口缩短为立即 FAILURE。
