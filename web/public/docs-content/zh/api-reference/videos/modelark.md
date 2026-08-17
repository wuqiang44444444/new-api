---
page-id: videos-modelark
kind: api-reference
last-verified: 2026-08-10
operations:
  - listModelArkVideoModels
  - retrieveModel
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
组合或扩展字段，以管理员上线前的技术审核和所选 Provider 的错误为准；模型目录不替 Provider 承诺
未明确登记的生成能力。

## 模型目录与可用状态

`GET /v1/models` 会在通用模型目录中返回所有已配置 Seedance 客户模型；
`GET /api/v3/contents/generations/models` 只返回同一批 Seedance 条目，便于 ModelArk 客户直接发现。
两者都包括已停用模型。客户应分别读取：

- `available`：当前 API Key 是否能发起新任务；
- `availability`：`available`、`disabled` 或 `restricted`；
- `supported_endpoint_types`：Seedance 固定为 `modelark-video`；
- `api.video.creation`：创建方法、路径、内容类型、必填字段和当前客户模型名；
- `api.video.operations`：创建、列表、查询、删除和内容下载接口；
- `api.assets`：无状态素材代理、引用格式、素材类型、逐操作支持状态及创建限制。

停用只影响调用资格，不会让模型及其接口说明从目录消失。`api` 只描述客户北向合同，不返回 Provider
模型、渠道 ID、凭据、上游协议或第三方私有路径。

客户模型名由部署方定义。调用方只能发送目录中的客户模型名，不需要也无法从公开 API 获知上游原始
模型或 Provider。

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

请求始终可以按对应模型合同使用请求级媒体。素材 API 返回 Provider opaque ID；视频中使用
`asset://<opaque-id>`。平台不查询该 ID、不验证所有权、状态、模型、Channel 或 Provider 作用域，也不
尝试其它 Provider；当前客户模型选中的 Provider 最终判断可用性。是否支持素材创建、素材组和真人认证，
以该模型 `api.assets.operations[].supported` 为准。不支持的操作明确返回
`unsupported_asset_operation`。公开响应不返回 Provider 名称、账号、Channel、协议或上游原始模型。

跨模型复用前比较两个模型的 `api.assets.reuse_scope`。只有两个非空值完全相同时才可尝试复用；scope
不同或缺失时不得复用。部署方可给相同 scope 的模型使用相同业务后缀，但客户端不能用后缀代替元数据。

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
