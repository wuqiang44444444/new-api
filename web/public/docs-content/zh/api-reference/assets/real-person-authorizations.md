---
page-id: assets-authorizations
kind: api-reference
last-verified: 2026-07-29
operations:
  - createRealPersonAuthorization
  - getRealPersonAuthorization
  - revokeRealPersonAuthorization
  - retryRealPersonAuthorization
---

# 真人授权

涉及真人媒体的素材可能要求先获得明确授权。API 客户端创建授权后，应把返回的同意页面交给本人完成，不得代替本人确认。

## 创建授权

`POST /v1/real-person-authorizations` · `application/json`

```bash
curl "{{OPENAI_BASE_URL}}/real-person-authorizations" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "{{MODEL_ID_PLACEHOLDER}}",
    "locale": "zh"
  }'
```

创建响应包含授权 ID、状态、同意页面地址和可能的过期时间。不要抓取、改写或嵌入同意页面表单。

## 查询状态

```bash
curl "{{OPENAI_BASE_URL}}/real-person-authorizations/authorization-placeholder" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}"
```

只有授权达到有效状态后，才能把其 ID 用于要求真人授权的素材请求。拒绝、撤销、删除或过期的授权不能继续使用。

## 撤销与重试

```text
POST /v1/real-person-authorizations/{authorization_id}/revoke
POST /v1/real-person-authorizations/{authorization_id}/retry
```

撤销是业务敏感操作。重试只适用于公开状态允许重试的授权；`409 authorization_not_retryable` 表示当前状态不可重试。

## 隐私边界

客户端只应保存业务所需的授权 ID、公开状态和时间。不要记录同意 token、回执 token、验证服务原文或个人敏感材料。
