---
page-id: videos-openai
kind: api-reference
last-verified: 2026-08-04
operations:
  - createVideo
  - getVideo
  - remixVideo
  - getVideoContent
  - createVideoGeneration
---

# OpenAI Videos

OpenAI Videos 是 NEWAPI rc23 原生合同，适用于 `sora-2`、`sora-2-pro` 等原生模型。Seedance、
Kling、即梦 Link SKU 必须使用各自文档中的路径，不能提交到本页入口。

## 创建视频

`POST /v1/videos` 支持 JSON 与 multipart。JSON 最小请求：

```bash
curl "{{OPENAI_BASE_URL}}/videos" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: video-operation-placeholder" \
  -d '{
    "model": "{{MODEL_ID_PLACEHOLDER}}",
    "prompt": "清晨的海岸线，固定镜头",
    "seconds": "8",
    "size": "1280x720"
  }'
```

`model` 省略时默认 `sora-2`。`seconds` 支持 `4`、`8`、`12`。参考图片可以使用 multipart
`input_reference` 文件，或 JSON `input_reference` 对象中的 `file_id` / `image_url`。
`image_url` 支持当前模型允许的 HTTP/HTTPS、Data URL 和平台 `asset://ast_xxx`；平台素材仍会
执行同一应用所有权、授权状态和渠道绑定检查。

## 查询任务

```bash
curl "{{OPENAI_BASE_URL}}/videos/video-task-placeholder" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}"
```

只能查询同一用户、同一 API Key 应用下由 OpenAI Videos 合同创建的任务。响应包含任务状态、
进度、模型、时长和错误信息。

## Remix

```bash
curl -X POST "{{OPENAI_BASE_URL}}/videos/video-task-placeholder/remix" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: video-remix-placeholder" \
  -d '{"prompt":"保持构图，将天气改为雪天"}'
```

源视频必须在同一应用可见、属于 OpenAI Videos 原生合同，且创建它的 Provider adapter 支持
Remix。Link SKU、其他应用任务、已删除任务或不支持 Remix 的任务都会被拒绝。

## 下载内容

任务完成后使用：

```bash
curl "{{OPENAI_BASE_URL}}/videos/video-task-placeholder/content" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  --output result.mp4
```

先检查 HTTP 状态和 `Content-Type`。任务未完成、素材授权已撤回或任务不属于当前应用时会返回
JSON 错误，不是视频字节。

## 旧版兼容入口

`POST /v1/video/generations` 与 `GET /v1/video/generations/{task_id}` 是 rc23 旧版平台视频合同。
新客户端优先使用 `/v1/videos`；显式登记的 Link SKU 同样不能借旧版入口创建。

## 计费与重试

平台在 Provider POST 前建立 durable create attempt 并持有额度。发送后无法确认创建结果时返回
`create_outcome_unknown`，不会自动退款、换渠道或再次创建。客户端应保存 `request_id`，停止自动
重试并等待平台对账；`Idempotency-Key` 不能消除一个已经处于未知状态的 Provider 结果。
