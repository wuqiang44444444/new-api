---
page-id: videos-modelark
kind: api-reference
last-verified: 2026-08-10
operations:
  - createModelArkVideoTask
  - listModelArkVideoTasks
  - getModelArkVideoTask
  - deleteModelArkVideoTask
  - getVideoContent
---

# ModelArk V3 Seedance 视频

所有 Seedance 客户模型统一使用 ModelArk V3 四组任务接口。`/v1/video/generations` 属于 NEWAPI 原生
DoubaoVideo 合同，不是本接口的别名。

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

平台验证统一 ModelArk V3 请求结构、媒体 URL 和影响计费的安全边界。模型是否支持具体分辨率、媒体
组合或扩展字段，以管理员上线前的技术审核和所选 Provider 的错误为准；平台不再提供 publication、
Link SKU capability 或模型能力目录接口。

不同渠道使用不同客户模型名。一个已启用客户模型只对应一个 Seedance 渠道，任务不会根据 Priority、
Weight 随机分发，也不会在失败后换到其它 Provider 再创建一次。

## 查询、列表与删除

```text
GET    /api/v3/contents/generations/tasks
GET    /api/v3/contents/generations/tasks/{task_id}
DELETE /api/v3/contents/generations/tasks/{task_id}
```

Task 会冻结创建时的渠道、Provider 模型、南向协议和计费事实。查询和删除始终使用该冻结链路，不会因
管理员后来修改渠道而重新选路。删除是否支持及返回状态以 Provider 官方行为为准；失败不会切换渠道。

## 素材引用

请求可使用 HTTP/HTTPS URL、Data URL 或平台素材 `asset://ast_*`。平台素材必须属于当前 API Key、客户
模型及其唯一 Seedance 渠道；Provider ID 和账号信息不会返回给客户端。真人认证直接使用素材组响应的
上游 `verification_url`，平台不提供独立真人授权 API。

## 创建结果不明与计费

每个创建请求最多发送一次 Provider POST。平台不接受 ModelArk V3 客户幂等键；发送后结果不明时保留
内部 create attempt 和资金 hold，不自动重发、换渠道或退款。客户端应停止重试并保存 `request_id`，由
技术人员核查。Task 成功建立后，预扣、结算、差额和退款继续使用平台统一计费底座。

任务成功后，如响应提供内容代理，可使用：

```bash
curl "{{OPENAI_BASE_URL}}/videos/video-task-placeholder/content" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  --output result.mp4
```
