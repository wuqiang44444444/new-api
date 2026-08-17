---
status: current
owner: Dev Team
last-reviewed: 2026-08-11
---

# doubao-seedance-2-0-fast

Seedance 2.0 fast 是豆包大模型团队推出的新一代多模态视频创作模型，它继承了 Seedance 2.0 模型的核心功能和优势，生成速度更快。支持图片、视频、音频等多模态素材参考生成视频，同时具备视频编辑与延长能力，使生视频工具进入可精准生成、可复用迭代的工业化新阶段。模型对物理规律的理解持续深化、更贴合真实世界，意图理解能力显著提升，能严格遵循指令细节约束，从而保障专业级叙事的可信度。

- **模型 ID**: `doubao-seedance-2-0-fast-260128`
- **提供商**: 豆包
- **类型**: video
- **创建入口**: `https://www.moxing.pro/v1/media/generations`

## 能力与接口

### 视频生成

`POST /v1/media/generations`

异步任务；提交后轮询 `GET /v1/media/tasks/:task_id`。针对 `doubao-seedance-2-0-fast-260128`，本页按火山官方多模态参数形态展示。请求请使用 `model`、`content`、`duration`、`resolution`、`ratio`、`generate_audio`、`watermark` 等字段。

#### 生成场景

- 文生视频
- 图生视频（首帧）
- 多图生视频
- 视频生视频
- 多模态参考生视频

#### 请求参数

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `duration` | any | 否 | 视频时长；单位秒。常用值 5；也支持更长时长场景。示例：`5` |
| `model` | string | 是 | 模型 ID；固定使用 `doubao-seedance-2-0-fast-260128`。 |
| `resolution` | string | 否 | 分辨率；建议使用 `720p`；fast 版本常用展示为 `480p` / `720p`。 |
| `content` | string | 是 | 多模态输入；必填数组。元素支持 `type=text` / `image_url` / `video_url` / `audio_url`。图片可用 `role=first_frame` 或 `reference_image`；视频可用 `role=reference_video`；音频可用 `role=reference_audio`。多图生视频通过多个 `reference_image` 组合；视频生视频通过 `reference_video`，可叠加 `reference_image` / `reference_audio`。`image_url` / `video_url` / `audio_url` 推荐使用嵌套格式 `{"url":"..."}`。模型专属/上游扩展：不属于平台公共基础参数；按本行说明放入 extra 或对应平台兼容字段后，平台会透传或映射到上游。示例：`[{"text":"海边日落","type":"text"}]` |
| `generate_audio` | string | 否 | 同步生成音频；`true` 时生成带音频视频；`false` 时不生成。模型专属/上游扩展。示例：`true` |
| `ratio` | string | 否 | 画幅比例；常见支持 `16:9` / `4:3` / `1:1` / `3:4` / `9:16` / `21:9` / `adaptive`。模型专属/上游扩展。示例：`16:9` |
| `watermark` | string | 否 | 水印；是否带水印；如需无水印请显式传 `false`。模型专属/上游扩展。示例：`false` |

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
| 多图生视频 | 通过 `content` 内多个 `role=reference_image` 的 `image_url` 组合 |
| 平台到上游字段映射 | 本页按火山官方多模态参数形态展示；`content`、`duration`、`resolution`、`ratio`、`generate_audio`、`watermark` 均为官方字段，平台原样透传上游，不做字段名映射。 |
| 视频生视频 | 通过 `content` 内 `role=reference_video` 的 `video_url`，可叠加 `reference_image` / `reference_audio` |
| 轮询 | 提交后使用 `GET /v1/media/tasks/:task_id` 查询结果 |
| 音频参考 | `audio_url` 需配合图片或视频使用，不建议仅传音频 |

## 素材库

### 推荐流程

创建素材组 → 上传素材 → 等待 Active → 引用素材生成视频

1. 使用本页国内官key素材库创建素材组，并上传图片、视频或音频素材。
2. 查询素材状态；仅 `Active` 状态的素材可以用于视频生成。
3. 切换到 模型 API，在 `content[].image_url.url` / `video_url.url` / `audio_url.url` 中传入 `asset://{素材 ID}`。

### 国内官key素材库

本接口封装火山方舟国内私域素材库。客户端只使用平台 Bearer `sk-...`，不会看到火山 AK/SK、Action URL 或上游素材 ID。

素材接口不传 `model`。平台素材 ID 可在 `doubao-seedance-2-0-fast-260128`、`doubao-seedance-2-0-mini-260615` 与 `doubao-seedance-2-5-260628` 之间复用。

请求/响应字段采用官方大驼峰结构；`Id`、`GroupId`、`SessionId` 均为平台 ID。

### 接口清单

| 能力 | 方法 | 路径 |
| --- | --- | --- |
| 创建 / 列出素材组 | POST | `/v1/volc/assets/groups` · `/groups/list` |
| 查询素材组 | GET | `/v1/volc/assets/groups/:group_id` |
| 更新 / 删除素材组 | POST | `/v1/volc/assets/groups/:group_id/update` · `/delete` |
| 创建 / 列出素材 | POST | `/v1/volc/assets` · `/v1/volc/assets/list` |
| 查询素材 | GET | `/v1/volc/assets/:asset_id` |
| 更新 / 删除素材 | POST | `/v1/volc/assets/:asset_id/update` · `/delete` |
| 创建真人认证 | POST | `/v1/volc/assets/visual-validate/sessions` |
| 查询认证状态 / 结果 | GET | `/visual-validate/sessions/:id` · `/results/:id` |

### 创建素材组

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `Name` | string | 是 | 名称，最多 64 字符。 |
| `Description` | string | 否 | 描述，最多 300 字符。 |
| `GroupType` | string | 否 | API 直建当前使用 `AIGC`；真人分组通过 H5 认证生成。 |
| `ProjectName` | string | 否 | 项目名称，默认 `default`。 |

```bash
curl -X POST 'https://www.moxing.pro/v1/volc/assets/groups' \
  -H 'Authorization: Bearer sk-xxxx' \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: group-20260723-001' \
  -d '{
    "Name": "品牌虚拟人像",
    "Description": "广告视频参考人物",
    "GroupType": "AIGC",
    "ProjectName": "default"
  }'
```

### 创建素材

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `GroupId` | string | 是 | 平台素材组 ID。 |
| `URL` | string | 是 | 公网 URL；官方不支持 Base64。 |
| `Name` | string | 否 | 名称，最多 64 字符。 |
| `AssetType` | string | 是 | `Image` / `Video` / `Audio`。 |
| `ProjectName` | string | 否 | 默认 `default`，必须与素材组一致。 |

```bash
curl -X POST 'https://www.moxing.pro/v1/volc/assets' \
  -H 'Authorization: Bearer sk-xxxx' \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: asset-20260723-001' \
  -d '{
    "GroupId": "group-volc-cn-平台分组ID",
    "URL": "https://example.com/portrait.png",
    "Name": "人物正面图",
    "AssetType": "Image",
    "ProjectName": "default"
  }'
```

### 素材组：列表、查询、更新与删除

`POST /groups/list` 使用官方 `Filter.GroupIds` / `GroupType` / `Name`，以及 `PageNumber` / `PageSize` / `SortBy` / `SortOrder` / `ProjectName`。

单项查询通过路径传平台 ID，项目名用 `?ProjectName=default`。更新组支持 `Name` / `Description`；删除请求体可传 `ProjectName`。

**列出素材组：**

```bash
curl -X POST 'https://www.moxing.pro/v1/volc/assets/groups/list' \
  -H 'Authorization: Bearer sk-xxxx' \
  -H 'Content-Type: application/json' \
  -d '{
    "Filter": {
      "GroupType": "AIGC",
      "Name": "品牌"
    },
    "PageNumber": 1,
    "PageSize": 20,
    "SortBy": "CreateTime",
    "SortOrder": "Desc",
    "ProjectName": "default"
  }'
```

**查询单个素材组：**

```bash
curl 'https://www.moxing.pro/v1/volc/assets/groups/group-volc-cn-平台分组ID?ProjectName=default' \
  -H 'Authorization: Bearer sk-xxxx'
```

**更新素材组：**

```bash
curl -X POST 'https://www.moxing.pro/v1/volc/assets/groups/group-volc-cn-平台分组ID/update' \
  -H 'Authorization: Bearer sk-xxxx' \
  -H 'Content-Type: application/json' \
  -d '{
    "Name": "更新后的分组名称",
    "Description": "更新后的描述",
    "ProjectName": "default"
  }'
```

**删除素材组：**

```bash
curl -X POST 'https://www.moxing.pro/v1/volc/assets/groups/group-volc-cn-平台分组ID/delete' \
  -H 'Authorization: Bearer sk-xxxx' \
  -H 'Content-Type: application/json' \
  -d '{"ProjectName": "default"}'
```

### 素材：列表、查询、更新与删除

`POST /list` 支持官方 `Filter.GroupIds` / `Statuses`（`Active` / `Processing` / `Failed`）/ `Name`，以及 `PageNumber` / `PageSize` / `SortBy` / `SortOrder` / `ProjectName`。

单项查询通过路径传平台 ID，项目名用 `?ProjectName=default`。更新素材支持 `Name`；删除请求体可传 `ProjectName`。

**列出素材：**

```bash
curl -X POST 'https://www.moxing.pro/v1/volc/assets/list' \
  -H 'Authorization: Bearer sk-xxxx' \
  -H 'Content-Type: application/json' \
  -d '{
    "Filter": {
      "GroupIds": ["group-volc-cn-平台分组ID"],
      "Statuses": ["Active"],
      "Name": "人物"
    },
    "PageNumber": 1,
    "PageSize": 20,
    "SortBy": "CreateTime",
    "SortOrder": "Desc",
    "ProjectName": "default"
  }'
```

**查询单个素材：**

```bash
curl 'https://www.moxing.pro/v1/volc/assets/asset-volc-cn-平台素材ID?ProjectName=default' \
  -H 'Authorization: Bearer sk-xxxx'
```

**更新素材：**

```bash
curl -X POST 'https://www.moxing.pro/v1/volc/assets/asset-volc-cn-平台素材ID/update' \
  -H 'Authorization: Bearer sk-xxxx' \
  -H 'Content-Type: application/json' \
  -d '{
    "Name": "更新后的素材名称",
    "ProjectName": "default"
  }'
```

**删除素材：**

```bash
curl -X POST 'https://www.moxing.pro/v1/volc/assets/asset-volc-cn-平台素材ID/delete' \
  -H 'Authorization: Bearer sk-xxxx' \
  -H 'Content-Type: application/json' \
  -d '{"ProjectName": "default"}'
```

### 素材状态

| 状态 | 说明 |
| --- | --- |
| `Processing` | 异步处理中，不能用于视频生成。 |
| `Active` | 处理完成，可使用 `asset://` 引用。 |
| `Failed` | 处理失败，查看 `Error.Code` / `Error.Message`。 |

## 真人人像认证

真人素材组不能通过 `CreateAssetGroup` 直接创建，需要先完成 H5 人脸活体认证。流程如下：

1. 创建认证会话，打开响应 `Result.H5Link` 让被授权人完成认证。
2. 轮询 `GET /visual-validate/sessions/:session_id`，等待状态变为 `callback_received`。
3. 调用 `GET /visual-validate/results/:session_id`，获得真人 `GroupId`（类型为 `LivenessFace`）。
4. 使用普通创建素材接口，为该真人分组补充图片、视频或音频；火山会执行人脸一致性校验。

真实 `BytedToken` 不对外暴露，平台用会话 ID 替代。服务端生成并校验官方回调，运维需配置公网 HTTPS 的 `VOLC_CN_ASSET_CALLBACK_BASE_URL`。客户端传入的 `CallbackURL` 会被平台安全覆盖。

### 步骤 1：创建认证会话

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `ProjectName` | string | 否 | 项目名称，默认 `default`。 |

```bash
curl -X POST 'https://www.moxing.pro/v1/volc/assets/visual-validate/sessions' \
  -H 'Authorization: Bearer sk-xxxx' \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: visual-20260723-001' \
  -d '{"ProjectName":"default"}'
```

响应 `Result` 包含 `SessionId`（平台会话 ID）、`H5Link`（认证页面地址）。原始 `BytedToken` 已被替换，不对外暴露。

### 步骤 2：查询认证会话状态

```bash
curl 'https://www.moxing.pro/v1/volc/assets/visual-validate/sessions/session-volc-cn-平台会话ID' \
  -H 'Authorization: Bearer sk-xxxx'
```

响应 `Result` 包含 `SessionId`、`Status`、`ProjectName`、`CreateTime`、`UpdateTime`。建议每 3–5 秒轮询一次。

**会话状态：**

| 状态 | 说明 |
| --- | --- |
| `pending` | 等待 H5 认证，用户尚未完成。 |
| `callback_received` | 收到官方成功回调，可查询结果。 |
| `group_ready` | 已获得真人素材组，可直接使用 `GroupId`。 |
| `failed` | 认证失败，需重新创建会话。 |

### 步骤 3：获取真人素材组

```bash
curl 'https://www.moxing.pro/v1/volc/assets/visual-validate/results/session-volc-cn-平台会话ID' \
  -H 'Authorization: Bearer sk-xxxx'
```

响应 `Result.GroupId` 为平台真人分组 ID（`group-volc-cn-...`），类型为 `LivenessFace`。平台内部使用真实 `BytedToken` 调用官方接口，并自动创建分组映射。

### 步骤 4：为真人分组上传素材

使用上方「创建素材」接口，将 `GroupId` 设为真人分组 ID 即可。火山会对上传的素材执行人脸一致性校验，不匹配时返回 `FaceMismatch` 错误。

```bash
curl -X POST 'https://www.moxing.pro/v1/volc/assets' \
  -H 'Authorization: Bearer sk-xxxx' \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: asset-20260723-001' \
  -d '{
    "GroupId": "group-volc-cn-平台分组ID",
    "URL": "https://example.com/portrait.png",
    "Name": "人物正面图",
    "AssetType": "Image",
    "ProjectName": "default"
  }'
```

## 用于 fast / mini 视频

素材达到 `Active` 后使用 `asset://asset-volc-cn-...`。平台会固定到素材创建时的同一 channel key；同一请求混用不同火山账号素材返回 409，不回退其他 key。

```bash
curl -X POST 'https://www.moxing.pro/v1/media/generations' \
  -H 'Authorization: Bearer sk-xxxx' \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "doubao-seedance-2-0-fast-260128",
    "content": [
      { "type": "text", "text": "图片1中的人物在海边自然行走，保持主体一致" },
      {
        "type": "image_url",
        "role": "reference_image",
        "image_url": { "url": "asset://asset-volc-cn-平台素材ID" }
      }
    ],
    "duration": 5,
    "resolution": "720p",
    "ratio": "16:9",
    "generate_audio": true,
    "watermark": false
  }'
```

## 错误处理

| HTTP | 典型场景 |
| --- | --- |
| 400 | JSON 或官方参数无效。 |
| 403 | 资源不属于当前用户。 |
| 404 | 素材、分组或认证会话不存在。 |
| 409 | 素材未 Active、跨火山账号混用、绑定 key 未挂载目标模型。 |
| 429 | 平台或火山限流。 |
| 502 | 火山上游调用失败。 |
| 503 | 国内 AK/SK、回调地址或 channel key 未配置/不可用。 |
