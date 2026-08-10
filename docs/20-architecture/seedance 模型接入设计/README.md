---
status: accepted
owner: Dev Team
last-reviewed: 2026-08-10
---

# Seedance 模型接入设计索引

本目录按 Provider 收纳 Seedance 模型接入目标架构。所有线路使用 `ChannelTypeSeedanceLink` 和
ModelArk V3 北向；各 Provider 可以使用不同代码化上游协议、路径、鉴权、任务信封和素材方式。
不同线路必须使用不同客户模型名，每个客户模型只对应一个已启用 Seedance Channel。

## Provider 设计

| Provider | 设计入口 | 当前状态 |
| --- | --- | --- |
| FunCloud | [国内 Seedance 2.0 模型接入设计](funcloud/FunCloud国内Seedance-2.0模型接入设计.md) | Standard/Fast 使用独立客户模型与 Channel；已有部分真实成功证据，素材协议和 Provider 账单仍阻断全面开放 |
| 墨行 | [Seedance 模型接入设计](墨行/README.md) | oversea/doubao 使用独立客户模型与 Channel；doubao 有部分视频证据，oversea 预检 401；素材需按一对一代理重新验收 |
| 飞彩 | [Seedance 全模型接入设计](飞彩/README.md) | v2/v3 及各档位使用独立客户模型；v3 仅 Mini/Fast/Standard 720p 16:9 有精确成功证据，其它组合保持禁用 |

逐模型脱敏返回值、账务勾稽和当前缺口集中记录在
[Seedance 多 Provider 真实验证矩阵](../../50-planning/Seedance多Provider真实验证矩阵.md)。架构文档只保留
稳定边界和证据结论，不承载逐次调用流水。

`status: accepted` 表示设计边界已确定，不表示代码、渠道或生产 Ability 已开放。配置权威是客户模型、
Channel/Ability、`model_mapping`、价格和代码协议；是否兼容与是否上线由技术人员线下验证后通知管理员。
系统不建立 publication、SKU、implementation 或 execution binding 自动门禁。

## 共同上位架构

- [Link 扩展概念与协作关系](../Link服务合同概念与协作关系.md)
- [Link 服务合同注册与履约架构](../Link服务合同注册与履约架构.md)
- [Link 资源合同与解析架构](../Link资源合同与解析架构.md)
- [Link 视频服务合同与异步任务架构](../Link视频服务合同与异步任务架构.md)
- [API Key 用量账单架构](../API-Key用量账单架构.md)
