# Seedance 2.0 视频生成 API 对接文档

## 概述

Seedance 2.0 视频生成接口，支持文本、图片、视频、音频等多模态输入生成视频。

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
curl -X POST "https://mm-internal-cn.leonecloud.com/api/v2/open/aigc/seedance2-0" \
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
curl -X POST "https://mm-internal-cn.leonecloud.com/api/v2/open/aigc/seedance2-0" \
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
curl -X POST "https://mm-internal-cn.leonecloud.com/api/v2/open/aigc/seedance2-0" \
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
curl -X POST "https://mm-internal-cn.leonecloud.com/api/v2/open/aigc/seedance2-0" \
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
curl -X POST "https://mm-internal-cn.leonecloud.com/api/v2/open/aigc/seedance2-0" \
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
curl -X POST "https://mm-internal-cn.leonecloud.com/api/v2/open/aigc/seedance2-0" \
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

Seedance 2.0 支持使用可信素材库中的素材生成视频：

- **真人素材**：需先完成真人认证再上传。
- **虚拟素材**：用于虚拟人像，先创建素材组再上传，无需真人认证。

两类素材上传成功后均返回 `assetUrl`（格式 `asset://<assetId>`），在 Seedance 2.0 生成任务中应使用 `assetUrl`，不要使用素材的 OSS `fileUrl`。

完整素材链路见：[Seedance 2.0 素材库 API 对接文档](open-api-material.md)。

```bash
curl -X POST "https://mm-internal-cn.leonecloud.com/api/v2/open/aigc/seedance2-0" \
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
curl -X POST "https://mm-internal-cn.leonecloud.com/api/v2/open/aigc/seedance2-0" \
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

### 1. 创建 Seedance2 任务

**POST** `/api/v2/open/aigc/seedance2-0`

创建一个 Seedance 2.0 视频生成任务。

**Content-Type**: `application/json`

#### 请求参数

| 参数 | 类型 | 必填 | 说明 |
|-----|------|-----|------|
| content | array | 是 | 内容数组，详见下方 [content 内容项](#content-内容项) |
| ratio | string | 否 | 宽高比：`16:9`(默认) / `9:16` / `1:1` / `4:3` / `3:4` / `21:9` / `adaptive` |
| duration | int | 否 | 视频时长（秒），范围 4~15，默认 5 |
| resolution | string | 否 | 输出分辨率：`480p` / `720p`(默认) / `1080p` / `480pto720p`（增强到 720p，按 720p 计费，详见 [增强分辨率文档](open-api-seedance2-超分%20%28国内%29.md)） |
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
> - 真人素材必须先走素材认证链路入库；虚拟素材先创建素材组再上传。生成时使用素材接口返回的 `assetUrl`
> - 输入包含视频时，计费价格与纯文本/图片输入不同

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
    "taskId": "task_20260429150000_abc12345",
    "status": "processing",
    "createdAt": "2026-04-29 15:00:00"
  }
}
```

---

### 2. 查询任务状态

**GET** `/api/v2/open/aigc/{taskId}`

查询单个任务的执行状态。

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
| data.status | string | 任务状态：`processing` / `success` / `failed` |
| data.result | string[] | 生成的视频 URL 列表（成功时返回） |
| data.errorMsg | string | 错误信息（失败时返回） |
| data.createdAt | string | 创建时间 |
| data.updatedAt | string | 更新时间 |
| data.output | object | 官方格式结果，详见下方 |
| data.output.id | string | 上游任务 ID |
| data.output.model | string | 模型名称 |
| data.output.status | string | 官方状态：`submitted` / `running` / `succeeded` / `failed` |
| data.output.content | object | 结果内容（成功时返回） |
| data.output.content.video_url | string | 视频 URL |
| data.output.content.last_frame_url | string | 尾帧图片 URL（需开启 returnLastFrame） |
| data.output.error | object | 错误信息（失败时返回） |
| data.output.error.code | string | 错误码 |
| data.output.error.message | string | 错误消息 |
| data.output.created_at | int | 创建时间（Unix 秒） |
| data.output.updated_at | int | 更新时间（Unix 秒） |

#### 响应示例

**处理中**
```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "taskId": "task_20260429150000_abc12345",
    "status": "processing",
    "createdAt": "2026-04-29 15:00:00",
    "updatedAt": "2026-04-29 15:00:05",
    "output": {
      "id": "cgt-20260429150000-xxxxx",
      "model": "doubao-seedance-2-0-260128",
      "status": "running",
      "created_at": 1745910000,
      "updated_at": 1745910005
    }
  }
}
```

**成功**
```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "taskId": "task_20260429150000_abc12345",
    "status": "success",
    "result": [
      "https://fc-gw-sh.oss-accelerate.aliyuncs.com/videos/2026/04/29/output.mp4"
    ],
    "createdAt": "2026-04-29 15:00:00",
    "updatedAt": "2026-04-29 15:03:30",
    "output": {
      "id": "cgt-20260429150000-xxxxx",
      "model": "doubao-seedance-2-0-260128",
      "status": "succeeded",
      "content": {
        "video_url": "https://fc-gw-sh.oss-accelerate.aliyuncs.com/videos/2026/04/29/output.mp4"
      },
      "created_at": 1745910000,
      "updated_at": 1745910210
    }
  }
}
```

**失败**
```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "taskId": "task_20260429150000_abc12345",
    "status": "failed",
    "errorMsg": "视频生成失败，请重试",
    "createdAt": "2026-04-29 15:00:00",
    "updatedAt": "2026-04-29 15:02:00",
    "output": {
      "id": "cgt-20260429150000-xxxxx",
      "model": "doubao-seedance-2-0-260128",
      "status": "failed",
      "error": {
        "code": "InternalError",
        "message": "视频生成失败，请重试"
      },
      "created_at": 1745910000,
      "updated_at": 1745910120
    }
  }
}
```

---

### 3. 批量查询任务状态

**POST** `/api/v2/open/aigc/batch`

批量查询多个任务的执行状态（最多 100 个）。

#### 请求参数

| 参数 | 类型 | 必填 | 说明 |
|-----|------|-----|------|
| taskIds | string[] | 是 | 任务 ID 列表，最多 100 个 |

#### 请求示例

```json
{
  "taskIds": ["task_20260429150000_abc12345", "task_20260429150200_def67890"]
}
```

---

## 回调通知

当任务完成（成功或失败）时，如果创建任务时提供了 `callbackUrl`，系统会向该 URL 发送 POST 请求。

### 回调请求

**Headers**
```
Content-Type: application/json
X-Funcloud-Event: task.completed
X-Funcloud-Signature: {签名}
```

**Body**
```json
{
  "event": "task.completed",
  "taskId": "task_20260429150000_abc12345",
  "status": "success",
  "result": ["https://fc-gw-sh.oss-accelerate.aliyuncs.com/videos/output.mp4"],
  "errorMsg": "",
  "timestamp": "2026-04-29T15:03:30+08:00",
  "signature": "a1b2c3d4e5f6..."
}
```

---

## 参数取值范围

### 分辨率 (resolution)

| 值 | 说明 |
|----|------|
| `480p` | 低清 |
| `720p` | 默认，推荐 |
| `1080p` | 高清 |
| `480pto720p` | 增强到 720p，成功后返回 [720p, 原始视频] 两条链接，按 720p 价格计费，单任务一次性扣费 |

### 宽高比 (ratio)

| 值 | 说明 |
|----|------|
| `16:9` | 横屏（默认） |
| `9:16` | 竖屏 |
| `1:1` | 方形 |
| `4:3` | 标准横屏 |
| `3:4` | 标准竖屏 |
| `21:9` | 超宽屏 |
| `adaptive` | 自适应（根据输入图片比例） |

### 时长 (duration)

- 范围：4 ~ 15 秒
- 默认：5 秒

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

## 错误码

| code | 说明 |
|------|------|
| 0 | 成功 |
| 10002 | 参数缺失或格式错误 |
| 10005 | API Key 无效或缺失 |
| 10006 | 余额不足 |
| 30003 | 任务不存在 |
| 90003 | 服务器内部错误 |

---

## 最佳实践

### 1. 轮询策略

建议的轮询间隔：
- 前 30 秒：每 3 秒查询一次
- 30 秒 ~ 2 分钟：每 5 秒查询一次
- 2 分钟后：每 10 秒查询一次

### 2. 处理时间参考

- 纯文本生视频：通常 1 ~ 3 分钟
- 图片+文本生视频：通常 1 ~ 3 分钟
- 多模态输入（含视频/音频）：通常 2 ~ 5 分钟
- 时长越长、分辨率越高，处理时间越长

### 3. Prompt 建议

- 提示词 = 主体 + 运动，背景 + 运动，镜头 + 运动
- 用简洁准确的自然语言写出想要的效果
- 可以指定镜头运动（推进、拉远、环绕等）
- 通过 "图片1"、"视频1"、"音频1" 引用 content 中对应位置的参考素材
- 当生成结果不符合预期时，建议修改提示词，将抽象描述换成具象描述
- 如果有明确的效果预期，建议先用生图模型生成符合预期的图片，再用图生视频