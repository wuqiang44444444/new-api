---
status: current
owner: Dev Team
last-reviewed: 2026-09-14
---

# 视频模型 API 调用指南

## 1. 两类入口

NEWAPI 原生视频入口与 Seedance Link 入口彼此独立：

| 业务 | 客户入口 | 渠道类型 |
| --- | --- | --- |
| NEWAPI 原生视频 | `/v1/videos`、`/v1/video/generations` | 原生 Provider 渠道 |
| Seedance | `/api/v3/contents/generations/tasks` | `ChannelTypeSeedanceLink` |

`/v1/video/generations` 的 DoubaoVideo 会在南向调用火山 ModelArk V3，但它仍是 NEWAPI 原生北向合同。
Seedance Link 不接管、不包装也不收紧该入口。

## 2. Seedance 创建

先查询 `GET /v1/models/{customer_model}`，确认 `available=true` 并读取
`api.video.creation.parameters` 与 `content_types`。下面的客户模型名与参数仅为示例，需替换为当前
模型允许的值；每个媒体项必须携带该模型支持的 `role`。

```http
POST /api/v3/contents/generations/tasks
Authorization: Bearer <platform-api-key>
Content-Type: application/json
```

```json
{
  "model": "seedance-global",
  "content": [
    {"type": "text", "text": "清晨海岸线，固定镜头"},
    {"type": "image_url", "image_url": {"url": "https://cdn.example.com/reference.png"}, "role": "first_frame"}
  ],
  "duration": 5,
  "ratio": "adaptive",
  "resolution": "720p",
  "generate_audio": true
}
```

平台先校验统一 ModelArk V3 请求结构和计费安全上界，再按所选模型的已发布合同校验字段与媒体组合。
当前公共结构约束包括：

- `duration` 为 `-1` 或 `1..3600`；
- `frames` 为 `29..289` 且满足 `25 + 4n`；
- `execution_expires_after` 为 `3600..259200`；
- `priority` 为 `0..9`；
- `seed` 为 `-1..2147483647`。

合同外字段会明确失败，不会静默删除、钳制或改义。已发布字段按统一北向语义履约；南向不使用的
条件生效字段由 adapter 按合同转换或忽略，不能因为 Provider 没有同名字段而拒绝客户请求。
`output_format` 是统一北向合同中的显式可选字段，不是任意 Provider 参数透传；当前只有部分已登记模型
接受 `mp4` 或 `mov`，其它模型显式传入时返回 400。

### 2.1 按模型读取调用合同

调用前查询 `GET /v1/models/{customer_model}`：

- `api.video.creation` 给出创建方法、路径、内容类型、必填字段和应填写的客户模型名；
- `api.video.operations` 给出全部统一视频入口；
- `api.assets` 给出该客户模型当前是否支持素材及其限制。

客户模型名由部署方定义，目录不会返回上游原始模型名或 Provider。分辨率、时长、媒体数量和扩展字段
以公开参数、媒体合同及已验证场景限制为准；未发布字段在发送上游前明确返回 400。

### 2.2 请求级参考音频

支持参考音频的模型使用 `type=audio_url`、`role=reference_audio` 和 `audio_url.url`：HTTPS URL 原样
传递；音频 Data URL 或裸 Base64 在预扣前上传本站私有对象存储。文件上传使用同一创建路径的
`multipart/form-data`，`request` 项承载完整 JSON，`audio_url.url` 使用 `file://<附件表单项名称>`
引用本次上传附件。附件名必须唯一，且所有附件都必须被引用。

每个附件或解码后的 Base64 音频最多 15 MiB，全请求仍受请求体上限限制。对象存储不可用、上传或签名
失败时返回 `503 reference_audio_unavailable`，不提交生成任务、不预扣生成费用。网关不检查实际音频
时长、不转码；这条路径不创建音频素材，也不改变图片或视频字段。结构、组合和时长要求仍按模型合同
执行，完整文件示例见[公开 ModelArk 文档](../../web/public/docs-content/zh/api-reference/videos/modelark.md)。

## 3. 确定性路由

每个已启用的 Seedance 客户模型只允许配置在一个 `ChannelTypeSeedanceLink` 渠道。请求链为：

```text
Token / Group / price / Seedance Channel 模型登记
  -> 客户模型唯一 Seedance Channel
  -> model_mapping
  -> video_upstream_protocol adapter
  -> 一次 Provider POST
```

Seedance 不使用 Priority、Weight、Affinity、随机分发、失败重选、跨渠道重试或 fallback。创建请求发送
后只有成功、明确失败或结果未知；结果未知时也不重发，因为重复创建会产生额外成本和对账歧义。

## 4. 查询、列表和删除

```text
GET    /api/v3/contents/generations/tasks/{task_id}
GET    /api/v3/contents/generations/tasks
DELETE /api/v3/contents/generations/tasks/{task_id}
```

创建前系统建立 durable create attempt 和资金 hold；取得可信 Provider task ID 后才创建 Task。Task 冻结
客户模型、Channel、上游协议、Provider 模型、查询连接、素材和计费事实。查询和删除使用冻结事实，
不会按当前配置重新选渠。

列表只查询本地主数据库中当前 `user_id + app_id` 的 Task。删除能力以对应上游官方行为为准；不支持时
返回明确错误。

公开状态为 `queued`、`running`、`succeeded`、`failed`、`cancelled`、`expired`。`running` 也可能表示
结果待核实；客户端应有界轮询并保留原 ID，不能据此重新创建或认定退款。视频任务不随同账号 API Key
共享，查询和下载应使用创建时的 Key。

成功交付与最终结算可以分开完成。当前用量响应省略零值，缺失 `usage.completion_tokens` 或空 `usage`
不能区分已确认零用量与未报告；不得补零计算费用，最终费用以账单为准。

## 5. 素材

媒体输入形式与组合以模型合同为准；公共结构支持 HTTP/HTTPS URL、Data URL、`asset://<opaque-id>`，
参考音频另有 §2.2 的 Base64 与附件传输。素材引用由调用方从素材 API 取得，并保存 `model + id + reference`。

- `api.assets.management_mode=caller_managed_stateless`：普通 opaque 引用不在本地查询或校验，
  存在性、权限和兼容性由当前 Provider 判断。
- `platform_hosted`：本站持久化图片与可选组关系，同账号所有 API Key 共享。视频引用在预扣前检查
  账号归属、可引用状态与存储位置，冻结对象事实后以内部临时 URL 履约；只允许图片位置。删除素材
  仅停止新引用，已受理视频继续执行，不删除对象。

部分视频协议只能消费直接媒体 URL 或本站托管图片，不能消费普通代理素材 ID；客户端应遵守公开能力，
不自行拼接、解析 ID 或切换模型探测。跨模型复用只在两个非空 `reuse_scope` 完全相同时尝试。
平台不再提供 `ast_*` / `pubref_*` 命名空间或旧素材兼容 resolver。
素材管理详见[无状态素材代理对接指南](素材库对接指南.md)。

## 6. 模型上线

原生模型可用性以 `/v1/models`、Token/Group/Ability 和管理配置为准；Seedance 模型可用性以客户模型目录、
Token/Group 和管理配置为准，并只通过专用 `ChannelTypeSeedanceLink`、代码协议和只读模型能力投影履约。
系统不提供旧 Link publication、SKU、
capability、implementation 或 execution binding 合同接口。

技术人员在线下确认新模型或上游是否兼容已有代码协议；兼容时管理员可配置任意业务客户模型名，并用
`model_mapping` 精确映射到已登记上游模型；不兼容时由技术人员新增代码 adapter。管理员不编辑协议
JSON。下游只保存和发送客户模型名。
