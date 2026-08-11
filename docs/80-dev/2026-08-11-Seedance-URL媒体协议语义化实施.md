---
status: completed
owner: Dev Team
last-reviewed: 2026-08-11
---

# Seedance URL 媒体协议语义化实施

## 问题与目标

现有 Seedance 第三方视频协议使用 `media_arrays_v2` 作为管理员配置值，并在页面显示为
“Media Arrays V2 / 飞彩媒体数组 V2”。该名称只描述了南向 JSON 形状，没有明确告诉管理员此协议只接受
请求级 URL/Data URL，也不支持平台 `ast_*`、公共 `pubref_*` 或 Provider 素材库，容易产生错误配置预期。

本次目标是：

1. 将管理员可见协议标识直接改为 `url_media_arrays_v1`；
2. 页面明确显示“仅 URL，不支持素材库”的中英文及其他已支持语言文案；
3. 保持 ModelArk V3 北向合同统一，由南向 adapter 转换为 `images[]`、`audios[]`、`videos[]`；
4. 保持飞彩渠道 `asset_upstream_protocol=none`，不为其模拟 `ast_*` 或 `pubref_*`；
5. 保持现有飞彩结果内容代理：平台使用冻结的 Provider Bearer 凭据回源，不暴露需飞彩认证的原始结果地址。
6. 删除飞彩 v1/v2/v3 配置和验证分支，只维护当前无后缀 Provider 模型与当前协议。

## 当前实际情况

### 已验证事实

- 飞彩已验证创建接口是 `POST /v1/videos`，查询接口是 `GET /v1/videos/{task_id}`。
- 图片输入使用 `images[]`，支持 HTTP、HTTPS 和图片 Data URL；音频与视频使用公网 URL 数组。
- 当前证据没有公共预置素材 ID、私域素材 CRUD、素材组或真人认证接口。
- `media_arrays_v2` 当前只能与 `asset_upstream_protocol=none` 配对。
- URL 媒体 adapter 会拒绝未被解析的 `asset://` 引用；Resolver 也不会为该协议解析 `pubref_*`。
- 平台已经通过 `/v1/videos/{task_id}/content` 使用任务冻结的 Bearer 凭据代理飞彩结果，并校验结果 URL
  与冻结 Base URL 同源。

### 需要变更的事实

- `video_upstream_protocol` 的代码枚举、前后端类型、表单选项和协议配对仍使用 `media_arrays_v2`。
- 页面文案没有明确表达“不支持素材库”。
- 当前架构、工程和运维事实文档仍使用旧协议标识。
- 飞彩 size evidence、验证命令和验收文档仍混有 v2/v3 命名与旧 `-feicai` Provider 模型。
- 已保存 Channel 的 `settings.video_upstream_protocol` 可能仍为旧值，需要一次性、幂等地迁移；历史 Task
  冻结的 transport profile 与 adapter revision 不应改写。

## 优化方案

### 1. 稳定协议身份

```text
video_upstream_protocol = url_media_arrays_v1
asset_upstream_protocol = none
```

代码仅注册新标识，不保留旧标识 alias 或双读分支。数据库启动迁移只修改 Seedance Channel 当前设置中的
精确旧值；历史 Task、attempt 和审计快照继续保存其创建时事实。

飞彩验证命令、Provider 模型常量和 size evidence 同样只保留当前版本；不建立
`feicai_assets_v1/v2/v3`、`media_arrays_v2/v3` 或其它飞彩品牌化版本族。

### 2. 页面展示

```text
中文：URL 媒体数组（仅 URL，不支持素材库）
英文：URL Media Arrays (URL Only, No Asset Library)
```

协议标识存入 Channel；展示文案由 i18n 代码映射提供，不把中文或英文名称持久化到数据库。

### 3. 南向履约

URL/Data URL 继续由现有 typed adapter 转换。不同飞彩模型的图片必填、音视频支持范围、真人内容和
画幅差异由 Provider 判断并返回错误；平台不建设逐模型 capability 表。

`ast_*` 和 `pubref_*` 在 Provider POST 前返回不可解析错误，因为 URL-only 协议没有可序列化的 Provider
素材身份。这是协议语义检查，不是资源真实性或安全审核。

### 4. 结果回源

沿用现有内容代理，不修改公共响应合同：任务成功后客户获得平台内容 URL；平台使用冻结连接、同源结果
URL 和 Provider Bearer 凭据流式回源。原始飞彩结果 URL 和凭据不返回客户。

### 5. 验证

- relaykit 独立构建及协议枚举/固定路径测试；
- Channel 旧协议值迁移测试；
- URL-only 与 `none` 的前端配对和双语可见文案测试；
- 现有素材拒绝、任务轮询脱敏和带凭据内容代理测试；
- `task docs:check`、`task ai:check`，以及前端类型、lint、i18n 和相关测试。

## 风险与剩余事项

- 飞彩文档顶部提到 multipart 文件上传，但缺少请求字段、响应和生命周期证据；本次不把它解释成素材库。
- 真实 Provider、账单和生产灰度不在本地实施验证范围内，不能因代码和单元测试通过而标记为已发布。
- 如果飞彩以后提供稳定素材 ID、公共素材或真人认证合同，应新增独立
  `asset_upstream_protocol`，而不是扩张 `url_media_arrays_v1` 的语义。

## 实施结果

- 管理员协议标识已改为 `url_media_arrays_v1`，旧 `media_arrays_v2` 不再是有效配置值；启动迁移会
  幂等更新现有 Seedance Channel，历史 Task/attempt 快照不改写。
- 页面已通过七种语言展示 URL-only/无素材库语义；中文为“URL 媒体数组（仅 URL，不支持素材库）”，
  英文为“URL Media Arrays (URL Only, No Asset Library)”。
- `url_media_arrays_v1` 只允许配对 `asset_upstream_protocol=none`；`ast_*` 和 `pubref_*` 在 Provider
  POST 前明确返回不可解析错误。
- 飞彩 Provider 模型常量、size evidence 和真实验证命令已归一到当前无后缀模型；验证命令路径改为
  `cmd/verify-feicai`，不再选择 v2/v3。
- 已删除未使用的 `size_feicai_v2.go`、旧飞彩模型版本常量和旧验证版本分支。
- 飞彩结果 URL 继续使用既有冻结 Bearer 凭据和同源检查代理，未新增重复实现。

验证通过：受影响 Go 包测试、完整 `model` 测试、relaykit 独立构建、前端类型检查、相关 Vitest、
受影响文件 oxlint/oxfmt、七语言 i18n 同步、公开文档校验、`task docs:check` 与 `task ai:check`。
全量前端格式检查另报告三个未触碰文件的既有格式问题，本次未扩大范围修改。
