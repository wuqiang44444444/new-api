---
status: reference
owner: Dev Team
last-reviewed: 2026-09-28
source-url: https://shengshu.feishu.cn/wiki/PkFHwrme8ihMA8kshbgc5QwPnxe
---

# Vidu 国内站 Drama 官方协议调用原始信息

## 1. 正确来源与保存边界

- 官方文档：[【异步-官方协议】Vidu-drama 生视频模型-国内](https://shengshu.feishu.cn/wiki/PkFHwrme8ihMA8kshbgc5QwPnxe)。
- 读取日期：2026-09-28；页面显示最新修改时间：09月17日（页面未显示年份，不推定年份）。
- 读取方式：浏览器匿名打开飞书，逐节读取基础信息、模型能力、请求、媒体规格、参数、响应、完整示例、错误码和定价。
- 本文为原始接口事实的结构化摘录，保留技术字段、数值及文档冲突；不是完整原文镜像。
- 本文件替换此前误用的普通 Vidu `/ent/v2` 七类接口资料；那些接口不能用于判断本次 Drama/Seedance 线路。
- 未保存或调用用户提供的真实 Key。示例只含占位符；未创建任务或产生费用。
- 代码兼容判断另见[两站与现有 Seedance 渠道适配结论](Vidu两站差异与现有渠道适配结论.md)。

## 2. 连接与完整接口地址

以下完整地址由文档“基础信息”的站点域名和路径组合；示例中的矛盾单列第 9 节，不混入地址表。

| 操作 | 方法 | 完整 API 地址 |
| --- | --- | --- |
| 创建任务 | POST | `https://api.vidu.cn/ent/api/v3/contents/generations/tasks` |
| 查询单任务 | GET | `https://api.vidu.cn/ent/api/v3/contents/generations/tasks/{id}` |
| 查询任务列表 | GET | `https://api.vidu.cn/ent/api/v3/contents/generations/tasks` |
| 删除任务 | DELETE | `https://api.vidu.cn/ent/api/v3/contents/generations/tasks/{id}` |

请求头：`Authorization: Bearer <API_KEY>`、`Content-Type: application/json`。
创建为异步调用：提交取得 `id` 后查询。这里不使用普通 Vidu 的 `Token` 鉴权或 `/ent/v2/text2video`。

## 3. 模型与生成模式

| 模型 ID | 文档说明 | 输出分辨率 | 输出时长 |
| --- | --- | --- | --- |
| `viduq3.1-drama-std` | 加强满血版 | 480p / 720p / 1080p | 4–30 秒或 -1 |
| `viduq3-drama-std` | 标准满血版 | 480p / 720p / 1080p / 4k | 4–15 秒或 -1 |
| `viduq3-drama-fast` | 快速版 | 480p / 720p | 4–15 秒或 -1 |
| `viduq3-drama-mini` | mini 版 | 480p / 720p | 4–15 秒或 -1 |

表中时长范围综合顶层字段与时长说明；原文时长专表末行重复 fast、遗漏 mini，保留为第 9 节疑点。

默认分辨率 720p，默认时长 5 秒；`duration=-1` 为自动选择时长。
三类场景互斥：纯文本；首帧/首尾帧关键图；多模态参考。不能把关键帧角色与多模态参考角色混用。
Q3 支持参考图最多 9 张、视频 3 段、音频 3 段；Q3.1 分别为 30、10、10。

## 4. 请求字段

| 字段 | 类型 | 必填 | 原文默认/限制 |
| --- | --- | --- | --- |
| model | string | 是 | 本站四个精确模型 ID |
| content | array | 是 | 内容元素数组，见下表 |
| resolution | string | 否 | 720p；按模型选择，原文字面值为 `4k` |
| ratio | string | 否 | adaptive；16:9 / 4:3 / 1:1 / 3:4 / 9:16 / 21:9 / adaptive |
| duration | integer | 否 | 5；范围见模型表 |
| generate_audio | boolean | 否 | 顶层表 false；第 6.4 节 true，存在冲突 |
| watermark | boolean | 否 | false；右下角 AI 生成水印 |
| callback_url | string | 否 | 状态变化 POST 回调地址 |
| return_last_frame | boolean | 否 | false；返回 JPEG 尾帧 |
| execution_expires_after | integer | 否 | 172800 秒；范围 3600–259200 |
| tools | array | 否 | `[{"type":"web_search"}]` |
| safety_identifier | string | 否 | 终端用户哈希标识，最多 64 字符 |

| content.type | 内容字段 | role |
| --- | --- | --- |
| text | text | 无媒体角色；中文建议 ≤500 字，英文建议 ≤1000 词 |
| image_url | image_url.url | first_frame / last_frame / reference_image |
| video_url | video_url.url | reference_video |
| audio_url | audio_url.url | reference_audio |

媒体 role 必填；首尾帧每种角色一张。文档 URL 方式只列公网 HTTPS 地址，未声明 `asset://`、Base64
或素材库 API，因此不能据此配置火山/BytePlus 素材协议，也不能声明本站托管素材兼容。

## 5. 输入素材与输出规格

| 媒体 | 原始规格事实 |
| --- | --- |
| 图片格式 | jpeg / png / webp / bmp / tiff / gif |
| 图片尺寸 | 宽高比 0.4–2.5；边长 300–6000 px |
| 图片大小 | 单张 <30 MB；请求体总计 ≤64 MB |
| 图片数量 | 首帧 1、首尾帧 2；Q3 参考图片专表写 1–9，能力矩阵写 0–9；Q3.1 专表为 0–30 |
| 视频格式 | mp4 / mov；H.264/H.265 + AAC/MP3 |
| 视频时长 | Q3 单段 2–15 秒、最多 3 段、总计 ≤15 秒；Q3.1 单段 4–30 秒、最多 10 段、总计 ≤30 秒 |
| 视频尺寸 | 宽高比 0.4–2.5；边长 300–6000；总像素 409600–8295044 |
| 视频大小与帧率 | 单段 ≤200 MB；24–60 fps |
| 音频格式与大小 | wav / mp3；单段 ≤15 MB |
| 音频时长 | Q3 单段 2–15 秒、最多 3 段、总计 ≤15 秒；Q3.1 最多 10 段、总计 ≤30 秒，编辑任务单段 2–30 秒，非编辑任务单段 4–30 秒 |

`adaptive`：纯文本按提示词选比例；关键帧按首图匹配最近枚举；多模态以媒体为准，视频优先图片。
比例与输入图不同时文档描述为居中裁剪。4K 输出标注 10bit + H.265。

| 分辨率 | 16:9 | 4:3 | 1:1 | 3:4 | 9:16 | 21:9 |
| --- | --- | --- | --- | --- | --- | --- |
| 480p | 864×496 | 752×560 | 640×640 | 560×752 | 496×864 | 992×432 |
| 720p | 1280×720 | 1112×834 | 960×960 | 834×1112 | 720×1280 | 1470×630 |
| 1080p | 1920×1080 | 1664×1248 | 1440×1440 | 1248×1664 | 1080×1920 | 2206×946 |
| 4k | 3840×2160 | 3326×2494 | 2880×2880 | 2494×3326 | 2160×3840 | 4398×1886 |

文档返回时长说明：实际帧数 / 24，向下取整。该说明不等于已验证 Provider 计费时长。

## 6. 响应、状态与回调

创建成功 HTTP 200，返回根级字符串 `id`。查询示例字段为：

| 字段 | 示例类型/语义 |
| --- | --- |
| id | string，任务 ID |
| status | string，任务状态 |
| content.video_url | string，视频结果 URL |
| content.last_frame_url | string，尾帧 URL |
| duration | number，输出时长 |
| resolution / ratio | string，输出规格 |
| usage.completion_tokens | integer，示例为 108900 |
| usage.total_tokens | integer，示例为 108900 |

状态表：`queued`、`running`、`succeeded`、`failed`、`expired`。
成功从 `content.video_url` 取结果；`expired` 与 `execution_expires_after` 相关。
本页没有完整失败响应的 JSON 字段合同，也未承诺结果 URL 的固定有效期；不从普通 Vidu 文档补入。
配置回调后状态变化以 POST 推送，内容与查询响应相同；5 秒无成功响应会重试，最多 3 次。
本页未给出回调验签合同，不直接沿用普通 Vidu 的 HMAC 规则。

HTTP 错误表：400 参数错误、401 鉴权失败、403 无权限、404 任务不存在、429 请求频繁、500 服务内部错误、503 服务不可用。

## 7. 调用示例（按基础信息整理，未执行）

以下不是逐字复制原文错误示例；域名按本站基础信息，提示词为本文自拟，并显式指定音频开关避开默认值歧义。

```bash
curl --request POST 'https://api.vidu.cn/ent/api/v3/contents/generations/tasks' \
  --header 'Authorization: Bearer <API_KEY>' \
  --header 'Content-Type: application/json' \
  --data '{
    "model": "viduq3-drama-std",
    "content": [{"type":"text","text":"清晨湖面，薄雾散开，镜头缓慢向前。"}],
    "resolution": "720p",
    "ratio": "16:9",
    "duration": 5,
    "generate_audio": false,
    "watermark": false
  }'

curl 'https://api.vidu.cn/ent/api/v3/contents/generations/tasks/<TASK_ID>' \
  --header 'Authorization: Bearer <API_KEY>'
```

## 8. 文档定价事实（非账号实测账单）

本站 token 积分单价：¥0.000001。
下表金额单位为人民币元/百万输出 Token；对应系数为 N。
原文计价式：`output_tokens × token积分单价 × N`，积分/百万 Token 为 `N × 1000000`。

| 模型 | 分辨率 | 不含视频输入：金额 / N | 含视频输入：金额 / N |
| --- | --- | --- | --- |
| `viduq3.1-drama-std` | 480p/720p | 70 / 70 | 42 / 42 |
| `viduq3.1-drama-std` | 1080p | 77 / 77 | 46 / 46 |
| `viduq3-drama-std` | 480p/720p | 46 / 46 | 28 / 28 |
| `viduq3-drama-std` | 1080p | 51 / 51 | 31 / 31 |
| `viduq3-drama-std` | 4k | 26 / 26 | 16 / 16 |
| `viduq3-drama-fast` | 480p/720p | 37 / 37 | 22 / 22 |
| `viduq3-drama-mini` | 480p/720p | 23 / 23 | 14 / 14 |

价格不代表当前 Key 的结算折扣、实际扣款或已生效客户价格。成功示例给出 completion_tokens 与 total_tokens
相同的数值，但 `output_tokens` 与实际响应字段的对应、失败费用及退款仍需真实任务与账单核验。

## 9. 原文冲突与尚未证明的语义

1. 顶层表 `generate_audio` 默认 false，第 6.4 节默认 true；调用应显式指定，不能替 Provider 判断默认。
2. 时长专表重复 fast 行、未单列 mini；顶层表按 Q3 系列给出 4–15 秒，不能静默改写原表当作无冲突。
3. 通用 content 角色说明仍写 9/3/3 与 15 秒，后面的 Q3.1 专表扩展到 30/10/10 与 30 秒。
4. 请求结构示例把 first_frame 与 reference_video/reference_audio 同时列出，与三类场景互斥说明冲突；
   本文示例不混用这些角色。
5. 第 8.5 节查询 curl 使用 `https://api.vidu.cn/ent/v2/tasks/.../creations`，与基础信息和第 7.2 节的
   `/ent/api/v3/contents/generations/tasks/{id}` 不一致；以 V3 为候选核验，不能实现自动尝试两套查询。
6. 正文分辨率使用小写 `4k`，网关官方协议元数据使用大写 `4K`；大小写是否都被 Provider 接受尚未验证。
7. 删除 curl 把 Authorization 写成 `Bearer Bearer YOUR_API_KEY`，与基础鉴权声明冲突；实际应只有一个 Bearer。
8. 删除示例返回 `{}`，后文又说 Result 为空对象；完整删除响应形状和在途取消/终态删除含义需 Provider 确认。

## 真实调用差异（2026-09-28）

本次两站 `Seedance 2.0 std` 图片＋参考音频请求均创建成功。真实查询中的 `duration`、`seed`、
`created_at`、`updated_at`、`frames_per_second` 为字符串，和上文文档示例的数值类型不同。
原始文档摘录保持不变；实测事实及渠道状态见[双站实测记录](../99-archive/2026/09/2026-09-28-Vidu双站渠道接入与实测.md)。
