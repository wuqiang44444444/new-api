---
status: accepted
owner: Dev Team
last-reviewed: 2026-08-18
---

# BytePlus 官方供应商原始技术文档整理

## 来源与范围

资料来自共享官方研究与 BytePlus ModelArk 文档链接；代码以 `modelark_v3_byteplus` 和官方 Action adapter 核对。当前资料未在仓库保留完整原始 OpenAPI 快照，因此本文件明确区分“已从资料核对”和“待账号验证”。

## 视频 API 原始事实

| 项目 | 资料/代码事实 |
| --- | --- |
| Endpoint | `https://ark.<region>.bytepluses.com/api/v3/contents/generations/tasks`（资料示例为 ap-southeast） |
| 模型 ID | `dreamina-seedance-2-5-260628`、`dreamina-seedance-2-0-260128`、Fast、Mini |
| 计费 | USD/百万 Token，按输出规格和是否含视频输入分档 |
| 成功用量 | 官方资料建议读取 `usage.completion_tokens` |
| 分辨率 | Standard 480/720/1080p/4K；Fast/Mini 480/720p；2.5 480/720p |

## 素材 Action 原始事实

官方素材与火山使用同一 Action 形状：`Action`、`Version=2024-01-01`、Region、ProjectName，AK/SK 签名；代码 endpoint 为 `https://ark.<region>.byteplusapi.com`。操作包括 Asset/AssetGroup CRUD、列表和真人认证结果查询。

## 资料限制

BytePlus 文档中的地域、Model ID、价格和资源包会变化；海外在线/离线价格不可套用中国区，也不能把资源包当固定条数。当前未取得目标账号的实际开通、失败扣费和素材复用证据前，不宣称生产发布。

