---
status: current
owner: Dev Team
last-reviewed: 2026-08-01
---

# Seedance 国内与 BytePlus 官方模型及价格调研

> 调研时间：2026-08-01（Asia/Shanghai）。
>
> 范围：中国大陆火山方舟与国际 BytePlus ModelArk API 路线及可公开核验的官方价格；不包含第三方中转价、即梦/Dreamina 消费端订阅价或平台自定义零售价。人民币与美元保持官方原币种，不按浮动汇率换算。

## Executive Summary

- **火山方舟当前公开的 Seedance 2.0 系列按量价格为 14–46 元/百万 Tokens。** 标准版、Fast 和 Mini 都按请求是否包含输入视频分档；含输入视频的 Token 单价更低，但会同时计入输入视频的 Token 量，不代表最终单条视频一定更便宜。
- **BytePlus 当前 Seedance 2.0 系列按量价格为 2.1–7.7 美元/百万 Tokens。** 国际版标准、Fast、Mini 的 Model ID 使用 `dreamina-seedance-*` 命名空间，与国内 `doubao-seedance-*` 不可混用；2.0 系列均不支持离线推理。
- **Seedance 1.5 Pro、1.0 Pro、1.0 Pro Fast 和 1.0 Lite 仍能在官方页面查到历史公开单价。** 它们不应与当前 2.0 系列主价格表混为同一个“当前可新开通清单”，实际可用性以账号所在地域的方舟控制台为准。
- **BytePlus 仍公开 Seedance 1.5 Pro、1.0 Pro 和 1.0 Pro Fast 的在线与离线价格。** 最低离线单价为 0.5 美元/百万 Tokens；这些价格适合核对国际区存量模型，不应套用到中国区模型。

## 1. 国内 Seedance 2.0 系列官方按量价格

火山方舟当前产品页在“视频生成 / 推理”下列出三个 2.0 系列模型。价格单位均为人民币元/百万 Tokens。

| 官方展示名 | 中国区常见 Model ID | 含输入视频 | 无输入视频 | 价格状态 |
| --- | --- | ---: | ---: | --- |
| Doubao-Seedance-2.0 | `doubao-seedance-2-0-260128` | ¥28 / 百万 Tokens | ¥46 / 百万 Tokens | 当前官方公开按量价 |
| Doubao-Seedance-2.0-fast | `doubao-seedance-2-0-fast-260128` | ¥22 / 百万 Tokens | ¥37 / 百万 Tokens | 当前官方公开按量价 |
| Doubao-Seedance-2.0-mini | `doubao-seedance-2-0-mini-260615` | ¥14 / 百万 Tokens | ¥23 / 百万 Tokens | 当前官方公开按量价 |

证据与口径：

- [火山方舟产品页](https://www.volcengine.com/product/ark) 和[豆包大模型产品页](https://www.volcengine.com/product/doubao) 均展示了上述三个模型及 28/46、22/37、14/23 元两档单价。
- 中国区官方 API 使用 `doubao-*` 命名空间；现存官方 [API Explorer](https://api.volcengine.com/api-explorer/?action=CreateContentsGenerationsTasks&groupName=%E8%A7%86%E9%A2%91%E7%94%9F%E6%88%90API&serviceCode=ark&version=2024-01-01) 可核验中国区 Model ID 与创建任务路径。
- Mini 的完整快照 Model ID 在公开资料中的曝光度低于标准版和 Fast；正式配置前仍应从当前账号的“模型列表/开通管理”复制 Model ID，不应只依赖本调研文档。

### 1.1 价格不能直接当作“每条视频价”

上表是 Token 单价，不是固定每次调用价。最终费用受输出时长、分辨率、帧率以及输入视频时长影响。请求含输入视频时，虽然命中的每百万 Token 单价较低，但输入视频也会增加总 Token 量。

## 2. Seedance 2.0 系列官方资源包

火山引擎 Seedance 2.0 活动页公开了标准版和 Mini 的 90 天资源包。这些是预付费 Token 包，不是包时或固定条数套餐。

| 适用模型 | 资源包 | 官方价 | Token 额度 | 折算基础单价 | 有效期 |
| --- | --- | ---: | ---: | ---: | ---: |
| Doubao-Seedance-2.0 | 轻量创作包 | ¥196 | 700 万 | ¥28 / 百万 Tokens | 90 天 |
| Doubao-Seedance-2.0 | 全能臻享包 | ¥280 | 1,000 万 | ¥28 / 百万 Tokens | 90 天 |
| Doubao-Seedance-2.0 | 高效量产包 | ¥364 | 1,300 万 | ¥28 / 百万 Tokens | 90 天 |
| Doubao-Seedance-2.0-mini | 轻享创作包 | ¥196 | 1,400 万 | ¥14 / 百万 Tokens | 90 天 |
| Doubao-Seedance-2.0-mini | 畅拍进阶包 | ¥280 | 2,000 万 | ¥14 / 百万 Tokens | 90 天 |
| Doubao-Seedance-2.0-mini | 高产爆量包 | ¥1,400 | 1 亿 | ¥14 / 百万 Tokens | 90 天 |

官方页同时说明：

- 标准版资源包仅适用 Doubao-Seedance-2.0，不含 Fast 和 Mini；
- Mini 资源包仅适用 Doubao-Seedance-2.0-mini；
- 不同分辨率和输入模式按实际 Token 单价比例抵扣；
- 资源包耗尽后转按量后付费。

来源：[火山引擎 Seedance 2.0 官方活动页](https://www.volcengine.com/activity/seedance2)。

## 3. 旧版 Seedance 模型的官方公开价格

下表用于核对存量接入和历史账单，不代表每个新账号仍能新建这些模型的接入点。

| 官方展示名 | 计费条件 | 官方公开单价 | 本文定位 |
| --- | --- | ---: | --- |
| Doubao-Seedance-1.5-pro | 无声视频 | ¥8 / 百万 Tokens | 旧版/存量价格口径 |
| Doubao-Seedance-1.5-pro | 有声视频 | ¥16 / 百万 Tokens | 旧版/存量价格口径 |
| Doubao-Seedance-1.0-pro | 视频生成 | ¥15 / 百万 Tokens | 旧版/存量价格口径 |
| Doubao-Seedance-1.0-pro-fast | 视频生成 | ¥4.2 / 百万 Tokens | 旧版/存量价格口径 |
| Doubao-Seedance-1.0-lite | 视频生成 | ¥10 / 百万 Tokens | 旧版；官方扣子计费页口径 |

来源与限制：

- [火山引擎豆包历史产品页入口](https://www.volcengine.com/product/doubao/) 公开展示 1.5 Pro、1.0 Pro 和 1.0 Pro Fast 的上述单价。
- [扣子官方“模型调用费用”文档](https://www.volcengine.com/docs/84458/1585097?lang=zh&redirect=1) 公开列出 1.0 Lite = 0.01 元/千 Tokens、1.0 Pro = 0.015 元/千 Tokens、1.5 Pro 无声 = 0.008 元/千 Tokens、有声 = 0.016 元/千 Tokens；上表统一换算为每百万 Tokens。
- 官方 API Explorer 中可核验 1.0 Pro 的 Model ID 示例 `doubao-seedance-1-0-pro-250528`。旧模型曾有多个快照和接入方式，存量配置应以原账号接入点与当前控制台为准。

## 4. BytePlus Seedance 官方按量价格

BytePlus ModelArk 国际站按美元计费。下表以官方价格文档 2026-07-31 更新版本为准，单位均为美元/百万 Tokens。

### 4.1 BytePlus Seedance 2.0 系列

| 官方模型 ID | 输出分辨率 | 无输入视频 | 含输入视频 | 离线推理 |
| --- | --- | ---: | ---: | --- |
| `dreamina-seedance-2-0-260128` | 480p / 720p | $7.0 | $4.3 | 不支持 |
| `dreamina-seedance-2-0-260128` | 1080p | $7.7 | $4.7 | 不支持 |
| `dreamina-seedance-2-0-260128` | 4K | $4.0 | $2.4 | 不支持 |
| `dreamina-seedance-2-0-fast-260128` | 480p / 720p | $5.6 | $3.3 | 不支持 |
| `dreamina-seedance-2-0-mini-260615` | 480p / 720p | $3.5 | $2.1 | 不支持 |

能力限制：标准版支持 480p、720p、1080p、4K；Fast 和 Mini 不支持 1080p、4K。国际区创建任务的官方接口为 `POST https://ark.ap-southeast.bytepluses.com/api/v3/contents/generations/tasks`。

来源：[BytePlus ModelArk 价格文档](https://docs.byteplus.com/en/docs/ModelArk/1544106?redirect=1)、[Seedance 2.0 系列教程与模型清单](https://docs.byteplus.com/en/docs/ModelArk/2291680)、[创建视频生成任务 API](https://docs.byteplus.com/en/docs/modelark/1520757)。

### 4.2 BytePlus 旧版 Seedance 模型

| 官方模型 ID | 计费条件 | 在线推理 | 离线推理 |
| --- | --- | ---: | ---: |
| `seedance-1-5-pro-251215` | 有声视频 | $2.4 | $1.2 |
| `seedance-1-5-pro-251215` | 无声视频 | $1.2 | $0.6 |
| `seedance-1-0-pro-250528` | 视频生成 | $2.5 | $1.25 |
| `seedance-1-0-pro-fast-251015` | 视频生成 | $1.0 | $0.5 |

来源：[BytePlus ModelArk 价格文档](https://docs.byteplus.com/en/docs/ModelArk/1544106?redirect=1)。旧版 Model ID 是否仍可新建接入点，应以目标 BytePlus 账号控制台为准。

### 4.3 BytePlus 计费公式与官方单条示例

BytePlus 仅对成功生成的视频收费；因内容审核等原因失败的生成任务不收费。官方估算公式为：

`预计费用 = Token 单价 × Token 消耗量`

`预计 Token 消耗量 =（输入视频时长 + 输出视频时长）× 输出宽度 × 输出高度 × 输出帧率 ÷ 1024`

实际结算应读取响应中的 `usage.completion_tokens`，不能仅依赖估算值。官方对“无输入视频、16:9、输出 5 秒”的示例价如下：

| 模型 | 480p | 720p | 1080p | 4K |
| --- | ---: | ---: | ---: | ---: |
| Seedance 2.0 Standard | $0.35 | $0.76 | $1.87 | $3.89 |
| Seedance 2.0 Fast | $0.28 | $0.60 | 不支持 | 不支持 |
| Seedance 2.0 Mini | $0.18 | $0.38 | 不支持 | 不支持 |

当输入视频为 2–15 秒、输出仍为 5 秒、16:9 时，官方示例给出的成本区间为：

| 模型 | 480p | 720p | 1080p | 4K |
| --- | ---: | ---: | ---: | ---: |
| Seedance 2.0 Standard | $0.39–$0.86 | $0.84–$1.86 | $2.06–$4.57 | $4.20–$9.33 |
| Seedance 2.0 Fast | $0.30–$0.66 | $0.64–$1.43 | 不支持 | 不支持 |
| Seedance 2.0 Mini | $0.19–$0.42 | $0.41–$0.91 | 不支持 | 不支持 |

上述单条价格只是特定时长、画幅和默认帧率下的官方示例，不是固定包条价。标准版和 Fast 在含输入视频时还可能应用官方规定的最低 Token 消耗量，正式计费仍以响应中的实际 Token 为准。

## 5. BytePlus Seedance 2.0 官方资源包

BytePlus 为三个 2.0 模型分别提供预付费在线推理 Token 包，官方标价如下：

| 适用模型 | Token 额度 | 官方价 | 折算包价 | 购买限制 |
| --- | ---: | ---: | ---: | --- |
| Dreamina Seedance 2.0 | 100 万 | $4.30 | $4.30 / 百万 Tokens | 最少购买 7 份 |
| Dreamina Seedance 2.0 | 1,000 万 | $43.00 | $4.30 / 百万 Tokens | — |
| Dreamina Seedance 2.0 | 1 亿 | $430.00 | $4.30 / 百万 Tokens | — |
| Dreamina Seedance 2.0 Fast | 100 万 | $3.30 | $3.30 / 百万 Tokens | 最少购买 9 份 |
| Dreamina Seedance 2.0 Fast | 1,000 万 | $33.00 | $3.30 / 百万 Tokens | — |
| Dreamina Seedance 2.0 Fast | 1 亿 | $330.00 | $3.30 / 百万 Tokens | — |
| Dreamina Seedance 2.0 Mini | 1,000 万 | $21.00 | $2.10 / 百万 Tokens | 最少购买 2 份 |
| Dreamina Seedance 2.0 Mini | 1,400 万 | $29.40 | $2.10 / 百万 Tokens | — |
| Dreamina Seedance 2.0 Mini | 1 亿 | $210.00 | $2.10 / 百万 Tokens | — |

资源包有效期为 90 天，只抵扣匹配模型的在线推理 Token，不可退款；资源包优先于按量余额抵扣，过期或耗尽后自动转按量计费。最终成交价以结算页面为准。

来源：[BytePlus ModelArk Seedance 2.0 资源包](https://docs.byteplus.com/en/docs/ModelArk/2191775)。

## 6. 未列入定价表的项目

### Seedance 2.5

截至本次核验，火山方舟对外公开的国内按量价格表仍以 Seedance 2.0、2.0 Fast 和 2.0 Mini 为主，未在同一官方价格页发现 Seedance 2.5 的可核验人民币 API 单价。第三方站点的“2.5”上架或积分价不能代替火山方舟官方价格，因此本文不猜测、不换算。

### 即梦/Dreamina 消费者订阅

即梦会员、活动积分、移动端免费额度和方舟 API Token 计费属于不同产品路线。本文不把消费者端积分换算为 API “官方单价”。

## 7. 采购与系统配置建议

1. 新接入优先按当前主价格表评估 2.0、2.0 Fast 和 2.0 Mini，再在目标账号内核对模型是否已开通、分辨率和 QPM 等权限。
2. 计费规则必须区分“含视频输入”和“无视频输入”，不能只为每个模型保存一个单价。
3. 预估单条成本时应根据实际分辨率、时长、帧率和输入视频计算 Token，不能用“每百万 Token 单价”直接作为每条费用。
4. 存量 1.x 模型要保留当时的模型快照 ID、地域、账号权限与计费规则；不要仅依据家族名称自动迁移。
5. 将地域、平台、Model ID、币种和计费档位作为一组配置保存。中国区 `doubao-*` 与 BytePlus `dreamina-*` / `seedance-*` 不能只按家族名映射，也不应在账单中自动按实时汇率混算。
6. BytePlus 结算优先读取 `usage.completion_tokens`；资源包还应监控模型匹配、剩余 Token 与 90 天到期日，避免过期后无感切回按量计费。

## 8. 资料来源与可信度

- [火山方舟产品页](https://www.volcengine.com/product/ark)：当前 Seedance 2.0 系列模型与按量单价。
- [豆包大模型产品页](https://www.volcengine.com/product/doubao)：当前 2.0 系列价格交叉核验。
- [豆包大模型历史产品页入口](https://www.volcengine.com/product/doubao/)：1.5 Pro、1.0 Pro、1.0 Pro Fast 的历史公开单价。
- [Seedance 2.0 官方活动页](https://www.volcengine.com/activity/seedance2)：标准版与 Mini 资源包、有效期与适用范围。
- [火山方舟创建视频任务 API Explorer](https://api.volcengine.com/api-explorer/?action=CreateContentsGenerationsTasks&groupName=%E8%A7%86%E9%A2%91%E7%94%9F%E6%88%90API&serviceCode=ark&version=2024-01-01)：中国区 API 路径与 Model ID 示例。
- [扣子官方模型调用费用](https://www.volcengine.com/docs/84458/1585097?lang=zh&redirect=1)：1.0 Lite、1.0 Pro、1.5 Pro 的 Token 单价交叉核验。
- [BytePlus ModelArk 价格文档](https://docs.byteplus.com/en/docs/ModelArk/1544106?redirect=1)：国际区 2.0 与 1.x 模型的按量、在线/离线单价、计费公式和官方示例。
- [BytePlus Seedance 2.0 系列教程](https://docs.byteplus.com/en/docs/ModelArk/2291680)：国际区当前三个 2.0 Model ID。
- [BytePlus Seedance 2.0 资源包](https://docs.byteplus.com/en/docs/ModelArk/2191775)：美元资源包价格、最低份数、有效期与抵扣规则。
- [BytePlus 创建视频生成任务 API](https://docs.byteplus.com/en/docs/modelark/1520757)：国际区 API 域名与任务接口。

## 9. 维护规则

模型上架状态、活动包和价格都可能变更。任何用于正式报价、采购或计费配置的更新，都应重新打开火山方舟或 BytePlus 官方价格页、资源包页和目标账号控制台进行核对，并更新本文 `last-reviewed`。
