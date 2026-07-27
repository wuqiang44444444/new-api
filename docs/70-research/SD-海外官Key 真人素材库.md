---
status: current
owner: Dev Team
last-reviewed: 2026-07-21
---

# 海外官Key 真人素材库

H5 真人认证、素材组与素材管理；合规人像素材上传与 `asset://` 引用。

## 概述

面向模型 `dreamina-seedance-2-0-260128` 的合规人像参考素材。完整接入路径见模型文档。

- **鉴权**：`Authorization: Bearer sk-...`
- **对外标识**：直接使用官方 `group-xxx` / `asset-xxx`
- **视频引用**：`asset://{asset_id}`
- **回调配置**：服务端需配置 `ARK_ASSET_CALLBACK_BASE_URL`（公网 HTTPS），用于 H5 认证回调

## 推荐流程

### 虚拟人像

```
CreateAssetGroup → 上传素材 → 轮询 Active
```

### 真人肖像

```
H5 认证建组 → 上传素材 → 轮询 Active
```

详细步骤：

1. `POST /v1/ark/assets/groups`（`GroupType=AIGC`）→ 返回 Id（`group-xxx`）
2. `POST /v1/ark/assets/visual-validate/session` → 返回 `h5_link` + `session_id`
3. 终端用户打开 H5 完成真人认证（平台不托管 H5 页）
4. `GET .../visual-validate/sessions/:session_id` 或 `.../result/:session_id` → 获得 `group_id`
5. `POST /v1/ark/assets`（传 `GroupId`）→ 返回 Id
6. `GET /v1/ark/assets/:id` 轮询至 `Status=Active`
7. `POST /v1/ark/media/generations` 在 `content[]` 中使用 `asset://{Id}`

## 接口清单

| 能力 | 方法 | 路径 |
|------|------|------|
| 发起 H5 认证 | POST | `/v1/ark/assets/visual-validate/session` |
| 创建虚拟人像素材组 | POST | `/v1/ark/assets/groups` |
| 查询 session 状态 | GET | `/v1/ark/assets/visual-validate/sessions/:session_id` |
| 获取 GroupId | GET | `/v1/ark/assets/visual-validate/result/:session_id` |
| 创建素材 | POST | `/v1/ark/assets` |
| 查询素材 | GET | `/v1/ark/assets/:id` |
| 素材列表（本账号） | POST | `/v1/ark/assets/list` |
| 素材组列表（本账号） | POST | `/v1/ark/assets/groups/list` |
| 更新素材 | POST | `/v1/ark/assets/:id/update` |
| 删除素材 | POST | `/v1/ark/assets/:id/delete` |
| 查询/更新/删除素材组 | GET/POST | `/v1/ark/assets/groups/:group_id` |

## Session 状态

| 状态 | 说明 |
|------|------|
| `pending` | 已创建，等待用户打开 H5 并完成认证。 |
| `callback_received` | 已收到 H5 回调，正在拉取 GroupId。 |
| `group_ready` | 认证成功，已获得 `group_id`，可上传素材。 |
| `failed` | 认证失败，查看 `error_message`。 |
| `expired` | BytedToken / session 超时（约 30 分钟），需重新发起 session。 |

## 调用示例

### 虚拟人像素材组（API 直建）

```bash
curl -X POST 'https://www.moxing.pro/v1/ark/assets/groups' \
  -H 'Authorization: Bearer sk-xxxx' \
  -H 'Content-Type: application/json' \
  -d '{
    "Name": "virtual-portrait-group",
    "Description": "虚拟人像素材组",
    "GroupType": "AIGC"
  }'
```

### 真人肖像素材组（H5 认证）

```bash
curl -X POST 'https://www.moxing.pro/v1/ark/assets/visual-validate/session' \
  -H 'Authorization: Bearer sk-xxxx' \
  -H 'Content-Type: application/json' \
  -d '{
    "client_redirect_url": "https://your-app.com/h5-done",
    "project_name": "default"
  }'
```

打开响应中的 `h5_link` 完成认证后：

```bash
# 查询 session 状态
curl 'https://www.moxing.pro/v1/ark/assets/visual-validate/sessions/{session_id}' \
  -H 'Authorization: Bearer sk-xxxx'

# 获取 GroupId
curl 'https://www.moxing.pro/v1/ark/assets/visual-validate/result/{session_id}' \
  -H 'Authorization: Bearer sk-xxxx'
```

获得 `group_id` 后上传素材：

```bash
# 上传素材
curl -X POST 'https://www.moxing.pro/v1/ark/assets' \
  -H 'Authorization: Bearer sk-xxxx' \
  -H 'Content-Type: application/json' \
  -d '{
    "GroupId": "group-20260331145705-xxxxx",
    "URL": "https://example.com/portrait-front.png",
    "AssetType": "Image",
    "Name": "portrait-front"
  }'

# 查询素材
curl 'https://www.moxing.pro/v1/ark/assets/asset-20260318071009-xxxxx' \
  -H 'Authorization: Bearer sk-xxxx'

# 素材列表
curl -X POST 'https://www.moxing.pro/v1/ark/assets/list' \
  -H 'Authorization: Bearer sk-xxxx' \
  -H 'Content-Type: application/json' \
  -d '{"Statuses":["Active"],"PageNumber":1,"PageSize":20}'
```

### 用于 dreamina 视频

素材 Active 后，在海外官Key 视频请求的 `content[]` 中引用：

```bash
curl -X POST 'https://www.moxing.pro/v1/ark/media/generations' \
  -H 'Authorization: Bearer sk-xxxx' \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "dreamina-seedance-2-0-260128",
    "duration": 5,
    "resolution": "720p",
    "ratio": "16:9",
    "generate_audio": true,
    "content": [
      { "type": "text", "text": "参考图主体自然微笑并看向镜头" },
      {
        "type": "image_url",
        "role": "reference_image",
        "image_url": { "url": "asset://asset-20260318071009-xxxxx" }
      }
    ]
  }'
```

## Python 全流程

```python
import time
import requests

base = "https://www.moxing.pro"
headers = {"Authorization": "Bearer sk-xxxx", "Content-Type": "application/json"}

# 1. 发起 H5 认证
sess = requests.post(f"{base}/v1/ark/assets/visual-validate/session", headers=headers, json={
    "client_redirect_url": "https://your-app.com/done",
}, timeout=30).json()
session_id = sess["session_id"]
print("Open H5:", sess["h5_link"])

# 2. 用户完成 H5 后轮询 session / result
for _ in range(60):
    st = requests.get(f"{base}/v1/ark/assets/visual-validate/sessions/{session_id}", headers=headers, timeout=30).json()
    print("session", st["status"], st.get("group_id"))
    if st["status"] == "group_ready":
        group_id = st["group_id"]
        break
    if st["status"] in ("failed", "expired"):
        raise RuntimeError(st)
    time.sleep(3)
else:
    raise TimeoutError("H5 session timeout")

# 3. 上传素材
created = requests.post(f"{base}/v1/ark/assets", headers=headers, json={
    "GroupId": group_id,
    "URL": "https://example.com/portrait.png",
    "AssetType": "Image",
    "Name": "portrait",
}, timeout=60).json()
asset_id = created["Id"]
print("asset id", asset_id)

# 4. 轮询 Active
for _ in range(36):
    asset = requests.get(f"{base}/v1/ark/assets/{asset_id}", headers=headers, timeout=30).json()
    print("asset", asset.get("Status"))
    if asset.get("Status") == "Active":
        asset_uri = f"asset://{asset_id}"
        break
    if asset.get("Status") == "Failed":
        raise RuntimeError(asset)
    time.sleep(5)

# 5. 用于 dreamina 视频（见海外官Key 视频文档）
print("use in video:", asset_uri)
```
