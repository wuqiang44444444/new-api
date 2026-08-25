---
status: current
owner: Dev Team
last-reviewed: 2026-08-25
---

# FunCloud 模型与素材库对接设计

## 1. 身份与模型映射

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

## 2. 视频合同

ModelArk V3 的 `content` 转为 FunCloud 富内容；支持 text/image/video/audio、`ratio`、`duration`、`resolution`、`generate_audio`、`watermark`。Standard/Fast/Mini 为 4–15 秒、图片≤3/视频≤1/音频≤1；2.5 为 4–30 秒或 `-1`、图片≤9/视频≤3/音频≤3。2.5 仅支持 480p/720p；Fast/Mini 不支持 1080p；Standard 支持至 1080p。

当前合同支持标准比例与 `adaptive`，但不开放 callback、output_format、tools、draft、priority、frames、`480pto720p` 或 Provider 私有任务类型。显式传入合同外字段必须在预扣、hold 和 Provider POST 前拒绝。

## 3. 素材库对接

`funcloud_material` 只与 Standard/Fast/Mini 配对。同一素材连接的三个模型可以配置在一个 Channel；如果
Channel 任一客户模型最终映射到 2.5，则不能选择该素材协议：

| 操作 | Provider 路径/行为 | 平台发布 |
| --- | --- | --- |
| 创建虚拟组 | `/api/v2/open/material/group/create` | 支持 |
| 查询组 | `/material/group/list` | 仅 adapter 按冻结 opaque ID 唯一匹配 |
| 删除组 | `/material/group/delete`；Provider 会级联删除组内素材 | 不发布；多人共享下游不允许通过公共 API 级联删除整组素材 |
| 上传虚拟素材 | `/material/virtual/upload` | HTTPS 安全回源、流式 multipart，≤100MB |
| 查询素材 | `/material/list`；实际列表可能省略 `assetStatus` | 单资源查询，返回 `asset://`；缺失状态仅在 `isAsset=true` 且引用合法时归一为可用 |
| 组更新、单素材改名/删除、真人组 | — | `unsupported_asset_operation` |

平台不持久化 Asset/AssetGroup 或 source URL，不提供列表；视频中的 `asset://<opaque-id>` 不查询本地，直接进入 FunCloud 请求，由 Provider 判断存在性、权限和兼容性。2.5 的素材 CRUD 明确不支持。

2026-08-25 的 Provider 直测和系统 Channel 复测确认：上传响应为 `Active`，系统创建投影为 `ready`；
同一素材随后出现在 Provider 列表中并带有 `assetUrl`、`isAsset=true`，但列表省略 `assetStatus`。adapter
据此只在两个事实同时成立时把缺失状态归一为 `active`，其它缺失或未知状态继续保持 `processing`。
Provider 对非空组的级联删除虽已真实成功，平台仍按多人共享下游的保守合同完全不发布 FunCloud 素材组
删除，避免掌握 opaque 组 ID 的调用方级联删除其他调用方放入同组的素材。完整脱敏证据记录在
[Seedance 渠道素材库边界设计](../../../80-dev/2026-08-25-Seedance渠道素材库边界设计.md)。

当前 Provider 文档只明确 Standard/Fast 可以在视频请求中使用素材 `assetUrl`，没有给出 Mini 的相同
声明。Channel 内 3 个模型查询同一素材已经通过，只能证明控制面共享；Mini 的视频素材引用能力仍需真实
付费任务验证，不能由查询成功或相同 `reuse_scope` 推断。

## 4. 异步与计量

创建必须先建立 durable attempt；只有 `code=0 + data.taskId + status=processing` 才创建 Task。查询需校验 task ID、状态和唯一 HTTPS 结果。成功终态的 `data.completionTokens` 作为客户实际用量；非法、缺失、零、负数或超预扣上界进入 reconciliation，禁止用 `pointConsume`、价格或时长替代。未知结果不重发、不换渠道、不退款。

## 5. 代码事实

`relaykit/dto/upstream_protocol.go`、`relay/channel/task/seedance/funcloud_models.go`、`thirdparty/funcloud/`、`assets/funcloud.go` 和 `model/channel_seedance_public_catalog.go` 是唯一实现依据。
