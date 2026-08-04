---
page-id: videos-openai
kind: api-reference
last-verified: 2026-08-04
operations:
  - createVideo
  - getVideo
  - remixVideo
  - createVideoGeneration
---

# OpenAI Videos

本页描述仓库上游现有的 OpenAI Videos 路由，适用于渠道实际支持的 Sora/OpenAI 视频模型。
Seedance、Kling、即梦 Link SKU 使用各自的类型化入口，不通过本页推断或扩展能力。

## 创建视频

`POST /v1/videos` 支持渠道声明的请求格式。JSON 最小请求示例：

```bash
curl "{{OPENAI_BASE_URL}}/videos" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "{{MODEL_ID_PLACEHOLDER}}",
    "prompt": "清晨的海岸线，固定镜头",
    "seconds": "8",
    "size": "1280x720"
  }'
```

模型、时长、尺寸和参考输入必须以所选渠道的实际合同为准；本页不会把 Link SKU capability
投影到原生 OpenAI Videos。

## 查询任务

```bash
curl "{{OPENAI_BASE_URL}}/videos/video-task-placeholder" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}"
```

查询返回任务状态、进度以及渠道提供的结果信息。调用方应保存创建接口返回的任务 ID，并对未完成
状态进行有界轮询。

## Remix

```bash
curl -X POST "{{OPENAI_BASE_URL}}/videos/video-task-placeholder/remix" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  -H "Content-Type: application/json" \
  -d '{"prompt":"保持构图，将天气改为雪天"}'
```

只有 Provider adapter 实际支持 Remix 时才能使用该操作。

## 既有视频生成入口

仓库上游当前仍登记 `POST /v1/video/generations`。它与本次飞彩 Link 南向 `/v1/videos` 没有
身份或兼容关系；飞彩客户入口仍是 ModelArk v3。

## 内容下载

任务完成后可通过受鉴权内容端点下载：

```bash
curl "{{OPENAI_BASE_URL}}/videos/video-task-placeholder/content" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  --output result.mp4
```

先检查 HTTP 状态和 `Content-Type`；错误响应是 JSON，而不是视频字节。
