---
status: current
owner: Dev Team
last-reviewed: 2026-09-14
---

# Azure Batch 渠道与批处理作业架构

## 1. 范围与状态

本文描述 Azure Batch 类型化本地扩展的渠道身份、北向接口、文件边界、计费与客户交付。作业生命周期、
三类耐久事实（上游执行/文件交付/资金结算）与恢复边界由
[异步任务与计费事实架构](账单计费-异步任务与计费事实架构.md) §7a 负责，本文不复制。

当前状态：代码已实现并通过本地回归；真实 Azure 验收、Batch 表的三数据库实机迁移、
10,000 行/大结果容量演练与生产灰度均未完成，不得写成生产已发布。

## 2. 渠道身份与路由

- 专用渠道类型 `ChannelTypeAzureBatch = 66`，按 Link 类本地类型化扩展治理；不继承 Seedance 的
  模型唯一性、素材或视频合同。渠道不写入原生 Ability 分发池，普通同步请求不会误选 Batch 渠道。
- 每个方法与路径只保留一个权威 handler；`/v1/files`、`/v1/batches` 是与未来上游实现重合的
  合并冲突面，接取上游时按[上游代码合并指南](../30-engineering/上游代码合并指南.md)形成一次明确
  接线或迁移，不并列注册两套公共状态。`DELETE /v1/files/:id` 保持明确未实现。
- 上传文件时不确定执行渠道；创建作业时按文件内客户模型、调用 Key 授权与适用合同规则选择有资格的
  Batch 渠道并冻结映射。创建后查询、取消和结果获取均使用冻结渠道，不重新选渠。

## 3. 北向接口（首期发布方法）

客户使用本站地址与本站 Key，沿用官方“上传 JSONL → 创建作业 → 查询 → 下载结果”流程：

| 方法 | 行为 |
| --- | --- |
| `POST /v1/files` | 接收用途为 Batch 的 JSONL，存本站私有 OSS，返回本站文件 ID |
| `GET /v1/files`、`GET /v1/files/{id}`、`GET /v1/files/{id}/content` | 按调用者归属列出、读取元数据与内容 |
| `POST /v1/batches` | 接收本站文件 ID、Chat Completions 端点与必填 `completion_window: "24h"` |
| `GET /v1/batches`、`GET /v1/batches/{id}` | 按调用者归属列出与读取作业投影 |
| `POST /v1/batches/{id}/cancel` | 向冻结渠道提交取消并继续跟踪最终结果与费用 |

- `completion_window` 仅接受字符串 `24h`；缺失、类型错误或其他值在文件上传、资金 hold 与作业创建
  前本地 400，不静默补默认值。24 小时是上游处理目标，不是平台截止时间或超时退款依据。
- 容量边界：每批最多 10,000 条请求、输入文件最多 20 MiB（20,971,520 字节），两项同时校验；每条
  请求必须带适用于所选模型的输出上限。超限、坏行、混合模型、重复 `custom_id` 指出行号整批拒绝。
- 输入校验分两段：上传阶段完整校验用于提前反馈；创建阶段从 OSS 全量流式重解析并重新完成容量、
  权限、归属与渠道资格校验，按冻结价格逐行估算。上传摘要不能替代创建时校验。同一对象中的重复
  JSON 字段（含转义同名键）在输入边界递归拒绝，防止读取分叉。
- 文件正文只进私有对象存储；数据库仅保存对象引用、校验摘要与行级用量事实。对象缺失按明确文件
  不可用报告，已结算资金不因 infra 清理回滚。

## 4. 计费

- 售价配置键为 `batch_billing_setting.batch_billing_expr`，管理页有专用配置卡，客户价格页明确
  选择原生、Azure Batch 或某份合同；缺少 Batch 价格不静默使用普通价或零价，也不按模型名推断身份。
- 计价时间冻结：`billingexpr.RequestInput.PricingTime` 为可选显式时刻；Batch 创建时持久化同一
  UTC 时刻并在预扣、结算与恢复间复用；未传入的原生调用保持 `time.Now()` 语义；共享编译缓存不固化
  作业时间。条件只冻结允许的标量请求参数（如 `n`、输出上限）与计价时间，不存储任意 header/正文探针。
- 预扣是资金预算器而非第二套结算引擎：对支持的 token 算术与条件/tier 分支求保守上界，无法安全求
  上界的表达式在保存或创建前拒绝。预扣逐行估算输入并纳入输出上限与 `n` 等数量；缓存未知时采用不
  依赖预期缓存优惠的预算。
- 结算按行执行：单行按自身请求上下文与实际 usage 计模型费用，再应用组/合同倍率，Batch 总费用为
  逐行之和；阶梯按单行判定，不汇总 Token 后套一次。完整终态证据到齐后一次最终结算；取消、部分
  失败按实际费用求目标，明确零费用证据才结为零；已执行但用量缺失的行与未执行行区分，保持待核实。
- 实际费用高于预扣时沿冻结资金来源补扣/欠费机制；客户资金与 Provider 成本分账，供应商金额未知
  保持未知。行级 checked quota 的 clamp 持久化到行事实，并写入消费日志
  `admin_info.quota_saturation` 与关联 Task 告警。
- 非有限求值结果在进入十进制合同换算前拒绝；饱和诊断的 NaN/Inf 以具名字符串持久化（见
  [异步任务与计费事实架构](账单计费-异步任务与计费事实架构.md) §6.2）。

## 5. 推进、结果与客户交付

- 生命周期只由注册的 `batch_progress` SystemTask 处理器推进（独立递增 poll_version、失败退避、
  租约与条件更新防双写）；通用超时/失败退款扫描、终态计费补偿与人工恢复入口按
  `platform <> 'azure_batch'` 排除 Batch。
- Adapter 将 5xx、无可信 ID、无法解析的受理响应保留为 unknown，不释放 hold、不自动重发；仅明确
  拒绝返回 rejected。创建时冻结 base URL、凭据、API 版本与 adapter 版本到私有 Batch 快照。
- 结果归集先流式落临时文件、只持久化一次；同时读取成功与错误文件，成功行必须具备完整 usage。
  交付前逐行生成客户文件：`response.body.model` 使用创建时冻结的客户模型名，行级错误使用统一
  `request_failed` 元数据；回答正文、`custom_id` 与 usage 保持原义；文件大小记录交付实际字节。
  普通用户不获得上游模型身份或私有存储引用。
- `delivery_state=ready` 表示完整行级事实已原子提交；专用推进器可直接按已持久化金额恢复共享资金、
  日志和终态，不依赖外部文件或再次访问 Azure/OSS。
- 创建与查询、文件与作业读写均按 `user_id + app_id` 隔离；合同/渠道/价格更新后已创建作业按冻结
  身份处理，新请求才使用新配置。

## 6. 合同与授权集成

- Batch 资格不依赖原生 Ability；合同 Key 调用 Batch 时遵守[用户模型合同定价架构](账单计费-用户模型合同定价架构.md)：
  规则无法由 Batch 渠道履约时在预扣前明确拒绝，不回退普通池。批量绑定迁移使用 Token cache fence，
  失败零写入。
- 会话价格页由客户显式选择原生、Azure Batch 或某份合同，不自动推断默认合同；绑定 Key 只读自身合同。

## 7. 架构不变量

1. Batch 渠道不进入原生分发池；同模型的普通同步请求不会选中 Batch 渠道。
2. `completion_window="24h"` 必填且仅接受该值；超过 24 小时不是超时，不触发失败或退款。
3. 每方法每路径唯一 handler；不通过注册顺序或双读猜测归属。
4. 取消不等于零费用，completed 不等于全部行成功；结算、交付与财务状态互相不可代替。
5. 逐行明细是核算证据，不生成第二套资金账本；重复轮询/结算不重复扣费或退款。
6. 正文与结果只进私有对象存储，凭据与原始 Provider 响应不进入 Task、日志或公共响应。
7. 代码实现不等于生产可用：真实 Azure、三库 Batch 表、容量与灰度验收前不开放。

## 8. 代码事实映射

| 事实 | 代码位置 |
| --- | --- |
| 渠道类型与北向路由 | `constant/channel.go`、`router/batch_relay_router.go` |
| 创建、推进与恢复 | `service/batch_create.go`、`service/batch_progress.go`、`model/batch_job_task.go` |
| 南向 adapter 与结果解析 | `relay/channel/azurebatch/client.go`、`relay/channel/azurebatch/result.go` |
| 行级事实与结算 | `model/batch_result.go`、`model/batch_quota_data.go`、`service/batch_pricing.go` |
| 客户文件交付 | `service/batch_result_delivery.go` |
| 售价配置 | `batch_billing_setting`、`pkg/billingexpr`（`PricingTime`） |
