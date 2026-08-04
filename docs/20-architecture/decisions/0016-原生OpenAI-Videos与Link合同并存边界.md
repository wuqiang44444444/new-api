---
adr: 0016
status: accepted
date: 2026-08-04
superseded-by: ""
---

# ADR-0016: 原生OpenAI-Videos与Link合同并存边界

## Context

本决策取代 [ADR-0007](./0007-视频Link合同与共享任务底座.md)。ADR-0007 对 Seedance、Kling、
即梦 Link 合同、共享 Task、素材约束和渠道适配协议的决定继续有效；被取代的是“OpenAI 视频已
下架，只保留存量读取”这一与 NEWAPI `v1.0.0-rc.23` 原生事实冲突的结论。

rc23 原生提供 `POST /v1/videos`、`POST /v1/videos/{video_id}/remix`、
`GET /v1/videos/{task_id}`、内容代理以及旧版 `/v1/video/generations` 创建/查询链路；Sora
adapter、请求校验、模型发现、渠道分配、任务轮询和计费代码也仍在。此前仅移除路由并禁止 Sora
渠道/模型，形成了本地业务分叉，也违反“NEWAPI 原生合同永远优先”的硬约束。

同时，本项目已经发布显式注册的 Link SKU 和虚拟素材库。恢复原生链路不能让已登记 Link SKU
绕过其类型化 DTO、能力声明、实现身份、exposure 和错误合同，也不能削弱 `asset://ast_*` 的
应用隔离、授权撤回和渠道绑定交集。

## Decision

1. 以 NEWAPI `v1.0.0-rc.23` 为原生实现权威，完整恢复其 OpenAI Videos 与旧版平台视频入口，
   直接复用上游现有 Router、Relay、Sora adapter、DTO、JSON/multipart 校验、模型发现、渠道分配、
   轮询和计费链，不建设第二套视频实现。
2. 原生合同使用 `client_protocol=openai_videos`；旧版 `/v1/video/generations` 使用
   `platform_video`。两者与 ModelArk、Kling、即梦 Link 合同共享 Task、计费和轮询事实，但不冻结
   Link capability 或 `link_implementation_id/version/hash`。
3. 显式登记的 Link SKU 只能通过其已发布 Link 路由创建。若客户端把 Link SKU 提交给
   `/v1/videos` 或旧版创建入口，必须返回稳定的 `link_sku_contract_mismatch`，不得进入原生宽松
   候选集；Remix 的源任务也执行同一边界。
4. 原生请求继续允许 rc23 支持的 HTTP/HTTPS URL、Data URL 和 multipart 文件。使用本项目
   `asset://ast_*` 扩展时，仍须在选渠前经过现有虚拟素材所有权、应用、授权、状态、模型、绑定和
   多素材渠道交集检查，并在发送前解析；不新增原生专属素材表或旁路。
5. OpenAI Videos 与旧版平台视频创建同样属于共享异步 Task 创建，必须在 Provider POST 前建立
   durable attempt 并持有计费额度；发送后结果未知时不退款、不换渠道、不重发，进入现有有界对账。
6. rc23 早期任务可能没有 `client_protocol`。兼容读取只接受同一用户、同一 API Key 应用且能明确
   识别为非 Link 模型的空协议任务；模型为空或属于 Link SKU 时 fail closed，不进行歧义协议回填。
7. 恢复代码不自动启用生产渠道或 Ability。管理员仍须配置模型价格、预扣上界、分组、渠道和真实
   Provider 验收，之后才开放流量。

## Consequences

- 收益：rc23 原生 OpenAI Videos 创建、Remix、查询、内容和旧版兼容链路重新完整可用，后续上游
  同步不再维护“路由删除但实现残留”的分叉。
- 收益：Link 合同和虚拟素材库继续保持显式注册、fail-closed、应用隔离与实现身份冻结；原生模型
  不会误带 Link implementation 快照。
- 代价：同一共享 Task 底座同时服务原生与 Link 视频合同，所有查询、幂等、恢复和内容回源都必须
  按 `client_protocol` 与应用作用域测试。
- 风险：生产渠道若未完成价格、上游账号和错误语义验收，恢复代码存在并不等于可以开放 Sora；
  上线仍由 Channel、Ability 和分组控制。
- 后续约束：上游已存在的原生视频逻辑必须直接同步，不得另写同类 Router/DTO/adapter；本地只允许
  保留 Link 隔离、虚拟素材授权和创建尝试安全所必需的最小接线。

## Alternatives Considered

- 只恢复路由：未采用。Sora 模型发现、渠道启用、durable attempt、存量任务作用域和 Link SKU
  旁路仍会断裂或不安全。
- 把 OpenAI Videos 注册成新的 Link 合同：未采用。它是 rc23 原生合同，重复注册会违反原生优先
  和禁止并行实现的硬约束。
- 用 ModelArk 统一承接 `/v1/videos`：未采用。两者字段、默认值、响应、Remix 和生命周期不同，
  会改变原生合同并让 Link SKU 绕过自己的类型化入口。
- 对所有空协议历史任务做批量协议回填：未采用。无法可靠区分旧 OpenAI Videos 与旧版平台视频，
  歧义迁移可能造成跨合同或跨应用暴露。
