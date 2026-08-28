---
status: current
owner: Dev Team
last-reviewed: 2026-08-28
---

# Seedance 无状态素材代理架构

本文只描述素材代理的内部边界、路由责任和安全不变量，不重复列出每个客户模型的公开能力矩阵。
对外调用合同、操作支持和 `api.assets` 字段以
[Seedance 模型素材库支持矩阵](Seedance模型素材库支持矩阵.md) 为唯一入口。

## 1. 架构边界

中转站用户是自行管理最终用户和素材记录的受信应用。素材 API 是按客户模型选路的无状态单资源代理，
不是平台素材目录。平台不保存媒体二进制，也不持久化客户 Asset/AssetGroup 映射、所有权、状态或
Provider 作用域事实。

已实现的第一批 Channel 默认素材组管理能力不改变该边界：平台只为每个支持普通 AIGC 素材组的 Channel
保存一个内部 Provider group ID；它不是客户 AssetGroup 资源，也不形成客户列表、状态或所有权模型。
将该 ID 用作普通素材创建南向默认参数属于尚未启用的第二批运行时行为。

```text
customer model -> unique Seedance Channel -> model_mapping -> code adapter -> Provider
```

统一北向协议是客户合同的第一优先级。南向 adapter 负责吸收 Provider 是否使用素材组、字段名和必填性
差异；客户不按 Provider 或模型改变素材创建请求形状。

客户模型名是唯一公开路由身份。Channel、Provider、上游原始模型名、Region/Project 和凭据只属于内部
执行事实，不进入素材成功响应、错误响应或模型元数据。模型元数据只发布匿名 `reuse_scope`；它不能证明
存在性、权限、所有权或模型兼容性。

## 2. 代理责任与不变量

平台负责：

- 校验客户模型、Token 模型白名单和请求结构；
- 根据客户模型选择唯一 Seedance Channel；
- 按统一北向合同解析调用方组 ID 或 Channel 默认组 ID；
- 调用代码注册的 `asset_upstream_protocol` adapter；
- 脱敏 Provider 错误并保持明确的 HTTP/错误码语义；
- 在视频请求中把非空 `asset://<opaque-id>` 原样交给冻结的 video adapter。

平台不负责：

- 建立客户 Asset/AssetGroup 数据表、resolver、列表索引或跨 Provider 探测；
- 认证素材所有权、应用归属、ready 状态、创建模型、Channel、账号或 Region/Project 兼容性；
- 在 Provider 失败时切换 Channel、fallback、迁移素材或自动物化；
- 保存 source URL、签名 URL、媒体二进制、凭据或原始 Provider 响应。

不支持的模型或操作必须由代码 adapter 和已验证 Provider 合同明确返回
`unsupported_asset_operation`，不能伪装成上游故障，也不能尝试其它 Provider。

已发布北向字段在某个南向协议中不生效，不等于整个操作不支持。特别是 `asset_group_id`：不使用素材组
的 adapter 必须不发送该字段并继续创建素材；只有客户直接请求的素材组操作无法履约时才返回
既有 `unsupported_asset_operation`。

## 3. 北向单资源代理

平台只提供带客户 `model` 的单资源操作：

| 操作 | 路径 | 内部事实 |
| --- | --- | --- |
| 创建素材 | `POST /v1/assets` | source URL 仅在本次 Provider 调用中存在 |
| 查询素材 | `GET /v1/assets/{asset_id}?model=...` | `asset_id` 直接作为 Provider opaque ID |
| 更新素材 | `PATCH /v1/assets/{asset_id}` | adapter 决定是否支持 |
| 删除素材 | `DELETE /v1/assets/{asset_id}?model=...` | 只发送到该模型选定的 Provider |
| 创建素材组/认证会话 | `POST /v1/asset-groups` | 真人认证属于 Provider 素材组流程 |
| 查询素材组/认证会话 | `GET /v1/asset-groups/{id}?model=...` | 认证会话使用显式查询参数区分 |

不注册 `GET /v1/assets`、`GET /v1/asset-groups` 或 `DELETE /v1/asset-groups/{group_id}`。当前全部
素材协议 adapter 均不实现素材组删除，公开模型元数据也不登记该操作；单素材
`DELETE /v1/assets/{asset_id}` 按各自协议的已验证合同保留。素材组可能承载多人素材并发生级联删除，
平台不主动清理；确需清理时由 Provider 管理员确认归属和级联影响后执行。第三方 adapter 如需分页
List，只能在南向调用内部使用，不得将结果发布为客户目录。

Provider 返回的 `id` 和视频 `reference` 均由调用方保存，二者可以不同。平台可以返回
`asset://<opaque-id>`，但不把该引用转换成本地资源 ID，也不验证其来源。视频 adapter 最终将存在性、
权限、内容审核和模型兼容性交给 Provider 裁决。

### 3.1 普通 AIGC 默认素材组

当前代码已实现本节的管理控制面和持久化：Channel 一对一表是默认组 ID 的主数据库事实源；管理员通过
渠道编辑页“连通性测试”下方的动作查询、创建或复用固定名称组；Channel 删除或确认替换素材租户时只
删除本地记录。当前素材业务请求仍不读取该表，以下组 ID 解析顺序是已接受但尚待第二批启用的运行时合同。

固定默认组名为 `aigctokenaigeneral`，并作为普通北向素材组创建的系统保留名称。管理员只能在已保存
Channel 的管理页面手工创建或复用该组；业务 `POST /v1/assets` 不得触发 Provider 组创建或查询。
已登记组查询能力的 adapter 必须返回名称并遍历可证明完整的结果集；查询失败、名称缺失或分页无法证明
耗尽时停止，不能推断为不存在。升级前历史调用方创建的同名组可能被采用，管理员执行动作即接受返回顺序
中的第一个完全同名组，平台不判断创建来源。

普通 AIGC 素材创建由代码注册的唯一 `GeneralAssetGroupPolicy` 驱动；该策略同时供运行时与公开元数据
读取，不从 `GroupAdapter`、requirement 接口或模型名推断。组 ID 解析顺序固定为：

1. 策略为 `none`：忽略北向 `asset_group_id`，adapter 不发送组字段；
2. 策略为 `default_fallback` 且调用方提供裁剪后非空 ID：保持原值并使用调用方 ID；
3. 调用方未传、传空或空白：使用当前 Channel 默认组 ID；
4. 非空值违反已发布北向结构约束：返回 `invalid_request`，不得改用默认组；
5. 默认 ID 未配置：返回 `default_asset_group_not_configured`；
6. Provider 拒绝显式调用方 ID：返回脱敏错误，不回退默认组。

默认组 ID 只由主数据库中的 Channel 一对一内部配置承载。管理状态接口只返回是否支持、是否配置和固定
名称，不向页面返回 opaque ID。停用 Channel 保留该 ID，复制 Channel 不复制，
删除 Channel 只删除本地配置且不调用 Provider 删除。`real_person` 继续使用 Provider 认证产生的专用组，
不使用普通 AIGC 默认组。

发布必须分两批：先上线持久化和管理能力并配置全部存量 `default_fallback` Channel，再同时启用运行时
默认回退和普通素材 optional 元数据。北向字段合同不随单个 Channel 的配置状态改变；配置缺失以稳定 409
暴露为 Channel 管理错误。

## 4. 路由与凭据边界

素材协议由代码注册表决定，管理员只能选择已注册协议并配置其所需字段。平台不根据模型名、域名、Key
或 Provider 名称动态认证兼容性，也不允许管理员编写协议 JSON、字段映射或状态脚本。

素材调用的 Base URL、协议、账号、Region 和 Project 决定实际 Provider 作用域。一个启用素材协议的
Channel 是一个素材租户边界：identity 建立后，Channel Type 不可修改；Base URL、视频/素材协议、
Project 或 Region 的变化必须由管理员在当前渠道编辑界面确认“替换素材租户”。后端在同一事务中保存
新边界并重建 identity（改为 `none` 时移除），未确认则返回冲突且不修改。Channel ID 和客户模型保持
不变；新请求使用新的匿名 `reuse_scope`，既有 Task 继续使用创建时快照。Key/AK/SK 单独轮换仍须确认
“素材租户未变化”。identity 与匿名 scope 机制由
[Seedance 专用渠道与 Link 架构](Seedance专用渠道与Link架构.md) §5.3 负责。平台不解释、不迁移或探测
opaque ID；旧 opaque ID 是否可见由 Provider 决定。

错误的 Provider ID 只发送到当前客户模型选定的 Provider；不得探测其来源、换渠道或重试其它 Provider。

## 5. 视频引用与任务隔离

ModelArk V3 接受 HTTP/HTTPS URL、Data URL 和 `asset://<opaque-id>`。素材代理只负责单请求结构和非空
校验；不把引用写入平台素材表。视频创建后，Task 和 create attempt 冻结客户模型、Channel、Provider
模型、协议/adapter 版本、连接和请求中的 opaque 引用；后续查询、删除、内容回源和结算不重新解释素材
或重新选渠。

旧 `asset://ast_*`、`asset://pubref_*` 不是平台命名空间，也没有兼容 resolver；它们只能作为普通 opaque
字符串发送，是否有效由当前 Provider 判断。

## 6. 迁移与数据兼容

新安装不创建客户 `assets` / `asset_groups` 表。默认素材组实现可以新增 Channel 一对一内部配置表，但
不得读取或恢复旧客户 AssetGroup 表。升级不自动删除既有表或数据，运行时也不读取它们，避免破坏性
迁移。旧 `ast_*` / `astgrp_*` 是硬切换前的历史合同，不双读、不做别名；需要提取历史 Provider ID 时，
由管理员在升级前离线处理。存量 Channel 不自动创建或回填默认组，必须由管理员逐个配置。

## 7. 相关文档

- [Seedance 模型素材库支持矩阵](Seedance模型素材库支持矩阵.md)：对外素材能力、操作和错误合同；
- [Seedance 专用渠道与 Link 架构](Seedance专用渠道与Link架构.md)：客户模型、Channel、协议和 Task 总体关系；
- [异步任务与计费事实架构](账单计费-异步任务与计费事实架构.md)：create attempt、Task、资金和结算事实；
- [ADR-0017：调用方自管无状态素材代理](decisions/0017-调用方自管无状态素材代理.md)：无状态素材边界的长期决策。
- [ADR-0018：Channel 默认素材组基础设施配置](decisions/0018-Channel默认素材组基础设施配置.md)：统一北向组字段、默认组事实源与发布边界。
