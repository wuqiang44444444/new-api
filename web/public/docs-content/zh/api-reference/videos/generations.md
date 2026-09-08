---
page-id: videos-generations
kind: api-reference
last-verified: 2026-09-09
operations:
  - createVideoGeneration
  - getVideoGeneration
---

# 通用视频生成

`POST /v1/video/generations` · Bearer 鉴权 · `application/json`

本页用于已明确使用此入口的集成。只有目标客户模型公开支持该路径时才可调用；不能用它代替模型要求的
OpenAI Videos、ModelArk V3、Kling 或即梦入口。通用查询使用 `code / message / data` 信封。

## 创建任务

先从模型目录确认客户模型和规格，再提交最小请求：

```bash
curl "{{OPENAI_BASE_URL}}/video/generations" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "{{MODEL_ID_PLACEHOLDER}}",
    "prompt": "清晨海面上，一艘帆船缓慢驶过"
  }'
```

| 字段 | 类型 | 使用条件 |
| --- | --- | --- |
| `model` | string | 填写支持本入口的客户模型 ID |
| `prompt` | string | 必填视频描述，不能为空 |
| `image` | string | 模型支持时使用单张参考图片 |
| `images` | string[] | 模型支持时使用多张参考图片；不要与 `image` 同时填写 |
| `size` | string | 模型明确支持的尺寸 |
| `duration` | integer | 视频秒数；安全上限 `3600`，实际允许范围通常小得多，按模型说明填写 |
| `seconds` | string | 仅在模型明确要求时使用；不要与 `duration` 同时填写 |
| `mode` | string | 模型公开的生成模式 |
| `input_reference` | string 或 object | 模型明确支持时使用；对象只选 `file_id` 或 `image_url` 之一 |
| `metadata` | object | 仅填写目标模型已经公开说明的扩展字段；没有说明时省略 |

这些字段不是每个模型都支持。顶层 `width`、`height`、`fps`、`n`、`seed` 和 `response_format`
不是本页发布的通用参数，不能据此控制输出。不要把其他视频协议的私有字段塞进 `metadata`。

HTTP `200` 创建响应示例：

```json
{
  "id": "task-public-id",
  "task_id": "task-public-id",
  "status": "queued",
  "model": "customer-video-model",
  "created_at": 1760000000
}
```

保存任务 ID 与所用协议。创建响应中的 `queued` 表示已受理，后续查询不会沿用这一顶层响应结构。
本入口没有承诺客户幂等键可恢复原创建结果；创建超时后不要盲目重发。

## 查询任务

```bash
curl "{{OPENAI_BASE_URL}}/video/generations/task-public-id" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}"
```

HTTP `200` 表示查询成功。先检查 `code=success`，再解析 `data`。常规任务的业务字段示例：

```json
{
  "code": "success",
  "message": "",
  "data": {
    "task_id": "task-public-id",
    "status": "SUCCESS",
    "progress": "100%",
    "fail_reason": "",
    "result_url": "https://example.com/result.mp4"
  }
}
```

部分任务的实时查询返回简化形状：

```json
{
  "code": "success",
  "message": "",
  "data": {
    "task_id": "task-public-id",
    "status": "succeeded",
    "url": "https://example.com/result.mp4",
    "format": "mp4",
    "metadata": null,
    "error": null
  }
}
```

| 状态语义 | 常规 `data.status` | 简化 `data.status` | 客户端动作 |
| --- | --- | --- | --- |
| 排队 | `NOT_START`、`SUBMITTED`、`QUEUED` | `queued` | 等待后查询 |
| 处理中 | `IN_PROGRESS` | `processing` | 退避查询 |
| 成功 | `SUCCESS` | `succeeded` | 分别读取 `data.result_url` 或 `data.url` |
| 失败 | `FAILURE` | `failed` | 停止轮询；常规形状读取 `data.fail_reason` |

按每次响应的 `data` 形状解析；不要假设同一任务每次都使用简化形状。`progress` 为带百分号的字符串，
不能凭进度判断成功。`metadata`、`error` 可以为 `null`，不要假设一定提供时长、分辨率或错误详情。
表中没有的状态保留原值并核查，不直接归类为失败；其他附加字段不应成为业务依赖。

## 错误、轮询与结果

查询业务错误使用 `code` 与 `message`，例如任务不存在可返回 HTTP `400`、`code=task_not_exist`，
不能统一按 `404` 处理。鉴权等前置错误也可能使用通用错误信封，先检查 HTTP 状态再解析正文。

建议每次等待 2 秒起步，退避到 10～30 秒；`429`、网络异常或 `5xx` 时有限重试查询。
已有任务 ID 就继续查询原任务，本地等待超时不取消生成。成功后及时下载，检查 HTTP 状态和媒体类型；
外部结果地址不附带平台 API Key。不保证存在任务列表、删除或取消能力。
