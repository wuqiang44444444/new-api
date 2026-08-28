---
page-id: videos-openai
kind: api-reference
last-verified: 2026-08-28
operations:
  - createVideo
  - getVideo
  - remixVideo
  - createVideoGeneration
  - getVideoGeneration
  - getVideoContent
---

# OpenAI Videos

本页说明原生 OpenAI Videos 合同，以及仓库保留的通用视频生成合同。Seedance、Kling 和即梦使用各自的
类型化入口，不能把本页字段复制到那些接口。

## 调用前查询模型合同

先调用 `GET /v1/models/{model}`，读取 `api.video`：

| 字段 | 说明 |
| --- | --- |
| `protocol` | 本页模型应为 `openai_videos` |
| `creation.path` | 创建路径，原生 OpenAI Videos 为 `/v1/videos` |
| `creation.content_type` | 请求 Content-Type |
| `creation.parameters` | 当前客户模型允许的字段、枚举、默认值和长度边界 |
| `operations[]` | 查询、Remix 和内容下载等操作是否可用 |

模型目录是逐模型合同。不要根据模型名或其它视频接口推断时长、尺寸、参考图或 Remix 能力。

## 创建 OpenAI 视频

`POST /v1/videos` · Bearer 鉴权 · `multipart/form-data`

```bash
curl "{{OPENAI_BASE_URL}}/videos" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  -F "model={{MODEL_ID_PLACEHOLDER}}" \
  -F "prompt=清晨的海岸线，固定镜头" \
  -F "seconds=8" \
  -F "size=1280x720"
```

上传参考图时增加文件字段；不要手工设置 multipart boundary：

```bash
curl "{{OPENAI_BASE_URL}}/videos" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  -F "model={{MODEL_ID_PLACEHOLDER}}" \
  -F "prompt=让画面中的海浪缓慢移动" \
  -F "seconds=4" \
  -F "size=1280x720" \
  -F "input_reference=@reference.png"
```

### 创建参数

| 表单字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `model` | string | 是 | 当前 Key 可访问、且 `api.video.protocol=openai_videos` 的客户模型名 |
| `prompt` | string | 是 | 视频描述；当前公开模型合同最大 `32000` 个字符 |
| `seconds` | integer string | 否 | 当前公开 Sora 合同允许 `"4"`、`"8"`、`"12"`，默认 `"4"` |
| `size` | string | 否 | 当前模型允许的输出尺寸；普通模型为 `720x1280`、`1280x720`，部分 Pro 模型还支持更大尺寸 |
| `input_reference` | file | 否 | 参考图片文件；是否支持及文件限制以当前模型合同为准 |

只发送 `api.video.creation.parameters` 中存在的字段。未公开字段、另一种视频协议的 `duration`、`ratio`
或 `content` 都不属于本接口。

### 创建响应

HTTP `200` 返回视频任务对象：

```json
{
  "id": "task-public-id",
  "object": "video",
  "model": "customer-video-model",
  "status": "queued",
  "progress": 0,
  "created_at": 1760000000,
  "seconds": "8",
  "size": "1280x720"
}
```

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | string | 平台视频任务 ID；后续查询、Remix 和下载都使用该值 |
| `task_id` | string | 兼容字段，可能存在；新客户端使用 `id` |
| `object` | string | `video` |
| `model` | string | 创建任务时使用的客户模型 |
| `status` | string | `queued`、`in_progress`、`completed` 或 `failed` |
| `progress` | integer | 进度百分比；不能单独作为成功依据 |
| `created_at` | integer | 创建时间，Unix 秒 |
| `completed_at` | integer | 完成时间，终态时可能返回 |
| `expires_at` | integer | 内容过期时间，Provider 提供时返回 |
| `prompt` | string | 提示词，Provider 返回时存在 |
| `seconds` | string | 视频时长 |
| `size` | string | 视频尺寸 |
| `remixed_from_video_id` | string | Remix 任务的源视频 ID |
| `error.code` / `error.message` | string | 失败任务的错误码和错误说明 |
| `metadata` | object | 公开扩展结果；其中可能包含内容 URL，按实际键存在性读取 |

HTTP `200` 只表示任务已经创建，不表示视频已经完成。

## 查询 OpenAI 视频任务

`GET /v1/videos/{task_id}`

```bash
curl "{{OPENAI_BASE_URL}}/videos/task-public-id" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}"
```

HTTP `200` 返回与创建响应相同的视频任务对象。处理规则：

| `status` | 客户端行为 |
| --- | --- |
| `queued` | 任务排队中，退避后继续查询 |
| `in_progress` | 任务执行中，可结合 `progress` 展示进度 |
| `completed` | 停止轮询，通过内容端点下载结果 |
| `failed` | 停止轮询，读取 `error` 并保留请求 ID |

查询不到任务时返回 `404`。任务按认证账户隔离，只能查询当前账户可见的任务。

## Remix

`POST /v1/videos/{video_id}/remix` · `application/json`

```bash
curl -X POST "{{OPENAI_BASE_URL}}/videos/task-public-id/remix" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  -H "Content-Type: application/json" \
  -d '{"prompt":"保持构图，将天气改为雪天"}'
```

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `video_id` | path string | 是 | 当前调用方可见的源视频任务 ID |
| `prompt` | string | 是 | Remix 指令，去除首尾空白后不能为空 |

源任务必须仍可读取、原渠道可用且对应 adapter 支持 Remix。成功返回新的 OpenAI 视频任务对象；新任务
使用自己的 `id` 查询。不要把源任务 ID 当作新任务 ID。

## 下载内容

任务为 `completed` 后调用：

```bash
curl "{{OPENAI_BASE_URL}}/videos/task-public-id/content" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  --output result.mp4
```

先检查 HTTP 状态和 `Content-Type`。成功响应是视频字节；任务不存在、未完成、内容地址失效或上游读取
失败时返回 JSON 错误，不能把错误体保存成视频。

## 通用视频生成合同

仓库还保留 `POST /v1/video/generations`。它与 `/v1/videos` 的字段和响应不同，也不是 Seedance Link
入口。只有明确要求此合同的现有集成才应使用它。

```bash
curl "{{OPENAI_BASE_URL}}/video/generations" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "{{MODEL_ID_PLACEHOLDER}}",
    "prompt": "宇航员在月球上漫步",
    "duration": 5,
    "width": 1280,
    "height": 720,
    "fps": 30,
    "n": 1,
    "response_format": "url"
  }'
```

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `model` | string | 按渠道要求 | 客户模型名 |
| `prompt` | string | 是 | 视频描述 |
| `image` | string | 否 | 图生视频输入，通常为 URL 或 Base64 |
| `duration` | number | 否 | 视频秒数；公共安全上限为 `3600`，实际范围由模型决定 |
| `width` / `height` | integer | 否 | 输出宽高 |
| `fps` | integer | 否 | 输出帧率 |
| `seed` | integer | 否 | 随机种子 |
| `n` | integer | 否 | 生成数量 |
| `response_format` | string | 否 | 结果格式，例如 `url` |
| `user` | string | 否 | 调用方最终用户标识 |
| `metadata` | object | 否 | 渠道扩展参数；只能发送目标渠道明确支持的键 |

创建响应为 `{"task_id":"...","status":"queued"}` 形式。随后调用
`GET /v1/video/generations/{task_id}`，响应字段如下：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `task_id` | string | 任务 ID |
| `status` | string | `queued`、`in_progress`、`completed` 或 `failed` |
| `url` | string | 成功时的视频地址 |
| `format` | string | 输出格式，例如 `mp4` |
| `metadata.duration` | number | 实际时长 |
| `metadata.fps` | integer | 实际帧率 |
| `metadata.width` / `metadata.height` | integer | 实际宽高 |
| `metadata.seed` | integer | 实际随机种子 |
| `error.code` | integer | 失败错误码 |
| `error.message` | string | 失败说明 |

## 错误与重试

OpenAI 兼容错误通常使用 `error.message`、`error.type`、`error.param`、`error.code` 和可选
`error.request_id`。`400` 表示字段或模型合同错误，`401/403` 表示鉴权或权限错误，`404` 表示任务不
存在，`429` 表示限流或额度问题，`5xx` 表示平台或上游异常。

创建和 Remix 都可能产生费用。网络中断不证明请求未被受理；本合同没有承诺可重放的客户幂等结果，
不要在超时后盲目重复创建。查询 GET 可以使用指数退避重试。
