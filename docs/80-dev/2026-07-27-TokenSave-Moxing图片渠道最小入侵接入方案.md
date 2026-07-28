---
status: draft
owner: Dev Team
last-reviewed: 2026-07-28
---

# TokenSave / Moxing 图片渠道最小入侵接入方案

## 1. 目标

在不新增专用渠道类型、不改数据库结构、不侵入通用图片中继主链路的前提下，通过 Advanced Custom 渠道接入 TokenSave / Moxing 图片上游，并支持以下模型：

- `seedream-5-0-260128`
- `doubao-seedream-4-5-251128`
- `gemini-3.1-flash-image-preview-usage`

这里的“支持”表示执行策略已具备请求校验和协议适配能力，不等同于三个模型都已作为生产 SKU 发布。本轮公开范围只有 `seedream-5` 和 `nano-banana-2`；`doubao-seedream-4-5-251128` 暂不配置公开别名、生产 ability 或价格，后续发布必须另行完成真实任务与成本验收。

明确不支持按 `1K`、`2K`、`4K` 分档固定价格的 `Gemini-3.1-Flash-Image-Preview`。该模型不进入渠道模板；即使管理员手工配置或通过模型映射命中，也由执行策略在发送上游前返回 HTTP 400。

本次北向响应格式只支持 `response_format=url`；省略时按 `url` 处理，其他值均在发送上游前拒绝。

对外继续提供 OpenAI 图片接口：

```text
POST /v1/images/generations
```

上游在等待窗口内完成时直接使用同步结果；上游返回异步任务时，由渠道适配逻辑完成轮询，并最终转换为 OpenAI 图片 `data[]` 响应。

本方案遵循以下优先级：

1. 降低未来同步上游代码时的冲突面。
2. 避免为单一上游新增完整 ChannelType。
3. 保持现有图片预扣、结算、退款和日志主链路不变。
4. 北向保持统一的 OpenAI 图片合同，南向差异收敛在供应商无关、可复用的媒体任务执行策略中。
5. TokenSave、Moxing 等供应商名称只出现在渠道配置、模型规则和运维示例中，不进入运行时常量、包名或架构组件名称。

## 2. 结论

推荐在 Advanced Custom 渠道内部增加轻量执行策略：

```text
media_task_image_blocking
```

总体边界为：

```text
北向统一合同
POST /v1/images/generations
        │
        ▼
现有图片路由、预扣、响应和结算主链路
        │
        ▼
Advanced Custom 南向执行策略
        ├─ OpenAI 同步图片协议
        └─ 统一媒体异步任务协议
```

`media_task_image_blocking` 不是新的公共渠道类型，也不是通用请求格式转换器。它属于 Advanced Custom 路由的南向执行策略，负责把同步 OpenAI 图片请求桥接到“创建媒体任务、轮询终态、归一化结果”的上游合同。

为降低当前改动面，第一阶段继续复用 Advanced Custom 路由已有的 `converter` 配置字段保存该策略枚举，但必须遵守以下边界：

- 不把它注册到 `service/relayconvert/request_registry.go`；该注册表只负责无网络 I/O 的请求格式转换。
- 不新增供应商专用 ChannelType、Router、数据库字段或任务表。
- 不根据 Base URL、Key 或供应商名称推断协议；管理员必须在渠道路由上显式选择执行策略。
- 如果未来出现多种带网络编排的执行策略，再单独评估是否增加 `execution_policy` 字段；本次不为字段纯化扩大配置结构和前后端改动面。

该边界遵循 `docs/20-architecture/decisions/0001-视频上游协议适配与任务执行快照.md` 的供应商中立约束，并延续 `docs/20-architecture/视频上游接入与异步任务架构.md` 中“北向薄适配、共享内部能力、南向按协议合同扩展”的原则。

TokenSave 已为图片模型提供同步兼容入口：

```text
POST /v1/images/generations
```

其行为为：

- 在默认等待窗口内完成：返回 HTTP 200 和 OpenAI 风格 `data[]`。
- 超过等待窗口：返回 HTTP 202 和 `task_id`。
- 任务结果通过 `GET /v1/media/tasks/:id` 查询。

因此不应无条件把所有图片请求转换为 `POST /v1/media/generations`。最小生产实现应采用：

```text
同步入口优先
    ├─ 200：沿用现有 OpenAI 图片响应链路
    └─ 202：读取 task_id，轮询媒体任务，转换为内部 OpenAI 图片响应
```

如果某个兼容上游只实现异步创建接口，也允许把 Advanced Custom 的 `upstream_path` 配置为 `/v1/media/generations`；执行策略对 HTTP 200 或 202 的任务信封采用相同的持久化和轮询处理。只要南向请求、创建响应、任务状态和结果合同一致，其他上游应能通过渠道配置复用该策略，不需要新增代码。

## 3. 上游协议核对

参考文档：

- [Seedream 5.0](https://tokensave.pro/docs/models/seedream-5-0-260128)
- [Seedream 4.5](https://tokensave.pro/docs/models/doubao-seedream-4-5-251128)
- [Gemini 3.1 Flash Image Preview 固定尺寸价版（仅用于排除范围核对）](https://tokensave.pro/docs/models/Gemini-3.1-Flash-Image-Preview)
- [Gemini 3.1 Flash Image Preview 按量版](https://tokensave.pro/docs/models/gemini-3.1-flash-image-preview-usage)
- [图片模型 API](https://tokensave.pro/docs/api/image)
- [媒体任务机制](https://tokensave.pro/docs/api/media-task)

### 3.1 统一接口

三个目标模型都支持：

```text
POST /v1/images/generations
POST /v1/media/generations
GET  /v1/media/tasks/:id
```

图片任务公共字段包括：

- `model`
- `capability=image_generation`
- `prompt`
- `size`
- `response_format`
- 可选的 `n`
- 可选的参考图片和画幅字段

任务状态只应识别：

- `queued`
- `running`
- `succeeded`
- `failed`

成功结果可能位于：

- `result.primary_url`
- `result.urls`

### 3.2 模型差异

| 模型 | 尺寸 | 数量与参考图 | 专有字段 |
|---|---|---|---|
| `gemini-3.1-flash-image-preview-usage` | `1K`、`2K`、`4K` | `n` 默认 1；`reference_images` 最多 10 张 | `aspect_ratio`；实际 Token 结算见阶段 D |
| `doubao-seedream-4-5-251128` | `2048x2048`、`2304x1728`、`1728x2304`、`2560x1440`、`1440x2560`、`2496x1664`、`1664x2496`、`3024x1296` | `n=1–4`；图生图参考图使用顶层 `image` | `callback_url` 不应向上游透传 |
| `seedream-5-0-260128` | `2K`、`3K` | `image` 可为字符串或数组；`images` 为兼容别名；参考图最多 14 张 | `extra.sequential_image_generation`、`extra.sequential_image_generation_options.max_images`、`extra.watermark` |

`Gemini-3.1-Flash-Image-Preview` 虽然使用相同媒体协议，但其固定尺寸价销售合同不在本站支持范围内，不能通过把它映射为 usage 模型绕过拒绝。

## 4. 现有系统能力与缺口

### 4.1 已具备能力

Advanced Custom 已具备：

- `/v1/images/generations` 路由配置。
- 按请求路径和模型精确筛选渠道。
- Bearer、Header、Query 等鉴权配置。
- 原生 OpenAI 图片请求和响应处理。
- 模型级路由匹配，避免图片渠道被错误选入其他端点。

现有 OpenAI 图片响应处理能够：

- 返回上游 `data[]`。
- 读取响应中的图片数量。
- 用实际成功图片数量更新固定价格中的 `n` 倍率。
- 复用现有预扣和最终结算链路。

### 4.2 请求字段会被丢弃

`dto.ImageRequest` 会读取未知字段到 `Extra`，但重新序列化时不会合并这些字段。

因此以下 TokenSave 字段在普通 Advanced Custom 原生转换模式下会丢失：

- `capability`
- `aspect_ratio`
- `reference_images`
- `callback_url`
- `extra`

临时开启“请求体直接透传”可以保留字段，但会产生新的问题：

- 模型映射结果不会写回原始请求体。
- 未知字段没有经过供应商级边界校验。
- `extra.max_images` 等可能参与上游费用的参数存在计费绕过风险。
- 透传是渠道级开关，影响该渠道配置的全部路由。

因此请求体透传只适合联调，不应作为正式生产方案。

### 4.3 HTTP 202 会被当作错误

当前图片中继在适配器响应转换之前要求上游状态码为 HTTP 200。除 Replicate 的 HTTP 201 特例外，HTTP 202 会直接进入错误处理。

这会产生以下后果：

1. TokenSave 已经创建图片任务并可能产生上游费用。
2. 本系统把 202 当作调用失败。
3. 用户预扣额度被退款。
4. 上游任务仍可能继续成功，形成无法自动对账的成本损失。

这是配置模式无法可靠上线的核心原因。

### 4.4 内置渠道测试不适用

当前图片渠道测试固定发送：

```json
{
  "model": "<test-model>",
  "prompt": "a cute cat",
  "n": 1,
  "size": "1024x1024"
}
```

该请求：

- 缺少必填的 `capability=image_generation`。
- 尺寸不符合 usage Gemini 模型的 `1K/2K/4K` 约束。
- 尺寸不符合 Seedream 4.5 的固定枚举。
- 尺寸不符合 Seedream 5.0 的 `2K/3K` 约束。

因此需要模型感知的渠道测试请求，不能使用当前通用图片测试体判断渠道是否可用。

此外，当前未指定 endpoint type 时的自动检测只对 VolcEngine Seedream 有图片特判。Advanced Custom 图片路由可能仍构造聊天请求。正式实现应优先根据渠道路由声明的 endpoint type 选择图片测试体，不应继续按 TokenSave、Moxing 或更多模型名称增加硬编码分支。

## 5. 临时联调配置

在正式适配逻辑完成前，可以用 Advanced Custom 进行同步成功路径联调。

### 5.1 渠道配置

- 渠道类型：Advanced Custom
- Base URL：`https://tokensave.pro`
- API Key：TokenSave API Key
- 开启：请求体直接透传
- 模型映射：不配置
- 模型列表：
  - `seedream-5-0-260128`
  - `doubao-seedream-4-5-251128`
  - `gemini-3.1-flash-image-preview-usage`

Advanced Custom 路由：

```json
{
  "advanced_routes": [
    {
      "incoming_path": "/v1/images/generations",
      "upstream_path": "/v1/images/generations",
      "converter": "none",
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

### 5.2 联调限制

该配置只能验证 HTTP 200 同步成功路径，不满足正式上线要求：

- HTTP 202 不会自动轮询。
- 不支持安全的模型映射。
- 不具备供应商参数边界校验。
- 内置渠道测试请求不适用。

## 6. 正式实现设计

### 6.1 Advanced Custom 路由配置

正式实现后，渠道配置应改为：

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

正式模式下必须关闭“请求体直接透传”，由 `media_task_image_blocking` 完成受控转换。

这里继续使用字段名 `converter` 只是为了复用现有 Advanced Custom 配置合同、避免新增数据库和前端表单结构；该枚举在运行时按执行策略处理，不属于 `relayconvert` 的纯格式转换器。

### 6.2 请求处理

执行策略读取经过通用解析的 `dto.ImageRequest` 及其 `Extra` 字段，构建供应商无关的媒体图片任务请求。模型差异通过公开模型名对应的受控规则处理，不定义供应商专用运行时类型。

模型规则选择不能只依赖映射后的上游模型名：如果北向原始模型是三个目标模型之一，应按原始模型执行尺寸、数量和扩展字段校验，再把映射后的模型名写入南向请求；如果北向使用自定义别名、映射结果是已知模型，则按映射后的已知模型校验。这样既允许供应商别名映射，又不会因为把标准模型映射为自定义名称而落入宽松的未知模型规则。

固定尺寸价模型的拒绝同时检查北向原始模型和映射后的上游模型，防止通过任一方向的模型映射绕过范围限制。

处理顺序：

1. 使用现有模型映射结果写入上游 `model`。
2. 强制设置或验证 `capability=image_generation`。
3. 保留 `prompt`、`size`、`response_format`。
4. 保留显式 `n`，缺省时使用 1。
5. 按模型选择性保留：
   - `aspect_ratio`
   - `image`
   - `images`
   - `reference_images`
   - 允许的 `extra` 子字段
6. 删除 `callback_url`。
7. 拒绝未列入允许表且可能改变上游费用或行为的字段。

可选标量字段必须使用指针类型和 `omitempty`，确保显式 `0`、`false` 与字段缺失不会混淆。

### 6.3 模型级校验

校验应放在新增的媒体图片任务执行文件中，不把具体上游模型规则扩散到通用 `dto.ImageRequest`。

至少校验：

- 所有模型：
  - `prompt` 非空。
  - `response_format` 只允许 `url`；省略时补为 `url`。
  - `n` 为正数且不超过 `dto.MaxImageN`。
- `gemini-3.1-flash-image-preview-usage`：
  - `size` 只能为 `1K/2K/4K`。
  - `reference_images` 不超过 10 张。
  - `aspect_ratio` 只能为文档枚举。
- `Gemini-3.1-Flash-Image-Preview`：
  - 无条件拒绝，不进入南向请求。
- Seedream 4.5：
  - `n` 为 1–4。
  - `size` 只能为文档指定像素尺寸。
  - 图生图使用单个顶层 `image`。
- Seedream 5.0：
  - `size` 只能为 `2K/3K`。
  - `image/images` 数量不超过 14。
  - `sequential_image_generation_options.max_images` 必须为正数且不超过 `dto.MaxImageN`。
  - TokenSave 当前公开元数据未声明比 `dto.MaxImageN` 更小的 `max_images` 上限，因此本站不另建臆测常量；若上游后续公布更小上限，再在模型规则中收紧。
  - `max_images` 必须小于等于请求 `n`，否则在发送上游前拒绝，确保预扣数量覆盖上游最多输出数量。
  - 多图输出数量必须参与预扣和最终结算。

所有能够成为计费乘数的数量必须在进入额度计算前完成校验。

### 6.4 同步与异步响应

初始请求返回 HTTP 200 时：

- 如果 Body 是 OpenAI `data[]`，不重建业务结果，直接交给现有 OpenAI 图片响应处理。
- 如果 Body 是 `status=queued/running + task_id` 的媒体任务信封，进入与 HTTP 202 相同的持久化和轮询流程。
- 如果任务信封已经 `succeeded`，校验结果 URL 后合成 OpenAI `data[]`。
- 如果任务信封已经 `failed` 或状态未知，失败关闭且不得重新创建。

初始请求返回 HTTP 202，或 HTTP 200 返回未完成任务信封时：

1. 有界读取响应体。
2. 读取并验证非空 `task_id`。
3. 只允许由 URI 非保留字符和冒号组成的 `task_id`，拒绝斜杠、反斜杠、百分号、查询/片段分隔符、空白、控制字符和其他未明确允许的字符。
4. 忽略上游返回的任意绝对 `poll_url`。
5. 使用渠道 Base URL 和经过路径转义的 `task_id` 构造：

   ```text
   GET /v1/media/tasks/:task_id
   ```

6. 复用渠道代理和鉴权配置。
7. 按退避策略轮询。
8. 遇到终态立即停止。

不得直接使用上游返回的绝对 `poll_url`，避免上游响应导致请求被引导到非预期主机。

### 6.5 轮询策略

第一阶段不新增环境变量、数据库字段或前端超时配置。阻塞执行策略在新增文件中定义 5 分钟的保守总等待上限，并按以下顺序确定实际截止时间：

1. 请求上下文已有更早截止时间时服从请求上下文。
2. `RELAY_TIMEOUT` 大于 0 且更短时，使用该值作为总等待上限。
3. 其他情况使用 5 分钟默认上限。

`TASK_TIMEOUT_MINUTES` 属于持久化后台任务，不用于本次同步阻塞轮询。以后只有真实上游普遍需要不同等待窗口时，才评估增加渠道级配置。

初始轮询参数：

- 首次间隔：1 秒。
- 逐步退避。
- 最大间隔：5 秒。
- 总等待上限：按上述规则计算，必须始终有界。
- 支持渠道现有的代理配置。
- 支持测试中的无等待模式，但生产环境不得高频轮询。

轮询期间应记录：

- 本系统请求 ID。
- 渠道 ID。
- 上游请求 ID。
- 上游任务 ID。
- 当前状态。
- 已等待时间。

实现中把创建请求 ID、最后一次轮询请求 ID、`task_id`、轮询次数和已等待毫秒数保存到请求上下文，并在成功消费日志和失败日志的 `other.admin_info.upstream_task` 中持久化。该字段继续沿用现有管理员信息过滤机制，不向普通用户暴露；运行日志使用同一组字段便于现场排查。

不得在正常日志中记录 API Key、原始 base64 图片或完整敏感提示词。

### 6.6 结果转换

任务成功时按以下优先级提取结果：

1. `result.urls`
2. `result.primary_url`

处理规则：

- 如果 `primary_url` 已存在于 `urls`，不得重复加入。
- 如果 `urls` 为空而 `primary_url` 非空，生成单项 `data`。
- 如果成功状态没有任何可用结果，按上游坏响应处理。
- 初期只生成 URL 响应：

  ```json
  {
    "created": 1774834790,
    "data": [
      {
        "url": "https://resource.example/image.png"
      }
    ]
  }
  ```

HTTP 202 的读取、轮询和结果合成必须全部在 Advanced Custom 的 `DoRequest` 返回前闭环。原因是通用 `ImageHelper` 会在调用适配器 `DoResponse` 之前拒绝非 200 响应，不能让 202 透传到 `DoResponse` 后再处理。

执行策略完成轮询后构造一个内部 HTTP 200 OpenAI 图片响应，再交回现有 OpenAI 图片 `DoResponse`。这样无需修改通用 `ImageHelper` 的响应和结算流程。

### 6.7 失败转换

任务失败时：

- 读取 `error_message`。
- 生成稳定的 OpenAI 错误结构。
- 标记为不可重试，避免外层渠道重试再次创建任务。
- 触发现有请求失败退款。

轮询超时时：

- 返回明确的上游任务超时错误。
- 标记为不可重试。
- 在管理员日志保留 `task_id`。
- 触发现有退款。

一旦取得 `task_id`，后续任何轮询网络错误、任务失败或超时都不得重新提交创建请求。

## 7. 计费设计

### 7.1 正确语义

路线图中的“只在成功后结算费用”应改为：

```text
请求前预扣
    ├─ 成功：按实际成功图片数量完成结算
    └─ 创建失败、任务失败、轮询超时：退款
```

这样与现有图片中继主链路一致。

### 7.2 图片数量

固定价格模式已通过 `n` 参与预扣，并能根据最终 OpenAI `data[]` 数量修正实际图片数。

媒体图片任务执行策略必须保证：

- 请求 `n` 已验证。
- Seedream 5.0 的 `max_images` 已验证，且必须满足 `max_images <= n`。
- 所有请求数量和最终 `data[]` 数量都不超过全局安全上限 `dto.MaxImageN`。
- 上游模型有更小限制时使用 `min(上游限制, dto.MaxImageN)`，不得另建重复的全局图片数量常量。
- 多图结果使用实际数量结算。
- 上游若违反请求并返回超过 `n` 张去重结果，同步 HTTP 200 和异步 Task 均在返回前按协议违规失败关闭并退款，不向用户补扣超发图片。

### 7.3 计费合同

本接入不实现尺寸感知固定价格：

- `Gemini-3.1-Flash-Image-Preview` 因按尺寸固定价而明确不支持。
- Seedream 按系统配置的 `ModelPrice` 和实际图片数量结算，不增加尺寸倍率、供应商价格表或专用价格常量。
- `gemini-3.1-flash-image-preview-usage` 如果在本站按固定 `ModelPrice` 销售，现有图片计费链路能够正确完成按结果数结算；但固定价格本身必须覆盖所有已开放尺寸和数量的上游成本。
- 如果 usage 模型必须按上游实际 Token 精确结算，则必须取得可信 usage 并由持久化 Task 完成终态结算；这属于阶段 D 的计费闭环，不是阶段 C 的尺寸价格功能。

2026-07-27 真实 `n=1` 文生图样本已经证明当前 `$0.077` 固定售价不能覆盖全尺寸：

| 尺寸 | 文本输入 Token | 图片输出 Token | 按 `$0.5/M` 文本输入、`$60/M` 图片输出计算 |
|---|---:|---:|---:|
| 1K | 16 | 1120 | `$0.067208` |
| 2K | 13 | 1680 | `$0.1008065` |
| 4K | 13 | 2520 | `$0.1512065` |

因此渠道 34 的 Nano `default` ability 已关闭，只在 `randy` 组灰度。恢复默认组前必须二选一：

1. 取得覆盖提示词、参考图、多结果和全部尺寸的可信 Token 预扣上界，使用 `tier("nano_image", p * 0.5 + img * 0.5 + img_o * 60)` 按实际 usage 结算。
2. 重新确定固定售价及其公开尺寸/数量合同，并以真实账单验证不会倒挂。

不能把已测 4K 的 `2520` 图片输出 Token 直接当作生产上界。

不得仅凭模型名中的 `usage` 推断可用 Token 数，也不得从图片尺寸反推 Token 用量。

### 7.4 上游晚成功风险

如果本系统轮询超时并退款，但上游任务后来成功，上游仍可能收费。

阻塞式最小方案不持久化图片任务，因此只能：

- 记录上游 `task_id`。
- 禁止重复提交。
- 在管理员日志中标记“本地超时退款、上游状态未知”。
- 通过运营对账识别成本损失。

该风险对生产闭环不可接受，因此阶段 D 调整为必需阶段，而不是继续扩大阻塞执行策略。

阶段 D 不得另建图片专用任务系统。它必须复用现有共享 Task 的持久化、状态机、后台轮询、终态 CAS、计费补偿和审计能力，同时保留图片与视频各自独立的北向 DTO、响应和错误外壳。

内部持久化可以解决客户端断开后的继续轮询和上游晚成功对账，但不会自动改变 OpenAI Images 的同步北向合同。完整方案见 [图片异步任务共享 Task 持久化闭环方案](./2026-07-27-图片异步任务共享Task持久化闭环方案.md)，其中通过供应商无关的本地 HTTP 202 和查询接口补齐用户结果闭环。

## 8. 代码组织与冲突控制

### 8.1 新增文件

建议新增：

```text
relay/channel/advancedcustom/media_task_image_blocking.go
relay/channel/advancedcustom/media_task_image_blocking_test.go
controller/channel_test_image_profile.go
controller/channel_test_image_profile_test.go
```

其中：

- `media_task_image_blocking.go`：供应商无关的请求类型、模型规则、202 解析、轮询、结果转换和错误转换。
- `media_task_image_blocking_test.go`：统一媒体任务协议回归测试；TokenSave / Moxing 只作为测试场景名称或 fixture 说明。
- `channel_test_image_profile.go`：按模型生成最小渠道测试请求。

允许这些新增文件包含少量局部重复，不为消除重复而重构通用图片链路。

### 8.2 现有文件的必要接线

仅做窄修改：

1. `relay/channel/advancedcustom/adaptor.go`
   - 在 `ConvertImageRequest` 中为 `media_task_image_blocking` 增加唯一窄特判，解除“所有非 `none` 图片 converter 均拒绝”的现有硬门禁。
   - 保持其他非 `none` 图片 converter 继续拒绝。
   - 在 `DoRequest` 中识别该策略，完成初始请求、202 轮询和内部 HTTP 200 响应合成后再返回。
   - 在 `DoResponse` 中仅把合成的 HTTP 200 交给现有 OpenAI 图片响应处理。

2. `dto/channel_settings.go`
   - 增加供应商无关的策略枚举常量。
   - 加入允许列表。
   - 在 `validateAdvancedCustomConverterPath` 中新增规则，限制只能用于 `/v1/images/generations`。

3. `web/src/features/channels/types.ts`
   - 增加供应商无关的策略联合类型。

4. `web/src/features/channels/lib/advanced-custom.ts`
   - 增加可视化选项。
   - 增加 TokenSave / Moxing 图片路由模板。

5. `controller/channel-test.go`
   - 根据 Advanced Custom 路由声明的图片 endpoint type 选择图片测试请求。
   - 仅保留一处窄调用；具体模型规则放入新增文件。

明确不修改 `service/relayconvert/request_registry.go`。`media_task_image_blocking` 有网络请求、轮询和响应合成，不满足纯请求格式转换器的职责和 `From → To RelayFormat` 合同。

不修改：

- Router。
- 数据库 Model 和迁移。
- 通用 `channel.Adaptor` 接口。
- 视频任务表和后台任务轮询。
- 全局 `ImageRequest.MarshalJSON` 的未知字段行为。
- 其他渠道类型。
- `service/relayconvert` 请求转换注册表。

## 9. 回归测试

### 9.1 配置测试

- converter 只能用于 `/v1/images/generations`。
- 未知执行策略拒绝保存。
- `media_task_image_blocking` 不进入通用 `relayconvert` 请求转换注册表。
- 模型级路由匹配正确。
- 未配置 Base URL 时拒绝相对上游路径。

### 9.2 请求测试

- 三个目标模型的最小文生图请求。
- 固定尺寸价 Gemini 直接使用、别名映射到它或由它映射到别名时均返回 HTTP 400。
- `response_format` 省略或为 `url` 时接受，其他值在发送上游前返回 HTTP 400。
- 模型映射发生在发送上游前。
- Gemini 画幅枚举。
- Gemini 参考图数量上限。
- Seedream 4.5 的尺寸和 `n=1–4`。
- Seedream 5.0 的图片数量和 `max_images` 上限。
- `callback_url` 被删除。
- 未允许的 `extra` 字段不透传。

### 9.3 同步响应测试

- HTTP 200 的 OpenAI `data[]` 原样进入现有响应处理。
- 上游错误响应保持稳定错误结构。
- 成功图片数量能够更新结算倍率。

### 9.4 异步任务测试

- 创建返回 202。
- `queued → running → succeeded`。
- 成功结果只有 `primary_url`。
- 成功结果包含多个 `urls`。
- `primary_url` 和 `urls` 不重复。
- `failed` 返回 `error_message`。
- 轮询超时。
- 查询临时网络错误。
- 取得 `task_id` 后不会重新创建任务。

### 9.5 计费测试

- 请求前完成预扣。
- 创建失败退款。
- 任务失败退款。
- 轮询超时退款。
- 成功任务结算。
- 实际多图数量补扣或退还差额。
- 超大 `n`、参考图数量和 `max_images` 在发送上游前返回 HTTP 400。
- 所有额度换算继续使用现有安全换算方法，不引入裸 `int` 转换。

## 10. 上线顺序

### 阶段 A：配置联调

- 使用 Advanced Custom + 请求体透传。
- 只验证 HTTP 200 同步成功路径。
- 每个模型完成至少一次真实调用。
- 暂不作为正式生产渠道。

### 阶段 B：阻塞执行策略

- 实现 `media_task_image_blocking`。
- 关闭请求体直接透传。
- 完成 HTTP 202 轮询。
- 完成参数边界和重复提交保护。
- 完成渠道测试请求配置。

### 阶段 C：范围收口

- 支持三个目标模型，不支持固定尺寸价 `Gemini-3.1-Flash-Image-Preview`。
- 北向仅支持 `response_format=url`，不支持 `b64_json`。
- Seedream 5.0 多图输出继续按 `n` 预扣、按实际 `data[]` 数量结算。
- 不实现尺寸感知价格；Seedream 固定按张价格使用系统 `ModelPrice`。
- 阶段 D 已交付 usage 采集和冻结表达式结算框架；具体 usage 模型须经真实账单核验后再启用 `tiered_expr`，核验前使用固定 `ModelPrice`。

阶段 C 在上述产品边界下已经完成，不再保留独立开发项。

### 阶段 D：共享 Task 持久化闭环

已作为生产闭环的必需阶段实施：

- [x] 复用现有共享 Task 基础设施持久化图片任务。
- [x] 复用后台异步轮询、终态 CAS、计费补偿和审计能力。
- [x] Task 接管后继续处理晚成功、实际图片数量差额和失败/过期退款。
- [x] 增加供应商无关的本站 HTTP 202 与 `GET /v1/images/tasks/:task_id` 查询合同。
- [x] 复用创建幂等日志，在已取得上游任务 ID 后禁止重新 POST，并支持恢复本地 Task。
- [x] 对 `media_image` 应用 30 分钟生命周期保护下限，并在上游结果数超过请求 `n` 时失败关闭、退款且不补扣用户。

该阶段未新建第二套图片异步任务系统。详细设计和验收记录见 [图片异步任务共享 Task 持久化闭环方案](./2026-07-27-图片异步任务共享Task持久化闭环方案.md)，长期决策见 [ADR-0006](../20-architecture/decisions/0006-media-image-shared-task-persistence.md)。

## 11. 路线图建议调整

`docs/50-planning/路线图.md` 中的对应条目建议修改为：

- [ ] 在 Advanced Custom 渠道中增加供应商无关的 `media_task_image_blocking` 执行策略，不新增专用渠道类型，也不注册为通用请求格式 converter。
- [ ] 优先原生转发上游 `POST /v1/images/generations`；HTTP 200 直接返回，HTTP 202 时轮询 `GET /v1/media/tasks/:task_id`。
- [ ] 允许仅实现异步协议的兼容上游把创建路径配置为 `POST /v1/media/generations`。
- [ ] 将成功结果中的 `result.urls` 或 `result.primary_url` 转换为 OpenAI 图片 `data[]`；正确处理失败、坏响应和超时。
- [ ] 一旦获得 `task_id`，失败和超时不得重新提交创建请求。
- [ ] 按模型分别校验尺寸、图片数量、参考图数量、画幅和允许的扩展字段。
- [ ] 请求前预扣；成功后按实际图片数量结算；创建失败、任务失败和超时退款。
- [ ] 增加同步成功、202 轮询成功、任务失败、超时、重复提交保护和计费边界回归测试。

## 12. 验收标准

正式上线前必须同时满足：

1. 每个拟开放生产 ability 的目标模型都完成至少一条真实文生图任务；仅保留适配器能力但未发布的模型不计入当期发布门禁。
2. HTTP 200 同步结果能够返回 OpenAI `data[]`。
3. HTTP 202 能够轮询到成功并返回 OpenAI `data[]`。
4. 上游任务失败能返回稳定错误并退款。
5. 轮询超时不会触发第二次任务创建。
6. 渠道测试使用与目标模型匹配的尺寸和必填字段。
7. 模型映射、渠道模型和本站计费模型保持一致。
8. 用户控制的数量字段都在发送上游前完成上限校验。
9. 多图结果按实际成功数量结算。
10. 管理员日志可以关联请求 ID、渠道 ID、上游请求 ID和 `task_id`。
11. HTTP 200/202 任务信封的轮询和 OpenAI HTTP 200 合成在 `DoRequest` 返回前完成。
12. `media_task_image_blocking` 未进入 `service/relayconvert` 请求转换注册表。
13. 阻塞轮询始终受请求上下文、`RELAY_TIMEOUT` 和 5 分钟硬上限共同约束。
14. `Gemini-3.1-Flash-Image-Preview` 直接请求及双向模型映射均在发送上游前返回 HTTP 400。
15. `response_format` 只接受省略或 `url`，任何其他值均在发送上游前返回 HTTP 400。

## 13. 实施结果

2026-07-27 已完成阶段 B 的代码接入：

- 后端增加 `media_task_image_blocking`，支持同步 OpenAI HTTP 200，以及 HTTP 200/202 媒体任务信封。
- HTTP 200/202 任务信封在 `DoRequest` 内完成创建响应解析、持久化、受控轮询和内部 HTTP 200 合成；取得 `task_id` 后的错误均标记为不可重试。
- 轮询地址只基于渠道 Base URL 和转义后的 `task_id` 构造，不接受上游绝对 `poll_url`。
- 三个目标模型已加入请求边界；未知行为字段采用失败关闭策略。
- 当前公开产品范围只有 Seedream 5 和 Nano Banana 2；Doubao Seedream 4.5 只保留适配器能力，尚无公开别名、生产 ability、价格或真实任务验收。
- 固定尺寸价 `Gemini-3.1-Flash-Image-Preview` 已从渠道模板和模型测试配置移除；直接请求和双向模型映射均显式拒绝。
- `response_format` 已收敛为只允许省略或 `url`，不支持 `b64_json`。
- 模型校验优先使用已知北向原始模型，公开别名映射到已知上游模型时回退使用映射结果，避免自定义上游别名绕过三模型边界。
- `task_id` 改为允许字符白名单，并继续只基于渠道 Base URL 构造轮询地址。
- 创建请求 ID、最后一次轮询请求 ID、`task_id`、轮询次数和等待时间已写入管理员专属消费/错误日志。
- 渠道测试会根据 Advanced Custom 路由声明自动选择图片端点，并为三个目标模型生成兼容尺寸。
- 前端增加供应商无关的策略选项及 TokenSave / Moxing 渠道配置模板，并同步全部语言资源。
- 已增加后端与前端回归测试，覆盖路由合同、三模型转换、固定尺寸价模型拒绝、URL 响应格式边界、模型映射后的规则选择、计费数量边界、OpenAI HTTP 200 直返、HTTP 200 queued 信封、202 轮询成功及追踪字段、任务失败、超时不重提及配置模板。

本次未完成且仍属于上线前操作验收的事项：

- 使用真实 API Key 继续验证 Seedream 目标模型。
- 已确认 Nano 的 `/v1/media/generations` 以 HTTP 200 返回 queued 任务信封；其他模型和错误体仍需继续采样。
- 通过真实账单和本系统消费日志核对成功结算、失败退款及晚成功任务。
- 已用真实 Nano `1K/2K/4K` usage 核对文本输入和图片输出 Token 语义；图片输入、多结果和可信预扣上界仍需核对，完成前不为该模型启用 `tiered_expr`，也不开放 `default` ability。

2026-07-27 已完成阶段 D 的代码接入：

- 上游 HTTP 200/202 任务信封在返回客户端前写入共享 `Task`，请求内等待和后台 `async_task_poll` 复用同一图片轮询服务。
- Task 私有快照冻结创建时的 Base URL、查询路径、代理、鉴权模板、实际 Key、上游任务 ID、请求 ID和计费合同；不保存 prompt、参考图内容、完整响应或 base64。
- Task 持久化后，请求链路不再退款或重复结算；成功按不超过请求 `n` 的实际 URL 数量结算，上游超发按协议违规失败退款，其他失败和生命周期超时进入共享退款/补偿状态机。
- 长任务返回本站 HTTP 202 和本地 `task_xxx`，客户端可查询自己的图片任务；上游任务 ID与凭证不进入公开响应。
- 相同 `Idempotency-Key` 可恢复已有任务；异步图片调用应强烈建议使用并在网络重试时复用该键。本站不按 prompt 隐式去重；同步 HTTP 200 不做长期结果缓存，成功后释放本次幂等占位。
- 表达式计费使用管理员配置的任务 Token 上界保守预扣，并冻结紧凑 `_task` 探针；实际 usage 缺失时保持安全预扣上界，不根据尺寸或图片数量臆测 Token。
- `media_image` 的有效任务生命周期不少于 30 分钟；生产环境仍应按上游 P99/SLA 留出安全余量，推荐保持 `TASK_TIMEOUT_MINUTES=1440` 默认值。
- `response_format=url`、三个目标模型和固定尺寸价模型拒绝边界保持不变。
