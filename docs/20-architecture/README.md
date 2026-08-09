---
status: current
owner: Dev Team
last-reviewed: 2026-08-09
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
2. [Link 服务合同概念与协作关系](Link服务合同概念与协作关系.md)：理解客户合同、发布事实、接入方案与渠道实现的分工。
3. [Link 服务合同注册与履约架构](Link服务合同注册与履约架构.md)：理解本地扩展合同、公开 SKU 与实现注册的技术边界。
4. [异步任务与计费事实架构](异步任务与计费事实架构.md)：理解 create attempt、Task、资金状态和平台风险。
5. 再按任务选择图片、视频、资源、真人授权、账单、公开文档、[Seedance 统一北向合同](Seedance统一北向合同架构.md)
   或 [Seedance Provider 接入设计](<seedance 模型接入设计/README.md>)。
6. 需要理解“为什么这样设计”时阅读[架构决策索引](decisions/README.md)。

## 当前架构文档

| 层次 | 文档 | 负责回答 | 主要代码事实 |
| --- | --- | --- | --- |
| 全局 | [架构概览](架构概览.md) | 系统由什么组成，边界和事实源在哪里 | `router/`、`controller/`、`service/`、`model/`、`relay/` |
| 概念边界 | [Link 服务合同概念与协作关系](Link服务合同概念与协作关系.md) | 客户合同、发布事实、SKU、接入方案和 Provider 实现如何协作 | `model/link_model_publication*.go`、`model/link_execution_binding.go` |
| 合同治理 | [Link 服务合同注册与履约架构](Link服务合同注册与履约架构.md) | 哪些能力属于 Link，公开合同与实现注册如何解耦 | `model/link_implementation.go`、SKU capability 注册 |
| 共享异步事实 | [异步任务与计费事实架构](异步任务与计费事实架构.md) | create attempt、Task、计费和 exposure 如何形成耐久事实 | `model/task*.go`、Task billing、exposure |
| 图片数据面 | [Link 图片服务合同与异步任务架构](Link图片服务合同与异步任务架构.md) | 同步/异步图片如何共享入口、Task 和计费 | 图片 relay、`media_task_image_blocking`、`media_image` Task |
| 视频数据面 | [Link 视频服务合同与异步任务架构](Link视频服务合同与异步任务架构.md) | Link 视频合同、Provider adapter、共享 Task、轮询和内容代理 | Link 视频 Router、Task relay、capability 与 polling；原生视频以上游代码为准 |
| Seedance 北向合同 | [Seedance 统一北向合同架构](Seedance统一北向合同架构.md) | ModelArk v3 下的规范词汇、模型级 capability、版本、机器发现和失败关闭如何协作 | `VideoSKUCapability`、ModelArk middleware、capability API 与 OpenAPI 投影 |
| Provider 接入设计 | [Seedance 模型接入设计索引](<seedance 模型接入设计/README.md>) | FunCloud、墨行、飞彩如何在共同 Link 合同下表达各自路径、素材、Task 和计费差异 | Provider implementation、execution binding、adapter 与 capability 目标设计 |
| Link 资源 | [Link 资源合同与解析架构](Link资源合同与解析架构.md) | `ast_*`、source/binding、Provider 物化和 Resolver 如何协作 | `Asset`、`AssetSource`、`AssetBinding`、Resolver、Asset job |
| 真人授权 | [真人素材授权与撤回架构](真人素材授权与撤回架构.md) | 真人认证、任务使用、撤回和内容访问如何线性化 | authorization、verification、Task authorization reservation |
| 只读投影 | [API Key 用量账单架构](API-Key用量账单架构.md) | 结算日志如何投影为用户账单 | billing statement controller/model、前端 billing feature |
| 只读投影 | [客户与上游对账架构](客户与上游对账架构.md) | 管理员客户/上游对账、月度折扣与审计、计费快照如何从结算事实聚合 | billing-reconciliation router/controller/model、ProviderBillingDiscount/Audit、statement snapshot |
| 公开文档 | [公开 API 文档交付架构](公开API文档交付架构.md) | 公开合同内容如何构建、校验并随 Web 发布 | `web/src/features/docs/`、`web/scripts/docs/`、`web/public/docs-content/` |

## 文档边界

- 架构文档写当前结构、设计约束和稳定取舍；“代码已存在”与“生产已启用”必须分开表述。
- 产品行为与验收写入 `10-product/`，工程命令写入 `30-engineering/`，运维和灰度写入 `40-operations/`。
- 未完成实施计划写入 `50-planning/`；一次性实现和排障记录写入 `80-dev/`，完成后归档到 `99-archive/`。
- `decisions/` 保留已接受及已被取代的 ADR。编号创建后不可重排；被取代的 ADR 通过
  `status: superseded` 与 `superseded-by` 保持决策链，当前事实以本目录专题架构和代码为准。

## 编写要求

- 先明确专题边界、权威事实和不承担的职责，再描述组件、数据/控制流和架构不变量；
- 同一事实只由一份专题架构详细解释，其它文档通过链接引用，不复制整套状态机或字段清单；
- 标题必须准确反映覆盖范围，避免使用“数据模型”“总体设计”等超出实际内容的泛化名称；
- “代码已存在”“真实 Provider 已验证”“生产已发布”必须分开表述；
- 不写实施步骤、日期进展、排障流水和待办清单；这些内容分别进入 30、40、50 或 80；
- ADR 只记录长期取舍及其后果，不承载当前实现清单；ADR 编号一经创建永不复用或重排。

## 状态约定

| `status` | 含义 |
| --- | --- |
| `current` | 已由当前代码支撑的架构事实；生产可用性仍可能受配置和外部验收控制 |
| `accepted` | 已接受但尚未完整落地的目标设计 |
| `superseded` | 已被后续决策取代，保留在 ADR 目录并指向取代者 |
| `historical` | 已归档的过程或旧事实，仅放在 `99-archive/` |
