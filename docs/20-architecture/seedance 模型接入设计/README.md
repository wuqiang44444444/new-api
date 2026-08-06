---
status: accepted
owner: Dev Team
last-reviewed: 2026-08-06
---

# Seedance 模型接入设计索引

本目录按 Provider 收纳 Seedance 模型接入 new-api 的目标架构。各 Provider 可以使用不同路径、鉴权、任务信封和素材方式，但都必须服从 Link 服务合同、客户模型 publication、不可变 implementation、共享异步 Task、NEWAPI quota 计费和 Link 资源治理边界。

## Provider 设计

| Provider | 设计入口 | 当前状态 |
| --- | --- | --- |
| FunCloud | [国内 Seedance 2.0 模型接入设计](funcloud/FunCloud国内Seedance-2.0模型接入设计.md) | Standard 三档文本与 Fast 文本/参考视频/图片音频/Link Asset 已有真实成功证据；仅 `randy` 验收组可用，Standard 媒体稳定性和 Provider 账单仍阻断生产开放 |
| 墨行 | [Seedance 模型接入设计](墨行/README.md) | relay v2 已与历史 Ark 隔离；TokenSave 480p/1080p 直连成功且 480p 完成本站 E2E，另有请求快照不完整的 720p 成功产物；oversea 凭据预检 401；两条渠道与 Ability 均禁用 |
| 飞彩 | [Seedance 全模型接入设计](飞彩/README.md) | 单轨 v2 与 10 个 SKU 已结构登记；6 个模型直连生成成功，3 个模型完成本站 E2E 并进入精确 16:9 size registry；渠道与 Ability 禁用，任务级 Provider 成本未闭合 |

逐模型脱敏返回值、账务勾稽和当前缺口集中记录在
[Seedance 多 Provider 真实验证矩阵](../../50-planning/Seedance多Provider真实验证矩阵.md)。架构文档只保留
稳定边界和证据结论，不承载逐次调用流水。

`status: accepted` 表示设计边界已确定，不表示代码、渠道或生产 Ability 已开放。运行时权威仍是代码注册表、publication、Channel/Ability、价格、exposure 策略及真实 Provider 验收结果。

## 共同上位架构

- [Link 服务合同概念与协作关系](../Link服务合同概念与协作关系.md)
- [Link 服务合同注册与履约架构](../Link服务合同注册与履约架构.md)
- [Link 资源合同与解析架构](../Link资源合同与解析架构.md)
- [Link 视频服务合同与异步任务架构](../Link视频服务合同与异步任务架构.md)
- [API Key 用量账单架构](../API-Key用量账单架构.md)
