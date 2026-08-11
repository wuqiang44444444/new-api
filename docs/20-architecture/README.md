---
status: current
owner: Dev Team
last-reviewed: 2026-08-11
---

# 20-architecture — 架构索引

本目录描述当前架构事实及少量已经接受、尚待完整落地的目标设计。标记为 `accepted` 的专题不能
当作当前代码清单；实现过程、上线清单和排障记录不放在这里。

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

## 命名格式

- 专题架构使用稳定的中文业务名称，例如 `Seedance专用渠道与Link架构.md`；不使用日期前缀。
- Provider 子目录使用可识别的 Provider 名称，文件名说明具体模型、协议或验收边界。
- ADR 固定使用 `decisions/NNNN-中文短标题.md`；编号创建后不改名、不复用、不回填缺号。

## 内容格式

专题架构按范围与边界、职责与事实源、数据/控制流、接口或状态、约束与取舍组织。ADR 只写
Context、Decision、Consequences 和 Alternatives；实施步骤、验证流水和剩余事项不进入架构正文。

## 阅读顺序

1. [架构概览](架构概览.md)：先理解系统边界、运行平面和事实所有权。
2. [Seedance 专用渠道与 Link 架构](Seedance专用渠道与Link架构.md)：理解 ModelArk V3、确定性渠道、代码协议、视频任务和素材代理。
3. [异步任务与计费事实架构](异步任务与计费事实架构.md)：理解 create attempt、Task、资金状态和平台风险。
4. 再按任务选择图片、账单、公开文档或 [Seedance Provider 接入设计](Seedance模型接入设计/README.md)。
5. 需要理解用户生命周期邮件通知时阅读[用户生命周期邮件通知架构](用户生命周期邮件通知架构.md)。
6. 需要理解“为什么这样设计”时阅读[架构决策索引](decisions/README.md)。

## 当前架构文档

| 层次 | 文档 | 负责回答 | 主要代码事实 |
| --- | --- | --- | --- |
| 全局 | [架构概览](架构概览.md) | 系统由什么组成，边界和事实源在哪里 | `router/`、`controller/`、`service/`、`model/`、`relay/` |
| Seedance Link | [Seedance 专用渠道与 Link 架构](Seedance专用渠道与Link架构.md) | 专用渠道、ModelArk V3、代码协议、视频任务和一对一素材如何协作 | `ChannelTypeSeedanceLink`、ModelArk V3 Router、协议注册表、Task、Asset |
| 共享异步事实 | [异步任务与计费事实架构](异步任务与计费事实架构.md) | create attempt、Task、计费和 exposure 如何形成耐久事实 | `model/task*.go`、Task billing、exposure |
| 图片数据面 | [Link 图片服务合同与异步任务架构](Link图片服务合同与异步任务架构.md) | 同步/异步图片如何共享入口、Task 和计费 | 图片 relay、`media_task_image_blocking`、`media_image` Task |
| Provider 接入设计 | [Seedance 模型接入设计索引](Seedance模型接入设计/README.md) | FunCloud、墨行、飞彩如何选择独立客户模型和代码协议 | Provider adapter、模型映射与真实验证事实 |
| 只读投影 | [API Key 用量账单架构](API-Key用量账单架构.md) | 结算日志如何投影为用户账单 | billing statement controller/model、前端 billing feature |
| 只读投影 | [客户与上游对账架构](客户与上游对账架构.md) | 管理员客户/上游对账、月度折扣与审计、计费快照如何从结算事实聚合 | billing-reconciliation router/controller/model、ProviderBillingDiscount/Audit、statement snapshot |
| 公开文档 | [公开 API 文档交付架构](公开API文档交付架构.md) | 公开合同内容如何构建、校验并随 Web 发布 | `web/src/features/docs/`、`web/scripts/docs/`、`web/public/docs-content/` |
| 用户通知 | [用户生命周期邮件通知架构](用户生命周期邮件通知架构.md) | 账户创建、密码修改、API Key 创建三类事件邮件如何按 per-event 开关异步投递 | `notification_setting`、`service/user_lifecycle_notify.go`、controller 接线、前端开关 |

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
