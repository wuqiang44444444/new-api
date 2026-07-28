---
status: current
owner: Dev Team
last-reviewed: 2026-07-28
---

# 图片模型 API 用户调用指南

## 1. 适用范围

本文面向使用 new-api 图片模型的客户端、SDK 和业务系统，说明统一图片生成、URL 参考图编辑、异步任务查询和错误处理方式。

普通调用方只需要了解：

- 平台 Base URL。
- 自己的 API Key。
- 公开模型名。
- 本文定义的统一图片字段。

调用方不需要也不应持有供应商密钥、渠道 ID、上游模型 ID或上游任务 ID。

本文重点覆盖以下公开模型：

| 模型 | 主要能力 | 当前稳定入口 |
| --- | --- | --- |
| `seedream-5-moxing` | 文生图、URL 参考图编辑 | `POST /v1/images/generations` |
| `seedream-5-qihang` | 文生图、URL 参考图编辑 | `POST /v1/images/generations` |
| `nano-banana-2` | 文生图、URL 参考图编辑 | `POST /v1/images/generations` |

模型是否对当前 API Key开放，以 `GET /v1/models` 的实时结果为准。未出现在模型列表中的模型可能尚未向当前分组开放。

## 2. 快速开始

### 2.1 地址和认证

```bash
export NEWAPI_BASE_URL="https://api.example.com"
export NEWAPI_API_KEY="sk-your-key"
```

所有请求使用 Bearer Token：

```http
Authorization: Bearer sk-your-key
```

不要把 API Key放在 URL、前端浏览器代码、日志或公开代码仓库中。

### 2.2 查询可用模型

```bash
curl -sS "$NEWAPI_BASE_URL/v1/models" \
  -H "Authorization: Bearer $NEWAPI_API_KEY"
```

只调用响应 `data[].id` 中实际存在的模型。模型列表会受到账号分组、灰度范围和管理员配置影响。

### 2.3 最小文生图请求

三个目标模型使用相同的北向路径和字段；切换模型时通常只需修改 `model`：

```bash
curl -sS "$NEWAPI_BASE_URL/v1/images/generations" \
  -H "Authorization: Bearer $NEWAPI_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "seedream-5-moxing",
    "prompt": "一只放在白色桌面上的蓝色陶瓷杯，产品摄影，柔和棚拍光线",
    "size": "2K",
    "n": 1,
    "response_format": "url",
    "stream": false
  }'
```

同步成功返回 HTTP `200`：

```json
{
  "created": 1785207890,
  "data": [
    {
      "url": "https://cdn.example.com/generated-image.png"
    }
  ]
}
```

结果 URL可能有有效期。业务系统应按自身合规要求及时下载或转存，不要把供应商临时 URL当作永久存储。

## 3. 统一请求字段

```http
POST /v1/images/generations
Content-Type: application/json
```

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | ---: | --- |
| `model` | string | 是 | 使用 `GET /v1/models` 返回的公开模型名 |
| `prompt` | string | 是 | 生成或编辑指令；建议不超过 3000 个字符 |
| `image` | string 或 string[] | 否 | HTTP(S) 参考图 URL；建议始终使用数组 |
| `size` | string | 是 | 值域由模型决定，见模型能力表 |
| `n` | integer | 否 | 当前稳定合同使用 `1` |
| `response_format` | string | 否 | 当前使用 `url` |
| `stream` | boolean | 否 | 当前使用 `false` 或省略 |
| `watermark` | boolean | 否 | 仅在模型能力表明确支持时使用 |
| `aspect_ratio` | string | 否 | 仅在模型能力表明确支持时使用 |

统一语义不表示所有模型拥有相同的尺寸和高级能力。客户端应根据模型能力表选择值，不要把某个模型的专有参数直接用于另一个模型。

当前三个目标模型不承诺以下字段：

- `quality`
- `style`
- `background`
- `mask`
- `input_fidelity`
- `output_format`
- `output_compression`
- `partial_images`
- `stream=true`
- `response_format=b64_json`

显式发送不支持的字段可能返回 HTTP `400`，平台不会静默伪造能力。

## 4. 模型能力

### 4.1 能力矩阵

| 模型 | 尺寸 | URL 参考图 | 模型专有参数 | 响应特点 |
| --- | --- | --- | --- | --- |
| `seedream-5-moxing` | `2K`、`3K` | 最多 14 个；当前已验收单图 | `watermark` | 可能同步完成，也可能进入异步任务 |
| `seedream-5-qihang` | 当前公开并已验收 `2K` | 当前已验收单图和双图 | 暂无公开高级参数 | 同步 HTTP `200`，生成时间可能较长 |
| `nano-banana-2` | `1K`、`2K`、`4K` | 最多 10 个 | `aspect_ratio` | 可能进入异步任务 |

当前稳定合同建议：

- `n` 固定为 `1`。
- `response_format` 固定为 `url`。
- 新业务优先从 `2K` 和单张参考图开始验证。
- 使用额外尺寸或多参考图前，先确认模型已经向当前 API Key开放，并完成业务侧成本与时延评估。

### 4.2 Nano Banana 2 画面比例

`nano-banana-2` 可使用：

```text
1:1
4:3
3:4
16:9
9:16
3:2
2:3
21:9
```

示例：

```json
{
  "model": "nano-banana-2",
  "prompt": "一片漂浮在水面上的绿色叶子，极简摄影",
  "size": "2K",
  "aspect_ratio": "16:9",
  "n": 1,
  "response_format": "url"
}
```

### 4.3 Seedream 水印

只有模型能力明确支持时才发送 `watermark`。例如：

```json
{
  "model": "seedream-5-moxing",
  "prompt": "城市夜景，电影感摄影",
  "size": "2K",
  "n": 1,
  "response_format": "url",
  "watermark": false
}
```

不要假设另一个 Seedream SKU必然接受相同高级参数。

## 5. 使用 URL 参考图进行编辑

### 5.1 调用方式

这三个 URL-only 模型的生成与编辑使用同一个 JSON 入口：

```http
POST /v1/images/generations
```

是否为编辑请求由 `image` 字段决定。客户端不需要改用供应商路径或供应商私有字段。

```bash
curl -sS "$NEWAPI_BASE_URL/v1/images/generations" \
  -H "Authorization: Bearer $NEWAPI_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "nano-banana-2",
    "prompt": "保持主体形状不变，把主色改为橙色，并增加细微纸张纹理",
    "image": [
      "https://cdn.example.com/reference.png"
    ],
    "size": "1K",
    "n": 1,
    "response_format": "url",
    "stream": false
  }'
```

更换为其他目标模型时，`image` 的字段语义保持不变：

```json
{
  "model": "seedream-5-qihang",
  "prompt": "保持台灯结构不变，把灯罩改为深蓝色",
  "image": [
    "https://cdn.example.com/lamp.png"
  ],
  "size": "2K",
  "n": 1,
  "response_format": "url"
}
```

### 5.2 参考图 URL要求

参考图必须：

- 使用 `http://` 或 `https://`。
- 能被图片模型服务从公网访问。
- 在整个生成期间保持有效。
- 返回真实图片内容，而不是登录页、HTML 错误页或需要 Cookie 的页面。

不要发送：

- 本地文件路径，例如 `/Users/me/image.png`。
- `file://` URL。
- 只能从公司内网访问的地址。
- 已过期或即将过期的签名 URL。
- data URI或 Base64 字符串。

使用短期签名 URL时，建议有效期至少覆盖客户端超时、排队和生成时间，并预留重试余量。

### 5.3 多参考图

多参考图仍使用一个 `image` 数组：

```json
{
  "model": "seedream-5-moxing",
  "prompt": "使用图一的主体和图二的背景生成一张产品图",
  "image": [
    "https://cdn.example.com/subject.png",
    "https://cdn.example.com/background.png"
  ],
  "size": "2K",
  "n": 1,
  "response_format": "url"
}
```

不要同时发送 `image`、`images` 或供应商字段 `reference_images`。新客户端只使用 `image`。

## 6. 同步与异步响应

### 6.1 默认行为

创建请求可能返回两种合法结果：

| HTTP 状态 | 含义 | 客户端动作 |
| --- | --- | --- |
| `200` | 图片已生成 | 读取 `data[].url` |
| `202` | 已创建异步任务 | 保存任务 ID并轮询任务 |

平台可能先在内部等待异步任务完成。如果任务在等待窗口内完成，客户端最终仍会收到 HTTP `200`。

### 6.2 优先异步返回

不希望长时间保持创建连接时，可以发送：

```http
Prefer: respond-async
```

```bash
curl -sS "$NEWAPI_BASE_URL/v1/images/generations" \
  -H "Authorization: Bearer $NEWAPI_API_KEY" \
  -H "Content-Type: application/json" \
  -H "Prefer: respond-async" \
  -H "Idempotency-Key: image-order-20260728-001" \
  -d '{
    "model": "nano-banana-2",
    "prompt": "一枚绿色叶子图标，白色背景",
    "size": "1K",
    "n": 1,
    "response_format": "url"
  }'
```

`Prefer` 是偏好而不是强制。模型如果只提供同步执行，仍可能在生成完成后直接返回 HTTP `200`。客户端创建请求的读取超时建议至少设置为 180 秒；高峰期或复杂编辑可设置得更长。

### 6.3 HTTP 202 响应

响应头：

```http
Location: /v1/images/tasks/task_xxx
Retry-After: 2
X-Task-ID: task_xxx
```

响应体：

```json
{
  "id": "task_xxx",
  "object": "image.generation.task",
  "status": "queued",
  "created_at": 1785208013,
  "model": "nano-banana-2"
}
```

任务 ID是平台公开 ID。不要尝试使用供应商任务 ID或自行拼接供应商查询路径。

## 7. 查询异步任务

```bash
curl -sS "$NEWAPI_BASE_URL/v1/images/tasks/task_xxx" \
  -H "Authorization: Bearer $NEWAPI_API_KEY"
```

状态：

```text
queued -> in_progress -> completed
                      -> failed
                      -> unknown
```

完成响应：

```json
{
  "id": "task_xxx",
  "object": "image.generation.task",
  "status": "completed",
  "created_at": 1785208013,
  "completed_at": 1785208032,
  "model": "nano-banana-2",
  "result": {
    "created": 1785208032,
    "data": [
      {
        "url": "https://cdn.example.com/result.png"
      }
    ]
  }
}
```

失败响应示例：

```json
{
  "id": "task_xxx",
  "object": "image.generation.task",
  "status": "failed",
  "created_at": 1785208013,
  "completed_at": 1785208032,
  "model": "nano-banana-2",
  "error": {
    "code": "image_generation_failed",
    "message": "Image generation failed"
  }
}
```

轮询建议：

1. 优先读取 `Retry-After`。
2. 首次等待 2 秒。
3. 随后每 5 秒查询一次。
4. 收到 `429`、`502` 或 `503` 时使用指数退避，最长间隔建议 15 秒。
5. 任务进入终态后停止轮询。

客户端超时或用户关闭页面不代表服务端任务失败。业务系统应保存任务 ID，以便稍后继续查询。

## 8. 幂等语义

创建图片时可以发送：

```http
Idempotency-Key: <业务侧唯一请求号>
```

但同步与异步语义不同。

### 8.1 异步持久化任务

当平台创建了本地图片 Task：

- 同 Key、同请求可以返回已有任务或已有结果。
- 不会重复创建上游任务。
- 不会重复计费。
- 同 Key、不同请求返回 HTTP `409 idempotency_conflict`。

网络异常后应复用原 Key和完全相同的请求体，不要立即换 Key创建新任务。

### 8.2 同步 HTTP 200

同步图片成功后不会创建 Task，也不会保存响应用于幂等回放。平台会完成计费并释放临时幂等 claim。

因此：

- 客户端收到 HTTP `200` 后，不要再次提交同一业务请求。
- 如果响应在到达客户端前断网，以同一 Key重试仍可能创建新图片并再次计费。
- `Idempotency-Key` 不能被理解为同步图片的 exactly-once 或结果存储保证。
- HTTP 客户端不要对图片创建 POST开启无条件自动重试。

业务上必须避免重复图片时，应在客户端保存业务请求状态、平台请求 ID、返回 URL或异步 Task ID，并让人工或补偿流程处理同步结果不确定的情况。

## 9. Python 完整示例

以下示例同时处理 HTTP `200` 和 `202`：

```python
import os
import time
import requests

BASE_URL = os.environ["NEWAPI_BASE_URL"].rstrip("/")
API_KEY = os.environ["NEWAPI_API_KEY"]

headers = {
    "Authorization": f"Bearer {API_KEY}",
    "Content-Type": "application/json",
    "Prefer": "respond-async",
    "Idempotency-Key": "image-order-20260728-001",
}

payload = {
    "model": "nano-banana-2",
    "prompt": "保持主体不变，把主色改为橙色",
    "image": ["https://cdn.example.com/reference.png"],
    "size": "1K",
    "n": 1,
    "response_format": "url",
    "stream": False,
}

response = requests.post(
    f"{BASE_URL}/v1/images/generations",
    headers=headers,
    json=payload,
    timeout=360,
)
response.raise_for_status()
body = response.json()

if response.status_code == 200:
    image_urls = [item["url"] for item in body["data"]]
else:
    task_id = body["id"]
    poll_headers = {"Authorization": f"Bearer {API_KEY}"}
    wait_seconds = int(response.headers.get("Retry-After", "2"))

    while True:
        time.sleep(wait_seconds)
        task_response = requests.get(
            f"{BASE_URL}/v1/images/tasks/{task_id}",
            headers=poll_headers,
            timeout=30,
        )
        task_response.raise_for_status()
        task = task_response.json()

        if task["status"] == "completed":
            image_urls = [item["url"] for item in task["result"]["data"]]
            break
        if task["status"] in {"failed", "unknown"}:
            raise RuntimeError(task.get("error", {"message": "Image task failed"}))

        wait_seconds = min(max(wait_seconds, 5), 15)

print(image_urls)
```

生产代码还应：

- 为 `429`、`502`、`503` 增加有限次数的指数退避。
- 记录平台返回的请求 ID和 Task ID。
- 不记录完整 API Key和带签名的图片 URL。
- 将同步 POST重试与异步 Task查询重试分开处理。

## 10. 错误处理

统一错误通常使用：

```json
{
  "error": {
    "message": "size must be one of 1K, 2K or 4K",
    "type": "new_api_error",
    "code": "invalid_request"
  }
}
```

常见状态：

| HTTP 状态 | 常见原因 | 建议 |
| --- | --- | --- |
| `400` | 字段、尺寸、参考图 URL或模型能力不合法 | 修正请求，不要原样重试 |
| `401` | API Key缺失、无效或过期 | 更新认证信息 |
| `403` | 当前账号或分组没有权限 | 检查账号和模型开放范围 |
| `404` | 模型或图片 Task不存在 | 检查公开模型名、Task ID和资源归属 |
| `409` | 幂等 Key冲突或原请求仍在协调 | 保留原请求信息，不要随意换 Key重建 |
| `429` | 频率或并发限制 | 指数退避并遵守 `Retry-After` |
| `500` | 平台内部错误 | 记录请求 ID并联系管理员 |
| `502` | 上游响应异常或创建结果未知 | 不要盲目换模型或换 Key重复创建 |
| `503` | 临时不可用或系统保护 | 退避后重试；同步结果不确定时先核对账单 |

错误消息只描述北向公开字段，不应出现供应商密钥、内部渠道 ID、上游模型 ID或上游任务 ID。

## 11. `/v1/images/edits` 与本指南模型的关系

`POST /v1/images/edits` 是标准 multipart 图片编辑入口，继续供明确支持 OpenAI multipart 合同的模型使用，例如已有的 OpenAI 兼容图片模型：

```bash
curl -sS "$NEWAPI_BASE_URL/v1/images/edits" \
  -H "Authorization: Bearer $NEWAPI_API_KEY" \
  -F "model=gpt-image-2" \
  -F "prompt=把背景改成白色" \
  -F "image=@./original.png"
```

本指南的三个 URL-only 模型：

```text
seedream-5-moxing
seedream-5-qihang
nano-banana-2
```

使用 `/v1/images/generations` + JSON `image` URL，不接受本地文件上传，也不要求平台为上游模拟文件暂存。

## 12. 计费与结果保存

- 实际价格以控制台价格页和当前账号分组为准。
- 不要在客户端写死本文撰写时的测试价格。
- 图片尺寸、数量和模型可能影响价格。
- 异步任务只由 Task完成一次结算；轮询不会重复计费。
- 同步请求在成功返回时完成结算；重复 POST可能再次计费。
- 返回 URL可能过期，平台计费成功不代表承担永久图片托管责任。

对成本敏感的业务建议：

1. 上线前分别验证所需模型、尺寸和参考图数量。
2. 为每个业务订单生成稳定的幂等 Key。
3. 禁止 HTTP 库自动重试图片创建 POST。
4. 记录模型、尺寸、Task ID、请求 ID和业务订单号。
5. 使用平台账单核对每次创建，而不是把轮询次数当作计费次数。
