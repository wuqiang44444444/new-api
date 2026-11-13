---
page-id: videos-seedance-v
kind: api-reference
last-verified: 2026-11-13
operations:
  - listModelArkVideoModels
  - retrieveModel
  - createModelArkVideoTask
  - listModelArkVideoTasks
  - getModelArkVideoTask
  - getVideoContent
---

# Seedance V 视频

Seedance V 使用统一 ModelArk V3：创建、列表与查询位于
`/api/v3/contents/generations/tasks`，生成成功且交付就绪后，通过查询返回的 `content.video_url`
下载本站托管结果，无需附带 API Key。创建、查询和访问内容入口携带
`Authorization: Bearer {{API_KEY_PLACEHOLDER}}`；JSON 创建使用
`Content-Type: application/json`。公共响应、资金进度和错误处理见
[ModelArk V3 标准视频](api-reference/videos/modelark)。

## 先读取模型元数据

部署方可使用 `seedance-2-0-v` 等客户名。它们只是配置名称，
不代表可由名称推断的参数合同；请求必须使用当前目录返回的 ID。

```bash
curl "{{SITE_BASE_URL}}/v1/models/{{MODEL_ID_PLACEHOLDER}}" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}"
```

路径中的模型 ID 需要 URL 编码。也可查询 `GET /api/v3/contents/generations/models`。
确认 `available=true`、`api.video.protocol=modelark_v3`，并读取：

| 元数据 | 调用方用途 |
| --- | --- |
| `api.video.documentation_path` | 本系列指向 `/docs/api-reference/videos/seedance-v` |
| `api.video.creation.model` | 创建时使用的客户模型 ID |
| `api.video.creation.parameters` | 字段、默认值、枚举、上下限与特殊值 |
| `api.video.creation.content_types` | 文本、图片、视频、音频角色及各自数量上限 |
| `api.video.operations` | 支持创建、列表、查询、内容下载；删除/取消不支持 |
| `api.assets` / `api.hosted_assets` | 查看本站托管素材库的创建合同 |

`available=false` 时，通过 `availability` 查看不可调用的原因。

## 创建参数

字段均放在 JSON 顶层，不能添加供应商私有参数或 `parameters` 包裹。

| 字段 | 允许范围 | 省略时 |
| --- | --- | --- |
| `model` | 目录中的客户模型 ID | 必填 |
| `content` | 至少一项非空文本或合法媒体，使用下节角色 | 必填 |
| `duration` | 整数 4–15 或 4–30 秒，取当前元数据上限；`-1` 为智能时长 | 5 秒 |
| `resolution` | 取元数据 `enum`，按小写发送 | `720p` |
| `ratio` | `16:9`、`4:3`、`1:1`、`3:4`、`9:16`、`21:9`、`adaptive` | `adaptive` |
| `generate_audio` | `true` / `false`，是否生成音频 | `false` |
| `watermark` | `true` / `false` | `false` |
| `return_last_frame` | `true` / `false`，请求返回末帧 | `false` |
| `callback_url` | 合法 HTTP(S) 回调地址 | 不设置 |
| `execution_expires_after` | 整数 3600–259200 秒 | 172800 秒 |
| `tools` | 对象数组，目前每项只允许 `{"type":"web_search"}` | 不设置 |
| `safety_identifier` | 最多 64 个字符 | 不设置 |

当前元数据有三组参数范围；部署方可以使用任意客户名，不能通过名称匹配此表：

| 时长上限 | 分辨率枚举 | 图片 / 视频 / 音频上限 |
| --- | --- | --- |
| 15 秒 | `480p`、`720p`、`1080p`、`4k` | 9 / 3 / 3 |
| 15 秒 | `480p`、`720p` | 9 / 3 / 3 |
| 30 秒 | `480p`、`720p`、`1080p` | 30 / 10 / 10 |

`-1` 不是固定输出秒数；实际输出与费用按任务结果和模型价格处理。
`seed`、`camera_fixed`、`frames`、`draft`、`output_format`、`priority`、`service_tier`
均未发布，显式发送会被拒绝。不要因为其它 ModelArk 模型支持就附加这些字段。
回调不替代本站任务查询与资金进度；不能仅凭收到回调判定本站已结算。

## 媒体与组合限制

| 输入 | `type` | `role` | 媒体字段 |
| --- | --- | --- | --- |
| 提示词 | `text` | 不填写 | `text` |
| 首帧 / 尾帧 | `image_url` | `first_frame` / `last_frame` | `image_url.url` |
| 参考图片 | `image_url` | `reference_image` | `image_url.url` |
| 参考视频 | `video_url` | `reference_video` | `video_url.url` |
| 参考音频 | `audio_url` | `reference_audio` | `audio_url.url` |

- 首帧和尾帧各最多一张；尾帧必须搭配首帧，不能只传尾帧。
- 首尾帧不能与参考图片、参考视频或参考音频混用。
- 参考音频必须搭配参考图片或参考视频；文本加音频仍属于不支持的纯音频参考组合。
- 各媒体数量以元数据为准；分辨率、时长或组合不合法时在占款前拒绝。

推荐使用可访问的 HTTPS 媒体 URL。参考音频还支持 Base64、Data URL 和 multipart 文件，
遵循[标准参考音频传输](api-reference/videos/modelark#参考音频的-url-base64-与文件)：
每段内联或文件音频不超过 15 MiB。存储不可用时返回 `503 reference_audio_unavailable`。
参考媒体须能在生成期间访问，并满足目标模型的格式和时长要求。

[托管素材库](api-reference/assets#平台托管素材)统一接收图片、MP3 和 MP4。添加新 Seedance 模型后，
可直接使用同账号下已有素材的 `reference`，填入对应的媒体 URL 字段。提交生成时才按目标模型校验媒体限制。
例如，已入库的 MP3 用于 Seedance V 生成时，仍须搭配参考图片或参考视频。

## 创建与查询示例

以下示例使用一张参考图和一段音频；替换客户模型与媒体 URL 后提交：

```bash
curl "{{SITE_BASE_URL}}/api/v3/contents/generations/tasks" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "{{MODEL_ID_PLACEHOLDER}}",
    "content": [
      {"type":"text","text":"参考图片中的场景与音频节奏，生成自然连贯的镜头。"},
      {
        "type": "image_url",
        "role": "reference_image",
        "image_url": {"url": "https://example.com/scene.jpg"}
      },
      {
        "type": "audio_url",
        "role": "reference_audio",
        "audio_url": {"url": "https://example.com/music.mp3"}
      }
    ],
    "duration": 4,
    "resolution": "480p",
    "ratio": "16:9",
    "generate_audio": true,
    "watermark": false
  }'
```

成功受理返回 HTTP `200` 与平台 ID：`{"id":"task-public-id"}`。
使用创建时同一 API Key 查询：

```bash
curl "{{SITE_BASE_URL}}/api/v3/contents/generations/tasks/task-public-id" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}"
```

`status=succeeded` 后还需检查 `content.delivery_status`：`preparing` 时继续查询原任务，
`ready` 时读取返回的 `content.video_url`；`unavailable` 时保留 ID 核查，不重新生成。
请求了末帧且结果可用时读取 `content.last_frame_url`。下载完整签名 URL 时不附带平台或 Provider Key：

```bash
curl --fail "$VIDEO_URL" --output result.mp4
```

`VIDEO_URL` 为本次查询返回的完整地址。签名从每次签发起有效 24 小时（86400 秒）。文件仍保留且授权有效时，
重新查询原任务可续签。
既有 `/v1/videos/{task_id}/content` 入口仍需平台鉴权，就绪后返回 `302`；curl 使用
`--location` 跟随跳转，不用 `--location-trusted`。完整错误处理见
[标准下载合同](api-reference/videos/modelark#下载视频或末帧)。
`queued` / `running` 继续退避轮询；失败或过期依据本站状态与资金进度处理。
本系列不支持取消/删除，不要调用 DELETE 作为退款手段。

`400 invalid_request` 应修正参数；`503 create_outcome_unknown` 表示创建结果待核实，
保留请求 ID，不要重复 POST。
