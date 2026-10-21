---
page-id: authentication
kind: guide
last-verified: 2026-10-21
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

- 面向其他用户的应用：由服务端保存并使用 Key，不分发到浏览器或安装包。
- 个人桌面工具、CLI：可保存在受信任电脑的凭据库或受保护的本地配置中，使用专用、可撤销的 Key。
- 含 Key 的本地配置不要同步到公开仓库、共享网盘或诊断报告；仅当前用户可读取。
- 使用不同 Key 隔离开发、测试和生产环境。
- 怀疑泄露时立即撤销并轮换 Key。
- 不要把 Key 放入 URL 查询参数、异常文本、截图或客户端分析事件。
- 不要把控制台会话 Cookie 当作 API Key 使用。

## 合同绑定与模型访问

启用合同绑定的 API Key 按合同当前有效范围授权新调用，同时仍受 Key 自身模型限制约束。
使用发起请求的同一 Key 查询模型目录；其他 Key 或控制台可见的模型不一定可由此 Key 调用。
合同范围没有可用模型时先核对授权；合同授权加载异常返回服务错误，不能当作模型不存在。

## 任务与素材使用哪一把 Key

图片异步任务与 ModelArk 视频任务绑定创建时的用户和应用。请保存 Key 的内部标识，并用原 Key 查询；
同账号换另一把 Key 不等于同一个应用。重试创建还必须遵守对应接口的幂等规则。

Seedance 与 H3 标准视频交付就绪后，授权查询返回本站 OSS 预签名下载 URL；下载该完整地址无需
附带平台或 Provider Key。既有 `/v1/videos/{task_id}/content` 入口仍需原 Key，就绪后返回 302；
跟随跳转时不要将平台鉴权转发给对象存储。签名地址本身带下载授权，勿公开分享或写入普通日志。
每次签发有效 24 小时；续签仍须用有效原 Key 查询原任务，且对象仍在保留期内。
详见[视频交付合同](api-reference/videos/modelark#下载视频或末帧)。

English: hosted standard video result URLs authorize downloads without a gateway or provider key for
86400 seconds from issuance. Task queries and the content endpoint still require the original valid API key.
Follow content redirects without forwarding that key to storage; an authorized query can renew a URL while
the object is retained. Treat the complete signed URL as a download credential.

已有任务查询等历史资源操作继续按原归属与任务事实校验，不因当前合同模型下架而自动失去访问权；
Key 本身仍须有效。重新生成或派生新任务属于新调用，仍须满足当前授权。素材代理按模型发起新操作，
不属于历史任务查询例外。

平台托管素材按账号共享，属于同账号的 Key 可以访问；代理素材由调用方管理 ID。
这与任务隔离规则不同，详见[素材与素材组](api-reference/assets)。

## 常见鉴权错误

| 状态  | 常见原因                 | 建议                                  |
| ----- | ------------------------ | ------------------------------------- |
| `401` | 缺少、无效或已撤销的 Key | 检查请求头和 Key 状态                 |
| `403` | Key 无权访问模型或能力   | 查询模型列表并检查权限                |
| `429` | 频率、并发或额度限制     | 按响应信息退避，不要更换随机 Key 绕过 |

鉴权失败不应无限重试。修复凭据或权限后再发送请求。
