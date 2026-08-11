---
status: current
owner: Dev Team
last-reviewed: 2026-08-10
---

# 飞彩 Seedance 全模型回归规约

## 1. 边界

飞彩使用 `ChannelTypeSeedanceLink` 和代码化 `url_media_arrays_v1` 协议。该协议只接受 URL/Data URL，
必须配对 `asset_upstream_protocol=none`。每个客户模型配置独立渠道和独立
`model_mapping`；系统不维护 Link SKU、publication、implementation 或 execution binding。

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
- resolution、ratio 和已验证 size evidence；
- text、image、audio、video 的允许数量、role 和组合；
- HTTP/HTTPS、Data URL 成功，以及 `asset://ast_*`、`asset://pubref_*` 明确失败；
- adapter 不支持字段明确失败，不静默删除、钳制或改义；
- converter 与 billing probe 对同一请求解析相同 duration、size 和计费档位。

模型的图片必填、音视频支持范围和真人内容差异由飞彩判断；平台只维护线协议与 size evidence，不建立
逐模型 capability 门禁。

## 4. 异步与失败

- Provider POST 前建立 durable attempt、资金 hold 和 `sending`；
- 每个客户创建请求最多发送一次 Provider POST；
- 只有非空、长度受控的可信 Provider task ID 才能创建 Task；
- 断连、坏 JSON、缺失 ID 或不确定响应进入视频 `unknown`；
- unknown 不自动重发、换渠道或退款；确定失败也不切到另一飞彩/官方线路；
- Task 冻结客户模型、Channel、Provider 模型、协议、连接、素材和计费事实；轮询、删除和结算不重选。

## 5. 素材

飞彩 Channel 固定使用 `asset_upstream_protocol=none`。`asset://ast_*`、`asset://pubref_*` 和真人认证
流程必须在 Provider POST 前明确失败；普通 URL/Data URL 不创建平台素材。不得借用其它 Seedance
Channel 的素材，也不得建立自动物化、迁移、source fallback 或多 binding。

## 6. 计费

- duration、媒体数量和倍率在 quota 计算前有上界；
- quota 换算使用 checked helper，NaN、Inf 或超大值不会产生负费用；
- 预扣和终态使用创建时冻结的同一计费事实；
- 失败退款与 unknown hold 均幂等；unknown 不按期限自动释放，只有技术人员核实明确未创建后才人工释放；
- Provider 返回的额度字段不覆盖客户 quota 或历史结算。

## 7. 验证命令

```bash
go test ./model ./middleware ./relay/common ./relay/channel/task/doubao/... ./relay ./service ./controller
```

涉及 `relaykit/` 时另执行：

```bash
cd relaykit && GOWORK=off go build ./...
```

测试使用确定输入和精确断言，不用随机循环、sleep、日志存在性或私有文件布局代替行为合同。
