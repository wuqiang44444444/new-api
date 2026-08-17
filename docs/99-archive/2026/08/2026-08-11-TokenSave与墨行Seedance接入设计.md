---
status: historical
owner: Dev Team
last-reviewed: 2026-08-11
archived-at: 2026-08-11
source-path: docs/80-dev/2026-08-11-TokenSave与墨行Seedance接入设计.md
superseded-by:
  - docs/20-architecture/Seedance模型接入设计/墨行/渠道对接设计.md
  - docs/20-architecture/Seedance模型接入设计/墨行/素材库对接设计.md
---

# TokenSave 与墨行 Seedance 接入设计

> 本文是已完成实施的历史设计原文；当前架构以 `docs/20-architecture/Seedance模型接入设计/墨行/`
> 下的渠道对接设计与素材库对接设计为准。

## 目标与边界

本设计在既有 Seedance 专用 Channel、ModelArk V3 北向合同、代码注册南向协议和一对一素材作用域内，
接入 TokenSave 海外线路与墨行国内线路。客户模型、Provider 模型和协议身份相互分离；模型后缀只表达
客户可见线路，`model_mapping` 只完成客户模型到 Provider 模型的精确转换，不参与协议推断。

本次直接实现以下五个独立客户模型；每个已启用客户模型只能对应一个 Seedance Channel：

| 客户模型 | Provider 模型 | 视频协议 | 素材协议 |
| --- | --- | --- | --- |
| `doubao-seedance-2-0-260128-tokensave` | `doubao-seedance-2-0-260128` | `tokensave_media_task_v1` | `tokensave_assets_v1` |
| `doubao-seedance-2-0-260128-moxing` | `doubao-seedance-2-0-260128` | `moxing_media_task_v1` | `moxing_joycreator_assets_v1` |
| `doubao-seedance-2-0-fast-260128-moxing` | `doubao-seedance-2-0-fast-260128` | `moxing_modelark_media_v1` | `moxing_volc_assets_v1` |
| `doubao-seedance-2-0-mini-260615-moxing` | `doubao-seedance-2-0-mini-260615` | `moxing_modelark_media_v1` | `moxing_volc_assets_v1` |
| `doubao-seedance-2-5-260628-moxing` | `doubao-seedance-2-5-260628` | `moxing_modelark_media_v1` | `moxing_volc_assets_v1` |

Channel 的 `model_mapping` 必须只包含该行的一条映射。Fast、Mini 和 2.5 虽可使用同一墨行账号，也必须
分别建立 Channel；素材固定到创建它的 Channel，禁止跨模型或跨 Channel 复用。

## 协议身份与管理员名称

管理员只能选择 Provider 命名的代码协议，不再暴露描述 JSON 形状的通用协议身份：

| 配置值 | 中文名称 | 英文名称 |
| --- | --- | --- |
| `tokensave_media_task_v1` | TokenSave 媒体任务 V1 | TokenSave Media Task V1 |
| `tokensave_assets_v1` | TokenSave 素材库 V1 | TokenSave Asset Library V1 |
| `moxing_media_task_v1` | 墨行媒体任务 V1 | Moxing Media Task V1 |
| `moxing_joycreator_assets_v1` | 墨行 JoyCreator 素材库 V1 | Moxing JoyCreator Asset Library V1 |
| `moxing_modelark_media_v1` | 墨行 ModelArk 媒体任务 V1 | Moxing ModelArk Media Task V1 |
| `moxing_volc_assets_v1` | 墨行火山素材库 V1 | Moxing Volcengine Asset Library V1 |

`media_task_v1` 和 `relay_assets_v1` 从可配置协议中删除，不保留 deprecated alias、运行时回退或按
Base URL/模型名自动迁移。已有通用配置会在保存与履约时被判为无效，管理员必须显式重选 Provider 协议。
内部 adapter 可以复用相同的传输和响应归一逻辑，但任务快照必须冻结公开协议身份与 adapter revision。

## 北向合同与逐模型参数

北向仍为 ModelArk V3 typed contract。新增显式 `output_format` 字段；它不是任意参数透传，也不得通过
`extra` 或 metadata 绕过校验。不支持的字段必须在 Provider POST 前返回 400，不能静默删除、钳制或改义。

| 模型线路 | `duration` | `resolution` | 参考媒体 | `generate_audio` | `output_format` |
| --- | --- | --- | --- | --- | --- |
| TokenSave 2.0 | `4..15` 或 `-1` | `480p/720p/1080p` | 图片；首帧、尾帧、参考图 | 依 TokenSave 2.0 合同 | 不支持 |
| 墨行 2.0 | `4..15` 或 `-1` | `480p/720p` | 图片、视频、音频 | 依墨行 2.0 合同 | 不支持 |
| 墨行 Fast | `4..15` 或 `-1` | `480p/720p` | 图片最多 9、视频最多 3、音频最多 3；音频不可单独作为输入 | 支持，省略时按 `true` 计费 | 不支持 |
| 墨行 Mini | `4..15` 或 `-1` | `480p/720p` | 图片最多 9、视频最多 3、音频最多 3；音频不可单独作为输入 | 支持，省略时按 `true` 计费 | 不支持 |
| 墨行 2.5 | `4..30` 或 `-1` | `480p/720p` | 图片最多 30、视频最多 10、音频最多 10；允许纯音频输入 | 支持，省略时按 `true` 计费 | `mp4`、`mov` |

Fast/Mini 的每段参考视频或音频为 2..15 秒，参考媒体总时长不超过 15 秒；2.5 对应范围为 2..30 秒，
总时长不超过 30 秒。平台只校验能够从 typed 请求可靠获得的字段；Provider 媒体探测值仍需在真实联调中
验证，但不得因此放宽已记录合同。

2.0、Fast、Mini 的智能时长 `-1` 按 15 秒预扣，2.5 按 30 秒预扣。Fast、Mini、2.5 在省略
`generate_audio` 时按 Provider 默认 `true` 进入预扣和结算，避免请求语义与计费语义分叉。

证据优先级为：墨行或 TokenSave 当前合同高于通用上游模型能力；BytePlus/火山官方资料只补充精确模型
范围；真实线路验证用于证明实现，不用于自动扩张公开合同。

## 南向视频转换

### TokenSave 2.0

`tokensave_media_task_v1` 使用 `POST /v1/media/generations` 和任务查询路径
`/v1/media/tasks/{task_id}`。adapter 将 ModelArk V3 内容转换为 TokenSave 旧媒体任务字段。当前只接受
文本、首帧、尾帧和参考图片；视频或音频输入在发送前明确拒绝。

### 墨行国内 2.0

`moxing_media_task_v1` 同样使用 `/v1/media/generations` 外观，但由独立 adapter 转换并支持墨行合同中的
`reference_images`、`reference_videos` 和 `reference_audios`，不能复用 TokenSave 的音视频拒绝规则。

### 墨行 Fast、Mini 与 2.5

`moxing_modelark_media_v1` 向 `/v1/media/generations` 发送 ModelArk 风格 typed payload，保留
`content[].image_url/video_url/audio_url`、相机、宽高比、时长、音频、水印及受支持的 `output_format`。
创建与查询响应仍归一为现有 Seedance Task 合同；协议身份和逐模型校验独立于通用第三方 relay。

## 素材库

### TokenSave 素材库

`tokensave_assets_v1` 对接 TokenSave `/assets/*` 资源接口，复用现有 `ast_*`、`astgrp_*` 平台身份和
一对一作用域。只有 Active 的 Provider 素材可被解析为南向 `asset://{ProviderAssetID}`。

### 墨行 JoyCreator 素材库

墨行国内 2.0 使用 `moxing_joycreator_assets_v1`：

- 素材组：`/joycreator/openApi/v1/asset/group/create`、`group/detail/{id}`、`group/{id}`；
- 素材：`/joycreator/openApi/v1/asset/create`、`asset/detail/{id}`、`asset/{id}`；
- 解析墨行 `requestId/error/result` envelope，并将 Provider 状态归一到平台状态；
- 创建素材组、上传素材、等待 Active 后，视频请求把平台引用解析为 `asset://{ProviderAssetID}`。

Provider 未提供或未确认的删除/更新动作不得伪造成功。

### 墨行火山素材库

Fast、Mini、2.5 使用 `moxing_volc_assets_v1` 对接 `/v1/volc/assets/*`。adapter 只承担协议字段和响应
envelope 转换，不暴露 Provider 私域 ID。平台 Asset/AssetGroup 继续固定到
`user_id + app_id + Channel + Provider账号 + BaseURL + protocol`；Fast、Mini、2.5 的 Channel 隔离使
它们即使共享 Provider 账号也不能互引素材。

素材配对必须精确：TokenSave 视频只能配 TokenSave 素材；墨行 2.0 只能配 JoyCreator；墨行
ModelArk 只能配墨行火山素材。无素材协议或其它 Provider 素材协议均不得兼容降级。

## 失败、异步与计费不变量

- Provider POST 前使用冻结的客户模型、Channel、Provider 模型、视频协议、素材协议和计费参数；
- 发送请求字节前建立 durable attempt、资金 hold 与 `sending` 状态；未知创建结果不自动重发或退款；
- 素材 Resolver 在每次发送前复检租户、app、状态和固定作用域；
- 协议不匹配、字段越界、未知字段、素材未 Active 或跨 Channel 引用均失败关闭；
- 智能时长和音频默认值在预扣、结算和审计快照中使用同一归一结果；
- 不持久化 source URL、Provider 原始响应或凭据，不向客户暴露 Provider 素材 ID。

## 配置示例

墨行 Mini Channel 的核心配置应为：

```json
{
  "models": "doubao-seedance-2-0-mini-260615-moxing",
  "model_mapping": {
    "doubao-seedance-2-0-mini-260615-moxing": "doubao-seedance-2-0-mini-260615"
  },
  "settings": {
    "video_upstream_protocol": "moxing_modelark_media_v1",
    "asset_upstream_protocol": "moxing_volc_assets_v1"
  }
}
```

其它四个模型按表格逐一建立独立 Channel，不把多个客户模型合并到同一个 Channel。

## 验证范围

- relaykit 协议枚举、传输 profile 和独立构建；
- 五个模型的映射、参数边界、显式零值、`duration=-1`、默认音频和 `output_format`；
- 三种视频转换与创建/查询响应归一；
- TokenSave、JoyCreator、墨行火山素材 CRUD、Active 状态和精确协议配对；
- 跨用户、跨 app、跨 Channel、跨模型素材引用拒绝；
- durable attempt、单次 Provider POST、unknown、预扣/结算/退款与脱敏回归；
- 管理端中英文及其余支持语言的协议选项、类型检查和配对校验；
- `task docs:check`、`task ai:check`、相关 Go 测试、relaykit 独立构建和前端 Bun 校验。
