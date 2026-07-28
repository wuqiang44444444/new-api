---
status: current
owner: Dev Team
last-reviewed: 2026-07-28
---

# 视频模型 API 调用指南

## 1. 适用范围

本文面向使用  API 的开发者，介绍当前 Seedance 视频模型的任务创建、状态查询、结果下载、历史列表和删除方式。

当前开放以下公开视频模型：

| 模型 ID | 说明 |
| --- | --- |
| `seedance-byteplus` | Seedance BytePlus SKU |
| `seedance-2-0-oversea` | Seedance 2.0 海外版 SKU |
| `doubao-seedance-2-0-260128` | Seedance 2.0 国际版 SKU |

三个模型使用完全相同的客户端 API。调用方只需更换 `model`，不需要了解或切换上游地址、鉴权方式、任务路径和响应格式。

模型可用范围可能随账户权限调整。正式调用前应通过模型列表或控制台确认当前 API Key 可以使用的模型。平台当前不提供 OpenAI/Sora 视频新任务创建，不要使用 `POST /v1/videos` 或旧的 `/v1/video/generations` 创建视频。

## 2. 调用准备

准备以下信息：

1. 从控制台创建 API Key；
2. 复制当前站点的 API Base URL；
3. 确认账户余额和模型权限。

示例使用环境变量，避免把密钥写进代码：

```bash
export LINKMETAX_API_BASE="https://<your-api-host>"
export LINKMETAX_API_KEY="sk-your-key"
```

本文的 `LINKMETAX_API_BASE` 是站点根地址，不包含末尾的 `/v1`。视频创建接口会在其后追加 `/api/v3/...`。

所有请求均使用 Bearer Token：

```http
Authorization: Bearer sk-your-key
```

API Key 只能保存在服务端或安全的密钥管理系统中，不得提交到代码仓库、前端代码、移动端安装包或日志。

## 3. 查询可用模型

```bash
curl -sS "$LINKMETAX_API_BASE/v1/models" \
  -H "Authorization: Bearer $LINKMETAX_API_KEY"
```

只查看 Seedance 模型：

```bash
curl -sS "$LINKMETAX_API_BASE/v1/models" \
  -H "Authorization: Bearer $LINKMETAX_API_KEY" \
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
  "$LINKMETAX_API_BASE/api/v3/contents/generations/tasks" \
  -H "Authorization: Bearer $LINKMETAX_API_KEY" \
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
  "$LINKMETAX_API_BASE/api/v3/contents/generations/tasks" \
  -H "Authorization: Bearer $LINKMETAX_API_KEY" \
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
| `duration` | integer | 否 | 默认 5 秒；也可传 `-1` 或 4～15 秒，具体模型可能进一步限制 |
| `resolution` | string | 否 | `480p`、`720p`、`1080p`、`4k` |
| `ratio` | string | 否 | `16:9`、`4:3`、`1:1`、`3:4`、`9:16`、`21:9`、`adaptive` |
| `generate_audio` | boolean | 否 | 是否生成音频；支持情况以所选模型为准 |

建议只发送业务确实需要的可选字段。省略字段表示使用模型默认行为；显式的 `false` 不会被当成未填写。

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

为获得三个当前模型之间最稳定的可移植性，建议优先使用 `model`、文本或图片 `content`、`duration`、`resolution`、`ratio` 和 `generate_audio`。

## 6. 查询任务

### 6.1 查询单个任务

```bash
export VIDEO_TASK_ID="task_xxxxxxxxx"

curl -sS \
  "$LINKMETAX_API_BASE/api/v3/contents/generations/tasks/$VIDEO_TASK_ID" \
  -H "Authorization: Bearer $LINKMETAX_API_KEY"
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
    "$LINKMETAX_API_BASE/api/v3/contents/generations/tasks/$VIDEO_TASK_ID" \
    -H "Authorization: Bearer $LINKMETAX_API_KEY")"
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
  "$LINKMETAX_API_BASE/api/v3/contents/generations/tasks?page_num=1&page_size=10" \
  -H "Authorization: Bearer $LINKMETAX_API_KEY"
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
  "$LINKMETAX_API_BASE/api/v3/contents/generations/tasks/$VIDEO_TASK_ID" \
  -H "Authorization: Bearer $LINKMETAX_API_KEY" \
  | jq -r '.content.video_url // empty')"

curl -L "$VIDEO_URL" \
  -H "Authorization: Bearer $LINKMETAX_API_KEY" \
  -o output.mp4
```

只有 `succeeded` 任务可以下载。内容接口支持标准下载和 Range 请求；不要把受鉴权下载地址当成永久公开 URL。

## 8. 取消或删除任务

```bash
curl -sS -X DELETE \
  "$LINKMETAX_API_BASE/api/v3/contents/generations/tasks/$VIDEO_TASK_ID" \
  -H "Authorization: Bearer $LINKMETAX_API_KEY"
```

不同状态下的行为：

| 当前状态 | 结果 |
| --- | --- |
| `queued` | 尝试取消任务 |
| `running` | 返回 `409 task_running` |
| `cancelled` | 返回 `409 task_cancelled` |
| `succeeded`、`failed`、`expired` | 删除结果并从当前用户列表隐藏 |

如果返回 `503 cancellation_unknown`，表示上游取消结果暂时未知。应继续查询原任务，不要立即创建重复任务。

## 9. 幂等与安全重试

每次新的业务操作都应生成唯一的 `Idempotency-Key`：

```http
Idempotency-Key: order-12345-video-1
```

规则：

- 同一个 Key 和相同请求会返回原任务，不会重复生成；
- 同一个 Key 搭配不同请求会返回 `409 idempotency_conflict`；
- 创建请求超时、网络断开或返回 `create_outcome_unknown` 时，必须使用原 Key 重试；
- 只有用户明确发起一次新的生成操作时，才应更换 Key。

推荐使用业务订单 ID、UUID 或其他不可碰撞的操作 ID，不要使用固定字符串。

## 10. Python 示例

```python
import os
import time

import requests

api_base = os.environ["LINKMETAX_API_BASE"].rstrip("/")
api_key = os.environ["LINKMETAX_API_KEY"]

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

下表中的字符串错误码适用于 Seedance/ModelArk 合同。Kling 错误保持数字 `code`（例如参数错误 `1200`、资源不存在 `1203`、限流 `1303`、服务暂不可用 `5001`）；即梦错误同时返回数字 `code` 和 `status`（例如参数错误 `50200`、鉴权失败 `50400`、并发限制 `50430`、内部错误 `50500`）。三类协议都应先判断 HTTP 状态，再按各自官方数字或字符串错误码处理。

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
| `503` | `create_outcome_unknown` | 使用原幂等 Key 重试，不要创建新操作 |
| `503` | `cancellation_unknown` | 继续查询原任务状态 |

排查时记录响应中的 `request_id`，但不要记录完整 API Key、签名 URL、素材源地址或真人认证地址。

## 12. 计费说明

- 创建异步任务时平台可能先预扣额度；
- 成功、失败或取消后按任务实际结果结算；
- 具体价格、计费单位和可用额度以控制台当前展示为准；
- 任务响应中的 `usage` 只在模型返回可用量时出现；没有 `usage` 不代表任务免费；
- 对账时使用平台任务 ID 和调用日志，不使用上游任务 ID。
