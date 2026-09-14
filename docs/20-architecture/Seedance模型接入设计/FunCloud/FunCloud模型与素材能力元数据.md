---
status: current
owner: Dev Team
last-reviewed: 2026-09-10
---

# FunCloud 模型与素材能力元数据

## 1. V3 运行时与公开投影

FunCloud 新配置与新提交只接受 V3。V2 的 `funcloud_seedance` 不再发布创建能力；
只保留已创建任务按冻结事实查询、响应解析与结算所需的内部实现。

`funcloud_modelark_v3` 的四模型共同读取 `relaykit/dto/funcloud_modelark_models.go`：

| Provider 模型 | 分辨率 | 时长 | 图/视频/音频 |
| --- | --- | --- | --- |
| `seedance-2-0` / `seedance-2-0-fast` / `seedance-2-0-mini` | 480/720p | 4–15 秒 | 30/10/10 |
| `seedance-2-5` | 480/720/1080p | 4–30 秒或 -1 | 30/10/10 |

四模型均可配对既有 `funcloud_material`，公开创建/查询组、创建/查询/删除单素材；更新素材继续不支持。
同一 Channel 的四模型具有相同非空 `reuse_scope`，其含义仅为可尝试复用。视频协议替换确认后作用域会变化。
V3 公开视频创建、查询、本地列表和内容读取；删除操作标记不支持，与运行时能力一致。
代码与自动测试完成不代表真实 Provider、账单和生产发布均已验收；状态见 80-dev 实施记录。

## 2. 参考媒体与素材边界

数量来自 [FunCloud V3 内容表](https://docs.leonecloud.com/docs/seedance-2-5-v3-protocol/)（2026-09-10 核查）。
四模型均接收参考图、视频和音频；没有文档禁止的纯音频组合不额外要求图片或视频。
`generate_audio` 控制输出声音，独立于输入参考音频；显式 false 保留。
四模型公开 `priority`（0–9）和 `return_last_frame`，显式 0/false 原值出站。
成功查询保留上游尾帧结果，并对客户投影本站 `content?part=last_frame` 地址。
其它 ModelArk 扩展参数仍有未开放项，具体缺口见实施记录第 4 节；不得将其解释为供应商禁止。


能力由 `funcloud_modelark_v3` 的精确 Provider 模型表生成，不从客户模型名推断。
公开元数据只描述客户模型和北向协议，不返回 Provider 模型、路径或账号。
`asset://` 是不透明引用；不承诺状态、所有权或跨渠道兼容性，由 Provider 判断。
普通素材代理保留既有单资源操作；本站托管图片按独立 `funcloud_material_hosted` 能力提供。

实施和待验证事项见 [归档的非墨行参考音视频清理记录](../../../../99-archive/2026/09/2026-09-10-非墨行参考音视频限制清理实施记录.md)。
