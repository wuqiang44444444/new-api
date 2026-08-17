---
status: current
owner: Dev Team
last-reviewed: 2026-08-15
---

# 飞彩 Seedance 全模型回归规约

## 1. 边界

飞彩使用 `ChannelTypeSeedanceLink` 和代码化 `feicai_videos_v1` 专用协议。素材 CRUD 必须配对
`asset_upstream_protocol=none`；视频媒体接受 URL/Data URL，并将非空 `asset://<opaque-id>` 交给上游
最终判断。VIP 与性价比分别使用一个五模型 Channel，每个客户模型拥有
独立 `model_mapping`；系统不维护 Link SKU、publication、implementation 或 execution binding。

真实 Provider 行为见[飞彩上线验收手册](../40-operations/06-飞彩Seedance全模型上线验收手册.md)。

## 2. 渠道与协议

行为测试至少证明：

- 只有代码登记的飞彩协议可保存；管理员不能配置请求/响应 JSON 映射；
- 一个已启用客户模型只能存在于一个 Seedance Channel；停用渠道允许保留重复名称；
- 飞彩请求不使用 Priority、Weight、Affinity、随机分发、失败换渠或 fallback；
- `model_mapping` 精确产生预期 Provider 模型，未登记模型由 adapter 明确拒绝；
- `/v1/video/generations` 的原生 DoubaoVideo 行为不受影响。

## 3. 请求和转换

对准备上线的每个飞彩 Provider 模型使用确定表格测试覆盖：

- duration 的合法边界与首个非法值；
- resolution 和 ratio 必须由精确 Provider 模型登记表验证；南向发送 ratio，不生成或发送 size；
- SD2 只接受`16:9`、`9:16`，其它比例范围按精确 Provider 模型登记，不根据客户模型名称判断；
- text、image、audio、video 的允许数量、role 和组合；
- HTTP/HTTPS、Data URL 成功；`asset://<opaque-id>` 原样进入上游请求且不触发本地素材查询；
- adapter 不支持字段明确失败，不静默删除、钳制或改义；
- converter 与 billing probe 对同一请求解析相同 duration、ratio 和计费档位。

平台对十个精确 Provider 模型维护已验证的 duration、固定分辨率、图片/音频/视频数量和计费模式，
未知模型发送前失败。飞彩继续负责内容审核、媒体可拉取性和真实生成结果；代码登记表不演化为管理员
可编辑的逐模型 capability 系统。

## 4. 异步与失败

- Provider POST 前建立 durable attempt、资金 hold 和 `sending`；
- 每个客户创建请求最多发送一次 Provider POST；
- 只有非空、长度受控的可信 Provider task ID 才能创建 Task；
- 断连、坏 JSON、缺失 ID 或不确定响应进入视频 `unknown`；
- unknown 不自动重发、换渠道或退款；确定失败也不切到另一飞彩/官方线路；
- Task 冻结客户模型、Channel、Provider 模型、协议、连接、素材和计费事实；轮询、删除和结算不重选。

## 5. 素材

飞彩 Channel 固定使用 `asset_upstream_protocol=none`，因此素材 CRUD 和真人认证元数据明确为不支持。
视频中的 `asset://<opaque-id>` 只检查非空并交给飞彩 Provider；平台不判断该 ID 是否来自其它 Provider，
也不建立素材、自动物化、迁移、source fallback 或 binding。

## 6. 计费

- duration、媒体数量和倍率在 quota 计算前有上界；
- quota 换算使用 checked helper，NaN、Inf 或超大值不会产生负费用；
- 预扣和终态使用创建时冻结的同一计费事实；
- 失败退款与 unknown hold 均幂等；unknown 不按期限自动释放，只有技术人员核实明确未创建后才人工释放；
- Provider 返回的额度字段不覆盖客户 quota 或历史结算。
- 同一 Provider 模型、同时长的所有合法比例必须得到相同客户 quota，`size_multiplier` 恒为`1`。

## 7. 验证命令

```bash
go test ./model ./dto ./relay/common ./relay/channel/task/seedance/...
go test ./controller -run '^TestVideoFeicaiContentSourceUsesFrozenSnapshot$'
go test ./relay -run '^TestFeicaiVideoCreateHTTPDispositionFailsClosedWithoutVerifiedProviderContract$'
go test ./service -run '^(TestVideoPollingUsesFrozenFeicaiAdapterAndRedactsResultURLFromTaskData|TestVideoPollingRejectsUnknownFrozenAdapterBeforeFetch)$'
```

涉及 `relaykit/` 时另执行：

```bash
cd relaykit && GOWORK=off go test ./... && GOWORK=off go build ./...
```

测试使用确定输入和精确断言，不用随机循环、sleep、日志存在性或私有文件布局代替行为合同。
