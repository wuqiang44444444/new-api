---
status: current
owner: Dev Team
last-reviewed: 2026-08-16
---

# Seedance 无状态素材代理与素材引用设计

## 1. 边界

中转站用户是一个自行管理最终用户和素材记录的受信应用。素材 API 是按客户模型选路的无状态单资源
代理，不是平台素材目录。平台不保存媒体二进制，也不持久化 Asset/AssetGroup 映射。

客户模型名是唯一公开路由身份：

```text
customer model -> unique Seedance Channel -> model_mapping -> code adapter -> Provider
```

Channel、Provider、上游原始模型名、Region/Project 和凭据均为内部事实，不进入素材成功响应、错误响应
或模型元数据。模型元数据只发布匿名 `reuse_scope`：相同非空 scope 表示当前配置落在同一素材域，可由
调用方尝试复用；实际存在性、权限与兼容性仍由 Provider 自然决定。平台不增加共享素材组或兼容白名单。

## 2. 北向合同

只开放单资源操作：

| 操作 | 接口 | model 位置 |
| --- | --- | --- |
| 创建素材 | `POST /v1/assets` | JSON body |
| 查询素材 | `GET /v1/assets/{asset_id}?model=...` | query |
| 更新素材 | `PATCH /v1/assets/{asset_id}` | JSON body |
| 删除素材 | `DELETE /v1/assets/{asset_id}?model=...` | query |
| 创建素材组/认证会话 | `POST /v1/asset-groups` | JSON body |
| 查询素材组/认证会话 | `GET /v1/asset-groups/{id}?model=...` | query |
| 删除素材组 | `DELETE /v1/asset-groups/{group_id}?model=...` | query |

不注册 `GET /v1/assets` 和 `GET /v1/asset-groups`。内部 adapter 可以因第三方只提供分页查询而调用上游
List，但不能把它发布为客户目录。

素材响应最小字段为：

```json
{
  "object": "asset",
  "id": "provider-opaque-resource-id",
  "model": "customer-model-name",
  "reference": "asset://provider-opaque-reference-id",
  "status": "ready"
}
```

`id` 用于后续 CRUD；`reference` 用于视频生成，两者由 adapter 分别归一，允许不相同。素材尚未产生
可用引用时省略 `reference`，调用方按 `model + id` 查询后保存最新响应。

真人认证创建返回 `object=asset_group_verification`，`id` 是上游会话 ID；完成后单项查询返回
`group_id`。调用方使用实际 `group_id` 创建真人素材。查询会话时显式传
`verification_session=true`，避免由 opaque ID 猜测资源类型。

## 3. 视频引用

ModelArk V3 接受并转发：

```text
asset://<opaque-upstream-asset-id>
```

平台只做请求结构和非空字符串校验，不执行素材语义：不查数据库、不查 Provider、不验证所有权、
`user_id + app_id`、ready 状态、创建模型、ChannelID、账号作用域或模型兼容性。Provider 在实际生成
请求中最终判断存在性、权限、内容审核和模型支持。

旧 `asset://ast_*`、`asset://pubref_*` 不再是平台命名空间，也没有兼容 resolver；它们只会作为普通
opaque 字符串发送，是否有效由当前 Provider 判断。

## 4. 错误与安全

- 未配置素材协议或 adapter 未实现的操作返回 `422 unsupported_asset_operation`。
- 上游明确不存在返回 `404 asset_not_found`；其它业务拒绝返回脱敏 `asset_upstream_error`。
- 错误 ID 只发送到当前客户模型选定的 Provider；不得探测来源、切换 Channel 或 fallback。
- Token 的模型白名单同时覆盖 body model 与 GET/DELETE query model。
- 不记录 source URL、签名 URL、凭据或原始 Provider 响应。
- 共享 Provider 账号时，本合同不提供“知道其它应用素材 ID 仍不可访问”的技术强隔离。需要该保证时，
  使用独立 Provider 账号/Project，或另行决策有状态所有权层。

## 5. 模型元数据

模型发现中的 `api.assets` 是客户端唯一能力依据：

- `supported`：该客户模型是否配置已验证素材协议；
- `management_mode=caller_managed_stateless`；
- `requires_model=true`；
- `reference_format=asset://{opaque_upstream_asset_id}`；
- `reuse_scope`：匿名素材复用域，不支持素材时省略；
- `operations`：逐项列出实际支持，列表固定为 `supported=false`；
- `media`：逐 kind/media 描述素材组为 required、optional 或 unsupported；
- `creation`：发布已验证的 URL TTL、长度、MIME、大小和重定向限制。

不得从模型名、同一供应商或共享账号推断素材能力；客户模型后缀只能作为部署方展示约定。当前支持矩阵见
[Seedance 模型素材库支持矩阵](Seedance模型素材库支持矩阵.md)。

## 6. 数据与迁移

新安装不创建 `assets` / `asset_groups` 表。升级不自动删除既有表或数据，避免破坏性迁移；运行时也不
读取它们。旧 `ast_*` / `astgrp_*` 是硬切换前的历史合同，不双读、不做别名。需要提取历史 Provider ID
时由管理员在升级前离线处理。

本设计由 [ADR-0017](decisions/0017-调用方自管无状态素材代理.md) 决定。
