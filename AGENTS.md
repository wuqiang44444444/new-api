# AGENTS.md — Project Conventions for new-api
> 本仓库遵循 docs/ 目录规范，开始任务前必须先读 docs/README.md。
> 开始编码前还必须阅读 `docs/00-context/硬约束.md`；涉及 Link、异步任务、资源或计费时，再读取对应的 `docs/20-architecture/` 专题架构。

DO NOT send optional commentary

## Overview

This is an AI API gateway/proxy built with Go. It aggregates 40+ upstream AI providers (OpenAI, Claude, Gemini, Azure, AWS Bedrock, etc.) behind a unified API, with user management, billing, rate limiting, and an admin dashboard.

## Tech Stack

- **Backend**: Go 1.22+, Gin web framework, GORM v2 ORM
- **Frontend**: React 19, TypeScript, Rsbuild, Base UI, Tailwind CSS
- **Databases**: SQLite, MySQL, PostgreSQL (all three must be supported)
- **Cache**: Redis (go-redis) + in-memory cache
- **Auth**: JWT, WebAuthn/Passkeys, OAuth (GitHub, Discord, OIDC, etc.)
- **Frontend package manager**: Bun (preferred over npm/yarn/pnpm)

## Architecture

Layered architecture: Router -> Controller -> Service -> Model

```
router/        — HTTP routing (API, relay, dashboard, web)
controller/    — Request handlers
service/       — Business logic
model/         — Data models and DB access (GORM)
relay/         — AI API relay/proxy with provider adapters
  relay/channel/ — Provider-specific adapters (openai/, claude/, gemini/, aws/, etc.)
middleware/    — Auth, rate limiting, CORS, logging, distribution
setting/       — Configuration management (ratio, model, operation, system, performance)
common/        — Shared utilities (JSON, crypto, Redis, env, rate-limit, etc.)
dto/           — Data transfer objects (request/response structs)
constant/      — Constants (API types, channel types, context keys)
types/         — Type definitions (relay formats, file sources, errors)
i18n/          — Backend internationalization (go-i18n, en/zh)
oauth/         — OAuth provider implementations
pkg/           — Internal packages (cachex, ionet)
web/           — Frontend (React 19, Rsbuild, Base UI, Tailwind)
  src/i18n/    — Frontend internationalization (i18next, en/zh/zh-TW/fr/ru/ja/vi)
```

### Current contract boundaries

- **NEWAPI 原生能力**：上游已经提供的 Router、DTO、Relay、Provider adapter、模型发现和计费语义继续以上游代码为权威。本地代码不得包装、复制、收紧或为原生入口增加 Link 模型推断。
- **Link 类型化合同**：Link 只承载本地明确支持的扩展入口。Seedance 由专用 ChannelType 和代码协议
  注册表达；技术人员线下审核模型兼容性，管理员按明确结论配置。
- **统一北向协议优先**：Link 已发布北向协议定义客户请求形状和字段语义；南向代码 adapter 必须吸收
  Provider 的字段、路径和必填性差异，不得要求客户按 Provider 或模型改变请求结构。该规则不允许透传
  合同外 Provider 私有参数，也不允许静默改变其它北向字段语义。
- 两类能力可以共享鉴权、渠道、Task、计费、资源和日志底座，但不得互相推断、兼容降级或建立第二套公共状态。

### Current Link facts

- Seedance 使用 `ChannelTypeSeedanceLink`；一个已启用客户模型只对应一个 Seedance Channel。
- Seedance 北向固定为 ModelArk V3，南向由代码注册的 `video_upstream_protocol` /
  `asset_upstream_protocol` adapter 履约。
- Link 不使用 publication、Link SKU、逐模型 capability、不可变 implementation、execution binding、
  内容 hash 等价证明、Link Access Plan 或候选交集。
- Seedance 的 Channel 模型清单描述其专用路由；它不写入 NEWAPI 原生 Ability 或通用分发缓存。
  模型发现与价格接口只读取等价投影。Task、create attempt、计费和审计快照描述已经发生的事实；
  素材控制面是调用方自管的无状态单资源代理，不建立客户 Asset/AssetGroup 事实；每个支持普通 AIGC
  素材组的 Channel 可以保存一个内部默认 Provider group ID，但它只是南向履约配置。
- 客户模型可使用部署方自定义名称，`model_mapping` 精确映射到代码协议登记的上游模型。公开元数据
  不返回上游身份；素材支持以 `api.assets` 为准，相同非空 `reuse_scope` 仅表示可尝试复用同一素材域。

## Internationalization (i18n)

### Backend (`i18n/`)
- Library: `nicksnyder/go-i18n/v2`
- Languages: en, zh

### Frontend (`web/src/i18n/`)
- Library: `i18next` + `react-i18next` + `i18next-browser-languagedetector`
- Languages: en (base), zh (fallback), zh-TW, fr, ru, ja, vi
- Translation files: `web/src/i18n/locales/{lang}.json` — flat JSON, keys are English source strings
- Usage: `useTranslation()` hook, call `t('English key')` in components
- CLI tools: `bun run i18n:sync` (from `web/`)

## Rules

### Documentation Governance

**99 归档目录只增不改（最高优先级硬约束）：**

- `docs/99-archive/` 只允许接收通过归档闸门的新文件：源文档的已验证事实和未完成事项必须先收敛到
  `docs/00-context/`—`docs/90-ui-ux/`，补齐 `status: historical`、`archived-at`、`source-path` 和
  `superseded-by`，确认目标路径不存在后再移动进入。文件一旦进入 `docs/99-archive/` 即永久只读；
  不得修改、覆盖、删除、重命名或格式化任何既有归档文件，包括勘误、frontmatter 刷新和链接修复。
  后续补充或修正只能写入 `docs/00-context/`—`docs/90-ui-ux/` 的当前事实文档，不得回写归档。

**60/70 目录禁止主动更新（硬约束）：**

- 开发过程中，除非用户针对当前任务主动、明确授权，否则不得在 `docs/60-marketing/` 或 `docs/70-research/` 下创建、修改、删除、移动、重命名、格式化或自动同步任何文档。一般性的“更新文档”“维护 docs”或事实收敛请求不构成该授权；只读检查不受影响。

- 架构事实写入 `20-architecture/`，产品行为写入 `10-product/`，工程步骤写入 `30-engineering/`，运维流程写入 `40-operations/`，临时实施记录写入 `50-planning/` 或 `80-dev/`。
- ADR 编号创建后永不重排、复用或回填缺号。新决策取代旧决策时，保留旧文件并更新为 `status: superseded`，同时填写 `superseded-by`。
- 架构文档必须按当前边界、职责、数据/控制流和不变量组织，不得记录实施流水。
- 未完成真实 Provider、账单、外部数据库和灰度验证的能力，不得写成生产已发布。

### Common Code Quality

**NEWAPI 原生代码最小入侵（最高优先级硬约束）：**

- 所有本地改动必须首先以降低未来接取上游代码时的冲突面、合并成本和出错风险为目标；本约束优先于本节其他代码组织偏好。
- 本地扩展优先零修改 NEWAPI 原生文件。能够通过 Link 专属 Router、middleware、service、adapter、注册表或新增文件完成时，禁止侵入原生实现。
- 新增类型、常量、辅助逻辑、适配器和可独立放置的测试等，原则上放入额外的单独文件，不为追求抽象复用而扩写、重排或重构现有上游文件。
- 现有热路径只允许保留单行调用或极窄分支；如无法做到，必须把绝大部分新增逻辑隔离到新文件，并把现有文件改动压缩到完成接线所必需的最小范围。
- 允许在新增文件中保留少量、清晰且局部的重复，以换取更小的上游文件改动与更低的未来合并冲突；不得仅为消除这类重复而扩大现有文件的修改范围。
- 禁止借功能改动之机对现有代码做无关的重命名、移动、格式化、抽象提取或顺手重构。只有在无法安全实现、无法满足既有接口或无法通过必要验证时，才可扩大现有文件改动，并必须明确说明原因。
- 每次修改 NEWAPI 原生文件都必须说明必要性、最小接线点和未来上游同步的冲突影响；无法说明时不得修改。
- 旧 Link 合同中已无用途的 publication、SKU、capability、implementation、execution binding、Access
  Plan、素材 binding、独立真人授权及其路由、字段和 UI 必须直接删除；不得增加兼容层、deprecated
  alias、双读双写、历史 decoder、空壳接口或回退分支。
- 接取上游代码必须遵循 `docs/30-engineering/上游代码合并指南.md`，并验证数据库迁移与本地保留白名单。

- New code should stay direct and readable. Prefer early returns, clear branches, and well-named local variables to deep nesting or layered control flow.
- Minimize nested function definitions. Use them only when required by a callback API or when keeping the closure local is clearly simpler than adding another symbol.
- Avoid adding package-level or module-level helper functions that have only one caller and do not express a stable business concept. Inline that logic at the call site instead.
- A separate function is appropriate when it represents reusable behavior, a required interface/framework callback, an exported API, a test fixture, or complex business logic that deserves direct tests.
- If a single-use helper is kept, its name must describe a durable domain concept rather than a mechanical step extracted only to shorten the caller.

### Backend Rules

**relaykit module independence:** The `relaykit/` Go module MUST remain independently buildable.

- Code under `relaykit/` MUST NOT import or depend on packages from the root `new-api` module, or rely on root-only configuration, generated files, or workspace wiring.
- Any change affecting `relaykit/` or its public APIs MUST be verified with `cd relaykit && GOWORK=off go build ./...`; a successful root-module build is not sufficient.

**JSON package:** All JSON marshal/unmarshal operations MUST use the wrapper functions in `common/json.go`:

- `common.Marshal(v any) ([]byte, error)`
- `common.Unmarshal(data []byte, v any) error`
- `common.UnmarshalJsonStr(data string, v any) error`
- `common.DecodeJson(reader io.Reader, v any) error`
- `common.GetJsonType(data json.RawMessage) string`

Do NOT directly import or call `encoding/json` in business code. `json.RawMessage`, `json.Number`, and other type definitions from `encoding/json` may still be referenced as types, but actual marshal/unmarshal calls must go through `common.*`.

**Database compatibility:** All database code MUST work with SQLite, MySQL >= 5.7.8, and PostgreSQL >= 9.6 simultaneously.

- Prefer GORM methods (`Create`, `Find`, `Where`, `Updates`, etc.) over raw SQL.
- Let GORM handle primary key generation; do not use `AUTO_INCREMENT` or `SERIAL` directly.
- Standard `SELECT ... FOR UPDATE` row locks built with GORM query methods in `model/` MUST use `lockForUpdate(tx)`. Do not use the legacy GORM v1 pattern `tx.Set("gorm:query_option", "FOR UPDATE")`, because GORM v2 silently ignores it and no lock is acquired. Do not duplicate `clause.Locking{Strength: "UPDATE"}` at call sites; the shared helper emits `FOR UPDATE` for MySQL/PostgreSQL and skips it for SQLite, where the syntax is unsupported. Dialect-specific locking with different semantics (for example, a MySQL next-key/gap lock) may use raw SQL only behind explicit database-type branches with valid fallbacks for every supported database.
- When raw SQL is unavoidable, account for dialect differences:
  - PostgreSQL uses `"column"` quoting, while MySQL/SQLite use `` `column` ``.
  - Use `commonGroupCol`, `commonKeyCol` from `model/main.go` for reserved-word columns like `group` and `key`.
  - Use `commonTrueVal`/`commonFalseVal` for boolean values.
  - Use `common.UsingMainDatabase(...)` for primary database branches and `common.UsingLogDatabase(...)` for log database branches.
- Do not use database-specific features without cross-DB fallback, including MySQL-only functions, PostgreSQL-only operators, SQLite-unsupported `ALTER COLUMN`, or database-specific JSON column types without a `TEXT` fallback.
- Migrations must work on all three databases. For SQLite, use `ALTER TABLE ... ADD COLUMN` instead of `ALTER COLUMN` (see `model/main.go` for patterns).
- Avoid GORM boolean default tags such as `gorm:"default:true"` when the default is a business rule already enforced by code. MySQL and PostgreSQL can normalize boolean defaults differently, causing GORM `AutoMigrate` to repeatedly issue `ALTER TABLE` on restart. Prefer setting these defaults in request/model normalization, hooks, constructors, or service logic; do not replace `default:true` with `default:1` unless the behavior is verified across SQLite, MySQL, and PostgreSQL.

**Relay and provider behavior:**

- When implementing a new channel, confirm whether the provider supports `StreamOptions`; if supported, add the channel to `streamSupportedChannels`.
- For request structs parsed from client JSON and re-marshaled to upstream providers, optional scalar fields MUST use pointer types with `omitempty` (for example, `*int`, `*uint`, `*float64`, `*bool`).
- Preserve explicit zero values in upstream relay request DTOs: absent client JSON fields must become `nil` and be omitted, while explicit `0`, `0.0`, or `false` values must remain non-`nil` and be sent upstream.
- Avoid non-pointer scalars with `omitempty` for optional request parameters, because zero values will be silently dropped during marshal.

**Link channel and protocol contract:**

- Link 身份只能来自专用类型化入口、ChannelType 和代码协议注册，不得根据模型名、价格或请求字段推断。
- 客户模型、Channel、Provider 模型和代码协议必须分离；`model_mapping` 只负责客户模型到 Provider
  模型的转换。
- Seedance 客户模型必须使用 `ChannelTypeSeedanceLink`。同一个已启用客户模型只能对应一个
  Seedance Channel；唯一性只在管理保存/启用路径校验，不增加请求时校验、数据库唯一约束、启动扫描
  或自动修复。
- Seedance 专用渠道不得写入 NEWAPI 原生 Ability 和通用内存分发池；模型发现与价格展示可以使用
  只读投影，但不能由此获得 `/v1/video/generations` 履约资格。
- Seedance 不使用 Priority、Weight、Affinity、随机分发、失败重选、跨渠道重试或 fallback。
- 南向差异必须由代码注册的 `video_upstream_protocol` / `asset_upstream_protocol` adapter 实现；
  管理员不编写协议 JSON、字段映射或状态脚本。
- Link 已发布入口以统一北向协议为第一优先级。已发布北向字段在具体 Provider 不生效时，adapter 必须
  按合同转换或忽略，不能反向导致客户请求错误；只有整个业务操作无法履约时才返回不支持。
- 不得修改 NEWAPI 原生入口去识别或拒绝 Link 模型，也不得把 ModelArk V3 请求降级到原生视频入口。
- 客户端不得透传合同外 Provider 私有参数；合同外字段必须明确拒绝或保持未发布，不得静默删除、钳制、
  降级或改义。统一北向协议定义的条件生效字段不属于合同外字段。

**Durable asynchronous execution:**

- 适用的异步 Provider POST 在发送请求字节前必须建立 durable `TaskCreateAttempt`；资金 hold、必要的额度 reservation 与 `sending` 状态必须在同一事务提交。
- 未取得可信 Provider task ID 时不得创建 `Task`。Task 创建、attempt hold 转移和 attempt 完成必须原子提交。
- 创建结果为 `unknown` 时禁止自动重发、换渠道或退款；单次轮询结果不可采信时不得直接判定业务失败。
- Task 生命周期必须使用创建时冻结的客户模型、Channel、Provider 模型、代码协议/adapter 版本、连接、
  素材和计费事实，不因当前配置、凭据或价格变化而重新选渠或重解释。
- 预扣、结算、差额、退款和补偿必须幂等。客户退款与 Provider exposure 分账；Provider 金额未知时必须保持未知，不得以客户 quota 冒充供应商成本。

**Link stateless asset proxy:**

- `/v1/assets` 只提供带客户 `model` 的单资源创建、查询、已验证更新或删除；`/v1/asset-groups` 只提供
  创建与单项查询，不提供列表、更新或删除。平台不主动删除素材组；确需清理由 Provider 管理员执行。
  平台按客户模型选择唯一 Seedance Channel，并由其 `asset_upstream_protocol` adapter 调用 Provider。
- 素材和素材组 ID 是 Provider 返回的不透明 ID；平台可返回 `asset://<opaque-id>`，但不得返回 Provider
  名称、Channel ID、账号、Region/Project、协议细节或上游原始模型。调用方保存 `model + id + reference`。
- 视频请求中的 `asset://<opaque-id>` 不查询本地数据库，也不验证所有权、应用、状态、模型、Channel、
  账号或 Provider 作用域；所选视频 adapter 原样接收引用，最终兼容性由 Provider 判定。
- 平台不建立客户 Asset/AssetGroup、素材 binding、resolver、列表索引、跨 Provider 探测、fallback、
  迁移或自动物化。允许每个支持普通 AIGC 素材组的 Channel 保存一个固定名称
  `aigctokenaigeneral` 的内部默认 Provider group ID；它不是客户资源，也不得扩展成通用 AssetGroup
  表、列表、所有权或状态机。错误 Provider ID 只发送给客户模型选定的 Provider；失败按脱敏统一错误
  返回，不换渠道。
- 默认素材组只能由管理员在已保存 Channel 的管理页面手工创建或复用；业务素材请求和升级过程不得自动
  创建。普通 AIGC 素材创建中，裁剪后非空的调用方 `asset_group_id` 必须保持原值并优先使用；仅未传、空
  或空白时使用 Channel 默认组。北向结构不合法时返回 `invalid_request`，不得静默改用默认组；Provider
  拒绝显式 ID 后也不得回退。默认组缺失时返回 `default_asset_group_not_configured`。
- `asset_group_id` 是统一北向素材创建协议的合法可选字段。普通素材组策略必须由代码注册的唯一
  `GeneralAssetGroupPolicy`（`none` / `default_fallback`）同时驱动运行时和公开元数据，不得从
  `GroupAdapter`、requirement 接口或模型名重复推断。`none` 时 adapter 不发送组字段并继续履约；直接
  素材组操作无法履约时继续返回既有 `unsupported_asset_operation`。
- `aigctokenaigeneral` 是普通北向素材组创建的系统保留名称。Channel 停用保留默认 ID，复制不继承，
  素材租户替换与删除 Channel 只删除本地默认组配置而不删除 Provider 组；真人素材继续使用 Provider
  认证产生的专用组。
- source URL 只允许出现在创建请求和当次 Provider 调用中，不得持久化、记录或返回；平台不保存媒体
  二进制。真人认证仅代理 Provider 素材组流程和认证 URL，不自建人脸表单、授权域或撤回状态机。
- 不支持的模型或操作必须根据代码 adapter 与已验证 Provider 文档明确返回
  `unsupported_asset_operation`，不得伪装为上游故障或尝试其它 Provider。

**Security and authoritative facts:**

- 不得提交、记录或输出密钥、Token、Cookie、支付凭据、生产配置、完整签名 URL、Task 私有数据或原始 Provider 响应。
- Task、资金及平台持久化客户资源必须按 `user_id + app_id` 隔离；无状态素材代理不建立平台资源所有权，
  由受信应用自行管理其 Provider opaque ID。Channel 默认素材组 ID 是渠道基础设施配置，不是客户资源。
- 主数据库是合同、Task、资金、资源和授权的持久化事实源；Redis、进程缓存和前端状态必须可重建，不得成为唯一权威。

**Billing expression system:** When working on tiered/dynamic billing (expression-based pricing), MUST read `pkg/billingexpr/expr.md` first. It documents the design philosophy, expression language, full architecture, token normalization rules, quota conversion, and expression versioning. All billing expression changes must follow that document.

**Billing safety invariants:** Quota/billing code MUST never produce a negative charge (a credit) from arithmetic overflow or unvalidated input. Apply defense in depth:

- Every user-controlled quantity that becomes a billing multiplier (image `n`, video `seconds`/`duration`, resolution/quality ratios, batch counts) MUST be bounded before it reaches quota calculation. Reject out-of-range values at request validation with a 400. Existing bounds: `dto.MaxImageN` for image generation count, `relaycommon.MaxTaskDurationSeconds` for task video duration, `maxTokensLimit` (`relay/helper/valid_request.go`) for `max_tokens`-family fields on every relay format (OpenAI, Claude, Gemini, Responses). Reuse these constants instead of introducing new ad hoc limits for the same concepts. When adding a new relay format or request DTO, bound its max-tokens and count fields in its validator from day one.
- Watch for validation bypass paths: passthrough fields (e.g. `Extra["parameters"]`), task `metadata` maps, and multipart form fields can carry the same quantities around the standard DTO validation. Any adaptor that reads a multiplier from such a path must enforce the same bound (or clamp) locally.
- Durations parsed from media metadata are user/upstream-controlled too: audio file headers (transcription token counting, TTS response duration) and upstream deduction numbers (e.g. Kling `FinalUnitDeduction`) can claim absurd values. Convert them with saturation before they become token counts.
- Never convert a computed quota or token count to `int` with a bare cast like `int(float64(quota) * ratio)`, `int(math.Round(...))` on unbounded input, or `int(decimal.IntPart())`. All quota rounding/conversion is centralized in `common/quota_math.go`; use those helpers: `common.QuotaFromFloat` (truncating) for float products, `common.QuotaRound` (half-away-from-zero) where rounding is intended, and `common.QuotaFromDecimal` for decimal products. `billingexpr.QuotaRound` delegates to `common.QuotaRound`. Do not reintroduce local conversion helpers or bare casts. Saturation bounds are int32 because quota columns (user/token/log) are 32-bit integers in the database, and every clamp/NaN fallback is logged via `common.SysError` since a single request should never approach those bounds.
- Saturation events are also audited: each helper has a `*Checked` variant (`common.QuotaFromFloatChecked` / `QuotaRoundChecked` / `QuotaFromDecimalChecked`) that additionally returns a `*common.QuotaClamp` when clamping occurred. Billing paths that compute a charge capture that clamp onto `relayInfo.QuotaClamp` (or thread it into task settlement) and, right before writing the consume/task log, call `attachQuotaSaturation` (in `service/log_info_generate.go`) which nests the marker under the log's `other.admin_info.quota_saturation` and emits a request-correlated `logger.LogWarn`. Nesting under `admin_info` makes it admin-only for free (non-admin log views strip `admin_info`). When adding a new billing path, use the `*Checked` variant and surface the clamp the same way so the anomaly stays auditable in both the admin log UI and backend logs.
- Multiplier maps go through `types.PriceData.AddOtherRatio`, which rejects non-positive, NaN, and +Inf ratios. Do not write to `PriceData.OtherRatios` directly, and do not weaken these guards.
- Pre-consume (预扣费) and settle (结算/差额) must both be safe: a saturated oversized quota must fail pre-consume with insufficient-quota, never silently wrap. When adding a new billing path (new relay format, new task platform, new adjustment hook), trace the full chain — validation → EstimateBilling/OtherRatios → quota conversion → pre-consume → settle/refund — and confirm each step preserves these invariants.
- Fields parsed into unsigned types (`*uint`) accept huge positive JSON numbers (e.g. `18446744073686646784`, a wrapped negative); a `>= 0` check is not sufficient, an upper bound is mandatory.
- Regression tests for these invariants belong with the boundary they protect (request validators, converter helpers). See `relay/helper/openai_image_request_test.go`, `relay/common/relay_utils_test.go`, and `common/quota_math_test.go` for the expected style.

**Backend test quality:** Backend tests must protect real behavior, API contracts, billing/accounting invariants, data compatibility, or regression paths.

- Do not add tests that only improve coverage numbers, prove that code happens to run, or lock in implementation details without a user-visible or cross-module contract.
- Avoid fake fuzz/stress/smoke/performance tests built from random inputs, large loop counts, sleeps, timing comparisons, or log-only assertions.
- Avoid duplicate tests that exercise the same branch with different names but no new invariant.
- Avoid tests that force incorrect provider/protocol semantics into production code.
- Avoid tests that assert private constants, select-field lists, helper internals, or file layout when observable behavior is already covered elsewhere.
- Prefer deterministic table tests with explicit inputs and exact expected outputs.
- When tests need database, request context, user group, settings, or cache state, initialize that state explicitly inside the test fixture.
- New or substantially rewritten Go backend tests MUST use `github.com/stretchr/testify/require` for setup and fatal assertions, and `github.com/stretchr/testify/assert` for non-fatal value checks.
- Avoid hand-written assertion helpers unless they encode a reusable project-specific invariant.
- When cleaning tests, preserve meaningful regression coverage. If a deleted test covered a real contract indirectly, replace it with a smaller test that asserts that contract directly.

### Frontend Rules

- Use `bun` as the preferred package manager and script runner for the frontend (`web/`):
  - `bun install` for dependency installation
  - `bun run dev` for development server
  - `bun run build` for production build
  - `bun run i18n:*` for i18n tooling
- Frontend UI text must support i18n with `i18next`/`react-i18next`. Use flat JSON locale files in `web/src/i18n/locales/{lang}.json`, with English source strings as keys.
- In React components, use `useTranslation()` and call `t('English key')` for user-facing text.
- Follow `web/AGENTS.md` for detailed frontend conventions, including TypeScript, component structure, styling, accessibility, testing, and build checks.

### Verification

- Run tests at the narrowest boundary that proves the changed contract, then expand only when risk requires it. Link channel/protocol, async state, billing, asset and verification changes must test their observable invariants and failure-closed paths.
- Documentation changes must run `task docs:check` and `task ai:check`; public API documentation changes must also run `cd web && bun run docs:validate`.
- Frontend checks use Bun. Changes under `relaykit/` must additionally pass `cd relaykit && GOWORK=off go build ./...`.

**Pull requests:**

- First compare the current git user (`git config user.name` / `git config user.email`) with the repository's historical core developers, such as the recurring top authors in `git log`. Do not change git config.
- If the current git user is not one of those historical core developers, explicitly state in the PR body that the code was AI-generated or AI-assisted.
- Always use the repository PR template at `.github/PULL_REQUEST_TEMPLATE.md` when drafting the PR title/body. Preserve the template structure and fill in the relevant sections instead of replacing it with an ad hoc format.
