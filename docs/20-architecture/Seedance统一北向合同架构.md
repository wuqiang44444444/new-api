---
status: accepted
owner: Dev Team
last-reviewed: 2026-08-10
---

# Seedance 统一北向合同架构

## 1. 结论

所有 `ChannelTypeSeedanceLink` 渠道统一对客户提供 ModelArk V3 视频任务合同；南向由不同代码 adapter
调用火山、BytePlus 或第三方 Seedance 上游。本文描述已接受的目标设计，尚不表示全部接口已经完成
生产验收。

```text
客户端 ModelArk V3
  -> ChannelTypeSeedanceLink
  -> 唯一客户模型对应渠道
  -> video_upstream_protocol adapter
  -> 官方或第三方 Seedance Provider
```

`/v1/video/generations` 继续属于 NEWAPI 原生 `DoubaoVideo` 等既有渠道。Seedance 专用渠道不接入该
历史 OpenAI 式视频入口，也不把 `/api/v3` 请求转给 `DoubaoVideo`。

## 2. 北向四组行为

| 行为 | 客户接口 | 数据来源 |
| --- | --- | --- |
| 创建 | `POST /api/v3/contents/generations/tasks` | 唯一客户模型对应 Channel |
| 查询单项 | `GET /api/v3/contents/generations/tasks/{task_id}` | Task 冻结 adapter 与连接 |
| 查询列表 | `GET /api/v3/contents/generations/tasks` | 本地主数据库，按 `user_id + app_id` |
| 删除 | `DELETE /api/v3/contents/generations/tasks/{task_id}` | Task 冻结 adapter；不支持则明确返回 |

客户端只看到平台 Task ID、客户模型和 ModelArk V3 响应。Provider task ID、Provider 模型、渠道凭据、
连接快照和私有错误不进入普通响应。

## 3. 请求结构校验

北向入口校验 ModelArk V3 的统一结构：

- 必填字段和 JSON 类型；
- 文本、图片、视频等媒体节点的标准结构；
- URL、Data URL 与 `asset://ast_*` 的语法边界；
- duration、分辨率、数量等影响计费的安全上界；
- 平台不允许透传的 Provider 私有字段。

入口不再通过 publication、Link SKU capability、implementation 或 execution binding 判断一个模型
是否具备履约资格。模型是否属于 Seedance、某个 Provider 是否兼容已有 adapter，由技术人员线下确认
后交付管理员配置。

adapter 可以校验自身明确的协议限制。不支持字段必须返回稳定错误，不得静默删除、改名、钳制或
降级。完全兼容已有协议的新模型可以仅通过管理配置上线。

## 4. 模型和渠道

不同 Seedance 渠道必须使用不同客户模型名。一个已启用客户模型只对应一个 Seedance 专用渠道，
因此北向入口不运行候选交集或分发算法：

```text
customer_model
  -> Token / Group / Ability / price
  -> unique ChannelTypeSeedanceLink
  -> model_mapping
  -> Provider model
```

模型重名只在保存或启用渠道时检查。请求时不重复检查，也不使用 Priority、Weight、Affinity、随机、
重试或 fallback。

## 5. 创建语义

Seedance 视频创建只允许一次 Provider POST：

1. 校验北向请求和客户权限；
2. 由客户模型确定唯一 Channel；
3. 解析并验证所有平台素材属于同一 Channel/账号/Region/Project；
4. 应用 `model_mapping` 和 adapter 转换；
5. 在发送前建立 durable `TaskCreateAttempt`、资金 hold 和冻结快照；
6. 发送一次 Provider POST；
7. 取得可信 Provider task ID 后原子创建 Task；结果不明则进入视频 `unknown`。

禁止在创建失败或不明时自动重发、切换火山/BytePlus/第三方线路或退款。公开的官方视频创建合同没有
可稳定依赖的 `Idempotency-Key` / `ClientToken`，因此北向不强制客户提供平台自定义幂等键。

## 6. 媒体和素材

同一请求可以混用：

- 普通 HTTP/HTTPS URL；
- Data URL；
- 平台 `asset://ast_*`。

只要出现平台素材，请求渠道就必须与素材冻结渠道一致；所有平台素材必须共享 Provider 账号、Region
和 Project。普通 URL/Data URL 跟随该唯一渠道发送。错误分别为：

```text
asset_channel_mismatch
asset_scope_conflict
```

平台不接受裸 `asset://asset-*` Provider ID。控制台既有资源必须由管理员显式导入并分配给
`user_id + app_id` 后生成 `ast_*`。

## 7. 状态和后续操作

Task 保存创建时的客户模型、Channel、Provider 模型、adapter、查询连接、素材和计费快照。

- 单项查询按冻结 adapter 读取 Provider；一次不可采信响应不直接判定业务失败；
- 列表只使用本地租户数据，不逐项跨 Provider 拉取；
- 删除只调用冻结 adapter；上游不支持时返回明确错误，不伪造成功；
- Channel 停用、模型映射变化或凭据轮换不触发重新选渠。

## 8. 错误原则

| 情况 | 行为 |
| --- | --- |
| 客户模型无已启用唯一 Channel | 返回模型/渠道不可用 |
| adapter 不支持字段 | 返回明确的参数不支持 |
| 上游不支持删除 | 返回明确的不支持 |
| 平台素材渠道不一致 | `asset_channel_mismatch` |
| 多素材上游作用域不一致 | `asset_scope_conflict` |
| 创建结果不明 | 保存视频 attempt `unknown`，不重试或退款 |
| 单次轮询不可采信 | 保持活动并记录观测失败 |

## 9. 架构不变量

1. ModelArk V3 是所有 Seedance 专用渠道的唯一北向视频合同。
2. 南向协议可以不同，但只能由代码 adapter 实现。
3. 一个 Seedance 客户模型只对应一个已启用 Channel。
4. Seedance 不使用 Priority、Weight、Affinity、重试、切换或 fallback。
5. 创建只发送一次 Provider POST；`unknown` 不自动退款。
6. GET/DELETE 使用 Task 冻结 adapter，不重新路由。
7. 北向不暴露 Provider 模型、任务 ID、资源 ID或凭据。
8. 不支持字段明确失败，不静默改义。

## 10. 相关文档

- [Link 渠道与上游协议履约架构](Link服务合同注册与履约架构.md)
- [Link 视频服务合同与异步任务架构](Link视频服务合同与异步任务架构.md)
- [Link 资源合同与解析架构](Link资源合同与解析架构.md)
- [ADR-0016](decisions/0016-Seedance专用渠道与确定性素材代理.md)
