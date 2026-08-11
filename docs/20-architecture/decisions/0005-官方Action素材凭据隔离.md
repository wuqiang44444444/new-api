---
adr: 0005
status: superseded
date: 2026-07-23
last-reviewed: 2026-08-11
superseded-by: "ADR-0016"
---

# ADR-0005: 官方 Action 素材凭据隔离

> 本决策已被 [ADR-0016](0016-Seedance专用渠道与确定性素材代理.md) 取代；当前事实见
> [Seedance 专用渠道与 Link 架构](../Seedance专用渠道与Link架构.md)。

## Context

BytePlus ModelArk 素材与真人认证使用 AK/SK 签名的 Action 合同，视频模型调用使用 Bearer API
Key。两类凭据具有不同权限、轮换方式和泄露风险，不能因为归属于同一渠道而复用同一字段、账号
指纹或客户端合同。

平台是 Provider 控制面代理。普通客户只使用平台 API Key，不接触 AK/SK、`BytedToken`、上游
GroupId 或素材组 CRUD；短期真人验证凭据也不能进入普通响应或日志。

## Decision

- `official_action_assets` 是已验证的 AK/SK Action 素材协议，固定使用代码确认的
  `ServiceName=ark`、`Version=2024-01-01` 和由 Region 推导的官方 Host。
- 素材 AK/SK 成对保存在一对一 `channel_asset_credentials` 中；`Channel.Key` 始终保留为视频
  Bearer API Key。管理 API 只返回配置状态和脱敏 Access Key ID。
- `official_action_assets` 只能与已确认的官方视频 profile 配对。Region、ProviderProject、Action
  Host、AK/SK 和 profile 共同形成素材账号作用域与凭据指纹；视频 Base URL 和 Bearer Key 不进入
  素材指纹。
- 保存、轮换、清除和渠道身份校验在同一事务内完成。旧指纹、孤儿 claim 或凭据身份不一致必须
  fail closed，不通过猜测或自动重算继续调用 Provider。
- 官方素材、素材组和真人认证 Action 只有在协议、签名和真实账号能力已验证后才能登记；不得为了
  能力对称猜测删除、对账或其它管理端点。
- 客户端只使用平台 `/v1/assets` 与 `/v1/real-person-authorizations`；素材组和 Provider 资源 ID
  始终是内部实现事实。
- `BytedToken` 只以短期密文和不可逆查找摘要保存；成功、明确失败、过期或撤销时清除 handle、H5
  URL 密文和 token hash。

## Consequences

- 收益：视频模型凭据和素材管理凭据的权限、轮换、审计与泄露半径相互隔离。
- 收益：普通客户不会观察到 Provider 凭据、素材组或上游资源身份。
- 代价：运营需要分别配置和验证视频 Bearer Key 与素材 AK/SK。
- 风险：Provider 签名或 Action 合同变化会使真实合同验证失败；重新验证前应停用对应素材能力。
- 后续约束：新增 Action、Version、签名字段或公开素材组能力必须先取得可验证合同；本 ADR 不授权
  新增通用 Bearer 兼容层、供应商端点猜测或第二套客户素材 API。

## Alternatives Considered

- 把 AK/SK 存入 `Channel.Key`：未采用，会混淆两类凭据并扩大泄露与误用范围。
- 复用视频账号指纹：未采用，视频连接事实不能证明素材 Action 属于同一资源作用域。
- 向普通客户公开 Provider 素材组 CRUD：未采用，素材组属于平台自动维护的实现细节。
- 为未确认端点编写兼容调用：未采用，无法证明协议，且错误管理操作风险不可接受。
