---
page-id: images-edits
kind: api-reference
last-verified: 2026-08-25
operations:
  - createImageEdit
---

# 图片编辑

`POST /v1/images/edits` · Bearer 鉴权 · `multipart/form-data`

图片编辑通过表单上传源图片。不要手工设置 multipart boundary，应由 HTTP 客户端生成 `Content-Type`。

## 最小请求

```bash
curl "{{OPENAI_BASE_URL}}/images/edits" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  -F "model={{MODEL_ID_PLACEHOLDER}}" \
  -F "prompt=把天空改成日落" \
  -F "image=@input.png" \
  -F "n=1"
```

| 字段              | 必填 | 说明                           |
| ----------------- | ---- | ------------------------------ |
| `model`           | 是   | 当前 Key 可访问的图片编辑模型  |
| `prompt`          | 是   | 编辑指令                       |
| `image`           | 是   | 一个或多个图片文件；数量与格式由模型决定 |
| `mask`            | 否   | 模型支持时指定编辑区域         |
| `n`               | 否   | 生成数量，范围 `1` 到 `128`    |
| `size`            | 否   | 输出尺寸                       |
| `response_format` | 否   | `url` 或 `b64_json`            |
| `stream`          | 否   | 字符串布尔值 `true` 或 `false` |

## 文件要求

文件大小、格式、透明通道和多图数量受所选模型约束。多图请求使用重复的 `image` 字段或客户端支持的数组表单写法。客户端应在上传前验证文件，并为请求设置合理的总超时。

## 响应与等待

响应与图片生成相同。客户端只会在本次请求内收到最终图片或失败错误，不会收到图片任务 ID。上传失败时不要把错误体当作图片保存。

不支持图片编辑的模型、文件数量或字段组合会返回 `400`。不要根据模型名称推断编辑、遮罩、多图或流式能力，应以当前模型的公开说明和实际校验结果为准。

## 重试

multipart 请求可能较大。网络中断后，只有在确认服务端未受理或存在可靠幂等边界时才重试。不要在日志中记录图片二进制、Base64 或完整敏感提示词。
