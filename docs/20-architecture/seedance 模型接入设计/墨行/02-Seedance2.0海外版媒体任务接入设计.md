---
status: accepted
owner: Dev Team
last-reviewed: 2026-08-10
---

# 墨行 Seedance 2.0 海外版媒体任务接入设计

## 1. 渠道

| 维度 | 设计值 |
| --- | --- |
| Provider 模型 | `seedance-2-0-oversea` |
| Provider origin | `https://www.moxing.pro` |
| ChannelType | `ChannelTypeSeedanceLink` |
| 视频协议 | `media_task_v1` |
| 创建/查询 | `/v1/media/generations` / `/v1/media/tasks/{task_id}` |
| 客户计费 | 独立按量价格 |

该 Channel 使用独立客户模型名，不与 doubao 或历史 Ark 共用模型、Key、价格或素材。

## 2. 已知字段边界

- 至少一个非空文本，转换后 prompt 最长 2500；
- duration 显式 `4..15` 或 `-1`，`-1` 按 15 秒上界预扣；
- resolution 只允许 480p/720p；
- 七种已确认比例；
- 支持文生、单首帧、首尾帧和最多 9 张参考图；
- 当前不开放音频/视频输入、1080p/4K、watermark、last-frame 结果或真人模式；
- `generate_audio=false` 必须保留。

adapter 把 ModelArk content 转为 `prompt`、`input_mode`、`control_mode`、图片字段、
`duration_seconds`、`resolution`、`aspect_ratio` 与 `with_audio`。Provider 私有字段不能从 metadata 或
extra 旁路进入。

## 3. Task 与计费

客户模型确定唯一 Channel 后建立 durable attempt、hold 和冻结快照，只发送一次 Provider POST。
断连、超时、坏 JSON 或缺少可信 task ID 进入视频 unknown，不重试或切到 doubao/Ark。

Task 查询、内容和结算使用冻结 Channel、adapter、Key 和价格。Provider usage 在类型、单位和账单一致
前不参与客户结算。当前认证预检为 401，尚无付费任务证据，因此 Channel/Ability 保持禁用。

## 4. 素材

旧设计的 relay asset 需要按新的一对一 Asset 架构重新验收。验收前配置
`asset_upstream_protocol=none`；不得以旧 AssetSource、binding 或历史 Ark 素材继续履约。

## 5. 不变量

1. 本线路只使用自己的客户模型和 Channel。
2. 1080p、4K、音视频输入和真人素材未开放。
3. 创建只发送一次；unknown 不自动退款。
4. 生产凭据验证完成前不启用。
