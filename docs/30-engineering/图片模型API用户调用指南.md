---
status: current
owner: Dev Team
last-reviewed: 2026-09-06
---

# 图片模型 API 用户调用指南

## 1. 入口与生命周期

图片生成统一使用 `POST /v1/images/generations`，编辑使用 `POST /v1/images/edits`。默认模式下请求
始终等待本次 Provider 调用完成并返回 HTTP `200`；客户端不接收、不查询 Provider task ID，也不存在
图片专用 `/v1/images/tasks/:task_id`。

已发布图片异步能力的模型可显式选择异步模式：请求头 `Prefer: respond-async`。受理成功返回
HTTP `202` 与平台任务 ID，结果经 `GET /v1/tasks/{task_id}` 查询（见 §6）；客户端断开不取消任务。
`Prefer: respond-async` 与 `stream=true` 互斥。

渠道类型分两类：原生 Gemini/Vertex 渠道上的 imagine 图片模型（如映射到 `gemini-3.1-flash-image`
的客户模型，见 §4），以及管理端「图片中转」渠道类型（每条渠道显式选择 `funcloud_aigc_v2` 或
`moxing_images_v1`，见 §3）。

FunCloud 虽然南向创建任务并轮询，但 adaptor 在同一请求内完成等待。Moxing 南向使用一次同步 POST。
两者都有代码固定的 10 分钟总时限，无需配置 `RELAY_TIMEOUT`；显式配置的更短正数只会提前终止。
超时或取消按普通同步图片失败/退款语义处理。调用方
不要盲目重发，因为 Provider 可能已经受理请求。Moxing 当前代码支持 Lite/Pro 固定 `2K` 单图文生图；
真实 Provider、账单与超时歧义尚未验收，管理员启用前不能把下述代码合同视为生产可用承诺。

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

## 4. FunCloud 模型兼容子集

| Provider 模型 | Prompt | 当前发布规格 | 参数 |
| --- | --- | --- | --- |
| `nano-banana-2-lite` | 最多 20000 字符 | 无参考图；单一分辨率 | 15 个宽高比（含 `auto`）；不支持 resolution/outputFormat |
| `nano-banana-2` | 最多 20000 字符 | 无参考图；固定 `resolution=1K` | `outputFormat=jpg/png` |
| `seedream-5.0-lite` | 3–3000 字符 | 无参考图；固定 `2K/basic` | 8 个宽高比 |
| `seedream-5.0-pro` | 3–3000 字符 | 无参考图；固定 `1K/basic` | 7 个宽高比 |

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
两个模型当前都只允许 prompt 1—3000 字符、`n` 省略或为 `1`、固定 `2K`、URL 响应。参考图、组图、
联网搜索、Base64、stream、任意宽高、输出格式、watermark、未知顶层字段和非空 `extra_fields` 均在
发送 Provider 请求前返回 HTTP `400`。客户模型名本身不赋予能力；映射目标不在代码登记表时同样拒绝。
Pro `1K` 与按实际像素结算尚未开放，不能通过修改请求或 Param Override 绕过固定规格。

## 6. 请求字段与当前限制

当前 FunCloud 渠道尚未发布参考图能力。由于 input-image 价格和失败扣费规则仍需核实，
客户端传入 `extra_fields.reference_images` 会明确返回 HTTP `400`；不会静默删除或改义。
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
- `Idempotency-Key` 仅异步模式支持（同步请求携带返回 `400`）：同 key 等价请求重放原任务 ID，
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
