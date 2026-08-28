---
page-id: videos-kling
kind: api-reference
last-verified: 2026-08-28
operations:
  - createKlingText2Video
  - getKlingText2Video
  - createKlingImage2Video
  - getKlingImage2Video
---

# Kling 视频

Kling 合同分别提供文生视频和图生视频入口。两类任务使用相同请求字段与响应信封，但路径、必填媒体和
查询路径不同。当前合同不提供列表、删除、取消或平台内容下载接口。

当前内置适配器登记的模型为 `kling-v1`、`kling-v1-6` 和 `kling-v2-master`。实际可用模型仍以
`GET /v1/models` 和当前 API Key 权限为准。

## 创建文生视频

`POST /kling/v1/videos/text2video` · Bearer 鉴权 · `application/json`

```bash
curl "{{SITE_BASE_URL}}/kling/v1/videos/text2video" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: kling-text-example-001" \
  -d '{
    "model_name": "{{MODEL_ID_PLACEHOLDER}}",
    "prompt": "纸船沿着雨后的街道缓慢漂流",
    "duration": "5",
    "aspect_ratio": "16:9"
  }'
```

文生视频不能发送 `image` 或 `image_tail`。

## 创建图生视频

`POST /kling/v1/videos/image2video` · Bearer 鉴权 · `application/json`

```bash
curl "{{SITE_BASE_URL}}/kling/v1/videos/image2video" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: kling-image-example-001" \
  -d '{
    "model_name": "{{MODEL_ID_PLACEHOLDER}}",
    "prompt": "镜头向前推进，云层缓慢移动",
    "image": "https://example.com/first-frame.png",
    "duration": "5",
    "aspect_ratio": "16:9"
  }'
```

图生视频必须发送非空 `image`；`image_tail` 表示尾帧，并且只能与 `image` 一起使用。图片字段可使用
目标模型支持的 URL、Base64 或 `asset://<opaque-id>` 引用。

## 创建请求参数

| 字段 | 类型 | 必填 | 取值与说明 |
| --- | --- | --- | --- |
| `model_name` | string | 是 | 当前 Key 可访问的客户模型名；映射后的上游模型不会公开 |
| `prompt` | string | 是 | 视频描述，去除首尾空白后不能为空 |
| `image` | string | 图生视频是 | 首帧 URL、Base64 或 `asset://` 引用；文生视频禁止 |
| `image_tail` | string | 否 | 尾帧；仅图生视频可用，且要求同时存在 `image` |
| `negative_prompt` | string | 否 | 负向提示词 |
| `mode` | string | 否 | `std` 或 `pro`；省略时为 `std` |
| `duration` | string | 否 | `"5"` 或 `"10"`；必须是字符串，省略时为 `"5"` |
| `aspect_ratio` | string | 否 | `16:9`、`9:16` 或 `1:1`；省略时为 `1:1` |
| `cfg_scale` | number | 否 | `0`～`1`；省略时为 `0.5` |
| `static_mask` | string | 否 | 静态遮罩；格式由 Kling 模型合同决定 |
| `dynamic_masks` | array | 否 | 动态遮罩数组 |
| `dynamic_masks[].mask` | string | 否 | 单个动态遮罩 |
| `dynamic_masks[].trajectories` | array | 否 | 轨迹点数组 |
| `dynamic_masks[].trajectories[].x` / `.y` | integer | 否 | 轨迹点坐标 |
| `camera_control` | object | 否 | 镜头控制对象 |
| `camera_control.type` | string | 否 | 镜头控制类型 |
| `camera_control.config` | object | 否 | 只允许 `horizontal`、`vertical`、`pan`、`tilt`、`roll`、`zoom` 数值字段 |
| `callback_url` | string | 否 | Kling 回调地址；是否可用由模型和渠道决定 |
| `external_task_id` | string | 否 | 调用方外部任务标识 |

请求采用严格字段白名单。未知顶层字段、`dynamic_masks` 未知子字段和 `camera_control` 未知子字段都会
返回 `400`，不会透传给上游。

## 创建响应

HTTP `200`：

```json
{
  "code": 0,
  "message": "SUCCEED",
  "request_id": "req-placeholder",
  "data": {
    "task_id": "task-public-id",
    "task_status": "submitted"
  }
}
```

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `code` | integer | `0` 表示本次 API 操作成功 |
| `message` | string | 成功时为 `SUCCEED` |
| `request_id` | string | 请求追踪 ID，排障时保留 |
| `data.task_id` | string | 平台任务 ID；立即保存 |
| `data.task_status` | string | 创建响应固定为 `submitted` |

## 查询任务

文生视频与图生视频必须使用各自的查询路径：

```bash
curl "{{SITE_BASE_URL}}/kling/v1/videos/text2video/task-public-id" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}"
```

```bash
curl "{{SITE_BASE_URL}}/kling/v1/videos/image2video/task-public-id" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}"
```

HTTP `200` 示例：

```json
{
  "code": 0,
  "message": "SUCCEED",
  "request_id": "req-placeholder",
  "data": {
    "task_id": "task-public-id",
    "task_status": "succeed",
    "task_status_msg": "",
    "task_result": {
      "videos": [
        {"url": "https://example.com/generated-video.mp4"}
      ]
    },
    "created_at": 1760000000,
    "updated_at": 1760000120
  }
}
```

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `code` | integer | `0` 表示查询成功；任务业务状态仍看 `data.task_status` |
| `message` | string | API 操作消息 |
| `request_id` | string | 本次查询的追踪 ID |
| `data.task_id` | string | 平台任务 ID |
| `data.task_status` | string | `submitted`、`processing`、`succeed` 或 `failed` |
| `data.task_status_msg` | string | 任务失败原因；可能为空 |
| `data.task_result.videos` | array | 成功时的视频结果数组；当前平台投影一项 |
| `data.task_result.videos[].url` | string | 视频结果 URL，应及时下载或转存 |
| `data.created_at` / `data.updated_at` | integer | Unix 秒时间戳 |

`submitted` 和 `processing` 使用退避轮询；`succeed` 或 `failed` 后停止。查询接口不会返回平台内容代理
路径，客户端直接使用成功响应中的 URL。

## 幂等、计费与重试

创建接口支持可选 `Idempotency-Key`，最大 `191` 个字符，同一键在 `24` 小时内应绑定完全相同的协议和
请求体。相同请求成功后可返回同一个任务；同一键配不同请求返回 `409`；原请求仍在处理或等待核查时也
返回 `409`。遇到网络中断时应携带原键重试，不能换新键盲目创建。

任务费用按创建时冻结的客户模型和计费事实处理。创建结果不明时平台不自动重发、换渠道或退款；保留
`request_id` 和幂等键，查询已有任务或联系管理员。

## 错误响应

Kling 错误始终使用相同信封：

```json
{
  "code": 1200,
  "message": "invalid request body",
  "request_id": "req-placeholder",
  "data": null
}
```

| HTTP 状态 | `code` | 含义 |
| --- | --- | --- |
| `400` | `1200` | JSON、字段、类型、必填项或模型参数无效 |
| `401` | `1000` | API Key 无效 |
| `402` | `1101` | 额度不足 |
| `403` | `1103` | 模型或分组权限不足 |
| `404` | `1203` | 当前调用方下没有该 Kling 任务 |
| `429` | `1303` | 请求过多 |
| `500` | `5000` | 平台或上游处理失败 |
| `503` | `5001` | 上游不可用或创建结果需要核查 |
| `504` | `5002` | 上游超时 |

HTTP 非 `2xx` 时先按 HTTP 状态处理；不要只检查数值 `code`。错误消息已经脱敏，不能据此推断上游身份。
