---
page-id: media-workflow
kind: guide
last-verified: 2026-09-09
operations: []
---

# 图片与视频调用实战

本页按“选模型 → 提交 → 保存任务 ID → 查询 → 保存结果”说明如何接入。
示例只使用公开模型和接口。请先将占位 Key、模型 ID、素材 URL 和任务 ID 换为自己的值。

## 1. 准备地址与 API Key

下列命令在终端运行。生产应用将 Key 放在服务端环境变量或密钥管理系统中：

```bash
export MEDIA_API_BASE="{{SITE_BASE_URL}}"
export MEDIA_API_KEY="{{API_KEY_PLACEHOLDER}}"
export MEDIA_MODEL="{{MODEL_ID_PLACEHOLDER}}"
```

`MEDIA_API_BASE` 使用站点根地址，不含 `/v1`。本文据此拼接完整路径；其他页面使用
`{{OPENAI_BASE_URL}}` 时，它已包含 `/v1`，不要重复添加。示例需要 curl；解析 JSON 和轮询示例使用
Python 3 标准库，无需安装 SDK。

API Key 必须有模型访问权限和足够额度。图片编辑还需准备可读取的本地图片，或有效期覆盖处理过程的公网
HTTPS 参考图。不要把 API Key 放在媒体 URL 中。

## 2. 确认模型和参数

先查询可访问的模型：

```bash
curl --fail-with-body "$MEDIA_API_BASE/v1/models" \
  -H "Authorization: Bearer $MEDIA_API_KEY"
```

使用模型返回的 `id` 更新 `MEDIA_MODEL`。读取单模型详情时对模型名进行 URL 路径编码：

```bash
python3 - <<'PY'
import os
import urllib.parse
import urllib.request

model = urllib.parse.quote(os.environ["MEDIA_MODEL"], safe="")
url = os.environ["MEDIA_API_BASE"].rstrip("/") + "/v1/models/" + model
request = urllib.request.Request(url, headers={
    "Authorization": "Bearer " + os.environ["MEDIA_API_KEY"]
})
with urllib.request.urlopen(request, timeout=30) as response:
    print(response.read().decode("utf-8"))
PY
```

| 想做什么 | 检查哪些公开字段 |
| --- | --- |
| 生成图片 | `available` 返回时为 `true`，`api.image.operations` 中 `create_image.supported=true` |
| 编辑图片 | `edit_image.supported=true`，并读取 `api.image.edit` |
| 图片异步任务 | 存在 `api.image.async`，按其中的请求头和值选择异步 |
| ModelArk V3 视频 | `api.video.protocol=modelark_v3`，`create_video.supported=true` |
| 视频参考素材 | `api.assets.supported=true`，并核对媒体类型、操作与管理模式 |

按 `parameters` 中的 `required`、`fixed_value`、`default_value`、`enum`、上下限组装请求。
`additional_properties=false` 时不要发送未列出的字段；带点的参数名代表嵌套字段。
不要依据模型名称猜测功能，也不要直接把图片的 `size`、视频的 `resolution` 和 `ratio` 互换。
字段含义与适用条件见[模型与参数](concepts/model-parameters)。

## 3. 创建图片：同步或异步二选一

把下面内容保存为 `image-request.json`，替换模型 ID。第一次调用先使用最小参数，之后按模型合同增加
尺寸、质量或数量：

```json
{
  "model": "{{MODEL_ID_PLACEHOLDER}}",
  "prompt": "白色桌面上的蓝色陶瓷杯，柔和自然光，产品摄影"
}
```

同步调用：

```bash
curl --fail-with-body "$MEDIA_API_BASE/v1/images/generations" \
  -H "Authorization: Bearer $MEDIA_API_KEY" \
  -H "Content-Type: application/json" \
  --data-binary @image-request.json
```

成功为 HTTP `200`，从 `data[]` 读取 `url` 或 `b64_json`。同步没有平台任务 ID。
JSON 中的模型 ID 不会自动从环境变量替换，必须在文件中填入真实的公开模型 ID。

如果业务希望提交后断开连接，并且模型声明支持异步，改用以下请求；不要把两条命令都执行为同一个订单：

```bash
curl --fail-with-body \
  -D image-create.headers -o image-create.json -w '%{http_code}\n' \
  "$MEDIA_API_BASE/v1/images/generations" \
  -H "Authorization: Bearer $MEDIA_API_KEY" \
  -H "Content-Type: application/json" \
  -H "Prefer: respond-async" \
  -H "Idempotency-Key: image-order-example-001" \
  --data-binary @image-request.json
```

确认状态码为 `202` 且正文 `object=image_task`，再保存 `id` 和 `query_url`。
若是 `200`，按同步结果处理；不能虚构任务 ID。若请求超时，只有明确支持异步且使用了幂等键，
才能用原键和原文件确认同次受理；不要改键或自动切换为同步。

图片编辑沿用这套流程，将路径改为 `/v1/images/edits` 并提供参考图。文件示例：

```bash
curl --fail-with-body \
  -D image-create.headers -o image-create.json -w '%{http_code}\n' \
  "$MEDIA_API_BASE/v1/images/edits" \
  -H "Authorization: Bearer $MEDIA_API_KEY" \
  -H "Prefer: respond-async" \
  -H "Idempotency-Key: edit-order-example-001" \
  -F "model=$MEDIA_MODEL" \
  -F "prompt=把背景改成浅灰色，保留杯子外观" \
  -F "image=@input.png"
```

不要手工设置 multipart 的 `Content-Type`。JSON 多参考图、遮罩与输入上限见
[图片编辑](api-reference/images/edits)。

## 4. 创建视频

以下步骤用于 ModelArk V3。若模型公开的是其他协议，请使用相应视频文档中的路径和字段。
把下面内容保存为 `video-request.json`，替换模型 ID，并确认示例规格在模型范围内：

```json
{
  "model": "{{MODEL_ID_PLACEHOLDER}}",
  "content": [
    {"type": "text", "text": "镜头缓慢环绕蓝色陶瓷杯，柔和光线，背景整洁"}
  ],
  "duration": 5,
  "resolution": "720p",
  "ratio": "16:9"
}
```

发送一次创建请求：

```bash
curl --fail-with-body \
  -D video-create.headers -o video-create.json -w '%{http_code}\n' \
  "$MEDIA_API_BASE/api/v3/contents/generations/tasks" \
  -H "Authorization: Bearer $MEDIA_API_KEY" \
  -H "Content-Type: application/json" \
  --data-binary @video-request.json
```

成功为 HTTP `200`，响应形如 `{"id":"task-public-id"}`，此时视频尚未完成。保存原始 `id`，
不要添加图片异步偏好或假设图片幂等机制适用于视频。

图生视频在 `content` 中加入 `image_url` 项；角色可为模型公开的首帧、末帧或参考图。
多模态参考与完整参数见[ModelArk V3 视频](api-reference/videos/modelark)。
使用托管图片时先[创建素材](api-reference/assets)，拿到 `reference` 后放入 `image_url.url`。
直接 URL 生成不会自动创建可复用素材。

## 5. 用 Python 轮询原任务

将下面脚本保存为 `poll_media.py`。它只执行 GET，不会重新创建任务。使用当前步骤对应的任务 ID：

```bash
python3 poll_media.py image task_xxxxxxxx
python3 poll_media.py video task-public-id
```

两条命令分别演示图片与 ModelArk 视频；选择自己已创建的任务运行。脚本使用创建时相同的
`MEDIA_API_BASE` 和 `MEDIA_API_KEY`，总等待最多约 20 分钟，结果保存在 `media-result.json`。

```python
import json
import os
import random
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

if len(sys.argv) != 3 or sys.argv[1] not in ("image", "video"):
    raise SystemExit("用法: python3 poll_media.py image|video TASK_ID")

kind, task_id = sys.argv[1:]
base = os.environ["MEDIA_API_BASE"].rstrip("/")
key = os.environ["MEDIA_API_KEY"]
prefix = "/v1/tasks/" if kind == "image" else "/api/v3/contents/generations/tasks/"
url = base + prefix + urllib.parse.quote(task_id, safe="")
active = {"queued", "in_progress"} if kind == "image" else {"queued", "running"}
terminal = {"succeeded", "failed", "expired", "unknown"} if kind == "image" else {"succeeded", "failed", "expired", "cancelled"}
deadline = time.monotonic() + 20 * 60
delay = 2.0

while time.monotonic() < deadline:
    request = urllib.request.Request(url, headers={"Authorization": "Bearer " + key})
    try:
        with urllib.request.urlopen(request, timeout=60) as response:
            result = json.load(response)
    except urllib.error.HTTPError as error:
        if error.code != 429 and error.code < 500:
            raise SystemExit(f"查询被拒绝，HTTP {error.code}；核对 ID 和创建时的 Key")
        print(f"查询暂不可用，HTTP {error.code}；仅重试 GET")
    except (urllib.error.URLError, TimeoutError):
        print("查询网络异常；仅重试 GET")
    except (ValueError, UnicodeError):
        raise SystemExit("查询响应无法解析；保留任务 ID，不重新创建")
    else:
        if not isinstance(result, dict) or result.get("id") != task_id:
            raise SystemExit("查询结果与原任务不匹配；保留 ID 并核查")
        if kind == "image" and result.get("object") != "image_task":
            raise SystemExit("响应不是图片任务；保留 ID 并核对所用协议")
        status = result.get("status")
        if status not in active and status not in terminal:
            raise SystemExit("收到未识别状态；保留 ID 并核查，不重新创建")
        print("任务状态:", status)
        if status in terminal:
            # 结果可能含临时下载地址，只保存在应用受保护的本地文件中。
            fd = os.open("media-result.json", os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
            with os.fdopen(fd, "w", encoding="utf-8") as output:
                json.dump(result, output, ensure_ascii=False, indent=2)
            if status == "unknown":
                print("结果待核实；可检查已保存的部分图片，请联系管理员")
            elif status != "succeeded":
                print("任务未成功；查看结果中的公开 error")
            else:
                print("生成成功；检查结果字段并及时下载")
            break
    remaining = deadline - time.monotonic()
    if remaining > 0:
        time.sleep(min(remaining, delay + random.uniform(0, 1)))
    delay = min(delay * 1.5, 15)
else:
    print("本地等待结束；保留任务 ID，稍后运行同一查询命令")
```

单次 GET 最长等待 60 秒；客户端等待结束不取消任务。脚本在图片 `unknown` 时退出，保存可能存在的
部分结果；后续可以重新查询，但不要自动重新生成。真实应用应为不同任务分别保存结果文件。

## 6. 保存图片或视频

图片：遍历 `data[]`，只读取 `status=available` 的图片。URL 有效期以 `url_expires_at` 为准，图片任务
URL 每次签发有效 300 秒；过期重新 GET 任务获取新地址。Base64 按原文解码，不带 Data URL 前缀。
签名 URL 下载不要附带平台 API Key。逐张下载、Base64 解码和续签重试的完整脚本见
[图片结果保存](guides/image-results)，详细字段见[图片任务查询](images/tasks)。

视频：确认 `status=succeeded`，使用返回的 `content.video_url`，或以下鉴权内容入口：

```bash
curl --fail "$MEDIA_API_BASE/v1/videos/task-public-id/content" \
  -H "Authorization: Bearer $MEDIA_API_KEY" \
  --output result.mp4
```

只有响应提供 `content.last_frame_url` 时才下载末帧。视频代理使用平台 Key；外部签名图片地址不附 Key。
两类下载均检查 HTTP 状态和 `Content-Type`，失败 JSON 不能当作图片或视频保存。

## 7. 常见问题

| 现象 | 处理方式 |
| --- | --- |
| 图片带了异步偏好却返回 `200` | 检查模型异步声明，按实际同步结果处理 |
| 图片异步返回 `409 idempotency_conflict` | 同键对应的请求内容不同；先确认原订单，不自动改键 |
| 图片异步返回 `409 idempotency_in_progress` | 原受理尚未确认，等待后使用原键原内容确认 |
| 查询 `404` | 核对查询路径、任务 ID 和创建时的同一 Key |
| 创建超时或视频 `create_outcome_unknown` | 保留业务订单和公开请求 ID，停止盲目重发并核查 |
| 图片 `unknown` | 不等于失败或退款，保留部分结果并核查 |
| 图片 URL 过期 | 查询原任务续签，不重新生成 |
| 视频成功但没有 `usage` | 不代表免费，等待费用核实；不重新生成 |
| 多图、分辨率或字段返回 `400` | 读取当前模型参数，按明确支持范围修正请求 |

排障时只提供公开任务 ID、请求 ID、接口、时间和错误码，不提交 Key、完整签名 URL 或媒体原文。
