---
status: current
owner: Dev Team
last-reviewed: 2026-07-17
---

# Seedance 2.0 海外官 Key

> 文档中心：https://www.moxing.pro/docs/models/dreamina-seedance-2-0-260128

Seedance 2.0 海外版是字节跳动推出的新一代 AI 视频生成模型，被官方定位为"可导演的电影级全流程生成引擎"。其核心能力在于将模糊的创意转化为精确的 AI 指令，让用户从"祈祷 AI 能听懂"转变为"手握控制台的导演"。

## 基本信息

| 字段 | 值 |
|------|-----|
| Model ID | `dreamina-seedance-2-0-260128` |
| 提供方 | 豆包 |
| 类型 | video |
| 能力 | 视频生成 |
| 接口 | `POST /v1/ark/media/generations` |

> 方舟官方直通线（海外官Key）：请求/响应与 BytePlus ModelArk 原生格式一致，平台仅做鉴权、路由、估价与账务。本模型**不支持** `POST /v1/media/generations`（V2）。

## 接入指引

本模型走海外官 Key 方舟直通，视频生成与参考素材分属不同接口，请按顺序接入：

| 步骤 | 说明 | 文档 |
|------|------|------|
| 1a | （虚拟人像）创建素材组 `GroupType=AIGC` | 海外官Key 真人素材库 |
| 1b | （真人肖像）H5 认证并上传参考素材，获得 `asset://` | 海外官Key 真人素材库 |
| 2 | 提交视频生成任务，在 `content[]` 中引用 `asset://` 或公网 URL | 海外官Key 视频 |
| 3 | 轮询任务结果 | 海外官Key 视频 |

> 参考素材引用格式：`asset://{asset_id}`（如 `asset-20260318-xxx`），须为 Active 且属于当前 API Key 用户；平台校验归属后原样转发，不做 id 改写。

## 生成场景

- 文生视频
- 图生视频（首帧）
- 图生视频（首尾帧）
- 参考生视频（多模态可组合）
- 编辑视频
- 延长视频

## 请求参数

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `duration` | any | 否 | 时长（秒）；整数，范围 4–15；或 `-1` 表示智能时长。缺省时平台按模型默认时长估价 |
| `model` | string | 是 | 模型 ID；固定使用 `dreamina-seedance-2-0-260128`，与 BytePlus Ark 上游模型名一致，平台不做 model mapping |
| `resolution` | string | 否 | 分辨率；支持 `480p` / `720p` / `1080p` / `4k`。缺省时平台按模型配置估价（常见 720p） |
| `content` | any | 是 | 多模态输入；必填数组。元素含 `type`（`text` / `image_url` / `video_url` / `audio_url`）与 `role`（`first_frame` / `last_frame` / `reference_image` / `reference_video` / `reference_audio` 等） |
| `generate_audio` | any | 否 | 同步生成音频；`true` 时生成带音频视频；`false` 时不生成 |
| `ratio` | any | 否 | 画幅比例；`16:9` / `4:3` / `1:1` / `3:4` / `9:16` / `21:9` / `adaptive`。`adaptive` 表示由模型根据输入自动选择比例，为默认推荐值 |
| `watermark` | any | 否 | 水印；本平台不解析，原样透传给上游。上游默认添加水印（`watermark=true`）；如需无水印请显式传 `watermark=false` |

### content role 映射

| 组合 | 场景 |
|------|------|
| `text`（无 role） | 文生视频 |
| `image_url` + `first_frame` | 图生视频（首帧） |
| `image_url` + `first_frame` + `last_frame` | 图生视频（首尾帧） |
| `reference_image` / `reference_video` / `reference_audio` | 参考生视频 |
| `reference_image` / `reference_video` / `reference_audio` | 编辑/延长视频 |

## 响应格式

| 字段 | 类型 | 说明 |
|------|------|------|
| `content.video_url` | string | 终态 `succeeded` 时的视频地址（下载链接有效期通常 48 小时，见 `execution_expires_after`） |
| `error` | string | 终态失败时的错误信息（如有） |
| `id` | string | 创建成功时返回的上游任务 ID，用于 GET 轮询（保留约 7 天） |
| `status` | string | `queued` / `running` / `succeeded` / `failed` / `expired` / `cancelled` |

## 错误码

| 错误码 | HTTP | 说明 |
|--------|------|------|
| `invalid_request_error` | 400 | 请求参数缺失、格式错误或字段值不在允许范围内 |
| `invalid_api_key` | 401 | API Key 为空、格式错误或已失效 |
| `insufficient_quota` | 402 | 账户余额不足；平台在转发上游前拦截，不会产生上游任务 |
| `model_not_found` | 404 | 请求的 model 不存在或当前账户无权访问 |
| `rate_limit_exceeded` | 429 | 触发并发或 RPM 限流；建议指数退避后重试 |
| `internal_server_error` | 500 | 平台内部异常 |
| `upstream_unavailable` | 502 | 模型服务暂时不可用或超时 |

## 限制

| 限制项 | 说明 |
|--------|------|
| 人脸限制 | 2.0 系列不支持直接上传含真人脸的参考图/视频；可使用 `asset://` 数字角色库等合规资产 |
| 分辨率 | `480p` / `720p` / `1080p` / `4k` |
| 参考素材 | 公网 URL、`data:...;base64,...` 或 `asset://`（须为本账号海外官Key 真人素材库 Active 资产） |
| 多模态数量 | 参考生场景：图片 0~9 张、视频 0~3 个、音频 0~3 个；不支持纯音频或仅文本+音频 |
| 接入线 | 方舟官方直通（本页）；不支持 V2 `/v1/media/generations` |
| 画幅比例 | `16:9` / `4:3` / `1:1` / `3:4` / `9:16` / `21:9` / `adaptive` |
| 视频时长 | 4–15 秒，或 `-1` 智能时长 |

## 模型价格

| 生成方式 | 规格 | 单价 |
|---------|------|------|
| 文生视频 | 分辨率 480p / 视频输入 是 | ¥30.1 / 百万 tokens |
| 文生视频 | 分辨率 480p / 视频输入 否 | ¥49 / 百万 tokens |
| 文生视频 | 分辨率 720p / 视频输入 是 | ¥30.1 / 百万 tokens |
| 文生视频 | 分辨率 720p / 视频输入 否 | ¥49 / 百万 tokens |
| 文生视频 | 分辨率 1080p / 视频输入 是 | ¥32.9 / 百万 tokens |
| 文生视频 | 分辨率 1080p / 视频输入 否 | ¥53.9 / 百万 tokens |
| 文生视频 | 分辨率 4K / 视频输入 是 | ¥16.8 / 百万 tokens |
| 文生视频 | 分辨率 4K / 视频输入 否 | ¥28 / 百万 tokens |
| 图生视频 | 分辨率 480p / 视频输入 是 | ¥30.1 / 百万 tokens |
| 图生视频 | 分辨率 480p / 视频输入 否 | ¥49 / 百万 tokens |
| 图生视频 | 分辨率 720p / 视频输入 是 | ¥30.1 / 百万 tokens |
| 图生视频 | 分辨率 720p / 视频输入 否 | ¥49 / 百万 tokens |
| 图生视频 | 分辨率 1080p / 视频输入 是 | ¥32.9 / 百万 tokens |
| 图生视频 | 分辨率 1080p / 视频输入 否 | ¥53.9 / 百万 tokens |
| 图生视频 | 分辨率 4K / 视频输入 是 | ¥16.8 / 百万 tokens |
| 图生视频 | 分辨率 4K / 视频输入 否 | ¥28 / 百万 tokens |
| 视频生视频 | 分辨率 480p / 视频输入 是 | ¥30.1 / 百万 tokens |
| 视频生视频 | 分辨率 480p / 视频输入 否 | ¥49 / 百万 tokens |
| 视频生视频 | 分辨率 720p / 视频输入 是 | ¥30.1 / 百万 tokens |
| 视频生视频 | 分辨率 720p / 视频输入 否 | ¥49 / 百万 tokens |
| 视频生视频 | 分辨率 1080p / 视频输入 是 | ¥32.9 / 百万 tokens |
| 视频生视频 | 分辨率 1080p / 视频输入 否 | ¥53.9 / 百万 tokens |
| 视频生视频 | 分辨率 4K / 视频输入 是 | ¥16.8 / 百万 tokens |
| 视频生视频 | 分辨率 4K / 视频输入 否 | ¥28 / 百万 tokens |

## 示例

```bash
curl --request POST https://www.moxing.pro/v1/ark/media/generations \
  --header 'Authorization: Bearer sk-your-api-key-here' \
  --header 'Content-Type: application/json' \
  --data-raw '{
    "content": [
      {
        "text": "一只橘猫在霓虹灯街道上滑滑板，电影感镜头，动态光影",
        "type": "text"
      }
    ],
    "duration": 5,
    "generate_audio": true,
    "model": "dreamina-seedance-2-0-260128",
    "ratio": "16:9",
    "resolution": "720p",
    "watermark": false
  }'
```
