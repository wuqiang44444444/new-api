---
status: accepted
owner: Dev Team
last-reviewed: 2026-08-05
---

# 墨行 Seedance 模型接入设计索引

本目录把 `docs/70-research/墨行/` 中的官方资料收敛为墨行 Seedance 的目标接入设计。设计遵循 Link
服务合同、客户模型 publication、不可变 implementation、共享异步 Task、NEWAPI quota 计费和 Link
资源虚拟素材库边界。

`status: accepted` 表示接入边界已经形成设计，不表示渠道已经启用或通过生产验收。当前代码已存在
`moxing.seedance-media-task/v1` 和 `moxing.seedance-ark-assets/v1` 注册项，但官方资料对两条线路、
模型身份和素材能力作了明确区分；完成本文门禁前不得仅因代码存在而发布 Ability。

## 1. 模型结论

| Provider 模型 | 官方线路 | 设计结论 |
| --- | --- | --- |
| `seedance-2-0-oversea` | V2 `/v1/media/*` | 当前唯一允许继续设计为墨行可发布实现的模型；使用 `third_party_relay + relay_assets` |
| `dreamina-seedance-2-0-260128` | Ark `/v1/ark/*` | 历史官 Key 线路；保留独立规格和重新接入条件，不与当前 V2 SKU 混用 |
| `doubao-seedance-2-0-260128` | TokenSave V2 | 不属于墨行实现；不得映射到墨行 implementation |

客户模型名仍可由管理员自定义。上表中的 Provider 模型只参与 `model_mapping` 与 implementation
execution binding，不直接承担客户模型 publication 身份。

## 2. 官方资料覆盖

| 墨行目录文档 | 在本设计中的用途 |
| --- | --- |
| [Seedream Moxing](<../../../70-research/墨行/Seedream Moxing.md>) | Seedream 图片模型资料；不进入 Seedance 视频合同 |
| [单次请求信息查询](<../../../70-research/墨行/moxing 单次查询.md>) | Provider 单次费用证据、集成 Token 和请求 ID 对账边界 |
| [海外官 Key 真人素材库](<../../../70-research/墨行/moxing-SD-海外官Key 真人素材库.md>) | 历史 Ark 真人认证与素材线路，只服务历史 `dreamina` 边界 |
| [海外素材库](<../../../70-research/墨行/moxing-SD-海外素材库.md>) | 当前 V2 `relay_assets` 的创建、状态、归属和引用合同 |
| [JoyCreator 素材库](<../../../70-research/墨行/moxing-SD-素材库.md>) | JoyCreator 管理 facade；不参与当前视频路由或 Link Resolver |
| [Seedance 2.0 海外官 Key](<../../../70-research/墨行/moxing-Seedance 2.0 海外官 Key.md>) | 历史 `dreamina` Ark 模型合同与隔离规则 |
| [Seedance 2.0 海外版](<../../../70-research/墨行/moxing-Seedance 2.0 海外版.md>) | 当前 `seedance-2-0-oversea` V2 模型合同 |

资料中的营销能力、价格表和示例不能单独形成公开 capability。字段默认值、终态 `usage` 结构、
结果结构、失败扣费和素材角色仍需使用目标生产凭据完成黑盒验证。

## 3. 阅读顺序

1. [模型线路与合同边界](01-模型线路与合同边界.md)
2. [Seedance 2.0 海外版媒体任务接入设计](02-Seedance2.0海外版媒体任务接入设计.md)
3. [海外官 Key 历史线路隔离设计](03-海外官Key历史线路隔离设计.md)
4. [Link 资源与墨行素材适配设计](04-Link资源与墨行素材适配设计.md)
5. [异步任务计费与 Provider 账单对账设计](05-异步任务计费与Provider账单对账设计.md)
6. [发布门禁与验证矩阵](06-发布门禁与验证矩阵.md)

## 4. 上位架构

- [Link 服务合同概念与协作关系](../../Link服务合同概念与协作关系.md)
- [Link 服务合同注册与履约架构](../../Link服务合同注册与履约架构.md)
- [Link 视频服务合同与异步任务架构](../../Link视频服务合同与异步任务架构.md)
- [Link 资源合同与解析架构](../../Link资源合同与解析架构.md)
- [API Key 用量账单架构](../../API-Key用量账单架构.md)
