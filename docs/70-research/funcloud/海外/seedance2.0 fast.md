# Seedance 2.0 Fast 视频生成 API 对接文档

## 概述

Seedance 2.0 Fast 视频生成接口，支持文本、图片、视频、音频等多模态输入生成视频。

**特点**：
- ⚡ **更快速度**：生成速度比标准版更快
- 💰 **更低成本**：价格约为标准版的 70-80%
- 🎯 **适用场景**：注重成本与速度，不要求极限品质的场景

**限制**：
- ⚠️ **分辨率限制**：仅支持 480p 和 720p，不支持 1080p

**Base URL**: `https://mm-internal-cn.leonecloud.com`

---

## 认证方式

所有接口均需要在请求头中携带 Token 进行认证：

```
Authorization: Bearer {YOUR_AUTH_TOKEN}
```

---

## 支持的使用场景

### 场景1：文生视频

纯文本描述生成视频，结果具有较大随机性。

```bash
curl -X POST "https://mm-internal-cn.leonecloud.com/api/v2/open/aigc/seedance2-0-fast" \
  -H "Authorization: Bearer your_auth_token_here" \
  -H "Content-Type: application/json" \
  -d '{
    "content": [
      {
        "type": "text",
        "text": "写实风格，晴朗的蓝天之下，一大片白色的雏菊花田，镜头逐渐拉近，最终定格在一朵雏菊花的特写上，花瓣上有几颗晶莹的露珠"
      }
    ],
    "ratio": "16:9",
    "duration": 5,
    "resolution": "720p"
  }'
```

### 场景2：图片参考生视频

传入参考图片，模型基于图片内容生成视频。

```bash
curl -X POST "https://mm-internal-cn.leonecloud.com/api/v2/open/aigc/seedance2-0-fast" \
  -H "Authorization: Bearer your_auth_token_here" \
  -H "Content-Type: application/json" \
  -d '{
    "content": [
      {
        "type": "text",
        "text": "女孩抱着狐狸，女孩睁开眼，温柔地看向镜头，狐狸友善地抱着，镜头缓缓拉出，女孩的头发被风吹动"
      },
      {
        "type": "image_url",
        "image_url": {
          "url": "https://example.com/fox_girl.png"
        },
        "role": "reference_image"
      }
    ],
    "ratio": "16:9",
    "duration": 5,
    "resolution": "720p"
  }'
```

### 场景3：多图参考生视频

传入多张参考图片，在提示词中通过 "图片1"、"图片2" 引用。

```bash
curl -X POST "https://mm-internal-cn.leonecloud.com/api/v2/open/aigc/seedance2-0-fast" \
  -H "Authorization: Bearer your_auth_token_here" \
  -H "Content-Type: application/json" \
  -d '{
    "content": [
      {
        "type": "text",
        "text": "[图片1]戴着眼镜穿着蓝色T恤的男生和[图片2]的柯基小狗，坐在[图片3]的草坪上，视频卡通风格"
      },
      {
        "type": "image_url",
        "image_url": { "url": "https://example.com/boy.png" },
        "role": "reference_image"
      },
      {
        "type": "image_url",
        "image_url": { "url": "https://example.com/dog.png" },
        "role": "reference_image"
      },
      {
        "type": "image_url",
        "image_url": { "url": "https://example.com/grass.png" },
        "role": "reference_image"
      }
    ],
    "ratio": "16:9",
    "duration": 5,
    "resolution": "720p"
  }'
```

### 场景4：多模态参考生成（图片+视频+音频）

同时参考图片、视频和音频素材进行视频生成。

```bash
curl -X POST "https://mm-internal-cn.leonecloud.com/api/v2/open/aigc/seedance2-0-fast" \
  -H "Authorization: Bearer your_auth_token_here" \
  -H "Content-Type: application/json" \
  -d '{
    "content": [
      {
        "type": "text",
        "text": "以图片1为首帧，全程使用视频1的第一视角构图，全程使用音频1作为背景音乐。第一人称视角果茶宣传广告"
      },
      {
        "type": "image_url",
        "image_url": {
          "url": "https://example.com/pic1.jpg"
        },
        "role": "reference_image"
      },
      {
        "type": "video_url",
        "video_url": {
          "url": "https://example.com/video1.mp4"
        },
        "role": "reference_video"
      },
      {
        "type": "audio_url",
        "audio_url": {
          "url": "https://example.com/audio1.mp3"
        },
        "role": "reference_audio"
      }
    ],
    "ratio": "16:9",
    "duration": 11,
    "resolution": "720p"
  }'
```

### 场景5：编辑视频

替换视频中的主体、局部画面重绘/修复等。

```bash
curl -X POST "https://mm-internal-cn.leonecloud.com/api/v2/open/aigc/seedance2-0-fast" \
  -H "Authorization: Bearer your_auth_token_here" \
  -H "Content-Type: application/json" \
  -d '{
    "content": [
      {
        "type": "text",
        "text": "将视频1中的房子外立面墙壁刷成蓝色，天气改为雪天"
      },
      {
        "type": "video_url",
        "video_url": {
          "url": "https://example.com/house_video.mp4"
        },
        "role": "reference_video"
      },
      {
        "type": "image_url",
        "image_url": {
          "url": "https://example.com/snow_scene.jpg"
        },
        "role": "reference_image"
      }
    ],
    "ratio": "16:9",
    "duration": 5,
    "resolution": "720p"
  }'
```

### 场景6：延长视频

向后延长已有视频，可同时指定音频。

```bash
curl -X POST "https://mm-internal-cn.leonecloud.com/api/v2/open/aigc/seedance2-0-fast" \
  -H "Authorization: Bearer your_auth_token_here" \
  -H "Content-Type: application/json" \
  -d '{
    "content": [
      {
        "type": "text",
        "text": "将视频1向后延长，汽车丝滑行驶到一片沙漠绿洲，背景音乐使用音频1"
      },
      {
        "type": "video_url",
        "video_url": {
          "url": "https://example.com/car_driving.mp4"
        },
        "role": "reference_video"
      },
      {
        "type": "audio_url",
        "audio_url": {
          "url": "https://example.com/car_bgm.mp3"
        },
        "role": "reference_audio"
      }
    ],
    "ratio": "16:9",
    "duration": 11,
    "resolution": "720p"
  }'
```

### 场景7：素材参考生成（真人 / 虚拟）

Seedance 2.0 Fast 支持使用可信素材库中的素材生成视频：

- **真人素材**：需先完成真人认证再上传。
- **虚拟素材**：用于虚拟人像，先创建素材组再上传，无需真人认证。

两类素材上传成功后均返回 `assetUrl`（格式 `asset://<assetId>`），在生成任务中应使用 `assetUrl`，不要使用素材的 OSS `fileUrl`。

完整素材链路见：[Seedance 2.0 素材库 API 对接文档](open-api-material.md)。

```bash
curl -X POST "https://mm-internal-cn.leonecloud.com/api/v2/open/aigc/seedance2-0-fast" \
  -H "Authorization: Bearer your_auth_token_here" \
  -H "Content-Type: application/json" \
  -d '{
    "content": [
      {
        "type": "text",
        "text": "图片1中的人物面带笑容，向镜头介绍产品"
      },
      {
        "type": "image_url",
        "image_url": {
          "url": "asset://asset-20260515143020-xxxxx"
        },
        "role": "reference_image"
      }
    ],
    "ratio": "16:9",
    "duration": 5,
    "resolution": "720p"
  }'
```

### 场景8：真人模式

开启 `realPersonMode` 后，直接传入普通图片 URL 即可，系统会自动对图片中的人物形象做真人化处理，再用于视频生成，**无需提前认证或上传素材**。适合「让图中真人开口介绍 / 动起来」这类需求。

```bash
curl -X POST "https://mm-internal-cn.leonecloud.com/api/v2/open/aigc/seedance2-0-fast" \
  -H "Authorization: Bearer your_auth_token_here" \
  -H "Content-Type: application/json" \
  -d '{
    "content": [
      {
        "type": "text",
        "text": "图片1中的真人面带笑容，向镜头介绍产品"
      },
      {
        "type": "image_url",
        "image_url": {
          "url": "https://example.com/person.jpg"
        },
        "role": "reference_image"
      }
    ],
    "ratio": "16:9",
    "duration": 5,
    "resolution": "720p",
    "realPersonMode": true
  }'
```

> **注意**：真人模式下图片需排队进行处理，任务会有额外的等待时长，请确保上传的人像素材符合合规要求（详见下方 [真人模式](#真人模式)）。

---

## 查询任务状态

```bash
curl -X GET "https://mm-internal-cn.leonecloud.com/api/v2/open/aigc/{taskId}" \
  -H "Authorization: Bearer your_auth_token_here"
```

---

## 接口详情

### 1. 创建 Seedance2 Fast 任务

**POST** `/api/v2/open/aigc/seedance2-0-fast`

创建一个 Seedance 2.0 Fast 视频生成任务。

**Content-Type**: `application/json`

#### 请求参数

| 参数 | 类型 | 必填 | 说明 |
|-----|------|-----|------|
| content | array | 是 | 内容数组，详见下方 [content 内容项](#content-内容项) |
| ratio | string | 否 | 宽高比：`16:9`(默认) / `9:16` / `1:1` / `4:3` / `3:4` / `21:9` / `adaptive` |
| duration | int | 否 | 视频时长（秒），范围 4~15，默认 5 |
| resolution | string | 否 | 输出分辨率：`480p` / `720p`(默认)。**注意：Fast 版本不支持 1080p** |
| generateAudio | bool | 否 | 是否生成音频，默认 `false` |
| watermark | bool | 否 | 是否带水印，默认 `false` |
| seed | int | 否 | 随机种子，用于复现结果 |
| cameraFixed | bool | 否 | 是否固定摄像头 |
| returnLastFrame | bool | 否 | 是否返回视频尾帧图片URL |
| realPersonMode | bool | 否 | 真人模式，默认 `false`。开启后系统会自动对输入图片做真人形象处理，详见下方 [真人模式](#真人模式) |
| callbackUrl | string | 否 | 任务完成后的回调通知 URL |

#### content 内容项

content 为数组，每个元素为一个内容项：

| 类型 | 字段 | 说明 |
|-----|------|------|
| text | `type`: "text", `text`: "提示词" | **必须**，视频描述提示词 |
| image_url | `type`: "image_url", `image_url`: {"url": "URL"}, `role`: 见下方 | 参考图片 |
| video_url | `type`: "video_url", `video_url`: {"url": "URL"}, `role`: "reference_video" | 参考视频 |
| audio_url | `type`: "audio_url", `audio_url`: {"url": "URL"}, `role`: "reference_audio" | 参考音频 |

#### image_url 的 role 取值

| role 值 | 说明 |
|---------|------|
| `reference_image` | 参考图片 |
| `first_frame` | 首帧图片 |
| `last_frame` | 尾帧图片 |

> **说明**：
> - content 数组中**必须包含至少一个 text 类型**的内容项作为提示词
> - 在提示词中通过 "图片1"、"图片2"、"视频1"、"音频1" 引用对应位置的参考素材
> - 真人素材必须先走素材认证链路入库；虚拟素材先创建素材组再上传。生成时使用素材接口返回的 `assetUrl`（详见[素材库 API 对接文档](open-api-material.md)）
> - 输入包含视频时，计费价格与纯文本/图片输入不同
> - **Fast 版本仅支持 480p 和 720p 分辨率，传入 1080p 将返回错误**

#### 响应参数

| 参数 | 类型 | 说明 |
|-----|------|------|
| code | int | 状态码，0 表示成功 |
| msg | string | 状态信息 |
| data.taskId | string | 任务 ID，用于查询任务状态 |
| data.status | string | 任务状态，创建时固定为 `processing` |
| data.createdAt | string | 创建时间 |

#### 响应示例

```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "taskId": "task_20260514150000_abc12345",
    "status": "processing",
    "createdAt": "2026-05-14 15:00:00"
  }
}
```

---

### 2. 查询任务状态

**GET** `/api/v2/open/aigc/{taskId}`

查询指定任务的状态和结果。

#### 路径参数

| 参数 | 类型 | 必填 | 说明 |
|-----|------|-----|------|
| taskId | string | 是 | 任务 ID |

#### 响应参数

| 参数 | 类型 | 说明 |
|-----|------|------|
| code | int | 状态码，0 表示成功 |
| msg | string | 状态信息 |
| data.taskId | string | 任务 ID |
| data.status | string | 任务状态：`processing` / `completed` / `failed` |
| data.result | array | 结果 URL 数组（status 为 `completed` 时返回） |
| data.errorCode | string | 错误码（status 为 `failed` 时返回） |
| data.errorMsg | string | 错误信息（status 为 `failed` 时返回） |
| data.createdAt | string | 创建时间 |
| data.completedAt | string | 完成时间（已完成时返回） |

#### 响应示例（处理中）

```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "taskId": "task_20260514150000_abc12345",
    "status": "processing",
    "createdAt": "2026-05-14 15:00:00"
  }
}
```

#### 响应示例（成功）

```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "taskId": "task_20260514150000_abc12345",
    "status": "completed",
    "result": [
      "https://example.com/output/video_abc12345.mp4"
    ],
    "createdAt": "2026-05-14 15:00:00",
    "completedAt": "2026-05-14 15:02:30"
  }
}
```

#### 响应示例（失败）

```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "taskId": "task_20260514150000_abc12345",
    "status": "failed",
    "errorCode": "INVALID_RESOLUTION",
    "errorMsg": "Seedance 2.0 Fast 不支持 1080p 分辨率，仅支持 480p 和 720p",
    "createdAt": "2026-05-14 15:00:00",
    "completedAt": "2026-05-14 15:00:05"
  }
}
```

---

## 真人模式

在创建任务时传入 `realPersonMode: true` 即可开启真人模式。开启后：

- **无需提前认证或上传素材**：直接在 `content` 中传入普通图片 URL（`image_url`），系统会自动对图片中的人物形象进行真人化处理后用于生成。
- **处理需要排队**：图片素材需排队进行处理，任务会有额外的等待时长，整体耗时通常比普通任务更长，请耐心轮询任务状态。

### 合规告知

开启真人模式即代表您确认上传的人像素材符合以下条件：

1. 您合法拥有该素材，并享有完整的使用及处分权限。素材不包含未获授权的第三方商标、标识类内容。
2. 素材不得与任何自然人肖像或形象雷同，素材不存在抄袭、盗用情形，不会侵害任何第三方的人格权、知识产权等合法权益。
3. 素材不包含违反法规、违背公序良俗、危害国家安全的内容。

如因上传素材引发的任何权利争议或法律责任，由上传方自行承担。

---

## 错误码说明

| 错误码 | 说明 | 解决方案 |
|--------|------|---------|
| INVALID_RESOLUTION | 不支持的分辨率 | Fast 版本仅支持 480p 和 720p，请修改 resolution 参数 |
| INSUFFICIENT_BALANCE | 余额不足 | 请充值后重试 |
| INVALID_CONTENT | 内容格式错误 | 检查 content 数组格式，必须包含至少一个 text 类型 |
| INVALID_URL | 素材 URL 无效 | 检查图片/视频/音频 URL 是否可访问 |
| TASK_NOT_FOUND | 任务不存在 | 检查 taskId 是否正确 |

---

## 常见问题

### Q1: Seedance 2.0 Fast 与标准版有什么区别？

**A**:
- **速度**：Fast 版本生成速度更快
- **价格**：Fast 版本价格约为标准版的 70-80%
- **分辨率**：Fast 版本仅支持 480p 和 720p，不支持 1080p
- **质量**：Fast 版本适合注重成本与速度的场景，标准版追求最高生成品质

### Q2: 如何选择使用 Fast 版本还是标准版？

**A**:
- 选择 **Fast 版本**：注重成本与速度，不要求极限品质，且不需要 1080p 分辨率
- 选择 **标准版**：追求最高生成品质，或需要 1080p 分辨率输出

### Q3: Fast 版本支持哪些分辨率？

**A**: Fast 版本仅支持 **480p** 和 **720p**，不支持 1080p。如果传入 1080p 将返回错误。

### Q4: 如何计费？

**A**:
- 按视频时长和分辨率计费
- 输入包含视频时，价格与纯文本/图片输入不同
- Fast 版本价格约为标准版的 70-80%
- 具体价格请咨询商务

---

## 技术支持

如有问题，请联系技术支持团队。