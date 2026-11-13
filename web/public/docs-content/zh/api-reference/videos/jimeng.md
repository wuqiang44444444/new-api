---
page-id: videos-jimeng
kind: api-reference
last-verified: 2026-09-09
operations:
  - createJimengVideo
---

# 即梦视频

即梦使用一个 `POST /jimeng/` 入口，并通过查询参数 `Action` 区分创建和查询。`Action`、`Version`、请求
字段和响应信封都是合同的一部分，不能改用 Kling、ModelArk 或 OpenAI Videos 字段。

`req_key` 填写模型目录返回、且支持本页入口的客户模型 ID。不要把上游示例中的固定值直接复制到请求中。本合同不提供任务列表、删除、取消或平台内容下载接口。

## 提交任务

`POST /jimeng/?Action=CVSync2AsyncSubmitTask&Version=2022-08-31`

```bash
curl "{{SITE_BASE_URL}}/jimeng/?Action=CVSync2AsyncSubmitTask&Version=2022-08-31" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: jimeng-example-001" \
  -d '{
    "req_key": "{{MODEL_ID_PLACEHOLDER}}",
    "prompt": "阳光穿过窗帘，房间里的植物轻轻摇曳",
    "seed": 12345,
    "aspect_ratio": "16:9",
    "frames": 121
  }'
```

### 提交参数

| 参数                 | 位置/类型     | 必填 | 说明                                                                |
| -------------------- | ------------- | ---- | ------------------------------------------------------------------- |
| `Action`             | query string  | 是   | 创建固定为 `CVSync2AsyncSubmitTask`                                 |
| `Version`            | query string  | 是   | 固定为 `2022-08-31`                                                 |
| `req_key`            | body string   | 是   | 模型目录返回的客户模型 ID，填入 `{{MODEL_ID_PLACEHOLDER}}` 所在位置 |
| `prompt`             | body string   | 是   | 视频描述，去除首尾空白后不能为空                                    |
| `binary_data_base64` | body string[] | 否   | Base64 图片数组；仅在模型支持图片输入时使用                         |
| `image_urls`         | body string[] | 否   | 图片 URL 或 `asset://` 引用数组；仅在模型支持图片输入时使用         |
| `seed`               | body integer  | 否   | 随机种子；允许范围由当前模型合同决定                                |
| `aspect_ratio`       | body string   | 否   | 输出画幅；允许值由当前模型合同决定                                  |
| `frames`             | body integer  | 否   | 输出帧数；默认 `121`，显式值按模型合同校验          |

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

| 字段           | 类型    | 说明                            |
| -------------- | ------- | ------------------------------- |
| `code`         | integer | `10000` 表示 API 操作成功       |
| `status`       | integer | 成功时同为 `10000`              |
| `message`      | string  | API 操作消息                    |
| `request_id`   | string  | 请求追踪 ID                     |
| `data.task_id` | string  | 平台任务 ID；立即保存并用于查询 |

## 查询结果

查询仍使用 POST，但 Action 和请求体不同：

```bash
curl "{{SITE_BASE_URL}}/jimeng/?Action=CVSync2AsyncGetResult&Version=2022-08-31" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  -H "Content-Type: application/json" \
  -d '{"task_id":"task-public-id"}'
```

查询请求体只允许 `task_id`：

| 参数      | 位置/类型    | 必填 | 说明                               |
| --------- | ------------ | ---- | ---------------------------------- |
| `Action`  | query string | 是   | 查询固定为 `CVSync2AsyncGetResult` |
| `Version` | query string | 是   | 固定为 `2022-08-31`                |
| `task_id` | body string  | 是   | 创建响应中的平台任务 ID            |

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

| 字段              | 类型    | 说明                                         |
| ----------------- | ------- | -------------------------------------------- |
| `code` / `status` | integer | `10000` 表示查询操作成功，不等于任务已经完成 |
| `message`         | string  | API 操作消息                                 |
| `request_id`      | string  | 本次查询的追踪 ID                            |
| `data.task_id`    | string  | 平台任务 ID                                  |
| `data.status`     | string  | `in_queue`、`generating`、`done` 或 `failed` |
| `data.video_url`  | string  | `done` 时返回的视频地址；应及时下载或转存    |

`in_queue` 和 `generating` 使用退避轮询；`done` 或 `failed` 后停止。单次查询异常不能证明任务失败，
可保留任务 ID 后再次查询。

## 幂等、计费与重试

创建可传 `Idempotency-Key`，去除首尾空白后最多 191 个 UTF-8 字节。每次新生成使用新键。
重试同次创建时，沿用原幂等键、API Key、入口和请求正文。查询无需幂等键。

- 可重放时返回原任务。处理中、待核查、请求冲突或不可重放时返回 `409`，按响应处理。
- 已完成的幂等记录保留 24 小时，过期后同键可能创建新任务。仍在处理或待核查的请求应继续核查。
- 创建结果不明时，已有任务 ID 就查询原任务，否则保留请求 ID 和幂等键核查。不要换键重发或假定已退款。

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

| HTTP 状态     | `code` / `status` | 含义                                          |
| ------------- | ----------------- | --------------------------------------------- |
| `400`         | `50200`           | Action、Version、JSON、字段、类型或必填项无效 |
| `401` / `403` | `50400`           | API Key、模型或分组权限错误                   |
| `404`         | `50200`           | 当前调用方下没有该任务                        |
| `429`         | `50430`           | 请求过多或额度限制                            |
| `5xx`         | `50500`           | 平台或上游失败；创建结果可能需要核查          |

HTTP 非 `2xx` 时，结合 HTTP 状态和响应错误码处理。
