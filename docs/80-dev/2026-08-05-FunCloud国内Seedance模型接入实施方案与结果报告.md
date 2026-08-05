---
status: in-progress
owner: Dev Team
last-reviewed: 2026-08-05
---

# FunCloud 国内 Seedance 模型接入实施方案与结果报告

## 1. 结论

本次实施以
[FunCloud 国内 Seedance 2.0 模型接入设计](../20-architecture/seedance%20模型接入设计/funcloud/FunCloud国内Seedance-2.0模型接入设计.md)
为权威边界，对当前代码、本地 SQLite 配置和真实 FunCloud 凭据进行了逐项核对与实施。

截至 2026-08-05：

- `seedance-2.0-standard` 与 `seedance-2.0-fast` 的代码合同、渠道、Ability、publication、计费表达式、
  durable create attempt 和结果内容代理均已配置；
- Standard 的 `480p`、`720p`、`1080p` 文本生成和 Fast 的 `480p`、`720p` 文本生成均真实成功；
- Fast 的参考视频、图片加音频和 `asset://` 图片三条媒体链路均真实成功；
- 所有成功结果只向客户返回平台 `/v1/videos/{task_id}/content` 投影，Range 下载验证为
  `206 video/mp4`；
- Standard 的图片、图片加音频和参考视频在多次真实调用中仍被 Provider 判为失败；平台均完成客户退款，
  但不能据此宣称 Standard 媒体能力已通过生产验收；
- 两条渠道因此仅在 `randy` 验收组可用，没有开放到 `default` 或其他客户组。

结论是：**代码与测试组配置已完成，Fast 已具备较完整的黑盒成功证据；整个 FunCloud 接入尚不满足生产
发布门禁。** 阻断项是 Standard 媒体稳定性、Provider 账单核对和错误合同取证，而不是平台任务或计费
状态机故障。Quota 专项复核进一步确认：当前系统内的价格表达式求值、分组倍率、预扣、Task 转移、
成功结算、失败退款和账单净额没有发现计算或账本不一致；但这只能证明平台按当前已配置客户价格正确
执行，不能替代 FunCloud 真实账单和客户售价审批。

## 2. 实施前事实与差异

实施前本地已经存在两条 FunCloud 渠道及大部分 adapter 代码，但与设计仍有以下差异：

| 项目 | 实施前 | 本次处理 |
| --- | --- | --- |
| 媒体 role | capability 空列表会跳过二次校验 | 显式登记 image、video、audio role 白名单 |
| 创建错误 | `10002/10005/10006` 被两条路径共同当作确定拒绝 | 删除该推断；所有发送后的 FunCloud 应用错误保持 unknown |
| `90003` | 容易与“未知错误码”混写 | 按官方“服务器内部错误”处理，但因不能证明未创建，仍为 unknown |
| Fast 错误合同 | 沿用 Standard 数值码的可能性未隔离 | 不登记 Fast 确定拒绝；等待数值/字符串真实形态取证 |
| `end_user_subject` | 中间件已有哈希后清空行为，但缺少 FunCloud 精确测试 | 保留平台安全哈希、不转发 Provider，并增加 FunCloud 回归测试 |
| 渠道分组 | 两条渠道原在 `default` | 收敛到 `randy`，Ability 同步限定为 `randy` |
| publication | 启动前本地表尚未迁移 | 启动迁移后生成两个显式 Link publication 及审计记录 |

现有入站中间件已经有基础媒体 role 校验。本次仍把白名单写入 SKU capability，目的是保证任何不经过该
中间件的内部调用路径也不能扩大 FunCloud 合同。

## 3. 代码实施

### 3.1 capability 与 execution binding

`model/video_funcloud_sku_capability.go` 对 Standard/Fast 共同登记：

```text
image roles: reference_image / first_frame / last_frame
video roles: reference_video
audio roles: reference_audio
max media: 3 images / 1 video / 1 audio
direct media: HTTPS only
Link assets: source_url only
mixed direct URL + asset://: forbidden
```

Standard 分辨率为 `480p/720p/1080p`，Fast 为 `480p/720p`；两者 duration 为 4—15 秒，默认 5 秒，
画幅为设计登记的 7 个值。implementation 保持 `funcloud.seedance-json@v1`，adapter 保持
`54:third_party_funcloud_seedance_v2:v2`。当前最终 capability 哈希已由实现注册表重新计算，并在最新
真实 attempt 中冻结验证。

新增/补强的回归测试覆盖：

- 两个 SKU 的 role 白名单与分辨率差异；
- 渠道 profile、创建/查询路径、implementation 与 capability 的一致性；
- 非法 role、HTTP 媒体 URL、未解析 `asset://` 和超量媒体的 fail-closed；
- `end_user_subject` 被哈希记录但不进入 Provider DTO；
- `10002/10005/10006/30003/90003` 及未知应用码都不能自行形成 terminal rejection。

### 3.2 创建错误与 durable attempt

`relay/channel/task/doubao/thirdparty/funcloud/create_response.go` 不再维护只按 body code 判断的
`Definitive` 集合，`relay/channel/task/doubao/adaptor.go` 也删除了对应终态标记分支。

当前规则为：

```text
Provider POST 前失败
  -> safe_to_retry_before_create

Provider POST 后取得可信 taskId
  -> 原子创建 Task、转移 hold、完成 attempt

Provider POST 后无可信 taskId，或收到任何 FunCloud 非零应用码
  -> create outcome unknown，保留 hold，禁止自动重发/换渠道/退款
```

这是对 NEWAPI 现有 adapter 文件的唯一必要 FunCloud 接线调整：删除一条未经证据支持的 Provider 专属
确定拒绝分支。没有修改 NEWAPI 原生路由、公共 Task 状态机或普通模型语义；FunCloud 解析和测试继续隔离
在 Provider 专属文件中，未来接取上游代码的冲突面保持最小。

### 3.3 并行工作区兼容修复

实施期间同一工作区的飞彩 v2 改动把 `ChannelOtherSettings` 固化为值类型并调整 media-arrays 调用签名。
为保持当前工作树可编译，仅对三个已被该改动触发的调用点进行了值类型空值判断和测试 fixture 接线修复。
这些修改不改变 FunCloud 合同，也没有把飞彩 v2 行为引入 FunCloud 路径。

## 4. 实际配置

### 4.1 渠道与 publication

| 配置 | Standard | Fast |
| --- | --- | --- |
| 本地渠道 ID | 53 | 54 |
| 客户模型 / Link SKU | `seedance-2.0-standard` | `seedance-2.0-fast` |
| 分组 | `randy` | `randy` |
| 渠道状态 | enabled，仅测试组有 Ability | enabled，仅测试组有 Ability |
| Base URL | `https://mm-internal-cn.leonecloud.com` | 同左 |
| create path | `/api/v2/open/aigc/seedance2-0` | `/api/v2/open/aigc/seedance2-0-fast` |
| query path | `/api/v2/open/aigc/{task_id}` | 同左 |
| profile | `third_party_funcloud_seedance_v2` | 同左 |
| asset profile | `none` | `none` |
| implementation | `funcloud.seedance-json@v1` | 同左 |
| publication | `(link, modelark_video, seedance-2.0-standard) -> 同名 SKU, v1` | `(link, modelark_video, seedance-2.0-fast) -> 同名 SKU, v1` |

两条旧 Ability 已替换为精确的 `randy + customer_model + channel_id` 记录。普通 Channel、模型映射或价格
没有被用来推断 publication；publication 由数据库权威表持久化并有不可变迁移审计。

生产凭据没有写入文档、测试输出或代码。数据库变更前快照保存在
`/private/tmp/one-api-pre-funcloud-implementation-20260805.db`，用于本次本地配置回退。

### 4.2 计费配置

两个 SKU 均配置：

```text
billing_mode = tiered_expr
preconsume_tokens = 1
```

表达式按 `has_video_input + resolution + duration_seconds` 选择费率：

| SKU | 无视频输入 | 有视频输入 |
| --- | --- | --- |
| Standard 480p / 720p / 1080p | 67,900 / 146,500 / 365,100 × 秒 | 146,500 / 160,000 / 398,900 × 秒 |
| Fast 480p / 720p | 67,900 / 146,500 × 秒 | 146,500 / 160,000 × 秒 |

本次验收用户组倍率为 `0.87`。真实 4 秒文本任务最终 quota 分别为：Standard `118146 / 254910 /
635274`，Fast `118146 / 254910`；Fast 720p 参考视频为 `278400`。这些值与当前计费引擎、模型计费
表达式及用户组倍率计算一致。它们证明平台预扣/结算链路一致，不替代供应商账单核对。

## 5. 验证方案与结果

### 5.1 确定性验证

| 验证 | 结果 |
| --- | --- |
| FunCloud capability、转换、错误和中间件定向测试 | 通过 |
| `model`、`middleware`、Doubao task 全子包、`service` 完整测试 | 通过 |
| `go build ./...` | 通过 |
| `git diff --check` | 通过 |
| `task docs:check` / `task ai:check` | 通过 |
| Fast 1080p | HTTP 400 `unsupported_parameter`，attempt 数不变 |
| 非法 image role | HTTP 400 `invalid_request`，attempt 数不变 |
| nonzero create application code | 单元测试证明不进入 terminal rejection |

相关包完整测试第一次在受限沙箱内因 `httptest` / `miniredis` 无法监听随机本地端口而失败；在允许本地
loopback bind 的环境中以同一命令重跑后全部通过。这是测试环境权限差异，不是代码失败。

### 5.2 真实 Provider 黑盒

| 模型与场景 | 结果 | 账务/结果投影 |
| --- | --- | --- |
| Standard 文本，480p/720p/1080p，4 秒 | 全部成功 | settled；平台内容 URL |
| Fast 文本，480p/720p，4 秒 | 全部成功 | settled；平台内容 URL |
| Fast 参考视频，720p，4 秒 | 成功 | settled；Range 206 |
| Fast 中性图片 + 音频，480p，4 秒 | 成功 | settled；Range 206 |
| Fast `asset://` 图片，480p，4 秒 | Asset `ready`，视频成功 | publication/implementation/Asset 快照一致；Range 206 |
| Fast 含真人特征样例图片 + 音频 | Provider 内容/隐私拒绝 | 客户 quota 归零并 settled |
| Standard 图片、图片 + 音频、参考视频，4 秒与 5 秒 | 多次 Provider 失败 | 每次客户 quota 归零并 settled |

Standard 媒体失败包含 Provider 内部生成失败、下载链路失败和内容/隐私策略拒绝。相同网络素材在 Fast
链路存在成功证据，因此不能把 Standard 的所有失败简单归因为平台 URL 转换；也不能反过来把 Fast 成功
推断为 Standard 已可发布。

本地网络把普通域名解析到 `198.18.0.0/15` 基准测试保留网段，Asset SSRF 保护因此正确拒绝了最初的
域名素材。改用证书有效、可公开访问的公网 IP 图片后，source-only Asset 创建为 `ready`，Fast 经
`asset://` 解析后成功。这个现象属于本地代理网络限制，不能通过放宽 SSRF 规则规避。

### 5.3 持久化不变量核对

当前数据库共冻结 20 条真实 Provider Task：12 条成功、8 条失败。其中 Standard 为 12 条
（5 成功、7 失败），Fast 为 8 条（7 成功、1 失败）。数据库核对结果：

- 20 个 create attempt 均为 `complete + transferred`，因为每次都取得了可信 Provider task ID；
- 没有残留 `sending`、`outcome_unknown`、活动 hold 或自动重发；
- 12 个成功 Task 均为 `SUCCESS + settled`；
- 8 个 Provider 失败 Task 均为 `FAILURE + settled + quota=0`，退款日志存在；
- 每个 attempt 均冻结 `link/modelark_video` publication、对应 SKU、publication v1、
  `funcloud.seedance-json@v1` 和 adapter v2；
- 最终代码快照下补跑的 Standard 文本任务冻结当前 capability 哈希并成功；
- `end_user_subject` 定向测试有非空哈希快照，Provider 请求 DTO 不包含原始值；
- Asset 响应不回显 source URL，客户视频响应不回显 Provider 结果 URL。

负向参数测试在调用前后 attempt 总数保持不变，证明非法合同输入在资金 hold 和 Provider POST 前拒绝。
核对过程中曾发现本机未确认来源的短生命周期网关进程追加真实 FunCloud 请求；新增记录同样满足上述
attempt、Task 与结算不变量。最后一次核对后 8100 端口已无监听进程；后续复验必须在操作人明确授权
凭据范围和费用上限后再恢复服务。

### 5.4 Quota 专项审计

本节复核的是客户端请求经过本系统 ModelArk、Link publication、渠道、durable attempt、共享 Task 和
异步结算链路后形成的 quota 事实，没有为本次审计新增绕过网关的上游直调。证据来自当前 SQLite 中
已经由系统产生的 20 条 FunCloud Task、对应 attempt、冻结计费快照、消费/退款日志，以及计费与结算
定向回归测试。

#### 5.4.1 计算公式与配置生效关系

当前 `QuotaPerUnit=500000`，计费表达式输出采用“美元/百万单位”口径，因此 v1 换算为：

```text
quota_before_group = expression_output / 1,000,000 × 500,000
quota_after_group  = round_half_away_from_zero(quota_before_group × group_ratio)
```

`tiered_expr` 分支以表达式为唯一价格事实，跳过普通 `ModelRatio`、`OtherRatios` 和提交后 adapter 倍率
调整。计费 probe 从与 Provider 出站相同的类型化请求提取 `duration_seconds`、`resolution`、
`has_video_input` 和 `generate_audio`，再与表达式及 `QuotaPerUnit` 一起冻结到 Task，终态不能使用当前
配置重新解释历史请求。

`randy` 验收组倍率为 `0.87`。典型 4 秒请求的独立重算如下：

| 场景 | 基础金额 | 分组前 quota | `× 0.87` 后最终 quota |
| --- | ---: | ---: | ---: |
| 480p、无视频输入 | `$0.0679 × 4 = $0.2716` | 135,800 | 118,146 |
| 720p、无视频输入 | `$0.1465 × 4 = $0.5860` | 293,000 | 254,910 |
| Standard 1080p、无视频输入 | `$0.3651 × 4 = $1.4604` | 730,200 | 635,274 |
| 720p、视频输入 | `$0.1600 × 4 = $0.6400` | 320,000 | 278,400 |

实际 Task 与上述结果一致。另有一条 `default` 分组、倍率 `1` 的 4 秒 480p 任务最终为 `135800`，证明
同一基础价格在不同分组中按冻结倍率执行，而不是被隐藏的模型倍率二次修改。

`preconsume_tokens=1` 不是“只扣一个 quota”，也不是 FunCloud 价格上界。两条表达式均不读取 `c`，
该值当前只承担异步 tiered 配置存在性和求值载体作用；实际预扣完全由冻结请求维度决定。运维和管理端
不得把这个字段解释为客户最大费用。

#### 5.4.2 实际任务和账本勾稽

对 20 条 FunCloud Task 解码冻结 probe，并按当前价格表独立重算后，结果为：

| 核对项 | 结果 |
| --- | ---: |
| Task 总数 | 20 |
| 成功 / 失败 | 12 / 8 |
| 预扣消费日志总额 | 4,469,140 |
| 失败退款日志总额 | 2,082,824 |
| 日志净额 | 2,386,316 |
| 成功 Task 最终 quota 合计 | 2,386,316 |
| probe → 基础价重算不一致 | 0 |
| 分组倍率和取整不一致 | 0 |
| 成功结算不一致 | 0 |
| 失败退款不一致 | 0 |
| 未完成或残留 hold attempt | 0 |

勾稽关系为：

```text
4,469,140 - 2,082,824 = 2,386,316
日志净额 = 12 条成功 Task 的最终 quota 合计
```

20 个 attempt 均为 `complete + transferred`，没有 `sending`、`outcome_unknown` 或活动 hold。成功任务
在 Provider 未返回可信 usage 时保留基于冻结请求维度计算的预扣额度；8 条可信终态失败任务均幂等把
Task quota 结算为 0，并通过退款日志返还客户余额和 Token 额度。

本次定向回归命令及结果：

```text
go test ./relay/helper \
  -run 'TestModelPriceHelperTaskTieredMatchesFunCloudListPrices|TestModelPriceHelperTaskTieredRejectsMissingConfigAndOverflow' \
  -count=1
# PASS

go test ./service \
  -run 'TestRefundFailureIsPersistedAndReconcileRetriesOnce|TestTieredSettlementNeverSupplementsBeyondUpperBound|TestTieredSettlementUsesFrozenExpressionAndMissingUsageKeepsPrecharge|TestTieredSettlementLogRecordsCompletionTokens|TestTieredSettlementSaturationIsAdminAuditedWithoutNegativeCharge' \
  -count=1
# PASS
```

#### 5.4.3 `used_quota` 与最终费用不是同一口径

用户和渠道表的 `used_quota` 在任务提交时累计预扣，当前语义是毛消费统计；可信终态失败退款会恢复
客户余额、订阅/Token 额度并写退款日志，但不会把用户和渠道的毛 `used_quota` 反向减少。因此当前两条
FunCloud 渠道的 `used_quota` 合计为 `4,469,140`，不能当成最终实收金额。

客户账单以日志为权威，分别保存：

```text
gross_quota  = 消费日志 quota 合计
refund_quota = 退款日志 quota 合计
net_quota    = max(gross_quota - refund_quota, 0)
```

本批任务的客户最终费用是 `2,386,316`，不是渠道毛计数 `4,469,140`。后台展示和运营对账应明确把
`used_quota` 标注为毛使用量，涉及收入或客户实扣时必须读取消费/退款日志净额。

#### 5.4.4 仍不能证明的商业计费事实

上述结果证明“平台按当前配置执行正确”，不证明当前数字就是 FunCloud 真实成本或已经审批的最终客户
售价。FunCloud 官方资料仍未提供价格、任务级 usage、失败扣费和退款合同，当前操作手册中的数值也明确
只是客户端基础价配置。

生产价格验收仍有以下阻断：

1. 逐笔取得 Standard/Fast 的实际 Provider 账单，确认币种、精度、取整和价格版本；
2. 验证 Provider 是按请求时长还是实际输出时长收费；若按实际产出收费，必须先取得可信终态用量，不能
   继续只按请求 probe 结算；
3. 验证文本、图片、视频输入和 `generateAudio` 是否属于不同价格维度；当前表达式不因生成音频改变价格；
4. 验证内容拒绝、生成失败和下载失败是否产生 Provider 成本；客户失败退款与 Provider 成本必须分账，
   不能因为客户退款正确就推断供应商没有扣费；
5. 独立审批客户售价。当前表达式填写的是文档所称“供应商原价”，但 `randy=0.87` 会使测试组实际支付
   基础价的 87%；若目标是严格按表中基础价收费，目标生产分组倍率必须为 `1`，若 13% 折扣是有意策略，
   则必须保留对应审批依据。

Quota 专项结论：**技术核算和客户资金账本通过；供应商成本与生产售价未验收，继续作为生产发布门禁。**

## 6. 发布门禁与剩余工作

当前发布决策：**保持测试组，不进入生产组。**

必须完成以下事项后才能评估扩大 Ability：

1. 用供应商确认的中性图片、视频、音频规格重跑 Standard 媒体矩阵，得到连续成功证据；
2. 要求 FunCloud 对 Standard 失败提供可关联的排障结论，确认路径、文件规格、风控与账户能力；
3. 取得 Standard/Fast 创建失败的真实 HTTP status、数值/字符串错误结构，以及“确定未创建”的书面且
   可测试保证；在此之前不得登记 terminal rejection；
4. 对成功、内容拒绝、生成失败三类任务逐笔核对供应商账单，补齐 Provider 金额、失败扣费和退款证据；
5. 复核结果 URL TTL、CDN 跳转和连续 Range 下载；
6. 保持真人模式、callback、批量查询、尾帧和多结果未发布，除非对应合同证据单独闭合；
7. 完成灰度观察后，以显式 publication/Ability 变更进入目标生产组，不直接把渠道 group 改为全局。

Fast 当前虽然通过文本、视频、图片加音频和 Link Asset，但第 3、4、5 项仍是两条 SKU 共同的生产门禁。

## 7. 回退方案

如果测试组需要立即停止：

1. 禁用渠道 53、54 或移除其 `randy` Ability；
2. 保留已产生的 Task、attempt、日志、Asset 和 publication 审计，不删除或改写历史事实；
3. 需要恢复实施前本地配置时，停服务后从上述 SQLite 快照恢复，并重新核对 migration 结果；
4. 代码回退只需撤销 FunCloud role 白名单、错误分类及其测试，不应改动共享 Task/计费/资源状态机。

回退不会承诺取消已发送的 Provider 任务，也不会删除客户已经可访问的历史结果。

## 8. 复验命令

```bash
env GOCACHE=/tmp/codex-yuan-gateway-go-cache \
  go test ./model ./middleware ./relay/channel/task/doubao/... ./service -count=1

env GOCACHE=/tmp/codex-yuan-gateway-go-cache go build ./...

task docs:check
task ai:check
```

真实 API 复验使用测试组 Token，按
[视频模型 API 用户调用指南](../30-engineering/视频模型API用户调用指南.md) 发起；凭据、Provider task ID、
完整 source URL 和原始 Provider 响应不得固化到文档、脚本或版本库。
