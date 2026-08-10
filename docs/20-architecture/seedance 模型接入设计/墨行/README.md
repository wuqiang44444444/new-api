---
status: accepted
owner: Dev Team
last-reviewed: 2026-08-10
---

# 墨行 Seedance 模型接入设计索引

墨行当前有两条 Moxing/TokenSave 模型线路，均使用 `ChannelTypeSeedanceLink` 和 ModelArk V3 北向，
南向共享 `/v1/media/*` 协议形状但必须配置为不同客户模型、不同 Channel 和独立价格：

| Provider 模型 | 设计结论 |
| --- | --- |
| `seedance-2-0-oversea` | 独立客户模型/Channel；按量计费；`media_task_v1` |
| `doubao-seedance-2-0-260128` | 独立客户模型/Channel；按秒计费；`media_task_v1` |
| `dreamina-seedance-2-0-260128` | 历史 Ark 线路；重新取证前不创建新渠道或素材 |

两条当前线路不共享客户模型，不使用 Priority/Weight/Affinity，也不互相 fallback。旧 implementation
字符串只用于迁移期解释历史 Task，不再构成新请求身份。

当前证据：oversea 在 `www.moxing.pro` 与裸域名认证预检均为 401，没有付费创建；doubao 的 4 秒
480p/16:9 已完成直连和本站 ModelArk V3 E2E，4 秒 1080p/16:9 有直连成功，结果为对象且缺少 usage，
MP4 与 Range 成功。另一个 720p 产物为 1280×720，但缺少完整请求快照，不能作为精确合同证据。
素材、`duration=-1`、音频开关和 Provider 账单仍未闭合；两条 Channel/Ability 均应保持禁用。

## 阅读顺序

1. [模型线路与合同边界](01-模型线路与合同边界.md)
2. [Seedance 2.0 海外版媒体任务](02-Seedance2.0海外版媒体任务接入设计.md)
3. [doubao Seedance 2.0 媒体任务](03-doubao-Seedance2.0媒体任务接入设计.md)
4. [墨行素材适配](04-Link资源与墨行素材适配设计.md)
5. [异步任务计费与 Provider 对账](05-异步任务计费与Provider账单对账设计.md)
6. [上线证据与验证矩阵](06-发布门禁与验证矩阵.md)

## 上位架构

- [Seedance 统一北向合同架构](../../Seedance统一北向合同架构.md)
- [Link 视频服务合同与异步任务架构](../../Link视频服务合同与异步任务架构.md)
- [Link 素材库与确定性解析架构](../../Link资源合同与解析架构.md)
