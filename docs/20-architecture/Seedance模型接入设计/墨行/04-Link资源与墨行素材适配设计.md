---
status: accepted
owner: Dev Team
last-reviewed: 2026-08-11
---

# 墨行素材库适配设计

## 1. 目标

墨行素材必须使用代码化 `relay_assets_v1` 协议，并按平台一对一资源模型重新验收：

```text
ast_* / astgrp_*
  -> user_id + app_id
  -> 固定墨行 Channel / Key 账号
  -> 一个 Provider Asset / Group
```

不再使用通用 AssetBinding、一个 Asset 多账号物化、source URL fallback 或自动迁移。

## 2. 当前边界

现有资料描述 `/assets/*` 的创建、状态、归属和引用，并把 Pending/Processing/Active/Failed 映射到
平台状态。但两条视频线路的素材账号、创建/查询/删除、Key 轮换和真实视频引用仍需重新验证。验收前
两条 Channel 都应配置 `asset_upstream_protocol=none`。

如果验证通过，每个 Asset 固定到创建时 Channel、Provider 账号和上游 ID。同账号正常轮换 Key 可继续
使用；改变账号、Base URL或素材协议必须新建 Channel。JoyCreator 与历史 Ark 素材不参与当前线路。

## 3. 真人边界

当前 `/assets/*` 资料没有完整真人认证邀请、Group、状态和删除合同，不能开放真人 AssetGroup。未来若
Provider 提供验证页面，adapter 直接返回上游链接/二维码，平台不自建 H5、人脸表单或授权域。

## 4. 请求与失败

视频可以混用平台 Asset 与普通 URL/Data URL，但所有平台 Asset 必须匹配客户模型唯一 Channel和相同
账号作用域。创建素材未取得可信 Provider ID 即失败；删除结果不明确即失败并保留状态，不建立
create_unknown/delete_unknown 或管理员核查系统。

## 5. 不变量

1. 客户只看到 `ast_*` / `astgrp_*`，不看到墨行资源 ID。
2. 一个平台资源只对应一个墨行 Provider 资源。
3. 当前线路、历史 Ark 和 JoyCreator 资源不互换。
4. 协议真实验收前素材能力保持关闭。
