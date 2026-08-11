---
status: current
owner: Dev Team
last-reviewed: 2026-08-11
---

# Docs Index

## 目标

让 AI、开发者和运维人员能够从统一入口找到 new-api 的项目背景、产品行为、当前架构、工程规则、
运维流程和未完成事项，并明确当前事实、临时记录与历史归档的边界。

## 放什么

- 本文件只提供阅读顺序、目录导航和全局文档治理规则；
- 各一级目录的 `README.md` 负责说明该目录的目标、边界、命名和内容格式；
- 当前有效事实放入 `00-context/`—`40-operations/` 与 `90-ui-ux/`，未完成计划和开发增量分别放入
  `50-planning/`、`80-dev/`。

## 不放什么

- 不在根索引复制产品流程、架构状态机、工程命令或运维步骤；
- 不把实施流水、会议记录或完成项堆积为“当前阶段”；
- 不把 `50-planning/`、`80-dev/` 或 `99-archive/` 当作当前架构事实来源；
- 未经用户针对当前任务明确授权，不读取或更新 `60-marketing/`、`70-research/`；既有
  `99-archive/` 文件永久只读。

## 命名格式

- 一级目录使用固定编号和英文目录名；业务文档使用稳定的中文名称；
- `50-planning/`、`80-dev/` 的增量文件按各自 README 约定使用日期前缀；
- ADR 使用 `20-architecture/decisions/NNNN-中文短标题.md`，编号不重排、不复用、不回填缺号；
- README、AI 入口、脚本和模板等工具文件保留通用名称。

## 内容格式

### 必读顺序

1. [AGENTS.md](../AGENTS.md)
2. [硬约束](00-context/硬约束.md)
3. [术语表](00-context/术语表.md)
4. [项目简报](00-context/项目简报.md)
5. [架构概览](20-architecture/架构概览.md)
6. [命令清单](30-engineering/命令清单.md)
7. [人工智能编码指南](30-engineering/人工智能编码指南.md)

### 目录导航

| 目录 | 负责内容 | 状态 |
| --- | --- | --- |
| [00-context](00-context/README.md) | 项目背景、需求、术语和硬约束 | 当前事实 |
| [10-product](10-product/README.md) | 产品流程、角色与可观察验收 | 当前事实 |
| [20-architecture](20-architecture/README.md) | 系统边界、架构事实与 ADR | 当前事实/已接受设计 |
| [30-engineering](30-engineering/README.md) | 工程命令、开发规范与接入指南 | 当前事实 |
| [40-operations](40-operations/README.md) | 部署、配置、监控、验收和处置 | 当前事实 |
| [50-planning](50-planning/README.md) | 未完成计划、路线图与正式变更记录 | 当前计划 |
| [60-marketing](60-marketing/README.md) | 对外市场内容 | 受保护，需用户明确授权 |
| [70-research](70-research/README.md) | 外部调研与证据 | 受保护，需用户明确授权 |
| [80-dev](80-dev/README.md) | 有日期的开发分析、实施与验证增量 | 临时工作区 |
| [90-ui-ux](90-ui-ux/README.md) | 页面、交互、组件和体验约定 | 当前事实 |
| `99-archive/` | 已完成且通过归档闸门的历史原文 | 永久只读 |

## 编写要求

- 先判断内容属于当前事实、未来计划、开发增量还是历史原文，再选择目录；
- 当前事实文档只写现状、边界、职责、流程和不变量，不写实施时间线；
- “代码已存在”“真实 Provider 已验证”“生产已发布”必须分开表述；
- 每日复核 `80-dev/`，每周复核 `50-planning/`；已验证事实先收敛到当前事实目录，获得用户明确
  授权后才可将原文移动到 `99-archive/`；
- 文档变更后执行 `task docs:check` 与 `task ai:check`；公开 API 文档变更还需执行
  `cd web && bun run docs:validate`。
