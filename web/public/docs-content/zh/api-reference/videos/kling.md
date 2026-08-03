---
page-id: videos-kling
kind: api-reference
last-verified: 2026-07-31
operations:
  - createKlingText2Video
  - getKlingText2Video
  - createKlingImage2Video
  - getKlingImage2Video
---

# Kling 视频

Kling 合同分别提供文生视频和图生视频入口。请求与响应使用 Kling 公开信封。
合同 operation 已发布不表示所有账户都能使用任意 `model_name`；调用前应以 `GET /v1/models`
和当前账户权限为准。当前 Kling Link 合同只发布分类创建和对应单任务查询，不提供列表或删除合同。
当前内置适配器登记的公开模型为 `kling-v1`、`kling-v1-6` 和 `kling-v2-master`。

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

## 已发布创建能力

三个公开模型使用同一版本化合同：`model_name`、`prompt` 必填；`mode` 允许 `std`、`pro`，
`duration` 允许字符串 `"5"`、`"10"`，`aspect_ratio` 允许 `16:9`、`9:16`、`1:1`，
`cfg_scale` 范围为 0～1。公开字段还包括 `image`、`image_tail`、`negative_prompt`、
`static_mask`、`dynamic_masks`、`camera_control`、`callback_url` 和 `external_task_id`。
文生视频不接受 `image`/`image_tail`；图生视频的 `image_tail` 必须与 `image` 同时使用。

## 查询任务

文生视频和图生视频分别查询：

```text
GET /kling/v1/videos/text2video/{task_id}
GET /kling/v1/videos/image2video/{task_id}
```

响应的 `code` 为 `0` 表示请求处理成功，业务状态位于 `data.task_status`。非零 `code` 应按 Kling 错误信封处理。
当前合同不发布列表、平台内容代理、取消或删除操作。

## 字段与计费

`mode`、`duration`、`aspect_ratio`、遮罩、镜头控制和回调能力取决于模型。时长和模式通常影响费用；不要发送未在公开合同中的渠道参数。
