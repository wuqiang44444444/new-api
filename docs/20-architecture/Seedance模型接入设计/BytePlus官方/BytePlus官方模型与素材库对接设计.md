---
status: current
owner: Dev Team
last-reviewed: 2026-08-18
---

# BytePlus 官方模型与素材库对接设计

## 1. 协议与连接

视频使用 `modelark_v3_byteplus`；客户模型经 `model_mapping` 得到 `dreamina-*`/`seedance-*` 精确模型。素材使用 `byteplus_assets_action_v2024_01_01`，独立 AK/SK、Region 和 ProjectName；Action endpoint 由代码按 Region 生成。中国区 `doubao-*` 不得映射到 BytePlus Channel。

## 2. 视频与素材流

北向 ModelArk V3 的 content、duration、resolution、ratio、generate_audio、watermark 按官方合同转换。素材 CRUD 由官方 Action adapter 履约；公开 API 仍是带 `model` 的单资源代理，不提供列表。`asset://<opaque-id>` 直接进入视频 adapter，不本地查询或验证作用域；source URL 不持久化。

## 3. 异步和签名边界

创建前 durable attempt 与资金 hold 原子提交，只有可信 task ID 才创建 Task。Task 冻结协议、Provider 模型、Region、Project、连接和计费事实；后续查询/内容回源不重选当前配置。AK/SK 只用于签名，任何日志、公开元数据和普通响应都必须脱敏。

## 4. 不支持与验收

目标账号未开通的模型、规格或素材操作必须明确返回不支持；不因国内/海外相似模型自动 fallback。上线前逐模型核对官方 usage、失败收费、资源包抵扣、素材真人认证与 URL 生命周期。

