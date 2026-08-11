---
status: completed
owner: Dev Team
last-reviewed: 2026-08-11
---

# Seedance 专用渠道与 Link 简化实施计划

> P0—P6 应用实现和 P8 文档收口已经完成；当前架构见
> [Seedance 专用渠道与 Link 架构](../20-architecture/Seedance专用渠道与Link架构.md)。P7 的真实
> Provider、账单、外部数据库和灰度事项已转入[路线图](路线图.md#seedance-生产验收)。本文只保留
> 原实施分解，等待用户明确授权后归档，不再作为当前架构或执行入口。

## 1. 计划定位

本计划把已接受的 [ADR-0016：Seedance 专用渠道与确定性素材代理](../20-architecture/decisions/0016-Seedance专用渠道与确定性素材代理.md)
拆成可实施、可验证、可迁移的工程阶段。架构边界、产品行为和验收口径仍分别以 `20-architecture/`、
`10-product/` 为准；本文只回答实施顺序、改动范围、迁移方法和阶段闸门。

本次不直接在旧 `DoubaoVideo` 渠道上继续叠加 Link 逻辑，而是新增独立的
`ChannelTypeSeedanceLink`。新增能力通过专属文件实现，NEWAPI 原生文件只保留必要接线；旧 Link 合同
机制不做兼容，迁移直接使用方后立即删除无用代码、接口与配置。

## 2. 交付目标

实施完成后应得到以下结果：

1. 所有 Seedance 官方和第三方线路由 `ChannelTypeSeedanceLink` 管理，北向统一使用 ModelArk V3
   create/get/list/delete 四组接口。
2. `/v1/video/generations` 和 `ChannelTypeDoubaoVideo` 保持 NEWAPI 原生语义，不承担 Seedance Link
   合同。
3. 一个已启用 Seedance 客户模型只对应一个 Channel；运行时不使用 Priority、Weight、Affinity、随机
   分发、失败重选或跨渠道 fallback。
4. 南向差异由代码化 `video_upstream_protocol` / `asset_upstream_protocol` adapter 处理，管理员只选择
   协议并配置普通 Channel 字段。
5. Link 主链收敛为：

   ```text
   customer model -> Group / price -> Seedance Channel 模型登记 -> unique Channel
     -> model_mapping -> code-backed upstream_protocol -> Provider model
   ```

6. 视频创建只发送一次 Provider POST，继续保留 durable create attempt、资金 hold、可信 task ID、Task
   冻结快照和 `unknown` 语义。
7. 素材收敛为平台 Asset/AssetGroup 与 Provider 资源一对一绑定；国内、海外、第三方按 Channel 和账号
   严格隔离。
8. 删除整体 Link 的 publication、SKU、capability、implementation、execution binding、接入方案和候选
   交集依赖；图片任务迁移到简化链路，但不继承 Seedance 的单渠道规则。

## 3. 明确不做

- 不让管理员编写协议 JSON、字段映射或状态脚本。
- 不根据模型名、价格、普通 Ability 或请求参数自动推断 Link/Seedance 身份。
- 不允许同一个 Seedance 客户模型通过 Priority/Weight 连接多个渠道。
- 不增加请求时重复配置检查、数据库唯一约束、启动扫描、自动修复或并发补偿。
- 不增加视频自动重发、换渠道、fallback 或跨 Provider 幂等合同。
- 不自建真人认证表单、人脸/活体采集、法律授权域或平台 H5 包装。
- 不增加素材自动迁移、物化、source fallback、孤儿扫描或低概率异常核查后台。
- 不把现有 `DoubaoVideo` 渠道原地改型，也不把多个旧线路映射成同一个新客户模型名。
- 不借本次改造重构无关 NEWAPI 原生路由、渠道分配或通用 Task 代码。
- 不为旧 Link publication、SKU、implementation、binding、Access Plan、素材 binding 或独立授权域
  增加兼容分支、历史 decoder、双写、双读或过渡 API。

## 4. 当前实施状态

P0—P6 的应用代码改造已完成：硬约束已切换到 ADR-0016，ModelArk V3 使用 Seedance 确定性路由，
旧 publication/SKU/implementation/binding/独立真人授权代码与兼容读取已删除，后台改为代码化协议配置。
当前仍未完成的是 P7 的生产数据物理清理、真实 Provider、实际账单、三种外部数据库和灰度启用验收；
这些外部步骤完成前不得把相关协议标记为生产已发布。

当前 fail-closed 边界是：

- Seedance 身份由代码中的 ChannelType 和协议注册表明确表达；
- 管理配置在保存/启用时保证一个客户模型对应一个 Seedance Channel；
- 请求结构、租户、资产作用域、Task 状态和计费事实继续严格校验；
- 不再用系统内 SKU 等价证明代替线下技术审核。

## 5. 总体实施顺序

```text
P0 统一硬约束与盘点
  ↓
P1 建立 Seedance ChannelType、协议注册与后台配置
  ↓
P2 接通 ModelArk V3 北向和确定性选渠
  ↓
P3 完成视频南向 adapter、Task 冻结与计费
  ↓
P4 重建 Asset / AssetGroup 确定性代理
  ↓
P5 迁移图片及其他 Link 使用方
  ↓
P6 直接删除旧合同代码、接口、字段与后台配置
  ↓
P7 数据迁移、真实 Provider 验证和分批启用
  ↓
P8 无用引用审计与文档收口
```

各阶段只能在前一阶段验收通过后进入。新 Channel 在 P7 前保持禁用；不通过兼容层、运行时双写、
双路由或 fallback 做灰度。

## 6. 分阶段实施

### P0：统一规则、建立改造清单

#### 工作项

- 更新 `docs/00-context/硬约束.md` 和根 `AGENTS.md`：用 ADR-0016 的简化合同替换旧 Link publication、
  SKU、implementation、execution binding、AssetBinding 和真人授权硬约束。
- 检查并更新 `docs/00-context/`、`docs/10-product/`、`docs/20-architecture/`、`docs/40-operations/` 中仍把
  旧链路写成当前目标的文档；不得把未完成代码写成生产事实。
- 建立一次性工程盘点，列出：
  - 旧 Link 表、字段、Router、middleware、controller、service、前端入口；
  - 使用 publication/SKU/implementation 的视频、图片、Kling、Jimeng 及其他任务链；
  - 已有 `DoubaoVideo` 渠道、Ability、客户模型和活动 Task；
  - 当前火山、BytePlus、飞彩、FunCloud、TokenSave/Moxing 的实际协议和凭据形状。
- 为数据库迁移准备只读统计命令或临时工程脚本。脚本不进入产品运行时，也不形成新的管理员核查系统。
- 确认国内火山官方素材 API 的真实合同；在官方合同、鉴权和状态语义未验证前，不得用 BytePlus 实现
  仅替换 Host/Region 后冒充支持。

#### 阶段闸门

- 编码规则与 ADR-0016 不再冲突。
- 所有旧 Link 使用方都有明确的“迁移、保留原生或删除”归属。
- 数据盘点能区分需要保留的共享业务事实与可直接删除的旧 Link 合同数据；旧合同数据不形成兼容读路径。

### P1：新增 Seedance 专用渠道与代码协议注册

#### 工作项

- 在不重排既有编号的前提下，为 `ChannelTypeSeedanceLink` 分配下一个稳定 ChannelType 值，并补齐
  Channel 名称、默认 BaseURL、模型列表、后台元数据和必要的能力声明。
- 新增 Seedance 专属代码目录，优先将新增实现隔离在类似以下位置：

  ```text
  relay/channel/task/seedance/
  middleware/seedance_*.go
  service/seedance_*.go
  ```

  允许复制少量 Doubao/Task 公共逻辑，以减少对 NEWAPI 原生文件的侵入；现有热路径只保留必要接线。
- 定义代码枚举和注册表：
  - `video_upstream_protocol`：官方 ModelArk V3、BytePlus、飞彩、FunCloud、TokenSave/Moxing 等；
  - `asset_upstream_protocol`：`none`、国内官方、海外官方和已验证的第三方素材协议。
- 协议注册项至少声明适用 ChannelType、配置校验器、adapter factory、状态映射和可选素材能力。不得让
  Channel 配置创建未注册协议。
- 后台保留 NEWAPI 普通字段和高级 `param_override` / `header_override`；新增协议选择与协议需要的条件
  字段。管理员不接触协议转换 JSON。
- Seedance 渠道页面禁用或明确标为“不参与 Seedance 路由”的 Priority、Weight；提示使用普通人可以
  理解的文案，例如“一个 Seedance 模型只能绑定一个渠道，系统不会在失败后切换渠道”。
- 在 Channel 创建、保存已启用配置、编辑已启用配置和重新启用时校验：同一客户模型不能同时出现在
  两个已启用 Seedance Channel 中。禁用渠道允许保留重复配置。
- 校验放在管理写入路径；不增加请求时校验、数据库唯一索引、启动扫描或自动修复。

#### 阶段闸门

- 管理员可以创建但暂不启用 Seedance 专用渠道，并能清楚区分视频协议和素材协议。
- 未注册协议、缺少协议必填字段、重复启用客户模型会在保存/启用时返回可理解错误。
- `DoubaoVideo` 的路由、模型和后台配置行为不发生变化。

### P2：接通 ModelArk V3 北向与确定性选渠

#### 工作项

- 保留并规范 ModelArk V3 四组北向行为：
  - `POST /api/v3/contents/generations/tasks`
  - `GET /api/v3/contents/generations/tasks/{task_id}`
  - `GET /api/v3/contents/generations/tasks`
  - `DELETE /api/v3/contents/generations/tasks/{task_id}`
- ModelArk V3 创建入口只接受统一官方请求结构，执行鉴权、Group、价格、租户、请求字段和资产
  作用域校验。
- 以 Seedance Channel 保存的客户模型登记取得唯一渠道；不写入 NEWAPI 原生 Ability 或通用分发缓存，
  不调用 Priority/Weight 分配，不创建候选列表，不发生随机选择、Affinity、重选或 fallback。模型发现
  与价格接口读取只读投影。
- 运行时不新增重复配置检查或修复分支；非标准数据库写入绕过管理校验形成的非法状态由管理员和技术
  人员处理，不把极端情况扩张为热路径复杂度。
- 删除该入口的 publication、SKU capability、implementation、execution binding、candidate intersection
  和 Link Access Plan 门禁。
- 删除 ModelArk V3 的平台自定义创建幂等合同；保留内部 durable create attempt。图片等其他已定义幂等
  产品不受此项影响。
- 将当前模型 capability 接口改为纯可用模型/协议信息，或在没有明确客户价值时移除；不得继续借它
  生成 Link SKU 履约资格。
- 明确错误语义，包括请求结构错误、模型不可用、渠道禁用、协议未注册、素材渠道不匹配和素材作用域
  冲突；不把这些错误降级到 `/v1/video/generations`。

#### 阶段闸门

- 同一请求只能解析到一个 Seedance Channel 和一个代码 adapter。
- 路由测试能证明 Priority、Weight、Affinity 和其他 Seedance Channel 不参与请求。
- `/api/v3` 不再依赖旧 Link 合同对象；`/v1/video/generations` 回归测试保持通过。

### P3：视频南向 adapter、Task 冻结和计费

#### 工作项

- 为每个已确认协议建立独立 adapter。兼容协议可复用同一 adapter；不兼容时新增代码 adapter，不把
  差异下放给管理员。
- adapter 负责：请求转换、鉴权、Provider URL、Provider 模型、创建响应、查询/列表/删除状态转换、
  Provider 错误归一和素材引用格式。
- 创建流程保持：

  ```text
  validate -> resolve exact Channel/protocol -> freeze request facts
    -> transaction(create attempt + hold + sending)
    -> exactly one Provider POST
    -> credible Provider task ID
    -> transaction(create Task + transfer hold + complete attempt)
  ```

- 未取得可信 Provider task ID 时不得创建 Task。发送结果不明记录视频 `unknown`，不得自动重发、换渠道、
  退款或把客户 quota 当成 Provider 成本。
- 新 Task 冻结客户模型、Channel ID、Provider 模型、`video_upstream_protocol`、adapter 版本、连接/账号、
  Region/Project、素材引用和计费表达式版本；后续查询、删除、结算使用冻结事实，不重新选渠。
- 不为旧 Link Task 增加兼容 decoder、双读或 fallback。迁移窗口先闭合必须保留的在途事实，随后直接
  删除旧合同字段与读取路径；无法迁移的历史任务由技术人员离线处理，不进入新 Seedance 路由。
- 计费继续复用 NEWAPI Group、价格、quota、日志和表达式底座。验证预扣、结算、差额、退款和 Provider
  exposure 幂等；所有计费乘数继续遵守额度饱和与审计规则。
- DELETE 只作用于 Task 冻结的原 Channel/adapter。失败不切换 Provider，也不推断 Provider 已删除。

#### 阶段闸门

- 每个创建请求最多产生一次 Provider POST；测试覆盖成功、明确失败和结果不明。
- 价格或 Channel 配置在创建后变化，不影响历史 Task 的查询、删除和结算。
- 国内官方、海外官方和至少一个第三方协议的 DTO/状态/错误测试通过。
- 对每个会产生费用的分支，能从日志追溯客户扣费、Provider exposure 和冻结快照。

### P4：重建 Asset / AssetGroup 确定性代理

#### 工作项

- 保留 `/v1/assets`，新增 `/v1/asset-groups` create/list/get/delete；平台身份统一使用 `ast_*`、
  `astgrp_*` 和 `asset://ast_*`。
- 一个 Asset/AssetGroup 只对应一个 Provider 资源，并冻结：

  ```text
  user_id + app_id + customer_model + channel_id
  + provider_account + region/project + asset_upstream_protocol
  + provider_resource_id + status
  ```

- 平台不保存媒体二进制，source URL 只存在于当前请求内存，从不写入数据库；创建失败只保留安全错误
  摘要和技术日志。
- Resolver 是 `asset://` 到 Provider 引用的唯一转换点。发送前校验租户、app、状态、Channel、账号、
  Region/Project 和协议；不创建通用 0..N binding。
- ModelArk V3 允许 `asset://` 与 HTTP/Data URL 混用。平台素材与请求 Channel 不一致返回
  `asset_channel_mismatch`；多个素材的账号、Region 或 Project 不一致返回 `asset_scope_conflict`。
- 国内与海外素材完全隔离。控制台既有 Provider 素材只允许管理员显式导入并分配，不向普通客户提供
  账号级枚举或裸 Provider ID。
- 真人认证作为 AssetGroup 上游流程：返回 Provider 官方/第三方认证链接或二维码，按需查询状态。不得
  自建人脸表单、H5、reservation、撤回授权域或收集额外生物信息。
- 素材创建未取得可信 Provider ID 即返回失败；删除结果不明确即返回失败并保留原状态，后续 GET 明确
  不存在后才标记 deleted。只写正常系统/使用日志，不新增 `create_unknown`、`delete_unknown`、自动重试、
  管理员核查 UI 或孤儿扫描。
- 管理员选择的每个客户模型只支持三种素材形态之一：官方素材协议、第三方素材协议、无素材库。不得
  在一个模型上运行时切换素材协议。

#### 阶段闸门

- 普通客户看不到 Provider 账号、AK/SK、Region/Project 选择、裸 Provider ID 或完整签名 URL。
- 国内、海外、第三方素材各自通过独立 adapter 验证；未验证的协议保持不可选/不可启用。
- 跨租户、跨 app、跨 Channel、跨账号和跨 Region/Project 的素材引用全部被拒绝。
- 平台日志不包含密钥、完整 source URL、生物信息或原始 Provider 响应。

### P5：迁移图片及其他 Link 使用方

#### 工作项

- 逐项处理仍依赖 publication/SKU/implementation 的图片、Kling、Jimeng 和其他类型化任务入口：
  - 保留其原有北向 DTO 和代码 adapter；
  - 改用客户模型、Channel、`model_mapping`、代码 converter/protocol 和共享 Task/计费底座；
  - 删除 Link 合同推断和旧门禁；
  - 不把 Seedance 的单渠道、不切换规则自动施加给图片或其他产品。
- 图片继续使用既有 `media-image-task/v1` 生命周期和已定义的图片幂等行为，仅删除旧 Link 身份证明依赖。
- 迁移仍需保留的 Task、计费和资源业务事实，使其不再读取 `LinkModelPublication`、Link SKU
  capability、`LinkImplementation`、execution binding、Link Access Plan、AssetBinding 或独立真人授权。
- 不再使用的旧合同记录随 P6 的代码/结构删除，不建立只读兼容数据模型。

#### 阶段闸门

- 全仓运行路径中已没有新写入旧 Link 合同表的调用。
- 所有原有类型化任务入口都有明确替代链路，并通过各自回归测试。
- Seedance 专属规则没有泄漏到 NEWAPI 原生视频、图片或其他任务产品。

### P6：直接删除旧合同代码、接口、字段和后台配置

#### 工作项

- 直接删除旧 Link publication、SKU capability、implementation、execution binding、candidate
  intersection 相关 Router、middleware、controller、service、模型写接口和缓存。
- 从 `/api/v3` 创建链移除 `ResolveLinkModelPublication`、`ResolveVideoSKUCapability`、旧素材约束和通用
  `Distribute()` 接线，替换为 P2 的 Seedance 确定性解析。
- 删除后台 Link Access Plan、implementation 字段、publication 冲突/改绑对话框、SKU capability 管理和
  相关 API 类型。
- 删除客户素材迁移/binding API、管理员 service rule、独立真人授权 API 和验证 H5 路由；补齐
  AssetGroup 管理界面和客户可理解的错误提示。
- 所有新增或修改的前端文案通过项目 i18n 流程同步到 en、zh、zh-TW、fr、ja、ru、vi。
- 删除死代码前使用引用搜索和针对性测试证明无活动调用；避免对无关 NEWAPI 文件做顺手重构。
- 删除旧 Task/attempt/asset 中只服务旧 Link 合同的字段和 decoder；不得保留 deprecated alias、空壳
  handler、旧请求兼容或新旧字段回退。

#### 阶段闸门

- 前后端不再向管理员展示 publication、SKU、implementation、binding 或 Link Access Plan 概念。
- `rg` 和依赖分析不再发现旧 Link 合同类型、字段、路由或前端配置引用。
- 后台能完成 Seedance Channel 创建、协议选择、唯一性校验、启用、禁用和素材能力查看。

### P7：数据迁移、真实 Provider 验证与启用

#### 数据迁移

- 数据库迁移保留共享 Task、计费和审计业务事实；只服务旧 Link 合同且新链路不再读取的表/列直接删除，
  不为其建立应用层兼容读取。
- 新增迁移必须同时支持 SQLite、MySQL 和 PostgreSQL，并用 GORM/项目公共锁与列名约定实现。
- 历史 Task 只保留完成查询、删除、结算所需的真实业务事实；旧 publication/SKU/implementation/binding
  字段不构成保留理由。不能在无旧合同字段的情况下继续履约的活动 Task 必须在发布前处理完毕。
- 已有 `DoubaoVideo` 渠道：不原地改型。为每条 Seedance 线路新建独立 Seedance Channel 和独立客户
  模型，完成验证后再由管理员禁用旧入口。
- 历史素材不转换旧 0..N binding 或独立真人授权记录；需要继续使用的素材由用户/管理员按新的一对一
  合同重新创建，旧合同数据在切换时删除。
- 一次性迁移报告只服务发布决策，不演化成常驻核查、修复或对账产品。

#### 真实验证

- 按[路线图的 Seedance 生产验收](路线图.md#seedance-生产验收)至少完成：
  - 国内火山官方：视频四组行为、素材、审核/真人认证、计费；
  - 海外 BytePlus：视频四组行为、素材、审核/真人认证、计费；
  - 一个第三方线路：视频四组行为及其声明支持的素材能力。
- 验证请求转换、Provider task ID、状态映射、删除语义、超时/结果不明、错误脱敏、实际账单和限流。
- 未完成真实 Provider、真实账单和外部环境验证的协议不得标记生产可用。

#### 启用顺序

1. 部署代码和数据库增量，所有新 Seedance Channel 保持禁用。
2. 管理员在测试 Group 下创建独立客户模型和 Channel，完成保存时校验。
3. 使用真实 Provider 做黑盒验收和账单核对。
4. 启用单个客户模型；观察创建、查询、删除、素材、Task 和账单日志。
5. 逐线路启用，任何线路失败只禁用该 Channel，不自动切到另一条线路。
6. 确认无旧 Link 新写入后，关闭旧 Link 管理入口。

#### 回滚原则

- 回滚以“禁用新 Seedance Channel”实现，不修改已创建 Task 的冻结事实。
- 不自动恢复同名旧模型，不把失败请求转发到 `DoubaoVideo` 或其他 Seedance Provider。
- 已创建 Task 继续通过其冻结 adapter 查询、删除和结算；代码回滚只能回到支持新快照的版本，不为旧
  Link 合同补建兼容读取。

### P8：无用引用审计与文档收口

#### 工作项

- 对全仓做引用审计，确认旧表、旧列、旧 decoder、旧 API、旧 UI 和迁移脚本均已删除；不把清理推迟
  到不确定的后续版本。
- 数据库结构删除分别验证 SQLite、MySQL、PostgreSQL；如生产发布需分步执行，可拆分数据库 DDL，
  但应用代码不得保留旧合同兼容路径。
- 更新 OpenAPI、内置 API 文档、运维手册、渠道配置说明和验收标准；普通管理员文档只说明“模型、渠道、
  视频协议、素材协议”，不重新引入内部合同术语。
- 将已完成实施事实收敛到 `00-context/`—`40-operations/`；临时过程记录按文档治理规则处理，未经授权
  不修改 `60/70/99`。

#### 阶段闸门

- 旧 Link 运行时代码和无用数据库结构已清理；保留的审计事实不依赖旧合同代码解释。
- 当前事实文档、API 文档、后台 UI 和生产行为一致。
- 不存在两套可创建 Seedance Link 合同的公共状态或入口。

## 7. 关键代码改动边界

| 区域 | 主要动作 | 边界 |
| --- | --- | --- |
| `constant/`、Channel 元数据 | 新增稳定 ChannelType 和协议字段 | 不重排已有编号 |
| `router/video-router.go` | ModelArk V3 改接 Seedance 专属中间件 | 保留 `/v1/video/generations` 原生链路 |
| `middleware/` | 新增保存/启用校验和确定性解析；删除旧门禁 | 不改通用 Distribute 语义来迁就 Seedance |
| `relay/channel/task/seedance/` | 放置代码 adapter 和状态转换 | 允许局部重复，减少侵入旧 Doubao 文件 |
| `service/`、`model/` | durable attempt、Task 快照、资产一对一和旧合同删除 | 三数据库兼容，主库为权威 |
| `router/asset-router.go` | 收敛 Asset，新增 AssetGroup，删除旧授权/迁移入口 | 不暴露 Provider 账号和裸 ID |
| `web/` | 协议配置、唯一性错误、Seedance 路由提示、AssetGroup | 保留高级 override，不要求管理员写映射 |
| OpenAPI/内置文档 | 四组 ModelArk V3、Asset/AssetGroup 和错误码 | 不把上游私有字段承诺为公共合同 |

每次修改 NEWAPI 原生文件都要在变更说明中记录：为什么新增文件无法完成、最小接线点是什么、未来接取
上游时会产生什么冲突。能够放在 Seedance 专属新增文件中的逻辑不得扩写原生文件。

## 8. 测试与验证清单

### 后端行为

- Channel 保存/启用：重复客户模型、禁用重复、协议缺失、协议字段缺失。
- 路由：一个模型只到一个 Channel，Priority/Weight/Affinity 不生效，无失败换渠。
- ModelArk V3：create/get/list/delete 的请求、响应、分页、状态和错误。
- Task：发送前 attempt/hold、单次 Provider POST、可信 task ID、`unknown`、冻结快照、删除和结算。
- 素材：租户/app 隔离、Channel/账号/Region/Project 校验、混合 URL/Data URL、失败清理和错误脱敏。
- 真人 AssetGroup：上游链接/二维码透传安全、状态查询、无平台人脸数据保存。
- 图片及其他任务：移除旧门禁后的原有客户语义回归。
- 计费：验证 validation → estimate → quota → pre-consume → settle/refund 全链路及饱和审计。

### 数据与删除

- SQLite、MySQL、PostgreSQL 的结构迁移、回滚前置检查和共享业务事实读取。
- 新旧活动 Task 均只依赖简化后的必要冻结字段；旧合同字段没有 decoder 或 fallback。
- 旧素材 binding、独立授权和异常核查数据不迁移，按新合同重新创建。
- 旧 Link 合同表、字段和代码不再存在。

### 前端与文档

- 使用 Bun 运行相关单测、类型检查、构建和 i18n 同步。
- 后台验证 Seedance Channel、协议条件字段、普通人提示、启用/禁用和 AssetGroup。
- 文档改动运行：

  ```bash
  task docs:check
  task ai:check
  ```

- OpenAPI 或公共 API 文档变更额外运行：

  ```bash
  cd web && bun run docs:validate
  ```

- 真实 Provider 黑盒验证不能由 mock 代替；真实账单未核对的协议不得进入生产可用列表。

## 9. 建议的交付拆分

为降低评审和回滚风险，建议按以下变更集交付，不把全部改造塞进一个提交：

1. **规则与骨架**：硬约束更新、ChannelType、协议注册、后台基础字段。
2. **视频主链**：ModelArk V3 确定性解析、官方 adapter、Task/计费冻结。
3. **第三方协议**：逐个 adapter 和各自真实验证，不做“大一统转换器”。
4. **素材主链**：Asset/AssetGroup、一对一作用域、官方/第三方素材 adapter。
5. **Link 使用方迁移**：图片及其他类型化入口停止旧合同写入。
6. **旧体系退役**：后端门禁、管理 API、前端 Link 配置和死代码删除。
7. **数据与发布**：三数据库迁移、真实 Provider 启用。
8. **删除审计**：确认旧表/列、decoder、API、UI 和运行时引用已经全部移除。

每个变更集必须能独立说明用户可观察行为、数据库影响、验证结果和回滚方式。未完成前置闸门时，不得
通过临时 fallback、额外配置开关或双写扩张实现范围。

## 10. 完成定义

只有同时满足以下条件，才可认定本计划完成：

- `ChannelTypeSeedanceLink` 成为所有 Seedance 线路唯一业务渠道类型，且管理员能直接理解配置。
- ModelArk V3 四组行为不再依赖 Link publication/SKU/implementation/binding，也不走 Priority/Weight
  分配或失败切换。
- Seedance 不进入原生 Ability/分发池，`/v1/video/generations` 的数据库与内存缓存负向测试均通过。
- `DoubaoVideo` 与 `/v1/video/generations` 保持原生独立，没有被新合同收紧或包装。
- 视频单次发送、durable attempt、资金 hold、Task 冻结、`unknown` 和计费对账均有可观察测试证据。
- Asset/AssetGroup 与 Provider 资源一对一，国内/海外/第三方隔离，真人认证只代理上游入口。
- 图片及其他 Link 使用方已迁移，旧 Link 合同代码、接口、配置和无用数据结构已经直接删除。
- 三种数据库、前端、文档检查和规定的真实 Provider/账单验证全部通过。
- 当前事实文档、后台界面、公开 API 和生产行为一致，不存在第二套 Link 合同解释。
