---
status: current
owner: Dev Team
last-reviewed: 2026-07-21
---

# 海外素材库

用于海外素材创建、列表查询、状态查询、改名、删除，以及视频生成素材引用。

## 概述

- **API 端点**：`https://www.moxing.pro`
- **鉴权**：`Authorization: Bearer sk-your-api-key`
- **权限**：素材访问权限绑定到 API Key 对应用户，不能跨用户查询或引用

## 关键约定

- 所有接口都需要 Bearer Token
- 素材分组由服务端环境变量统一配置，客户端请求不需要传素材分组 ID
- 资产分组不对外提供增删改查接口；平台统一使用服务端配置的默认资产分组
- 创建素材后通常需要轮询 `GET /assets/:uuid`，直到状态变为 `Active` 或 `Failed`
- 创建图片素材时，`url` 支持 HTTP/HTTPS 链接或 base64 图片；base64 会先转存为资源 URL 后再提交上游，素材记录不保存原始 base64
- 视频生成前，平台会校验请求中的 `asset://` 引用必须属于当前用户，且素材状态必须为 `Active`

## 接口清单

| 能力 | 方法 | 路径 | 说明 |
|------|------|------|------|
| 创建素材 | POST | `/assets` | 提交素材 URL 或图片 base64，返回素材 uuid 和处理状态。 |
| 查询素材列表 | POST | `/assets/list` | 按状态、名称、分页查询当前用户绑定的素材。 |
| 查询素材 | GET | `/assets/:uuid` | 查询单个素材，并尽量同步刷新上游最新状态。 |
| 更新素材 | POST | `/assets/:uuid/update` | 更新素材名称。 |
| 删除素材 | POST | `/assets/:uuid/delete` | 删除素材，并解除当前用户绑定。 |

## 状态说明

| 状态 | 说明 |
|------|------|
| `Pending` | 已创建，等待上游处理。 |
| `Processing` | 上游处理中，建议退避轮询详情接口。 |
| `Active` | 素材处理完成，可以在视频生成请求中使用。 |
| `Failed` | 素材处理失败，查看 `error_message` 获取原因。 |

## 请求参数

### 创建素材

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `url` | string | 是 | 素材地址。`asset_type=Image` 时支持 HTTP(S) URL、`data:image/...;base64,...` 或裸 base64；Video/Audio 仍需 HTTP(S) URL。 |
| `name` | string | 否 | 素材名称，最长 64 字符。 |
| `asset_type` | string | 是 | 素材类型，支持 `Image` / `Video` / `Audio`。 |

### 查询列表

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `statuses` | string[] | 否 | 状态过滤，支持 `Active` / `Pending` / `Processing` / `Failed`。 |
| `name` | string | 否 | 按素材名称模糊搜索。 |
| `page_number` | integer | 否 | 页码，默认 1。 |
| `page_size` | integer | 否 | 每页数量，默认 20，最大 100。 |

### 更新素材

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `name` | string | 是 | 更新后的素材名称，最长 64 字符。 |

## 调用示例

### 创建素材

```bash
curl -X POST 'https://www.moxing.pro/assets' \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer sk-xxxx' \
  -d '{
    "url": "https://example.com/avatar.png",
    "name": "my-avatar",
    "asset_type": "Image"
  }'
```

### 查询列表

```bash
curl -X POST 'https://www.moxing.pro/assets/list' \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer sk-xxxx' \
  -d '{
    "statuses": ["Active"],
    "name": "avatar",
    "page_number": 1,
    "page_size": 20
  }'
```

### 查询素材详情

```bash
curl 'https://www.moxing.pro/assets/ea287e6e-13fb-4af4-a953-9e7cbb883ee5' \
  -H 'Authorization: Bearer sk-xxxx'
```

### 更新素材

```bash
curl -X POST 'https://www.moxing.pro/assets/ea287e6e-13fb-4af4-a953-9e7cbb883ee5/update' \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer sk-xxxx' \
  -d '{"name":"new-avatar-name"}'
```

### 删除素材

```bash
curl -X POST 'https://www.moxing.pro/assets/ea287e6e-13fb-4af4-a953-9e7cbb883ee5/delete' \
  -H 'Authorization: Bearer sk-xxxx'
```

## 响应示例

创建素材成功返回 `202 Accepted`，此时素材通常仍处于 `Pending` 或 `Processing`：

```json
{
  "uuid": "ea287e6e-13fb-4af4-a953-9e7cbb883ee5",
  "upstream_id": null,
  "name": "my-avatar",
  "asset_type": "Image",
  "source_url": "https://example.com/avatar.png",
  "asset_url": null,
  "status": "Pending",
  "error_code": null,
  "error_message": null,
  "project_name": "default",
  "created_at": "2026-04-01T08:00:00Z",
  "updated_at": "2026-04-01T08:00:00Z"
}
```

查询到 `Active` 后，可使用 `upstream_id` 或 `uuid` 拼接 asset URI：

```json
{
  "uuid": "ea287e6e-13fb-4af4-a953-9e7cbb883ee5",
  "upstream_id": "asset-20260401184821-8tkgk",
  "name": "my-avatar",
  "asset_type": "Image",
  "source_url": "https://example.com/avatar.png",
  "asset_url": "https://cdn.example.com/processed/avatar.png?token=xxx",
  "status": "Active",
  "error_code": null,
  "error_message": null,
  "project_name": "default",
  "created_at": "2026-04-01T08:00:00Z",
  "updated_at": "2026-04-01T08:01:00Z"
}
```

列表查询返回当前用户绑定的素材集合：

```json
{
  "total": 1,
  "page_number": 1,
  "page_size": 20,
  "items": [
    {
      "uuid": "ea287e6e-13fb-4af4-a953-9e7cbb883ee5",
      "upstream_id": "asset-20260401184821-8tkgk",
      "name": "my-avatar",
      "asset_type": "Image",
      "source_url": "https://example.com/avatar.png",
      "asset_url": "https://cdn.example.com/processed/avatar.png?token=xxx",
      "status": "Active",
      "error_code": null,
      "error_message": null,
      "project_name": "default",
      "created_at": "2026-04-01T08:00:00Z",
      "updated_at": "2026-04-01T08:01:00Z"
    }
  ]
}
```

## 轮询示例

创建素材后建议每 3 到 5 秒查询一次详情，进入终态后停止轮询：

```python
import time
import requests

base_url = "https://www.moxing.pro"
headers = {"Authorization": "Bearer sk-xxxx"}
uuid = "ea287e6e-13fb-4af4-a953-9e7cbb883ee5"

for _ in range(36):
    resp = requests.get(f"{base_url}/assets/{uuid}", headers=headers, timeout=30)
    resp.raise_for_status()
    asset = resp.json()
    print(asset["status"], asset.get("upstream_id"))
    if asset["status"] == "Active":
        print("asset uri:", f"asset://{asset['upstream_id'] or asset['uuid']}")
        break
    if asset["status"] == "Failed":
        raise RuntimeError(asset.get("error_message") or "asset failed")
    time.sleep(5)
```

## 用于视频生成

素材状态为 `Active` 后，可在视频请求中引用。V2 媒体任务（如 `sd2.0`）使用 `image` / `reference_images` 等字段；海外官Key 模型 `dreamina-seedance-2-0-260128` 请走海外官Key 视频，在官方 `content[]` 里使用 `asset://{upstream_id}`。

```bash
curl -X POST 'https://www.moxing.pro/v1/media/generations' \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer sk-xxxx' \
  -d '{
    "model": "doubao-seedance-2-0-fast-oversea",
    "capability": "video_generation",
    "prompt": "让这个人物自然微笑并看向镜头",
    "input_mode": "single_image",
    "image": "asset://asset-20260401184821-8tkgk",
    "duration_seconds": -1,
    "aspect_ratio": "16:9",
    "resolution": "720p"
  }'
```

## 响应字段说明

| 字段 | 类型 | 说明 |
|------|------|------|
| `uuid` | string | 平台侧素材唯一标识。 |
| `upstream_id` | string / null | 上游素材 ID；Active 后通常可用于 `asset://` 引用。 |
| `name` | string | 素材名称。 |
| `asset_type` | string | `Image` / `Video` / `Audio`。 |
| `source_url` | string | 创建素材时传入的原始 URL。 |
| `asset_url` | string / null | 上游处理完成后的素材 URL。 |
| `status` | string | `Pending` / `Processing` / `Active` / `Failed`。 |
| `error_code` | string / null | 上游错误码，当前可能为空。 |
| `error_message` | string / null | 失败原因。 |
| `project_name` | string | 默认项目名称，当前为 `default`。 |
| `created_at` | string | RFC3339 时间。 |
| `updated_at` | string | RFC3339 时间。 |

## 错误响应

| HTTP 状态码 | 说明 |
|-------------|------|
| 401 | API Key 无效或缺失。 |
| 403 | 素材不属于当前用户，或服务端默认素材分组未配置。 |
| 404 | 素材不存在，或当前用户没有绑定该素材。 |
| 409 | 上游资源冲突。 |
| 422 | 请求参数校验失败。 |
| 429 | 请求频率超限。 |
| 503 | 素材库上游不可用，或上游渠道 Key 未配置。 |

错误响应统一使用 `detail` 字段返回可读原因：

```json
{
  "detail": "asset does not belong to current user: asset://asset-20260401184821-8tkgk"
}
```
