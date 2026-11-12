---
status: current
owner: Dev Team
last-reviewed: 2026-11-12
---

# Nexhina Seedance 接口资料

Nexhina 文档中心「火山 Seedance」专栏，采集于 2026-11-12（Asia/Shanghai）。下文按公开文档整理，示例模型和能力需与账户实测区分。

| 接口 | Nexhina 请求 | 文档所示合同 |
| --- | --- | --- |
| 创建视频 | `POST /seedance/api/v3/contents/generations/tasks` | Bearer、JSON，ModelArk V3 形状，使用 `model` 和 `content[]`。示例包含 `resolution`、`ratio`、`duration`、`seed`、`camera_fixed`、`watermark`，创建返回顶层 `id`。 |
| 查询视频 | `GET /seedance/api/v3/contents/generations/tasks/{id}` | Bearer。示例返回顶层 `id`、`model`、`status`、字符串形式的 `created_at` / `updated_at`、`content.video_url` 及整数 `usage.completion_tokens` / `usage.total_tokens`。文档列出 HTTP 401、404。 |
| 素材与真人验证 | `POST /api/ark/v1/?Action=<Action>&Version=2024-01-01` | Bearer、JSON。结果采用 `ResponseMetadata` 与 `Result` 信封，错误示例含 `ResponseMetadata.Error`。 |

所有接口示例均在 `https://token.nexhina.cn`。专用创建示例使用 `doubao-seedance-1-5-pro-251215`。通用快速入门另列 `seedance-2.0`、`seedance-2.0-fast`、`seedance-2.5`。应分别确认账户模型列表和专用路径对实际模型 ID 的接受情况。

素材组文档提供创建、查询、列表、更新、删除。普通素材提供创建、查询、列表、更新、删除。创建组示例字段为 `Name`、`Description`、`ProjectName`。创建素材示例为 `Name`、`GroupId`、`ProjectName`、`Url`、`AssetType`，图片类型示例为 `image`。查询单项以 `Id` 和 `ProjectName` 定位，文档建议项目用 `default`。真人验证通过 `CreateVisualValidateSession` 取得 H5 地址及临时标识，认证完成后用 `GetVisualValidateResult` 查询素材组。以上是供应商能力，本站发布的操作范围仍由 Link 合同规定。

来源：[文档中心](https://doc.nexhina.cn/)、[创建任务](https://doc.nexhina.cn/512729013e0)、[查询任务](https://doc.nexhina.cn/501586097e0)、[快速入门](https://doc.nexhina.cn/8954496m0)、[创建素材组](https://doc.nexhina.cn/501586099e0)、[创建素材](https://doc.nexhina.cn/501586100e0)、[查询素材](https://doc.nexhina.cn/501586104e0)、[拉起真人验证](https://doc.nexhina.cn/513313038e0)、[获取真人验证结果](https://doc.nexhina.cn/513314715e0)。

## 本机视频渠道与素材探测

- 本机后端端口 3501，渠道 146，类型 67，测试组 `minimax-price-test`。客户模型 `seedance-nexhina-fast-qa` 映射 `doubao-seedance-2-0-fast-260128`。Nexhina 的模型列表还返回标准版、Mini、2.5 的精确 ID，联调仅提交 Fast。
- 视频用 `modelark_v3_volcengine` 和 Base URL `https://token.nexhina.cn/seedance`，素材用 `none`。独立测试模型使用本机 Fast 测试价，预扣预算 150,000 tokens，用于本次四秒样本。此价格配置不代表 Nexhina 报价。
- 从本站 ModelArk V3 入口提交 720p、16:9、4 秒文生视频，创建 HTTP 200。本站单查约 135 秒到达 `succeeded`。Nexhina 实际排队、运行及成功回包中的 `created_at` / `updated_at` 均为 JSON 整数，与文档示例不同。终态上游与本站均为 87,300 completion tokens、87,300 total tokens。
- 创建预扣 410,503 quota，结算后净扣 238,913 quota，退差额 171,590 quota。创建和差额日志均已送达。生成成功后转存准备中一度返回 409，转存就绪后授权 GET / HEAD 均为 302，对象存储 Range GET 为 206，并验证了 MP4 文件头。
- 直接调用 Nexhina 素材 `ListAssetGroups`，HTTP 200 且返回 `Result`。用不存在的 ID 调用 `GetAsset`，HTTP 200 但业务错误为 `ResponseMetadata.Error.Code=NotFound.asset_id`。本站渠道以同一类无效 ID 查询素材返回 HTTP 422 `unsupported_asset_operation`。本次素材探测没有写入远端素材、素材组或真人会话。

本次样本的视频创建、轮询、用量结算与交付均可由现有协议完成，无需修改视频解析器。真实生成范围限本次 Fast 文生样本，未对 Nexhina 实际账单对账。

## Mini 素材适配与本机验收

渠道 147 使用测试组 `minimax-price-test`，客户模型 `seedance-nexhina-mini-qa` 映射到凭据模型列表中可见的 `doubao-seedance-2-0-mini-260615`。渠道类型仍为 67。Base URL 为 `https://token.nexhina.cn`，配对 `nexhina_modelark_v3` 与 `nexhina_ark_assets_v1`。视频协议固定增加 `/seedance`，沿用 ModelArk V3 请求、状态、整数用量和本站 OSS 交付。素材协议通过渠道 Bearer 访问固定 Action 端点。

- 普通图片的 `CreateAsset` 使用 `Url`。文档例子写 `AssetType: "image"`，真实调用返回 HTTP 400 `InvalidParameter.AssetType`。改用 `"Image"` 后，本站创建返回 201，单查从 `processing` 到 `ready` 并提供 `asset://`。
- 普通组沿用 `default_fallback`。从管理端手工创建默认组，重复操作实测复用同一组，组单查 HTTP 200。素材不存在时 Nexhina 返回 HTTP 200 业务错误，本站映射为 HTTP 404 `asset_not_found`。
- 适配器当前登记普通图片和普通素材组。真人认证、视频与音频素材入库尚未完成真实验收，暂不声明支持。素材列表供南向管理使用，北向维持现有单资源边界。

同一张公开测试 JPEG、同一客户模型、同一个本地实例（3517，`seedance-link@1.6.1`）和同一个 `POST /api/v3/contents/generations/tasks`。两次均使用 `content[].image_url.url`、`role: reference_image`、480p、16:9 和 Mini 最短 4 秒。

| 参考图输入 | 本站创建 | 本站轮询观察到成功 | 本站结算 | OSS 交付 |
| --- | --- | ---: | ---: | --- |
| HTTPS URL 直传 | HTTP 200 | 约 111 秒 | 40,594 completion tokens，净扣 69,058 quota | GET 302，Range 206，H.264 MP4，864×496，容器 4.096 秒 |
| Nexhina `asset://` | HTTP 200 | 约 124 秒 | 40,594 completion tokens，净扣 69,058 quota | GET 302，Range 206，H.264 MP4，864×496，容器 4.096 秒 |

两条任务分别预扣 255,178 quota，完成后各退 186,120 quota，创建与差额日志均已送达。净扣是本站隔离测试价，未核对 Nexhina 供应商扣费。另有一条修正素材字段前在 3501 提交的 4 秒 HTTPS 样本也成功，独立于表中的同实例对照。

视频适配器回归验证两种引用保持原值。首个 HTTPS 样本在配置默认组和显式创建素材前已提交，最终两条对照生成后，Nexhina 默认组仍只包含显式创建的那一项素材。验收后通过本站改名（200，直接查询上游确认生效）、删除（204）并单查确认 404。默认组保留供本地测试，临时测试令牌已停用。
