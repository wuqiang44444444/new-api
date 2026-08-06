---
status: current
owner: Dev Team
last-reviewed: 2026-08-06
evidence-window: "2026-08-05—2026-08-06"
---

# Seedance 多 Provider 真实验证矩阵

## 1. 目的与证据边界

本文集中记录 2026-08-05 至 2026-08-06 已实际发生的 Seedance Provider 黑盒和本站 ModelArk v3
端到端验证，回答
“哪些模型真实调用过、实际返回了什么、当前还缺什么”。架构与合同仍以
[Seedance 模型接入设计](../20-architecture/seedance%20模型接入设计/README.md)及其 Provider 专题为权威；
本文只承载当前验证状态，不用单次成功横向推导其它模型、画幅、媒体或账单语义。

四份同日源报告存在时间先后：总报告记录的是飞彩公网黑盒继续执行前的阶段状态，飞彩专项报告记录了
随后获得费用授权后的真实调用和三个本站 E2E。冲突处以 Provider 专项报告的较晚证据为准。所有返回值
均为脱敏后的可观察字段；不保存 Provider task ID、request ID、完整结果 URL、凭据或原始响应。

2026-08-06 又对飞彩 Fast 720p 与 Standard 4K 执行了隔离 Provider 补证，并按精确
implementation、Provider 模型、resolution、ratio 登记 `feicai-prod-2026-08-06-r2`。该补证及随后
渠道 51 的发布结果晚于上述专项报告；涉及这两个模型的 size、内容和发布状态时，以本文新增的 r2 记录为准。

## 2. 真实验证总表

| Provider / 范围 | 实际验证 | 已验证结果 | 账务与持久化 | 当前结论 |
| --- | --- | --- | --- | --- |
| FunCloud Standard / Fast | 通过本站客户入口持续创建、轮询、内容代理和负向参数验证 | 当前数据库共 21 个真实 Task，13 成功、8 失败；Standard 三档文本成功，Fast 文本及三类媒体链成功 | 21 个 attempt 均 `complete/transferred`；成功均 `SUCCESS/settled`，失败均 `FAILURE/settled/quota=0`；毛消费 4,762,140、退款 2,082,824、净额 2,679,316 quota | 仅 `randy` 验收组可用；Standard 媒体稳定性、Provider 账单和确定拒绝未闭合 |
| Moxing oversea | 对 `www.moxing.pro` 与裸域名执行认证预检 | 两处均 HTTP 401，未进入字段校验 | 未创建付费任务，本轮费用 ¥0 | 凭据阻断；Channel/Ability 禁用，无 publication |
| TokenSave doubao | Provider 直连 480p、1080p 文生及本站 480p E2E；另有一份文件名标识 720p、但未冻结请求参数的成功产物 | 三档产物均为对象型 `result`、`usage=null`，MP4 与 Range 可用；480p/1080p 精确请求证据分别输出 864×496、1920×1080 | 480p E2E attempt `complete/transferred`，Task `succeeded/settled`，预扣与结算均 135,800 quota；直连费用仅有保守上界，不是 Provider 账单 | publication 保留但 Channel/Ability 禁用；素材、智能时长、音频、720p 请求溯源、720p/1080p 本站 E2E 与 Provider 账单未闭合 |
| 飞彩 10 模型直连 | 正式 HTTPS Base URL，首次矩阵加受控复测 | 6 个精确模型生成同源 HTTPS MP4；SD2 为 create 503 unknown；value 1080p、value 4K、Pro PI 未成功 | 直连窗口账户 `total_usage` 增加 5,688 分，即 ¥56.88；只能作为账户聚合变化，不能拆为逐任务成本 | 逐模型证据不得互用；`/v1/tasks` 404，任务级成本与 create-unknown 恢复未闭合 |
| 飞彩本站 E2E 与 r2 发布补证 | Mini 720p、Standard 720p、Standard 1080p 通过本站客户入口；Fast 720p、Standard 4K 通过隔离 Provider 创建、轮询、同源内容下载和媒体探测 | 三条本站 Task 均 `queued -> running -> succeeded`；Fast 两次 4 秒任务均 `completed` 并输出 1280×720；Standard 4K 两次 4 秒任务均 `completed`，完整探测输出 3840×2160、4.016667 秒 | 三条本站 Task 均 `SUCCESS/settled`、attempt 均 `complete/transferred`，客户 quota 合计 865,715；Fast 隔离窗口各增加 180 分，Standard 4K 各增加 1,120 分；这些账户增量不是逐任务正式账单 | 五个 `16:9` 精确条目已进入 size registry；渠道 51 已创建/核对 5 条 publication、启用 5 条 Ability，渠道 52 继续禁用 |
| 飞彩渠道 52 r3 最小矩阵 | SD2 11 秒加一张公共中性图；其余四个模型使用各自最小时长；统一请求 `size=1280x720` | 仅 Value 720p completed，真实 MP4 为 1256×720、4.062993 秒；SD2/Pro PI create 503 unknown；Value 1080p/4K 终态 failed | 隔离 value 账户窗口增加 144 分，不是任务级账单；收窄发布后的客户 E2E 一次失败并全额退款、一次成功并 settled 177,143 quota | 新增第六个 `16:9` 精确条目；渠道 52 仅发布 Value 720p；其余四模型移入禁用渠道 55，0 publication、0 enabled Ability |

## 3. 当前发布状态与剩余证据

| 范围 | 当前运行姿态 | 下一项不可替代证据 |
| --- | --- | --- |
| FunCloud Standard | `randy` 验收组 | 中性图片、图片加音频、参考视频的稳定成功；逐笔 Provider 账单与错误合同 |
| FunCloud Fast | `randy` 验收组 | Provider 账单、结果 URL TTL/CDN 跳转、确定拒绝和未发布字段 |
| Moxing oversea | 禁用 | 有效模型 Key、最小成功任务、内容与 Provider 对账 |
| TokenSave doubao | 禁用，publication 保留 | 图片模式、`duration=-1`、音频开关、失败/exposure、720p 请求溯源、720p/1080p 本站 E2E 与实际账单 |
| 飞彩渠道 51 五个 VIP 模型 | 已发布并启用；只开放各自已登记的 `16:9` 组合 | 逐任务 Provider 成本、usage 口径、其它 ratio、媒体组合与更长时长仍须逐模型验证；不得扩大当前合同 |
| 飞彩渠道 52 Value 720p | 已收窄发布并启用；只开放 `720p / 16:9` | 逐任务 Provider 成本、其它 ratio、媒体组合和更长时长 |
| 飞彩渠道 55 四个隔离模型 | manually disabled；4 条 Ability 均禁用；无 publication | 各自独立的成功内容、size、媒体与账单证据；失败或 unknown 不能由 Value 720p 成功补足 |

## 4. 追溯来源

以下历史报告只用于追溯本矩阵的证据来源，不再作为当前架构权威：

- [Seedance 多 Provider 单轨接入实施与验证结果报告](../99-archive/2026/08/2026-08-05-Seedance多Provider单轨接入实施与验证结果报告.md)
- [FunCloud 国内 Seedance 模型接入实施方案与结果报告](../99-archive/2026/08/2026-08-05-FunCloud国内Seedance模型接入实施方案与结果报告.md)
- [墨行双 Seedance 模型实施与实测结果报告](../99-archive/2026/08/2026-08-05-墨行双Seedance模型实施与实测结果报告.md)
- [飞彩 Seedance 全模型 v2 实施方案与结果报告](../99-archive/2026/08/2026-08-05-飞彩Seedance全模型v2实施方案与结果报告.md)

## 5. 每个模型的真实返回值

| Provider / implementation | Link SKU / Provider 模型 | 实测场景 | 创建与终态返回 | 结果与内容返回 | 账务/解释边界 |
| --- | --- | --- | --- | --- | --- |
| FunCloud / `funcloud.seedance-json@v1` | `seedance-2.0-standard` | 文本，4 秒，480p/720p/1080p | 三档均取得可信 task ID 并成功终态 | 本站内容投影成功；已验证 Range `206 video/mp4` | 最终 quota 分别 118,146 / 254,910 / 635,274；只证明当前客户价格执行正确 |
| FunCloud / `funcloud.seedance-json@v1` | `seedance-2.0-standard` | 图片、图片加音频、参考视频，4 秒与 5 秒 | 多次创建后进入可信 Provider 失败 | 无可交付内容；失败类型包含内部生成、下载链路和内容/隐私拒绝 | 客户 quota 归零并 settled；不等于 Provider 未扣费 |
| FunCloud / `funcloud.seedance-json@v1` | `seedance-2.0-fast` | 文本，4 秒，480p/720p | 两档均取得可信 task ID 并成功终态 | 本站内容投影成功；Range `206 video/mp4` | 最终 quota 分别 118,146 / 254,910 |
| FunCloud / `funcloud.seedance-json@v1` | `seedance-2.0-fast` | 参考视频，4 秒，720p | 成功终态 | Range 206 | 最终 quota 278,400 |
| FunCloud / `funcloud.seedance-json@v1` | `seedance-2.0-fast` | 中性图片加音频，4 秒，480p | 成功终态 | Range 206 | settled；供应商账单未核对 |
| FunCloud / `funcloud.seedance-json@v1` | `seedance-2.0-fast` | `asset://` 图片，4 秒，480p | Asset `ready`，任务成功终态 | publication/implementation/Asset 快照一致；Range 206 | settled；只证明 `source_url` 链路 |
| FunCloud / `funcloud.seedance-json@v1` | `seedance-2.0-fast` | 含真人特征样例图片加音频 | Provider 内容/隐私拒绝 | 无可交付内容 | 客户 quota 归零并 settled；不形成真人能力 |
| Moxing / `moxing.seedance-media-task@v2` | `seedance-2-0-oversea` | `www` 与裸域名空请求认证预检 | 两处均 HTTP 401；未进入预期字段校验 400 | 无 Task、无内容 | 真实付费任务 0，费用 ¥0；凭据无效，不能换 Provider 代测 |
| Moxing / `tokensave.seedance-media-task@v2` | `doubao-seedance-2-0-260128` | Provider 直连，4 秒，480p，16:9 文生 | 预检 HTTP 400；创建 HTTP 200；52 次轮询后 `succeeded` | `result` 为对象，`usage` 缺失；内容 200 `video/mp4`、421,555 B；Range 206；864×496 | 闭合基础文生结果形状，不证明 480p 固定像素或 Provider 账单 |
| Moxing / `tokensave.seedance-media-task@v2` | `doubao-seedance-2-0-260128` | 文件名标识 720p 的 Provider 直连产物；报告未冻结请求参数 | 预检 HTTP 400；创建 HTTP 200；74 次轮询后 `succeeded` | `result` 为对象，`usage=null`；内容 200 `video/mp4`、1,255,662 B；Range 206；1280×720 | `passed=true`、费用保守上界 ¥5.12；因请求分辨率/时长/画幅字段为空，只作为补充产物证据，不登记精确合同 |
| Moxing / `tokensave.seedance-media-task@v2` | `doubao-seedance-2-0-260128` | Provider 直连，4 秒，1080p，16:9 文生 | 预检 HTTP 400；创建 HTTP 200；89 次轮询后 `succeeded` | `result` 为对象，`usage=null`；内容 200 `video/mp4`、3,239,140 B；Range 206；1920×1080；音频流 0 | `passed=true`、费用保守上界 ¥12.77；尚无本站 E2E 和 Provider 实际账单 |
| Moxing / `tokensave.seedance-media-task@v2` | `doubao-seedance-2-0-260128` | 本站 E2E，同规格无音频文生 | 本站创建 HTTP 200；66 次轮询后 `succeeded`；attempt `complete/transferred` | 本站内容 200、484,028 B；864×496、约 4.04 秒、无音频流 | 预扣=结算=135,800 quota，Task `settled` |
| 飞彩 / `feicai.seedance-videos@v2` | `seedance-2.0-mini-720p` / `seedance-2.0-vip-720p-mini-azhw-feicai` | 4 秒、720p、16:9 文生 | 直连 200 / `completed`；本站 `queued -> running -> succeeded` | 1280×720；E2E 3,875,608 B、4.096009 秒；内容 200、Range 206/1024 B | 客户 quota 157,143；该精确组合已登记 registry，逐任务 Provider 成本未知 |
| 飞彩 / `feicai.seedance-videos@v2` | `seedance-2.0-sd2-720p` / `seedance2.0-sd2-feicai` | 11 秒、1 张公共中性图、16:9 | 两次独立矩阵均创建 HTTP 503，outcome `unknown` | 无内容 | 不自动重试、换渠道或退款；已移入禁用渠道 55 |
| 飞彩 / `feicai.seedance-videos@v2` | `seedance-2.0-fast-720p` / `seedance-2.0-vip-720p-fast-azhw-feicai` | 4 秒、720p、16:9 文生 | 早期矩阵首次 `failed`；其后两次隔离复测均 `completed` | MP4，1280×720；最近一次完整探测约 4.086712 秒 | 两个隔离窗口 usage 各 +180 分；精确组合已登记 `feicai-prod-2026-08-06-r2` 并随渠道 51 发布。研究估算 240 分与实测口径不一致，不能反推价格 |
| 飞彩 / `feicai.seedance-videos@v2` | `seedance-2.0-value-720p` / `seedance-2.0-933-720p-azhw-feicai` | 请求 `size=1280x720`，4 秒、16:9 | 最近一次 create 200 / queued，30 次轮询后 `completed` | MP4，4,032,300 B、1256×720、4.062993 秒 | 已登记 `feicai-prod-2026-08-06-r3` 并随收窄后的渠道 52 发布；请求 size 与最终编码像素仍是两个事实 |
| 飞彩 / `feicai.seedance-videos@v2` | `seedance-2.0-standard-720p` / `seedance-2.0-vip-720p-azhw-feicai` | 4 秒、720p、16:9 文生 | 直连 200 / `completed`；本站 `queued -> running -> succeeded` | 1280×720；E2E 4,556,997 B、4.086712 秒；内容 200、Range 206/1024 B | 客户 quota 211,429；该精确组合已登记 registry，逐任务 Provider 成本未知 |
| 飞彩 / `feicai.seedance-videos@v2` | `seedance-2.0-value-1080p` / `seedance-2.0-933-1080p-azhw-feicai` | 4 秒、1080p、16:9 文生 | 三次创建 200，终态均 `failed` | 无内容 | 稳定失败，已移入禁用渠道 55，不能借用 Standard 1080p 证据 |
| 飞彩 / `feicai.seedance-videos@v2` | `seedance-2.0-standard-1080p` / `seedance-2.0-vip-1080p-azhw-feicai` | 请求 `size=1280x720`，4 秒、1080p、16:9 | 直连 200 / `completed`；本站 `queued -> running -> succeeded` | 1920×1080；E2E 7,353,559 B、4.086712 秒；内容 200、Range 206/1024 B | 客户 quota 497,143；精确组合已登记；一次 DNS 超时未误判失败 |
| 飞彩 / `feicai.seedance-videos@v2` | `seedance-2.0-value-4k` / `seedance-2.0-933-4k-azhw-feicai` | 4 秒、4K、16:9 文生 | 三次创建 200，终态均 `failed` | 无内容 | 已移入禁用渠道 55，不能借用 Standard 4K 证据 |
| 飞彩 / `feicai.seedance-videos@v2` | `seedance-2.0-standard-4k` / `seedance-2.0-vip-4k-azhw-feicai` | 请求 `size=1280x720`，4 秒、4K、16:9 文生 | 两次隔离任务均 200 / `completed`；首次验证器在终态成功后因内容下载超过本地 30 秒超时而整体报错，该报错不是 Provider 失败 | 修正验证器内容下载超时后完成真实 MP4 检查：29,307,473 B、3840×2160、4.016667 秒 | 两个隔离窗口 usage 各 +1,120 分；精确组合已登记 `feicai-prod-2026-08-06-r2` 并随渠道 51 发布；仍无任务级正式 Provider 账单 |
| 飞彩 / `feicai.seedance-videos@v2` | `seedance-2.0-pro-pi-720p` / `seedance-933-pro-pi-feicai` | 固定 15 秒最小矩阵 | 早期任务曾进入 `in_progress` 后 failed；最近一次 create HTTP 503 unknown | 无内容 | 已移入禁用渠道 55；按次账单和成功内容未闭合 |

## 6. 可查询到的原始响应字段投影

本节来自当前 `one-api.db` 与本机仍存在的脱敏验证 JSON。为遵守安全边界，只记录原始字段名、非敏感
枚举/数值以及敏感字段是否存在；`id`、`task_id`、request ID、完整 URL、原始错误 message、时间戳和
响应正文不写入文档。这里的“原始”表示从持久化响应或验证产物直接读取，而不是根据设计文档推断。

### 6.1 FunCloud 原始字段

| 场景 | 创建响应安全投影 | 轮询/错误响应安全投影 | 当前数据库复核 |
| --- | --- | --- | --- |
| Standard 文本 480p/720p/1080p | `id=<present>` | `status="succeeded"`、`model="seedance-2.0-standard"`、`service_tier="default"`、`content.video_url=<present>` | 成功 5 条；最终 quota 为 118,146×3、254,910×1、635,274×1 |
| Fast 文本与成功媒体 | `id=<present>` | `status="succeeded"`、`model="seedance-2.0-fast"`、`service_tier="default"`、`content.video_url=<present>` | 成功 8 条；最终 quota 分布为 118,146×4、135,800×1、254,910×1、278,400×1、293,000×1 |
| Standard 媒体失败 | `id=<present>` | 北向投影为 `status="failed"`、`error.code="generation_failed"` | 冻结响应 code：`InternalServiceError`×6、`TASK_FAILED`×1；7 条均 `quota=0/settled` |
| Fast 含真人特征样例失败 | `id=<present>` | 北向投影为 `status="failed"`、`error.code="generation_failed"` | 冻结响应 code：`InputImageSensitiveContentDetected.PrivacyInformation`×1；`quota=0/settled` |
| Fast 1080p 负向参数 | 无任务 ID | `error.code="unsupported_parameter"`、request ID 存在 | attempt 数不增加，Provider POST 前拒绝 |
| 非法图片 role | 无任务 ID | `error.code="invalid_request"`、request ID 存在 | attempt 数不增加，Provider POST 前拒绝 |
| 不安全 Asset URL | 无任务 ID | `error.code="unsafe_asset_url"`、`error.type="asset_error"`、request ID 存在 | Asset 安全边界拒绝，不形成 Task |

原始报告快照统计为 20 个 FunCloud Task（12 成功、8 失败）；2026-08-06 重新查询当前数据库时发现
又增加 1 个 Fast 成功 Task，故当前累计为 21 个 Task（13 成功、8 失败）。新增记录解释了毛消费与
净额分别比历史快照增加 293,000 quota，退款总额未变化。

### 6.2 TokenSave 脱敏验证产物原始字段

| 产物 | 请求字段 | 原始返回字段 | 内容探测 | 费用字段 |
| --- | --- | --- | --- | --- |
| 480p 验证产物 | 旧版产物未写入 `requested_*`；原实施记录冻结为 4 秒、480p、16:9 | `auth_preflight_http_status=400`、`create_http_status=200`、`create_request_id_captured=true`、`poll_count=52`、`terminal_status="succeeded"`、`result_type="object"`、`usage_type="null"`、`passed=true` | `content_http_status=200`、`content_type="video/mp4"`、`content_length=421555`、`range_http_status=206`、`video_width=864`、`video_height=496` | `estimated_spend_upper_cny=2.38` |
| 720p 命名产物 | `requested_duration/resolution/ratio=null`，不能仅凭文件名当成精确请求快照 | `auth_preflight_http_status=400`、`create_http_status=200`、`create_request_id_captured=true`、`poll_count=74`、`terminal_status="succeeded"`、`result_type="object"`、`usage_type="null"`、`passed=true` | `content_http_status=200`、`content_type="video/mp4"`、`content_length=1255662`、`range_http_status=206`、`video_width=1280`、`video_height=720` | `estimated_spend_upper_cny=5.12` |
| 1080p 验证产物 | `requested_duration=4`、`requested_resolution="1080p"`、`requested_ratio="16:9"` | `auth_preflight_http_status=400`、`create_http_status=200`、`create_request_id_captured=true`、`poll_count=89`、`terminal_status="succeeded"`、`result_type="object"`、`usage_type="null"`、`passed=true` | `content_http_status=200`、`content_type="video/mp4"`、`content_length=3239140`、`range_http_status=206`、`video_width=1920`、`video_height=1080`、`audio_stream_count=0` | `estimated_spend_upper_cny=12.77` |

`estimated_spend_upper_cny` 是验证器保守费用上界，不是 Provider 实际账单。720p 产物缺少请求字段，不能
据此登记 publication、capability 或价格；1080p 已证明该精确直连请求的生成和内容链，但仍未闭合本站
E2E、失败路径与账单。

### 6.3 本站持久化返回字段

| 模型 | `Task.data.status` | `Task.data.content.video_url` | attempt 原始事实 | Task / billing |
| --- | --- | --- | --- | --- |
| `doubao-seedance-2-0-260128` | `succeeded` | 存在但不记录值 | `complete/transferred`；Provider request/task ID 均已捕获但不记录值；`adapter_version="54:third_party_relay:v2"` | `SUCCESS/settled`，135,800 quota |
| `seedance-2.0-mini-720p` | `succeeded` | 存在但不记录值 | `complete/transferred`；两类 Provider ID 均已捕获；`adapter_version="54:third_party_json_video_media_arrays:v2"` | `SUCCESS/settled`，157,143 quota |
| `seedance-2.0-standard-720p` | `succeeded` | 存在但不记录值 | 同上 | `SUCCESS/settled`，211,429 quota |
| `seedance-2.0-standard-1080p` | `succeeded` | 存在但不记录值 | 同上 | `SUCCESS/settled`，497,143 quota |

### 6.4 飞彩 r2 补证字段投影

| 模型 | 请求与终态安全投影 | 同源内容探测 | 账户 usage 窗口 | 解释边界 |
| --- | --- | --- | --- | --- |
| `seedance-2.0-fast-720p` | `size="1280x720"`、4 秒、16:9；两次隔离任务均 `completed` | MP4，1280×720；最近一次约 4.086712 秒 | 两次各 +180 分 | 证明该精确组合可登记和发布，不证明其它 ratio、媒体组合、时长或逐任务成本 |
| `seedance-2.0-standard-4k` | `size="1280x720"`、4 秒、16:9；两次隔离任务均 `completed` | 完整检查为 `video/mp4`、29,307,473 B、3840×2160、4.016667 秒 | 两次各 +1,120 分 | Provider 请求 size 与最终编码像素是两个独立事实；usage 只能作为隔离窗口证据 |

Standard 4K 第一次运行的 Provider 创建和轮询已经成功，验证器随后在下载较大 MP4 时触发本地 HTTP
客户端 30 秒超时，因而把“内容检查未完成”汇总为验证失败。该结果不得改写成 Provider 生成失败。
验证器把内容检查超时下限调整为 5 分钟后，第二次运行完成真实视频下载与媒体探测；发布证据采用
Provider 成功终态、实际 MP4 属性和隔离 usage 增量，不采用响应文本或模拟测试替代。

### 6.5 飞彩渠道 52 r3 字段投影

| 模型 | 创建/终态 | 内容投影 | 发布处理 |
| --- | --- | --- | --- |
| SD2 | HTTP 503，unknown | 无 | 不重发、不发布 |
| Value 720p | HTTP 200 / queued，30 次轮询后 completed | HTTP 200 `video/mp4`、4,032,300 B、1256×720、4.062993 秒 | 登记 r3，仅此模型进入渠道 52 |
| Value 1080p | HTTP 200 / queued，2 次轮询后 failed | 无 | 不发布，从渠道候选移除 |
| Value 4K | HTTP 200 / queued，2 次轮询后 failed | 无 | 不发布，从渠道候选移除 |
| Pro PI | HTTP 503，unknown | 无 | 不重发、不发布 |

本轮 value 账户 `total_usage` 从 14,556 增至 14,700。144 分是五模型隔离窗口聚合变化；由于
`/v1/tasks` 404，不能把它写成 Value 720p 的正式逐任务成本。验证器估算费用上限 ¥40.37 也不是账单。

### 6.6 未能恢复的原始返回

当前工作区没有找到飞彩早期 10 模型直连矩阵的独立 JSON/数据库原始响应文件，也没有找到 Moxing
oversea HTTP 401 的完整脱敏响应产物；因此不补写这两部分的原始 body。飞彩早期逐模型直连仍只采用
专项报告中已经脱敏记录的 HTTP/终态、像素与账户 usage 结果；Fast/Standard 4K 的 r2 补证采用本次
验证运行的安全字段投影；Moxing oversea 仍只采用双域名预检 HTTP 401 结论。

## 7. 渠道 51 发布闸门闭合结论

| 闭合项 | 已验证结论 |
| --- | --- |
| 原始阻断 | `seedance-2.0-fast-720p` 缺少精确 Provider ratio/size evidence；同一发布批次中的 Standard 4K 也缺少可登记证据，发布门禁按设计失败关闭 |
| 证据补齐 | Fast 720p 与 Standard 4K 仅为 `16:9` 登记四元 size 证据，evidence version 均为 `feicai-prod-2026-08-06-r2`；未增加全局 fallback，也未借用其它 SKU 证据 |
| capability 与 implementation | 两个 SKU capability 升为 `feicai-media-arrays-v2-r2`，implementation 固定到对应的新 capability hash；其它飞彩 SKU 不受此补证扩张 |
| 主库发布事实 | 渠道 51 状态为启用；5 条 publication 已创建或核对，5 条 Ability 已启用，均绑定渠道 51 的五个 VIP Provider 模型 |
| 客户可见性 | 当前运行环境的 `/v1/models` 已能查询 `seedance-2.0-fast-720p` 与 `seedance-2.0-standard-4k` |
| 验证结果 | 精确 model gate、capability/hash、size resolver 和五模型发布门禁测试通过；全量 Go 编译、文档检查和公开 API 文档校验通过 |

本次闭合没有绕过门禁，也没有把“上游成功但本地内容下载超时”误写成 Provider 失败。发布范围仅为
渠道 51 五个 VIP SKU 已登记的 `16:9` 合同；其它 ratio、未验证媒体组合、时长语义和 Provider
任务级成本继续失败关闭。现有客户计费表达式未因账户 usage 观察值而改写。

## 8. 渠道 52 收窄发布结论

渠道 52 原五模型集合无法整体闭合：四个模型在新的最小矩阵中再次失败或 unknown。系统没有为它们
伪造 ratio，也没有用 Value 720p 或渠道 51 的成功证据横向补足。当前闭合方式是把渠道 52 的 Models、
model mapping 和 Ability 收窄为 `seedance-2.0-value-720p`，并为该精确组合登记
`feicai-prod-2026-08-06-r3`。主库当前为 1 条 publication、1 条启用 Ability；`/v1/models` 仅新增
Value 720p，原其余四个 SKU 不可见。收窄发布后的客户入口最终已取得一条
`queued -> running -> succeeded` Task，内容 URL 存在且结算为 177,143 quota；另一次可信 Provider
终态失败按 `quota=0` 完整退款，证明成功与失败账务路径均按冻结合同完成。

2026-08-06 又把原其余四个客户模型拆入渠道 55 `SD-08 飞彩待验证四模型（隔离）`。渠道 55 为
manually disabled，只有 4 条禁用 Ability，没有 publication；逐模型只读门禁检查仍分别返回“无精确
Provider ratio/size evidence”。该拆分不扩张合同，只让渠道 52 的已验证候选与未验证候选在运营数据上
物理隔离，因此渠道 52 可以继续保持发布，渠道 55 仍禁止启用。
