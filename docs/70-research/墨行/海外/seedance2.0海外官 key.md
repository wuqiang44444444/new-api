---
status: current
owner: Dev Team
last-reviewed: 2026-08-11
---

# 海外官Key 视频 (dreamina-seedance-2-0)

`dreamina-seedance-2-0-260128` 海外官Key 视频生成，走方舟官方 JSON 直通。

- **模型 ID**: `dreamina-seedance-2-0-260128`
- **提供商**: 豆包
- **类型**: video
- **创建入口**: `https://tokensave.pro/v1/ark/media/generations`

## 概述

海外官Key seedance 系列走方舟官方 JSON 直通；平台仅做鉴权、路由、估价与账务，请求/响应与 BytePlus ModelArk 一致。

### 接口

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | `/v1/ark/media/generations` | 创建生成任务 |
| GET | `/v1/ark/media/tasks/:id` | 查询任务状态 |

> **鉴权**：`Authorization: Bearer sk-...`（平台 Token）。创建响应原样透传上游 JSON，其中的 `id` 用于 GET 轮询（非平台 `task_id`）。GET 须使用创建任务时的同一用户 Token。
>
> **注意**：本模型不支持 `POST /v1/media/generations`（V2 平台封装接口）。其它走 V2 的视频模型见视频模型文档。
>
> 参考素材可使用 `asset://`（须为本账号海外官Key真人素材库 Active 资产，直接传上游 asset id 如 `asset-20260318-xxx`；平台校验归属后原样转发）。

## 请求参数

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `model` | string | 是 | 固定使用 `dreamina-seedance-2-0-260128`，与 BytePlus 上游模型名一致，平台不做 model mapping。 |
| `content` | array | 是 | 官方多模态输入数组。元素含 `type`（`text` / `image_url` / `video_url` / `audio_url`）与 `role`（见下方映射表）。`image_url` / `video_url` / `audio_url` 推荐使用嵌套格式 `{"url": "..."}`。 |
| `duration` | integer | 否 | 视频时长秒数，4–15；或 `-1` 智能时长。缺省时平台按模型默认时长估价。 |
| `resolution` | string | 否 | `480p` / `720p` / `1080p` / `4k`。缺省时平台按模型配置估价。 |
| `ratio` | string | 否 | `16:9` / `4:3` / `1:1` / `3:4` / `9:16` / `21:9` / `adaptive`。`adaptive` 表示由模型根据输入自动选择比例，为默认推荐值。 |
| `generate_audio` | boolean | 否 | 是否同步生成音频。 |
| `watermark` | boolean | 否 | 本平台不解析，原样透传。上游默认添加水印（`watermark=true`）；如需无水印请显式传 `watermark=false`。 |

### content[].type + role 映射

| type | role | 含义 | 适用场景 |
| --- | --- | --- | --- |
| `text` | — | 文本提示词 | 所有场景均可搭配 |
| `image_url` | `first_frame` | 首帧图片 | 图生视频（首帧） |
| `image_url` | `last_frame` / `end_frame` | 尾帧图片 | 图生视频（首尾帧） |
| `image_url` | `reference_image` | 参考图片 | 参考生 / 编辑视频 |
| `video_url` | `reference_video` | 参考视频 | 参考生 / 编辑 / 延长视频 |
| `audio_url` | `reference_audio` | 参考音频 | 参考生（多模态组合） |

> **注意**：文生、图生（首帧/首尾帧）、参考生、编辑、延长属于不同场景，不可混用。参考生场景内 `reference_image` / `reference_video` / `reference_audio` 可组合；不支持纯音频或仅文本+音频输入。

### 场景与 content

| 场景 | content 结构 | 备注 |
| --- | --- | --- |
| 文生视频 | `content` 仅含 `type=text` | — |
| 图生视频（首帧） | `image_url` + `role=first_frame` | 可选 text |
| 图生视频（首尾帧） | `first_frame` + `last_frame` | 可选 text；与参考生互斥 |
| 参考生视频 | `reference_image` / `reference_video` / `reference_audio` | 三种类型可组合（图片 0~9、视频 0~3、音频 0~3） |
| 编辑视频 | `text` + `reference_video`（可选 `reference_image`） | 基于参考视频按提示词编辑 |
| 延长视频 | `text` + `reference_video` | 延续参考视频内容 |

## 代码示例

### 文生视频（创建 + 查询）

**创建：**

```bash
curl -sS -X POST "https://tokensave.pro/v1/ark/media/generations" \
  -H "Authorization: Bearer <YOUR_API_KEY>" \
  -H "Content-Type: application/json" \
  -d '{
  "model": "dreamina-seedance-2-0-260128",
  "duration": 5,
  "resolution": "720p",
  "ratio": "16:9",
  "generate_audio": true,
  "content": [
    { "type": "text", "text": "一只橘猫在霓虹灯街道上滑滑板，电影感镜头，动态光影" }
  ]
}'
```

**查询（`:id` 为创建响应中的上游 id）：**

```bash
curl -sS "https://tokensave.pro/v1/ark/media/tasks/UPSTREAM_ID" \
  -H "Authorization: Bearer <YOUR_API_KEY>"
```

**创建成功响应：**

```json
{
  "id": "cgt-xxxxxxxx",
  "status": "queued"
}
```

**查询成功响应：**

```json
{
  "id": "cgt-xxxxxxxx",
  "model": "dreamina-seedance-2-0-260128",
  "status": "succeeded",
  "content": {
    "video_url": "https://example.com/output.mp4"
  },
  "usage": {
    "completion_tokens": 246840,
    "total_tokens": 246840
  },
  "resolution": "1080p",
  "ratio": "16:9",
  "duration": 5,
  "framespersecond": 24,
  "execution_expires_after": 172800
}
```

查询成功时 `content.video_url` 下载链接有效期通常为 48 小时（见 `execution_expires_after`）。

### 图生视频（首帧）

```json
{
  "model": "dreamina-seedance-2-0-260128",
  "duration": 5,
  "resolution": "720p",
  "ratio": "16:9",
  "generate_audio": true,
  "content": [
    { "type": "image_url", "role": "first_frame", "image_url": { "url": "https://example.com/first-frame.png" } },
    { "type": "text", "text": "让人物自然转身并看向镜头，镜头轻微推进" }
  ]
}
```

### 参考生视频（多模态可组合）

```json
{
  "model": "dreamina-seedance-2-0-260128",
  "duration": 11,
  "ratio": "16:9",
  "generate_audio": true,
  "watermark": false,
  "content": [
    { "type": "text", "text": "保持参考主体一致，镜头轻微推进" },
    { "type": "image_url", "role": "reference_image", "image_url": { "url": "asset://asset-20260318-xxxxx" } },
    { "type": "video_url", "role": "reference_video", "video_url": { "url": "https://example.com/ref.mp4" } },
    { "type": "audio_url", "role": "reference_audio", "audio_url": { "url": "https://example.com/ref.wav" } }
  ]
}
```

### 编辑视频

```json
{
  "model": "dreamina-seedance-2-0-260128",
  "duration": 5,
  "content": [
    { "type": "text", "text": "将视频背景替换为雪夜场景，保持人物动作不变" },
    { "type": "video_url", "role": "reference_video", "video_url": { "url": "https://example.com/source.mp4" } }
  ]
}
```

### 延长视频

```json
{
  "model": "dreamina-seedance-2-0-260128",
  "duration": 5,
  "content": [
    { "type": "text", "text": "延续参考视频的镜头节奏，自然延长画面" },
    { "type": "video_url", "role": "reference_video", "video_url": { "url": "https://example.com/source.mp4" } }
  ]
}
```

## 上游错误响应

上游 4xx/5xx 通常原样透传；平台鉴权失败、余额不足等由平台返回，结构可能不同。

```json
{
  "error": {
    "code": "InvalidParameter",
    "message": "content is required",
    "type": "BadRequest"
  }
}
```
