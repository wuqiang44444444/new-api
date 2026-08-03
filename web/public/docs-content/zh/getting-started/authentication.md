---
page-id: authentication
kind: guide
last-verified: 2026-07-29
operations: []
---

# 鉴权

公开 API 使用 Token 鉴权。绝大多数入口接受 HTTP `Authorization` 请求头。

## Bearer 鉴权

```http
Authorization: Bearer {{API_KEY_PLACEHOLDER}}
```

对于 Anthropic Messages 协议，也可以按客户端要求使用 `x-api-key`，同时发送受支持的 `anthropic-version`。不要在同一请求中混用不相关协议的认证方式。

## 安全要求

- 只在服务端保存 API Key。
- 使用不同 Key 隔离开发、测试和生产环境。
- 怀疑泄露时立即撤销并轮换 Key。
- 不要把 Key 放入 URL 查询参数、异常文本、截图或客户端分析事件。
- 不要把控制台会话 Cookie 当作 API Key 使用。

## 常见鉴权错误

| 状态  | 常见原因                 | 建议                                  |
| ----- | ------------------------ | ------------------------------------- |
| `401` | 缺少、无效或已撤销的 Key | 检查请求头和 Key 状态                 |
| `403` | Key 无权访问模型或能力   | 查询模型列表并检查权限                |
| `429` | 频率、并发或额度限制     | 按响应信息退避，不要更换随机 Key 绕过 |

鉴权失败不应无限重试。修复凭据或权限后再发送请求。
