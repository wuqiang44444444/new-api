---
status: current
owner: Dev Team
last-reviewed: 2026-08-26
---

# Seedance 无状态素材代理架构

本文只描述素材代理的内部边界、路由责任和安全不变量，不重复列出每个客户模型的公开能力矩阵。
对外调用合同、操作支持和 `api.assets` 字段以
[Seedance 模型素材库支持矩阵](Seedance模型素材库支持矩阵.md) 为唯一入口。

## 1. 架构边界

中转站用户是自行管理最终用户和素材记录的受信应用。素材 API 是按客户模型选路的无状态单资源代理，
不是平台素材目录。平台不保存媒体二进制，也不持久化 Asset/AssetGroup 映射、所有权、状态或 Provider
作用域事实。

```text
customer model -> unique Seedance Channel -> model_mapping -> code adapter -> Provider
```

客户模型名是唯一公开路由身份。Channel、Provider、上游原始模型名、Region/Project 和凭据只属于内部
执行事实，不进入素材成功响应、错误响应或模型元数据。模型元数据只发布匿名 `reuse_scope`；它不能证明
存在性、权限、所有权或模型兼容性。

## 2. 代理责任与不变量

平台负责：

- 校验客户模型、Token 模型白名单和请求结构；
- 根据客户模型选择唯一 Seedance Channel；
- 调用代码注册的 `asset_upstream_protocol` adapter；
- 脱敏 Provider 错误并保持明确的 HTTP/错误码语义；
- 在视频请求中把非空 `asset://<opaque-id>` 原样交给冻结的 video adapter。

平台不负责：

- 建立 Asset/AssetGroup 数据表、resolver、列表索引或跨 Provider 探测；
- 认证素材所有权、应用归属、ready 状态、创建模型、Channel、账号或 Region/Project 兼容性；
- 在 Provider 失败时切换 Channel、fallback、迁移素材或自动物化；
- 保存 source URL、签名 URL、媒体二进制、凭据或原始 Provider 响应。

不支持的模型或操作必须由代码 adapter 和已验证 Provider 合同明确返回
`unsupported_asset_operation`，不能伪装成上游故障，也不能尝试其它 Provider。

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

## 4. 路由与凭据边界

素材协议由代码注册表决定，管理员只能选择已注册协议并配置其所需字段。平台不根据模型名、域名、Key
或 Provider 名称动态认证兼容性，也不允许管理员编写协议 JSON、字段映射或状态脚本。

素材调用的 Base URL、协议、账号、Region 和 Project 决定实际 Provider 作用域。一个启用素材协议的
Channel 是一个素材租户边界：identity 建立后，Channel Type、Base URL、视频/素材协议、Project 和
Region 在更新事务中不可变，修改这些字段必须新建 Channel；Key/AK/SK 轮换必须由管理员显式确认
“素材租户未变化”并通过后端校验。identity 与匿名 `reuse_scope` 的生成机制由
[Seedance 专用渠道与 Link 架构](Seedance专用渠道与Link架构.md) §5.3 负责。平台不解释 opaque ID；
旧 opaque ID 是否可见由 Provider 决定。

错误的 Provider ID 只发送到当前客户模型选定的 Provider；不得探测其来源、换渠道或重试其它 Provider。

## 5. 视频引用与任务隔离

ModelArk V3 接受 HTTP/HTTPS URL、Data URL 和 `asset://<opaque-id>`。素材代理只负责单请求结构和非空
校验；不把引用写入平台素材表。视频创建后，Task 和 create attempt 冻结客户模型、Channel、Provider
模型、协议/adapter 版本、连接和请求中的 opaque 引用；后续查询、删除、内容回源和结算不重新解释素材
或重新选渠。

旧 `asset://ast_*`、`asset://pubref_*` 不是平台命名空间，也没有兼容 resolver；它们只能作为普通 opaque
字符串发送，是否有效由当前 Provider 判断。

## 6. 迁移与数据兼容

新安装不创建 `assets` / `asset_groups` 表。升级不自动删除既有表或数据，运行时也不读取它们，避免破坏性
迁移。旧 `ast_*` / `astgrp_*` 是硬切换前的历史合同，不双读、不做别名；需要提取历史 Provider ID 时，
由管理员在升级前离线处理。

## 7. 相关文档

- [Seedance 模型素材库支持矩阵](Seedance模型素材库支持矩阵.md)：对外素材能力、操作和错误合同；
- [Seedance 专用渠道与 Link 架构](Seedance专用渠道与Link架构.md)：客户模型、Channel、协议和 Task 总体关系；
- [异步任务与计费事实架构](账单计费-异步任务与计费事实架构.md)：create attempt、Task、资金和结算事实；
- [ADR-0017：调用方自管无状态素材代理](decisions/0017-调用方自管无状态素材代理.md)：无状态素材边界的长期决策。
