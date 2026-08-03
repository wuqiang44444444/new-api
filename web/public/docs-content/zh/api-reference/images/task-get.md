---
page-id: images-task-get
kind: api-reference
last-verified: 2026-07-31
operations:
  - getImageTask
---

# 查询图片任务

`GET /v1/images/tasks/{task_id}` · Bearer 鉴权

## 请求

```bash
curl "{{OPENAI_BASE_URL}}/images/tasks/image-task-placeholder" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}"
```

`task_id` 来自图片生成或编辑的异步响应。任务仅对拥有相应权限的鉴权主体可见。

## 任务响应

```json
{
  "id": "image-task-placeholder",
  "object": "image.generation.task",
  "status": "completed",
  "created_at": 1760000000,
  "completed_at": 1760000012,
  "model": "available-image-model",
  "result": {
    "created": 1760000012,
    "data": [{ "url": "https://example.com/generated-image.png" }]
  }
}
```

## 状态

- `queued`：已受理，等待执行。
- `in_progress`：正在生成。
- `completed`：`result` 包含结果。
- `failed`：`error.code` 和 `error.message` 包含公开错误。当前可信终态结果违反交付合同时同样
  返回 `image_generation_failed`。
- `unknown`：上游状态暂时无法映射，继续采用有限轮询。

## 轮询

使用 1 到 2 秒起步的退避并加入抖动。到达终态后停止轮询。任务超时不等于失败；保留 ID，允许稍后继续查询。
