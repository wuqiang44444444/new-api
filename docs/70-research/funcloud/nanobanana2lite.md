图片生成 / Nano Banana
Nano Banana 2 Lite 图片生成
复制 Markdown

概述
Nano Banana 2 Lite 图片生成接口，是 Nano Banana 2 的轻量快速版，支持文生图（text-to-image）和图生图编辑（image-to-image），单一分辨率输出，生成速度更快、成本更低，最多可提供10张参考图片进行图生图编辑，支持丰富的宽高比选项。

Base URL: https://mm-internal-cn.leonecloud.com

认证方式
所有接口均需要在请求头中携带 Token 进行认证：

Copy
Authorization: Bearer {YOUR_AUTH_TOKEN}
快速开始
cURL 示例
创建文生图任务

Copy
curl -X POST "https://mm-internal-cn.leonecloud.com/api/v2/open/aigc/nano-banana-2-lite" \
  -H "Authorization: Bearer your_auth_token_here" \
  -H "Content-Type: application/json" \
  -d '{
    "prompt": "A surreal painting of a giant banana floating in space",
    "aspectRatio": "16:9"
  }'
创建图生图编辑任务

Copy
curl -X POST "https://mm-internal-cn.leonecloud.com/api/v2/open/aigc/nano-banana-2-lite" \
  -H "Authorization: Bearer your_auth_token_here" \
  -H "Content-Type: application/json" \
  -d '{
    "prompt": "将图片转换为水彩画风格",
    "imageUrls": ["https://example.com/input.jpg"],
    "aspectRatio": "1:1"
  }'
查询任务状态

Copy
curl -X GET "https://mm-internal-cn.leonecloud.com/api/v2/open/aigc/task_abc123" \
  -H "Authorization: Bearer your_auth_token_here"
接口列表
1. 创建 Nano Banana 2 Lite 图片生成任务
POST /api/v2/open/aigc/nano-banana-2-lite

创建一个 Nano Banana 2 Lite 图片生成任务。

Content-Type: application/json

请求参数
参数	类型	必填	说明
prompt	string	是	图片描述提示词，最多20000字符
imageUrls	string[]	否	参考图片 URL（最多10张，JPEG/PNG/WebP，每张最大30MB）
aspectRatio	string	否	宽高比：auto(默认) / 1:1 / 1:4 / 16:9 / 1:8 / 21:9 / 2:3 / 3:2 / 3:4 / 4:1 / 4:3 / 4:5 / 5:4 / 8:1 / 9:16
taskNickname	string	否	任务昵称，便于识别管理
callbackUrl	string	否	任务完成后的回调通知 URL
说明：
- Lite 版为单一分辨率输出，不支持 resolution / outputFormat 参数
- 提供 imageUrls 时自动进入图生图编辑模式，模型将基于参考图片进行生成
- 支持最多10张参考图片，每张最大30MB，支持 JPEG/PNG/WebP 格式
请求示例
基础文生图：

Copy
{
  "prompt": "A surreal painting of a giant banana floating in space"
}
指定宽高比：

Copy
{
  "prompt": "一只可爱的猫咪坐在窗台上，阳光洒落，超写实风格",
  "aspectRatio": "16:9"
}
图生图编辑（单张参考图）：

Copy
{
  "prompt": "将图片转换为水彩画风格，保持构图不变",
  "imageUrls": ["https://example.com/input.jpg"],
  "aspectRatio": "1:1"
}
图生图编辑（多张参考图）：

Copy
{
  "prompt": "融合这些图片的风格，生成一张新的艺术作品",
  "imageUrls": [
    "https://example.com/ref1.jpg",
    "https://example.com/ref2.jpg",
    "https://example.com/ref3.jpg"
  ],
  "aspectRatio": "3:2"
}
带回调地址：

Copy
{
  "prompt": "星空下的古老城堡，油画风格",
  "aspectRatio": "21:9",
  "callbackUrl": "https://your-server.com/callback"
}
响应参数
参数	类型	说明
code	int	状态码，0 表示成功
msg	string	状态信息
data.taskId	string	任务 ID，用于查询任务状态
data.status	string	任务状态，创建时固定为 processing
data.createdAt	string	创建时间
响应示例
成功

Copy
{
  "code": 0,
  "msg": "success",
  "data": {
    "taskId": "task_20260509150000_abc12345",
    "status": "processing",
    "createdAt": "2026-05-09 15:00:00"
  }
}
2. 查询任务状态
GET /api/v2/open/aigc/{taskId}

查询单个任务的执行状态。

响应示例
成功

Copy
{
  "code": 0,
  "msg": "success",
  "data": {
    "taskId": "task_20260509150000_abc12345",
    "status": "success",
    "result": [
      "https://fc-gw-sh.oss-accelerate.aliyuncs.com/images/2026/05/09/output_001.png"
    ],
    "createdAt": "2026-05-09 15:00:00",
    "updatedAt": "2026-05-09 15:00:25"
  }
}
3. 批量查询任务状态
POST /api/v2/open/aigc/batch

批量查询多个任务的执行状态（最多 100 个）。

回调通知
当任务完成（成功或失败）时，如果创建任务时提供了 callbackUrl，系统会向该 URL 发送 POST 请求。

回调请求
Headers

Copy
Content-Type: application/json
X-Funcloud-Event: task.completed
X-Funcloud-Signature: {签名}
Body

Copy
{
  "event": "task.completed",
  "taskId": "task_20260509150000_abc12345",
  "status": "success",
  "result": ["https://fc-gw-sh.oss-accelerate.aliyuncs.com/images/output_001.png"],
  "errorMsg": "",
  "timestamp": "2026-05-09T15:00:25+08:00",
  "signature": "a1b2c3d4e5f6..."
}
错误码
code	说明
0	成功
10002	参数缺失或格式错误
10005	API Key 无效或缺失
30003	任务不存在
90003	服务器内部错误
最佳实践
1. 轮询策略
建议的轮询间隔：
- 前 30 秒：每 3 秒查询一次
- 30 秒后：每 5 秒查询一次

2. 使用回调
生产环境建议使用回调通知而非轮询。

3. 处理时间参考
文生图：通常 5 ~ 15 秒
图生图编辑：通常 5 ~ 20 秒
4. 版本选择
版本	适用场景
Nano Banana 2 Lite	快速预览、批量生成、社交媒体等对速度和成本敏感的场景
Nano Banana 2	需要多分辨率（1K/2K/4K）输出、更高画质的场景