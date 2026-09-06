---
page-id: images-tasks
kind: api-reference
last-verified: 2026-09-06
operations:
  - retrieveImageTask
---

# 图片任务查询

`GET /v1/tasks/{task_id}` · Bearer 鉴权

查询以 `Prefer: respond-async` 创建的显式异步图片任务。任务按用户与应用（API Key）隔离；
查询其它 Key 的任务与不存在返回同样的 `404`。

## 请求

```bash
curl "{{OPENAI_BASE_URL}}/v1/tasks/task_xxxxxxxx" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}"
```

## 响应字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | string | 平台任务 ID |
| `object` | string | 固定 `image_task` |
| `status` | string | `queued`、`in_progress`、`succeeded`、`failed`、`expired`、`unknown` |
| `created_at` / `finished_at` | integer | 受理 / 终态时间，Unix 秒 |
| `image_count` | integer | 成功交付的图片数量 |
| `data[]` | array | 已持久化登记的逐张结果；`unknown` 期间也可交付已保存的部分图片 |
| `data[].status` | string | `available`、`deleted`、`unavailable`；仅 `available` 携带下载内容 |
| `data[].url` | string | 300 秒有效的签名下载地址；到期后再次查询即获得新地址 |
| `data[].url_expires_at` | integer | 该 URL 的到期时间，Unix 秒 |
| `data[].b64_json` | string | 创建时显式 `response_format=b64_json` 的图片原文 |
| `error` | object | 失败或 `unknown` 时的脱敏错误 |

`deleted` 表示部署方已删除该对象：历史生成状态保留，且不影响同一任务中其余图片的交付；
`unavailable` 表示对象存储暂时无法访问，不代表已删除，稍后重查即可。

`unknown` 表示发送结果或图片保存结果待核实：平台不会自动重发、切换供应商或退款，请联系部署方核实。
已上传但尚未登记的对象可自动补登记；若图片字节从未成功保存且没有可重新查询的来源，
无法仅凭对象地址恢复。此时已保存的部分结果仍可查询。
失败任务的费用全额退还；部分成功不折价，按已验证图片如实交付。
