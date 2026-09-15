---
status: current
owner: Dev Team
last-reviewed: 2026-09-15
---

# Synlink 失败任务处置手册

## 适用范围与前置条件

适用于冻结协议为 `synlink_video_v1`、Provider 已报告失败而本站仍待核对的 Seedance 视频任务。
执行前由 Root 与技术人员核实单个任务身份、冻结连接、业务证据与当前资金状态；不得按模型名、
金额或相近时间批量认定任务归属。先取得数据库一致性备份，并核对所有运行节点的版本和调度状态。

## 核对失败证据

1. 在任务详情记录本站公开 Task ID、冻结插件版本、当前状态、计费状态和占用 quota。
2. 按[音视频证据运维手册](11-音视频证据运维手册.md)读取该 Task 的查询事件；如需重新查询，使用
   该 Task 冻结连接及凭据，不能以当前渠道配置替代。不在命令参数、日志或工作文档放置密钥与原始响应。
3. 确认查询 HTTP 200、`task.id` 等于冻结上游 ID、`task.status=failed`，且失败形状符合
   [协议架构](../20-architecture/Seedance专用渠道与Link架构.md#54-synlink-视频协议)。根级 `error`
   是查询错误；`task.error` 才是任务错误；`metadata.status`、页面 100% 或重复响应摘要均不能单独判终态。
4. 记录受保护证据索引、核实人及脱敏结论。另一任务的账单不得按时间接近归入本任务；供应商 Request ID
   与本站从创建响应头捕获的 ID 不一致时，先取得 Provider 关联证明。

## 新版发布与验证

1. 部署包含内置 `seedance-link` 1.3.5 的构建。既有启动同步负责入库及晋升，检查实际 active、启用状态
   和各节点加载结果，不能仅用仓库版本判断生产已生效。若 active 已高于 1.3.5，先核对其包含本修复，
   不降级覆盖。
2. 新建验收任务，确认创建快照固定于已验证版本；观察生成中、成功、字符串型失败和对象型失败，不伪造 Provider 失败来验收
   生产资金。测试环境可使用确定性模拟响应。
3. 可信失败应进入 `FAILURE`、进度 100%、完成时间非零；计费完成后 quota 与退款目标为零，计费状态
   为 `settled`（表示退款处理已结清），钱包或订阅、Token 配额和唯一退款日志一致。
4. 未知状态、ID 不匹配、接口错误、失败携带矛盾输出仍应待核对；不得自动重发视频、换渠道或提前退款。

## 冻结旧版本任务的处置边界

发布新版不会替换旧任务冻结的 1.3.3、1.3.4 或更早制品。同版本源码不可覆盖，禁止直接改 Task 插件版本、
增加 active fallback，或者修改共享轮询器去按 Synlink 原始字段另建一套解析。

### 单笔维护工具

仓库提供 `cmd/recover-synlink-failure`，默认只读预览。仅支持主库与日志／审计同库的本地 SQLite，
执行环境必须为全部应用节点和后台任务已停止、部署未使用 Redis。MySQL、PostgreSQL、日志分库及
使用 Redis 的部署不适用此 CLI；不得把在线数据库复制后执行退款再覆盖回生产。
维护进程不加载当前渠道配置、不启动迁移／调度、不访问 Provider。Root 操作人 ID 用于审计归属，
不能替代实际维护环境访问授权；执行人必须已经获得该单笔处置授权。

执行前在受保护证据系统逐项确认上述失败证据，将索引编号作为 `-evidence-ref`。该值仅接受字母、
数字、下划线和连字符，不能填原始响应、凭据、URL 或错误全文。以下参数均为占位符：

```bash
go run ./cmd/recover-synlink-failure \
  -database <本地SQLite文件> -task-id <公开TaskID> \
  -user-id <客户ID> -app-id <应用ID> -channel-id <冻结渠道ID> \
  -operator-id <Root用户ID> -expected-quota <已核对占用quota> \
  -evidence-ref <受保护证据索引>
```

`app-id` 必须显式传入，历史无应用任务可为 `0`。工具检查任务仍是冻结 `seedance-link` 1.3.3、
API 3、`synlink_video_v1`，处于此身份违例导致的待核对，资金 pending 且占用额度匹配，未接收用量或结果。
不符合条件则拒绝，不能修改冻结快照或放宽条件强行执行。旧 Go profile 不属于此工具范围。

预览与人工证据一致后，在维护窗口对同一命令追加：

```bash
-apply -verified-failed -offline-no-redis -backup <新的备份文件路径>
```

工具先验证、创建权限为 `0600` 的一致性 SQLite 备份，再在事务中重读单笔任务，原子写入 `FAILURE`、
100% 进度、完成时间、通用脱敏失败原因、零退款目标及审计事件 `task.synlink_verified_failure`。
审计保留操作人、客户／应用／渠道、公开 Task ID、核实额度与受保护证据索引；不写入上游 ID 或响应。
冻结插件、连接与价格保持原值。状态或额度已变化、审计写入失败时，整个终态决定回滚。

随后复用 `model.ApplyTaskBillingTarget` 按行锁中的实际占用额度退款，处理钱包／订阅与 Token，
通过既有 `DeliverTaskBillingLog` 交付日志。`expected-quota` 仅是核对条件，不是手工加款金额。
终态和退款使用独立事务：退款失败会保留已审计失败和零目标，既有失败退款清扫可继续处理。
中断后可以使用**相同任务清单、操作人、额度和证据索引**以及新的备份路径重跑；即使上次已退款，
也只补交付未完成日志，不再次退款。工具不覆盖任何既有备份，不重发生成，不改变供应商成本事实。

退出失败时区分输出中的“终态拒绝／回滚”“退款待处理”“退款完成但日志待交付”，核实并排除对应
问题后续跑。成功输出必须包含 `billing=settled`、`held_quota=0`、`refund_logs_delivered=true`；
再核对任务详情、账户资金、Token、唯一退款日志及审计记录，完成后重启应用节点。
错误诊断使用 `stage=<阶段> code=<类别>`，只输出固定说明，不回显参数值、数据库原始异常、SQL 或路径。

| 阶段／类别 | 处理方式 |
| --- | --- |
| `arguments` / `invalid_input` | 使用 `-help` 核对参数格式，不把原始证据或凭据放入参数 |
| `preflight` / `operator_invalid`、`task_scope_mismatch` | 核对启用 Root 与任务所属客户／应用／渠道 |
| `preflight` / `frozen_contract_mismatch`、`frozen_funding_invalid` | 核实冻结版本、协议与资金来源，不改快照绕过限制 |
| `quota_mismatch`、`task_state_conflict`、`task_changed`、`evidence_conflict` | 重新预览并核实状态、额度及既有审计，禁止强制执行 |
| `environment_mismatch`、`database_unavailable`、`database_error`、`backup_unavailable` | 检查离线环境、同库要求、文件权限、数据库结构／约束及备份条件；数据库错误不代表操作人或任务条件错误 |
| `refund` / `funding_conflict`、`frozen_record_missing` | 核对冻结资金记录及当前余额，不手工叠加退款 |
| `operation_failed` | 未分类异常；按阶段调查维护环境和受保护证据，不猜测业务失败原因 |

`terminal_decision` 表示终态事务阶段，`refund` 表示资金阶段，`log_delivery` 表示日志查询／交付阶段；
同一个 `database_error` 必须结合阶段和“退款待处理／日志待交付”说明处置。排除故障后沿用相同核对事实、
使用新备份续跑，不能因为诊断类别改变而跳过资金与审计核对。

本工具只接受 1.3.3；若线上已有冻结 1.3.4 的对象型失败任务，需另行核实处置，不能改版本绕过工具校验。
当前管理接口仍不提供已创建 Task 的通用人工失败按钮，不能调用 attempt reject 替代本流程。
本地测试不代表存量线上任务已处置。

## 超时、对账与失败处置

合同违例跳过普通连续轮询失败计数，但全局 `TASK_TIMEOUT_MINUTES` 默认 1440，配置不大于零时关闭
超时清扫。符合条件的待核对 Task 超时后进入 `EXPIRED`，客户退款与 Provider 成本风险分别处理。
不能将超时处置当作已经确认 Provider 失败，也不能依靠等待超时代替协议修复。

客户退款与供应商争议分开核实。Provider 内部错误码只说明其报告的错误，不直接证明故障发生于哪一层，
也不证明某笔供应商扣费属于此任务。未知 Provider 成本保持未知。

## 验收边界

查询 HTTP 404 表示本次结果未确认，不等同于失败，也不按次数自动归因为应退款。
视频下载只提供临时访问；控制台提示及时下载，不按任务完成时间承诺固定 24 小时。
下载返回 `410 video_content_expired` 时核对明确的内容到期时间或签名时间加有效时长；
缺少签名日期、参数畸形、HTTP 403/404 或 `transport_error` 均不能单独证明签名过期。
下载故障不改变生成成功与结算事实；在有效期内无法交付时单独调查交付故障，不自动刷新、
重新生成或退款。产品不提供长期视频存储，旧任务不自动修复。

本地代码与模拟测试通过不等于生产部署、真实 Provider 验收或历史退款完成。发布后逐项记录版本、
任务状态、资金与日志结果，未完成事项由[路线图](../50-planning/路线图.md)跟踪。
