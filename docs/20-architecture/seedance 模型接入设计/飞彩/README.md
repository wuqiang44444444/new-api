---
status: accepted
owner: Dev Team
last-reviewed: 2026-08-05
---

# 飞彩 Seedance 全模型接入设计

本目录定义飞彩 10 个 Seedance 模型接入 new-api 的当前架构。飞彩只保留一个活动实现：
`feicai.seedance-videos@v2`。v1 窄实现已从注册、adapter 分派和确定拒绝规则中移除，不建设
v1/v2 双轨、兼容解析或历史任务 fallback。

## 1. 文档职责与阅读顺序

1. [总体架构与履约设计](飞彩总体架构与履约设计.md)：单轨 v2、请求转换、Task、Link 资源、内容代理以及 v1 移除边界。
2. [全模型 SKU 与计费设计](飞彩全模型SKU与计费设计.md)：10 个模型逐一对应的身份、能力、size、媒体和计费合同。
3. [发布门禁与验证设计](飞彩发布门禁与验证设计.md)：架构级证据门禁、切换条件和发布不变量。

执行层文档：

- [确定性回归规约](../../../30-engineering/飞彩Seedance全模型回归规约.md)：代码注册、请求、Task、素材、内容和计费回归。
- [真实上线验收手册](../../../40-operations/06-飞彩Seedance全模型上线验收手册.md)：隔离分组中的 10 模型黑盒、账单和停流验收。

## 2. 当前事实与发布边界

当前代码已完成单轨替换：

```text
已删除飞彩 v1 注册与 v1 专用分支
  -> 以相同 implementation ID 登记唯一 v2
  -> 登记 10 个独立 Link SKU capability 与 execution binding
  -> 只允许 media-arrays adapter v2 创建、轮询和内容回源
  -> size registry 只登记三个已经本系统 E2E 的 16:9 组合
  -> 其余未取证组合在 Provider POST 前失败关闭
  -> 逐模型通过证据门禁后发布 Ability
```

本地部署已完成 Mini 720p、Standard 720p 和 Standard 1080p 的受控系统 E2E：三条 Task 均
`SUCCESS/settled`，create attempt 均 `complete/transferred`，鉴权内容下载与 Range 均成功。当前渠道
51/52 仍为 v2 且禁用，10 条 Ability 全部禁用；受控验证建立的三条 publication 作为不可变审计
事实保留，但因没有当前合格候选实现而不对客户暴露。其它部署环境仍必须独立执行同样审计，
不能继承本地结论。

## 3. 全模型覆盖

| Provider 模型 | Link SKU | 核心差异 | 当前代码 |
| --- | --- | --- | --- |
| `seedance-2.0-vip-720p-mini-azhw-feicai` | `seedance-2.0-mini-720p` | 720p、4–15 秒、按秒 | v2 已登记，未发布 |
| `seedance2.0-sd2-feicai` | `seedance-2.0-sd2-720p` | 720p、11–15 秒、图片必填 | v2 已登记，未发布 |
| `seedance-2.0-vip-720p-fast-azhw-feicai` | `seedance-2.0-fast-720p` | 720p、4–15 秒、按秒 | v2 已登记，未发布 |
| `seedance-2.0-933-720p-azhw-feicai` | `seedance-2.0-value-720p` | 720p、value 档 | v2 已登记，未发布 |
| `seedance-2.0-vip-720p-azhw-feicai` | `seedance-2.0-standard-720p` | 720p、standard 档 | v2 已登记，未发布 |
| `seedance-2.0-933-1080p-azhw-feicai` | `seedance-2.0-value-1080p` | 1080p、value 档 | v2 已登记，未发布 |
| `seedance-2.0-vip-1080p-azhw-feicai` | `seedance-2.0-standard-1080p` | 1080p、standard 档 | v2 已登记，未发布 |
| `seedance-2.0-933-4k-azhw-feicai` | `seedance-2.0-value-4k` | 4K、value 档 | v2 已登记，未发布 |
| `seedance-2.0-vip-4k-azhw-feicai` | `seedance-2.0-standard-4k` | 4K、standard 档 | v2 已登记，未发布 |
| `seedance-933-pro-pi-feicai` | `seedance-2.0-pro-pi-720p` | 720p、固定 15 秒、参考视频、按次 | v2 已登记，未发布 |

“v2 已登记”只表示 capability、execution binding 和 adapter 结构完整，不等于价格已批准或
客户合同已发布。10 个模型必须分别完成 size、任务、内容和账单证据闭环，不能互相替代。

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

当前未闭合项包括：SD2 默认时长、除三个已验证 `16:9` 条目外的逐模型像素映射、宽幅 1.667 倍适用范围、
Pro PI 按次计费与宽幅倍率关系、v2 确定拒绝组合以及任务级 Provider 成本。2026-08-05 正式凭据黑盒
确认 6 个模型能生成同源 HTTPS MP4，但 SD2 create 503 unknown，value-1080p、value-4k 和 Pro PI 未成功；
生产 HTTPS `/v1/tasks` 返回 404，Fast 的隔离 usage 增量也与研究报价不一致。Mini 720p、Standard
720p 和 Standard 1080p 已追加完成本系统 `:8100` 端到端验证，因而只登记其精确 `16:9`
size/billing class 条目；这不能替代逐任务 Provider 账单或客户价格审批，也不能扩张为其它 ratio/SKU。

研究输入：

- [飞彩 Seedance 更新资料](<../../../70-research/飞彩/飞彩-seedance%20更新.md>)
- [飞彩账单与查询资料](../../../70-research/飞彩/飞彩-账单与查询.md)

上位架构见父级 [Seedance 模型接入设计索引](../README.md)。
