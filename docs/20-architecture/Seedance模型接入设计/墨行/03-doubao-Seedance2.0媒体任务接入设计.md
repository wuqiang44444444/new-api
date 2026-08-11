---
status: accepted
owner: Dev Team
last-reviewed: 2026-08-11
---

# doubao Seedance 2.0 媒体任务接入设计

## 1. 渠道

| 维度 | 设计值 |
| --- | --- |
| Provider | Moxing / TokenSave 模型体系 |
| Provider 模型 | `doubao-seedance-2-0-260128` |
| ChannelType | `ChannelTypeSeedanceLink` |
| 视频协议 | `media_task_v1` |
| 创建/查询 | `/v1/media/generations` / `/v1/media/tasks/{task_id}` |
| 客户计费 | 独立按秒价格 |

该线路使用自己的客户模型、Channel、Key、价格和素材账号，不与 oversea 或历史 Ark fallback。

## 2. 字段边界

现有资料支持文生、图生和参考图，duration `4..15/-1`、480p/720p/1080p、七种比例以及
`generate_audio`。初始继续要求非空文本、显式 duration/resolution/ratio；营销文案中的视频/音频参考、
编辑和延长未形成可执行字段合同，不开放。

adapter 使用 `model_mapping` 后的精确 Provider 模型，并转换到 `/v1/media/*` 请求。可选 false/0 不得
丢失；Provider 私有字段不得由客户透传。

## 3. 已验证事实

- 4 秒 480p/16:9 文生完成 Provider 直连和本站 ModelArk V3 E2E；
- 4 秒 1080p/16:9 直连成功；
- 两个成功样本均为对象型 result、没有 usage，MP4 下载和 Range 成功；
- 另有 720p 产物 1280×720，但缺少完整请求快照，不能作为精确合同证据；
- 素材、`duration=-1`、音频开关、高分辨率本站 E2E 和 Provider 实际账单仍未闭合。

## 4. Task 与计费

创建只发送一次。发送前建立 durable attempt 和 hold；断连、超时或结果不明进入视频 unknown，不
切换 oversea、Ark 或其它 Provider。

客户按冻结 duration、resolution 和场景的按秒表达式结算，不读取缺失/未验证 usage。`duration=-1`
按 15 秒上界预扣。Task 查询、内容回源和结算始终使用创建时冻结 Channel、adapter、连接和价格。

## 5. 素材

旧 relay asset 资料必须按一对一 Asset/AssetGroup 架构重新验证。在协议、账号和生命周期闭合前使用
`asset_upstream_protocol=none`，不复用历史 binding 或 Ark H5。

## 6. 不变量

1. 本线路不与 oversea 或 Ark 共用客户模型和 Channel。
2. 不使用 Priority/Weight/Affinity/fallback。
3. 未验证 usage 不参与客户计费。
4. 创建只发送一次，unknown 不重试或自动退款。
5. 当前证据不足以视为生产开放。
