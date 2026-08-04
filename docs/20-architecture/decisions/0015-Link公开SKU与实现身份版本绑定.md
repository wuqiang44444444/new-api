---
adr: 0015
status: accepted
date: 2026-08-03
superseded-by: ""
---

# ADR-0015: Link 公开 SKU 与实现身份版本绑定

## Context

本决策统一 Link 合同术语、公开 SKU capability、候选实现等价、Provider 实现身份，以及最小
`AssetSource`、`upstream_binding` / `source_url` 双模式 Resolver。此前分散在多个实施阶段文档中的
这些内容不再构成独立决策。

此前公开 SKU 能力、Provider 实现身份和渠道配置没有完全正交：公开视频能力由
`VideoSKUCapability` 表达，内部能力按 `(channel_type, profile)` 查找，但不同 Provider 可以共享
相同 type/profile，profile 只能说明协议形状，不能证明渠道选择了哪份经批准的 Link 实现。图片侧
又只有 converter 中的模型字符串和值域分支，缺少可枚举、可版本化的 Link SKU 能力注册。

同时，实施方案没有定义实现版本升级时已有渠道、binding 和在途任务如何保持身份稳定。若渠道只
保存实现 ID，而代码原地修改该 ID 对应的版本或能力，管理员未重新确认也会被动切换实现语义，
削弱显式注册、故障对账和 fail-closed 保证。

## Decision

1. **Link 身份只来自显式代码注册。** Link 从上到下分为客户 API 合同、公开 SKU capability、
   Provider 实现声明、渠道实例和任务执行快照。Ability、模型发现、渠道配置、URL、profile 或
   运行时探测都不能创建或反向推断 Link 身份。
2. **一个 Link 公开 SKU 对应一份公开客户合同和能力声明。** 同一 SKU 可以由一个或多个
   Provider 实现服务，但所有实现必须完整覆盖同一公开 capability。`contract_id`、SKU capability
   version 和 capability hash 是客户语义权威；Provider、渠道类型、profile、converter 和解析模式
   不进入公开 capability hash。无法证明公开语义等价时必须排除实现或拆分 SKU。
3. **Provider 实现使用独立的代码注册身份。** 每个实现以
   `link_implementation_id + link_implementation_version` 唯一标识，并声明允许的公开 SKU、渠道
   类型、profile、converter、适配器版本、Link 资源解析模式、素材约束、任务和计费链路。一个实现
   可以覆盖多个公开 SKU；一个公开 SKU 也可以列出多个已验证等价实现。
4. **渠道必须显式绑定精确实现版本。** 渠道设置同时保存 `link_implementation_id` 和
   `link_implementation_version`。只配置 route、模型名、Ability、Base URL、profile 或前端模板不
   产生 Link 实现身份。保存、Ability 发布、运行时过滤和发送前复检都解析同一注册项并校验精确
   ID/version；缺失、未知、退役或版本不匹配均 fail closed。
5. **注册版本不可原地改义。** 实现 ID/version 对应声明是不可变事实。改变公开 SKU 集合、
   必需 profile/converter、解析模式、素材限制、任务或计费接线时必须提升实现版本；破坏性变化应
   使用新的实现 ID。注册表为声明计算内部 content hash，并以回归测试防止同一 ID/version 静默
   改义。
6. **profile 只描述协议适配形状。** Link 内部的 `supports_managed_assets`、
   `asset_resolution_modes`、`asset_source_min_ttl_seconds` 及素材类型/数量约束由实现注册项声明；
   channel/profile 可以作为该声明的适配条件，但不能单独授予 Provider 身份或 Link 资格。非 Link
   原生渠道继续使用既有 profile 和 Ability 逻辑，不受本决策收紧。
7. **binding、任务和风险事实冻结实现身份。** `AssetBinding`、credential fingerprint、Task、
   create attempt、Provider exposure 和管理员审计至少携带实现 ID/version；任务和 binding 另冻结
   content hash。冻结用于防止同一次当前版本部署中的渠道配置漂移；版本不匹配时 fail closed，
   不建立旧版本 Resolver、双读、alias 或 fallback。
8. **Link 资源 Resolver 可服务图片和视频，但公开能力分别注册。** 视频继续使用
   `VideoSKUCapability`；显式 Link 图片 SKU 使用最小 `ImageSKUCapability`，只表达公开字段值域、
   `supports_link_assets` 和相关组合限制。图片继续复用 `/v1/images/generations`、现有 DTO、Task 和
   计费链，不新增 Provider 专属公开 API。图片 Resolver 和真实 Provider 未完成验收前，对应公开
   SKU 的 `supports_link_assets` 必须保持 false。
9. **exposure 不自动生成宽松默认。** 注册实现后，管理员按实现 ID/version 显式配置有效 exposure
   策略，之后才能启用渠道。允许先保存禁用渠道进行连接测试；启用保存、Ability 发布和运行时均
   在策略缺失、失效或预算耗尽时 fail closed。
10. **最小 AssetSource 与双模式解析。** `AssetSource` 只保存 Asset 作用域认证加密 URL
   和客户声明的 `expires_at`；不增加 URL HMAC、独立状态机、密钥注册表或原地刷新。Resolver 仍
   优先 active binding，再按实现声明选择 `source_url`，并在选渠和发送前复检所有权、授权、作用域
   和 TTL。换源继续创建新 `ast_*`。
11. **一次性 URL 继续走请求级媒体路径。** 仅为当前任务服务的短时 URL 不包装为长期可复用 Link
    资源；同一请求是否允许混用请求级媒体与 `asset://ast_*` 由公开 SKU capability 声明，未声明
    时 fail closed。
12. **当前开发期实现只保留单一版本。** 版本升级采用一次性硬切换；删除旧注册、专属 adapter、
    parser、fixture 和测试，不保留多版本运行时、旧任务 fallback 或兼容状态机。首次正式发布后
    如需跨版本无损升级，必须另行作出架构决策。
13. **旧未发布实现的硬删除必须经过部署核查。** 每个实际部署环境在删除旧路由、模型和 attempt
    前核查该入口未发布、无在途 attempt 且相关表无业务记录。若存在记录，先制定显式清理或迁移
    方案；不得把“本地未发布”当作所有环境均无数据的证明。
14. **NEWAPI 原生合同不属于 Link。** 原生 Router、DTO、Relay、Adapter、模型发现和客户端协议
    以上游代码为唯一权威。本决策不能授权增加原生专属 Link SKU 拒绝中间件、`client_protocol`、
    兼容读取、fallback 或历史协议推断；隔离必须在 Link 自身注册和候选实现选择中完成。

## Consequences

- 收益：客户合同、Provider 实现和渠道实例形成三个正交身份；相同 type/profile 的不同 Provider
  不会互相冒充。
- 收益：同一公开 SKU 可以在多个已验证等价实现间路由，不需要为纯 Provider 差异复制客户合同；
  实现不等价时仍通过拆分 SKU 保持公开语义稳定。
- 收益：渠道、binding、Task、计费和 exposure 可按精确实现版本追溯，代码升级不会静默改写历史。
- 收益：图片和视频共享 Link 资源治理边界，同时保留各自公开 DTO、能力声明和任务投影。
- 代价：渠道设置、binding、Task/create attempt 和 exposure 增加实现身份字段，并需要三数据库迁移
  与历史渠道显式登记。
- 代价：每次实现能力变化都需要版本升级、注册一致性测试和管理员重新确认，发布流程更严格。
- 风险：若把 implementation ID 加入公开 capability hash，会错误地把等价 Provider 拆成不同客户
  合同；实现时必须维持公开能力与内部身份分离。
- 后续约束：source/binding、TTL 和安全边界是本决策的一部分；不得因实现注册改造恢复 multipart、
  原地 source refresh、Provider 专属客户资源类型或历史版本兼容路径。

## Alternatives Considered

- **一个 Provider 一个公开 SKU**：身份最简单，但会把内部供应商差异暴露为客户产品，并导致等价
  Provider 复制价格、文档和合同，未采用；仅在公开能力不等价时拆分 SKU。
- **继续按 `(channel_type, profile)` 表达实现**：改动最小，但相同协议形状的不同 Provider 无法
  区分，不能满足渠道显式登记和审计，未采用。
- **只保存 implementation ID，不保存版本**：字段更少，但代码原地升级会使既有渠道隐式切换，
  无法证明管理员选择的精确实现，未采用。
- **把 implementation ID 加入公开 capability hash**：可以强制一实现一 SKU，但混淆客户能力与
  Provider 身份，也阻止多个等价实现服务同一 SKU，未采用。
- **另建 Moxing 图片 API、DTO 或 Task**：能够隔离 Provider，但会形成第二套公开图片产品并增加
  上游同步冲突，未采用。
