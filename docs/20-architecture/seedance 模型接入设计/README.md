---
status: accepted
owner: Dev Team
last-reviewed: 2026-08-05
---

# Seedance 模型接入设计索引

本目录按 Provider 收纳 Seedance 模型接入 new-api 的目标架构。各 Provider 可以使用不同路径、鉴权、任务信封和素材方式，但都必须服从 Link 服务合同、客户模型 publication、不可变 implementation、共享异步 Task、NEWAPI quota 计费和 Link 资源治理边界。

## Provider 设计

| Provider | 设计入口 | 当前状态 |
| --- | --- | --- |
| FunCloud | [国内 Seedance 2.0 模型接入设计](funcloud/FunCloud国内Seedance-2.0模型接入设计.md) | implementation 与 Standard/Fast 已登记，仅 `randy` 验收分组发布 |
| 墨行 | [Seedance 模型接入设计](墨行/README.md) | relay v2 已与历史 Ark 隔离；渠道禁用，结果、usage 与 Provider 对账门禁未闭合 |
| 飞彩 | [Seedance 全模型接入设计](飞彩/README.md) | 单轨 v2 与 10 个 SKU 已结构登记；size registry 仅登记 3 个已验证的 16:9 组合，渠道与 Ability 禁用 |

`status: accepted` 表示设计边界已确定，不表示代码、渠道或生产 Ability 已开放。运行时权威仍是代码注册表、publication、Channel/Ability、价格、exposure 策略及真实 Provider 验收结果。

## 共同上位架构

- [Link 服务合同概念与协作关系](../Link服务合同概念与协作关系.md)
- [Link 服务合同注册与履约架构](../Link服务合同注册与履约架构.md)
- [Link 资源合同与解析架构](../Link资源合同与解析架构.md)
- [Link 视频服务合同与异步任务架构](../Link视频服务合同与异步任务架构.md)
- [API Key 用量账单架构](../API-Key用量账单架构.md)
