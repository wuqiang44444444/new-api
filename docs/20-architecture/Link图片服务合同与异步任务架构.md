---
status: current
owner: Dev Team
last-reviewed: 2026-08-11
---

# Link 图片服务合同与异步任务架构

## 1. 范围与状态

本文定义 Link 图片扩展在简化架构中的客户入口、Channel、converter、同步/异步响应和计费边界。
图片设计可以继续单独演进；本次只删除 publication、Link SKU、implementation 和 execution binding
依赖，并保留已经确认的通用图片任务协议。

NEWAPI 原生图片生成和编辑入口继续以上游实现为权威，不因 Link 图片设计被包装或收紧。

## 2. 简化履约链

```text
customer_model
  -> Token / Group / Ability / price
  -> NEWAPI 当前图片渠道选择
  -> model_mapping
  -> code-backed converter / upstream protocol
  -> Provider
```

客户模型用于模型发现、权限、价格、日志和响应；Provider 模型只用于上游调用。Link 图片不再要求
客户模型 publication、图片 SKU capability、`LinkImplementation` 或 execution binding。

本次不把 Seedance 的“单模型单渠道”“Priority/Weight 无效”和“一次 POST 不切换”自动复制到所有
图片模型。图片的渠道选择与重试语义必须由图片产品和具体 Provider 合同单独确认。

## 3. 客户合同

Link 图片可以复用 NEWAPI 图片创建入口，并在已明确接入的异步图片路径上提供任务查询。同步和异步
是同一客户响应联合合同：

- Provider 同步返回完整图片时，按标准图片响应投影；
- Provider 返回可轮询 task ID 时，创建共享 Task 并返回平台 task ID；
- Provider 返回既无完整图片也无可信 task ID 时，返回明确合同错误；
- 普通客户不看到 Provider task ID、Provider 模型、凭据或私有连接。

客户请求字段由统一图片 DTO 和具体 converter 代码校验。不支持字段必须明确拒绝，不得为了兼容某个
Provider 静默删除、钳制或改义。

## 4. Converter 与协议

图片 Provider 差异由代码 converter/protocol 表达，包括：

- 请求路径、鉴权和字段转换；
- 可选标量的 absent 与显式零值语义；
- Provider 模型和尺寸/质量翻译；
- 同步图片、异步 task 或错误响应识别；
- 轮询状态、结果和错误归一；
- 计费所需的受控维度提取。

管理员选择现有 converter/protocol，不编写 JSON 字段映射或状态脚本。完全兼容已有协议的新模型可以
通过客户模型、`model_mapping`、Channel 和价格配置上线；不兼容时由技术人员新增代码。

## 5. 通用图片任务协议标识

跨 Provider 的图片任务型适配使用共享协议标识：

```text
protocol = media-image-task/v1
```

该标识描述可观察的任务交换合同，不代表某个 Provider 品牌或 Link SKU。它至少约定：

- 创建响应如何提供可信 task ID；
- 查询请求和 Provider 状态如何归一；
- 成功结果如何提供可代理的图片内容；
- Provider 错误如何映射为平台错误；
- 哪些字段是计费和终态所需的受控事实。

## 6. 南向协议与共享生命周期边界

`relay/mediaimage` 是任务型图片协议的共享实现边界。Provider adapter 只负责协议转换，不建立第二套
Provider 专用 Task、计费状态机、轮询器或补偿表。

```mermaid
flowchart LR
    R["图片客户请求"] --> C["Channel + converter"]
    C --> M["relay/mediaimage"]
    M --> U["Provider task API"]
    U --> T["共享 Task / billing lifecycle"]
```

共享生命周期负责：

- 发送前需要时建立 durable create attempt；
- 取得可信 task ID 后创建 Task；
- 冻结 Channel、Provider 模型、protocol、adapter、连接和计费事实；
- 轮询、终态、结果代理、结算和补偿；
- 防止 Provider 响应或客户乘数造成负扣费和溢出。

Provider adapter 不得复制共享状态机，也不得用私有状态覆盖已经确认的 Task 事实。

## 7. 创建安全

只有明确接入持久化任务协议的图片 Provider POST 才使用 durable attempt。同步图片不因共享模型存在
而自动创建 attempt。

图片是否允许自动重试、是否提供客户幂等、创建结果不明时如何处置，必须由具体图片合同明确，不从
Seedance 视频规则反向推断。无可信 task ID 时不得创建伪 Task。

## 8. Task 与查询

异步图片 Task 冻结：

- user、token、app、客户模型和客户入口；
- Channel、Provider 模型、converter/protocol 和 adapter 版本；
- 查询连接、proxy 和受保护凭据引用；
- 计费上下文、预扣上界和资金来源；
- 最小结果代理信息。

查询和结算读取冻结快照，不因 Channel 编辑、模型映射或价格变化重新选渠或重算历史。单次不可采信
轮询不直接证明 Provider 任务失败。

## 9. 素材边界

当前图片入口是否接受 `asset://ast_*` 由图片产品和 converter 明确决定，不能从 Seedance 素材库能力
自动推导。未明确支持时返回参数不支持；不得把平台 Asset 自动下载、迁移或改写为图片输入。

## 10. 计费

客户价格和日志使用客户模型。所有图片数量、尺寸、质量和其它乘数必须先做上界校验，再使用统一
checked quota 转换。饱和事件进入管理员日志；预扣和结算都不能溢出为负数。

Provider 返回的最终扣量或媒体元数据同样是不可信输入，进入客户 quota 前必须验证或饱和转换。

## 11. 架构不变量

1. Link 图片不再依赖 publication、SKU、implementation 或 execution binding。
2. 客户模型、Channel、`model_mapping` 和代码 converter 构成直接履约链。
3. `media-image-task/v1` 使用共享 Task 与计费生命周期，不建立 Provider 专用第二套状态机。
4. 只有可信 task ID 才能创建异步 Task。
5. 不支持字段明确失败，不静默删除或改义。
6. 客户模型用于权限、价格和日志；Provider 模型仅用于上游调用。
7. Seedance 的唯一渠道和不切换规则不会未经评审自动扩张到图片。

## 12. 相关文档

- [Seedance 专用渠道与 Link 架构](Seedance专用渠道与Link架构.md)
- [异步任务与计费事实架构](异步任务与计费事实架构.md)
- [架构概览](架构概览.md)
- [ADR-0008](decisions/0008-共享异步任务计费状态机与原子补偿.md)
- [ADR-0016](decisions/0016-Seedance专用渠道与确定性素材代理.md)
