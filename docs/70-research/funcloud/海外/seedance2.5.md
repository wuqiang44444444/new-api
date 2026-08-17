# Seedance 2.5 视频生成 API 对接文档

## 概述

Seedance 2.5 是新一代旗舰视频生成接口，支持文本、图片、视频、音频等多模态输入生成视频，在画面质量、镜头稳定性和多模态理解上较上一代显著提升。

**特性**：
- 🎬 **旗舰画质**：新一代模型，画面细节、动态表现与镜头稳定性全面升级
- 🧠 **更强多模态理解**：更精准地融合图片、视频、音频等参考素材
- 🎞️ **更长时长**：单次生成支持 4~30 秒
- 🇨🇳 **国内专线**：专为国内用户优化的访问速度

**限制**：
- ⚠️ **分辨率限制**：仅支持 480p 和 720p，不支持 1080p

**Base URL**: `https://mm-internal-cn.leonecloud.com`（国内专线）

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
curl -X POST "https://mm-internal-cn.leonecloud.com/api/v2/open/aigc/seedance2-5" \
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

### 场景2：首帧/尾帧图片生视频

传入首帧或尾帧图片，模型基于图片内容生成视频。

```bash
curl -X POST "https://mm-internal-cn.leonecloud.com/api/v2/open/aigc/seedance2-5" \
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
        "role": "first_frame"
      }
    ],
    "ratio": "16:9",
    "duration": 5,
    "resolution": "720p"
  }'
```

### 场景3：多图参考生视频

传入多张参考图片，在提示词中通过 "图片1"、"图片2" 引用（最多9张）。

```bash
curl -X POST "https://mm-internal-cn.leonecloud.com/api/v2/open/aigc/seedance2-5" \
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
curl -X POST "https://mm-internal-cn.leonecloud.com/api/v2/open/aigc/seedance2-5" \
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

替换视频中的主体、局部画面重绘/修复等。传入参考视频时，输出视频比例与时长将自动跟随输入视频。

```bash
curl -X POST "https://mm-internal-cn.leonecloud.com/api/v2/open/aigc/seedance2-5" \
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
    "resolution": "720p"
  }'
```

---

## 查询任务状态

```bash
curl -X GET "https://mm-internal-cn.leonecloud.com/api/v2/open/aigc/{taskId}" \
  -H "Authorization: Bearer your_auth_token_here"
```

---

## 接口详情

### 1. 创建 Seedance 2.5 任务

**POST** `/api/v2/open/aigc/seedance2-5`

创建一个 Seedance 2.5 视频生成任务。

**Content-Type**: `application/json`

#### 请求参数

| 参数 | 类型 | 必填 | 说明 |
|-----|------|-----|------|
| content | array | 是 | 内容数组，详见下方 [content 内容项](#content-内容项) |
| taskType | string | 否 | 任务类型：`reference_to_video` / `video_edit` / `video_extend` / `keyframes` / `auto`。不传时由系统按输入素材自动推断，详见下方 [任务类型 (taskType)](#任务类型-tasktype) |
| ratio | string | 否 | 宽高比：`16:9`(默认) / `9:16` / `1:1` / `4:3` / `3:4` / `21:9` / `adaptive`。部分任务类型会强制为 `adaptive`，详见任务类型说明 |
| duration | int | 否 | 视频时长（秒），范围 4~30，默认 5；传 `-1` 表示智能时长（由系统自动决定输出时长）。部分任务类型会强制为 `-1`，详见任务类型说明 |
| resolution | string | 否 | 输出分辨率：`480p` / `720p`(默认)。**不支持 1080p** |
| generateAudio | bool | 否 | 是否生成音频，默认 `false` |
| watermark | bool | 否 | 是否带水印，默认 `false` |
| seed | int | 否 | 随机种子，用于复现结果 |
| cameraFixed | bool | 否 | 是否固定摄像头 |
| returnLastFrame | bool | 否 | 是否返回视频尾帧图片URL（已弃用） |
| realPersonMode | bool | 否 | 真人模式开关 |
| callbackUrl | string | 否 | 任务完成后的回调通知 URL |

#### content 内容项

content 为数组，每个元素为一个内容项：

| 类型 | 字段 | 说明 |
|-----|------|------|
| text | `type`: "text", `text`: "提示词" | **必须**，视频描述提示词（3-20000字符） |
| image_url | `type`: "image_url", `image_url`: {"url": "URL"}, `role`: 见下方 | 参考图片（最多9张，支持jpeg/png/webp/bmp/tiff/gif，宽高比0.4-2.5，尺寸300-6000px，最大30MB） |
| video_url | `type`: "video_url", `video_url`: {"url": "URL"}, `role`: "reference_video" | 参考视频（最多3个，mp4/mov，480p/720p，2-30秒，最大50MB，24-60FPS） |
| audio_url | `type`: "audio_url", `audio_url`: {"url": "URL"}, `role`: "reference_audio" | 参考音频（最多3个，wav/mp3，2-30秒，最大15MB） |

#### image_url 的 role 取值

| role 值 | 说明 |
|---------|------|
| `reference_image` | 参考图片 |
| `first_frame` | 首帧图片 |
| `last_frame` | 尾帧图片 |

> **说明**：
> - content 数组中**必须包含至少一个 text 类型**的内容项作为提示词
> - 在提示词中通过 "图片1"、"图片2"、"视频1"、"音频1" 引用对应位置的参考素材
> - 参考图片最多9张，参考视频最多3个，参考音频最多3个
> - 输入包含视频时（视频编辑场景），输出视频的比例与时长将自动跟随输入视频（等效于 `ratio=adaptive`、`duration=-1`），此时指定的 `ratio` / `duration` 不生效
> - 输入包含视频时，计费按实际输出计量（token 计费），与纯文本/图片输入不同
> - **仅支持 480p 和 720p 分辨率，不支持 1080p**

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
    "taskId": "task_20260508150000_abc12345",
    "status": "processing",
    "createdAt": "2026-05-08 15:00:00"
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

#### 响应示例

**处理中**
```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "taskId": "task_20260508150000_abc12345",
    "status": "processing",
    "createdAt": "2026-05-08 15:00:00",
    "updatedAt": "2026-05-08 15:00:05"
  }
}
```

**成功**
```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "taskId": "task_20260508150000_abc12345",
    "status": "success",
    "result": [
      "https://fc-gw-sh.oss-cn-shanghai.aliyuncs.com/videos/2026/05/08/output.mp4"
    ],
    "createdAt": "2026-05-08 15:00:00",
    "updatedAt": "2026-05-08 15:03:30"
  }
}
```

**失败**
```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "taskId": "task_20260508150000_abc12345",
    "status": "failed",
    "errorMsg": "视频生成失败，请重试",
    "createdAt": "2026-05-08 15:00:00",
    "updatedAt": "2026-05-08 15:02:00"
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
  "taskIds": ["task_20260508150000_abc12345", "task_20260508150200_def67890"]
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
  "taskId": "task_20260508150000_abc12345",
  "status": "success",
  "result": ["https://fc-gw-sh.oss-cn-shanghai.aliyuncs.com/videos/output.mp4"],
  "errorMsg": "",
  "timestamp": "2026-05-08T15:03:30+08:00",
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

> **注意**：Seedance 2.5 不支持 1080p

### 宽高比 (ratio)

| 值 | 说明 |
|----|------|
| `16:9` | 横屏（默认） |
| `9:16` | 竖屏 |
| `1:1` | 方形 |
| `4:3` | 标准横屏 |
| `3:4` | 标准竖屏 |
| `21:9` | 超宽屏 |
| `adaptive` | 自适应（根据输入素材比例） |

### 时长 (duration)

- 范围：4 ~ 30 秒
- 默认：5 秒
- `-1`：智能时长，由系统自动决定输出时长
- 视频编辑场景（输入含参考视频）时长自动跟随输入视频（等效于 `-1`），此时按实际输出计量计费

### 任务类型 (taskType)

`taskType` 用于声明本次生成的任务类型。不同任务类型对 `ratio` / `duration` 有不同的约束规则，且对所需素材有不同要求。**不传 `taskType` 时，系统按输入素材自动推断**：带参考视频（video_url）→ `auto`；否则含首帧/尾帧图片 → `keyframes`；否则 → `reference_to_video`。

| taskType | 说明 | 素材要求 | ratio | duration |
|----------|------|----------|-------|----------|
| `reference_to_video` | 文本 / 参考图片生成视频 | 无强制 | 可自定义 | 可自定义（4~30 或 -1） |
| `video_edit` | 视频编辑：基于参考视频生成 | 必须含参考视频 | 强制 `adaptive`（跟随输入） | 强制 `-1`（跟随输入视频） |
| `video_extend` | 视频延长 | 必须含参考视频 | 强制 `adaptive`（跟随输入） | 可自定义（4~30 或 -1） |
| `keyframes` | 首/尾帧生成视频 | 必须含首帧或尾帧图片 | 强制 `adaptive`（跟随输入） | 可自定义（4~30 或 -1） |
| `auto` | 自动：带参考视频时等效视频编辑 | 无强制 | 带参考视频时强制 `adaptive` | 带参考视频时强制 `-1` |

> **说明**：
> - 当任务类型强制 `ratio=adaptive` 时，输出比例自动跟随输入素材，此时传入的 `ratio` 不生效。
> - 当任务类型强制 `duration=-1` 时，输出时长由系统根据输入视频自动决定，此时传入的 `duration` 不生效，且该场景仅支持按 token 计费。
> - `video_edit` / `video_extend` 缺少参考视频，或 `keyframes` 缺少首/尾帧图片时，请求会被拒绝（参数错误）。

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
- 多模态输入（含视频/音频）：通常 2 ~ 4 分钟
- 时长越长，处理时间越长

### 3. Prompt 建议

- 提示词长度：3 ~ 20,000 字符
- 提示词 = 主体 + 运动，背景 + 运动，镜头 + 运动
- 用简洁准确的自然语言写出想要的效果
- 可以指定镜头运动（推进、拉远、环绕等）
- 通过 "图片1"、"图片2"、"视频1"、"音频1" 引用 content 中对应位置的参考素材
- 当生成结果不符合预期时，建议修改提示词，将抽象描述换成具象描述
- 如果有明确的效果预期，建议先用生图模型生成符合预期的图片，再用图生视频

---

## 常见问题

### Q1: Seedance 2.5 相比上一代有什么提升？

**A**: Seedance 2.5 为新一代旗舰模型，在画面细节、动态表现、镜头稳定性以及多模态素材理解上均有明显提升，更适合追求高质量画面的场景。

### Q2: 为什么不支持 1080p？

**A**: 当前 Seedance 2.5 仅支持 480p 和 720p 输出。传入 1080p 会返回参数错误，请使用 480p 或 720p。

### Q3: 单次最长能生成多长的视频？

**A**: 4 ~ 30 秒。视频编辑场景（输入含参考视频）下，输出时长自动跟随输入视频。

### Q4: content 中可以不传 text 吗？

**A**: 不可以。content 数组中必须包含至少一个 `text` 类型的内容项作为提示词，否则请求会被拒绝。

---

## 技术支持

如有疑问，请联系我们的技术支持团队。