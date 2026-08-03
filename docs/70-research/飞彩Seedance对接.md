# 临时 Seedance 视频模型 API 接口文档

> API Key 已脱敏；真实密钥只允许通过运行环境或受控输入提供。

文档版本：2026-07-27

适用模型：

- `seedance-933-pro-pi-feicai`
- `seedance2.0-sd2-feicai`
- 本文列出的 8 个 AZHW Seedance 模型

> 本文面向直接调用临时 API 的开发者，不包含模型价格。
>
> 上述模型已经开放标准 API Token 调用权限。调用方只需要临时 API Key；不需要额外账号验证请求头、Cookie、上游登录态或任何中继服务。

---

## 1. 快速开始

### 1.1 基础地址

```text
https://feicai123.top/v1
```

### 1.2 鉴权

所有 `/v1/*` 请求统一使用：

```http
Authorization: Bearer sk-YOUR_TOKEN
```

创建视频任务时还需要：

```http
Content-Type: application/json
```

请勿把真实 Token 写入前端源码、公开仓库或日志。

### 1.3 异步调用流程

```text
POST /v1/videos
       |
       v
  返回任务 id
       |
       v
GET /v1/videos/{id}  -- 每 5 秒查询一次
       |
       +-- queued / in_progress --> 继续查询
       |
       +-- failed -------------> 读取 error
       |
       `-- completed ----------> 读取 metadata.url
                                  或下载 /v1/videos/{id}/content
```

不要对一次已经发出的 `POST /v1/videos` 请求做无条件自动重试。若网络超时发生在服务端已经创建任务之后，重复提交可能生成两笔任务。

---

## 2. 公共接口

### 2.1 创建视频任务

```http
POST https://feicai123.top/v1/videos
Authorization: Bearer sk-YOUR_TOKEN
Content-Type: application/json
```

不同模型的请求体字段不同，请直接使用本文对应模型章节中的示例。

提交成功示例：

```json
{
  "id": "video_abc123",
  "object": "video",
  "status": "queued",
  "model": "seedance2.0-sd2-feicai",
  "created_at": 1785120000,
  "progress": 0,
  "seconds": "15",
  "size": "1280x720"
}
```

必须保存返回的 `id`，后续查询和下载都需要使用它。

### 2.2 查询任务状态

```http
GET https://feicai123.top/v1/videos/{video_id}
Authorization: Bearer sk-YOUR_TOKEN
```

进行中示例：

```json
{
  "id": "video_abc123",
  "object": "video",
  "status": "in_progress",
  "progress": 42
}
```

完成示例：

```json
{
  "id": "video_abc123",
  "object": "video",
  "status": "completed",
  "progress": 100,
  "completed_at": 1785120120,
  "seconds": "15",
  "size": "1280x720",
  "metadata": {
    "url": "https://feicai123.top/v1/videos/video_abc123/content"
  }
}
```

失败示例：

```json
{
  "id": "video_abc123",
  "object": "video",
  "status": "failed",
  "error": {
    "message": "上游审核拒绝",
    "code": "prompt_blocked"
  }
}
```

任务状态只有以下四类需要业务方处理：

| 状态          | 含义   | 调用方动作                     |
| ------------- | ------ | ------------------------------ |
| `queued`      | 已排队 | 继续轮询                       |
| `in_progress` | 生成中 | 继续轮询                       |
| `completed`   | 已完成 | 读取 `metadata.url` 或下载内容 |
| `failed`      | 已失败 | 停止轮询并读取 `error`         |

建议每 5 秒查询一次。不要每秒高频查询。

### 2.3 下载视频

```http
GET https://feicai123.top/v1/videos/{video_id}/content
Authorization: Bearer sk-YOUR_TOKEN
```

成功时返回视频字节流，通常为：

```http
Content-Type: video/mp4
```

也可以直接使用完成响应中的 `metadata.url`。不要自行拼接上游存储域名。

### 2.4 查询当前可用模型

```http
GET https://feicai123.top/v1/models
Authorization: Bearer sk-YOUR_TOKEN
```

建议在应用启动或配置模型时查询一次，不要为每个视频任务重复查询。

---

## 3. 素材 URL 通用要求

### 3.1 推荐做法

- 使用公网可访问的 HTTPS URL。
- URL 必须能在不携带 Cookie、不增加自定义请求头的情况下直接下载。
- 签名 URL 的有效期必须覆盖排队、生成和上游拉取素材的时间。
- URL 路径最好保留正确文件扩展名，例如 `.png`、`.jpg`、`.mp3`、`.wav`、`.mp4`。
- 不要传本地文件路径、`blob:` URL、仅局域网可访问的 URL。

### 3.2 推荐素材格式

| 素材 | 首选格式            | 说明                                                    |
| ---- | ------------------- | ------------------------------------------------------- |
| 图片 | JPG/JPEG、PNG、WebP | 建议使用标准 RGB 图片，避免损坏文件和异常超大尺寸       |
| 音频 | MP3、WAV、M4A       | AZHW 兼容层还可识别 AAC、OGG、FLAC；优先使用前三种      |
| 视频 | MP4（H.264）        | `seedance-933-pro-pi-feicai` 使用；建议保留 `.mp4` 后缀 |

`seedance2.0-sd2-feicai` 明确只接受公网 HTTP/HTTPS 图片 URL，不接受 Data URL。

AZHW 系列兼容标准 Data URL，例如 `data:image/png;base64,...` 和 `data:audio/mpeg;base64,...`，但大素材仍建议使用公网 URL，以免 JSON 请求体过大。

---

## 4. 模型能力总览

| 模型或系列                   |       时长 |       分辨率 | 画幅           |        图片 |      视频 |      音频 | 真人素材 |
| ---------------------------- | ---------: | -----------: | -------------- | ----------: | --------: | --------: | -------- |
| `seedance-933-pro-pi-feicai` | 固定 15 秒 |    固定 720p | 6 种           |   最多 9 张 | 最多 3 段 | 最多 3 段 | 支持     |
| `seedance2.0-sd2-feicai`     |   11–15 秒 |    固定 720p | `16:9`、`9:16` |   必须 1 张 |    不支持 |    不支持 | 不支持   |
| AZHW 8 型                    |    4–15 秒 | 由模型名固定 | 6 种           |   最多 9 张 |    不支持 | 最多 3 段 | 支持     |

六种画幅是：

```text
21:9  16:9  4:3  1:1  3:4  9:16
```

---

## 5. `seedance-933-pro-pi-feicai`

### 5.1 模型备注

- 固定生成 15 秒。
- 固定输出 720p。
- 支持六种画幅。
- 支持纯文字、单图和多参考素材。
- 最多 9 张参考图、3 段参考视频、3 段参考音频。
- 支持真人素材。
- 不开放首尾帧专用模式。
- 不需要传 `reference_mode`。
- 不要传 `video_config`。

### 5.2 请求字段

| 字段                   | 类型     | 必填 | 说明                                |
| ---------------------- | -------- | ---- | ----------------------------------- |
| `model`                | string   | 是   | 固定为 `seedance-933-pro-pi-feicai` |
| `prompt`               | string   | 是   | 视频描述                            |
| `seconds`              | string   | 是   | 固定传 `"15"`                       |
| `resolution`           | string   | 是   | 固定传 `"720p"`                     |
| `aspect_ratio`         | string   | 是   | 六种画幅之一                        |
| `image_url`            | string   | 否   | 第 1 张参考图 URL                   |
| `reference_image_urls` | string[] | 否   | 第 2–9 张参考图 URL                 |
| `reference_videos`     | string[] | 否   | 参考视频 URL，最多 3 段             |
| `audio_urls`           | string[] | 否   | 参考音频 URL，最多 3 段             |

### 5.3 多参考图格式

该模型的图片不是全部放进同一个数组，而是拆成“首图 + 其余图片”：

```json
{
  "image_url": "https://cdn.example.com/ref/image-01.png",
  "reference_image_urls": [
    "https://cdn.example.com/ref/image-02.png",
    "https://cdn.example.com/ref/image-03.png"
  ]
}
```

对应关系：

| 提示词中的称呼 | 实际字段                  |
| -------------- | ------------------------- |
| 参考图 1       | `image_url`               |
| 参考图 2       | `reference_image_urls[0]` |
| 参考图 3       | `reference_image_urls[1]` |

提示词建议直接写清楚素材序号，例如：

```text
保持参考图1中的人物外观和服装，采用参考图2的室内场景，
让人物按照参考视频1的运镜移动，并配合参考音频1的节奏。
```

### 5.4 音频和视频格式

音频必须使用 `audio_urls` 数组：

```json
{
  "audio_urls": ["https://cdn.example.com/ref/music.mp3", "https://cdn.example.com/ref/voice.wav"]
}
```

不要使用旧字段 `reference_audios`。当前正式字段是 `audio_urls`。

为保证上游正确接受音频参考，使用 `audio_urls` 时请至少同时提供 1 张参考图。

参考视频使用：

```json
{
  "reference_videos": ["https://cdn.example.com/ref/camera-motion.mp4"]
}
```

视频优先使用 MP4/H.264，音频优先使用 MP3、WAV 或 M4A。所有 URL 都应为公网可读地址。

### 5.5 完整请求示例

```bash
curl -X POST "https://feicai123.top/v1/videos" \
  -H "Authorization: Bearer sk-YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "seedance-933-pro-pi-feicai",
    "prompt": "保持参考图1中的人物外观和服装，采用参考图2的夜景环境，参考视频1的推进运镜，并配合参考音频1的节奏。",
    "seconds": "15",
    "resolution": "720p",
    "aspect_ratio": "16:9",
    "image_url": "https://cdn.example.com/ref/person.png",
    "reference_image_urls": [
      "https://cdn.example.com/ref/night-scene.png"
    ],
    "reference_videos": [
      "https://cdn.example.com/ref/camera-motion.mp4"
    ],
    "audio_urls": [
      "https://cdn.example.com/ref/music.mp3"
    ]
  }'
```

纯文字示例只保留基础字段：

```json
{
  "model": "seedance-933-pro-pi-feicai",
  "prompt": "电影质感的雨夜街道，镜头缓慢向前推进，霓虹灯倒映在湿润路面上。",
  "seconds": "15",
  "resolution": "720p",
  "aspect_ratio": "16:9"
}
```

---

## 6. `seedance2.0-sd2-feicai`

### 6.1 模型备注

- 固定输出 720p。
- 支持 11、12、13、14、15 秒。
- 只支持 `16:9` 和 `9:16`。
- 必须提供 1 张参考图，不支持纯文字生成。
- 不支持参考音频。
- 不支持参考视频。
- 不支持真人素材。
- 一次请求只生成 1 个视频。

### 6.2 推荐请求字段

| 字段         | 类型     | 必填 | 说明                                |
| ------------ | -------- | ---- | ----------------------------------- |
| `model`      | string   | 是   | 固定为 `seedance2.0-sd2-feicai`     |
| `prompt`     | string   | 是   | 视频描述，可引用图片序号            |
| `image`      | string   | 是   | 1 个公网 HTTP/HTTPS 图片 URL        |
| `duration`   | integer  | 是   | 11–15                               |
| `resolution` | string   | 否   | 只允许 `720p`，省略时也是 720p      |
| `ratio`      | string   | 否   | `16:9` 或 `9:16`，省略时默认 `16:9` |
| `n`          | integer  | 否   | 只能为 `1`，省略时自动使用 1        |
| `user`       | string   | 否   | 调用方自己的用户标识                |
| `metadata`   | object   | 否   | 调用方自己的附加信息                |

### 6.3 参考图格式

`image` 必须直接传一个 URL 字符串，不能传数组：

```json
{
  "image": "https://cdn.example.com/ref/character.png"
}
```

提示词可使用以下引用写法：

```text
@图1
@图片1
@image1
@image_file_1
```

发送前，临时会统一整理为模型所需的图片引用格式。

示例：

```text
让 @图1 中的人物缓慢转身，保持人物外观和服装细节一致。
```

不要把 `image` 写成数组，也不要在同一次请求里混用 `image`、`images`、`image_url`
和 `reference_image_urls`。实测数组会被接口拒绝并返回
`cannot unmarshal array ... image of type string`。

### 6.4 图片格式要求

- 必须是公网 HTTP/HTTPS URL。
- 不接受 `data:image/...;base64,...`。
- 不接受本地路径、`blob:` URL 或需要 Cookie 才能访问的地址。
- 推荐 JPG/JPEG、PNG、WebP。
- 只接受 1 张。

### 6.5 完整请求示例

```bash
curl -X POST "https://feicai123.top/v1/videos" \
  -H "Authorization: Bearer sk-YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "seedance2.0-sd2-feicai",
    "prompt": "让 @图1 中的人物缓慢转身，保持人物外观和服装细节一致，镜头缓慢跟随。",
    "image": "https://cdn.example.com/ref/character.png",
    "duration": 15,
    "resolution": "720p",
    "ratio": "16:9",
    "n": 1
  }'
```

带调用方信息的示例：

```json
{
  "model": "seedance2.0-sd2-feicai",
  "prompt": "@图1 中的人物看向镜头，然后缓慢转身。",
  "image": "https://cdn.example.com/ref/character.png",
  "duration": 11,
  "resolution": "720p",
  "ratio": "9:16",
  "n": 1,
  "user": "customer-10001",
  "metadata": {
    "order_id": "order-20260727-0001"
  }
}
```

### 6.6 会被拒绝的请求

| 情况                         | 原因                      |
| ---------------------------- | ------------------------- |
| 没有图片                     | 该模型必须有 1 张参考图   |
| `image` 使用数组             | 接口字段类型是字符串      |
| `duration` 小于 11 或大于 15 | 时长不支持                |
| `resolution` 不是 `720p`     | 只开放 720p               |
| `ratio` 为 `1:1` 等其他比例  | 只支持 `16:9`、`9:16`     |
| 携带音频字段                 | 不支持音频参考            |
| 携带视频字段                 | 不支持视频参考            |
| `n` 不是 1                   | 一次只能生成一个视频      |
| 图片使用 Data URL            | 只接受公网 HTTP/HTTPS URL |
| 提示词引用不存在的图片序号   | 图片引用越界              |

---

## 7. AZHW Seedance 系列

### 7.1 正式开放的 8 个模型

| 模型 ID                                  | 固定分辨率 |    时长 | 备注                                            |
| ---------------------------------------- | ---------: | ------: | ----------------------------------------------- |
| `seedance-2.0-vip-720p-azhw-feicai`      |       720p | 4–15 秒 | 优质渠道，9 图 + 3 音频 + 0 视频，支持真人      |
| `seedance-2.0-vip-1080p-azhw-feicai`     |      1080p | 4–15 秒 | 优质渠道，9 图 + 3 音频 + 0 视频，支持真人      |
| `seedance-2.0-vip-4k-azhw-feicai`        |         4K | 4–15 秒 | 优质渠道，9 图 + 3 音频 + 0 视频，支持真人      |
| `seedance-2.0-vip-720p-fast-azhw-feicai` |       720p | 4–15 秒 | Fast 优质渠道，9 图 + 3 音频 + 0 视频，支持真人 |
| `seedance-2.0-vip-720p-mini-azhw-feicai` |       720p | 4–15 秒 | Mini 优质渠道，9 图 + 3 音频 + 0 视频，支持真人 |
| `seedance-2.0-933-720p-azhw-feicai`      |       720p | 4–15 秒 | 933，9 图 + 3 音频 + 0 视频，支持真人           |
| `seedance-2.0-933-1080p-azhw-feicai`     |      1080p | 4–15 秒 | 933，9 图 + 3 音频 + 0 视频，支持真人           |
| `seedance-2.0-933-4k-azhw-feicai`        |         4K | 4–15 秒 | 933，9 图 + 3 音频 + 0 视频，支持真人           |

以下型号不在本文开放范围：

- VIP 480p
- Fast 933

### 7.2 请求字段

| 字段             | 类型     | 必填 | 说明                                |
| ---------------- | -------- | ---- | ----------------------------------- |
| `model`          | string   | 是   | 使用上表中的完整模型 ID             |
| `prompt`         | string   | 是   | 视频描述，可引用图片和音频          |
| `duration`       | integer  | 是   | 4–15                                |
| `ratio`          | string   | 否   | 六种画幅之一，省略时默认 `16:9`     |
| `reference_mode` | string   | 否   | 参考模式，见下一节                  |
| `input_images`   | string[] | 否   | 参考图 URL 或 Data URL，最多 9 张   |
| `audio_url_list` | string[] | 否   | 参考音频 URL 或 Data URL，最多 3 段 |

分辨率由模型 ID 固定，不需要传 `resolution`。即使传入其他分辨率，也不能把 720p 型号改成 1080p 或 4K。

请严格传 4–15 的整数时长。当前服务会把小于 4 的值调整为 4，把大于 15 的值调整为 15，但调用方不要依赖自动调整。

### 7.3 参考模式

| `reference_mode` | 使用场景   | 素材规则                                            |
| ---------------- | ---------- | --------------------------------------------------- |
| `text_to_video`  | 文生视频   | 不传图片和音频                                      |
| `omni`           | 全能参考   | 可混合图片和音频                                    |
| `first_frame`    | 首帧参考   | 传 1 张图片                                         |
| `last_frame`     | 尾帧参考   | 传 1 张图片                                         |
| `both_frames`    | 首尾帧参考 | 恰好 2 张图片；第 1 张首帧、第 2 张尾帧；不要传音频 |

自动处理规则：

- 没有任何素材时，自动使用 `text_to_video`。
- 有素材但未传 `reference_mode` 时，自动使用 `omni`。
- 有素材却传了 `text_to_video` 时，也会自动改为 `omni`。

### 7.4 多参考图和音频格式

推荐字段：

```json
{
  "input_images": [
    "https://cdn.example.com/ref/character.png",
    "https://cdn.example.com/ref/scene.png"
  ],
  "audio_url_list": ["https://cdn.example.com/ref/music.mp3"]
}
```

临时会保持每一类素材内部的数组顺序，并按“图片在前、音频在后”的顺序提交。

建议在提示词中使用按类型编号的写法：

```text
@图片1 中的人物进入 @图片2 的场景，并跟随 @音频1 的节奏跳舞。
```

可用引用写法：

| 写法               | 含义                                         |
| ------------------ | -------------------------------------------- |
| `@图片1`、`@图片2` | 按图片数组内部顺序引用                       |
| `@音频1`、`@音频2` | 按音频数组内部顺序引用                       |
| `@1`、`@2`         | 按全部素材的提交顺序引用；图片在前、音频在后 |

按类型引用更清晰，推荐优先使用 `@图片N` 和 `@音频N`。

### 7.5 素材限制与格式

| 素材 |      数量 | 建议格式            | 其他说明                                                 |
| ---- | --------: | ------------------- | -------------------------------------------------------- |
| 图片 | 最多 9 张 | JPG/JPEG、PNG、WebP | 最大边超过 1920 像素时，上游可能等比例优化；不会主动裁剪 |
| 音频 | 最多 3 段 | MP3、WAV、M4A       | 单段建议控制在 2–15 秒；兼容层还可识别 AAC、OGG、FLAC    |
| 视频 |         0 | 不适用              | 本批 AZHW 模型不开放视频参考                             |

如果传入 `input_videos` 或 `video_url`，视频不会参与生成。调用方应在提交前直接拒绝这类参数，不要让用户误以为视频已经生效。

公网 URL 应保留可识别的文件扩展名。素材格式、编码、最小尺寸、宽高比或内容安全不符合上游规则时，任务仍可能失败，请读取任务的 `error` 字段。

### 7.6 全能参考完整示例

下面的请求可替换为上表任意一个 AZHW 模型 ID：

```bash
curl -X POST "https://feicai123.top/v1/videos" \
  -H "Authorization: Bearer sk-YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "seedance-2.0-vip-720p-azhw-feicai",
    "prompt": "@图片1 中的人物进入 @图片2 的夜景街道，保持人物、服装和场景一致，并跟随 @音频1 的节奏自然行走。",
    "duration": 10,
    "ratio": "16:9",
    "reference_mode": "omni",
    "input_images": [
      "https://cdn.example.com/ref/character.png",
      "https://cdn.example.com/ref/night-street.png"
    ],
    "audio_url_list": [
      "https://cdn.example.com/ref/music.mp3"
    ]
  }'
```

### 7.7 首尾帧示例

```json
{
  "model": "seedance-2.0-933-1080p-azhw-feicai",
  "prompt": "镜头从首帧自然过渡到尾帧，人物动作连贯，环境光线保持一致。",
  "duration": 8,
  "ratio": "16:9",
  "reference_mode": "both_frames",
  "input_images": [
    "https://cdn.example.com/ref/start-frame.png",
    "https://cdn.example.com/ref/end-frame.png"
  ]
}
```

首尾帧模式不要再传 `audio_url_list`。

### 7.8 文生视频示例

```json
{
  "model": "seedance-2.0-vip-4k-azhw-feicai",
  "prompt": "清晨云海之上的雪山，阳光逐渐照亮山脊，电影级航拍镜头缓慢向前推进。",
  "duration": 6,
  "ratio": "21:9",
  "reference_mode": "text_to_video"
}
```

### 7.9 旧调用方兼容字段

新接入请只使用 `input_images` 和 `audio_url_list`。以下字段仅为旧调用方兼容：

| 推荐字段         | 兼容的旧字段                                                                    |
| ---------------- | ------------------------------------------------------------------------------- |
| `input_images`   | `images`、`reference_images`；单图还兼容 `image`、`image_url`                   |
| `audio_url_list` | `input_audios`、`audios`、`reference_audios`；单音频还兼容 `audio`、`audio_url` |
| `ratio`          | `aspect_ratio`                                                                  |
| `duration`       | `seconds`                                                                       |

不要在同一次请求中混用推荐字段和旧字段，否则素材可能重复计入。

---

## 8. 标准错误响应

创建任务阶段的错误通常采用：

```json
{
  "error": {
    "message": "错误说明 (request id: ...)",
    "type": "new_api_error",
    "code": "invalid_request"
  }
}
```

常见 HTTP 状态码：

| 状态码 | 含义                                       | 建议处理                 |
| -----: | ------------------------------------------ | ------------------------ |
|    400 | 参数错误、缺少必填字段、素材数量或格式错误 | 修正请求后再提交         |
|    401 | Token 缺失或无效                           | 检查 `Authorization`     |
|    403 | Token 被禁用、模型未授权或 IP 限制         | 检查控制台配置           |
|    429 | 触发频率或并发限制                         | 指数退避后重试           |
|    500 | 网关内部错误                               | 保存 `request id` 后反馈 |
|    503 | 当前模型没有可用渠道                       | 稍后重试或切换模型       |

常见错误码：

| `code`                    | 含义                          |
| ------------------------- | ----------------------------- |
| `invalid_request`         | 请求结构或参数不正确          |
| `invalid_token`           | Token 无效                    |
| `model_not_found`         | 模型不存在或当前 Token 无权限 |
| `insufficient_user_quota` | 账户余额不足                  |
| `rate_limit_exceeded`     | 触发限流                      |
| `prompt_blocked`          | 提示词或素材未通过内容审核    |
| `do_request_failed`       | 上游请求失败                  |

任务已经创建后发生的生成错误会出现在轮询结果的 `error` 字段中。此时不要继续轮询，也不要在没有业务确认的情况下自动重新创建付费任务。

---

## 9. 上线前检查清单

### 9.1 所有模型

- [ ] 使用 `Authorization: Bearer sk-...`
- [ ] 请求地址为 `https://feicai123.top/v1/videos`
- [ ] 保存创建响应中的任务 `id`
- [ ] 以约 5 秒间隔轮询
- [ ] 正确处理 `failed`，并保存 `request id`
- [ ] 完成后使用 `metadata.url` 或 `/content` 下载
- [ ] 创建请求不做无条件自动重试

### 9.2 PI

- [ ] `seconds` 为字符串 `"15"`
- [ ] `resolution` 为 `"720p"`
- [ ] 第 1 张图放 `image_url`
- [ ] 第 2–9 张图放 `reference_image_urls`
- [ ] 音频使用 `audio_urls`
- [ ] 使用音频时至少带 1 张图片
- [ ] 不发送 `video_config` 或 `reference_mode`

### 9.3 SD2

- [ ] 使用 `image` 字符串，且只传 1 个公网 URL
- [ ] 图片全部为公网 HTTP/HTTPS URL
- [ ] `duration` 为 11–15
- [ ] `resolution` 为 `720p`
- [ ] `ratio` 只用 `16:9` 或 `9:16`
- [ ] 不传音频和视频
- [ ] `n` 为 1

### 9.4 AZHW

- [ ] 只使用本文列出的 8 个模型
- [ ] `duration` 为 4–15
- [ ] 分辨率由模型名选择，不依赖请求体切换
- [ ] 图片使用 `input_images`，最多 9 张
- [ ] 音频使用 `audio_url_list`，最多 3 段
- [ ] 不传视频参考
- [ ] 首尾帧使用 `both_frames`，且恰好 2 张图片、不带音频
- [ ] 提示词优先使用 `@图片N`、`@音频N`

---

## 10. 飞彩 Seedance 模型名称说明与选型

这些名称是飞彩 API 暴露的模型/渠道别名，不应仅凭名称将 `vip`、`933`、`fast`、
`mini`、`pi` 或 `sd2` 解释为字节跳动官方公布的独立基础模型版本。下表中的“稳定”与
“不算稳定但便宜”来自当前渠道清单，是供应侧的经验标注，不是可用性承诺；实际选型仍应
以小规模验收结果、价格和当时的渠道状态为准。

| 模型名称 | 名称拆解与定位 | 输出画质 | 当前渠道备注 | 适合场景 |
| -------- | -------------- | -------- | ------------ | -------- |
| `seedance-2.0-vip-720p-azhw-feicai` | Seedance 2.0 的 AZHW `vip` 优质渠道，标准 720p 档 | 720p | 稳定、偏贵 | 日常生产、先保证成功率；无需高分辨率时优先 |
| `seedance-2.0-vip-1080p-azhw-feicai` | 同一 `vip` 渠道的 1080p 档 | 1080p | 稳定、偏贵 | 成片交付、需要全高清细节 |
| `seedance-2.0-vip-4k-azhw-feicai` | 同一 `vip` 渠道的 4K 档 | 4K | 稳定、偏贵 | 大屏或后期裁切；通常成本和生成耗时最高 |
| `seedance-2.0-vip-720p-fast-azhw-feicai` | `vip` 渠道的 Fast 变体；名称表示速度取向，但不代表固定时延保证 | 720p | 稳定、偏贵 | 预览、迭代和更看重响应速度的任务 |
| `seedance-2.0-vip-720p-mini-azhw-feicai` | `vip` 渠道的 Mini 变体；名称表示轻量取向，不应直接等同于标准档画质 | 720p | 稳定、偏贵 | 草稿、批量试提示词、成本或速度敏感任务 |
| `seedance2.0-sd2-feicai` | 飞彩的 `sd2` 兼容入口；与 AZHW 系列请求合同不同，仅支持 1 张图片，不支持音频或视频参考 | 720p | 不算稳定但便宜 | 仅需单张图片参考、可接受失败重试的低成本任务 |
| `seedance-933-pro-pi-feicai` | 飞彩的 `933` Pro PI 多模态入口；固定 15 秒，支持图片、视频和音频参考 | 720p | 不算稳定但便宜 | 需要参考视频，或需要固定 15 秒成片的任务 |
| `seedance-2.0-933-720p-azhw-feicai` | AZHW `933` 渠道的 720p 档，能力合同与 AZHW 系列一致 | 720p | 不算稳定但便宜 | 低成本验证、草稿生成 |
| `seedance-2.0-933-1080p-azhw-feicai` | AZHW `933` 渠道的 1080p 档 | 1080p | 不算稳定但便宜 | 预算有限但需要全高清输出 |
| `seedance-2.0-933-4k-azhw-feicai` | AZHW `933` 渠道的 4K 档 | 4K | 不算稳定但便宜 | 预算有限的 4K 尝试；生产使用前应重点验收稳定性 |

快速选型建议：

- 优先稳定性：选择 `vip`；720p、1080p、4K 按最终交付分辨率选择。
- 优先生成速度：先试 `vip-720p-fast`，但应实测首帧时间和总耗时。
- 优先低成本试错：先试 `vip-720p-mini`；若仍需降低价格且能接受波动，再试
  `sd2` 或 `933` 渠道。
- 需要参考视频：选择 `seedance-933-pro-pi-feicai`；AZHW 8 型不支持视频参考。
- 仅靠模型名无法比较真实画质、时延或成功率；不要把 `4k` 自动理解为内容细节一定优于
  1080p，也不要把 `fast` 理解为服务等级保证。
