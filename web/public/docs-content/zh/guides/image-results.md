---
page-id: image-results
kind: guide
last-verified: 2026-09-19
operations: []
---

# 图片结果保存

图片生成成功后，应把可用结果保存到自己的存储。本文脚本可处理同步响应的 `data[]`，以及
[图片任务查询](images/tasks)响应中的可用图片；只下载或解码结果，不会创建任务。

## 1. 准备结果文件

将同步 HTTP `200` 的 JSON 正文保存为 `image-result.json`，或将任务查询结果保存为此文件。
[调用实战](guides/media-workflow)中的轮询脚本写入 `media-result.json`，可直接使用该文件。
`502 image_delivery_failed` 的正文若带有原始图片 `data[]`，也可保存到此文件后运行脚本。
不要把创建时 `202` 的受理正文当作图片结果，也不要把视频响应交给本脚本。

响应中的 URL 可能携带临时下载授权，应限制结果文件的读取权限，不上传到公开日志或诊断系统。

例如，将同步请求正文保存为 `image-request.json`（需要 URL 时设置 `"response_format": "url"`），
并把 HTTP 响应正文保存下来：

```bash
umask 077
curl --fail-with-body -o image-result.json -w '%{http_code}\n' \
  "{{OPENAI_BASE_URL}}/images/generations" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  -H "Content-Type: application/json" \
  --data-binary @image-request.json
```

`--fail-with-body` 在 HTTP 错误时仍保存正文。不要用 `&&` 把取图脚本只绑定到 curl 成功退出：
`502 image_delivery_failed` 也可能带可恢复图片。先检查 HTTP 状态与 `error.code`，再处理 `data`；
普通错误没有结果时停止，不自动重发生成。结果字段为 `url` 或 `b64_json`，不是 `image_url`。

## 2. 逐张保存 URL 或 Base64 图片

将以下内容保存为 `save_images.py`，需要 Python 3 标准库。第二个参数是输出目录；脚本使用结果在
数组中的序号命名文件，遇到已保存项会跳过，单张失败不会阻止保存其他图片。

```bash
python3 save_images.py image-result.json ./image-output
```

```python
import base64
import binascii
import json
import sys
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path


def image_type(data):
    if data.startswith(b"\x89PNG\r\n\x1a\n"):
        return "image/png", ".png"
    if data.startswith(b"\xff\xd8\xff"):
        return "image/jpeg", ".jpg"
    if data[:4] == b"RIFF" and data[8:12] == b"WEBP":
        return "image/webp", ".webp"
    if data[:6] in (b"GIF87a", b"GIF89a"):
        return "image/gif", ".gif"
    if data.startswith(b"BM"):
        return "image/bmp", ".bmp"
    if data[:4] in (b"II*\x00", b"MM\x00*"):
        return "image/tiff", ".tiff"
    raise ValueError("内容不是已支持的图片格式，未写入文件")


if len(sys.argv) != 3:
    raise SystemExit("用法: python3 save_images.py RESULT_JSON OUTPUT_DIR")
result = json.loads(Path(sys.argv[1]).read_text(encoding="utf-8"))
if not isinstance(result, dict) or not isinstance(result.get("data"), list):
    raise SystemExit("没有图片结果数组；检查是否误用了创建受理或错误响应")
if result.get("object") not in (None, "image_task"):
    raise SystemExit("响应不是图片结果")
output = Path(sys.argv[2])
output.mkdir(parents=True, exist_ok=True, mode=0o700)
failed = False
for index, item in enumerate(result["data"], start=1):
    if not isinstance(item, dict):
        print(f"第 {index} 项结构无效")
        failed = True
        continue
    if item.get("status", "available") != "available":
        print(f"第 {index} 张当前不可下载；保留任务 ID 后再查询")
        failed = True
        continue
    if list(output.glob(f"image-{index}.*")):
        print(f"第 {index} 张已有本地文件，跳过")
        continue
    try:
        if item.get("b64_json"):
            data = base64.b64decode(item["b64_json"], validate=True)
        else:
            url = item.get("url", "")
            if urllib.parse.urlsplit(url).scheme not in ("http", "https"):
                raise ValueError("没有有效图片地址")
            # 下载地址自带授权，不添加平台 API Key。
            with urllib.request.urlopen(url, timeout=60) as response:
                media_type = response.headers.get_content_type()
                if not media_type.startswith("image/") and media_type != "application/octet-stream":
                    raise ValueError("响应类型不是图片，未写入文件")
                data = response.read()
        mime, suffix = image_type(data)
        if item.get("mime_type") and item["mime_type"] != mime:
            raise ValueError("图片内容与声明的 MIME 不一致")
        target = output / f"image-{index}{suffix}"
        temporary = output / f".image-{index}.part"
        with temporary.open("wb") as saved:
            saved.write(data)
        temporary.replace(target)
        print(f"第 {index} 张已保存")
    except urllib.error.HTTPError as error:
        print(f"第 {index} 张下载失败，HTTP {error.code}；重新查询原任务获取当前地址")
        failed = True
    except (urllib.error.URLError, TimeoutError, OSError, ValueError, binascii.Error):
        print(f"第 {index} 张未保存；检查结果格式、网络和输出目录，保留原任务")
        failed = True
raise SystemExit(1 if failed else 0)
```

脚本支持 PNG、JPEG、WebP、GIF、BMP、TIFF 的文件签名识别；它不是完整的图片解码或安全检测器。
Base64 必须是 `b64_json` 原文，不添加 Data URL 前缀。文件扩展名根据实际内容选择，不能一律写成 PNG。
每个任务使用独立目录；同一任务重试保存可继续使用原目录，避免覆盖已下载项。

## 3. 图片任务地址过期后续签

图片任务的 URL 自签发起有效 300 秒，`url_expires_at` 是 Unix 秒。过期时重新查询**同一任务**，
取新 URL 再运行保存脚本；不要重发生成或编辑请求。

```bash
umask 077
curl --fail-with-body "{{OPENAI_BASE_URL}}/tasks/task_xxxxxxxx" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  --output image-result-renewed.json
```

确认查询成功且正文仍为原任务的 `object=image_task` 后：

```bash
python3 save_images.py image-result-renewed.json ./image-output
```

上面的鉴权只用于查询平台 API；下载图片时不附 Key。同步响应没有平台任务 ID，不能通过此流程续签
其 URL，应在首次响应后及时保存。下载 `403/404` 不一定只由过期引起，要以重新查询的图片状态为准。

## 4. 部分结果与失败

- `available`：可以下载；脚本尝试保存。
- `unavailable`：当前无法读取；稍后查询同一任务，不直接认定已删除。
- `deleted`：已确认图片不存在；重新查询不会重新生成。
- 任务 `unknown` 但有可用图片：可以保存部分结果，仍需核实整个任务，不能因下载成功就判定结算完成。

脚本退出码 `1` 表示有图片未保存，已写入的其他图片仍然保留。检查失败项后有限重试，不循环创建新任务。
