---
page-id: async-tasks
kind: guide
last-verified: 2026-07-29
operations: []
---

# 异步任务

图片、视频和素材操作可能先返回任务或资源 ID，再异步完成。创建接口的 HTTP 成功只表示请求已受理，不表示媒体已经可用。

## 状态机

常见状态包括 `queued`、`in_progress`、`completed` 和 `failed`。具体协议可能使用同义状态，客户端应以对应 API Reference 为准。

```text
queued -> in_progress -> completed
                    \-> failed
```

## 轮询建议

1. 保存创建响应中的任务 ID。
2. 先等待 1 到 2 秒，再开始查询。
3. 使用带抖动的退避，避免固定高频轮询。
4. 遇到 `completed` 后读取结果或内容地址。
5. 遇到 `failed` 后记录公开错误码并停止轮询。
6. 设置客户端总超时；超时后保留任务 ID，允许稍后继续查询。

## 任务隔离

任务和素材都绑定创建它们的用户或 Token 权限。不要把另一个用户的任务 ID 当作可共享下载链接。返回 `404` 时，同时检查 ID、接口族和鉴权主体。

## 内容下载

视频内容代理要求任务已经完成。内容 URL 可能有有效期；下载或转存前应检查 HTTP 状态和媒体类型，不要把错误 JSON 当作视频文件保存。
