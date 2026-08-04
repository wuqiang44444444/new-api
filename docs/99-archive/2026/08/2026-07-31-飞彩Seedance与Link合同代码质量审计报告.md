---
status: historical
owner: Dev Team
last-reviewed: 2026-08-04
archived-at: 2026-08-04
source-path: docs/80-dev/2026-07-31-飞彩Seedance与Link合同代码质量审计报告.md
superseded-by: "docs/20-architecture/Link合同架构.md; docs/20-architecture/视频上游接入与异步任务架构.md; docs/50-planning/路线图.md"
---

# 飞彩 Seedance 与 Link 合同代码质量审计报告

审计对象：

- [飞彩 Seedance 视频接口适配方案](2026-07-30-飞彩Seedance视频接口适配方案.md)
- [Link 合同架构修复实施方案](2026-07-30-Link合同架构修复实施方案.md)

审计标准：`AGENTS.md` 的最小入侵硬约束、两份方案的完成标准、ADR-0009/0010/0011、
`audit-code-quality` 的证据优先规则，以及当前架构、用户和运维文档。

## 1. 审计范围与验证

### 1.1 基线与限制

- 审计基线为 `main@a8630a8b` 的当前工作区，而不是一个已提交快照。
- 工作区审计时包含 108 个已跟踪变更和 87 个未跟踪文件；这些变更不能全部归因于本轮审计，
  因而结论只覆盖两份方案、本文及其可达调用链。
- 已核对客户端路由与请求转换、SKU capability、Ability 发布与候选过滤、adapter 复检、
  `TaskCreateAttempt`、创建三态、计费 hold/结算、真人授权撤回、共享轮询、OpenAPI 和用户指南。
- 未连接真实飞彩账号、MySQL 或 PostgreSQL，也未接入生产日志/告警平台；这些内容仍是发布门禁，
  不能由 SQLite/单元测试和 build 结果替代。

### 1.2 当前结论

两份方案中原有的 S1、S2 和 B1 主链问题已经关闭；专家复审确认的 ModelArk
标量值域权限重叠、媒体重复解码、Vitest 收集缺口和低风险清理项也已实施。修复后的当前实现未发现仍可到达、
高或中置信度的严重代码缺陷：公开视频 SKU 由单一 capability 注册表驱动，路由、候选渠道、
adapter 和 Task 快照都校验或冻结同一合同；所有公开视频创建协议都进入 durable attempt；
创建结果使用显式三态，unknown 不会自动换渠道重发；计费、授权、撤回和取消复用共享 Task 底座。

关键证据：

| 合同目标 | 当前证据 | 判断 |
| --- | --- | --- |
| 单一 SKU 能力权威 | `model/video_sku_capability.go:44-250` 定义并解析 capability；`model/video_sku_implementation.go:16-97` 独立 pin adapter 能力并比较 hash | 已闭环 |
| 客户端 fail-closed 与候选约束 | `middleware/video_sku_capability.go:15-86`；`router/video-router.go:44-100` | 已闭环 |
| 在途任务冻结协议事实 | `controller/task_protocol_snapshot.go:40-44` 及 Task 合同字段 | 已闭环 |
| 发送前 durable attempt | `service/task_create_attempt_scope.go:8-27`；`controller/relay.go:542-650` | 已闭环 |
| 创建结果三态 | `relay/common/task_create_disposition.go:5-11`；`controller/relay.go:682-744` | 已闭环 |
| 资金与 exposure | `model/task_create_attempt_release.go:151-169`；`model/task_billing_atomic.go:18-145` | 已闭环，真实数据库仍需验收 |
| 公开 SKU 机器合同 | OpenAPI `x-fixed-seedance-2-capability`；`model/video_sku_contract_consistency_test.go` 逐值比较 capability | 已闭环 |
| omni 创建单一路径 | `video_contract.go` 直接调用 `jsonvideo.CreateRequest`；通用 converter 对 omni fail closed | 已闭环 |

### 1.3 本轮直接修复

复核优化后的代码时发现一处仍可到达的合同缺口，并已直接修复：

1. 即梦 `CVSync2AsyncGetResult` 原先读取 `task_id` 后会静默忽略其他请求字段，查询动作存在
   provider escape hatch。现在该分支只接受 `task_id`，未知字段返回 `400`：
   `middleware/jimeng_adapter.go:38-47`。
2. 增加查询动作携带 `extra` 时必须拒绝的回归用例：
   `middleware/video_contract_adapter_test.go:73-104`。
3. OpenAPI 即梦请求 schema 增加 `additionalProperties: false`，并用 `oneOf` 表达提交必需
   `req_key`、查询必需 `task_id`：`docs/openapi/relay.json:1027-1081`。
4. 一致性测试现在结构化检查上述未知字段策略和动作必填项：
   `model/video_sku_contract_consistency_test.go:136-152`。
5. 飞彩 omni 曾同时保留“类型化 capability builder”和“通用 body 重解析 builder”。
   调用链证明后者不可达，且在模型映射后并不等价；已删除 `jsonvideo.CreateRequestBody`，
   并让通用 converter 对 omni 显式 fail closed。
6. 五个固定分辨率 Seedance 2.0 SKU 的公开值域已写入 OpenAPI
   `x-fixed-seedance-2-capability`；一致性测试逐 SKU 比较 resolution、duration、ratio、
   媒体上限、媒体路径、禁用字段和生命周期。

### 1.4 专家复审意见的实施结果

对专家修正后保留的一项严重问题和五项建议再次追踪调用链，已按最小入侵原则完成：

1. **ModelArk 标量权限已归一。** `ModelArkVideoCreateConvert` 只保留 JSON 结构、必填项、
   content 形状和通用媒体 URL 校验：`middleware/modelark_video.go:121-150`。公开 SKU
   统一通过 `VideoSKUCapability.ValidateModelArkRequest` 执行值域校验：
   `model/video_sku_capability.go:279-345`；共享的 `service_tier` / `execution_expires_after` /
   `seed` / `safety_identifier` 合同规则集中于 `model/video_sku_modelark_contract.go:10-30`。
   duration、resolution、ratio 及 unsupported fields 仍由每个 SKU capability 授权，不再存在
   converter 的硬编码联合值域。回归同时证明 converter 允许未来结构化标量通过、
   已发布 SKU 必须 fail-closed：`middleware/modelark_video_contract_authority_test.go:16-68`、
   `model/video_sku_modelark_contract_test.go:12-62`。
2. **JSON omni 媒体验证已去除候选渠道线性重放。** 候选过滤现在按
   `(VideoUpstreamProfile, AllowServiceTier)` 请求内记忆兼容性结果，同配置的 N 个渠道
   只解析一次：`middleware/modelark_video.go:206-253`。选中 adapter 移除了重复的 profile
   预检，但保留 frozen SKU capability 复检和 JSON builder 自身的媒体校验，因而没有削弱
   middleware ↔ adapter 边界：`relay/channel/task/doubao/video_contract.go:33-49`。
3. **品牌回归已纳入默认 Vitest。** 测试收集范围精确增加
   `src/hooks/__tests__/**/*.{test,spec}.{ts,tsx}`，未扩展到会吸入 `node:test` 文件的
   `src/**`：`web/vitest.config.ts:14-21`。默认 `bun run test` 已实际执行品牌用例。
4. **低风险债务已清理。** durable attempt 退款从 `Refund` 提取为稳定业务概念
   `refundTaskAttempt`：`service/billing_session_task_attempt_refund.go:8-30`；对账重试写入失败现在
   记录 request-aware warn：`service/task_create_attempt_reconcile.go:52-59`；无效 `tokenID` 占位已删除。
5. **前端 logo 回退已归一。** footer 使用共享 `DEFAULT_LOGO`，不再保留不可达的
   `/logo.png` 字面量：`web/src/components/layout/components/footer.tsx:23-26,153-163`。
6. **SQLite JSON storage class 兼容性已修复。** 实际运行数据显示同一 `channel_info`
   列可由 SQLite 驱动以 `BLOB` 或 `TEXT` 返回；原 scanner 忽略了 `[]byte` 以外的类型，
   把合法 JSON 字符串转成 `nil` 后触发 `unexpected end of JSON input`。现在 scanner
   显式支持 `BLOB`、`TEXT` 和历史 `NULL`，并继续拒绝其他类型：
   `model/channel.go:169-184`。数据兼容回归见 `model/channel_info_test.go:12-42`。

因此，专家复审中的 S1 和 B1～B5 均已关闭；没有将已撤回的误报重新引入实现。

### 1.5 已执行验证

已通过：

- `GOWORK=off go build ./...`；
- `cd relaykit && GOWORK=off go build ./... && GOWORK=off go test ./...`；
- `GOWORK=off go test ./middleware ./model ./service -count=1`；
- 沙箱外完整重跑 `GOWORK=off go test ./model ./controller -count=1`，覆盖 SQLite `channel_info`
  storage class 兼容回归；
- `GOWORK=off go test ./relay/channel/task/doubao/... -count=1`；
- `go test ./relay ./controller ./router` 的 VideoSKU、attempt、disposition、即梦和 protocol snapshot 定向回归；
- `TestPublishedVideoSKURegistryMatchesOpenAPIAndUserGuides`；
- `cd web && bun run typecheck && bun run test && bun run build`，其中 Vitest 实际执行 7 个文件、23 个用例；
- 本轮修改的前端文件通过定向 `oxfmt --check`；
- `git diff --check`、`task docs:check`、`task ai:check`。

仓库级 `bun run lint` 仍被既有的全局 lint 债务阻断，`bun run format:check` 仍报告四个
与本轮无关的已有文件；本轮修改行未新增 lint/format 失败。

## 2. 严重问题

修复后未发现仍存在的严重问题。没有证据支持继续保留旧报告中的高/中严重度结论，也不把
“未完成真实环境验收”误报成已确认代码缺陷。

## 3. 建议项

### [B1 已关闭] ModelArk per-SKU 值域已形成机器可比较的公开合同

- **运行时权威**：`model/video_sku_capability.go` 仍是唯一执行权威。
- **公开投影**：`docs/openapi/relay.json` 的
  `ModelArkVideoCreateRequest.x-fixed-seedance-2-capability` 结构化发布五个 SKU 的精确值域。
- **漂移防护**：`TestPublishedVideoSKURegistryMatchesOpenAPIAndUserGuides` 解析该 extension，
  逐值与 capability 比较；任一受保护值只修改一侧都会使 CI 失败。
- **文档边界**：用户指南和内置 API Reference 明确该 extension 是机器合同，Markdown
  表格只是人类可读摘要，不参与运行时执行。
- **置信度**：高。

## 4. 代码健康度

总分：**9/10**。

| 维度 | 分数 | 依据 |
| --- | ---: | --- |
| 需求符合度 | 2/2 | 两份方案的主链目标均有可达实现和业务回归证据 |
| 单一路径与权威实现 | 2/2 | capability、attempt、共享 Task/轮询与资金状态均有唯一主路径 |
| 冗余与死代码控制 | 2/2 | 未发现本次范围内仍可到达的第二套 worker、Task 表、重试或计费路径 |
| 抽象与类型质量 | 2/2 | capability、disposition、attempt 状态均为显式领域类型，新增逻辑主要隔离在独立文件 |
| 验证与可运营性 | 1/2 | 自动化和机器合同已闭环，但真实三数据库、Provider 媒体组合与生产观测链仍未验收 |

扣分只对应明确列出的外部验收缺口；不对已关闭问题重复扣分。

## 5. 最优先的三个根因

1. **真实飞彩媒体组合尚未验收。** 2026-08-01 串行测试只覆盖纯文本、4 秒、
   `16:9` 和 `generate_audio=false`；需补齐首帧、首尾帧、参考图和参考音频真实链路。
2. **跨数据库事务正确性缺少真实引擎证据。** 在 MySQL、PostgreSQL 分别验证迁移、`FOR UPDATE`、
   attempt hold/transfer/release、reservation/revoke 和 cancellation claim 的并发顺序。
3. **真实 Provider 与生产观测闭环尚未形成验收证据。** 逐 SKU 验证 create → poll → content、
   Range、鉴权、异常响应和资金对账，并确认 metadata URL 健康事件、exposure paging、自动停用和
   人工恢复可被采集、聚合和告警。

建议状态：**代码评审通过；完成第 2、3 项真实环境门禁后再按单 SKU、单渠道灰度。**
