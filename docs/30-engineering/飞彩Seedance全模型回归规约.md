---
status: accepted
owner: Dev Team
last-reviewed: 2026-08-05
---

# 飞彩 Seedance 全模型回归规约

## 1. 目的与边界

本文定义飞彩单轨 `feicai.seedance-videos/v2` 的确定性行为回归。测试保护客户合同、实现身份、
异步资金、计费、资源与内容代理，不以文件布局、私有常量或覆盖率为目标。真实 Provider 行为见
[上线验收手册](../40-operations/06-飞彩Seedance全模型上线验收手册.md)。

架构权威见：

- [总体架构与履约设计](<../20-architecture/seedance 模型接入设计/飞彩/飞彩总体架构与履约设计.md>)
- [全模型 SKU 与计费设计](<../20-architecture/seedance 模型接入设计/飞彩/飞彩全模型SKU与计费设计.md>)

## 2. 单轨 v2 与 v1 移除回归

必须以行为测试证明：

- `feicai.seedance-videos/v2` 能解析为唯一飞彩 implementation，并包含 10 个 Public SKU 与 10 条 binding；
- `feicai.seedance-videos/v1` 不再解析，不能保存、启用或创建新任务；
- media-arrays profile 为新任务冻结 adapter v2；冻结 v1、空版本、未知版本和 profile/version 错配均拒绝；
- 飞彩 v2 Task 的轮询和内容代理只接受冻结 v2 implementation/hash/adapter；
- 其它 Provider 的 implementation v1 继续正常解析，证明删除范围没有扩大；
- 含 v1 引用的部署数据审计结果会阻止单轨切换，而不是触发运行时 fallback。

不增加“v1 和 v2 同时成功”的测试，因为目标架构明确不支持双轨。

## 3. 10 模型 capability 回归矩阵

每个 Link SKU 至少使用表格测试覆盖精确 Provider binding、合法边界和首个非法边界：

| # | Link SKU | duration 用例 | 图片用例 | 音频用例 | 视频用例 | billing mode |
| ---: | --- | --- | --- | --- | --- | --- |
| 1 | `seedance-2.0-mini-720p` | 4、15、3、16、缺省策略 | 0、9、10 | 0、3、4 | 任意非零拒绝 | per-second |
| 2 | `seedance-2.0-sd2-720p` | 11、15、10、16、缺省拒绝 | 0 拒绝、1、9、10 | 任意非零拒绝 | 任意非零拒绝 | per-second |
| 3 | `seedance-2.0-fast-720p` | 4、15、3、16、缺省策略 | 0、9、10 | 0、3、4 | 任意非零拒绝 | per-second |
| 4 | `seedance-2.0-value-720p` | 4、15、3、16、默认 4 | 0、9、10 | 0、3、4 | 任意非零拒绝 | per-second |
| 5 | `seedance-2.0-standard-720p` | 同上但独立 SKU | 0、9、10 | 0、3、4 | 任意非零拒绝 | per-second |
| 6 | `seedance-2.0-value-1080p` | 4、15、3、16、缺省策略 | 0、9、10 | 0、3、4 | 任意非零拒绝 | per-second |
| 7 | `seedance-2.0-standard-1080p` | 同上但独立 SKU | 0、9、10 | 0、3、4 | 任意非零拒绝 | per-second |
| 8 | `seedance-2.0-value-4k` | 4、15、3、16、缺省策略 | 0、9、10 | 0、3、4 | 任意非零拒绝 | per-second |
| 9 | `seedance-2.0-standard-4k` | 同上但独立 SKU | 0、9、10 | 0、3、4 | 任意非零拒绝 | per-second |
| 10 | `seedance-2.0-pro-pi-720p` | 缺省、15 成功；14、16 拒绝 | 0、9、10 | 0、3、4 | 0、3、4；错误 role | per-request |

“缺省策略”只有在对应模型证据进入 capability 后才测试成功；未闭合时应测试为拒绝。所有模型还需覆盖：

- 空文本和至少一个非空文本；
- 正确/错误 `resolution`；
- 正确/错误媒体 role；
- 直接媒体、`asset://` 与两者混用；
- 全部 unsupported 字段及 metadata/extra 旁路；
- capability version/hash 稳定且对 min/max、role、billing mode 的变化敏感。

## 4. execution binding 与发布回归

- 10 个 Provider 模型分别唯一解析表中 Link SKU；
- Mini、Fast、value、standard、SD2、Pro PI 不能互为等价 binding；
- 飞彩 Fast 不能解析为 FunCloud `seedance-2.0-fast`；
- 未登记 Provider 模型、route/action/profile 错误或重复 binding 全部失败关闭；
- 自定义 customer model 经 `model_mapping` 后仍由 Provider 模型唯一解析冻结 SKU；
- publication 与冻结 capability 不一致时不进入选渠；
- 零飞彩候选时不降级到普通 DoubaoVideo、FunCloud 或原生 OpenAI Videos。

## 5. size registry 回归

### 5.1 resolver 合同

resolver 用例必须包含完整键：

```text
(implementation v2, provider_model, resolution, ratio)
```

并验证返回的 Provider size 与 billing size class 同时来自同一记录。以下任一变化都应无法命中：

- implementation version；
- Provider 模型；
- 固定分辨率；
- ratio。

不得为未登记组合回退到旧 `{resolution}:{ratio}` 表，也不得从同分辨率的另一模型借用 size。

### 5.2 逐模型覆盖

10 个模型各自至少包含：

- 每个已批准 ratio 的精确像素值；
- 一个未登记 ratio；
- 一个错误分辨率；
- 一个其它 Provider 模型的同名 ratio；
- converter 输出 size 与 billing probe 的 size/billing class 一致。

当前两组 720p 值只能作为 v2 重新取证后的 fixture；不能让高分辨率或其它 720p 模型因共享表而通过。

## 6. 请求转换回归

- 上游只出现整数 `duration`，不出现字符串 `seconds`，也不同时发送两字段；
- prompt 按非空 text 输入顺序换行拼接；
- `images[]`、`audios[]`、`videos[]` 保持客户顺序；
- 空数组因 `omitempty` 省略，显式媒体项不能被静默丢弃；
- SD2 无图片在 Provider POST 前拒绝；
- Pro PI 的 `reference_video` 进入 `videos[]`，其它模型的任意视频输入在 POST 前拒绝；
- HTTP、内网、localhost、本地路径、相对路径和非法 Data URL 按媒体合同拒绝；
- converter 只消费已解析媒体，不从数据库、metadata 或原始 `asset://` 自行补值。

## 7. durable create 与确定拒绝回归

- 非空、长度受控、无控制字符的顶层 `id` 才能创建 Task；
- 创建响应只有 `task_id` 时不创建 Task；
- 断连、坏 JSON、缺失 id、未知非 2xx 均进入 unknown，不重发、不换渠道、不退款；
- v1 的 `403 + feicai_account_required` 不再触发飞彩确定拒绝；
- v2 未登记确定拒绝时，相同组合仍为 unknown；
- 若后续登记 v2 精确组合，正确 status+code 才 terminal rejection，近似 status、近似 code、同 message 和无 code 均 unknown；
- unknown hold、CAS 恢复、到期释放与 ProviderCostExposure 幂等。

## 8. 主轮询与任务列表对账回归

### 8.1 `/v1/videos/{id}`

- `queued -> queued`、`processing -> running`、`completed -> succeeded`、`failed -> failed`；
- 响应顶层 `id` 必须等于冻结 id；变化的 `task_id` 被忽略；
- 未知大小写、空状态、坏 JSON、ID 不匹配和完成无 URL进入 reconciliation required；
- 成功 URL 必须绝对 HTTPS、同源、无 userinfo，跨域或明文 URL 不结算成功。

### 8.2 `/v1/tasks`

- `QUEUED/IN_PROGRESS/SUCCESS/FAILURE` 使用独立 parser；
- 小写主轮询状态不能被任务列表 parser 接受，反之亦然；
- 只有稳定唯一键关联才能 CAS 恢复 create unknown；
- 相似时间、模型、prompt 或“最近一条”不得建立关联；
- 重复对账不重复建 Task、转移 hold、结算、退款或写 exposure。

## 9. Link 资源与内容代理回归

- v2 implementation 上限为图片 9、音频 3、视频 3，但请求仍受各 SKU 更窄 capability 约束；
- 只有 `general` Asset 可用，`real_person` 始终拒绝；
- 所有权、App、状态、publication、implementation v2、TTL 与多素材交集逐项失败关闭；
- 直接媒体与 `asset://` 混用拒绝；
- 内容代理使用 Task 冻结的 v2 implementation/hash、Base URL、Key 和结果 URL；
- Key 轮换不改变历史 Task 使用的冻结 Key；
- 同源 HTTPS、拒绝重定向、Range 与响应头白名单行为可观察；
- Key、Provider 模型、任务 ID 和完整 URL 不出现在普通响应、日志或错误正文。

## 10. 计费回归

- 九个 per-second SKU 使用 `unit price × duration × verified size multiplier`；
- Pro PI 使用 per-request，duration=15 只进入快照，不成为费用乘数；
- 未登记 size 不生成 billing probe；
- converter 与 billing probe 对同一请求解析相同 size 和 billing class；
- duration、媒体数量和倍率越界在 quota 计算前失败；
- quota 换算使用 checked helper，NaN、Inf、超大值不会变成负费用；
- saturation 进入 `QuotaClamp`、管理员日志和请求关联告警；
- 预扣与终态使用同一冻结 probe；
- 失败退款、unknown hold、到期释放与 exposure 均只执行一次；
- Provider `quota`、`total_usage`、`soft_limit_usd` 不覆盖客户 quota 或历史结算。

## 11. 建议验证入口

实现落地后，至少执行相关包的确定性测试：

```bash
go test ./model ./relay/common ./relay/channel/task/doubao/... ./relay ./service ./controller
```

若改动触及 `relaykit/` 公共 API，另执行：

```bash
cd relaykit && GOWORK=off go build ./...
```

测试必须使用确定输入和精确断言，不使用随机循环、sleep、日志存在性或私有文件布局代替行为合同。
