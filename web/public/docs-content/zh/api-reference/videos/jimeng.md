---
page-id: videos-jimeng
kind: api-reference
last-verified: 2026-08-28
operations:
  - createJimengVideo
---

# 即梦视频

即梦使用一个 `POST /jimeng/` 入口，并通过查询参数 `Action` 区分创建和查询。`Action`、`Version`、请求
字段和响应信封都是合同的一部分，不能改用 Kling、ModelArk 或 OpenAI Videos 字段。

当前内置适配器登记的 `req_key` 为 `jimeng_vgfm_t2v_l20`。实际可用范围仍以 `GET /v1/models` 和当前
API Key 权限为准。本合同不提供任务列表、删除、取消或平台内容下载接口。

## 提交任务

`POST /jimeng/?Action=CVSync2AsyncSubmitTask&Version=2022-08-31`

```bash
curl "{{SITE_BASE_URL}}/jimeng/?Action=CVSync2AsyncSubmitTask&Version=2022-08-31" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: jimeng-example-001" \
  -d '{
    "req_key": "jimeng_vgfm_t2v_l20",
    "prompt": "阳光穿过窗帘，房间里的植物轻轻摇曳",
    "seed": 12345,
    "aspect_ratio": "16:9",
    "frames": 121
  }'
```

### 提交参数

| 参数 | 位置/类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `Action` | query string | 是 | 创建固定为 `CVSync2AsyncSubmitTask` |
| `Version` | query string | 是 | 固定为 `2022-08-31` |
| `req_key` | body string | 是 | 客户模型标识；当前内置值为 `jimeng_vgfm_t2v_l20` |
| `prompt` | body string | 是 | 视频描述，去除首尾空白后不能为空 |
| `binary_data_base64` | body string[] | 否 | Base64 图片数组；仅在模型支持图片输入时使用 |
| `image_urls` | body string[] | 否 | 图片 URL 或 `asset://` 引用数组；仅在模型支持图片输入时使用 |
| `seed` | body integer | 否 | 随机种子；允许范围由当前模型合同决定 |
| `aspect_ratio` | body string | 否 | 输出画幅；允许值由当前模型合同决定 |
| `frames` | body integer | 否 | 输出帧数；省略时当前适配器使用 `121`，显式值按模型合同校验 |

请求采用严格字段白名单，任何未列出的顶层字段都会返回 `400`。`image_urls` 和
`binary_data_base64` 是两种图片传输方式；除非当前模型明确支持，不要同时发送，也不要把视频入口的
`duration`、`size` 或 `content` 混入本请求。

### 创建响应

HTTP `200`：

```json
{
  "code": 10000,
  "message": "Success",
  "request_id": "req-placeholder",
  "status": 10000,
  "data": {
    "task_id": "task-public-id"
  }
}
```

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `code` | integer | `10000` 表示 API 操作成功 |
| `status` | integer | 成功时同为 `10000` |
| `message` | string | API 操作消息 |
| `request_id` | string | 请求追踪 ID |
| `data.task_id` | string | 平台任务 ID；立即保存并用于查询 |

## 查询结果

查询仍使用 POST，但 Action 和请求体不同：

```bash
curl "{{SITE_BASE_URL}}/jimeng/?Action=CVSync2AsyncGetResult&Version=2022-08-31" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  -H "Content-Type: application/json" \
  -d '{"task_id":"task-public-id"}'
```

查询请求体只允许 `task_id`：

| 参数 | 位置/类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `Action` | query string | 是 | 查询固定为 `CVSync2AsyncGetResult` |
| `Version` | query string | 是 | 固定为 `2022-08-31` |
| `task_id` | body string | 是 | 创建响应中的平台任务 ID |

成功任务响应：

```json
{
  "code": 10000,
  "message": "Success",
  "request_id": "req-placeholder",
  "status": 10000,
  "data": {
    "task_id": "task-public-id",
    "status": "done",
    "video_url": "https://example.com/generated-video.mp4"
  }
}
```

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `code` / `status` | integer | `10000` 表示查询操作成功，不等于任务已经完成 |
| `message` | string | API 操作消息 |
| `request_id` | string | 本次查询的追踪 ID |
| `data.task_id` | string | 平台任务 ID |
| `data.status` | string | `in_queue`、`generating`、`done` 或 `failed` |
| `data.video_url` | string | `done` 时返回的视频地址；应及时下载或转存 |

`in_queue` 和 `generating` 使用退避轮询；`done` 或 `failed` 后停止。单次查询异常不能证明任务失败，
可保留任务 ID 后再次查询。

## 幂等、计费与重试

提交 Action 支持可选 `Idempotency-Key`，最大 `191` 个字符，同一键在 `24` 小时内应绑定同一请求。
相同请求可恢复同一个任务；同一键配不同请求或原结果仍待核查时返回 `409`。查询 Action 不使用幂等键。

创建结果不明时平台不自动重发、换渠道或退款。网络中断后使用原幂等键恢复，不能换新键盲目提交。
任务费用使用创建时冻结的客户模型和计费事实。

## 错误响应

```json
{
  "code": 50200,
  "data": null,
  "message": "Invalid request body",
  "request_id": "req-placeholder",
  "status": 50200
}
```

| HTTP 状态 | `code` / `status` | 含义 |
| --- | --- | --- |
| `400` | `50200` | Action、Version、JSON、字段、类型或必填项无效 |
| `401` / `403` | `50400` | API Key、模型或分组权限错误 |
| `404` | `50200` | 当前调用方下没有该任务 |
| `429` | `50430` | 请求过多或额度限制 |
| `5xx` | `50500` | 平台或上游失败；创建结果可能需要核查 |

HTTP 非 `2xx` 时先按 HTTP 状态处理。错误消息已经脱敏，不应从中推断上游 Provider 或内部渠道。
