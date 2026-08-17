---
status: historical
owner: Dev Team
last-reviewed: 2026-08-11
archived-at: 2026-08-11
source-path: docs/50-planning/2026-08-11-TokenSave与墨行Seedance实施方案.md
superseded-by:
  - docs/20-architecture/Seedance模型接入设计/墨行/渠道对接设计.md
  - docs/20-architecture/Seedance模型接入设计/墨行/素材库对接设计.md
  - docs/50-planning/路线图.md
---

# TokenSave 与墨行 Seedance 实施方案

> 本文是已完成实施的历史方案；当前架构以
> [渠道对接设计](../../../../20-architecture/Seedance模型接入设计/墨行/渠道对接设计.md)和
> [素材库对接设计](../../../../20-architecture/Seedance模型接入设计/墨行/素材库对接设计.md)为准，
> 未完成的真实 Provider、账单与灰度验收以[路线图](../../../../50-planning/路线图.md)为准。

## 实施依据

本方案落实当时的 TokenSave 与墨行 Seedance 接入设计，
并遵守 Seedance 专用 Channel、ModelArk V3 北向、代码协议注册、确定性单 Channel 路由、durable attempt、
冻结 Task、素材一对一作用域和计费安全边界。

## 阶段一：协议身份和模型合同

- [x] 在 relaykit 与根模块 DTO 中新增六个 Provider 命名协议常量和公开名称映射。
- [x] 删除 `media_task_v1`、`relay_assets_v1` 的可配置枚举、配对与运行时处理，不保留 alias。
- [x] 为墨行 ModelArk 视频、JoyCreator 素材和墨行火山素材增加独立 transport profile/revision。
- [x] 注册五个客户模型及其精确 Provider 模型映射示例，确保一个 Channel 只承载一个客户模型。
- [x] 为五个模型建立集中、代码化的时长、分辨率、参考媒体、音频默认值和输出格式规格。

验收信号：协议枚举测试、合法/非法协议配对测试和逐模型规格表测试通过；旧通用协议配置明确失败。

## 阶段二：北向 typed contract 与南向视频 adapter

- [x] 在 ModelArk V3 创建请求增加可选 `output_format`，同步未知字段白名单和 OpenAPI/前端类型。
- [x] 保证省略值与显式零值由指针字段区分，禁止任意 `extra` 参数进入南向。
- [x] TokenSave 2.0 继续使用旧媒体任务转换，只接受已确认的图片输入。
- [x] 墨行 2.0 增加独立旧媒体任务转换，支持 `reference_images/videos/audios`。
- [x] 墨行 Fast、Mini、2.5 增加 ModelArk 风格直接转换和现有媒体任务响应归一。
- [x] Provider POST 前按协议和冻结 Provider 模型执行规格校验；不支持字段返回 400。

验收信号：三类 adapter 的精确请求快照、非法媒体组合、边界值、未知字段、显式零值和响应归一测试通过。

## 阶段三：素材 adapter 与 Resolver

- [x] 将现有 `/assets/*` adapter 注册为 `tokensave_assets_v1`。
- [x] 实现 `moxing_joycreator_assets_v1` 的素材组和素材动作、envelope 错误解析与状态归一。
- [x] 实现 `moxing_volc_assets_v1` 的 `/v1/volc/assets/*` 动作与响应归一。
- [x] 在素材服务注册三种协议，不复制平台 Asset/AssetGroup 状态机。
- [x] 固化视频/素材协议精确配对，并验证素材在每次发送前按租户、app、Channel 和协议复检。
- [x] 增加 Fast、Mini、2.5 跨 Channel/跨模型引用拒绝测试。

验收信号：三种素材协议的创建、查询、更新、删除能力按 Provider 合同工作；Active 引用成功，跨作用域和
未 Active 引用失败关闭，source URL 与 Provider 私域 ID 不进入公共响应或持久化字段。

## 阶段四：计费和异步闭环

- [x] TokenSave/墨行 2.0/Fast/Mini 的 `duration=-1` 统一按 15 秒预扣；墨行 2.5 按 30 秒预扣。
- [x] Fast、Mini、2.5 省略 `generate_audio` 时统一按 `true` 进入请求快照和计费 probe。
- [x] 五个客户模型可读取已有价格配置，不因后缀导致 ratio 查找失败。
- [x] 回归 durable attempt 创建、资金 hold、Task ID 原子转移、unknown 不重发、幂等结算和退款。
- [x] 核对计费乘数边界和 quota checked conversion，确保异常输入不能产生负费用或绕过预扣。

验收信号：预估、预扣、成功结算、失败退款、unknown 和重复轮询的合同测试通过；请求默认值与计费默认值
完全一致。

## 阶段五：管理端与国际化

- [x] 更新前端视频/素材协议 union、选项和精确配对校验。
- [x] 增加六个协议名称的 en、zh、zh-TW、fr、ja、ru、vi 翻译。
- [x] 管理表单只显示 Provider 命名协议，不再显示两个通用旧协议。
- [x] 补充五个客户模型配置示例或可发现模型选项，不自动替管理员生成多模型 Channel。

验收信号：Bun 类型检查、相关前端测试、i18n 同步和受影响文件格式检查通过；中英文显示与设计一致。

## 阶段六：验证与事实收敛

- [x] 运行最窄 Go 包测试并扩展至受影响 Seedance、service、middleware、dto 包。
- [x] 运行 `cd relaykit && GOWORK=off go build ./...`。
- [x] 运行前端相关测试、`bun run typecheck`、i18n 校验及受影响文件 lint/format。
- [x] 运行 `task docs:check`、`task ai:check`；若改动公开 API 文档，再运行 `cd web && bun run docs:validate`。
- [x] 将已验证实现结果回填本实施方案；未被真实 Provider 或账单证明的部分明确列为联调项。

## 最小入侵接线点

预计只在既有 NEWAPI 文件保留以下窄接线：

1. DTO 协议 alias、ModelArk `output_format` 和未知字段白名单；
2. Seedance adaptor 保存公开协议身份并调用新增的 Provider adapter/spec validator；
3. 视频 profile 的固定路径与响应归一分支；
4. 素材服务的协议到 adapter 注册 switch；
5. 前端类型、选项、配对和 locale 条目。

逐模型规格、墨行请求转换、JoyCreator/火山素材实现和测试优先放入新增文件，不借本功能重构 NEWAPI
原生入口、通用任务状态机、素材模型或计费底座。

## 回滚方式

若本地验证失败，禁用对应客户模型 Channel 即停止履约；不修改历史 Task/attempt/Asset 快照，不把请求
切换到其它 Provider。代码回滚按新增协议和 adapter 边界进行，禁止恢复通用协议 alias 或自动推断分支。

## 实施结果

本地实现已完成：六个 Provider 命名协议、五个客户模型精确映射、逐模型参数校验、三类视频转换、三类
素材注册、墨行 JoyCreator/火山素材 adapter、typed `output_format`、默认音频/智能时长计费、管理端精确
配对与七语言名称均已落地。旧 `media_task_v1` / `relay_assets_v1` 不再是有效配置，也没有兼容 alias。

验证通过：Seedance 全包测试，受影响 middleware/model/service/relay 全包测试，relaykit 独立测试与
`GOWORK=off` 构建，根模块构建，前端类型检查、17 个相关 Vitest、受影响文件 oxlint/oxfmt、七语言 i18n
同步、公开 API 文档校验、`task docs:check` 和 `task ai:check`。

本地测试没有使用真实 TokenSave/墨行凭据，因此 Provider 任务、素材 Active、真实账单和生产灰度证据仍按
[路线图](../../../../50-planning/路线图.md)中的逐模型 E2E 项执行；这不改变本次已完成的代码合同。
