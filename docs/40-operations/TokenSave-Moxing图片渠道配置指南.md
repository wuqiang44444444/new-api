---
status: current
owner: Dev Team
last-reviewed: 2026-07-27
---

# TokenSave / Moxing 图片渠道配置指南

## 1. 适用范围

本文用于通过 **Advanced Custom** 渠道接入 TokenSave 或 Moxing 图片上游。当前生产实现支持：

- `seedream-5-0-260128`
- `doubao-seedream-4-5-251128`
- `gemini-3.1-flash-image-preview-usage`

客户端统一调用：

```text
POST /v1/images/generations
```

上游同步完成时返回 OpenAI 图片 `data[]`；上游返回异步任务时，系统会持久化为共享 `media_image` Task，并在等待期内继续轮询或向客户端返回本站 HTTP 202 任务。

本文不适用于：

- 固定尺寸价模型 `Gemini-3.1-Flash-Image-Preview`；
- `response_format=b64_json`；
- `/v1/images/edits`；
- 已存在的 `DoubaoVideo` 视频渠道。图片渠道必须单独新建为 Advanced Custom，不能把视频渠道直接改成图片渠道。

## 2. 配置前检查

### 2.1 上游信息

准备以下信息：

| 配置项 | TokenSave | Moxing |
| --- | --- | --- |
| 渠道 Base URL | `https://tokensave.pro` | `https://www.moxing.pro` |
| 创建路径 | 优先 `/v1/images/generations` | 当前公开合同使用 `/v1/media/generations` |
| 查询路径 | 系统固定使用 `/v1/media/tasks/:id` | 系统固定使用 `/v1/media/tasks/:id` |
| 鉴权 | `Authorization: Bearer <API Key>` | `Authorization: Bearer <API Key>` |

Base URL 只填站点根地址，不追加 `/v1`，也不要以 `/` 结尾。Moxing 文档把 API Base URL 写作 `https://www.moxing.pro/v1`，但本渠道把 `/v1/...` 放在路由的“上游路径”中，所以渠道 Base URL 仍应填写 `https://www.moxing.pro`。

先在供应商控制台确认当前 Key 实际开通的模型。不要仅因为模板列出了三个模型，就把未开通的模型暴露给用户。

### 2.2 后台任务

生产环境应保持：

```text
UPDATE_TASK=true
TASK_TIMEOUT_MINUTES=1440
```

`UPDATE_TASK` 未启用时，客户端断开或请求等待结束后的异步图片任务不能依靠后台轮询完成。`TASK_TIMEOUT_MINUTES` 应大于上游任务 P99/SLA 加安全余量；建议保持默认 1440 分钟。

### 2.3 计费

在创建渠道前，先确定每个对外模型的售价：

- 当前三个模型均可先按管理员配置的固定 `ModelPrice` 计费，并按最终成功图片数量结算；
- `gemini-3.1-flash-image-preview-usage` 在完成真实 usage 字段和上游账单核验前，不得启用 `tiered_expr`；
- 价格必须配置在客户端使用的公开模型名上，而不是模型映射后的上游模型名上；
- 固定尺寸价 `Gemini-3.1-Flash-Image-Preview` 无论直接配置还是通过模型映射命中，都会在请求上游前被拒绝。

价格与分组倍率的配置方法见 [价格设置与倍率说明](价格设置与倍率说明.md) 和 [用户分组、计费分组与模型折扣配置说明](用户分组与模型折扣配置说明.md)。

## 3. 在管理后台创建渠道

进入“渠道管理”，新增一条渠道。

### 3.1 基本信息

| 字段 | 配置要求 |
| --- | --- |
| 渠道类型 | `Advanced Custom` |
| 名称 | 建议包含供应商和用途，例如 `TokenSave Images` |
| Base URL | TokenSave 填 `https://tokensave.pro`；Moxing 填 `https://www.moxing.pro` |
| API Key | 填供应商平台 Key，不是本站用户令牌 |
| 状态 | 启用 |
| 分组 | 选择允许使用该线路的调用分组 |
| Models | 只添加该 Key 已开通、准备对外提供的公开模型名 |

如果客户端直接使用三个标准模型名，可从以下列表按需选择：

```text
seedream-5-0-260128
doubao-seedream-4-5-251128
gemini-3.1-flash-image-preview-usage
```

### 3.2 填入 Advanced Custom 图片模板

在“Advanced Custom Routes”中打开编辑器：

1. 选择模板 `TokenSave / Moxing Images`。
2. 点击“填入模板”。
3. 检查生成的路由。
4. TokenSave 保持“上游路径”为 `/v1/images/generations`。
5. Moxing 把“上游路径”改为 `/v1/media/generations`。
6. 删除供应商账号未开通的模型。
7. 保存路由配置。

生产路由必须满足：

| 路由字段 | 值 |
| --- | --- |
| 客户端路径 | `/v1/images/generations` |
| 上游路径 | TokenSave：`/v1/images/generations`；Moxing：`/v1/media/generations` |
| 执行策略 / Converter | `media_task_image_blocking` |
| 鉴权类型 | `Header` |
| Header 名称 | `Authorization` |
| Header 值 | `Bearer {api_key}` |
| Models | 与该路由要承接的客户端公开模型名一致 |

不要把 Converter 改为 `none`。`none` 只能处理同步 OpenAI 兼容响应，无法安全接管上游 HTTP 202 任务。

### 3.3 JSON 配置

TokenSave 可直接使用：

```json
{
  "advanced_routes": [
    {
      "incoming_path": "/v1/images/generations",
      "upstream_path": "/v1/images/generations",
      "converter": "media_task_image_blocking",
      "models": [
        "seedream-5-0-260128",
        "doubao-seedream-4-5-251128",
        "gemini-3.1-flash-image-preview-usage"
      ],
      "auth": {
        "type": "header",
        "name": "Authorization",
        "value": "Bearer {api_key}"
      }
    }
  ]
}
```

Moxing 当前公开的统一媒体任务合同使用异步创建入口，配置为：

```json
{
  "advanced_routes": [
    {
      "incoming_path": "/v1/images/generations",
      "upstream_path": "/v1/media/generations",
      "converter": "media_task_image_blocking",
      "models": [
        "seedream-5-0-260128",
        "doubao-seedream-4-5-251128",
        "gemini-3.1-flash-image-preview-usage"
      ],
      "auth": {
        "type": "header",
        "name": "Authorization",
        "value": "Bearer {api_key}"
      }
    }
  ]
}
```

如果供应商只开通其中部分模型，应同时从渠道的 `Models` 和路由的 `models` 中删除其余模型。

### 3.4 关闭不兼容开关

在“渠道额外设置”中确认：

- “透传请求体 / Pass Through Body”：关闭；
- “跳过异步任务轮询延迟”：生产环境关闭；
- Header Override：不要用于本图片路由鉴权；
- 参数覆盖模板：初次上线保持为空。

`media_task_image_blocking` 会受控读取图片扩展字段、注入 `capability=image_generation`、处理模型映射并校验计费数量。开启请求体透传会绕过这些必要处理。

### 3.5 模型映射

公开模型名与上游模型名一致时，模型映射留空。

需要对外使用别名时，映射方向必须是：

```text
客户端公开模型名 -> 上游真实模型名
```

例如：

```json
{
  "my-seedream-5": "seedream-5-0-260128"
}
```

此时必须同时配置：

- 渠道 `Models`：`my-seedream-5`
- Advanced Custom 路由 `models`：`my-seedream-5`
- 模型价格：配置在 `my-seedream-5`
- 模型映射：`my-seedream-5 -> seedream-5-0-260128`

系统会按映射后的已知模型执行尺寸和数量校验。不要把固定尺寸价 Gemini 映射成 usage 模型或反向映射，两个方向都会被拒绝。

## 4. 保存后测试

### 4.1 渠道内置测试

保存渠道后，在渠道测试中选择“图片生成”端点和一个已开通模型。系统会按目标模型生成兼容的最小请求。

渠道测试会真实调用图片生成接口，可能产生上游费用。建议使用专门的低额度测试 Key，并逐个模型验收。

### 4.2 通过本站 API 验收

以下请求中的地址是本站 API 地址，`<NEW_API_KEY>` 是本站用户令牌，不是渠道上游 Key。

Seedream 5.0：

```bash
curl --fail-with-body \
  --request POST \
  'http://localhost:8100/v1/images/generations' \
  --header 'Authorization: Bearer <NEW_API_KEY>' \
  --header 'Content-Type: application/json' \
  --header 'Idempotency-Key: <本次生成的稳定唯一值>' \
  --data-raw '{
    "model": "seedream-5-0-260128",
    "prompt": "一只橘猫坐在窗边看雨，柔和电影光影",
    "n": 1,
    "size": "2K",
    "response_format": "url"
  }'
```

Seedream 4.5：

```bash
curl --fail-with-body \
  --request POST \
  'http://localhost:8100/v1/images/generations' \
  --header 'Authorization: Bearer <NEW_API_KEY>' \
  --header 'Content-Type: application/json' \
  --header 'Idempotency-Key: <本次生成的稳定唯一值>' \
  --data-raw '{
    "model": "doubao-seedream-4-5-251128",
    "prompt": "极简风格的未来城市海报",
    "n": 1,
    "size": "2048x2048",
    "response_format": "url"
  }'
```

Gemini 3.1 usage：

```bash
curl --fail-with-body \
  --request POST \
  'http://localhost:8100/v1/images/generations' \
  --header 'Authorization: Bearer <NEW_API_KEY>' \
  --header 'Content-Type: application/json' \
  --header 'Idempotency-Key: <本次生成的稳定唯一值>' \
  --data-raw '{
    "model": "gemini-3.1-flash-image-preview-usage",
    "prompt": "一张适合产品首页的抽象科技背景图",
    "n": 1,
    "size": "1K",
    "aspect_ratio": "16:9",
    "response_format": "url"
  }'
```

`capability=image_generation` 可由系统自动注入，客户端不必填写。

### 4.3 验收异步任务

要明确测试本站 HTTP 202 合同，可在创建请求中增加：

```text
Prefer: respond-async
```

该首部只在上游返回异步任务时生效；如果上游已经同步返回 HTTP 200，本站仍直接返回图片结果。

收到 HTTP 202 后：

1. 保存响应中的本站任务 `id`，例如 `task_xxx`。
2. 优先使用响应头 `Location` 指向的路径查询。
3. 使用与创建请求相同用户的本站令牌。

```bash
curl --fail-with-body \
  --request GET \
  'http://localhost:8100/v1/images/tasks/task_xxx' \
  --header 'Authorization: Bearer <NEW_API_KEY>'
```

终态 `completed` 的图片位于：

```text
result.data[].url
```

同一次业务生成发生网络重试时必须复用原 `Idempotency-Key`；新的生成意图必须使用新键。不要在超时后直接用新键重新 POST，否则可能在上游创建并计费两次。

## 5. 模型参数速查

| 模型 | `size` | `n` | 参考图片和扩展字段 |
| --- | --- | --- | --- |
| `gemini-3.1-flash-image-preview-usage` | `1K`、`2K`、`4K` | 省略时为 1，且不得超过全局上限 | `reference_images` 最多 10 张；支持 `aspect_ratio` |
| `doubao-seedream-4-5-251128` | `2048x2048`、`2304x1728`、`1728x2304`、`2560x1440`、`1440x2560`、`2496x1664`、`1664x2496`、`3024x1296` | 1–4 | 图生图使用单个顶层 `image` |
| `seedream-5-0-260128` | `2K`、`3K` | 1–128 | `image` 可为一个 URL 或最多 14 个 URL；支持受控 `extra` |

所有参考图只接受 HTTP(S) URL。`response_format` 只能省略或填写 `url`。

Seedream 5.0 连续多图使用：

```json
{
  "n": 4,
  "extra": {
    "sequential_image_generation": "auto",
    "sequential_image_generation_options": {
      "max_images": 4
    },
    "watermark": false
  }
}
```

其中 `max_images` 必须为 1–128，且不得大于 `n`。

## 6. 上线验收清单

- [ ] 图片渠道是独立的 Advanced Custom，没有修改现有 DoubaoVideo 渠道。
- [ ] Base URL 是站点根地址，不含 `/v1` 和尾部 `/`。
- [ ] TokenSave 使用 `/v1/images/generations`，或已按供应商实际合同确认替代路径。
- [ ] Moxing 使用 `/v1/media/generations`，或已通过供应商文档和真实请求确认同步入口。
- [ ] Converter 为 `media_task_image_blocking`。
- [ ] 路由鉴权为 `Authorization: Bearer {api_key}`。
- [ ] 请求体透传已关闭。
- [ ] 渠道 `Models`、路由 `models`、模型映射和模型价格使用一致的公开模型名。
- [ ] `UPDATE_TASK=true`，任务生命周期覆盖上游 SLA。
- [ ] 三个准备上线的模型已分别完成真实生成。
- [ ] 至少验证一次 HTTP 200 和一次 HTTP 202/任务查询。
- [ ] 已核对成功结算、失败退款和实际图片数量。
- [ ] usage 模型未在账单核验前启用 `tiered_expr`。

## 7. 常见问题

| 现象 | 主要原因 | 处理 |
| --- | --- | --- |
| “无可用渠道” | 渠道状态、分组、渠道 Models 或路由 models 不匹配 | 用客户端公开模型名逐项对齐，并检查调用令牌分组 |
| 保存时报 converter 与路径不匹配 | `media_task_image_blocking` 被配到了非图片入口 | 客户端路径改为 `/v1/images/generations` |
| 上游返回 404 | Base URL 含重复 `/v1`，或 TokenSave/Moxing 创建路径选错 | Base URL 改为站点根地址，再核对上游路径 |
| `response_format must be url` | 客户端传了 `b64_json` | 删除该字段或改为 `url` |
| `size is not supported` | 使用了其他图片模型的尺寸 | 按第 5 节为当前模型选择尺寸 |
| 上游 202 后任务长期不完成 | 后台轮询关闭、查询鉴权失败或上游任务异常 | 检查 `UPDATE_TASK`、Task 日志和渠道路由鉴权 |
| 相同请求生成了两次 | 网络重试时换了幂等键或未使用幂等键 | 同一业务生成始终复用一个 `Idempotency-Key` |
| usage 模型费用不符 | 未核验 usage 就启用了表达式计费 | 立即回退固定 `ModelPrice`，按真实账单核验 |
| `quality`、`style` 等字段被拒绝 | 当前执行策略采用允许表，未支持这些 OpenAI 扩展字段 | 删除未支持字段，不要开启请求体透传绕过 |

## 8. 相关文档

- [TokenSave / Moxing 图片渠道最小入侵接入方案](../80-dev/2026-07-27-TokenSave-Moxing图片渠道最小入侵接入方案.md)
- [媒体图片异步任务运维手册](媒体图片异步任务运维手册.md)
- [图片异步任务共享 Task 持久化闭环方案](../80-dev/2026-07-27-图片异步任务共享Task持久化闭环方案.md)
- [价格设置与倍率说明](价格设置与倍率说明.md)
- [TokenSave 图片模型 API](https://tokensave.pro/docs/api/image)
- [TokenSave 媒体任务机制](https://tokensave.pro/docs/api/media-task)
- [Moxing API 文档](https://www.moxing.pro/docs)
