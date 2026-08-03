---
status: research
owner: Dev Team
last-reviewed: 2026-08-02
---

# Seedream Moxing：NEWAPI 统一生成/改图接口映射

> 本文保留供应商调研过程和历史方案。 Link 合同、公开模型命名及发布边界以
> [`2026-07-27-NEWAPI统一图片接口生产级多渠道兼容方案.md`](../99-archive/2026-07-27-NEWAPI统一图片接口生产级多渠道兼容方案.md)
> 为唯一事实来源；本文出现的共享 `seedream-5`、multipart 优先或供应商私有字段建议不再作为生产合同。

## 1. 目标与结论

客户端只使用 NEWAPI 客户 API 合同：

| 能力 | NEWAPI 客户 API 合同 |
|---|---|
| 文生图 | `POST /v1/images/generations` |
| 改图 | `POST /v1/images/edits` |
| 异步任务查询 | `GET /v1/images/tasks/:task_id` |

客户端统一模型名为 `seedream-5`，Moxing 上游模型名为 `seedream-5-0-260128`。

总体结论：

- 文生图：当前 `media_task_image_blocking` 已覆盖大部分请求、轮询、持久化和响应转换；通过模型映射和 Advanced Custom 配置即可接入。
- 改图：Moxing 没有使用 OpenAI Multipart `/v1/images/edits`，而是在 JSON `/v1/images/generations` 中传图片 URL。因此不能只靠现有配置，需要一个新的 Provider 改图转换器。
- 最大技术缺口：NEWAPI 客户端上传的是图片文件，Moxing 要求 HTTP(S) 图片 URL。当前仓库没有可直接复用的“临时文件上传并返回短期签名 URL”服务；必须增加媒体暂存能力，或由 Moxing 提供可用的文件上传接口。
- Moxing 的异步任务不能暴露原始任务格式；必须继续映射为 NEWAPI 的 `202 + /v1/images/tasks/:task_id` 合同。

## 2. NEWAPI Link 合同

NEWAPI 公共 `seedream-5` 档位与 Qihang 调研保持一致。

### 2.1 文生图

```http
POST /v1/images/generations
Authorization: Bearer <NEWAPI_KEY>
Content-Type: application/json
Idempotency-Key: <unique-key>
```

```json
{
  "model": "seedream-5",
  "prompt": "一只在草地上奔跑的金毛犬，电影级摄影",
  "n": 1,
  "size": "2K",
  "response_format": "url",
  "stream": false
}
```

### 2.2 改图

```http
POST /v1/images/edits
Authorization: Bearer <NEWAPI_KEY>
Content-Type: multipart/form-data
Idempotency-Key: <unique-key>
```

```bash
curl https://newapi.example/v1/images/edits \
  -H "Authorization: Bearer $NEWAPI_KEY" \
  -H "Idempotency-Key: edit-request-001" \
  -F "model=seedream-5" \
  -F "prompt=把背景改成白色，主体保持不变" \
  -F "image=@original.png" \
  -F "response_format=url"
```

第一阶段统一限制：

- 单个 `image` 文件。
- `n=1`。
- `response_format=url`。
- `stream` 省略或为 `false`。
- 改图不允许显式指定 `size`；Moxing Provider 默认使用 `2K`。
- 不支持 `mask`。
- 不开放 `image/images` URL、`capability`、`extra`、`reference_images` 等 Moxing 私有字段。

“完全使用 NEWAPI 统一接口”意味着客户端不能为了 Moxing 改为调用 `/v1/images/generations`，也不能被要求先把文件上传成 URL。

## 3. Moxing 上游合同

模型：

```text
seedream-5-0-260128
```

### 3.1 同步等待接口

```http
POST https://tokensave.pro/v1/images/generations
Authorization: Bearer <MOXING_KEY>
Content-Type: application/json
```

该接口在等待窗口内完成时返回 OpenAI 风格 `data[]`；上游任务未及时完成时可能返回 `202`。

### 3.2 原生异步接口

```http
POST https://tokensave.pro/v1/media/generations
GET  https://tokensave.pro/v1/media/tasks/:task_id
```

上游任务状态：

- `queued`
- `running`
- `succeeded`
- `failed`

### 3.3 上游参数

| 参数 | 类型 | 必填 | 说明 |
|---|---|---:|---|
| `model` | string | 是 | 固定 `seedream-5-0-260128` |
| `capability` | string | 是 | 固定 `image_generation` |
| `prompt` | string | 是 | 生成或修改提示词 |
| `n` | integer | 否 | 图片数量 |
| `size` | string | 是 | 当前转换器仅允许 `2K`、`3K` |
| `response_format` | string | 否 | 文档支持 `url`/`b64_json`，当前转换器只允许 `url` |
| `image` | string/string[] | 改图时 | HTTP(S) 图片 URL |
| `images` | string[] | 改图时 | `image` 数组别名 |
| `stream` | boolean | 否 | 当前统一转换器只允许 `false` |
| `extra` | object | 否 | Ark/Seedream 私有参数 |

`extra` 可包含：

- `sequential_image_generation`
- `sequential_image_generation_options.max_images`
- `watermark`

输入图片最多 14 张，但统一公共档位第一阶段只允许单图。

## 4. 文生图请求映射

| NEWAPI 客户端 | Moxing Provider | 处理方式 |
|---|---|---|
| `model=seedream-5` | `model=seedream-5-0-260128` | 渠道模型映射 |
| `prompt` | `prompt` | 原样传递，并限制不超过 3000 字符 |
| `n=1` | `n=1` | 原样传递 |
| `size=2K/3K` | `size=2K/3K` | 原样传递 |
| `response_format=url` | `response_format=url` | 原样传递 |
| `stream=false` | 不发送流式，或固定 `false` | `true` 在客户端返回 `400` |
| 无 | `capability=image_generation` | 转换器自动补充 |
| `watermark` | `extra.watermark` | 当前转换器具备映射，但公共档位暂不开放 |
| `user` | 无 | 仅 NEWAPI 内部审计 |
| `quality/style/...` | 无 | 客户端统一验证阶段拒绝 |

Provider 请求：

```json
{
  "model": "seedream-5-0-260128",
  "capability": "image_generation",
  "prompt": "一只在草地上奔跑的金毛犬，电影级摄影",
  "n": 1,
  "size": "2K",
  "response_format": "url"
}
```

当前 `media_task_image_blocking` 已经完成：

- 自动设置 `capability=image_generation`。
- 默认 `n=1` 和 `response_format=url`。
- 拒绝 `stream=true`。
- 校验 Seedream 5 的 `size` 为 `2K/3K`。
- 校验图片 URL 和最多 14 张输入图片。
- 映射顶层 `watermark` 到 `extra.watermark`。
- 处理同步 `200` 和异步 `202`。
- 持久化异步任务、轮询、计费转移和最终响应转换。

## 5. 改图请求映射

### 5.1 协议差异

| 阶段 | NEWAPI 客户端 | Moxing Provider |
|---|---|---|
| 路径 | `/v1/images/edits` | `/v1/images/generations` |
| Content-Type | `multipart/form-data` | `application/json` |
| 图片 | 文件二进制 | HTTP(S) URL |
| 模型 | `seedream-5` | `seedream-5-0-260128` |
| 任务 | NEWAPI 同步/202 扩展 | Moxing 同步或异步任务 |

### 5.2 目标转换流程

```text
NEWAPI /v1/images/edits Multipart
  -> 校验单图、MIME、大小、prompt、n 和不支持字段
  -> 将图片暂存到网关控制的私有媒体存储
  -> 生成短期、只读、可被 Moxing 访问的 HTTPS 签名 URL
  -> 构造 Moxing JSON /v1/images/generations
  -> 同步等待或持久化异步任务
  -> 统一返回 NEWAPI 200/202
```

目标 Provider 请求：

```json
{
  "model": "seedream-5-0-260128",
  "capability": "image_generation",
  "prompt": "把背景改成白色，主体保持不变",
  "n": 1,
  "size": "2K",
  "response_format": "url",
  "image": "https://media.example/signed/input.png"
}
```

映射表：

| NEWAPI 客户端 | Moxing Provider | 处理方式 |
|---|---|---|
| `model=seedream-5` | `model=seedream-5-0-260128` | 渠道模型映射 |
| `prompt` | `prompt` | 原样传递 |
| `image` 文件 | `image` HTTPS URL | 必须经过临时媒体暂存 |
| `n=1` | `n=1` | 原样传递 |
| 未指定 `size` | `size=2K` | Moxing 编辑默认值 |
| `response_format=url` | `response_format=url` | 原样传递 |
| `stream=false` | 不发送或 `false` | `true` 返回 `400` |
| 无 | `capability=image_generation` | 自动补充 |
| `mask` | 无 | 返回 `400` |
| 多个图片文件 | `image[]` 有能力支持 | 公共档位第一阶段返回 `400` |

### 5.3 媒体暂存是实施前置条件

Moxing 文档和当前转换器只接受 HTTP(S) URL，不接受 Multipart 文件；当前转换器还明确拒绝非 HTTP(S) URL。

因此不能采用以下做法：

- 不能把本地临时文件路径发给上游。
- 不能假设 Moxing 接受 `data:` URL 或裸 base64。
- 不能让客户端直接提供任意远程 URL 来替代标准 Multipart 上传。
- 不能把上传图片长期公开。

临时媒体存储至少需要：

- 文件类型和尺寸限制。
- 内容嗅探，不能只相信扩展名。
- 短期签名 URL。
- 只读权限。
- 请求完成或任务终态后的过期清理。
- 不在普通日志中记录完整签名 URL。
- 防止 SSRF、路径穿越和可执行内容上传。

如果现有部署没有可用的对象存储，必须先确认 Moxing 是否提供文件上传接口。没有“上游上传接口”或“网关临时媒体存储”中的任意一个，标准 Multipart 改图无法安全映射到 Moxing。

## 6. 异步映射

### 6.1 默认同步调用

客户端不传 `Prefer: respond-async`：

| Moxing 行为 | NEWAPI 行为 |
|---|---|
| 上游直接 `200 + data[]` | 校验数量后返回 `200 + data[]` |
| 上游返回 `202 + task_id`，等待窗口内成功 | 内部轮询，返回 `200 + data[]` |
| 上游返回 `202 + task_id`，等待窗口耗尽 | 持久化任务，返回 NEWAPI `202` |
| 上游任务失败 | 返回规范化错误，不伪造成功响应 |

### 6.2 客户端偏好异步

客户端传：

```http
Prefer: respond-async
```

当 Moxing 返回 `202` 时，NEWAPI 持久化任务后立即返回：

```http
HTTP/1.1 202 Accepted
Location: /v1/images/tasks/img_task_xxx
Retry-After: 2
X-Task-ID: img_task_xxx
```

```json
{
  "id": "img_task_xxx",
  "object": "image.generation.task",
  "status": "queued",
  "created_at": 1761223754,
  "model": "seedream-5"
}
```

客户端轮询：

```http
GET /v1/images/tasks/img_task_xxx
```

完成响应：

```json
{
  "id": "img_task_xxx",
  "object": "image.generation.task",
  "status": "completed",
  "created_at": 1761223754,
  "completed_at": 1761223800,
  "model": "seedream-5",
  "result": {
    "created": 1761223800,
    "data": [
      {
        "url": "https://example.com/result.png"
      }
    ]
  }
}
```

不得向客户端暴露：

- Moxing 原始任务 ID。
- Moxing 查询路径。
- Moxing 鉴权信息。
- 上游内部状态字段或签名输入 URL。

## 7. 渠道配置

### 7.1 模型与模型映射

对外模型列表：

```text
seedream-5
```

渠道模型映射：

```json
{
  "seedream-5": "seedream-5-0-260128"
}
```

Advanced Custom 路由匹配必须填写客户端别名 `seedream-5`，不能填写上游版本名。

### 7.2 当前可用的文生图配置

```json
{
  "advanced_custom": {
    "advanced_routes": [
      {
        "incoming_path": "/v1/images/generations",
        "upstream_path": "/v1/images/generations",
        "converter": "media_task_image_blocking",
        "models": [
          "seedream-5"
        ],
        "auth": {
          "type": "header",
          "name": "Authorization",
          "value": "Bearer {api_key}"
        }
      }
    ]
  }
}
```

### 7.3 改图的目标配置

当前不能直接保存以下配置，因为目标转换器尚未注册：

```json
{
  "incoming_path": "/v1/images/edits",
  "upstream_path": "/v1/images/generations",
  "converter": "media_task_image_edit_blocking",
  "models": [
    "seedream-5"
  ],
  "auth": {
    "type": "header",
    "name": "Authorization",
    "value": "Bearer {api_key}"
  }
}
```

不能把现有 `media_task_image_blocking` 直接配置到 `/v1/images/edits`：当前配置校验只允许它绑定 `/v1/images/generations`，并且它接收的是已经结构化的 JSON 图片请求，不会把 Multipart 文件变成上游 URL。

## 8. 最小入侵实施方案

### 8.1 新增独立文件

建议把主要新增逻辑隔离在新文件中：

- `relay/channel/advancedcustom/media_task_image_edit_blocking.go`
  - Multipart 文件提取。
  - 统一改图参数校验。
  - 临时媒体 URL 创建。
  - Moxing JSON 请求构造。
- 对应独立测试文件。
- 临时媒体存储使用独立服务接口和实现文件，不把对象存储逻辑塞入现有图片转换器。

现有 `media_task_image_blocking.go` 中稳定的任务创建、轮询、响应和计费逻辑应复用；不要复制第二套异步任务生命周期。

### 8.2 现有文件只做窄接线

需要的最小接线：

1. `dto/channel_settings.go`
   - 注册 `media_task_image_edit_blocking`。
   - 允许它绑定 `/v1/images/edits`。
2. `relay/channel/advancedcustom/adaptor.go`
   - 在图片转换分支调用新的改图转换器。
3. `relay/image_task_prepare.go`
   - 将持久化图片任务准备扩展到 `RelayModeImagesEdits`。
   - 按实际请求路径匹配生成或改图路由。
4. `router/relay-router.go`
   - 让 `/v1/images/edits` 使用与生成接口一致的 `TaskClientProtocol("openai_images")` 和 `TaskCreateIdempotency()`。
5. 新增公共 `seedream-5` 档位验证文件。
   - 在选渠道前执行相同的生成/改图参数约束。

不需要修改：

- NEWAPI 对外路径。
- `dto.ImageRequest` 的现有字段。
- 同步图片成功响应格式。
- `GET /v1/images/tasks/:task_id` 格式。

## 9. 计费与重试

- 公共模型名统一后，默认模型售价也是统一的，不能按客户端选择的上游分别定价。
- 公共售价应覆盖较贵的兼容渠道，或者明确限制昂贵渠道只作故障备用。
- Moxing 返回有效任务 ID 后，不能盲目重试创建请求，否则可能重复生图并重复扣费。
- 上游创建结果未知时必须保留当前的“不自动退款、不盲重试”保护。
- `Idempotency-Key` 应同时覆盖生成和改图。
- 任务转入异步持久化后，预扣费和最终结算必须沿用当前图片任务链路。

## 10. 验收矩阵

| 用例 | 期望 |
|---|---|
| 生成，`size=2K` | 返回 `200` 或规范化 `202` |
| 生成，`size=3K` | 返回 `200` 或规范化 `202` |
| 生成，省略 `size` | NEWAPI 默认为 `2K` |
| 生成，`n=2` | 选渠道前返回 `400` |
| 生成，`stream=true` | 返回 `400` |
| 改图，单个 PNG/JPEG/WebP | 暂存后映射为 Moxing `image` URL |
| 改图，带 `mask` | 返回 `400` |
| 改图，多文件 | 返回 `400` |
| 改图，上游同步成功 | 返回 `200 + data[].url` |
| 改图，上游返回任务 | 等待成功返回 `200`，超时返回 `202` |
| `Prefer: respond-async` | 上游创建任务后立即返回 NEWAPI `202` |
| 使用同一 `Idempotency-Key` 和同一请求重放 | 返回同一结果或同一任务 |
| 同一 `Idempotency-Key` 配不同请求 | 返回 `409` |
| 上游返回图片数量大于 `n` | 失败关闭，不能多扣费或多返回 |
| 暂存 URL 过期 | 已提交任务不泄漏凭据；失败信息对客户端脱敏 |

## 11. 代码依据

- NEWAPI 图片请求 DTO：`dto/openai_image.go`
- 生成、改图和任务查询路由：`router/relay-router.go`
- 图片请求解析：`relay/helper/valid_request.go`
- 当前 Moxing 请求转换：`relay/channel/advancedcustom/media_task_image_blocking.go`
- Advanced Custom 转换器注册和路径校验：`dto/channel_settings.go`
- Advanced Custom 图片分派：`relay/channel/advancedcustom/adaptor.go`
- 图片任务持久化准备：`relay/image_task_prepare.go`
- NEWAPI 图片任务响应：`relay/image_task_response.go`
- NEWAPI 图片任务 DTO：`dto/image_task.go`
- 图片任务查询：`controller/image_task.go`

## 12. 外部资料

- Moxing 模型文档：https://tokensave.pro/docs/models/seedream-5-0-260128
- Moxing 文档数据接口：https://tokensave.pro/api/docs/models/seedream-5-0-260128
