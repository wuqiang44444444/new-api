---
status: research
owner: Dev Team
last-reviewed: 2026-07-27
---

# Seedream Qihang：NEWAPI 统一生成/改图接口映射

> 本文保留供应商调研过程和历史方案。北向合同、公开模型命名及发布边界以
> [`2026-07-27-NEWAPI统一图片接口生产级多渠道兼容方案.md`](../99-archive/2026-07-27-NEWAPI统一图片接口生产级多渠道兼容方案.md)
> 为唯一事实来源；本文出现的共享 `seedream-5`、multipart 优先或供应商私有字段建议不再作为生产合同。

## 1. 目标与结论

客户端只使用 NEWAPI 北向接口，不感知启航的接口差异：

| 能力 | NEWAPI 北向接口 |
|---|---|
| 文生图 | `POST /v1/images/generations` |
| 改图 | `POST /v1/images/edits` |
| 异步任务查询 | `GET /v1/images/tasks/:task_id`，仅在创建接口返回 `202` 时使用 |

北向统一模型名为 `seedream-5`。启航的上游模型名同样是 `seedream-5`，不需要模型名转换。

启航的两个接口与 NEWAPI 路径基本一致：

- 生成：JSON 请求，`POST /v1/images/generations`
- 改图：Multipart 请求，`POST /v1/images/edits`
- 成功响应：同步 `200`，OpenAI 风格 `data[].url`

因此，启航是两个上游中更接近 NEWAPI 原生协议的一方。生成接口主要可以通过“高级自定义渠道 + 原生透传 + 参数覆盖”配置完成；改图接口只有在北向请求严格限制为 `model + prompt + image` 时才能原生透传。若要接受并过滤更多 NEWAPI 可选字段，应增加启航专用的轻量 Multipart 过滤转换器。

## 2. NEWAPI 北向合同

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

`seedream-5` 统一档位建议：

| 参数 | 北向规则 | 原因 |
|---|---|---|
| `model` | 固定 `seedream-5` | 隐藏供应商版本名 |
| `prompt` | 必填 | 两家共同字段 |
| `n` | 默认并限制为 `1` | 启航正式 Body schema 未承诺批量；Moxing 多图另有私有语义 |
| `size` | 默认 `2K`；允许 `2K`、`3K` | 与 Moxing 的明确值域对齐；启航侧做南向尺寸转换 |
| `response_format` | 默认并限制为 `url` | 两家和当前转换器的共同返回格式 |
| `stream` | 省略或 `false` | 当前统一链路不提供流式图片事件 |
| `user` | 可接收，但只作 NEWAPI 内部审计 | 不依赖上游透传 |
| 其他图像参数 | 对该公共模型返回 `400` | 防止渠道切换后产生不同语义 |

### 2.2 改图

北向保持 NEWAPI 已有的 OpenAI 风格 Multipart 接口：

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
  -F "image=@original.png"
```

第一阶段统一档位：

| 参数 | 北向规则 |
|---|---|
| `model` | 固定 `seedream-5` |
| `prompt` | 必填 |
| `image` | 必填，只允许一个文件 |
| `n` | 默认并限制为 `1` |
| `response_format` | 返回固定为 URL；第一阶段请求中省略，显式传入时返回 `400` |
| `size` | 第一阶段不允许显式指定；各适配器使用各自安全默认值 |
| `mask` | 不支持，传入时返回 `400` |
| `stream` | 省略或 `false` |
| 其他扩展字段 | 不支持，传入时返回 `400` |

限制为单图和 URL 响应，是为了让启航和 Moxing 在渠道切换时保持相同请求合同。Moxing 的多参考图能力可以以后作为单独公共模型或版本化能力开放，不能静默扩张 `seedream-5` 的统一合同。

### 2.3 同步成功响应

生成和改图都统一返回：

```json
{
  "created": 1761223754,
  "data": [
    {
      "url": "https://example.com/result.png"
    }
  ]
}
```

### 2.4 NEWAPI 异步扩展

如果上游在同步等待窗口内未完成，NEWAPI 可以返回：

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

客户端仅在收到 `202` 后轮询：

```http
GET /v1/images/tasks/img_task_xxx
```

启航本身是同步接口，正常情况下直接返回 `200`，不会创建本地轮询任务。`Prefer: respond-async` 是偏好而非强制；启航渠道可以忽略该偏好并继续返回同步 `200`。

## 3. 启航上游合同

### 3.1 文生图

```http
POST https://api.qhaigc.net/v1/images/generations
Authorization: Bearer <QIHANG_KEY>
Content-Type: application/json
```

正式文档列出的 Body：

| 参数 | 类型 | 必填 | 启航语义 |
|---|---|---:|---|
| `model` | string | 是 | 模型名，目标值 `seedream-5` |
| `prompt` | string | 是 | 文本提示词 |
| `size` | string | 是 | `宽x高` |
| `n` | integer | 文档示例 | SDK 示例使用 `1`，正式 Body 表未列出 |

响应为同步 `200`：

```json
{
  "created": 1761223754,
  "data": [
    {
      "url": "https://images.qhaigc.net/result.png"
    }
  ]
}
```

### 3.2 改图

```http
POST https://api.qhaigc.net/v1/images/edits
Authorization: Bearer <QIHANG_KEY>
Content-Type: multipart/form-data
```

| 参数 | 类型 | 必填 |
|---|---|---:|
| `model` | string | 是 |
| `image` | file | 是 |
| `prompt` | string | 是 |

响应同样为同步 `200 + data[].url`。

重要限制：启航的改图文档示例模型是 `nano-banana-1`，没有明确声明 `seedream-5` 可以通过 `/v1/images/edits` 改图。因此在完成真实请求验证前，不应向 NEWAPI 能力表发布“启航 seedream-5 支持改图”。

## 4. 文生图参数映射

| NEWAPI 北向 | 启航南向 | 处理方式 |
|---|---|---|
| `model=seedream-5` | `model=seedream-5` | 原样传递 |
| `prompt` | `prompt` | 原样传递 |
| `n=1` | `n=1` | 可传；如实测上游不接受则删除，返回数量仍必须校验为 1 |
| `size=2K/3K` | `size=<宽x高>` | 通过渠道参数覆盖或轻量转换器映射 |
| `response_format=url` | 未在正式 Body 表中声明 | 不向上游发送；按 URL 响应处理 |
| `stream=false` | 未声明 | 删除 |
| `user` | 无对应字段 | 不向上游发送，仅 NEWAPI 内部审计 |
| `quality/style/watermark/...` | 未声明 | 北向验证阶段拒绝 |

### 4.1 尺寸映射是唯一的配置验证门槛

启航文档要求 `宽x高`，并列出常见尺寸：

- `720x1280`
- `1280x720`
- `1024x1024`
- `1024x1792`
- `1792x1024`

Moxing 和现有转换器要求 `2K/3K`。因此北向不能把供应商尺寸直接暴露给客户端。

上线前按以下顺序验证：

1. 直接探测启航 `seedream-5` 是否接受 `size=2K` 和 `size=3K`。
2. 如果接受，生成接口可完全原生透传，不需要尺寸转换。
3. 如果拒绝，分别确认启航 Seedream 5 对应的 2K、3K 方图尺寸。
4. 将已确认的尺寸写入渠道参数覆盖规则。
5. 未确认前禁止用猜测值投产。

参数覆盖规则形态如下。`<CONFIRMED_*>` 必须替换为实测值；删除规则用于防止 NEWAPI 内部字段被原生透传给未声明这些字段的启航接口：

```json
{
  "operations": [
    {
      "description": "Map NEWAPI Seedream 2K tier to Qihang dimensions",
      "path": "size",
      "mode": "set",
      "value": "<CONFIRMED_QIHANG_2K_SIZE>",
      "logic": "AND",
      "conditions": [
        {
          "path": "request_path",
          "mode": "full",
          "value": "/v1/images/generations"
        },
        {
          "path": "size",
          "mode": "full",
          "value": "2K"
        }
      ]
    },
    {
      "description": "Map NEWAPI Seedream 3K tier to Qihang dimensions",
      "path": "size",
      "mode": "set",
      "value": "<CONFIRMED_QIHANG_3K_SIZE>",
      "logic": "AND",
      "conditions": [
        {
          "path": "request_path",
          "mode": "full",
          "value": "/v1/images/generations"
        },
        {
          "path": "size",
          "mode": "full",
          "value": "3K"
        }
      ]
    },
    {
      "description": "Remove NEWAPI-only response format from Qihang generation",
      "path": "response_format",
      "mode": "delete",
      "conditions": [
        {
          "path": "request_path",
          "mode": "full",
          "value": "/v1/images/generations"
        }
      ]
    },
    {
      "description": "Remove unsupported stream field from Qihang generation",
      "path": "stream",
      "mode": "delete",
      "conditions": [
        {
          "path": "request_path",
          "mode": "full",
          "value": "/v1/images/generations"
        }
      ]
    },
    {
      "description": "Keep NEWAPI user metadata inside the gateway",
      "path": "user",
      "mode": "delete",
      "conditions": [
        {
          "path": "request_path",
          "mode": "full",
          "value": "/v1/images/generations"
        }
      ]
    }
  ]
}
```

## 5. 改图参数映射

启航改图协议与 NEWAPI 的 Multipart 结构一致，确认 Seedream 5 能力后可原生透传：

| NEWAPI 北向 | 启航南向 | 处理方式 |
|---|---|---|
| `model=seedream-5` | `model=seedream-5` | 原样传递 |
| `image` 文件 | `image` 文件 | 保留 MIME、文件名和二进制内容 |
| `prompt` | `prompt` | 原样传递 |
| `n=1` | 未声明 | 不发送 |
| 未显式传 `response_format` | 未声明 | NEWAPI 按固定 URL 响应处理 |
| `size` | 未声明 | 第一阶段北向禁止显式传入 |
| `mask` | 未声明 | 北向直接返回 `400` |
| `stream=false` | 未声明 | 不发送 |

第一阶段只有 `model + prompt + image` 的最小 Multipart 请求可以用 `converter=none`。NEWAPI 必须在选渠道前拒绝其他表单字段，不能把“整个请求体透传”作为绕过验证的手段。

如果未来要接受 `response_format=url` 等 NEWAPI 可选表单字段但仍不向启航发送，应增加独立的轻量 Multipart 过滤转换器。现有参数覆盖只作用于序列化后的 JSON 请求，不能可靠修改已经构造好的 Multipart `bytes.Buffer`。

## 6. 响应、错误和异步映射

| 启航结果 | NEWAPI 北向结果 |
|---|---|
| `200 + data[].url` | 原样规范化为 `200 + {created,data:[{url}]}` |
| `4xx` | OpenAI 风格 `error`，保留合理 HTTP 状态 |
| `5xx`/网络失败 | NEWAPI 上游错误；是否重试由既有渠道策略决定 |
| 同步耗时较长 | 保持连接等待；不伪造任务 ID |
| 客户端传 `Prefer: respond-async` | 启航可忽略偏好并返回 `200` |

不得把启航同步请求包装成伪轮询，除非未来为所有同步渠道实现统一的本地异步执行队列。当前最小入侵方案是：同步上游返回同步结果，异步上游才使用 NEWAPI 图片任务合同。

## 7. 渠道配置方案

当前渠道 25 是 Gemini 类型，并同时承载聊天模型。普通渠道不会按请求路径过滤；如果继续让它暴露 `seedream-5`，图片请求可能被 Gemini 适配器选中。

推荐配置：

1. 渠道 25 保留 Gemini 聊天模型，移除 `seedream-5`。
2. 新建启航图片专用的“高级自定义”渠道。
3. 只为该渠道配置图片生成和改图路径。
4. 改图能力在 Seedream 5 实测通过后再启用。

生成接口当前可用的目标配置如下。改图路由只有在 Seedream 5 能力实测通过、且北向严格限制为最小 Multipart 字段时才可以加入：

```json
{
  "advanced_custom": {
    "advanced_routes": [
      {
        "incoming_path": "/v1/images/generations",
        "upstream_path": "/v1/images/generations",
        "converter": "none",
        "models": [
          "seedream-5"
        ],
        "auth": {
          "type": "header",
          "name": "Authorization",
          "value": "Bearer {api_key}"
        }
      },
      {
        "incoming_path": "/v1/images/edits",
        "upstream_path": "/v1/images/edits",
        "converter": "none",
        "models": ["seedream-5"],
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

生成路由在尺寸探测完成后启用。改图路由在 Seedream 5 改图探测完成后启用；如果要支持最小字段之外的 NEWAPI Multipart 请求，则将 `none` 替换为届时新增并注册的启航过滤转换器。

## 8. 需要的 NEWAPI 公共保护

即使启航大部分可以透传，也需要一层公共模型档位验证，确保负载均衡到 Moxing 时行为不变：

- 对 `seedream-5` 生成请求执行统一默认值和允许值校验。
- 对 `seedream-5` 改图请求限制单文件、`n=1`、URL 返回、非流式。
- 未支持字段返回 `400`，不能静默丢弃。
- 验证逻辑应放入新增独立文件；现有请求热路径只增加一次窄调用。
- 不修改 `dto.ImageRequest` 和现有北向路由定义。

这部分不能仅放在启航渠道参数覆盖中，因为参数覆盖发生在选定渠道之后，无法保证不同渠道收到同一非法请求时给出一致结果。

## 9. 验收矩阵

| 用例 | 期望 |
|---|---|
| 生成，`size=2K` | 启航返回 `200 + data[0].url` |
| 生成，`size=3K` | 启航返回 `200 + data[0].url` |
| 生成，省略 `size` | NEWAPI 默认为 `2K` 后成功 |
| 生成，`n=2` | NEWAPI 在选渠道前返回 `400` |
| 生成，`response_format=b64_json` | NEWAPI 返回 `400` |
| 生成，`stream=true` | NEWAPI 返回 `400` |
| 改图，单图片文件 | 确认启航 `seedream-5` 能否成功 |
| 改图，带 `mask` | NEWAPI 返回 `400` |
| 改图，多图片文件 | NEWAPI 返回 `400` |
| 启航上游 `4xx` | 返回规范化 OpenAI 错误，不切换到语义不同渠道重试 |
| 启航上游 `5xx` | 按既有重试策略尝试兼容渠道 |

## 10. 代码依据

- NEWAPI 图片请求 DTO：`dto/openai_image.go`
- 生成、改图和图片任务路由：`router/relay-router.go`
- 图片请求解析：`relay/helper/valid_request.go`
- Advanced Custom 路由配置：`dto/channel_settings.go`
- Advanced Custom 原生图片和 Multipart 转发：`relay/channel/advancedcustom/adaptor.go`
- 普通渠道与 Advanced Custom 的路径过滤差异：`middleware/distributor.go`
- 参数覆盖：`relay/common/override.go`

## 11. 外部资料

- 启航生成接口：https://www.qhaigc.net/docs/api-reference/images/generate
- 启航改图接口：https://www.qhaigc.net/docs/api-reference/images/edit
- 启航模型列表：https://www.qhaigc.net/models
