---
status: current
owner: Dev Team
last-reviewed: 2026-08-09
---

# 07 Seedance 渠道配置清单

## 1. 目的与范围

本文给出当前代码显式注册的 Link Seedance 新任务渠道模板，供管理员新建、复核和停流渠道时使用。
清单覆盖 BytePlus、墨行 Moxing、飞彩和 FunCloud，共 7 个运营渠道模板；Moxing 下的两个当前
Seedance Provider 模型均属 TokenSave 模型体系，按 Link SKU 及 capability 区分。其中本轮
Seedance Provider 专题设计中的飞彩、墨行与 FunCloud 占 6 个。

管理界面中两个 Moxing 方案分别显示为
`Moxing · Seedance 2.0 Overseas` 和
`Moxing · Seedance 2.0 Overseas (Official Key)`。

本文中的 `SD-01`—`SD-08` 只是文档清单编号，不是数据库 Channel ID。客户模型名允许按产品需要
自定义；下表的客户模型是当前运营配置，Provider 模型是后台真实映射目标。Link SKU 由系统内部
通过 execution binding 推导，不作为管理员日常模型命名配置。
真实 Channel 状态、分组、价格、凭据权限、publication、exposure 和 Provider 验收结果仍以运行时
权威事实为准，本清单不表示渠道已经生产发布。

## 2. 渠道总表

| 编号 | 渠道 | Provider | 模型数 | Link implementation | 视频 profile | 创建路径 | 查询路径 |
| --- | --- | --- | ---: | --- | --- | --- | --- |
| `SD-01` | BytePlus 官方 | BytePlus | 1 | `byteplus.seedance-ark@v1` | `official` | 系统内置 | 系统内置 |
| `SD-02` | `seedance-2-0-oversea` Link 方案 | Moxing | 1 | `moxing.seedance-media-task@v2` | `third_party_relay` | `/v1/media/generations` | `/v1/media/tasks/{task_id}` |
| `SD-03` | `doubao-seedance-2-0-260128` Link 方案 | Moxing | 1 | `tokensave.seedance-media-task@v2` | `third_party_relay` | `/v1/media/generations` | `/v1/media/tasks/{task_id}` |
| `SD-04` | 飞彩稳定 VIP | 飞彩 | 5 | `feicai.seedance-videos@v2` | `third_party_json_video_media_arrays` | `/v1/videos` | `/v1/videos/{task_id}` |
| `SD-05` | 飞彩非稳定 933／实验 | 飞彩 | 1 | `feicai.seedance-videos@v2` | `third_party_json_video_media_arrays` | `/v1/videos` | `/v1/videos/{task_id}` |
| `SD-06` | FunCloud Standard | FunCloud | 1 | `funcloud.seedance-json@v1` | `third_party_funcloud_seedance_v2` | `/api/v2/open/aigc/seedance2-0` | `/api/v2/open/aigc/{task_id}` |
| `SD-07` | FunCloud Fast | FunCloud | 1 | `funcloud.seedance-json@v1` | `third_party_funcloud_seedance_v2` | `/api/v2/open/aigc/seedance2-0-fast` | `/api/v2/open/aigc/{task_id}` |
| `SD-08` | 飞彩待验证四模型（隔离） | 飞彩 | 4 | `feicai.seedance-videos@v2` | `third_party_json_video_media_arrays` | `/v1/videos` | `/v1/videos/{task_id}` |

`SD-04`、`SD-05` 与 `SD-08` 共用飞彩 implementation、profile 和路径，但保持为独立运营渠道。
`SD-08` 只是未验证候选的禁用隔离容器，Models、mapping 和禁用 Ability 不创建 Link 合同；没有逐模型
成功证据、publication 和发布批准时不得启用。

## 3. 全部模型映射

### 3.1 `SD-01` BytePlus 官方

| 当前客户模型 | Provider 执行目标 | 素材方式 |
| --- | --- | --- |
| `seedance-byteplus` | 管理员配置的 BytePlus Endpoint ID；由官方 implementation 复检 | `official_action_assets` / `upstream_binding` |

官方渠道不手工填写第三方创建、查询路径。视频 Key、素材 AK/SK、Project 和 Region 按
[02 视频与素材渠道运维手册](02-视频与素材渠道运维手册.md)分别配置，不得混填。

### 3.2 `SD-02` `seedance-2-0-oversea` Link 方案

| 当前客户模型 | Provider 模型 | Base URL | 素材方式 |
| --- | --- | --- | --- |
| `seedance-2-0-oversea` | `seedance-2-0-oversea` | `https://www.moxing.pro` | `relay_assets` / `upstream_binding` |

本渠道只选择 `moxing.seedance-media-task@v2`。不得加入 TokenSave、历史 Ark 或普通 NEWAPI 视频模型，
也不得复用其它线路的 Key、价格或 AssetBinding。

### 3.3 `SD-03` `doubao-seedance-2-0-260128` Link 方案

| 当前客户模型 | Provider 模型 | Base URL | 素材方式 |
| --- | --- | --- | --- |
| `doubao-seedance-2-0-260128` | `doubao-seedance-2-0-260128` | `https://tokensave.pro` | `relay_assets` / `upstream_binding` |

本渠道只选择 `tokensave.seedance-media-task@v2`。它与 `SD-02` 同属 Moxing Provider 的 TokenSave 模型体系且
共享 `/v1/media/*` 协议形状，但 Link SKU、capability、域名、implementation、凭据、价格、素材 binding 和
Provider 证据都必须隔离。`moxing.*` 与旧 `tokensave.*` implementation ID 只保留为不可变履约身份；
方案展示名由代码注册的人类可读文案提供，不使用 implementation ID、版本或 Link SKU。

### 3.4 `SD-04` 飞彩稳定 VIP

| 当前客户模型 | Provider 模型 | 运营分层 |
| --- | --- | --- |
| `seedance-2.0-mini-720p` | `seedance-2.0-vip-720p-mini-azhw-feicai` | VIP Mini |
| `seedance-2.0-fast-720p` | `seedance-2.0-vip-720p-fast-azhw-feicai` | VIP Fast |
| `seedance-2.0-standard-720p` | `seedance-2.0-vip-720p-azhw-feicai` | VIP Standard 720p |
| `seedance-2.0-standard-1080p` | `seedance-2.0-vip-1080p-azhw-feicai` | VIP Standard 1080p |
| `seedance-2.0-standard-4k` | `seedance-2.0-vip-4k-azhw-feicai` | VIP Standard 4K |

“稳定 VIP”是渠道运营分组，不是生产就绪声明。模型名包含 `vip` 只决定本清单的渠道归属，不能替代
逐模型 size、任务、内容、Range、账单、价格和 exposure 验收。

### 3.5 `SD-05` 飞彩非稳定 933／实验

| 当前客户模型 | Provider 模型 | 运营状态 |
| --- | --- | --- |
| `seedance-2.0-unstable-720p` | `seedance-2.0-933-720p-azhw-feicai` | `16:9` 已验证并发布 |

2026-08-06 五模型最小矩阵中，SD2 与 Pro PI create 为 `503 unknown`，Value 1080p 与 Value 4K
明确终态失败；这四个 SKU 已从渠道 52 的 Models 和 model mapping 移除并转入禁用渠道 55（清单编号
`SD-08`）。它们仍保留独立 capability 与 execution binding，但在取得各自成功内容证据前不得重新加入任何启用渠道。以后只要 customer model、
Provider model、implementation 与 Link SKU 不变，调整候选渠道归属不构成 publication 改绑。

两个飞彩渠道都必须使用正式可验证的 HTTPS Base URL、单 Key 和 `source_url` 素材解析；只支持
`general` Link 资源，不建立 `real_person` 合同。不得恢复飞彩 v1、两元 size fallback 或运行时降级。

### 3.6 `SD-06` FunCloud Standard

| 当前客户模型 | Provider execution model | Base URL | 素材方式 |
| --- | --- | --- | --- |
| `seedance-2.0-standard` | `seedance-2.0-standard` | `https://mm-internal-cn.leonecloud.com` | `source_url` |

创建路径必须固定为 `/api/v2/open/aigc/seedance2-0`。本渠道不得加入 Fast 或其它 Provider 的
Seedance 模型。

### 3.7 `SD-07` FunCloud Fast

| 当前客户模型 | Provider execution model | Base URL | 素材方式 |
| --- | --- | --- | --- |
| `seedance-2.0-fast` | `seedance-2.0-fast` | `https://mm-internal-cn.leonecloud.com` | `source_url` |

创建路径必须固定为 `/api/v2/open/aigc/seedance2-0-fast`。Standard 与 Fast 即使使用相同 Base URL
和 Key，也必须使用不同渠道实例；不得在一条渠道中按请求临时切换创建路径。

### 3.8 `SD-08` 飞彩待验证四模型（隔离）

| 当前客户模型 | Provider 模型 | 运行状态 |
| --- | --- | --- |
| `seedance-2.0-sd2-720p` | `seedance2.0-sd2-feicai` | disabled；无 publication |
| `seedance-2.0-value-1080p` | `seedance-2.0-933-1080p-azhw-feicai` | disabled；无 publication |
| `seedance-2.0-value-4k` | `seedance-2.0-933-4k-azhw-feicai` | disabled；无 publication |
| `seedance-2.0-pro-pi-720p` | `seedance-933-pro-pi-feicai` | disabled；无 publication |

当前数据库 Channel ID 为 55，状态为 manually disabled，4 条 Ability 全部禁用。不要使用渠道测试按钮
触发付费生成；只有形成明确的新验证假设并获得成本授权后，才执行单模型最小验证。门禁未闭合前禁止
启用整个渠道或其中任一 Ability。

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
5. 飞彩按 `SD-04`、`SD-05`、`SD-08` 分组验收；`SD-08` 默认整体禁用。任一模型缺少精确 size 或
   账单证据时不得启用对应 Ability，也不得把请求降级到另一飞彩组。
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
