---
status: current
owner: Dev Team
last-reviewed: 2026-08-06
---

# 08 Seedance 模型能力接口操作手册

## 1. 目的与适用范围

本文说明第三方调用方和运维人员如何发现 Seedance 模型、读取模型参数能力，并在提交视频任务前判断
当前 API Key 是否可以使用该模型。

本文只描述客户可见的 ModelArk v3 合同。Provider 模型名、implementation、渠道、凭据、上游地址、
内部价格和路由细节不属于该接口，也不得从 capability 响应中推断。

## 2. 三个接口的职责

| 目的 | 方法与路径 | 返回范围 |
| --- | --- | --- |
| 查询当前 Key 可见的模型身份 | `GET /v1/models` | NEWAPI 原生模型列表，只包含当前 Key 实际可见的模型 |
| 查询 Seedance 参数能力 | `GET /api/v3/contents/generations/models` | 全部代码登记候选，以及当前 Key 已发布的客户模型 alias |
| 创建 Seedance 视频任务 | `POST /api/v3/contents/generations/tasks` | 按所选模型的 capability 校验并创建任务 |

`/v1/models` 是模型发现接口，不承担参数说明；capability 接口是参数合同查询接口，不会因为登记了模型
就把它发布为可用模型。客户端必须同时检查两类结果。

## 3. 鉴权与基础调用

所有调用都使用同一 API Key：

```bash
export API_BASE="https://api.example.com"
export API_KEY="replace-with-your-api-key"
```

不要把真实 Key 写入文档、脚本仓库、工单、截图或聊天记录。

### 3.1 查询全部 Seedance capability

```bash
curl -sS "$API_BASE/api/v3/contents/generations/models" \
  -H "Authorization: Bearer $API_KEY"
```

接口固定返回 15 个代码登记的 Seedance ModelArk 基础候选，并追加当前 Key 已发布且映射到这些候选的
客户模型 alias。每一行都针对当前 Key 分别计算发布状态、`/v1/models` 可见性和实际可用性。结果包含
`available=false` 的模型，便于第三方提前完成表单和参数适配。

### 3.2 精确查询一个模型

```bash
curl -sS \
  "$API_BASE/api/v3/contents/generations/models?model=seedance-2.0-fast" \
  -H "Authorization: Bearer $API_KEY"
```

`model` 只接受精确客户模型 ID 或基础候选 ID，不做前缀或模糊匹配。publication 已发布客户 alias 时，
可直接使用 `/v1/models` 返回的 alias 查询；响应的外层 `id` 和 `capability.public_model` 都保持该客户
模型，不暴露其绑定的 Link SKU。查询成功仍返回列表结构，匹配项位于 `data[0]`；未知模型返回
`404 model_not_found`。

### 3.3 快速查看可用状态

```bash
curl -sS "$API_BASE/api/v3/contents/generations/models" \
  -H "Authorization: Bearer $API_KEY" \
  | jq '.data[] | {
      id,
      published,
      visible_in_v1_models,
      available,
      version: .capability.version
    }'
```

## 4. 响应结构

```json
{
  "object": "list",
  "data": [
    {
      "id": "seedance-2.0-fast",
      "object": "model.capability",
      "available": true,
      "published": true,
      "visible_in_v1_models": true,
      "capability": {
        "public_model": "seedance-2.0-fast",
        "contract_id": "modelark.contents.generations.v3",
        "version": "public-video-contract-v2",
        "content_hash": "sha256-content-hash",
        "request_fields": ["model", "content", "duration", "resolution", "ratio"],
        "required_fields": ["model", "content"],
        "unsupported_fields": ["callback_url"],
        "resolutions": ["480p", "720p"],
        "default_resolution": "720p",
        "ratios": ["16:9", "4:3", "1:1", "3:4", "9:16", "21:9", "adaptive"],
        "default_ratio": "16:9",
        "requires_resolution": false,
        "requires_ratio": false,
        "duration": {
          "minimum": 4,
          "maximum": 15,
          "default": 5,
          "allows_automatic": false,
          "required": false
        },
        "media": {
          "content_types": ["text", "image_url", "video_url", "audio_url"],
          "min_images": 0,
          "max_images": 3,
          "max_videos": 1,
          "max_audio": 1
        },
        "constraints": {
          "requires_text": true,
          "supports_generate_audio": true,
          "supports_direct_media": true,
          "supports_link_assets": true,
          "supports_mixed_media_paths": false,
          "reference_modes_exclusive": false,
          "audio_requires_reference_image": false,
          "audio_requires_visual_reference": true
        },
        "lifecycle": {
          "supports_content": true,
          "supports_last_frame": false,
          "supports_cancel_queued": false,
          "supports_delete": false
        }
      }
    }
  ]
}
```

示例只展示字段形状。客户端必须读取运行时返回值，不得把示例中的布尔值、数组或哈希固化为当前
生产状态。

### 4.1 模型状态字段

| 字段 | 含义 | 创建任务 |
| --- | --- | --- |
| `published` | 存在该精确客户模型的 `LinkModelPublication` | `false` 时禁止创建 |
| `visible_in_v1_models` | 当前 Key 调用 `/v1/models` 时可以看到该模型 | `false` 时禁止创建 |
| `available` | 当前 Key 存在已发布、可路由且无冲突的履约候选 | `false` 时禁止创建 |

只有三个字段全部为 `true` 时，客户端才可以尝试创建任务。即使全部为 `true`，请求仍可能因参数、
额度、限流、授权或 Provider 结果失败。

常见状态解释：

| 状态 | 解释 |
| --- | --- |
| `published=false` | 模型只有代码 capability，没有精确发布记录 |
| `published=true`、`visible_in_v1_models=false` | 当前 Key 的分组、模型权限或计费可见性不满足 |
| `published=true`、`visible_in_v1_models=true`、`available=false` | 当前没有合格渠道，或路由、实现、配置存在冲突 |
| 三者均为 `true` | 允许按 capability 构造并尝试提交请求 |

### 4.2 capability 字段

| 字段 | 类型 | 使用规则 |
| --- | --- | --- |
| `public_model` | string | 客户请求中的 `model`，与外层 `id` 相同；客户 alias 不会被替换成 Link SKU |
| `contract_id` | string | 当前为 `modelark.contents.generations.v3` |
| `version` | string | capability 的语义版本；变化时重新生成表单和校验规则 |
| `content_hash` | string | capability 内容哈希；用于缓存失效和变更检测，不是授权凭据 |
| `request_fields` | string[] | 该模型允许提交的顶层字段 |
| `required_fields` | string[] | 无条件必填的顶层字段 |
| `unsupported_fields` | string[] | 明确不支持；提交后会被拒绝，不会静默删除或降级 |
| `resolutions` | string[] | 可选分辨率；固定分辨率模型也只返回一个值 |
| `default_resolution` | string? | 未传 `resolution` 时使用的默认值 |
| `ratios` | string[] | 支持的画幅；空数组不是“任意画幅” |
| `default_ratio` | string? | 未传 `ratio` 时使用的默认值 |
| `resolution_ratio_combinations` | object[]? | 存在时只允许列出的分辨率与画幅组合，不能按两个数组做笛卡尔积 |
| `requires_resolution` | boolean | 为 `true` 时必须显式提交 `resolution` |
| `requires_ratio` | boolean | 为 `true` 时必须显式提交 `ratio` |
| `duration` | object | 时长范围、离散值、默认值、自动时长和必填规则 |
| `media` | object | `content` 支持的类型、图片下限以及图片/视频/音频数量上限和角色 |
| `constraints` | object | 文本、生成音频、媒体路径、引用模式和音频依赖约束 |
| `lifecycle` | object | 内容下载、尾帧、排队取消和删除能力 |

`duration.values` 存在时只选择列出的离散值。`duration.allows_automatic=true` 表示可提交 `-1`；它不表示
任意负数都合法。没有 `default` 且 `required=true` 时必须显式传值。

`media.content_types` 描述 `content` 条目类型。数量为 `0` 表示不支持该类输入。角色数组存在时，媒体
条目的角色必须从数组中选择；`reference_modes_exclusive=true` 表示参考图模式与首帧/尾帧等模式不可
混用。

`supports_direct_media` 只表示模型合同允许受支持的直接媒体 URL/Data URL；`supports_link_assets`
表示允许平台 `asset://ast_*` 资源。`supports_mixed_media_paths=false` 时，同一请求不要混用直接媒体与
平台素材路径。真人素材仍须遵守独立授权规则，不能仅凭这两个字段获得真人能力。

## 5. 当前登记的 15 个 Seedance 基础模型

下表是 2026-08-05 的结构性能力摘要，不代表当前 Key 的实时可用性。发布与可用状态始终以接口返回的
三个状态字段为准。客户模型 alias 复用其 publication 绑定 SKU 的参数能力，可能使列表行数大于 15；
alias 不是新的 capability 注册，也不能反向改变原 SKU。

| 公开模型 ID | capability 版本 | 分辨率 | 时长 | 画幅 | 媒体上限（图/视频/音频） | 关键约束 |
| --- | --- | --- | --- | --- | --- | --- |
| `seedance-byteplus` | `public-video-contract-v2` | 480p/720p/1080p/4k，必传 | 4–15，默认 5，可 `-1` | 7 种规范画幅，必传 | 9/3/3 | 可生成音频；文本非必需 |
| `seedance-2-0-oversea` | `moxing-media-task-v2` | 480p/720p，必传 | 4–15 或 `-1`，必传 | 7 种规范画幅，必传 | 9/0/0 | 文本必需；图片引用模式互斥 |
| `doubao-seedance-2-0-260128` | `tokensave-media-task-v2` | 480p/720p/1080p，必传 | 4–15 或 `-1`，必传 | 7 种规范画幅，必传 | 9/0/0 | 文本必需；图片引用模式互斥 |
| `seedance-2.0-standard` | `public-video-contract-v2` | 480p/720p/1080p，默认 720p | 4–15，默认 5 | 7 种规范画幅，默认 16:9 | 3/1/1 | 文本必需；可生成音频 |
| `seedance-2.0-fast` | `public-video-contract-v2` | 480p/720p，默认 720p | 4–15，默认 5 | 7 种规范画幅，默认 16:9 | 3/1/1 | 文本必需；可生成音频 |
| `seedance-2.0-mini-720p` | `feicai-media-arrays-v2` | 固定 720p，必传 | 4–15，必传 | 仅 16:9，必传 | 9/0/3 | 文本必需；引用模式互斥 |
| `seedance-2.0-sd2-720p` | `feicai-media-arrays-v2` | 固定 720p，必传 | 11–15，必传 | 当前无已验证画幅 | 9/0/0，至少 1 图 | 不可发布执行，不能把空画幅解释为任意值 |
| `seedance-2.0-fast-720p` | `feicai-media-arrays-v2-r2` | 固定 720p，必传 | 4–15，必传 | 仅 16:9，必传 | 9/0/3 | 文本必需；引用模式互斥 |
| `seedance-2.0-value-720p` | `feicai-media-arrays-v2-r3` | 固定 720p，必传 | 4–15，必传 | 仅 16:9，必传 | 9/0/3 | 文本必需；引用模式互斥 |
| `seedance-2.0-standard-720p` | `feicai-media-arrays-v2` | 固定 720p，必传 | 4–15，必传 | 仅 16:9，必传 | 9/0/3 | 文本必需；引用模式互斥 |
| `seedance-2.0-value-1080p` | `feicai-media-arrays-v2` | 固定 1080p，必传 | 4–15，必传 | 当前无已验证画幅 | 9/0/3 | 当前无完整可执行参数组合 |
| `seedance-2.0-standard-1080p` | `feicai-media-arrays-v2` | 固定 1080p，必传 | 4–15，必传 | 仅 16:9，必传 | 9/0/3 | 文本必需；引用模式互斥 |
| `seedance-2.0-value-4k` | `feicai-media-arrays-v2` | 固定 4k，必传 | 4–15，必传 | 当前无已验证画幅 | 9/0/3 | 当前无完整可执行参数组合 |
| `seedance-2.0-standard-4k` | `feicai-media-arrays-v2-r2` | 固定 4k，必传 | 4–15，必传 | 仅 16:9，必传 | 9/0/3 | 文本必需；引用模式互斥 |
| `seedance-2.0-pro-pi-720p` | `feicai-media-arrays-v2` | 固定 720p，必传 | 固定 15，默认 15 | 当前无已验证画幅 | 9/3/3 | 当前无完整可执行参数组合 |

7 种规范画幅为 `16:9`、`4:3`、`1:1`、`3:4`、`9:16`、`21:9` 和 `adaptive`。

## 6. 第三方接入流程

1. 使用业务 API Key 调用 `GET /v1/models`，取得当前 Key 可见的客户模型集合，包括可能存在的 alias。
2. 使用同一个模型 ID 调用 capability 接口；如果只接一个模型，使用精确 `model` 查询。
3. 确认 `published`、`visible_in_v1_models`、`available` 全部为 `true`。
4. 只提交 `request_fields` 中的字段，并补齐 `required_fields` 和各嵌套约束要求的字段。
5. 选择 `resolutions`、`ratios` 和 `duration` 中允许的值；存在组合清单时按组合清单选择。
6. 按 `media` 和 `constraints` 生成上传控件及本地校验，不提交超限或互斥媒体。
7. 使用唯一的 `Idempotency-Key` 提交创建请求；同一业务重试复用原 Key，新业务使用新 Key。
8. 保存服务端返回的任务 ID 和 `request_id`，通过任务查询接口跟踪终态。

客户端可以用 `public_model + version + content_hash` 缓存参数表单。任一值变化，或响应头要求重新验证
时，都应刷新 capability。服务端响应带 `Cache-Control: private, no-cache`，共享代理不得跨 API Key
缓存，因为状态字段与当前 Key 有关。

## 7. 创建请求示例

先确认 `seedance-2.0-fast` 的三个状态字段都为 `true`，再提交：

```bash
curl -sS "$API_BASE/api/v3/contents/generations/tasks" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: replace-with-a-unique-business-key" \
  -d '{
    "model": "seedance-2.0-fast",
    "content": [
      {
        "type": "text",
        "text": "镜头缓慢掠过清晨的山谷"
      }
    ],
    "duration": 5,
    "resolution": "720p",
    "ratio": "16:9",
    "generate_audio": false
  }'
```

不要向请求中加入 capability 未列入 `request_fields` 的 Provider 私有参数。服务端对不支持字段会明确
拒绝，不会静默删除、钳制、换模型或降级到另一套语义。

## 8. capability 接口错误

错误响应使用统一结构：

```json
{
  "error": {
    "code": "model_not_found",
    "message": "model capability not found (request id: ...)",
    "request_id": "..."
  }
}
```

| HTTP 状态 | 常见错误 | 处理方式 |
| --- | --- | --- |
| 401 | 认证失败 | 检查 Bearer Key 是否有效，不要自动改用匿名请求 |
| 404 | `model_not_found` | 检查精确模型 ID；不要自动尝试别名或 Provider 模型名 |
| 500 | `internal_error` | 保存 `request_id` 并联系平台；不要根据旧缓存强行创建 |

## 9. 运维排障

### 9.1 capability 有模型，但 `/v1/models` 没有

这是允许的候选状态，不是列表同步故障。依次检查：

1. 是否存在该精确客户模型的 publication；
2. 当前 Key 的模型限制和用户分组是否允许该模型；
3. 客户模型是否有有效计费配置；
4. Ability、渠道模型名、模型映射与客户模型是否一致；
5. 候选 implementation、profile、创建路径和执行绑定是否完整且无冲突。

修复后重新调用两个模型接口确认。不得通过修改 `/v1/models` 响应或删除 capability 候选掩盖配置问题。

反向情况也不应出现：`/v1/models` 返回的已发布 Seedance 客户 alias 必须能用相同 ID 查询 capability。
若查询返回 404，应检查 publication 的 `CustomerModel -> LinkSKU` 投影和运行版本，而不是要求客户端改用
内部 SKU。

### 9.2 `published=true` 但 `available=false`

这表示发布事实存在，但当前没有可安全履约的精确候选。先停流并核对 Ability、渠道状态、实现绑定、
账号作用域和路由冲突。不得自动换到另一 SKU、另一 Provider 模型或普通 NEWAPI 语义。

### 9.3 capability 版本或哈希变化

把变化视为客户参数合同更新：清除客户端能力缓存，重新生成校验规则，执行创建、查询、内容下载、
失败和计费回归。完成真实 Provider 与账单验收前，不得仅因为 capability 已登记就开放生产流量。

## 10. 安全与发布检查

- [ ] capability 接口要求 Bearer 鉴权，未提供匿名模型能力查询。
- [ ] 响应没有 Provider 模型、implementation、渠道 ID、Key、Base URL、签名 URL或内部价格。
- [ ] 第三方只使用外层 `id`/`capability.public_model` 作为请求模型。
- [ ] 客户端同时检查 `published`、`visible_in_v1_models` 和 `available`。
- [ ] 空 `ratios`、空组合或数量上限 `0` 被解释为不支持，而不是无限制。
- [ ] capability 变化后完成参数、生命周期、幂等、退款与计费验收。
- [ ] 日志和工单只记录公开模型 ID、HTTP 状态、错误码和 `request_id`。

## 11. 关联文档

- [Seedance 渠道配置清单](07-Seedance渠道配置清单.md)
- [视频与素材渠道运维手册](02-视频与素材渠道运维手册.md)
- [视频模型 API 用户调用指南](../30-engineering/视频模型API用户调用指南.md)
- [Link 视频服务合同与异步任务架构](../20-architecture/Link视频服务合同与异步任务架构.md)
- [公开 API 文档交付架构](../20-architecture/公开API文档交付架构.md)
