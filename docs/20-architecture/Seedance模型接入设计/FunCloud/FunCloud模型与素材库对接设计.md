---
status: current
owner: Dev Team
last-reviewed: 2026-09-08
---

# FunCloud 模型与素材库对接设计

代码同时登记旧 V2 与独立 V3 协议。V3 已完成本地实现与自动测试，真实验收进行中；现有渠道未自动迁移，不能视为生产发布。V2 事实见第 1–5 节，V3 边界见第 6 节。

## 1. V2 身份与模型映射

协议固定为 `video_upstream_protocol=funcloud_seedance`；可配模型与路径如下：

| 客户模型（示例） | Provider 模型 | 创建路径 | 素材协议 |
| --- | --- | --- | --- |
| `seedance-2-funcloud` | `seedance-2` | `/api/v2/open/aigc/seedance2-0` | `funcloud_material` 或 `none` |
| `seedance-2-fast-funcloud` | `seedance-2-fast` | `/api/v2/open/aigc/seedance2-0-fast` | `funcloud_material` 或 `none` |
| `seedance-2-mini-funcloud` | `seedance-2-mini` | `/api/v2/open/aigc/seedance2-0-mini` | `funcloud_material` 或 `none` |
| `seedance-2-5-funcloud` | `seedance-2-5` | `/api/v2/open/aigc/seedance2-5` | 只允许 `none` |

每个客户模型只对应一个启用 Channel，但一个 Channel 可以包含多个客户模型。客户模型名称、Models 和
`model_mapping` 由管理员维护；保存时按 NEWAPI 原生映射语义逐项解析 Channel Models，代码只校验每个
最终 Provider 模型属于上表登记范围。路径由映射后的 Provider 模型精确查表，不使用
`contains("fast")`、默认模型或 fallback，也不要求一个模型独占一个 Channel。

## 2. V2 视频合同

ModelArk V3 的 `content` 转为 FunCloud 富内容；支持 text/image/video/audio、`ratio`、`duration`、`resolution`、`generate_audio`、`watermark`。Standard/Fast/Mini 为 4–15 秒、图片≤3/视频≤1/音频≤1；2.5 为 4–30 秒或 `-1`、图片≤9/视频≤3/音频≤3。2.5 仅支持 480p/720p；Fast/Mini 不支持 1080p；Standard 支持至 1080p。

当前合同支持标准比例与 `adaptive`，但不开放 callback、output_format、tools、draft、priority、frames、`480pto720p` 或 Provider 私有任务类型。显式传入合同外字段必须在预扣、hold 和 Provider POST 前拒绝。

## 3. 既有素材传输与 V2 配对

`funcloud_material` 只与 Standard/Fast/Mini 配对。同一素材连接的三个模型可以配置在一个 Channel；如果
Channel 任一客户模型最终映射到 2.5，则不能选择该素材协议：

| 操作 | Provider 路径/行为 | 平台发布 |
| --- | --- | --- |
| 创建虚拟组 | `/api/v2/open/material/group/create` | 支持 |
| 查询组 | `/material/group/list` | 仅 adapter 按冻结 opaque ID 唯一匹配 |
| 删除组 | `/material/group/delete`；Provider 会级联删除组内素材 | 不发布；多人共享下游不允许通过公共 API 级联删除整组素材 |
| 上传虚拟素材 | `/material/virtual/upload` | HTTPS 安全回源、流式 multipart，≤100MB |
| 查询素材 | `/material/list`；实际列表可能省略 `assetStatus` | 单资源查询，返回 `asset://`；缺失状态仅在 `isAsset=true` 且引用合法时归一为可用 |
| 删除单素材 | `POST /api/v2/open/material/delete?materialId=...` | `code=0` 返回 204；`90003`（不存在或无权限）返回 404 `asset_not_found`；其它失败沿用上游错误映射 |
| 组更新、单素材改名、真人组 | — | `unsupported_asset_operation` |

单素材删除端点依据 2026-09-07 Provider 实测，供应商文档尚未登记；当前代码已接入，
修复版本的北向真实生命周期验收仍待执行。不使用素材组级联删除替代单素材删除。

平台不持久化 Asset/AssetGroup 或 source URL，不提供列表；视频中的 `asset://<opaque-id>` 不查询本地，直接进入 FunCloud 请求，由 Provider 判断存在性、权限和兼容性。2.5 的素材 CRUD 明确不支持。

2026-08-25 的 Provider 直测和系统 Channel 复测确认：上传响应为 `Active`，系统创建投影为 `ready`；
同一素材随后出现在 Provider 列表中并带有 `assetUrl`、`isAsset=true`，但列表省略 `assetStatus`。adapter
据此只在两个事实同时成立时把缺失状态归一为 `active`，其它缺失或未知状态继续保持 `processing`。
Provider 对非空组的级联删除虽已真实成功，平台仍按多人共享下游的保守合同完全不发布 FunCloud 素材组
删除，避免掌握 opaque 组 ID 的调用方级联删除其他调用方放入同组的素材。完整脱敏证据记录在
[Seedance 渠道素材库边界设计](../../../99-archive/2026/09/2026-08-25-Seedance渠道素材库边界设计.md)。

最新 Mini 文档已明确视频请求可以引用素材 `assetUrl`，见[原始技术文档整理](FunCloud供应商原始技术文档整理.md)。
Channel 内 3 个模型查询同一素材已经通过，只能证明控制面共享；Mini 的视频素材引用能力仍需真实
付费任务验证，不能由查询成功或相同 `reuse_scope` 推断。

## 4. V2 异步与计量

创建必须先建立 durable attempt；只有 `code=0 + data.taskId + status=processing` 才创建 Task。查询需校验 task ID、状态和唯一 HTTPS 结果。成功终态的 `data.completionTokens` 作为客户实际用量；非法、缺失、零、负数进入 reconciliation，禁止用 `pointConsume`、价格或时长替代。未知结果不重发、不换渠道、不退款。

计量边界分两层的当前事实：

- 信任上界与预扣上界分离：成功证据是否合理由按冻结探针（`resolution + duration_seconds`）推导的
  协议上界判定（实测 token 速率 × 不少于 2.4 倍余量：480p 30k/s、720p 60k/s、1080p 120k/s，时长缺省
  30 秒，下限 100k），不复用预扣预算；超过信任上界才进入合同违例，并记录可解释的脱敏 `FailReason`。
- 本地实测 token 速率（与分辨率面积成正比、跨模型一致）：480p ≈ 10.1k/s、720p ≈ 21.9k/s、
  1080p ≈ 48.7k/s。预扣上界按满规格真实用量 × 1.25–1.3 余量配置，当前部署值见
  [FunCloud 模型价格与计费](FunCloud模型价格与计费.md)。
- 查询返回 HTTP 200 + `code=30003` 是确定性“任务不存在”，映射为 not-found：轮询按连续失败计数
  （`TaskPollMaxFailures`，默认 20 次）宽限后判 `FAILURE` 并原子退款；不按合同违例无限 reconciliation。
- 实际费用高于预扣时先对冻结资金来源原子补扣，资金不足进入可重试 `debt`（业务任务保持
  `SUCCESS`）；失败终态统一原子退款，不在失败路径结算。

## 5. 代码事实

`relaykit/dto/upstream_protocol.go`、`relay/channel/task/seedance/funcloud_models.go`、`thirdparty/funcloud/`、`assets/funcloud.go` 和 `model/channel_seedance_public_catalog.go` 是唯一实现依据。

## 6. V3 统一视频与素材配对

新视频协议为 `funcloud_modelark_v3`，独立传输 profile 为 `third_party_funcloud_modelark_v3`，
冻结 adapter revision 为 `v1`。创建与查询统一使用 `/api/v3/contents/generations/tasks` 和 `/{task_id}`。
管理员精确映射到 `seedance-2-0`、`seedance-2-0-fast`、`seedance-2-0-mini` 或 `seedance-2-5`。
旧 `seedance-2` 等名称不是 V3 别名。四模型可配置在一个 Channel，配对已有 `funcloud_material` 或 `none`。

模型范围、时长、分辨率和数量唯一登记在 `relaykit/dto/funcloud_modelark_models.go`，运行时与公开投影共读。
四模型均支持 9 图、3 视频、3 音频；2.0 系列 4–15 秒、480/720p；2.5 为 4–30 秒或 -1、480/720/1080p。
第 10 张图在预扣前拒绝，URL 与 asset 图片合计计数；图片顺序、role 与 opaque 引用保持原值。

请求使用类型化 ModelArk 内容，显式 false/0 保留。未传时长时明确发送北向默认 5 秒，未传分辨率发送 720p；
音频缺省按现有 Seedance 模型规则发送 true。参考视频在南向内部设置 `omni_reference_task_type=reference`，
防止上游自动编辑模式覆盖时长；该私有字段不接受客户透传。当前发布参数范围沿用既有验证，另支持 mp4/mov。

创建只接受顶层可信 id；查询识别 submitted/running/succeeded/failed，验证 ID 对应关系与 HTTPS 视频 URL。
终态 Token 使用现有通用归一逻辑，保留来源及用量证据；缺失用量不制造 Token，也不改变视频成功状态。
按现有表达式与创建时冻结探针结算。纯参数表达式无需 Token 预扣上限，依赖 Token 的表达式仍要求配置上限。

任务内容读取可用，取消排队及删除终态任务不支持，不继承官方渠道生命周期能力。任务列表仍来自本地用户与
应用隔离的 Task。旧任务继续按冻结 V2 连接、协议和价格查询，V3 请求不回退 V2。

素材传输、上传限制、组策略与第 3 节相同，只有 V3 的四模型配对范围扩展。平台不增加 Asset 表或 resolver。
改变视频协议仍触发租户替换确认；确认后生成新 identity/reuse_scope 并清除本地默认组关联，管理员可重新关联
确认同域的原 Provider 组。Provider 原组、素材和引用均不删除、不重建、不重传。

当前 FunCloud 价格继续适用（用户确认），独立测试模型复制对应旧客户模型的价格配置。真实四模型生成、
统一素材引用、九图及账单验收记录在[归档实施方案](../../../99-archive/2026/09/2026-09-07-FunCloud新版视频协议与统一素材库实施方案.md)。


### Mini 素材引用验收边界

当前 V3 的 Mini 素材引用仍未通过真实验收：使用 Mini 自己创建的独立素材组与图片素材，生成前后
通过 Mini 查询均为 ready，但视频生成侧返回 InvalidParameter、素材不存在。修复版后台已将该实际
失败任务结算并全额退回客户预扣；供应商账单未核验。素材控制面 ready 不构成生成侧可用的证明，
不得据此宣称 Mini 素材生成已发布，也不得增加隐式重传或跨渠道 fallback。具体证据见
[归档实施记录第 13 节](../../../99-archive/2026/09/2026-09-07-FunCloud新版视频协议与统一素材库实施方案.md)。


### Mini 同图对照的验收结论

新建独立素材组和真实图片素材后，Mini 素材引用生成及一次明确失败后的重试均报素材不存在，
失败前后同一素材查询仍为 ready；相同图片的普通 URL 独立生成成功并完成后台结算。
现有证据将问题限定在素材引用链路，尚不能区分供应商内部素材域、权限或模型绑定原因。
普通 URL 对照不代表素材库验收通过，不改变 adapter 原样传递引用、禁止自动 fallback 的合同。
成功视频的 generate_audio=false 仍有非静音 AAC 音轨；音频开关和供应商账单验收仍未完成。
具体任务、费用与媒体检查见实施方案第 14 节。
