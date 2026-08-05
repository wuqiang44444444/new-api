---
status: accepted
owner: Dev Team
last-reviewed: 2026-08-05
---

# 墨行 Seedance 2.0 海外版媒体任务接入设计

## 1. 边界与权威事实

本文只定义 `seedance-2-0-oversea` 的 Link 视频履约设计。客户合同、共享 Task、create attempt、
内容代理和结算状态机分别以上位 Link 架构为权威；本文负责冻结该模型的 capability、Provider
implementation、请求转换、素材范围和计费差异。

`seedance-2-0-oversea` 与 `doubao-seedance-2-0-260128` 是两个独立 Link SKU。协议形状相似不允许
共享 publication、implementation、execution binding、凭据、素材 binding、价格或历史 Task。

## 2. 模型与实现身份

| 维度 | 设计值 |
| --- | --- |
| Provider 模型 / Link SKU | `seedance-2-0-oversea` |
| Provider origin | `https://www.moxing.pro` |
| implementation | `moxing.seedance-media-task@v2` |
| capability version | `moxing-media-task-v2` |
| 客户接入合同 / route family | `modelark.contents.generations.v3` / `modelark_video` |
| video / asset profile | `third_party_relay` / `relay_assets` |
| create / query | `/v1/media/generations` / `/v1/media/tasks/{task_id}` |
| adapter | `54:third_party_relay:v2` |
| Task / billing | `shared_video_task` / `newapi_quota` |

Provider 模型由 `model_mapping` 得到，并由 execution binding 精确复检到本 SKU。客户模型名即使与
Provider 模型同名，也不能替代 publication。

## 3. 公开 capability

公开能力只取官方模型页、类型化 ModelArk 合同和当前 v2 implementation 的共同交集：

| 能力 | 当前合同 |
| --- | --- |
| 场景 | 文生视频、单图首帧、首尾帧、参考图生视频 |
| 文本 | 至少一个非空 `content[type=text]`；转换后 prompt 最长 2500 字符 |
| 时长 | 必须显式传入 `4..15` 或 `-1`；`-1` 表示自动时长 |
| 分辨率 | 必须显式传入 `480p` 或 `720p` |
| 比例 | 必须显式传入 `16:9`、`4:3`、`1:1`、`3:4`、`9:16`、`21:9` 或 `adaptive` |
| 图片 | 最多 9 张；首帧和尾帧各最多 1 张；参考图模式与首尾帧模式互斥 |
| 音频/视频输入 | 不发布 |
| 生成音频 | 支持 `generate_audio`，显式 `false` 必须保留 |
| 水印 | Provider 有字段，但当前公开合同不发布 |
| 生命周期 | 创建、查询和本站内容代理；不发布取消、删除和 last-frame |
| Link 资源 | 仅 general 图片 `asset://ast_*`；不与请求级媒体混用 |

价格表中的 1080p 和视频输入档不扩张 capability。营销描述中的视频、音频参考、编辑或延长也不能
替代可执行字段合同。

## 4. 请求转换

Resolver 完成 Link 资源解析后，converter 按下表构造 V2 请求：

| ModelArk 输入 | `seedance-2-0-oversea` V2 输出 | 约束 |
| --- | --- | --- |
| 映射后的模型 | `model` | 必须为 `seedance-2-0-oversea` |
| `content[type=text]` | `prompt` | 按输入顺序换行拼接，总长不超过 2500 |
| 无图片 | `input_mode=text`、`control_mode=none` | 文生视频 |
| 单首帧 | `input_mode=single_image`、`control_mode=none`、`image` | role 为空或 `first_frame` |
| 首帧 + 尾帧 | `input_mode=multi_image`、`control_mode=end_frame`、`image/end_image` | 各最多一张 |
| `reference_image` | `input_mode=multi_image`、`control_mode=reference`、`reference_images[]` | 保序，不能混入首尾帧模式 |
| `duration` | `duration_seconds` | 保留 `-1`，不使用 Provider 默认值 |
| `resolution` | `resolution` | 只允许 480p/720p |
| `ratio` | `aspect_ratio` | 必须来自已登记值域 |
| `generate_audio` | `with_audio` | 保留显式 false |

出站固定补充 `capability=video_generation`。客户不能直接透传 `capability`、`input_mode`、
`control_mode`、`duration_seconds`、`aspect_ratio`、`image`、`end_image` 或 `reference_images`，也不能
经 `metadata/extra` 旁路注入 Provider 私有字段。

## 5. 素材与凭据作用域

implementation 只支持 `upstream_binding` 模式的 general 图片，单请求最多 9 张。Resolver 在选渠前
和发送前复检 Asset 所有者、App、publication、状态、implementation、Channel、凭据指纹和 TTL，
再把 `asset://ast_*` 改写为当前 Moxing 账号作用域内的 Provider 引用。

Key、Base URL、implementation 或 profile 变化后，旧 binding 失败关闭。Provider 文档出现“人像库”
或 `asset://` 不会自动创建平台 `real_person` 合同。

## 6. Task、内容与计费

发送前必须先原子提交 TaskCreateAttempt、资金 hold 与 `sending`。发送后断连、超时、非可信响应或
缺少可信任务 ID 进入 create unknown，不自动重发、换渠道或退款。

Provider 的 `queued/running/succeeded/failed` 分别归一到共享 Task；未知状态、任务 ID 不匹配、
成功无可信结果或无效结果结构进入有界对账。公开 Task ID、内容地址和错误均不得暴露 Provider task
ID、Key、签名 URL 或原始响应。

本 SKU 按冻结的 token 上界表达式计费；`duration=-1` 按 15 秒上界预扣。Provider `usage` 在类型、
单位和账单一致性完成生产取证前不参与结算，成功任务按创建时冻结的计费合同处理。Provider 对账
只产生管理员成本证据，不覆盖客户 quota。

## 7. 不变量

1. 只有 `moxing.seedance-media-task@v2` 能履约本 SKU。
2. 1080p、4K、视频输入、音频输入、watermark 和 real_person 均不在当前合同内。
3. Converter 只消费完成 capability 校验和 Resolver 解析的类型化请求。
4. 创建未知不自动重发；轮询不可采信不直接判业务失败。
5. Task 查询、内容回源和结算始终使用创建时冻结的连接、凭据、实现和价格事实。
6. 本 SKU 不得降级到 doubao、Ark 或 NEWAPI 原生视频语义。
