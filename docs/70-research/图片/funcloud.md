# Funcloud 图片生成 API

Base URL: `https://mm-internal-cn.leonecloud.com`

所有接口均需要在请求头中携带 Token 进行认证：

```
Authorization: Bearer {YOUR_AUTH_TOKEN}
```

---

## Nano Banana 2 Lite 图片生成

### 概述

Nano Banana 2 Lite 图片生成接口，是 Nano Banana 2 的轻量快速版，支持文生图（text-to-image）和图生图编辑（image-to-image），单一分辨率输出，生成速度更快、成本更低，最多可提供 10 张参考图片进行图生图编辑，支持丰富的宽高比选项。

### 快速开始

创建文生图任务：

```bash
curl -X POST "https://mm-internal-cn.leonecloud.com/api/v2/open/aigc/nano-banana-2-lite" \
  -H "Authorization: Bearer your_auth_token_here" \
  -H "Content-Type: application/json" \
  -d '{
    "prompt": "A surreal painting of a giant banana floating in space",
    "aspectRatio": "16:9"
  }'
```

创建图生图编辑任务：

```bash
curl -X POST "https://mm-internal-cn.leonecloud.com/api/v2/open/aigc/nano-banana-2-lite" \
  -H "Authorization: Bearer your_auth_token_here" \
  -H "Content-Type: application/json" \
  -d '{
    "prompt": "将图片转换为水彩画风格",
    "imageUrls": ["https://example.com/input.jpg"],
    "aspectRatio": "1:1"
  }'
```

查询任务状态：

```bash
curl -X GET "https://mm-internal-cn.leonecloud.com/api/v2/open/aigc/task_abc123" \
  -H "Authorization: Bearer your_auth_token_here"
```

### 接口列表

#### 1. 创建 Nano Banana 2 Lite 图片生成任务

```
POST /api/v2/open/aigc/nano-banana-2-lite
```

创建一个 Nano Banana 2 Lite 图片生成任务。

`Content-Type: application/json`

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| prompt | string | 是 | 图片描述提示词，最多 20000 字符 |
| imageUrls | string[] | 否 | 参考图片 URL（最多 10 张，JPEG/PNG/WebP，每张最大 30MB） |
| aspectRatio | string | 否 | 宽高比：auto(默认) / 1:1 / 1:4 / 16:9 / 1:8 / 21:9 / 2:3 / 3:2 / 3:4 / 4:1 / 4:3 / 4:5 / 5:4 / 8:1 / 9:16 |
| taskNickname | string | 否 | 任务昵称，便于识别管理 |
| callbackUrl | string | 否 | 任务完成后的回调通知 URL |

说明：

- Lite 版为单一分辨率输出，不支持 resolution / outputFormat 参数
- 提供 imageUrls 时自动进入图生图编辑模式，模型将基于参考图片进行生成
- 支持最多 10 张参考图片，每张最大 30MB，支持 JPEG/PNG/WebP 格式

**请求示例**

基础文生图：

```json
{
  "prompt": "A surreal painting of a giant banana floating in space"
}
```

指定宽高比：

```json
{
  "prompt": "一只可爱的猫咪坐在窗台上，阳光洒落，超写实风格",
  "aspectRatio": "16:9"
}
```

图生图编辑（单张参考图）：

```json
{
  "prompt": "将图片转换为水彩画风格，保持构图不变",
  "imageUrls": ["https://example.com/input.jpg"],
  "aspectRatio": "1:1"
}
```

图生图编辑（多张参考图）：

```json
{
  "prompt": "融合这些图片的风格，生成一张新的艺术作品",
  "imageUrls": [
    "https://example.com/ref1.jpg",
    "https://example.com/ref2.jpg",
    "https://example.com/ref3.jpg"
  ],
  "aspectRatio": "3:2"
}
```

带回调地址：

```json
{
  "prompt": "星空下的古老城堡，油画风格",
  "aspectRatio": "21:9",
  "callbackUrl": "https://your-server.com/callback"
}
```

**响应参数**

| 参数 | 类型 | 说明 |
| --- | --- | --- |
| code | int | 状态码，0 表示成功 |
| msg | string | 状态信息 |
| data.taskId | string | 任务 ID，用于查询任务状态 |
| data.status | string | 任务状态，创建时固定为 processing |
| data.createdAt | string | 创建时间 |

**响应示例**

成功：

```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "taskId": "task_20260509150000_abc12345",
    "status": "processing",
    "createdAt": "2026-05-09 15:00:00"
  }
}
```

#### 2. 查询任务状态

```
GET /api/v2/open/aigc/{taskId}
```

查询单个任务的执行状态。

**响应示例**

成功：

```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "taskId": "task_20260509150000_abc12345",
    "status": "success",
    "result": [
      "https://fc-gw-sh.oss-accelerate.aliyuncs.com/images/2026/05/09/output_001.png"
    ],
    "createdAt": "2026-05-09 15:00:00",
    "updatedAt": "2026-05-09 15:00:25"
  }
}
```

#### 3. 批量查询任务状态

```
POST /api/v2/open/aigc/batch
```

批量查询多个任务的执行状态（最多 100 个）。

### 回调通知

当任务完成（成功或失败）时，如果创建任务时提供了 callbackUrl，系统会向该 URL 发送 POST 请求。

**回调请求 Headers**

```
Content-Type: application/json
X-Funcloud-Event: task.completed
X-Funcloud-Signature: {签名}
```

**回调请求 Body**

```json
{
  "event": "task.completed",
  "taskId": "task_20260509150000_abc12345",
  "status": "success",
  "result": ["https://fc-gw-sh.oss-accelerate.aliyuncs.com/images/output_001.png"],
  "errorMsg": "",
  "timestamp": "2026-05-09T15:00:25+08:00",
  "signature": "a1b2c3d4e5f6..."
}
```

### 错误码

| code | 说明 |
| --- | --- |
| 0 | 成功 |
| 10002 | 参数缺失或格式错误 |
| 10005 | API Key 无效或缺失 |
| 30003 | 任务不存在 |
| 90003 | 服务器内部错误 |

### 最佳实践

**1. 轮询策略**

建议的轮询间隔：

- 前 30 秒：每 3 秒查询一次
- 30 秒后：每 5 秒查询一次

**2. 使用回调**

生产环境建议使用回调通知而非轮询。

**3. 处理时间参考**

- 文生图：通常 5 ~ 15 秒
- 图生图编辑：通常 5 ~ 20 秒

**4. 版本选择**

| 版本 | 适用场景 |
| --- | --- |
| Nano Banana 2 Lite | 快速预览、批量生成、社交媒体等对速度和成本敏感的场景 |
| Nano Banana 2 | 需要多分辨率（1K/2K/4K）输出、更高画质的场景 |

---

## Nano Banana 2 图片生成

### 概述

Nano Banana 2 图片生成接口，支持文生图（text-to-image）和图生图编辑（image-to-image），支持多分辨率输出（1K/2K/4K），最多可提供 14 张参考图片进行图生图编辑，支持更丰富的宽高比选项。

### 快速开始

创建文生图任务：

```bash
curl -X POST "https://mm-internal-cn.leonecloud.com/api/v2/open/aigc/nano-banana-2" \
  -H "Authorization: Bearer your_auth_token_here" \
  -H "Content-Type: application/json" \
  -d '{
    "prompt": "A surreal painting of a giant banana floating in space",
    "aspectRatio": "16:9",
    "resolution": "2K",
    "outputFormat": "png"
  }'
```

创建图生图编辑任务：

```bash
curl -X POST "https://mm-internal-cn.leonecloud.com/api/v2/open/aigc/nano-banana-2" \
  -H "Authorization: Bearer your_auth_token_here" \
  -H "Content-Type: application/json" \
  -d '{
    "prompt": "将图片转换为水彩画风格",
    "imageUrls": ["https://example.com/input.jpg"],
    "aspectRatio": "1:1",
    "resolution": "2K"
  }'
```

查询任务状态：

```bash
curl -X GET "https://mm-internal-cn.leonecloud.com/api/v2/open/aigc/task_abc123" \
  -H "Authorization: Bearer your_auth_token_here"
```

### 接口列表

#### 1. 创建 Nano Banana 2 图片生成任务

```
POST /api/v2/open/aigc/nano-banana-2
```

创建一个 Nano Banana 2 图片生成任务。

`Content-Type: application/json`

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| prompt | string | 是 | 图片描述提示词，最多 20000 字符 |
| imageUrls | string[] | 否 | 参考图片 URL（最多 14 张，JPEG/PNG/WebP，每张最大 30MB） |
| aspectRatio | string | 否 | 宽高比：auto(默认) / 1:1 / 1:4 / 16:9 / 1:8 / 21:9 / 2:3 / 3:2 / 3:4 / 4:1 / 4:3 / 4:5 / 5:4 / 8:1 / 9:16 |
| resolution | string | 否 | 输出分辨率：1K(默认) / 2K / 4K |
| outputFormat | string | 否 | 输出格式：jpg(默认) / png |
| callbackUrl | string | 否 | 任务完成后的回调通知 URL |

说明：

- 提供 imageUrls 时自动进入图生图编辑模式，模型将基于参考图片进行生成
- 支持最多 14 张参考图片，每张最大 30MB，支持 JPEG/PNG/WebP 格式

**请求示例**

基础文生图：

```json
{
  "prompt": "A surreal painting of a giant banana floating in space"
}
```

指定分辨率和尺寸：

```json
{
  "prompt": "一只可爱的猫咪坐在窗台上，阳光洒落，超写实风格",
  "aspectRatio": "16:9",
  "resolution": "4K",
  "outputFormat": "png"
}
```

图生图编辑（单张参考图）：

```json
{
  "prompt": "将图片转换为水彩画风格，保持构图不变",
  "imageUrls": ["https://example.com/input.jpg"],
  "aspectRatio": "1:1",
  "resolution": "2K"
}
```

图生图编辑（多张参考图）：

```json
{
  "prompt": "融合这些图片的风格，生成一张新的艺术作品",
  "imageUrls": [
    "https://example.com/ref1.jpg",
    "https://example.com/ref2.jpg",
    "https://example.com/ref3.jpg"
  ],
  "aspectRatio": "3:2",
  "resolution": "2K"
}
```

带回调地址：

```json
{
  "prompt": "星空下的古老城堡，油画风格",
  "aspectRatio": "21:9",
  "resolution": "4K",
  "callbackUrl": "https://your-server.com/callback"
}
```

**响应参数**

| 参数 | 类型 | 说明 |
| --- | --- | --- |
| code | int | 状态码，0 表示成功 |
| msg | string | 状态信息 |
| data.taskId | string | 任务 ID，用于查询任务状态 |
| data.status | string | 任务状态，创建时固定为 processing |
| data.createdAt | string | 创建时间 |

**响应示例**

成功：

```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "taskId": "task_20260509150000_abc12345",
    "status": "processing",
    "createdAt": "2026-05-09 15:00:00"
  }
}
```

#### 2. 查询任务状态

```
GET /api/v2/open/aigc/{taskId}
```

查询单个任务的执行状态。

**响应示例**

成功：

```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "taskId": "task_20260509150000_abc12345",
    "status": "success",
    "result": [
      "https://fc-gw-sh.oss-accelerate.aliyuncs.com/images/2026/05/09/output_001.png"
    ],
    "createdAt": "2026-05-09 15:00:00",
    "updatedAt": "2026-05-09 15:00:25"
  }
}
```

#### 3. 批量查询任务状态

```
POST /api/v2/open/aigc/batch
```

批量查询多个任务的执行状态（最多 100 个）。

### 回调通知

当任务完成（成功或失败）时，如果创建任务时提供了 callbackUrl，系统会向该 URL 发送 POST 请求。

**回调请求 Headers**

```
Content-Type: application/json
X-Funcloud-Event: task.completed
X-Funcloud-Signature: {签名}
```

**回调请求 Body**

```json
{
  "event": "task.completed",
  "taskId": "task_20260509150000_abc12345",
  "status": "success",
  "result": ["https://fc-gw-sh.oss-accelerate.aliyuncs.com/images/output_001.png"],
  "errorMsg": "",
  "timestamp": "2026-05-09T15:00:25+08:00",
  "signature": "a1b2c3d4e5f6..."
}
```

### 错误码

| code | 说明 |
| --- | --- |
| 0 | 成功 |
| 10002 | 参数缺失或格式错误 |
| 10005 | API Key 无效或缺失 |
| 30003 | 任务不存在 |
| 90003 | 服务器内部错误 |

### 最佳实践

**1. 轮询策略**

建议的轮询间隔：

- 前 30 秒：每 3 秒查询一次
- 30 秒后：每 5 秒查询一次

**2. 使用回调**

生产环境建议使用回调通知而非轮询。

**3. 处理时间参考**

- 文生图（1K）：通常 5 ~ 20 秒
- 文生图（2K/4K）：通常 10 ~ 30 秒
- 图生图编辑：通常 10 ~ 30 秒

**4. 分辨率选择**

| 分辨率 | 适用场景 |
| --- | --- |
| 1K | 快速预览、社交媒体 |
| 2K | 高质量展示、网页素材 |
| 4K | 印刷品、大尺寸海报 |

---

## Seedream 5.0 Lite 图片生成

### 概述

Seedream 5.0 Lite 图片生成接口，支持文生图（text-to-image）和图生图（image-to-image）两种模式。

### 快速开始

创建文生图任务：

```bash
curl -X POST "https://mm-internal-cn.leonecloud.com/api/v2/open/aigc/seedream-5.0-lite" \
  -H "Authorization: Bearer your_auth_token_here" \
  -H "Content-Type: application/json" \
  -d '{
    "prompt": "一只可爱的猫咪坐在窗台上，阳光洒落",
    "genType": "t2i",
    "aspectRatio": "16:9",
    "quality": "high"
  }'
```

创建图生图任务：

```bash
curl -X POST "https://mm-internal-cn.leonecloud.com/api/v2/open/aigc/seedream-5.0-lite" \
  -H "Authorization: Bearer your_auth_token_here" \
  -H "Content-Type: application/json" \
  -d '{
    "prompt": "将图片转换为水彩画风格",
    "genType": "i2i",
    "imageUrls": ["https://example.com/reference.jpg"],
    "aspectRatio": "1:1",
    "quality": "basic"
  }'
```

查询任务状态：

```bash
curl -X GET "https://mm-internal-cn.leonecloud.com/api/v2/open/aigc/task_abc123" \
  -H "Authorization: Bearer your_auth_token_here"
```

### 接口列表

#### 1. 创建 Seedream 5.0 Lite 图片生成任务

```
POST /api/v2/open/aigc/seedream-5.0-lite
```

创建一个 Seedream 5.0 Lite 图片生成任务。

`Content-Type: application/json`

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| prompt | string | 是 | 图片描述提示词，3-3000 字符 |
| genType | string | 否 | 生成类型：t2i(文生图,默认) / i2i(图生图) |
| imageUrls | string[] | 条件 | 参考图片 URL（i2i 时必填，最多 14 张，JPEG/PNG/WebP，每张最大 10MB） |
| aspectRatio | string | 否 | 宽高比：1:1(默认) / 4:3 / 3:4 / 16:9 / 9:16 / 2:3 / 3:2 / 21:9 |
| quality | string | 否 | 输出质量：basic(2K,默认) / high(3K) |
| nsfwChecker | boolean | 否 | 内容过滤开关，设为 false 时禁用内容过滤，默认 false |
| callbackUrl | string | 否 | 任务完成后的回调通知 URL |

说明：

- 图生图(i2i)时必须提供 imageUrls
- 创建任务时会预扣费，余额不足将返回错误

**请求示例**

文生图（2K 质量）：

```json
{
  "prompt": "一只可爱的猫咪坐在窗台上，阳光洒落",
  "aspectRatio": "16:9",
  "quality": "basic"
}
```

文生图（3K 质量）：

```json
{
  "prompt": "星空下的古老城堡，超写实风格",
  "genType": "t2i",
  "aspectRatio": "21:9",
  "quality": "high"
}
```

图生图：

```json
{
  "prompt": "将图片转换为水彩画风格，保持构图不变",
  "genType": "i2i",
  "imageUrls": ["https://example.com/input.jpg"],
  "aspectRatio": "1:1",
  "quality": "basic"
}
```

**响应参数**

| 参数 | 类型 | 说明 |
| --- | --- | --- |
| code | int | 状态码，0 表示成功 |
| msg | string | 状态信息 |
| data.taskId | string | 任务 ID，用于查询任务状态 |
| data.status | string | 任务状态，创建时固定为 processing |
| data.createdAt | string | 创建时间 |

**响应示例**

成功：

```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "taskId": "task_20260506150000_abc12345",
    "status": "processing",
    "createdAt": "2026-05-06 15:00:00"
  }
}
```

余额不足：

```json
{
  "code": 40001,
  "msg": "余额不足: 当前余额 0.0100, 需要 0.0413",
  "data": null
}
```

#### 2. 查询任务状态

```
GET /api/v2/open/aigc/{taskId}
```

查询单个任务的执行状态。

**响应示例**

成功：

```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "taskId": "task_20260506150000_abc12345",
    "status": "success",
    "result": [
      "https://fc-gw-sh.oss-accelerate.aliyuncs.com/images/2026/05/06/output_001.png"
    ],
    "createdAt": "2026-05-06 15:00:00",
    "updatedAt": "2026-05-06 15:00:30"
  }
}
```

#### 3. 批量查询任务状态

```
POST /api/v2/open/aigc/batch
```

批量查询多个任务的执行状态（最多 100 个）。

#### 4. 查询账户余额

```
GET /api/v2/open/balance
```

### 回调通知

当任务完成（成功或失败）时，如果创建任务时提供了 callbackUrl，系统会向该 URL 发送 POST 请求。

**回调请求 Headers**

```
Content-Type: application/json
X-Funcloud-Event: task.completed
X-Funcloud-Signature: {签名}
```

**回调请求 Body**

```json
{
  "event": "task.completed",
  "taskId": "task_20260506150000_abc12345",
  "status": "success",
  "result": ["https://fc-gw-sh.oss-accelerate.aliyuncs.com/images/output_001.png"],
  "errorMsg": "",
  "timestamp": "2026-05-06T15:00:30+08:00",
  "signature": "a1b2c3d4e5f6..."
}
```

### 错误码

| code | 说明 |
| --- | --- |
| 0 | 成功 |
| 10002 | 参数缺失或格式错误 |
| 10005 | API Key 无效或缺失 |
| 30003 | 任务不存在 |
| 40001 | 余额不足 |
| 90003 | 服务器内部错误 |

### 最佳实践

**1. 轮询策略**

建议的轮询间隔：

- 前 30 秒：每 3 秒查询一次
- 30 秒后：每 5 秒查询一次

**2. 使用回调**

生产环境建议使用回调通知而非轮询。

**3. 处理时间参考**

- 文生图(basic/2K)：通常 5 ~ 15 秒
- 文生图(high/3K)：通常 10 ~ 30 秒
- 图生图(basic/2K)：通常 5 ~ 15 秒
- 图生图(high/3K)：通常 10 ~ 30 秒

**4. 余额管理**

- 创建任务前建议先查询余额
- 任务成功后会从冻结余额中扣费
- 任务失败后冻结金额会自动退还到可用余额

---

## Seedream 5.0 Pro 图片生成

### 概述

Seedream 5.0 Pro 图片生成接口，支持文生图（text-to-image）和图生图（image-to-image）两种模式。

### 快速开始

创建文生图任务：

```bash
curl -X POST "https://mm-internal-cn.leonecloud.com/api/v2/open/aigc/seedream-5.0-pro" \
  -H "Authorization: Bearer your_auth_token_here" \
  -H "Content-Type: application/json" \
  -d '{
    "prompt": "一只可爱的猫咪坐在窗台上，阳光洒落",
    "genType": "t2i",
    "aspectRatio": "16:9",
    "quality": "high"
  }'
```

创建图生图任务：

```bash
curl -X POST "https://mm-internal-cn.leonecloud.com/api/v2/open/aigc/seedream-5.0-pro" \
  -H "Authorization: Bearer your_auth_token_here" \
  -H "Content-Type: application/json" \
  -d '{
    "prompt": "将图片转换为水彩画风格",
    "genType": "i2i",
    "imageUrls": ["https://example.com/reference.jpg"],
    "aspectRatio": "1:1",
    "quality": "basic"
  }'
```

查询任务状态：

```bash
curl -X GET "https://mm-internal-cn.leonecloud.com/api/v2/open/aigc/task_abc123" \
  -H "Authorization: Bearer your_auth_token_here"
```

### 接口列表

#### 1. 创建 Seedream 5.0 Pro 图片生成任务

```
POST /api/v2/open/aigc/seedream-5.0-pro
```

创建一个 Seedream 5.0 Pro 图片生成任务。

`Content-Type: application/json`

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| prompt | string | 是 | 图片描述提示词，3-3000 字符 |
| genType | string | 否 | 生成类型：t2i(文生图,默认) / i2i(图生图) |
| imageUrls | string[] | 条件 | 参考图片 URL（i2i 时必填，最多 10 张，JPEG/PNG/WebP，每张最大 10MB） |
| aspectRatio | string | 否 | 宽高比：1:1(默认) / 4:3 / 3:4 / 16:9 / 9:16 / 2:3 / 3:2 |
| quality | string | 否 | 输出质量：basic(1K,默认) / high(2K) |
| nsfwChecker | boolean | 否 | 内容过滤开关，设为 false 时禁用内容过滤，默认 false |
| callbackUrl | string | 否 | 任务完成后的回调通知 URL |

说明：

- 图生图(i2i)时必须提供 imageUrls
- 创建任务时会预扣费，余额不足将返回错误

**请求示例**

文生图（1K 质量）：

```json
{
  "prompt": "一只可爱的猫咪坐在窗台上，阳光洒落",
  "aspectRatio": "16:9",
  "quality": "basic"
}
```

文生图（2K 质量）：

```json
{
  "prompt": "星空下的古老城堡，超写实风格",
  "genType": "t2i",
  "aspectRatio": "16:9",
  "quality": "high"
}
```

图生图：

```json
{
  "prompt": "将图片转换为水彩画风格，保持构图不变",
  "genType": "i2i",
  "imageUrls": ["https://example.com/input.jpg"],
  "aspectRatio": "1:1",
  "quality": "basic"
}
```

**响应参数**

| 参数 | 类型 | 说明 |
| --- | --- | --- |
| code | int | 状态码，0 表示成功 |
| msg | string | 状态信息 |
| data.taskId | string | 任务 ID，用于查询任务状态 |
| data.status | string | 任务状态，创建时固定为 processing |
| data.createdAt | string | 创建时间 |

**响应示例**

成功：

```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "taskId": "task_20260506150000_abc12345",
    "status": "processing",
    "createdAt": "2026-05-06 15:00:00"
  }
}
```

余额不足：

```json
{
  "code": 40001,
  "msg": "余额不足: 当前余额不足以支付本次任务",
  "data": null
}
```

#### 2. 查询任务状态

```
GET /api/v2/open/aigc/{taskId}
```

查询单个任务的执行状态。

**响应示例**

成功：

```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "taskId": "task_20260506150000_abc12345",
    "status": "success",
    "result": [
      "https://fc-gw-sh.oss-accelerate.aliyuncs.com/images/2026/05/06/output_001.png"
    ],
    "createdAt": "2026-05-06 15:00:00",
    "updatedAt": "2026-05-06 15:00:30"
  }
}
```

#### 3. 批量查询任务状态

```
POST /api/v2/open/aigc/batch
```

批量查询多个任务的执行状态（最多 100 个）。

#### 4. 查询账户余额

```
GET /api/v2/open/balance
```

### 回调通知

当任务完成（成功或失败）时，如果创建任务时提供了 callbackUrl，系统会向该 URL 发送 POST 请求。

**回调请求 Headers**

```
Content-Type: application/json
X-Funcloud-Event: task.completed
X-Funcloud-Signature: {签名}
```

**回调请求 Body**

```json
{
  "event": "task.completed",
  "taskId": "task_20260506150000_abc12345",
  "status": "success",
  "result": ["https://fc-gw-sh.oss-accelerate.aliyuncs.com/images/output_001.png"],
  "errorMsg": "",
  "timestamp": "2026-05-06T15:00:30+08:00",
  "signature": "a1b2c3d4e5f6..."
}
```

### 错误码

| code | 说明 |
| --- | --- |
| 0 | 成功 |
| 10002 | 参数缺失或格式错误 |
| 10005 | API Key 无效或缺失 |
| 30003 | 任务不存在 |
| 40001 | 余额不足 |
| 90003 | 服务器内部错误 |

### 最佳实践

**1. 轮询策略**

建议的轮询间隔：

- 前 30 秒：每 3 秒查询一次
- 30 秒后：每 5 秒查询一次

**2. 使用回调**

生产环境建议使用回调通知而非轮询。

**3. 处理时间参考**

- 文生图(basic/1K)：通常 5 ~ 15 秒
- 文生图(high/2K)：通常 10 ~ 30 秒
- 图生图(basic/1K)：通常 5 ~ 15 秒
- 图生图(high/2K)：通常 10 ~ 30 秒

**4. 余额管理**

- 创建任务前建议先查询余额
- 任务成功后会从冻结余额中扣费
- 任务失败后冻结金额会自动退还到可用余额
