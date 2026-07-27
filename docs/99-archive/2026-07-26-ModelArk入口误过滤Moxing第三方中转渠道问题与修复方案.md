---
status: historical
owner: Dev Team
last-reviewed: 2026-07-27
superseded-by: "../20-architecture/视频上游接入与异步任务架构.md; ../40-operations/Seedance视频渠道与计费配置手册.md"
---

# ModelArk 入口误过滤 Moxing 第三方中转渠道：问题与修复方案

## 1. 结论

报错“分组 default 下模型 seedance-2-0-oversea 无可用渠道”不是渠道停用、分组错误、模型名错误，也不是重启未生效，而是 ModelArk 创建接口的渠道约束与已完成的 Moxing 协议适配没有接通。

`seedance-2-0-oversea` 唯一可用渠道使用 `DoubaoVideo + third_party_relay`。该渠道本身配置正确，模型能力记录也已启用；但请求进入 `/api/v3/contents/generations/tasks` 后，`ModelArkVideoChannelConstraint` 仅保留空协议或 `official` 渠道，提前排除了 `third_party_relay`。随后分发器按 `default + seedance-2-0-oversea` 取交集时结果为空，因此返回 503。

修复方向是：ModelArk 作为北向客户端协议，不应等同于官方南向渠道。只要 DoubaoVideo 渠道的南向协议是系统已经实现并登记的协议，就应允许其进入后续分发；具体请求转换、响应归一化和任务轮询由对应适配器处理。

## 2. 现场信息

- 失败接口：`POST /api/v3/contents/generations/tasks`
- 请求模型：`seedance-2-0-oversea`
- 请求分组：`default`
- 失败请求 ID：`202607261505565962650008268d9d68giZ3Hqv`
- 返回结果：HTTP 503，`分组 default 下模型 seedance-2-0-oversea 无可用渠道`
- 目标渠道类型：`DoubaoVideo`
- 目标渠道协议：`third_party_relay`
- 上游创建路径：`/v1/media/generations`
- 上游查询路径：`/v1/media/tasks/{task_id}`

安全说明：本文不记录任何 API Key。排障过程中已经出现在聊天或截图中的 Key 应按泄露凭据处理并尽快轮换。

## 3. 已确认的渠道状态

| 检查项 | 结果 |
| --- | --- |
| 渠道状态 | 已启用 |
| 渠道类型 | `DoubaoVideo` |
| 渠道分组 | `default` |
| 渠道模型 | `seedance-2-0-oversea` |
| 能力记录 | `default + seedance-2-0-oversea + 目标渠道` 已启用 |
| 视频上游方案 | `third_party_relay` |
| 上游地址及路径 | 已按 Moxing 统一媒体异步任务协议配置 |
| 旧视频入口选渠 | 能命中该渠道；无效时长请求返回业务校验错误，证明选渠链路可达 |
| 应用重启 | 已执行，问题仍稳定复现 |

以上证据排除了“未保存”“未重启”“渠道被禁用”“能力表没有同步”等常见原因。

## 4. 实际请求链路

ModelArk 创建请求经过以下处理：

1. `TokenAuth` 完成令牌和分组识别。
2. `TaskClientProtocol("modelark_v3")` 标记北向返回协议。
3. `ModelArkVideoCreateConvert` 把 ModelArk `content` 请求转换为内部视频任务结构，并把内部路径改写为 `/v1/video/generations`。
4. `AssetRouteConstraint` 根据素材引用情况缩小允许渠道集合。
5. `ModelArkVideoChannelConstraint` 再次缩小允许渠道集合。
6. `Distribute` 按分组、模型、状态和允许渠道集合选择最终渠道。
7. DoubaoVideo 适配器按渠道的 `video_upstream_profile` 构造上游请求并归一化响应。

问题发生在第 5 步。原实现只加入以下渠道：

```go
profile == "" || profile == official
```

因此 Moxing 渠道的 `third_party_relay` 在分发器运行前就被删除。此时即使数据库中存在正确的渠道和能力记录，也一定会得到“无可用渠道”。

## 5. 为什么这是适配接线问题

系统中已经存在 `third_party_relay` 的完整核心适配：

- 创建地址使用渠道配置的 `video_upstream_create_path`。
- `RelayCreateRequest` 把内部视频请求转换成第三方统一媒体任务结构。
- 创建响应被归一化为内部任务 ID。
- 轮询地址使用渠道配置的 `video_upstream_query_path_template`。
- `RelayTaskResponse` 归一化任务状态、结果 URL、用量和错误。
- 任务创建时会冻结上游协议、地址、查询路径和凭据快照，后续轮询不依赖渠道配置被再次修改。
- `service_tier` 等协议差异在 DoubaoVideo 适配层按上游能力处理。

也就是说，南向适配已经存在，缺失的是 ModelArk 北向入口对这些已知南向协议的放行。

北向协议和南向协议应保持解耦：

| 层次 | 本例取值 | 职责 |
| --- | --- | --- |
| 北向客户端协议 | `modelark_v3` | 接收 ModelArk 请求并投影 ModelArk 响应 |
| 内部任务合同 | 统一视频任务 | 统一选渠、计费、持久化和轮询 |
| 南向渠道协议 | `third_party_relay` | 调用 Moxing 创建/查询接口并归一化返回 |

ModelArk 入口不要求南向一定是 `official`。它只要求内部适配器能把该请求安全地转换到目标渠道。

## 6. 修复方案

### 6.1 渠道兼容规则

`ModelArkVideoChannelConstraint` 应允许：

- 空协议：历史渠道，按 `official` 兼容处理。
- `official`：官方 Ark 协议。
- `third_party_reverse_proxy`：Ark 合同兼容的第三方代理。
- `third_party_relay`：Moxing 等统一媒体任务中转协议。

未知协议仍必须拒绝，避免未实现的协议绕过约束。实现时复用 `VideoUpstreamProfile.IsValid()` 中的受控协议集合，不另建一份容易漂移的白名单。

### 6.2 保持不变的安全边界

本次修复只改变“哪些已知视频上游协议可从 ModelArk 创建入口参与选渠”，以下条件保持不变：

- 仍只查询启用的 `DoubaoVideo` 渠道。
- 仍由分发器校验用户分组、模型能力、渠道状态和优先级。
- 仍与 `AssetRouteConstraint` 产生的允许渠道集合取交集。
- 未知 `video_upstream_profile` 仍拒绝。
- 素材与目标上游协议不兼容时，仍由素材路由约束拒绝。
- `video_url`、`audio_url`、`service_tier` 等 Moxing 不支持的具体能力，仍由请求校验或适配器明确报错，不通过伪造官方渠道能力来放行。
- 第三方渠道暂不支持的取消、删除等生命周期操作，仍按现有能力边界返回不支持；这不应阻止创建与查询。

### 6.3 最小代码改动

修改 `middleware/modelark_video.go`：

- 增加一个表达稳定业务含义的兼容性函数。
- 空协议或 `IsValid()` 为真的协议允许参与 ModelArk 视频分发。
- 不改路由顺序、分发器、渠道配置模型和 DoubaoVideo 适配器。

修改 `middleware/modelark_video_test.go`：

- 增加中间件行为回归测试。
- 覆盖空协议、官方协议、第三方反向代理、Moxing 第三方中转、未知协议、禁用渠道和非视频渠道。

## 7. 验证矩阵

| 场景 | 预期 |
| --- | --- |
| ModelArk + `third_party_relay` + 匹配模型/分组 | 能进入分发并命中 Moxing 渠道 |
| ModelArk + `third_party_reverse_proxy` | 能进入分发 |
| ModelArk + `official` 或空协议 | 保持原行为 |
| ModelArk + 未知协议 | 在渠道约束阶段拒绝 |
| 已有素材限制只允许部分渠道 | 与协议允许集合取交集，不扩大范围 |
| 模型或分组确实不匹配 | 分发器继续返回无可用渠道 |
| Moxing 不支持的输入参数 | 由适配器返回明确的参数/能力错误 |

建议执行：

```bash
go test ./middleware ./relay/channel/task/doubao/... ./service
go test ./router ./controller
go build ./...
task docs:check
task ai:check
```

## 8. 上线与验收

1. 部署包含本修复的新后端二进制并重启后端；只重启旧二进制不会改变行为。
2. 确认目标渠道仍为启用状态，模型为 `seedance-2-0-oversea`，分组包含 `default`。
3. 通过 `/api/v3/contents/generations/tasks` 发起一个最小文本视频任务。
4. 后端日志应显示命中 Moxing 渠道，不再出现“分组 default 下模型 seedance-2-0-oversea 无可用渠道”。
5. 创建响应应返回任务 ID，随后 `/api/v3/contents/generations/tasks/{task_id}` 能完成状态查询。
6. 检查任务记录中的上游协议快照为 `third_party_relay`，上游任务 ID、状态、结果 URL 和用量均被正确归一化。

回滚仅需恢复 ModelArk 渠道约束的协议判断并重新部署，不涉及数据库迁移。

## 9. 后续文档一致性

现有 API 对接文档中如果仍将 ModelArk `model` 描述为“必须由 ModelArk 官方视频渠道支持”，应改为“必须由当前 Token 分组中的兼容视频渠道支持”。否则文档会继续把北向协议错误地等同于南向官方渠道。

## 10. 后续排障发现：Moxing 出站字段名

渠道约束修复后，任务已经能够创建并进入 Moxing 队列。后续生成失败排障发现，`third_party_relay` 转换器仍沿用了内部字段名：

| 内部/ModelArk 字段 | Moxing 媒体任务 V1 字段 |
| --- | --- |
| `ratio` | `aspect_ratio` |
| `generate_audio` | `with_audio` |

TokenSave 可以接受旧字段并创建任务，但其公开模型合同使用右侧字段；旧字段可能被忽略，不能保证画幅和音频开关被传递给最终供应商。

转换器现已保留北向内部字段不变，只在 Moxing 南向请求中输出 `aspect_ratio` 和 `with_audio`。回归测试同时断言旧字段不会残留，且显式 `false` 不会因 `omitempty` 被丢弃。
