---
status: accepted
owner: Dev Team
last-reviewed: 2026-08-05
---

# 飞彩 Seedance 全模型接入设计

本目录定义飞彩 10 个 Seedance 模型接入 new-api 的目标架构。飞彩只保留一个活动实现：
`feicai.seedance-videos/v2`。当前代码中的 v1 是尚未生产就绪的旧窄实现，目标落地时整体移除，
不建设 v1/v2 双轨、兼容解析或历史任务 fallback。

## 1. 文档职责与阅读顺序

1. [总体架构与履约设计](飞彩总体架构与履约设计.md)：单轨 v2、请求转换、Task、Link 资源、内容代理以及 v1 移除边界。
2. [全模型 SKU 与计费设计](飞彩全模型SKU与计费设计.md)：10 个模型逐一对应的身份、能力、size、媒体和计费合同。
3. [发布门禁与验证设计](飞彩发布门禁与验证设计.md)：架构级证据门禁、切换条件和发布不变量。

执行层文档：

- [确定性回归规约](../../../30-engineering/飞彩Seedance全模型回归规约.md)：代码注册、请求、Task、素材、内容和计费回归。
- [真实上线验收手册](../../../40-operations/06-飞彩Seedance全模型上线验收手册.md)：隔离分组中的 10 模型黑盒、账单和停流验收。

## 2. 当前事实与目标边界

当前代码只登记两个 v1 720p SKU，并只支持两组全局 size、图片/音频数组以及 media-arrays
adapter v1。该实现没有完成正式 HTTPS、成功任务、生产账单和内容回源证据，因此不作为需要长期兼容的生产合同。

目标采用单轨替换：

```text
删除飞彩 v1 注册与 v1 专用分支
  -> 以相同 implementation ID 登记唯一 v2
  -> 登记 10 个独立 Link SKU capability 与 execution binding
  -> 只允许 media-arrays adapter v2 创建、轮询和内容回源
  -> 逐模型通过证据门禁后发布 Ability
```

单轨替换不表示可以忽略部署数据。任何环境只要存在 v1 渠道配置、publication 依赖、在途 Task、
create attempt、AssetBinding、exposure 或仍需访问的历史内容，就不得部署删除 v1 的版本；必须先完成
停流、终态结算、数据处置和内容保留决策，并证明运行时不再需要 v1。

## 3. 全模型覆盖

| Provider 模型 | 目标 Link SKU | 核心差异 | 当前代码 |
| --- | --- | --- | --- |
| `seedance-2.0-vip-720p-mini-azhw` | `seedance-2.0-mini-720p` | 720p、4–15 秒、按秒 | 未登记 |
| `seedance2.0-sd2` | `seedance-2.0-sd2-720p` | 720p、11–15 秒、图片必填 | 未登记 |
| `seedance-2.0-vip-720p-fast-azhw` | `seedance-2.0-fast-720p` | 720p、4–15 秒、按秒 | 未登记 |
| `seedance-2.0-933-720p-azhw` | `seedance-2.0-value-720p` | 720p、value 档 | v1 窄登记 |
| `seedance-2.0-vip-720p-azhw` | `seedance-2.0-standard-720p` | 720p、standard 档 | v1 窄登记 |
| `seedance-2.0-933-1080p-azhw` | `seedance-2.0-value-1080p` | 1080p、value 档 | 仅有 SKU 标识 |
| `seedance-2.0-vip-1080p-azhw` | `seedance-2.0-standard-1080p` | 1080p、standard 档 | 仅有 SKU 标识 |
| `seedance-2.0-933-4k-azhw` | `seedance-2.0-value-4k` | 4K、value 档 | 仅有 SKU 标识 |
| `seedance-2.0-vip-4k-azhw` | `seedance-2.0-standard-4k` | 4K、standard 档 | 未登记 |
| `seedance-933-pro-pi` | `seedance-2.0-pro-pi-720p` | 720p、固定 15 秒、参考视频、按次 | 未登记 |

“仅有标识”不等于 capability、execution binding、价格或已发布合同。10 个模型必须分别完成代码注册和证据闭环，
不能用任一模型的成功结果替代其它模型。

## 4. 共同边界

- 客户入口保持 ModelArk v3 `POST /api/v3/contents/generations/tasks`，`route_family=modelark_video`。
- 飞彩 `/v1/videos`、`/v1/videos/{id}`、`/v1/tasks` 和 billing 端点只属于 Provider 上游与对账合同。
- 不修改、包装或识别 NEWAPI 原生 OpenAI Videos `/v1/videos`。
- 客户模型、Link SKU、Provider 模型三层分离；Provider 模型只通过 `model_mapping` 和 execution binding 进入上游。
- 客户费用继续使用 NEWAPI quota 与结算日志；飞彩金额只作为 Provider 成本证据，不改写客户历史账单。
- Link 资源继续使用 `asset://ast_*`；飞彩只消费 `source_url`，不建立专用素材表或真人授权体系。
- 研究环境中的明文 HTTP 地址只属于原始资料，不是生产 Base URL；正式渠道必须使用可验证 HTTPS。

## 5. 证据边界

证据优先级为：NEWAPI 与 Link 当前不变量、飞彩正式可版本化合同、目标生产凭据黑盒与实际账单、
2026-08-03 研究资料。研究报价、示例 size、模型列表和明文环境不能替代生产证据。

当前未闭合项包括：SD2 默认时长、六种画幅的逐模型像素映射、宽幅 1.667 倍适用范围、
Pro PI 按次计费与宽幅倍率关系、v2 确定拒绝组合以及 `/v1/tasks` 的稳定对账身份。缺口全部 fail closed。

研究输入：

- [飞彩 Seedance 更新资料](<../../../70-research/飞彩/飞彩-seedance%20更新.md>)
- [飞彩账单与查询资料](../../../70-research/飞彩/飞彩-账单与查询.md)

上位架构见父级 [Seedance 模型接入设计索引](../README.md)。
