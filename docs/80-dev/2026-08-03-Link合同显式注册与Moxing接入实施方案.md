---
status: current
owner: Dev Team
last-reviewed: 2026-08-03
---

# Link 合同显式注册与 Moxing 接入实施方案

> 实施状态（2026-08-03）：代码注册表、渠道显式 ID/version、可执行图片 SKU 请求合同、
> Ability/运行时门禁、Task/attempt/binding/exposure 快照与轮询时版本围栏、管理端选择器、
> AssetSource 双模式 Resolver 和查询时 Asset 聚合状态已完成。重复的视频专属门禁、profile exposure
> 口径、Moxing 前端静态合同模板和未接线真人流式接口已经删除。数据库审计得到的 9 个现有 Link
> 渠道已逐一登记并通过 `Channel.ValidateSettings`。Moxing 图片公开
> `supports_link_assets` 继续保持 false，等待通用图片 Resolver 与真实 Provider 验收；生产灰度和
> 外部 MySQL/PostgreSQL DSN 验证仍是发布门禁。

## 1. 结论

本方案按项目负责人本次确认重新定义“显式注册”的实施含义：

> 对接 Moxing 时，必须新增一份代码内 Link 接入合同，声明它如何实现客户合同、公开 SKU、任务计费和 Link 虚拟素材库；管理员随后必须在具体渠道上明确选择该 Moxing 接入合同并完成路径、模型、凭据和素材能力配置。这两步共同构成显式注册。

因此，Link 接入不能只靠以下任一单独事实成立：

- 代码中出现某个公开模型名；
- 渠道名、Base URL 或模板名称看起来像 Moxing；
- 渠道选择了通用 `third_party_reverse_proxy`、`ark_assets` 或某个 converter；
- Ability、模型映射、分组、优先级、权重或价格中出现该模型；
- 管理端套用了一个前端模板，但没有把合同身份保存到渠道。

正确的可用条件是：

```text
代码已登记 Link 客户合同与 Moxing 实现
  ∩ 渠道显式保存 Moxing 实现 ID
  ∩ 渠道配置通过该实现的严格校验
  ∩ Ability / 分组 / 价格已发布
  ∩ Link 资源解析能力覆盖公开 SKU
= 可进入执行候选集
```

代码登记负责定义“系统允许存在什么合同”；渠道配置负责登记“这个渠道实例实际实现哪一份合同”。
两者缺一不可，但渠道配置不能凭空创建代码中不存在的合同。

现有 `northbound_contract_id` 继续表示客户 API 合同，不改成 Moxing 专属 ID。例如 Moxing Seedance
仍使用 `modelark.contents.generations.v3`。新增的供应商实现身份使用独立
`link_implementation_id + link_implementation_version`，避免把 Provider 名称泄漏进客户合同或为
每个渠道复制一套客户 API。

## 2. 审计范围与验证

### 2.1 范围

本次核对当前工作区中与 Link 合同落地有关的完整执行链，而不是对整个仓库做无差别格式审查：

- Link 总合同与 Link 资源架构；
- 图片、ModelArk、Kling、即梦入口和 DTO；
- 视频 SKU 能力注册、实现等价门和 Ability 发布；
- Advanced Custom 路由、converter、模板和管理端表单；
- 渠道 `OtherSettings`、保存时校验和运行时选渠；
- `/v1/assets`、`AssetBinding`、素材渠道选择、Resolver 和真人授权；
- Task/create attempt 的合同、profile、adapter 和素材快照；
- 相关后端与前端回归测试。

当前工作区存在大量未提交改动和未跟踪文件；实施与验证只处理本文定义的 Link 执行链，不把无关
改动纳入本方案，也不覆盖它们。

### 2.2 已读取的权威资料

- `AGENTS.md`、`docs/README.md` 及其必读文档；
- [Link 合同架构](../20-architecture/Link合同架构.md)；
- [Link 资源虚拟素材库架构](../20-architecture/Link资源虚拟素材库架构.md)；
- [ADR-0015：Link 公开 SKU 与实现身份版本绑定](../20-architecture/decisions/0015-Link公开SKU与实现身份版本绑定.md)；
- [Link 资源虚拟素材库实施方案](./2026-08-02-Link资源虚拟素材库实施方案.md)；
- `web/AGENTS.md`。

### 2.3 已运行验证

```text
go test ./...
cd relaykit && GOWORK=off go build ./...
cd web && bun test
cd web && bun run typecheck
cd web && bun run build
task docs:check
task ai:check
```

结果：全部通过；前端为 11 个测试文件、35 个测试。变更文件的 oxlint 无错误。全仓 `bun run lint`
仍被本方案范围外的既有前端 lint 存量阻断，不能记为本次通过项；发布前需由仓库级 lint 治理单独
清零。外部 MySQL/PostgreSQL disposable DSN 与真实 Provider 灰度仍未执行。

这些结果证明当前已有行为可运行，不代表已经满足本次确认的 Moxing 显式注册语义。

## 3. 实施前代码审计结论

### 3.1 严重

#### [S1] 渠道没有保存 Moxing 合同身份，运行时只能按通用 profile 推断

- 位置：`relaykit/dto/channel_settings.go:68-96`、`model/video_sku_implementation.go:8-58`、`middleware/video_sku_capability.go:60-77`
- 类型：需求偏离 / 多套事实来源 / 逻辑分叉
- 置信度：高
- 证据：`ChannelOtherSettings` 只有通用视频、素材 profile 和 Advanced Custom 路由；视频实现等价键只有 `PublicModel + ChannelType + Profile`；运行时也只用渠道类型和 profile 判断实现等价。
- 问题：两个不同 Provider 只要使用相同渠道类型和通用 profile，就会被视为同一个 SKU 的等价实现。代码无法回答“该渠道是否明确登记为 Moxing”。
- 影响：不能落实“在渠道处配置 Moxing 才算显式注册”；以后新增另一个 reverse proxy 或 Ark-like Provider 时，候选集可能被无意扩大。
- 最小修改：增加代码注册的 `link_implementation_id + link_implementation_version`，由渠道显式
  保存；实现查找键和运行时候选过滤必须包含该精确身份。公开 capability hash 和客户合同等价比较
  不包含实现身份；同一 SKU 可以关联多个已验证等价实现。
- 验证方式：同一 type/profile 下创建 Moxing 和非 Moxing 两个渠道，只有显式选择目标实现且配置一致的渠道可以进入候选集。

#### [S2] Advanced Custom 的 Moxing 只是前端模板，模板身份没有持久化

- 位置：`web/src/features/channels/lib/advanced-custom.ts:225-310`、`web/src/features/channels/lib/channel-form.ts:867-875`、`relaykit/dto/channel_settings.go:124-140`
- 类型：需求偏离 / 单一事实来源缺失
- 置信度：高
- 证据：前端存在 `tokensave_moxing_images` 模板，但保存时只写 `advanced_routes`；后端只看到 path、converter、models 和 auth，不知道管理员选择过 Moxing。手工写出相同 JSON 与套用模板完全等价。
- 问题：模板是配置便利功能，不是合同注册。模板名称既不落库，也不参与后端校验、运行时快照或审计。
- 影响：渠道页面无法稳定显示“这是 Moxing 合同”；模板内容变更后，历史渠道也无法证明创建时绑定的合同版本。
- 最小修改：管理端单独选择后端代码注册的实现 ID/version；模板只负责填充建议配置，不能承担
  身份判断。
- 验证方式：保存、重新加载和克隆渠道后仍能显示同一实现 ID；只复制 routes 而不选择实现 ID 时，已注册 Link SKU 必须保存失败或运行时失败关闭。

#### [S3] 图片 Link SKU 没有与视频同等级的显式合同注册

- 位置：`router/relay-router.go:83-95`、`relay/channel/advancedcustom/media_task_image_blocking.go:80-197`、`relay/channel/advancedcustom/media_task_image_blocking.go:305-360`
- 类型：多套实现 / 类型与合同重复
- 置信度：高
- 证据：视频请求在分发前解析 `VideoSKUCapability`；图片请求直接进入通用 `Distribute`。Moxing 图片模型能力散落在 converter 的字符串分支、模型映射、前端模板和测试 fixture 中，没有 `contract_id + version + capability hash` 注册项。
- 问题：`seedream-5-moxing` 虽被文档称为 Link SKU，但代码没有一个可枚举的图片 Link 合同事实源。
- 影响：无法在渠道保存时验证 Moxing 图片 route 是否完整实现客户合同，也无法阻止另一个同模型名渠道绕过合同门禁。
- 最小修改：增加最小 `ImageSKUCapability`/Link 注册项，复用现有 OpenAI 图片 DTO 和计费链；不要新建第二套图片路由或 Task。
- 验证方式：未登记图片 SKU 继续走 NEWAPI 原生行为；已登记的 Moxing 图片 SKU 必须先命中合同注册，再进入显式登记的 Advanced Custom 渠道。

#### [S4] Link 资源能力仍由通用素材 profile 代表，不能证明属于哪份 Moxing 合同

- 位置：`model/channel_asset_validation.go:14-87`、`service/asset_binding_service.go:73-180`、`middleware/asset_route_constraint.go:228-254`、`relay/asset_reference_resolver.go:18-125`
- 类型：逻辑分叉 / 多套事实来源
- 置信度：高
- 证据：素材渠道只允许 DoubaoVideo；adapter 按 `AssetUpstreamProfile` 选择；`ark_assets` 与 `third_party_reverse_proxy` 的组合即被视为可用。binding 和发送前校验没有 Moxing 实现 ID。
- 问题：profile 描述协议形状，不是供应商合同身份。当前代码可以证明 binding 属于某个 channel/credential/profile，却不能证明它属于已登记的 Moxing Link 实现版本。
- 影响：同名通用 profile 的新 Provider 可能复用错误 adapter、binding 或能力声明；任务与素材审计也无法精确定位合同实现。
- 最小修改：Link 实现注册项显式声明 `asset_resolution_modes`、asset profile、媒体类型和最小 TTL；
  binding、Resolver 和发送前复检携带实现 ID/version。
- 验证方式：Moxing binding 不能被另一个 implementation ID 的渠道消费；修改渠道实现 ID 后旧 binding 必须变为不可用或要求重新物化。

#### [S5] 当前 exposure 门禁既不能覆盖 Moxing，也没有统一实现隔离与熔断范围

- 位置：`setting/provider_exposure_setting/config.go:9-44`、`service/task_provider_exposure_policy.go:80-99`、`model/provider_cost_exposure.go:17-29`
- 类型：需求偏离 / 计费与风险门禁缺失
- 置信度：高
- 证据：默认只监控 `third_party_json_video_omni_reference`；策略按通用 upstream profile 判断，exposure
  事实没有实现 ID；当前阈值按 channel + public model + profile 聚合，自动动作只禁用事故渠道的单个
  Ability，而剩余候选计数只是熔断后的可用性结果。
- 问题：Moxing reverse-proxy/Ark 组合不会因“代码已登记为 Link 实现”自动受策略覆盖；文档也没有
  区分策略模板、风险聚合桶和熔断动作的键。
- 影响：相同 profile 的不同实现可能污染风险指标；若错误地按整个实现熔断，又会因一个凭据异常关闭
  全部等价渠道。剩余候选计数若被误当阈值，还会错误排除本应接管的 B 实现。
- 最小修改：策略模板按实现 ID/version 配置，事实、聚合和熔断至少按
  channel + implementation ID/version + public SKU 隔离；剩余候选统计所有仍合格的等价实现且不参与
  阈值。注册表不生成宽松缺省策略，禁用渠道可测试连接，启用和运行时缺策略或风险桶耗尽均失败关闭。
- 验证方式：删除 Moxing 策略后启用和运行时失败关闭；A 风险桶耗尽后 B 等价实现接管，A/B exposure
  不互相污染，两者均不可用时稳定失败关闭。

### 3.2 建议

#### [B1] Task 与 create attempt 未冻结 Provider 实现身份

- 位置：`model/task.go:118-139`、`model/task_create_attempt.go:33-99`、`controller/task_protocol_snapshot.go:14-38`
- 类型：可观测性 / 事实快照不完整
- 置信度：高
- 证据：当前冻结客户合同 ID、SKU 版本/哈希、profile 和 adapter version，但没有 `link_implementation_id`。
- 问题：当多个 Provider 共享 type/profile/adapter revision 时，历史任务不能仅靠快照证明实际使用的是哪份已审批实现。
- 影响：故障对账、exposure、素材撤回和实现退出难以按合同版本精确定位。
- 最小修改：在 Task private data、TaskCreateAttempt 普通列和必要管理审计中冻结实现 ID、version 和
  内部 content hash；不向普通客户响应暴露 Provider 名称。
- 验证方式：在同一当前实现版本内修改或删除渠道后，已创建任务仍按冻结实现 ID 和连接快照查询、
  结算与对账；这不是对已删除旧版本的兼容承诺。

#### [B2，文档已完成] 架构文档需要把“创建合同”和“登记渠道实例”写成两个连续动作

- 位置：`docs/20-architecture/Link合同架构.md:71-83`、`docs/20-architecture/Link合同架构.md:400-411`
- 类型：规范表述不完整
- 置信度：高
- 证据：当前文档正确禁止 Ability 或任意配置凭空推断 Link 身份，但没有明确写出管理员在渠道上选择已注册实现 ID也是显式注册的必要组成部分。
- 原问题：容易被理解为“代码登记后所有相同 profile 渠道自动属于该合同”，与本次确认不一致。
- 影响：实现者会继续把 Provider 身份藏在 profile、模板名或模型名中。
- 文档处理：ADR-0015、`Link合同架构.md` 与 `Link资源虚拟素材库架构.md` 已明确“一个 SKU 一份
  公开 capability、可对应多个等价实现；渠道登记精确实现 ID/version；profile 只描述适配形状”。
- 剩余验证：管理端说明、运维手册、代码注册和测试继续使用相同术语。

## 4. 唯一权威模型

### 4.1 三个不同对象

| 对象 | 身份 | 职责 | 是否客户可见 |
| --- | --- | --- | --- |
| Link 客户合同 | `contract_id + version` | 定义路径、DTO、错误、生命周期和计费语义 | 可通过 API/文档稳定承诺 |
| Link Provider 实现 | `link_implementation_id + version` | 定义某 Provider 如何完整实现客户合同与 Link 资源 | 仅管理员和内部审计可见 |
| 渠道显式登记 | 渠道保存的实现 ID/version | 声明这个具体渠道实例选择哪份代码实现 | 管理端可见，普通客户不可见 |

不得把三者合并成一个字符串：

- `contract_id` 不能改成 `moxing.*` 后再复制一份 ModelArk 客户合同；
- profile 不能升级成 Provider 身份；
- `link_implementation_id` 不能替代公开 SKU、Ability、价格或渠道 ID。

### 4.2 代码注册结构

新增本地独立文件，建议最小结构如下：

```go
type LinkImplementation struct {
    ID                string
    Version           string
    ContentHash       string
    Provider          string
    ContractID        string
    PublicSKUs        []string
    ChannelType       int
    RequiredProfile   string
    RequiredConverter string
    AssetCapability   LinkAssetImplementationCapability
}
```

实际类型可按图片和视频需要拆成少量嵌套结构，但必须保持一个注册项可完整回答：

1. 它实现哪一个客户合同；
2. 它可以承载哪些公开 SKU；
3. 它只允许什么渠道类型、route、converter、profile 和 adapter version；
4. 它是否实现 Link 资源，以及使用 `upstream_binding`、`source_url` 或两者；
5. 它支持哪些素材类型、数量、真人授权和 TTL；
6. 它的任务、计费和错误投影由哪条既有链路承担。

注册表由代码构建并在启动测试中检查 ID 唯一、SKU/客户合同一致、实现能力覆盖公开能力。不要建可由数据库任意写入的合同定义表，也不要允许 JSON 自定义新合同。

基数和版本规则固定为：

- 一个公开 SKU 只有一份公开合同和 capability hash，但可以列出多个已验证等价实现；
- `PublicSKUs` 只表示一个实现可以覆盖多个 SKU，不代表 SKU 必须独占该实现；
- 实现 ID/version 对应的声明不可原地改义；声明内容变化必须提升 version，破坏性变化使用新 ID；
- 当前处于本地未发布阶段，每个实现 ID 只保留一个已确认的当前版本，不实现 v1/v2 并存；
- 确认升级到 v2 后，直接删除本地 v1 注册、专属 adapter/parser、分支、fixture 和测试，清理 v1
  渠道、Task、binding、attempt 与 exposure 数据，只保留 v2；
- 不新增 active/retired/revoked 状态机、历史实现 resolver、双读、alias、fallback 或兼容迁移；
- 注册表按 `link-implementation-hash-v1` 生成 content hash：使用专用 hash material，trim 字符串，
  对集合语义字段排序去重，排除展示、运行状态和凭据，经 `common.Marshal` 后计算 SHA-256 并表示为
  `sha256:<hex>`；注册、渠道、binding 和 Task 复用同一个函数；
- 固定回归 fixture 防止同一 ID/version 静默变化；
- implementation ID/version/content hash 不进入公开 capability hash。

这里有三条彼此独立的版本轴，不能因字符串都写成 `v1`/`v2` 而合并：客户 API/合同版本决定公开
请求和响应，Link implementation version 标识一份当前 Provider 实现声明，southbound adapter
version 标识网关与上游 Provider 的协议 revision。本方案只禁止 Link implementation 的旧、新版本
双轨；已确认 FunCloud 只支持最新 adapter v2，因此直接删除本地 adapter v1。当前 Moxing/Qihang
implementation 名称中的 `/v1` 是它们各自唯一的首版声明，不代表仓库还保留同一实现的另一条 v2。

`link-implementation-hash-v1` 的材料明确包含 ID/version、客户合同、公开 SKU 集合、渠道类型、路径与
鉴权约束、模型 scope、必需 profile/converter/adapter version、素材解析能力和限制、Task/计费接线；
明确排除 `ContentHash` 自身、Provider 展示名、管理端文案、建议配置、凭据和运行统计。

### 4.3 渠道设置

在 `relaykit/dto/link_implementation.go` 定义通用嵌套引用，`ChannelOtherSettings` 只增加一个字段：

```json
{
  "link_implementation": {
    "id": "moxing.images.media-task",
    "version": "v1"
  }
}
```

`LinkImplementationRef` 只含 `ID` 和 `Version` 两个字符串。`relaykit/` 不导入根模块注册表；根模块
`model.Channel.ValidateSettings()` 负责按代码注册表校验，保持 relaykit 独立构建。采用一个嵌套字段
而不是在上游结构平铺两个本地字段，便于把上游修改压缩为一行。

第一阶段每个渠道只允许一组实现 ID/version。一个渠道若要承载两个不共享协议、凭据或素材能力的
实现，应拆成两个渠道，不增加数组、优先级或嵌套覆盖规则。禁用渠道可以在 exposure 策略尚未配置
时保存和测试连接；启用前必须补齐策略并重新通过完整校验。

### 4.4 管理端显式配置

渠道表单新增“Link 接入合同”选择器：

- Advanced Custom 渠道可选择 `Moxing Images / media-task v1`；
- Advanced Custom 渠道可选择 `Qihang Images / OpenAI-compatible v1`；
- DoubaoVideo 渠道可选择 `Moxing Seedance + Ark Assets v1`；
- 选项来自后端只读注册表接口，前端不维护第二份 Provider 合同清单；
- 选择后可以填充建议 routes/profile，但最终保存仍由后端严格校验；
- 编辑、克隆和重新加载渠道必须保留实现 ID/version；
- 管理端显示“已登记 / 配置不完整 / 版本不匹配”，不能只显示模板名称。

当前 `tokensave_moxing_images` 可保留为一次性界面便利模板，但不再是权威身份；更推荐由后端注册项
返回无敏感信息的建议配置，删除这份 Moxing 专用前端重复事实。

## 5. 图片与 Moxing 注册项

统一图片表中的三个名称都是本地 Link 图片 SKU，不因 `converter=none` 自动变成 NEWAPI 原生模型：

| 公开 SKU | 目标实现登记 | 当前发布约束 |
| --- | --- | --- |
| `seedream-5-moxing` | `moxing.images.media-task/v1` | 代码登记和真实验收完成后发布 |
| `seedream-5-qihang` | `qihang.images.openai-compatible/v1` | 必须单独登记并验收；登记前不能进入 Link 门禁后的候选集 |
| `nano-banana-2` | 独立验收后加入 `moxing.images.media-task/v1` 或其它等价实现 | 验收前保持未发布 |

SKU 名中的 Provider 后缀只是历史产品命名，不参与实现识别和路由。若不准备为 Qihang 建立上述实现
注册，就必须从 Link 公开 SKU 表和专属合同测试中移除，降为普通 Advanced Custom 自定义模型；不得
保留“本地公开合同存在但不受 Link 注册门禁”的中间状态。

### 5.1 Moxing 图片 Advanced Custom

建议实现 ID：

```text
id = moxing.images.media-task
version = v1
```

必须声明：

| 维度 | 值 |
| --- | --- |
| 客户合同 | `newapi.images.generations.v1`，入口 `/v1/images/generations` |
| 渠道类型 | Advanced Custom（58） |
| 公开 SKU | `seedream-5-moxing`、经单独验收后可包含 `nano-banana-2` |
| converter | `media_task_image_blocking` |
| Task | 复用现有持久化图片 Task/create attempt |
| 计费 | 复用公开模型价格键、预扣、结算和 exposure |
| Link 资源 | 只有完成通用 Resolver 接线并实测后才可声明 `source_url` 或 `upstream_binding` |

图片客户合同需新增最小版本化注册，字段仍复用现有 `dto.ImageRequest`。不要为 Moxing 新建另一条
公开图片路由、Moxing 专属 Task 表或计费账本。

当前 converter 只会校验和转换普通 HTTP(S) 图片 URL，图片入口也没有执行 Link Asset 候选交集和
发送前解析。因此第一阶段公开 SKU capability 的 `supports_link_assets` 必须保持 false；Moxing
实现注册项只声明已经验证的内部解析模式。完成第 7 节通用 Resolver 接线后，再通过公开 capability
版本/hash 变更升级为 true，不能因 Provider 理论上支持 URL 就提前开放。

### 5.2 Qihang 图片 Advanced Custom

目标实现 ID：

```text
id = qihang.images.openai-compatible
version = v1
```

该实现只覆盖 `seedream-5-qihang`，要求 Advanced Custom `/v1/images/generations`、上游模型
`seedream-5` 和 `converter=none`，并继续复用统一图片 DTO、同步响应投影和既有计费链。`none` 只表示
请求无需转换，不表示该公开 SKU 是 NEWAPI 原生合同。Link Resolver、真实 URL/binding 和 Provider
验收完成前，其 `supports_link_assets` 同样保持 false。

### 5.3 Moxing Seedance 与 Ark Assets

建议实现 ID：

```text
id = moxing.seedance-ark-assets
version = v1
```

必须声明：

| 维度 | 值 |
| --- | --- |
| 客户合同 | `modelark.contents.generations.v3` |
| 渠道类型 | DoubaoVideo（54） |
| 公开 SKU | 只列出已完成 Moxing 合同验收的 Seedance SKU |
| 视频 profile | `third_party_reverse_proxy` |
| 素材 profile | `ark_assets` |
| Link 资源模式 | `upstream_binding`；只有实测支持源 URL 后才增加 `source_url` |
| 真人能力 | 必须继续经过 API service rule、end-user scope、授权和撤回状态机 |
| Task/计费 | 复用 ModelArk Task、create attempt、quota 和 exposure |

`seedance-2-0-oversea` 是否继续作为公开 SKU，必须以当前 Moxing 文档、真实请求/查询/终态和账单验收
为准。实现身份不能从该模型名反推；只有渠道显式选择上述 ID/version 后才具备 Moxing 身份。

### 5.4 为什么 Moxing 图片和视频使用两个实现 ID

两者的客户协议、渠道类型、路径、Task 响应、素材能力和故障面不同。把它们塞进一个
`moxing.v1` 会使一次配置错误扩大到两个产品面，也会让 Advanced Custom 与 DoubaoVideo 的保存校验
互相耦合。它们可以共享管理员显示分组 `provider=moxing`，但不能共享一个模糊执行合同。

## 6. 渠道保存与 Ability 发布门禁

### 6.1 保存时校验

新增 `ValidateLinkImplementationRegistration(channel)`，由现有 `Channel.ValidateSettings()` 窄接线调用：

1. 实现 ID/version 必须精确匹配代码注册表中的唯一当前版本；旧版本一律未知并 fail-closed；
2. 渠道类型必须与实现声明相同；
3. 渠道 models 中的 Link SKU 必须全部位于注册项允许集合；同渠道上的非 Link 原生模型继续按原生
   规则处理，不因本校验被禁止；
4. Advanced Custom 的 incoming path、upstream path、converter、model scope 和 auth 类型必须符合实现；
5. 视频/素材 profile、单 Key、TTL、Project/Region 等必须覆盖实现要求；
6. 公开 SKU 声明支持 Link 资源时，实现必须至少提供一种可用解析模式；
7. 禁用渠道允许在 exposure 策略尚未配置时保存并测试连接；启用状态下缺少价格、有效 exposure
   策略或必要连接信息时保存失败；
8. 配置含已登记 Link SKU 但没有实现 ID 时失败；非 Link 原生模型不受该规则影响。

不要根据名称、Base URL、path 或 profile 自动补上实现 ID。错误消息可以建议管理员选择 Moxing，
但不能自动修正。

### 6.2 Ability

`AddAbilities`、`UpdateAbilities` 和重新启用 Ability 继续调用保存时同一校验入口。Ability 只发布已经
登记且配置合格的渠道，不承担 Provider 识别。

现有 `ValidateVideoSKUAbilityBindings` 需要按实现 ID/version 查找声明，并验证该声明的公开 capability
与 SKU 等价；implementation identity 本身不进入公开 capability hash 或等价比较。图片增加对应的
Link SKU 门禁。NEWAPI 原生模型继续使用原生 Ability 规则，不要求 Link 实现身份。

### 6.3 运行时

运行时顺序调整为：

```text
解析客户合同和公开 SKU
  -> 读取代码注册的唯一当前实现 ID/version 集合
  -> 过滤 Ability 候选
  -> 校验渠道保存的实现 ID/version、内部 content hash 与配置
  -> 计算全部 Link 资源的可解析渠道交集
  -> NEWAPI 原生优先级/权重选出具体渠道
  -> 发送前按同一实现 ID/version、素材状态和冻结快照复检
  -> converter
```

候选耗尽返回稳定的 SKU/资源不可用错误；不能退回“相同 profile 的任意渠道”。

轮询、查询、内容回源和终态结算同样要求 Task 冻结 ID/version/content hash 与代码注册的当前版本
一致。Task 快照只防止同一次当前版本部署中渠道配置漂移，不提供已删除旧实现的兼容执行。开发期
升级前直接清理本地旧 Task 和相关数据，再一次性替换注册表和代码。

## 7. Link 虚拟素材库接线

本节建立在 [2026-08-02 Link 资源实施方案](./2026-08-02-Link资源虚拟素材库实施方案.md) 的
`AssetSource + 双模式 Resolver` 已完成基础上，不重复建设第二套素材库。

### 7.1 能力归属

- 公开 SKU 的 `supports_link_assets` 是客户承诺；
- Moxing Link 实现注册项声明内部 `asset_resolution_modes`；
- 渠道只配置该实现 ID/version、凭据和必要运行参数，不能自行把不支持的能力改成 true；
- Asset profile 只决定 adapter 形状，不再决定 Provider 身份。

### 7.2 binding

`AssetBinding` 增加或可验证地冻结实现 ID/version/content hash。凭据指纹至少覆盖实现 ID/version，
避免同一 Base URL、Key 和 profile 在切换合同实现或版本后继续复用旧 binding。

本地历史 binding 无需保留，实施时直接清理并按最新实现重新物化，不从渠道名或 URL 推断 Moxing，
不写迁移或运行时 fallback。确认实现升级后，旧版本 binding、Task、attempt 和 exposure 一并清理；
当前版本不得复用旧版本 binding，也不保留仅用于旧版本清理或审计的执行链。

### 7.3 通用 Resolver

把当前视频专用的素材候选与发送前改写收敛为产品无关的 Link Resolver：

- 视频继续从官方 DTO 的媒体字段提取 `asset://ast_*`；
- 图片从 `dto.ImageRequest.image/images` 等已发布字段提取；
- Resolver 根据选中实现返回 `upstream_binding` 或 `source_url`；
- converter 只接收已解析引用，不查 Asset 数据库、不识别裸平台 `asset://ast_*`；
- 图片和视频都必须在分发前计算渠道交集，并在真正发送前复检。

共享上游路由文件只增加单行中间件/调用；大部分逻辑放在新增本地 Link 文件，遵守最小入侵。

## 8. Task、计费和审计

### 8.1 冻结字段

新增 `TaskLinkImplementationSnapshot` 本地类型并冻结：

```text
link_implementation.id
link_implementation.version
link_implementation.content_hash
```

覆盖：

- `TaskPrivateData` 中只增加一个嵌套 `LinkImplementation` 字段，具体类型和冻结逻辑放新增文件；
- `TaskCreateAttempt` 普通列；
- 素材 binding/operation job 的必要审计字段；
- 管理员日志的 `admin_info`。

`northbound_contract_id` 等 new-api 已有任务字段继续保留。当前本地 Link 历史任务数据无需兼容；新
任务缺少实现 ID 时，不得进入已登记 Link SKU 的创建链。

### 8.2 计费

实现 ID 只用于实现选择、快照和审计，不成为新的客户价格键。客户继续按公开 SKU 计费，复用：

- NEWAPI quota 单位；
- 公开模型价格；
- 分组倍率；
- 预扣、差额结算和退费；
- channel `used_quota`；
- provider exposure。

新增 Moxing 实现时需要做全链追踪：请求数量上界、EstimateBilling、预扣、Task 转移、终态结算、
unknown 对账和 exposure。不得建立 Moxing 专属余额或账本。

exposure 策略不能继续只按通用 profile 判断。策略模板按实现 ID/version 配置；实际 exposure 事实、
incident 聚合、unknown 转 exposure 比例和熔断状态至少按
`channel_id + implementation ID/version + public SKU` 隔离。注册实现不自动生成宽松默认策略；
管理员必须先配置策略，再启用渠道和 Ability，策略缺失、失效或风险桶预算耗尽时失败关闭。

自动熔断只关闭触发风险的渠道/实现/SKU 候选，不把共享 profile 的其它实现 exposure 合并进来。
`CountEnabledEquivalentVideoCandidates` 的现有 profile 口径应被本地直接重构：阈值聚合按上述风险桶
完成，剩余候选数则统计熔断后所有仍为当前注册版本、能力等价且策略有效的实现。B 实现被计入剩余可用候选
是正确行为，不得把该计数误用为 A 的 exposure 阈值。

### 8.3 可观测性

管理员可按以下维度定位一次执行：

```text
contract_id / contract_version
public_sku / capability_hash
link_implementation_id / version / content_hash
channel_id / profile / adapter_version
asset resolution mode / binding id
task or attempt id / billing / exposure
```

普通客户日志继续隐藏 Provider、渠道、凭据、上游资源 ID 和完整源 URL。

## 9. 最小入侵代码布局

建议新增文件：

```text
model/link_implementation.go
model/link_implementation_moxing_images.go
model/link_implementation_moxing_seedance.go
model/channel_link_implementation_validation.go
model/image_sku_capability.go
middleware/link_asset_constraint.go
relay/link_asset_resolver.go
relaykit/dto/link_implementation.go
model/task_link_implementation_snapshot.go
web/src/features/channels/components/drawers/link-implementation-field.tsx
web/src/features/channels/lib/link-implementation-validation.ts
```

new-api 上游文件只做必要接线：

| 文件 | 最小修改 |
| --- | --- |
| `relaykit/dto/channel_settings.go` | 增加一行通用嵌套 `LinkImplementation` 字段；引用类型在新增文件 |
| `model/channel.go` | 在 `ValidateSettings` 增加一行注册校验 |
| `router/relay-router.go` / `router/video-router.go` | 接入通用 Link 素材约束 |
| `model/task.go` | `TaskPrivateData` 只增加一行嵌套 Link 快照字段；类型和逻辑在新增文件 |
| `channel-form.ts` / `types.ts` | 序列化字段的窄接线 |
| 渠道 drawer | 渲染独立字段组件 |

本地 Link 文件直接收敛为最新唯一实现：

| 本地范围 | 允许处理 |
| --- | --- |
| `model/video_sku_implementation.go` | 重构为唯一当前版本精确查找；公开等价比较仍只看 capability |
| `model/provider_cost_exposure.go`、`model/provider_exposure_incident.go`、`service/task_provider_exposure_policy.go` | 重构为实现隔离风险桶和跨实现剩余候选统计，不保留 profile 旧口径 |
| `model/asset*.go`、Resolver、binding service | 直接增加精确实现版本并删除旧推断、迁移和 fallback |
| 本地 Task create attempt、Advanced Custom 媒体图片文件 | 直接按最新 schema 和注册事实重构，不保留旧数据形状 |

不借本次改造重排现有渠道表单、重写通用 distributor、移动整个视频注册表或抽象所有 Provider。

## 10. 实施顺序

### P0：锁定术语和注册模型

1. 已完成：以 ADR-0015 和两份架构文档中的“代码创建合同 + 渠道登记精确实现版本”为实施权威；
2. 新增通用实现注册类型和只读枚举；
3. 固定“每个实现 ID 只有一个当前版本；升级即删除本地旧版本”的单轨规则；
4. 固定 `link-implementation-hash-v1`，为现有视频注册建立一致性测试，不改变运行时。

### P1：渠道显式登记

1. 增加嵌套实现引用，并建立注册 content hash 的不可变测试；
2. 增加 Moxing 图片、Qihang 图片和 Moxing Seedance/Ark 代码注册项；
3. 增加后端管理枚举接口和前端选择器；
4. 增加按实现 ID/version 配置的 exposure 策略入口，以及按渠道/实现/SKU 隔离的风险桶；
5. 保存、编辑、克隆、测试连接和 Ability 更新都经过同一校验。

### P2：视频与素材 binding 门禁

1. 视频按实现 ID/version 查找声明，再单独验证公开 capability 等价；
2. `AssetBinding`、credential fingerprint 和 Resolver 冻结实现 ID/version/content hash；
3. Moxing Seedance 只允许登记过的 Moxing 渠道；
4. 完成真实 Provider 的创建、查询、终态、素材、撤回和清理验收。

### P3：图片合同注册

1. 新增最小图片 SKU 能力注册；
2. 将 converter 内已验证的模型值域接到注册项；
3. `seedream-5-moxing` 候选要求 `moxing.images.media-task/v1`，`seedream-5-qihang` 候选要求
   `qihang.images.openai-compatible/v1`；`nano-banana-2` 按验收后的实现集合过滤；
4. 冻结图片 Task/create attempt 的客户合同和实现 ID/version/content hash。

### P4：Link 资源双模式

1. 先完成既有 `AssetSource` 实施方案；
2. 通用化候选交集与发送前 Resolver；
3. Moxing 图片只有完成真实 URL/binding 验收后才升级 `supports_link_assets=true`；
4. capability version/hash 随公开能力变更一同升级。

### P5：上线与清理

1. 清理本地旧渠道、Task、binding、attempt 和 exposure 扩展数据，不做回填或双读；
2. 按最新结构重新创建并由管理员显式选择实现，禁止按名称/Base URL 自动推断；
3. 验证后一次性启用 fail-closed；
4. 删除 Moxing 前端静态合同重复项和任何临时兼容分支；
5. 同步架构、调用指南、运维手册、OpenAPI 和渠道说明。

## 11. 测试门禁

### 11.1 注册表

- implementation ID/version 组合唯一，同一组合的规范化内部 content hash 不可静默变化；集合声明
  仅调整顺序不会改变 hash，执行字段变化必须改变 hash 并提升 version；
- 公开 SKU 唯一对应一份客户 capability，但允许出现在多个已验证等价实现的 `PublicSKUs` 中；
- 客户合同、SKU capability 和每个实现声明一致，implementation identity 不进入公开 capability hash；
- 重复、多个版本并存或声明不完整的实现启动测试失败；注册表只能解析唯一当前版本；
- Moxing 注册项不改变 NEWAPI 原生模型行为。

### 11.2 渠道配置

- Advanced Custom 选择 Moxing 后保存并可重载；
- Advanced Custom 选择 Qihang 后保存并可重载；`converter=none` 不能绕过实现登记；
- 只有 routes、没有实现 ID/version 的 Moxing Link SKU 保存失败；
- 实现 ID/version 与 type/profile/converter/model 不匹配时失败；
- 禁用渠道可在策略缺失时保存；启用、Ability 发布和运行时必须有有效 exposure 策略；
- 非 Link 原生渠道不要求实现 ID；
- 多 Key、空 Key、错误 TTL 和不完整素材配置继续失败。

### 11.3 运行时与 fail closed

- 相同 type/profile 的非 Moxing 渠道不能进入 Moxing SKU；
- 指定渠道 ID 也不能绕过实现校验；
- affinity 命中的错误实现会被排除；
- 合格候选耗尽不回退到任意 profile；
- 开发期确认从 v1 升级到 v2 后，v1 注册、专属代码、fixture 和测试不存在，v1 请求与快照均
  fail-closed；
- 本地 v1 渠道、Task、binding、attempt 和 exposure 已清理，运行时没有 v1 双读或 fallback；
- 同 SKU 有 A、B 两个等价实现时，A 风险桶耗尽后 B 成功接管；A、B exposure 不互相污染；两者均
  不可用时稳定 fail-closed；
- 保存校验、Ability 发布、运行时过滤和发送前复检使用同一注册事实。

### 11.4 Link 资源

- binding 必须匹配 implementation ID/version/content hash、channel、credential fingerprint 和 scope；
- 版本升级后旧 binding 已清理，当前版本不得读取、迁移或复用旧版本 binding；
- source URL 只在声明支持的实现上解析；
- 多素材必须对同一渠道全部可解析；
- 授权撤回、TTL 不足、source 解密失败和 binding 失效均失败关闭；
- converter 收到裸平台 `asset://ast_*` 时拒绝，证明没有绕过 Resolver。

### 11.5 Task 与计费

- Task 和 create attempt 冻结正确实现 ID/version/content hash；
- 渠道配置修改不改变在途任务；
- unknown 不重发、不立即退款；
- 终态结算和 exposure 可按实现 ID/version 审计；
- Moxing 实现缺少 exposure 策略时不能启用或执行；
- exposure 阈值按渠道/实现/SKU 风险桶计算，剩余等价候选数包含仍合格的其它实现且不反向参与阈值；
- 所有 quota 转换继续使用集中安全函数。

### 11.6 必跑命令

```bash
go test ./model ./middleware ./relay ./relay/channel/advancedcustom ./service ./router ./controller
go test ./...
cd relaykit && GOWORK=off go build ./...
cd web && bun test
cd web && bun run typecheck
cd web && bun run lint
cd web && bun run build
```

涉及数据库列时，在 SQLite、MySQL 5.7.8+、PostgreSQL 9.6+ 分别验证最新 schema 创建、索引、唯一
约束和运行行为；本地旧 Link 数据直接清理，不增加回填、双读或回滚到旧 schema 的产品代码。

## 12. 切换与数据策略

- `northbound_contract_id` 等存量字段不重命名；
- 实现 ID/version 是新增内部身份，不进入普通客户响应；
- `ChannelOtherSettings` 使用嵌套 JSON 引用，不要求渠道表 schema 迁移；不保留平铺字段、alias 或
  双格式读取；
- `AssetBinding`、`TaskCreateAttempt` 和 exposure 本地模型直接使用最新普通列，并通过三数据库
  AutoMigrate 建表；本地旧数据清理，不做回填；
- 当前本地 Link 历史 Task、渠道和 binding 不保留；实施时清理后按最新结构重新登记和物化；
- 每个实现 ID 只保留一个当前版本；开发期确认升级后直接删除本地旧版本注册、代码、测试和数据；
- 不从渠道名称、域名、模型映射、profile 或模板内容自动迁移为 Moxing；
- 不保留数据兼容 fallback、shadow write、旧 schema 回滚分支或运行时推断。

## 13. 完成定义

本方案完成需同时满足：

1. 代码可枚举 Moxing 图片、Qihang 图片与 Moxing Seedance/Ark 的实现合同和版本；
2. 管理员能在对应渠道类型上明确选择 Moxing 或 Qihang，保存后身份可重载和审计；
3. 只有 routes/profile 而没有实现 ID/version，不再构成 Moxing Link 注册；
4. Ability 和运行时只能使用同时通过客户合同、实现合同和渠道配置校验的候选；
5. Moxing 通过平台 `ast_*` 接入 Link 虚拟素材库，Provider ID 与凭据不暴露给客户；
6. 图片的 Link 资源能力在通用 Resolver 与真实 Provider 验收前保持关闭；
7. Task、create attempt、binding、计费和 exposure 能精确追溯到实现 ID/version/content hash；
8. 实现升级采用单版本硬切换，旧版本本地注册、专属代码、测试和数据均已删除；
9. 单个实现风险桶熔断后等价实现可接管，exposure 不跨实现污染；
10. NEWAPI 原生合同、普通 Advanced Custom 和非 Link Ability 行为不被收紧；
11. 后端、relaykit、前端和三数据库门禁全部通过；
12. 架构、开发者指南、运维手册和 OpenAPI 与代码注册表一致。

## 14. 多套实现归一化

| 能力 | 当前实现 | 权威实现 | 迁移调用方 | 删除项 | 必须保留的 new-api/当前合同依据 |
| --- | --- | --- | --- | --- | --- |
| Provider 实现身份 | 前端模板名、测试名称、模型名、通用 profile | 实现 ID/version 代码注册 + 渠道显式保存 | 渠道保存、Ability、运行时、Task、素材 | 运行时名称/域名推断与重复前端清单 | 当前本地旧任务清理；new-api 已有合同字段不重命名 |
| 图片 Link 合同 | converter 字符串分支与 fixture | 版本化图片 SKU 注册 | 图片入口、Advanced Custom、计费快照 | 把模型名当合同身份的分支 | NEWAPI 原生图片继续原路径 |
| 素材实现能力 | `AssetUpstreamProfile` 兼作协议与 Provider 身份 | 实现注册的 asset capability；profile 只描述协议 | binding 选择、Resolver、发送前复检 | profile 自动获得 Link 身份 | 当前本地旧 binding/Task 清理；当前版本内任务只读自身冻结快照 |
| 视频实现查找与等价 | SKU + channel type + profile 混作一个判断 | 先按 SKU + implementation ID/version + type/profile 查找实现，再仅按公开 capability hash 判断客户等价 | 保存、Ability、运行时 | 相同 profile 自动获得实现身份；implementation ID 进入公开 hash | 客户 contract ID 与现有任务快照保留 |

## 15. 实施前健康度

- 总分：6/10
- 需求符合性：1/2——已有显式视频 SKU 注册，但缺少渠道级 Moxing 登记和图片注册。
- 单一路径与单一事实来源：1/2——视频能力较集中，Provider 身份仍分散在模板、模型名和 profile。
- 冗余与死代码控制：2/2——本次范围未发现可直接确认的大量死代码；现有回归测试有效。
- 抽象与类型设计：1/2——`contract_id`、profile、template 和 Provider 实现身份尚未正交。
- 规范与验证能力：1/2——相关测试通过且 fail-closed 基础较好，但尚未验证本次显式注册不变量。

## 16. 优先修复的三个根因

1. **没有渠道实现身份。** 先增加代码注册与实现 ID/version，让 Moxing 成为可保存、可验证、可审计的明确事实。
2. **图片没有显式 Link SKU 合同。** 再把 Moxing converter 和 Qihang `converter=none` 的字符串规则
   收敛为版本化图片合同，不新增并行图片 API。
3. **当前版本与 exposure 范围未正交。** 最后让全部执行只认唯一当前实现版本，并让风险事实按
   渠道/实现/SKU 隔离；Resolver 按 implementation capability 选择 binding/source，profile 只负责
   协议适配。
