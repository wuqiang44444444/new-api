---
status: current
owner: Dev Team
last-reviewed: 2026-08-14
---

# FunCloud 全模型与素材计费设计

## 1. 范围与状态

FunCloud 通过 `ChannelTypeSeedanceLink`、ModelArk V3 北向合同和代码协议 `funcloud_seedance` 接入。
Standard、Fast、Mini、2.5 分别使用独立客户模型和 Channel；代码实现不等于真实 Provider、账单或
生产灰度已经验收。

FunCloud 不使用 publication、SKU、capability、implementation、execution binding、Priority、Weight、
失败重选或运行时 fallback。客户模型、Provider 模型、路径和素材协议只能由明确配置及代码注册表确定。

当前可依赖的证据边界是：Standard/Fast 的历史成功请求已证明富 `content` 协议；Standard 的真实
查询已经确认 `GET /api/v2/open/aigc/{task_id}` 在成功终态返回 `completionTokens`；
Mini 的合同按用户确认与 Fast 一致；2.5 按独立文档适配。代码和本地回归已完成，但 Mini/2.5
真实生成、全量素材信封、账单和灰度仍是未验证事项。

## 2. 四模型与渠道合同

| 客户模型 | Provider 模型 | 创建路径 | 素材协议 |
| --- | --- | --- | --- |
| `seedance-2-funcloud` | `seedance-2` | `/api/v2/open/aigc/seedance2-0` | `funcloud_material` 或 `none` |
| `seedance-2-fast-funcloud` | `seedance-2-fast` | `/api/v2/open/aigc/seedance2-0-fast` | `funcloud_material` 或 `none` |
| `seedance-2-mini-funcloud` | `seedance-2-mini` | `/api/v2/open/aigc/seedance2-0-mini` | `funcloud_material` 或 `none` |
| `seedance-2-5-funcloud` | `seedance-2-5` | `/api/v2/open/aigc/seedance2-5` | 只允许 `none` |

每条渠道必须只有一个客户模型和一条完全匹配的 `model_mapping`。查询路径统一为
`/api/v2/open/aigc/{task_id}`。FunCloud body 不发送 `model`、`prompt` 或 `mode`；Provider 模型仍冻结在
Task 私有快照中，用于路径、校验和计费解释。

路径必须从 `Provider 模型 -> model spec` 精确注册表取得。保存和请求阶段都拒绝未知模型；
不存在 `Contains("fast")`、默认 Standard、相邻模型回退或跨 Channel fallback。

## 3. 视频能力边界

FunCloud 南向使用已验证的富 `content` 结构，包含 text/image/video/audio 内容以及
`ratio`、`duration`、`resolution`、`generateAudio`、`watermark`、`seed` 和
`cameraFixed`。至少存在一个非空 text，省略 duration 时发送 5 秒。

| 模型 | 时长 | 分辨率 | 最大图片/视频/音频 |
| --- | --- | --- | --- |
| Standard | 4—15 秒，不接受 `-1` | 480p/720p/1080p | 3/1/1 |
| Fast | 4—15 秒，不接受 `-1` | 480p/720p | 3/1/1 |
| Mini | 与 Fast 相同 | 480p/720p | 3/1/1 |
| 2.5 | 4—30 秒或 `-1` 智能时长 | 480p/720p | 9/3/3 |

- Standard/Fast/Mini 支持 `16:9`、`9:16`、`1:1`、`4:3`、`3:4`、`21:9` 和
  `adaptive`；
- 所有模型保留 `generate_audio`、`watermark`、`camera_fixed` 的显式 `false` 和 `seed=0`；
- 2.5 不向北向新增 `taskType` 或 `realPersonMode`，素材 CRUD 元数据明确为不支持；视频中若提交
  `asset://<opaque-id>`，平台不查询素材，由 FunCloud 最终拒绝或接受；
- 当前本地合同没有开放 `callback_url`、`return_last_frame`、`output_format`、`480pto720p`、tools、
  draft、priority、service tier 或 frames。部分字段可能出现在 Provider 文档中，但尚未进入已验证的
  北向兼容合同，因此提交前明确拒绝，不能写成“Provider 未发布”，也不能静默删除。

未知 Provider 模型、未注册路径和合同外参数在 Provider I/O 前失败关闭。

## 4. 异步任务与迁移

创建沿用共享 `TaskCreateAttempt + hold + sending` 原子边界；每个创建意图只发送一次。成功查询固定为
`GET /api/v2/open/aigc/{task_id}`，必须具有一致 task ID、受支持状态、唯一 HTTPS 结果和可信的
`data.completionTokens`，否则进入
`RECONCILIATION_REQUIRED`，保留预扣且不自动重发、换渠道或退款。

只有 `code=0 + data.taskId + data.status=processing` 才形成正式 Task。发送后网络错误、非法 JSON、
非零应用 code 或缺失可信 task ID 均保持 create unknown。查询状态的唯一映射为：

- `processing/running/submitted -> IN_PROGRESS`；
- `success/completed/succeeded -> SUCCESS`；
- `failed -> FAILURE`。

未知状态、task ID 冲突、结果 URL 冲突或非法 URL 都是合同违例，不得由单次轮询直接判为业务失败。

管理协议固定为无版本名 `funcloud_seedance` / `funcloud_material`，adapter revision 单独冻结为 v3。
运行时没有 FunCloud v2 decoder、alias 或 fallback。一次性旧名称迁移只处理终态 Task、完成/拒绝的
create attempt 和完成的 idempotency；发现任何引用旧协议的活动耐久事实时，启动迁移必须整体中止，
由技术人员先离线处置。

## 5. FunCloud 标准素材库

`funcloud_material` 只与 `funcloud_seedance` 配对，且只允许 Standard、Fast、Mini。当前发布虚拟素材组、
组查询、空组删除、虚拟素材 multipart 上传和素材查询；单素材重命名/删除、真人素材及 2.5 素材
明确不支持。

| 能力 | FunCloud 路径 | 发布边界 |
| --- | --- | --- |
| 创建虚拟素材组 | `/api/v2/open/material/group/create` | 支持 |
| 查询素材组 | group list | 只接受唯一冻结 group ID |
| 删除空素材组 | `/api/v2/open/material/group/delete` | 平台和 Provider 双重空组校验 |
| 上传虚拟素材 | `/api/v2/open/material/virtual/upload` | HTTPS 安全回源后流式 multipart |
| 查询素材 | material list | 只接受唯一冻结 material ID |

素材组删除需要同时满足：平台没有活动 Asset，且 Provider group list 的唯一匹配项明确返回
`materialCount == 0`。Provider 查询失败、响应歧义、重复 ID 或非零素材数均不得调用删除接口。这个
Provider 预检是防止删除平台未知素材的必要边界。

素材上传复用拨号期 SSRF 防护，限制 HTTPS/443、DNS 重绑定和逐跳重定向；回源与 multipart 均流式传输，
媒体上限 100MB，Provider 成功响应上限 1MB。source URL 不持久化、不记录、不返回；上传响应只保存
受控 Provider 资源身份和固定作用域。

列表信封只按代码明确的形状解码；重复 ID、多个集合字段、未命中、分页不完整或非法
`assetUrl` 全部失败关闭，不通过名称、URL 或模糊搜索猜测资源。Resolver 在每次视频发送前将
视频 `asset://<opaque-id>` 原样进入 FunCloud 请求，不经过本地素材解析或作用域复检。

统一北向没有素材组更新接口。Provider `/material/group/update` 不属于当前平台发布合同，不建立
FunCloud 专属北向，也不在共享 adapter 中保留无调用方实现。完整素材边界见
[FunCloud 素材协议与边界设计](FunCloud素材协议与边界设计.md)。

## 6. 客户价格与 Provider 计量

### 6.1 FunCloud 海外价格事实

价格证据以 `docs/70-research/funcloud/海外/价格.md` 为来源，计费单位均为每百万 Token。
当前客户价格使用 USD 原价；Standard 括号内的 85% 折扣价、按秒价和人民币价只是参考，
不参与当前客户结算。

| Provider 模型 | 输入类型 | 分辨率 | USD 原价 | 85% 参考价 | 按秒参考价 | 参考 RMB 价 |
| --- | --- | --- | ---: | ---: | ---: | ---: |
| Standard | t2v/i2v/r2v | 480p | $6.74 | $5.73 | $0.0679/秒 | 约 ¥45.47 |
| Standard | t2v/i2v/r2v | 720p | $6.74 | $5.73 | $0.1465/秒 | 约 ¥45.47 |
| Standard | t2v/i2v/r2v | 1080p | $7.48 | $6.36 | $0.3651/秒 | 约 ¥50.41 |
| Standard | v2v | 480p | $4.11 | $3.49 | $0.1465/秒 | 约 ¥27.68 |
| Standard | v2v | 720p | $4.11 | $3.49 | $0.1600/秒 | 约 ¥27.68 |
| Standard | v2v | 1080p | $4.55 | $3.86 | $0.3989/秒 | 约 ¥30.64 |
| Fast | t2v/i2v/r2v | 480p | $5.43 | — | $0.0547/秒 | 约 ¥36.57 |
| Fast | t2v/i2v/r2v | 720p | $5.43 | — | $0.1178/秒 | 约 ¥36.57 |
| Fast | v2v | 480p | $3.23 | — | $0.0618/秒 | 约 ¥21.75 |
| Fast | v2v | 720p | $3.23 | — | $0.1325/秒 | 约 ¥21.75 |
| Mini | t2v/i2v/r2v | 480p | $3.37 | — | — | 约 ¥22.73 |
| Mini | t2v/i2v/r2v | 720p | $3.37 | — | — | 约 ¥22.73 |
| Mini | v2v | 480p | $2.05 | — | — | 约 ¥13.84 |
| Mini | v2v | 720p | $2.05 | — | — | 约 ¥13.84 |
| 2.5 | t2v/i2v/r2v | 480p | $10.26 | — | — | 约 ¥69.19 |
| 2.5 | t2v/i2v/r2v | 720p | $10.26 | — | — | 约 ¥69.19 |
| 2.5 | v2v | 480p | $6.16 | — | — | 约 ¥41.51 |
| 2.5 | v2v | 720p | $6.16 | — | — | 约 ¥41.51 |

### 6.2 当前客户计费配置

四个客户模型必须使用冻结美元 `tiered_expr`。表达式系数是上表 USD/百万 Token
原价，表达式引擎统一执行百万 Token 到 quota 的换算：

| 客户模型 | 冻结表达式 | 预扣 Token 上界 |
| --- | --- | ---: |
| `seedance-2-funcloud` | `has_video ? (1080p ? c*4.55 : c*4.11) : (1080p ? c*7.48 : c*6.74)` | 730,000 |
| `seedance-2-fast-funcloud` | `has_video ? c*3.23 : c*5.43` | 324,000 |
| `seedance-2-mini-funcloud` | `has_video ? c*2.05 : c*3.37` | 324,000 |
| `seedance-2-5-funcloud` | `has_video ? c*6.16 : c*10.26` | 648,000 |

实际存储的表达式使用 `param("_task.has_video_input")`、
`param("_task.resolution")` 和 `tier(name, value)` 展开上表分支。表达式只读取创建时冻结的
输入类型、分辨率和 completion tokens；不能使用按秒参考价，也不能使用当前价格重算历史 Task。

### 6.3 `completionTokens` 实际 Token 用量

成功终态以查询响应中的 `data.completionTokens` 作为唯一实际 Token 用量：

```text
actual_completion_tokens = data.completionTokens
```

`completionTokens` 必须是 JSON 整数、严格大于零，且不超过创建时冻结的预扣 Token 上界。字段缺失、
类型错误、零、负数或超界均视为合同违例，Task 进入 `RECONCILIATION_REQUIRED` 并保留 hold；不得再用
`pointConsume`、价格、汇率或当前渠道配置反推、估算或替代实际 Token。

`pointConsume` 只作为 Provider 成本证据，不参与客户 Token 用量和 tiered expression 结算。若响应提供
该字段，它必须是有限且严格大于零的十进制数；顶层与 `output` 同时提供时必须数值一致。缺失
`pointConsume` 不影响已有可信 `completionTokens` 的客户结算，但 Provider 成本保持未知；非法或冲突
的消费字段仍视为不可采信响应。

成功结算会在 Task 私有计费事实中持久化实际 Token，并在管理员计费证据中记录来源字段
`completionTokens`、Provider 报告 Token、可选的原始 `pointConsume`、Provider 模型、分辨率和是否有
视频输入。相关证据只在 `other.admin_info.provider_billing_evidence` 中投影；客户 Task 数据和普通用户
日志不包含私有证据。

失败任务不要求或生成 Token 用量，按共享明确失败合同处理退款。旧的 `pointConsume` 反推算法已从
v3 查询适配器中直接删除，不保留历史 decoder、fallback 或双轨解释。

## 7. 职责与唯一事实源

- Channel 保存路径负责一模型一渠道、精确 `model_mapping` 和协议配对；
- 视频 adapter 负责 Provider 模型规格、路径、请求序列化、状态和计量解释；
- 素材 adapter 负责 FunCloud group/material 信封，共享 Asset service 负责租户、作用域和 source
  生命周期；
- Task、TaskCreateAttempt、Asset/AssetGroup 和私有计费快照是耐久事实；前端配对只做即时提示，
  后端保存校验始终是最终门禁；
- FunCloud 不建第二套 Router、Task、Asset、账本或运行时兼容层。

## 8. 生产门槛

四模型仍需逐一完成真实创建/查询、正确输出、明确失败和超时行为、结果 URL 生命周期；
Standard/Fast/Mini 还需完成素材组、上传/刷新、`asset://` 生成和严格信封验证。每个模型都要
核对 Provider 后台 Token、美元成本、失败账单和小流量灰度。

Standard 的单个样本已经证明 `completionTokens` 与 Provider 后台 Token 一致；仍需补齐跨模型、输入
类型、分辨率和账号的样本，验证字段稳定性和账单闭环。未完成这些外部验证前，不得写成生产已发布。

## 9. 代码事实映射

| 设计事实 | 代码位置 |
| --- | --- |
| 四模型、路径和素材协议注册 | `relaykit/dto/upstream_protocol.go` |
| 一模型一渠道和精确映射校验 | `model/channel_seedance_funcloud.go` |
| 四模型时长、分辨率和媒体上限 | `relay/channel/task/seedance/funcloud_models.go` |
| Provider 请求和创建响应 | `relay/channel/task/seedance/thirdparty/funcloud/` |
| 查询状态、`completionTokens` 用量和 `pointConsume` 成本证据 | `relay/channel/task/seedance/thirdparty/funcloud/task_response.go` |
| 冻结计费 probe 和私有证据 | `relay/channel/task/seedance/funcloud_billing_context.go`、`relay/common/provider_billing_evidence.go` |
| 素材边界 | [FunCloud 素材协议与边界设计](FunCloud素材协议与边界设计.md) |
