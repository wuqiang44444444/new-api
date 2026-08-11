---
adr: 0016
status: accepted
date: 2026-08-10
last-reviewed: 2026-08-11
superseded-by: ""
---

# ADR-0016: Seedance 专用渠道与确定性素材代理

## Context

旧 Link 架构把客户模型 publication、Link SKU、逐模型 capability、不可变 implementation、execution
binding、接入方案、候选交集和通用 AssetBinding 组合成一套独立合同治理系统。该设计可以在系统内
证明多个渠道等价履行同一合同，但管理人员无法从 Channel 和客户模型直接理解真实路由；运行时还要
重复执行多层门禁、候选过滤和绑定复检。

实际业务中 Link 主要承载 Seedance。运营明确要求不同 Provider 线路使用不同前端客户模型名，一个
Seedance 模型只对应一个渠道；技术人员会在线下判断新模型是否兼容已有协议，不要求系统或管理员
自动认证兼容性。Seedance 又是高成本重任务，失败后切换 Provider 再创建会产生重复成本和对账歧义。

官方素材库包含预处理、审核、一致性检测和真人认证等 Provider 控制面能力。中转平台必须代理这些
能力，但没有必要自建人脸表单、法律授权域、多 Provider 素材迁移或低概率异常核查系统。国内火山、
海外 BytePlus 和第三方素材协议也不能假设彼此兼容。

## Decision

- 新建业务渠道类型 `ChannelTypeSeedanceLink`，覆盖所有官方和第三方 Seedance 模型。Seedance 北向
  固定提供 ModelArk V3 create/get/list/delete 四组任务接口；`/v1/video/generations` 继续属于
  NEWAPI 原生渠道。
- 不同 Seedance 渠道必须使用不同客户模型名，一个已启用客户模型只对应一个 Seedance 渠道。唯一性
  只在保存、编辑已启用渠道和重新启用时校验；不增加请求时复检、数据库唯一约束、启动扫描、自动
  修复或并发补偿。
- Seedance 不使用 Priority、Weight、Affinity、随机分发、失败重选、跨渠道重试或 fallback。Group
  只控制访问；客户模型直接登记在 Seedance Channel，不写入 NEWAPI 原生 Ability 或通用分发缓存。
  模型发现和价格展示只使用只读投影。
- 南向差异由代码化 `video_upstream_protocol` / `asset_upstream_protocol` adapter 表达。管理员只选择
  已有协议并配置 NEWAPI 普通 Channel 字段，不编写 JSON 转换或状态脚本。技术人员线下判断兼容性；
  完全兼容时可零开发配置上线，不兼容时新增代码 adapter。
- 整体 Link 删除 publication、Link SKU、逐模型 capability、`LinkImplementation`、execution
  binding、内容 hash 等价证明、Link Access Plan、改绑审计和候选交集。履约主链收敛为：

  ```text
  customer model -> Group / price -> Seedance Channel 模型登记 -> Channel
    -> model_mapping -> code-backed upstream_protocol -> Provider model
  ```

- Seedance 视频创建只允许一次 Provider POST。发送前继续建立 durable attempt 和资金 hold；取得可信
  Provider task ID 后才创建 Task。结果不明保持视频 `unknown`，不自动重发、换渠道或退款。Task 冻结
  Channel、adapter、Provider 模型、连接、素材和计费事实。
- `unknown` 不设置自动释放期限，不进入 `released_with_exposure`。只有技术人员核实 Provider 明确未
  创建后才人工推进 `rejected` 并释放资金，或补录可信 task ID 恢复成功事实。
- 客户素材 API 使用 `/v1/assets`、`/v1/asset-groups`、`ast_*`、`astgrp_*` 和 `asset://ast_*`。一个平台
  Asset/AssetGroup 一对一固定到 `user_id + app_id`、Channel、Provider 账号、Region/Project 和一个
  Provider 资源；删除通用 0..N AssetBinding、自动迁移、物化和 source fallback。
- 国内与海外素材互不迁移。客户不提交区域选择，而是通过不同客户模型选择不同产品。控制台既有
  Provider 素材必须由管理员显式导入和分配，不能向普通客户暴露账号级列表或裸 Provider ID。
- 真人认证作为 AssetGroup 的一种上游流程。平台直接返回官方或第三方认证链接/二维码并按需查询
  状态；删除 `RealPersonAuthorization`、平台人脸/活体/授权表单、H5 包装、reservation 和自建撤回域。
- ModelArk V3 视频允许 `asset://ast_*` 与 HTTP/Data URL 混用。含平台素材时，客户模型的唯一 Channel
  必须与所有素材 Channel 相同，且多素材共享账号、Region、Project；否则分别返回
  `asset_channel_mismatch` 或 `asset_scope_conflict`。
- 素材作用域由固定 Channel、Base URL、协议、账号、Region/Project 表达，Key/AK/SK 值不参与作用域
  指纹；同一作用域允许凭据轮换，作用域变化必须新建渠道。
- 素材管理不使用视频级 `create_unknown` / `delete_unknown`、自动重试、后台核查或孤儿扫描。创建未
  取得可信 Provider ID 即失败并记录技术日志；删除不明确即失败并保留状态，后续 GET 明确不存在后
  再标记 deleted。
- 图片 Link 同步删除 publication/SKU/implementation/binding 依赖，继续使用客户模型、Channel、
  `model_mapping`、代码 converter 和共享 `media-image-task/v1` Task 生命周期；Seedance 的唯一渠道与
  不切换规则不自动扩张到图片。

## Consequences

- 收益：管理员可从“客户模型—Seedance 专用渠道—上游协议”直接理解路由，运维、计费和对账不再
  隐藏在 SKU 或实现绑定中。
- 收益：高成本视频不存在自动切换造成的重复任务和双重 Provider 成本；Task 与素材仍保留必要的
  耐久事实和租户隔离。
- 收益：兼容新模型时可以通过普通配置上线；系统不承担低价值自动认证、修复和核查复杂度。
- 代价：允许 Seedance 专属 adapter 保留少量重复；技术人员必须在线下判断兼容性并给管理员明确
  配置指引。
- 代价：一个 Provider 线路必须使用独立客户模型名，不能把多个渠道聚合成同一个前端模型。
- 代价：素材不能自动跨渠道、账号、Project 或国内/海外复用；需要分别创建和审核。
- 风险：保存时唯一性校验被绕过或数据库被非标准修改时可能形成非法配置；该低频问题由管理员检查，
  不在请求热路径增加补丁。
- 风险：国内官方素材合同尚需独立验证；当前 BytePlus 实现不能通过替换 Host/Region 直接复用。
- 迁移约束：已有生产 `DoubaoVideo` 渠道不得原地改型；应新建 Seedance 专用渠道和独立客户模型，验证
  后停用旧配置。存量 Task 继续读取其创建时旧快照。

## Alternatives Considered

- 继续使用 `DoubaoVideo`，在内部挂载 Seedance Link profile：未采用。管理人员无法从渠道类型判断
  北向是 `/v1/video/generations` 还是 ModelArk V3，业务语义混乱。
- 保留 publication、SKU、implementation 和 execution binding：未采用。它们解决的是多候选等价履约，
  与“一个 Seedance 客户模型只对应一个渠道”的业务规则不匹配，维护成本高。
- 允许管理员编写 JSON 请求/响应映射：未采用。管理员无法可靠理解协议和状态机，错误会直接影响高
  成本任务和账务；协议必须由代码和技术评审承担。
- 让一个客户模型配置多个渠道并依靠 Priority/Weight：未采用。视频失败切换会重复创建和增加对账
  复杂度，且管理员无法确认实际 Provider。
- 自建真人认证与授权平台：未采用。涉及人脸、活体、证件和法律流程，超出中转平台边界；应直接使用
  Provider 页面和审核。
- 为素材创建/删除建立 unknown 对账和管理员核查系统：未采用。素材操作低频异常不足以证明额外状态、
  后台任务和运维页面的长期成本合理，技术日志和人工排查足够。
