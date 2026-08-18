---
status: current
owner: Dev Team
last-reviewed: 2026-08-18
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

每个客户模型只对应一个启用 Channel；路径由代码精确表查得，不使用 `contains("fast")`、默认模型或 fallback。

## 2. 视频合同

ModelArk V3 的 `content` 转为 FunCloud 富内容；支持 text/image/video/audio、`ratio`、`duration`、`resolution`、`generate_audio`、`watermark`。Standard/Fast/Mini 为 4–15 秒、图片≤3/视频≤1/音频≤1；2.5 为 4–30 秒或 `-1`、图片≤9/视频≤3/音频≤3。2.5 仅支持 480p/720p；Fast/Mini 不支持 1080p；Standard 支持至 1080p。

当前合同支持标准比例与 `adaptive`，但不开放 callback、output_format、tools、draft、priority、frames、`480pto720p` 或 Provider 私有任务类型。显式传入合同外字段必须在预扣、hold 和 Provider POST 前拒绝。

## 3. 素材库对接

`funcloud_material` 只与 Standard/Fast/Mini 配对：

| 操作 | Provider 路径/行为 | 平台发布 |
| --- | --- | --- |
| 创建虚拟组 | `/api/v2/open/material/group/create` | 支持 |
| 查询组 | `/material/group/list` | 仅 adapter 按冻结 opaque ID 唯一匹配 |
| 删除组 | `/material/group/delete` | 仅 Provider 明确 `materialCount=0` |
| 上传虚拟素材 | `/material/virtual/upload` | HTTPS 安全回源、流式 multipart，≤100MB |
| 查询素材 | `/material/list` | 单资源查询，返回 `asset://` |
| 组更新、单素材改名/删除、真人组 | — | `unsupported_asset_operation` |

平台不持久化 Asset/AssetGroup 或 source URL，不提供列表；视频中的 `asset://<opaque-id>` 不查询本地，直接进入 FunCloud 请求，由 Provider 判断存在性、权限和兼容性。2.5 的素材 CRUD 明确不支持。

## 4. 异步与计量

创建必须先建立 durable attempt；只有 `code=0 + data.taskId + status=processing` 才创建 Task。查询需校验 task ID、状态和唯一 HTTPS 结果。成功终态的 `data.completionTokens` 作为客户实际用量；非法、缺失、零、负数或超预扣上界进入 reconciliation，禁止用 `pointConsume`、价格或时长替代。未知结果不重发、不换渠道、不退款。

## 5. 代码事实

`relaykit/dto/upstream_protocol.go`、`relay/channel/task/seedance/funcloud_models.go`、`thirdparty/funcloud/`、`assets/funcloud.go` 和 `model/channel_seedance_public_catalog.go` 是唯一实现依据。

