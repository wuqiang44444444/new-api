---
status: historical
owner: Dev Team
last-reviewed: 2026-09-09
archived-at: 2026-09-14
source-path: docs/80-dev/2026-09-09-Seedance定价回归修复与Doubao渠道迁移评估.md
superseded-by:
  - docs/20-architecture/账单计费-计费表达式与协议探针架构.md
  - docs/20-architecture/账单计费-APIKEY用量账单架构.md
---

# Seedance 定价回归修复与 Doubao 渠道迁移评估

## 1. 问题与目标

上游 rc.31 合并提交 `2023de5d1` 引入任务插件及任务用量编辑器后，部分 Seedance 价格条目不再显示
Token 预扣上限。本次目标是明确修复边界，并回答现有渠道在同时考虑视频 API、素材 API、素材引用和
计费合同后，能否迁到原生 Doubao 渠道。

本文保留最初分析与方案，并在第 4、5 节记录本次修复结果。用户已明确暂不考虑合并或迁移：
Seedance Link 与原生 Doubao 继续作为完全不同的渠道。本次已修改并验证业务代码；没有修改数据库中的
价格、渠道类型、素材或资金，没有执行迁移，没有发送真实 Provider 请求，也没有部署到生产。
本机数据不等同于远程生产；真实 Provider、账单和灰度验收仍未完成。

“Doubao 渠道”在本文指 `ChannelTypeDoubaoVideo`（54）及其内置 `doubao` 任务插件。
插件同时声明支持 VolcEngine（45），但这不意味着 Link 专用渠道（62）或它的素材协议可以直接转入。
这里不提出把所有渠道改成通用 TaskPlugin 渠道，也不将 Provider 模型同名视为兼容证据。

## 2. 当前实际情况（修复前取证）

### 2.1 原生与 Link 的不同职责

| 项目 | 原生 Doubao 插件 | Seedance Link |
| --- | --- | --- |
| 北向创建 | `/doubao/api/v3/contents/generations/tasks`，以及宿主视频/Responses 接口 | `/api/v3/contents/generations/tasks`，统一 ModelArk V3 |
| 北向查询 | 插件声明的单任务查询和宿主查询 | ModelArk 单任务查询、列表、删除及冻结生命周期 |
| 视频南向 | 固定 ModelArk 路径、Bearer 鉴权、顶层 `id/status/content.video_url` | 按登记协议处理不同路径、鉴权、请求字段、响应包裹和状态 |
| 素材管理 | Doubao 插件没有实现本站 `/v1/assets`、`/v1/asset-groups` 合同 | 依专用渠道与素材协议路由，包含默认组、真人认证及托管例外 |
| 选渠 | 原生分发及插件筛选 | 每个客户模型唯一启用的 Seedance Channel |
| 预扣 | 插件提取请求估计用量 | 已存 Token 预算加冻结的请求探针；仅已登记纯参数特例可不要求 Token 上限 |
| 表达式 | `u("tokens")`、`u("seconds")` 等；输出美元 | `c`、`param("_task.xxx")`；当前表达式输出再除一百万 |
| 完成结算 | 实际 facts 覆盖冻结预估 facts | 实际 Token 替换 `c`，使用冻结价格与探针 |

原生插件适用于通用供应商任务接入，含脚本版本管理、请求转换、任务查询、媒体结果和用量提取；
任务用量编辑器只是该体系的管理页面。两套业务共享 Task、表达式引擎等底座，不共享同一用量合同。

例：$5/百万 Token、实际 100000 Token。Link 的 `c * 5` 经后端除一百万得到 $0.50；
原生插件的 `u("tokens") * 5 / 1000000` 已输出 $0.50。两者不能只替换变量名或复制单价表达式。

依据：[Doubao 插件](../../plugins/tasks/doubao/plugin.js)、[视频 Router](../../router/video-router.go)、
[适配器分流](../../relay/relay_adaptor.go)、[表达式合同](../../pkg/billingexpr/expr.md)、
[额度换算](../../pkg/billingexpr/settle.go)。

### 2.2 已确认的回归及影响

**P1：价格元数据把 Link 模型归到原生插件。**

`model/pricing.go` 根据模型名或通用别名匹配插件并添加 `billing_usage_schema`，没有让专用渠道身份
阻止插件元数据覆盖。别名索引扫描启用渠道映射时包含 Link。前端仅凭 schema 选择
`TaskUsagePricingEditor`，该组件没有 Token 上限参数，也不理解原 `c / _task` 表达式矩阵。

本机价格配置合并核对包含 38 个 Seedance 名字：37 个表达式、1 个旧按 Token 条目。13 个条目被误选
编辑器，其中 9 个启用 Link 模型、4 个停用原名；其上限全部仍在。23 个启用且已定价的 Link 模型通过
真实预扣函数的合成探针试算。缺失上限的新请求仍会在 Provider POST 前失败，而非自动免预扣。

13 个受影响名字：

| 类别 | 模型 |
| --- | --- |
| 停用原名 | `doubao-seedance-2-0-260128`、`doubao-seedance-2-0-fast-260128`、`doubao-seedance-2-0-mini-260615`、`doubao-seedance-2-5-260628` |
| 启用别名 | `doubao-seedance-2-5-m`、`seedance-2-0`、`seedance-2-0-fast`、`seedance-2-0-fast-m`、`seedance-2-0-m`、`seedance-2-0-mini`、`seedance-2-0-mini-m`、`seedance-2-0-t`、`seedance-2-5` |

对截图表达式切换可视化模式时，解析失败会生成全零矩阵并改写草稿为
`tier("base", u("tokens") * 0 / 1000000)`；保存后才影响持久配置。已启用 Link 通常会在保存校验阶段
拒绝不适用的用量表达式；停用同名模型可能先通过插件校验，重新启用时暴露不兼容。
不得仅靠保存校验兜底，更不得只补一个上限输入框。

依据：[价格生成](../../model/pricing.go)、[别名视图](../../model/task_model_alias.go)、
[编辑器选择](../../web/src/features/system-settings/models/model-pricing-sheet.tsx)、
[可视化转换](../../web/src/features/system-settings/models/task-usage-pricing-editor.tsx)。

**P1：本地异步计费接线截断原生插件实际用量结算。**

共用控制器创建任务时调用 `AttachAsyncTaskBilling`。该函数对任何表达式快照附加本地异步上下文，
包括 `TaskUsageBilling=true`。完成时 `settleTaskBillingWithState` 先处理，只传 `c` 和 Body 探针，
没有提供合并后的 Usage；返回已接管后，原生插件的完成 facts 合并分支不执行。

隔离复现使用 Go overlay 加载仓库外测试、服务包既有内存 SQLite fixture，调用真实
`AttachAsyncTaskBilling → prepareTerminalTaskBilling → settleTaskBillingOnComplete`。
各场景预扣 1000 quota、应收 500 quota，同时断言任务额度和用户余额：

| 场景 | 结果 |
| --- | --- |
| 原生 Token 插件，仅原生结算上下文 | 正确退 500 |
| 相同 Token 插件，经过当前控制器附加逻辑 | `u("tokens")` 为 nil，计费 failed，保留 1000 |
| 原生按秒插件，仅原生结算上下文 | 正确退 500 |
| 相同按秒插件，经过当前控制器附加逻辑 | 错误判无实际用量，保留 1000，并标 settled |

两个控制组通过、两个真实附加路径的业务断言失败。Token 示例表达式为
`tier("base", u("tokens") * 5 / 1000000)`，预估 200000、实际 100000；按秒表达式为
`tier("base", u("seconds") * 0.1)`，预估 10、实际 5；测试的 `QuotaPerUnit=1000`。
既有只手工构造 `BillingContext.TieredSnapshot` 的测试不能覆盖真实附加边界。

本机所有任务中 `TaskUsageBilling=true` 的快照数为 0，因此没有证据证明本机已发生此类错账；
线上影响需另取脱敏只读清单。它是接取新原生快照合同后，本地旧接管条件过宽造成的集成回归，
不能归因为 Doubao 原生用量算法本身失效。

依据：[共用任务创建](../../controller/relay.go)、[本地上下文附加](../../model/task_async_billing.go)、
[完成结算分流](../../service/task_polling.go)、[本地表达式结算](../../service/task_tiered_settle.go)。

**P2：上限说明与当前结算行为不符。**

现有说明声称超过上限不补扣而记欠费；当前资金实现会先补扣，资金不足才进入 debt。
上限是预扣预算，既不是视频生成参数，也不是最终收费封顶。依赖实际用量的 Link 任务缺少用量时
应保留 hold 并等待补查，不补零。

### 2.3 本机全部专用渠道：视频与素材双协议核对

本次读取本机主库 `type=62` 的全部 12 个渠道，包括 8 个启用、4 个停用。
只列管理识别信息和协议分类，不记录 Base URL、凭据、Project/Region、素材 ID、完整配置或私有任务。
渠道显示名不作为协议或启停判断依据。

| ID / 管理名称 | 状态 | 视频协议 | 素材协议 | 迁入当前 Doubao 的判断 |
| --- | --- | --- | --- | --- |
| 66 飞彩-SD2-VIP | 启用 | `feicai_videos_v1` | `none` | 不可直接迁；视频路径、请求结构与完成解析不同 |
| 71 不用了-飞彩-SD2-不稳定 5 渠道 | 启用 | `feicai_videos_v1` | `none` | 不可直接迁；同上，名称“不用了”不代表停用 |
| 72 FunCloud Seedance V3 · 统一素材库 | 启用 | `funcloud_modelark_v3` | `funcloud_material_hosted` | 不可直接迁；专有字段、submitted 状态、托管引用解析均有差异 |
| 75 FUNCLOUD-SD2.5-海外-无素材 | 停用 | `funcloud_modelark_v3` | `funcloud_material` | 不可直接迁；同类视频差异，且实际配置了素材协议 |
| 76 MOXING-SD2-9 折-单素材库 | 启用 | `moxing_media_task_v1` | `moxing_joycreator_assets_v1` | 不可直接迁；媒体任务请求/响应及素材协议不同 |
| 77 MOXING-SD 2/2.5国内-共享素材 | 启用 | `moxing_modelark_media_v1` | `moxing_volc_assets_v1` | 不可直接迁；正文类似 ModelArk，但路径及响应归一不同 |
| 79 MOXING-SD2-海外-9 折-素材库 | 启用 | `tokensave_media_task_v1` | `tokensave_assets_v1` | 不可直接迁；实际使用 TokenSave 协议，不能根据名称当官方 |
| 81 火山官方-seedance-双思-官网价格 | 启用 | `modelark_v3_volcengine` | `volcengine_assets_action_v2024_01_01` | 纯视频候选；不能整体迁素材或保持原完整北向 API |
| 90 火山官KEY-SD2-85折 | 停用 | `modelark_v3_volcengine` | `volcengine_assets_action_v2024_01_01` | 纯视频候选；停用状态保持，不自动启用 |
| 92 移动云-SD2-共享素材-8折 | 启用 | `modelark_v3_cmcc` | `cmcc_aicc_assets_v2` | 不可直接迁；参考视频需动态请求头，素材需独立签名协议 |
| 97 火山官KEY-SD2-85折-新 KEY | 停用 | `modelark_v3_volcengine` | `volcengine_assets_action_v2024_01_01` | 纯视频候选；停用状态保持，不自动启用 |
| 104 Synlink 海外 Seedance · 统一素材库 | 停用 | `synlink_video_v1` | `funcloud_material_hosted` | 不可直接迁；创建/查询路径、task 包裹、状态、结果及托管引用不同 |

81 的四个客户模型 `seedance-2-0`、`seedance-2-0-fast`、`seedance-2-0-mini`、`seedance-2-5`，以及
90、97 的四个官方原名，其映射最终模型均在当前 Doubao 插件声明表内。这只满足模型识别条件，
不证明供应商实际可用、素材可迁或客户 API 兼容。

结论分两层：

- **原视频 API、素材 API 与现有引用能力都必须保持：当前 0 个可以直接整体迁入。**
- **接受切换客户视频 API，且不要求由 Doubao 承接本站素材管理：81、90、97 是纯视频试点候选。**
  仍须先修复原生插件结算缺陷并完成视频/用量验收，不能直接改类型上线。

### 2.4 视频 API 兼容不能只比较路径

原生 Doubao 南向固定为 `POST /api/v3/contents/generations/tasks` 和
`GET /api/v3/contents/generations/tasks/{id}`，使用 Bearer，解析顶层 `id/status/content.video_url`。
北向则是带 `/doubao` 前缀的声明路由或宿主协议，不接管 Link 原来的无前缀入口。
其 `buildSubmitRequest` 会从 metadata/content 提取并重组文本与引用；即便正文大体相似，也不是
对 Link 请求字节和语义的无损透传。

原有 ModelArk 列表、取消/删除、尾帧交付、错误脱敏、任务 ID、引用和完成快照行为也必须逐项核对；
给客户端只改一个 Base URL 或前缀不构成完整 API 迁移。

关键供应商差异：

- **火山官方**：视频路径和基本响应合同匹配，属于最接近的候选。素材另走官方 Action API，
  使用 AK/SK、Region、Project 和请求签名；不是视频 Bearer Key 能替代的接口。
- **FunCloud V3**：路径相同，但现有 adapter 固定发送 `real_person_mode=true`，有参考视频时添加
  `omni_reference_task_type=reference`，并将 `submitted` 归一为排队。Doubao 插件没有这些等价处理，
  `submitted` 不在其当前已识别状态中。托管 `fhas_*` 还必须在预扣前校验并转为内部临时 URL。
- **移动云**：视频使用官方形状，但参考视频请求须添加 `Input-Has-Video: true`，由请求内容决定。
  Doubao 插件不生成该头；静态填一个 true 不能等价覆盖有/无视频两类请求。素材是 CMCC AICC V2 签名。
- **墨行/TokenSave**：使用 `/v1/media/generations`、`/v1/media/tasks/{id}`；至少路径不同，且相关协议
  还承担请求转换或响应包裹归一。固定 Doubao 路径无法只靠 Base URL 同时改写创建和查询。
- **飞彩**：使用 `/v1/videos` 与对应查询，并有专属请求能力与计费事实，缺素材并不意味着视频兼容。
- **Synlink**：使用 `/v1/video/generate` 和 `/v1/video/tasks/{id}`，读取 `task.id/task.status/outputs`，
  与 Doubao 的顶层响应不同；不能消费 Provider opaque ID，本站托管图片要先解析 URL。

依据：[协议路径登记](../../relaykit/dto/upstream_protocol.go)、[视频协议转换](../../relay/channel/task/seedance/video_upstream.go)、
[FunCloud 请求](../../relay/channel/task/seedance/funcloud_modelark.go)、
[FunCloud 状态](../../relay/channel/task/seedance/thirdparty/funcloud_modelark.go)、
[移动云请求头](../../relay/channel/task/seedance/request_headers_cmcc.go)、
[官方素材签名](../../relay/channel/task/seedance/assets/official_action.go)、
[移动云素材](../../relay/channel/task/seedance/assets/cmcc_aicc_v2.go)。

代码还登记了本次 12 个渠道未使用的 `modelark_v3_byteplus`、`ark_media_v1` 和 `funcloud_seedance`：
BytePlus 视频基本形状接近官方，但 Provider 模型、区域和素材 Action 独立，且相关海外模型未在当前
Doubao 插件模型声明中，需另外验证模型识别与南向，不能自动列为可迁；Ark 的 `/v1/ark/media/*`、
FunCloud 旧版的 `/api/v2/open/aigc/*` 均与 Doubao 固定路径不匹配。

### 2.5 素材控制面、视频引用与存量任务必须分开判断

**素材控制面不可直接迁。** `assetAdapterForModel` 查找唯一启用的 Seedance Channel，
`seedanceAssetAdapter` 再强制检查类型 62。改成 54 后，即使保留原 `asset_upstream_protocol` 字段，
本站素材服务也不会继续执行该协议。普通素材 CRUD、默认组、真人认证与查询不会随视频接口自动继承。

**已有 Provider opaque 引用不等于素材 API 已迁。** Doubao 可把 metadata.content 中的 opaque URL
作为内容交给上游；同一 Provider 账号/Project 下已有 `asset://...` 可能仍可使用，但必须实测。
这只说明视频能消费一个引用，不意味着它能创建、管理素材，或能使用本站 `fhas_*`。
换账号/Project/区域时不得承诺旧引用可用，也不自动复制或删除 Provider 资源。

**托管素材更不能仅迁字段。** `asset://fhas_*` 是本站资源，不是 Provider ID。Doubao 插件没有本站
归属校验、冻结和签名 URL 解析；直接发送会泄漏无法被上游解释的命名空间。72、104 不能按普通引用迁。

**视频迁走、同模型素材留下也不是当前配置功能。** 现行素材选渠绑定 Seedance 客户模型及唯一渠道；
不能未经设计把同名模型拆成原生视频与 Link 素材两套路由，并继续声称具有一个相同 API 合同。
若未来确需独立素材控制面，应另行评审职责、模型身份、授权、复用域和客户端合同，本修复不扩展此能力。

**渠道与已受理任务不原地改写。** 已建立素材租户后渠道类型不可变；迁移不是数据库里把 62 改成 54。
新入口应另建渠道及独立客户模型，旧 Task、attempt、冻结连接、价格和素材事实仍按旧合同完成。
原生和 Link 的价格键不能在并存期间指向同名却要求不同计费单位；不得把旧任务快照迁成插件快照。

依据：[素材服务类型约束](../../service/asset_service.go)、[素材租户边界](../../model/channel_asset_tenant_boundary.go)、
[素材对外合同](../20-architecture/Seedance模型素材库支持矩阵.md)、
[无状态素材架构](../20-architecture/Seedance无状态素材代理架构.md)。

## 3. 优化方案

### 3.1 先修定价与结算，不以迁移绕过回归

| 步骤 | 拟采取动作 | 可观察验收 |
| --- | --- | --- |
| A：价格身份 | 复用专用渠道目录，明确已配置 Link 身份，覆盖启用与停用。价格接口对其跳过原生插件 schema、样例及不适用的表达式继承 | 同名原名、启用别名、停用别名均显示原表达式与上限；真实原生插件仍显示用量编辑器 |
| B：无损编辑 | 解析不了现有表达式时保持原式，不用默认零矩阵回写；已有非空价格不得因模式切换被隐式覆盖 | 打开、切换、保存、重开、切换模型都保留表达式和上限；新建模型仍有合理初始化 |
| C：结算责任 | 按冻结的 `TaskUsageBilling` 区分合同，让原生用量任务保留原生 facts 合并与结算责任；仅支持 c/_task 的本地上下文不得接管 | 经过真实控制器附加与持久化后，Token/秒/credit 的完成值正确产生补退差额 |
| D：说明 | 将上限说明改为预扣预算，说明实际费用补退、资金不足进入 debt，视频成功状态不因此回退 | 说明与实际资金状态一致，按项目 i18n 规则完成所有语言 |

C 的优先最小实现是限制本地异步附加入口的适用合同，保留原生结算路径，不新建另一套插件解释器。
必须同时检查已错误带上 AsyncBilling 的原生任务、重启补偿扫描和旧 pending/failed/settled 状态；
不能只修新任务的附加分支后认为存量已修复。先产出只读异常清单，核对实际用量与已发生资金，
再制定幂等恢复动作；不得批量清空标记、自动退款或重置 settled 触发重复资金操作。
本机无此类任务不代表线上无存量。

### 3.2 原生文件最小接线与冲突面

| 文件/范围 | 必要性与最小接线 | 上游同步影响 |
| --- | --- | --- |
| `model/pricing.go` | 现有 Link 目录投影不足以挡住后续插件元数据；仅增加调用本地身份判定的窄分支，主体放 Link 专属文件 | 价格字段或插件分支更新时需保留此处边界，避免重排原生循环 |
| 价格页与两个编辑器 | 现有分支消费 schema，需核对受控选择和无损切换；不复制第三套定价编辑器 | 局部 props/模式切换冲突；修复后保留原生用量能力 |
| `controller/relay.go` | 已有单行 AttachAsyncTaskBilling 接线优先保持，不为新修复扩写流程 | 避免扩大提交控制器冲突面 |
| `model/task_async_billing.go` 等本地扩展 | 在本地合同入口收窄接管范围，处理补偿适用边界 | 主体留本地新增文件；不得依模型名猜计费模式 |
| `service/task_polling.go` | 优先不改原生用量结算体；如存量分流必须接线，仅保留窄调用 | 禁止为消除重复重写整个完成/退款过程 |

不全局禁用 Doubao 插件，不用模型正则或改名遮盖当前错误，不改通用原生分发去识别或拒绝 Link。
不把管理端页面上缺一个字段解释为可以放宽预扣校验。

### 3.3 回归验证矩阵

- 价格身份：官方原名、启用映射、停用映射、未匹配插件的第三方、真正原生 Doubao、普通 Token 模型。
- 表单：打开/保存/重开，原始与可视化切换，无法解析表达式，新建/复制/跨模型切换；断言无价格和上限丢失。
- Link 资金：缺必要预算发送前拒绝，预扣充足/不足，实际高于/低于预算，失败退款，unknown 不重发/退款，
  缺用量恢复，冻结价格不受当前改价影响，重复观察与补偿幂等。
- 原生插件资金：从真实创建对象进入完成结算，覆盖 Token、秒、credit、枚举变化、完成部分字段缺失、
  实际高于/低于预估、失败退款、补扣不足；不能仅手工构造少了 AsyncBilling 的快照来验证。
- 存量：带错误 AsyncBilling 的原生快照与正常 Link 快照分别处理，避免吞掉原生结算或重复执行资金。
- 检查次序：先最窄行为测试，再按实际修改扩大到相关包；修改 `relaykit/` 时追加独立构建。
  前端使用 Bun；若改文案，编码前读取项目 i18n skill。

### 3.4 官方纯视频候选的分阶段试点

本节仅保留前期条件式迁移评估。用户现已明确暂不考虑合并与迁移，以下试点不在本次实施范围；
不授权执行迁移，也不改变现有 Link 边界规范。

1. 先完成 3.1–3.3 的修复验证，尤其是原生插件实际用量结算；当前缺陷存在时不进入收费试点。
2. 从 81、90、97 中选择一个经管理员确认有效的线路；保持原渠道原状态，不自动启用停用渠道。
3. 另建 Doubao 渠道与独立客户模型，精确映射已登记官方模型。核对视频 Bearer 凭据，而非复制素材 AK/SK。
   使用原生用量表达式重新表达同一美元价格，按有/无参考视频、分辨率与实际用量逐项验证金额等价。
4. 明确该新模型对外采用原生视频 API。现有 ModelArk 客户端、查询/列表/取消/删除/尾帧与素材调用
   逐项出具差异清单，不设置无感转发、不让原生入口推断 Link 模型。
5. 首轮仅验证不依赖本站素材管理的直接 URL 请求：文本、图片、视频、音频、默认/显式参数与零值，
   创建后查询、结果交付、失败、用量缺失及补退费。真实请求与账单验收须另行执行并记录。
6. 已有官方 opaque 引用若需要使用，单独验证相同 Provider 租户和有效引用；标明这是消费旧引用的验收，
   不是素材控制面迁移。本站 `fhas_*` 不进入该试点。
7. 按独立新模型灰度，由客户端明确切换。旧模型与旧任务继续原生命周期；回退只停止新入口，
   不重发未知任务、不改历史资金与快照、不删除素材。

如果目标是“视频与素材都迁到 Doubao，客户无感”，当前没有实施入口；需另行决定原生任务能力是否
扩展独立素材接口及其身份/授权合同，不能作为此次定价修复的附带改动。

## 4. 本次实施与验证

### 4.1 已实施边界

- 价格接口沿用专用渠道目录识别启用及停用 Seedance，跳过原生插件用量 schema、样例和别名价格继承；
  既有本地表达式与预算不做批量修改。原生 Doubao 的 schema 与别名继承保留。
- 保存 Seedance 价格时先用其已启用协议校验；没有启用渠道时核对全部已配置的停用合同，不能退回
  原生插件校验。明确拒绝原生 `u(...)` 用量引用，包括能通过空值分支算出零费用的表达式。
- 原生 `TaskUsageBilling=true` 快照不再附加本地 AsyncBilling，继续由原生流程合并实际 UsageFacts，
  按其美元合同结算；Seedance 仍使用自己的 `c / _task`、冻结事实及资金流程。
- 已错误附加 AsyncBilling 的原生旧任务停止进入本地自动结算、退款和补偿扫描；处理入口记录对账警告，
  保留原状态及资金。此措施是避免继续错误处理，**不是已经修复存量账单**，不得批量重置或自动退款。
- 两个编辑器均在无法无损转换时保持原式并提示；Seedance 编辑器还保留不可解析的请求乘数，包含首次
  打开和模式切换。主动清空后仍可使用默认可视化初始化。上限说明改为预扣预算及结算差额语义。
- Link 与原生渠道不能共用客户价格键，停用渠道也保留其价格身份。管理保存拒绝跨类型同名；旧冲突在
  价格 API 返回 `billing_contract_conflict`，不声明某一方 API，编辑面板提示并拒绝提交。原始 JSON
  的表达式、计费模式、预算、固定价格和倍率选项均在写入前拒绝冲突；删除价格键仍可进行。
  不按 Provider 映射推断冲突，不限制两类渠道使用不同客户名称映射同一 Provider 模型。

实现依据：[价格隔离](../../model/pricing.go)、[保存校验](../../controller/billing_expression_validation.go)、
[Link 表达式校验](../../setting/billing_setting/task_billing.go)、
[异步附加边界](../../model/task_async_billing.go)、
[旧快照保护](../../service/task_usage_billing_boundary.go)、
[编辑器保护](../../web/src/features/system-settings/models/task-usage-pricing-editor.tsx)。

本次原生文件接线及必要性：

| 文件 | 必要的最小改动及上游同步关注点 |
| --- | --- |
| `model/pricing.go` | 两个插件条件与目录身份判定，增加一个可选冲突标志；防止两种合同被静默合并，保持原生价格循环结构 |
| `controller/option.go` | 值规范化后增加一次本地校验调用，必须位于倍率缓存修改和持久化之前；具体规则全部放独立本地文件 |
| `model-pricing-sheet.tsx`、`features/pricing/types.ts` | 读取可选冲突标志、显示提示并阻止提交，不能依赖隐藏输入框作为后端校验 |
| 两个价格编辑器 | 仅调整无损解析、初始模式和错误提示；不得遗漏请求乘数，不重构可视化矩阵或 Token 定价体系 |

`controller/relay.go`、`service/task_polling.go`、原生插件、原生分发、视频和素材 adapter 均不改；
管理渠道的已有单行校验接线复用本地规则，没有增加请求时身份检查、数据库唯一约束或自动修复。
未来同步上游需保留上述管理与显示边界，其它后端变化留在本地扩展文件或新增独立文件。

### 4.2 已验证结果

- 新增测试先复现价格 schema/别名污染、原生真实附加链的 token/秒/credit 结算错误、可视化切换覆盖
  表达式，再验证修复通过。另复现并阻止 `u("seconds") == nil ? 0.0 : ...` 被错误保存。
- 新增后端测试覆盖真实附加后持久化/重载的补退差额、原生失败重复退款幂等、旧 native AsyncBilling 的
  pending/failed/debt/settled 隔离、停用价格与启用合同优先校验；既有 Link 资金和协议测试继续通过。
- 真实前端组件配合公开价格响应缓存，验证截图公式的预算从 325000 改为 400000 后提交草稿仍保留原式，
  切换原生模型后使用原生用量编辑器，保留原生价格。未把真实数据库价格改为测试值。
- 本机只读重核 38 个价格名字：31 个有 Seedance 渠道配置，13 个曾匹配原生插件元数据；修复后这 13 个
  均不再进入原生编辑器。37 条表达式保存校验全部通过；价格接口中可见条目的表达式均与配置一致；
  23 个启用且已定价 Link 模型调用真实预扣函数的合成探针通过。
- 本机 470 条历史 Seedance 任务：357 条冻结表达式重算一致，60 条失败额度为零，21 条旧任务缺实际用量
  且保留预扣，32 条旧任务无表达式快照。后两组不能据此断言最终费用正确，未执行任何资金修改。
- 本机全量任务中 `BillingContext.TieredSnapshot.TaskUsageBilling=true` 为 0，未发现本次防护针对的
  原生旧错误上下文；这不证明远程生产无此类任务。

验证命令与结果：

| 检查 | 结果 |
| --- | --- |
| `go test ./model ./middleware ./controller ./service ./relay/... ./setting/billing_setting ./pkg/billingexpr` | 通过；沙箱不允许本机监听端口，获准在可监听的测试环境重跑后全通过 |
| `go vet ./model ./middleware ./controller ./service ./relay/... ./setting/billing_setting` | 通过 |
| `cd web && bun run test src/features/system-settings/models src/features/pricing/lib --maxWorkers=2` | 11 个文件、84 个测试通过；首次与后端并发运行时旧切换测试超时，限制并发后通过 |
| `cd web && bun run typecheck` | 通过 |
| `cd web && bun run build` | 通过，含 API 文档、构建产物及品牌校验 |
| `task docs:check`、`task ai:check` | 通过 |
| 修改文件定向 lint | 旧 `tiered-pricing-editor.tsx` 有基线 11 个 error、1 个 warning；与 HEAD 相同规则和消息逐项比较，无新增诊断，其余修改文件通过 |
| `bun run i18n:sync` | 通过；七种语言累计各新增三条文案，无其它词条改写 |

### 4.3 剩余验证与上线边界

代码修复没有替代远程生产核对与发布。上线前需确认运行版本，读取原生 TaskUsageBilling 任务的冻结合同、
状态和资金，单独核对有错误 AsyncBilling 的异常清单；先核实实际用量和已发生资金，再制定幂等恢复动作。
已有 settled 状态不能直接重开；历史缺用量也不能按当前价格猜测或自动退款。

真实 Provider 请求、真实账单、生产灰度、MySQL/PostgreSQL 环境尚未验证；本次 Go 测试使用隔离 SQLite
及模拟 Provider，不把它写成三数据库或生产已验收。未修改 `relaykit/`，未实施任何渠道或素材协议迁移。
长期事实保持引用现有架构；后续按文档治理规则收敛，本开发记录不充当已发布能力证明。

### 4.4 评审发现的补充修复

评审发现首轮“模式切换不丢公式”的结论只覆盖原生用量编辑器，未覆盖 Seedance 恢复使用的旧编辑器。
补充测试先确认截图公式切换会变成 `tier("base", p * 0 + c * 0)`，再验证修复。进一步发现可解析的
Token 主公式配合自定义请求乘数时，旧编辑器首次打开就会清空乘数；已一起修复并覆盖首次提交和切换。

`UsedUsageKeys` 只统计字面量参数，不能用于禁止全部 `u()`；真实保存接口的新增测试复现
`u(param("_task.input_mode"))` 空值分支通过校验且落库，改用已有 `UsedVars` 后阻止该写入，
未改动原生插件的用量函数或校验器。

跨类型同名测试覆盖双方创建顺序、启用与停用、插入失败事务回滚，以及改成独立名称后正常保存。
存量冲突通过直接数据库 fixture 构造，实际价格 API 标记冲突；11 类选项的保存接口均拒绝写入，
原选项保持不变。前端冲突面板不生成可提交草稿，截图价格则完成修改预算、切换模式、保存草稿、
切换模型和重开校验；合法 Token 价格的可视化往返仍正常。

补充修复后的只读全量核对：本机跨类型同名冲突 0；38 个 Seedance 价格名字无表达式替换、无保存校验
失败、无 Link 被选入原生用量编辑器，23 个启用定价模型预扣试算通过。本次没有修改运行中数据库。

## 5. 全部 Seedance 价格条目的修复后只读核对

下表来自本机只读数据库与修复后真实价格生成函数。无渠道配置的旧价格仍按原路径处理，不由模型名字
推断 Link 身份；“停用”仍可维护价格，并不表示允许发送请求。“预算”是现有配置，不是此次写入。
所有有表达式条目的保存校验通过，所有价格接口可见条目未发生表达式替换。

| 模型价格名 | Seedance 合同 | 现有预算 | 修复前误匹配插件 | 修复后原生用量编辑器 |
| --- | --- | ---: | --- | --- |
| `Seedance2.0` | 无配置 | 未配置 | 否 | 否 |
| `doubao-seedance-2-0-260128` | 停用 | 250000 | 是 | 否 |
| `doubao-seedance-2-0-260128-synlink` | 停用 | 250000 | 否 | 否 |
| `doubao-seedance-2-0-fast-260128` | 停用 | 325000 | 是 | 否 |
| `doubao-seedance-2-0-fast-260128-synlink` | 停用 | 325000 | 否 | 否 |
| `doubao-seedance-2-0-mini-260615` | 停用 | 350000 | 是 | 否 |
| `doubao-seedance-2-0-mini-260615-synlink` | 停用 | 350000 | 否 | 否 |
| `doubao-seedance-2-5-260628` | 停用 | 650000 | 是 | 否 |
| `doubao-seedance-2-5-260628-synlink` | 停用 | 650000 | 否 | 否 |
| `doubao-seedance-2-5-m` | 启用 | 648000 | 是 | 否 |
| `dreamina-seedance-2-0-260128` | 无配置 | 520000 | 否 | 否 |
| `seedance-2-0` | 启用 | 500000 | 是 | 否 |
| `seedance-2-0-fast` | 启用 | 348000 | 是 | 否 |
| `seedance-2-0-fast-m` | 启用 | 350000 | 是 | 否 |
| `seedance-2-0-m` | 启用 | 500000 | 是 | 否 |
| `seedance-2-0-mini` | 启用 | 348000 | 是 | 否 |
| `seedance-2-0-mini-m` | 启用 | 324000 | 是 | 否 |
| `seedance-2-0-oversea` | 无配置 | 300000 | 否 | 否 |
| `seedance-2-0-oversea-key` | 无配置 | 520000 | 否 | 否 |
| `seedance-2-0-t` | 启用 | 520000 | 是 | 否 |
| `seedance-2-5` | 启用 | 2100000 | 是 | 否 |
| `seedance-2-5-f` | 启用 | 860000 | 否 | 否 |
| `seedance-2-5-m` | 无配置 | 648000 | 否 | 否 |
| `seedance-2-f` | 启用 | 920000 | 否 | 否 |
| `seedance-2-fast-f` | 启用 | 420000 | 否 | 否 |
| `seedance-2-mini-f` | 启用 | 420000 | 否 | 否 |
| `seedance-2.0-1080p` | 启用 | 524000 | 否 | 否 |
| `seedance-2.0-4k` | 启用 | 2100000 | 否 | 否 |
| `seedance-2.0-720p` | 启用 | 524000 | 否 | 否 |
| `seedance-2.0-fast` | 无配置 | 未配置 | 否 | 否 |
| `seedance-2.0-fast-720p` | 启用 | 524000 | 否 | 否 |
| `seedance-2.0-mini-720p` | 启用 | 328000 | 否 | 否 |
| `seedance-2.0-pro-pi-720p` | 启用 | 524000 | 否 | 否 |
| `seedance-2.0-sd2-720p` | 启用 | 524000 | 否 | 否 |
| `seedance-2.0-value-1080p` | 启用 | 725000 | 否 | 否 |
| `seedance-2.0-value-4k` | 启用 | 725000 | 否 | 否 |
| `seedance-2.0-value-720p` | 启用 | 348000 | 否 | 否 |
| `seedance-byteplus` | 无配置 | 520000 | 否 | 否 |
