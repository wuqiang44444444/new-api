---
adr: 0005
status: accepted
date: 2026-07-23
superseded-by: ""
---

# ADR-0005: 官方 Action 素材合同与 Bearer 兼容边界

## Context

[ADR-0004](./0004-中转型素材代理与上游绑定.md) 要求未取得可验证合同的 Provider 素材 profile 保持关闭，且不得猜测官方 AK/SK 接口。现已依据 BytePlus ModelArk Action 合同实现素材、素材组和真人认证能力，但原有 `ark_assets` 仍是 Bearer REST 兼容线，两者的鉴权、能力完整度和兼容声明不能混为一谈。

平台还必须维持网关边界：普通客户只使用平台 API Key，不接触 AK/SK、`BytedToken`、上游 GroupId 或素材组 CRUD；真人验证凭证必须短期保存并在终态、过期或撤销后清除。

## Decision

- 新增并使用供应商名称中立的 `official_action_assets` profile 表达已验证的 AK/SK Action 合同；固定 `ServiceName=ark`、`Version=2024-01-01`，并要求显式配置 HTTPS BaseURL、Region 和 ProviderProject。
- `official_action_assets` 使用独立于 `Channel.Key` 的 AK/SK 配置并在 Provider 请求时签名；`Channel.Key` 始终保留为视频 Bearer API Key。独立凭据存入一对一表 `channel_asset_credentials`，普通管理响应只返回配置状态与 Access Key ID 脱敏提示。
- 官方素材账号指纹使用 `official_action_assets/v2 + Action host + AK + SK + profile + Project + Region`；视频 Base URL 和视频 API Key 不进入该素材指纹。它支持 `Create/List/Get/Update/DeleteAsset`、`Create/List/Get/Update/DeleteAssetGroup`、`CreateVisualValidateSession` 和 `GetVisualValidateResult`。生产启用前必须以真实账号通过只读 `ListAssets` 测试并确认账号已开通相应能力。
- 原 `ark_assets` 继续作为既有 Bearer REST 兼容 profile，不宣称 ModelArk 官方 Action 兼容。已确认的 TokenSave 合同允许它以 `POST /v1/ark/assets/list` 执行只读连通性测试；未确认的素材组删除、对账等端点仍不得猜测。需要完整素材生命周期和对账的渠道必须选择 `official_action_assets`。
- `relay_assets` 可依据已确认的海外素材合同以 `POST /assets/list` 执行只读连通性测试。TokenSave `/v1/asset/*` 是 JoyCreator management-only facade，不复用现有 `joycreator_assets` 的 `/joycreator/openApi/v1/asset/*` 直连合同；如需接入必须新增独立 profile，不能覆盖参与视频路由的 `relay_assets`。
- 客户端继续只提供平台 `/v1/assets` 与 `/v1/real-person-authorizations`；不新增普通客户可见的素材组 API。平台自动管理素材组，并分别对素材和素材组建立 Provider 账号范围内的唯一所有权认领。
- `BytedToken` 只以短期密文和不可逆查找摘要保存；成功、明确失败、过期或撤销均在同一状态事务中清除 handle、H5 URL 密文和 token hash。

## Amendment — 2026-07-28

以下实现事实补充并修正原 Decision：

- 官方 Action host 不再由管理员显式填写，也不复用视频 `Channel.BaseURL`；系统根据 Region 固定推导 `https://ark.{region}.byteplusapi.com`。原 Decision 第一条中的“显式配置 HTTPS BaseURL”由本条取代；
- `official_action_assets` 必须与 `official` 视频 profile 配对，Region 必须符合 Provider region 格式，Project 与 Region 共同进入资源作用域；
- 素材 AK/SK 成对写入 `channel_asset_credentials`，管理 API 只返回配置状态和脱敏 Access Key ID；保留、轮换和清除具有显式语义，并与渠道身份校验在同一事务内完成；
- v2 指纹固定为 Action host、AK、SK、profile、Project、Region 的组合。存量旧指纹必须使用 `cmd/migrate-official-asset-credential` 先 dry-run、再只读验证、最后单事务迁移；启动守卫拒绝旧指纹、孤儿 claim 或不一致账号；
- 系统周期性列举官方素材与素材组，将上游孤儿和本地缺失记录为 `asset_reconciliation_findings`。对账只报告并审计差异，不自动认领或删除未知上游资源。

## Consequences

- 收益：官方 Action 与 Bearer 兼容线的能力声明和鉴权语义清晰，Link 合同不暴露 Provider 凭据或资源标识。
- 收益：素材与素材组均有数据库唯一约束保护，同一 Provider 账号下的上游资源不能被跨用户绑定。
- 代价：运营需要分别配置视频模型 API Key、素材 AK/SK、Region、ProviderProject，并分别执行视频和素材只读测试。
- 代价：历史 `official_action_assets` 记录使用旧指纹时必须按受控迁移流程验证同一 Provider 身份，不能静默重算。
- 代价：`ark_assets` 与 `relay_assets` 保持有限兼容能力；除已确认的只读列表合同外，其未确认端点不会为了能力对称而猜测实现。
- 风险：Provider 文档或签名规则变化会使真实合同测试失败；在重新验证前应停用受影响的 official profile。
- 后续约束：任何新增 Action、Version、签名字段或公开素材组合同都必须先取得可验证协议，并补充合同测试和本文档。

## Alternatives Considered

- 把 AK/SK Action 能力并入 `ark_assets`：未采用，因为会把官方签名合同与第三方 Bearer REST 兼容线混为一谈，造成错误的能力声明和凭据处理。
- 为能力对称猜测 Bearer profile 的素材组删除或其他管理路径：未采用，因为无法证明线级合同，且错误删除的风险不可接受。只读列表路径仅在供应商文档确认后用于连通性测试。
- 向普通客户公开上游素材组 CRUD：未采用，因为素材组是平台自动维护的 Provider 账号级实现细节，会泄露上游 ID 并扩大所有权和迁移合同。
- 把 `BytedToken` 返回客户端或通过 query 轮询：未采用，因为它是短期 Provider 凭据，不属于普通客户 API 合同。
