---
status: accepted
owner: Dev Team
last-reviewed: 2026-08-10
---

# Link 扩展概念与协作关系

## 1. 目的

本文定义简化后的 Link 概念、事实所有权和人机分工。该设计已经接受但尚待完整实施；旧代码中的
publication、Link SKU、capability、`LinkImplementation`、execution binding 和 Link Access Plan
均是待移除机制，不再构成目标架构。

## 2. Link 的含义

Link 是项目新增产品能力的集合，不是一套独立的模型身份认证系统。它由以下部分组成：

```text
Link
├── 客户入口与统一请求结构
├── NEWAPI 的 Token / Group / Ability / Channel / price
├── 代码化 Provider 协议 adapter
├── durable Task 与计费事实
└── 平台素材代理
```

客户模型、Provider 模型和 Channel 是三个直接概念：

| 概念 | 用途 | 谁维护 |
| --- | --- | --- |
| 客户模型 | 模型发现、Token 权限、价格、Ability、日志和请求 | 管理员 |
| Channel | 凭据、Base URL、协议、模型列表、账号作用域 | 管理员 |
| Provider 模型 | 上游实际模型名 | `model_mapping` |

Link 不再在三者之间插入 Link SKU、publication 或 implementation 身份。

## 3. 人与系统的职责

技术人员负责线下确认上游是否兼容某个已有协议。管理员根据确认结果配置客户模型、Channel、
`model_mapping` 和代码中已有的 `upstream_protocol`。如果不兼容，技术人员新增 adapter 后再交付
管理员配置。

系统负责：

- 校验公开入口的统一请求结构和计费安全上界；
- 执行 Token、Group、Ability 和价格规则；
- 根据确定 Channel 应用 `model_mapping` 与代码 adapter；
- 保护 Task、资金、素材所有权和敏感信息。

系统不负责：

- 根据模型名称、价格、profile 或 Provider 名称自动认证兼容性；
- 让管理员编写 JSON 字段映射或状态机；
- 为每个理论异常建立运行时修复、启动扫描或后台核查机制。

## 4. Seedance 的业务分类

`ChannelTypeSeedanceLink` 表示“这个渠道销售 Seedance 模型”，而不是“这个上游一定使用 ModelArk
V3”。所有官方和第三方 Seedance 线路都属于该类型；技术差异由 `video_upstream_protocol` 与
`asset_upstream_protocol` 表达。

不同 Seedance 渠道必须使用不同客户模型名。一个客户模型只对应一个已启用的 Seedance 渠道：

```text
客户模型
  -> Group / Ability / price
  -> 唯一 Seedance Channel
  -> model_mapping
  -> upstream_protocol
  -> Provider 模型
```

因此 Seedance 不使用 Priority、Weight、Affinity、随机分配、失败重选或 fallback。Group 只控制
客户访问，Ability 只登记模型与 Channel 的关系。

## 5. 上游协议

`upstream_protocol` 是代码 adapter 的稳定选择值，不是管理员可编程配置。一个 adapter 负责：

- 鉴权、路径和请求转换；
- Provider 模型字段；
- 创建、查询、列表或删除能力；
- 状态、错误和结果归一；
- 协议需要的素材处理。

管理员仍可使用 NEWAPI 已有 `param_override` 与 `header_override` 高级配置，但这些字段不能创建新的
协议、Link 身份或自动兼容性保证。

## 6. 素材概念

平台素材是 Provider 可信资源控制面的代理，不是 URL 缓存：

| 平台对象 | 客户身份 | 上游关系 |
| --- | --- | --- |
| AssetGroup | `astgrp_*` | 一个固定渠道/账号下的一个 Provider Group 或认证邀请 |
| Asset | `ast_*` | 一个固定渠道/账号下的一个 Provider Asset |
| 视频引用 | `asset://ast_*` | 发送前改写为该 Provider 的资源引用 |

Asset/AssetGroup 固定到 `user_id + app_id`、Channel、账号和协议需要的 Region/Project，不建立 0..N
binding、自动迁移或 source fallback。创建时的客户模型只是找到唯一渠道的路由提示；同一渠道和账号
作用域下的其他兼容模型可以复用素材。

## 7. 已发生事实

Channel、Ability 和价格描述下一次请求。Task、create attempt、Asset、AssetGroup 和计费快照描述已经
发生的事实。视频创建后必须冻结：

- 客户模型、渠道与 Provider 模型；
- `upstream_protocol` 与 adapter 版本；
- 查询连接和 Provider task ID；
- 使用的素材与 Provider 作用域；
- 预扣、价格和结算事实。

后续查询、删除、内容获取和结算不得重新选渠或按当前配置改写历史。

## 8. 架构不变量

1. Channel、Ability、价格和模型映射是路由配置，不创建隐藏合同身份。
2. 技术兼容性由线下评审和代码 adapter 保证，不由运行时推断系统保证。
3. 管理员不编辑协议 JSON。
4. 一个 Seedance 客户模型只对应一个已启用渠道。
5. Seedance 请求不使用 Priority、Weight、Affinity、重试、切换或 fallback。
6. Task 与素材保存已发生事实，不反向决定未来渠道配置。
7. NEWAPI 原生能力保持原生行为，Link 不据模型名接管原生入口。

## 9. 相关文档

- [Link 服务合同注册与履约架构](Link服务合同注册与履约架构.md)
- [Seedance 统一北向合同架构](Seedance统一北向合同架构.md)
- [异步任务与计费事实架构](异步任务与计费事实架构.md)
- [Link 资源合同与解析架构](Link资源合同与解析架构.md)
- [ADR-0016](decisions/0016-Seedance专用渠道与确定性素材代理.md)
