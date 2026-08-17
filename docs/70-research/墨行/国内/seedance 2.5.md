---
status: current
owner: Dev Team
last-reviewed: 2026-08-11
---

# doubao-seedance-2.5

火山方舟 Seedance 2.5 视频生成模型。支持 4～30 秒视频输出、最多 50 个多模态参考素材，支持文生/图生/参考生/延长与纯音频参考；分辨率 `480p`/`720p`。视频编辑暂未对外开放。

- **模型 ID**: `doubao-seedance-2-5-260628`
- **提供商**: 豆包
- **类型**: video
- **创建入口**: `https://www.moxing.pro/v1/media/generations`

## 能力与接口

### 视频生成

`POST /v1/media/generations`

异步任务；提交后轮询 `GET /v1/media/tasks/:task_id`。针对 `doubao-seedance-2-5-260628`，本页按火山官方多模态参数形态展示（`model` + `content` + `duration`/`resolution`/`ratio`/`generate_audio`/`watermark`/`output_format`）。上游按 `content.role` + 提示词意图判定任务类型（文生/参考生/延长/首尾帧等）；延长、首尾帧对 `ratio`/`duration` 有硬约束，违反时可能异步返回 `InvalidParameter.TaskTypeConstraint`。视频编辑能力暂未对外开放，请勿在提示词中使用编辑类意图词。

#### 生成场景

- 文生视频
- 图生视频（首帧）
- 首尾帧生视频
- 多图参考生视频
- 参考视频生视频
- 视频延长
- 多模态参考生视频
- 纯音频参考生视频

#### 请求参数

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `duration` | any | 否 | 视频时长；单位秒。默认 `-1`；取值 `[4, 30]` 或 `-1`（模型在有效范围内自动选择）。视频延长、首帧/首尾帧可设 `[4, 30]` 或 `-1`。平台当前估价按时长枚举计费，建议优先使用已配置的秒数（如 `4`/`5`/`6`/`8`/`10`/`12`）；`duration=-1` 或未覆盖秒数可能导致估价失败。示例：`10` |
| `model` | string | 是 | 模型 ID；固定使用 `doubao-seedance-2-5-260628`。 |
| `resolution` | string | 否 | 分辨率；默认 `720p`；可选 `480p` / `720p`。Seedance 2.5 暂不支持 `1080p` / `4k`。示例：`720p` |
| `content` | string | 是 | 多模态输入；必填数组。元素支持 `type=text` / `image_url` / `video_url` / `audio_url`。图片 role：`first_frame`（可不填）、`last_frame`、`reference_image`；视频 role：`reference_video`；音频 role：`reference_audio`。首帧/首尾帧与多模态参考（含图/视频/音频）互斥不可混用。单次最多 50 个素材（图≤30、视频≤10、音频≤10；视频/音频各自总时长≤30s）。2.5 支持纯音频参考。url 支持公网 URL、Base64、`asset://{素材 ID}`。不支持直接上传含真人人脸的参考图/视频。模型专属/上游扩展：不属于平台公共基础参数；按本行说明放入 extra 或对应平台兼容字段后，平台会透传或映射到上游。示例：`[{"text":"海边日落","type":"text"}]` |
| `generate_audio` | string | 否 | 生成有声视频；默认 `true`。`true`：输出含同步人声/音效/背景音乐的视频（单声道）；建议对话放在双引号内以优化音频。`false`：输出无声视频。模型专属/上游扩展。示例：`true` |
| `output_format` | string | 否 | 输出格式；默认 `mp4`。`mp4`：通用兼容格式；`mov`：高色彩精度专业格式，推荐视频延长场景作为输入与输出。mov 采用 H.264 + yuv444p + PCM，部分播放器可能不兼容（可用 VLC / mpv / IINA / ffplay）。仅 Seedance 2.5 支持 mov。模型专属/上游扩展。示例：`mp4` |
| `ratio` | string | 否 | 画幅比例；默认 `adaptive`；可选 `16:9` / `4:3` / `1:1` / `3:4` / `9:16` / `21:9` / `adaptive`。`adaptive`：文生/参考生按 prompt 在可选比例中自动选择；延长保持待处理视频比例；首帧/首尾帧保持首帧图比例。视频延长、首帧/首尾帧任务仅支持 `adaptive`，不可指定具体宽高比。模型专属/上游扩展。示例：`16:9` |
| `seed` | string | 否 | 随机种子；默认 `-1`。用于控制生成随机性；相同 seed 有助于结果复现，但不保证完全一致。模型专属/上游扩展。示例：`-1` |
| `watermark` | string | 否 | 视频水印；默认 `false`。`true`：右下角添加「AI生成」水印；`false`：不添加水印。模型专属/上游扩展。示例：`false` |

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
| 任务类型判定 | 上游根据 `content.role` 与提示词意图判定为：文生视频、参考生视频、视频延长、首帧/首尾帧等。延长触发词示例：向前/向后延长、延续、续写。普通参考生请避免延长相关词，防止误判。视频编辑暂未对外开放，请勿使用编辑类意图词（编辑视频、增加/删除/修改/替换等）。 |
| 保存时间 | 任务记录保存 7 天；视频 URL 保存 24 小时，请及时下载或转存。 |
| 分辨率 | 仅 `480p` / `720p`，不支持 `1080p` / `4k`。 |
| 参考素材上限 | 单次最多 50 个：图片≤30、视频≤10、音频≤10；参考视频/音频各自总时长不超过 30s。 |
| 定价 | 平台定价待后台配置完整；当前估价矩阵时长/分辨率枚举可能未覆盖全部官方合法值，正式扣费前请确认定价规则。 |
| 宽高比硬约束 | 延长 / 首帧/首尾帧：`ratio` 仅 `adaptive`。 |
| 平台到上游字段映射 | 本页按火山官方多模态参数形态展示；`content`、`duration`、`resolution`、`ratio`、`generate_audio`、`watermark`、`output_format`、`seed` 均为官方字段，平台原样透传上游。接口为 `POST /v1/media/generations`（国内），不是 `/v1/ark/media/generations`。 |
| 时长 | 上游支持 4~30 秒或 `-1`；相比 Seedance 2.0 系列（4~15）上限提升至 30 秒。平台当前估价时长枚举有限，未覆盖秒数或 `-1` 可能估价失败。 |
| 国内官key素材库 | 素材 Active 后，可在 `content` 中使用 `asset://{素材 ID}`；可与 fast/mini 复用同一平台素材 ID。 |
| 纯音频参考 | 2.5 新增支持仅传 `reference_audio` 生成视频，无需搭配图片或视频。 |
| 视频延长 | `role=reference_*` 且提示词含延长/续写意图时触发；`ratio` 仅 `adaptive`；`duration` 可为 `[4,30]` 或 `-1`；推荐 `output_format=mov`。 |
| 轮询 | 提交后使用 `GET /v1/media/tasks/:task_id` 查询结果。 |
| 非法参数 | 延长/首尾帧任务下 `ratio`、`duration` 不合法时，上游可能异步返回 `InvalidParameter.TaskTypeConstraint`（需等任务启动后才返回）；多参数违规会一次性列出。 |
| 首帧/首尾帧 | `role=first_frame[/last_frame]`；`ratio` 仅 `adaptive`；`duration` 可为 `[4,30]` 或 `-1`；与多模态参考场景互斥。 |

## 素材库

### 推荐流程

创建素材组 → 上传素材 → 等待 Active → 引用素材生成视频

1. 使用本页国内官key素材库创建素材组，并上传图片、视频或音频素材。
2. 查询素材状态；仅 `Active` 状态的素材可以用于视频生成。
3. 切换到模型 API，在 `content[].image_url.url` / `video_url.url` / `audio_url.url` 中传入 `asset://{素材 ID}`。

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

## 用于 fast / mini / 2.5 视频

素材达到 `Active` 后使用 `asset://asset-volc-cn-...`。平台会固定到素材创建时的同一 channel key；同一请求混用不同火山账号素材返回 409，不回退其他 key。

```bash
curl -X POST 'https://www.moxing.pro/v1/media/generations' \
  -H 'Authorization: Bearer sk-xxxx' \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "doubao-seedance-2-5-260628",
    "content": [
      { "type": "text", "text": "图片1中的人物在海边自然行走，保持主体一致" },
      {
        "type": "image_url",
        "role": "reference_image",
        "image_url": { "url": "asset://asset-volc-cn-平台素材ID" }
      }
    ],
    "duration": 10,
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
