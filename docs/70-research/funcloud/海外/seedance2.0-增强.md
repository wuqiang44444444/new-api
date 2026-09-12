# Seedance 2.0 视频生成（480pto720p 增强分辨率）API 对接文档

## 概述

Seedance 2.0 视频生成接口支持一个特殊的增强分辨率取值 **`480pto720p`**：在一次生成任务内先生成视频、再自动将画面增强到 720p，**无需创建第二个任务、无需二次计费**。任务成功后同时返回增强后的 720p 视频与原始视频两条链接，`480pto720p` 按 720p 价格计费。

> 本文档聚焦 `480pto720p` 增强分辨率的用法。Seedance 2.0 的基础生成能力（文生视频、图生视频、多模态、编辑、延长、真人模式等）请参见 [Seedance 2.0 视频生成 API 对接文档](open-api-seedance2%20%28国内%29.md)。

**Base URL**: `https://mm-internal-cn.leonecloud.com`

---

## 认证方式

所有接口均需要在请求头中携带 Token 进行认证：

```
Authorization: Bearer {YOUR_AUTH_TOKEN}
```

---

## 工作原理

将 `resolution` 设为 `480pto720p` 后，**一次创建请求只对应一个任务**（`taskId`），流程如下：

```
创建请求(resolution=480pto720p)
        │
        └── 单个任务 taskId → 生成视频 → 自动增强到 720p → 返回 [720p, 原始视频]
```

- 整个过程在**同一个任务**内完成，只需轮询这一个 `taskId`。
- 任务成功后 `result` 数组返回**两条链接**：第一条为增强后的 720p 视频，第二条为原始视频。
- **只计费一次**：`480pto720p` 按 720p 分辨率价格计费，不额外收取增强费用。
- 若增强环节异常，任务仍会成功返回原始视频，保证可用性。

---

## 创建任务

**POST** `/api/v2/open/aigc/seedance2-0`

在标准 Seedance 2.0 创建请求的基础上，将 `resolution` 设为 `480pto720p` 即可。

### 请求参数

| 参数 | 类型 | 必填 | 说明 |
|-----|------|-----|------|
| content | array | 是 | 内容数组（text/image_url/video_url/audio_url），详见基础文档 |
| ratio | string | 否 | 宽高比：`16:9`(默认) / `9:16` / `1:1` / `4:3` / `3:4` / `21:9` / `adaptive` |
| duration | int | 否 | 视频时长（秒），范围 4~15，默认 5 |
| resolution | string | 否 | 输出分辨率：`480p` / `720p`(默认) / `1080p` / **`480pto720p`（增强到 720p，按 720p 计费）** |
| generateAudio | bool | 否 | 是否生成音频，默认 `false` |
| watermark | bool | 否 | 是否带水印，默认 `false` |
| seed | int | 否 | 随机种子，用于复现结果 |
| cameraFixed | bool | 否 | 是否固定摄像头 |
| returnLastFrame | bool | 否 | 是否返回视频尾帧图片URL |
| realPersonMode | bool | 否 | 真人模式，默认 `false` |
| callbackUrl | string | 否 | 任务完成后的回调通知 URL |

> **`480pto720p` 说明**：
> - 与 `480p` / `720p` / `1080p` 一样是 `resolution` 的一个取值，无需额外字段。
> - 任务成功后 `result` 返回两条链接：`result[0]` 为增强后的 720p 视频，`result[1]` 为原始视频。
> - 按 720p 分辨率价格计费，单任务一次性扣费。

### 请求示例

以「文生视频 + 480pto720p」为例：

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
    "resolution": "480pto720p"
  }'
```

图生视频同样支持，只需把标准图生视频请求的 `resolution` 设为 `480pto720p`：

```bash
curl -X POST "https://mm-internal-cn.leonecloud.com/api/v2/open/aigc/seedance2-0" \
  -H "Authorization: Bearer your_auth_token_here" \
  -H "Content-Type: application/json" \
  -d '{
    "content": [
      {
        "type": "text",
        "text": "女孩抱着狐狸，女孩睁开眼，温柔地看向镜头，镜头缓缓拉出"
      },
      {
        "type": "image_url",
        "image_url": { "url": "https://example.com/fox_girl.png" },
        "role": "reference_image"
      }
    ],
    "ratio": "16:9",
    "duration": 5,
    "resolution": "480pto720p"
  }'
```

### 响应参数

| 参数 | 类型 | 说明 |
|-----|------|------|
| code | int | 状态码，0 表示成功 |
| msg | string | 状态信息 |
| data.taskId | string | 任务 ID，用于查询任务状态 |
| data.status | string | 任务状态，创建时固定为 `processing` |
| data.createdAt | string | 创建时间 |

### 响应示例

```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "taskId": "task_20260729150000_abc12345",
    "status": "processing",
    "createdAt": "2026-07-29 15:00:00"
  }
}
```

---

## 查询任务状态

**GET** `/api/v2/open/aigc/{taskId}`

只需轮询创建时返回的这一个 `taskId`。任务成功后 `result` 返回两条链接：`result[0]` 为增强后的 720p 视频，`result[1]` 为原始视频。

```bash
curl -X GET "https://mm-internal-cn.leonecloud.com/api/v2/open/aigc/task_20260729150000_abc12345" \
  -H "Authorization: Bearer your_auth_token_here"
```

### 响应示例

**处理中**
```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "taskId": "task_20260729150000_abc12345",
    "status": "processing",
    "createdAt": "2026-07-29 15:00:00",
    "updatedAt": "2026-07-29 15:00:05"
  }
}
```

**成功**（`result[0]`=720p 增强视频，`result[1]`=原始视频）
```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "taskId": "task_20260729150000_abc12345",
    "status": "success",
    "result": [
      "https://fc-gw-sh.oss-accelerate.aliyuncs.com/videos/2026/07/29/output_720p.mp4",
      "https://fc-gw-sh.oss-accelerate.aliyuncs.com/videos/2026/07/29/output.mp4"
    ],
    "createdAt": "2026-07-29 15:00:00",
    "updatedAt": "2026-07-29 15:03:30",
    "output": {
      "id": "cgt-20260729150000-xxxxx",
      "status": "succeeded",
      "content": {
        "video_url": "https://fc-gw-sh.oss-accelerate.aliyuncs.com/videos/2026/07/29/output_720p.mp4"
      },
      "created_at": 1753772400,
      "updated_at": 1753772610
    }
  }
}
```

> **说明**：`output.content.video_url` 与 `result[0]` 一致，均为增强后的 720p 视频。

**失败**
```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "taskId": "task_20260729150000_abc12345",
    "status": "failed",
    "errorMsg": "视频生成失败，请重试",
    "createdAt": "2026-07-29 15:00:00",
    "updatedAt": "2026-07-29 15:02:00"
  }
}
```

---

## 回调通知

若创建任务时提供了 `callbackUrl`，任务完成（成功或失败）时会向该 URL 发送 POST 回调。回调格式与基础文档一致，成功时 `result` 同样返回两条链接（`result[0]`=720p 增强视频，`result[1]`=原始视频）。

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
  "taskId": "task_20260729150000_abc12345",
  "status": "success",
  "result": [
    "https://fc-gw-sh.oss-accelerate.aliyuncs.com/videos/output_720p.mp4",
    "https://fc-gw-sh.oss-accelerate.aliyuncs.com/videos/output.mp4"
  ],
  "errorMsg": "",
  "timestamp": "2026-07-29T15:03:30+08:00",
  "signature": "a1b2c3d4e5f6..."
}
```

---

## 计费说明

- `480pto720p` **按 720p 分辨率价格计费，单任务一次性扣费**，不额外收取增强费用。
- 计费与 `resolution: 720p` 相同，但同时交付增强后的 720p 视频与原始视频两条链接。

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

1. **只轮询一个任务**：`480pto720p` 全程在单个 `taskId` 内完成，无需管理第二个任务。
2. **两条链接各取所需**：`result[0]` 为 720p 增强视频（通常用于交付），`result[1]` 为原始视频（可留档或对比）。
3. **轮询间隔**：
   - 前 30 秒：每 3 秒查询一次
   - 30 秒 ~ 2 分钟：每 5 秒查询一次
   - 2 分钟后：每 10 秒查询一次
4. **计费可预期**：与 720p 同价，单次扣费，便于成本核算。