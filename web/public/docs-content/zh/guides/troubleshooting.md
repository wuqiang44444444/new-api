---
page-id: troubleshooting
kind: guide
last-verified: 2026-11-13
operations: []
---

# 排障清单

## Base URL

- OpenAI 兼容 SDK 使用 `{{OPENAI_BASE_URL}}`。
- Anthropic SDK 使用 `{{ANTHROPIC_BASE_URL}}`。
- ModelArk、Kling 和即梦示例使用 `{{SITE_BASE_URL}}` 下的原生路径。
- 检查反向代理是否保留请求方法、认证头、SSE 和较大的请求体。

## 鉴权与模型

- 确认请求头中只有占位位置被替换为你的 Key。
- 使用当前 Key 调用 `GET /v1/models`，检查模型可用状态并复制返回的 `id`。
- `401` 先查 Key，`403` 先查权限，`429` 再区分限流与额度。

## Content-Type

- JSON 请求使用 `application/json`。
- 图片编辑按模型声明使用 JSON 或 `multipart/form-data`；文件表单让客户端自动生成 boundary。
- 视频内容下载前验证状态和媒体类型。
- 错误响应通常是 JSON，不要按图片或视频解码。

## 图片与视频

- 检查远程媒体 URL 是否可公开访问、有效期是否覆盖处理时间。
- JSON 参考图按模型合同使用 HTTPS URL 或带 MIME 前缀的 Data URL；结果 `b64_json` 则是无前缀 Base64。
- `n`、时长、分辨率、宽高比和文件大小必须在模型范围内。
- 不同视频协议的字段不能互换。
- 图片默认同步；显式异步 `202` 返回任务 ID 后查询 `/v1/tasks/{task_id}`，不要重复拼接 `/v1`。同步或流式没有平台任务幂等保证，超时不能盲目重发。

## 素材

- 创建托管素材时，从 `api.assets` 或 `api.hosted_assets` 读取 `platform_hosted` 合同。提交生成时再核对目标模型的媒体限制。
- 调用方需要同时保存客户模型、不透明 ID 和返回的 `asset://` 引用。
- 代理素材的存在性和兼容性由素材服务判断。托管素材在视频预扣前检查账号归属、媒体槽位和对象可用性。
- `404 model_not_found` 表示客户模型不可用。`422 unsupported_asset_operation` 表示所选素材合同未开放该操作，
  `422 unsupported_asset_type` 表示所选素材合同不支持该素材类型。

## 异步任务

- 对明确返回任务 ID 的视频或显式异步图片执行以下步骤，查询路径必须与创建接口匹配。
- 保存创建响应里的任务 ID 和请求 ID。
- 使用退避轮询，不要每几十毫秒查询一次。
- 客户端超时后先查询原任务，不要立即重复创建。
- 终态失败后记录公开错误码，并停止轮询。
- 图片 `unknown` 时保存已有结果并联系管理员，不自动重发或认定已退款。
- 图片签名结果过期时重新 GET 任务取新 URL；`unavailable` 可稍后重查，`deleted` 不能靠重查恢复。
- ModelArk 视频任务单查和列表都返回完整的 `usage`。任务 `succeeded` 时，计量值为 `0` 仍可能表示用量尚未确认；以最终账单为准，不要重新生成来补齐用量。
- Seedance 与 H3 视频 `succeeded` 后仍需检查 `content.delivery_status`：`preparing` 时继续有界查询，
  `ready` 时下载，`unavailable` 时保留 ID 核查交付；不要因转存失败重新生成或假定退款。
- 签名 URL 到期后，使用原 Key 重新查询任务。详见[视频下载](api-reference/videos/modelark#下载视频或末帧)。
- 逐步示例见[图片与视频调用实战](guides/media-workflow)。

## 提交支持信息

提供时间、公开请求 ID、接口路径、HTTP 状态、公开错误码和已脱敏的最小请求。不要提供 API Key、Cookie、完整媒体、个人材料或内部管理页面导出。
