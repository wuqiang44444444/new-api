---
status: accepted
owner: Dev Team
last-reviewed: 2026-08-05
---

# 墨行 Seedance 模型接入设计索引

本目录把 `docs/70-research/墨行/` 中的官方资料收敛为墨行 Seedance 的目标接入设计。设计遵循 Link
服务合同、客户模型 publication、不可变 implementation、共享异步 Task、NEWAPI quota 计费和 Link
资源虚拟素材库边界。

`status: accepted` 表示接入边界已经形成设计，不表示渠道已经启用或通过生产验收。墨行当前产品范围
固定为两个互不降级的 Link SKU：按量计费的 `seedance-2-0-oversea` 与按秒计费的
`doubao-seedance-2-0-260128`。两个 Provider 模型均属于 Moxing 的 TokenSave 模型体系，并共享
`/v1/media/*` 协议形状；方案必须按 Link SKU 及其 capability 命名和展示，不得用 `moxing` /
`tokensave` 把两者伪装成不同 Provider。当前展示名分别为
`Moxing · tokensave.seedance-2-0-oversea/v2` 与
`Moxing · tokensave.doubao-seedance-2-0-260128/v2`。域名、不可变 implementation ID、凭据、价格与
Provider 证据仍必须隔离。完成本文门禁前不得仅因代码存在而发布 Ability。

## 1. 模型结论

| Provider 模型 | 官方线路 | 设计结论 |
| --- | --- | --- |
| `seedance-2-0-oversea` | `seedance-2-0-oversea` Link 方案 | 当前模型之一；`moxing.seedance-media-task@v2`，按量计费 |
| `doubao-seedance-2-0-260128` | `doubao-seedance-2-0-260128` Link 方案 | 当前模型之二；`tokensave.seedance-media-task@v2`，按秒计费 |
| `dreamina-seedance-2-0-260128` | 历史 Ark `/v1/ark/*` | 模型页仍保留旧合同；素材/H5 配套资料已失效，重新取证前不得新增候选或 binding |

客户模型名仍可由管理员自定义。上表中的 Provider 模型只参与 `model_mapping` 与 implementation
execution binding，不直接承担客户模型 publication 身份。

## 2. 官方资料覆盖

| 墨行目录文档 | 在本设计中的用途 |
| --- | --- |
| [Seedream Moxing](<../../../70-research/墨行/Seedream Moxing.md>) | Seedream 图片模型资料；不进入 Seedance 视频合同 |
| [单次请求信息查询](<../../../70-research/墨行/moxing 单次查询.md>) | Provider 单次费用证据、集成 Token 和请求 ID 对账边界 |
| [海外官 Key 真人素材库](<../../../70-research/墨行/moxing-SD-海外官Key 真人素材库.md>) | 文件名未同步，但正文现为当前 `doubao-seedance-2-0-260128` V2 模型页；不再证明 Ark 素材或真人认证 |
| [海外素材库](<../../../70-research/墨行/moxing-SD-海外素材库.md>) | 当前 V2 `relay_assets` 的创建、状态、归属和引用合同 |
| [JoyCreator 素材库](<../../../70-research/墨行/moxing-SD-素材库.md>) | JoyCreator 管理 facade；不参与当前视频路由或 Resolver |
| [Seedance 2.0 海外官 Key](<../../../70-research/墨行/moxing-Seedance 2.0 海外官 Key.md>) | 历史 `dreamina` Ark 模型合同与隔离规则 |
| [Seedance 2.0 海外版](<../../../70-research/墨行/moxing-Seedance 2.0 海外版.md>) | 当前 `seedance-2-0-oversea` V2 模型合同 |

`doubao-seedance-2-0-260128` 的当前补充资料证明文生、图生、参考生，`480p/720p/1080p`、
`4..15/-1`、七种画幅及 `result/usage` 字符串响应口径。该页没有给出 Provider origin、账号、素材
CRUD、真人认证或价格，不能单独证明该 Link 方案的 implementation 身份、按秒计费或
素材授权合同；这些
事实仍分别依赖代码注册、其它 Provider 证据和生产验收。营销文案中的视频/音频参考、编辑和延长
能力未给出本次可执行字段合同，初版仍保持未发布。

资料中的营销能力、价格表和示例不能单独形成公开 capability。字段默认值、终态 `usage` 结构、
结果结构、失败扣费和素材角色仍需使用目标生产凭据完成黑盒验证。

## 3. 阅读顺序

1. [模型线路与合同边界](01-模型线路与合同边界.md)
2. [Seedance 2.0 海外版媒体任务接入设计](02-Seedance2.0海外版媒体任务接入设计.md)
3. [doubao Seedance 2.0 媒体任务接入设计](03-doubao-Seedance2.0媒体任务接入设计.md)
4. [Link 资源与墨行素材适配设计](04-Link资源与墨行素材适配设计.md)
5. [异步任务计费与 Provider 账单对账设计](05-异步任务计费与Provider账单对账设计.md)
6. [发布门禁与验证矩阵](06-发布门禁与验证矩阵.md)

## 4. 上位架构

- [Link 服务合同概念与协作关系](../../Link服务合同概念与协作关系.md)
- [Link 服务合同注册与履约架构](../../Link服务合同注册与履约架构.md)
- [Link 视频服务合同与异步任务架构](../../Link视频服务合同与异步任务架构.md)
- [Link 资源合同与解析架构](../../Link资源合同与解析架构.md)
- [API Key 用量账单架构](../../API-Key用量账单架构.md)
