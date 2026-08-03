---
status: current
owner: Dev Team
last-reviewed: 2026-08-03
---

# 20-architecture — 目标说明

## 目标
说明系统如何构成、关键设计决策及其理由。

## 放什么
架构概览、数据模型、接口边界、架构决策记录（ADR）。

## 当前文档

- [架构概览](架构概览.md)
- [数据模型](数据模型.md)
- [Link 合同架构](Link合同架构.md)
- [Link 资源虚拟素材库架构](Link资源虚拟素材库架构.md)
- [统一图片生成与异步任务架构](统一图片生成与异步任务架构.md)
- [API Key 用量账单架构](API-Key用量账单架构.md)
- [内置 API 文档中心架构](内置API文档中心架构.md)
- [视频上游接入与异步任务架构](视频上游接入与异步任务架构.md)
- [图片异步任务共享 Task 与客户端恢复决策](decisions/0006-media-image-shared-task-persistence.md)
- [视频 Link 合同与共享任务底座决策](decisions/0007-视频Link合同与共享任务底座.md)
- [共享异步任务计费状态机与原子补偿决策](decisions/0008-共享异步任务计费状态机与原子补偿.md)
- [请求级媒体与平台托管素材双路径决策](decisions/0009-请求级媒体与平台托管素材双路径.md)
- [视频公开 SKU 能力与候选渠道等价决策](decisions/0010-视频公开SKU能力与候选渠道等价.md)
- [Link 合同与渠道适配协议决策](decisions/0013-Link合同与渠道适配协议.md)
- [异步创建未知与轮询合同违例对账决策](decisions/0011-异步创建未知与轮询合同违例对账.md)
- [真人素材单图同步流式边界决策（已被 ADR-0014 取代）](decisions/0012-真人素材单图同步流式边界.md)
- [Link 资源源引用与双模式解析决策（已被 ADR-0015 取代）](decisions/0014-Link资源源引用与双模式解析.md)
- [Link 公开 SKU 与实现身份版本绑定决策](decisions/0015-Link公开SKU与实现身份版本绑定.md)
- [素材代理与真人授权架构](素材代理与真人授权架构.md)
- [架构决策记录](decisions/)

## 不放什么
- 产品需求与流程 -> 10-product/
- 运维手册与监控 -> 40-operations/
- 编码过程中的临时修复方案 -> 80-dev/
