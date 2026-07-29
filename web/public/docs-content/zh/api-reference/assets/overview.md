---
page-id: assets-lifecycle
kind: api-reference
last-verified: 2026-07-29
operations:
  - createAsset
  - listAssets
  - getAsset
  - updateAsset
  - deleteAsset
  - listAssetBindings
  - migrateAsset
---

# 素材生命周期

素材 API 把远程媒体注册为当前用户拥有的受控资源，并用 `asset://` 引用复用于支持的媒体请求。

## 创建素材

`POST /v1/assets` · `application/json` · 支持 `Idempotency-Key`

```bash
curl "{{OPENAI_BASE_URL}}/assets" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  -H "Idempotency-Key: asset-request-placeholder" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "品牌片头",
    "asset_kind": "video",
    "media_type": "video/mp4",
    "model": "{{MODEL_ID_PLACEHOLDER}}",
    "source": {
      "type": "url",
      "url": "https://example.com/brand-intro.mp4",
      "expires_at": 1760003600
    }
  }'
```

返回 `202` 表示素材已受理。远程 URL 必须安全、可由服务端访问，并满足所需有效期。若素材涉及真人，先取得有效的[真人授权](api-reference/assets/real-person-authorizations)。

## 查询与管理

```text
GET    /v1/assets
GET    /v1/assets/{asset_id}
PATCH  /v1/assets/{asset_id}
DELETE /v1/assets/{asset_id}
GET    /v1/assets/{asset_id}/bindings
```

列表可按 `status`、`asset_kind`、`media_type` 和 `name` 过滤。更新接口只用于修改名称。删除前检查绑定；删除不会撤回你已经复制到其他系统的媒体。

## 迁移

`POST /v1/assets/{asset_id}/migrations` 使用新的源和迁移原因创建替代素材。迁移同样受模型访问、幂等和资源状态约束。

## asset 引用

素材就绪后，在支持的媒体字段中使用：

```text
asset://asset-placeholder
```

网关只解析当前用户已注册且状态可用的素材。不要依赖上游绑定 ID、渠道字段或内部任务数据。

## 状态与错误

常见状态包括处理中、可用和失败。`409` 通常表示幂等、绑定或状态冲突；`422` 表示素材类型或目标不受支持；`503` 表示素材上游暂不可用。
