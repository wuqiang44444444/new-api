---
status: current
owner: Dev Team
last-reviewed: 2026-08-04
---

# 20-architecture — 架构索引

本目录描述系统当前的结构、职责边界、数据与控制流，以及已经接受的架构决策。实现过程、上线清单和排障记录不放在这里。

## 目标

让维护者能够从当前代码事实出发，快速定位系统边界、组件职责、接口与状态流、持久化事实和关键架构取舍。

## 放什么

- 当前系统架构、组件职责和依赖方向；
- 稳定的接口、数据、状态与安全边界；
- 专题架构及其与实际代码的对应关系；
- 已接受和已被取代的架构决策记录。

## 不放什么

- 产品需求与验收流程，放入 `10-product/`；
- 编码命令和本地搭建，放入 `30-engineering/`；
- 部署、监控、灰度和处置步骤，放入 `40-operations/`；
- 未完成计划和一次性实施/排障过程，分别放入 `50-planning/`、`80-dev/` 或 `99-archive/`。

## 阅读顺序

1. [架构概览](架构概览.md)：先理解系统边界、运行平面和事实所有权。
2. [Link 合同架构](Link合同架构.md)：理解本地扩展合同、公开 SKU 与渠道实现的关系。
3. [数据模型](数据模型.md)：理解 Task、创建尝试、计费和素材的持久化事实。
4. 再按任务选择图片、视频、素材、账单或文档中心专题。
5. 需要理解“为什么这样设计”时阅读[架构决策索引](decisions/README.md)。

## 当前架构文档

| 层次 | 文档 | 负责回答 | 主要代码事实 |
| --- | --- | --- | --- |
| 全局 | [架构概览](架构概览.md) | 系统由什么组成，边界和事实源在哪里 | `router/`、`controller/`、`service/`、`model/`、`relay/` |
| 合同治理 | [Link 合同架构](Link合同架构.md) | 哪些能力属于 Link，公开合同与实现如何解耦 | `model/link_implementation.go`、SKU capability 注册 |
| 持久化 | [数据模型](数据模型.md) | 哪些状态必须落库，哪些只是缓存或私有快照 | `model/task*.go`、`model/asset*.go`、`model/provider_cost_exposure.go` |
| 图片数据面 | [统一图片生成与异步任务架构](统一图片生成与异步任务架构.md) | 同步/异步图片如何共享入口、Task 和计费 | 图片 relay、`media_task_image_blocking`、`media_image` Task |
| 视频数据面 | [视频上游接入与异步任务架构](视频上游接入与异步任务架构.md) | Link 视频合同、Provider adapter、共享 Task、轮询和内容代理 | Link 视频 Router、Task relay、capability 与 polling；原生视频以上游代码为准 |
| 素材产品 | [Link 资源虚拟素材库架构](Link资源虚拟素材库架构.md) | `ast_*` 对客户代表什么，binding/source 双路径如何解析 | `Asset`、`AssetSource`、`AssetBinding`、Resolver |
| 素材实现 | [素材代理与真人授权架构](素材代理与真人授权架构.md) | 素材创建、Provider 生命周期、真人认证和撤回如何实现 | Asset service/adapter/job、授权 reservation |
| 只读投影 | [API Key 用量账单架构](API-Key用量账单架构.md) | 结算日志如何投影为用户账单 | billing statement controller/model、前端 billing feature |
| 公开文档 | [内置 API 文档中心架构](内置API文档中心架构.md) | 公开合同内容如何构建、校验并随 Web 发布 | `web/src/features/docs/`、`web/scripts/docs/`、`web/public/docs-content/` |

## 文档边界

- 架构文档写当前结构、设计约束和稳定取舍；“代码已存在”与“生产已启用”必须分开表述。
- 产品行为与验收写入 `10-product/`，工程命令写入 `30-engineering/`，运维和灰度写入 `40-operations/`。
- 未完成实施计划写入 `50-planning/`；一次性实现和排障记录写入 `80-dev/`，完成后归档到 `99-archive/`。
- `decisions/` 只保留仍约束当前实现的有效 ADR。被吸收、未发布或事实错误的旧决策从有效目录移除，必要历史由 Git 或 `99-archive/` 追溯；当前事实以本目录专题文档和代码为准。

## 状态约定

| `status` | 含义 |
| --- | --- |
| `current` | 已由当前代码支撑的架构事实；生产可用性仍可能受配置和外部验收控制 |
| `accepted` | 已接受但尚未完整落地的目标设计 |
| `superseded` | 已被后续决策取代，应移出有效 ADR 目录 |
| `historical` | 已归档的过程或旧事实，仅放在 `99-archive/` |
