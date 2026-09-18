---
status: current
owner: Dev Team
last-reviewed: 2026-09-17
---

# 图片模型 API 用户调用指南

## 1. 入口与生命周期

图片生成统一使用 `POST /v1/images/generations`，编辑使用 `POST /v1/images/edits`。默认模式下请求
始终等待本次 Provider 调用完成并返回 HTTP `200`；客户端不接收、不查询 Provider task ID，也不存在
图片专用 `/v1/images/tasks/:task_id`。

已发布图片异步能力的模型可显式选择异步模式：请求头 `Prefer: respond-async`。受理成功返回
HTTP `202` 与平台任务 ID，结果经 `GET /v1/tasks/{task_id}` 查询（见 §6）；客户端断开不取消任务。
OpenAI／Azure 同时传 `stream=true` 时仍优先异步受理，由后台接收上游流式结果；未传异步偏好时
沿用原生流式响应。Gemini／Vertex 和图片中转的异步偏好仍与 `stream=true` 互斥。未接入平台任务的其他渠道忽略异步偏好，继续原生响应。
原生生成、JSON／multipart 编辑及 mask 沿用已有支持；异步需要已启用私有对象存储和后台图片任务。

统一图片合同的渠道类型分两类：原生 Gemini/Vertex 渠道上的 imagine 图片模型（如映射到 `gemini-3.1-flash-image`
的客户模型，见 §4），以及管理端「图片中转」渠道类型（每条渠道显式选择 `funcloud_aigc_v2` 或
`moxing_images_v1`，见 §3）。

FunCloud 虽然南向创建任务并轮询，但 adaptor 在同一请求内完成等待。Moxing 南向使用一次同步 POST。
两者都有代码固定的 10 分钟总时限，无需配置 `RELAY_TIMEOUT`；显式配置的更短正数只会提前终止。
超时或取消按普通同步图片失败/退款语义处理。调用方
不要盲目重发，因为 Provider 可能已经受理请求。Moxing 当前代码支持 Lite/Pro 固定 `2K` 单图生成与编辑；
真实 Provider、账单与超时歧义尚未验收，管理员启用前不能把下述代码合同视为生产可用承诺。

### 原生 GPT Image JSON 编辑

OpenAI 原生 GPT Image 2、2.5 的 JSON 编辑均要求 `images` 对象数组，单图也保留数组。
例如 `{"images":[{"image_url":"https://example.com/reference.png"}]}`；URL 或 Data URL 放在
字符串 `image_url` 内，不使用字符串数组，也不嵌套 `{url: ...}`。每个元素在 `image_url` 与
当前原生服务可访问的 `file_id` 中恰好选一；本站 Batch 文件 ID 不能用于此处。

模型元数据的 `api.image.edit.content_type=application/json` 描述推荐编码；`images.item_type=object`、
`images[].image_url` 和参数级 `required_one_of` 描述对象结构，最多 16 张输入、输出 `n` 为 1～10。
可选 `mask` 为同形引用对象。multipart 上传继续使用 `image` / `image[]` 文件，不受 JSON 字段名影响。
同步默认 Base64，显式 `response_format=url` 时由平台转换交付；2.5 质量档位包含 `xhigh`、`max`，2 不包含。
客户别名通过已有模型映射获得公开参数，运行时不新增按名称猜测或格式转换。

下面 Gemini/Vertex、FunCloud、Moxing 的字符串引用合同及 14 张预算不适用于原生 OpenAI。
完整示例见 `web/public/docs-content/zh/api-reference/images/edits.md`；协议依据为
[OpenAI 图片编辑接口](https://developers.openai.com/api/reference/resources/images/methods/edit)。

## 2. 最小请求

```bash
export NEWAPI_BASE_URL="https://api.example.com"
export NEWAPI_API_KEY="sk-your-key"

curl -sS "$NEWAPI_BASE_URL/v1/images/generations" \
  -H "Authorization: Bearer $NEWAPI_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "nano-banana-2",
    "prompt": "一只放在白色桌面上的蓝色陶瓷杯，产品摄影",
    "n": 1,
    "size": "1K",
    "response_format": "url"
  }'
```

成功响应：

```json
{
  "created": 1785207890,
  "data": [{"url": "https://example.com/generated-image.png"}]
}
```

URL 可能有有效期，需要长期使用时请及时下载到你控制的存储。

## 3. Gemini/Vertex 图片模型（gemini_image 族）

客户模型由管理员映射到 imagine 登记模型（例如 `nano-banana-2-gemini → gemini-3.1-flash-image`）。
两个操作均已发布；逐字段合同以模型详情 `api.image` 投影为唯一权威。

- `n` 恒为 `1`；`response_format` 默认 `b64_json`（显式 `url` 需要平台对象存储，返回 300 秒
  签名 URL）。
- `size` 接受 `auto` 或模型公开的 `WxH`；不接受 `a:b`。网关使用精确宽高比与分辨率档，
  只做等比例缩放到所请求像素；不支持的规格事前 400，上游返回比例不符时报交付错误，不裁切或拉伸。
- 未发布字段显式 `400`：`quality`、`style`、`background`、`moderation`、`output_format`、
  `output_compression`、`watermark`、`input_fidelity`、`partial_images`、`stream=true`、`mask`
  与任何未知顶层字段。
- 编辑（`/v1/images/edits`）支持 multipart `image`/`image[]` 文件、JSON `images` 数组
  （Data URL 或 HTTPS URL）或单图 `image` 字符串；最多 14 张，二进制单张 ≤ 20 MB、合计 ≤ 50 MB
  （按解码字节）；HTTPS URL 原样交给 Provider，网关不下载。

```bash
curl -sS "$NEWAPI_BASE_URL/v1/images/edits" \
  -H "Authorization: Bearer $NEWAPI_API_KEY" \
  -F "model=nano-banana-2-gemini" \
  -F "prompt=把天空改成日落" \
  -F "image=@input.png"
```

### Gemini 3.1 Flash-Lite Image

Lite 沿用标准 `POST /v1/images/generations` 与 `POST /v1/images/edits`，客户端无需使用 Google
原生请求。两者显式设置 `response_format=url`，成功通过 `data[].url` 返回图片。
生成使用 JSON；编辑支持现有 JSON 图片引用或 multipart `image`/`image[]` 文件。

Lite 的标准接口目前仅发布 `size=auto`（也可省略）和 `size=1024x1024`，以模型详情的 `size.enum`
为准。前者由 Provider 决定原生 1K 输出比例，后者请求原生 1K 正方形；网关不缩放、裁切或重新编码。
不接受 `size=1K`、任意 WxH 或 2K/4K；不把其他 Gemini 型号的像素表套用到 Lite。
Vertex Lite 每张内联编辑图最多 7,000,000 解码字节，参数 `max_decoded_bytes` 发布该限制；
客户端仍提交标准 `image` / `images` 或 multipart 文件，南向差异由网关适配。

代码已登记及通过自动化测试不代表所有渠道均已完成真实履约。部署前应核对当前模型列表覆盖项，
并分别验证生成、编辑、适用的异步执行及 URL 交付。Gemini 按实际 Token 用量计费，文本与图片输出
单价可能不同；未配置 ModelPrice 不能据此认定缺价，亦不能据此改成按张收费。

## 4. FunCloud 模型兼容子集

| Provider 模型 | Prompt | 当前发布规格 | 参数 |
| --- | --- | --- | --- |
| `nano-banana-2-lite` | 最多 20000 字符 | 编辑最多 10 张；单一分辨率 | 15 个宽高比（含 `auto`）；不支持 resolution/outputFormat |
| `nano-banana-2` | 最多 20000 字符 | 编辑最多 14 张；固定 `resolution=1K` | `outputFormat=jpg/png` |
| `seedream-5.0-lite` | 3–3000 字符 | 编辑最多 14 张；固定 `2K/basic` | 8 个宽高比 |
| `seedream-5.0-pro` | 3–3000 字符 | 编辑最多 10 张；固定 `1K/basic` | 7 个宽高比 |

客户模型名可以不同；管理员通过 `model_mapping` 精确映射到 Provider 模型。请以 `GET /v1/models`
的实时结果确认当前 Key 是否开放模型。

## 5. Moxing 兼容子集

Moxing 客户模型名可由管理员定义；选择 `moxing_images_v1` 的一条图片中转渠道可在同一 Key 下承载
两个独立客户别名：

| 客户模型 | Provider 模型 | 固定规格 | 默认客户价 |
| --- | --- | --- | --- |
| `seedream-5-moxing` | `doubao-seedream-5-0-260128` | Lite、`2K`、单图 URL | `$0.035/次` |
| `seedream-5-pro-moxing` | `doubao-seedream-5-0-pro-260628` | Pro、`2K`、单图 URL | `$0.09/次` |

客户端仍只提交客户模型名：

```json
{
  "model": "seedream-5-moxing",
  "prompt": "一只蓝色陶瓷杯的产品摄影",
  "n": 1,
  "size": "2K",
  "response_format": "url"
}
```

客户模型和 `model_mapping` 均由管理员配置；选择协议不会创建或改写 mapping。代码只要求每个渠道模型
经过 NEWAPI 原生映射链后，最终落到所选协议登记的 Provider profile。客户模型直接使用 Provider 模型名
时可以不配置映射。
两个模型当前都只允许 prompt 1—3000 字符、`n` 省略或为 `1`、固定 `2K`、URL 响应。组图输出、
联网搜索、Base64、stream、任意宽高、输出格式、watermark、未知顶层字段和非空 `extra_fields` 均在
发送 Provider 请求前返回 HTTP `400`。客户模型名本身不赋予能力；映射目标不在代码登记表时同样拒绝。
Pro `1K` 与按实际像素结算尚未开放，不能通过修改请求或 Param Override 绕过固定规格。

## 6. 请求字段与当前限制

FunCloud 与 Moxing 的编辑适配已实现，使用 `/v1/images/edits` 的标准 `image/images` 或 multipart 文件；
客户端传入 `extra_fields.reference_images` 仍明确返回 HTTP `400`。FunCloud 字节输入通过 OSS 转 URL，
Moxing 字节直接转 Data URL；异步 FunCloud 复用已存输入对象。Lite 最多 14 张、Pro 最多 10 张，
Nano Lite 最多 10 张、Nano 2 最多 14 张。共同预算为单张 20 MiB、总计 50 MiB；FunCloud Seedream
单张为 10,000,000 字节，Moxing 字节图片另验证尺寸。具体真实 Provider 账单及生产发布仍须验收。

Pro 多参考图必须配置含内部 `param("input_image_count")` 的模型计费表达式；未配置表达式时，
多参考图在预扣前拒绝，单参考图可沿用已配置固定价。同步和异步共用标准转换；异步查询支持结果托管。
固定规格示例：

```json
{
  "model": "seedream-5.0-lite",
  "prompt": "一只蓝色陶瓷杯的产品摄影",
  "n": 1,
  "size": "2K",
  "quality": "basic",
  "response_format": "url",
  "extra_fields": {"aspect_ratio": "1:1"}
}
```

`callbackUrl`、`b64_json`、显式提交的 `stream`（包括 `false`）、未知字段和 Provider 私有 JSON 均返回 HTTP `400`。
更高分辨率/质量档位也会在请求校验阶段拒绝，直到对应预扣计费倍率完成配置和验收。

`n` 必须为 `1`；不拆分多图请求。成功结果必须恰好包含一个 URL，否则返回上游响应错误。

## 7. 异步模式与任务查询

```bash
curl -sS "$NEWAPI_BASE_URL/v1/images/generations" \
  -H "Authorization: Bearer $NEWAPI_API_KEY" \
  -H "Prefer: respond-async" \
  -H "Idempotency-Key: order-2026-09-05-0001" \
  -H "Content-Type: application/json" \
  -d '{"model":"nano-banana-2-gemini","prompt":"雾中灯塔","size":"1024x1024"}'
```

受理成功：

```json
HTTP 202
{
  "created": 1785207890,
  "id": "task_xxxxxxxx",
  "object": "image_task",
  "status": "queued",
  "query_url": "/v1/tasks/task_xxxxxxxx"
}
```

查询 `GET /v1/tasks/{task_id}`（同一 API Key；任务按 user + 应用隔离）：

- `status`：`queued | in_progress | succeeded | failed | expired | unknown`；
- 已登记结果（含 `unknown` 下的部分结果）通过 `data[]` 逐张给出 `status`（`available/deleted/unavailable`）与 `url`（300 秒
  有效，附带 `url_expires_at`；过期后重新查询即续签）或创建时显式 `b64_json` 的原文；`deleted`
  表示对象已被部署方删除（不影响其余图片），`unavailable` 表示暂不可访问、稍后重查；
- `failed/expired` 给出脱敏 `error`；`unknown` 表示结果待核实（不会自动退款），联系平台核实；
- `Idempotency-Key` 仅实际异步提供任务幂等（未传 Prefer 时仍返回 `400`；未提供异步执行的渠道忽略 Prefer 时不认领）：同 key 等价请求重放原任务 ID，
  不同请求体返回 `409`；未携带 key 视为新的创建意图。
- 背压：应用未完成任务超限返回 `429`，平台排队容量耗尽返回 `503`；两者都没有受理、扣费或发送，
  可安全稍后重试。

## 8. 错误与重试

| 状态 | 含义 |
| --- | --- |
| `400` | 请求字段、模型能力或 Provider 参数错误 |
| `502` | Provider 鉴权、余额、任务状态、非法 JSON 或结果合同错误 |
| `504` | 图片中转固定 10 分钟上限、更短的全局上限或客户端 context 取消/超时 |

客户端应记录自己的 request ID 和业务订单。收到 `504` 后不要自动重复创建；如业务决定重试，需
接受重复生成和重复计费风险。

## 9. 安全

- API Key 只放在服务端环境变量或密钥管理系统中；不要写入 URL、前端代码或日志。
- 不要记录完整签名 URL、参考图 URL、Base64、提示词或 Provider 原始响应。
- 结果 URL 由 Provider 控制有效期和访问权限，业务应按自己的存储与合规策略处理。

## 图片返回格式

两个标准图片入口统一使用可选 `response_format=url|b64_json`。省略时保持各模型与执行模式原有
默认格式，不能全局假定 URL。multipart 对应 `-F 'response_format=url'`。
已有目标格式直接交付；Base64 转 URL 保存到平台存储并签发 300 秒 URL，返回 `url_expires_at`；
Provider URL 直接复用，有效期由 Provider 决定。URL 转 Base64 复用受保护下载。
GPT Image 上游不支持的格式字段由网关消费，不转发；原生 SSE 事件保持原有格式。
显式异步查询遵循受理时冻结的返回格式；同步结果没有任务接口续签或后台交付恢复。
已知需要存储的请求预检失败返回 503；可信生成成功后的交付失败返回 502 `image_delivery_failed`，
按生成事实结算一次，不自动退款或重生成。格式转换计入服务端处理耗时，客户端后续下载不计入。


## 同步图片交付失败时取回结果

显式 `response_format` 的同步 JSON 转换失败时，HTTP 为 `502`，错误码为 `image_delivery_failed`。
生成已完成且只结算一次；失败正文同时保留 `data` 中的原始图片结果，可能为 `b64_json` 或原始 `url`，
并包含 `requested_response_format`。客户端应保存错误正文，解码 Base64 或及时下载 URL；不要把再次
发送生成 POST 当成下载重试。成功响应仍严格采用请求格式。

网关在本次交付预算内重试结果 GET、同对象键上传或签名，最多三次；已收到的图片通过私有临时文件
转换，不再因同步交付层原有 50 MiB 单图／512 MiB 响应阈值被丢弃。请求退出后临时文件删除，
不建立同步任务查询；断连未收到正文、Provider URL 已过期的情况不能由该机制保证恢复。
