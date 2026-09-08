---
status: current
owner: Dev Team
last-reviewed: 2026-09-08
---

# doubao-seedance-2.0

豆包大模型团队推出的新一代专业级多模态创作视频模型 Seedance 2.0，支持图像、视频、音频等多模态作为参考输入生成视频，还具备视频编辑、延长等能力，能高精度还原各类细节并稳定角色特征，具备极致拟真的视听稳定性，深度适配商业广告、影视制作与社交媒体营销等各大核心场景。

- **模型 ID**: `doubao-seedance-2-0-260128`
- **提供商**: 豆包
- **类型**: 视频
- **创建入口**: `https://tokensave.pro/v1/media/generations`

## 能力与接口

### 视频生成

`POST /v1/media/generations`

异步任务；提交后轮询 `GET /v1/media/tasks/:task_id`。支持文生视频、图生视频和参考生视频三类场景。

#### 生成场景

- 文生视频
- 图生视频
- 参考生视频

#### 请求参数

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `aspect_ratio` | string | 否 | 画幅比例；支持 `16:9` / `4:3` / `1:1` / `3:4` / `9:16` / `21:9` / `adaptive`。示例：`16:9` |
| `capability` | string | 是 | 能力类型；固定为 `video_generation` |
| `control_mode` | string | 是 | 控制模式；文生视频固定 `none`；图生视频可用 `none` 或 `end_frame`；参考生视频固定 `reference`。示例：`none` |
| `duration_seconds` | integer | 否 | 视频时长（秒）；支持 4-15 秒，或 `-1` 表示自动。示例：`4` |
| `input_mode` | string | 是 | 输入模式；文生视频使用 `text`；图生视频使用 `single_image`；参考生视频使用 `multi_image`。示例：`text` |
| `model` | string | 是 | 模型 ID；要调用的视频模型 logical key |
| `prompt` | string | 是 | 提示词；描述想要生成的视频内容，最长 2500 字符。参考生视频场景中用于描述基于参考素材生成的目标效果。示例：图片1中的人物站在海边，夕阳下回眸一笑 |
| `watermark` | string | 否 | 水印；上游官方字段；`true` 含水印，`false` 无水印。平台原样透传，缺省时以上游默认为准。示例：`false` |

#### 响应格式

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `created_at` | string | 任务创建时间（Unix 时间戳） |
| `error_message` | string | 终态失败时的错误描述 |
| `progress` | string | 0-100 之间的整数，表示生成进度 |
| `result` | string | 终态成功时返回视频/资源信息，包含 `url`、`duration_seconds` 等字段 |
| `status` | string | `queued` / `running` / `succeeded` / `failed` |
| `task_id` | string | 任务唯一 ID，用于轮询 `GET /v1/media/tasks/:id` |
| `usage` | string | 本次任务的额度消耗（如有） |

### 异步查询生成结果

`GET /v1/media/tasks/:id`

创建任务后保留 `task_id`，再轮询此接口直到状态变为 `succeeded` 或 `failed`。图片和视频模型都使用这个查询接口。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `task_id` | string | 任务唯一标识。 |
| `status` | string | `queued` / `running` / `succeeded` / `failed`。 |
| `status_code` | integer | `queued` / `running` 通常为 202，`succeeded` 为 200，`failed` 为 400。 |
| `result` | object | 视频任务成功后，`result` 通常包含主视频地址、带水印地址和封面图。 |
| `error_message` | string | 失败原因，仅失败时返回。 |

建议使用退避轮询，避免把异步查询接口当作同步接口高频调用。

```bash
curl -sS "https://tokensave.pro/v1/media/tasks/:id" \
  -H "Authorization: Bearer sk-xxxx"
```

响应示例：

```json
{
  "object": "media.task",
  "task_id": "df6b6427206441c9adaad913bee84f8e",
  "status": "succeeded",
  "status_code": 200,
  "capability": "video_generation",
  "model": "your-video-model",
  "created_at": 1774834768,
  "updated_at": 1774834790,
  "result": {
    "type": "video",
    "primary_url": "https://resource.moxing.pro/video/df6b6427206441c9adaad913bee84f8e/main.mp4",
    "urls": [
      "https://resource.moxing.pro/video/df6b6427206441c9adaad913bee84f8e/main.mp4"
    ],
    "watermark_url": "https://resource.moxing.pro/video/df6b6427206441c9adaad913bee84f8e/watermark.mp4",
    "watermark_urls": [
      "https://resource.moxing.pro/video/df6b6427206441c9adaad913bee84f8e/watermark.mp4"
    ],
    "cover_url": "https://resource.moxing.pro/image/df6b6427206441c9adaad913bee84f8e/cover.jpg"
  }
}
```

## 错误与限制

### 错误码

| 错误码 | HTTP | 说明 |
| --- | --- | --- |
| `invalid_request_error` | 400 | 请求参数缺失、格式错误或字段值不在允许范围内 |
| `invalid_api_key` | 401 | API Key 为空、格式错误或已失效；请确认 `Authorization: Bearer sk-...` |
| `insufficient_quota` | 403 | 账户余额或配额不足；请充值或申请扩容 |
| `model_not_found` | 404 | 请求的 `model` 不存在或当前账户无权访问 |
| `rate_limit_exceeded` | 429 | 触发并发或 RPM 限流；建议指数退避后重试 |
| `internal_server_error` | 500 | 平台内部异常；可重试，持续异常请联系技术支持 |
| `upstream_unavailable` | 502 | 模型服务暂时不可用或超时；建议稍后重试 |

### Limits

| 限制项 | 说明 |
| --- | --- |
| 分辨率 | `480p` / `720p` |
| 参考素材 | 普通资产参考可传公网 URL 或 `asset://upstream_id`；人像库参考传 `asset://upstream_id`。 |
| 平台到上游字段映射 | 平台字段与火山方舟 Seedance 2.0 官方字段基本一致，`duration_seconds`、`aspect_ratio`、`with_audio`、`watermark`、`image`、`end_image`、`reference_images`、`reference_videos`、`reference_audios` 均按官方接口透传；`input_mode`、`control_mode` 为平台侧场景编排字段，不直接传给上游。 |
| 画幅比例 | `16:9` / `4:3` / `1:1` / `3:4` / `9:16` / `21:9` / `adaptive` |
| 视频时长 | 4-15 秒，或 `-1` 自动 |
