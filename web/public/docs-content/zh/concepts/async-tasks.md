---
page-id: async-tasks
kind: guide
last-verified: 2026-09-16
operations: []
---

# 异步任务

异步调用分为创建、查询、获取结果三步。创建接口的 HTTP 成功只表示任务已受理，不表示媒体已生成。
图片默认在本次请求内返回；模型公开异步能力且请求携带 `Prefer: respond-async` 时，才可能返回图片任务。

## 按接口选择生命周期

| 接口                        | 创建成功                   | 查询入口                                           | 成功状态         |
| --------------------------- | -------------------------- | -------------------------------------------------- | ---------------- |
| 图片生成 / 编辑（同步）     | `200`，`data[]`            | 无平台任务查询                                     | 直接读取图片结果 |
| 图片生成 / 编辑（显式异步） | `202`，`id` 和 `query_url` | `GET /v1/tasks/{task_id}`                          | `succeeded`      |
| ModelArk V3 视频            | `200`，`id`                | `GET /api/v3/contents/generations/tasks/{task_id}` | `succeeded`      |
| OpenAI Videos               | 任务对象                   | `GET /v1/videos/{video_id}`                        | `completed`      |

Kling、即梦和[通用视频生成](api-reference/videos/generations)使用各自的任务响应与查询路径，详见对应 API Reference。不要将不同协议的 ID、字段或状态名
混用。素材接口返回素材或素材组 ID，使用自己的 `processing` / `ready` / `failed` 状态。

## 图片异步与流式的区别

- 同步 JSON：HTTP 请求保持连接，返回最终 `data[]`。
- SSE 流式：同一连接接收部分图和完成事件；不因为流式就获得平台任务 ID。
- 显式异步：收到 `202` 后保存 ID，断开连接仍继续执行，使用 GET 查询。

图片模型不支持平台异步时可能忽略偏好并返回同步结果。声明 `api.image.async` 的模型收到
`Prefer: respond-async` 后选择平台任务，通过参数、资金及存储等受理检查后返回 `202`。
OpenAI／Azure 原生图片入口同时传 `stream=true` 时由后台接收上游结果，创建连接不返回 SSE；
Gemini／Vertex／图片中转入口仍拒绝两者同时使用。`stream_priority=false` 不代表支持流式参数。
先检查模型声明，并以实际响应状态和对象类型确认是否受理。

## 轮询流程

1. 创建成功后立即保存任务 ID、所用接口、客户模型和 API Key 的内部标识，不记录 Key 明文。
2. 先等待约 2 秒再查询；只查询该协议对应的路径。
3. 活动状态下使用退避并加入抖动，设置总等待时限。
4. 成功后保存结果；失败、取消或过期后停止轮询并读取公开错误。
5. 客户端等待到期或查询遇到临时错误时保留原 ID，稍后继续查询。
6. 图片 `unknown` 或创建结果不明时停止自动创建，联系管理员核实。

查询返回 `200` 仍需检查业务 `status`。ModelArk 的处理中状态为 `running`，图片为 `in_progress`；
不能只写一套未经区分的状态判断。完整脚本见[图片与视频调用实战](guides/media-workflow)。

## 重试与幂等

图片只有实际异步模式支持 `Idempotency-Key`：同一 API Key、同一操作、相同正文和原键用于确认同次受理。
`409 idempotency_conflict` 需检查请求差异；`409 idempotency_in_progress` 需等待原受理完成。
换键意味着新的生成意图。同步和流式不提供这项任务幂等保证。

ModelArk V3 创建不提供客户幂等键。`create_outcome_unknown` 或创建超时后不要自动重发 POST；
保存公开请求 ID 并核实原请求。已有 ID 时只重试 GET，不通过重新创建来“查询”结果。

## 权限与交付

图片与 ModelArk 任务按用户和创建时的应用（API Key）隔离。同账号其他 Key 也不能直接查询原任务。
`404` 时应同时核对 ID、接口族和创建时的 Key。

视频内容代理需要成功任务和原鉴权主体。图片任务的结果 URL 有效 300 秒，过期重新查询续签；
下载签名图片 URL 时无需附带平台 Key。下载前检查状态和媒体类型，不要把错误 JSON 当成媒体文件。

成功生成与最终结算可能分开完成。视频成功但未返回用量不代表免费；依赖用量的计费可以等待补齐后再结算，
无需为补齐用量重发生成。图片逐项为 `deleted` 或 `unavailable` 也不改变历史生成状态。
