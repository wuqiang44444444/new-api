---
status: current
owner: Dev Team
last-reviewed: 2026-09-06
---

# 图片服务与中转 Provider 适配架构

## 0. 完成度口径

本文描述代码已实现的事实。统一图片服务的“已实现 / 待实现 / 证据门控”三分口径以
[路线图](../50-planning/路线图.md) 第 4 项未完成项清单为唯一权威（历史拆解见
[归档的实施计划](../99-archive/2026/09/2026-09-05-统一图片服务实施计划.md) §5）；
编辑参数面（quality/mask/透明背景等）、中转 edits、SSE 流式与 file_id 等在取证/实现完成前
显式 `400`，不属于已发布能力。

## 1. 范围与状态

本文描述统一图片北向合同、原生 Gemini/Vertex 图片履约、显式异步执行底座、统一图片中转渠道
及其代码协议适配。图片创建继续使用 NEWAPI 原生 `POST /v1/images/generations`、
`POST /v1/images/edits`；Seedance Link 的 ModelArk V3、视频 Task 与无状态素材代理不进入图片入口。

图片执行形态（同一对北向入口）：

| 形态 | 选择方式 | 执行位置 |
| --- | --- | --- |
| 同步 | 默认 | 本次 HTTP 请求内等待并返回 OpenAI Images 响应 |
| 显式异步 | 请求头 `Prefer: respond-async` | 受理事务提交后返回 `202` + 平台任务 ID，后台 worker 执行，`GET /v1/tasks/{task_id}` 查询 |

显式异步由代码登记的图片执行协议 `image_openai_v1` 承载（硬约束 §4）。受限 v1 代码已实现
（完成度三分表见上）；真实 Provider、账单与生产灰度尚未验收，“代码已实现”不等于“生产已发布”。

## 2. 统一北向合同层（G1 v1）

`service/image_contract.go` 是与 Provider 无关的合同解析层，被同步 relay、受理事务与异步
worker 共同复用；族（模型）级字段生效矩阵由各 adapter 决定。字段合同冻结表的历史现场见
[归档的实施计划](../99-archive/2026/09/2026-09-05-统一图片服务实施计划.md) §2；当前字段合同以
本文 §2—§4 与[图片模型 API 用户调用指南](../30-engineering/图片模型API用户调用指南.md)为准。核心规则：

- `n`：未传/null 为 1；显式 `0` 或超过统一上限 10 一律 `400`，不钳制、不循环（P3/E6）；
  `dto.MaxImageN=128` 仍是无差别安全上限。
- 输入图（仅 edits）：multipart `image`/`image[]`、JSON `images` 数组（Data URL 或 HTTPS URL）、
  或单图 `image` 字符串；`image` 与 `images` 互斥。URL 只计数不下载（U1/U2）。
- 预算（E5）：最多 14 张；单张解码后 ≤ 20 MiB（20×1024×1024 字节），合计 ≤ 50 MiB，
  按解码字节计。
- `mask`：v1 全族未发布，统一 `400`，不降级为提示词（E2/C3）。
- 三态字段语义：未传、`null`、显式空串视为未设置；显式 `false`/`0` 合法（E6）。
- `Prefer: respond-async` 与 `stream=true` 互斥，在受理、预扣、上游调用前报冲突（P14）。

## 3. gemini_image 族（Gemini 24 / Vertex 41）

Gemini/Vertex 的 `generateContent` 图片模型（imagine 登记表，`setting/model_setting`）通过
`relay/channel/gemini/image_generate_content.go` 履约标准图片合同；imagen `:predict` 路径保持
原语义。Vertex 复用同一转换与响应核心，仅承载各自认证、项目与区域。

- 请求：`contents=[{role:user, parts:[text, 输入图…]}]`、
  `generationConfig.responseModalities=["TEXT","IMAGE"]`；`size` 映射 `imageConfig`
  （`auto`→不发送；`WxH`→精确宽高比 + 按长边映射 1K/2K/4K 档）。每边不超过 4096，
  且比例必须属于南向支持集合；不接受 `a:b` 或“最近比例”。返回图片仅允许等比例缩放到请求像素，
  上游比例不符时报交付错误，不裁切、补边或拉伸；`n` 恒为 1，其它值在 Provider 调用前 `400`。
- 二进制输入转 `inlineData`；HTTPS URL 以 `fileData` 原样透传（不下载、不改写）。
- 未发布字段显式拒绝：`quality`、`style`、`background`、`moderation`、`output_format`、
  `output_compression`、`watermark`、`input_fidelity`、`partial_images`、`stream=true`、
  未知顶层字段（缺证据即阻断，C3/P7—P11）。
- 响应：只取最终图片 parts（`thought` 标记、纯文本、非图片 inlineData 排除）；零图 +
  安全拒绝显式失败，不当成功空数组（R1/R5）；usage 采用可信 `usageMetadata`（R4）。
- `response_format`：默认 `b64_json`；显式 `url` 在对象存储启用时逐张返回 300 秒签名 URL，
  否则 `400`（P13，不以 Data URL 冒充）。
- 同步标准 Google 图片请求的 HTTP 交换失败不证明未发送，传输错误禁止自动重试；
  该保护仅接入图片错误出口，不改变原生聊天或其它模型的重试策略。
- Gemini/Vertex 图片转换成功后，南向请求头使用 `application/json` 匹配 generateContent
  正文；multipart 北向请求头保持原值，未经过图片转换的请求沿用原生行为。

## 4. 显式异步执行（image_openai_v1）

受理与执行的全部持久事实都在 Task 与既有共享底座上，不建第二套任务/账本：

```text
POST /v1/images/{generations|edits} + Prefer: respond-async
  -> ImageHelper 窄分派 imageAsyncHelper（controller 跳过请求级预扣）
  -> 合同/族校验 -> 输入二进制落私有 OSS（URL 不搬运）
  -> 受理事务：容量槽位 + 钱包/令牌额度预扣 + Task(QUEUED/FundsHeld) + 幂等绑定，一次提交
  -> 202 {id, status:queued, query_url}
后台 image_task_execute system task（10s 周期）：
  恢复扫描（排队过期/租约过期/已保存结果） -> 领取(CAS+执行槽位)
  -> SENDING 持久提交（发送许可，之后才允许写出请求字节）
  -> 冻结事实驱动 Provider 调用（复用各 adaptor ConvertImageRequest）
  -> 结果归一 -> 生成结果清单/实际 usage 持久化 -> 下载/上传 OSS，逐图登记
  -> 终态 CAS -> 按冻结价格与实际 usage 计算目标 -> 共享原子结算（失败退款；未知待核实）
GET /v1/tasks/{task_id} -> 图片投影（user_id + app_id 双重归属）
```

关键不变量：

1. Task 自身状态机承载发送许可：`QUEUED → IN_PROGRESS(SENDING 提交后) → 终态`；停留在
   SENDING 的租约过期任务置 `RECONCILIATION_REQUIRED`，禁止自动重发、换渠道或退款（R7）。
   可信上游任务 ID（FunCloud）取得即持久化；此后查询错误保持待核实，不能按创建拒绝退款。
   待核实任务由恢复查询入口只查询续查，不重建；候选按最久未更新优先并以 ID 稳定排序，
   分页使用更新时间与 ID，避免持续不可恢复的首批任务挡住后续候选。
2. 已证实从未发送（`SentAt=0`）的过期租约释放执行槽位回队重派；排队期限（
   `IMAGE_ASYNC_QUEUE_SECONDS`）已过且从未发送的任务安全失败并退款。
3. 资金单一所有者：受理的全部事实与钱包/令牌预扣同事务；失败整笔回滚，不创建“已结算”补偿任务。
   `service/image_task_billing.go` 只用冻结价格、表达式、加密请求探针与持久化实际 usage 计算目标；
   资金差额、Task 计费状态、最终用量统计及受理槽释放由 `ApplyTaskBillingTarget` 同事务提交。
   不调用旧非原子重算入口。固定价或缺失 usage 保留预扣，显式零 usage 不伪造 Token。异步 v1 使用钱包。
4. 容量与背压（§3.10）：受理上限（全局等待 `IMAGE_ASYNC_MAX_WAITING`、每 user+app 未完成
   `IMAGE_ASYNC_MAX_PER_APP`）与执行并发（`IMAGE_ASYNC_EXECUTE_CONCURRENCY`、每渠道
   `IMAGE_ASYNC_CHANNEL_EXECUTE_CONCURRENCY`）以 `image_task_slots` 计数行在事务内
   `FOR UPDATE` 校验并递增；受理、领取、终态释放、计费释放和重建均先锁同一全局计数行。
   状态和槽位变更同事务，终态待结算仍保留受理占用。计数可由 Task 重建；
   应用超限 `429`、全局受理容量耗尽 `503`，均不受理、不预扣、不发送。
5. 旧视频轮询 feeder、通用超时退款与请求结束退款按 `client_protocol` 排除图片任务（NULL
   安全谓词）；图片任务的退款/结算复用 `AsyncBilling` 状态机与补偿扫描。
6. 幂等（§3.7）：`Idempotency-Key` 仅异步模式支持（同步请求携带返回 `400`）；键域
   user + app(token) + 操作 + 客户键，摘要进既有 `TaskCreateIdempotency` 表；重放返回原
   任务 ID；不同请求体 `409`；绑定任务未终态期间 claim 到期不重置；multipart 摘要忽略
   boundary；受理未完成窗口内重放返回 `in_progress`，不签发第二个 202。
7. 输入/结果对象键为 `images/tasks/{taskID}/input-N|result-N`。结果上传前持久化清单与 usage，
   上传失败在执行预算内只重试保存，逐图登记失败不丢弃其它已生成图片。恢复领取占用相同执行槽位，
   并通过版本 CAS 排除过期 worker；无 Provider ID 也能按清单 HEAD 补登记。部分结果在待核实时可查询。
   没有成功落存储、没有可重查来源且进程已丢失的字节不能凭对象键恢复，保留待核实并转人工处置；
   不自动重生成或退款。FunCloud 恢复每轮只查询一次已有任务。

## 5. 对象存储（upstream / S3 兼容 / Azure Blob，G9）

数据库是对象存储配置的唯一持久事实：完整标准化配置以单行 JSON 持久化在 Option 表
（`ObjectStorageSetting` 命名空间，凭据为 `objstore.v1.` 加密信封密文），由
`updateOptionMap` 唯一接线点转发给 service 观察者装载；普通选项接口不进入、也绕不过
该命名空间。装载即初始化（替代旧的包 `init` 环境变量装载），每次以完整不可变配置
快照原子替换存储实例，revision 未变化不重建；多节点经既有 `SyncOptions` 周期刷新。
启动环境变量 `TASK_ARTIFACT_STORE_*` 不再是运行期配置源，只提供一次性显式导入
（预览不含密钥，验证通过后写入数据库；导入完成后旧变量不覆盖、不作失败 fallback）。

存储类型三种：`upstream`（未启用行为；已启用存储的停用须离线维护，不删除远端
对象）、`s3`（path-style、代码内 SigV4，零新依赖，签名实现通过 AWS 官方测试向量；
MinIO/R2/OSS 等兼容端点可用）、`azure_blob`（原生 Azure Blob，Shared Key 请求签名与
service SAS 由官方 azblob SDK 承担；凭据支持连接字符串或手动录入，连接字符串按首个
等号拆分、保留 Base64 尾部等号，冲突重复属性与 SharedKey 之外的鉴权方式显式拒绝，
含 BlobEndpoint 时显式生效；标准化后不保存原始连接字符串）。加密信封主密钥从部署侧
稳定主密钥（`CRYPTO_SECRET`/`SESSION_SECRET`）域分隔派生，不与数据库密文同表存储；
各节点主密钥必须一致，解密失败存储失败关闭为禁用并告警。配置读写与连通性测试走专用
最高管理员接口（`/api/option/object_storage*`）：读取返回脱敏账号与
`credential_configured`；「测试连接」只测当前表单值、密钥未修改时后端按
backend+账号复用已存密钥；「保存并启用」对同一完整配置先校验再验证、通过后原子保存，
测试失败保留既有配置。测试在目标容器创建随机对象验证 PUT（If-None-Match 条件）、
HEAD、鉴权 GET、短期签名 GET、删除与删除后 404；只清理本次创建的对象，清理失败单独
提示；结果不回显签名 URL、Authorization 或上游响应正文。

统一存储能力：图片业务只依赖 `imageObjectStore` 最小接口（put/HEAD/签名/读取），
不断言具体 S3 类型；S3 与 Azure Blob 实现各自履约，对象命名与前缀语义一致。
图片结果签名固定 300 秒；旧 `TaskArtifactStore` 消费者沿用 900 秒默认值。
Azure SAS 的起止时间统一使用 UTC；生效时间向前容错两分钟，过期时间从当前时刻计算，
不受宿主机本地时区影响。

图片输入暂存、执行/恢复、同步 URL 投递和一次任务查询通过 `WithImageObjectStore` 绑定同一不可变
存储实例，凭据刷新不拆分一次操作。图片回读传递调用方取消与总预算，最大 64 MiB，超限明确失败而非
返回截断图片；此上限不用于客户 HTTPS 参考图。任务查询总预算 60 秒，单次 HEAD 最长 5 秒。

异步受理预扣前及后台领取待发送任务前，以绑定实例做小对象 PUT/GET 一致性检查（最长 5 秒）。
同节点同实例合并并发探测，成功缓存 10 秒、失败 2 秒；实际图片写入失败使缓存失效。健康检查对象位于
`object-storage-health/`，每实例复用一个无客户内容的随机对象，清理由桶生命周期承担。探测不可用时
新受理返回 503，后台不领取或提交 SENDING；排队期限仍生效。短期观测不保证之后写入成功，发送后的
故障继续按既有交付恢复/待核实处理，不重生成。

- 异步输入与最终图片存私有桶；数据库只保存对象引用、归属与 MIME（`PrivateData.ImageTask`），
  不持久化签名 URL；源 URL 只存在于冻结快照，不进日志或公共响应。
- 结果投递：图片查询授权后逐张 HEAD 校验并签发 300 秒 presigned GET URL 与 `url_expires_at`；
  到期后再次授权查询续签；显式 `b64_json` 逐张读取对象原文返回。完整签名 URL 不进入日志。
- 逐图可用性（评审 S12）：HEAD 404 → `deleted`（保留历史生成状态，不抹除其它图片）；其它探测/
  读取错误 → `unavailable`（不判删除、不伪装可交付）；仅可用图片签发 URL/b64。
- Provider 结果 URL 下载必须走 SSRF 防护客户端（拨号校验，评审 S11）；管理员上游连接客户端
  不得复用为任意媒体下载器。
- 既有 `task_artifact_access` 长期 capability 保留原消费者；`TaskArtifactStore.Resolve` 只对
  有持久化登记事实的图片产物返回引用（评审 S5），未入库对象交回原 Provider 下载路径，启用
  对象存储不改变旧产物行为。存储不可用时拒绝新异步受理。
- 首次启用后，存储位置边界（backend、账号、端点、容器、前缀、region）在线固定，停用或更换须
  离线维护。配置保存事务统一拒绝位置变更，不以进行中任务计数证明历史/同步对象已无人依赖；
  第一版无后台迁移、多配置路由或跨存储探测。
  同账号容器内轮换密钥经测试后保存；不保存第二把备用 Key，不自动尝试旧凭据。
- 清理由部署方桶生命周期策略承担；平台不新增图片清理任务。

## 6. 图片中转渠道（ChannelTypeAsyncImage=63）

管理面仍只有 `ChannelTypeAsyncImage` 一个图片中转类型，协议、模型映射与保存校验
（`model/channel_image_relay.go`）不变：

| 协议值 | Provider 合同 | 同步执行 | 异步执行 |
| --- | --- | --- | --- |
| `funcloud_aigc_v2` | FunCloud `/api/v2/open/aigc/*` | 创建后在本次请求内轮询 | worker 内创建+轮询（`asyncimage/headless.go`） |
| `moxing_images_v1` | Moxing `/v1/images/generations` | 单次同步 POST | worker 内单次 POST（`moxingimage/headless.go`） |

- 同步模式继续返回 Provider URL；成功结果按 `data[]` 交付 Provider 返回的全部合法 URL
  （零合法 URL 失败关闭），不补生成（R2/R3）。
- 异步模式下结果下载后保存私有 OSS 并按 300 秒签名交付。
- edits、mask、参考图与 `stream=true` 在 Provider 计费证据完成前显式 `400`（B7/G 门控）；
  该未发布状态是证据门控，不是合同缺失。
- 10 分钟总时限（`relay/channel/image_relay_timeout.go`）只约束同步模式；异步模式使用
  worker 的排队/执行/存储预算。

## 7. 公开投影与管理测试

- `api.image` 投影新增 gemini_image 族（`pkg/publicmodel/image_gemini.go`）：按管理员映射后
  落在 imagine 登记表的 Provider 模型识别（不从客户模型名推断），同时发布 `create_image`
  与 `edit_image` 操作及逐字段参数；图片中转与原生图片模型投影规则不变。
- 管理端渠道测试：Gemini/Vertex 渠道在映射模型为 imagine 登记模型时默认使用图片生成
  endpoint（`controller/channel_test_image_profile.go`）；图片中转渠道测试维持协议感知尺寸。

## 8. 架构不变量

1. 图片统一使用 NEWAPI 原生图片入口；异步是同一入口的显式模式，不建第二套客户 API。
2. 显式图片执行身份只来自 `Prefer: respond-async` + 代码登记协议 `image_openai_v1`，不从
   模型、价格或请求字段推断。
3. 受理容量、钱包/令牌预扣、Task 写入与幂等绑定一次事务提交；任何失败全部回滚。
4. 发送字节前必须持久提交 SENDING；发送后的一切不可判定结果按待核实处理，不自动重发、
   换渠道或退款。
5. 图片 Task 按创建时冻结的渠道、连接、模型与计费事实执行，不因当前配置重选渠道。
   自定义请求头在受理时由既有 `ResolveHeaderOverride` 解析，按 Task ID 绑定加密快照；
   发送和恢复只应用冻结值（含 Host），不再次解释模板。快照缺失或不可解密时保持待核实，
   不从当前渠道补取请求头。
6. 管理面只有一个图片中转 ChannelType/APIType；南向协议必须由管理员显式选择。
7. `model_mapping` 完全由管理员维护；代码只解析当前客户模型并校验最终 Provider profile。
8. FunCloud 与 Moxing adaptor 独立履约，不共享请求 DTO、轮询状态或响应猜测。
9. 不支持字段显式失败；成功交付零图即失败关闭。
10. 模型列表、单模型详情和价格目录必须返回一致的图片入口与逐模型参数合同。
11. 敏感凭据、媒体正文、完整签名 URL 与 Provider 原始响应不进入日志、公共响应或普通任务字段。
12. 代码实现不能替代真实 Provider、账单、超时 exposure、外部数据库和生产灰度验收。

## 9. 代码事实与相关文档

主要代码事实：

- `service/image_contract.go`（统一合同层）、`relay/image_async.go`（受理）、
  `relay/image_task_executor.go`（Provider 执行器）、`service/image_task_worker.go`（worker）；
- `model/task_image_lifecycle.go`、`model/image_task_slots.go`（任务生命周期与容量槽位）；
- `relay/channel/gemini/image_generate_content.go`、`relay/channel/vertex/adaptor.go`（窄分支）；
- `relay/channel/asyncimage/headless.go`、`relay/channel/moxingimage/headless.go`；
- `service/sigv4.go`、`service/task_artifact_store_s3.go`、`service/task_artifact_store_azure.go`、`service/task_artifact_store_runtime.go`、`service/task_artifact_store_image.go`、`service/object_storage_probe.go`（S3/Azure 存储、运行时装载、图片能力接口与连通性测试）；
- `setting/system_setting/object_storage.go`、`common/object_storage_credential.go`、`model/object_storage_setting.go`、`controller/object_storage_admin.go`（标准化配置、凭据信封、持久化分发与专用管理接口）；
- `middleware/image_create_idempotency.go`、`controller/image_task_query.go`、
  `controller/system_task_image_handler.go`；
- `pkg/publicmodel/image_gemini.go`、`model/public_image_model_api.go`、
  `controller/channel_test_image_profile.go`；
- `setting/system_setting/image_task.go`（容量与预算默认值）。

相关当前事实：

- [架构概览](架构概览.md)
- [异步任务与计费事实架构](账单计费-异步任务与计费事实架构.md)
- [公开模型元数据投影架构](公开模型元数据投影架构.md)
- [图片模型 API 用户调用指南](../30-engineering/图片模型API用户调用指南.md)
- [图片渠道与异步任务运维手册](../40-operations/03-图片渠道与异步任务运维手册.md)
