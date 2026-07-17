# 单次请求信息查询

**文档中心 / API 文档 / 单次请求信息查询**

根据请求 ID 查询单次请求的扣费明细与 Token 用量。

**场景**：服务端对账

**接口**：`GET /v1/account/usage-records`

---

## 接口说明

按 `request_id` 查询单次模型调用的扣费与 token 用量；仅返回费用相关字段，不含 request/response body。

鉴权使用集成 Token（`itk-mxai-...`），`request_id` 则来自此前的模型 API 调用。

---

## request_id 说明

调用模型 API（如 `POST /v1/chat/completions`，使用 `sk-mxai-...`）成功后，从 HTTP 响应头读取 `X-Oneapi-Request-Id`，保存到业务侧后再用于本接口查询。

| 字段 | 位置 | 说明 | 能否用于本接口 |
|------|------|------|---------------|
| `X-Oneapi-Request-Id` | 响应头 | 平台为本次模型调用生成的追踪 ID | ✅ 用于本接口 query 参数 `request_id` |
| `X-Oneapi-Client-Request-Id` | 响应头 | 若请求时传过客户端 ID，原样回显 | ❌ 不能用于本接口 |

### 读取响应头示例（模型调用）

```bash
curl -sS -D - -o /dev/null "https://moxing.pro/v1/chat/completions" \
  -H "Authorization: Bearer sk-mxai-your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "your-text-model",
    "messages": [{ "role": "user", "content": "你好" }]
  }'
# 在响应头中查找 X-Oneapi-Request-Id: 2026070710304512345678
```

> 随后使用集成 Token 查询：`GET /v1/account/usage-records?request_id={X-Oneapi-Request-Id}`

---

## 请求

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `request_id` | string | ✅ 是 | 单次模型 API 调用的请求 ID，取值来自响应头 `X-Oneapi-Request-Id`；只能查询本账户下的记录。示例：`2026070710304512345678` |

---

## 代码示例

```bash
curl -sS "https://moxing.pro/v1/account/usage-records?request_id=2026070710304512345678" \
  -H "Authorization: Bearer itk-mxai-your-integration-token"
```

---

## 响应示例

### success

```json
{
  "code": 200,
  "success": true,
  "message": "",
  "data": {
    "queried_at": "2026-07-06T18:04:00+08:00",
    "request_id": "2026070710304512345678",
    "status": "success",
    "model_product_name": "gpt-4o",
    "token_id": 12,
    "token_name": "生产环境 Key",
    "quota": 1200,
    "quota_yuan": 0.12,
    "charged_quota": 960,
    "charged_yuan": 0.096,
    "original_quota": 1200,
    "original_yuan": 0.12,
    "discount_ratio": 0.8,
    "input_tokens": 800,
    "output_tokens": 200,
    "total_tokens": 1000,
    "created_at": "2026-07-06T09:58:12Z",
    "finished_at": "2026-07-06T09:58:14Z"
  }
}
```

### error (400)

```json
{
  "code": 400,
  "success": false,
  "message": "request_id is required",
  "error_code": "invalid_parameter",
  "data": null
}
```

### error (404)

```json
{
  "code": 404,
  "success": false,
  "message": "request not found",
  "error_code": "request_not_found",
  "data": null
}
```

---

## status 枚举

| status | 费用字段 | 说明 |
|--------|---------|------|
| `pending` | `quota=0`，`charged_quota=0` | 尚未结算完成 |
| `success` | 按实际结算值返回 | 已成功扣费 |
| `failed` | `quota=0`，`charged_quota=0` | 调用失败，无扣费 |

---

## 响应字段（data）

| 字段 | 类型 | 说明 |
|------|------|------|
| `queried_at` | string | 查询时刻（Asia/Shanghai） |
| `request_id` | string | 与 query 参数一致，表示查到的调用记录 |
| `status` | string | `pending` / `success` / `failed` |
| `model_product_name` | string | 模型产品名 |
| `token_id` | int | 使用的 Key ID |
| `token_name` | string | 使用的 Key 名称 |
| `quota` | int64 | 结算额度；pending/failed 时为 0 |
| `quota_yuan` | float64 | 结算额度（元） |
| `charged_quota` | int64 | 折后实际扣费，对账优先使用此字段 |
| `charged_yuan` | float64 | 折后扣费（元） |
| `original_quota` | int64 | 折前额度 |
| `original_yuan` | float64 | 折前（元） |
| `discount_ratio` | float64 | 折扣比例；无折扣时固定为 1 |
| `input_tokens` | int | 输入 token 数 |
| `output_tokens` | int | 输出 token 数 |
| `total_tokens` | int | 总 token 数 |
| `created_at` | string | 请求创建时间（UTC ISO 8601） |
| `finished_at` | string \| null | 完成时间；未完成时为 null |

---

## 错误处理

| code | HTTP | 场景 | message 示例 | error_code |
|------|------|------|-------------|------------|
| 400 | 400 | 未传 `request_id` | `request_id is required` | `invalid_parameter` |
| 401 | 401 | 未提供 Token 或 Token 无效/已禁用 | `invalid integration token` | `invalid_token` |
| 403 | 403 | 用户封禁/禁用 | `account disabled` | `account_disabled` |
| 404 | 404 | `request_id` 不存在或不属于当前用户 | `request not found` | `request_not_found` |
| 500 | 500 | 服务内部错误 | `internal error` | `internal_error` |

---

## 鉴权与响应格式

- **Base URL**：`https://moxing.pro/v1`
- **账户接口路径**：`/v1/account/*`

在控制台 **秘钥管理** 创建集成 Token（`itk-mxai-...`），请求头：

```
Authorization: Bearer itk-mxai-your-integration-token
```

> ⚠️ 请使用**服务端托管集成 Token**，不要用模型 API Key（`sk-mxai-`）调用账户 API。

### 响应格式

响应体为 `{ code, success, message, data }`：
- HTTP 状态码与 `code` 一致
- 失败时 `data` 为 `null`

> 💡 金额以 `quota` 整数为准，`*_yuan` 仅为展示换算。
