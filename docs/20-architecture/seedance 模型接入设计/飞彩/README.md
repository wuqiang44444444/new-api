---
status: accepted
owner: Dev Team
last-reviewed: 2026-08-10
---

# 飞彩 Seedance 全模型接入设计索引

飞彩 Seedance 通过 `ChannelTypeSeedanceLink` 接入，北向固定 ModelArk V3，南向使用代码化
`media_arrays_v2` / `media_arrays_v3` adapter。v2 与 v3 Provider 模型名和证据独立，必须使用不同客户
模型名和独立 Channel；不能把两个版本映射成同一个前端模型，也不能互相 fallback。

飞彩资料覆盖 10 个模型档位。每个准备上线的 Provider 模型都由技术人员独立验证字段、size、媒体、
Task、内容和账单，再给管理员独立客户模型、模型映射、价格和协议配置。系统不建立 publication、
Link SKU、implementation 或 execution binding 门禁。

## 当前 v3 证据

2026-08-10 使用正式 v3 路径验证：

- Mini 720p、Fast 720p、Standard 720p 的 16:9 创建成功，产物为 1280×720，并有 task quota/usage
  观测，因此这三个精确组合可以进入 v3 adapter 的 size 表；
- Standard 1080p/4K、Value 720p/1080p/4K 和 Pro PI 未获得可发布成功证据；
- SD2 创建返回 503，结果不明，按视频 unknown 处理，不自动重发；
- Pro PI 窗口曾观察到 CNY 11.55 usage 异常，之后净额回退，不能作为任务成本或成功证据；
- 其它七个 v3 模型不得复制 v2 size、账单或内容证据。

v2 的历史成功证据继续只用于 v2 Provider 模型，不会自动授权 v3。

## 阅读顺序

1. [总体架构与履约](飞彩总体架构与履约设计.md)
2. [全模型与计费](飞彩全模型SKU与计费设计.md)
3. [上线证据与验证](飞彩发布门禁与验证设计.md)

## 上位架构

- [Seedance 统一北向合同架构](../../Seedance统一北向合同架构.md)
- [Link 视频服务合同与异步任务架构](../../Link视频服务合同与异步任务架构.md)
- [异步任务与计费事实架构](../../异步任务与计费事实架构.md)
