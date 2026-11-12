---
status: current
owner: Dev Team
last-reviewed: 2026-11-12
---

# ModelArk 视频任务用量设计

本文定义 ModelArk V3 查询代码与 OpenAPI 使用的用量合同。各环境以实际部署版本为准。

## 适用范围

API 客户通过 `GET /api/v3/contents/generations/tasks/{task_id}` 查询单项，或通过
`GET /api/v3/contents/generations/tasks` 查询列表。每个返回的任务对象都有固定结构的 `usage`，
适用于排队、运行、成功、失败、取消和过期任务。列表和单查在读取到同一任务版本时返回相同用量。

该合同适用于使用本站 ModelArk V3 标准视频入口的 H3、Seedance、JD 及后续接入的模型。
创建、删除、错误响应和 NEWAPI 原生、OpenAI、Kling、即梦等独立接口各按其现行响应合同交付。

## 响应结构

以下是任务尚无可展示用量时的完整 `usage`。秒数是非负 JSON 数字，允许小数。图片、Token 和工具调用
次数是非负整数。所有字段均存在，已确认的值按原单位展示，缺失或无法确认的值在响应中用 `0` 表示。

```json
{
  "usage": {
    "total_seconds": 0,
    "input_seconds": 0,
    "output_seconds": 0,
    "input_audio_seconds": 0,
    "input_image_count": 0,
    "prompt_tokens": 0,
    "completion_tokens": 0,
    "total_tokens": 0,
    "tool_usage": {
      "web_search": 0
    }
  }
}
```

| 字段 | 客户侧含义 |
| --- | --- |
| `total_seconds` | 已确认的输入视频与输出视频秒数之和。分项不齐时，可展示经确认采用同一口径的上游总数。无法确认总数时显示 `0`。音频单列。 |
| `input_seconds` | 已确认的输入视频秒数。 |
| `output_seconds` | 已确认的生成视频秒数。顶层 `duration` 保留原接口的时长语义。 |
| `input_audio_seconds` | 已确认的输入音频秒数。 |
| `input_image_count` | 已确认的输入图片数量。 |
| `prompt_tokens` | 来源明确的输入 Token 数。 |
| `completion_tokens`、`total_tokens` | 完成 Token 数与总 Token 数，沿用已有的标准视频 Token 归一语义。 |
| `tool_usage.web_search` | 已确认的搜索调用次数。 |

按秒计费的任务也可以同时展示 Token。JD 若只有请求时长、没有确认的输出时长，
`usage.output_seconds` 显示 `0`。Seedance 若只有 Token 数据，媒体用量字段显示 `0`。
已有任务直接展示已保存且可确认的用量，其余字段显示 `0`。

## 客户可观察结果

- 无用量时 `usage` 仍存在。既有后台轮询或用量补查取得可信用量后，单查和列表展示更新后的数值。
- 上游缺字段或后续提供冲突值时，已确认的用量继续可读。
- 客户可以用同一个数据结构解析所有 ModelArk 视频模型。客户端曾通过 `usage` 是否存在判断用量齐全的，
  需要改用其业务所需的状态或账单信息。
- `usage` 是任务用量展示。费用以账单为准。展示 `0` 可能表示未报告或确认无用量，不能据此判断免费、
  已结算或是否会退款。

技术边界见 [ModelArk V3 北向合同](../20-architecture/Seedance专用渠道与Link架构.md#6-modelark-v3-北向合同)。
生产灰度与剩余协议证据门槛见[路线图](../50-planning/路线图.md)第 39 项。
