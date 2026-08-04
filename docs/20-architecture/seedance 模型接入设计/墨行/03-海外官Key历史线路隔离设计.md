---
status: accepted
owner: Dev Team
last-reviewed: 2026-08-05
---

# 墨行海外官 Key 历史线路隔离设计

## 1. 定位

`dreamina-seedance-2-0-260128` 使用历史 Ark 官 Key 线路：

```text
POST /v1/ark/media/generations
GET  <待重新取证的 Ark 任务查询路径>
asset://<Ark asset ID>
```

该模型名来自墨行官方历史研究资料。当前生产代码只在端到端测试 fixture 中出现该字符串，没有独立
注册对应 capability、implementation、execution binding 或价格；下文描述的是历史资料合同和未来
重新取证边界，不是当前代码已经可履约的事实。

墨行官方资料已明确标记该线路为历史资料，并说明它不适用于当前
`doubao-seedance-2-0-260128` TokenSave V2 relay。它也不能被当作当前
`seedance-2-0-oversea` V2 `/v1/media/*` 的等价实现。

## 2. 历史模型规格

| 维度 | 官方历史资料 |
| --- | --- |
| Provider 模型 | `dreamina-seedance-2-0-260128` |
| 创建协议 | Ark 兼容 `content[]` 请求 |
| 时长 | 4–15 秒或 `-1` |
| 分辨率 | `480p`、`720p`、`1080p`、`4k` |
| 比例 | `16:9`、`4:3`、`1:1`、`3:4`、`9:16`、`21:9`、`adaptive` |
| 媒体 | 图片、视频、音频及首帧/尾帧/多模态参考角色 |
| 生成音频 | `generate_audio` |
| 水印 | `watermark` |
| 状态 | `queued/running/succeeded/failed/expired/cancelled` |
| 内容保留 | 文档称结果下载通常约 48 小时、任务约 7 天 |
| 素材 | `/v1/ark/assets/*` 与 H5 真人认证 |

该表只用于隔离和未来取证，不构成当前 Link capability。营销场景、价格或历史示例不能使模型自动
获得 publication。

## 3. 当前处理规则

1. 不为该历史模型自动创建客户模型 publication 或 Ability。
2. 不把 `dreamina-seedance-2-0-260128` 映射到 `seedance-2-0-oversea` Link SKU。
3. 不让 `moxing.seedance-ark-assets/v1` 以 `seedance-2-0-oversea` Provider 模型进入候选。
4. 历史 Ark AssetBinding 只在其冻结的模型、publication、implementation、凭据和作用域内解释。
5. 当前 V2 relay Asset 不迁移或重写为 Ark asset，反向也一样。
6. 旧文档中的 Provider `group-*`、`asset-*` 不进入客户 API；当前客户仍只使用平台 `ast_*`。

## 4. 重新接入条件

只有项目负责人明确批准重新发布后，才能为历史线路新增独立 Link SKU 与 implementation。重新接入
至少需要：

- 使用目标生产域名和目标 Key 重新验证模型仍可调用；
- 取得明确的创建、查询、取消、删除和内容下载路径；
- 冻结请求 DTO、全部媒体角色、数量/格式/大小、默认值和错误外壳；
- 证明 Ark 素材、H5 真人认证、视频模型和凭据作用域属于同一 Provider 账号边界；
- 验证任务/结果保留时长、失效状态和下载鉴权；
- 验证 token usage、价格、失败扣费和单次账单查询的可关联性；
- 注册新的 capability、implementation version、execution binding、adapter/hash 和 exposure 策略；
- 对当前数据库的 publication、Task、attempt、AssetBinding 和 exposure 做只读审计。

重新接入时的候选 SKU 必须与 V2 SKU 分离，因为它们的分辨率、媒体、素材、生命周期和 Provider
模型均不同。不得为了复用现有客户模型而扩大当前 SKU。

## 5. 历史任务与素材

若部署中已经存在历史 Ark Task 或 AssetBinding：

- 查询和清理只能使用创建时冻结的 Ark adapter、Base URL、单 Key 和 implementation；
- 当前渠道配置、当前 V2 Key 或新 publication 不能改写历史任务；
- 找不到精确 adapter/version 时 fail closed，不能用 V2 查询路径或当前 Key 猜测；
- 真人授权撤回继续优先于技术引用可用性，内容代理每次回源前复查授权；
- 若实现版本已无法执行，进入显式人工迁移/清理流程，不建设无限期 fallback。

## 6. 不变量

1. 历史 Ark 与当前 V2 是不同模型合同，不是两条等价网络线路。
2. 历史模型不因代码中存在 reverse-proxy profile 而获得发布资格。
3. `ark_assets` 只与已验证的 Ark 视频实现和同一账号作用域协作。
4. `doubao-seedance-2-0-260128` 不属于墨行 Ark implementation。
5. 重新接入必须新增或提升显式合同身份，不允许配置式解锁。
