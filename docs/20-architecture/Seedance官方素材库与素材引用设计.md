---
status: current
owner: Dev Team
last-reviewed: 2026-08-11
---

# Seedance 官方素材库与素材引用设计

## 1. 范围与状态

本文是 Seedance Link 国内火山与海外 BytePlus 官方素材库、素材引用命名空间及其参与 ModelArk V3
视频创建时的专题架构权威。Seedance 专用渠道、确定性路由、Task 和计费总边界继续由
[Seedance 专用渠道与 Link 架构](Seedance专用渠道与Link架构.md)负责。

当前代码已经实现本文的北向身份、Resolver、官方 Action adapter 和租户隔离边界。真实 Provider
凭据、账号权益、素材额度、区域可用性和生产灰度仍需按具体线路验收；代码实现不等于生产账号已经
通过验收。

本文不定义第三方素材协议。第三方是否支持公共引用、真人或 AIGC 私域素材，必须由各自
`asset_upstream_protocol` adapter 明确表达，不能从官方协议推断。

## 2. 设计目标

本设计解决四个问题：

1. 让客户能够引用官方公共预置虚拟人像，但不让中转站承担公共目录管理职责；
2. 让平台通过统一私域资源合同支持真人素材和 AIGC 虚拟人像素材；
3. 保持国内火山与海外 BytePlus 官方协议身份独立，禁止靠换域名隐式兼容；
4. 让请求级 URL/Data URL、公共引用和平台私域资源保持不同语义，避免中转站演变为素材管理平台。

平台只提供转发所需的最小资源代理，不负责替 Provider 管理账号权益、素材容量或公共目录。

### 2.1 官方素材库的作用

官方素材库不是中转站对象存储，也不是通用文件管理系统。它的核心作用是在 Provider 账号内建立
可以重复使用的素材身份，并让 Provider 在入库阶段执行其定义的下载、预处理、内容预检、真人一致性
或 AIGC 分组流程。后续视频请求通过 `asset://<Provider Asset ID>` 引用该身份，不必再次表达素材组、
账号和入库过程。

入库成功不等于平台或 Provider 承诺跳过全部后续审核。素材入库可以减少重复传输或部分前置检查，
但视频创建的输入、提示词和输出仍受 Provider 当次审核、模型限制和账号权益约束。中转站不把
`ready`、`active` 或公共预置身份宣传为“视频一定生成成功”或“已经完成全面法律审核”。

公共素材库与私域素材库的差异是所有权和控制面：

| 对比项 | 官方公共预置素材 | Provider 账号私域素材 |
| --- | --- | --- |
| 谁创建和维护 | Provider | Provider 账号客户通过控制台或 Assets API |
| 客户操作 | 从官方目录取得 ID 后只读引用 | 创建、分组、查询、更新和删除 |
| 平台发现能力 | 不提供；`ListAssets` 不能据此推断公共目录 | 平台只管理通过自身接口创建的私域映射 |
| 容量归属 | 由 Provider 公共服务决定 | 消耗对应 Provider 账号的素材权益和容量 |
| 北向身份 | `pubref_*` | `ast_*` / `astgrp_*` |

## 3. 素材形态与责任边界

| 素材形态 | 北向表示 | 是否进入平台资源库 | 平台责任 | Provider 责任 |
| --- | --- | --- | --- | --- |
| 请求级媒体 | HTTP/HTTPS URL、Data URL | 否 | 按请求合同校验并转发 | 下载、审核、识别和生成 |
| 官方公共预置素材 | `asset://pubref_<Provider公共AssetID>` | 否 | 校验命名格式、去命名空间并转发 | 公共资格、存在性、权限、审核和可用性 |
| 平台私域素材 | `asset://ast_*` | 是 | 租户隔离、状态和固定 Provider 作用域解析 | 私域入库、审核、状态和资源生命周期 |
| 平台私域素材组 | `astgrp_*` | 是 | 租户隔离、固定渠道和上游组映射 | AIGC 分组或真人认证流程 |
| 同账号模型受信产物 | 普通 HTTP/HTTPS URL | 否 | 保持请求级媒体语义 | 按原始产物和账号判断受信期限 |

平台不检测输入是否包含真人，也不根据内容自动选择公共、真人或 AIGC 私域路径。调用方选择北向
表示，Provider 对真实内容、权益和审核结果作最终判断。

## 4. 北向身份合同

### 4.1 私域资源

私域资源使用平台身份：

```text
ast_*          一个平台私域 Asset
astgrp_*       一个平台私域 AssetGroup
asset://ast_*  视频请求中的私域 Asset 引用
```

`ast_*` 可以映射官方私域真人素材，也可以映射官方私域 AIGC 虚拟人像素材。`ast_*` 本身不编码
素材类型、Provider、账号、Region 或 Project；这些事实由主数据库中的 Asset 快照持有。

客户不能通过平台提交、查询或获得裸 Provider 私域 Asset ID。

### 4.2 官方公共引用

官方公共预置素材使用调用方声明型身份：

```text
asset://pubref_<Provider公共AssetID>
```

例如：

```json
{
  "type": "image_url",
  "image_url": {
    "url": "asset://pubref_asset-20260811-example"
  },
  "role": "reference_image"
}
```

`pubref_*` 不是 Asset：

- 不由 `/v1/assets` 创建；
- 不写入主数据库；
- 不归属某个 `user_id + app_id`；
- 不占用平台管理的私域素材数量；
- 不提供平台列表、搜索、详情、修改或删除；
- 不表示平台已经确认该 ID 属于公共目录。

平台要求使用 `pubref_*` 是为了明确调用方声称“这是公共引用”，不是为了替调用方或 Provider
证明该声明。

### 4.3 禁止裸 Provider 引用

客户请求中的 `asset://` 只接受平台明确登记的命名空间。以下形式必须拒绝：

```text
asset://<裸ProviderID>
asset://<未知命名空间>
```

这条约束只维护北向语义一致性，不构成公共资格、安全或法律审核。

## 5. 私域素材控制面

### 5.1 客户接口

私域素材使用统一接口：

```text
POST   /v1/asset-groups
GET    /v1/asset-groups
GET    /v1/asset-groups/{group_id}
DELETE /v1/asset-groups/{group_id}

POST   /v1/assets
GET    /v1/assets
GET    /v1/assets/{asset_id}
PATCH  /v1/assets/{asset_id}
DELETE /v1/assets/{asset_id}
```

列表和单项接口只暴露平台创建并持久化的私域资源，不等同于 Provider 账号素材库浏览器。平台不提供
“从 Provider 控制台云导入”或“管理员分配已有 Provider 资源”的控制面。

### 5.2 AIGC 私域素材

普通 `AssetGroup(group_kind=general)` 在官方协议中映射为 Provider `GroupType=AIGC`。客户可以在该组
中创建图片、视频或音频 Asset，具体媒体支持、权益、审核和额度以 Provider 响应为准。

平台不依据 Provider 账号容量给租户分配素材额度，也不建设可售卖的素材配额系统。现有统一平台资源
数量上限只用于保护数据库和控制滥用，不映射 Provider 容量。Provider 账号容量耗尽、接口限流或权益
不足时，错误通过统一上游错误适配返回。

### 5.3 真人私域素材

`AssetGroup(group_kind=real_person)` 映射为 Provider 真人认证流程：

```text
创建平台 AssetGroup
  -> 固定官方 Channel 和 Provider 作用域
  -> 创建 Provider 验证会话
  -> 返回 Provider verification_url / H5
  -> 用户在 Provider 页面完成流程
  -> 查询平台 AssetGroup 时按需刷新上游状态
```

平台不建立独立真人授权域，不保存人脸、活体、证件、特征或授权正文。真人 Asset 必须属于已经就绪
且作用域一致的真人 AssetGroup；Provider 对真人一致性、认证和入库结果负责。

## 6. 官方南向协议

国内与海外使用不同代码协议和不同构造入口：

| 线路 | 视频协议 | 素材协议 | Action 控制面 | Region |
| --- | --- | --- | --- | --- |
| 火山国内 | `modelark_v3_volcengine` | `volcengine_assets_action_v2024_01_01` | `https://ark.cn-beijing.volcengineapi.com` | `cn-beijing` |
| BytePlus 海外 | `modelark_v3_byteplus` | `byteplus_assets_action_v2024_01_01` | `https://ark.<region>.byteplusapi.com` | Channel 明确配置 |

两套协议可以复用 HMAC Action 传输实现，但不能通过替换 Host、Region 或 Provider 名称互相推断。
Channel 保存时必须使用代码注册的合法视频/素材协议组合。

官方素材控制面使用 `ark` 服务、`2024-01-01` 版本，覆盖：

- AssetGroup 创建、查询、列表、更新和删除；
- Asset 创建、查询、列表、更新和删除；
- 真人验证会话创建和结果查询；
- 只读 `ListAssets` 连通性检查。

Provider 新增、删除或改变字段时，必须通过协议 adapter 和合同测试显式升级，不允许管理员配置 JSON
字段映射或运行时脚本。

## 7. 数据与控制流

### 7.1 请求级媒体

```text
客户 URL/Data URL
  -> ModelArk V3 合同校验
  -> 官方视频 adapter
  -> Provider
```

该路径不访问 Asset 表，不创建 `ast_*`，不自动物化，不获得真人认证或长期审核语义。

### 7.2 公共预置素材

```text
asset://pubref_<id>
  -> 确认当前视频协议为火山国内或 BytePlus 官方
  -> 校验 <id> 长度和安全字符
  -> 改写为 asset://<id>
  -> Provider 判断存在性、公共资格、权限和审核
```

公共引用不要求 Channel 启用私域 `asset_upstream_protocol`，因为它不调用私域素材控制面。Provider
拒绝、资源不存在或账号无权使用时，平台返回经过脱敏和标准化的上游错误，不在本地伪造成功或兜底。

### 7.3 创建私域素材

```text
客户 source URL + astgrp_*
  -> 校验 user_id + app_id、客户模型和素材组
  -> 确定唯一 Seedance Channel
  -> 冻结素材协议、Provider 账号、Region、Project 和凭据身份
  -> 官方 Action CreateAsset
  -> 取得可信 Provider Asset ID
  -> 创建 ast_* 一对一映射
```

source URL 只存在于创建请求和当次 Provider 调用中，不写入 Asset、Task 或日志。没有取得可信
Provider Asset ID 时不创建平台 Asset。

### 7.4 使用私域素材生成视频

```text
asset://ast_*
  -> 按 user_id + app_id 加载 Asset
  -> 校验 ready、客户模型、Channel、素材协议和 Provider 作用域
  -> 真人素材继续复检 AssetGroup
  -> 改写为冻结的 Provider asset://<id>
  -> 与其它请求级媒体一起发送
```

私域素材不能跨国内/海外、Channel、Provider 账号、Region 或 Project 自动迁移。需要在另一线路使用
时，调用方必须在目标线路重新创建并接受该 Provider 的处理结果。

## 8. 持久化事实

主数据库只持久化平台私域资源：

```text
Asset / AssetGroup
  -> user_id + app_id
  -> requested_model
  -> channel_id
  -> asset_upstream_protocol
  -> credential_fingerprint
  -> Provider account + Region + Project
  -> 一个 Provider resource/session
```

`pubref_*` 不进入 Asset、AssetGroup 或租户资源表。视频 Task 可以保存已发生的客户请求与执行事实，
但不能将公共引用解释为平台所有的私域资源。

同一 Provider 账号作用域内轮换 AK/SK 不改变素材身份；改变控制面、协议、账号、Region 或 Project
必须新建 Channel，既有 `ast_*` 不重新解释到新作用域。

## 9. 校验与错误语义

| 场景 | 平台行为 |
| --- | --- |
| `pubref_*` 格式非法 | `400 invalid_asset_reference` |
| 裸 Provider ID 或未知 `asset://` 命名空间 | `400 invalid_asset_reference` |
| 第三方协议使用官方 `pubref_*` | 明确拒绝，不推断兼容 |
| `ast_*` 不存在 | `404 asset_not_found` |
| `ast_*` 未就绪 | `409 asset_not_ready` |
| 私域素材 Channel 不一致 | `409 asset_channel_mismatch` |
| 私域素材账号、Region、Project 或协议不一致 | `409 asset_scope_conflict` |
| Provider 拒绝公共或私域资源 | 返回脱敏、标准化的上游错误 |
| Provider 配额或限流耗尽 | 返回上游错误，不建立租户配额补偿系统 |

平台不得把原始 Provider 响应、凭据、完整签名 URL 或私有诊断信息直接返回客户。

## 10. 明确不承担的职责

本设计不建立：

1. 官方公共素材目录的列表、搜索、详情或缓存；
2. 公共 ID 的存在性、公共资格或安全预检；
3. Provider 控制台既有素材的管理员云导入和租户分配；
4. Provider 素材容量的租户配额、reservation、预占或自动回收；
5. 根据图片内容自动判断真人、虚拟人像或应该使用的素材路径；
6. 外部 AI 人像自动入库、请求级 URL 自动物化或同账号产物自动转 Asset；
7. 国内、海外或第三方素材的自动迁移、复制、fallback 或失败重选；
8. 平台人脸认证、法律授权、撤回或 Provider 数据删除承诺；
9. `create_unknown`、`delete_unknown`、孤儿扫描或管理员核查系统。

素材库容量和账号权益属于 Provider 账号及其管理员的运营责任，不参与中转站租户合同。应用端可以
优先使用请求级 URL；需要 Provider 私域审核、真人流程或稳定资源身份时再创建 `ast_*`。

## 11. 架构不变量

1. `ast_*` 始终表示平台持久化并按 `user_id + app_id` 隔离的私域资源。
2. `pubref_*` 始终表示调用方声明的官方公共引用，不是平台 Asset。
3. Resolver 是 `asset://` 的唯一命名空间解释和改写入口。
4. 裸 Provider Asset ID 不进入北向合同。
5. 公共引用不触发私域 ListAssets、CreateAsset、数据库写入或配额计算。
6. 真人与 AIGC 都可以使用私域 Asset 合同，类型差异由 AssetGroup 和 Provider 流程表达。
7. 国内火山与海外 BytePlus 使用独立代码协议和固定控制面规则。
8. 请求级媒体不自动获得素材库、真人认证、长期审核或迁移语义。
9. Provider 对公共资格、素材审核、账号权益和容量作最终判断。
10. 平台只返回脱敏、标准化错误，不透传原始 Provider 私有响应。

## 12. 代码事实映射

| 设计事实 | 代码位置 |
| --- | --- |
| `ast_*` / `pubref_*` 识别和改写 | `relay/asset_reference_resolver.go` |
| 对外素材解析错误 | `relay/asset_reference_errors.go` |
| 官方 Action adapter | `relay/channel/task/seedance/assets/official_action.go` |
| 国内/海外 adapter 选择 | `service/asset_service.go` |
| 官方控制面与凭据作用域 | `model/channel_asset_credential.go` |
| 视频/素材协议枚举 | `relaykit/dto/upstream_protocol.go` |
| Asset / AssetGroup 持久化 | `model/asset.go`、`model/asset_group.go` |

## 13. 相关文档

- [Seedance 专用渠道与 Link 架构](Seedance专用渠道与Link架构.md)
- [异步任务与计费事实架构](异步任务与计费事实架构.md)
- [素材库对接指南](../30-engineering/素材库对接指南.md)
- [Seedance 视频与素材产品设计](../10-product/Seedance视频与素材产品设计.md)
- [火山方舟私域素材说明](https://www.volcengine.com/docs/82379/2315856?lang=zh)
- [BytePlus 私域素材 API](https://docs.byteplus.com/en/docs/modelark/2333589)
