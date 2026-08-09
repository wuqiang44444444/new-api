---
status: current
owner: Dev Team
last-reviewed: 2026-08-09
---

# Link 服务合同概念与协作关系

## 1. 目的与边界

本文定义 Link 扩展产品的统一概念、事实所有权与协作关系。它回答“客户得到什么”“管理员选择什么”
以及“系统如何证明某个 Provider 实现能够履约”，不展开单个 Provider 的字段、上线步骤或排障过程。

本文中的 **Link 服务合同**是端到端总称；仓库既有文档和代码中的“Link 合同”是其简写。NEWAPI
原生路由、模型和渠道继续遵守上游原生语义，只有经本地代码显式注册的扩展能力进入本文边界。

详细注册、任务和素材设计分别见 [Link 服务合同注册与履约架构](Link服务合同注册与履约架构.md)、
[Link 视频服务合同与异步任务架构](Link视频服务合同与异步任务架构.md) 与
[Link 资源合同与解析架构](Link资源合同与解析架构.md)。

## 2. 概念模型

```text
Link 服务合同
├── 客户接入合同（北向合同）
│   └── 客户模型发布事实（客户模型名 -> Link SKU）
├── Link SKU 能力合同
├── Link 资源合同
├── 任务与计费合同
└── 上游调用合同（南向合同）
    ├── 视频上游合同
    └── 素材上游合同
```

客户接入、SKU 能力、Link 资源、任务与计费共同定义客户可依赖的服务承诺。上游调用合同定义网关
如何借助 Provider 履行该承诺。南向实现只能完整覆盖已经发布的客户合同，不能反向扩张、缩减或
改变客户语义；同一 Link SKU 可以由多个经验证等价的实现履约。

### 2.1 Link 服务合同

Link 服务合同覆盖客户调用、能力校验、资源解析、同步或异步任务、计费以及 Provider 结果归一化。
它不是管理端可编辑对象，也不包含某条渠道的凭据、价格、权重、状态或健康度。

### 2.2 客户接入合同

客户接入合同定义路径、鉴权、请求与响应、错误外壳、幂等和客户可见的任务操作。客户使用的模型名
继续参与 NEWAPI 的模型发现、Token 权限、价格、Ability、日志和响应，可以与 Link SKU 完全不同。

代码中的 `contract_id`、`northbound_contract_id` 和 `northbound_contract_version` 继续作为协议族、
响应投影和版本键；这些稳定标识不包含 Provider 路径、Provider 模型或上游素材 ID。

### 2.3 客户模型发布事实

客户模型发布事实是系统发布 Link 客户模型时形成的持久化合同关系：

```text
(contract_namespace, route_family, customer_model)
  -> link_sku + publication_version
```

当前默认命名空间为 `link`。当前代码注册的路由族为：

| 路由族 | 稳定入口边界 |
| --- | --- |
| `image_generation` | Link 图片生成入口 |
| `modelark_video` | ModelArk/Seedance 类型化视频入口 |
| `kling_video` | Kling 类型化视频入口 |
| `jimeng_video` | 即梦类型化视频入口 |

`group` 负责访问、倍率和渠道路由，不进入默认合同身份键。同名客户模型可以在不同路由族发布不同
SKU，但同一命名空间、路由族和客户模型只能有一个当前发布 SKU。

发布事实独立于 Channel、Ability、Redis 和当前候选集合。渠道禁用、删除最后一条 Ability、服务
重启或缓存重建只影响“当前能否履约”，不会把“已经发布”改成“从未发布”。普通保存和启用只能
创建或核对发布事实；改变 SKU 必须执行带预期版本、操作人和原因的显式改绑，并追加不可变审计。

### 2.4 Link SKU 能力合同

Link SKU 是代码注册的稳定能力身份，定义字段、类型、值域、默认值、媒体组合、Link 资源支持、
任务生命周期和计费维度。它不等于客户模型名，也不等于 Provider 模型名。这里的“公开”表示该
能力可以作为客户承诺发布，不要求客户调用时直接传入 SKU 字符串。

一个 SKU 只有一份客户可见能力合同。Provider 实现必须完整覆盖该能力，不能在运行时取交集后静默
缩小；无法证明等价时必须排除实现或拆分 SKU。

### 2.5 Link 资源合同

Link 资源合同定义 `ast_*` / `asset://ast_*` 的平台身份、所有权、应用隔离、真人授权、绑定、解析和
生命周期。客户不感知 Provider 上游素材 ID，也不需要知道本次执行使用 `upstream_binding` 还是
受保护 `source_url`。

Provider 是否有独立素材 API 属于素材上游合同。`asset_upstream_profile=none` 只表示没有独立
Provider 素材生命周期；实现若支持 `source_url`，仍可履行支持 Link 资源的 SKU。

### 2.6 任务与计费合同

任务与计费合同定义创建、查询、取消、终态、预扣、结算、退费和对账。客户价格、倍率、权限和消费
日志继续使用客户模型名；Link SKU 用于能力等价和内部审计，Provider 成本或最终选中的渠道不能在
运行时改变客户价格语义。

### 2.7 上游调用合同

上游调用合同定义 Provider 鉴权、路径、请求转换、Provider 模型、响应与错误归一、任务状态以及
profile、converter 和 adapter 版本。视频上游合同负责创建、查询、取消和结果生命周期；素材上游
合同负责 binding/source URL、凭据作用域、最小 TTL 和媒体类型。

“北向/南向”只表示调用方向：北向合同面向客户，南向合同面向 Provider；两者仍属于同一个 Link
服务合同，不形成两套可独立发布的产品身份。

## 3. 实现与配置概念

### 3.1 Link 实现

`LinkImplementation` 是代码注册的 Provider 实现身份，以
`link_implementation_id + version + content_hash` 标识。它声明 Provider、渠道类型、公开 SKU、
视频与素材 profile、解析模式、路径、adapter、任务/计费接线和 execution bindings。

Link 实现是执行与审计事实，不是客户模型，也不能由 Provider 名称、Base URL、模型相似度或渠道
配置临时推断。

### 3.2 Link 接入方案

Link 接入方案是一个精确 `LinkImplementation` 的管理端投影。管理员选择一次后，Provider、视频和
素材上游合同、解析模式、路径与 adapter 由注册事实自动带出并锁定。接入方案不建立平行持久化对象，
也不承担客户合同身份。

### 3.3 渠道实例

渠道实例保存可变的运营与连接配置：凭据、Base URL、Project/Region、客户模型列表、
`model_mapping`、价格、分组、优先级、权重、状态和并发。渠道必须显式选择接入方案，但不能重新
定义 Link SKU、视频上游合同或素材上游合同。

### 3.4 实现执行绑定

实现执行绑定是代码注册的多维执行证明：

```text
(implementation ID/version, route_family, action, profile, provider_model)
  -> Link SKU
```

Provider 模型只是判别维度之一。所有决定 SKU 的执行维度必须在发布时固定并唯一解析；缺失、重复或
歧义均失败关闭。execution binding 用于发布推导和发送前复检，不是按当前在线候选动态决定客户合同
的反向发现机制。

### 3.5 三种模型身份

| 身份 | 权威用途 | 客户可见性 |
| --- | --- | --- |
| 客户模型名 | 模型发现、Token 权限、价格、Ability、日志和响应 | 可见且可自定义 |
| Link SKU | capability、资源合同、实现等价、任务与审计 | 通常作为内部合同身份 |
| Provider 模型 | `model_mapping` 的执行结果和 Provider 请求字段 | 仅管理员与内部审计可见 |

三者的稳定关系是：

```text
发布时：
  客户模型名
    -> 既有 model_mapping
    -> Provider 模型
    -> 所选 implementation 的 execution binding
    -> Link SKU
    -> 创建或核对客户模型发布事实

请求时：
  命名空间 + 路由族 + 客户模型名
    -> 客户模型发布事实
    -> 冻结 Link SKU / publication version
    -> NEWAPI 正常选渠和 model_mapping
    -> execution binding 复检 Provider 模型仍履行同一 SKU
```

### 3.6 管理端模型命名与默认投影

管理员在渠道配置中只有两个可操作模型概念：

| 概念 | NEWAPI 字段 | 管理员决定什么 |
| --- | --- | --- |
| 客户模型名 | `channels.models` | 客户可见、可授权、可路由的模型名 |
| 真实模型名 | `model_mapping` 的最终目标 | 选中渠道在 Provider 请求中实际使用的模型名 |

Link SKU 是内部能力、等价履约、计费和审计身份，不进入管理员的模型命名工作流：不塞入 `models` 或
`model_mapping`，不要求管理员理解 SKU 字符串，也不作为必填、必选或必读信息。模型映射沿用 NEWAPI 原生
方向——客户模型名经 `model_mapping` 得到真实模型名，无 mapping 时使用身份翻译（客户模型名 == 真实模型名），
Link 不增加第二张映射表，也不在无 mapping 时暗中改写为 Provider 模型或 SKU。

选择 Link 接入方案后，新建且 `models` 与 `model_mapping` 均为空的表单可由系统填入真实模型候选作为
可编辑默认值。真实模型候选只来自当前 implementation 的 create execution bindings 的 `provider_model`
（过滤 `action=create`、当前 video profile，且单一 route_family 时），由前端
`linkAccessPlanProviderModelDefaults` 计算，后端 `ListSelectableLinkImplementations` 只透传注册的
`execution_bindings`。`public_skus` / Link SKU、Provider 或方案展示名、`plan_name`、模型名相似度、其它
渠道的映射或当前 publication 都不得作为候选来源。无法唯一确定 route 时不填充。这只是未保存的编辑辅助，
不触发后台保存、Ability 发布或 publication 创建。

切换、选择或清除 Link 方案都不得自动创建、改写或删除 `model_mapping`，也不自动生成自映射；`models` 或
`model_mapping` 已有内容时切换方案不改写。真实模型候选可作为 `Models` 或 `Model Mapping` 的受控建议项，
但选择、改名和映射仍是管理员在 NEWAPI 原有字段中的显式操作。

## 4. 事实所有权

| 事实 | 权威来源 | 不负责 |
| --- | --- | --- |
| 客户入口和响应语义 | `contract_id`、类型化 Router/DTO | Provider 选择 |
| 客户模型发布身份 | `LinkModelPublication` | 当前渠道可用性 |
| 发布变更历史 | `LinkModelPublicationAudit` | 运行时选渠 |
| Link SKU 能力 | `VideoSKUCapability` / `ImageSKUCapability` | Provider 凭据与成本 |
| Provider 实现 | `LinkImplementation` 代码注册表 | 客户价格与模型别名 |
| 客户模型到 Provider 模型 | 渠道 `model_mapping` | Link SKU 合同权威 |
| 当前可履约候选 | Channel、Ability、分组、exposure 和 Link 资格过滤 | 已发布合同身份 |
| 已发生执行 | Task、create attempt、Asset/Binding 与 exposure 快照 | 后续配置变化 |

`LinkModelPublication` 使用唯一键原子创建或核对；当前行保存最新 SKU 与版本，
`LinkModelPublicationAudit` 对每个版本保存前后 SKU、操作人、来源渠道、原因和时间。两者通过 GORM
迁移并与主数据库一起支持 SQLite、MySQL 和 PostgreSQL。

## 5. 协作流程

### 5.1 配置与发布

```text
管理员选择 Link 接入方案
  -> 系统锁定精确 implementation
  -> 管理员填写客户模型与 model_mapping
  -> 系统沿映射链取得 Provider 模型
  -> execution binding 唯一解析 Link SKU
  -> 校验 implementation 完整覆盖 SKU 与 Link 资源能力
  -> 启用渠道时在同一事务创建/核对 publication，再发布 Ability
```

禁用渠道可以完成配置校验，但不把尚未启用的模型自动发布给客户。后续等价渠道只能加入同一 SKU 的
履约集合；不同 SKU 必须先显式改绑。管理员无需维护第二张“客户模型到 Link SKU”映射表。

同一分组和路由族中出现同名普通 NEWAPI 候选时，Link 发布和运行失败关闭；普通渠道本身的原生语义
不被 Link 收紧。

### 5.2 请求执行

```text
客户按客户接入合同发起请求
  -> 读取 publication，冻结客户模型、路由族、Link SKU 和版本
  -> 按 Link SKU 校验字段、值域、资源和计费维度
  -> 施加 implementation / Asset / exposure 候选资格约束
  -> NEWAPI 按 Ability、分组、优先级、权重和重试选渠
  -> 应用选中渠道的 model_mapping
  -> Provider POST 前复检 execution binding、Provider 模型和冻结 SKU
  -> 按客户接入、任务与计费合同归一结果
```

Link 不自动寻找 Provider、不选择 Standard/Value/Fast 档位，也不复制 NEWAPI 的分发算法。零合格候选
表示“合同已发布但暂不可履约”，不是“模型从未发布”，更不能降级到普通候选。

### 5.3 素材协作

Link SKU 先决定是否允许 `asset://` 及类型、数量、角色和组合；选中实现的素材上游合同再决定解析为
`upstream_binding` 或 `source_url`。实现素材能力必须完整覆盖 SKU：

```text
implementation_asset_capability >= sku_asset_capability
```

以客户模型创建 Asset 时先解析 publication。若 SKU 允许 Link 资源且存在可用的 `source_url` 实现，
即使当前没有渠道候选也可以创建 source-only Asset；需要预建 Provider binding 时才要求当前存在可
履约实现。

Asset 与 AssetBinding 冻结 publication 身份。显式改绑后，旧版本素材不会被静默解释成新 SKU 的
素材；请求引用时必须与本次冻结的 publication 一致。

### 5.4 Task、计费与审计

Task 与 durable create attempt 冻结：

- 客户合同命名空间、路由族、客户模型名、Link SKU 和 publication version；
- Provider 模型、渠道 ID、implementation ID/version/content hash；
- 客户接入合同、SKU capability、视频/素材执行和计费快照。

异步查询、轮询、remix 和内容代理沿用任务创建时快照，不重新选渠，也不按当前 publication 重新解释
历史任务。普通客户只看到客户模型名和合同结果；管理员审计可以关联 SKU、publication、实现、渠道、
Provider 模型及视频/素材执行方式。

## 6. 架构不变量

1. 客户模型名可以自定义，Link 身份不能依赖名称相似度。
2. 客户模型一旦发布，必须在命名空间和路由族内稳定指向一个 Link SKU。
3. 发布事实与履约可用性分离，渠道和缓存生命周期不能删除或改写客户合同。
4. 改绑 SKU 必须显式确认、乐观锁定版本并写不可变审计。
5. 一个 Link SKU 只有一份客户可见能力合同，可以由多个完整等价实现履约。
6. execution binding 只在精确 implementation 和完整执行维度内解析。
7. 选择 Link 接入方案后，Provider、视频和素材上游合同不再自由拼装。
8. `model_mapping` 只表达客户模型到 Provider 模型，不承担 Link SKU 合同存储。
9. 同一范围内 Link 与普通候选不混用；冲突只让 Link 失败关闭。
10. 客户价格使用客户模型名，渠道成本和权重不进入 Link SKU 能力合同。
11. Task、attempt、Asset 与 Binding 冻结已发生的 publication 和执行事实。
12. NEWAPI 继续负责渠道分发，Link 不建立 Provider 自动选择或兼容降级。
13. 管理端真实模型候选只来自 create execution bindings 的 `provider_model`；SKU 不进入 `models` 或
    `model_mapping`；切换方案不自动改写或删除 `model_mapping`；无 mapping 时使用身份翻译，不生成自映射。

## 7. 相关文档

- [Link 服务合同注册与履约架构](Link服务合同注册与履约架构.md)
- [异步任务与计费事实架构](异步任务与计费事实架构.md)
- [Link 图片服务合同与异步任务架构](Link图片服务合同与异步任务架构.md)
- [Link 视频服务合同与异步任务架构](Link视频服务合同与异步任务架构.md)
- [Link 资源合同与解析架构](Link资源合同与解析架构.md)
- [真人素材授权与撤回架构](真人素材授权与撤回架构.md)
- [ADR-0015：Link 服务合同发布与实现身份绑定](decisions/0015-Link服务合同发布与实现身份绑定.md)
