---
status: current
owner: Dev Team
last-reviewed: 2026-08-10
---

# Seedance 统一北向合同架构

本文定义 Seedance Link 模型在 ModelArk v3 北向入口下的当前合同边界、能力权威、机器投影和
失败关闭规则。共享异步 Task、计费、资源和 Provider adapter 的完整状态流由
[Link 视频服务合同与异步任务架构](Link视频服务合同与异步任务架构.md)负责；各 Provider 的精确
执行差异由 [Seedance 模型接入设计索引](<seedance 模型接入设计/README.md>)负责。

本文不覆盖 NEWAPI 原生 `/v1/videos`、Kling 或即梦合同，也不把代码登记解释为生产发布。

## 1. 合同边界

Seedance Link 客户模型统一使用：

```text
POST /api/v3/contents/generations/tasks
contract_id = modelark.contents.generations.v3
contract_version = 2024-01-01
```

“统一”只表示相同的字段名、JSON 类型、媒体 content 结构、规范词汇和异步任务语义。每个模型仍使用
自己的稀疏 `VideoSKUCapability` 子集，不共享一张字段并集。南向 adapter 只能把已验证的统一语义
确定性转换为 Provider 请求，不能把 Provider 私有参数反向加入北向合同。

合同族保持分离：

| 合同族 | 客户入口 | 边界 |
| --- | --- | --- |
| Seedance ModelArk v3 | `/api/v3/contents/generations/tasks` | 本文负责模型级稀疏 capability |
| NEWAPI 原生 OpenAI Videos | `/v1/videos` 等上游原生入口 | 以上游 Router、DTO、Relay 和模型发现为权威 |
| Kling v1 | `/kling/v1/videos/...` | 保留 Kling 官方字段和响应外壳 |
| 即梦 | `/jimeng/?Action=...` | 保留即梦 Action/Version 与响应外壳 |

ModelArk middleware 为复用任务基础设施而进行的内部路径重写不是客户入口。严格解析得到的 typed
contract 必须保存在 context 中，并贯穿 publication、capability、选渠、Provider 发送与任务快照；
后续链路不得按重写后的路径重新解释请求。

## 2. 权威对象与版本

| 对象 | 唯一职责 | 变化规则 |
| --- | --- | --- |
| 客户接入合同 | 请求、响应、错误和生命周期协议 | 不兼容变化提升合同版本 |
| `VideoSKUCapability` | 单一 SKU 的客户可见字段、值域、默认值、媒体与生命周期 | 客户可观察变化提升 capability version/hash |
| `LinkModelPublication` | `(namespace, route family, customer model) -> SKU + publication version` | 只有改绑另一 SKU 时提升 publication version 并审计 |
| implementation | Provider 履约方式及不可变 execution binding | 路径、profile、adapter、Provider 模型或资源方式变化提升 implementation version/hash |
| Channel / Ability | 下一次请求的候选范围和开放范围 | 不能创建、猜测或扩张 Link 合同 |
| Task / create attempt | 已发生执行的冻结事实 | 后续配置变化不得重解释 |

capability、implementation 和 publication version 各自升级。同一 SKU 的 capability 变化不提升
publication version；同一客户模型改绑另一 SKU 也不能通过普通 Channel 保存或请求形状隐式完成。
Task、create attempt、Asset 与 Binding 冻结创建时的 publication、capability 和 implementation
身份及内容 hash。

## 3. ModelArk capability 语义

### 3.1 规范词汇

允许进入 Seedance capability 的 resolution 词汇为：

```text
480p | 720p | 1080p | 4k
```

ratio 词汇为：

```text
16:9 | 4:3 | 1:1 | 3:4 | 9:16 | 21:9 | adaptive
```

词汇表不是每个模型的共同值域。每个 SKU 只能声明经过注册和证据门禁的子集。`2k`、`4K`、像素尺寸、
`horizontal` 等别名不进入北向合同；客户端传入非成员值时返回稳定 `400`，不能静默改写。

### 3.2 默认值、组合与字段存在性

duration、resolution 和 ratio 的默认值属于 capability，并进入 `ContentHash`。adapter 只能读取冻结的
`DefaultDuration`、`DefaultResolution` 和 `DefaultRatio`，不能从数组顺序、硬编码值或 Provider
当前默认推断。分辨率与画幅不是笛卡尔积时，使用 `ResolutionRatioCombinations` 显式登记。

未冻结默认值的维度必须由客户显式提交；缺少精确 Provider evidence 时不能为填满 capability 猜测
默认值或组合。以下值一旦出现在 typed request 中都代表明确客户意图：

- `false`；
- `0`；
- `-1`；
- `"default"`；
- schema 允许的空数组或空对象。

不支持字段必须在计费和 Provider POST 前拒绝，不能因为值看似默认而删除。ModelArk typed 路径中，
capability 允许 `service_tier` 但候选不能等价发送时也必须明确失败，包括显式 `"default"`。只有 legacy
`/v1/video/generations` metadata 路径保留把不支持的 `default` 归一化为缺失的历史兼容语义；该语义
不得反向进入 ModelArk 合同。

### 3.3 媒体与资源

`content` 只使用 `text`、`image_url`、`video_url`、`audio_url` 及 capability 声明的 role、数量和组合。
请求级 HTTP(S)/Data URL 只具备当前请求的直接媒体语义；平台资源只使用 `asset://ast_*`，由 Resolver
在 user、app、publication、implementation、账号、授权和 TTL 门禁后转换为执行引用。DTO 能解析某种
媒体不代表所有 SKU 或 profile 都支持它。

Provider asset ID、Provider 模型、implementation、账号和完整签名 URL 不进入北向合同。

## 4. 当前注册分组

代码当前登记 15 个 ModelArk/Seedance SKU；登记只证明存在可校验身份，不证明存在数据库 publication、
当前 Token 可见 Ability 或生产验收：

| 模型组 | capability version | implementation | 数量与边界 |
| --- | --- | --- | --- |
| BytePlus official | `public-video-contract-v2` | `byteplus.seedance-ark` v1 | `seedance-byteplus`；高级字段仍受精确官方模型证据门禁 |
| 墨行中转 | `moxing-media-task-v2` | `moxing.seedance-media-task` v2 | `seedance-2-0-oversea`；严格媒体子集 |
| TokenSave 中转 | `tokensave-media-task-v2` | `tokensave.seedance-media-task` v2 | `doubao-seedance-2-0-260128`；严格媒体子集 |
| FunCloud | `public-video-contract-v2` | `funcloud.seedance-json` v1 | Standard/Fast 两个独立 SKU；显式默认值由 capability 驱动 |
| 飞彩 media-arrays | `feicai-media-arrays-v2` | `feicai.seedance-videos` v2/v3 | 10 个固定档位 SKU；v2 绑定带后缀模型，v3 绑定无后缀模型；每个实现版本、Provider 模型、分辨率、画幅和计费形状独立履约 |

`seedance-2-0-oversea` 与 `doubao-seedance-2-0-260128` 是两个独立客户 SKU，不是同一 SKU 的可互换
Provider 候选。相似模型名、同源 Provider 或相同请求字段都不能建立互换关系。

所有 capability 的 request/required/unsupported fields、值域、默认值、组合矩阵、媒体数量/role、
profile/channel 约束和 lifecycle 都进入 capability hash。运行时使用代码 registry 的
`ContentHash`；OpenAPI 和 capability API 只是投影，不是第二套手写权威。

## 5. 请求与履约流

```text
严格解析 typed request
  -> 读取并冻结客户模型 publication
  -> 读取并冻结 SKU capability version/hash
  -> 校验字段存在性、值域、默认值、组合和媒体
  -> 对 asset:// 求可执行 implementation/channel 交集
  -> NEWAPI 按 Ability、分组、优先级和权重选渠
  -> 应用 model_mapping 得到 Provider 模型
  -> 复检 implementation、execution binding、账号和资源
  -> adapter 按冻结 capability 与 Provider evidence 构造请求
  -> 建立 durable create attempt、资金 hold 与 sending
  -> Provider POST
  -> 取得可信 Provider task ID 后原子创建 Task
```

publication 缺失、实现未知或不完整、字段不支持、组合无证据、资源交集为空或 execution binding 冲突
都必须在发送与收费前失败。已发布但无合格候选时保留 publication，并返回暂不可履约；不得降级到另一
SKU、普通 NEWAPI 语义或按请求内容猜测合同。

飞彩等需要精确 `size` 的 implementation 使用完整 evidence 键：

```text
(implementation ID/version, Provider model, resolution, ratio)
  -> Provider size + multiplier + billing class + evidence version
```

门禁、adapter 与计费探针必须读取同一 evidence registry。未登记组合在预扣和 Provider POST 前失败，
不能复用同分辨率兄弟模型的 size。Standard 1080p 当前 registry 请求 size 为 `1280x720`，即使黑盒
产物曾观察到 1920×1080，也不能在逐任务 Provider 账单和交付质量复核前据此宣称生产可用。

## 6. 机器发现与发布状态

`GET /v1/models` 继续只回答当前 Token 的客户模型可见性和 endpoint type，不加入庞大的 capability、
Provider、价格或 implementation 对象。

Seedance 参数能力由两类投影提供：

- OpenAPI 的 `ModelArkVideoCreateRequest.x-modelark-model-capabilities`：由 runtime registry 生成获准
  公开的静态模型子集；
- 带 Token 鉴权的 `GET /api/v3/contents/generations/models`：返回全部代码登记的 ModelArk 基础候选，
  并追加该 Token 的 publication 客户模型 alias。

alias 通过 `CustomerModel -> LinkSKU` publication 读取 capability，但响应继续使用客户模型 ID，不能
暴露绑定 SKU、Provider 模型或 implementation。接口将三个状态与参数能力分开：

| 字段 | 含义 |
| --- | --- |
| `published` | 当前客户模型存在 publication |
| `visible_in_v1_models` | 当前 Token 能从 `/v1/models` 看见该客户模型 |
| `available` | 当前 Token 有合格候选可尝试执行 |

代码登记、机器展示、publication、Ability 开放和生产验收是五种独立状态。调用方只能在
`published=true`、`visible_in_v1_models=true`、`available=true` 时尝试创建；这仍不替代真实 Provider、
账单和灰度验收。

OpenAPI 投影由 `go run ./cmd/generate-modelark-capabilities` 生成，后端一致性测试对规范化后的 runtime
和投影做深比较。新增或修改公开 capability 时，两者不一致必须使 CI 失败。

## 7. 架构不变量

1. NEWAPI 原生视频、ModelArk、Kling 和即梦合同不得互相推断、包装或降级。
2. 同一 ModelArk 字段结构不等于所有 Seedance 模型共享字段和值域。
3. capability 是客户能力权威；Channel、Ability、价格、profile 和 Provider 当前默认不能扩张它。
4. 显式零值、默认字符串和空集合不得被南向静默删除或改义。
5. adapter 只消费冻结 capability、execution binding、资源解析结果和精确 Provider evidence。
6. capability、implementation 与 publication 版本分别管理，不得联动误升级。
7. 代码登记、机器展示、publication、Ability 与生产验收必须分别表达。
8. Task 与 create attempt 使用创建时快照，后续配置变化不得重选渠或重解释。
9. Provider 创建结果为 unknown 时不自动重发、换渠道或退款。
10. 未完成真实 Provider、账单、外部数据库和灰度验证的 SKU 不得写成生产已发布。

## 8. 相关文档

- [Link 视频服务合同与异步任务架构](Link视频服务合同与异步任务架构.md)
- [Link 服务合同注册与履约架构](Link服务合同注册与履约架构.md)
- [公开 API 文档交付架构](公开API文档交付架构.md)
- [Seedance 模型接入设计索引](<seedance 模型接入设计/README.md>)
- [视频模型 API 用户调用指南](../30-engineering/视频模型API用户调用指南.md)
- [Seedance 多上游视频合同收口与灰度](../50-planning/路线图.md#seedance-多上游视频合同收口与灰度)
