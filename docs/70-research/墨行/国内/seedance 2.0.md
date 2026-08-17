---
status: current
owner: Dev Team
last-reviewed: 2026-08-11
---

# doubao-seedance-2.0

豆包大模型团队推出的新一代专业级多模态创作视频模型 Seedance 2.0，支持图像、视频、音频等多模态作为参考输入生成视频，还具备视频编辑、延长等能力，能高精度还原各类细节并稳定角色特征，具备极致拟真的视听稳定性，深度适配商业广告、影视制作与社交媒体营销等各大核心场景。

- **模型 ID**: `doubao-seedance-2-0-260128`
- **提供商**: 豆包
- **类型**: video
- **创建入口**: `https://www.moxing.pro/v1/media/generations`

## 能力与接口

### 视频生成

`POST /v1/media/generations`

异步任务；提交后轮询 `GET /v1/media/tasks/:task_id`。支持文生视频、图生视频和参考生视频三类场景。

#### 生成场景

- 文生视频
- 图生视频
- 参考生视频

#### 请求参数

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `aspect_ratio` | string | 否 | 画幅比例；支持 `16:9` / `4:3` / `1:1` / `3:4` / `9:16` / `21:9` / `adaptive`。示例：`16:9` |
| `capability` | string | 是 | 能力类型；固定为 `video_generation` |
| `control_mode` | string | 是 | 控制模式；文生视频固定 `none`；图生视频可用 `none` 或 `end_frame`；参考生视频固定 `reference`。示例：`none` |
| `duration_seconds` | integer | 否 | 视频时长（秒）；支持 4-15 秒，或 `-1` 表示自动。示例：`4` |
| `input_mode` | string | 是 | 输入模式；文生视频使用 `text`；图生视频使用 `single_image`；参考生视频使用 `multi_image`。示例：`text` |
| `model` | string | 是 | 模型 ID；固定使用 `doubao-seedance-2-0-260128`。 |
| `prompt` | string | 是 | 提示词；描述想要生成的视频内容，最长 2500 字符。参考生视频场景中用于描述基于参考素材生成的目标效果。示例：图片1中的人物站在海边，夕阳下回眸一笑 |
| `resolution` | string | 否 | 分辨率；支持 `480p` / `720p`。示例：`480p` |
| `with_audio` | boolean | 否 | 同步生成音频；`true` 时同步生成音频；`false` 时不生成音频。示例：`true` |
| `watermark` | string | 否 | 水印；上游官方字段；`true` 含水印，`false` 无水印。平台原样透传，缺省时以上游默认为准。模型专属/上游扩展：不属于平台公共基础参数；按本行说明放入 extra 或对应平台兼容字段后，平台会透传或映射到上游。示例：`false` |

#### 响应格式

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `created_at` | string | 任务创建时间（Unix 时间戳） |
| `error_message` | string | 终态失败时的错误描述 |
| `progress` | string | 0-100 之间的整数，表示生成进度 |
| `result` | string | 终态成功时返回视频/资源信息，包含 `url`、`duration_seconds` 等字段 |
| `status` | string | `queued` / `running` / `succeeded` / `failed` |
| `task_id` | string | 任务唯一 ID，用于轮询 `GET /v1/media/tasks/:id` |
| `usage` | string | 本次任务的额度消耗（如有） |

### 异步查询生成结果

`GET /v1/media/tasks/:id`

创建任务后先保留 `task_id`，再轮询这个接口直到 `succeeded` 或 `failed`。图片和视频模型都适用。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `task_id` | string | 任务唯一标识。 |
| `status` | string | `queued` / `running` / `succeeded` / `failed`。 |
| `status_code` | integer | `queued` / `running` 通常为 202，`succeeded` 为 200，`failed` 为 400。 |
| `result` | object | 视频任务成功后，`result` 通常包含主视频地址、带水印地址和封面图。 |
| `error_message` | string | 失败时返回，说明失败原因。 |

建议保留退避轮询，不要把这个接口当同步请求反复高频调用。

```bash
curl -sS "https://www.moxing.pro/v1/media/tasks/:id" \
  -H "Authorization: Bearer sk-xxxx"
```

响应示例：

```json
{
  "object": "media.task",
  "task_id": "df6b6427206441c9adaad913bee84f8e",
  "status": "succeeded",
  "status_code": 200,
  "capability": "video_generation",
  "model": "your-video-model",
  "created_at": 1774834768,
  "updated_at": 1774834790,
  "result": {
    "type": "video",
    "primary_url": "https://resource.moxing.pro/video/df6b6427206441c9adaad913bee84f8e/main.mp4",
    "urls": [
      "https://resource.moxing.pro/video/df6b6427206441c9adaad913bee84f8e/main.mp4"
    ],
    "watermark_url": "https://resource.moxing.pro/video/df6b6427206441c9adaad913bee84f8e/watermark.mp4",
    "watermark_urls": [
      "https://resource.moxing.pro/video/df6b6427206441c9adaad913bee84f8e/watermark.mp4"
    ],
    "cover_url": "https://resource.moxing.pro/image/df6b6427206441c9adaad913bee84f8e/cover.jpg"
  }
}
```

## 错误与限制

### 错误码

| 错误码 | HTTP | 说明 |
| --- | --- | --- |
| `invalid_request_error` | 400 | 请求参数缺失、格式错误或字段值不在允许范围内 |
| `invalid_api_key` | 401 | API Key 为空、格式错误或已失效；请确认 `Authorization: Bearer sk-...` |
| `insufficient_quota` | 403 | 账户余额或配额不足；请充值或申请扩容 |
| `model_not_found` | 404 | 请求的 `model` 不存在或当前账户无权访问 |
| `rate_limit_exceeded` | 429 | 触发并发或 RPM 限流；建议指数退避后重试 |
| `internal_server_error` | 500 | 平台内部异常；可重试，持续异常请联系技术支持 |
| `upstream_unavailable` | 502 | 模型服务暂时不可用或超时；建议稍后重试 |

### Limits

| 限制项 | 说明 |
| --- | --- |
| 分辨率 | `480p` / `720p` |
| 参考素材 | 普通资产参考可传公网 URL 或 `asset://upstream_id`；人像库参考传 `asset://upstream_id`。 |
| 平台到上游字段映射 | 平台字段与火山方舟 Seedance 2.0 官方字段基本一致，`duration_seconds`、`aspect_ratio`、`with_audio`、`watermark`、`image`、`end_image`、`reference_images`、`reference_videos`、`reference_audios` 均按官方接口透传；`input_mode`、`control_mode` 为平台侧场景编排字段，不直接传给上游。 |
| 画幅比例 | `16:9` / `4:3` / `1:1` / `3:4` / `9:16` / `21:9` / `adaptive` |
| 视频时长 | 4-15 秒，或 `-1` 自动 |

## 素材库

### 推荐流程

创建素材组 → 上传素材 → 等待 Active → 引用素材生成视频

1. 使用本页素材库创建素材组并上传素材，记录创建成功后的素材 ID。
2. 查询素材状态；仅 `Active` 状态的素材可以用于视频生成。
3. 切换到模型 API，在 `reference_images[]` 中传入 `asset://{素材 ID}`。

### 接口范围

接口根路径：`https://www.moxing.pro/joycreator/openApi/v1/asset`

素材库接口独立于统一 `/v1/*` 网关路径，当前用于 JoyCreator TOB 素材组与素材管理。

鉴权方式为平台 API Key：`Authorization: Bearer sk-...`。

### 接口清单

| 能力 | 方法 | 路径 | 说明 |
| --- | --- | --- | --- |
| 创建素材组 | POST | `/joycreator/openApi/v1/asset/group/create` | 创建素材组，返回对外使用的 `id` 与 `groupId`。 |
| 更新素材组 | POST | `/joycreator/openApi/v1/asset/group/:id` | 按接口返回的 `id` 更新素材组名称和描述。 |
| 查询素材组 | POST | `/joycreator/openApi/v1/asset/group/detail/:id` | 查询素材组详情。 |
| 创建素材 | POST | `/joycreator/openApi/v1/asset/create` | 上传素材记录，`groupId` 使用素材组接口返回的 `id`。 |
| 更新素材 | POST | `/joycreator/openApi/v1/asset/:id` | 更新素材名称。 |
| 删除素材 | DELETE | `/joycreator/openApi/v1/asset/:id` | 删除素材。 |
| 查询素材 | POST | `/joycreator/openApi/v1/asset/detail/:id` | 查询素材处理状态和 `vendorUrl`。 |

### 关键约定

- 接口返回中的 `id` 为后续调用的主键，不是本地数据库自增 id。
- 素材组详情返回的 `groupId` 与素材详情返回的 `assetId` 为上游业务标识，可用于排障和对账。
- 创建素材后通常需要轮询 `/asset/detail/:id`，直到 `status=1` 且拿到 `vendorUrl`。
- 创建图片素材时，URL 支持 HTTP/HTTPS 链接或 base64 图片；base64 会先转存为资源 URL 后再提交上游，素材记录不保存原始 base64。

### 创建素材组

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `Name` | string | 是 | 素材组名称，最长 64 字符。 |
| `Description` | string | 否 | 素材组描述，最长 300 字符。 |
| `GroupType` | string | 否 | 当前仅支持 `AIGC`。 |

```bash
curl -X POST 'https://www.moxing.pro/joycreator/openApi/v1/asset/group/create' \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer sk-xxxx' \
  -d '{
    "Name": "tenant-A-group",
    "Description": "租户A的素材组",
    "GroupType": "AIGC"
  }'
```

### 创建素材

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `groupId` | string / number | 是 | 使用创建素材组接口返回的 `id`。 |
| `URL` | string | 是 | 素材地址。`AssetType=Image` 时支持 HTTP(S) URL、`data:image/...;base64,...` 或裸 base64；Video/Audio 仍需 HTTP(S) URL。 |
| `AssetType` | string | 是 | 支持 `Image` / `Video` / `Audio`。 |
| `Name` | string | 否 | 素材名称，最长 64 字符。 |

```bash
curl -X POST 'https://www.moxing.pro/joycreator/openApi/v1/asset/create' \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer sk-xxxx' \
  -d '{
    "groupId": 34,
    "URL": "https://example.com/material.jpg",
    "AssetType": "Image",
    "Name": "test-face"
  }'
```

### 更新素材组

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `Name` / `Description` | string | 二选一 | 至少传一个字段。 |

### 更新素材

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `Name` | string | 是 | 更新后的素材名称。 |

### 查询素材详情

```bash
curl -X POST 'https://www.moxing.pro/joycreator/openApi/v1/asset/detail/52' \
  -H 'Authorization: Bearer sk-xxxx'
```

### 素材组详情响应

```json
{
  "requestId": "2026051910061719312600056043374",
  "error": null,
  "result": {
    "group": {
      "id": "34",
      "groupId": "group-20260519100444-99fv8",
      "groupName": "tenant-A-group-updated",
      "groupDesc": "更新后的素材组描述",
      "groupType": "AIGC",
      "provider": "joycreator",
      "status": 1
    }
  }
}
```

### 素材状态查询

创建素材成功后，建议轮询素材详情接口，直到 `status` 进入终态。

| 字段 | 说明 |
| --- | --- |
| `status` | 本地状态：`0` 处理中，`1` 成功，`2` 失败。 |
| `vendorStatus` | 上游原始状态，例如 `Processing`、`Active`、`Failed`。 |
| `vendorUrl` | 上游处理完成后的可用素材地址。 |
| `errorMsg` | 失败原因；仅失败时有值。 |

```json
{
  "requestId": "2026051910300012345600000000001",
  "error": null,
  "result": {
    "asset": {
      "id": "52",
      "assetId": "asset-20260519102959-abcd1",
      "groupId": "group-20260519100444-99fv8",
      "groupName": "tenant-A-group-updated",
      "assetName": "test-face",
      "assetType": "Image",
      "assetUrl": "https://example.com/material.jpg",
      "vendorUrl": "https://resource.moxing.pro/image/xxx.jpg",
      "vendorStatus": "Active",
      "status": 1,
      "errorMsg": ""
    }
  }
}
```

### 模型调用示例

素材处理完成后，在 `reference_images[]` 中传入 `asset://{素材 ID}`。

```bash
curl -X POST 'https://www.moxing.pro/v1/media/generations' \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer sk-xxxx' \
  -d '{
    "model": "doubao-seedance-2-0-260128",
    "capability": "video_generation",
    "input_mode": "multi_image",
    "control_mode": "reference",
    "prompt": "让参考素材中的人物在海边自然行走，镜头缓慢推进",
    "reference_images": ["asset://asset-20260519102959-abcd1"],
    "duration_seconds": 5,
    "resolution": "720p",
    "aspect_ratio": "16:9",
    "with_audio": true
  }'
```

### 错误响应

素材库接口统一返回 `requestId` / `error` / `result` 结构；排查时优先记录 `requestId`。

```json
{
  "requestId": "2026051910165121091900099033045",
  "error": {
    "code": 500,
    "message": "素材创建失败：任务执行失败，请稍后重试"
  },
  "result": {}
}
```
