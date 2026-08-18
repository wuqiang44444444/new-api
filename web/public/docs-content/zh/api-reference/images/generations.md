---
page-id: images-generations
kind: api-reference
last-verified: 2026-08-18
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
| `n`               | 否   | 生成数量由模型决定；FunCloud 中转异步图片渠道固定为 `1` |
| `size`            | 否   | 模型支持的尺寸                     |
| `quality`         | 否   | 模型支持的质量档位                 |
| `response_format` | 否   | 由渠道声明；FunCloud 中转异步图片渠道仅支持 `url` |
| `stream`          | 否   | 图片生成渠道通常不支持 `true` |

显式的 `false`、`0` 和空字符串是否有业务含义由对应字段合同决定；不要通过省略与零值互相替代。

FunCloud 中转异步图片渠道当前只发布固定规格：`nano-banana-2` 为 `1K`、
`seedream-5.0-lite` 为 `2K/basic`、`seedream-5.0-pro` 为 `1K/basic`；参考图字段尚未发布，
传入 `extra_fields.reference_images` 会返回 `400`。更高规格、质量档位和参考图计费规则完成配置与
真实验收后再开放。

## 同步响应

同步完成时返回创建时间和图片数组：

```json
{
  "created": 1760000000,
  "data": [{ "url": "https://example.com/generated-image.png" }]
}
```

URL 可能有有效期。需要长期使用时请及时下载到你控制的存储。

## 等待、超时与计费

数量、尺寸、质量和模型都会影响费用。`n` 超过模型上限会在计费前以 `400` 拒绝。部分 Provider
在南向使用异步任务，但网关在本次请求内轮询并只返回图片响应；等待上限由部署的 `RELAY_TIMEOUT`
决定。超时或客户端取消按普通同步失败/退款语义处理，不提供图片任务查询接口。客户端不要盲目重复
创建，以免产生重复生成和计费。
