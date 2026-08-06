---
status: current
owner: Dev Team
last-reviewed: 2026-08-05
---

# 视频模型 API 调用指南

## 1. 适用范围

本文面向使用 new-api 的开发者，介绍 rc23 原生 OpenAI Videos，以及 Seedance 模型通过 ModelArk v3 Link 合同执行任务创建、状态查询和结果下载的方式。两类合同共享任务底座，但路径、字段、模型和响应不能混用。

当前 ModelArk v3 合同定义以下 Seedance 视频模型；实际可用性必须以 API Key 的模型列表和
publication 为准：

| 模型 ID | 说明 |
| --- | --- |
| `seedance-byteplus` | Seedance BytePlus SKU |
| `seedance-2-0-oversea` | 墨行 Seedance 2.0 海外版；按量计费 |
| `doubao-seedance-2-0-260128` | 墨行 doubao Seedance 2.0；按秒计费 |
| `seedance-2.0-standard` | Seedance 2.0 标准可变分辨率 SKU；支持普通 Link 资源，不支持真人素材 |
| `seedance-2.0-fast` | Seedance 2.0 Fast 可变分辨率 SKU；仅支持 480p/720p |
| `seedance-2.0-mini-720p` | 飞彩 Mini 720p 固定档位候选；是否可用以 capability 接口为准 |
| `seedance-2.0-sd2-720p` | 飞彩 SD2 720p 固定档位候选；是否可用以 capability 接口为准 |
| `seedance-2.0-fast-720p` | 飞彩 Fast 720p 固定档位候选；是否可用以 capability 接口为准 |
| `seedance-2.0-value-720p` | 飞彩 Value 720p 固定档位候选；是否可用以 capability 接口为准 |
| `seedance-2.0-standard-720p` | 飞彩 Standard 720p 固定档位候选；是否可用以 capability 接口为准 |
| `seedance-2.0-value-1080p` | 飞彩 Value 1080p 固定档位候选；是否可用以 capability 接口为准 |
| `seedance-2.0-standard-1080p` | 飞彩 Standard 1080p 固定档位候选；是否可用以 capability 接口为准 |
| `seedance-2.0-value-4k` | 飞彩 Value 4K 固定档位候选；是否可用以 capability 接口为准 |
| `seedance-2.0-standard-4k` | 飞彩 Standard 4K 固定档位候选；是否可用以 capability 接口为准 |
| `seedance-2.0-pro-pi-720p` | 飞彩 Pro PI 720p 固定档位候选；是否可用以 capability 接口为准 |

上述 Seedance SKU 使用相同的 ModelArk v3 客户端 API。调用方只需更换 `model`，不需要了解或切换上游地址、鉴权方式、任务路径和响应格式。

其中墨行产品范围固定为上表中的 `seedance-2-0-oversea` 与 `doubao-seedance-2-0-260128` 两个模型；
其它 Seedance SKU 属于各自独立 Provider 线路，不得被视为墨行模型或作为这两个模型的降级候选。

渠道可以使用官方实现或经过 capability 等价校验的第三方实现，但这不会改变 Link 合同。FunCloud 仅承接独立的 `seedance-2.0-standard` 与 `seedance-2.0-fast`；其路径、业务 code、Bearer Key 与上游素材 ID 均不会向客户端公开。现有官方、TokenSave 和飞彩 SKU 不会加入 FunCloud profile，也不会因 FunCloud 报价改变定价。

模型可用范围可能随账户权限调整。正式调用前应通过模型列表或控制台确认当前 API Key 可以使用的
模型。`sora-2`、`sora-2-pro` 等 NEWAPI 原生模型使用 `/v1/videos`；上表 Seedance Link SKU 的
公开合同只承诺 `/api/v3/contents/generations/tasks`。本文不定义原生入口如何处理合同外模型，
客户端不得依赖本地 `link_sku_contract_mismatch` 兼容行为。

`GET /v1/models` 只列出当前 Key 实际可调用的客户模型。查询全部已登记 Seedance 基础候选、当前 Key
已发布的客户模型 alias 及模型级参数支持时，使用 `GET /api/v3/contents/generations/models`。调用方可用
`/v1/models` 返回的同一模型 ID 精确查询 capability；alias 行不会暴露内部 Link SKU。候选模型可以出现
在 capability 接口中，但只有 `published=true`、`available=true` 且同时满足
`visible_in_v1_models=true` 时才允许创建任务。

Kling 和即梦使用各自独立的 Link 合同中的官方协议，不能使用本文的 ModelArk 路径、字段、状态或错误
信封。对应调用方式见 [Kling 视频 API Reference](../../web/public/docs-content/zh/api-reference/videos/kling.md)
和[即梦视频 API Reference](../../web/public/docs-content/zh/api-reference/videos/jimeng.md)。

### 1.1 rc23 原生 OpenAI Videos

原生合同提供：

```text
POST /v1/videos
POST /v1/videos/{video_id}/remix
GET  /v1/videos/{task_id}
GET  /v1/videos/{task_id}/content
POST /v1/video/generations
GET  /v1/video/generations/{task_id}
```

推荐使用 `/v1/videos`。JSON 创建示例：

```bash
curl -sS -X POST "$TokenAI_API_BASE/v1/videos" \
  -H "Authorization: Bearer $TokenAI_API_KEY" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: sora-order-20260804-001" \
  -d '{
    "model": "sora-2",
    "prompt": "清晨的海岸线，固定镜头，电影感",
    "seconds": "8",
    "size": "1280x720"
  }'
```

`model` 省略时默认 `sora-2`；`seconds` 支持 `4`、`8`、`12`。`input_reference` 可按 rc23
合同使用 multipart 文件，或在 JSON 中使用包含 `file_id` / `image_url` 的对象；`image_url` 支持
HTTP/HTTPS、Data URL，以及经过平台虚拟素材授权与渠道绑定检查的 `asset://ast_xxx`。

Remix 请求只提交新的 `prompt`，源视频必须属于同一 API Key 应用、由 OpenAI Videos 合同创建，
且上游 adapter 支持 Remix：

```bash
curl -sS -X POST "$TokenAI_API_BASE/v1/videos/task_xxx/remix" \
  -H "Authorization: Bearer $TokenAI_API_KEY" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: sora-remix-20260804-001" \
  -d '{"prompt":"保持构图，将天气改为雪天"}'
```

创建成功返回 OpenAI Videos 任务对象。使用 `GET /v1/videos/{task_id}` 轮询，成功后通过
`GET /v1/videos/{task_id}/content` 下载。发送到 Provider 后结果未知时，平台返回
`create_outcome_unknown`，客户端不得自行换 Key 或重复创建，应保留 `request_id` 等待对账。

## 2. 调用准备

准备以下信息：

1. 从控制台创建 API Key；
2. 复制当前站点的 API Base URL；
3. 确认账户余额和模型权限。

示例使用环境变量，避免把密钥写进代码：

```bash
export TokenAI_API_BASE="https://<your-api-host>"
export TokenAI_API_KEY="sk-your-key"
```

本文的 `TokenAI_API_BASE` 是站点根地址，不包含末尾的 `/v1`。视频创建接口会在其后追加 `/api/v3/...`。

所有请求均使用 Bearer Token：

```http
Authorization: Bearer sk-your-key
```

API Key 只能保存在服务端或安全的密钥管理系统中，不得提交到代码仓库、前端代码、移动端安装包或日志。

## 3. 查询可用模型

```bash
curl -sS "$TokenAI_API_BASE/v1/models" \
  -H "Authorization: Bearer $TokenAI_API_KEY"
```

只查看 Seedance 模型：

```bash
curl -sS "$TokenAI_API_BASE/v1/models" \
  -H "Authorization: Bearer $TokenAI_API_KEY" \
  | jq -r '.data[].id | select(contains("seedance"))'
```

不要根据本文中的固定清单跳过模型发现；应以当前 API Key 实际返回的模型为准。

## 4. 创建视频任务

### 4.1 接口

```http
POST /api/v3/contents/generations/tasks
Content-Type: application/json
Authorization: Bearer <API Key>
Idempotency-Key: <本次业务操作的唯一标识>
```

视频生成是异步任务。创建成功只表示任务已被接受，不表示视频已经生成完成。

### 4.2 文生视频

以下是推荐的最小请求：

```bash
curl -sS -X POST \
  "$TokenAI_API_BASE/api/v3/contents/generations/tasks" \
  -H "Authorization: Bearer $TokenAI_API_KEY" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: video-order-20260728-001" \
  -d '{
    "model": "seedance-byteplus",
    "content": [
      {
        "type": "text",
        "text": "清晨的雪山被第一缕阳光照亮，固定镜头，电影感，写实风格"
      }
    ],
    "duration": 5,
    "resolution": "720p",
    "ratio": "16:9",
    "generate_audio": false
  }'
```

创建成功返回：

```json
{
  "id": "task_xxxxxxxxx"
}
```

请立即保存返回的 `id`，后续查询、下载和删除都使用这个平台任务 ID。

### 4.3 图生视频

使用公网图片 URL：

```bash
curl -sS -X POST \
  "$TokenAI_API_BASE/api/v3/contents/generations/tasks" \
  -H "Authorization: Bearer $TokenAI_API_KEY" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: video-order-20260728-002" \
  -d '{
    "model": "seedance-byteplus",
    "content": [
      {
        "type": "text",
        "text": "人物缓慢转身看向镜头，背景树叶随风摆动"
      },
      {
        "type": "image_url",
        "role": "first_frame",
        "image_url": {
          "url": "https://cdn.example.com/reference.png"
        }
      }
    ],
    "duration": 5,
    "resolution": "720p",
    "ratio": "16:9",
    "generate_audio": false
  }'
```

图片必须能够被模型服务访问。临时签名 URL 应留出足够有效期，不要使用本机地址、内网地址或需要 Cookie 才能访问的链接。

公网 URL 和 Data URL 属于请求级媒体，只用于当前生成请求，不会自动创建平台素材或获得复用、
迁移、撤回和渠道绑定能力。平台作为模型 API 中转层，不主动下载、长期保存或人工审核请求级
媒体；调用方应确保有权向模型服务提交该内容，模型 Provider 仍可能执行自身的格式和内容安全
检查。平台已经识别为真人或需要同意、撤回、审计治理的素材必须使用平台素材路径；平台不对
普通请求级 URL/Data URL 做真人自动识别，未开放真人能力时调用方不得借直接媒体规避业务
准入政策。

### 4.4 使用平台素材

已经通过平台素材库创建且状态为 `ready` 的素材，可以在媒体 URL 中使用 `asset://`：

```json
{
  "type": "image_url",
  "role": "reference_image",
  "image_url": {
    "url": "asset://ast_xxxxxxxxx"
  }
}
```

客户端只保存和使用平台 `ast_...` ID，不需要了解上游素材 ID 或渠道信息。完整的素材创建、真人授权和迁移流程参见[素材库对接指南](素材库对接指南.md)。

## 5. 请求字段

### 5.1 常用字段

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `model` | string | 是 | 从模型列表中取得的模型 ID |
| `content` | array | 是 | 至少包含一个有效内容项 |
| `duration` | integer | 按模型 | `-1` 或 4～15 秒及默认值都由模型 capability 决定 |
| `resolution` | string | 按模型 | `480p`、`720p`、`1080p`、`4k` 的模型级子集 |
| `ratio` | string | 按模型 | `16:9`、`4:3`、`1:1`、`3:4`、`9:16`、`21:9`、`adaptive` 的模型级子集 |
| `generate_audio` | boolean | 否 | 是否生成音频；支持情况以所选模型为准 |

建议只发送业务确实需要的可选字段。只有模型 capability 明确给出默认值时才可省略对应字段；显式的
`false`、`0`、`-1`、`default` 或空数组不会被当成未填写。OpenAPI 的
`ModelArkVideoCreateRequest.x-modelark-model-capabilities` 是当前公开模型能力的机器可读投影，值与运行时
registry 不一致会使测试失败。`seedance-2.0-standard` 与 `seedance-2.0-fast` 当前显式默认
duration/resolution/ratio 为 `5`、`720p`、`16:9`；`seedance-byteplus` 的 resolution 与 ratio 在默认值
完成精确模型验证前必须显式提供。

### 5.2 `content` 内容项

文本：

```json
{"type":"text","text":"视频提示词"}
```

图片：

```json
{
  "type": "image_url",
  "role": "first_frame",
  "image_url": {"url":"https://cdn.example.com/image.png"}
}
```

图片角色包括：

| `role` | 含义 |
| --- | --- |
| `first_frame` | 首帧图 |
| `last_frame` | 尾帧图，必须同时提供首帧图 |
| `reference_image` | 参考图 |

平台合同还定义了 `reference_video` 和 `reference_audio`，但并非每个当前模型都支持。调用前应查看对应模型能力；不支持的字段会返回 `400 unsupported_parameter`，不会被静默忽略。

### 5.3 约束

- 图片最多 9 个，视频和音频各最多 3 个；
- 首帧和尾帧各最多 1 个；
- 尾帧必须与首帧同时提供；
- 只有音频、没有图片或视频时会被拒绝；
- 不接受 `metadata`、`extra` 或任意 Provider 参数透传；
- `callback_url` 当前不支持，请使用任务查询接口；
- `frames` 当前未开放；
- 模型不支持的高级字段会明确返回 4xx，不会静默降级。
- capability 允许 `service_tier` 但所选候选未开启等价透传时，显式 `default` 和 `flex` 都会返回
  `400 unsupported_parameter`，不会被删除后继续请求。

为获得现有通用 Seedance SKU 之间最稳定的可移植性，建议优先使用 `model`、文本或图片
`content`、`duration`、`resolution`、`ratio` 和 `generate_audio`。

### 5.4 `doubao-seedance-2-0-260128`

该模型仍使用本文统一的客户端创建接口，客户端不应直接拼接 Provider 路径：

```text
POST /api/v3/contents/generations/tasks
```

平台将请求适配为 TokenSave V2 `POST /v1/media/generations`，并通过
`GET /v1/media/tasks/{task_id}` 查询。公开模型名与上游模型名相同，不需要 model mapping。

| 能力 | 当前合同 |
| --- | --- |
| 分辨率 | `480p`、`720p`、`1080p` |
| 时长 | 4～15 秒整数，或 `-1` 表示智能时长 |
| 画幅 | `16:9`、`4:3`、`1:1`、`3:4`、`9:16`、`21:9`、`adaptive` |
| 文本 | 支持文生视频 |
| 图片 | 支持首帧、首尾帧和参考图；平台适配为 V2 `reference_images` |
| 视频/音频参考输入 | 当前未发布；Provider 的具体 V2 字段尚未在模型页定义，传入会返回 `unsupported_parameter` |
| 音频生成 | `generate_audio` 适配为 V2 `with_audio` |

Provider 的模型介绍提到视频、音频等多模态参考能力，但当前公开的请求字段只明确给出了图片引用。
平台按已验证的具体字段发布能力，不根据营销描述猜测未定义参数。 Provider 合同与价格以
[TokenSave 模型页](https://tokensave.pro/docs/models/doubao-seedance-2-0-260128)为核对来源。

2026-08-05 的 4 秒 480p 文生黑盒已完成创建、成功轮询、MP4 下载和 Range 验证，终态 `result` 为
对象且没有 `usage`。这只证明基础文生链路，不代表其它场景、分辨率或 Provider 账单已经验收；本地
Channel/Ability 继续禁用，实际可用性仍以模型列表为准。

### 5.5 `seedance-2-0-oversea`

墨行海外版当前只使用 `moxing.seedance-media-task/v2` relay 线路，不会降级到历史 Ark。请求必须包含
非空文本，并显式传入 `duration`（4～15 或 `-1`）、`resolution`（仅 `480p`/`720p`）和 `ratio`。
支持文生视频、单首帧、首尾帧和 `reference_image`；参考图与首尾帧互斥，音频/视频输入、watermark、
seed、camera_fixed、真人素材和 last-frame 结果均未发布。`generate_audio=false` 会原样发送。该渠道在
真实终态结果和 usage 取证完成前保持禁用，实际可用性以模型列表为准。

### 5.6 飞彩 Seedance 2.0 固定分辨率 SKU

飞彩 v2 已在代码中分别登记 Mini、SD2、Fast、value/standard 720p、1080p、4K 与 Pro PI 共 10 个
SKU。当前模型列表发布 Mini 720p、Fast 720p、Standard 720p、Standard 1080p、Standard 4K 和
Value 720p，六者都只开放已验证的 `16:9`。SD2、Value 1080p、Value 4K 和 Pro PI 因真实 Provider
验证失败或 create outcome unknown 保持未发布；调用方不得依赖研究资料中的默认时长、其它画幅或
价格，也不得用已发布 SKU 的成功横向推导这四个模型。

## 6. 查询任务

### 6.1 查询单个任务

```bash
export VIDEO_TASK_ID="task_xxxxxxxxx"

curl -sS \
  "$TokenAI_API_BASE/api/v3/contents/generations/tasks/$VIDEO_TASK_ID" \
  -H "Authorization: Bearer $TokenAI_API_KEY"
```

任务状态：

```text
queued -> running -> succeeded
                  -> failed
                  -> cancelled
                  -> expired
```

成功响应示例：

```json
{
  "id": "task_xxxxxxxxx",
  "model": "seedance-byteplus",
  "status": "succeeded",
  "content": {
    "video_url": "https://<your-api-host>/v1/videos/task_xxxxxxxxx/content"
  },
  "service_tier": "default",
  "created_at": 1785200000,
  "updated_at": 1785200120
}
```

失败任务会返回 `status: "failed"`，并在 `error` 中提供稳定错误信息：

```json
{
  "id": "task_xxxxxxxxx",
  "model": "seedance-byteplus",
  "status": "failed",
  "error": {
    "code": "generation_failed",
    "message": "Video generation failed"
  }
}
```

### 6.2 轮询建议

- 创建成功后首次等待约 2 秒；
- 随后每 5 秒查询一次；
- 长任务可逐步增加到 10～15 秒；
- 收到 `429`、`502` 或 `503` 时指数退避；
- 客户端超时不代表服务端任务失败，应继续查询原任务；
- 不要在每次轮询时重新创建任务。

简单轮询示例：

```bash
while true; do
  RESPONSE="$(curl -sS \
    "$TokenAI_API_BASE/api/v3/contents/generations/tasks/$VIDEO_TASK_ID" \
    -H "Authorization: Bearer $TokenAI_API_KEY")"
  STATUS="$(printf '%s' "$RESPONSE" | jq -r '.status')"
  printf 'status=%s\n' "$STATUS"

  case "$STATUS" in
    succeeded|failed|cancelled|expired)
      printf '%s\n' "$RESPONSE" | jq
      break
      ;;
  esac

  sleep 5
done
```

### 6.3 查询历史任务

```bash
curl -sS \
  "$TokenAI_API_BASE/api/v3/contents/generations/tasks?page_num=1&page_size=10" \
  -H "Authorization: Bearer $TokenAI_API_KEY"
```

可用过滤条件：

- `filter.status`
- `filter.model`
- `filter.service_tier`
- 可重复提供的 `filter.task_ids`

列表只返回当前用户最近 7 天、尚未删除的 ModelArk v3 视频任务。`page_num` 和 `page_size` 取值为 1～500。

## 7. 下载视频

查询结果中的 `content.video_url` 是受鉴权的平台内容地址，下载时仍需携带同一个 Bearer Token：

```bash
VIDEO_URL="$(curl -sS \
  "$TokenAI_API_BASE/api/v3/contents/generations/tasks/$VIDEO_TASK_ID" \
  -H "Authorization: Bearer $TokenAI_API_KEY" \
  | jq -r '.content.video_url // empty')"

curl -L "$VIDEO_URL" \
  -H "Authorization: Bearer $TokenAI_API_KEY" \
  -o output.mp4
```

只有 `succeeded` 任务可以下载。内容接口支持标准下载和 Range 请求；不要把受鉴权下载地址当成永久公开 URL。

使用真人托管素材的任务会在发送前预留授权，并在每次内容回源前重新检查授权状态；授权撤回后，
平台会阻断后续内容回源。平台始终无法撤回此前已经由客户下载或复制到平台之外的内容。

## 8. 取消或删除任务

```bash
curl -sS -X DELETE \
  "$TokenAI_API_BASE/api/v3/contents/generations/tasks/$VIDEO_TASK_ID" \
  -H "Authorization: Bearer $TokenAI_API_KEY"
```

不同状态下的行为：

| 当前状态 | 结果 |
| --- | --- |
| `queued` | 尝试取消任务 |
| `running` | 返回 `409 task_running` |
| `cancelled` | 返回 `409 task_cancelled` |
| `succeeded`、`failed`、`expired` | 删除结果并从当前用户列表隐藏 |

如果返回 `503 cancellation_unknown`，表示上游取消结果暂时未知。应继续查询原任务，不要立即创建重复任务。

飞彩 v2 的 10 个固定分辨率 SKU 当前尚未发布，不得依赖历史两个 720p 合同的取消或删除语义。
模型发布后仍必须以当时 capability 与服务端 409 错误为准，不会伪装取消或删除成功。

## 9. 幂等与安全重试

每次新的业务操作都应生成唯一的 `Idempotency-Key`：

```http
Idempotency-Key: order-12345-video-1
```

规则：

- Header 是推荐的可选平台扩展；缺失时创建请求不会因此被拒绝；
- 已成功持久化 Task 后，同一个 Key 和相同请求会返回原任务；
- 同一个 Key 搭配不同请求会返回 `409 idempotency_conflict`；
- 平台在预扣前建立内部创建记录；发送后无法确认结果时会保留预扣并进入有界对账，不会换渠道
  或重复提交上游创建请求；
- 原请求仍在创建或结果未知时，同 Key 重放返回 `409 idempotency_in_progress`。不要更换 Key
  或自动重新创建，应保存 `request_id` 并继续查询或联系平台核对；
- 原请求未提供 Key 时不会建立客户端幂等 claim，超时或 `create_outcome_unknown` 后再次提交
  可能创建第二个上游任务。事后补充 Key 也不能恢复原操作；
- 只有用户明确发起一次新的生成操作时，才应更换 Key。

推荐使用业务订单 ID、UUID 或其他不可碰撞的操作 ID，不要使用固定字符串。

## 10. Python 示例

```python
import os
import time

import requests

api_base = os.environ["TokenAI_API_BASE"].rstrip("/")
api_key = os.environ["TokenAI_API_KEY"]

headers = {
    "Authorization": f"Bearer {api_key}",
    "Content-Type": "application/json",
    "Idempotency-Key": "video-order-20260728-003",
}
payload = {
    "model": "seedance-byteplus",
    "content": [
        {
            "type": "text",
            "text": "海边灯塔在暴风雨中亮起，电影感，写实风格",
        }
    ],
    "duration": 5,
    "resolution": "720p",
    "ratio": "16:9",
    "generate_audio": False,
}

response = requests.post(
    f"{api_base}/api/v3/contents/generations/tasks",
    headers=headers,
    json=payload,
    timeout=30,
)
response.raise_for_status()
task_id = response.json()["id"]

while True:
    response = requests.get(
        f"{api_base}/api/v3/contents/generations/tasks/{task_id}",
        headers={"Authorization": f"Bearer {api_key}"},
        timeout=30,
    )
    response.raise_for_status()
    task = response.json()
    print(task["status"])

    if task["status"] == "succeeded":
        video_url = task["content"]["video_url"]
        break
    if task["status"] in {"failed", "cancelled", "expired"}:
        raise RuntimeError(task.get("error", task))

    time.sleep(5)

with requests.get(
    video_url,
    headers={"Authorization": f"Bearer {api_key}"},
    stream=True,
    timeout=120,
) as response:
    response.raise_for_status()
    with open("output.mp4", "wb") as output:
        for chunk in response.iter_content(chunk_size=1024 * 1024):
            if chunk:
                output.write(chunk)
```

生产代码还应为 `429`、`502`、`503` 添加带抖动的指数退避，并设置业务级最长等待时间。

## 11. 常见错误

下表只适用于 Seedance/ModelArk 合同。Kling 和即梦使用各自 API Reference 中定义的数字错误
信封，客户端不得混用三套协议的错误解析逻辑。

| HTTP | 错误码示例 | 处理建议 |
| --- | --- | --- |
| `400` | `invalid_request` | 检查字段类型、枚举、内容组合和时长 |
| `400` | `unsupported_parameter` | 删除当前模型不支持的字段，不要改为私有透传 |
| `401` | `authentication_error` | 检查 API Key 和 `Bearer` 前缀 |
| `403` | `permission_denied` | 检查令牌模型权限 |
| `404` | `task_not_found` | 检查任务 ID、用户和查询协议 |
| `409` | `idempotency_conflict` | 同一 Key 不得用于不同请求 |
| `429` | `rate_limit_exceeded` | 按 `Retry-After` 或指数退避重试 |
| `502` | `upstream_unavailable` | 保留原幂等 Key，稍后重试 |
| `503` | `create_outcome_unknown` | 停止自动重试，保存 `request_id`；有 Key 时不得换 Key，无 Key 时再次提交可能重复创建 |
| `503` | `cancellation_unknown` | 继续查询原任务状态 |

排查时记录响应中的 `request_id`，但不要记录完整 API Key、签名 URL、素材源地址或真人认证地址。

## 12. 计费说明

- 创建异步任务时平台可能先预扣额度；
- 成功、失败或取消后按任务实际结果结算；
- 具体价格、计费单位和可用额度以控制台当前展示为准；
- 任务响应中的 `usage` 只在模型返回可用量时出现；没有 `usage` 不代表任务免费；
- 对账时使用平台任务 ID 和调用日志，不使用上游任务 ID。
