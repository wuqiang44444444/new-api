---
status: current
owner: Dev Team
last-reviewed: 2026-09-07
---

# Seedance 2.5 视频生成说明与实际示例

通过四张参考图片和一段文字说明，生成一条 4 秒的产品展示视频。
本文以红色陶瓷杯为例，介绍图片准备、生成参数、任务提交和结果查看。

## 生成效果与参数

示例画面包含一只红色陶瓷杯、木桌、绿叶和柔和窗光，镜头缓慢向前推进，保持杯体和场景稳定。

| 参数 | 示例值 | 说明 |
| --- | --- | --- |
| `model` | `seedance-2-5` | 本示例使用的模型名称，实际调用时以账号可用模型为准 |
| `content` | 一段文字和四张图片 | 文字描述目标画面，图片提供外观、颜色和环境参考 |
| `role` | `reference_image` | 将图片作为生成参考 |
| `duration` | `4` | 请求生成 4 秒视频 |
| `resolution` | `480p` | 请求的分辨率档位 |
| `ratio` | `1:1` | 正方形画面 |
| `generate_audio` | `false` | 请求不生成音频 |

## 准备参考图片

四张图片按以下顺序提供：陶瓷杯原图、红杯编辑图、另一张陶瓷杯原图、另一张红杯编辑图。
提示词以第 2、4 张图片确定红色杯体，以四张图片共同参考桌面、绿叶和光线。
换用其他图片时，请同步调整提示词。

图片需提前上传至对象存储，并取得服务端可以读取的 HTTPS URL。
使用临时读取地址时，有效期需要覆盖任务处理过程。`127.0.0.1` 的图片预览地址仅本机可用，
不能作为远程服务的图片输入。

## 准备请求

需要安装 `curl` 和 `jq`。以下命令在同一个 shell 中依次执行。
将 API Key 和四个图片地址替换为自己的值；示例服务地址为本地 `http://127.0.0.1:3501`，
部署到其他环境时替换为实际 API 地址。

```bash
export NEW_API_KEY='你的本地网关 API Key'
export REF1='第一张图片的 OSS URL'
export REF2='第二张图片的 OSS URL'
export REF3='第三张图片的 OSS URL'
export REF4='第四张图片的 OSS URL'

PAYLOAD=$(jq -n \
  --arg r1 "$REF1" \
  --arg r2 "$REF2" \
  --arg r3 "$REF3" \
  --arg r4 "$REF4" \
  '{
    content: [
      {
        type: "text",
        text: "参考图片1至图片4中的陶瓷杯、木桌、绿叶与柔和窗光，生成连贯的写实产品展示镜头。颜色以图片2和图片4的红杯为准。画面始终只有一只红色陶瓷杯，镜头缓慢向前推进，保持杯体形状、桌面和光线稳定。无文字，无标志。"
      },
      {type: "image_url", role: "reference_image", image_url: {url: $r1}},
      {type: "image_url", role: "reference_image", image_url: {url: $r2}},
      {type: "image_url", role: "reference_image", image_url: {url: $r3}},
      {type: "image_url", role: "reference_image", image_url: {url: $r4}}
    ],
    duration: 4,
    resolution: "480p",
    ratio: "1:1",
    generate_audio: false
  }')
```

## 提交生成任务

```bash
curl --fail-with-body -sS \
  'http://127.0.0.1:3501/api/v3/contents/generations/tasks' \
  -H "Authorization: Bearer $NEW_API_KEY" \
  -H 'Content-Type: application/json' \
  --data "$(printf '%s' "$PAYLOAD" | jq '. + {model: "seedance-2-5"}')"
```

每次提交都会创建新任务并可能产生费用。保存返回的任务 `id`，使用查询接口查看进度。

## 查询任务结果

创建接口异步返回任务 `id`，不代表视频已经生成。使用创建任务时的同一个 API Key 查询：

```bash
TASK_ID='创建接口返回的 id'

curl --fail-with-body -sS \
  "http://127.0.0.1:3501/api/v3/contents/generations/tasks/$TASK_ID" \
  -H "Authorization: Bearer $NEW_API_KEY"
```

状态为 `succeeded` 后读取 `content.video_url`。如果返回的是本地网关的受保护内容地址，
读取视频时同样需要本地 API Key；不要把本地 API Key 发送给对象存储或上游域名。

## 实际生成示例

以下三条视频于 2026-09-07 使用上述四图、4 秒参数实测生成。
均已下载并核对时长与抽帧画面，输出为 640×640，画面呈现红色陶瓷杯与缓慢推进的镜头。
耗时是本次实测值，不是固定生成时间。

| 示例 | 生成耗时 | 文件时长 | 音频检查 | 实际视频 |
| --- | ---: | ---: | --- | --- |
| 示例一 | 362 秒 | 4.042 秒 | 无音轨 | [查看视频](http://127.0.0.1:3502/seedance25-81-fourrefs-4sec.mp4) |
| 示例二 | 170 秒 | 4.064 秒 | 含非静音音轨 | [查看视频](http://127.0.0.1:3502/seedance25-77-fourrefs-4sec.mp4) |
| 示例三 | 960 秒 | 4.064 秒 | 含非静音音轨 | [查看视频](http://127.0.0.1:3502/seedance25-75-fourrefs-4sec.mp4) |

视频链接由本地预览服务提供，仅在保存了这些文件并运行该服务的电脑上可用。
示例一对应上面的 `seedance-2-5` 调用；另外两条展示相同生成参数的实测效果，不表示执行一次请求会返回三条视频。

三次请求均设置 `generate_audio: false`，但示例二、三仍包含非静音音轨，因此本次结果不能保证关闭音频始终生效。
文件时长约 4.04～4.06 秒，较请求的 4 秒略长，以上表中的实际媒体检查值为准。
