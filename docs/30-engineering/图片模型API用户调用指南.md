---
status: current
owner: Dev Team
last-reviewed: 2026-08-18
---

# 图片模型 API 用户调用指南

## 1. 入口与生命周期

图片生成统一使用 `POST /v1/images/generations`，编辑使用 `POST /v1/images/edits`。普通图片请求
始终等待本次 Provider 调用完成并返回 HTTP `200`；客户端不接收、不查询 Provider task ID，也不存在
图片专用 `/v1/images/tasks/:task_id`。

FunCloud 中转异步图片渠道虽然南向创建任务并轮询，但 adaptor 在同一请求内完成等待。等待上限由部署
的 `RELAY_TIMEOUT` 决定；超时或取消按普通同步图片失败/退款语义处理。调用方不要盲目重发，因为
Provider 可能已经受理请求。

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

## 3. FunCloud 模型兼容子集

| Provider 模型 | Prompt | 当前发布规格 | 参数 |
| --- | --- | --- | --- |
| `nano-banana-2-lite` | 最多 20000 字符 | 无参考图；单一分辨率 | 15 个宽高比（含 `auto`）；不支持 resolution/outputFormat |
| `nano-banana-2` | 最多 20000 字符 | 无参考图；固定 `resolution=1K` | `outputFormat=jpg/png` |
| `seedream-5.0-lite` | 3–3000 字符 | 无参考图；固定 `2K/basic` | 8 个宽高比 |
| `seedream-5.0-pro` | 3–3000 字符 | 无参考图；固定 `1K/basic` | 7 个宽高比 |

客户模型名可以不同；管理员通过 `model_mapping` 精确映射到 Provider 模型。请以 `GET /v1/models`
的实时结果确认当前 Key 是否开放模型。

## 4. 请求字段与当前限制

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

`callbackUrl`、`b64_json`、`stream=true`、未知字段和 Provider 私有 JSON 均返回 HTTP `400`。
更高分辨率/质量档位也会在请求校验阶段拒绝，直到对应预扣计费倍率完成配置和验收。

`n` 必须为 `1`；不拆分多图请求。成功结果必须恰好包含一个 URL，否则返回上游响应错误。

## 5. 错误与重试

| 状态 | 含义 |
| --- | --- |
| `400` | 请求字段、模型能力或 Provider 参数错误 |
| `502` | Provider 鉴权、余额、任务状态、非法 JSON 或结果合同错误 |
| `504` | `RELAY_TIMEOUT` 或客户端 context 取消/超时 |

客户端应记录自己的 request ID 和业务订单。收到 `504` 后不要自动重复创建；如业务决定重试，需
接受重复生成和重复计费风险。

## 6. 安全

- API Key 只放在服务端环境变量或密钥管理系统中；不要写入 URL、前端代码或日志。
- 不要记录完整签名 URL、参考图 URL、Base64、提示词或 Provider 原始响应。
- 结果 URL 由 Provider 控制有效期和访问权限，业务应按自己的存储与合规策略处理。
