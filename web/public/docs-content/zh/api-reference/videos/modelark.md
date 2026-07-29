---
page-id: videos-modelark
kind: api-reference
last-verified: 2026-07-29
operations:
  - createModelArkVideoTask
  - listModelArkVideoTasks
  - getModelArkVideoTask
  - deleteModelArkVideoTask
  - getVideoContent
---

# ModelArk 视频

ModelArk 使用 `/api/v3/contents/generations/tasks` 原生合同。它不是 `/v1` 子路径，字段也不能与 Kling 或即梦请求混用。

## 创建任务

`POST /api/v3/contents/generations/tasks` · `application/json`

```bash
curl "{{SITE_BASE_URL}}/api/v3/contents/generations/tasks" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "{{MODEL_ID_PLACEHOLDER}}",
    "content": [
      {"type": "text", "text": "镜头缓慢掠过清晨的山谷"}
    ],
    "duration": 5,
    "ratio": "16:9",
    "generate_audio": false
  }'
```

`model` 和非空 `content` 必填。`content` 可包含该模型支持的文本与媒体 URL 条目。`duration`、`resolution`、`ratio`、音频、草稿和服务档位等可选字段必须以当前模型能力为准。

## 列表与查询

```bash
curl "{{SITE_BASE_URL}}/api/v3/contents/generations/tasks" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}"
```

```bash
curl "{{SITE_BASE_URL}}/api/v3/contents/generations/tasks/video-task-placeholder" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}"
```

列表支持服务端公开的分页查询参数。查询响应中的状态与结果地址是权威来源。

## 下载内容

任务完成后可通过受鉴权内容代理下载：

```bash
curl "{{OPENAI_BASE_URL}}/videos/video-task-placeholder/content" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  --output result.mp4
```

先检查响应状态和 `Content-Type`。任务未完成、ID 不存在或无权限时会返回 JSON 错误，不是视频字节。

## 删除任务

`DELETE /api/v3/contents/generations/tasks/{task_id}` 用于删除当前用户拥有的任务。删除是不可逆操作；调用前应在业务侧确认目标 ID，并妥善处理已经下载的副本。

## 计费与重试

时长、分辨率、服务档位、音频和模型可能影响费用。创建请求超时后先查询任务或使用接口支持的幂等边界，不要直接重复提交。
