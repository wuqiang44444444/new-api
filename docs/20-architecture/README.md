---
status: current
owner: Dev Team
last-reviewed: 2026-08-18
---

# 20-architecture — 架构索引

本目录是当前架构事实和已接受目标设计的入口。代码、数据库和已验证的 Provider 证据优先；文档不替代
代码，也不把“已接入”误写成“已生产发布”。

## 目标

让维护者从代码事实出发，快速定位系统边界、组件职责、接口与状态流、持久化事实、Provider 对接合同和
关键架构取舍。

## 放什么

- 当前系统架构、组件职责和依赖方向；
- 稳定的接口、数据、状态与安全边界；
- 专题架构及其与实际代码、真实 Provider 验证的对应关系；
- 已接受和已被取代的架构决策记录（ADR）。

## 不放什么

- 产品需求与验收流程，放入 `10-product/`；
- 编码命令和本地搭建，放入 `30-engineering/`；
- 部署、监控、灰度和处置步骤，放入 `40-operations/`；
- 未完成计划和一次性实施/排障过程，分别放入 `50-planning/`、`80-dev/`；只有通过归档闸门的历史文档
  才进入 `99-archive/`。

## 命名格式

- 专题架构使用稳定的中文业务名称，例如 `Seedance专用渠道与Link架构.md`；不使用日期前缀。
- Provider 子目录使用可识别的 Provider 名称。每个 Provider 目录固定四份文档：原始技术文档整理、
  模型与素材库对接设计、模型价格与计费、模型与素材能力元数据；不再使用 README 作为第二套事实入口。
- ADR 固定使用 `decisions/NNNN-中文短标题.md`；编号创建后不改名、不复用、不回填缺号。

## 内容格式

专题架构按范围与边界、职责与事实源、数据/控制流、接口或状态、约束与取舍组织。ADR 只写
Context、Decision、Consequences 和 Alternatives；实施步骤、验证流水和剩余事项不进入架构正文。

## 三分钟阅读路径

1. [架构概览](架构概览.md)：系统边界、运行平面和事实所有权。
2. [Seedance专用渠道与Link架构](Seedance专用渠道与Link架构.md)：ModelArk V3、确定性渠道、代码协议、视频任务和素材代理。
3. 涉及素材：先看[Seedance模型素材库支持矩阵](Seedance模型素材库支持矩阵.md)，再看[Seedance无状态素材代理架构](Seedance无状态素材代理架构.md)。
4. 涉及异步或计费：看[账单计费-异步任务与计费事实架构](账单计费-异步任务与计费事实架构.md)；动态计费再看[账单计费-计费表达式与协议探针架构](账单计费-计费表达式与协议探针架构.md)。
5. 涉及 Provider：进入[Seedance模型接入设计](Seedance模型接入设计/README.md)，按火山官方、BytePlus官方、FunCloud、墨行/TokenSave、飞彩选择目录。
6. 需要“为什么”时看[架构决策索引](decisions/README.md)；需要公开文档交付、账单投影或通知时按下表定位专题。

## 当前架构文档

| 层次 | 文档 | 负责回答 | 主要代码事实 |
| --- | --- | --- | --- |
| 全局 | [架构概览](架构概览.md) | 系统组成、边界和事实源 | `router/`、`controller/`、`service/`、`model/`、`relay/` |
| Seedance Link | [Seedance专用渠道与Link架构](Seedance专用渠道与Link架构.md) | 专用渠道、ModelArk V3、代码协议、视频任务和素材代理 | `ChannelTypeSeedanceLink`、ModelArk V3 Router、协议注册表、Task |
| Seedance 素材 | [Seedance模型素材库支持矩阵](Seedance模型素材库支持矩阵.md) | 客户模型的素材操作、素材组要求、引用和错误合同 | `PublicAssetAPI`、`seedancePublicAssetAPI`、素材 Controller |
| Seedance 代理 | [Seedance无状态素材代理架构](Seedance无状态素材代理架构.md) | opaque ID、无状态路由、Provider 边界和安全不变量 | 素材 Service、`asset_upstream_protocol` adapter |
| 异步与计费 | [账单计费-异步任务与计费事实架构](账单计费-异步任务与计费事实架构.md) | create attempt、Task、资金、结算和 Provider exposure | `model/task*.go`、Task billing、exposure |
| 计费表达式 | [账单计费-计费表达式与协议探针架构](账单计费-计费表达式与协议探针架构.md) | 表达式校验、协议探针、价格快照和终态结算 | `pkg/billingexpr/`、`setting/billing_setting/` |
| 图片数据面 | [Link图片服务合同与异步任务架构](Link图片服务合同与异步任务架构.md) | 同步/异步图片、Task 和计费 | 图片 relay、图片 Task |
| Provider 接入 | [Seedance模型接入设计](Seedance模型接入设计/README.md) | 五类 Provider 的原始文档、对接、价格和能力元数据 | Provider adapter、模型映射、真实验证 |
| 账单投影 | [账单计费-APIKEY用量账单架构](账单计费-APIKEY用量账单架构.md) | 结算日志到用户账单的只读投影 | billing statement controller/model |
| 对账投影 | [账单计费-客户与上游对账架构](账单计费-客户与上游对账架构.md) | 客户/上游对账、折扣审计和计费快照聚合 | reconciliation router/controller/model |
| 公开文档 | [公开API文档交付架构](公开API文档交付架构.md) | 公开合同的构建、校验和 Web 发布 | `web/src/features/docs/`、`web/scripts/docs/` |
| 用户通知 | [运维-用户生命周期邮件通知架构](运维-用户生命周期邮件通知架构.md) | 账户、密码、API Key 事件邮件和开关 | `notification_setting`、通知 service |
| 合同定价 | [账单计费-用户模型合同定价架构](账单计费-用户模型合同定价架构.md) | 模型授权、渠道组、缓存围栏和冻结计费事实 | `customer_contract` model/service/middleware |

## 事实与变更边界

- 架构文档写当前结构、设计约束和稳定取舍；“代码已存在”与“生产已启用”必须分开表述。
- 产品行为与验收写入 `10-product/`，工程命令写入 `30-engineering/`，运维和灰度写入 `40-operations/`。
- 未完成实施计划写入 `50-planning/`；一次性实现和排障记录写入 `80-dev/`，完成后归档到 `99-archive/`。
- `decisions/` 保留已接受及已被取代的 ADR。编号创建后不可重排；被取代的 ADR 通过
  `status: superseded` 与 `superseded-by` 保持决策链，当前事实以专题架构和代码为准。
- `docs/70-research/` 只作为供应商原始资料的参考来源，本目录不回写或改造研究资料。
- `docs/99-archive/` 永久只读；补充和修正写入当前事实文档，不回写归档。
- 供应商新增或能力变化时，先更新对应 Provider 四份文档、素材支持矩阵和代码验证，再更新索引；不创建
  旧文件名兼容别名。

## 编写要求

- 先明确专题边界、权威事实和不承担的职责，再描述组件、数据/控制流和架构不变量；
- 同一事实只由一份专题架构详细解释，其它文档通过链接引用，不复制整套状态机或字段清单；
- 标题必须准确反映覆盖范围，避免使用“数据模型”“总体设计”等超出实际内容的泛化名称；
- “代码已存在”“真实 Provider 已验证”“生产已发布”必须分开表述；
- 不写实施步骤、日期进展、排障流水和待办清单；这些内容分别进入 30、40、50 或 80；
- ADR 只记录长期取舍及其后果，不承载当前实现清单；ADR 编号一经创建永不复用或重排。
- 文档改动完成后运行 `task docs:check`、`task ai:check`；公开 API 文档同时运行
  `cd web && bun run docs:validate`。

## 状态约定

| `status` | 含义 |
| --- | --- |
| `current` | 已由当前代码支撑的架构事实；生产可用性仍可能受配置和外部验收控制 |
| `accepted` | 已接受但尚未完整落地的目标设计 |
| `superseded` | 已被后续决策取代，保留在 ADR 目录并指向取代者 |
| `historical` | 已归档的过程或旧事实，仅放在 `99-archive/` |
