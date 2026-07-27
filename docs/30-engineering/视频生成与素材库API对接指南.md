---
status: current
owner: Dev Team
last-reviewed: 2026-07-27
---

# 视频生成与素材库 API 对接指南

## 1. 目的与适用范围

本文描述当前 new-api 已实现的北向视频、素材和真人授权接口合同，供客户端、SDK 和业务系统接入。本文只说明平台合同；具体模型、尺寸、时长和媒体组合仍受所选渠道能力约束。

OpenAI Videos Core 与 ModelArk v3 的真实 Provider 全生命周期矩阵仍须在持有正式凭据的环境逐渠道验收。完成该验收前，不得宣称“完整兼容所有 Provider”。

## 2. 协议选择

| 场景 | 推荐协议 | 创建入口 |
| --- | --- | --- |
| 通用视频应用、OpenAI 风格 SDK | OpenAI Videos Core | `POST /v1/videos` |
| Seedance / ModelArk 原生客户端 | ModelArk Video v3 | `POST /api/v3/contents/generations/tasks` |
| 需要跨任务复用 Provider 素材 | 平台素材扩展 | `POST /v1/assets` |
| 真人参考图 | 真人授权 + 平台素材扩展 | `POST /v1/real-person-authorizations` |
| 已接入旧协议的存量客户端 | 平台旧视频接口 | `POST /v1/video/generations` |

OpenAI Videos 与 ModelArk v3 共享内部任务、路由、计费和审计能力，但请求字段、状态、分页和错误响应彼此独立。客户端必须使用创建任务时的同一协议查询和删除任务。

## 3. 通用约定

### 3.1 地址、认证与资源隔离

```text
Base URL: https://api.example.com
Authorization: Bearer <platform-api-key>
```

除真人同意页面外，本文接口均使用平台 Bearer Token。客户端不持有上游 API Key、AK/SK、Provider Project、渠道 ID 或上游资源 ID。

资源归属绑定平台用户，而不是可轮换的 Token 字符串。同一用户的不同 Token 可以访问该用户资源；跨用户访问按资源不存在处理。

### 3.2 内容类型

- JSON：`Content-Type: application/json`
- OpenAI 图片文件参考：`multipart/form-data`
- 视频内容：受鉴权的二进制流，支持 `200` 和 Range 请求的 `206`

### 3.3 ID、时间与敏感信息

| 资源 | 对外格式 |
| --- | --- |
| 视频任务 | `task_...` |
| 素材 | `ast_...` |
| 素材绑定 | `ab_...` |
| 真人授权 | `rpa_...` |
| 时间 | Unix 秒 |

客户端只保存平台 ID。响应不会包含上游任务 ID、上游素材 ID、渠道 ID、凭据指纹或密钥。

### 3.4 幂等

以下创建接口支持 `Idempotency-Key`：

- `POST /v1/videos`
- `POST /v1/videos/{video_id}/remix`
- `POST /api/v3/contents/generations/tasks`
- `POST /v1/assets`
- `POST /v1/assets/{asset_id}/migrations`

规则：

- 同 Key、同请求：返回原任务或原素材，不重复调用上游；
- 同 Key、不同请求：返回 `409 idempotency_conflict`；
- 创建幂等记录有效期为 24 小时；
- 网络超时或结果不确定时复用原 Key，不要换 Key 重建。

### 3.5 轮询建议

首次等待 2 秒，随后以 5 秒、10 秒轮询，最长间隔不超过 15 秒。收到 `429`、`502` 或 `503` 时指数退避。客户端业务超时不等于服务端任务失败。

## 4. 接口总览

### 4.1 OpenAI Videos Core

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `POST` | `/v1/videos` | 创建视频 |
| `GET` | `/v1/videos` | 查询当前用户视频列表 |
| `GET` | `/v1/videos/{task_id}` | 查询视频详情 |
| `GET` | `/v1/videos/{task_id}/content` | 下载内容 |
| `POST` | `/v1/videos/{task_id}/remix` | Remix |
| `DELETE` | `/v1/videos/{task_id}` | 删除终态视频 |

### 4.2 ModelArk Video v3

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `POST` | `/api/v3/contents/generations/tasks` | 创建任务 |
| `GET` | `/api/v3/contents/generations/tasks` | 查询最近 7 天任务 |
| `GET` | `/api/v3/contents/generations/tasks/{task_id}` | 查询任务 |
| `DELETE` | `/api/v3/contents/generations/tasks/{task_id}` | 取消排队任务或删除终态任务 |
| `GET` | `/v1/videos/{task_id}/content` | 下载视频或尾帧 |

### 4.3 平台素材与真人授权

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `POST` | `/v1/assets` | 从公网 HTTPS URL 创建素材 |
| `GET` | `/v1/assets` | 查询素材列表 |
| `GET` | `/v1/assets/{asset_id}` | 查询素材 |
| `PATCH` | `/v1/assets/{asset_id}` | 修改名称 |
| `DELETE` | `/v1/assets/{asset_id}` | 删除素材 |
| `GET` | `/v1/assets/{asset_id}/bindings` | 查询素材绑定 |
| `POST` | `/v1/assets/{asset_id}/migrations` | 用新 URL 创建迁移后的新素材 |
| `POST` | `/v1/real-person-authorizations` | 创建真人授权 |
| `GET` | `/v1/real-person-authorizations/{id}` | 查询并刷新授权 |
| `POST` | `/v1/real-person-authorizations/{id}/retry` | 重试认证 |
| `POST` | `/v1/real-person-authorizations/{id}/revoke` | 撤回授权 |

平台不提供素材二进制上传、素材内容下载、`complete` 或后绑定接口。

## 5. OpenAI Videos Core

### 5.1 创建视频

```bash
curl -sS -X POST "$API_BASE/v1/videos" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $REQUEST_ID" \
  -d '{
    "model": "sora-2",
    "prompt": "A paper boat moving through a rainy miniature city",
    "seconds": "4",
    "size": "720x1280"
  }'
```

| 字段 | 必填 | 合同 |
| --- | --- | --- |
| `model` | 是 | 当前 Token 和渠道允许的视频模型 |
| `prompt` | 是 | 去除首尾空白后非空，最长 32000 字符 |
| `seconds` | 否 | 字符串；基础合同允许 `"4"`、`"8"`、`"12"` |
| `size` | 否 | `720x1280`、`1280x720`、`1024x1792`、`1792x1024` |
| `input_reference` | 否 | `file_id` 与 `image_url` 二选一 |

URL 或平台素材参考：

```json
{
  "model": "sora-2",
  "prompt": "Animate the reference image",
  "seconds": "4",
  "size": "1280x720",
  "input_reference": {
    "image_url": "asset://ast_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
  }
}
```

`image_url` 支持合同允许的 HTTP(S)、data URL，以及平台扩展 `asset://ast_...`。平台素材必须属于当前用户、状态为 `ready`、绑定模型一致，且与同一请求中的其他素材存在共同可用渠道。

文件参考使用 `multipart/form-data`，单文件最大 20 MiB：

```bash
curl -sS -X POST "$API_BASE/v1/videos" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Idempotency-Key: $REQUEST_ID" \
  -F "model=sora-2" \
  -F "prompt=Animate the reference image" \
  -F "seconds=4" \
  -F "size=1280x720" \
  -F "input_reference=@reference.png"
```

成功返回平台任务：

```json
{
  "id": "task_xxx",
  "object": "video",
  "model": "sora-2",
  "status": "queued",
  "progress": 0,
  "created_at": 1784778000,
  "seconds": "4",
  "size": "720x1280"
}
```

### 5.2 查询、列表与下载

详情：

```http
GET /v1/videos/{task_id}
```

状态为 `queued`、`in_progress`、`completed` 或 `failed`。失败时读取 `error.code`，不要把客户端轮询超时写成服务端失败。

列表：

```http
GET /v1/videos?limit=20&order=desc&after=task_xxx
```

`limit` 为 1～100，`order` 为 `asc` 或 `desc`。下一页使用本页 `last_id` 作为 `after`。

下载：

```bash
curl -fL "$API_BASE/v1/videos/$VIDEO_ID/content" \
  -H "Authorization: Bearer $TOKEN" \
  -o result.mp4
```

只有 `completed` 可下载；支持 `Range`。内容过期后返回 `410 video_content_expired`。

### 5.3 Remix 与删除

```http
POST /v1/videos/{video_id}/remix
Content-Type: application/json
Idempotency-Key: <unique-key>

{"prompt":"Keep the scene and change the weather to snow"}
```

原视频必须属于当前用户、由 OpenAI Videos 创建、未删除且原 Provider 支持 Remix。Remix 固定使用原渠道和计费参数，不自动切换账号。

`DELETE /v1/videos/{task_id}` 只删除 `completed` 或 `failed` 等终态视频；运行中任务返回 `video_not_terminal`。OpenAI Videos 当前合同没有独立的运行中取消端点。

## 6. ModelArk Video v3

### 6.1 创建任务

```bash
curl -sS -X POST "$API_BASE/api/v3/contents/generations/tasks" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $REQUEST_ID" \
  -d '{
    "model": "seedance-model",
    "content": [
      {"type": "text", "text": "雨夜城市中行驶的复古汽车"}
    ],
    "duration": 5,
    "resolution": "720p",
    "ratio": "16:9",
    "generate_audio": true,
    "watermark": false
  }'
```

| 字段 | 必填 | 平台合同 |
| --- | --- | --- |
| `model` | 是 | 当前 Token 和兼容 DoubaoVideo 渠道支持的模型 |
| `content` | 是 | 至少一个内容项 |
| `duration` | 否 | `-1` 或 4～15；Provider 可进一步收紧 |
| `resolution` | 否 | `480p`、`720p`、`1080p`、`4k` |
| `ratio` | 否 | `16:9`、`4:3`、`1:1`、`3:4`、`9:16`、`21:9`、`adaptive` |
| `service_tier` | 否 | `default`；`flex` 仅在渠道明确支持时可用 |
| `generate_audio` | 否 | 布尔值，显式 `false` 会保留 |
| `watermark` | 否 | 布尔值，显式 `false` 会保留 |
| `callback_url` | 否 | 当前不支持 |

`content` 支持文本，以及 `first_frame`、`last_frame`、`reference_image`、`reference_video`、`reference_audio`。媒体 URL 可使用合同允许的 HTTP(S)、data URL 或 `asset://ast_...`。

组合约束：

- 图片最多 9 个，视频和音频各最多 3 个；
- 首帧和尾帧各最多 1 个；
- 尾帧必须与首帧同时出现；
- 只有音频、没有图片或视频时拒绝；
- 实际 Provider 可能有更严格的能力限制。

成功返回：

```json
{"id":"task_xxx"}
```

### 6.2 查询、列表与取消/删除

详情：

```http
GET /api/v3/contents/generations/tasks/{task_id}
```

状态链：

```text
queued -> running -> succeeded
                  -> failed / cancelled / expired
```

成功任务的 `content.video_url` 和 `last_frame_url` 指向平台受鉴权内容代理，访问时仍须携带 Bearer Token。

列表：

```http
GET /api/v3/contents/generations/tasks?page_num=1&page_size=10
```

只返回最近 7 天、当前用户、未删除的 ModelArk v3 任务。支持 `filter.status`、可重复的 `filter.task_ids`、`filter.model` 和 `filter.service_tier`。

删除入口根据状态执行不同语义：

| 状态 | 行为 |
| --- | --- |
| `queued` | 请求取消；确认后结算退款 |
| `running` | `409 task_running` |
| `cancelled` | `409 task_cancelled` |
| `succeeded` / `failed` / `expired` | 删除上游结果并隐藏本地任务 |

`503 cancellation_unknown` 表示取消结果未知；继续查询原任务，不要立即创建重复任务。

## 7. 平台素材

### 7.1 创建普通素材

```bash
curl -sS -X POST "$API_BASE/v1/assets" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $REQUEST_ID" \
  -d '{
    "name": "reference-image",
    "asset_kind": "general",
    "media_type": "image",
    "model": "seedance-model",
    "source": {
      "type": "url",
      "url": "https://cdn.example.com/signed/reference.png?signature=...",
      "expires_at": 1784785200
    }
  }'
```

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `name` | 是 | 1～64 字符 |
| `asset_kind` | 是 | `general` 或 `real_person` |
| `media_type` | 是 | `image`、`video` 或 `audio` |
| `model` | 二选一 | 绑定视频模型 |
| `target` | 二选一 | 当前仅 `joycreator_library`，不参与视频路由 |
| `authorization_id` | 真人素材必填 | 当前用户的有效授权 |
| `source.type` | 是 | 固定为 `url` |
| `source.url` | 是 | Provider 可访问的公网 HTTPS URL |
| `source.expires_at` | 否 | URL 到期 Unix 秒；临时 URL 应提供 |

`model` 与 `target` 必须且只能提供一个。真人素材只能是图片、必须绑定 `model`，并与授权共享用户、模型和 Provider 账号。

成功返回 HTTP `202`。素材状态为：

```text
creating / processing -> ready
creating              -> create_unknown / failed
ready                  -> deleting -> deleted / deletion_failed
```

只有 `ready` 素材可用于新视频。

### 7.2 URL 安全合同

素材 URL 必须使用 HTTPS、公网地址、无 userinfo、无 fragment、无非 443 端口，并满足渠道配置的最小剩余 TTL。平台拒绝 loopback、私网、link-local、云元数据地址和非 URL 来源。

平台只做控制面校验，不主动 GET/HEAD 源 URL，也不替代 Provider 的 SSRF 防护。创建素材不会使平台保存或托管素材二进制。

### 7.3 查询、改名、删除与迁移

```text
GET    /v1/assets?page=1&page_size=20&status=ready
GET    /v1/assets/{asset_id}
GET    /v1/assets/{asset_id}/bindings
PATCH  /v1/assets/{asset_id}  {"name":"new-name"}
DELETE /v1/assets/{asset_id}
```

删除成功返回 `204`。本地事务提交后立即禁止新引用，上游清理可异步完成；平台不会删除客户自己的源对象。

渠道账号、Project、Region 或模型需要变化时，使用迁移接口和新的源 URL：

```http
POST /v1/assets/{asset_id}/migrations
```

迁移创建新的 `ast_...`，不原地修改旧 binding。`migration_reason` 必填；真人素材迁移需要新的或再次确认有效的授权。

### 7.4 视频引用

OpenAI Videos 使用：

```json
{"input_reference":{"image_url":"asset://ast_xxx"}}
```

ModelArk v3 使用：

```json
{
  "content": [{
    "type": "image_url",
    "role": "reference_image",
    "image_url": {"url": "asset://ast_xxx"}
  }]
}
```

平台在选择兼容渠道后，把平台 ID 改写为创建时冻结的上游引用；客户端不会看到该值。

## 8. 真人素材授权

创建：

```http
POST /v1/real-person-authorizations
Content-Type: application/json

{"model":"seedance-model","locale":"zh-CN"}
```

响应中的 `consent_url` 必须交给照片本人打开。客户端或运维人员不得代替本人提交同意。

查询：

```http
GET /v1/real-person-authorizations/{authorization_id}
```

只有 `authorized` 可创建真人素材。常见状态包括 `awaiting_consent`、`awaiting_verification`、`verifying`、`authorized`、`failed`、`expired`、`revoked`、`deleting` 和 `deleted`。

可重试认证使用 `/retry`；撤回使用 `/revoke`。撤回事务完成后，相关真人素材立即不能用于新任务，上游清理可在后台继续。

## 9. 错误与客户端动作

三类协议保持各自错误外壳，但稳定错误码可用于客户端分支。

| HTTP | 典型错误 | 客户端动作 |
| --- | --- | --- |
| `400` | `invalid_request`、`unsafe_asset_url` | 修正请求，不自动重试 |
| `401` | `authentication_error` | 检查平台 Token |
| `403` | `permission_denied`、授权未生效 | 检查权限或授权 |
| `404` | `video_not_found`、`asset_not_found` | 停止轮询并核对协议/ID |
| `409` | `idempotency_conflict`、`asset_binding_required`、状态冲突 | 按错误码处理，不盲重试 |
| `410` | `video_content_expired` | 内容已过期 |
| `429` | `rate_limit_exceeded` | 指数退避 |
| `502` | `upstream_unavailable`、`upstream_auth_error` | 保留原 Key，稍后重试 |
| `503` | `create_outcome_unknown`、`cancellation_unknown` | 查询原资源并对账 |

上游 401/403 会转换为上游鉴权错误，不会冒充客户端平台 Token 错误。

## 10. 对接检查表

- [ ] 选择并固定一种视频北向协议。
- [ ] 每次新业务操作生成唯一 `Idempotency-Key`，重试复用原 Key。
- [ ] 只持久化平台 `task_...`、`ast_...` 和 `rpa_...`。
- [ ] 下载平台内容代理时继续携带 Bearer Token。
- [ ] 等素材进入 `ready` 后再创建视频。
- [ ] 多素材请求确认存在共同冻结渠道。
- [ ] 不记录完整签名 URL、Token、真人认证 URL 或临时凭证。
- [ ] 把 `create_unknown` 和 `cancellation_unknown` 当作结果未知，而不是已失败。

## 11. 相关文档

- [视频上游接入与异步任务架构](../20-architecture/视频上游接入与异步任务架构.md)
- [素材代理与真人授权架构](../20-architecture/素材代理与真人授权架构.md)
- [素材库验收操作手册](../40-operations/素材库验收操作手册.md)
- [素材代理与真人授权配置手册](../40-operations/素材代理与真人授权配置手册.md)
- 历史分析：[视频生成与素材接口标准性及实现差距分析](../99-archive/2026-07-23-视频生成与素材接口标准性及实现差距分析.md)
