---
status: historical
owner: Dev Team
last-reviewed: 2026-09-14
archived-at: 2026-09-14
source-path: docs/80-dev/2026-09-14-Seedance统一任务状态与Synlink生成中状态修复方案.md
superseded-by:
  - docs/20-architecture/Seedance专用渠道与Link架构.md
  - docs/10-product/Seedance视频与素材产品设计.md
  - docs/50-planning/路线图.md
---

# Seedance 统一任务状态与 Synlink 生成中状态修复方案

## 问题与目标

Synlink 视频生成曾在排队后进入 `RECONCILIATION_REQUIRED`，显示
`upstream_contract_violation: unverified Synlink task status`，随后又恢复成功并结算。
用户提供的解密取证分析确认，查询状态依次为 `pending → processing → completed`；
本地插件缺少 `processing` 的映射，导致正常生成阶段被归类为不可采信观察。

用户希望该处理可扩展到其他 Seedance 协议，形成统一状态超集，同时保持现有协议逻辑。
本方案将其限定为：复用现有平台任务状态，各协议将已验证的上游状态映射到平台状态；
不把各 Provider 的原始枚举白名单合并成所有协议通用的宽松解析器。

2026-09-14 已按下述方案实施本地插件 `1.3.3`、待核对界面和回归测试，详情见
[四方案评审修复记录](2026-09-14-四方案评审修复记录.md)。未修改生产数据库或线上服务，
未发起付费生成；新版发布和真实 Provider 验收仍待完成。

目标：

1. 新版 Synlink 查询识别 `processing`，正常进入执行中，不产生该状态引起的待核对告警。
2. 所有 Seedance 协议继续复用唯一任务生命周期与资金入口，不增加状态表或第二套状态机。
3. 除 Synlink 新增已验证状态外，其他协议已有输入的解析结果、交付和资金行为保持一致。
4. 页面准确区分执行中、待核对和失败，避免诱导客户重复提交。
5. 新插件修复与历史任务冻结合同分开处理，不重写已结算事实。

## 当前实际情况

### 1. 证据来源与可信范围

| 来源 | 已获得的结论 | 限制 |
| --- | --- | --- |
| 用户提供的上游解密取证分析 | 同一查询接口返回 HTTP 200、合法 JSON；状态为 `pending → processing → completed`；生成阶段内部 `metadata.status=running` | 本轮没有独立读取或解密生产证据对象，属于用户提供的外部取证结论 |
| 同一取证分析的批次摘要 | 报告 8 个任务均经历相同告警并最终成功、计费 settled | 不据此推断其他协议、失败状态或全部模型已验收；不据 settled 单独证明金额正确 |
| 当前本地代码 | 插件版本 `1.3.2` 只接受 Synlink 的 `pending`、`completed`；宿主已支持 `running` | 代码核对不能代替部署版本、激活制品和多节点一致性核验 |
| 方案形成时的本地基线 | 7 项现有定向测试通过，覆盖结果解析、待核对恢复、插件升级和冻结版本保护 | 当时尚未新增 `processing` 成功路径测试；本轮新增回归结果见修复记录 |

仅保留上述脱敏结论，不复制解密响应、Task 私有数据、供应商任务 ID、生产凭据、完整签名 URL
或实际账务记录。测试样本必须使用合成 ID、示例域名及必要字段。

取证报告中“模型身份校验通过”不能由当前报错分支推导：该分支检查任务 ID 和错误对象，
没有比较 Provider 模型身份。签名 URL 的签名时间也不能单独证明对象落盘时间。
这两点不影响已定位的 `processing` 状态映射缺口。

### 2. 处理链与实施前基线

```text
Provider 查询响应
  → 原始轮询证据捕获
  → 创建时冻结的 Seedance 插件版本
  → 对应协议的状态与结果解析
  → 宿主归一响应检查及 Task 状态转换
  → 既有 Task 条件更新、终态交付与资金结算
```

代码定位：

| 职责 | 当前入口 |
| --- | --- |
| Synlink 查询解析 | `plugins/seedance-link/plugin.js`：`parseSynlinkVideoTask` |
| 插件版本元数据 | 同文件 `meta.version`；分析基线 `1.3.2`，本地修复版 `1.3.3` |
| 精确冻结版本分派 | `relay/channel/task/seedance/plugin_extension_bridge.go`：`normalizeSeedanceVideoTaskResponse` |
| 平台执行中转换 | `relay/channel/task/seedance/adaptor.go`：`ParseTaskResult` |
| 活动与终态分类 | `model/task_video_lifecycle.go`：`IsActive`、`IsTerminal` |
| 待核对与恢复 | `service/task_video_polling_link.go` |
| 任务持久更新和终态接线 | `service/task_polling.go`：`updateVideoSingleTask` |
| 冻结制品存储 | `pkg/seedanceplugin/store.go`、`model/task_plugin.go` |

`RECONCILIATION_REQUIRED` 是活动状态。它保留已有进度并继续查询，单次合同违例不触发终态退款。
后续可信观察可恢复排队、执行中或成功，并清除活动失败原因。既有整体超时与资金处置仍适用，
不能把待核对理解为无限等待；本期不改变这些规则。

当前 Seedance adapter 对排队、执行中、成功分别提供 10%、50%、100% 阶段进度。
进入待核对时可保留此前的 10%；这不是 Provider 真实生成百分比。开始时间由宿主首次接受
执行中观察时记录，不代表 Provider 的精确开始时间。

### 3. 当前已有共享能力

共享宿主已把 `processing` 和 `running` 转为 `IN_PROGRESS`。飞彩、Relay 媒体协议和 Ark
协议已有各自的生成中处理；FunCloud 和官方协议也有各自的状态合同及校验入口。
因此本次不需要新增公共状态枚举或通用状态猜测函数。

实施前，前端 `web/src/features/usage-logs/constants.ts` 的通用任务状态映射缺少
`RECONCILIATION_REQUIRED`；任务详情对任意非空 `fail_reason` 都显示“失败原因”，列表使用红色。
这是与后端协议缺口独立的呈现问题。

## 优化方案

### 1. 统一状态超集的边界

以现有 `TaskStatus` 与生命周期方法为权威，本表只是本方案中的职责说明，不建立第二份代码注册表。

| 平台语义 | 现有平台状态 | 协议职责 |
| --- | --- | --- |
| 排队 | `QUEUED`，其他已有受理状态保持原义 | 识别自身已验证的排队响应 |
| 执行中 | `IN_PROGRESS` | 将自身已验证的生成中状态归一到执行中 |
| 成功 | `SUCCESS` | 验证任务身份和交付结果，提交真实用量事实 |
| 失败、取消、过期 | `FAILURE`、`CANCELLED`、`EXPIRED` | 仅按该协议已有的可信终态语义进入，保留资金处理差异 |
| 当前观察不可采信 | `RECONCILIATION_REQUIRED` | 沿已有合同违例入口处理，不从未知文本推断业务结果 |
| 已确认无法按合同交付 | `PROVIDER_CONTRACT_FAILURE` | 保留已有适用规则，不由单次未知状态触发 |

不同协议可以映射到同一平台状态，但不能继承其他协议的原始枚举、字段位置、大小写宽容、
结果字段或退款判定。例如某协议识别 `processing`，不代表所有协议均自动接受它。
已有别名和大小写处理保持原行为，不借本次归一工作扩大或收紧。

官方协议现有宿主用量权威与透传校验路径保持不变；不为统一外观而把它改造成另一套插件解析。

### 2. Synlink 最小修复

在 `parseSynlinkVideoTask` 的状态分支增加一条：

```javascript
if (status === "pending") result.status = "queued";
else if (status === "processing") result.status = "running";
else if (status === "completed") {
  // 保留现有成功结果和用量处理。
}
```

新版目标路径：

| 上游 `task.status` | 插件输出 | Task |
| --- | --- | --- |
| `pending` | `queued` | `QUEUED` |
| `processing` | `running` | `IN_PROGRESS` |
| `completed` 且结果可信 | `succeeded` | `SUCCESS` |
| 其他未登记值或不可信观察 | 现有 violation | 现有待核对处理 |

具体边界：

- 状态唯一读取 `task.status`，不读取 `metadata.status` 作为备用状态或完成信号。
- 视频仍读取 `task.outputs`，用量仍从 `task.usage` 进入既有归一；不合并两层重复 usage。
- `processing` 不要求 `metadata` 存在；只根据已验证的封装状态表达执行中。
- 即使 `processing` 载荷已有内部成功状态、URL 或 usage，也不提前交付或结算。
- 保留身份、错误对象、结果数量、URL 与用量校验，以及未知状态保护。
- 创建响应合同保持 `pending + task.id`；查询样本不能扩大创建响应的接受范围。
- 本次不新增 `failed`、`cancelled` 等 Synlink 状态，不调整 `task.error` 前置判断；取得这些状态的
  真实合同后另行处理，避免把失败终态与查询失败混同。
- 不改请求路径、模型映射、素材域、价格公式、轮询频率、整体超时或通用退款规则。

官网最新文档中的 ModelArk 路由和本任务使用的旧封装接口不同。当前修复仅针对已冻结的
`synlink_video_v1` 查询合同，新接口迁移保持独立待验收事项。

### 3. 权威实现与改动清单

| 文件或范围 | 拟改动 | 不扩大范围的理由 |
| --- | --- | --- |
| `plugins/seedance-link/plugin.js` | Synlink 增加映射，发布新补丁版本 | 对应本次真实履约的插件权威入口 |
| `relay/channel/task/seedance/` 下独立测试 | 新版解析、三阶段序列、用量与版本隔离回归 | 不只以旧实现输出为期望 |
| `service/task_contract_reconciliation_test.go` 或同边界独立测试 | 按需要补状态序列与持久资金不变量 | 复用真实 Task 更新与资金入口，不复制生产逻辑 |
| `web/src/features/usage-logs/` | 共享状态标签、详情及列表的待核对呈现与测试 | 覆盖任务详情、列表和复用该列的移动视图 |
| 前端七语言 locale | 配套翻译 | 实施前遵循项目 i18n 技能，不手工绕过工具写 locale |
| 相关当前事实文档 | 实施验证后更新 Synlink 状态说明及待核对呈现 | 生产验收与本地实现状态分别表述 |

`relay/channel/task/seedance/thirdparty/synlink.go` 服务无插件快照的历史路径，本次保持旧合同。
只修改它不能修复已冻结插件版本的任务，也不能通过改写旧 adapter 行为绕过历史版本边界。
若仍有此类活动任务，先盘点，再单独评审历史处置，不建立运行时 fallback。

后端预计无需修改 NEWAPI 原生热路径。前端通用任务日志文件的修改仅限新增状态标签和根据状态
选择诊断呈现，其他任务既有显示保持不变；未来同步上游的冲突面限于上述局部状态表和显示分支，
禁止重排整文件、抽取无关组件或改写通用生命周期。

### 4. 前端呈现

- 为 `RECONCILIATION_REQUIRED` 增加统一可翻译标签“状态待核对”和提醒色；各 Seedance 协议共享。
- 详情在该状态下显示“核对原因”；列表不把该诊断渲染成业务失败的红色提示。
- 提供简短说明：暂未确认最新结果，系统将继续查询，请勿重复提交。后续到达实际终态后，
  说明必须随状态变化，不能继续承诺查询。
- 仅按后端持久状态展示，不解析错误字符串猜测任务状态，不在前端轮询或推断供应商状态。
- 管理员保留脱敏诊断；普通客户沿现有权限和公开错误过滤规则显示，不扩大诊断字段权限。
- 恢复成功后使用既有清理结果，不再显示活动告警；历史原因保留在受保护证据中。
- 本次只补缺失的待核对呈现，不顺手重定义其他状态或增加虚构实时进度。

### 5. 插件发布、历史任务与回退

1. 实施时先核对制品库中版本占用，使用新的补丁版本；若未占用，可从 `1.3.2` 升为 `1.3.3`。
   插件 API 版本保持 `3`，不新增接口。`plugins/seedance_embed.go` 从制品读取版本，不另建版本常量。
2. 同版本源码不得覆盖。已有 `SaveTaskPlugin`、`EnsureSeeded` 与激活校验保持不变。
3. 部署并通过现有机制激活新版本后，核对实际活动版本、发布校验与各运行节点同步情况；不能仅凭
   本地文件版本判断上线完成。
4. 新版本生效后受理的任务必须冻结新版；已在创建阶段固定旧版的请求和已有 Task 继续使用旧版。
5. 旧版活动任务仍可能在 `processing` 时进入待核对，但可按原合同接受后续 `completed`。
   不为消除显示告警重写 Task 的插件版本、连接、状态或计费快照。
6. 已成功且结算的事故任务无需数据迁移、重新生成或重新结算。长期未完成的历史任务按既有证据和
   正式处置入口核查，不执行直接 SQL 改状态或退款。
7. 如新版出现新缺陷，通过既有管理入口调整后续新受理使用的版本；保留所有被任务引用的制品。
   切回旧版不会改变已冻结新版的任务，也会重新带回旧版的已知状态缺口，必须明确评估这一影响。

### 6. 测试与验收

先添加能在当前版本稳定失败的新版 `processing` 用例，再实施分支修复。
全部样本去除真实身份、签名参数和生产媒体；使用确定性输入，不重放付费创建。

| 场景 | 必须保护的结果 |
| --- | --- |
| `pending → processing → completed` | 同一任务依次排队、执行中、成功，全程无该状态引发的待核对 |
| 连续重复 `processing` | 保持执行中；开始时间只记录一次；不产生终态结算、退款或新的创建请求 |
| `processing` 不含 metadata | 合法进入执行中，不引入额外必填条件 |
| `processing` 携带 metadata 成功、视频 URL 或 usage | 不提前成功、交付或结算，不读取内部状态作为第二权威 |
| completed 的两层相同 usage | 只使用现有 `task.usage` 来源，数量不相加 |
| 未知、空或错误类型状态 | 保持现有不可采信处理，不伪装执行中或失败 |
| ID 不匹配、错误对象、非法或缺失视频结果 | 继续失败关闭，不弱化现有校验 |
| 未知观察后取得可信完成结果 | 恢复成功并清除活动原因，保留既有资金幂等 |
| 重复或并发完成观察 | 只接受既有规则允许的用量与资金目标，不重复扣款 |
| 已成功后收到迟到的 processing | 成功和结算事实不倒退 |
| 其他已登记 Seedance 协议的既有样本 | 映射、字段校验、结果与计费事实保持原输出 |
| 新旧插件并存及旧版本缺失 | 新任务固定新版，旧任务固定旧版；缺失冻结制品时不回退活动版 |
| 前端待核对及恢复 | 友好状态标签、提醒色和核对原因；恢复成功后告警消失；其他状态显示不变 |

现有 `TestSynlinkUntrustedObservationCannotBecomeFailure` 中拒绝 `processing` 的断言属于历史
Go 路径，不作为新版插件语义标准。本次不为让新旧实现机械相等而改写历史合同；新增插件测试应
直接断言 `processing → running`。已有终态一致性测试保留其他未改变场景，并补版本行为隔离。

验证命令按[命令清单](../30-engineering/命令清单.md)执行。先运行受影响的定向测试，再完成项目
要求的 Seedance 包级检查：

```bash
go test ./model ./middleware ./controller ./service ./relay/...
go vet ./model ./middleware ./controller ./service ./relay/...
```

前端使用 `cd web && bun run test` 运行受影响测试，并完成 `bun run typecheck`、涉及文件 lint
及必要生产构建；i18n 遵循项目技能与七语言要求。若实际变更触及 `relaykit/` 或其公共 API，追加
`cd relaykit && GOWORK=off go build ./...`，本方案预计无需修改该模块。

文档改动执行 `task docs:check` 和 `task ai:check`；若后续修改公开 API 文档，再执行
`cd web && bun run docs:validate`。

生产验收应记录脱敏的协议、冻结插件版本、状态序列、是否出现待核对、用量来源与结算次数。
优先观察正常业务中的新版任务；额外付费生成须有明确授权。代码与本地测试通过不代替真实供应商
履约、金额核对、MySQL/PostgreSQL 验证或多节点发布验证。

### 7. 完成条件与剩余事项

- [x] 新版 Synlink processing 映射及最小回归通过（本地插件 `1.3.3`）。
- [x] 其他 Seedance 协议既有行为回归通过（本地模拟测试）。
- [x] 待核对页面呈现、既有权限回归与七语言检查通过。
- [ ] 新版发布、冻结版本隔离和各节点实际加载核对完成。
- [ ] 新版真实查询观察到完整三阶段序列，交付及结算符合既有合同。
- [ ] 已有事故任务保持原成功结果和账务；未完成旧任务按需核查。
- [ ] 实施完成后列出事实收敛清单，按项目治理更新对应架构、产品、运维和 UI 文档。
- [ ] 路线图中仅单独收敛 processing 已验证部分；失败、不存在、鉴权、尾帧、账单和新接口迁移
  等未完成事项继续保留。
- [ ] 前端通用任务状态映射仍缺 `CANCELLED`、`EXPIRED`、`PROVIDER_CONTRACT_FAILURE` 的本地化
  标签（未映射状态回退为原始枚举文本与中性徽章）。本次按边界只补待核对呈现；该同类缺口
  保留为独立呈现事项，不并入本修复。

本文件保留为实施方案。后续事实收敛与归档遵循项目治理；不因方案写入自动更新受保护目录，
不修改任何既有归档。

## 相关依据

- [硬约束](../00-context/硬约束.md)
- [Seedance 专用渠道与 Link 架构](../20-architecture/Seedance专用渠道与Link架构.md)
- [异步任务与计费事实架构](../20-architecture/账单计费-异步任务与计费事实架构.md)
- [音视频请求证据架构](../20-architecture/音视频请求证据架构.md)
- [音视频证据运维手册](../40-operations/11-音视频证据运维手册.md)
- [路线图](../50-planning/路线图.md)

## 本次文档验证

以下为方案形成时的文档检查记录；本轮业务实施和验证见关联修复记录。

- 2026-09-14，本地 `task docs:check` 通过。
- 2026-09-14，本地 `task ai:check` 通过。
- 本文件相对链接目标检查通过，未发现失效目标。
- 方案形成阶段未修改公开 API 文档或业务代码；本轮代码实施已另行验证，仍未修改公开 API 文档。
- 2026-09-14，评审收尾：登记前端通用状态映射遗留缺口（见第 7 节末项）并复核本地实施状态后，
  重新执行 `task docs:check`、`task ai:check`、Go 全量测试与 vet、前端全量 vitest 及 typecheck，
  均通过。
