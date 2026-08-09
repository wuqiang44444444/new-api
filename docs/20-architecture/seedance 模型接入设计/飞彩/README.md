---
status: accepted
owner: Dev Team
last-reviewed: 2026-08-10
---

# 飞彩 Seedance 全模型接入设计

本目录定义飞彩 10 个 Seedance 模型接入 new-api 的当前架构。代码显式注册两个并行且不可变的活动实现：
`feicai.seedance-videos@v2` 只绑定带 `-feicai` 后缀的 Provider 模型，`v3` 只绑定无后缀模型；两者
履行同一组 10 个 Link SKU，但证据、渠道选择、任务快照和 hash 相互独立。v1 窄实现已从注册、adapter
分派和确定拒绝规则中移除，不提供 v1 兼容解析或历史任务 fallback。

## 1. 文档职责与阅读顺序

1. [总体架构与履约设计](飞彩总体架构与履约设计.md)：v2/v3 并行实现、请求转换、Task、Link 资源、内容代理以及 v1 移除边界。
2. [全模型 SKU 与计费设计](飞彩全模型SKU与计费设计.md)：10 个模型逐一对应的身份、能力、size、媒体和计费合同。
3. [发布门禁与验证设计](飞彩发布门禁与验证设计.md)：架构级证据门禁、切换条件和发布不变量。

执行层文档：

- [确定性回归规约](../../../30-engineering/飞彩Seedance全模型回归规约.md)：代码注册、请求、Task、素材、内容和计费回归。
- [真实上线验收手册](../../../40-operations/06-飞彩Seedance全模型上线验收手册.md)：隔离分组中的 10 模型黑盒、账单和停流验收。

## 2. 当前事实与发布边界

当前代码已完成 v1 移除和 v2/v3 显式分离：

```text
已删除飞彩 v1 注册与 v1 专用分支
  -> 以相同 implementation ID 登记不可变 v2 与 v3
  -> 两个版本共享 10 个独立 Link SKU capability
  -> v2 的 10 条 binding 只接受带后缀模型，v3 的 10 条 binding 只接受无后缀模型
  -> 只允许 media-arrays adapter v2 创建、轮询和内容回源
  -> size registry 只登记 v2 下六个已取得真实内容证据的 16:9 组合
  -> v3 当前没有 size evidence，全部组合在 Provider POST 前失败关闭
  -> 逐模型通过证据门禁后发布 Ability
```

本地部署已完成 Mini 720p、Standard 720p 和 Standard 1080p 的受控系统 E2E，并以隔离 Provider
验证补齐 Fast 720p、Standard 4K 和 Value 720p 的精确内容证据。渠道 51 已发布五个 VIP SKU；渠道
52 在五模型矩阵中仅 Value 720p 成功，因此已收窄为该单一候选并发布。SD2、Value 1080p、Value 4K
和 Pro PI 已拆入 manually disabled 的渠道 55；该隔离渠道只有禁用 Ability，没有 publication，四个模型
继续保持未发布。其它部署环境仍必须独立执行同样审计，不能继承本地结论。

## 3. 全模型覆盖

| Provider 模型 | Link SKU | 核心差异 | 当前代码 |
| --- | --- | --- | --- |
| `seedance-2.0-vip-720p-mini-azhw-feicai` | `seedance-2.0-mini-720p` | 720p、4–15 秒、按秒 | v2 已登记，渠道 51 已发布 |
| `seedance2.0-sd2-feicai` | `seedance-2.0-sd2-720p` | 720p、11–15 秒、图片必填 | v2 已登记，未发布 |
| `seedance-2.0-vip-720p-fast-azhw-feicai` | `seedance-2.0-fast-720p` | 720p、4–15 秒、按秒 | v2-r2 已登记，渠道 51 已发布 |
| `seedance-2.0-933-720p-azhw-feicai` | `seedance-2.0-value-720p` | 720p、value 档 | v2-r3 已登记，渠道 52 已发布 |
| `seedance-2.0-vip-720p-azhw-feicai` | `seedance-2.0-standard-720p` | 720p、standard 档 | v2 已登记，渠道 51 已发布 |
| `seedance-2.0-933-1080p-azhw-feicai` | `seedance-2.0-value-1080p` | 1080p、value 档 | v2 已登记，未发布 |
| `seedance-2.0-vip-1080p-azhw-feicai` | `seedance-2.0-standard-1080p` | 1080p、standard 档 | v2 已登记，渠道 51 已发布 |
| `seedance-2.0-933-4k-azhw-feicai` | `seedance-2.0-value-4k` | 4K、value 档 | v2 已登记，未发布 |
| `seedance-2.0-vip-4k-azhw-feicai` | `seedance-2.0-standard-4k` | 4K、standard 档 | v2-r2 已登记，渠道 51 已发布 |
| `seedance-933-pro-pi-feicai` | `seedance-2.0-pro-pi-720p` | 720p、固定 15 秒、参考视频、按次 | v2 已登记，未发布 |

表中“v2 已登记”描述当前生产渠道使用的带后缀实现。v3 另以无后缀模型登记相同 10 个 SKU，但
尚无独立 size evidence，不能继承表中的 v2 发布状态。任一版本的结构登记都不等于价格已批准或
客户合同已发布；10 个模型及两个实现版本必须按精确证据键分别闭环，不能互相替代。

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

当前未闭合项包括：SD2 默认时长、除六个已验证 `16:9` 条目外的逐模型像素映射、宽幅 1.667 倍适用范围、
Pro PI 按次计费与宽幅倍率关系、v2 确定拒绝组合以及任务级 Provider 成本。2026-08-05 正式凭据黑盒
确认 6 个模型能生成同源 HTTPS MP4，但 SD2 create 503 unknown，value-1080p、value-4k 和 Pro PI 未成功；
生产 HTTPS `/v1/tasks` 返回 404，Fast 与 Value 720p 的隔离 usage 增量也不能替代任务级账单。
2026-08-06 渠道 52 五模型矩阵中，Value 720p 完整生成 1256×720 MP4；SD2 与 Pro PI create 503
unknown，Value 1080p 与 Value 4K 明确终态失败。因此只为 Value 720p 登记精确 `16:9`
size/billing class 并收窄发布；该事实不能扩张为其它 ratio/SKU。

研究输入：

- [飞彩 Seedance 更新资料](<../../../70-research/飞彩/飞彩-seedance%20更新.md>)
- [飞彩账单与查询资料](../../../70-research/飞彩/飞彩-账单与查询.md)

上位架构见父级 [Seedance 模型接入设计索引](../README.md)。
