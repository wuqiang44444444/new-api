---
page-id: images-generations
kind: api-reference
last-verified: 2026-08-25
operations:
  - createImageGeneration
---

# 图片生成

`POST /v1/images/generations` · Bearer 鉴权 · `application/json`

该接口使用 OpenAI 兼容的图片请求。客户只提交当前 Key 可访问的模型和公开字段，不需要了解模型背后的内部实现。

## 请求

```bash
curl "{{OPENAI_BASE_URL}}/images/generations" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "{{MODEL_ID_PLACEHOLDER}}",
    "prompt": "雾中灯塔的水彩插画"
  }'
```

| 字段                 | 必填 | 说明                                      |
| -------------------- | ---- | ----------------------------------------- |
| `model`              | 是   | `GET /v1/models` 返回且当前 Key 可访问的模型 |
| `prompt`             | 是   | 图片描述                                  |
| `n`                  | 否   | 生成数量，公共上限为 `128`；模型可以有更低上限 |
| `size`               | 否   | 所选模型支持的尺寸或分辨率                |
| `quality`            | 否   | 所选模型支持的质量档位                    |
| `style`              | 否   | 所选模型支持的风格                        |
| `response_format`    | 否   | 所选模型支持时使用 `url` 或 `b64_json`    |
| `stream`             | 否   | 仅在所选模型明确支持流式图片时使用        |
| `background`         | 否   | 所选模型支持的背景设置                    |
| `moderation`         | 否   | 所选模型支持的内容审核设置                |
| `output_format`      | 否   | 所选模型支持的输出格式                    |
| `output_compression` | 否   | 所选模型支持的输出压缩参数                |
| `partial_images`     | 否   | 所选模型支持的部分图片数量                |
| `watermark`          | 否   | 所选模型支持的水印开关                    |

显式的 `false`、`0` 和空字符串是否有业务含义由对应字段合同决定；不要通过省略与零值互相替代。

上表是公共字段全集，不代表每个模型都支持每一项。调用前读取对应条目的
`api.image.creation.parameters`：它是该客户模型允许的完整字段列表；固定规格、`n` 上限、枚举和默认值
也在其中。`additional_properties=false` 时，未列出的字段必须省略。例如某模型没有 `size` 或
`watermark` 条目，就不能发送该字段，即使值为 `false`。

不要根据模型名称猜测尺寸、质量、数量、流式响应或参考图能力；不支持的字段会在发送前返回 `400`，
不会被静默删除或改写。不要提交未在模型目录和 OpenAPI 中声明的内部字段或私有请求结构。

## 同步响应

非流式请求完成时返回创建时间和图片数组。每个结果按所选模型能力包含 `url` 或 `b64_json`：

```json
{
  "created": 1760000000,
  "data": [{ "url": "https://example.com/generated-image.png" }]
}
```

URL 可能有有效期。需要长期使用时请及时下载到你控制的存储。

## 等待、超时与计费

数量、尺寸、质量和模型可能影响费用。`n` 超过公共上限或模型上限时，会在计费前以 `400` 拒绝。

普通图片请求始终在本次 HTTP 请求内等待结果，不返回图片任务 ID，也没有图片任务查询接口。客户端应设置足够的请求超时；收到超时或网络中断后不要盲目重复创建，因为请求可能已经被受理，重复提交可能产生重复图片和费用。

## 错误处理

- `400`：字段、取值或模型能力不匹配；修正请求后再提交。
- `401` / `403`：检查 API Key、模型权限和可用分组。
- `429`：按照响应提示退避，并检查额度或限流。
- `5xx`：保留公开请求 ID；除非能接受重复生成风险，否则不要自动重试。
