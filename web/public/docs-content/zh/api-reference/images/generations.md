---
page-id: images-generations
kind: api-reference
last-verified: 2026-07-31
operations:
  - createImageGeneration
---

# 图片生成

`POST /v1/images/generations` · Bearer 鉴权 · `application/json`

该接口使用 OpenAI 兼容的图片请求，不接受仅属于供应商渠道适配协议的 `input.messages` 或 `parameters` 结构。

## 请求

```bash
curl "{{OPENAI_BASE_URL}}/images/generations" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "{{MODEL_ID_PLACEHOLDER}}",
    "prompt": "雾中灯塔的水彩插画",
    "n": 1,
    "size": "1024x1024",
    "response_format": "url"
  }'
```

| 字段              | 必填 | 说明                               |
| ----------------- | ---- | ---------------------------------- |
| `model`           | 是   | 当前 Key 可访问的图片模型          |
| `prompt`          | 是   | 图片描述                           |
| `n`               | 否   | 生成数量，范围 `1` 到 `128`        |
| `size`            | 否   | 模型支持的尺寸                     |
| `quality`         | 否   | 模型支持的质量档位                 |
| `response_format` | 否   | 常见值为 `url` 或 `b64_json`       |
| `stream`          | 否   | 模型和渠道支持时启用流式或异步行为 |

显式的 `false`、`0` 和空字符串是否有业务含义由对应字段合同决定；不要通过省略与零值互相替代。

## 同步响应

同步完成时返回创建时间和图片数组：

```json
{
  "created": 1760000000,
  "data": [{ "url": "https://example.com/generated-image.png" }]
}
```

URL 可能有有效期。需要长期使用时请及时下载到你控制的存储。

## 异步响应

部分模型会返回图片任务。保存任务 `id`，然后使用[查询图片任务](api-reference/images/tasks)轮询。创建成功不代表图片已经完成。

## 计费与幂等

数量、尺寸、质量和模型都会影响费用。`n` 超过上限会在计费前以 `400` 拒绝。异步 Task 可以
通过同 Key 或返回的任务 ID 恢复；同步 HTTP `200` 不保存结果用于回放，同 Key 重试仍可能重新
生成并再次计费。若客户端超时，不要盲目重复创建；保存平台 request ID、返回 URL 或任务 ID，
并按业务记录核对。
