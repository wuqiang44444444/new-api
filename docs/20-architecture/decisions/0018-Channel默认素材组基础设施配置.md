---
adr: 0018
status: accepted
date: 2026-08-28
superseded-by: ""
---

# ADR-0018: Channel 默认素材组基础设施配置

## Context

[ADR-0017](0017-调用方自管无状态素材代理.md) 确立了调用方自管 Provider opaque ID、平台不建立客户
Asset/AssetGroup 目录和所有权事实的边界。现有各素材 Provider 对普通素材组的要求不同：有的强制组 ID，
有的允许省略，有的完全不使用组字段。如果把这些差异直接发布到北向合同，调用方必须按模型或 Provider
创建素材组并改变请求，违背中转站统一调用体验。

平台需要在不恢复客户 AssetGroup 资源模型的前提下，为每个 Channel 准备一个稳定的普通 AIGC 默认组，
并让南向 adapter 吸收 Provider 的字段和必填性差异。这会新增一条 Channel 级持久化事实，因此必须明确
它与 ADR-0017 的关系、权威策略、失败语义和发布边界。

## Decision

- 统一北向合同是第一优先级。普通素材创建的 `asset_group_id` 在全部已发布素材协议上保持合法可选；客户
  不按 Provider、协议或模型改变请求结构。
- 普通素材组行为由代码注册的唯一 `GeneralAssetGroupPolicy` 决定，只允许 `none` 与
  `default_fallback`。运行时和公开元数据必须读取同一权威注册；`GroupAdapter` 只表示管理端可以创建组，
  现有 requirement 接口也不得成为第二套运行时判据。
- 当前已发布且能够创建普通 AIGC 素材组的协议统一登记为 `default_fallback`；协议不使用普通素材组时登记
  为 `none`。`real_person` 继续使用 Provider 认证产生的专用组，不进入普通默认回退。
- 每个支持普通 AIGC 素材组的 Seedance Channel 最多持久化一个内部 Provider group ID，固定名称为
  `aigctokenaigeneral`。该记录是 Channel 基础设施配置，不是客户 AssetGroup 资源；平台不建立客户列表、
  所有权、状态、迁移或 resolver。
- 素材租户原地替换会重建 Channel 的匿名 `reuse_scope`；替换保存的同一事务必须删除本地默认组记录。
  旧 Provider group ID 属于旧素材租户，不得在替换后的租户中继续发送；替换后默认组缺失按既有
  `default_asset_group_not_configured` 语义明确失败，管理员需重新创建或复用。
- 默认组只能由管理员在已保存 Channel 的管理页面手工创建或复用。业务素材请求、升级和启动过程不得自动
  创建、查询或回填；默认组缺失时返回 `default_asset_group_not_configured`。
- Service 只用裁剪结果判断调用方组 ID 是否为空。未传、空或空白时使用默认组；裁剪后非空值保持原值并
  交给 Provider。非空值违反已发布北向结构约束时返回 `invalid_request`，不得静默替换成默认组；Provider
  拒绝显式 ID 后也不得回退。
- `none` 策略忽略北向组字段并保证南向不发送。客户直接请求无法履约的素材组操作时沿用既有
  `unsupported_asset_operation`，不新增素材组专属公开错误码。
- 管理员每次执行创建或复用动作时，已登记查询能力的 adapter 必须遍历可证明完整的匹配结果集，并在
  Service 做名称完全相等过滤；查询失败、名称缺失或无法证明分页耗尽时停止。存在多个同名组时按 Provider
  返回顺序取第一个；没有完整查询能力时直接创建。
- 升级前由普通调用方创建的同名组可能被采用为默认组。管理员执行动作即表示接受第一个完全同名组，平台
  不判断其创建来源，也不建立组所有权事实。
- 发布分两批进行：先上线持久化和管理能力并由管理员配置全部存量 `default_fallback` Channel，再同时启用
  运行时默认回退与普通素材 optional 元数据。新 Channel 未配置时明确返回 409，不增加技术性启用门槛。

## Consequences

- 收益：调用方可以始终使用同一北向请求并直接创建普通素材，Provider 的组字段差异由南向 adapter 履约。
- 收益：运行时、公开元数据和测试共享一个协议策略事实源，避免管理创建能力、Provider 必填性和运行时
  fallback 相互漂移。
- 代价：主数据库新增一条 Channel 一对一基础设施配置；Channel 删除与素材租户替换路径必须同步清理本地
  记录，但不得删除 Provider 组。
- 代价：存量 Channel 必须在第二批发布前由管理员准备默认组；新 Channel 配置遗漏会产生明确 409。
- 风险：Provider 查询可能模糊或分页，只有能够证明结果集完整的 adapter 才能登记查询能力；否则直接创建
  可能产生同名重复组，这是已经接受的简化。
- 风险：同名组不代表平台创建或拥有。共享 Provider 账号下的调用方隔离仍遵守 ADR-0017 的受信应用边界。
- 后续约束：不得把该一对一配置扩展成客户 AssetGroup 表、列表、所有权、状态机、自动修复或业务请求内
  自动创建。

## Alternatives Considered

- 继续要求调用方显式创建并传入组 ID：未采用，因为它把南向 Provider 差异抬升为北向客户条件，破坏统一
  调用体验。
- 业务首次请求自动创建默认组：未采用，因为 Provider 成功而本地保存失败会产生不可判定状态，并把管理
  资源创建耦合进素材数据面。
- 用 `GroupAdapter` 或 `AssetGroupRequirementAdapter` 推断运行时行为：未采用，因为前者表达管理创建能力
  过宽，后者漏掉 adapter 内部强制组的协议，都会造成现有协议行为漂移。
- 对所有非空 ID 做跨 Provider 格式校验并在失败时回退默认组：未采用，因为 opaque ID 没有统一格式，静默
  回退可能把素材创建到错误组并返回成功。
- 让公开元数据按 Channel 是否已经配置默认组切换 required/optional：未采用，因为内部配置状态不能反向
  改变统一北向字段合同；配置缺失应作为明确的 Channel 管理错误处理。
