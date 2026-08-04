---
status: testing
owner: Dev Team
last-reviewed: 2026-08-04
---
# 飞彩 Seedance 新协议与 Link 虚拟素材统一对接分析

## 1. 目的与状态

本文根据新飞彩文档、当前 Link 合同与 Link 资源虚拟素材库设计，以及仓库实际代码和本地数据，分析如何将现有飞彩 Seedance 旧实现切换到 2026-08-03 版本的新南向协议，并让新实现通过统一 `asset://ast_*` 合同消费虚拟素材。

当前状态是**720p 实现已 fail-closed 收敛、生产验收待办**。新飞彩实现已按 ADR-0015 单轨硬切并同步到
`docs/20-architecture/`、`docs/40-operations/`、OpenAPI 与公开指南；这不表示渠道已发布。
1080p/4K 的 Provider size 组合、Provider HTTPS、客户价格和真实黑盒验收完成后，本文再转入
`docs/99-archive/`。

这里的“已完成”以当前工作区为基线。`mediaarrays` converter/parser、内容代理、capability 辅助文件
及本文仍包含未跟踪文件；形成包含代码、测试和文档的完整提交，并在全新 checkout 中验证之前，
不得把当前工作区描述成可复现的 CI、部署或回滚基线。

已落地的唯一当前实现为 `feicai.seedance-videos/v1`、
`third_party_json_video_media_arrays`、`54:third_party_json_video_media_arrays:v1`。旧 omni
实现、profile、adapter、parser、fixture 与专属测试已删除，不保留别名、双读或 fallback。

2026-08-04 ADR 治理收敛后，本分析只负责飞彩 Link Provider 南向协议切换，不再讨论或设计
NEWAPI 原生 OpenAI Videos 的“恢复”“共存”“隔离实现”或历史兼容。原生代码以上游为唯一权威；
本分析出现的 `/v1/videos` 只表示飞彩 Provider 的南向路径字符串。

## 2. 证据范围

### 2.1 设计与研究资料

- [飞彩 Seedance 更新文档](<../70-research/飞彩/飞彩-seedance%20更新.md>)；
- [Link 合同架构](../20-architecture/Link合同架构.md)；
- [Link 资源虚拟素材库架构](../20-architecture/Link资源虚拟素材库架构.md)；
- [ADR-0001：视频上游协议适配与任务执行快照](../20-architecture/decisions/0001-视频上游协议适配与任务执行快照.md)；
- [ADR-0008：共享异步任务计费状态机与原子补偿](../20-architecture/decisions/0008-共享异步任务计费状态机与原子补偿.md)；
- [ADR-0009：请求级媒体与平台托管素材双路径](../20-architecture/decisions/0009-请求级媒体与平台托管素材双路径.md)；
- [ADR-0011：异步创建未知与轮询合同违例对账](../20-architecture/decisions/0011-异步创建未知与轮询合同违例对账.md)；
- [ADR-0015：Link 公开 SKU 与实现身份版本绑定](../20-architecture/decisions/0015-Link公开SKU与实现身份版本绑定.md)；
- [架构决策索引与原生边界](../20-architecture/decisions/README.md)；
- [视频上游接入与异步任务架构](../20-architecture/视频上游接入与异步任务架构.md)；
- [素材代理与真人授权架构](../20-architecture/素材代理与真人授权架构.md)。

### 2.2 已核对的代码链路

- Link implementation 显式注册、content hash 和渠道精确绑定；
- `VideoSKUCapability`、Ability 门禁和 ModelArk v3 合同；
- `AssetRouteConstraint` 候选渠道交集；
- `resolveAssetReferencesForAttempt` 发送前二次解析与类型化请求改写；
- DoubaoVideo 旧 JSON omni-reference converter、状态归一和内容代理；
- Task create attempt、计费预扣、冻结连接和后台轮询；
- 本地 SQLite 的飞彩渠道、Ability、Task、Asset、attempt 和 exposure 数据。

当前分支中的原生视频专属 Router、中间件、`client_protocol`、素材接线和历史读取代码不属于飞彩
Link 实现依赖。本分析不复用、不扩展这些代码，也不以相关原生专属测试作为飞彩验收基线。

### 2.3 已运行验证

```text
GOCACHE=/tmp/yuan-gateway-go-cache \
go test ./middleware \
  -run 'Test(AssetRouteConstraintInspectsModelArkCreateRoute|UnregisteredVideoSKUFailsClosedBeforeDistribution)$'

GOCACHE=/tmp/yuan-gateway-go-cache \
go test ./model \
  -run 'Test(FeicaiVideoSKUCapabilitiesAreStableAndResolutionBound|PublishedVideoSKURegistryMatchesOpenAPIAndUserGuides|LinkImplementationRegistryAndExplicitChannelRegistration|LinkImplementationContentHashIsCanonicalAndScoped)$'

GOCACHE=/tmp/yuan-gateway-go-cache \
go test ./relay/channel/task/doubao \
  -run 'TestJSONVideoCreateRequest(UsesMappedModelFromTypedContract|RejectsGenericBodyFallback)$'
```

定向用例通过。全包测试在当前沙箱中受 miniredis 监听本地端口限制，未得到完整结果；
不得将该环境限制写成业务用例失败，也不得将定向通过外推为新协议已验收。这些验证只证明当前
飞彩旧实现、Link capability/implementation、ModelArk 素材门禁和类型化 converter 基线内部一致，
不证明旧实现兼容新飞彩协议。本次没有向新 Provider 发起真实请求；
文档只提供 HTTP 地址，且当前未提供本次联调所需的可用凭据和 HTTPS 入口。

代码切换完成后又执行并通过：

```text
go test ./... -run '^$'
go test ./dto ./model ./relay/common ./relay/channel/task/doubao/thirdparty/mediaarrays \
  ./relay/channel/task/doubao ./relay ./controller ./service -run '<飞彩与共享边界定向用例>'
cd relaykit && GOWORK=off go build ./...
cd web && bun run i18n:sync && bun run docs:validate && bun run typecheck
bash scripts/docs/check-docs.sh
```

定向用例覆盖新请求/响应、唯一任务身份、四态与坏 URL、精确 implementation/hash、渠道根地址、
capability/OpenAPI 字段存在性、billing probe、创建 unknown、轮询脱敏和冻结 Bearer 内容源。
上述验证仍不替代真实 Provider 黑盒。

## 3. 核心结论

需要更新代码，并且应按当前 Link 设计执行**一次性单轨硬切换**，不能只修改 Base URL 和数据库模型映射。

目标边界如下：

1. 客户继续使用 ModelArk v3 `POST /api/v3/contents/generations/tasks`；
2. 飞彩文档中的 `/v1/videos` 仅描述本次网关到 Provider 的南向调用，不以该路径反推或改写客户协议；NEWAPI rc23 原生 OpenAI Videos `/v1/videos` 的路径、DTO、默认值、错误、响应、Remix、Ability 和计费分发保持原样；
3. 删除本地旧 `feicai.seedance-json-omni/v1` 注册和旧协议专属实现；
4. 新实现通过现有 Link Resolver 使用 `source_url`，不建立飞彩专用素材表或第二套上传链路；
5. 第一阶段只发布有直接 size 证据的两个 720p SKU；原有三个 1080p/4K 标识在取得逐模型证据前
   不注册北向 capability、implementation 或 Ability。

这里的两个 `/v1/videos` 仅是路径字符串相同：一个是 NEWAPI 已有的原生客户合同，
另一个是飞彩 Link implementation 的 Provider 南向路径。本次不修改、复用、投影或收紧
NEWAPI 原生 OpenAI Videos，也不新增 `(SKU, client_protocol)` 支持矩阵。飞彩 Link SKU 只通过
ModelArk 类型化路由、capability 和显式候选实现发布；本分析不定义原生 `/v1/videos` 如何处理
合同外模型，更不修改原生入口来识别或拒绝 Link SKU。

### 3.1 原生代码排除边界

- 不修改或复制 NEWAPI 原生 Router、DTO、Relay、Sora adapter、模型发现、Ability 和计费接线；
- 不增加原生专属 Link SKU 中间件、`client_protocol`、AssetRouteConstraint、durable attempt、历史
  任务识别或 fallback；
- 不把原生视频专属测试加入飞彩实施完成条件；飞彩只验证自己的 ModelArk 客户合同和 Provider
  南向实现；
- 如果上游未来原生提供等价飞彩能力，先做部署数据核查，再删除未发布 Link 实现并迁回上游；
  不预留并行投影或兼容代码。

本次切换不需要新增 ADR。Provider profile/执行快照由 ADR-0001 负责，计费由 ADR-0008 负责，
媒体双路径由 ADR-0009 负责，create unknown 由 ADR-0011 负责，Link implementation/capability/
Resolver 由 ADR-0015 负责。

## 4. 新旧协议差异

| 维度     | 当前旧实现                               | 新飞彩文档                             | 影响                                                                                      |
| -------- | ---------------------------------------- | -------------------------------------- | ----------------------------------------------------------------------------------------- |
| 创建路径 | `/v1/videos`                           | `/v1/videos`                         | 路径相同不代表协议等价                                                                    |
| 时长     | `duration` 整数                        | `seconds` 字符串或 `duration` 整数 | 新 converter 应统一发`duration` 整数                                                    |
| 画幅     | `ratio`                                | `size`                               | 必须建立显式的 SKU/分辨率/画幅到 size 映射                                                |
| 图片     | `input_images`                         | `images`                             | 需要新 DTO/converter                                                                      |
| 音频     | `audio_url_list`                       | `audios`                             | 需要新 DTO/converter                                                                      |
| 视频     | 不支持                                   | `videos`，仅 Pro PI                  | 不能直接扩大现有 SKU 能力                                                                 |
| 参考语义 | `reference_mode`，区分首帧/尾帧/参考图 | 通用`images` 数组                    | 首帧和尾帧语义未证明等价                                                                  |
| 创建响应 | 使用`id`                               | 同时返回`id` 和 `task_id`          | `id` 是当前资料中跨创建、查询和内容地址稳定的唯一任务身份；`task_id` 暂不参与身份判断 |
| 任务状态 | 四态                                     | 四态                                   | 可继续归一到共享 Task                                                                     |
| 结果地址 | 同源 HTTPS，无下载 Bearer                | 同源 HTTP，下载需 Bearer               | 当前 parser 和内容代理均不兼容                                                            |

新文档虽称参数与价格经过实测，但公开内容仍不足以证明六种画幅在每个分辨率下的精确 `size` 白名单，也没有说明 `images` 顺序是否具有首帧、尾帧或普通参考图语义。这两项不得由 converter 猜测。

任务身份同样不得猜测或建立多字段 fallback。研究资料明确显示：创建响应中 `id == task_id`，但查询响应中两者不相等，而查询路径和 `video_url` 继续使用 `id`。因此当前唯一有证据的规则是：创建时只冻结非空 `id`，查询时要求响应 `id` 等于本次查询使用的冻结上游任务 ID；`task_id` 暂作为非权威 Provider 内部字段，不存储为任务身份、不向客户投影、不与 `id` 比较。

## 5. Link 实现身份与单轨切换

当前 Link 扩展仍处于本地未发布阶段。设计要求实现升级时删除旧注册、adapter、parser、fixture、测试和本地旧数据，不保留 v1/v2 并行、alias、双读或 fallback。

旧实现 ID 中的 `json-omni` 直接描述了已退出的 `reference_mode/input_images` 协议。Link implementation ID 用于标识具体 Provider 实现，可以使用飞彩名称；video profile 只表达可测试的协议形状，必须保持供应商中立。已落地的唯一当前身份与协议键为：

```text
link implementation: feicai.seedance-videos/v1
video profile:       third_party_json_video_media_arrays
adapter version:     54:third_party_json_video_media_arrays:v1
create path:         /v1/videos
query path:          /v1/videos/{upstream_task_id}
task contract:       shared_video_task
asset mode:          source_url
```

该 profile 按协议形状命名，不包含飞彩或其它供应商名称。现有 FunCloud 命名是存量例外，未在本次
切换中顺手重命名。客户合同版本、Link implementation version 和 southbound adapter version
继续保持三条正交版本轴。

`newapi` 不进入 implementation ID：它既不是 Provider 身份也不是稳定协议形状，并且容易与受保护的
NEWAPI 项目身份混淆。客户入口与 Provider 南向路径即使同为 `/v1/videos`，也只表示字符串相同；
飞彩的客户合同仍是显式登记的 ModelArk v3 Link 合同，飞彩 `/v1/videos` 只属于该合同内部的
渠道适配协议，不产生第二个客户入口或 NEWAPI 原生模型身份。

## 6. 公开 SKU 切换策略

### 6.1 第一阶段只启用两个已验证 720p SKU

| Link 公开 SKU                   | 新飞彩上游模型                  | 飞彩 v1 状态 |
| ------------------------------- | ------------------------------- | ------------ |
| `seedance-2.0-standard-720p`  | `seedance-2.0-vip-720p-azhw`  | 已登记       |
| `seedance-2.0-standard-1080p` | `seedance-2.0-vip-1080p-azhw` | 未发布，待 size 取证 |
| `seedance-2.0-value-720p`     | `seedance-2.0-933-720p-azhw`  | 已登记       |
| `seedance-2.0-value-1080p`    | `seedance-2.0-933-1080p-azhw` | 未发布，待 size 取证 |
| `seedance-2.0-value-4k`       | `seedance-2.0-933-4k-azhw`    | 未发布，待 size 取证 |

Provider 资料中的模型名称不含 `-feicai` 后缀。代码只为两个 720p SKU 冻结模型映射；1080p/4K
映射只作为待验证资料保留，不能写入当前飞彩 implementation 或启用 Ability。

### 6.2 首次切换不合并的其它模型

- `seedance-2.0-vip-720p-mini-azhw`；
- `seedance-2.0-vip-720p-fast-azhw`；
- `seedance-2.0-vip-4k-azhw`；
- `seedance2.0-sd2`；
- `seedance-933-pro-pi`。

SD2 只支持 11–15 秒，要求 1–9 张图片且不支持音频和视频；Pro PI 固定 15 秒、按次计费，且是唯一文档化支持参考视频的模型。mini、fast 和 VIP 4K 也代表新的产品档位。如果后续决定公开，应分别新增公开 SKU capability、价格键、模型映射和黑盒验收，不复用当前两个 720p SKU 的能力哈希。

## 7. 新公开 capability 边界

当前只为两个 720p SKU 注册北向 capability 和飞彩 implementation：

- `supports_link_assets=true`；
- `supports_mixed_media_paths=false`，继续禁止一个请求混用直接 URL 和 `asset://`；
- 两个北向 capability 保持 `max_images=9`、`max_audio=3`、`max_videos=0`；
- 第一阶段只发布已证明可转换的 `reference_image` 和 `reference_audio`；
- Provider 实测前不发布 `first_frame` / `last_frame`；
- 画幅只保留 `16:9` 和 `9:16`；当前仅 720p 组合分别映射到 `1280x720` 和 `720x1280`。

未证明的其它画幅、首尾帧能力以及 1080p/4K size 组合均 fail closed；未来如需恢复，必须先取得
逐模型精确映射和真实创建证据，再提升 capability/implementation 身份，不能在 v1 中原地放宽。

## 8. 虚拟素材库统一对接

### 8.1 复用现有 source_url Resolver

新飞彩协议可在任务请求中抓取图片、音频和视频 URL，因此不需要飞彩专用素材表或第二套上传链路：

```text
客户创建 Asset，平台保存受保护的 HTTPS 源 URL
  -> 客户在 ModelArk content 中传 asset://ast_*
  -> AssetRouteConstraint 计算能解析全部素材的渠道交集
  -> 分发选中精确飞彩 implementation
  -> Resolver 发送前再次校验状态、授权、媒体类型和 TTL
  -> 在内存中将 asset:// 改写为解密后的 HTTPS URL
  -> 新飞彩 converter 写入 images/audios/videos
```

Converter 只消费已解析引用，不查询数据库、不解密 AssetSource、不自行切换渠道。明文 URL 不得进入 Task、普通日志、幂等响应或指标。

### 8.2 已落地的 implementation asset capability

新实现保留 `RequiredAssetProfile=none`，并使用 `source_url` 解析普通素材。实现级边界与公开
capability 均收窄为图片/音频，不发布参考视频：

```text
supports_managed_assets: false
asset_resolution_modes:  [source_url]
asset_kinds:             [general]
media_types:            [image, audio]
max_images:             9
max_audio:              3
max_videos:             0
supports_mixed_paths:   false
source_min_ttl_seconds: 3600
```

实现级上限不预留未发布能力；当前两个 720p 实现 SKU 同样限制 `max_videos=0`。

### 8.3 TTL 采用保守平台下限

新文档只说生成通常需要 1–5 分钟，没有说明 Provider 在什么时点抓取素材，也没有排队上限。
当前实现采用 3600 秒保守平台下限，避免复制 FunCloud 的 300 秒配置；它不是 Provider SLA。
发布前仍必须用签名 URL 验证排队、抓取、重试和超时行为。`expires_at=0` 仍只能作 best-effort，
不得对外描述成永久可用。

## 9. 真人素材边界

新文档对多个模型标注“支持真人内容”，但这只是 Provider 生成能力描述，不等于满足平台 `real_person` Link 资源合同。

当前平台对真人 Asset 要求所有权、App、`end_user_subject`、active 授权和解析作用域全部匹配；`source_url` 不得用于 `real_person`。新飞彩文档没有素材创建、查询、删除、授权主体绑定或撤回合同，因此新实现只能注册 `general` Asset，不得注册 `real_person`。

直接 HTTP(S)/Data URL 不会自动得到真人分类。如果业务要求平台对飞彩任务执行严格真人禁止或授权强制，还需要禁止直接媒体路径或新增可验证的媒体分类能力；不能仅靠 URL 判断是否包含真人。

## 10. 请求转换设计

新飞彩 DTO 和 converter 已放入独立文件，现有热路径只保留 profile/adapter version 分发。converter 负责：

1. 复用 `VideoSKUCapability.ValidateModelArkRequest` 完成客户合同校验；
2. 合并所有非空 `text` 为 `prompt`；
3. 将 `duration` 作为整数发送，避免引入 `seconds` 字符串分支；
4. 通过显式白名单将公开 `resolution + ratio` 转换为 `size`；
5. 将 `reference_image` 写入 `images[]`；
6. 将 `reference_audio` 写入 `audios[]`；
7. 遇到未发布角色、媒体类型或混合路径时在发送上游前拒绝；
8. 只接收已由 Resolver 改写的 Asset 引用，不在 converter 中增加数据库或授权逻辑。

虽然新 Provider 接受图片 Data URL，虚拟素材库仍只应保存并解析 HTTPS 源 URL。Data URL 可作为公开 SKU 允许的请求级媒体，不应进入长期 AssetSource 持久化合同。

## 11. 响应、轮询、创建未知与内容代理

### 11.1 任务身份与轮询

新创建 parser 应要求非空 `id`，拒绝超长 ID 和控制字符，并将该 `id` 冻结为 `upstream_task_id`。不对创建响应中的 `task_id` 建立 fallback，也不要求 `id == task_id`。

后续查询路径使用冻结的 `upstream_task_id`；查询 parser 必须要求响应 `id` 等于该冻结值。响应中的 `task_id` 暂视为非权威 Provider 内部字段，不与 `id` 比较，不用于查询、内容下载或客户任务投影。只有 Provider 后续证明它具有稳定对账价值时，才能另行决定是否以脱敏、非身份方式记录。

轮询继续把 `queued`、`processing`、`completed`、`failed` 归一到共享 Task。响应 `id` 与冻结任务 ID 不匹配、未知状态和成功无结果 URL 都必须进入上游合同违例与对账链路。

### 11.2 创建发送后异常默认 unknown，精确账户拒绝除外

凡是已经尝试发送 Provider 的非 2xx HTTP 响应、连接异常、创建响应解析失败或无法取得可信
`id`，均默认进入 `unknown`，不自动释放 hold，不自动重新创建。

研究资料中的 `fail_to_fetch_task`、`upstream_request_rejected` 和 `diagnostic_id` 只证明中转服务暴露过这些错误外观，不能证明它们是上游原始 `error.code`，也不能证明任务绝未创建。只有获得精确 `HTTP status + Provider error.code + 未创建语义` 的书面或可重复证据，并增加对应回归测试后，才能登记最小精确的 terminal rejection 组合；不得整个放开 400、401、403 或其它状态码。

2026-08-04 本地对两个 720p SKU 的真实请求均收到
`HTTP 403 + feicai_account_required`，Provider 明确表示该模型仅限工作台已登录用户。该精确组合已登记为
`terminal_rejection`并增加回归；其它 403、其它错误码以及所有未登记组合仍然 fail closed 为
`unknown`。

### 11.3 带冻结 Bearer 的内容代理

新文档要求对 `video_url` 携带与创建任务相同的 Bearer Key。客户不应获取该 Key，应继续使用本站内容代理：

```text
客户 -> new-api Task content endpoint
new-api -> 校验客户/APP/授权/任务终态
new-api -> 使用 Task 中冻结的飞彩 Key 请求同源 HTTPS 结果 URL
new-api -> 流式返回视频内容
```

不得在任务完成后改用渠道当前 Key，否则 Key 轮换会导致旧任务内容无法下载或越过冻结连接边界。

实施形态是为新的供应商中立 profile 建立带冻结 Bearer 的 content source，同时继续强制同源 HTTPS、SSRF、Range、客户/App 所有权和授权校验。单轨硬切完成后，旧 omni 无鉴权 content source 及其专属测试应被删除，不建设新旧飞彩内容源长期并存。如发布环境只读审计发现真实旧任务，应暂停硬切并重新作迁移决策，而不在当前代码中预留双轨。

## 12. 安全阻塞：HTTP 不可用于发布

新文档提供的 Base URL 和结果 `video_url` 均是 `http://43.161.200.208`，而创建、查询和下载均要携带 Bearer Key。当前代码要求 JSON video 结果 URL 与冻结 Base URL 同源且使用 HTTPS，这是正确边界，不应为新对接放宽。

发布前必须获得：

- 可验证证书的 HTTPS 域名；
- HTTPS 创建、查询和内容下载地址；
- 下载地址的同源或可审计跳转规则；
- Bearer Key 在内容下载阶段的作用域和有效期说明。

生产分组在此之前必须保持禁用。本地专用数据库已按用户要求在 `default` 分组使用现有
`https://feicai123.top` 根地址启用新渠道与 Ability，仅用于黑盒联调；不得将该本地配置视为
Provider HTTPS 生产合同已验收。

## 13. 计费与 Provider 成本

### 13.1 本地测试配置与生产缺口

当前本地数据库已将两个已登记 720p Link 公开 SKU 设为 `tiered_expr`，预扣 Token 为 `1`，
并按 2026-08-03 Provider 文档的人民币每秒成本和本地 `USDExchangeRate=7` 配置测试表达式。
`default` 分组消费倍率 `1` 按现有规则生效。

上述表达式是本地联调基线，不代表 Provider 账单语义或客户生产售价已批准。
生产开放前仍须根据真实账单复核精度、取整、失败扣费和分组倍率后单独批准客户售价。

### 13.2 已落地的计费输入与剩余价格审批

- 按秒模型使用 `tiered_expr`，以受信 `_task.duration_seconds` 为计费维度；
- Pro PI 使用固定按次表达式，不依赖客户传入时长；
- 宽幅使用受信 `_task.size_multiplier` 或等价枚举，不读取未校验原始字符串；
- 预扣和终态结算使用同一份冻结 billing probe 和 expression；
- duration 在进入乘数前经过公开 SKU 范围校验；
- 价格、表达式和预扣 Token 上界在开放 Ability 前一次性验证。

DoubaoVideo billing probe 已补充 `ratio/size/size_multiplier`，并与 converter 复用同一个经验证的
size 映射。当前只有 720p 两种画幅可形成 billing probe，multiplier 均为 1；1080p/4K 和未发布
宽幅在预扣前失败关闭。本地只保留两条 720p 测试表达式；生产客户售价仍须管理员批准。

## 14. 本地数据切换审计

本次先完成 `one-api.db` 只读核对，确认无存量任务和绑定后执行单轨迁移。迁移后状态为：

| 对象                         | 当前数量 | 迁移结果                                      |
| ---------------------------- | -------: | --------------------------------------------- |
| 绑定新实现的启用渠道         |        2 | 渠道 51/52 已切换新 profile、implementation 和映射 |
| `default` 分组新飞彩 Ability |        2 | 只保留 standard/value 720p，已启用          |
| 飞彩 Task                  |        0 | 无需旧任务兼容路径                            |
| 飞彩 create attempt        |        2 | 真实测试均因 `feicai_account_required` 拒绝并释放预扣 |
| 飞彩 AssetBinding          |        0 | 无需 binding 迁移                             |
| 飞彩 exposure/incident     |        0 | 无需历史风险桶兼容                            |
| 全库 Asset / AssetSource     |    0 / 0 | 当前本地无需素材迁移                          |

本地协议身份已完成一次性硬切，没有为旧 parser、旧 Task 轮询或旧 binding Resolver 保留兼容代码。
渠道 51/52 的 1080p/4K 模型映射、三条 Ability 和三条价格键已删除，不依赖运行时拒绝代替配置治理。
生产或其它环境必须重复这份
只读审计；本地零数据和本地迁移不能替代生产审计。

## 15. 严重问题与发布阻塞

### P0：Provider 只文档化 HTTP

创建、查询和内容下载均涉及 Bearer Key 或客户媒体，不能放宽现有 HTTPS 规则。

### 已修复：size 白名单按已验证分辨率 fail closed

公开 capability 已删除无精确映射的四种画幅，只保留 `16:9 -> 1280x720` 和
`9:16 -> 720x1280`。当前白名单只登记 720p；1080p/4K 不再复用这两组像素值，请求转换、
billing probe、implementation 注册和北向 capability 查找都会失败关闭。

### 已修复：未证明的媒体角色已从公开 capability 删除

新协议只有通用 `images[]`。当前只发布 `reference_image` / `reference_audio`，明确拒绝
`first_frame`、`last_frame` 和参考视频，不以数组位置猜测角色。

### 已修复：结果内容使用冻结 Bearer

adapter v1 的 content source 只接受精确冻结实现身份/hash，验证同源 HTTPS 后使用 Task 冻结 Key，
并拒绝重定向。

### 部分闭环：素材 TTL 采用 3600 秒保守门禁

选渠道与发送前均按 3600 秒下限复检；Provider 真实抓取时点仍需黑盒验证。

### 部分闭环：本地测试价格已配置，生产售价尚未批准

billing probe 已冻结经验证的 `duration_seconds`、`ratio`、`size`、`size_multiplier`；本地
`default` 本地分组已配置测试表达式、预扣上界并启用 Ability。生产客户售价和真实账单
语义仍须管理员批准，在此之前不得扩展到生产分组。

### 已修复：OpenAPI capability 字段与 CI 的零值掩盖

切换前 `docs/openapi/relay.json` 的固定 Seedance 2 SKU capability extension 错用了
`supports_managed_assets`。该名称实际属于 implementation 级
`LinkAssetImplementationCapability`，不是公开 SKU 字段；一致性测试结构体没有消费这个 key，
反序列化时会将其静默丢弃。测试随后读取缺失的 `supports_link_assets`，得到 Go bool 零值 false，
恰好与当前运行时 false 相等，形成“字段从未被比较但测试通过”的假一致。

固定 Seedance 2 SKU 的 vendor extension 已使用 `supports_link_assets: true`，一致性测试以指针
校验字段存在，并逐值比较媒体角色、默认时长、画幅、数量、素材与生命周期边界。

## 16. 已完成的最小实施切片

按最小入侵约束，主要逻辑已由独立新文件承载，现有上游文件只保留极窄分发或调用：

1. **公开 capability**：新建飞彩新协议 capability 文件，只注册并冻结两个已验证 720p SKU 的 version/hash；
2. **Link implementation**：删除旧 omni 注册，新增唯一当前实现和 `source_url/general` 素材能力，仅登记已验证的两个 720p 模型映射；
3. **请求 converter**：独立实现 `model/prompt/duration/size/images/audios`；
4. **响应 parser**：以创建 `id` 作为唯一冻结上游身份，查询只校验响应 `id` 与冻结值，同时处理四态、结果 URL 和安全错误投影；
5. **adapter 接线**：新增按协议形状命名的供应商中立 profile 和 adapter version，删除旧专属分支、fixture 和测试；
6. **创建未知**：所有未登记发送后异常默认 unknown；真实验证后只为精确
   `HTTP 403 + feicai_account_required` 登记 terminal rejection 并增加回归；
7. **内容代理**：为新 profile 建立带冻结 Key 的 content source，继续强制同源 HTTPS 和 SSRF，单轨切换后删除旧 omni 无鉴权变体；
8. **OpenAPI 与 CI**：将 vendor extension 收敛为 `supports_link_assets`，同步新 capability 的所有公开值和人类可读指南；一致性测试使用可检测缺失的字段表示，必须同时证明字段存在且值与运行时一致；
9. **计费**：补充受信 size 维度；价格表达式、预扣上界和真实账单复核保留为发布门禁；
10. **数据切换**：代码只接受新身份；部署环境的渠道、Ability 与价格由运维审计后显式切换；
11. **文档**：更新架构事实、运维手册、公开 capability/OpenAPI 和路线图。

硬切换的显式删除与迁移清单至少包括：

- `model/link_implementation.go` 中的旧实现常量、注册项及 content hash 固定测试；
- 旧 SKU capability version、profile 常量、Ability gate 和 implementation 等价性测试；
- `relaykit/dto/upstream_profile.go` 的旧 profile 枚举，以及 Doubao adapter version/dispatch allowlist；
- 旧 omni converter/parser、轮询 fixture、content source 及其 controller/service 回归；
- `setting/provider_exposure_setting/config.go` 的默认 implementation 身份和 exposure 测试；
- 前端渠道 profile 类型、表单默认值、下拉项与校验；
- 当前事实文档、运维手册、路线图、公开指南和 OpenAPI；历史 `99-archive` 只保留追溯，不批量改写；
- 发布环境 Channel、模型映射、Ability、价格和旧 Task/attempt/binding 数据审计结果。

NEWAPI 原生 OpenAI Videos 文件明确排除在本次实施清单外。不修改其 Router、DTO、Relay、adapter、
Ability、模型发现、计费、Task 投影或素材接线；验收以“飞彩改动未触碰这些文件”为准，不新增或
强化本地原生兼容测试。

## 17. 必要回归与真实验收

### 17.1 确定性回归

- 两个 720p SKU 只命中新精确实现；三个高分辨率 SKU 没有飞彩实现候选，原始上游模型名不能作为未注册的 Link SKU 进入；
- 时长、画幅、图片、音频和视频角色/数量边界；
- 明确拒绝未发布字段、角色和媒体组合，不静默丢弃；
- 图片、音频 `asset://ast_*` 所有权、App、状态、多素材渠道交集和发送前 TTL 复检；
- 禁止直接 URL 与 `asset://` 混用，`real_person` Asset 不得选中飞彩 `source_url`；
- 明文 URL 不进入 Task、attempt、普通日志或幂等响应；
- 创建响应缺少 `id` 必须进入 unknown；存在 `id` 时只冻结该值，不以 `task_id` 回退或要求两者相等；
- 查询响应 `id` 必须等于冻结的上游任务 ID；查询响应 `task_id` 不同不得触发身份冲突，也不得替代 `id`；
- 四态归一、未知状态、查询响应 ID 不匹配、成功无 URL 和非 HTTPS URL；
- 下载使用冻结 Key，不回退到渠道当前 Key；
- 创建发送后的未登记非 2xx、断连、响应无法解析、缺少可信 `id`，以及出现 `fail_to_fetch_task`、`upstream_request_rejected` 等包装错误码时均保持 unknown，不自动重复创建或释放 hold；
- 只有精确 `HTTP 403 + feicai_account_required` 是 terminal rejection，必须释放预扣；其它 403 不得泛化匹配；
- OpenAPI vendor extension 必须显式包含 `supports_link_assets`，值与运行时两个 720p SKU capability 逐项一致；缺失字段不能再被 bool 零值伪装成 false；
- 新供应商中立 profile 的 adapter 与带冻结 Bearer content source 能唯一命中，旧 omni adapter 和无鉴权 content source 已删除；
- 飞彩改动不包含 NEWAPI 原生 OpenAI Videos 文件；上游原生 Router、DTO、Relay、Sora adapter、
  模型发现和计费 diff 保持为空；
- 飞彩 Link SKU 只通过 ModelArk 类型化路由和已登记候选实现验收；不把原生 `/v1/videos` 的
  本地拒绝行为作为飞彩合同或回归条件；
- 时长、已发布 size、预扣、终态复算和 quota 饱和安全。

### 17.2 真实 Provider 验收

| 类型       | 必测场景                                                                                                                     |
| ---------- | ---------------------------------------------------------------------------------------------------------------------------- |
| 基础创建   | 先完成两个 720p SKU 各一条从创建到终态；1080p/4K 先取证 size，升级实现身份后再验收                                              |
| 时长       | 最小、默认、最大和越界值                                                                                                     |
| 画幅       | 每个准备发布的 ratio/size 组合                                                                                               |
| 媒体       | 文生、单图、多图、图片+音频；不支持的参考视频拒绝                                                                            |
| Link Asset | 永久 HTTPS、短签名 URL、已过期 URL、多素材渠道交集                                                                           |
| 身份与状态 | 创建返回稳定`id`；查询返回相同 `id` 但不同 `task_id`；queued、processing、completed、failed、未知状态                  |
| 内容       | 携带冻结 Key 下载、Range、Key 轮换后旧任务下载                                                                               |
| 错误       | 未登记非 2xx 和文档包装错误码默认 unknown、发送后断连、轮询失败、超时、重复请求；精确 `403 + feicai_account_required` 必须拒绝并释放 hold |
| 对账       | 验证任务列表、`?task_id=` 和账单端点的鉴权、过滤条件、分页、字段稳定性，以及能否恢复没有上游任务 ID 的 create unknown      |
| 计费       | 预扣、成功结算、失败退款、已发布 size、Provider 账单核对                                                                     |
| 脱敏       | 客户响应不包含上游 Key、渠道、真实模型、源 URL 和内部任务 ID                                                                 |

## 18. 待确认项

以下信息尚未成为已验证事实。第 2～4 项属于未来能力扩展，不阻塞当前已收窄的 v1 代码；其它项
仍是生产发布门禁：

1. Provider 正式 HTTPS Base URL 与 HTTPS 结果下载地址；
2. 1080p/4K 在 16:9、9:16 下的精确 `size`，以及各分辨率其它画幅的精确白名单；
3. `1792x1024` / `1024x1792` 与公开 ratio 的对应及 ×1.667 计费范围；
4. `images[]` 顺序是否表示首帧、尾帧或只是无序参考集合；
5. 飞彩抓取素材所需的最小 source URL TTL；
6. `task_id` 的 Provider 内部语义及其是否具有非身份的对账价值；在证实前忽略该字段，不改变以 `id` 为唯一稳定任务身份的规则；
7. 结果 URL 是否跳转，以及跳转时 Bearer 和同源规则；
8. 人民币 Provider 成本与实际账单的精度、取整、失败扣费和宽幅乘数语义；
9. 文档已出现 `GET /v1/tasks`、`GET /v1/tasks?task_id=` 和账单 usage 端点，需用真实凭据验证其鉴权、过滤精度、分页与字段稳定性；目前仍没有证据证明它们能恢复缺少上游任务 ID、request ID 或幂等键的 create unknown；
10. 除已登记的 `HTTP 403 + feicai_account_required` 外，还有哪些精确
    `HTTP status + Provider error.code` 组合具有“任务确定未创建”语义。

## 19. 剩余发布顺序

1. 向 Provider 取得可调用 API 模型的账户凭据；当前两个 720p 模型均返回 `feicai_account_required`；
2. 取得可验证 HTTPS 合同，确认结果 URL、Bearer 作用域、`task_id` 语义、精确错误合同与对账候选端点；
3. 使用两个 720p Ability 完成真实 Provider 黑盒、媒体、账单、内容代理、脱敏和回滚验收；
4. 取得 1080p/4K 的逐模型 size 证据后提升 implementation 身份，再分别恢复对应映射和 Ability；
5. 管理员根据验收账单批准生产客户售价、billing expression 和预扣上界；
6. 在每个发布环境执行旧实现数据只读审计，再执行单轨渠道/Ability/价格切换；若发现存量任务则暂停并重新决策，不自动转为双轨；
7. 验收后再开放目标分组并归档本文。

上述步骤不包含任何 NEWAPI 原生 OpenAI Videos 代码变更。若未来 NEWAPI 上游原生提供等价的
飞彩能力，按硬约束另行重新评估并优先迁回原生合同，不在当前 Link 实现中预留并行投影。

## 20. 就绪度与根因

720p 工作区代码切换已完成；飞彩整体生产发布就绪度约为 **45/100**。剩余主要根因是：

1. 当前实现尚未形成包含全部新增文件的可复现 Git 基线；
2. 当前 Provider 凭据对两个 720p 模型均返回 `feicai_account_required`，无法创建任务；
3. Provider 尚未提供可发布的 HTTPS 合同；
4. 1080p/4K 的 `size` 组合尚未取得逐模型证据，当前实现主动不登记；
5. 本地测试价格已配置，但客户生产售价与真实 Provider 账单语义尚未批准；
6. 新协议尚未完成成功任务的媒体、账单和内容代理验收。

代码底座与飞彩 720p 南向实现已完成，本地高分辨率 Ability/映射/价格已清理。后续工作包括 Provider 账户授权、size/TTL
取证、发布环境数据切换、管理员价格审批和真实 Provider 验收；这些外部事实不能由代码兼容路径代替。
