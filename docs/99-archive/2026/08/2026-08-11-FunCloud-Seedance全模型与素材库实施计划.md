---
status: historical
owner: Dev Team
last-reviewed: 2026-08-12
archived-at: 2026-08-12
source-path: docs/50-planning/2026-08-11-FunCloud-Seedance全模型与素材库实施计划.md
superseded-by:
  - docs/20-architecture/Seedance模型接入设计/FunCloud/FunCloud全模型与素材计费设计.md
  - docs/40-operations/01-计费与分组运维手册.md
  - docs/50-planning/路线图.md
---

# FunCloud Seedance 全模型与素材库实施计划

## 目标与范围

按 [FunCloud Seedance 全模型渠道、素材与计费架构设计](2026-08-11-FunCloud-Seedance全模型渠道素材与计费架构设计.md)
完成以下交付：

1. `seedance-2-funcloud`、`seedance-2-fast-funcloud`、`seedance-2-mini-funcloud`、
   `seedance-2-5-funcloud` 四个客户模型的精确渠道合同；
2. Standard/Fast/Mini 的 `funcloud_material` 虚拟素材库；
3. 基于 FunCloud 成功终态 `pointConsume` 的临时 Token 反推和冻结美元表达式结算；
4. 管理端协议配置、配对校验、回归测试和文档自检。

不在本计划内：真人素材、2.5 素材、单素材重命名/删除、Provider 回调、批量任务查询、真实 Provider
生产验收、真实账单确认以及修改 `docs/70-research`。

## 批次与依赖

### P0：冻结合同与工作树保护

- [x] 核对历史数据库、成功任务时间窗和历史 serializer，确认 Standard/Fast 使用富 `content`；
- [x] 确认四个 Provider 模型、创建路径、统一查询路径和客户模型名；
- [x] 确认 Mini 参照 Fast、2.5 参照专用文档；
- [x] 确认当前只发布虚拟素材；
- [x] 确认成功缺失可信 `pointConsume` 时进入对账态并保留预扣；
- [x] 识别现有未提交改动，后续只做 FunCloud 必需增量，不回滚用户工作。
- [x] 冻结清理规则：删除 FunCloud adapter revision v2 的解析器、判断和支持分支；
  管理协议改为无版本标识，Provider `/api/v2` 路径只留在代码路径注册中；
- [x] 冻结最小入侵规则：复用现有协议值和 Link 底座，新增逻辑优先独立文件，原生热路径只保留必要
  接线，不做无关重构。

完成信号：本文和对应 80-dev 设计存在，所有待实现项都有唯一合同。

### P1：精确模型和视频协议

- [x] 在 `relaykit/dto` 建立 FunCloud Provider 模型到创建路径的精确注册，删除模糊 `fast` 判断；
- [x] 新增四模型路径回归和未知模型失败测试；
- [x] 在 Seedance 模型清单加入四个客户模型；
- [x] 渠道保存时要求一个客户模型、一条精确 `model_mapping`；
- [x] 新增 FunCloud model spec，校验四模型 duration、resolution、媒体数量和不支持字段；
- [x] 保留 Provider 模型任务快照，FunCloud body 继续不发送 `model`；
- [x] 扩充富 `content` 序列化测试，覆盖显式 false/0、Mini、2.5 和 `asset://` Provider 引用。

完成信号：四模型路径确定，未知/错配模型无法保存或发送，不再存在名称包含判断和默认回退。

### P2：素材协议和流式上传

- [x] 注册管理协议 `funcloud_material` 和内部 profile `funcloud_material`；
- [x] 新增 FunCloud 素材 adapter：连接测试、组创建、组获取、组更新、虚拟素材上传、素材获取；空组
  删除同时校验本地无活动素材和 Provider 唯一组 `materialCount=0`；
- [x] 对列表 envelope 做严格解析和唯一 ID 匹配；
- [x] 为素材请求增加只在 FunCloud 使用的流式 source；其他 adapter 行为不变；
- [x] 使用 SSRF 拨号保护的客户端回源，校验 HTTPS/443、重定向、MIME、Content-Length 和 100MB
  实际读取上限；
- [x] 使用 pipe multipart，不落盘、不整块缓存、不持久化 source URL；
- [x] 单素材 rename/delete 和真人素材返回明确不支持；
- [x] Resolver 把 `asset://ast_*` 转换为 Provider `asset://...`；
- [x] 渠道保存和前端只允许 Standard/Fast/Mini 配置该素材协议，2.5 只允许 `none`。

完成信号：标准素材北向合同能创建虚拟组、流式上传、刷新和用于视频；未发布操作不会误报成功。

### P3：`pointConsume` 计量和计费

- [x] adapter 仅接受当前 v3；历史 v2 Task 失败关闭，不保留兼容查询分支；
- [x] 成功终态解析严格大于零的有限十进制 `pointConsume`；
- [x] 从冻结 Provider 模型、resolution、has_video_input 和版本化价格表反推 completion tokens；
- [x] 使用 decimal 定点和安全整数边界；
- [x] 把可信 Token 写入内部规范化 `usage`，让既有 tiered snapshot 结算；
- [x] 把原始 `pointConsume`、K、算法/价格表版本、使用档位和推导 Token 冻结到 Task 私有计费事实，
  并仅向管理员日志投影；
- [x] 缺失、非法、冲突、无适用价格或超界时返回合同违例，由轮询主链进入
  `RECONCILIATION_REQUIRED`；
- [x] 失败任务不反推 Token；
- [x] 为四个客户模型形成美元表达式和预扣 Token 配置清单，禁止复用 Standard/Fast 旧倍率。

完成信号：可信成功任务按冻结美元表达式差额结算；不可计量成功任务保留预扣并可由对账流程恢复。

#### P3 管理配置清单

四个客户模型必须先配置为 `tiered_expr`，否则请求在发送 Provider 前失败关闭。美元/百万 completion
tokens 表达式如下，`c` 是冻结的预扣或终态 completion tokens：

| 客户模型 | 表达式 | 预扣 Token 上界 |
| --- | --- | ---: |
| `seedance-2-funcloud` | `has_video ? (1080p ? c*4.55 : c*4.11) : (1080p ? c*7.48 : c*6.74)` | 730000 |
| `seedance-2-fast-funcloud` | `has_video ? c*3.23 : c*5.43` | 324000 |
| `seedance-2-mini-funcloud` | `has_video ? c*2.05 : c*3.37` | 324000 |
| `seedance-2-5-funcloud` | `has_video ? c*6.16 : c*10.26` | 648000 |

实际 `billing_setting.billing_expr` 必须用 `param("_task.has_video_input")`、
`param("_task.resolution")` 和 `tier(name, value)` 展开上述分支；不得改成按秒价格或旧 FunCloud
Standard/Fast 列表价。配置键为：

```json
{
  "billing_setting.billing_mode": {
    "seedance-2-funcloud": "tiered_expr",
    "seedance-2-fast-funcloud": "tiered_expr",
    "seedance-2-mini-funcloud": "tiered_expr",
    "seedance-2-5-funcloud": "tiered_expr"
  },
  "task_billing_setting.preconsume_tokens": {
    "seedance-2-funcloud": 730000,
    "seedance-2-fast-funcloud": 324000,
    "seedance-2-mini-funcloud": 324000,
    "seedance-2-5-funcloud": 648000
  }
}
```

### P4：管理端和 i18n

- [x] 增加素材协议类型、选择项、默认配对和校验；
- [x] FunCloud 视频协议默认选择 `funcloud_material`，同时允许管理员明确选择 `none`；
- [x] 2.5 的不支持素材约束由前端模型感知选项和表单校验即时拦截，后端精确映射继续兜底；
- [x] 增加所有支持语言的协议标签；
- [x] 更新表单和协议配对测试。

完成信号：管理员可以配置合法组合，非法协议组合在前后端均失败。

### P5：验证和复核

- [x] 运行 FunCloud/Seedance/素材/计费的窄边界 Go 测试；
- [x] 运行 `cd relaykit && GOWORK=off go test ./... && GOWORK=off go build ./...`；
- [x] 运行受影响根模块测试；
- [x] 运行前端协议测试、类型检查和生产构建；
- [x] 运行 `task docs:check` 和 `task ai:check`；
- [x] 检查 git diff，确认没有覆盖既有用户改动、没有敏感信息，本任务未改动受保护 research/archive；
- [x] 运行 FunCloud 专项引用审计，删除 adapter revision v2 兼容分支和已被美元 Token
  合同取代的旧按秒价格常量；统一改为无版本 `funcloud_seedance` / `funcloud_material`
  协议和 `third_party_funcloud_seedance` profile；
- [x] 逐个复核既有文件接线点，能由新增 FunCloud 专属文件承载的逻辑不扩写 NEWAPI 原生文件；
- [x] 把真实 Provider、真实账单和生产灰度保留为未完成门槛，不误写为已发布。

完成信号：所有可在本地验证的合同通过，剩余事项只依赖外部 Provider/账单/发布环境。

### 本地数据库落地

- [x] 审计到 21 条冻结 FunCloud adapter revision v2 的历史 Task；13 条 `SUCCESS`、8 条
  `FAILURE`，全部已 `settled`，不存在待轮询或待结算任务；
- [x] 修改前通过 SQLite 在 `one-api-pre-funcloud-four-channels-20260811.db` 建立可恢复备份；
- [x] 在本地 `one-api.db` 建立并启用 ID 61—64 四个 `ChannelTypeSeedanceLink` 渠道，
  每个渠道只包含一个客户模型和一条精确 `model_mapping`；
- [x] Standard/Fast/Mini 渠道配置 `funcloud_material`，2.5 渠道配置 `none`；
- [x] 写入四模型 `tiered_expr`、美元 Token 表达式和预扣 Token 上界；
- [x] 通过 `one-api-pre-funcloud-protocol-rename-20260811.db` 建立重命名前备份，然后迁移
  6 条渠道配置、21 条 Task 快照和 21 条 create attempt 快照；
- [x] 新增跨 SQLite/MySQL/PostgreSQL 的幂等启动迁移；运行时不接受旧协议标识；
- [x] 启动迁移在发现旧协议活动 Task、attempt 或 idempotency 时整体中止，不改写仍需履约的快照；
- [x] `PRAGMA integrity_check` 返回 `ok`；数据库核验过程未输出或记录渠道密钥。

## 验收矩阵

| 维度 | 验收 |
| --- | --- |
| 模型 | 四个精确映射和路径；未知模型失败 |
| 请求 | Standard/Fast/Mini 富 content；2.5 专用边界；显式零值保留 |
| 状态 | processing/success/completed/failed；未知和冲突进入对账 |
| 计量 | 有效 `pointConsume` 反推；缺失/非法/冲突失败关闭 |
| 素材 | 虚拟组、multipart 上传、列表刷新、asset resolver |
| 安全 | SSRF、DNS 重绑定、重定向、MIME、100MB、资源关闭、无 source URL 持久化 |
| 不支持 | 真人、2.5 素材、单素材 rename/delete 明确失败 |
| 配置 | 前后端协议枚举和配对一致；一个渠道一个精确模型 |
| 工程 | relaykit 独立构建；根模块和前端窄测试；docs 校验 |

## 发布门槛与剩余事项

代码和单元测试完成后仍不得直接宣布生产发布。每个客户模型至少还需要：真实创建/查询成功、素材上传
和 `asset://` 生成（适用模型）、FunCloud 后台 Token 对账、美元成本核对、失败与超时行为、结果 URL
生命周期以及小流量灰度。`K=6.82` 至少完成跨模型、输入类型、分辨率和账号样本后，才能从临时
计量升级为经验证事实。
