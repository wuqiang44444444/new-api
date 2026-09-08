---
status: in-progress
owner: Dev Team
last-reviewed: 2026-09-08
---

# Synlink 渠道 Mini 四秒四图真实测试

## 1. 问题与目标

按用户授权，参照现有 Seedance 模型配置四个 Synlink 客户模型的 Token 预扣上限，通过已实施的网关
渠道执行一次 Mini、4 秒、4 张参考图片的真实生成，核对请求、返回、视频交付和客户结算。
本次仅提交一个 Mini 视频任务，不把单例成功扩大为四模型生产验收。

## 2. 当前实际情况

### 2.1 环境与配置

- 测试日期：2026-09-08；本记录时间均为北京时间。
- 环境：本机开发网关，北向 `http://127.0.0.1:3501`。
- 渠道：104，Synlink 海外 Seedance · 统一素材库。
- 南向：`https://synlinkai.com`，代码协议 `synlink_video_v1`；素材配置为本站托管图片库。
- 参照同名官方客户模型现有值，新增下表中的 `-synlink` 预扣配置。这里只复用运维预算，不宣称已证明
  Synlink 所有规格的最大 Token 数；实际结算仍以可信用量为准。

| Synlink 客户模型 | 参照模型 | 预扣 Token |
| --- | --- | ---: |
| doubao-seedance-2-0-260128-synlink | doubao-seedance-2-0-260128 | 250000 |
| doubao-seedance-2-0-fast-260128-synlink | doubao-seedance-2-0-fast-260128 | 325000 |
| doubao-seedance-2-0-mini-260615-synlink | doubao-seedance-2-0-mini-260615 | 350000 |
| doubao-seedance-2-5-260628-synlink | doubao-seedance-2-5-260628 | 650000 |

测试使用单独的模型受限 API Key，额度为 $2，只允许本次 Mini 客户模型。未增加用户钱包余额。
渠道只在本次测试期间启用，完成后恢复禁用，测试 Key 停用。配置使用项目现有管理方法，生成和查询
使用运行中的 HTTP 网关；没有直接写入 Task 或资金结果。

### 2.2 输入图片与北向请求

四张不同的既有 AI 产品测试图片，内容均为陶瓷杯、木桌、绿叶和窗光。逐张检查没有人物、文字、
标志或可见敏感信息。1、3 为蓝杯，2、4 为红杯；提示词明确以红杯为准。

| 引用 | 格式与像素 | 文件字节数 | SHA-256 前 16 位 |
| --- | --- | ---: | --- |
| 图 1 | JPEG，1408 × 768 | 97858 | 9849378b35ec20111 |
| 图 2 | JPEG，1024 × 1024 | 107762 | 683b879b954ba8302 |
| 图 3 | PNG，1408 × 768 | 1299251 | dcc1dca6280e69e4c |
| 图 4 | PNG，1024 × 1024 | 1294176 | 75d1d2b9a7430fd47 |

图片经现有私有对象存储取得临时 HTTPS URL。下面只保留 URL 占位符，实际请求携带四条不同的可访问
URL。此次验证的是普通 URL 透传路径，没有创建托管素材，也没有传入 `asset://fhas_*`。

```http
POST /api/v3/contents/generations/tasks
Content-Type: application/json
Authorization: Bearer [REDACTED]
```

```json
{
  "model": "doubao-seedance-2-0-mini-260615-synlink",
  "content": [
    {
      "type": "text",
      "text": "参考图片1至图片4中的陶瓷杯、木桌、绿叶与柔和窗光，生成连贯的写实产品展示镜头。颜色以图片2和图片4的红杯为准。画面始终只有一只红色陶瓷杯，镜头缓慢向前推进，保持杯体形状、桌面和光线稳定。无文字，无标志。"
    },
    {"type": "image_url", "role": "reference_image", "image_url": {"url": "[IMAGE_1_HTTPS_URL]"}},
    {"type": "image_url", "role": "reference_image", "image_url": {"url": "[IMAGE_2_HTTPS_URL]"}},
    {"type": "image_url", "role": "reference_image", "image_url": {"url": "[IMAGE_3_HTTPS_URL]"}},
    {"type": "image_url", "role": "reference_image", "image_url": {"url": "[IMAGE_4_HTTPS_URL]"}}
  ],
  "duration": 4,
  "resolution": "480p",
  "ratio": "1:1",
  "generate_audio": false
}
```

南向路径为 `POST /v1/video/generate`，映射模型为 `doubao-seedance-2-0-mini-260615`。
代码 adapter 保持四条图片 URL 和上述参数，不发送 FunCloud 私有真人字段。
这是根据本次映射及 adapter 代码说明的请求转换；本记录没有导出加密证据中的南向原始正文。
运行中证据索引记录了南向 POST 和 HTTP 200 响应，正文保留在平台既有证据系统中。

### 2.3 创建、查询与返回

| 时间 | 动作 | 观测 |
| --- | --- | --- |
| 22:24:28 | 唯一一次创建 POST | 约 4.899 秒返回 HTTP 200 和公开任务 ID |
| 22:25:11 | 北向 GET | HTTP 200，`status=running`，`duration=4` |
| 22:25:11、22:25:14、22:25:59 | 网关主动查询或后台轮询 | 未识别的中间状态导致 `RECONCILIATION_REQUIRED`；未重发、未退款 |
| 22:26:28 | 平台持久化终态及客户账务 | `SUCCESS`、`settled`，最终 65425 quota |
| 22:26:58 | 对同一冻结上游任务执行只读 GET | HTTP 200，`completed`，38800 Token |
| 22:27:22 | 北向 GET 与视频内容 GET | `succeeded`；视频交付 HTTP 200，`video/mp4` |

创建响应：

```json
{"id":"task_JxdYkAHxtuCUWVxGpgcZTWZicYFamQCN"}
```

查询请求：

```http
GET /api/v3/contents/generations/tasks/task_JxdYkAHxtuCUWVxGpgcZTWZicYFamQCN
Authorization: Bearer [REDACTED]
```

成功查询响应的实测字段摘要：

```json
{
  "id": "task_JxdYkAHxtuCUWVxGpgcZTWZicYFamQCN",
  "model": "doubao-seedance-2-0-mini-260615-synlink",
  "status": "succeeded",
  "created_at": 1788877473,
  "updated_at": 1788877642,
  "duration": 4,
  "content": {"video_url": "[GATEWAY_VIDEO_CONTENT_URL]"},
  "usage": {"completion_tokens": 38800, "total_tokens": 38800}
}
```

对同一任务的上游 `GET /v1/video/tasks/{provider_task_id}` 返回字段摘要，已删除私有任务 ID 与产物地址：

```json
{
  "task": {
    "id": "[REDACTED]",
    "status": "completed",
    "model": "doubao-seedance-2-0-mini-260615-max",
    "duration_seconds": 4,
    "created_at": 1788877486,
    "completed_at": 1788877575,
    "outputs": ["[VIDEO_URL_REDACTED]"],
    "usage": {"completion_tokens": 38800, "total_tokens": 38800}
  }
}
```

上游报告的生成区间为 89 秒。上游查询模型带 `-max`，不同于创建模型；这里只记录供应商返回事实，
没有改变代码允许集合、模型映射或计费模型。北向保持客户模型身份。

### 2.4 视频文件与计费核验

- 交付端点：`GET /v1/videos/task_JxdYkAHxtuCUWVxGpgcZTWZicYFamQCN/content`。
- HTTP 200；类型 `video/mp4`；文件大小 1170930 字节。
- 实际视频：H.264，640 × 640，24 FPS，97 帧；没有音频流。
- 容器及视频流时长均为 **4.041667 秒**。请求和供应商字段为 4 秒，实际文件多出约一帧，不能表述为
  文件精确等于 4.000 秒。`480p` 为请求档位，实测正方形输出是 640 × 640。
- 中间帧可见单只红色陶瓷杯、木桌和绿叶，符合颜色及主体提示。未逐帧评估所有运动和稳定性要求。
- 产物：`synlink-mini-4s-4refs.mp4`、`synlink-mini-preview.png`、`test-results-redacted.json`，
  已交付本次会话工作区；JSON 含输入文件校验值、脱敏请求、观测与媒体检查结果。

当前 Mini 无视频输入单价：$3.372434017595 / 百万输出 Token，来源于此前保存的官方刊例价
23 元 / 百万 Token，按 6.82 元 / 美元换算；测试组倍率为 1。

| 账务项 | quota | 美元 |
| --- | ---: | ---: |
| 350000 Token 预扣 | 590176 | 1.180352 |
| 38800 Token 最终结算 | 65425 | 0.130850 |
| 退回预扣差额 | 524751 | 1.049502 |

持久化 attempt 为 `complete + transferred`；Task 为 `SUCCESS + settled`；用量来源为
`task.usage.completion_tokens`。独立消费及差额日志各一条，后续成功 GET 没有产生额外结算日志。
测试 Key 初始 1000000 quota，结算后为 934575，与最终收费相符。
以上是客户账务核验；没有登录 Synlink 控制台核对其货币账单，不能把客户费用当作供应商实际成本。

## 3. 优化方案与剩余事项

1. 本次 Mini 普通 URL 四图路径的创建、成功查询、用量归一、客户结算和内容交付已验证。
   测试后渠道恢复禁用，保留预扣配置；临时测试 Key 停用。
2. 处理中状态存在适配缺口：至少三次有效 HTTP 查询被记为 `unverified Synlink task status`，随后
   `completed` 恢复成功。当前记录未导出这些中间响应的具体状态字符串，不能猜测增加状态映射。
   后续应从证据系统取得精确正文，再补充状态映射及回归验证。
3. 需要确认供应商 `-max` 返回模型命名、4 秒任务多一帧及 `480p` 正方形像素语义，避免对外承诺超出实测。
4. 其它三个模型、托管 `asset://fhas_*` 路径、尾帧、失败／不存在响应及供应商账单仍未完成本次实测。
   不据此将整个渠道的四模型能力写为生产验收完成。

关联：[分阶段实施计划](../50-planning/2026-09-08-Synlink海外火山中转分阶段实施计划.md)、
[协议与素材接入分析](2026-09-08-Synlink海外火山中转协议与托管素材接入分析.md)。
