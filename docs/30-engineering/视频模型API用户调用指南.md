---
status: current
owner: Dev Team
last-reviewed: 2026-08-12
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
    {"type": "image_url", "image_url": {"url": "https://cdn.example.com/reference.png"}}
  ],
  "duration": 5,
  "ratio": "16:9",
  "resolution": "1080p",
  "generate_audio": true
}
```

平台只校验统一 ModelArk V3 请求结构和计费安全上界。当前结构约束包括：

- `duration` 为 `-1` 或 `1..3600`；
- `frames` 为 `29..289` 且满足 `25 + 4n`；
- `execution_expires_after` 为 `3600..259200`；
- `priority` 为 `0..9`；
- `seed` 为 `-1..2147483647`。

具体 adapter 不支持的字段会明确失败，不会静默删除、钳制或改义。
`output_format` 是统一北向合同中的显式可选字段，不是任意 Provider 参数透传；当前只有部分已登记模型
接受 `mp4` 或 `mov`，其它模型显式传入时返回 400。

### 2.1 按模型读取调用合同

调用前查询 `GET /v1/models/{customer_model}`：

- `api.video.creation` 给出创建方法、路径、内容类型、必填字段和应填写的客户模型名；
- `api.video.operations` 给出全部统一视频入口；
- `api.assets` 给出该客户模型当前是否支持素材及其限制。

客户模型名由部署方定义，目录不会返回上游原始模型名或 Provider。分辨率、时长、媒体数量和扩展字段
仍以已登记 adapter 的校验结果为准；不支持的字段在发送上游前明确返回 400。

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

## 5. 素材

`content` 可以包含 HTTP/HTTPS URL、Data URL、`asset://<opaque-id>`。opaque ID 由调用方从素材 API
取得并与客户模型一起保存；平台不会查询或改写素材引用。素材是否属于当前账号或支持当前模型由
Provider 最终判断。平台不再提供 `ast_*` / `pubref_*` 命名空间。
素材管理详见[素材库对接指南](素材库对接指南.md)。

## 6. 模型上线

模型可用性以 `/v1/models`、Token/Group/Ability 和管理配置为准。系统不提供 Link publication、SKU
capability、implementation 或 execution binding 接口。

技术人员在线下确认新模型或上游是否兼容已有代码协议；兼容时管理员可配置任意业务客户模型名，并用
`model_mapping` 精确映射到已登记上游模型；不兼容时由技术人员新增代码 adapter。管理员不编辑协议
JSON。下游只保存和发送客户模型名。
