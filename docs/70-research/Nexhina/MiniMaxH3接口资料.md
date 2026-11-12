---
status: current
owner: Dev Team
last-reviewed: 2026-11-12
---

# Nexhina MiniMax H3 接口资料

Nexhina 文档中心「Minimax H3 视频」专栏，采集于 2026-11-12（Asia/Shanghai）。上游 Base URL 为 `https://token.nexhina.cn`，使用 Bearer 鉴权。

| 接口 | 方法和路径 | 文档所示合同 |
| --- | --- | --- |
| 创建视频 | `POST /hailuo/v2/video_generation` | JSON，文档模型为 `MiniMax-H3`，分辨率 `768P` 或 `2K`，时长 4–15 整数秒，`content[]` 至少有一条非空 `text`。图生视频另给 `image_url` 参考图。成功返回顶层 `task_id`。 |
| 查询视频 | `GET /hailuo/v2/query/video_generation/{task_id}` | 返回 `task` 对象，含 `id`、`model`、`status`、整数时间戳、`content.url`、`resolution`、`duration`。状态枚举为 `queued`、`processing`、`succeeded`、`failed`、`cancelled`。 |

查询示例的 `task.usage` 包含 `total_seconds`、`input_seconds`、`output_seconds`、`input_image_count`、`input_audio_seconds`。示例状态为 `queued` 但同时给出视频地址，联调以实际状态和可用交付为准。媒体用量字段与 Nexhina 的实际扣费是两个待分别核对的事实。

本站对应该线路的渠道为 MiniMax 标准视频（`ChannelTypeMiniMaxLink`，类型 69），南向协议 `nexhina_hailuo_v2`，客户模型经 `model_mapping` 解析后原样发送。北向视频创建和单查使用 ModelArk V3 任务入口。

来源：[创建任务](https://doc.nexhina.cn/531513206e0)、[查询任务](https://doc.nexhina.cn/531513207e0)、[既有本地验收](../../80-dev/2026-11-10-MiniMax三渠道官网价真实验收与代码审计.md)。

## 本机复测

2026-11-12 使用本机后端 3501 的既有标准视频渠道 141，测试分组 `minimax-price-test`。客户模型 `minimax-h3-nexhina-pilot` 映射 `MiniMax-H3`。从本站 ModelArk V3 入口真实提交 768p、16:9、4 秒文生任务，创建 HTTP 200，单查约 158 秒到达 `succeeded`。上游实际返回 768P、4 输出秒和 130,196 completion / total tokens，本站单查 Token 数一致。

本站预扣 600,000 quota，按 4 秒结算 160,000 quota，退差额 440,000 quota。创建和差额日志均已送达。对象转存就绪后，授权 GET / HEAD 均返回 302，对象存储 Range GET 返回 206，并验证了 MP4 文件头。本次样本沿用现有 H3 适配，未核对 Nexhina 账户实际扣费。
