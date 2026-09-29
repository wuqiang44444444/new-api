---
status: current
owner: Dev Team
last-reviewed: 2026-09-28
---

# Vidu Drama 两站与现有 Seedance 渠道适配结论

## 1. 更正后的结论

**2026-09-28 真实测试更正：当前官方 ModelArk V3 协议能够创建国内、海外 Drama 任务，但不能直接完成查询解析与结算，尚不支持仅靠配置上线。**

两站真实 GET 响应中的 `id` 与创建返回的 ID 完全一致，但 `seed`、`duration`、`created_at`、
`updated_at`、`frames_per_second` 返回 JSON 字符串，和文档示例、当前 Go 数字类型不同。
`decodeOfficialPluginObservation` 把结构解码失败也归为 `ModelArk task id mismatch`，因此这个报错
不代表本次真的 ID 不一致。两条测试 Task 进入 `RECONCILIATION_REQUIRED`，预扣尚未完成结算。

现有 Seedance 渠道类型、鉴权和路径可以保留；南向需要明确登记 Vidu 的响应转换合同并回归，
不能通过修改冻结 Task、跳过身份校验或放宽所有官方协议的计费用量类型来绕过。
此前“无需新增 Vidu 专属协议、可直接复用”的完整兼容判断撤回；本地合成响应测试只证明文档形状。
已创建国内渠道 139、海外渠道 140，各四个独立客户模型，价格按用户要求复制现有官方模型；
发现不兼容后两渠道暂时停用。详细结果见[真实测试记录](../99-archive/2026/09/2026-09-28-Vidu双站渠道接入与实测.md)。

原先依据普通 Vidu 官网 `/ent/v2` + `Token` 的结论撤回。原生 Vidu 渠道类型 52 的插件仍按普通
Vidu 合同工作，不能作为这份 Drama V3 接口的替代入口，也不需要为这次研究改造原生路由。

## 2. 正确来源

| 站点 | 官方来源 | 原始调用信息 |
| --- | --- | --- |
| 国内 | [Drama 国内飞书文档](https://shengshu.feishu.cn/wiki/PkFHwrme8ihMA8kshbgc5QwPnxe) | [国内字段、地址与价格](Vidu国内站视频接口原始信息.md) |
| 国外 | [Drama 海外飞书文档](https://shengshu.feishu.cn/wiki/UdZNw56HxiLfEykYOjmcsaYQn08) | [国外字段、地址与价格](Vidu国外站视频接口原始信息.md) |

读取日期 2026-09-28。两份文档的基础信息、模型、内容输入、素材规格、参数、响应、调用示例、
错误码与价格均已查看；国内额外含列表/删除。已重新生成原来的两份原始信息文件，保留正确来源和
完整接口地址，不再混合普通 Vidu 七类 API 的字段。旧国内初次调研只保留来源更正说明。

## 3. 本次试验渠道配置（当前停用，待完成响应适配）

| 配置项 | 国内 | 国外 |
| --- | --- | --- |
| 类型 | `ChannelTypeSeedanceLink` | `ChannelTypeSeedanceLink` |
| video_upstream_protocol | `modelark_v3_volcengine` | `modelark_v3_byteplus` |
| Base URL | `https://api.vidu.cn/ent` | `https://api.vidu.com/ent` |
| asset_upstream_protocol | `none` | `none` |
| Key | 国内 Key，安全填写 | 国外 Key，安全填写 |
| 客户模型 | 独立的国内客户别名 | 独立的国外客户别名 |

必须保留 Base URL 的 `/ent`，不用把完整 `/api/v3/contents/generations/tasks` 填进 Base URL。
代码 `joinVideoUpstreamURL` 的行为是删除 Base URL 末尾斜杠后拼接路径，会保留 `/ent`：

```text
https://api.vidu.cn/ent
  + /api/v3/contents/generations/tasks
  = https://api.vidu.cn/ent/api/v3/contents/generations/tasks

https://api.vidu.com/ent
  + /api/v3/contents/generations/tasks/{task_id}
  = https://api.vidu.com/ent/api/v3/contents/generations/tasks/{task_id}
```

代码证据：[URL 拼接](../../relay/channel/task/seedance/video_upstream.go)、
[创建与查询接线](../../relay/channel/task/seedance/adaptor.go)。

`model_mapping` 的右侧必须是本站精确 Provider 名称，不能改成 doubao/dreamina 官方模型 ID，
也不能把另一站的名字当作兼容回退。左侧为本次实际创建的客户模型名：

| Seedance 业务名称 | 国内客户名 → Provider 模型 | 国外客户名 → Provider 模型 |
| --- | --- | --- |
| 2.5 | `seedance-2-5-vidu-cn` → `viduq3.1-drama-std` | `seedance-2-5-vidu-global` → `viduq3.1-drama-ab-std` |
| 2.0 | `seedance-2-0-vidu-cn` → `viduq3-drama-std` | `seedance-2-0-vidu-global` → `viduq3-drama-ab-std` |
| 2.0 fast | `seedance-2-0-fast-vidu-cn` → `viduq3-drama-fast` | `seedance-2-0-fast-vidu-global` → `viduq3-drama-ab-fast` |
| 2.0 mini | `seedance-2-0-mini-vidu-cn` → `viduq3-drama-mini` | `seedance-2-0-mini-vidu-global` → `viduq3-drama-ab-mini` |

Seedance 与版本的业务对应来自用户截图，Provider 精确别名现已得到飞书模型表确认。
客户名不得与其他已启用 Seedance 渠道冲突；两站连接、Key、模型、价格和任务冻结事实分别维护。

## 4. 现有代码支持的证据

| 合同点 | 飞书文档 | 当前实现 | 判断 |
| --- | --- | --- | --- |
| 鉴权 | Bearer | 宿主设置 `Authorization: Bearer` | 相符 |
| 创建路径 | `/ent/api/v3/contents/generations/tasks` | `/ent` Base URL + 官方固定后缀 | 相符 |
| 请求体 | model、content、ratio、duration、generate_audio 等 | 官方 buildCreate 保持请求，只替换 model | 基础结构相符 |
| 创建响应 | 根级 `{id}` | parseOfficialVideoCreateResponse 读取 id | 相符 |
| 单任务查询 | 同一 V3 路径后加 ID | 使用冻结查询路径与凭据 | 相符 |
| 状态 | queued/running/succeeded/failed/expired | 任务解析识别这些状态 | 状态枚举相符，失败完整载荷仍需取证 |
| 结果 | content.video_url、last_frame_url | 官方响应保持形状并进入宿主解析 | 真实数字字段字符串类型使整个响应解码失败 |
| 用量 | 文档为整数 completion_tokens、total_tokens | 宿主按官方整数 Token 语义归一 | 真实为字符串 "108900"，当前不能作为可信用量结算 |
| 配置模型 | 八个 viduq3/viduq3.1 别名 | 两个官方协议 `modelPolicy: configured` | 不因未列入 models 数组而拒绝配置 |

代码入口：

- [seedance-link 插件](../../plugins/seedance-link/plugin.js)：官方协议声明、默认元数据、buildOfficialVideoCreate、创建和查询解析。
- [配置模型校验](../../pkg/jsplugin/seedance_configuration.go)：configured 模型策略及精确/默认元数据选择。
- [Go 官方请求及用量边界](../../relay/channel/task/seedance/plugin_official_video.go)：映射、预扣输入一致性、冻结路径和用量校验。
- [宿主状态解析](../../relay/channel/task/seedance/adaptor.go)：成功、失败、过期等状态和结果读取。

不推荐 `funcloud_modelark_v3`：它有 FunCloud 私有转换与固定真人模式，不应因为路径同为 V3 就套用。
`ark_media_v1`、TokenSave、Synlink 等也没有比官方透传更匹配这份合同。

## 5. 尚未达到“全功能直接上线”的具体差距

### 5.1 别名精确参数声明缺失

八个别名目前均落入 `OFFICIAL_DEFAULT_METADATA`，而非各自精确声明。该默认允许时长 1–60 秒、
480p/720p/1080p/4K 和完整 ModelArk 字段；这与 Vidu 逐模型的 4–15/30 秒、fast/mini 仅 480p/720p
不完全一致，也不能表达其全部参考媒体上限。

完整接入的最小方向是在已有 Seedance 插件声明中登记八个精确别名的元数据与经确认的操作范围，
保留既有官方传输函数；不复制 Router、Task、计费实现，不让管理员编写 JSON 协议转换。
这属于待实施建议，本次没有修改声明。

### 5.2 需要确认的参数与文档冲突

- 两站 `generate_audio` 顶层表默认 false、说明章节默认 true；试调用显式填写，不能依靠未知默认。
- 国外第 6.4 节另写“仅 2.0 系列支持”音频，Q3.1 音频边界需确认。
- Provider 文档字面值为 `4k`，当前官方元数据为 `4K`，且官方转换不改字段值；接受大小写的情况未经验证。
  不能仅通过后台参数覆盖掩盖该语义差异。
- 两站时长专表末行重复 fast、遗漏 mini；国外分辨率表漏写 `-ab-`；这些冲突原样记录在来源文件。
- 两站查询示例混入 `/ent/v2/tasks/.../creations`，但基础信息和响应章节写 V3；国外创建示例还沿用 `.cn`。
  以基础信息的站点与 V3 路径作为候选核验，不自动试探另一站或另一协议。
- 通用内容示例混用关键帧与多模态参考角色，和场景互斥说明矛盾；验收按互斥场景分别验证。

### 5.3 生命周期、素材与计费边界

- 国内登记列表和 DELETE，但在途任务取消、终态删除及错误响应语义还未完整说明；国外未登记这两个
  操作。当前官方协议的公开能力投影可能声明删除，必须补齐操作证据或缩小该线路声明，不能承诺国外全生命周期兼容。
- 公开列表由本地 Task 权限和事实负责，不因 Provider 有列表接口就直接暴露账号下全部任务。
- 两份文档只声明 HTTPS 媒体 URL，未给出素材组/素材库或 `asset://` 合同；先选 `none`，不沿用
  火山 Action、BytePlus Action、FunCloud 托管素材配对。
- 回调结构声明不是验签或交付保证。首期兼容性验证可使用现有轮询；不从普通 Vidu HMAC 文档补推规则。
- 价格按模型、分辨率和是否含参考视频区别，国内人民币、国外美元。官方传输可读取 Token 用量，
  不决定客户价格。本次按用户明确要求将八个客户模型公式及预扣预算逐项复制官方渠道 81 的对应模型；供应商账单和失败费用仍须独立核对。
- Task 与资金继续使用现有冻结快照、发送前 durable attempt、hold 原子提交、unknown 不重发和幂等结算，
  不因这次 Provider 接入建立第二套公共事实。

完整的现行边界见[Seedance 架构](../20-architecture/Seedance专用渠道与Link架构.md)。

## 6. 本次验证与剩余验收

已做本地静态和插件函数验证：两站八个别名经过官方 buildCreate，保留 content、duration、resolution
与显式 generate_audio=false；创建 id、文档成功响应结构解析通过。确认 configured 策略允许这些名称，
同时确认精确别名元数据尚不存在。该检查没有网络请求，不是 Provider 端到端测试。

已有官方协议回归通过：`TestOfficialPluginPreservesModelArkWireAndProbe`、
`TestOfficialPluginUsageMatchesFrozenGoSemantics`、`TestOfficialPluginCannotChangePricedRequest`。
这些验证请求保持、用量整数语义和预扣参数不被插件改变，不替代真实 Vidu 验收。

真实验收仍需：两站分别使用所属 Key 完成文本/关键帧/参考媒体成功与失败流程，核对 V3 查询路径、
4K 字面值、音频开关、Q3.1 参数、实际 Token 与账单，补齐删除和内容有效期证据。
已验证两站 Key 能提交图片与音频任务，本地运行插件为 1.3.5；真实账单、外部数据库和生产灰度仍未验证。

最终判断：**可复用 Seedance 渠道底座，但现有官方协议无法直接履约 Vidu 的真实查询响应；需 Vidu 响应转换与计费用量验收后再启用。**
