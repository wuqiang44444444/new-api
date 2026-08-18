---
status: accepted
owner: Dev Team
last-reviewed: 2026-08-18
---

# FunCloud 供应商原始技术文档整理

## 资料范围

来源为 `docs/70-research/funcloud/海外/seedance2.0.md`、`seedance2.0 fast.md`、`seedance2.5.md`、`seedance 素材库.md` 和 `价格.md`。资料中存在营销描述、多个版本参数和国内专线示例；下文把“资料事实”和“平台已验证合同”分开。

## 视频 API 摘要

| 项目 | 资料事实 |
| --- | --- |
| Base URL | `https://mm-internal-cn.leonecloud.com`（资料也出现私有/IP 示例） |
| 鉴权 | `Authorization: Bearer <token>` |
| 创建 | `/api/v2/open/aigc/seedance2-0`、`-fast`、`-mini`、`/seedance2-5` |
| 查询 | `/api/v2/open/aigc/{task_id}`；部分资料还列批量查询和 callback |
| 请求 | `content[]`（text/image/video/audio）、`resolution`、`ratio`、`duration`、`generateAudio`、`watermark` |
| 状态 | `processing` / `success` / `failed`，成功响应含视频 URL 与 `completionTokens` |

Standard/Fast/Mini 资料称支持多模态参考；2.5 资料称支持 4–30 秒、图片≤30、视频≤10、音频≤10、总计≤50，并支持 `asset://`。这些资料能力只有在代码合同中列出后才对外可用。

## 素材 API 摘要

资料列出真人认证、虚拟素材组和素材上传：`/api/v2/open/material/person/validate/session`、`/material/group/create`、`/material/virtual/upload`、`/material/list`，返回 `asset://<assetId>`。资料还描述素材组更新、删除和真人模式，但平台当前只发布代码已验证的虚拟素材组、空组删除和单素材查询；不把资料中的列表接口变成平台列表 API。

## 价格资料

价格资料以 USD/百万 Token 发布：Standard 6.74/4.11（无视频/含视频，1080p 为 7.48/4.55）、Fast 5.43/3.23、Mini 3.37/2.05、2.5 10.26/6.16。括号内折扣、人民币换算和按秒参考价只作商务比较。

## 资料风险

资料中出现 `realPersonMode`、callback、`480pto720p`、编辑/延长等扩展字段，但当前平台不发布这些字段。Provider 成功响应的 `completionTokens` 是可形成客户实际用量的候选证据；`pointConsume` 仅作为 Provider 成本证据，不能反推客户费用。

