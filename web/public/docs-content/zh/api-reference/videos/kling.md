---
page-id: videos-kling
kind: api-reference
last-verified: 2026-07-29
operations:
  - createKlingText2Video
  - getKlingText2Video
  - createKlingImage2Video
  - getKlingImage2Video
---

# Kling 视频

Kling 合同分别提供文生视频和图生视频入口。请求与响应使用 Kling 公开信封。

## 文生视频

`POST /kling/v1/videos/text2video` · `application/json`

```bash
curl "{{SITE_BASE_URL}}/kling/v1/videos/text2video" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  -H "Content-Type: application/json" \
  -d '{
    "model_name": "{{MODEL_ID_PLACEHOLDER}}",
    "prompt": "纸船沿着雨后的街道缓慢漂流",
    "duration": "5",
    "aspect_ratio": "16:9"
  }'
```

## 图生视频

`POST /kling/v1/videos/image2video` · `application/json`

```bash
curl "{{SITE_BASE_URL}}/kling/v1/videos/image2video" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  -H "Content-Type: application/json" \
  -d '{
    "model_name": "{{MODEL_ID_PLACEHOLDER}}",
    "prompt": "镜头向前推进",
    "image": "https://example.com/input-image.png",
    "duration": "5",
    "aspect_ratio": "16:9"
  }'
```

`image` 可以是模型支持的 URL、Base64 或 `asset://` 引用。使用远程 URL 时，确保服务端可访问且有效期足够。

## 查询任务

文生视频和图生视频分别查询：

```text
GET /kling/v1/videos/text2video/{task_id}
GET /kling/v1/videos/image2video/{task_id}
```

响应的 `code` 为 `0` 表示请求处理成功，业务状态位于 `data.task_status`。非零 `code` 应按 Kling 错误信封处理。

## 字段与计费

`mode`、`duration`、`aspect_ratio`、遮罩、镜头控制和回调能力取决于模型。时长和模式通常影响费用；不要发送未在公开合同中的渠道参数。
