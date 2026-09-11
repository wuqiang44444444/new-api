---
status: current
owner: Dev Team
last-reviewed: 2026-09-11
---

# 墨行 Seedance 2.0 API 来源记录

## 当前模型与来源

- 当前目标模型 ID：`doubao-seedance-2-0-260128-0818`。
- [模型 API 原页](https://www.moxing.pro/docs/models/doubao-seedance-2-0-260128-0818)。
- [素材库原页](https://www.moxing.pro/docs/models/doubao-seedance-2-0-260128-0818?tab=assets)。
- 核对日期：2026-09-11；已读取模型 API、音频参考示例和素材库页。
- 以下记录供应商字段与接口事实；“平台”在供应商内容中指墨行，不是本站。
  供应商接口清单不代表本站已经发布全部操作。

## 路径矛盾与用户确认

网页能力说明写 `POST /v1/ark/media/generations` 和 `GET /v1/ark/media/tasks/:task_id`，
但同页接口标题、异步查询模块和 cURL 示例写 `POST /v1/media/generations` 与
`GET /v1/media/tasks/:id`。页面同时将无 `-0818` 的旧 ID 描述为历史协议模型。

2026-09-11 用户明确确认：创建接口依然为 `POST /v1/media/generations`。
本站据此继续统一 Media 创建协议及现有 `GET /v1/media/tasks/{task_id}` 查询路径；
不新增 Ark 入口、不自动尝试另一条路径。页面矛盾保留作为外部验收事项。

## 模型 API 字段记录

| 参数 | 页面类型 | 必填 | 页面信息 |
| --- | --- | --- | --- |
| `model` | string | 是 | `doubao-seedance-2-0-260128-0818` |
| `content` | object[] | 是 | 官方多模态数组；文本、图片、视频、音频分别为 `text`、`image_url`、`video_url`、`audio_url` |
| `duration` | integer | 否 | 页面列出 4–15 秒，示例为 5 秒；本页未明确说明 `-1` |
| `resolution` | string | 否 | 页面列出 `480p`、`720p`、`1080p`、`4k`，示例为 `720p` |
| `ratio` | string | 否 | 示例 `16:9`；首尾帧、编辑与延长场景可用 `adaptive` |
| `generate_audio` | string | 否 | 页面声明默认 true；实际 JSON 示例使用布尔值 |
| `omni_reference_task_type` | string | 否 | 编辑 `editing`、延长 `extension`，其它可省略或用 `auto`；本站未发布该私有字段 |
| `output_format` | string | 否 | 页面明确说明本路由不支持该字段，调用时不要传入 |
| `watermark` | 示例为 boolean | — | 请求参数表未单列，文本与音频示例均发送布尔 false |

媒体 URL 使用嵌套对象 `{"url":"..."}`；role 包括 `first_frame`、`last_frame`、
`reference_image`、`reference_video`、`reference_audio`。
页面提供文生、首帧、首尾帧、多模态参考、编辑、延长、音频参考七个场景。
音频参考示例使用文本＋图片＋音频，但本页没有旧 ID 文档中的“不能只传音频”规则；
示例组合不能推导为强制搭配限制。

页面通用提示把部分官方字段描述为扩展字段，并提及 `extra`；具体示例却把 `content`、
`generate_audio`、`ratio` 与 `watermark` 放在顶层。本站北向类型化合同不因此开放任意 `extra`。

### 最小请求（按页面字段整理）

```http
POST /v1/media/generations
Authorization: Bearer <API_KEY>
Content-Type: application/json
```

```json
{
  "model": "doubao-seedance-2-0-260128-0818",
  "content": [{"type": "text", "text": "海边日落"}],
  "duration": 5,
  "resolution": "720p",
  "ratio": "16:9",
  "generate_audio": true,
  "watermark": false
}
```

音频参考使用以下数组结构，页面对应示例时长为 10 秒：

```json
[
  {"type": "text", "text": "按参考音频节奏生成产品视频"},
  {"type": "image_url", "role": "reference_image", "image_url": {"url": "https://example.com/product.jpg"}},
  {"type": "audio_url", "role": "reference_audio", "audio_url": {"url": "https://example.com/music.mp3"}}
]
```

### 异步响应

| 字段 | 页面语义 |
| --- | --- |
| `task_id` | 创建后保存，用于单任务查询 |
| `status` | `queued` / `running` / `succeeded` / `failed` |
| `status_code` | 等待或运行通常为 202，成功为 200，失败为 400 |
| `created_at` / `updated_at` | Unix 时间戳；部分表格类型标为 string，示例是数字 |
| `progress` | 0–100 进度 |
| `result` | 成功结果对象，可含 `primary_url`、`urls`、水印地址、封面；部分表格类型标为 string |
| `error_message` | 失败原因 |
| `usage` | 用量（如有）；本页没有足以确定完整账单计量的合同 |

查询继续使用 `GET /v1/media/tasks/{task_id}`。创建受理不等于生成成功；
实际字段效果、时长、用量与素材复用仍需准确模型的真实终态验收。

## 国内素材库

新模型素材库页使用 `/v1/volc/assets`，明确列出 0818、Fast、Mini、2.5 的素材复用范围。
以下国内素材接口记录与该页逐项核对一致；保留供应商的组列表、删除等接口用于查阅，
本站只开放自身北向合同允许的单资源操作。

## 推荐流程

创建素材组 → 上传素材 → 等待 Active → 引用素材生成视频

1. 使用本页国内官key素材库创建素材组，并上传图片、视频或音频素材。
2. 查询素材状态；仅 `Active` 状态的素材可以用于视频生成。
3. 切换到模型 API，在 `content[].image_url.url` / `video_url.url` / `audio_url.url` 中传入 `asset://{素材 ID}`。

## 国内官key素材库

本接口封装火山方舟国内私域素材库。客户端只使用平台 Bearer `sk-...`，不会看到火山 AK/SK、Action URL 或上游素材 ID。

素材接口不传 `model`。平台素材 ID 可在 `doubao-seedance-2-0-260128-0818`、`doubao-seedance-2-0-fast-260128`、`doubao-seedance-2-0-mini-260615` 与 `doubao-seedance-2-5-260628` 之间复用。

`ProjectName` 由服务端 `VOLC_CN_PROJECT_NAME` 配置；未配置时使用 `default`，客户端无需传入。

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

### 官方字段

#### 创建素材组

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `Name` | string | 是 | 名称，最多 64 字符。 |
| `Description` | string | 否 | 描述，最多 300 字符。 |
| `GroupType` | string | 否 | API 直建当前使用 `AIGC`；真人分组通过 H5 认证生成。 |

```bash
curl -X POST 'https://www.moxing.pro/v1/volc/assets/groups' \
  -H 'Authorization: Bearer sk-xxxx' \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: group-20260723-001' \
  -d '{
    "Name": "品牌虚拟人像",
    "Description": "广告视频参考人物",
    "GroupType": "AIGC"
  }'
```

#### 创建素材

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `GroupId` | string | 是 | 平台素材组 ID。 |
| `URL` | string | 是 | 公网 URL；官方不支持 Base64。 |
| `Name` | string | 否 | 名称，最多 64 字符。 |
| `AssetType` | string | 是 | `Image` / `Video` / `Audio`。 |

```bash
curl -X POST 'https://www.moxing.pro/v1/volc/assets' \
  -H 'Authorization: Bearer sk-xxxx' \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: asset-20260723-001' \
  -d '{
    "GroupId": "group-volc-cn-平台分组ID",
    "URL": "https://example.com/portrait.png",
    "Name": "人物正面图",
    "AssetType": "Image"
  }'
```

### 素材组：列表、查询、更新与删除

`POST /groups/list` 使用官方 `Filter.GroupIds` / `GroupType` / `Name`，以及 `PageNumber` / `PageSize` / `SortBy` / `SortOrder`。

单项查询通过路径传平台 ID。更新组支持 `Name` / `Description`。

#### 列出素材组

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
    "SortOrder": "Desc"
  }'
```

#### 查询单个素材组

```bash
curl 'https://www.moxing.pro/v1/volc/assets/groups/group-volc-cn-平台分组ID' \
  -H 'Authorization: Bearer sk-xxxx'
```

#### 更新素材组

```bash
curl -X POST 'https://www.moxing.pro/v1/volc/assets/groups/group-volc-cn-平台分组ID/update' \
  -H 'Authorization: Bearer sk-xxxx' \
  -H 'Content-Type: application/json' \
  -d '{
    "Name": "更新后的分组名称",
    "Description": "更新后的描述"
  }'
```

#### 删除素材组

```bash
curl -X POST 'https://www.moxing.pro/v1/volc/assets/groups/group-volc-cn-平台分组ID/delete' \
  -H 'Authorization: Bearer sk-xxxx' \
  -H 'Content-Type: application/json' \
  -d '{}'
```

### 素材：列表、查询、更新与删除

`POST /list` 支持官方 `Filter.GroupIds` / `Statuses`（`Active` / `Processing` / `Failed`）/ `Name`，以及 `PageNumber` / `PageSize` / `SortBy` / `SortOrder`。

单项查询通过路径传平台 ID。更新素材支持 `Name`。

#### 列出素材

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
    "SortOrder": "Desc"
  }'
```

#### 查询单个素材

```bash
curl 'https://www.moxing.pro/v1/volc/assets/asset-volc-cn-平台素材ID' \
  -H 'Authorization: Bearer sk-xxxx'
```

#### 更新素材

```bash
curl -X POST 'https://www.moxing.pro/v1/volc/assets/asset-volc-cn-平台素材ID/update' \
  -H 'Authorization: Bearer sk-xxxx' \
  -H 'Content-Type: application/json' \
  -d '{"Name": "更新后的素材名称"}'
```

#### 删除素材

```bash
curl -X POST 'https://www.moxing.pro/v1/volc/assets/asset-volc-cn-平台素材ID/delete' \
  -H 'Authorization: Bearer sk-xxxx' \
  -H 'Content-Type: application/json' \
  -d '{}'
```

#### 素材状态

| 状态 | 说明 |
| --- | --- |
| `Processing` | 异步处理中，不能用于视频生成。 |
| `Active` | 处理完成，可使用 `asset://` 引用。 |
| `Failed` | 处理失败，查看 `Error.Code` / `Error.Message`。 |

### 真人人像认证

真人素材组不能通过 `CreateAssetGroup` 直接创建，需要先完成 H5 人脸活体认证。流程如下：

1. 创建认证会话，打开响应 `Result.H5Link` 让被授权人完成认证。
2. 轮询 `GET /visual-validate/sessions/:session_id`，等待状态变为 `callback_received`。
3. 调用 `GET /visual-validate/results/:session_id`，获得真人 `GroupId`（类型为 `LivenessFace`）。
4. 使用普通创建素材接口，为该真人分组补充图片、视频或音频；火山会执行人脸一致性校验。

真实 `BytedToken` 不对外暴露，平台用会话 ID 替代。服务端生成并校验官方回调，运维需配置公网 HTTPS 的 `VOLC_CN_ASSET_CALLBACK_BASE_URL`。客户端传入的 `CallbackURL` 会被平台安全覆盖。

#### 步骤 1：创建认证会话

```bash
curl -X POST 'https://www.moxing.pro/v1/volc/assets/visual-validate/sessions' \
  -H 'Authorization: Bearer sk-xxxx' \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: visual-20260723-001' \
  -d '{}'
```

响应 `Result` 包含 `SessionId`（平台会话 ID）、`H5Link`（认证页面地址）。原始 `BytedToken` 已被替换，不对外暴露。

#### 步骤 2：查询认证会话状态

```bash
curl 'https://www.moxing.pro/v1/volc/assets/visual-validate/sessions/session-volc-cn-平台会话ID' \
  -H 'Authorization: Bearer sk-xxxx'
```

响应 `Result` 包含 `SessionId`、`Status`、`ProjectName`、`CreateTime`、`UpdateTime`。建议每 3–5 秒轮询一次。

| 会话状态 | 说明 |
| --- | --- |
| `pending` | 等待 H5 认证，用户尚未完成。 |
| `callback_received` | 收到官方成功回调，可查询结果。 |
| `group_ready` | 已获得真人素材组，可直接使用 `GroupId`。 |
| `failed` | 认证失败，需重新创建会话。 |

#### 步骤 3：获取真人素材组

```bash
curl 'https://www.moxing.pro/v1/volc/assets/visual-validate/results/session-volc-cn-平台会话ID' \
  -H 'Authorization: Bearer sk-xxxx'
```

响应 `Result.GroupId` 为平台真人分组 ID（`group-volc-cn-...`），类型为 `LivenessFace`。平台内部使用真实 `BytedToken` 调用官方接口，并自动创建分组映射。

#### 步骤 4：为真人分组上传素材

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
    "AssetType": "Image"
  }'
```

### 用于 fast / mini 视频

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


## 历史原文：无 0818 的旧模型（2026-09-10）

以下保留旧模型页面证据，供旧分析与调用记录追溯；不作为当前 0818 模型的请求或素材合同。

# doubao-seedance-2.0

豆包大模型团队推出的新一代专业级多模态创作视频模型 Seedance 2.0，支持图像、视频、音频等多模态作为参考输入生成视频，还具备视频编辑、延长等能力，能高精度还原各类细节并稳定角色特征，具备极致拟真的视听稳定性，深度适配商业广告、影视制作与社交媒体营销等各大核心场景。

- **模型 ID**: `doubao-seedance-2-0-260128`
- **提供商**: 豆包
- **类型**: 视频
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
| `model` | string | 是 | 模型 ID；要调用的视频模型 logical key |
| `prompt` | string | 是 | 提示词；描述想要生成的视频内容，最长 2500 字符。参考生视频场景中用于描述基于参考素材生成的目标效果。示例：图片1中的人物站在海边，夕阳下回眸一笑 |
| `resolution` | string | 否 | 分辨率；支持 `480p` / `720p`。示例：`480p` |
| `with_audio` | boolean | 否 | 同步生成音频；`true` 时同步生成音频；`false` 时不生成音频。示例：`true` |
| `watermark` | string | 否 | 水印；上游官方字段；`true` 含水印，`false` 无水印。平台原样透传，缺省时以上游默认为准。示例：`false` |

> 说明：`watermark` 为模型专属/上游扩展字段，不属于平台公共基础参数；需放入 `extra` 或对应平台兼容字段后，由平台透传或映射到上游。

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

创建入口：`https://www.moxing.pro/v1/media/generations`

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

### 常见错误

调用失败时，可根据 HTTP 状态和错误码快速定位问题。

| 错误码 | HTTP | 说明 |
| --- | --- | --- |
| `invalid_request_error` | 400 | 请求参数缺失、格式错误或字段值不在允许范围内 |
| `invalid_api_key` | 401 | API Key 为空、格式错误或已失效；请确认 `Authorization: Bearer sk-...` |
| `insufficient_quota` | 403 | 账户余额或配额不足；请充值或申请扩容 |
| `model_not_found` | 404 | 请求的 `model` 不存在或当前账户无权访问 |
| `rate_limit_exceeded` | 429 | 触发并发或 RPM 限流；建议指数退避后重试 |
| `internal_server_error` | 500 | 平台内部异常；可重试，持续异常请联系技术支持 |
| `upstream_unavailable` | 502 | 模型服务暂时不可用或超时；建议稍后重试 |

### 使用限制

提交任务前请确认模型支持的输入范围与组合规则。

| 限制项 | 说明 |
| --- | --- |
| 分辨率 | `480p` / `720p` |
| 参考素材 | 普通资产参考可传公网 URL 或 `asset://upstream_id`；人像库参考传 `asset://upstream_id`。 |
| 平台到上游字段映射 | 平台字段与火山方舟 Seedance 2.0 官方字段基本一致，`duration_seconds`、`aspect_ratio`、`with_audio`、`watermark`、`image`、`end_image`、`reference_images`、`reference_videos`、`reference_audios` 均按官方接口透传；`input_mode`、`control_mode` 为平台侧场景编排字段，不直接传给上游。 |
| 画幅比例 | `16:9` / `4:3` / `1:1` / `3:4` / `9:16` / `21:9` / `adaptive` |
| 视频时长 | 4-15 秒，或 `-1` 自动 |

## 推荐流程

创建素材组 → 上传素材 → 等待 Active → 引用素材生成视频

1. 使用本页素材库创建素材组并上传素材，记录创建成功后的素材 ID。
2. 查询素材状态；仅 `Active` 状态的素材可以用于视频生成。
3. 切换到模型 API，在 `reference_images[]` 中传入 `asset://{素材 ID}`。

## 接口范围

接口根路径：`https://www.moxing.pro/joycreator/openApi/v1/asset`

素材库接口独立于统一 `/v1/*` 网关路径，当前用于 JoyCreator TOB 素材组与素材管理。

鉴权方式仍为平台 API Key：`Authorization: Bearer sk-...`。

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

- 接口返回中的 `id` 为后续调用用的主键，不是本地数据库自增 id。
- 素材组详情返回的 `groupId` 与素材详情返回的 `assetId` 为上游业务标识，可用于排障和对账。
- 创建素材后通常需要轮询 `/asset/detail/:id`，直到 `status=1` 且拿到 `vendorUrl`。
- 创建图片素材时，URL 支持 HTTP/HTTPS 链接或 base64 图片；base64 会先转存为资源 URL 后再提交上游，素材记录不保存原始 base64。

### 请求参数

#### 创建素材组

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `Name` | string | 是 | 素材组名称，最长 64 字符。 |
| `Description` | string | 否 | 素材组描述，最长 300 字符。 |
| `GroupType` | string | 否 | 当前仅支持 `AIGC`。 |

#### 创建素材

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `groupId` | string / number | 是 | 使用创建素材组接口返回的 `id`。 |
| `URL` | string | 是 | 素材地址。`AssetType=Image` 时支持 HTTP(S) URL、`data:image/...;base64,...` 或裸 base64；Video/Audio 仍需 HTTP(S) URL。 |
| `AssetType` | string | 是 | 支持 `Image` / `Video` / `Audio`。 |
| `Name` | string | 否 | 素材名称，最长 64 字符。 |

#### 更新素材组

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `Name` / `Description` | string | 二选一 | 至少传一个字段。 |

#### 更新素材

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `Name` | string | 是 | 更新后的素材名称。 |

### 调用示例

#### 创建素材组

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

#### 创建素材

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

#### 查询素材详情

```bash
curl -X POST 'https://www.moxing.pro/joycreator/openApi/v1/asset/detail/52' \
  -H 'Authorization: Bearer sk-xxxx'
```

#### 素材组详情响应

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

图生视频

请求参数
参数	类型	必填	说明
aspect_ratio	string	否	画幅比例；支持 16:9 / 4:3 / 1:1 / 3:4 / 9:16 / 21:9 / adaptive。；示例：16:9
capability	string	是	能力类型；固定为 video_generation
control_mode	string	是	控制模式；文生视频固定 none；图生视频可用 none 或 end_frame；参考生视频固定 reference。；示例：none
duration_seconds	integer	否	视频时长（秒）；支持 4-15 秒，或 -1 表示自动。；示例：4
end_image	string	否	尾帧图片；图生视频可选。传入后用于首尾帧生成。支持 HTTP(S) URL、data:image/...;base64,... 或裸 base64；平台会在创建任务时先转存 base64，再以 URL 进入后续任务链路。；示例：https://example.com/last-frame.png
image	string	是	首帧图片；图生视频必传。支持公网可访问图片 URL。支持 HTTP(S) URL、data:image/...;base64,... 或裸 base64；平台会在创建任务时先转存 base64，再以 URL 进入后续任务链路。；示例：https://example.com/first-frame.png
input_mode	string	是	输入模式；文生视频使用 text；图生视频使用 single_image；参考生视频使用 multi_image。；示例：text
model	string	是	模型 ID；要调用的视频模型 logical key
prompt	string	是	提示词；描述想要生成的视频内容，最长 2500 字符。参考生视频场景中用于描述基于参考素材生成的目标效果。；示例：图片1中的人物站在海边，夕阳下回眸一笑
resolution	string	否	分辨率；支持 480p / 720p。；示例：480p
with_audio	boolean	否	同步生成音频；true 时同步生成音频；false 时不生成音频。；示例：true
watermark	string	否	水印；上游官方字段；true 含水印，false 无水印。平台原样透传，缺省时以上游默认为准。；模型专属/上游扩展：不属于平台公共基础参数；按本行说明放入 extra 或对应平台兼容字段后，平台会透传或映射到上游。；示例：false
响应格式
字段	类型	说明
created_at	string	任务创建时间（Unix 时间戳）
error_message	string	终态失败时的错误描述
progress	string	0-100 之间的整数，表示生成进度
result	string	终态成功时返回视频/资源信息，包含 url、duration_seconds 等字段
status	string	queued / running / succeeded / failed
task_id	string	任务唯一 ID，用于轮询 GET /v1/media/tasks/:id
usage	string	本次任务的额度消耗（如有）
异步查询生成结果
GET /v1/media/tasks/:id
创建任务后先保留 task_id，再轮询这个接口直到 succeeded 或 failed。 图片和视频模型都适用。

创建入口：https://www.moxing.pro/v1/media/generations

字段	类型	说明
task_id	string	任务唯一标识。
status	string	queued / running / succeeded / failed。
status_code	integer	queued / running 通常为 202，succeeded 为 200，failed 为 400。
result	object	视频任务成功后，result 通常包含主视频地址、带水印地址和封面图。
error_message	string	失败时返回，说明失败原因。
建议保留退避轮询，不要把这个接口当同步请求反复高频调用。

text
复制
curl -sS "https://www.moxing.pro/v1/media/tasks/:id" \
  -H "Authorization: Bearer sk-xxxx"
响应示例：

json
复制
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
错误与限制
常见错误
调用失败时，可根据 HTTP 状态和错误码快速定位问题。

查看完整错误码
错误码	HTTP	说明
invalid_request_error	400	请求参数缺失、格式错误或字段值不在允许范围内
invalid_api_key	401	API Key 为空、格式错误或已失效；请确认 Authorization: Bearer sk-...
insufficient_quota	403	账户余额或配额不足；请充值或申请扩容
model_not_found	404	请求的 model 不存在或当前账户无权访问
rate_limit_exceeded	429	触发并发或 RPM 限流；建议指数退避后重试
internal_server_error	500	平台内部异常；可重试，持续异常请联系技术支持
upstream_unavailable	502	模型服务暂时不可用或超时；建议稍后重试
使用限制
提交任务前请确认模型支持的输入范围与组合规则。

分辨率
480p / 720p
参考素材
普通资产参考可传公网 URL 或 asset://upstream_id；人像库参考传 asset://upstream_id。
平台到上游字段映射
平台字段与火山方舟 Seedance 2.0 官方字段基本一致，duration_seconds、aspect_ratio、with_audio、watermark、image、end_image、reference_images、reference_videos、reference_audios 均按官方接口透传；input_mode、control_mode 为平台侧场景编排字段，不直接传给上游。
画幅比例
16:9 / 4:3 / 1:1 / 3:4 / 9:16 / 21:9 / adaptive
视频时长
4-15 秒，或 -1 自动

参考生视频
请求参数
参数	类型	必填	说明
aspect_ratio	string	否	画幅比例；支持 16:9 / 4:3 / 1:1 / 3:4 / 9:16 / 21:9 / adaptive。；示例：16:9
capability	string	是	能力类型；固定为 video_generation
control_mode	string	是	控制模式；文生视频固定 none；图生视频可用 none 或 end_frame；参考生视频固定 reference。；示例：none
duration_seconds	integer	否	视频时长（秒）；支持 4-15 秒，或 -1 表示自动。；示例：4
input_mode	string	是	输入模式；文生视频使用 text；图生视频使用 single_image；参考生视频使用 multi_image。；示例：text
model	string	是	模型 ID；要调用的视频模型 logical key
prompt	string	是	提示词；描述想要生成的视频内容，最长 2500 字符。参考生视频场景中用于描述基于参考素材生成的目标效果。；示例：图片1中的人物站在海边，夕阳下回眸一笑
reference_audios	array	否	参考音频列表；参考生视频可选。不能只传音频，至少同时提供参考图片或参考视频。；示例：["https://example.com/reference-audio.wav"]
reference_images	array	是	参考图片列表；参考生视频必传。普通资产可传公网 URL 或 asset://upstream_id；人像库参考传 asset://upstream_id。；示例：["asset://asset-20260519163832-jhsv9","https://upload.nextself.top/api/files/170764587@qq.com/202605/scene.png"]
reference_videos	array	否	参考视频列表；参考生视频可选。用于参考运动、节奏或镜头风格。；示例：["https://example.com/reference-clip.mp4"]
resolution	string	否	分辨率；支持 480p / 720p。；示例：480p
with_audio	boolean	否	同步生成音频；true 时同步生成音频；false 时不生成音频。；示例：true
watermark	string	否	水印；上游官方字段；true 含水印，false 无水印。平台原样透传，缺省时以上游默认为准。；模型专属/上游扩展：不属于平台公共基础参数；按本行说明放入 extra 或对应平台兼容字段后，平台会透传或映射到上游。；示例：false
响应格式
字段	类型	说明
created_at	string	任务创建时间（Unix 时间戳）
error_message	string	终态失败时的错误描述
progress	string	0-100 之间的整数，表示生成进度
result	string	终态成功时返回视频/资源信息，包含 url、duration_seconds 等字段
status	string	queued / running / succeeded / failed
task_id	string	任务唯一 ID，用于轮询 GET /v1/media/tasks/:id
usage	string	本次任务的额度消耗（如有）
异步查询生成结果
GET /v1/media/tasks/:id
创建任务后先保留 task_id，再轮询这个接口直到 succeeded 或 failed。 图片和视频模型都适用。

创建入口：https://www.moxing.pro/v1/media/generations

字段	类型	说明
task_id	string	任务唯一标识。
status	string	queued / running / succeeded / failed。
status_code	integer	queued / running 通常为 202，succeeded 为 200，failed 为 400。
result	object	视频任务成功后，result 通常包含主视频地址、带水印地址和封面图。
error_message	string	失败时返回，说明失败原因。
建议保留退避轮询，不要把这个接口当同步请求反复高频调用。

text
复制
curl -sS "https://www.moxing.pro/v1/media/tasks/:id" \
  -H "Authorization: Bearer sk-xxxx"
响应示例：

json
复制
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
错误与限制
常见错误
调用失败时，可根据 HTTP 状态和错误码快速定位问题。

查看完整错误码
错误码	HTTP	说明
invalid_request_error	400	请求参数缺失、格式错误或字段值不在允许范围内
invalid_api_key	401	API Key 为空、格式错误或已失效；请确认 Authorization: Bearer sk-...
insufficient_quota	403	账户余额或配额不足；请充值或申请扩容
model_not_found	404	请求的 model 不存在或当前账户无权访问
rate_limit_exceeded	429	触发并发或 RPM 限流；建议指数退避后重试
internal_server_error	500	平台内部异常；可重试，持续异常请联系技术支持
upstream_unavailable	502	模型服务暂时不可用或超时；建议稍后重试
使用限制
提交任务前请确认模型支持的输入范围与组合规则。

分辨率
480p / 720p
参考素材
普通资产参考可传公网 URL 或 asset://upstream_id；人像库参考传 asset://upstream_id。
平台到上游字段映射
平台字段与火山方舟 Seedance 2.0 官方字段基本一致，duration_seconds、aspect_ratio、with_audio、watermark、image、end_image、reference_images、reference_videos、reference_audios 均按官方接口透传；input_mode、control_mode 为平台侧场景编排字段，不直接传给上游。
画幅比例
16:9 / 4:3 / 1:1 / 3:4 / 9:16 / 21:9 / adaptive
视频时长
4-15 秒，或 -1 自动