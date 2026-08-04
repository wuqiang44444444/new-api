---
status: accepted
owner: Dev Team
last-reviewed: 2026-08-05
---

# 墨行 Seedance 2.0 海外版媒体任务接入设计

## 1. 模型身份

| 维度 | 设计值 |
| --- | --- |
| Provider 模型 | `seedance-2-0-oversea` |
| Link SKU | `seedance-2-0-oversea` |
| 客户接入合同 | `modelark.contents.generations.v3` |
| route family | `modelark_video` |
| implementation | `moxing.seedance-media-task/<经部署审计确认的版本>` |
| video profile | `third_party_relay` |
| create/query | `/v1/media/generations`、`/v1/media/tasks/{task_id}` |
| asset profile | `relay_assets` |
| Task / billing | `shared_video_task` / `newapi_quota` |

Provider 模型是 execution binding 的判别维度，不要求客户使用该字符串。客户模型 publication 一旦
形成，不随当前渠道集合变化。

## 2. 初始公开 capability

初始公开能力只包含官方资料、现有类型化 ModelArk 合同和当前 relay adapter 可以共同证明的交集：

| 能力 | 初始合同 |
| --- | --- |
| 场景 | 文生视频、单图首帧、首尾帧、参考图生视频 |
| 文本 | 至少一个非空 `content[type=text]`，Provider prompt 最长 2500 字符 |
| 时长 | 显式整数 `4..15` 或 `-1` 智能时长；未验证缺省值前不得省略 |
| 分辨率 | 显式 `480p` 或 `720p` |
| 比例 | `16:9`、`4:3`、`1:1`、`3:4`、`9:16`、`21:9`、`adaptive` |
| 图片 | 最多 9 张候选上限；首帧最多 1、尾帧最多 1；参考图模式与首尾帧模式互斥 |
| 音频/视频输入 | 初版不支持；营销描述和价格维度不能替代 relay 字段与实测证据 |
| 生成音频 | 支持 `generate_audio`，必须保留显式 `false` |
| 水印 | Provider 有 `watermark` 字段，但在显式 true/false/default 黑盒完成前不发布 |
| 生命周期 | 创建、查询、本站内容代理；不发布取消、删除和 last-frame |
| Link 资源 | 支持 general `asset://ast_*`，不允许与请求级媒体混用 |

官方 V2 限制表只确认 `480p/720p`。价格表出现 1080p 项并不能证明当前模型 API 接受 1080p；4K
只出现在历史 Ark 文档。任何 1080p/4K 扩展必须提升 capability/implementation 身份并重新验收。

## 3. 请求转换

类型化 ModelArk 请求在选渠和 Resolver 完成后转换为墨行 V2：

| ModelArk 字段 | 墨行 V2 字段 | 规则 |
| --- | --- | --- |
| 映射后的 Provider 模型 | `model` | 固定为 `seedance-2-0-oversea` |
| `content[type=text]` | `prompt` | 按输入顺序以换行拼接；总长度不超过 2500 |
| 无图片 | `input_mode=text`、`control_mode=none` | 文生视频 |
| 一个首帧 | `input_mode=single_image`、`control_mode=none`、`image` | role 缺省或 `first_frame` |
| 首帧 + 尾帧 | `input_mode=multi_image`、`control_mode=end_frame`、`image/end_image` | 两者各最多一个 |
| `reference_image` | `input_mode=multi_image`、`control_mode=reference`、`reference_images[]` | 保序，不能混入首尾帧模式 |
| `duration` | `duration_seconds` | 指针语义，保留 `-1` |
| `resolution` | `resolution` | 仅 480p/720p |
| `ratio` | `aspect_ratio` | 不依赖 Provider 默认值 |
| `generate_audio` | `with_audio` | 显式 `false` 必须发送 |

出站固定补充 `capability=video_generation`。客户不能直接提交 `capability`、`input_mode`、
`control_mode`、`duration_seconds`、`aspect_ratio`、`image` 或 `reference_images`，也不能经
`metadata/extra` 旁路注入这些 Provider 私有字段。

## 4. 创建前顺序

```text
1. 读取 publication，冻结客户模型、Link SKU 和版本
2. 按 capability 校验字段、文本、时长、分辨率、比例和媒体组合
3. 计算全部 asset:// 引用的候选渠道交集
4. 校验 implementation、exposure、单 Key 与当前凭据作用域
5. NEWAPI 选渠并应用 model_mapping
6. execution binding 复检 provider_model -> 冻结 SKU
7. Resolver 二次校验并改写 Link 资源
8. 构造冻结 billing probe
9. 创建 TaskCreateAttempt，资金 hold 与 sending 原子提交
10. converter 构造 V2 请求并发送
```

所有客户错误必须在资金 hold 和 Provider POST 之前完成。发送后的断连、超时、非可信响应或缺少
任务 ID 进入 create unknown，不能自动换渠道、重复 POST 或立即退款。

## 5. 创建与轮询归一化

创建响应必须得到非空、长度受控且不含控制字符的 `task_id` 或已验证响应别名，并归一为 Task 冻结
的 `upstream_task_id`。公开 Task ID 由 new-api 生成，不向客户暴露墨行任务 ID。

| 墨行状态 | 共享 Task | 处理 |
| --- | --- | --- |
| `queued` | queued | 继续轮询 |
| `running` | running | 继续轮询 |
| `succeeded` | succeeded | 校验结果后结算 |
| `failed` | failed | 脱敏错误并按冻结合同处理 |
| 其它状态 | reconciliation required | 不直接判失败或退款 |

官方资料把 `result` 与 `usage` 描述为字符串，而当前 adapter 期望结构化结果和 token 字段。生产发布
前必须冻结真实 JSON 样例：成功结果 URL 的字段路径、URL 有效期、usage 类型、任务 ID 回显和失败
结构。无效 JSON、任务 ID 不匹配、未知状态或成功无可用结果只说明本次响应不可采信，应进入有界
对账。

## 6. 内容交付

客户只获得本站 Task 内容代理地址。代理按 Task 冻结的 Base URL、Key、implementation、结果地址
和授权回源，不读取渠道当前 Key。完整结果 URL、签名 query、Bearer 和上游任务 ID 不进入普通用户
响应、Task 公共数据、错误、日志或指标。

结果地址必须是绝对 HTTPS，并通过长度、userinfo、SSRF、重定向和允许来源校验。官方资料示例或
素材响应出现 HTTP 时不能降低网关安全要求。

## 7. 合同不变量

1. 墨行 V2 converter 只消费已校验并已解析的类型化请求。
2. 480p/720p 是初始唯一分辨率集合，不能从价格表推导 1080p/4K。
3. 音频/视频输入在 relay adapter 和 capability 同时版本化之前保持拒绝。
4. 缺省值、watermark 和 usage 未验证时不通过兼容猜测开放。
5. 创建未知不自动重发，轮询不可采信不直接判业务失败。
6. 查询和内容代理始终使用 Task 冻结连接与凭据。
