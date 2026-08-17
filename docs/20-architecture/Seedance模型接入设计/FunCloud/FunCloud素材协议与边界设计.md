---
status: current
owner: Dev Team
last-reviewed: 2026-08-17
---

# FunCloud 素材协议与边界设计

## 1. 范围

本文定义 `funcloud_material` 的无状态素材控制面、安全回源和 Provider 能力边界。客户模型、视频路径和
价格见[FunCloud 全模型与素材计费设计](FunCloud全模型与素材计费设计.md)，通用合同见
[Seedance 官方素材库与素材引用设计](../../Seedance官方素材库与素材引用设计.md)。

## 2. 北向与能力

FunCloud 复用 `/v1/assets` 与 `/v1/asset-groups` 的带模型单资源接口，不发布素材或素材组列表，也不增加
Provider 品牌路径。

`funcloud_material` 只能与 `funcloud_seedance` 配对，并且只允许 Standard、Fast、Mini。2.5 必须选择
`none`；真人素材、单素材更新和删除没有发布合同。

| 北向操作 | FunCloud 南向 | 当前状态 |
| --- | --- | --- |
| 创建普通素材组 | `POST /api/v2/open/material/group/create` | 支持 |
| 查询素材组 | `GET /api/v2/open/material/group/list` | 按调用方 opaque group ID 唯一匹配 |
| 删除素材组 | `POST /api/v2/open/material/group/delete` | Provider 明确 `materialCount=0` 才发送 |
| 创建素材 | `POST /api/v2/open/material/virtual/upload` | 支持 HTTPS 安全回源与流式 multipart |
| 查询素材 | `GET /api/v2/open/material/list` | 按调用方 opaque material ID 唯一匹配 |
| 更新/删除单素材 | 无已验证映射 | `unsupported_asset_operation` |

Provider list 只用于实现已知 ID 的单项查询和空组判断，不形成客户列表接口。

## 3. opaque ID 与视频引用

创建和查询直接返回 Provider opaque `groupId` / `materialId`；素材可用时返回 Provider 给出的
`asset://<opaque-id>`。调用方保存 `model + id + reference`。平台不建立 Asset/AssetGroup 表、所有权、
Channel、账号或 Project 映射，也不把 ID 包装为 `ast_*`。

视频中的 `asset://<opaque-id>` 原样进入 FunCloud 请求。adapter 只要求 `asset://` 后非空，不解释 ID
字符、来源、状态或所属模型；Provider 最终判断存在性、权限、审核与兼容性。失败不探测或切换其它
Provider。

## 4. 安全回源

创建素材只接受满足渠道最短 TTL 的公网 HTTPS/443 URL。当次请求内：

1. 限制 URL 长度、逐跳重定向并在拨号期复检公网 IP；
2. 校验响应 MIME 与声明媒体类型一致；
3. 使用 `io.Pipe` 流式写入 multipart，不落盘或缓存媒体二进制；
4. 按实际读取限制 100MB，Provider 响应限制 1MB；
5. 请求结束关闭 source，不持久化、记录或返回 source URL。

## 5. 查询、删除与错误

group/material list 只接受代码登记的信封和唯一命中。重复 ID、多个集合字段、未命中、分页超界或非法
Provider 引用均失败关闭，不通过名称或 URL 模糊猜测。

删除组只依赖 Provider 明确的 `materialCount=0`；平台没有本地素材表可作为第二事实源。Provider 未明确
确认时返回脱敏错误，不建立本地 `delete_unknown`、自动重试或孤儿扫描。

## 6. 不变量

1. Standard/Fast/Mini 可使用 `funcloud_material`；2.5、真人素材和未验证操作明确不支持。
2. 客户只使用 opaque ID；不提供列表、`ast_*`、binding、resolver 或 Provider 探测。
3. source URL 只参与当次安全回源，平台不保存媒体二进制。
4. 视频素材兼容性由 FunCloud 判定，平台不验证模型、Channel 或账号作用域。
5. Provider 失败使用脱敏统一错误，不自动重试、迁移或 fallback。

## 7. 代码事实

| 事实 | 代码位置 |
| --- | --- |
| 素材协议与 profile | `relaykit/dto/upstream_protocol.go` |
| FunCloud group/material adapter | `relay/channel/task/seedance/assets/funcloud.go` |
| HTTPS 安全回源 | `service/asset_funcloud_stream.go` |
| 无状态素材服务 | `service/asset_service.go` |
| 北向路由 | `router/asset-router.go`、`controller/asset.go` |
