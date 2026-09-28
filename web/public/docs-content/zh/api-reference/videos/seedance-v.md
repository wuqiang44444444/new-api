---
page-id: videos-seedance-v
kind: api-reference
last-verified: 2026-09-28
operations:
  - listModelArkVideoModels
  - retrieveModel
  - createModelArkVideoTask
  - listModelArkVideoTasks
  - getModelArkVideoTask
  - getVideoContent
---

# Seedance V 视频

Seedance V使用统一 ModelArk V3：创建、列表与查询位于
`/api/v3/contents/generations/tasks`，成功后通过 `/v1/videos/{task_id}/content` 鉴权下载。
所有请求携带 `Authorization: Bearer {{API_KEY_PLACEHOLDER}}`；JSON 创建使用
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
| `api.assets.supported` | 是否启用本站托管图片；为 `true` 时读取素材操作、创建限制与复用域 |

模型列表、详情和价格目录复用同一参数投影。目录可见不等于当前 Key 可调用；
`available=false` 时按 `availability` 处理，不尝试创建。

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
遵循[标准参考音频传输](api-reference/videos/modelark#参考音频的-urlbase64-与文件)：
每段内联或文件音频不超过 15 MiB，网关在占款前上传；存储不可用时返回
`503 reference_audio_unavailable`。网关不检查实际媒体时长或转码，上游仍可能拒绝不可访问或不合规媒体。

本系列可配置本站托管图片。当 `api.assets.supported=true` 且
`management_mode=platform_hosted` 时，通过[素材接口](api-reference/assets)创建图片，
保存返回的 `id` 与 `reference`，随后把 `reference` 填入 `image_url.url`。
首帧、尾帧与参考图片均支持这种引用，也可以与直接图片 URL 混用，角色和组合限制保持不变。
网关在预扣前检查当前账号归属、素材状态和对象可用性，发送时转换为内部临时 URL。
同账号可在发布相同托管复用域的模型间使用素材；删除只停止新引用，已受理任务继续。

托管库支持图片创建、单项查询/删除，以及素材组创建/单项查询；不支持列表、更新或真人认证，
也不支持将音频、视频存入该图片库。非本站 `asset://` 引用在预扣前拒绝。
`api.assets.supported=false` 时素材操作仍返回 `422 unsupported_asset_operation`，直接图片 URL
继续可用，且不会自动导入素材库。是否开放托管能力以当前模型元数据为准。

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
使用创建时同一 API Key 查询并下载：

```bash
curl "{{SITE_BASE_URL}}/api/v3/contents/generations/tasks/task-public-id" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}"
```

`status=succeeded` 后读取返回的 `content.video_url`，携带相同鉴权保存文件。
请求了末帧且结果可用时读取 `content.last_frame_url`。不要把 API Key 拼入 URL。
`queued` / `running` 继续退避轮询；失败或过期依据本站状态与资金进度处理。
本系列不支持取消/删除，不要调用 DELETE 作为退款手段。

`400 invalid_request` 应修正参数；`503 create_outcome_unknown` 表示创建结果待核实，
保留请求 ID，不要重复 POST。成功内容应尽快下载，本站不承诺永久保存。
公开枚举说明当前合同范围，不代表每个档位与组合都已完成真实生成验收；
模型是否开放及价格以当前部署目录为准。

## English contract notes

Seedance V uses ModelArk V3. Discover the exact customer model ID and follow its
`api.video.documentation_path`; names such as `seedance-2-0-v` do not define capabilities.
Creation defaults are 5 seconds, 720p, adaptive ratio and no generated audio. Duration is 4–15 or 4–30
seconds according to metadata; `-1` selects intelligent duration. Resolution and image/video/audio limits
come from the same metadata. The 15-second profiles allow 9/3/3 media items; the 30-second profile allows
30/10/10. Some 15-second profiles support only 480p/720p; the 30-second profile does not support 4k.

First and last frames are limited to one each, a last frame requires a first frame, and frames cannot mix
with reference media. Reference audio requires a reference image or video, even when a text prompt is present.
Inline or multipart audio follows the standard audio transport contract and is uploaded before the funds hold.
Only `web_search` is allowed in tools; safety identifiers are limited to 64 characters. Execution expiry is
3600–259200 seconds, default 172800. Watermark and last-frame return default to false.
Seed, camera controls, frames, draft, output format, priority and service tier are not published.

Use the same API key for creation, polling and protected downloads. Creation returns a public task ID;
save content promptly after success. When `api.assets.supported=true` and management mode is
`platform_hosted`, create images through the asset API and use the returned `asset://fhas_*` reference in
`image_url.url`. References support first frames, last frames and reference images, including mixtures with
direct image URLs. Ownership, state and storage availability are checked before the funds hold; outbound
requests use temporary URLs. The same account can reuse images across models with the same hosted reuse scope.
Deletion prevents new references but does not interrupt accepted tasks. Direct URLs do not create stored assets.
The library supports image create/get/delete and group create/get, but not list/update, audio/video assets or
real-person verification. Other opaque references are rejected before the hold. Channels without hosted
assets still support direct URLs. Task deletion/cancellation remains unsupported.
Do not repeat a POST after `create_outcome_unknown`; consult task and funds progress.
