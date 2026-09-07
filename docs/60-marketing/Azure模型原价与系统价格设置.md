---
status: current
owner: Dev Team
last-reviewed: 2026-09-07
---
# Azure 模型原价与系统价格设置

## 适用范围与价格口径

本文记录 GPT-5.6 Sol、GPT-5.6 Terra、GPT-5.6 Luna 和 GPT-6 Astra 的 Azure Standard Global
（全球标准按量计费）非促销价格，以及 New API `v1.0.0-rc.23` 可用的模型计费表达式。

- 价格核对日期：2026-09-07；所有单价单位均为 **美元 / 100 万 Token**。
- “原价”指核对日的非限时促销挂牌价，不指模型最初发布时的历史价格。
- GPT-5.6 Sol 使用输入 $5、输出 $30 的非促销短上下文价，不采用输入 $4、输出 $20 的限时促销价。
- 不包含 Data Zone、Priority Processing、Batch、PTU、工具调用、税费或企业合约折扣。
- 表达式是平台向客户收费的规则；不能据此推断某个渠道的实际采购成本或 Azure 账单。
- 本文只整理配置，不代表已修改线上设置，也不代表已完成真实缓存写入、长上下文和 Azure 账单对账。

Azure 对 GPT-5.6 和 GPT-6 的标准按量计费规定：输入不超过 **272,000 Token** 使用短上下文档；
超过时，**整个请求**的输入、缓存和输出均使用长上下文档，不是仅对超出部分加价。

## 完整原价表

| 模型              | 输入上下文长度 | 普通输入 | 缓存读取 | 缓存写入 | 输出 |
| ----------------- | -------------- | -------: | -------: | -------: | ---: |
| `gpt-5.6-sol`   | ≤ 272,000     |        5 |     0.50 |     6.25 |   30 |
| `gpt-5.6-sol`   | > 272,000      |       10 |        1 |    12.50 |   45 |
| `gpt-5.6-terra` | ≤ 272,000     |        2 |     0.20 |     2.50 |   12 |
| `gpt-5.6-terra` | > 272,000      |        4 |     0.40 |        5 |   18 |
| `gpt-5.6-luna`  | ≤ 272,000     |     0.20 |     0.02 |     0.25 | 1.20 |
| `gpt-5.6-luna`  | > 272,000      |     0.40 |     0.04 |     0.50 | 1.80 |
| `gpt-6-astra`   | ≤ 272,000     |       10 |        1 |    12.50 |   50 |
| `gpt-6-astra`   | > 272,000      |       20 |        2 |       25 |   75 |

GPT-5.6 数据来自 [Azure 官方动态价格表](https://azure.microsoft.com/en-us/pricing/details/azure-openai/)
的 GPT-5.6 Series、Global、Pricing 列；GPT-6 数据来自
[Azure GPT-6 Astra 发布文章](https://azure.microsoft.com/en-us/blog/gpt-6-astra-frontier-intelligence-for-work-now-generally-available-in-microsoft-foundry/)
的 Standard Global 表。长上下文阈值见
[Azure 模型说明](https://learn.microsoft.com/zh-cn/azure/foundry/foundry-models/concepts/models-sold-directly-by-azure)。

## 系统设置填写方式

1. 在系统设置的模型价格编辑界面中，找到对应的客户模型名称。
2. 将该模型的计费模式设为表达式计费（`tiered_expr`）。
3. 切换到“表达式编辑器”，填入下方对应表达式，保存模型价格设置。
4. 如果要求客户最终按本文原价计费，将对应分组倍率设为 `1`，不附加折扣或请求倍率。
5. 检查平台实际开放的别名、日期后缀和推理力度后缀名称，逐个配置应使用的表达式。

这是**系统级模型价格配置**：相同客户模型名可以在多个渠道共享价格，不需要为每个渠道重复填写。
rc23 的计费模式和表达式按模型名精确查找，不把 `gpt-5.6-sol` 的设置自动扩展为任意前缀匹配。
自定义模型名也应以平台实际用于计费的客户模型名配置。

不要把下方表达式填到“ChatCompletions → 响应兼容”策略 JSON、“模型固定价格”或“渠道参数覆盖”里；
协议转换和模型收费是不同设置。

### GPT-5.6 Sol

```text
len <= 272000 ? tier("standard", p * 5 + cr * 0.5 + cc * 6.25 + c * 30) : tier("long_context", p * 10 + cr * 1 + cc * 12.5 + c * 45)
```

### GPT-5.6 Terra

```text
len <= 272000 ? tier("standard", p * 2 + cr * 0.2 + cc * 2.5 + c * 12) : tier("long_context", p * 4 + cr * 0.4 + cc * 5 + c * 18)
```

### GPT-5.6 Luna

```text
len <= 272000 ? tier("standard", p * 0.2 + cr * 0.02 + cc * 0.25 + c * 1.2) : tier("long_context", p * 0.4 + cr * 0.04 + cc * 0.5 + c * 1.8)
```

### GPT-6 Astra

```text
len <= 272000 ? tier("standard", p * 10 + cr * 1 + cc * 12.5 + c * 50) : tier("long_context", p * 20 + cr * 2 + cc * 25 + c * 75)
```

## 表达式变量与倍率

| 变量或设置    | rc23 含义与填写要求                                                                  |
| ------------- | ------------------------------------------------------------------------------------ |
| `len`       | 完整输入上下文长度。阶梯判断用它，不使用扣除缓存后的`p`。                          |
| `p`         | 普通输入计价量。OpenAI 用量语义下，表达式引用的`cr`、`cc` 会自动从总输入中扣除。 |
| `cr`        | 缓存读取 Token，按缓存读取单价收费。                                                 |
| `cc`        | 缓存写入 Token，依赖上游实际返回且被 rc23 识别的缓存写入用量。                       |
| `c`         | 输出 Token，包含上游计入输出总量的推理 Token，无需再单独加收推理 Token。             |
| `tier(...)` | 标记命中档位并返回该档计算结果；档位名称不影响单价。                                 |
| 单价系数      | 直接填写美元 / 百万 Token，不除以 2，不再在表达式中除以 1,000,000。                  |
| 分组倍率      | 结算时仍会乘入；要按本文原价收费，应为`1`。                                        |

表达式返回值由系统统一除以 1,000,000，再转换为平台 quota 并乘以分组倍率。
不要自行写成 `(p - cr - cc) * 单价`，否则缓存会重复扣除。图片输入未使用 `img` 单独计价时保留在
普通输入中；上述表达式不额外增加图片 Token 费用。

`cc` 不能用 `p - cr` 猜测。上游未返回或转换链路未保留缓存写入用量时，仅填写写入单价无法补齐数据，
应先核对实际 usage 和账单，不能把未命中的输入全部视作收费缓存写入。

## 为什么不能使用 `p < 272000`

以下表达式只用作错误对照，**不要保存为本文原价配置**：

```text
p < 272000 ? tier("tier_1", p * 4 + c * 20 + cr * 0.4 + cc * 5) : tier("tier_2", p * 8 + c * 30 + cr * 0.8 + cc * 10)
```

它与本文 Sol 原价表达式有三个区别：

1. 价格系数不同：输入和输出使用较低的促销价口径，不符合本次非促销原价要求。
2. `p` 会扣除单独计价的缓存，缓存命中较多时会把长上下文误判为短上下文。
3. `<` 会让恰好 272,000、且没有缓存的输入提前进入长档；正确分界为 `len <= 272000`。

例如完整输入 300,000、其中缓存读取 250,000 时，`len = 300000`，`p = 50000`。
正确表达式命中长档，对照表达式误命中短档。

## 同用量金额核对

以下金额为美元，分组倍率为 `1`、缓存写入为 `0`，不包含工具费等额外费用。
完整输入列已经包含缓存读取，计算普通输入时只扣一次缓存读取。

| 完整输入 | 其中缓存读取 |   输出 | Sol 原价正确表达式 | 上述错误对照表达式 | 对比                               |
| -------: | -----------: | -----: | -----------------: | -----------------: | ---------------------------------- |
|  100,000 |            0 | 10,000 |               0.80 |               0.60 | 对照便宜 25%                       |
|  300,000 |            0 | 10,000 |               3.45 |               2.70 | 对照便宜约 21.7%                   |
|  300,000 |      250,000 | 10,000 |               1.20 |               0.50 | 对照便宜约 58.3%，包含档位误判     |
|  272,000 |            0 | 10,000 |               1.66 |              2.476 | 对照反而贵约 49.2%，源于临界点误判 |

在两式命中相同档位时，对照式的输入、缓存单价低 20%，输出单价低约 33.3%；
总价差取决于 Token 构成，不能统一表述成某个固定折扣。

保存后建议在价格编辑器的估算功能中核对：零用量、272,000 / 272,001 临界点、长输入且高缓存命中、
缓存写入非零等情况。实际消费记录仍需核对表达式计费模式、命中档位、用量和最终分组倍率。

## 官方来源与版本依据

- [Azure OpenAI 官方价格表](https://azure.microsoft.com/en-us/pricing/details/azure-openai/)：
  GPT-5.6 Series 的全球标准非促销挂牌价、缓存与长上下文档位。
- [Azure GPT-5.6 官方发布说明](https://azure.microsoft.com/en-us/blog/gpt-5-6-now-available-in-microsoft-foundry/)：
  Sol 原价与限时促销价的区别；促销不应用于本文表达式。
- [Azure GPT-6 Astra 官方发布说明](https://azure.microsoft.com/en-us/blog/gpt-6-astra-frontier-intelligence-for-work-now-generally-available-in-microsoft-foundry/)：
  Astra 全球标准长短上下文价格。
- [Azure 模型与长上下文说明](https://learn.microsoft.com/zh-cn/azure/foundry/foundry-models/concepts/models-sold-directly-by-azure)：
  GPT-5.6、GPT-6 的 272,000 输入阈值与整请求分档规则。
- [rc23 表达式规范](https://github.com/QuantumNous/new-api/blob/v1.0.0-rc.23/pkg/billingexpr/expr.md)：
  变量、价格单位、阶梯和归一化语义。
- [rc23 计费配置](https://github.com/QuantumNous/new-api/blob/v1.0.0-rc.23/setting/billing_setting/tiered_billing.go)：
  配置键为 `billing_setting.billing_mode` 与 `billing_setting.billing_expr`，按模型名精确查找。
- [rc23 用量归一化](https://github.com/QuantumNous/new-api/blob/v1.0.0-rc.23/service/tiered_settle.go)：
  `p`、`len`、缓存读取和缓存写入的处理。

核对的 rc23 上游提交为 `0ab02020603d22e5613bc4cf46bfab06f8567769`。
本地工作区版本更高，不能把本地新增字段或新语义直接当作线上 rc23 的可用能力。
