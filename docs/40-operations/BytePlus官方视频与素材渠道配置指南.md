---
status: current
owner: Dev Team
last-reviewed: 2026-07-27
---

# BytePlus 官方视频与素材渠道配置指南

## 1. 适用范围

本文用于配置一个同时支持以下两条南向链路的 BytePlus ModelArk 官方渠道：

- 视频生成：模型 API Key + Bearer 鉴权；
- 素材、素材组和真人认证 Action：Access Key ID + Secret Access Key + HMAC 签名。

两套凭据属于同一 BytePlus 账号和 Provider Project，但不能混填。普通客户始终只使用平台 API Key、公开模型名和 `asset://ast_xxx`。

本指南不适用于：

- TokenSave 第三方中转渠道；
- JoyCreator 管理用途素材渠道；
- 把素材 AK/SK 写成 `AK|SK` 后覆盖视频模型 API Key 的历史配置。

## 2. 配置模型

```text
同一个 DoubaoVideo 单 Key 渠道
├─ 视频数据面
│  ├─ video_upstream_profile = official
│  ├─ Model API Key = ark-...
│  ├─ BytePlus Video Base URL
│  └─ 公开模型名 -> Endpoint ID
└─ 素材控制面
   ├─ asset_upstream_profile = official_action_assets
   ├─ Access Key ID
   ├─ Secret Access Key
   ├─ Provider Project
   ├─ Provider Region
   └─ Minimum Asset URL TTL
```

视频和素材仍必须位于同一个渠道和 Provider Project，平台才能把素材绑定约束与视频路由安全地关联。

## 3. 配置前检查

管理员应从 BytePlus 控制台确认并单独保管：

| 配置项 | 来源 | 用途 |
| --- | --- | --- |
| Model API Key | ModelArk 推理凭据 | 视频 Bearer 调用 |
| Endpoint ID | 模型 Endpoint | 模型映射的目标值 |
| Video Base URL | BytePlus ModelArk 区域文档 | 视频创建与查询 |
| Access Key ID | BytePlus IAM | 素材 Action 签名 |
| Secret Access Key | BytePlus IAM | 素材 Action 签名 |
| ProjectName | Endpoint 和素材所在 Project | Action 隔离 |
| Region | Project/Endpoint 所在区域 | 签名和 Action host |
| 最小源 URL TTL | Provider 合同或实测 | 提交前 TTL 门槛 |

还须确认：

- AK/SK 身份拥有目标 Project 的 Ark 素材权限；
- 账号已开通所需素材库或高级创作权益；
- 目标 Endpoint 支持素材引用；
- 测试源素材可通过公网 HTTPS 访问；
- 测试渠道为单 Key，不使用多 Key 模式；
- 渠道所属用户组和模型权限与验收 Token 一致。

不要从历史工单、截图或聊天记录猜 Region、Project 或 Endpoint。

## 4. 新建官方双凭据渠道

### 4.1 基本信息

在管理后台新增渠道：

| 字段 | 值 |
| --- | --- |
| 渠道类型 | `DoubaoVideo` |
| 添加模式 | 单 Key |
| 渠道状态 | 启用 |
| 用户组 | 按业务实际选择 |
| Models | 客户端实际请求的公开模型名 |

素材绑定依赖单一、稳定的渠道账号。多 Key 渠道不会进入素材路由候选集。

### 4.2 视频协议

选择：

```text
Video Upstream Profile: Official Protocol
```

填写：

- `Model API Key`：只填写 BytePlus 模型 API Key；
- `Base URL`：BytePlus 视频 API 根地址；
- `Models`：平台对外公开模型名；
- 模型映射：`公开模型名 -> BytePlus Endpoint ID`。

模型映射方向不能反转。Endpoint ID 不是 Base URL，也不应直接暴露为平台模型名。

### 4.3 素材 Action

选择：

```text
Asset Upstream Profile: Official Action Assets
```

填写：

| 字段 | 要求 |
| --- | --- |
| 素材 Access Key ID | 与目标 Project 有关的 AK |
| 素材 Secret Access Key | 与 AK 配对的 SK |
| Provider Project | BytePlus 官方 `ProjectName`，区分大小写 |
| Provider Region | 从控制台确认的区域 |
| Minimum Asset URL TTL | 正整数；来自 Provider 合同或真实验收 |

Action host 由 Region 推导，不使用视频 Base URL。平台渠道分组 `group` 也不是 Provider Project。

最小 TTL 应覆盖：

```text
最晚首次抓取时间
+ Processing 阶段可能的重复抓取窗口
+ 网络、队列和时钟偏差裕量
```

Provider 返回 URL 的有效期不等于客户源 URL 的抓取 SLA，不要直接套用。

### 4.4 保存语义

- 新建渠道时 AK 和 SK 必须同时填写；
- 编辑已配置渠道时两项都留空表示保留；
- 同时填写两项表示轮换；
- 只填写其中一项应被拒绝；
- 清除凭据必须先把素材 profile 改为 `none`，再执行显式清除；
- 复制渠道不会复制敏感素材凭据，复制后必须重新填写。

## 5. 保存后分链路测试

### 5.1 视频 API 测试

在渠道编辑页执行“测试视频 API”。该测试使用视频 Base URL 和模型 API Key，只读查询最多一个已有任务，不创建新视频。

通过标准：

- 请求被 BytePlus 接受；
- 没有 `video_api_upstream_rejected`；
- 测试状态显示可用。

只读测试不能证明 Endpoint 映射和模型额度正确。仍须使用平台视频创建 API 完成生成验收。

### 5.2 素材 Action 测试

执行“测试素材 Action”。该测试使用独立 AK/SK、Region 和 Project 调用只读 `ListAssets`，不会创建或删除素材。

通过标准：

- 签名和 Action host 正确；
- Project 和 Region 被接受；
- 账号拥有素材查询权限；
- 测试状态显示可用。

视频测试与素材测试必须分别通过。任一条失败时只排查对应凭据和地址，不要互换两套凭据。

### 5.3 平台全链路验收

完成只读连通性测试后，按 [素材库验收操作手册](素材库验收操作手册.md) 验证：

1. 创建普通素材；
2. 轮询至 `ready`；
3. 用同一公开模型通过 `asset://ast_xxx` 创建视频；
4. 下载视频；
5. 验证幂等、用户隔离和 URL 安全；
6. 删除素材并确认不能再建立新引用。

真人素材只在相关权益、同意政策和认证配置均就绪后单独验收。

## 6. 编辑、轮换与删除

官方素材账号身份由 Action host、AK/SK、profile、Project 和 Region 共同决定。存在未删除素材、素材组或真人授权时，平台会拒绝：

- 改变素材账号身份；
- 轮换或清除 AK/SK；
- 改变 Project、Region 或素材 profile；
- 删除渠道。

标准轮换顺序：

1. 停止新建素材；
2. 撤回真人授权；
3. 删除或迁移活动素材；
4. 等待上游清理完成；
5. 轮换 AK/SK 或改变 Project/Region；
6. 重新执行两条只读测试和全链路验收。

平台不会让旧素材 binding 静默切换到新账号。

## 7. 存量旧渠道迁移

若历史渠道曾把 `AK|SK` 写入主 Key：

- 无存量素材时：把主 Key 改回视频模型 API Key，并在独立区域填写 AK/SK；
- 有存量官方素材、素材组或真人授权时：停服、备份后使用迁移命令逐渠道处理。

先生成迁移计划：

```bash
go run ./cmd/migrate-official-asset-credential < /secure/path/channel.json
```

确认计划中的渠道、活动资源和变更数量后再应用：

```bash
go run ./cmd/migrate-official-asset-credential --apply < /secure/path/channel.json
```

应用模式会先执行只读 `ListAssets` 校验，再在单事务内迁移独立凭据、模型 API Key、绑定、素材组、真人授权、所有权声明和对账指纹。未知指纹、唯一索引冲突或写入错误必须整体回滚。

凭据文件须位于权限受控路径，完成后安全删除。不要把真实密钥写进 shell 历史、文档、Issue 或仓库。

## 8. 地址速查

| 地址 | 示例 | 用途 |
| --- | --- | --- |
| Web 管理端 | `http://localhost:3100` | 打开后台 |
| 平台 API / ServerAddress | `http://localhost:8100` | 调用 new-api |
| BytePlus 视频 Base URL | `https://ark.ap-southeast.bytepluses.com` | 视频创建与查询 |
| BytePlus 素材 Action host | `https://ark.<region>.byteplusapi.com` | 系统按 Region 生成 |
| 客户 `source.url` | `https://cdn.example.com/signed/a.png` | 供 BytePlus 抓取 |

Web 端口、平台 API 地址、视频 Base URL、Action host 和素材源 URL 是五个不同概念。

## 9. 常见错误

| 错误或现象 | 主要原因 | 处理 |
| --- | --- | --- |
| `asset_binding_required` | 没有兼容的单 Key 素材渠道 | 检查渠道状态、用户组、模型、映射和两个 profile |
| `no compatible single-key asset channel` | 多 Key 或素材配置不完整 | 改为单 Key 并补齐字段 |
| `asset_upstream_unavailable` | 动态开关关闭、无可用渠道或上游不可用 | 检查 `asset_setting.enabled` 和素材测试 |
| `video_api_upstream_rejected` | 视频 API Key、Base URL 或权限错误 | 只排查视频链路 |
| `asset_action_upstream_rejected` | AK/SK、Project、Region 或权限错误 | 只排查素材链路 |
| `asset_url_ttl_insufficient` | 源 URL 剩余 TTL 不足 | 重新签发有效期更长的 URL |
| `asset_credential_changed` | 活动 binding 与当前素材账号不一致 | 恢复原配置清理，或创建迁移后的新素材 |
| `asset_credential_profile_active` | 素材 profile 启用时请求清除 AK/SK | 先改为 `none` |
| 页面持续“注册中” | 客户端保留了失败请求的占位 | 先确认 POST 最终状态，再清理占位 |
| 日志出现 `record not found` | 首次幂等查询或空 Job 队列 | 结合最终 HTTP 状态判断，不单独视为根因 |

## 10. 安全检查表

- [ ] Model API Key 中没有 AK/SK。
- [ ] AK/SK 没有写入 `Other Settings` JSON。
- [ ] 管理响应和截图没有完整素材凭据。
- [ ] 日志没有渠道对象、签名请求头或完整上游响应。
- [ ] Project、Region 和 Endpoint 均从控制台确认。
- [ ] 两条只读测试分别通过。
- [ ] 全链路使用测试素材和测试 Token。
- [ ] 轮换前已清理活动素材和授权。

## 11. 相关文档

- [素材代理与真人授权架构](../20-architecture/素材代理与真人授权架构.md)
- [视频上游接入与异步任务架构](../20-architecture/视频上游接入与异步任务架构.md)
- [素材代理与真人授权配置手册](素材代理与真人授权配置手册.md)
- [素材库验收操作手册](素材库验收操作手册.md)
- [视频生成与素材库 API 对接指南](../30-engineering/视频生成与素材库API对接指南.md)
- 历史实施记录：[BytePlus 官方视频与素材双凭据渠道接入完整方案](../99-archive/2026-07-26-BytePlus官方视频与素材双凭据渠道接入完整方案.md)
