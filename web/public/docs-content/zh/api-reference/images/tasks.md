---
page-id: images-tasks
kind: api-reference
last-verified: 2026-09-09
operations:
  - retrieveImageTask
---

# 图片任务查询

`GET /v1/tasks/{task_id}` · Bearer 鉴权

用于查询图片生成或编辑通过 `Prefer: respond-async` 返回 `202` 后的任务。
同步 `200` 和 SSE 响应不建立可供本接口查询的图片任务。创建方式见
[图片生成](api-reference/images/generations)与[图片编辑](api-reference/images/edits)。

## 查询请求

把创建响应的 `id` 替换到下面路径中，并使用创建时的同一 API Key：

```bash
curl "{{OPENAI_BASE_URL}}/tasks/task_xxxxxxxx" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}"
```

`{{OPENAI_BASE_URL}}` 已包含 `/v1`。如果直接使用创建响应中的
`query_url=/v1/tasks/task_xxxxxxxx` 或 `Location`，应与 `{{SITE_BASE_URL}}` 拼接。
不存在 `/v1/images/tasks/{task_id}` 这一图片查询路径。

| 参数 | 位置 | 必填 | 说明 |
| --- | --- | --- | --- |
| `task_id` | path | 是 | 创建响应的 `id`，不能使用其他接口的资源 ID |
| `Authorization` | header | 是 | `Bearer` 加创建任务的 API Key |

任务按用户与应用（API Key）隔离；即使同账号，更换 Key 查询其他应用的任务也返回 `404`。
不需要再次传入模型、提示词或参考图。查询参数不能改变创建时选择的结果格式。

`/v1/tasks/{task_id}` 是共享查询入口。本页只描述图片任务投影；先确认返回的 `object=image_task`
以及 `id` 与原任务一致，再按本页解析。其他任务可能返回不同形状，不保证因类型不同而返回 `404`。

## 响应示例

排队中的任务返回 HTTP `200`，此时通常没有 `data`：

```json
{
  "id": "task_xxxxxxxx",
  "object": "image_task",
  "status": "queued",
  "created_at": 1785207890
}
```

成功并返回 URL 的任务示例，域名和地址仅为占位值：

```json
{
  "id": "task_xxxxxxxx",
  "object": "image_task",
  "status": "succeeded",
  "created_at": 1785207890,
  "finished_at": 1785207950,
  "image_count": 1,
  "data": [
    {
      "status": "available",
      "mime_type": "image/png",
      "url": "https://example.com/result.png",
      "url_expires_at": 1785208250
    }
  ]
}
```

## 响应字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | string | 平台任务 ID |
| `object` | string | 固定 `image_task` |
| `status` | string | `queued`、`in_progress`、`succeeded`、`failed`、`expired`、`unknown` |
| `created_at` | integer | 受理时间，Unix 秒 |
| `finished_at` | integer | 终态时间，未完成时可省略 |
| `image_count` | integer | 已记录的图片数量；无结果时可省略，不代表每张图片当前都可下载 |
| `data` | array | 已保存的逐张结果；尚无登记结果时可省略，`unknown` 期间也可能有部分结果 |
| `data[].status` | string | `available`、`deleted`、`unavailable`；仅 `available` 携带下载内容 |
| `data[].mime_type` | string | 可选图片 MIME，例如 `image/png` |
| `data[].url` | string | 300 秒有效的签名下载地址；再次查询可获得新地址 |
| `data[].url_expires_at` | integer | 该 URL 的到期时间，Unix 秒 |
| `data[].b64_json` | string | 创建时显式选择 `response_format=b64_json` 的图片原文；须先确认该创建参数被模型支持 |
| `error` | object | 失败、过期或 `unknown` 时的脱敏错误，含 `code` / `message` |

HTTP `200` 表示查询成功，不表示生成成功，必须继续检查 `status`。

## 状态与轮询动作

| 任务状态 | 含义 | 下一步 |
| --- | --- | --- |
| `queued` | 已受理，等待执行 | 保留 ID，退避查询 |
| `in_progress` | 正在执行或保存结果 | 继续查询，不重复创建 |
| `succeeded` | 生成完成 | 逐项检查 `data[].status` 并保存可用图片 |
| `failed` | 已确认失败 | 停止生成状态轮询，读取公开错误；失败费用按退款流程退还 |
| `expired` | 任务已过期 | 停止轮询，读取公开错误；不要通过重发查询创建新任务 |
| `unknown` | 执行结果或结果保存待核实 | 保存已有结果和任务 ID，联系管理员；不能自动重发或当作已退款 |

建议从 2 秒开始退避到最多 10～30 秒，并设置客户端总等待上限。等待到期只停止本地轮询，不取消任务，
可稍后继续查询同一 ID。可直接使用[调用实战中的 Python 轮询示例](guides/media-workflow)。

一次 GET 网络异常或 `5xx` 不证明任务失败；可以有限重试 GET。未知状态应保留 ID 并停止自动创建，
不要自行归类为失败。

## 结果保存与地址过期

生成状态和图片当前可访问性是两回事，必须逐张处理：

| 图片状态 | 含义与处理 |
| --- | --- |
| `available` | 使用返回的 `url` 下载，或解码 `b64_json` |
| `deleted` | 存储已确认不存在该图片；历史生成状态保留，重新查询不会重生成 |
| `unavailable` | 当前暂时无法读取；不等于删除，稍后重新查询 |

URL 自签发起有效 300 秒，应及时下载到自己的存储。过期后重新调用本接口取新 URL；不要重发生成请求。
签名图片 URL 已包含下载授权，下载时不要附带平台 API Key，也不要将完整地址写入日志。

Base64 内容是原始图片编码，不含 Data URL 前缀。逐张下载、格式校验与地址续签的可运行示例见
[图片结果保存](guides/image-results)。

`unknown` 期间也可能返回已保存的部分图片。它们可以下载，但不能据此把整个任务判断为成功。
退款或结算以任务实际处理结果为准，不要用数组长度自行推算最终费用。

## 查询错误

| HTTP 状态 | 含义 | 处理 |
| --- | --- | --- |
| `401` / `403` | 鉴权失败或无权访问 | 检查 Key 与权限 |
| `404` | 图片任务不存在或不属于当前应用 | 核对原始 ID 和创建时的 Key |
| `429` | 查询限流 | 退避后继续 GET |
| `5xx` | 查询服务暂不可用 | 保留 ID，有限重试 GET |

任务自身的 `failed` / `unknown` 通常仍随 HTTP `200` 返回；不要只用 HTTP 状态判断业务结果。
