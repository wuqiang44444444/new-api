---
status: reference
owner: Dev Team
last-reviewed: 2026-09-24
retrieved-at: 2026-09-24T06:14:27+00:00
---

# MiniMax H3 京东云价格核查

## 结论与适用边界

截至本次查询，尚未取得可用于 `modelservice.jdcloud.com` MiniMax-H3 渠道的京东人民币单价。
本节京东材料只能证明公开报价的查询状态，不能证明 H3 免费，也不能作为京东成本价格配置依据。
京东报价核查阶段未写入价格；后续按用户明确指示采用 MiniMax 中国官网刊例价配置本地测试模型，见下文。

MiniMax 自营价、京东灵境会员灵感值和 JoyBuilder API 账户协议价属于不同计价来源，不能直接互换。
灵境价格接口即使恢复，也需确认其报价适用于当前 JoyBuilder API 账户。

## 官方来源及核查结果

| 官方来源 | 查询结果 | 可采信范围 |
| --- | --- | --- |
| [JoyBuilder 模型服务价格及计费规则](https://docs.jdcloud.com/cn/jdaip/maas-billing-rules) | 提取页面内嵌 Markdown 正文，未发现 MiniMax-H3 条目；其中 MiniMax-M2.7、MiniMax-M3 是文本模型 | 当前公开总表未提供 H3 单价；不能使用文本模型价格 |
| [京东海螺文生视频 API](https://docs.jdcloud.com/cn/jdaip/hailuo-text-to-video) | 示例模型为 MiniMax-Hailuo-2.3 | 不能将该示例视为 H3 价格合同 |
| [灵境产品定价](https://docs.jdcloud.com/cn/lingjing/ProductPricing) | 指向用户控制台及售前确认具体定价；区分会员、灵感值与 API 服务 | 不能用会员兑换比例推算当前 API 渠道成本 |
| [灵境 API 服务平台](https://open-lingjing.jdcloud.com/) | 官方公开目录包含 H3 文生、图生、参考生视频；对应详情的 `price` 均为 `null` | 已登记 H3，静态详情不提供单价 |

JoyBuilder 计费正文的 UTF-8 SHA-256：
`665c28989c8b5095d8e4844344085b025fbad8e1f1b91350990ced0c78dd9cac`。
摘要针对提取后的正文，不包括网页导航、脚本或本文件内容。

### 灵境官方报价查询

官方前端公开目录及模型详情接口：

- `GET https://open-lingjing.jdcloud.com/bizApi/modelmarket-console/v1/console/api-console/vendors/minimax/catalog`
- `GET https://open-lingjing.jdcloud.com/bizApi/modelmarket-console/v1/console/api-console/models/467`（文生）
- 同路径模型 `468`（图生）、`469`（参考生）。

三项详情均登记 `enablePriceQuery=true`、`priceQueryService="minimax-h3"`。
根据官方前端调用形状，对其只读报价服务发送以下参数，分别查询 `768P` 与 `2K`：

```http
POST https://lingjing.jdcloud.com/joycreator/AIModelApiConsole/calculatePrice
Content-Type: application/json
```

```json
{
  "enablePriceQuery": true,
  "priceQueryService": "minimax-h3",
  "params": {
    "shortVender": "minimax",
    "shortSenceCode": "t2v",
    "model_name": "h3",
    "duration": "6",
    "resolution": "768P"
  }
}
```

两次 HTTP 均为 200，但业务错误码均为 `500`，错误为“计费服务网络异常”，
未返回报价。本次为匿名公开报价查询，没有提交视频生成；该结果不证明登录账户查询也必然失败。
本文件仅保留脱敏结果摘要，不保存请求标识或原始响应。

## 系统汇率与后续入库口径

本地 `one-api.db` 的 `options.USDExchangeRate` 为 **6.76 CNY/USD**。
确认京东单价后，人民币请求金额应除以系统美元汇率，使用既有表达式
`usd_exchange_rate()` 在预扣时冻结汇率，结算继续使用同一快照。
不能将人民币价格直接写成美元价格，也不能将 `usage.video_output` 的 credit 值当作金额。

待补充的价格事实：768P 与 2K 的输出单价及计价单位；参考图片、视频、音频是否收费及其规则；
账户折扣、活动价、最小收费时长与取整规则。未确认前不生成代表京东实际成本的价格表达式。

价格确认后的验收需通过标准北向入口实际生成，逐项比较资金 hold、Task 最终结算、消费日志、
用户及令牌额度变化与独立计算结果。网关计费相符与京东账单对账应分别给出结论；
仅有生成成功或用量 credit 不足以证明供应商实际人民币扣费。


## 采用 MiniMax 官方刊例价的本地配置

用户已明确要求直接使用 H3 官方价格。2026-09-24 查询
[MiniMax 中国开放平台按量计费](https://platform.minimax.cn/docs/guides/pricing-paygo)，
本次采用中国站人民币刊例价，按系统汇率折算美元；这是客户侧测试定价依据，不表示已确认京东账户成本。

| H3 计费项 | 中国官网刊例价 | 按 6.76 CNY/USD 换算 |
| --- | --- | --- |
| 768P 输出 | ¥0.50/秒 | $0.073964497041/秒 |
| 2K 输出 | ¥0.80/秒 | $0.118343195266/秒 |
| 输入图片 | 前 5 张免费，超出部分 ¥0.20/张 | 超出部分 $0.029585798817/张 |
| 输入视频 | 按输入时长、生成分辨率，768P ¥0.50/秒、2K ¥0.80/秒 | 对应输出单价 |
| 输入音频 | 免费 | $0 |

[MiniMax 国际站](https://platform.minimax.io/docs/guides/pricing-paygo)另列美元价格，
属于不同地区刊例价；本次没有将国际站单价与人民币折算价混用。
再生成、H3-Max 和 Context-IR 不属于当前 H3 生成接口的配置范围。

### 本次已入库的输出定价

本地测试客户模型 `minimax-h3-jd` 使用独立 `minimax-price-test` 分组，分组倍率 1。
渠道为 MiniMax Link 专用类型 64，沿用已提供的京东连接及凭据，精确映射至 `MiniMax-H3`，
制品版本为 `minimax-link@1.2.0`；原有原生类型 35 渠道未改型。
价格通过数据库事务写入既有 options 表，并复用渠道唯一性、模型价格及表达式校验。

```text
billing_setting.billing_mode["minimax-h3-jd"] = "tiered_expr"

u("resolution") == "2k"
  ? tier("2K-output", u("duration_seconds") * 0.80 / usd_exchange_rate())
  : tier("768P-output", u("duration_seconds") * 0.50 / usd_exchange_rate())
```

表达式输出 USD/请求；内部 quota 为该结果乘以 500000 和分组倍率，最后按系统规则四舍五入。
6 秒 768P 的预期费用为 ¥3.00、221893 quota；6 秒 2K 为 ¥4.80、355030 quota。
汇率通过既有 `usd_exchange_rate()` 冻结，不硬编码进表达式。

**范围限制：此表达式仅配置输出费用。** 当前宿主计费 schema 没有图片数量或参考视频时长字段，
不能通过直接数据库配置完整计算这两项官方输入费用。因此测试限制在文生视频，独立测试分组不作为
多模态完整价格发布；未假装参考视频或超过 5 张图片免费，也未用 credit 推算输入秒数。
真实网关预扣、结算与额度变化的验收结果如下。

### 真实渠道金额验收

2026-09-24 在本地运行中的网关通过标准 ModelArk V3 入口提交两条文生视频请求，
渠道 138 实际调用京东；按既有 15 分钟查询节奏获得终态，两条均 `SUCCESS / settled`。

| 请求 | 官网人民币金额 | 未取整美元金额 | 预扣 quota | 最终 quota | 结算差额 |
| --- | --- | --- | --- | --- | --- |
| 768P，6 秒 | ¥3.00 | $0.443786982249 | 221893 | 221893 | 0 |
| 2K，6 秒 | ¥4.80 | $0.710059171598 | 355030 | 355030 | 0 |

- 每条 Task 的汇率快照均为 6.76，分组倍率均为 1，quota/USD 均为 500000。
- 两条创建 attempt 均完成，hold 原子转移至 Task；每条受理请求只提交一次。
- 专用测试用户与令牌初始额度均为 2000000，最终剩余均为 1423077，累计使用均为 576923。
- 消费日志为两条预扣及两条零差额结算，合计 576923 quota；成功后再次 GET，额度和日志数量不变。
- 内部 quota 取整后的实扣为 $0.443786 与 $0.710060；换回人民币分别为
  ¥2.99999336 与 ¥4.80000560，按人民币分显示即 ¥3.00 与 ¥4.80。
- 最初两次带 `parameters` 包裹的测试请求被当前北向合同明确拒绝，创建 attempt 数为 0；
  改用当前 API 文档的顶层 `duration/resolution/ratio` 后受理，拒绝请求未产生预扣或上游创建。
- 验收后停用临时测试用户及令牌，保留账务事实、模型价格和独立测试分组渠道；
  未向 NEWAPI 原生 Ability 写入该渠道，原有渠道 137 保持类型 35、停用状态。

结论：**上述两条文生请求的网关客户计费准确**，金额、汇率冻结、预扣、结算、日志及余额相符。
本次未取得京东账户账单，不构成京东供应商成本对账，也不覆盖参考素材附加费用。
MiniMax、任务计费与汇率相关后端回归测试，以及 `task docs:check`、`task ai:check` 均通过。
