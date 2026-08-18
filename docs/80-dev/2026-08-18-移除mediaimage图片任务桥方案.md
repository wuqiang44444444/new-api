---
status: current
owner: Dev Team
last-reviewed: 2026-08-18
---

# 移除 mediaimage 通用图片任务桥方案

## 1. 问题与目标

已确认的图片接入原则：

1. 图片模型北向统一走 `/v1/images/generations`（new-api 原生入口）；
2. new-api 官方已支持的 Provider / 模型不动，纯配置上线（模型名 + `model_mapping` + 价格）；
3. 第三方图片上游一事一议，不建通用架构，优先零代码、AdvancedCustom 配置直连；
4. nano banana 2 暂不上线，不为其编写任何入口转换代码。

目标模型 gpt-image-2、seedream 5、qwen image3 全部由原生渠道覆盖，零代码。现有
`media-image-task/v1` 通用图片任务桥（面向飞彩类任务式聚合上游的异步持久化图片路径，约 2000+ 行
本地代码）与上述原则冲突，决定彻底删除。本方案先记录完整删除面与执行顺序，代码删除待审阅后
另行执行，本次不改任何代码。

## 2. 当前实际情况

以下事实均经代码逐文件核对（基于当前 HEAD），行号为核对时快照。

### 2.1 四个目标模型的原生覆盖（已验证）

| 模型 | 原生渠道 | 原生入口 | 结论 |
| --- | --- | --- | --- |
| gpt-image-2 | openai adaptor（`relay/channel/openai/relay_image.go`） | `/v1/images/generations`、`/v1/images/edits` | 请求透传，配置即可上线 |
| seedream 5 | volcengine adaptor（`relay/channel/volcengine/adaptor.go:268` 生图/图生图统一映射 `{base}/api/v3/images/generations`） | `/v1/images/generations` | DTO 透传，seedream-4.0 已在默认模型列表，配置即可上线 |
| qwen image3 | ali adaptor（`relay/channel/ali/adaptor.go:112-131` 同步 multimodal-generation / 异步 text2image + `asyncTaskWait` 内联轮询；同步模型列表为运营配置 `setting/model_setting/qwen.go:44`） | `/v1/images/generations` | `qwen-image` 已在模型列表，配置即可上线 |
| nano banana 2 | gemini adaptor（模型列表已含全部变体） | `/v1/chat/completions`；images 入口原生仅支持 `imagen*`（`relay/channel/gemini/adaptor.go:64`） | 暂不上线，不写转换 |

第三方同步图片上游（OpenAI images 兼容形态）已可通过 AdvancedCustom `official_openai_images`
preset（converter `none`）纯配置接入，无需本桥。

### 2.2 特征全貌与出处

- 本地 commit `7959fde95`（2026-07-28）创建统一图片接口与异步任务持久化；`d0adc209a`（2026-08-10）
  增加 `relay/mediaimage` 协议；`4fa93e29e` 在 rc.23 恢复时重新导入。上游 `v1.0.0-rc.23` 无此代码，
  全部为本地新增。
- 北向额外暴露 `GET /v1/images/tasks/:task_id`、`Idempotency-Key` 恢复与 HTTP 202 异步语义；
  南向在 AdvancedCustom 渠道新增 `media_task_image_blocking` converter，轮询
  `GET /v1/media/tasks/{task_id}`，并把 `gemini-3.1-flash-image-preview-usage`、
  `doubao-seedream-4-5-251128`、`seedream-5-0-260128` 的逐模型校验硬编码在通用文件内。

### 2.3 删除面

#### 2.3.1 整删文件（含同名 `_test.go`）

- 协议与 relay 层：`relay/mediaimage/protocol.go`、`relay/image_task_prepare.go`、
  `relay/image_task_response.go`（内含 `TryHandlePersistedImageTaskResponse` 与
  `markSynchronousImageIdempotencyComplete`，均为图片专用）。
- AdvancedCustom 桥：`relay/channel/advancedcustom/media_task_image_blocking.go`（806 行）。
- controller / middleware：`controller/image_task.go`（`OpenAIImageTaskGet`）、
  `middleware/image_task_create_idempotency.go`。
- model 层：`model/task_media_image.go`（`TaskClientProtocolOpenAIImages`、
  `TaskMediaImagePrivateData`、`GetTaskForProtocol`、`ProjectOpenAIImageTask`）、
  `model/task_image_snapshot.go`、`model/task_lookup.go`（`GetByOnlyTaskId` 仅图片调用）。
- service 层：`service/media_image_task_lifecycle.go`、`service/media_image_task_contract.go`、
  `service/task_polling_media_image.go`。
- 计费辅助：`relay/helper/media_image_task_tiered_price.go`。
- DTO / 配置：`relaykit/dto/channel_settings_media_image.go`（本地新增，非上游）、
  `dto/channel_settings_media_image_scope_test.go`、`dto/channel_settings_media_image_test.go`、
  `dto/image_task.go`、`relaykit/dto/image_task.go`。
- 渠道测试画像：`controller/channel_test_image_profile.go`（含飞彩模型名硬编码）。
- 上游任务 trace 基建（唯一生产者是 `media_task_image_blocking.go:478`，删除后死亡）：
  `relay/common/upstream_task_trace.go`、`service/log_info_upstream_task_trace.go`。
- 图片任务专属测试：`router/image_task_router_contract_test.go`、
  `middleware/task_create_idempotency_image_test.go`、
  `middleware/task_create_idempotency_completed_no_replay_test.go`、
  `middleware/gpt_image_2_distribution_regression_test.go`、`relay/image_task_prepare_gpt_image_2_test.go`、
  `relay/gpt_image_2_task_isolation_integration_test.go`。
- 改写保留：`relay/channel/advancedcustom/image_public_contract_test.go` 为混合表——删除
  `media_task_image_blocking` 用例，保留 converter `none` 的同步图片（Qihang Seedream）用例。

无图片符号引用、原样保留的测试：`service/gpt_image_2_billing_regression_test.go`、
`dto/openai_image_gpt_image_2_test.go`、`relay/channel/openai/gpt_image_2_regression_test.go`、
`service/task_polling_test.go`、`service/task_billing_test.go`、
`service/task_billing_reconcile_test.go`、`relay/channel/advancedcustom/adaptor_test.go`。

#### 2.3.2 共享文件手术点

| 文件 | 移除内容 | 保留与理由 |
| --- | --- | --- |
| `router/relay-router.go:82-96` | 整个 `imageCreateRouter` / `imageReadRouter` 块（`TaskClientProtocol("openai_images")`、`ImageTaskCreateIdempotency()`、`GET /v1/images/tasks/:task_id`） | 必须把 `POST /images/generations` 恢复为上游 `httpRouter` 平铺形态（与 `/images/edits` 并列，保留 `Distribute()`） |
| `controller/relay.go` | `:132` 调用 `PreparePersistentImageTaskRequest`；`:169-173` 持久化分支折叠回上游 `FreeModel / PreConsumeBilling` 形态；`:186-191` `ReleaseRejectedTaskCreateAttempt` 块；`:94-97` `ContextKeyTaskCreateOutcomeUnknown` 分支（Relay 内仅图片设置）；`:420` `AppendUpstreamTaskTraceAdminInfo` | `writeOpenAITaskCreateOutcomeUnknown` 本体在共享 `controller/task_create_contract.go`，视频路径仍用 |
| `relay/image_handler.go` | `:101-103` `TryHandlePersistedImageTaskResponse`；`:128-130` `BillingTransferredToTask` 判断；`:162` `markSynchronousImageIdempotencyComplete` | `MarkTaskCreateAttemptOutcomeUnknown` 调用（:95/:112/:123）为通用 no-op 安全网，保留 |
| `model/task.go` | `:139` `TaskPrivateData.MediaImage` 字段；`:342-368` `GetTimedOutUnfinishedTasks` 恢复上游单截止形态（去掉双 platform 条件） | `GenerateTaskID`（:174）视频 `relay/relay_task.go:194` 仍在用 |
| `service/task_polling.go` | `:66-71` media-image 超时覆盖；`:184-186` media-image taskKey 特例；`:225-226` `DispatchPlatformUpdate` 的 `TaskPlatformMediaImage` 分支 | platform 分发机制、sweep、退款均为共享底座 |
| `service/task_create_attempt_manual_recovery.go` | `:64-125` 整个 `StageMediaImageTaskCreateAttemptRecovery` | `StageVideoTaskCreateAttemptRecovery`、`RecoverUnknownTaskCreateAttempt` 等保留 |
| `model/task_create_attempt_manual_recovery.go` | `:108-110` `PrivateData.MediaImage` 回填分支 | 其余为视频 attempt 共享恢复 API |
| `middleware/task_create_idempotency.go` | `:71-76` OpenAIImages 释放条款；`:117-130` `replayTaskCreateResponse` 的 OpenAIImages 分支（含 `/v1/images/tasks/` Location） | 文件本体与 ModelArk/Kling/Jimeng 分支保留（`router/video-router.go:59,83` 直接使用） |
| `relaykit/dto/channel_settings.go` | `:116` converter 常量；`:358` `IsAdvancedCustomConverterAllowed` case；`:556-559` 路径校验 case | `AdvancedCustom` 字段（:87）、`advancedCustomEndpointPathImageGeneration`（:155，:257 仍用）保留 |
| `dto/relaykit_aliases.go:64` | converter 别名 | — |
| `relay/channel/advancedcustom/adaptor.go` | `:181-183`（ConvertImageRequest）、`:280-282`（DoRequest）、`:303-304`（DoResponse）三处 converter case | none/claude/gemini converter 保留 |
| `relay/common/relay_info.go` | `:120-122` `BillingTransferredToTask`、`:127-129` `PersistedImageTask` | `SkipRequestRefund`（视频 `relay/relay_task.go:304`、`controller/relay.go:546` 在用）保留 |
| `relay/helper/price.go:80-82` | `ModelPriceHelperMediaImageTaskTiered` 分发分支 | `tiered_expr`、`modelPriceHelperTiered` 与 billingexpr / 用户模型合同定价共享，保留 |
| `constant/task.go` | `:8` `TaskPlatformMediaImage`、`:20` `TaskActionImageGeneration` | Suno/MJ/视频使用各自 action，不受影响 |
| `constant/task_context_key.go:11` | `ContextKeyTaskPersistenceEnabled` | 其余 key 视频共享 |
| `constant/env.go:22` | `MediaImageTaskMinTimeoutMinutes` | `TaskTimeoutMinutes` 共享保留 |
| `common/init.go:201-207` | TASK_TIMEOUT 低于图片下限的告警块 | 其余共享 |
| `controller/channel-test.go:716-717` | `EndpointTypeImageGeneration` 改回内联通用 `1024x1024` 图片测试请求 | — |
| `model/task_contract_foundation_test.go:54,65` | `TaskClientProtocolOpenAIImages` 替换为视频协议常量 | 共享测试保留 |

#### 2.3.3 前端（`web/src`）

- `features/channels/types.ts:168` 删除联合类型成员 `'media_task_image_blocking'`；
- `features/channels/lib/advanced-custom.ts:76-80`（converter 选项）、`:414-419`（路由默认值）、
  `:851-854`（路径校验分支）删除；整删测试
  `features/channels/lib/__tests__/advanced-custom-media-image.test.ts`；
- 保留 `:270` `official_openai_images` preset（converter `none`，第三方同步图片配置直连路径）。

#### 2.3.4 公开 API 文档中心与 OpenAPI

- 删 `web/public/docs-content/zh/api-reference/images/task-get.md`（`GET /v1/images/tasks/{task_id}`）；
- `images/generations.md:57,61` 去除 202/异步措辞，改为仅同步；
- `manifest.json:118-121` 删 `images-task-get` 条目；重生成 `generated/search-index.json`；
- `docs/openapi/relay.json:482` 删 `/v1/images/tasks/{task_id}` 路径。

#### 2.3.5 cmd/

- `cmd/verify-feicai/` 为视频专用（`feicai_videos_v1`、ffprobe、seedance thirdparty），保留；
- `cmd/verify-feicai-v2/`、`cmd/split-feicai-channel52/` 为无文件的空目录，仅 `rmdir`；
- `relay/channel/task/seedance/thirdparty/feicai/` 中的 `media_url.go` / `model_spec.go` 是视频参考图
  素材逻辑，与本特征无关，保留。

#### 2.3.6 数据库与存量数据

- 无图片专属表或列：复用共享 `tasks` 表（`platform='media_image'`、`client_protocol='openai_images'`、
  `private_data` 内的 `media_image` JSON）与共享 `TaskCreateIdempotency` / `TaskCreateAttempt` 表
  （`model/main.go:276-277,356-357`），后两者为视频共享，不得从 AutoMigrate 移除；
- 残留 `media_image` 任务行与幂等记录不做迁移；删除轮询分发后，预期由共享超时 sweep
  （恢复单截止后的 `GetTimedOutUnfinishedTasks`）统一置失败并退款，列为 2.5 待验证项。

### 2.4 必须保留的共享底座（明确不动）

- Seedance / ModelArk / Kling / Jimeng 视频链路：`relay/task_create_disposition.go`、
  `relay/task_lifecycle.go`、`relay/task_upstream_error.go`、`controller/task_create_contract.go`、
  `controller/task_protocol_error.go`、`controller/task_protocol_snapshot.go`、
  `router/task-contract-router.go`、`controller/task_contract_operations.go`；
- task create attempt 共享底座：`service/task_create_attempt.go`、`model/task_create_attempt*.go`
  （仅按 2.3.2 移除图片钩子）；
- 幂等底座：`middleware/task_create_idempotency.go` 文件本体、`middleware/task_protocol.go`
  （`TaskClientProtocol`，视频路由在用）、`relay/channel/api_request.go:510-512`
  （`ContextKeyTaskUpstreamStarted`，视频幂等同样依赖）；
- 计费底座：`pkg/billingexpr`、`relay/helper/task_tiered_price.go`、`service/tiered_settle.go`、
  `setting/billing_setting/task_billing.go`、`ApplyCustomerContractRatio` 等合同定价共享代码；
- 任务结算内部件：`refundTaskWithReconcile`、`recalculateTaskQuotaWithReconcile`、
  `setTaskBillingState`、`persistTaskBillingFailure`、`Task.UpdateBilling`、
  `model.AttachAsyncTaskBilling`、`LogTaskConsumption`、`FinalizeTaskCreateAttemptBillingTransfer`、
  `MarkTaskCreateAttemptOutcomeUnknown`。

### 2.5 待验证项（执行时确认）

1. 删除后残留 `platform='media_image'` 任务行的实际走向：确认共享超时 sweep 会将其置失败并退款
   （含幂等 claim 释放），必要时补一个最小回归测试；
2. `model/task_video_lifecycle.go:193` `ValidateTaskProtocol` 当前零调用者（死代码）：随本删除一并
   移除整个函数，或仅删 OpenAIImages case——执行时按最终调用图决定；
3. `service/task_create_attempt.go:320-329` `CompleteSynchronousTaskCreateAttempt` 唯一调用者在
   `relay/image_task_response.go:42`：删除后确认其与 `CompleteTaskCreateAttemptWithoutTask` 是否
   成为死代码并一并移除；
4. `controller/channel-test.go:52-60` 的 AdvancedCustom 端点归一化块虽由图片 commit 引入，但服务于
   所有单端点 AdvancedCustom 模型的测试，倾向保留——执行时确认无图片耦合后保留。

## 3. 优化方案

### 3.1 执行批次（审阅通过后）

1. **批次一（边缘配置）**：DTO / 常量 / 前端 channel 配置（2.3.1 的 DTO 文件、2.3.2 的
   `relaykit/dto/channel_settings.go` 等、2.3.3 前端）；
2. **批次二（service / model）**：`service/media_image_*`、`service/task_polling_media_image.go`、
   `model/task_media_image.go` 等，及 2.3.2 对应手术点；
3. **批次三（relay / controller / adaptor）**：AdvancedCustom 桥、`image_task_*`、trace 基建及
   `controller/relay.go`、`relay/image_handler.go` 手术点；
4. **批次四（router）**：恢复上游平铺 `/images/generations` 路由，删 `/v1/images/tasks/:task_id`；
5. **批次五（docs）**：按 3.2 收敛文档与公开 API 文档中心；
6. 每批次后构建 + 定向测试，保持可编译可回滚。

### 3.2 文档收敛

- 删除 `docs/20-architecture/Link图片服务合同与异步任务架构.md`；「图片走原生渠道配置上线、第三方
  一事一议配置直连、不建通用图片任务架构」的原则折叠进 `docs/20-architecture/架构概览.md`；
- 更新引用：`docs/20-architecture/README.md:63`（删图片数据面行及阅读顺序第 6 条中的图片项）、
  `docs/00-context/硬约束.md:113`、`docs/20-architecture/Seedance专用渠道与Link架构.md:350`、
  `docs/20-architecture/账单计费-异步任务与计费事实架构.md:205`；
- ADR 措辞修正（不改编号不改决策）：ADR-0008:14 去除「持久化异步图片任务」、ADR-0016:69 去除
  「共享 `media-image-task/v1` Task 生命周期」引用；
- `docs/10-product/验收标准.md:85`（§7 图片）删除 `media-image-task/v1` 生命周期条目并复核 84-87 行；
- `docs/40-operations/03-图片渠道与异步任务运维手册.md`：删除 Moxing `media_task_image_blocking` 与
  `GET /v1/images/tasks/:task_id` 相关章节（§1 部分、§2、§4 部分、§5-§8），保留 Qihang 同步路径
  （converter `none`）内容；同步 `docs/40-operations/README.md:25`、`05-全渠道上线验收手册.md:30,77`、
  `运维手册.md:15`；
- `docs/30-engineering/图片模型API用户调用指南.md` 改为仅同步（删 §6 异步 202、§7 任务查询、§8.1、
  §9 异步分支）；`公开API文档维护指南.md:79` 示例措辞改「创建图片」；`README.md:26` 链接更新；
- `docs/50-planning/路线图.md:104-112` 删「统一图片渠道生产验收」中 200/202 联调项，:159 附近措辞
  调整；`docs/50-planning/变更记录.md:148,150,154` 按历史记录处理，:154 的失效指南引用更新。

### 3.3 存量数据处置

不迁移、不建清理任务。删除轮询分发后依赖共享超时 sweep 将残留 `media_image` 任务终结退款；
若 2.5.1 验证不成立，再评估一次性脚本（另行决策），不进入本次范围。

### 3.4 验证

- `go build ./...` 与 `cd relaykit && GOWORK=off go build ./...`；
- 定向测试：`go test ./router/... ./middleware/... ./relay/... ./relay/channel/advancedcustom/...
  ./service/... ./model/...`（含保留的 gpt-image-2 回归与改写后的 image_public_contract_test）；
- `task docs:check`、`task ai:check`；`cd web && bun run docs:validate`（重生成 search-index 后）；
- 手工核对：`POST /v1/images/generations` 在 openai / volcengine / ali 渠道上行为与上游一致；
  `GET /v1/images/tasks/:task_id` 返回 404；视频任务（Seedance/Kling/Jimeng）回归不受影响。

### 3.5 明确不做

- 不为 nano banana 2 编写 images 入口转换；
- 不为第三方图片上游建立任何通用任务桥、协议标识或逐模型校验框架；后续第三方图片接入默认
  AdvancedCustom 同步配置直连，无法配置直连时一事一议单独决策；
- 不迁移、不双读双写、不留 deprecated alias 或空壳兼容层（遵循硬约束「无用途即删除」）。
