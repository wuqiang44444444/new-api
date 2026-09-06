---
status: accepted
owner: Dev Team
last-reviewed: 2026-09-06
---

# 统一图片生成、编辑与异步查询方案

## 1. 问题与目标

以“北向统一，南向适配”交付生成、参考图、编辑、多图融合及可选异步查询。
本文件记录用户已接受的目标与待验证门槛，不是已发布合同；方案轮只做设计收敛，实施轮（同日，
用户授权全量实施）已完成代码落地，修复说明见 §7。保留原文件名以维持引用，当前事实以正式架构、
硬约束与[统一图片服务实施计划](../50-planning/2026-09-05-统一图片服务实施计划.md)为准。

### 1.1 唯一范围边界

| 对象 | 要做什么 | 不做什么 |
| --- | --- | --- |
| Gemini / Vertex | 仅补既有生成/编辑入口的南向转换与共享同步/异步接线；复用认证和连接。 | 不重建类型/认证、不迁到图片中转、不改聊天或重构整个 adapter。 |
| 异步图片渠道 | 类型 63 完整改造：参数、生成/编辑、执行、查询、幂等、OSS、多图、计费接线、公开投影及管理测试；包含 93，不限于该实例。 | 旧 url-only、只生成、固定单图不能继续作为公共合同；不要求客户发送 Provider 私有结构。 |
| 共享底座 | 最小扩展既有图片入口、Task、计费及存储，共用任务事实和查询。 | 不建平行任务/账本，不借此改视频或扩展其他原生渠道功能。 |
| 其他原生渠道 | 仅回归共享改动影响，保留功能和默认行为。 | 不新增能力、迁移类型或改参数合同。 |

“仅新增接口支持”是为既有 `POST /v1/images/generations`、`POST /v1/images/edits` 增加履约，
不是新增北向路径，也不是让 Google 上游采用 OpenAI 路径。共享图片底座提供同步/异步，不按 Provider 各建一套。
“异步图片渠道”是类型名称，不限制客户模式或上游执行方式。

本轮取代旧结论：其他原生渠道不再泛化纳入新增功能；93 所在类型需完整改造；保留同步并新增显式异步；
OSS 从 TODO 升为本期异步依赖；结果由全局单图改为多图，Google 的 n=1 例外不变。

### 1.2 中转站业务目标与验收

原生已有渠道优先复用，原生没有的中转站使用图片中转渠道；中转站是供应来源，不是另一套客户产品。
本期原生实施范围仍以 §1.1 为准。

| 编号 | 业务目标 | 可观察的验收结果 |
| --- | --- | --- |
| B1 一次接入 | 客户只面对客户模型与统一合同。 | 切换已发布能力模型无需改接口、输入结构、尺寸含义和查询；模型限制可发现（C1—C4、V3）。 |
| B2 完整工作流 | 文生图、参考图生成、编辑、多图融合。 | 两个操作、三种输入和适用标准参数均真实履约（P1—P14、E1—E6）。 |
| B3 南向差异内收 | 技术人员维护代码协议，管理员配置渠道。 | adapter 转换路径、字段、认证、状态及 usage；客户不填写协议 JSON、上游 ID 或私有参数（C2、V2）。 |
| B4 同步与异步统一 | 兼顾标准同步客户端与长耗时任务。 | 相同创建路径/参数，以请求头选择 202 受理；同步及异步上游均使用统一平台查询（§3.7—3.8）。 |
| B5 低流量交付 | 任务可追踪，OSS 按需下载。 | 异步默认 URL、5 分钟续签、显式 Base64、对象删除与历史生成事实分离均符合 §5。 |
| B6 多图交付 | 如实交付多图与部分成功。 | data[] 完整、终态与未知分开、不补生成，符合 R1—R3、§3.9。 |
| B7 商业配置自主 | 管理员掌握模型、映射、连接和价格。 | 不自动写配置/价格；固定价、token、表达式按冻结规则执行，用量和资金可审计（§2.2、R4/R8/R9）。 |
| B8 控制重复成本 | 重提、重启和异常不造成无依据重复生成/收费。 | 单一受理和持久恢复符合 R7、§3.7—3.8。 |
| B9 隔离与安全 | 客户只访问自己的任务/图片，不获知内部配置。 | user_id + app_id 授权、脱敏及部署方 OSS 管理边界符合 V4、§5。 |
| B10 持续接入中转 | 新协议复用合同和底座，不形成第二套客户产品。 | FunCloud、Moxing 分别完成全链路验证；未来协议仍需 adapter 和证据，不承诺未知供应商自动兼容（C5、G11）。 |

客户流程：客户模型与统一参数提交 → 同步结果或平台任务 ID → 同一入口查询状态及结果 → URL 过期后授权重查。
管理员负责渠道/映射/价格，技术人员负责代码协议与证据，平台负责任务/资金/交付，部署方负责 OSS 生命周期。
不默认增加回调、取消接口、通用工作流引擎或客户可编辑协议脚本。适用标准语义无法履约时须报告证据和范围冲突，
不能自行缩减合同或用“字段已接收”宣布完成。B1—B10 分别由 §3 的规则实现，并由 G11 逐项验收。

## 2. 当前实际情况

### 2.1 仓库事实与复用入口

| 证据入口 | 本次核对的事实或后续接线关注点 |
| --- | --- |
| [硬约束](../00-context/硬约束.md) | 现行普通图片仍是请求内响应。本方案新增异步与受保护输入保存，对应局部修订草案见 §4.2；原生最小入侵、资金与安全要求不因此撤销。 |
| [图片服务与中转 Provider 适配架构](../20-architecture/图片服务与异步Provider适配架构.md) | 现有图片中转有自己的协议、单 URL 结果与严格参数子集，不能直接作为 Google 的北向合同。 |
| [渠道类型](../../constant/channel.go) | Gemini 为 24，Vertex 为 41，图片中转 `ChannelTypeAsyncImage` 为 63。渠道实例 ID 与类型编号不可混淆。 |
| [原生路由](../../router/relay-router.go) | generations、edits 已有入口；variations 和 Files 相关路由仍指向未实现处理。 |
| [图片 DTO](../../relaykit/dto/openai_image.go) | 共享安全上限 `MaxImageN` 为 128；未知字段收集与重新序列化需核对，不能假设 DTO 已完整支持拟发布合同。 |
| [图片处理链路](../../relay/image_handler.go) | 请求复制、模型映射、转换、usage 与结算需要贯通核对；不能让新字段在复制或重编码时丢失。 |
| [公开图片投影](../../pkg/publicmodel/image.go)、[渠道投影](../../model/public_image_model_api.go) | 新方案需要准确表达两个操作及其参数，不能直接复用现有仅生成的投影结果。 |
| [计费表达式说明](../../pkg/billingexpr/expr.md) | 实施计费接线前必须完整阅读；客户价格继续由管理员配置。 |
| [任务查询路由](../../router/task-router.go)、[查询实现](../../controller/task.go) | 已有 GET /v1/tasks/{task_id}，当前主要返回任务状态；图片结果、图片合同投影及 user_id + app_id 隔离需补齐，不能宣称现状已满足。 |
| [异步任务架构](../20-architecture/账单计费-异步任务与计费事实架构.md)、[创建恢复](../../model/task_create_attempt_persistence.go) | 现有共享创建流程要求真实 Provider task ID，不能原样承载平台先受理、同步上游无 task ID 的图片任务。 |
| [attempt 恢复](../../service/task_create_attempt_reconcile.go)、[任务轮询](../../service/task_polling.go) | 既有流程会关闭过期 prepared、处理超时及退款。新图片排队、待核实任务必须按显式类型隔离，不能被旧扫描规则误处理。 |
| [OSS 配置](../../setting/system_setting/task_artifact_store.go)、[存储接口](../../service/task_artifact_store.go) | 只有配置与接口预留，实际存储实现禁用；配置 s3 会退回 upstream，并非开启配置即可使用。 |
| [幂等中间件](../../middleware/task_create_idempotency.go)、[摘要](../../middleware/task_create_idempotency_digest.go)、[幂等模型](../../model/task_create_idempotency.go)、[视频接线](../../router/video-router.go) | 已有五态模型和重放机制；唯一键为 user_id + protocol + key_hash，没有独立 app/操作列。JSON 规范化、非 JSON 按原始正文摘要；现有重放不包含图片 202。图片复用与差异见 §3.7。 |
| [产物访问签名](../../service/task_artifact_access.go)、[访问鉴权](../../middleware/task_artifact_access.go)、[产物控制器](../../controller/task.go) | 已有 artifacts 列表和 GET/HEAD 内容路由；HMAC capability 绑定任务/产物但不含到期时间。它是现有投递能力，不是 5 分钟 OSS 签名，图片选择见 §5。 |
| [渠道测试](../../controller/channel-test.go)、[任务轮询](../../service/task_polling.go) | 管理测试须补本期图片能力；请求限流、下载并发保护或现有轮询调度均不能代替新增图片任务的跨节点受理容量与背压。 |

上述入口区分既有机制与新增缺口；不得把“已有配置/表/路由”当成新图片合同已可用。

### 2.2 渠道与管理员操作基线

以下是会话确认的目标，不是本轮数据库现状证明。后续执行前须只读复核实际记录与影响，保留无关配置。

| 渠道实例 | 已接受的目标 |
| --- | --- |
| 99 | 保留 Gemini 类型；使用独立客户模型 `nano-banana-2-gemini`，精确映射到 `gemini-3.1-flash-image`。会话拟定替换原 `nano-banana-2` 入口，实际变更前复核影响。 |
| 103 | 保留 Vertex 类型；添加独立客户模型 `nano-banana-2-vertex`，精确映射到 `gemini-3.1-flash-image`；保留其他模型和配置。 |
| 93 | 保留图片中转类型、南向协议和管理员价格；补齐统一北向合同与可选异步。不更换成 Google 类型，也不再把旧的受限参数合同视为最终目标。 |
| 88 | 不单独修改其渠道配置；如受共享图片能力变更影响，纳入回归，不据此自动修改模型或映射。 |

映射属于用户授权的管理员操作，不是默认代码、升级迁移、启动补全、自动发现或价格继承。
不处理无图片能力的聊天模型，也不把 Imagen、所有 gemini-* 或仅完成配置的模型列为已验证能力。

### 2.3 外部证据及其限度

以下保留讨论时的官方资料与结论，不证明实际 99/103 已通过；实施前须复核模型、API 版本、区域和中转端点。

| 来源 | 与本方案相关的结论 |
| --- | --- |
| [OpenAI 生成接口](https://developers.openai.com/api/reference/resources/images/methods/generate) | 参数及取值存在模型适用差异；北向应以标准字段语义为基准，不能把 Google 分辨率档位称为标准像素尺寸。 |
| [OpenAI 编辑接口](https://developers.openai.com/api/reference/resources/images/methods/edit) | 编辑需要核对 multipart、JSON 图片引用、mask、file_id 等，不只有 prompt 与参考图。 |
| [Gemini 图片生成](https://ai.google.dev/gemini-api/docs/generate-content/image-generation) | 响应中需要区分思考图片和最终图片，不能简单取第一个图片 part。 |
| [Gemini 请求与响应定义](https://ai.google.dev/api/generate-content) | Google 的图像尺寸配置、结束原因和安全反馈需要由南向转换吸收；candidate 数量不能直接等同输出图片数。 |
| [Gemini 文件输入方式](https://ai.google.dev/gemini-api/docs/generate-content/file-input-methods) | 存在外部 URL 输入方式，但仍需核对模型与实际端点支持；Provider 文件句柄不等于平台 file_id。 |
| [Vertex 推理请求](https://docs.cloud.google.com/gemini-enterprise-agent-platform/reference/models/inference) | 允许公开 HTTP URL；通用说明列出最多 10 张 URL 图片及 VPC Service Controls 限制。 |
| [Vertex Gemini 3.1 Flash Image](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/gemini/3-1-flash-image) | 模型页列出最多 14 张输入图、inline 单张 7 MB、Cloud Storage 单张 30 MB；这些条件不能混为同一输入方式的限制。 |

Vertex “URL 必须下载”的旧判断撤回，优先验证直传。14 张 URL、20 MB 二进制仍需 G2/G3 取证；
启用 OSS 不等于取得 Google Cloud Storage 或 Provider 文件接口能力。

## 3. 优化方案

C/P/E/U/R/V 编号保留为合同规则引用；重复的范围、恢复与存储约束分别集中到 §1.1、§3.8、§5。

### 3.1 合同原则（C1—C5）

| 编号 | 已接受的设计 |
| --- | --- |
| C1 | 全部标准字段按操作及模型适用条件履约，不把不同模型专属字段强行拼成共同必选功能。 |
| C2 | adapter 吸收字段、路径、认证和结构差异；实现未完成不等于模型不支持，能力限制或不可履约须有证据。 |
| C3 | 仅标准或已发布合同明确规定的条件生效字段可在条件不满足时不生效；不能任意忽略 mask、透明背景等结果要求。 |
| C4 | 本期创建操作为 generations、edits，并复用任务查询提供显式异步扩展；不自动发布 variations、Responses 图片工具或完整 Files 管理 API。file_id 的必要解析与授权依赖仍须解决。 |
| C5 | 复用原生路由、Task、幂等、计费与存储底座，不建第二套公共事实；具体最小接线遵循 §4.3。 |

具体字段必须生成按操作区分的合同清单；字段类型、必填性、默认值、枚举、上下限及条件生效规则要
有唯一实现来源，并用于校验、公开元数据和合同测试。不得另建管理员可编写的协议 JSON 或逐模型平行能力系统。

### 3.2 标准参数（P1—P14）

| 编号 | 参数 | 已接受的处理方向与完成条件 |
| --- | --- | --- |
| P1 | model | 使用客户模型，按原生管理员映射解析最终 Provider 模型；不按名称片段猜测能力。 |
| P2 | prompt | 保留客户原意，不自动改写业务请求；内部履约说明不得替代参数实现。长度和计数方式在合同冻结时明确，不把某个 Provider 的限制无差别施加给原生入口。 |
| P3 | n | 标准合同范围为整数 1—10，未传为 1；Google 图片仅允许 1，其他值在 Provider 调用前报错。不循环调用、钳制或把显式 0 当默认值。共享 MaxImageN 安全上限不因此全局改为 10。 |
| P4 | size | 以标准 auto、宽x高语义为准，南向转换到 Google 分辨率档位与宽高比；之前直接用 512/1K/2K/4K 充当标准 size 的方案被取代。 |
| P5 | 尺寸后处理 | 允许必要的等比例缩放；不得擅自裁切、拉伸、补边或返回另一尺寸。可发布的像素尺寸、auto 行为及转换表须验证；无法保持要求的规格不得虚假发布。 |
| P6 | aspect_ratio 扩展 | 不作为标准请求必填字段。优先由 size 表达尺寸；如需额外暴露原生比例，应另行明确可选扩展与冲突规则，不能出现两个尺寸权威。本次不默认新增扩展。 |
| P7 | quality | 标准适用值须有可靠映射，不把 high 等同 4K/更多 thinking；缺证据则阻断，不静默降级。 |
| P8 | output_format / output_compression | 支持标准适用格式及压缩语义；上游无法直出时允许网关转换。明确 PNG/JPEG/WebP、格式对应 MIME、透明度与压缩适用条件；保留合法压缩值 0，验证范围及无效组合。 |
| P9 | background | transparent 必须有真实透明通道，opaque 验证或处理透明度；允许必要处理，但提示词不能代替透明结果验证。 |
| P10 | moderation | 审核档位须验证映射或说明能力限制，不绕过 Provider 安全策略、不忽略字段掩盖实现缺口。 |
| P11 | style | 按标准模型适用条件处理，不强行把 DALL·E 专用字段变成 Google 风格提示词。 |
| P12 | user | 仅作调用方辅助标识，不替代 user_id、app_id 或真实授权，不凭空映射为 Google 账号。 |
| P13 | response_format | 异步未指定时默认 url，显式 b64_json 按数组返回；同步遵守已发布默认行为，显式格式按统一合同履约，不强行全局更改原生默认值。Data URL 不得冒充托管 URL。 |
| P14 | stream / partial_images | 显式 false、0 正常解析；同步流式按标准适用语义履约，不拿 thought 图片冒充中间成品。Prefer: respond-async 与 stream=true 互斥，在受理、预扣、调用上游前报参数冲突；异步允许 stream 未传或 false。partial_images 的条件生效按标准冻结，不能将其当 n 或异步任务开关。 |

### 3.3 参考图与编辑（E1—E6）

| 编号 | 已接受的设计 |
| --- | --- |
| E1 | 保留 multipart 文件、JSON Base64 Data URL、JSON HTTPS URL。multipart 支持标准单图 image 与多图 image[]；JSON 使用标准 images 结构，不要求 extra_fields.reference_images。 |
| E2 | mask 须真实编辑指定区域，验证遮罩方向、边缘及非编辑区域保持；允许经验证的合成处理，不能只转提示词或整图编辑。 |
| E3 | input_fidelity 的保真语义需实际验证，不能未经论证等同于输入解析分辨率。 |
| E4 | file_id 的解析、授权和生命周期是完成依赖；不把 Provider 句柄当平台文件 ID，不暗中扩建完整 Files 管理面。 |
| E5 | 以下为目标值，履约证据见 G2/G3：最多 14 张输入图片；二进制单张 20 MB、合计 50 MB，按解码后字节数而非 Base64 字符串长度计算。URL 计数量、不计未知远程字节。单位应在合同和测试中用明确字节常量固化，不混用 MB/MiB；具体常量在技术合同冻结时记录。 |
| E6 | 重复标量、互相冲突的单图/多图字段、错误类型明确报错，不任意取首值。未传、null、显式 false/0 分别按标准定义处理，DTO 复制与编码不得丢字段。 |

mask 是否占用张数或字节预算、混合 URL/二进制计数、单图/多图字段合法组合、MIME 与解码校验、
文件读取后释放时机，须在实施前的技术合同表中补齐；不能用未公开的隐含限制改变客户预期。
这些是执行细节待收敛，不是用户尚未批准业务方向。

### 3.4 URL 与大小履约（U1—U5）

| 编号 | 已接受的设计 |
| --- | --- |
| U1 | Gemini、Vertex 均优先直接透传 HTTPS URL，不为检查资源大小执行 HEAD/GET，不修改完整签名 URL；各实际渠道分别验证。 |
| U2 | Vertex 14 张 URL 目标与通用 10 张限制的冲突见 §2.3/G2；禁止偷偷下载绕过。 |
| U3 | 20 MB 目标与 Vertex inline 7 MB 的冲突由传输适配及 G3 证据解决；不能私自收紧为 7 MB 或先发布再让上游失败。 |
| U4 | 南向要求 MIME 时必须有可靠获取方式，不能依赖 URL 后缀或猜成 JPEG。若必要探测，应限于类型履约，落实 SSRF、重定向、超时和取消，不能演变为大小检查。具体无探测/探测路径需技术验证。 |
| U5 | 输入与输出存储分别履约。本期允许异步上传文件/Base64 保存到私有 OSS，并保存必要受保护执行资料；不自动下载直传 URL 图片。Vertex 特有的 Provider 文件传输仍需验证，不因网关启用 OSS 就视为已解决 7 MB 限制。 |

对已上传字节的资源限额，不等于对远程 URL 检查大小。客户端请求体、解码和图像后处理仍需有与已发布
额度相容的内存、读取和取消边界；不得引入一个更小、未公开的整体 body 限额使合法 Base64 请求失败。

### 3.5 响应、生命周期与计费（R1—R9）

| 编号 | 已接受的设计 |
| --- | --- |
| R1 | 仅返回有效最终图片，排除 thought；综合安全反馈、结束原因和内容有效性判断。零图、安全拒绝及无法证明完整的输出不能当成功空数组。 |
| R2 | 一个请求/平台任务可交付多图，不能全局只取第一张；最终数量与 n、模型限制一起校验。异常超量（含 Google n=1 却返回多张）的具体规则需技术合同冻结，不静默截断、追加收费或重发；完整可信 usage 保留。 |
| R3 | 图片结果按标准 created、data[] 及适用可选字段组织，单图只是长度为 1 的情况；不泄漏渠道、Provider 身份、内部认证或思考内容，不捏造 revised_prompt。异步状态封装是平台扩展，不冒充同步 Images 原生字段。 |
| R4 | 使用可信 usage 并正确归一化；图片数量不能伪造成 token。缺失 usage 按既有计费合同处置，动态计费缺少可信依据时不能编造用量后结算。 |
| R5 | 标准错误结构、稳定错误分类及可定位参数错误；无图、安全拒绝、认证、超时分别处理，上游错误脱敏，不原样转发 Provider 响应。 |
| R6 | 同步/流式保持现有请求生命周期兼容；异步受理后独立执行，客户断开不取消。两种模式均需完整执行时限，不能对子步骤重置预算或把超时直接当 Provider 未受理。异步队列、执行、存储与未知状态分别按事实处理。 |
| R7 | Provider 可能已收到生成请求后禁止自动重发、跨渠道重选、拆分调用或 fallback；认证准备与业务 POST 分开，不借刷新认证自动重复生成。 |
| R8 | 复用管理员选择的固定价、token 或表达式计费及既有退款、违规收费策略。模型映射不继承或生成价格；不改价格数据。客户退款与 Provider exposure 分开，未知供应商成本保持未知。 |
| R9 | 相同业务参数通过 JSON/multipart 得到一致逻辑计费输入；n、尺寸等不得因编码方式丢失。请求上界、预扣、checked quota 转换、成功结算、失败退款需贯通验证。 |

时间上限、取消和超时错误在最小接线设计中明确。异步不复用同步请求结束时的自动退款动作，
也不能直接进入既有通用超时扫描后被错误释放资金。新增图片类型的受理、执行及补偿边界见下文；
不会因支持后台执行而获得重复生成权限。

### 3.6 模型、认证、安全与发布（V1—V7）

| 编号 | 已接受的设计 |
| --- | --- |
| V1 | 渠道范围唯一遵循 §1.1，不新增全局模型拦截。 |
| V2 | 共用 Google 图片转换核心，复用 Gemini/Vertex 各自认证、项目、区域、连接与代理；不要求客户填写南向必填字段。 |
| V3 | 模型列表、详情与价格目录准确表达生成/编辑和异步合同；同一客户模型的实际候选须兑现一致能力，不展示未实现项目。 |
| V4 | 媒体正文、完整签名 URL、凭据、原始响应不进入日志、公共响应或普通任务字段；异步必要资料按 §3.8 私有保存，访问客户 URL 不携带 Provider 凭据。 |
| V5 | 99/103/93 及图片中转协议分别验证；管理面渠道测试覆盖 Gemini/Vertex 新图片操作和异步查询，不仅回归其他渠道。真实调用与费用另行取得执行授权。 |
| V6 | 93 管理员配置按 §2.2 保留；视频、插件及无关原生请求的合同保持不变。 |
| V7 | G0—G12 通过后再按治理流程收敛事实与发布文档，不以设计接受代替上线验收。 |

### 3.7 异步提交、幂等与查询

| 项目 | 合同与复用方式 |
| --- | --- |
| 创建与受理 | 两个既有创建路径保持同一请求合同；默认同步，以 `Prefer: respond-async` 显式异步。满足 §3.8 后返回 202、平台任务 ID 和查询地址；流式冲突遵循 P14。 |
| 查询 | 复用 `GET /v1/tasks/{task_id}`，一次返回状态与可用结果。仅图片类型使用图片投影，按 user_id + app_id 授权；其他任务响应不变。 |
| 幂等范围 | 可选 Idempotency-Key；同用户、应用、操作内，同 key 等价请求重放原受理结果，不同请求返回冲突；无 key 表示新意图。202 丢失或原任务待核实仍返回原 ID。 |
| 唯一事实 | 复用 TaskCreateIdempotency 表和五态定义，不建图片幂等表、不把任务状态复制进去。图片专属受理/重放方法放新增文件，不原样套用视频中间件与 Provider-ID 恢复流程。 |
| 隔离接线 | 优先新增固定图片 protocol 命名空间，将 app_id、操作和客户 key 用无歧义编码后摘要进 key_hash，复用既有 user_id + protocol + key_hash 唯一键；任务读取仍检查双重归属，不以摘要代替授权。不为图片改写视频 key 或全局唯一索引。 |
| 请求等价 | 对标准化参数、图片顺序、实际上传字节生成逻辑摘要；忽略 multipart boundary、临时对象名等传输噪声，保留有语义的显式值。URL 按提交值参与，不下载资源求 hash；具体等价规则由 G1 冻结。 |
| 状态含义 | 图片受理事务内绑定 Task 并记 complete，表示“创建受理已完成”，不是生成完成；后续生成/待核实以 Task 为准。creating 表示受理未完成，不伪造 202；其恢复不得获得重复发送权。upstream_succeeded 等原状态保留既有用途，不为同步 Google 伪造 ID。 |
| 保留与恢复 | 既有 complete/completed_no_replay 到期可重置规则不能原样用于图片：排队、执行、待核实及未完成交付/结算期间必须保留绑定；终态后的期限由 G1 冻结。受理失败的 claim 释放须证明无已提交 Task/资金事实。 |

新增方法须复用既有原子约束并接受事务上下文，使幂等绑定、受理与资金一次提交；不能先重复扣款再退款。
幂等重放先于新容量占用和重新选渠；创建尚在并发处理时返回明确进行中结果，不签发第二个任务。
既有五态函数中的超时、恢复和重放假设须逐项隔离；若同表最小扩展无法满足，先报告证据，不直接新建第二套机制。
幂等、任务及输入保留期限独立于 §5 的 5 分钟下载链接。

### 3.8 持久化受理与恢复

数据库驱动后台执行，不新增消息队列；复用 Task，新增显式图片执行类型。对客户返回平台任务 ID，
真实 Provider task ID 独立保存且允许尚无；该边界须先通过 §4.2 草案审定。

1. 校验合同、权限与幂等，初筛容量；选择渠道并冻结客户/Provider 模型、连接、adapter 与管理员计费事实。
2. 将必要文件/Base64 输入保存私有 OSS；数据库存对象引用。直传 URL 不搬运媒体，但恢复所需 URL 和参数
   必须受保护持久化，不填入普通 Task 数据。上传阶段并发/请求体资源也须受限，不能无界暂存。
3. 同一数据库事务原子检查并占用容量、写入可执行 Task、预扣和幂等绑定；输入可用且提交成功才返回 202。
   容量初筛不能代替最终原子校验；事务失败不受理、不扣款，已暂存对象按部署方 OSS 生命周期处置。
4. 后台原子取得执行容量及唯一发送许可，发送前持久确认预扣与 sending；沿用同一 hold，不再次预扣。
   适用的异步 Provider POST 在发送字节前建立 durable attempt，按图片执行类型关联平台 Task。
5. 同步上游由后台等待，异步上游保存真实 ID 后查询；冻结事实驱动恢复，不按当前配置重选渠道。
6. 校验结果、写 OSS 并可靠登记引用后才宣布可交付完成；结算单独幂等推进，不因结算写入失败抹去生成事实。

| 可恢复事实 | 允许动作 |
| --- | --- |
| 已证实尚未获得发送许可 | 输入有效且未超过排队期限时继续；已过期则明确未执行失败并按政策释放 hold。 |
| 已持久化真实上游 task ID | 恢复查询，不重新创建。 |
| 请求可能已发送，无可恢复结果 | 待核实；禁止自动重发、换渠道或退款，发送前后崩溃窗口从严处理。 |
| OSS 已保存，数据库登记未完成 | 按确定且可验证的对象关联补登记；此恢复能力须测试，不因缺记录重新生成。 |
| 上游成功，OSS 暂时失败 | 只重试读取/保存已有可恢复结果；仅在内存且随崩溃丢失时记录交付异常，不承诺恢复。 |

租约到期只允许释放工作进程资源，不授予第二次生成 POST 权限。单次查询异常不能判业务失败；
明确失败按政策释放，成功按冻结规则结算，未知保持待核实。发送安全唯一规则见 R7。
旧 prepared 清理、通用超时退款与请求结束退款必须排除不适用的新图片阶段，既有视频规则不变。
排队、执行及存储分别有时间预算；总预算不得被子步骤重置。输入 OSS 删除或客户 URL 过期须如实报错，
不能换图、换 URL、重新生成或擅自改变 OSS 清理策略。客户端在 202 后断开不取消任务。

### 3.9 多图与部分成功

一个任务对应一次生成/编辑，不拆调用凑 n。按 R1—R3 返回全部有效图片，每张独立保存并签发结果。
已知上游结束时区分全部成功、全部失败和部分成功；部分成功返回有效图片，不自动补生成。
剩余结果尚不可信时保持待核实，可展示已知结果，但不能提前判最终部分成功。
生成成功而部分 OSS 保存失败是交付问题，不冒充部分生成失败；逐图可用性和历史生成结论分开。
同步响应也为 data[]，任务状态只在异步封装表达；完整 usage 按 R4/R8 计费，不擅自按成功张数折价。
状态枚举、逐图错误及异常超量规则由 G1 冻结。

### 3.10 容量与背压

只约束本期显式异步图片，复用数据库任务事实，不把 HTTP 限流、下载并发或 Redis 计数当任务容量权威。

| 层次 | 最小策略 |
| --- | --- |
| 受理上限 | 部署方配置全局等待任务上限、每 user_id + app_id 未完成任务上限；后者覆盖等待、执行、待核实及未完成交付/结算，防止持续累积。已重放请求不重复占位。 |
| 执行上限 | 配置全局及每 Channel 并发；多节点原子抢占、完成后释放，不能每节点独立计算而突破总额。未确认的 Provider 工作不得因本地租约过期就当成已结束。 |
| 背压响应 | 应用额度耗尽返回 429，全局排队容量耗尽返回 503，附 Retry-After 提示而不保证届时有空位；均不返回 202、不预扣、不发送 Provider 请求。 |
| 一致性 | 受理容量与 §3.8 事务一致，禁止无锁 count 后 insert；执行容量也需原子约束。若需容量协调记录，仅保存可由 Task/attempt 重建的占用，不另建公共任务或账本。 |
| 时间边界 | 配置最大排队等待、执行及存储预算；仅证实未发送的排队过期任务可安全释放。待核实不因队列/租约超时自动退款或解除幂等绑定。 |

具体正数容量、默认值、时间预算及公平调度方式由 G12 经容量测试冻结，不在方案中编造通用数值，
也不作为价格配置。受理前上传资源须有界且容纳 E5 合法输入；存储不可用时拒绝新异步受理，已受理按事实恢复。

## 4. 待验证门槛与实施顺序

### 4.1 发布阻断项

业务方向已接受，以下证据与机制仍须完成；不得改成“不支持”后关闭。

| 门槛 | 必须取得的结果 |
| --- | --- |
| G0 边界与最小接线 | §4.2 草案经审定并先收敛到权威规范；§4.3 每个候选接线点明确必要性、最小改动和上游同步影响，没有全局重构或第二套公共事实。 |
| G1 标准合同冻结 | 按两种操作逐字段列出类型、默认值、取值、条件、错误和同步/SSE/异步响应；补齐单位、mask 预算、混合输入、状态枚举、多图与异常超量规则，明确标准资料版本。 |
| G2 两类认证及 URL | 使用实际 99/103 的连接验证 URL 直传、MIME、模型、项目/区域条件；验证 14 张 URL，不能把文档一般说明当实测。 |
| G3 大文件履约 | 验证单张 20 MB、合计 50 MB 及 Base64 编码开销；解决 Vertex inline 限制，必要暂存方案单独说明权限与生命周期。 |
| G4 参数真实效果 | size、quality、格式压缩、透明背景、moderation、mask、input_fidelity 和流式按标准适用语义验收；禁止用字段存在性代替实际效果。 |
| G5 文件引用 | 明确 file_id 的合法来源、解析路径、授权隔离、生命周期和不可用错误；依赖未完成不能宣称编辑参数完整支持。 |
| G6 结果与财务 | 最终图过滤、多图/部分成功、无图/截断/安全、真实 usage、编码方式计费一致性及取消/超时 exposure 均有证据；预扣、入队与幂等原子化，结算/退款幂等，不硬编码价格。 |
| G7 回归与公开投影 | Gemini/Vertex 图片接口新增及图片中转完整改造分别验证；管理面渠道测试覆盖 Gemini/Vertex 生成、编辑与异步查询，93 和图片中转各协议覆盖新合同；其他原生渠道/视频/插件任务不变；两个操作与异步查询的元数据、鉴权、运行时一致。 |
| G8 持久执行 | 崩溃点覆盖受理前后、发送许可前后、上游 task ID 写入前后、OSS 保存与数据库登记之间、结算失败；并发抢占不能重复生成，未知任务不能被旧清理/退款规则处理。 |
| G9 对象存储 | 真正实现存储后端；验证私有输入、结果对象关联、多图、5 分钟签名与重新查询续签、已删除对象和暂时访问失败的区分；旧 capability 不得取得新图片内容或充当无限续签入口；既有产物投递保持不变，不以配置项存在代替可用。 |
| G10 幂等与查询 | 相同 key 并发/202 丢失/待核实重提均返回同一任务且仅预扣一次；不同请求冲突；user_id + app_id 隔离；复用既有表及状态、图片 202 重放、跨编码摘要和非终态保留规则有测试，既有视频幂等行为不变；查询与链接可用性准确。 |
| G11 中转站业务闭环 | B1—B10 逐项验证，包含客户提交/查询/续签、多图部分成功、管理员配置、协议适配、故障恢复及资金审计；不能只用单次文生图成功作为完整改造验收。 |
| G12 容量与背压 | 冻结 §3.10 的正数容量、时间预算与 Retry-After；至少两节点并发验证不超卖、不重复扣费，满队列幂等重放不占位，用户隔离、执行并发、未发送过期释放与未知任务不误清理均成立。 |

### 4.2 硬约束局部修订草案（实施前审定）

下列文字是待审草案，尚未修改[硬约束](../00-context/硬约束.md)或正式架构；实施前先审定并更新唯一权威源，
本方案随后只保留引用。不改变 Seedance 素材、既有视频或其他原生入口合同。

| 拟修订位置 | 草案 |
| --- | --- |
| 硬约束 §4 普通图片 | 普通图片默认保持同步；仅本期支持的显式图片执行类型允许通过 Prefer: respond-async 创建持久任务，必要输入、受理容量、Task、资金 hold 与幂等绑定可靠提交后才返回 202。图片中转继续由管理员选择代码登记的协议，不以模型、价格或 URL 推断。 |
| 硬约束 §4 Task 与 attempt | 平台图片 Task 表示已持久接受的执行意图，允许先于 Provider ID 创建；不得伪造上游 ID。其他要求真实 Provider ID 的 Task 创建流程不变。适用的异步 Provider POST 仍需发送前 durable attempt；受理已有 hold 在 sending 事务中确认/关联，不重复预扣。取得可信上游 ID 后，其登记与 attempt 完成原子提交。 |
| 硬约束 §4 恢复扫描 | 新图片执行类型按阶段持有发送许可与资金，旧清理器不得以 prepared、请求结束或租约到期直接判其可退款/可重发；未知结果继续遵循原禁止自动重发、换渠和退款规则。 |
| 图片架构的输入保存边界 | 仅已启用的异步图片恢复需要时允许私有 OSS 输入与受保护参数/URL 持久化；普通字段、日志和公共响应仍禁存原始输入。此许可不修改硬约束 §5 的无状态素材代理及 source URL 禁存规则。 |

### 4.3 NEWAPI 原生代码最小接线

通用准则以[硬约束 §7](../00-context/硬约束.md)及[上游代码合并指南](../30-engineering/上游代码合并指南.md)为权威。
本期优先新增图片专属文件；复用事实和稳定能力，不为抽象复用重写原生代码，允许少量局部重复降低合并冲突。

| 接线面 | 拟采用的最小方式 | 禁止扩大的影响 |
| --- | --- | --- |
| 既有图片入口与 DTO | 保留现有路由与标准类型，新增图片解析/执行文件；仅在必要位置按明确渠道类型、操作及执行方式窄分派，并在原请求预扣前确定生命周期。 | 不复制一套北向路由/标准字段权威，不全局拦截模型；不改共享 MaxImageN=128，relaykit 不依赖根模块。 |
| Gemini/Vertex adapter | 图片转换入口委托新增转换核心，各自调用既有认证和连接能力；仅确有接口缺口时增加窄接线。 | 不重写认证、聊天转换、模型发现或把 Google 迁到类型 63；不让凭据刷新重发业务 POST。 |
| 图片中转协议 | 在图片专属实现中统一生成/编辑、执行及结果，各已登记南向 adapter 履约。 | “完整改造”限于图片中转链路，不扩写无关原生通用链路或要求管理员编写字段脚本。 |
| Task、幂等与计费 | 新文件实现图片受理事务、阶段恢复及 §3.7 的同表接入；确需新增共享字段/事务入口时只加必要接线。 | 不改视频幂等 key/五态含义，不另建公共状态或账本，不重构计费引擎；不能以复用为由绕过原子性。 |
| 查询、产物访问与存储 | 既有控制器仅对明确图片类型委托专属投影/授权，OSS 实现与短期签发放新增文件；所有现有产物入口核对隔离。 | 不全局更换旧 capability 或把当前禁用的存储后端直接切换所有消费者；视频/插件的投递语义不变。 |
| 后台启动与扫描 | 新增图片 worker；注册启动及旧扫描排除只留必要窄分支，由显式执行类型归属。 | 不重写全部轮询器，不让旧退款规则处理新图片阶段，不引入通用工作流框架。 |
| 公开投影与管理测试 | 在操作级投影和渠道测试入口按类型窄接入新增文件中的图片逻辑，覆盖两个操作及异步查询。 | 不因客户模型别名猜测测试端点，不更改其他原生渠道默认行为或管理员数据。 |

以上是候选接线面，不是整文件改造授权。G0 必须逐项列实际文件、为什么不能零修改、最小调用/分支、
未来上游同步冲突及验证用例；无法安全保持窄接线时先说明阻断与必要扩展，不直接顺带重构。

### 4.4 实施顺序

1. 先审定 §4.2 草案及 G0 接线清单，再冻结 G1 合同和 B1—B10 验收；权威规范未更新前不编码。
2. 取得 G2—G5 的 Provider/标准语义证据；落实 §5 存储及 G12 容量配置，再实现持久受理与同表幂等。
3. 按 §4.3 接入 Gemini/Vertex 与图片中转协议，贯通创建、执行、保存、查询续签、多图及幂等结算。
4. 执行最窄合同/故障测试和必要回归；数据库涉及 SQLite/MySQL/PostgreSQL，relaykit 变动额外独立构建。
5. 获真实调用授权后核验实际渠道、账单与灰度；管理员映射按 §2.2 单独复核操作。通过所有门槛后收敛事实和发布文档。

## 5. OSS 本期范围与结果访问

OSS 是本期异步依赖，不全局强制普通同步请求使用；同步显式托管结果按能力履约，返回格式遵循 P13。

| 项目 | 唯一规则 |
| --- | --- |
| 保存 | 异步文件输入与最终图片存私有 OSS；数据库保存对象标识、归属及必要 MIME，不持久保存临时下载链接。Task/资金仍以数据库为权威。 |
| 投递选择 | 图片授权查询后直接签发 OSS 原生 presigned URL，逐张有效 300 秒并给出到期时间；到期后再次授权查询签发新链接。客户端直连 OSS，不新增自定义签名协议或平台正文代理。 |
| 既有机制关系 | 复用任务/产物身份与适用存储接口，新增图片签发能力；现有 task_artifact_access 长期 capability 保留原消费者，不作为图片 URL 或刷新凭证。不能先给长期平台链接，再无限换取短期 OSS URL。 |
| 访问隔离 | 新图片结果只经 user_id + app_id 授权签发；已有 artifacts 列表及 GET/HEAD content 路径不得旁路签发长期图片 capability、读取新图片内容或重定向续签。图片类型按明确分派处理，其他消费者不变。 |
| 多图与 Base64 | 每张独立链接及到期时间；显式 b64_json 逐张读取并返回。存储格式不限制客户格式，Data URL 不冒充托管 URL。 |
| 清理 | 部署方管理 OSS 生命周期，平台不新增图片清理任务、保留期删除策略或自动修改桶策略；这不承诺永久保存。 |
| 已删除 | 确认对象不存在后保留历史生成状态，逐图表达不可用；不抹去其他图片，不重新生成或自动退款。 |
| 暂不可用 | 超时、权限失败不能判为删除；返回访问问题，不伪装已可交付。仅重试已有结果的读取/保存，恢复约束见 §3.8。 |
| 安全与费用 | 私有桶、对象命名及响应避免泄露内部配置；完整签名 URL 不进入日志。价格遵循 R8，不在代码写固定存储价格。 |

接口已预留不等于能直接签名：当前 TaskArtifactStore 没有 presign 方法，须以新增实现/最窄扩展提供，
不得为接入图片改写全部消费者。300 秒签名需验证对象存储实际支持及凭据剩余有效期，不能虚报到期时间。
5 分钟不是对象、输入、任务或幂等保留期限；部署方须保障输入在执行期间可用。
直连 OSS 减少 Base64 膨胀与查询正文流量，但生成接收、上传 OSS 及显式 Base64 的传输成本仍存在。

## 6. 本次文档验证

- 方案轮仅更新本文；实施轮的代码与文档验证见 §7.5。
- 已通过 `task docs:check`、`task ai:check` 与 `git diff --check`。
- 实施轮已修改公开 API 文档，`bun run docs:validate` 已按规范执行并通过。

## 7. 实施修复说明（2026-09-05 实施轮）

本节记录实施轮的实际落点与对方案的解读性决策；阶段拆解、G0 接线清单与 G1 合同冻结表在
[统一图片服务实施计划](../50-planning/2026-09-05-统一图片服务实施计划.md)，正式变更事实在
[变更记录](../50-planning/变更记录.md)，此处不复制第二套合同。

### 7.1 代码落点（按阶段）

| 阶段 | 新增/修改 | 内容 |
| --- | --- | --- |
| A 统一合同层 | `service/image_contract.go`（新） | Provider 无关的形状/预算/三态校验：n 1—10 且显式 0 拒绝、multipart `image`/`image[]` + JSON `images`（Data URL/HTTPS URL）+ 单图 `image` 三形态、14 张与 20/50 MiB 解码字节预算、mask 全族 400、`Prefer` 头解析。放 service 包供 relay、adaptor 与 worker 三方共用（避免 relay→channel→service 反向依赖） |
| B Google 同步核心 | `relay/channel/gemini/image_generate_content.go`（新）；`gemini/adaptor.go`、`vertex/adaptor.go` 各一窄分支 | imagine 登记模型经 `generateContent` 履约生成+编辑；size→宽高比+1K/2K/4K 档（长边映射）；二进制→`inlineData`、URL→`fileData` 透传；thought 图片过滤、`promptFeedback`/安全 finishReason 显式失败、usageMetadata 计费；未发布字段显式 400；默认 `b64_json`，显式 `url` 走 OSS 300 秒签名 |
| C OSS 底座 | `service/sigv4.go`、`service/task_artifact_store_s3.go`（新）；`task_artifact_store.go` 工厂、`system_setting/task_artifact_store.go` 移除 s3 强制回退 | 代码内 SigV4（header 签名 + query presign），零新 Go 依赖，AWS 官方测试向量逐字节验证；path-style 兼容 Azure Blob S3 网关/MinIO/R2；300 秒结果 URL、404 与暂不可用区分 |
| D 异步底座 | `relay/image_async.go`、`relay/image_task_executor.go`（新）；`service/image_task_worker.go`（新）；`model/task_image_lifecycle.go`、`model/image_task_slots.go`（新）；`middleware/image_create_idempotency.go`、`controller/image_task_query.go`、`controller/system_task_image_handler.go`（新） | 受理事务一次提交容量槽位（FOR UPDATE 计数行）+ Task(QUEUED) + 幂等绑定；worker 恢复扫描/领取(CAS+执行槽位)/SENDING 持久提交/冻结事实执行/终态结算；查询复用 `GET /v1/tasks/{task_id}` 图片投影（user+app 双重归属）；结算/退款复用 `AsyncBilling` 幂等状态机 |
| E 中转异步化 | `asyncimage/headless.go`、`moxingimage/headless.go`（新）；`asyncimage/adaptor.go` 多图交付 | FunCloud/Moxing headless 执行进 worker（异步模式）；同步结果按 `data[]` 交付全部合法 URL，零合法 URL 失败关闭 |
| F 投影与测试 | `pkg/publicmodel/image_gemini.go`（新）；`model/public_image_model_api.go`、`controller/channel_test_image_profile.go`、`controller/channel-test.go` | `api.image` 新增 gemini_image 族（按映射后 Provider 模型识别，发布 create+edit 操作）；渠道测试对 Gemini/Vertex imagine 模型默认图片 endpoint |
| G 文档 | 硬约束 §4、图片架构、投影架构、运维手册、用户指南、OpenAPI/公开文档/变更记录/路线图 | 见 §7.5 验证清单 |

原生文件改动严格限于 [实施计划 §1 G0 接线清单](../50-planning/2026-09-05-统一图片服务实施计划.md)
所列的最小接线（`image_handler.go` 顶部异步分派、`controller/relay.go` 3 行预扣跳过、
`router/relay-router.go` 2 行中间件、`model/task.go` 3 处 NULL 安全扫描谓词 + 1 个 PrivateData
字段、`main.go`/system task 注册各 1 行）。

### 7.2 对方案的解读性决策（已在 G0 声明）

1. **发送许可不建第二套 attempt 表**：图片 Task 在受理事务内先于 Provider 调用持久存在
   （§4.2 草案本身的创新），Task 自身 `QUEUED → IN_PROGRESS(SENDING 提交) → 终态` 状态机即
   承载 §3.8.4 要求的 durable 发送事实；恢复扫描发现停留在 SENDING 的任务置
   `RECONCILIATION_REQUIRED`，禁止自动重发/换渠道/退款。单一事实源，
   不与 TaskCreateAttempt 并存两套“可能已发送”状态机。
2. **资金顺序“先任务后扣款”**：quota 预扣（缓存优先的 TryReserve）与任务插入无法跨存储同事务。
   采用任务先入库、预扣紧随其后：预扣失败→任务安全失败（无款可退，恢复扫描释放容量与幂等）；
   预扣成功后崩溃→幂等重放收敛。任何崩溃窗口都不产生无依据扣款或重复扣款（§3.7）。
3. **63 类型 edits/参考图保持证据门控**：统一合同层与 worker 管道已可承载 edits，但按既有硬
   约束（输入图计费证据未核实前不得发布），FunCloud/Moxing 的 edits/参考图仍显式 400——这是
   B7/G 门控状态，不是合同缺失；证据完成后仅需放开 profile 声明。
4. **异步资金来源 v1 为钱包**：订阅资金来源在订阅侧证据完成后接入（G11 范围）。

### 7.3 验证证据

- 新增单测：SigV4 官方向量（header + presign 两个 AWS 示例）、统一合同层表驱动（合法/非法/
  边界/互斥/预算）、gemini 转换与响应（thought 过滤/安全拒绝/usage/size 映射/未发布字段）、
  受理-领取 CAS 与执行槽位、未完成受理/排队过期恢复查询、视频扫描排除（含 NULL 历史任务
  回归）、FunCloud 多图交付、投影合同形状。
- 全仓回归：`go test ./...` 通过；`cd relaykit && GOWORK=off go build ./...` 通过。
- 两处旧断言按已接受合同更新（非删除覆盖）：FunCloud“恰好单 URL”→ 多图交付 + 零合法 URL
  失败关闭；`TASK_ARTIFACT_STORE_MODE=s3` 强制回退 upstream → s3 激活（实现已落地）。

### 7.4 未完成项（转路线图）

G2—G5（真实 99/103 取证：URL 直传/MIME/区域、20/50 MiB 与 Vertex inline 限制、参数真实效果、
file_id）、G9（Azure S3 网关 presign 灰度）、G11（B1—B10 业务闭环）、G12（两节点容量/背压实测
与默认值冻结）。完成前异步模式与 gemini_image 族不得宣称生产可用。

### 7.6 评审修复轮（同日第二轮）

外部审计（audit-code-quality 口径）给出 4/10 评分与 S1—S12/N1 发现；逐条核对代码后确认
S1—S9、S11、S12、N1 全部成立，S10 部分成立（证据门控是方案内行为，但非法 size 静默回退与
完成度措辞失真是真实缺陷）。本节保留首轮处置记录；二次复审发现资金、恢复、容量与尺寸仍有缺陷，以下相关机制已由 §7.7 取代，不能作为当前实现依据。

| 编号 | 修复 | 回归测试 |
| --- | --- | --- |
| S1（P0 幻影退款） | 新增 `model.FailImageTaskUnfunded`：受理失败路径原子清零 `Quota`、置 `AsyncBilling=settled` 并同步刷新 `billing_state` 投影列；abandon 与未完成受理恢复扫描全部改用该入口 | `TestFailImageTaskUnfundedClearsBillingFacts`、`TestUnfundedImageTaskSurvivesBillingReconcileWithoutPhantomRefund`（守恒：未扣款→零净变动）、`TestFundedImageTaskFailureRefundsExactlyOnce`（有预扣→恰好一次退款） |
| S2（双预扣/结算偏离） | controller 跳过 `PrepareTieredBillingForSelectedGroup`（tiered 二次预扣入口）；成功结算按冻结规则分流（tiered 快照+实际用量重算 / token 倍率按可信 usage 重算 / 固定价保持预扣）；受理接入令牌额度预扣（`TryReserveTokenQuota`）与补偿；冻结 `TieredSnapshot/ContractFact/TokenId/BillingSource` | 结算分流逻辑随 worker 单测；令牌预扣路径在受理内联 |
| S3（异步绕过映射） | 新增 `mapImageAsyncRequest`（复用 `ModelMappedHelper`）在族判断与冻结之前执行；抽取为独立函数以便测试 | `TestMapImageAsyncRequestAppliesModelMapping`（同步/异步同别名→同 Provider 模型） |
| S4（Vertex 劫持 Chat） | Vertex 分支限定 `RelayModeImagesGenerations/Edits`（与 Gemini 侧一致），聊天补全回到 `GeminiChatHandler` | `TestVertexImagineModelKeepsChatShapeOutsideImagesRelayMode` |
| S5（OSS 劫持旧产物） | `Resolve` 改为只对有持久化登记事实的图片产物返回引用（`FindImageTaskArtifact`），插件产物返回 nil 保持原 Provider 下载路径 | `TestS3ResolveRequiresPersistedImageArtifact` |
| S6（恢复事实缺失） | FunCloud 取得 Provider ID 即回调持久化（worker 兜底消费返回值）；输入/结果对象键改确定性 `images/tasks/{taskID}/input-N|result-N`；每张上传成功即 `AppendImageTaskArtifact` 登记；新增恢复查询入口 `ResumeImageTaskPoll`（只查询不重建，worker 每轮对待核实任务续查） | `TestAppendAndFindImageTaskArtifact`、`TestGetReconcilableImageTasksRequiresProviderID` |
| S7（幂等生命周期） | claim 到期重置前检查绑定图片任务未终态则保留（`ImageTaskRequiresIdempotencyRetention`）；multipart 逻辑摘要（排序字段+文件内容 hash，忽略 boundary）；受理并发窗口内（FundsHeld=false 未终态）重放返回 `in_progress` 而非 202 | `TestImageIdempotencyRetentionWhileTaskActive` |
| S8（解析器漂移） | DTO 增 `NExplicitZero`（原生归一前记录显式 0）；multipart 分支补齐 response_format/background/output_format 等标准标量读取；合同层按三态语义裁决。测试从真实路由解析器进入 | `TestImageRouteParserPreservesExplicitNZeroForContract`、`TestImageRouteMultipartParserReadsStandardFields` |
| S9（容量重建覆盖并发） | `RebuildImageTaskSlots` 改为单事务：先按 scope 字典序锁定全部计数行（与受理/领取加锁顺序一致）再事务内统计并写入/归零 | `TestRebuildImageTaskSlotsZeroesStaleScopes` |
| S10（完成度/size） | 非法 size 显式 400（`IsValidGeminiImageSizeToken`），不再静默回退默认；完成度表述改为下方三分表，变更记录同步改写 | `TestGeminiImageContractRejectsInvalidSize` |
| S11（SSRF） | Provider 结果 URL 下载改用 `GetSSRFProtectedHTTPClient`（拨号时校验），不复用管理员上游客户端 | 随现有 SSRF 客户端测试边界 |
| S12（删除语义） | 查询逐图 HEAD：404→`deleted`（保留历史状态）、其它错误→`unavailable`、可用才签 300 秒 URL/b64；不再整单失败，不抹除其它图片 | 投影状态机代码路径（存储联调留 G9 灰度） |
| N1 | 删除无调用方 helper；publicmodel 测试包装内联 | — |

首轮修复未形成完整闭环；当前资金、参数快照、恢复与尺寸事实以 §7.7 及图片架构为准。

### 7.5 文档与检查

- 已更新：硬约束 §4、图片架构、公开投影架构、运维手册（§5 异步运维）、用户指南（§3/§7）、
  `docs/openapi/relay.json`（Prefer/Idempotency-Key/202/409 + `/v1/tasks/{task_id}`）、
  public-operations、公开文档（generations/edits 页 + 新增图片任务查询页，contentVersion
  2026-09-05.2）、变更记录、路线图、50-planning 实施计划。
- 已通过：`task docs:check`、`task ai:check`、`bun run docs:validate`（20 页 30 操作）、
  `git diff --check`。

### 7.7 二次复审修复：八项缺陷与最小接线

本轮仅修复二次评审中的八项缺陷，并增加真实请求复制路径的校验回归；不将尚未实施的编辑参数、
中转 edits、SSE、file_id 或真实 Provider/账单验证标成完成。当前架构以
[图片服务架构](../20-architecture/图片服务与异步Provider适配架构.md) 为准。

| 缺陷 | 当前修复及验收边界 |
| --- | --- |
| 受理扣款分离，崩溃丢失资金事实 | 容量、钱包、令牌、Task/FundsHeld、幂等绑定在一个数据库事务提交；删除事后资金标记、unfunded/abandon 补偿路径。故障测试覆盖令牌不足、Task 写入失败及幂等冲突，验证全部回滚。 |
| 实际 p/c 丢失，Token 价格不冻结 | 持久化完整 usage、价格倍率和已校验标量参数；表达式使用冻结快照及加密请求探针。不存在 usage 时保留 hold，显式零值不伪造 Token。 |
| 非原子重算后再原子结算 | 目标金额计算无资金副作用；唯一资金入口是既有 ApplyTaskBillingTarget。终态 usage 与价格足以在重启后重算，事务写入失败不产生重复差额。 |
| 无 Provider ID 或部分上传不能恢复 | 上传前记录结果清单与 usage；执行预算内仅重试保存，单图失败继续保存其它已生成图片。恢复领取占用执行容量，按清单 HEAD 补登记，无需 Provider ID；已登记部分结果可查询，保持原结果顺序。 |
| 重建与释放交错导致容量少算 | 受理、执行、恢复领取、终态释放、结算释放、重建共同使用全局计数行锁；Task 与计数同事务。终态待结算仍占受理容量，版本 CAS 拒绝过期 worker。 |
| 异步丢失标准参数、北向 b64 被发往 URL-only 上游 | 冻结并恢复已校验请求标量；在 202 前运行实际 adapter 校验；南向固定 URL 与北向 URL/b64 交付独立。显式 stream=false 不被误判为流式。 |
| Gemini 合同 400 被包装为 500 | converter 保留 NewAPIError 与 skip-retry，路由级校验测试覆盖。回归验证原生深拷贝保留 n=0/未知字段证据；没有为此增加原生接线。 |
| 最近比例静默改变请求尺寸 | 只接受 auto 或精确支持比例的 WxH；取消私有 a:b 与最近比例转换。仅等比例缩放到请求像素；比例不符报交付错误，不裁切、补边或拉伸。 |

恢复能力的物理边界：若 Google 图片字节从未成功进入对象存储、进程已退出且上游无可重查来源，
对象键不能还原图片。该情况保留待核实和资金事实并转人工，不自动重生成或退款；本轮不宣称
所有存储失败都能自动恢复。构造对象存储测试覆盖部分结果、无 Provider ID 补登记和完成后状态提交。

最小入侵：新增逻辑集中于图片专属文件。共享原子计费仅增加两个图片 hook，共享补偿扫描增加
一个图片分支；原生 ImageHelper 不因本轮修复增加新逻辑，非本期原生渠道保持原行为。
撤销价格表纯格式化变更。原生接线必要性及上游冲突面见实施计划 G0 表。

验证记录（2026-09-06，本地 Go 1.26.5、SQLite 测试库与模拟 HTTP；不调用真实 Provider）：

- 通过：图片定向回归（model/service/relay/controller/helper/Gemini/Vertex/两类中转/publicmodel/middleware）；
  model/service/relay 的图片用例 `-race`；相关包 `go vet`；根模块 `go build ./...`；
  `cd relaykit && GOWORK=off go build ./...`。
- 通过：`task docs:check`、`task ai:check`、`cd web && bun run docs:validate`（20 页、30 操作）、
  未触及保护目录的 `git diff --check`。
- 全量 `go test ./...` 未全绿：`TestLegacyVideoArtifactContentUsesGetResultURL` 与
  `TestDocumentPluginRunsGenericBatchArtifactChain` 在并行新增的
  `controller/task_request_evidence_delivery.go:40` 空 recorder 访问处崩溃；
  `TestDispatchPlatformUpdateUsesFetchMode` 在并行修改的 `service/task_polling.go:331`
  访问空 `resp.Request` 时崩溃。三项均已不运行图片用例而单独复现。
  另有并行存储配置用例 `TestApplyTaskArtifactStoreSetting/azure_blob_configuration_activates_azure_store`
  失败。本轮未改动这些并行功能，不宣称全库验收通过。
- 未验证：MySQL/PostgreSQL 外部数据库、多节点背压、真实 Gemini/Vertex/FunCloud/Moxing、
  对象存储真实账号、账单及生产灰度。未修改生产数据库或渠道配置。
