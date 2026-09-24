---
page-id: videos-minimax
kind: api-reference
last-verified: 2026-09-24
operations:
  - listModelArkVideoModels
  - retrieveModel
  - createModelArkVideoTask
  - getModelArkVideoTask
  - getVideoContent
---

# MiniMax H3 视频

H3 客户模型使用统一 **ModelArk V3**：创建与查询位于 `/api/v3/contents/generations/tasks`，
成功后使用返回的 `/v1/videos/{task_id}/content` 下载。所有请求携带
`Authorization: Bearer {{API_KEY_PLACEHOLDER}}`，创建使用 `Content-Type: application/json`。
完整响应、列表、错误与资金处理见 [ModelArk V3 标准视频](api-reference/videos/modelark)。

先用 `GET /v1/models/{{MODEL_ID_PLACEHOLDER}}` 读取当前模型的 `api.video`。客户模型名由部署方配置，
不要从名字判断能力。下表描述扩展后的合同；只有目录发布了对应的 `content_types` 和 `roles` 才能使用。
旧环境可能只有文本或参考图/音频，应以实际目录为准。代码与文档更新不代表每个站点已启用，也不代表生产验收完成。

## 创建参数

参数位于 JSON 顶层，不添加 `parameters` 包裹或供应商私有字段。

| 字段 | 扩展合同 | 省略时 |
| --- | --- | --- |
| `model` | 当前 Key 可用的客户模型 ID | 必填 |
| `content` | 1 个非空文本；参考模式最多 9 图＋3 视频＋3 音频（共 16 项）；首尾帧模式最多 2 图 | 必填 |
| `content[].text` | 文本最多 7000 个 Unicode 码点；不能只有空白 | 文本项必填 |
| `duration` | 4–15 的整数，单位秒；不接受 `-1` | 6 |
| `resolution` | `768p`、`2k`，使用小写 | `768p` |
| `ratio` | `16:9`、`21:9`、`4:3`、`1:1`、`3:4`、`9:16`、`adaptive` | `16:9` |
| `watermark` | 只能为 `false` | `false` |

纯文本请求显式传 `ratio: "adaptive"` 返回 `400 invalid_request`，不会自动改为 16:9；
携带图片、视频或音频时可使用 adaptive。首尾帧模式由输入图片决定比例；合法的 `ratio` 值会保留发送，
但上游按图生语义忽略该值，不承诺按显式比例裁剪。平台固定启用提示词优化，可能改写提示词以改善生成效果；
不开放对应开关。模型可生成带声音的视频，但未开放 `generate_audio`，传入 true 或 false 都会拒绝。
回调、seed、末帧返回、输出格式和其它未出现在当前元数据中的参数未开放。
不支持取消或删除任务；以 `operations[].supported` 为准。

## 图片、视频与音频输入

| 输入 | `type` | `role` | URL 字段 |
| --- | --- | --- | --- |
| 参考图片 | `image_url` | `reference_image` | `image_url.url` |
| 首帧 | `image_url` | `first_frame` | `image_url.url` |
| 尾帧 | `image_url` | `last_frame` | `image_url.url` |
| 参考视频 | `video_url` | `reference_video` | `video_url.url` |
| 参考音频 | `audio_url` | `reference_audio` | `audio_url.url` |

三种场景：纯文本；文本加参考图/视频/音频；文本加单首帧、单尾帧或首尾帧各一张。
首尾帧不能与任何参考图、参考视频或参考音频混用；重复首帧/尾帧和超出数量上限均返回
`400 invalid_request`，不创建任务、不占款。`role` 必须显式指定，不根据图片顺序推断。

图片和视频可传 HTTP(S) URL 或对应 MIME 的 Data URL；例如 `data:video/mp4;base64,...`。
不提供视频裸 Base64 或视频 multipart 上传。音频 HTTP(S) URL 原样传递；音频 Data URL、裸 Base64 和
multipart 文件复用 [标准参考音频传输](api-reference/videos/modelark#参考音频的-urlbase64-与文件)：
网关先上传本站对象存储，再交给生成服务。文件引用使用 `file://<本次附件字段名>`，不是本地路径。
音频存储不可用时返回 `503 reference_audio_unavailable`，不创建任务、不占款。

素材应符合 [H3 官方媒体要求](https://platform.minimax.cn/docs/api-reference/video-generation-v2-create)：
图片每张不超过 30 MB；参考视频最多 3 段、MP4/MOV、每段不超过 50 MB，单段 2–15 秒、
合计不超过 15 秒；参考音频限 WAV/MP3，单段 2–15 秒、总计不超过 15 秒且每段不超过 15 MB。
上游请求体上限为 64 MB；网关图片/视频单个 URL 字符串还受 20 MiB 限制，音频内联解码上限为
15 MiB，全请求另受站点配置限制。需同时满足；较大媒体请使用公网 URL。网关不下载参考视频、不检查实际媒体时长、
不转码；可访问性、媒体格式、内容审核和生成结果仍由上游校验。输入成功不保证每个素材都产生可辨识影响。

H3 标准渠道没有发布 `api.assets`，不提供素材库或复用域；不要调用素材/素材组接口，也不要传
`asset://` 或 `mm_file://`。有权访问模型的调用方发起合法素材/素材组操作时，明确返回
`422 unsupported_asset_operation`，不调用上游；视频中的不支持引用格式返回 `400 invalid_request`。
无模型访问权限时保持原有不可见错误语义。本渠道不提供供应商私有文件上传接口，不复用其它业务的
`/v1/files` 作为 H3 素材库。MiniMax [官方文件上传](https://platform.minimax.cn/docs/api-reference/file-management-upload)提供
`purpose=video_generation_input`，可上传图片、视频与音频，以 `mm_file://` 引用七天有效的文件。
这是官方临时文件素材机制，不等于本渠道已支持：当前连接按官方上传格式与列表接口实测均未通过。
请求级图片、视频和音频无需预先创建素材库资源。

## 参考请求

下面使用两张图和一段音频；按相同结构可扩展到 9 张图、3 段音频。替换示例 URL 为可访问的真实媒体，
并替换客户模型名。先确认目录发布了这些参数。

```bash
curl "{{SITE_BASE_URL}}/api/v3/contents/generations/tasks" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  -H "Content-Type: application/json" \
  -d '{
  "model": "{{MODEL_ID_PLACEHOLDER}}",
  "content": [
    {
      "type": "text",
      "text": "参考图片中的主体与场景，结合参考音频的节奏，生成自然连贯的竖屏视频。"
    },
    {
      "type": "image_url",
      "role": "reference_image",
      "image_url": {
        "url": "https://example.com/subject.jpg"
      }
    },
    {
      "type": "image_url",
      "role": "reference_image",
      "image_url": {
        "url": "https://example.com/scene.jpg"
      }
    },
    {
      "type": "audio_url",
      "role": "reference_audio",
      "audio_url": {
        "url": "https://example.com/music.mp3"
      }
    }
  ],
  "duration": 10,
  "resolution": "2k",
  "ratio": "9:16",
  "watermark": false
}'
```

首尾帧请求仍使用同一创建地址（只需首帧或尾帧时删除另一图片项）：

```json
{
  "model": "{{MODEL_ID_PLACEHOLDER}}",
  "content": [
    {
      "type": "text",
      "text": "从室内木桌自然过渡到湖岸场景。"
    },
    {
      "type": "image_url",
      "role": "first_frame",
      "image_url": {
        "url": "https://example.com/start.jpg"
      }
    },
    {
      "type": "image_url",
      "role": "last_frame",
      "image_url": {
        "url": "https://example.com/end.jpg"
      }
    }
  ],
  "duration": 6,
  "resolution": "768p",
  "ratio": "adaptive"
}
```

三参考视频请求（每个样例 URL 应替换为 2–15 秒的视频，三段合计不超过 15 秒）：

```json
{
  "model": "{{MODEL_ID_PLACEHOLDER}}",
  "content": [
    {
      "type": "text",
      "text": "参考三段视频的场景与运镜，生成连贯的新片段。"
    },
    {
      "type": "video_url",
      "role": "reference_video",
      "video_url": {
        "url": "https://example.com/one.mp4"
      }
    },
    {
      "type": "video_url",
      "role": "reference_video",
      "video_url": {
        "url": "https://example.com/two.mp4"
      }
    },
    {
      "type": "video_url",
      "role": "reference_video",
      "video_url": {
        "url": "https://example.com/three.mp4"
      }
    }
  ],
  "duration": 6,
  "resolution": "768p",
  "ratio": "16:9"
}
```

成功受理返回 `{"id":"task-public-id"}`。用同一 API Key 查询
`GET /api/v3/contents/generations/tasks/task-public-id`，状态为 `succeeded` 后鉴权下载
`content.video_url`。URL 是本站内容代理，不是公开分享链接，不应携带 Key 放进浏览器地址。
`create_outcome_unknown` 时保留请求 ID，不要重复 POST；参数不合法时在占款前拒绝。

## 下载窗口与验证范围

后台状态更新按约 15 分钟节奏查询；反复读取本站状态不会加速生成。成功后的内容下载在缓存缺失时可
按需查询临时地址，不受该后台节奏阻挡。生成结果只临时保留：当前上游文档给出成功后约 90 分钟的
结果保存窗口，不能从首次下载或本站观察到成功的时刻重新计时。请尽快下载；过期可能永久不可用，
重新查询不延长保留期，下载不可用也不会把成功任务改成失败或自动退款。

2026-09-24 的验证明确区分三个层次：

| 层次 | 已知事实 |
| --- | --- |
| 官方 H3 模型说明 | 768P/2K、4–15 秒；不套用 H3-Max 的分辨率范围 |
| 同一上游连接实测 | 3 图＋1 音频、9 图＋1 音频的 6 秒/768P/16:9；9 图＋3 音频的 10 秒/2K/9:16，均成功生成并下载 |
| 额外南向能力验证 | 单首帧、单尾帧、首尾帧组合（adaptive），以及三段参考视频（各 2 秒，16:9），均以 6 秒/768P 请求成功生成并下载；已纳入扩展合同，仍需标准网关真实验收 |
| 标准网关真实验收 | 文生 6 秒/768p 创建、查询和结算通过；受保护下载被测试机 DNS 安全检查阻止，尚未通过。新增多模态需补真实网关验收 |

15 秒、其它画幅、参考媒体模式的 adaptive、标准内联音频上传链路的真实 Provider 验收尚未完成，不能将官方枚举或
本地桩测试写成逐项实测通过。费用由当前模型价格决定；上游 credit 不能解释成 Token、秒或货币。

## English contract notes

Use ModelArk V3 with the same API key for creation, polling and protected content download. Read the model's
`api.video` first: an older deployment may still expose text-only 6-second generation. Expanded requests allow
exactly one nonempty text item (up to 7,000 Unicode code points), nine reference images, three reference videos
and three reference audios (16 content items total), or one first frame and/or one last frame. Duration is an integer from 4 to 15 seconds (default 6); resolution is lowercase `768p` or `2k`
(default `768p`). Ratios are `16:9` (default), `21:9`, `4:3`, `1:1`, `3:4`, `9:16` and `adaptive`.
Adaptive requires media. Frame generation derives its ratio from the input image; valid explicit ratios are forwarded but ignored by the provider for that mode. Watermark is fixed to false and prompt optimization is always enabled.
There is no `generate_audio` switch, although generated videos may contain sound.

Images use `image_url.url` with `reference_image`, `first_frame` or `last_frame`; video uses `video_url.url`
with `reference_video`; audio uses `audio_url.url` with `reference_audio`. First/last frames cannot mix with
any reference media, and each frame role can appear only once. Invalid combinations return 400 before a hold.
HTTP(S) references are forwarded. Inline images and videos use matching Data URLs. Reference videos must be
MP4/MOV, at most 50 MB each, 2–15 seconds each and 15 seconds total. The upstream request limit is 64 MB;
gateway image/video URL strings are limited to 20 MiB each, with additional site request limits. Use public
URLs for large media. Bare Base64 and multipart uploads are only supported for audio. Inline or multipart audio is uploaded
before the funding hold, following the standard audio transport contract; unavailable storage returns 503.
Asset libraries, asset/file IDs, cancellation and deletion are unsupported. Valid asset/group operations for
an accessible model return 422 `unsupported_asset_operation`; unsupported media references return 400
`invalid_request`. Other products' `/v1/files` endpoints do not provide an H3 file library.
MiniMax officially supports uploads with purpose `video_generation_input` and seven-day `mm_file://` references,
but the current upstream connection rejected the official upload and list request formats.
Media must also satisfy upstream file and duration limits; the gateway does not inspect media duration or transcode.

Background status queries run about every 15 minutes. Successful content downloads can refresh a temporary URL
on demand, independently of that schedule. Save results promptly within the upstream's approximately 90-minute
retention window after success; refreshing does not extend retention, and download expiry does not refund generation.
Do not repeat creation after an unknown outcome. Direct upstream tests passed up to nine images plus three audios
at 10 seconds, 2K and 9:16. Separate upstream tests also passed first-frame-only, last-frame-only and paired-frame
generation with adaptive ratio, plus three two-second reference videos, at requested 6 seconds and 768P;
these inputs are included in the expanded contract when exposed by the model metadata. The expanded standard gateway path still needs live acceptance; 15 seconds and the
remaining ratios are documented capabilities, not completed live tests. Credit evidence is not a token or currency amount.
