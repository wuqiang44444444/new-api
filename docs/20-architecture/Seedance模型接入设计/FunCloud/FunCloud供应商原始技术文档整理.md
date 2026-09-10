---
status: current
owner: Dev Team
last-reviewed: 2026-09-07
---

# FunCloud 供应商原始技术文档整理

## 资料范围与证据层级

2026-09-07 阅读供应商在线文档，以下是重新组织的接口摘要，不是原文镜像：

- [Seedance 2.0 Mini V2](https://docs.leonecloud.com/docs/seedance-2-0-mini/)：页面更新于 2026-09-06。
- [Seedance 全系 V3](https://docs.leonecloud.com/docs/seedance-2-5-v3-protocol)：页面更新于 2026-09-06。虽然 URL 含 2.5，正文明确覆盖 2.0、Fast、Mini 和 2.5。
- [Seedance 2.5 V2](https://docs.leonecloud.com/docs/seedance-2-5)：页面更新于 2026-09-02。

本文区分供应商已公开的协议与平台当前实现。旧协议 `funcloud_seedance` 仍走 V2；新增 `funcloud_modelark_v3` 已完成本地实现和部分真实生成验收。
旧离线资料中的数量与价格不能覆盖新版协议说明；真实验收范围以 80-dev 实施记录为准。

## V3 统一接口

Base URL 为 `https://mm-internal-cn.leonecloud.com`，使用 Bearer API Key 和 JSON 请求体。

| 操作 | 方法与路径 | 返回结构 |
| --- | --- | --- |
| 创建 | `POST /api/v3/contents/generations/tasks` | HTTP 200，顶层 `id` |
| 单项查询 | `GET /api/v3/contents/generations/tasks/{id}` | 顶层任务对象 |
| 列表查询 | `GET /api/v3/contents/generations/tasks` | `items` 与 `total` |

V3 用必填 `model` 选择模型，替代 V2 的逐模型创建路径。普通生成的请求结构共用，字段使用 snake_case。

| V3 Provider model | 分辨率 | 秒数范围 | 默认秒数 |
| --- | --- | --- | --- |
| `seedance-2-0` | 480p / 720p | 4–15 | 5 |
| `seedance-2-0-fast` | 480p / 720p | 4–15 | 5 |
| `seedance-2-0-mini` | 480p / 720p | 4–15 | 5 |
| `seedance-2-5` | 480p / 720p / 1080p | 4–30，另支持 -1 | -1 |

以上模型标识是供应商 V3 的枚举，不是部署方必须公开的客户模型名。现有 V2 代码登记的
`seedance-2`、`seedance-2-fast`、`seedance-2-mini` 不能直接作为 V3 枚举发送。

## 输入与默认值

`content` 必须包含文字项，提示词长度为 3–20000 字符；图片、视频、音频使用各自的 URL 对象。
图片 role 为 `reference_image`、`first_frame` 或 `last_frame`，视频与音频分别用 `reference_video`、
`reference_audio`。V3 在线通用表（2026-09-10 核查）给出的上限为图片 30 张、视频 10 个、音频 10 个，适用于全部四模型。
因此“2.0 只能三张”不能作为新版 V3 的供应商事实；也不能把新版上限未经验证直接套用到旧 V2 入口。

| 媒体 | V3 文档约束 |
| --- | --- |
| 图片 | JPEG/PNG/WebP/BMP/TIFF/GIF；宽高比 0.4–2.5；尺寸 300–6000px；最大 30MB |
| 视频 | MP4/MOV；480p/720p；2–15 秒；最大 50MB；24–60FPS |
| 音频 | WAV/MP3；2–15 秒；最大 15MB |

默认值：`ratio=adaptive`、`resolution=720p`、`generate_audio=true`、`output_format=mp4`、
`watermark=false`。`duration` 默认值见模型表。与 V2 Mini 的 `ratio=16:9`、`generateAudio=false`
不同，迁移不能靠省略字段来假定行为一致。

## 条件语义与扩展字段

- 首尾帧场景会使比例跟随输入素材，不能承诺传入的 `ratio` 始终生效。
- `frames` 范围 29–289，文档说明同时传入时优先于 `duration`；计费秒数取 `floor(frames/24)`，最低 1 秒。
- `omni_reference_task_type` 支持 `auto/reference/edit/extend`。带参考视频且未指定类型时自动进入 `auto`，
  文档说明此时按编辑处理并强制 `ratio=adaptive`、`duration=-1`；强制智能时长的场景仅适用 2.5。
  因此 2.0 的参考视频不能仅因通用输入表存在就认定全部组合已可用。
- V3 还列出 `output_format`、`return_last_frame`、`callback_url`、`priority`、`service_tier`、
  `execution_expires_after`、`safety_identifier`、`draft`、`tools`、`task_nickname`、`real_person_mode`。
  这些是供应商字段，不等于平台已发布字段；不得让调用者透传合同外私有字段。

V3 统一文档的参考音视频时长为 2–15 秒，而最新 2.5 V2 页面写 2–30 秒。两者应按协议分别登记，
不得用一个混合范围替代。V3 的分辨率表也不同于旧 V2 本地登记：2.0 不含 1080p，2.5 包含 1080p。

## 响应、状态与计费证据

单项查询状态为 `submitted/running/succeeded/failed`；成功结果在 `content.video_url`，错误在
`error.code/error.message`。恒定字段为 `id/model/status/created_at/updated_at`；视频参数、尾帧及
`usage` 按存在性解析。`usage.completion_tokens` 仅在结算完成或有实际用量时返回，缺失不等于零。

创建/查询错误使用 HTTP 状态码与 `error` 对象：400 参数错误、401 鉴权错误、402 余额不足、403 无权限、
404 未找到、500 服务错误。不能沿用 V2 的 `code/data.taskId/data.result` 解码结构。

列表支持重复的 `filter.task_ids`、`filter.status`、`page_num`、`page_size`。列表过滤状态写为
`queued/running/succeeded/failed`，与单项的 `submitted` 命名不完全一致，不能据此新增北向公共状态。
回调仍使用 `taskId/result` 等不同于查询的字段，不能复用查询解析器。

V3 文档将 2.0 系列描述为按秒计费，将 2.5 智能时长描述为按 Token 计费；固定秒数的精确费率需以账号报价
和真实账单确认。Mini V2 文档同样说明按秒任务可能没有 `completionTokens`。平台可按实际 Token 结算；用户已确认本次沿用当前 FunCloud 价格，测试模型复制对应原表达式。
文档本身不能证明账号实际费率，不得用时长伪造 Token。具体迁移边界见[对接设计](FunCloud模型与素材库对接设计.md)。

## 素材与统一接入边界

最新 Mini V2 文档明确支持可信素材的 `assetUrl`（`asset://...`），包括真人认证素材与虚拟素材；这补足了旧
资料对 Mini 引用的描述缺口，但不替代真实视频引用验收。`assetUrl` 与素材的 OSS `fileUrl` 语义不同。

V3 全系统一的视频端点不构成“全系共享素材域或素材 CRUD”的证据。旧 V2 代码禁止 2.5 与 `funcloud_material`
组合；新增 V3 已按用户确认的共享素材设计前提允许四模型配对。本次读取的页面本身不足以证明共享
素材兼容性，真实跨模型引用验收仍未完成。真人模式是供应商生成扩展，不能代替平台的无状态素材合同。

## 调用示例与平台状态

可复制的请求示例见 [FunCloud Seedance 统一调用说明](../../../60-marketing/seedance模型对账/FunCloud视频统一调用说明.md)。
该示例明确区分供应商 V3 与当前本地入口，使用占位凭据和图片 URL，不包含真实签名地址。
平台代码仍以[对接设计](FunCloud模型与素材库对接设计.md)、[能力元数据](FunCloud模型与素材能力元数据.md)
和[价格与计费](FunCloud模型价格与计费.md)中按 V2/V3 分别标注的实现事实为准。
