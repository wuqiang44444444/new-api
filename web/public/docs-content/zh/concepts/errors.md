---
page-id: errors
kind: guide
last-verified: 2026-10-21
operations: []
---

# 错误与重试

先结合 HTTP 状态、所调用的操作和公开错误信封中的 `error.type`、`error.code`、`error.message` 判断是否可以重试。不要依赖上游原始错误文本。

## HTTP 状态

| 状态  | 含义                       | 默认策略                           |
| ----- | -------------------------- | ---------------------------------- |
| `400` | 请求字段、格式或状态不合法 | 修正请求，不重试                   |
| `401` | 鉴权失败                   | 修复 Key，不重试                   |
| `403` | 权限或模型访问被拒绝       | 修复权限，不重试                   |
| `404` | 资源或任务不存在           | 检查 ID、接口及原应用 Key          |
| `402` | 额度不足（部分接口）       | 补充额度后再发起新请求，不持续重试 |
| `409` | 幂等冲突或资源状态冲突     | 读取错误码后决定                   |
| `410` | 内容交付不可用或已过期     | 按接口合同核查，不重新生成         |
| `413` | 请求正文超过服务上限       | 减少正文或内联媒体大小后重新发起   |
| `429` | 频率、并发或额度限制       | 按指数退避并加入抖动               |
| `5xx` | 服务暂时不可用             | 对可安全重放的请求有限重试         |

## 错误信封

```json
{
  "error": {
    "message": "request cannot be completed",
    "type": "invalid_request_error",
    "param": null,
    "code": "invalid_request"
  }
}
```

不同协议使用各自的公开错误信封：

| 场景                       | 读取位置                                                                                  |
| -------------------------- | ----------------------------------------------------------------------------------------- |
| 常见 OpenAI 兼容 HTTP 错误 | `error.message`、`error.type`、`error.code`                                               |
| 图片任务查询成功但任务失败 | HTTP 可为 `200`；检查 `status`，错误在 `error.code` / `error.message`，没有第二层 `error` |
| Kling                      | 顶层数值 `code`、`message` 和 `request_id`                                                |
| 即梦                       | 顶层数值 `code`、`status`、`message` 和 `request_id`                                      |
| 通用视频查询               | 顶层 `code` / `message`；查询成功后业务状态在 `data.status`                               |

客户端应先按所调用的协议解析，不能只判断错误文本。公开请求 ID 可用于排障，但不一定等于任务 ID。

## 请求记录相关错误

请求证据是尽力保存的观察数据。采集上限、正文格式、索引或存储故障不会拒绝业务，
不会改变重试、结算、退款或任务结果；证据缺失也不能证明请求没有发送。业务本身的正文校验
和正式输入、结果存储仍按对应接口规则执行。

| HTTP 状态 | 错误码 | 处理方式 |
| --- | --- | --- |
| `503` | `evidence_unavailable` | durable attempt 的发送资格或结果无法核实；先查询任务或联系管理员，不自动重发 |

此 `503` 不表示已退款，也不能作为可安全重放的依据。

## 安全重试

使用指数退避，例如 1 秒、2 秒、4 秒，并加入随机抖动。限制总次数和总等待时间。

- 查询请求通常可以重试。
- 流式媒体请求即使尚未收到数据也可能已开始生成，断流后不要盲目重发。
- 图片实际异步模式可使用原 `Idempotency-Key` 确认同次受理；ModelArk V3 创建和素材创建没有客户幂等保证。
- 客户端超时不代表服务端一定没有接收请求；有 ID 时先查询原任务，只有明确支持幂等的模式才能用原键确认。
- `create_outcome_unknown` 和图片任务 `unknown` 不能按普通 `5xx` 处理：不自动重发、不假定退款，保留请求或任务 ID 并核查。
- 图片 `429` / `503` 只有明确为受理容量或受理前存储检查失败时才表示未受理；不能把所有同状态响应都视为可安全重复创建。

## 视频已生成但交付未就绪或不可用

Seedance 与 H3 标准视频的生成状态与交付状态独立：查询可返回 HTTP `200`、`status=succeeded`，
同时 `content.delivery_status=preparing` 或 `unavailable`，此时没有主视频下载 URL。
`preparing` 时继续有界查询原任务；`unavailable` 时保留任务 ID 核查。

鉴权内容入口的错误按 `error.code` 区分：

| HTTP 状态 | 错误码 | 调用方处理 |
| --- | --- | --- |
| `409` | `video_delivery_preparing` | 等待转存后重试查询或内容 GET，不重新创建 |
| `410` | `video_delivery_unavailable` | 交付不可用，保留 ID 核查，不假定退款 |
| `503` | `video_delivery_unavailable` | 当前签名存储不可用，有限重试 GET 或联系管理员 |

这些交付故障不改变已确认的生成成功和计费事实，不自动退款或重新生成。本站 OSS 签名 URL 从每次
签发起有效 24 小时，下载不附平台 Key；对象仍保留且授权有效时查询原任务可续签。范围外原生视频
保留原有过期规则，`video_content_expired` 不能作为本站托管结果的通用错误口径。
完整下载与跳转规则见[ModelArk V3 视频](api-reference/videos/modelark#下载视频或末帧)。

English: hosted standard video delivery errors do not change generation or billing facts. Retry the original
GET for `409 video_delivery_preparing`; investigate `410 video_delivery_unavailable`; use bounded GET retries
for `503 video_delivery_unavailable`. Do not regenerate or assume a refund. An authorized task query can renew
an expired 86400-second signed URL while the object is retained; download the URL without an API key.

## 限流与计费

限流、并发和额度是不同约束。降低请求频率不能修复余额不足；切换模型可能改变计费和权限。始终保存自己的请求 ID、时间、模型与任务 ID，排障时不要记录完整敏感输入。

## 图片已生成但格式交付失败

`502 image_delivery_failed` 表示生成已完成并计费，但 URL／Base64 交付转换失败。错误正文可能
同时包含 `requested_response_format` 和原始 `data[]`；应按实际 `url` / `b64_json` 取回图片，
不要因 HTTP 非 2xx 丢弃正文，也不要自动重发生成或编辑请求。没有保留结果时 `data` 可省略。
参见[图片返回格式与失败示例](api-reference/images/generations)。
