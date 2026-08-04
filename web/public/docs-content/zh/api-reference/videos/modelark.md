---
page-id: videos-modelark
kind: api-reference
last-verified: 2026-08-02
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

## doubao Seedance 2.0

`doubao-seedance-2-0-260128` 只使用已登记的 TokenSave relay 实现，不会路由到 FunCloud。

`doubao-seedance-2-0-260128` 支持 `480p`、`720p`、`1080p`，时长支持 4～15 秒或 `-1`，
支持文生视频、首帧/首尾帧和参考图。客户端始终调用上面的 `/api/v3` 接口；平台会适配当前
TokenSave V2 `/v1/media/generations` 合同，无需也不应把模型映射为 `dreamina-*`。

该 Provider 的模型介绍提到视频、音频参考能力，但当前公开请求字段只明确定义了图片引用。
因此本平台暂不发布视频或音频参考输入；未支持的媒体会返回 `400 unsupported_parameter`。
`generate_audio` 用于控制输出视频是否带音频。上游合同可在
[TokenSave 模型页](https://tokensave.pro/docs/models/doubao-seedance-2-0-260128)核对。

## Seedance 2.0 可变分辨率 SKU

`seedance-2.0-standard` 与 `seedance-2.0-fast` 是 FunCloud 对接使用的独立供应商中立 SKU，
不会改变官方、TokenSave 或固定分辨率 SKU 的能力与价格。Standard 支持 480p/720p/1080p，
Fast 支持 480p/720p；时长均为显式 4～15 秒，不支持 `-1`。两者要求至少一个非空文本项，
并可按模型能力接收图片、参考视频和参考音频。两者都只支持 `general` Link 资源经
`source_url` 解析，不支持 `real_person` Link 资源；FunCloud 私有 `realPersonMode` 也不属于公开合同。

调用方继续使用本页 ModelArk v3 合同，不能提交 FunCloud 私有路径、Bearer Key、上游模型参数、
`bytedToken`、material ID 或上游 `asset://asset-*`。渠道未完成生产验收或 Ability 未启用时，
模型会按无可用等价渠道 fail closed。

## Seedance 2.0 固定分辨率 SKU

公开模型为 `seedance-2.0-standard-720p` 和 `seedance-2.0-value-720p`。它们的分辨率由模型名
固定为 720p，`resolution` 省略或传 `720p`；时长为
4～15 秒（默认 4），当前只发布经过协议资料确认的 `16:9`、`9:16`（默认 `16:9`）。

这两个 SKU 要求至少一个非空文本项，支持最多 9 张 `reference_image` 和 3 段
`reference_audio`，不支持参考视频、`first_frame` 或 `last_frame`。普通 `general` 图片和音频
可以使用 `asset://`，但不能和请求级 URL 混用；音频不能单独使用。`generate_audio` 只能省略或传 `false`，回调、服务档位、草稿、水印、seed
和帧数等高级字段不支持。无效字段或组合返回 `400 unsupported_parameter`。

上述固定 SKU 约束同时发布在 OpenAPI
`ModelArkVideoCreateRequest.x-fixed-seedance-2-capability` 中，便于 SDK 和工具机器读取。
平台会将它与运行时 SKU 能力逐值校验；当两者不一致时，发布检查会失败。

模型明确支持时，普通图片或音频可以使用请求级公网 HTTP(S) URL，受支持的图片还可使用 Base64
Data URL；它们不会自动进入平台素材库。需要平台复用、渠道绑定或授权治理的素材使用
`asset://ast_xxx`。平台已识别为真人的素材
必须走授权后的平台素材路径；平台不会仅凭普通 URL/Data URL 自动识别真人，调用方不得借直接
媒体规避未开放真人能力的业务政策。

引用 `real_person` 平台素材时，请求顶层必须同时传入创建认证时使用的
应用内稳定匿名 `end_user_subject`。平台只保存并向支持的 Provider 发送带
`app_id` 作用域的 HMAC 摘要，不保存或向 Provider 发送原文。该字段不支持只接受普通素材、
未发布真人素材能力的固定分辨率 Seedance 2.0 SKU；对这些模型传入时返回
`400 unsupported_parameter`。

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
使用真人托管素材的任务会在发送前预留授权，并在每次内容回源前重新检查授权状态；撤回会阻断
后续内容回源。撤回前已经下载到平台之外的副本始终不受平台控制。

## 删除任务

`DELETE /api/v3/contents/generations/tasks/{task_id}` 用于取消或删除当前用户拥有的任务。具体
能力取决于公开模型/SKU；上游不支持取消或删除时会返回明确的 409 错误，不会伪装操作成功。
删除是不可逆操作；调用前应在业务侧确认目标 ID，并妥善处理已经下载的副本。

上述两个固定 720p SKU 第一阶段只支持本地列表、任务查询和内容下载：排队任务返回
`409 cancellation_unsupported`，运行中返回 `409 task_running`，终态任务返回
`409 delete_unsupported`，平台不会伪装取消或删除成功。

## 计费与重试

时长、分辨率、服务档位、音频和模型可能影响费用。`Idempotency-Key` 是推荐的可选平台扩展。
平台在预扣前建立内部创建记录。已成功持久化 Task 后，同键同请求可以取回原任务；原请求仍在
创建或结果未知时，同 Key 重放返回 `409 idempotency_in_progress`。发送后无法确认结果时会
保留预扣并进入有界对账，不会换渠道或重复提交上游创建请求。客户端应停止自动重试、保存
`request_id` 并联系平台核对。原请求没有提供 Key 时，再次提交是新的业务操作，可能创建第二个
上游任务；事后补充 Key 不能恢复原操作。
