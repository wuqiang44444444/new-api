# 墨行（moxing.pro）图片生成 API

> 调研模型：Doubao-Seedream-5.0-lite、Doubao-Seedream-5.0-pro。

---

## Doubao-Seedream-5.0-lite

- **模型 ID**：`doubao-seedream-5-0-260128`
- **厂商**：豆包
- **类型**：image

Doubao-Seedream-5.0-lite 是字节跳动发布的最新图像创作模型，首次搭载联网检索功能，能融合实时网络信息，提升生图时效性。

### 能力与接口

- **图片生成**：`POST /v1/media/generations`
- 支持 `POST /v1/images/generations` 同步等待返回，也支持 `POST /v1/media/generations` 创建异步任务后轮询 `GET /v1/media/tasks/:task_id`。两种方式使用相同参数、模型范围、日志、扣费和对账链路。
- 火山方舟官方接口：`POST https://ark.cn-beijing.volces.com/api/v3/images/generations`。本模型按官方 Seedream 5.0 Lite 参数补充，火山专有字段放入 `extra`，平台提交上游时合并为官方 JSON 字段。

### 生成场景

- 文生图
- 图生图 / 图片编辑
- 组图生成
- 联网搜索生成

### 请求参数

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| model | string | 是 | 模型 logical key，固定为 `doubao-seedream-5-0-260128` |
| prompt | string | 是 | 提示词，支持中英文；建议不超过 300 汉字或 600 英文单词 |
| response_format | string | 否 | `url` 返回下载链接（24 小时内有效），`b64_json` 返回 Base64 编码 |
| size | string | 是 | 输出尺寸：宽高像素值（默认 2048x2048，总像素在 2560x1440~4096x4096 之间、宽高比 [1/16,16]）或分辨率档位 `2K`/`3K`/`4K` |
| capability | string | 是 | 能力类型，固定为 `image_generation` |
| image | string \| string[] | 否 | 输入图片，支持 URL 或 `data:image/<格式>;base64,<Base64>`；格式 jpeg/png/webp/bmp/tiff/gif/heic/heif |
| reference_images | string[] | 否 | 参考图数组，多图参考 2~14 张，平台按官方 image 数组语义处理 |
| extra | object | 否 | 上游扩展参数，放置火山方舟专有字段 |

`capability`、`image`、`reference_images`、`extra` 及以下 `extra` 内字段均属模型专属/上游扩展，放入 `extra` 或对应平台兼容字段后，平台会透传或映射到上游。

`extra` 内支持的字段：

| 参数 | 类型 | 说明 |
| --- | --- | --- |
| sequential_image_generation | string | 组图模式：`auto` 自动判断 / `disabled` 关闭组图 |
| sequential_image_generation_options | object | 组图选项，仅 `sequential_image_generation=auto` 时生效；`max_images` 范围 [1, 15]，输入参考图 + 最终生成图 <= 15 |
| tools | array\<object\> | 工具，`[{"type":"web_search"}]` 开启联网搜索，模型按提示词自主判断是否搜索 |
| stream | boolean | 流式输出，true 时上游每生成一张图即返回；异步任务建议传 false 或省略 |
| output_format | string | 输出格式 `jpeg`/`png`，默认 jpeg |
| watermark | boolean | 是否在右下角添加“AI生成”水印，官方默认 true |
| optimize_prompt_options | object | 提示词优化，`{"mode":"standard"}`；fast 模式当前不支持 |

### 响应格式

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| created | string | 本次请求创建时间的 Unix 时间戳 |
| data | string | 输出图像数组，每项含 url、b64_json、size 或单图错误信息 |
| data.b64_json | string | `response_format=b64_json` 时返回的 Base64 图片数据 |
| data.size | string | 输出图像宽高像素值 |
| data.url | string | `response_format=url` 时返回的图片链接，24 小时内有效 |
| error | string | 请求级错误信息 |
| model | string | 本次请求使用的模型 ID |
| usage.generated_images | string | 成功生成的图片张数，不含失败图片 |
| usage.output_tokens | string | 图片输出 token，计算逻辑为 `sum(图片长*图片宽)/256` 取整 |
| usage.tool_usage.web_search | string | 联网搜索工具调用次数，仅开启联网搜索且触发时返回 |
| usage.total_tokens | string | 本次请求消耗总 token，当前与 output_tokens 一致 |

### 异步查询生成结果

`GET /v1/media/tasks/:id`

创建任务后先保留 `task_id`，再轮询该接口直到 `succeeded` 或 `failed`（图片和视频模型都适用）。建议保留退避轮询，不要高频反复调用。

创建入口：`https://www.moxing.pro/v1/media/generations`

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| task_id | string | 任务唯一标识 |
| status | string | queued / running / succeeded / failed |
| status_code | integer | queued/running 通常为 202，succeeded 为 200，failed 为 400 |
| result | object | 图片任务成功后通常包含图片地址；b64_json 模式返回 Base64 图片数据 |
| error_message | string | 失败原因 |

```bash
curl -sS "https://www.moxing.pro/v1/media/tasks/:id" \
  -H "Authorization: Bearer sk-xxxx"
```

响应示例：

```json
{
  "object": "media.task",
  "task_id": "df6b6427206441c9adaad913bee84f8e",
  "status": "succeeded",
  "status_code": 200,
  "capability": "image_generation",
  "model": "your-image-model",
  "created_at": 1774834768,
  "updated_at": 1774834790,
  "result": {
    "type": "image",
    "primary_url": "https://resource.moxing.pro/image/df6b6427206441c9adaad913bee84f8e/image-0.png",
    "urls": [
      "https://resource.moxing.pro/image/df6b6427206441c9adaad913bee84f8e/image-0.png"
    ]
  }
}
```

### 错误与限制

错误码：

| 错误码 | HTTP | 说明 |
| --- | --- | --- |
| invalid_request_error | 400 | 请求参数类型错误、缺少必填字段、size 不满足像素/宽高比限制，或 Pro 不支持的组图/联网搜索/流式字段被传入 |
| invalid_api_key | 401 | API Key 为空、格式错误或已失效 |
| insufficient_quota | 403 | 账户余额或配额不足 |
| model_not_found | 404 | 模型未开通、模型 ID 错误或当前账号无权访问 |
| rate_limit_exceeded | 429 | 触发模型或 Endpoint 限流 |
| internal_server_error | 500 | 上游内部服务异常，组图场景下可能中止后续图片生成 |
| upstream_unavailable | 502 | 模型服务暂时不可用或超时 |

限制：

- **官方接口**：`POST https://ark.cn-beijing.volces.com/api/v3/images/generations`
- **平台到上游字段映射**：`prompt` 透传为官方 prompt；`size`（2K/3K/4K 或宽x高）映射为官方 size；`extra` 内的 watermark、output_format、sequential_image_generation、sequential_image_generation_options、tools、stream、optimize_prompt_options 由平台合并为官方 JSON 字段；`n` 仅为平台兼容字段，组图数量请用 `extra.sequential_image_generation` 控制。
- **组图**：`sequential_image_generation=auto` 时可生成组图，`max_images` 范围 [1, 15]，输入参考图数量 + 最终生成图数量 <= 15。
- **联网搜索**：仅 Lite 支持 `tools.type=web_search`，实际搜索次数见 `usage.tool_usage.web_search`。
- **输入图片**：URL 或 Base64；单图不超过 30MB，总像素不超过 6000x6000，宽高比 [1/16, 16]，宽高均需大于 14px；Lite 最多 14 张参考图。
- **输出尺寸**：像素值方式默认 2048x2048，总像素范围 [3686400, 16777216]；分辨率档位支持 2K/3K/4K。
- **返回链接**：`response_format=url` 时，火山官方图片链接生成后 24 小时内有效。

---

## Doubao-Seedream-5.0-pro

- **模型 ID**：`doubao-seedream-5-0-pro-260628`
- **厂商**：豆包
- **类型**：image

Seedream-5.0-pro 是字节跳动发布的最新图像创作模型，将图像创作推进到可控生产的新阶段，主要亮点是编辑更可控、生产更落地、效果更自然。

### 能力与接口

- **图片生成**：`POST /v1/media/generations`
- 支持 `POST /v1/images/generations` 同步等待返回，也支持 `POST /v1/media/generations` 创建异步任务后轮询 `GET /v1/media/tasks/:task_id`。两种方式使用相同参数、模型范围、日志、扣费和对账链路。
- 火山方舟官方接口：`POST https://ark.cn-beijing.volces.com/api/v3/images/generations`。本模型按官方 Seedream 5.0 Pro 参数补充。
- Seedream 5.0 Pro 只生成单图，不支持组图生成、联网搜索和流式输出。

### 生成场景

- 文生图
- 单图生图 / 图片编辑
- 多图参考生图

### 请求参数

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| model | string | 是 | 模型 logical key，固定为 `doubao-seedream-5-0-pro-260628` |
| prompt | string | 是 | 提示词，支持中英文；建议不超过 300 汉字或 600 英文单词 |
| response_format | string | 否 | `url` 返回下载链接（24 小时内有效），`b64_json` 返回 Base64 编码 |
| size | string | 是 | 输出尺寸：宽高像素值（默认 1024x1024，总像素在 921600~4624220 之间、宽高比 [1/16,16]）或分辨率档位 `1K`/`2K` |
| capability | string | 是 | 能力类型，固定为 `image_generation` |
| image | string \| string[] | 否 | 输入图片，支持 URL 或 `data:image/<格式>;base64,<Base64>`；格式 jpeg/png/webp/bmp/tiff/gif/heic/heif |
| reference_images | string[] | 否 | 参考图数组，多图生图 2~10 张，平台按官方 image 数组语义处理 |
| extra | object | 否 | 上游扩展参数，放置火山方舟专有字段 |

`capability`、`image`、`reference_images`、`extra` 及以下 `extra` 内字段均属模型专属/上游扩展，放入 `extra` 或对应平台兼容字段后，平台会透传或映射到上游。

`extra` 内支持的字段：

| 参数 | 类型 | 说明 |
| --- | --- | --- |
| output_format | string | 输出格式 `jpeg`/`png`，默认 jpeg；响应 `data.output_format` 仅 Pro 返回 |
| watermark | boolean | 是否在右下角添加“AI生成”水印，官方默认 true |
| optimize_prompt_options | object | 提示词优化，`{"mode":"standard"}`；fast 模式当前不支持 |

Seedream 5.0 Pro 不支持 `sequential_image_generation`、`tools` 和 `stream`。

### 响应格式

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| created | string | 本次请求创建时间的 Unix 时间戳 |
| data | string | 输出图像数组，Pro 只返回单图；每项含 url、b64_json、output_format、size 或单图错误信息 |
| data.b64_json | string | `response_format=b64_json` 时返回的 Base64 图片数据 |
| data.output_format | string | Seedream 5.0 Pro 返回的输出图像格式 |
| data.size | string | 输出图像宽高像素值 |
| data.url | string | `response_format=url` 时返回的图片链接，24 小时内有效 |
| error | string | 请求级错误信息 |
| model | string | 本次请求使用的模型 ID |
| usage.generated_images | string | 成功生成的图片张数，不含失败图片 |
| usage.input_images | string | Seedream 5.0 Pro 返回的输入图片张数 |
| usage.output_tokens | string | 图片输出 token，计算逻辑为 `sum(图片长*图片宽)/256` 取整 |
| usage.total_tokens | string | 本次请求消耗总 token，当前与 output_tokens 一致 |

### 异步查询生成结果

`GET /v1/media/tasks/:id`

创建任务后先保留 `task_id`，再轮询该接口直到 `succeeded` 或 `failed`（图片和视频模型都适用）。建议保留退避轮询，不要高频反复调用。

创建入口：`https://www.moxing.pro/v1/media/generations`

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| task_id | string | 任务唯一标识 |
| status | string | queued / running / succeeded / failed |
| status_code | integer | queued/running 通常为 202，succeeded 为 200，failed 为 400 |
| result | object | 图片任务成功后通常包含图片地址；b64_json 模式返回 Base64 图片数据 |
| error_message | string | 失败原因 |

```bash
curl -sS "https://www.moxing.pro/v1/media/tasks/:id" \
  -H "Authorization: Bearer sk-xxxx"
```

响应示例：

```json
{
  "object": "media.task",
  "task_id": "df6b6427206441c9adaad913bee84f8e",
  "status": "succeeded",
  "status_code": 200,
  "capability": "image_generation",
  "model": "your-image-model",
  "created_at": 1774834768,
  "updated_at": 1774834790,
  "result": {
    "type": "image",
    "primary_url": "https://resource.moxing.pro/image/df6b6427206441c9adaad913bee84f8e/image-0.png",
    "urls": [
      "https://resource.moxing.pro/image/df6b6427206441c9adaad913bee84f8e/image-0.png"
    ]
  }
}
```

### 错误与限制

错误码：

| 错误码 | HTTP | 说明 |
| --- | --- | --- |
| invalid_request_error | 400 | 请求参数类型错误、缺少必填字段、size 不满足像素/宽高比限制，或传入 Pro 不支持的组图/联网搜索/流式字段 |
| invalid_api_key | 401 | API Key 为空、格式错误或已失效 |
| insufficient_quota | 403 | 账户余额或配额不足 |
| model_not_found | 404 | 模型未开通、模型 ID 错误或当前账号无权访问 |
| rate_limit_exceeded | 429 | 触发模型或 Endpoint 限流 |
| internal_server_error | 500 | 上游内部服务异常，可重试，持续异常请联系技术支持 |
| upstream_unavailable | 502 | 模型服务暂时不可用或超时 |

限制：

- **官方接口**：`POST https://ark.cn-beijing.volces.com/api/v3/images/generations`
- **平台到上游字段映射**：`prompt` 透传为官方 prompt；`size`（1K/2K 或宽x高）映射为官方 size；`extra` 内的 watermark、output_format、optimize_prompt_options 由平台合并为官方 JSON 字段；`n` 仅为平台兼容字段。
- **能力限制**：Seedream 5.0 Pro 只生成单图，不支持组图生成、联网搜索和流式输出，传 `sequential_image_generation`、`tools` 或 `stream` 会被上游拒绝。
- **输入图片**：URL 或 Base64；单图不超过 30MB，总像素不超过 6000x6000，宽高比 [1/16, 16]，宽高均需大于 14px；Pro 最多 10 张参考图。
- **输出尺寸**：像素值方式默认 1024x1024，总像素范围 [921600, 4624220]；分辨率档位支持 1K/2K。
- **返回链接**：`response_format=url` 时，火山官方图片链接生成后 24 小时内有效。

### 请求示例

```bash
# 把下面的 sk-your-api-key-here 换成你的 API Key 即可发送请求
curl --request POST https://www.moxing.pro/v1/images/generations \
  --header 'Authorization: Bearer sk-your-api-key-here' \
  --header 'Content-Type: application/json' \
  --data-raw '{
  "capability": "image_generation",
  "extra": {
    "output_format": "jpeg",
    "watermark": false
  },
  "model": "doubao-seedream-5-0-pro-260628",
  "prompt": "一张高端香水产品海报，玻璃瓶置于黑色大理石台面，柔和侧逆光，商业摄影",
  "response_format": "url",
  "size": "2K"
}'
```
