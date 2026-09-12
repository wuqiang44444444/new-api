---
status: current
owner: Dev Team
last-reviewed: 2026-09-12
---

# Seedance参考音频传输架构

## 边界与职责

Seedance 的统一 ModelArk V3 入口负责参考音频传输，调用方无需按 Provider 预先准备 OSS。
本能力是请求级音频上传，不是 `/v1/assets` 素材操作，不建立音频资源表、素材组、列表、复用域或 resolver。
只处理 `type=audio_url` 的 `audio_url.url`，保留角色、内容顺序与其它字段语义；音频不会变成图片。
FunCloud 的固定真人模式字段保持启用。HTTP URL 与 opaque 引用继续遵守既有协议，不新增探测或回源。

## 输入与数据流

1. HTTPS URL 原样传给选定 Provider，不下载、不改写签名、不重新上传。
2. 音频 Data URL 与裸 Base64 在入口只解码一次。MIME 使用声明值或文件头识别结果；不能识别为
   audio 不构成拒绝理由，媒体有效性由 Provider 判定。不解析实际时长、裁剪、补齐或转码。
3. 同一创建入口接受 multipart：`request` 表单项承载完整 ModelArk JSON，音频文件表单项由
   JSON 中 `audio_url.url="file://<表单项名称>"` 引用。附件名须唯一且全部被引用；MIME 未声明或为
   `application/octet-stream` 时可识别文件头，识别结果只用于存储头，不作音频准入判断。`file://` 只指本次附件，不访问服务器本地文件系统。
4. 结构及渠道校验之后、资金 hold 之前，把音频字节写入本站私有 OSS。使用现有对象存储配置，固定
   本次存储客户端；对象名为网关生成的随机值，命名空间包含 `user_id + app_id`。上传失败返回
   `reference_audio_unavailable`，不创建资金 hold，也不向 Provider 提交生成任务。
5. 创建 attempt 的恢复快照与成功 Task 只保存音频内容序号、对象 key、存储位置、MIME、字节数。
   不保存 Base64、源 URL 或签名 URL。上传后、资金 hold 前签发 24 小时 HTTPS URL，并按内容序号
   替换请求内的音频地址，再由既有协议 adapter 转换出站字段。相同音频多次出现时，各项各自绑定
   已冻结的对象，不用源字符串作为唯一索引。插件只接收准备好的 URL，不接收上传文件或 Base64 字节。
   Synlink 的南向媒体 URL 校验在音频上传与地址替换之后执行，不能用南向 URL 要求拒绝北向
   Base64 或附件输入；原有 opaque 素材引用限制保持不变。

## 南向音频字段

九种已登记视频协议共用上述音频准备流程，再分别构建 Provider 请求：

| 协议 | 南向音频字段 |
| --- | --- |
| FunCloud、Synlink、Volcengine、BytePlus、Ark Media、Moxing、CMCC | `content[].audio_url.url`，保留 `type=audio_url` 与 `role=reference_audio` |
| TokenSave | `reference_audios[]` |
| Feicai | `audios[]` |

## 不变量与错误

- 上传文件与 Data URL 的字节保持一致，不根据文件名把音频解释为图片。
- 文件字节直接交给上传服务，不经过 Base64 编解码往返；已解码的 Base64 也不重新编码。
- 网关只检查传输结构、Base64 和解码后单文件 15 MiB 上限（含上限），不套用 URL 字符串长度上限；全请求仍受既有请求体/证据采集上限约束。
  实际音频时长、媒体有效性与 Provider 限制由上游判定；生成视频时长的计费安全校验保持不变。
- URL 签发失败在资金 hold 前停止，不提交生成请求。结果 unknown 不重发、不退款，
  人工核实恢复沿用已冻结对象事实。对象不自动去重或清理，现有存储基础设施负责生命周期。
- Provider 错误码与消息沿既有脱敏路径保留，不把 `NO_ASSET_ID` 自动改述为本站素材创建失败或音频
  时长错误。素材创建和视频任务失败是独立结果。
- 请求证据沿用私有加密存储与鉴权审计合同；multipart 的 `request` JSON 即使未声明 Content-Type
  也必须递归脱敏凭据。普通日志、Task/attempt 不保存原始媒体与签名 URL。

## 实现与验证边界

入口解析位于 Link 专属 middleware，上传与签名位于请求级 service，南向转换位于 Seedance adapter。
`model/task.go` 只增加一个可选私有快照字段，类型在独立文件中；这是复用 Task/attempt 持久事实所必需的
最小原生接线点，上游合并只需保留此字段，不改变原生路由、分发或计费行为。

自动化验证覆盖九种协议的 HTTP/HTTPS、Data URL、裸 Base64 与文件音频出站字段；FunCloud 与 Synlink
额外覆盖真实入口解析到模拟 Provider 的创建链路，包括 URL 不回源、Base64/文件精确上传、图音字段
不混淆、上传或签名失败不预扣、快照与 unknown 恢复。
本地 mock 验证不能替代真实 Provider、生产 OSS、外部数据库和供应商账单验收。
