---
status: current
owner: Dev Team
last-reviewed: 2026-09-07
---

# FunCloud Seedance 视频统一调用说明

## 文档来源与适用范围

依据 2026-09-07 阅读的 [Mini 调用文档](https://docs.leonecloud.com/docs/seedance-2-0-mini/)
及其同站的 [全系 V3 文档](https://docs.leonecloud.com/docs/seedance-2-5-v3-protocol)，两页均标注更新于
2026-09-06。以下示例是按公开协议编写的可替换参数模板，本次未实际提交这些 V3 生成请求。

供应商已经提供统一 V3 接口，2.0、Fast、Mini、2.5 共用创建与查询结构，普通生成切换模型只需修改
`model`。本文前半部分的 URL 与 Key 均属于供应商；本地网关调用方式另见末节，二者不能混用凭据。

## 模型与生成参数

| `model` | 分辨率 | 时长 | 默认时长 |
| --- | --- | --- | --- |
| `seedance-2-0` | 480p、720p | 4–15 秒 | 5 秒 |
| `seedance-2-0-fast` | 480p、720p | 4–15 秒 | 5 秒 |
| `seedance-2-0-mini` | 480p、720p | 4–15 秒 | 5 秒 |
| `seedance-2-5` | 480p、720p、1080p | 4–30 秒，支持 -1 智能时长 | -1 |

V3 通用输入表规定最多 9 张参考图片、3 个参考视频、3 个参考音频。图片须为服务端可访问的地址，
尺寸 300–6000px、宽高比 0.4–2.5、最大 30MB。参考视频和音频在 V3 表中为 2–15 秒。
这些是在线文档声明，尚未在本地完成全模型边界实测。

默认 `ratio=adaptive`、`resolution=720p`、`generate_audio=true`。要控制生成结果和预算，建议显式
给出时长、分辨率、比例和音频开关。以下示例采用四张参考图、4 秒、480p、1:1，不生成音频。

## V3：准备四张图片并提交

需要 `curl` 和 `jq`。替换 Key 和四张图片的地址；签名地址有效期需要覆盖生成过程，不能使用本机
`127.0.0.1` 图片地址。每次 POST 都会创建新任务，收到任务 ID 后应查询原任务。

```bash
export FUNCLOUD_API_KEY='供应商 API Key'
export REF1='第一张图片的 HTTPS URL'
export REF2='第二张图片的 HTTPS URL'
export REF3='第三张图片的 HTTPS URL'
export REF4='第四张图片的 HTTPS URL'
export VIDEO_MODEL='seedance-2-0-mini'

PAYLOAD=$(jq -n \
  --arg model "$VIDEO_MODEL" \
  --arg r1 "$REF1" --arg r2 "$REF2" --arg r3 "$REF3" --arg r4 "$REF4" \
  '{
    model: $model,
    content: [
      {type: "text", text: "参考图片1至图片4中的陶瓷杯、木桌与窗光，以图片2和图片4的红杯为准。画面只有一只红色陶瓷杯，镜头缓慢推进，保持杯体形状和光线稳定，无文字和标志。"},
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

curl --fail-with-body -sS \
  'https://mm-internal-cn.leonecloud.com/api/v3/contents/generations/tasks' \
  -H "Authorization: Bearer $FUNCLOUD_API_KEY" \
  -H 'Content-Type: application/json' \
  --data "$PAYLOAD"
```

如需换用其他模型，将 `VIDEO_MODEL` 改为模型表中的值，并重新构建 `PAYLOAD`。
纯文生视频只保留 `content` 中的文字项；首帧生成将对应图片的 role 改为 `first_frame`，
首尾帧生成分别用 `first_frame` 和 `last_frame`，此时比例会跟随输入图片。

## V3：查询结果

创建响应返回 `id`。以下 `TASK_ID` 是占位符，应填入本次创建返回的值。

```bash
TASK_ID='创建接口返回的 id'

curl --fail-with-body -sS \
  "https://mm-internal-cn.leonecloud.com/api/v3/contents/generations/tasks/$TASK_ID" \
  -H "Authorization: Bearer $FUNCLOUD_API_KEY"
```

`submitted/running` 表示处理中；`succeeded` 时读取 `content.video_url`；`failed` 时读取
`error.code` 与 `error.message`。可按前 30 秒每 3 秒、随后至 2 分钟每 5 秒、之后每 10 秒查询。
缺少 `usage` 不代表零费用；页面说明用量可能在结算后才出现。

常见错误为 HTTP 400 参数不支持、401 Key 无效、402 余额不足、403 无权限、404 任务不存在。
创建超时或没有取得可信任务 ID 时，不应通过重复 POST 来轮询。

## Mini V2：原链接对应的调用方式

用户提供的 Mini 页面本身介绍 V2：创建路径带模型名称，使用 `generateAudio`，默认不生成音频。
V2 与 V3 不能混用字段或响应解析。下面提供一个独立的文生视频示例：

```bash
curl --fail-with-body -sS \
  'https://mm-internal-cn.leonecloud.com/api/v2/open/aigc/seedance2-0-mini' \
  -H "Authorization: Bearer $FUNCLOUD_API_KEY" \
  -H 'Content-Type: application/json' \
  --data '{
    "content": [{"type":"text","text":"红色陶瓷杯置于木桌上，柔和窗光，镜头缓慢推进。"}],
    "duration":4,
    "resolution":"480p",
    "ratio":"1:1",
    "generateAudio":false
  }'
```

V2 创建成功读取 `data.taskId`，再调用 `GET /api/v2/open/aigc/{taskId}`，鉴权头保持一致。
状态 `processing/completed/failed` 位于 `data.status`，完成时读取 `data.result` 视频 URL 数组。
Mini V2 页面明确支持素材接口返回的 `assetUrl` 引用，但没有明确三张参考图上限；三图示例本身不是上限。

## 本地网关调用与接入状态

当前本地 `funcloud_seedance` 适配器仍调用供应商 V2。即使本地北向路径也叫
`/api/v3/contents/generations/tasks`，也不代表南向已经切换供应商 V3。
本地请求使用本地 API Key 和部署方发布的客户模型名；不能直接填供应商 V3 枚举来假定路由已配置。

已完成的本地四图视频生成示例见 [视频生成说明与实际示例](Seedance2.5四图视频调用示例.md)。
当前 2.0 本地三图限制尚未修改，新版九图能力需要南向适配和实测后才能作为本地可用能力。

协议可统一不代表分辨率、计费和素材能力全部相同：V3 将 2.0 描述为按秒计费，2.5 智能时长按 Token
计费；现有 Token 结算不能不经核对直接沿用。带参考视频的自动模式可能强制智能时长，仅 2.5 适用。
素材域是否共享及素材操作是否支持，需要独立证据，不能从一个生成端点推断。

详细协议差异与当前实现边界见 [FunCloud 对接设计](../../20-architecture/Seedance模型接入设计/FunCloud/FunCloud模型与素材库对接设计.md)。
