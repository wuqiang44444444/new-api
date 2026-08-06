---
status: historical
owner: Dev Team
last-reviewed: 2026-08-06
archived-at: 2026-08-06
source-path: "docs/80-dev/2026-08-05-Seedance多Provider单轨接入实施与验证结果报告.md"
superseded-by:
  - "docs/20-architecture/seedance 模型接入设计/README.md"
  - "docs/50-planning/Seedance多Provider真实验证矩阵.md"
---

# Seedance 多 Provider 单轨接入实施与验证结果报告

## 1. 结论

本次已完成墨行、飞彩与 FunCloud Seedance 的代码收敛、本地渠道配置、账单配置清理、
部署数据审计与确定性回归。飞彩只保留单轨 `feicai.seedance-videos@v2`，墨行海外版只保留
`moxing.seedance-media-task@v2`，FunCloud Standard/Fast 共用中立 implementation 但保持两个 SKU 与精确路径隔离。

当前生产姿态仍是失败关闭：

- 墨行渠道 49/50、飞彩渠道 51/52 和 BytePlus 渠道 48 禁用，不会产生新流量；
- 飞彩生产 size registry 为空，10 个 SKU 即使结构登记完整也会在预扣和 Provider POST 前拒绝；
- TokenSave 保留 publication 但 Ability 已禁用；本地只有 FunCloud Standard/Fast 存在已启用 Ability
  和 publication，且仅在 `randy` 验收分组可用；
- 后续已获得仅限墨行、总费用上限 ¥100 的真实 Provider 验证授权；本轮没有继续飞彩公网黑盒。

因此，“代码和配置完成”不等于“全模型已生产发布”。本报告保持 `in-progress`，直到真实 Provider 证据门禁闭合。

## 2. 代码实施结果

### 2.1 墨行当前两个模型

- 当前 Link implementation 提升到 `moxing.seedance-media-task@v2`，历史 Ark 实现不再履约同一 SKU。
- 专属 capability 只允许 480p/720p、4～15 秒或 `-1`、显式画幅、非空文本与最多 9 张图片。
- 单首帧稳定转换为 `input_mode=single_image, control_mode=none`；首尾帧与参考图互斥。
- `duration=-1` 的预扣上界固定按 15 秒评估，不用全局 3600 秒常量替代模型边界。
- 当前 implementation 只登记 `general` 图片素材，不声明 `real_person`、音频或视频素材。
- 未验证的 Provider usage 不再投影给客户或参与结算；TokenSave 成功样本实际未返回 usage，按秒费用
  只读取冻结请求规格。
- `doubao-seedance-2-0-260128` 已升级为 `tokensave.seedance-media-task@v2`，只登记 480p/720p/1080p、
  图片场景和按秒计费；TokenSave v1 仅保留 deprecated 历史解析。
- Moxing Ark v1 也只保留 deprecated 历史解析，不进入新任务候选。

### 2.2 飞彩全模型 v2

- 新增 10 个独立 SKU capability、10 条 execution binding 和唯一 adapter
  `54:third_party_json_video_media_arrays:v2`。
- capability 已表达 `MinImages`、`BillingMode`、显式 duration/resolution/ratio 要求和 Pro PI
  `reference_video`。SD2 必须有 1～9 张图片，Pro PI 固定 15 秒且按次计费。
- converter 只发送整数 `duration`，支持保序 `images[]/audios[]/videos[]`，且在发送前校验媒体角色与 HTTPS。
- size resolver 已替换为
  `(implementation, provider_model, resolution, ratio) -> size + billing class + evidence version`；
  converter 与 billing probe 共用同一权威。
- 禁用渠道允许先保存结构配置；但任一飞彩 SKU 没有已验 ratio/size 证据时，启用 Channel 或 Ability
  会被 publication readiness 门禁拒绝，不只依赖请求时失败。
- 旧两元 size fallback、v1 adapter 分派与 v1 `403 + feicai_account_required` 确定拒绝均已移除。
- 新增独立黑盒验证命令，需要显式费用确认与环境变量凭据；输出不包含 Provider task ID、
  内容 URL 或原始 Provider 错误响应。

飞彩的逐模型实施明细见[飞彩 Seedance 全模型 v2 实施方案与结果报告](2026-08-05-飞彩Seedance全模型v2实施方案与结果报告.md)。

### 2.3 FunCloud

- Standard/Fast 保持两个独立 SKU 和精确 create path，不与飞彩 Fast 或 TokenSave 混用。
- 图片角色显式登记 `reference_image/first_frame/last_frame`，音频和视频分别只允许参考角色。
- 只有 HTTP 状态成功且业务响应含可信 task ID 才创建 Task。未经独立取证的 body code 不再被当作确定未创建；
  发送后不确定继续 fail closed。
- `end_user_subject` 仍是平台作用域字段，不发给 Provider。

## 3. 本地配置结果

| 渠道 | 模型范围 | implementation | 分组 | Channel/Ability | publication |
| ---: | --- | --- | --- | --- | --- |
| 48 | `seedance-byteplus` | `byteplus.seedance-ark@v1` | default,randy | 禁用/禁用 | 无 |
| 49 | `seedance-2-0-oversea` | `moxing.seedance-media-task@v2` | randy | 禁用/禁用 | 无 |
| 50 | `doubao-seedance-2-0-260128` | `tokensave.seedance-media-task@v2` | default | 禁用/禁用 | 保留 v1 publication |
| 51 | Mini/Fast/standard 720p/1080p/4K | `feicai.seedance-videos@v2` | default | 禁用/禁用 | 无 |
| 52 | SD2/value 720p/1080p/4K/Pro PI | `feicai.seedance-videos@v2` | default | 禁用/禁用 | 无 |
| 53 | `seedance-2.0-standard` | `funcloud.seedance-json@v1` | randy | 启用/启用 | v1 |
| 54 | `seedance-2.0-fast` | `funcloud.seedance-json@v1` | randy | 启用/启用 | v1 |

配置审计结果：

- 渠道 51/52 各配置 5 个客户模型到 Provider 模型的精确映射，共 10 条禁用 Ability。
- 10 个飞彩 SKU 都配置 `tiered_expr` 与预扣上界；表达式是禁用渠道中的候选价格，不是已审批生产售价。
- 已删除错误嵌套的账单配置键、历史 dreamina 价格键和过时海外官 Key 价格键。
- 本地数据库一致性检查通过；启动后没有无效 Link implementation 渠道告警。
- 变更前数据库快照为 `one-api-pre-seedance-v2-20260805.db`。
- 墨行双模型升级前另建快照 `one-api-pre-moxing-dual-20260805.db`。

## 4. 验证结果

### 4.1 确定性回归

以下验证已通过：

```bash
env GOCACHE=/private/tmp/yuan-gateway-go-cache go test \
  ./dto ./model ./relay/common \
  ./relay/channel/task/doubao/thirdparty/mediaarrays \
  ./relay/channel/task/doubao/thirdparty/funcloud \
  ./relay/channel/task/doubao ./relay ./middleware ./service ./controller -count=1

env GOCACHE=/private/tmp/yuan-gateway-go-cache go test ./... -run '^$' -count=1
```

前一组覆盖 capability、implementation/binding、adapter 版本、请求转换、size 精确命中、计费探针、
durable attempt、unknown 姿态、轮询和内容代理。后一组证明全部 Go package 可编译。

### 4.2 已发生的 FunCloud 真实黑盒事实

本地数据库已冻结以下真实 Provider 履约事实，本报告不记录 Provider task ID、结果 URL、凭据或原始响应：

| 模型 | 完整 attempt | Provider 成功 | Provider 失败 | 计费结果 |
| --- | ---: | ---: | ---: | --- |
| FunCloud Standard | 12 | 5 | 7 | 全部已结算；成功任务扣费，失败任务目标费用为 0 |
| FunCloud Fast | 8 | 7 | 1 | 全部已结算；成功任务扣费，失败任务目标费用为 0 |

成功任务已覆盖创建、轮询到终态和受鉴权内容代理；失败任务也证明终态归一与零目标结算。
这些结果不能替代墨行、飞彩、TokenSave 或 BytePlus 的独立证据。

核对期间发现本机未确认来源的进程在网关运行时额外提交了 FunCloud 请求；这些请求已经进入 durable
attempt 并正常完成、结算。最后一次核对后 8100 端口已无监听进程。恢复网关和继续真实 Provider 验证前，
必须先取得凭据使用范围与费用上限的明确授权。

### 4.3 墨行真实调用进展与尚未完成项

| Provider/线路 | 待验证范围 | 当前原因 |
| --- | --- | --- |
| 墨行 oversea v2 | 创建、轮询、结果、usage、素材与账单 | 保存凭据认证预检为 401；未产生付费任务 |
| 飞彩 v2 | 10 个 Provider 模型逐一创建/轮询/内容/账单、精确 size | 生产 registry 为空；未获授权使用保存凭据并产生真实费用 |
| TokenSave v2 | 4 秒 480p 文生已完成创建、轮询、MP4 与 Range；仍需图片场景、720p/1080p、智能时长和账单 | 渠道保持禁用；成功样本 `result` 为对象且无 `usage` |
| BytePlus | 精确 endpoint/binding 与真实创建 | 本地渠道配置与当前 binding 不一致，已禁用 |

墨行专项实现、真实证据和费用边界见
[墨行双 Seedance 模型实施与实测结果报告](2026-08-05-墨行双Seedance模型实施与实测结果报告.md)。
飞彩 10 个模型各执行一次最小成功任务的研究估算为约 ¥72，失败探测、重试和其它 Provider 调用会增加费用；
本轮授权明确排除继续执行飞彩调用。

## 5. 发布与回滚

发布顺序必须是：精确 Provider 凭据黑盒 -> 实际账单复核 -> 完整确定性回归 -> 单模型 Ability ->
publication -> 隔离分组灰度。不能因为 10 个飞彩 SKU 已登记就一次性全部启用。

如需回滚，先停流并保留已存在的 Task、attempt、资金和审计事实，再使用变更前数据库快照和匹配的代码版本。
不在 v2 代码中增加 v1 fallback，也不删除或改写已存在的审计记录。

## 6. 原始实施目标完成度审计

| 原始目标 | 权威证据 | 当前判定 |
| --- | --- | --- |
| 完整代码优化与修复 | capability/implementation/binding、adapter、发布门禁、计费探针与相关回归 | 已完成 |
| 所有模型配置 | 渠道 48–54、16 条相关 Ability、3 条当前 publication、10 个飞彩模型映射和计费候选 | 已完成配置；未闭合证据的渠道保持禁用 |
| 测试所有模型调用正确 | FunCloud 有真实 Task 证据；墨行、飞彩、TokenSave、BytePlus 尚缺本轮独立公网证据 | 未完成，不能用确定性测试替代 |
| 在 80-dev 编写实施方案和结果报告 | 本报告、飞彩专项报告、FunCloud 专项报告 | 已完成并保持 `in-progress` |

因此整个目标尚不能宣称完成。最终关闭条件是：在明确的凭据与费用授权下完成 4.3 的真实调用，按逐模型
结果登记飞彩 size/evidence、复核 Provider 账单，重新执行确定性回归，并把本报告状态更新为当前事实。
