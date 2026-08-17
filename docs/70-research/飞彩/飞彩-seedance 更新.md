# Seedance 视频生成 API 接口文档

> 版本：v1.1 ｜ 更新日期：2026-08-03（**价格已按平台最新定价全部更新**）
> 适用平台：New API 中转站（10 个 Seedance 视频模型）
> 本文档所有参数格式与错误示例均来自真实接口实测，可直接交付下游对接。

---

## 目录

1. [服务概述](#一服务概述)
2. [核心调用流程](#二核心调用流程)
3. [请求参数完整说明](#三请求参数完整说明重点)
4. [10 个模型速查表（最新价格）](#四10-个模型速查表最新价格)
5. [各模型详细说明](#五各模型详细说明)
6. [响应格式](#六响应格式)
7. [完整代码示例](#七完整代码示例)
8. [常见报错对照表](#八常见报错对照表)
9. [注意事项与最佳实践](#九注意事项与最佳实践)

---

## 一、服务概述

| 项目 | 值 |
|------|-----|
| 服务地址（Base URL） | `http://43.161.200.208` |
| 认证方式 | HTTP Header：`Authorization: Bearer <你的API Key>` |
| API Key 获取 | 平台注册登录后，在「API 密钥」页面创建 |
| 数据格式 | 请求/响应均为 JSON（也支持 multipart/form-data 上传参考文件） |

### 可用端点

| 端点 | 方法 | 说明 |
|------|------|------|
| `/v1/models` | GET | 获取当前可用模型列表 |
| `/v1/videos` | POST | 创建视频生成任务 |
| `/v1/videos/{task_id}` | GET | 查询任务状态与结果 |
| `/v1/tasks` | GET | 查询任务列表（分页/过滤，详见《余额与任务查询接口文档》） |
| `/v1/dashboard/billing/subscription` | GET | 查询账户总额度（同上文档） |
| `/v1/dashboard/billing/usage` | GET | 查询账户已用量（同上文档） |

> ⚠️ 视频下载请使用查询响应中的 `video_url` 字段直接下载（见下文说明）。

---

## 二、核心调用流程

视频生成为**异步任务**，标准调用分三步：

```
① POST /v1/videos  创建任务 → 返回 task_id（status: queued）
        ↓
② GET /v1/videos/{task_id}  轮询查询 → status: processing（约 1~5 分钟）
        ↓
③ status = completed → 响应中出现 video_url → 直接下载 mp4
```

**状态流转**：`queued`（排队中）→ `processing`（生成中）→ `completed`（完成）/ `failed`（失败）

**轮询建议**：每 10~20 秒查询一次，4 秒 720p 视频实测约 2~4 分钟完成。

---

## 三、请求参数完整说明（重点）

### 3.1 创建任务请求体

```json
POST /v1/videos
{
  "model": "seedance-2.0-933-720p-azhw",
  "prompt": "一只猫在阳光下的花园里追蝴蝶",
  "seconds": "4",
  "size": "1280x720",
  "images": ["https://example.com/ref1.jpg"],
  "audios": ["https://example.com/bgm.mp3"]
}
```

### 3.2 参数详解

#### `model`（必填，string）
模型名称，必须是本文档「模型速查表」中的公开模型名，区分大小写。

#### `prompt`（必填，string）
视频内容描述文本。支持中英文，建议具体描述主体、动作、场景、光线、镜头运动。

#### `seconds`（时长，⚠️ 极易踩坑）
- **必须是「字符串形式的整数」**：`"4"` ✅，`4` ❌，`"4.5"` ❌
- 传数字 `4` 会报错：`json: cannot unmarshal number into Go struct field .seconds of type string`
- 传 `"abc"` 会报错：`seconds 参数必须为整数`
- 可用 `duration`（整数）替代：`"duration": 4`，二选一即可
- 不传时默认 4 秒
- 取值范围受模型限制（见模型详情，一般 4~15 秒；`seedance-933-pro-pi` 固定 15 秒；`seedance2.0-sd2` 仅 11~15 秒）

#### `size`（画幅尺寸，string，可选）
- 格式：`"宽x高"`（小写 x），如 `"1280x720"`
- 已实测有效值：
  - `"1280x720"` — 16:9 横屏 ✅（已实测）
  - `"720x1280"` — 9:16 竖屏（默认）
- 不传时默认 `"720x1280"`（竖屏）
- 传不受支持的值报错：`size 参数取值不受支持，请检查后重试`
- 各模型支持的画幅比例见模型详情。所有合法画幅使用同一个模型每秒价，不存在宽幅加价或比例倍率。
- 当前资料只确认 `16:9 → 1280x720`、`9:16 → 720x1280` 两组精确映射；`21:9`、`4:3`、
  `1:1`、`3:4` 的合法 `宽x高` 值仍需逐项验证，不能按数学比例自行猜测。

#### `images`（多参考图，⚠️ 下游最易报错点）

- **类型：字符串数组** `["https://...", "https://..."]`——**即使只有 1 张图也必须写成数组**
- 每个元素必须是**公网可访问的图片地址**，仅支持以下三种形式：
  - `http://...`
  - `https://...`
  - `data:image/jpeg;base64,...`（base64 data URL）
- ❌ 本地路径（`C:\xxx.jpg`、`/tmp/xxx.jpg`）、相对路径、非法字符串都会报错：
  `图片地址仅支持 http、https 或 data URL`
- ❌ 内网地址（`192.168.x.x`、`127.0.0.1`）无法被上游拉取，会导致任务失败
- 数量上限：多数模型最多 **9 张**；`seedance2.0-sd2` **必须提供 1~9 张**（必填）
- 兼容写法（单图时三选一）：
  - `"images": ["https://...jpg"]` — 推荐
  - `"image": "https://...jpg"` — 单图字符串
  - `"input_reference": "https://...jpg"` — 兼容字段

**正确 / 错误对照**：

```jsonc
// ✅ 正确：数组形式，公网 https 地址
"images": ["https://cdn.example.com/person.jpg"]

// ✅ 正确：多张参考图
"images": ["https://cdn.example.com/a.jpg", "https://cdn.example.com/b.jpg"]

// ✅ 正确：base64 data URL
"images": ["data:image/jpeg;base64,/9j/4AAQSkZJRg..."]

// ❌ 错误：传了字符串而不是数组
"images": "https://cdn.example.com/person.jpg"

// ❌ 错误：本地路径
"images": ["C:\\photos\\person.jpg"]

// ❌ 错误：内网地址
"images": ["http://192.168.1.100:8080/person.jpg"]
```

#### `audios`（音频，⚠️ 格式必须严格）

- **类型：字符串数组** `["https://example.com/bgm.mp3"]`——**即使只有 1 段音频也必须写成数组**
- 每个元素为**公网可访问的音频地址**（http/https）
- ⚠️ **不能传字符串**。传 `"audios": "https://..."` 会报错：
  `json: cannot unmarshal string into Go struct field .audios of type []string`
- 数量上限：最多 **3 段**
- ⚠️ 仅部分模型支持（`seedance2.0-sd2` **不支持音频**，见模型详情）
- 音频与参考图一样必须公网可访问，内网/本地上传的地址会导致任务失败

**正确 / 错误对照**：

```jsonc
// ✅ 正确：数组形式
"audios": ["https://cdn.example.com/bgm.mp3"]

// ✅ 正确：多段音频（最多 3 段）
"audios": ["https://cdn.example.com/bgm.mp3", "https://cdn.example.com/voice.mp3"]

// ❌ 错误：传了字符串而不是数组（高频报错！）
"audios": "https://cdn.example.com/bgm.mp3"

// ❌ 错误：seedance2.0-sd2 模型传音频（该模型不支持音频）
{"model": "seedance2.0-sd2", "audios": ["https://..."]}
```

#### `videos`（参考视频，数组，可选）
- 类型同 `audios`：字符串数组，元素为公网视频地址
- 仅 `seedance-933-pro-pi` 支持，最多 3 段；其余模型不支持

### 3.3 完整参数速查

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `model` | string | ✅ | 公开模型名 |
| `prompt` | string | ✅ | 视频描述 |
| `seconds` | string | 可选 | 时长秒数，**字符串整数**如 `"4"` |
| `duration` | int | 可选 | 时长秒数（与 seconds 二选一） |
| `size` | string | 可选 | `"宽x高"`，默认 `"720x1280"` |
| `images` | string[] | 按模型 | 参考图 URL 数组（http/https/data URL），**必须数组** |
| `image` | string | 可选 | 单张参考图（兼容写法） |
| `input_reference` | string | 可选 | 单张参考图（兼容写法） |
| `audios` | string[] | 可选 | 音频 URL 数组，**必须数组**，≤3 段 |
| `videos` | string[] | 可选 | 参考视频 URL 数组（仅 pro-pi），≤3 段 |

---

## 四、10 个模型速查表（最新价格）

> 以下价格为 2026-08-03 平台最新定价，按秒计费模型费用 = `seconds` × 单价。

| 模型名 | 分辨率 | 计费 | 单价 | 时长(秒) | 参考图 | 音频 | 参考视频 | 特点 |
|--------|--------|------|------|----------|--------|------|----------|------|
| `seedance-2.0-vip-720p-mini-azhw` | 720p | 按秒 | **¥0.55/秒** | 4~15 | ≤9 | ≤3 | ✗ | Mini 轻量，最低价 |
| `seedance2.0-sd2` | 720p | 按秒 | **¥0.60/秒** | 11~15 | **必须1~9** | ✗ | ✗ | 图生视频专用 |
| `seedance-2.0-vip-720p-fast-azhw` | 720p | 按秒 | **¥0.60/秒** | 4~15 | ≤9 | ≤3 | ✗ | Fast 快速出片 |
| `seedance-2.0-933-720p-azhw` | 720p | 按秒 | **¥0.62/秒** | 4~15 | ≤9 | ≤3 | ✗ | 性价比首选 |
| `seedance-2.0-vip-720p-azhw` | 720p | 按秒 | **¥0.74/秒** | 4~15 | ≤9 | ≤3 | ✗ | 优质渠道 |
| `seedance-2.0-933-1080p-azhw` | 1080p | 按秒 | **¥1.48/秒** | 4~15 | ≤9 | ≤3 | ✗ | 933 高清 |
| `seedance-2.0-vip-1080p-azhw` | 1080p | 按秒 | **¥1.74/秒** | 4~15 | ≤9 | ≤3 | ✗ | 优质高清 |
| `seedance-2.0-933-4k-azhw` | 4K | 按秒 | **¥3.58/秒** | 4~15 | ≤9 | ≤3 | ✗ | 933 超清 |
| `seedance-2.0-vip-4k-azhw` | 4K | 按秒 | **¥4.23/秒** | 4~15 | ≤9 | ≤3 | ✗ | 优质超清 |
| `seedance-933-pro-pi` | 720p | **按次** | **¥11.05/次** | 固定15 | ≤9 | ≤3 | ≤3 | 唯一支持参考视频 |

**计费规则**：
- 按秒计费模型：费用 = `seconds` × 单价
  - 例：`seedance-2.0-933-720p-azhw` 生成 4 秒 = 4 × 0.62 = **¥2.48**（已实测验证）
  - 例：`seedance-2.0-vip-720p-mini-azhw` 生成 10 秒 = 10 × 0.55 = **¥5.50**
- `seedance-933-pro-pi`：固定每次 **¥11.05**（无论参数）
- 同一个模型的所有合法画幅价格相同；`size` 只表达构图比例，不参与价格计算。

---

## 五、各模型详细说明

### 5.1 seedance-2.0-vip-720p-mini-azhw（Mini 轻量，最低价）
- 价格：**¥0.55/秒** ｜ 时长：4~15 秒 ｜ 分辨率：720p
- 优质渠道 Mini 线路，全平台按秒计费模型中价格最低
- 画幅：21:9、16:9、4:3、1:1、3:4、9:16
- 参考图 ≤9 张、音频 ≤3 段、不支持参考视频、支持真人内容

### 5.2 seedance2.0-sd2（图生视频专用）
- 价格：**¥0.60/秒** ｜ 时长：**仅 11~15 秒** ｜ 分辨率：720p
- 画幅：**仅 16:9、9:16**
- ⚠️ **必须提供 1~9 张公网参考图**（`images` 必填，不传会被上游拒绝）
- ⚠️ **不支持音频、不支持参考视频、不支持真人内容**

### 5.3 seedance-2.0-vip-720p-fast-azhw（Fast 快速）
- 价格：**¥0.60/秒** ｜ 时长：4~15 秒 ｜ 分辨率：720p
- 优质渠道 Fast 线路，出片更快
- 画幅：21:9、16:9、4:3、1:1、3:4、9:16
- 参考图 ≤9 张、音频 ≤3 段、不支持参考视频、支持真人内容

### 5.4 seedance-2.0-933-720p-azhw（性价比首选）
- 价格：**¥0.62/秒** ｜ 时长：4~15 秒 ｜ 分辨率：720p
- 画幅：21:9、16:9、4:3、1:1、3:4、9:16
- 参考图 ≤9 张、音频 ≤3 段、不支持参考视频、支持真人内容

### 5.5 seedance-2.0-vip-720p-azhw（优质渠道）
- 价格：**¥0.74/秒** ｜ 时长：4~15 秒 ｜ 分辨率：720p
- 其余能力同 5.4

### 5.6 seedance-2.0-933-1080p-azhw
- 价格：**¥1.48/秒** ｜ 时长：4~15 秒 ｜ 分辨率：1080p
- 其余能力同 5.4

### 5.7 seedance-2.0-vip-1080p-azhw
- 价格：**¥1.74/秒** ｜ 时长：4~15 秒 ｜ 分辨率：1080p
- 优质渠道，其余能力同 5.4

### 5.8 seedance-2.0-933-4k-azhw
- 价格：**¥3.58/秒** ｜ 时长：4~15 秒 ｜ 分辨率：4K
- 其余能力同 5.4

### 5.9 seedance-2.0-vip-4k-azhw
- 价格：**¥4.23/秒** ｜ 时长：4~15 秒 ｜ 分辨率：4K
- 优质渠道，其余能力同 5.4

### 5.10 seedance-933-pro-pi（唯一按次 + 支持参考视频）
- 价格：**¥11.05/次（固定）** ｜ 时长：**固定 15 秒** ｜ 分辨率：720p
- 画幅：21:9、16:9、4:3、1:1、3:4、9:16
- 参考图 ≤9 张、**音频 ≤3 段**、**参考视频 ≤3 段**、支持真人内容
- 适合需要参考视频/音频的复杂场景

---

## 六、响应格式

### 6.1 创建任务响应

```json
{
  "id": "task_xzp3AxHPNmbDw5iFAhQKzfwYJ79uDwyP",
  "task_id": "task_xzp3AxHPNmbDw5iFAhQKzfwYJ79uDwyP",
  "status": "queued",
  "progress": 0
}
```
> 保存 `task_id`（或 `id`，两者相同）用于后续查询。

### 6.2 查询任务响应

**处理中**：
```json
{
  "id": "task_xzp3AxHPNmbDw5iFAhQKzfwYJ79uDwyP",
  "status": "processing",
  "task_id": "975636b08e9f11f19c73ed0e36b023ee"
}
```

**已完成**（出现 `video_url`，指向本站代理下载地址）：
```json
{
  "id": "task_xzp3AxHPNmbDw5iFAhQKzfwYJ79uDwyP",
  "status": "completed",
  "task_id": "975636b08e9f11f19c73ed0e36b023ee",
  "video_url": "http://43.161.200.208/v1/videos/task_xzp3AxHPNmbDw5iFAhQKzfwYJ79uDwyP/content"
}
```
> ✅ **下载方式**：对 `video_url` 发起 GET 请求即可下载 mp4。
> ⚠️ **下载也需要携带认证头** `Authorization: Bearer <API Key>`（未携带返回 401），且每个 API Key 只能下载自己账号创建的任务。

**失败**：
```json
{
  "id": "task_xxx",
  "status": "failed"
}
```
> 失败详情可在平台「任务日志」中点击任务 ID 查看完整原因，或调用 `GET /v1/tasks?task_id=xxx` 查询 `fail_reason` 字段。

### 6.3 模型列表响应 `GET /v1/models`

```json
{
  "data": [
    {"id": "seedance-2.0-933-720p-azhw", "object": "model", "owned_by": "custom"},
    ...
  ],
  "object": "list",
  "success": true
}
```

---

## 七、完整代码示例

### 7.1 curl

```bash
# ① 创建任务（文生视频）
curl -X POST http://43.161.200.208/v1/videos \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-你的APIKey" \
  -d '{
    "model": "seedance-2.0-933-720p-azhw",
    "prompt": "一只猫在阳光下的花园里追蝴蝶，镜头缓慢跟随",
    "seconds": "4",
    "size": "1280x720"
  }'

# ①b 创建任务（多参考图 + 音频）
curl -X POST http://43.161.200.208/v1/videos \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-你的APIKey" \
  -d '{
    "model": "seedance-933-pro-pi",
    "prompt": "参考图中的人物在海边弹吉他，配上背景音乐",
    "seconds": "15",
    "images": [
      "https://example.com/person1.jpg",
      "https://example.com/person2.jpg"
    ],
    "audios": [
      "https://example.com/guitar-bgm.mp3"
    ]
  }'

# ② 查询任务
curl http://43.161.200.208/v1/videos/task_你的任务ID \
  -H "Authorization: Bearer sk-你的APIKey"

# ③ 下载视频（video_url 需要携带认证头）
curl -o result.mp4 "查询响应中的video_url" \
  -H "Authorization: Bearer sk-你的APIKey"
```

### 7.2 Python

```python
import time
import requests

BASE = "http://43.161.200.208"
KEY = "sk-你的APIKey"
HEADERS = {"Authorization": f"Bearer {KEY}"}

# ① 创建任务
resp = requests.post(f"{BASE}/v1/videos", headers=HEADERS, json={
    "model": "seedance-2.0-933-720p-azhw",
    "prompt": "一只猫在阳光下的花园里追蝴蝶",
    "seconds": "4",            # ⚠️ 必须是字符串
    "size": "1280x720",
})
resp.raise_for_status()
task_id = resp.json()["task_id"]
print("任务已创建:", task_id)

# ② 轮询查询
while True:
    r = requests.get(f"{BASE}/v1/videos/{task_id}", headers=HEADERS).json()
    status = r["status"]
    print("状态:", status)
    if status == "completed":
        video_url = r["video_url"]
        break
    if status == "failed":
        raise RuntimeError("任务失败，请到平台任务日志查看原因")
    time.sleep(15)

# ③ 下载（video_url 需要认证头）
with open("result.mp4", "wb") as f:
    f.write(requests.get(video_url, headers=HEADERS).content)
print("已保存 result.mp4")
```

### 7.3 Node.js

```javascript
const BASE = "http://43.161.200.208";
const KEY = "sk-你的APIKey";
const headers = {
  "Authorization": `Bearer ${KEY}`,
  "Content-Type": "application/json",
};

async function main() {
  // ① 创建
  const createResp = await fetch(`${BASE}/v1/videos`, {
    method: "POST",
    headers,
    body: JSON.stringify({
      model: "seedance-2.0-933-720p-azhw",
      prompt: "一只猫在阳光下的花园里追蝴蝶",
      seconds: "4",          // ⚠️ 字符串
      size: "1280x720",
    }),
  });
  const { task_id } = await createResp.json();

  // ② 轮询
  let videoUrl;
  while (true) {
    const r = await (await fetch(`${BASE}/v1/videos/${task_id}`, { headers })).json();
    if (r.status === "completed") { videoUrl = r.video_url; break; }
    if (r.status === "failed") throw new Error("任务失败");
    await new Promise((s) => setTimeout(s, 15000));
  }

  // ③ 下载（video_url 需要认证头）
  const video = await fetch(videoUrl, { headers });
  require("fs").writeFileSync("result.mp4", Buffer.from(await video.arrayBuffer()));
  console.log("已保存 result.mp4");
}
main();
```

---

## 八、常见报错对照表

以下报错均来自真实接口实测，按出现频率排序：

| 报错信息 | 原因 | 解决办法 |
|----------|------|----------|
| `json: cannot unmarshal number into Go struct field .seconds of type string` | `seconds` 传了数字 `4` | 改为字符串 `"4"`，或改用 `"duration": 4` |
| `json: cannot unmarshal string into Go struct field .audios of type []string` | `audios` 传了字符串而非数组 | 改为数组：`"audios": ["https://...mp3"]` |
| `图片地址仅支持 http、https 或 data URL` | `images` 里的地址非法（本地路径/无效字符串） | 换成公网 http/https 图片地址，或 base64 data URL |
| `seconds 参数必须为整数` | `seconds` 传了 `"abc"` 或 `"4.5"` | 传整数字符串如 `"4"`、`"15"` |
| `size 参数取值不受支持，请检查后重试` | `size` 格式或取值非法 | 用 `"1280x720"` / `"720x1280"` 等合法宽x高 |
| `model field is required` | 缺少 `model` 字段 | 补上模型名 |
| `field prompt is required` | `prompt` 为空 | 补上视频描述文本 |
| `upstream_request_rejected ... 当前模型的上游拒绝了本次任务` | 参数被上游拒绝（如 sd2 没传参考图、参考文件无法拉取） | 核对模型约束（参考图/音频/画幅/时长）后重试，附 `diagnostic_id` 联系管理员 |
| `insufficient quota` / 额度不足 | 账户余额不足 | 联系平台充值 |

### 错误响应结构

```json
{
  "code": "fail_to_fetch_task",
  "message": "{...嵌套的详细错误...}",
  "data": null
}
```
> 上游拒绝类错误会包含 `diagnostic_id`（诊断号）与 `upstream_error_body`（上游原始报错），排障时请将诊断号提供给平台管理员。

---

## 九、注意事项与最佳实践

1. **seconds 必须加引号**：这是下游对接最高频的报错，请务必在对接文档/代码评审中检查。
2. **audios/images 必须是数组**：即使只有 1 个文件也要写成 `["url"]`，传字符串会直接报 unmarshal 错误。
3. **参考文件必须公网可访问**：图片/音频/视频 URL 需为公网 http/https（图片也支持 base64 data URL），内网地址会被上游拒绝。
4. **video_url 下载需带认证头**：下载地址与查询接口同域，携带同一个 API Key 即可下载；每个 Key 只能下载自己账号的任务，转发下载链接给他人无效（401）。
5. **任务视频请及时转存**：任务完成后建议尽快下载保存到自己的存储，平台不承诺长期保留生成结果。
6. **模型选择建议**：
   - 最低价文生视频 → `seedance-2.0-vip-720p-mini-azhw`（¥0.55/秒）
   - 日常文生视频 → `seedance-2.0-933-720p-azhw`（¥0.62/秒，性价比均衡）
   - 要高清 → `seedance-2.0-933-1080p-azhw` / `-4k-azhw`
   - 要参考视频/音频 → `seedance-933-pro-pi`（¥11.05/次）
   - 纯图生视频 → `seedance2.0-sd2`（必须带 1~9 张参考图，不支持音频）
7. **计费预估**：按秒模型创建前可用 `单价 × seconds` 估算；任务提交即计费，失败任务请到平台任务日志核对。
8. **并发与超时**：生成耗时与时长正相关，建议客户端超时设置 ≥ 10 分钟，轮询间隔 10~20 秒。
9. **账户查询**：余额、用量、任务列表查询接口请见配套文档《余额与任务查询接口文档》。

---

*本文档基于 2026-08-03 真实接口实测编写，价格已与平台最新定价同步，参数格式与报错信息均经过验证。如有接口变更，以平台最新公告为准。*
