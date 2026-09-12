# Seedance 视频生成 API（V3 协议）对接文档

## 概述

V3 协议是 Seedance 全系模型共用的统一接口：只有一个生成端点，用 `model` 字段指定具体模型。Seedance 2.5 与 Seedance 2.0 / 2.0 Fast / 2.0 Mini 全部支持 V3，请求体、查询响应、错误结构完全一致，换模型只改 `model` 一个字段。请求与响应字段均为 snake_case。

**特性：**

- 🎛️ 一个端点全系模型：`model` 必填，Seedance 全系可选，取值见 [model 取值](#model-取值)
- 🎚️ 档位自由切换：旗舰画质到轻量快速档共用同一套请求体，无需按模型改代码
- 🧠 多模态输入：支持文本、图片、视频、音频等参考素材
- 🔊 默认生成音频：`generate_audio` 未传时默认 `true`
- ⏱️ 帧数控制：支持用 `frames` 直接指定总帧数（与 `duration` 二选一）

**能力差异（只体现在取值范围上，接口形状不变）：**

- ⚠️ 分辨率：480p / 720p 全系支持；1080p 仅 seedance-2-5 系列支持；不支持 4K
- ⚠️ 时长：seedance-2-5 系列 4~30 秒并支持智能时长（-1）；seedance-2-0 系列 4~15 秒，不支持智能时长

**Base URL:** `https://mm-internal-cn.leonecloud.com`

## 认证方式

所有接口均需要在请求头中携带 Token 进行认证：

```http
Authorization: Bearer {YOUR_AUTH_TOKEN}
```

认证失败时返回 HTTP 401：

```json
{
  "error": {
    "code": "AuthenticationError",
    "message": "API Key 无效",
    "type": "Unauthorized"
  }
}
```

## model 取值

`model` 为必填字段。下表列出 V3 协议支持的全部 Seedance 模型，任选其一即可，其余参数用法完全相同：

| model | 说明 | 支持的分辨率 | duration 范围 | duration 默认值 |
| --- | --- | --- | --- | --- |
| seedance-2-5 | 旗舰画质，支持智能时长 | 480p / 720p / 1080p | 4~30 或 -1 | -1（智能时长） |
| seedance-2-0 | 标准画质 | 480p / 720p | 4~15 | 5 |
| seedance-2-0-fast | 快速版，出片更快 | 480p / 720p | 4~15 | 5 |
| seedance-2-0-mini | 轻量版 | 480p / 720p | 4~15 | 5 |

不同 model 的计费单价不同，详见控制台的价格页。

传入不在上表中的 model 会返回 HTTP 400 InvalidParameter。

## 默认值

| 字段 | 默认值 | 说明 |
| --- | --- | --- |
| ratio | adaptive | 有参考图片 / 视频时跟随输入素材比例；纯文本输入时由模型决定 |
| duration | seedance-2-5 系列为 -1；其余为 5 | 见下方说明 |
| resolution | 720p | |
| generate_audio | true | |
| output_format | mp4 | |
| watermark | false | |

### duration 的两种模式

- **seedance-2-5**：默认 -1，即智能时长，输出时长由模型根据内容决定。此时按 token 计费，费用与实际生成的时长、分辨率相关，创建时先冻结额度，任务结束后按实际用量结算多退少补。需要可预期的固定费用请显式传具体秒数（4 ~ 30）。
- **seedance-2-0 系列（含 fast / mini）**：默认 5 秒，按秒计费，范围 4 ~ 15 秒。传 `duration = -1` 会返回 HTTP 400，请指定具体秒数。

## 支持的使用场景

### 场景 1：文生视频

```bash
curl -X POST "https://mm-internal-cn.leonecloud.com/api/v3/contents/generations/tasks" \
  -H "Authorization: Bearer your_auth_token_here" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "seedance-2-5",
    "content": [
      {
        "type": "text",
        "text": "写实风格，晴朗的蓝天之下，一大片白色的雏菊花田，镜头逐渐拉近，最终定格在一朵雏菊花的特写上，花瓣上有几颗晶莹的露珠"
      }
    ],
    "ratio": "16:9",
    "duration": 5,
    "resolution": "720p",
    "generate_audio": true
  }'
```

### 场景 2：图生视频（首帧）

```bash
curl -X POST "https://mm-internal-cn.leonecloud.com/api/v3/contents/generations/tasks" \
  -H "Authorization: Bearer your_auth_token_here" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "seedance-2-5",
    "content": [
      {
        "type": "text",
        "text": "镜头缓慢推近，人物微笑，头发随微风轻轻飘动，电影感光影"
      },
      {
        "type": "image_url",
        "image_url": { "url": "https://example.com/portrait.jpg" },
        "role": "first_frame"
      }
    ],
    "ratio": "16:9",
    "duration": 5,
    "resolution": "480p"
  }'
```

### 场景 3：首尾帧生视频

传入首帧与尾帧图片，模型生成两图之间的过渡视频。含首帧 / 尾帧时输出比例跟随输入素材，`ratio` 不生效。

```bash
curl -X POST "https://mm-internal-cn.leonecloud.com/api/v3/contents/generations/tasks" \
  -H "Authorization: Bearer your_auth_token_here" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "seedance-2-5",
    "content": [
      {
        "type": "text",
        "text": "画面从第一张图自然过渡到第二张图，镜头平稳推进，光影柔和，电影感转场"
      },
      {
        "type": "image_url",
        "image_url": { "url": "https://example.com/frame_start.jpg" },
        "role": "first_frame"
      },
      {
        "type": "image_url",
        "image_url": { "url": "https://example.com/frame_end.jpg" },
        "role": "last_frame"
      }
    ],
    "duration": 5,
    "resolution": "480p"
  }'
```

### 场景 4：用 frames 指定帧数

`frames` 与 `duration` 二选一，同时传入时 `frames` 优先。计费时长按 `frames / 24` 向下取整换算成秒。

```bash
curl -X POST "https://mm-internal-cn.leonecloud.com/api/v3/contents/generations/tasks" \
  -H "Authorization: Bearer your_auth_token_here" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "seedance-2-5",
    "content": [
      { "type": "text", "text": "一只橘猫在阳光下的窗台上伸懒腰，慢镜头" }
    ],
    "ratio": "16:9",
    "frames": 121,
    "resolution": "720p"
  }'
```

### 场景 5：多图参考生视频

在提示词中通过 "图片1"、"图片2" 引用（最多 30 张）。

```bash
curl -X POST "https://mm-internal-cn.leonecloud.com/api/v3/contents/generations/tasks" \
  -H "Authorization: Bearer your_auth_token_here" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "seedance-2-5",
    "content": [
      {
        "type": "text",
        "text": "[图片1]戴着眼镜穿着蓝色T恤的男生和[图片2]的柯基小狗，坐在[图片3]的草坪上，视频卡通风格"
      },
      { "type": "image_url", "image_url": { "url": "https://example.com/boy.png" }, "role": "reference_image" },
      { "type": "image_url", "image_url": { "url": "https://example.com/dog.png" }, "role": "reference_image" },
      { "type": "image_url", "image_url": { "url": "https://example.com/grass.png" }, "role": "reference_image" }
    ],
    "ratio": "16:9",
    "duration": 5,
    "resolution": "720p"
  }'
```

### 场景 6：视频编辑

`edit` 会强制 `ratio=adaptive` 与 `duration=-1`（跟随输入视频），仅 seedance-2-5 系列可用。

```bash
curl -X POST "https://mm-internal-cn.leonecloud.com/api/v3/contents/generations/tasks" \
  -H "Authorization: Bearer your_auth_token_here" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "seedance-2-5",
    "content": [
      { "type": "text", "text": "将视频1中的房子外立面墙壁刷成蓝色，天气改为雪天" },
      { "type": "video_url", "video_url": { "url": "https://example.com/house.mp4" }, "role": "reference_video" }
    ],
    "omni_reference_task_type": "edit",
    "resolution": "720p"
  }'
```

### 场景 7：联网搜索增强

```bash
curl -X POST "https://mm-internal-cn.leonecloud.com/api/v3/contents/generations/tasks" \
  -H "Authorization: Bearer your_auth_token_here" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "seedance-2-5",
    "content": [
      { "type": "text", "text": "制作一段介绍当下最热门旅游目的地的短视频" }
    ],
    "ratio": "16:9",
    "duration": 5,
    "resolution": "720p",
    "tools": [{ "type": "web_search" }]
  }'
```

## 接口详情

### 1. 创建任务

`POST /api/v3/contents/generations/tasks`

`Content-Type: application/json`

#### 请求参数

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| model | string | 是 | 模型名称，取值见 [model 取值](#model-取值) |
| content | array | 是 | 内容数组，详见 [content 内容项](#content-内容项) |
| omni_reference_task_type | string | 否 | 任务类型：auto / reference / edit / extend。不传时按输入素材自动推断，详见 [任务类型](#任务类型-omni_reference_task_type) |
| ratio | string | 否 | 宽高比：adaptive(默认) / 16:9 / 9:16 / 1:1 / 4:3 / 3:4 / 21:9。部分场景会强制为 adaptive |
| duration | int | 否 | 视频时长（秒）：seedance-2-5 系列 4~30，seedance-2-0 系列 4~15；-1 为智能时长，仅 seedance-2-5 系列支持。各 model 的范围与默认值见 [model 取值](#model-取值)。传了 `frames` 时本字段被忽略 |
| frames | int | 否 | 总帧数，范围 [29, 289]，与 `duration` 二选一，`frames` 优先。计费按 `frames / 24` 向下取整换算成秒 |
| resolution | string | 否 | 输出分辨率，默认 720p。各 model 支持的取值见 [model 取值](#model-取值) |
| generate_audio | bool | 否 | 是否生成音频，默认 true |
| output_format | string | 否 | 输出容器格式：mp4(默认) / mov |
| watermark | bool | 否 | 是否带水印，默认 false |
| seed | int | 否 | 随机种子，用于复现结果 |
| camera_fixed | bool | 否 | 是否固定摄像头 |
| return_last_frame | bool | 否 | 是否返回尾帧图片 |
| callback_url | string | 否 | 任务完成后的回调通知 URL，回调体格式见 [回调通知](#回调通知) |
| priority | int | 否 | 优先级，范围 [0, 9]，默认 0 |
| service_tier | string | 否 | 服务档位：default(默认) / flex |
| execution_expires_after | int | 否 | 任务执行超时（秒），范围 [3600, 259200]，默认 172800 |
| safety_identifier | string | 否 | 安全标识，用于业务侧追踪 |
| draft | bool | 否 | 样片模式，输出 480p 预览，默认 false |
| tools | array | 否 | 工具列表，当前支持 `[{"type": "web_search"}]` 联网搜索 |
| task_nickname | string | 否 | 任务昵称，便于业务侧标记 |
| real_person_mode | bool | 否 | 真人模式：自动把图片 / 视频转为虚拟素材再生成 |

#### content 内容项

| 类型 | 字段 | 说明 |
| --- | --- | --- |
| text | `type: "text", text: "提示词"` | 必须，视频描述提示词（3-20000 字符） |
| image_url | `type: "image_url", image_url: {"url": "URL"}, role: 见下方` | 参考图片（最多 30 张，支持 jpeg/png/webp/bmp/tiff/gif，宽高比 0.4-2.5，尺寸 300-6000px，最大 30MB） |
| video_url | `type: "video_url", video_url: {"url": "URL"}, role: "reference_video"` | 参考视频（最多 10 个，mp4/mov，480p/720p，2-15 秒，最大 50MB，24-60FPS） |
| audio_url | `type: "audio_url", audio_url: {"url": "URL"}, role: "reference_audio"` | 参考音频（最多 10 个，wav/mp3，2-15 秒，最大 15MB） |

content 数组中必须包含至少一个 `text` 类型的内容项作为提示词。在提示词中通过 "图片1"、"图片2"、"视频1"、"音频1" 引用对应位置的参考素材。输入包含视频时，计费单价与纯文本 / 图片输入不同。

#### image_url 的 role 取值

| role 值 | 说明 |
| --- | --- |
| reference_image | 参考图片 |
| first_frame | 首帧图片 |
| last_frame | 尾帧图片 |

#### 响应参数

创建成功返回 HTTP 200：

| 参数 | 类型 | 说明 |
| --- | --- | --- |
| id | string | 任务 ID，用于查询任务状态 |

响应示例：

```json
{
  "id": "task_20260817163906_i0ing18n"
}
```

创建失败返回对应的 HTTP 状态码，详见 [错误码](#错误码)：

```json
{
  "error": {
    "code": "InvalidParameter",
    "message": "不支持的 model: seedance-9",
    "type": "BadRequest"
  }
}
```

### 2. 查询任务状态

`GET /api/v3/contents/generations/tasks/{id}`

#### 路径参数

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| id | string | 是 | 任务 ID（创建接口返回的 id） |

#### 响应参数

| 参数 | 类型 | 说明 |
| --- | --- | --- |
| id | string | 任务 ID |
| model | string | 创建任务时传入的 model 原值 |
| status | string | 任务状态：submitted / running / succeeded / failed |
| content.video_url | string | 生成的视频 URL（succeeded 时返回） |
| content.last_frame_url | string | 尾帧图片 URL（请求了 `return_last_frame` 且有结果时返回） |
| error.code | string | 错误码（failed 时返回） |
| error.message | string | 错误消息（failed 时返回） |
| created_at | int | 创建时间（Unix 秒） |
| updated_at | int | 更新时间（Unix 秒） |
| usage.completion_tokens | int | 生成 token 数 |
| usage.prompt_tokens | int | 提示 token 数（上游返回时才出现） |
| usage.total_tokens | int | 总 token 数（上游返回时才出现） |

恒定返回的字段只有 `id` / `model` / `status` / `created_at` / `updated_at`。`duration` / `frames` / `frames_per_second` / `generate_audio` / `ratio` / `resolution` / `seed` / `service_tier` / `priority` / `output_format` / `draft` / `execution_expires_after` / `usage` / `content.last_frame_url` 等字段仅在有对应信息时返回，请按「字段可能不存在」的方式做兼容解析。token 用量只在任务结算完成后（或有上游用量数据时）返回；尾帧图片仅在请求了 `return_last_frame` 且上游返回了尾帧时才有。

#### 响应示例

**处理中**

```json
{
  "id": "task_20260817163906_i0ing18n",
  "model": "seedance-2-5",
  "status": "running",
  "created_at": 1786955946,
  "updated_at": 1786955950
}
```

**成功**

```json
{
  "id": "task_20260817163906_i0ing18n",
  "model": "seedance-2-5",
  "status": "succeeded",
  "content": {
    "video_url": "https://fc-gw-sh.oss-accelerate.aliyuncs.com/videos/2026/08/17/output.mp4"
  },
  "duration": 5,
  "ratio": "16:9",
  "resolution": "720p",
  "output_format": "mp4",
  "usage": {
    "completion_tokens": 49725,
    "total_tokens": 49725
  },
  "created_at": 1786955946,
  "updated_at": 1786956142
}
```

**成功（请求了 `return_last_frame: true` 且有尾帧结果）**

```json
{
  "id": "task_20260817163906_i0ing18n",
  "model": "seedance-2-5",
  "status": "succeeded",
  "content": {
    "video_url": "https://fc-gw-sh.oss-accelerate.aliyuncs.com/videos/2026/08/17/output.mp4",
    "last_frame_url": "https://fc-gw-sh.oss-accelerate.aliyuncs.com/images/2026/08/17/last_frame.png"
  },
  "usage": {
    "completion_tokens": 49725,
    "prompt_tokens": 318,
    "total_tokens": 50043
  },
  "created_at": 1786955946,
  "updated_at": 1786956142
}
```

**失败**

```json
{
  "id": "task_20260817163906_i0ing18n",
  "model": "seedance-2-5",
  "status": "failed",
  "error": {
    "code": "InternalServiceError",
    "message": "视频生成失败，请重试"
  },
  "created_at": 1786955946,
  "updated_at": 1786956060
}
```

**任务不存在（HTTP 404）**

```json
{
  "error": {
    "code": "NotFound",
    "message": "任务不存在",
    "type": "NotFound"
  }
}
```

### 3. 查询任务列表

按条件批量查询任务，一次返回多个任务的状态。

`GET /api/v3/contents/generations/tasks`

#### Query 参数

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| filter.task_ids | string[] | 否 | 任务 ID 筛选，精确匹配，支持多个。通过重复参数名传递：`filter.task_ids=id1&filter.task_ids=id2`。不传则返回当前 API Key 名下的任务列表 |
| filter.status | string | 否 | 状态筛选，可选值：queued / running / succeeded / failed。其中 queued 与 running 均表示处理中。不传则不按状态筛选（注意：暂不支持 cancelled，传入该值或其它未知值将返回空列表） |
| page_num | int | 否 | 页码，默认 1，取值范围 [1, 500]，越界自动收敛到边界 |
| page_size | int | 否 | 每页数量，默认 20，取值范围 [1, 500]，越界自动收敛到边界 |

只能查询当前 API Key 名下的任务，无法查询他人任务。

#### 响应参数

| 参数 | 类型 | 说明 |
| --- | --- | --- |
| items | array | 任务列表，每个元素结构与「2. 查询任务状态」的响应完全一致（id / model / status / content / error / created_at / updated_at / usage 等，字段说明见上一节） |
| total | int | 符合筛选条件的任务总数 |

#### 请求示例

```http
GET /api/v3/contents/generations/tasks?filter.task_ids=task_20260817163906_i0ing18n&filter.task_ids=task_20260817170000_abcd1234&filter.status=succeeded&page_num=1&page_size=20
Authorization: Bearer {API_KEY}
```

#### 响应示例

```json
{
  "items": [
    {
      "id": "task_20260817163906_i0ing18n",
      "model": "seedance-2-5",
      "status": "succeeded",
      "content": {
        "video_url": "https://fc-gw-sh.oss-accelerate.aliyuncs.com/videos/2026/08/17/output.mp4"
      },
      "duration": 5,
      "ratio": "16:9",
      "resolution": "720p",
      "output_format": "mp4",
      "usage": {
        "completion_tokens": 49725,
        "total_tokens": 49725
      },
      "created_at": 1786955946,
      "updated_at": 1786956142
    },
    {
      "id": "task_20260817170000_abcd1234",
      "model": "seedance-2-5",
      "status": "succeeded",
      "content": {
        "video_url": "https://fc-gw-sh.oss-accelerate.aliyuncs.com/videos/2026/08/17/output2.mp4"
      },
      "created_at": 1786957200,
      "updated_at": 1786957380
    }
  ],
  "total": 2
}
```

## 回调通知

当任务完成（成功或失败）时，如果创建任务时提供了 `callback_url`，系统会向该 URL 发送 POST 请求。回调体结构与查询接口不同，请单独实现解析。

### Headers

```http
Content-Type: application/json
X-Funcloud-Event: task.completed
X-Funcloud-Signature: {签名}
```

### Body

```json
{
  "event": "task.completed",
  "taskId": "task_20260817163906_i0ing18n",
  "status": "success",
  "result": ["https://fc-gw-sh.oss-accelerate.aliyuncs.com/videos/output.mp4"],
  "errorMsg": "",
  "timestamp": "2026-08-17T16:42:22+08:00",
  "signature": "a1b2c3d4e5f6..."
}
```

## 参数取值范围

### 分辨率 (resolution)

| 值 | 说明 |
| --- | --- |
| 480p | 低清 |
| 720p | 默认，推荐 |
| 1080p | 高清，仅 seedance-2-5 支持 |

向不支持 1080p 的 model 传入 1080p 会返回 HTTP 400。

### 宽高比 (ratio)

| 值 | 说明 |
| --- | --- |
| adaptive | 自适应（默认，跟随输入素材比例） |
| 16:9 | 横屏 |
| 9:16 | 竖屏 |
| 1:1 | 方形 |
| 4:3 | 标准横屏 |
| 3:4 | 标准竖屏 |
| 21:9 | 超宽屏 |

### 时长 (duration) 与帧数 (frames)

- duration 范围：seedance-2-5 系列 4 ~ 30 秒，seedance-2-0 系列 4 ~ 15 秒；传 -1 为智能时长，仅 seedance-2-5 系列支持
- frames 范围：29 ~ 289
- 两者同时传入时 frames 优先，duration 被忽略
- 传 frames 时计费时长 = `frames / 24` 向下取整（不足 1 秒按 1 秒计）
- 默认值见 [默认值](#默认值)

### 输出格式 (output_format)

| 值 | 说明 |
| --- | --- |
| mp4 | 默认，通用性最好 |
| mov | 适合后期剪辑流程 |

### 任务类型 (omni_reference_task_type)

不传时按输入素材自动推断：带参考视频 → auto；否则 → reference。

| omni_reference_task_type | 说明 | 素材要求 | ratio | duration |
| --- | --- | --- | --- | --- |
| auto | 自动：带参考视频时等效视频编辑 | 无强制 | 推荐 adaptive（带参考视频时强制） | 推荐 -1（带参考视频时强制） |
| reference | 文本 / 参考图片 / 首尾帧生成视频 | 无强制 | 可自定义（含首尾帧时强制 adaptive） | 可自定义（范围见 [model 取值](#model-取值)） |
| edit | 视频编辑：基于参考视频生成 | 必须含参考视频 | 强制 adaptive | 强制 -1 |
| extend | 视频延长 | 必须含参考视频 | 强制 adaptive | 可自定义（4~30 或 -1） |

强制 adaptive 时输出比例跟随输入素材，传入的 ratio 不生效；强制 -1 时输出时长由模型决定，该场景按 token 计费，仅 seedance-2-5 系列可用。edit / extend 缺少参考视频时请求会被拒绝。

## 错误码

错误通过 HTTP 状态码与 `error` 对象返回：

```json
{
  "error": {
    "code": "InvalidParameter",
    "message": "错误描述",
    "type": "BadRequest"
  }
}
```

| HTTP | error.code | error.type | 说明 |
| --- | --- | --- | --- |
| 400 | InvalidParameter | BadRequest | 参数缺失或格式错误、model 取值不支持、分辨率 / 时长不被该 model 支持 |
| 401 | AuthenticationError | Unauthorized | API Key 无效或缺失 |
| 402 | InsufficientBalance | PaymentRequired | 余额不足 |
| 403 | PermissionDenied | Forbidden | 无权限访问该资源 |
| 404 | NotFound | NotFound | 任务不存在 |
| 500 | InternalServiceError | InternalServerError | 服务器内部错误，可重试 |

建议客户端优先按 HTTP 状态码做分支（可重试 / 需改参数 / 需充值），再用 `error.code` 做细分。

## 从 V2 迁移

- **路径**：统一为 `POST /api/v3/contents/generations/tasks`，模型由 `model` 字段指定；查询为 `GET /api/v3/contents/generations/tasks/{id}`
- **字段命名**：snake_case（`generate_audio`、`camera_fixed`）
- **响应结构**：创建返回 `{"id": "..."}`，查询直接返回任务对象，错误用 HTTP 状态码 + `error` 对象
- **默认值**：`ratio` 为 adaptive；`duration` 见 [默认值](#默认值)
- **V3 新增能力**：`frames`、`output_format`、`priority`、`service_tier`、`draft`、`tools`

V2 接口继续可用，两者共享同一套任务与计费体系，可按客户端习惯任选其一。

## 最佳实践

### 1. 轮询策略

- 前 30 秒：每 3 秒查询一次
- 30 秒 ~ 2 分钟：每 5 秒查询一次
- 2 分钟后：每 10 秒查询一次

### 2. 状态判断

用 `status` 做业务分支：submitted / running 为处理中，succeeded 取 `content.video_url`，failed 读 `error.message`。

### 3. Prompt 建议

- 提示词长度：3 ~ 20,000 字符
- 提示词 = 主体 + 运动，背景 + 运动，镜头 + 运动
- 用简洁准确的自然语言写出想要的效果
- 可以指定镜头运动（推进、拉远、环绕等）
- 通过 "图片1"、"图片2"、"视频1"、"音频1" 引用 content 中对应位置的参考素材
- 生成结果不符合预期时，建议把抽象描述换成具象描述
- 有明确效果预期时，建议先用生图模型生成符合预期的图片，再用图生视频

## 常见问题

**Q1: frames 和 duration 都传了会怎样？**

A: `frames` 优先，`duration` 被忽略。计费时长 = `frames / 24` 向下取整，不足 1 秒按 1 秒计。

**Q2: 为什么传 `duration: -1` 报错了？**

A: 智能时长仅 seedance-2-5 支持。seedance-2-0 系列按秒计费，无法处理不确定的时长，请指定 4 ~ 15 之间的具体秒数。

**Q3: 查询响应里为什么缺少 duration、resolution 这些字段？**

A: 这些字段仅在有对应信息时返回，恒定存在的字段只有 `id` / `model` / `status` / `created_at` / `updated_at`。请按「字段可能不存在」的方式做兼容解析。

**Q4: 我传的 model 会被改写吗？**

A: 不会。查询响应里的 `model` 原样回显创建任务时传入的值。

**Q5: V3 只能用 Seedance 2.5 吗？**

A: 不是。V3 协议对 Seedance 全系模型开放，2.5 与 2.0 / 2.0 Fast / 2.0 Mini 都可以用，取值见 [model 取值](#model-取值)。同一套请求体只换 `model` 即可切换模型，各档位的差异只在分辨率与时长的取值范围上。

**Q6: content 中可以不传 text 吗？**

A: 不可以。content 数组中必须包含至少一个 `text` 类型的内容项作为提示词，否则请求会被拒绝（HTTP 400）。
