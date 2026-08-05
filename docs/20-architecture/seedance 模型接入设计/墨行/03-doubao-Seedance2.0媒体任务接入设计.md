---
status: accepted
owner: Dev Team
last-reviewed: 2026-08-05
---

# doubao Seedance 2.0 媒体任务接入设计

## 1. 边界与权威事实

本文只定义 `doubao-seedance-2-0-260128` 的 Link 视频履约设计。客户合同、共享 Task、create attempt、
内容代理和结算状态机分别以上位 Link 架构为权威；本文负责冻结该模型的 capability、Provider
implementation、请求转换、素材范围和按秒计费差异。

当前参考模型页证明 `/v1/media/*` 协议、文生/图生/参考生场景、4–15 秒或自动时长、最高 1080p、
七种比例以及 `reference_images + asset://` 的消费形状。Provider origin、账号、价格和素材管理合同
不能从该模型页反向推断，分别以代码注册、独立 Provider 证据和生产验收为准。

## 2. 模型与实现身份

| 维度 | 设计值 |
| --- | --- |
| Provider 模型 / Link SKU | `doubao-seedance-2-0-260128` |
| Provider origin | `https://tokensave.pro` |
| implementation | `tokensave.seedance-media-task@v2` |
| capability version | `tokensave-media-task-v2` |
| 客户接入合同 / route family | `modelark.contents.generations.v3` / `modelark_video` |
| video / asset profile | `third_party_relay` / `relay_assets` |
| create / query | `/v1/media/generations` / `/v1/media/tasks/{task_id}` |
| adapter | `54:third_party_relay:v2` |
| Task / billing | `shared_video_task` / `newapi_quota` |

Provider 模型由 `model_mapping` 得到，并由 execution binding 精确复检到本 SKU。已废弃的
`tokensave.seedance-media-task@v1` 只能解释完整冻结的历史 Task，不得进入新任务候选。

## 3. 公开 capability

公开能力只取当前模型页、类型化 ModelArk 合同和 TokenSave v2 implementation 的共同交集：

| 能力 | 当前合同 |
| --- | --- |
| 场景 | 文生视频、单图首帧、首尾帧、参考图生视频 |
| 文本 | 至少一个非空 `content[type=text]`；转换后 prompt 最长 2500 字符 |
| 时长 | 必须显式传入 `4..15` 或 `-1`；`-1` 表示自动时长 |
| 分辨率 | 必须显式传入 `480p`、`720p` 或 `1080p` |
| 比例 | 必须显式传入 `16:9`、`4:3`、`1:1`、`3:4`、`9:16`、`21:9` 或 `adaptive` |
| 图片 | 最多 9 张；首帧和尾帧各最多 1 张；参考图模式与首尾帧模式互斥 |
| 音频/视频输入 | 不发布 |
| 生成音频 | 支持 `generate_audio`，显式 `false` 必须保留 |
| 水印 | 当前公开合同不发布 |
| 生命周期 | 创建、查询和本站内容代理；不发布取消、删除和 last-frame |
| Link 资源 | 仅 general 图片 `asset://ast_*`；不与请求级媒体混用 |

模型营销文案提到多模态参考、编辑和延长，但没有形成当前 adapter 可执行的音频/视频字段合同，不能
据此扩张 capability。

## 4. 请求转换

Resolver 完成 Link 资源解析后，converter 按下表构造 V2 请求：

| ModelArk 输入 | TokenSave V2 输出 | 约束 |
| --- | --- | --- |
| 映射后的模型 | `model` | 必须为 `doubao-seedance-2-0-260128` |
| `content[type=text]` | `prompt` | 按输入顺序换行拼接，总长不超过 2500 |
| 无图片 | `input_mode=text`、`control_mode=none` | 文生视频 |
| 单首帧 | `input_mode=single_image`、`control_mode=none`、`image` | role 为空或 `first_frame` |
| 首帧 + 尾帧 | `input_mode=multi_image`、`control_mode=end_frame`、`image/end_image` | 各最多一张 |
| `reference_image` | `input_mode=multi_image`、`control_mode=reference`、`reference_images[]` | 保序，不能混入首尾帧模式 |
| `duration` | `duration_seconds` | 保留 `-1`，不使用 Provider 默认值 |
| `resolution` | `resolution` | 允许 480p/720p/1080p |
| `ratio` | `aspect_ratio` | 必须来自已登记值域 |
| `generate_audio` | `with_audio` | 保留显式 false |

出站固定补充 `capability=video_generation`。客户不能直接透传 `capability`、`input_mode`、
`control_mode`、`duration_seconds`、`aspect_ratio`、`image`、`end_image` 或 `reference_images`，也不能
经 `metadata/extra` 旁路注入 Provider 私有字段。

## 5. 素材与凭据作用域

implementation 只支持 `upstream_binding` 模式的 general 图片，单请求最多 9 张。模型页中的
`reference_images + asset://` 只证明生成请求消费素材引用，不证明素材 CRUD、真人认证或与 Moxing
账号互操作。

Resolver 在选渠前和发送前复检 Asset 所有者、App、publication、状态、implementation、Channel、
凭据指纹和 TTL。TokenSave Key、Base URL、implementation 或 profile 变化后，旧 binding 失败关闭；
Moxing 的 `relay_assets` binding 不得复用。

## 6. Task、内容与计费

发送前必须先原子提交 TaskCreateAttempt、资金 hold 与 `sending`。发送后断连、超时、非可信响应或
缺少可信任务 ID 进入 create unknown，不自动重发、换渠道或退款。

Provider 的 `queued/running/succeeded/failed` 分别归一到共享 Task；未知状态、任务 ID 不匹配、
成功无可信结果或无效结果结构进入有界对账。公开 Task ID、内容地址和错误均不得暴露 Provider task
ID、Key、签名 URL 或原始响应。

2026-08-05 的 4 秒 480p 文生实测返回对象型 `result`、缺失 `usage`，并得到可下载的 MP4 与 Range
响应。因此 v2 adapter 同时接受白名单对象、JSON 字符串对象和直接 HTTPS URL，但不会根据模型页的
字符串描述强制改写真实对象；缺失或字符串型 `usage` 均不投影为 token。该单样本只闭合基础文生结果
形状，不证明其它分辨率、素材场景、智能时长或 Provider 账单。

本 SKU 按冻结的时长、分辨率和场景表达式进行按秒计费；`duration=-1` 按 15 秒上界预扣。文本、
单图/首尾帧和参考图模式必须命中各自已登记档位。Provider `usage` 在真实类型和单位完成生产取证前
不参与结算，也不得改写已经冻结的按秒费用。

## 7. 发布门禁与不变量

1. 只有 `tokensave.seedance-media-task@v2` 能创建本 SKU 的新任务。
2. 1080p 只扩张本 SKU，不得扩张 `seedance-2-0-oversea`。
3. 视频输入、音频输入、watermark 和 real_person 均不在当前合同内。
4. Converter 只消费完成 capability 校验和 Resolver 解析的类型化请求。
5. 创建未知不自动重发；轮询不可采信不直接判业务失败。
6. Task 查询、内容回源和结算始终使用创建时冻结的 TokenSave 连接、凭据、实现和价格事实。
7. 本 SKU 不得降级到 Moxing oversea、Ark 或 NEWAPI 原生视频语义。
