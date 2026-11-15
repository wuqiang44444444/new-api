---
page-id: account-balance
kind: api-reference
last-verified: 2026-11-15
operations:
  - getUserBalance
---

# 查询账户余额

`GET /api/user/balance` · Bearer API Key 鉴权

查询 API Key 所属账户的当前钱包余额。同一账户下的 API Key 查询的是同一份钱包余额，单个 Key 的额度单独管理。此接口只读，查询不会扣费。

## 请求

在 `Authorization` 请求头中传入 `Bearer` 和你的 API Key。无需登录控制台，也无需传查询参数或请求体。

```bash
curl "{{SITE_BASE_URL}}/api/user/balance" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}"
```

## 响应

查询成功返回 HTTP `200`，响应类型为 `application/json`。

```json
{
  "success": true,
  "data": {
    "balance": 12.345678,
    "currency": "USD"
  }
}
```

| 字段            | 类型    | 说明                                     |
| --------------- | ------- | ---------------------------------------- |
| `success`       | boolean | 查询成功时为 `true`                      |
| `data`          | object  | 查询成功时返回的余额信息                 |
| `data.balance`  | number  | 账户钱包余额，最低为 `0`，可包含小数     |
| `data.currency` | string  | 余额币种，固定为 `USD`                   |

API Key 缺失、无效或已停用时返回 `401`，所属账户不可用时返回 `403`。触发限流时返回 `429`，请退避后重试。鉴权服务异常时返回 `500`，可以稍后重试。

读取余额失败时也可能收到 HTTP `200`。请先检查 `success`，值为 `false` 时读取 `message`，仅在成功时读取余额。
