---
status: historical
owner: Dev Team
last-reviewed: 2026-09-08
archived-at: 2026-09-09
source-path: docs/80-dev/2026-09-08-TokenSave素材连通性测试边界修复.md
superseded-by:
  - docs/20-architecture/Seedance专用渠道与Link架构.md
  - docs/20-architecture/Seedance无状态素材代理架构.md
---

# TokenSave 素材连通性测试边界修复

## 问题与目标

渠道 79 使用 `tokensave_media_task_v1` 视频协议与 `tokensave_assets_v1` 素材协议。更新 Key 后，管理页面素材库测试立即显示“官方素材 Action 配置无效”，耗时 0.00 秒。

本轮先记录方案，再修复测试能力声明与错误语义。不改变客户素材 API、视频协议、渠道凭据、价格或既有任务。

## 当前实际情况

### 文档与代码证据

用户指定的[TokenSave 海外素材库文档](../70-research/墨行/海外/素材库.md)提供素材/组创建、单项查询、更新和素材删除，鉴权为平台 Bearer Key。文档没有提供素材或组列表接口。查询已有资源使用 `POST /v1/asset/detail/:id` 或 `POST /v1/asset/group/detail/:id`。

修复前，前端连通性测试名单包含 `tokensave_assets_v1`，并承诺“最多列出一个素材”。后端 `TokenSaveAssetAdapter` 及其嵌入实现没有 `CheckConnectivity`。`CheckAssetChannelConnectivity` 的接口断言失败被错误归类为配置无效，并使用官方 Action 文案。这发生在 Provider 请求前，不能据此判定新 Key 无效。

### 设计原则

遵循[Seedance 专用渠道与 Link 架构](../20-architecture/Seedance专用渠道与Link架构.md)、[Seedance 无状态素材代理架构](../20-architecture/Seedance无状态素材代理架构.md)和[硬约束](../00-context/硬约束.md)：

- 能力取决于代码登记协议与已经验证的南向接口，不从模型名称、Key 或素材 CRUD 能力推断无参数探针能力。
- 不支持“连通性测试”不等于不支持素材业务；只拒绝无法履约的测试操作，保留已有单资源代理。
- 素材测试只读，不创建收费视频、素材或素材组，不删除资源，不猜测资源 ID，不新增列表、扫描、fallback 或本地客户素材索引。
- 素材与视频的权限和可用性分别判断；素材测试失败或不支持，不改变视频渠道状态，不证明视频创建权限。
- 新 Key 的实际可用性可通过调用方已有资源的明确 ID 查询验证；本轮不新增测试表单、资源查询模式或探针框架。

## 优化方案

1. 在现有前端支持名单中移除 TokenSave，与已无列表探针的 JoyCreator 保持相同行为。测试按钮禁用，面板明确提示“此素材协议暂不支持只读连通性测试”，不继续承诺列出一个素材。
2. 后端继续以现有 `ConnectivityAdapter` 为执行入口；未实现此接口时返回独立的 `asset_connectivity_unsupported`，不返回配置错误或伪造成功。即使旧前端直接调用，也得到准确、稳定的结果。
3. 前端沿用已有错误映射和翻译流程，为新提示补齐七种语言，不新建公共状态、协议注册表或管理配置项。
4. 先补失败回归，再实施修复：覆盖 TokenSave 禁用并显示原因、受支持协议仍可测试、后端不支持分支不访问上游且不泄露 Key。检查类型、相关 lint、相关前后端测试及文档规范。
5. 修复后需要更新前端构建并重启后端。没有重新验证真实 Key、视频权限或账单时，不标记真实 Provider 验收通过。

本方案仅新增本 80-dev 记录，不修改用户指定的 70-research 原文或既有归档。

## 实施与验证结果

已按上述方案完成前端能力名单、禁用原因、后端错误分类和七种语言提示的修改。变更限于现有 Link 素材连通性测试路径及相关测试，没有新增探针接口或协议配置。

- 前后端回归均先复现旧行为失败，再验证修复通过：前端两个测试文件共 10 项通过；后端 TokenSave、JoyCreator 不支持探针时返回准确错误且不访问 Provider，相关 service/controller 连通性测试通过。
- `bun run typecheck` 与四个变更 TypeScript 文件的定向 oxlint 检查通过。
- `bun run i18n:sync` 完成，七种语言均无缺失翻译键。
- `task docs:check`、`task ai:check` 通过。

上述代码验证阶段未调用真实 Provider；后续用户重启后，按既有授权完成以下独立实测。

## 重启后渠道 79 实测

2026-09-08，以新临时限额 Key 查询已有素材，并仅提交一次 `seedance-2-0-t` 视频创建：4 秒、480p、1:1、不生成音频，使用查询返回的 `asset://` 引用。不创建或删除素材及素材组，不修改渠道配置。

| 检查 | 实际结果 |
| --- | --- |
| 管理端素材测试接口 | HTTP 200，`success=false`，错误码 `asset_connectivity_unsupported`；不再误报官方 Action 配置无效，属于预期的不支持结果 |
| 已有素材查询 | HTTP 200，状态 `ready`，成功取得视频引用 |
| 4 秒带素材视频 | 创建 HTTP 200，经 queued、running 后 succeeded；主库记录用时 348 秒 |
| 返回媒体 | 可读取并解析，H.264、640×640、4.041667 秒，未返回音频流 |
| 任务与扣费 | 仅一条 Task；attempt complete、hold transferred；Task SUCCESS、billing_state settled；最终扣费 135800 quota |
| 临时权限回收 | 测试 Key 已禁用，临时管理员访问凭据已撤销 |

本次请求未再出现此前的模型权限拒绝，证明当前 Key 对该已有素材和这次视频创建可用。该单例不代表全部模型、全部素材或生产验收通过。浏览器验证停留在登录页，未确认登录后实际按钮展示；前端行为仍以前述组件回归为证据。
