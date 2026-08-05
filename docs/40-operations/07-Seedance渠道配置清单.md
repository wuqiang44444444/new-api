---
status: current
owner: Dev Team
last-reviewed: 2026-08-05
---

# 07 Seedance 渠道配置清单

## 1. 目的与范围

本文给出当前代码显式注册的 Link Seedance 新任务渠道模板，供管理员新建、复核和停流渠道时使用。
清单覆盖 BytePlus、墨行 Moxing、飞彩和 FunCloud，共 7 个运营渠道模板；Moxing 下的两个当前
Seedance Provider 模型均属 TokenSave 模型体系，按 Link SKU 及 capability 区分。其中本轮
Seedance Provider 专题设计中的飞彩、墨行与 FunCloud 占 6 个。

管理界面中两个 Moxing 方案分别显示为
`Moxing · tokensave.seedance-2-0-oversea/v2` 和
`Moxing · tokensave.doubao-seedance-2-0-260128/v2`。

本文中的 `SD-01`—`SD-07` 只是文档清单编号，不是数据库 Channel ID。客户模型名允许按产品需要
自定义；下表使用当前 Link SKU 作为推荐客户模型名，以便直接核对 publication、价格和 Ability。
真实 Channel 状态、分组、价格、凭据权限、publication、exposure 和 Provider 验收结果仍以运行时
权威事实为准，本清单不表示渠道已经生产发布。

## 2. 渠道总表

| 编号 | 渠道 | Provider | 模型数 | Link implementation | 视频 profile | 创建路径 | 查询路径 |
| --- | --- | --- | ---: | --- | --- | --- | --- |
| `SD-01` | BytePlus 官方 | BytePlus | 1 | `byteplus.seedance-ark@v1` | `official` | 系统内置 | 系统内置 |
| `SD-02` | `seedance-2-0-oversea` Link 方案 | Moxing | 1 | `moxing.seedance-media-task@v2` | `third_party_relay` | `/v1/media/generations` | `/v1/media/tasks/{task_id}` |
| `SD-03` | `doubao-seedance-2-0-260128` Link 方案 | Moxing | 1 | `tokensave.seedance-media-task@v2` | `third_party_relay` | `/v1/media/generations` | `/v1/media/tasks/{task_id}` |
| `SD-04` | 飞彩稳定 VIP | 飞彩 | 5 | `feicai.seedance-videos@v2` | `third_party_json_video_media_arrays` | `/v1/videos` | `/v1/videos/{task_id}` |
| `SD-05` | 飞彩非稳定 933／实验 | 飞彩 | 5 | `feicai.seedance-videos@v2` | `third_party_json_video_media_arrays` | `/v1/videos` | `/v1/videos/{task_id}` |
| `SD-06` | FunCloud Standard | FunCloud | 1 | `funcloud.seedance-json@v1` | `third_party_funcloud_seedance_v2` | `/api/v2/open/aigc/seedance2-0` | `/api/v2/open/aigc/{task_id}` |
| `SD-07` | FunCloud Fast | FunCloud | 1 | `funcloud.seedance-json@v1` | `third_party_funcloud_seedance_v2` | `/api/v2/open/aigc/seedance2-0-fast` | `/api/v2/open/aigc/{task_id}` |

`SD-04` 与 `SD-05` 共用飞彩 implementation、profile 和路径，但必须保持为两个运营渠道，以便按
稳定线路和非稳定／实验线路分别配置凭据、分组、权重、并发、exposure、告警与停流。两个渠道中的
每个客户模型仍分别保留自己的 Link SKU、capability、execution binding、publication、价格和 Ability。

## 3. 全部模型映射

### 3.1 `SD-01` BytePlus 官方

| 推荐客户模型 / Link SKU | Provider 执行目标 | 素材方式 |
| --- | --- | --- |
| `seedance-byteplus` | 管理员配置的 BytePlus Endpoint ID；由官方 implementation 复检 | `official_action_assets` / `upstream_binding` |

官方渠道不手工填写第三方创建、查询路径。视频 Key、素材 AK/SK、Project 和 Region 按
[02 视频与素材渠道运维手册](02-视频与素材渠道运维手册.md)分别配置，不得混填。

### 3.2 `SD-02` `seedance-2-0-oversea` Link 方案

| 推荐客户模型 / Link SKU | Provider 模型 | Base URL | 素材方式 |
| --- | --- | --- | --- |
| `seedance-2-0-oversea` | `seedance-2-0-oversea` | `https://www.moxing.pro` | `relay_assets` / `upstream_binding` |

本渠道只选择 `moxing.seedance-media-task@v2`。不得加入 TokenSave、历史 Ark 或普通 NEWAPI 视频模型，
也不得复用其它线路的 Key、价格或 AssetBinding。

### 3.3 `SD-03` `doubao-seedance-2-0-260128` Link 方案

| 推荐客户模型 / Link SKU | Provider 模型 | Base URL | 素材方式 |
| --- | --- | --- | --- |
| `doubao-seedance-2-0-260128` | `doubao-seedance-2-0-260128` | `https://tokensave.pro` | `relay_assets` / `upstream_binding` |

本渠道只选择 `tokensave.seedance-media-task@v2`。它与 `SD-02` 同属 Moxing Provider 的 TokenSave 模型体系且
共享 `/v1/media/*` 协议形状，但 Link SKU、capability、域名、implementation、凭据、价格、素材 binding 和
Provider 证据都必须隔离。`moxing.*` 与旧 `tokensave.*` implementation ID 只保留为不可变履约身份；
方案展示名另由代码注册的 `tokensave.<Link SKU>` 提供。

### 3.4 `SD-04` 飞彩稳定 VIP

| 推荐客户模型 / Link SKU | Provider 模型 | 运营分层 |
| --- | --- | --- |
| `seedance-2.0-mini-720p` | `seedance-2.0-vip-720p-mini-azhw-feicai` | VIP Mini |
| `seedance-2.0-fast-720p` | `seedance-2.0-vip-720p-fast-azhw-feicai` | VIP Fast |
| `seedance-2.0-standard-720p` | `seedance-2.0-vip-720p-azhw-feicai` | VIP Standard 720p |
| `seedance-2.0-standard-1080p` | `seedance-2.0-vip-1080p-azhw-feicai` | VIP Standard 1080p |
| `seedance-2.0-standard-4k` | `seedance-2.0-vip-4k-azhw-feicai` | VIP Standard 4K |

“稳定 VIP”是渠道运营分组，不是生产就绪声明。模型名包含 `vip` 只决定本清单的渠道归属，不能替代
逐模型 size、任务、内容、Range、账单、价格和 exposure 验收。

### 3.5 `SD-05` 飞彩非稳定 933／实验

| 推荐客户模型 / Link SKU | Provider 模型 | 运营分层 |
| --- | --- | --- |
| `seedance-2.0-value-720p` | `seedance-2.0-933-720p-azhw-feicai` | 933 Value 720p |
| `seedance-2.0-value-1080p` | `seedance-2.0-933-1080p-azhw-feicai` | 933 Value 1080p |
| `seedance-2.0-value-4k` | `seedance-2.0-933-4k-azhw-feicai` | 933 Value 4K |
| `seedance-2.0-pro-pi-720p` | `seedance-933-pro-pi-feicai` | 933 Pro PI／按次 |
| `seedance-2.0-sd2-720p` | `seedance2.0-sd2-feicai` | 未标注 VIP/933；暂归实验 |

SD2 没有 VIP 或 933 标识，当前归入实验渠道只是保守的运营安排，不创建新的 SKU 语义。现有黑盒中
SD2 create 为 `503 unknown`；在 Provider 明确线路归属且逐模型门禁闭合前，不得把它移入稳定渠道
或启用 Ability。以后只要 customer model、Provider model、implementation 与 Link SKU 不变，调整候选
渠道归属不构成 publication 改绑。

两个飞彩渠道都必须使用正式可验证的 HTTPS Base URL、单 Key 和 `source_url` 素材解析；只支持
`general` Link 资源，不建立 `real_person` 合同。不得恢复飞彩 v1、两元 size fallback 或运行时降级。

### 3.6 `SD-06` FunCloud Standard

| 推荐客户模型 / Link SKU | Provider execution model | Base URL | 素材方式 |
| --- | --- | --- | --- |
| `seedance-2.0-standard` | `seedance-2.0-standard` | `https://mm-internal-cn.leonecloud.com` | `source_url` |

创建路径必须固定为 `/api/v2/open/aigc/seedance2-0`。本渠道不得加入 Fast 或其它 Provider 的
Seedance 模型。

### 3.7 `SD-07` FunCloud Fast

| 推荐客户模型 / Link SKU | Provider execution model | Base URL | 素材方式 |
| --- | --- | --- | --- |
| `seedance-2.0-fast` | `seedance-2.0-fast` | `https://mm-internal-cn.leonecloud.com` | `source_url` |

创建路径必须固定为 `/api/v2/open/aigc/seedance2-0-fast`。Standard 与 Fast 即使使用相同 Base URL
和 Key，也必须使用不同渠道实例；不得在一条渠道中按请求临时切换创建路径。

## 4. 不进入新渠道清单的线路

| 线路或模型 | 处理 |
| --- | --- |
| `dreamina-seedance-2-0-260128` / Moxing Ark v1 | 只解释完整冻结的历史 Task；不得新增 Channel、Ability 或 AssetBinding |
| TokenSave v1 | 只解释完整冻结的历史 Task；不得创建新任务候选 |
| JoyCreator 素材管理 | management-only，不是视频 execution binding |
| NEWAPI 原生 Seedance、Doubao 或 OpenAI Videos 模型 | 继续服从上游原生 Router、DTO、Relay、Provider adapter 和计费语义；不配置为本清单 Link 渠道 |

## 5. 新建与启用检查

每个渠道逐项执行，不得用同 Provider 的其它模型结果替代：

1. 选择精确 Link 接入方案，确认 implementation ID/version、profile、adapter 和路径均由注册事实锁定。
2. `Models` 使用客户模型名；`model_mapping` 严格按本清单从客户模型映射到 Provider 模型或执行目标。
3. 使用专用测试分组、单 Key 和经批准的 HTTPS Base URL；不要在文档、日志或工单记录真实凭据。
4. 分别核对每个客户模型的 publication、capability/hash、execution binding、价格和 exposure 策略。
5. 飞彩按 `SD-04`、`SD-05` 分组验收；任一模型缺少精确 size 或账单证据时只停用该模型 Ability，
   必要时停用整个渠道，不把请求降级到另一飞彩组。
6. FunCloud Standard 与 Fast 分别验收固定创建路径，禁止同渠道动态切换。
7. 墨行两个渠道分别验收凭据、素材账号作用域、创建、轮询、内容和 Provider 对账。
8. 完成真实成功、确定拒绝、create unknown、幂等、预扣、结算、退款和 Range 验收后，才允许从测试
   分组扩展到生产分组。

## 6. 权威文档

- [Seedance 模型接入设计索引](../20-architecture/seedance%20模型接入设计/README.md)
- [飞彩 Seedance 全模型接入设计](../20-architecture/seedance%20模型接入设计/飞彩/README.md)
- [墨行 Seedance 模型接入设计](../20-architecture/seedance%20模型接入设计/墨行/README.md)
- [FunCloud 国内 Seedance 2.0 模型接入设计](../20-architecture/seedance%20模型接入设计/funcloud/FunCloud国内Seedance-2.0模型接入设计.md)
- [02 视频与素材渠道运维手册](02-视频与素材渠道运维手册.md)
- [06 飞彩 Seedance 全模型上线验收手册](06-飞彩Seedance全模型上线验收手册.md)
