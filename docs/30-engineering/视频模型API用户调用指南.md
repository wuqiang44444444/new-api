---
status: current
owner: Dev Team
last-reviewed: 2026-08-10
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

`content` 可以包含 HTTP/HTTPS URL、Data URL 和 `asset://ast_*`。平台素材必须属于当前
`user_id + app_id`，状态为 `ready`，并且创建时客户模型、Channel、协议和凭据身份与本次请求一致。
素材管理详见[素材库对接指南](素材库对接指南.md)。

## 6. 模型上线

模型可用性以 `/v1/models`、Token/Group/Ability 和管理配置为准。系统不提供 Link publication、SKU
capability、implementation 或 execution binding 接口。

技术人员在线下确认新模型或上游是否兼容已有代码协议；兼容时管理员可直接配置渠道、客户模型和
`model_mapping`，不兼容时由技术人员新增 `video_upstream_protocol` adapter。管理员不编辑协议 JSON。
