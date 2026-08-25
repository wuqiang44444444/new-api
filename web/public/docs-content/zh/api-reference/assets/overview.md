---
page-id: assets-lifecycle
kind: api-reference
last-verified: 2026-08-25
operations:
  - createAsset
  - getAsset
  - updateAsset
  - deleteAsset
  - createAssetGroup
  - getAssetGroup
---

# 素材与素材组

素材 API 是调用方自管的无状态单资源代理。应用保存 Provider opaque 素材 ID，中转站不提供素材或素材
组列表，不保存 `Asset` / `AssetGroup` 映射，也不在视频生成前验证素材可用性。

调用前查询模型条目的 `api.assets`。`supported` 和 `operations` 说明当前客户模型实际支持哪些操作；
`creation` 和 `media` 说明 URL、媒体类型与素材组限制。所有单项操作都必须携带客户模型名。

`reuse_scope` 是匿名素材复用域。两个模型只有非空 scope 完全相同时才可尝试复用；不同或缺失时不得
跨模型发送素材 ID。相同 scope 也不证明所有权或 ready 状态，实际结果仍由上游裁决。客户模型后缀可
用于人工识别，但不能替代 scope 比较。

## 创建素材

```bash
curl "{{OPENAI_BASE_URL}}/assets" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "品牌片头",
    "asset_kind": "general",
    "media_type": "video",
    "model": "{{MODEL_ID_PLACEHOLDER}}",
    "source": {
      "type": "url",
      "url": "https://example.com/brand-intro.mp4",
      "expires_at": 1800000000
    }
  }'
```

成功响应直接返回 opaque ID。`id` 用于 CRUD；`reference` 用于视频，两者可能不同：

```json
{
  "object": "asset",
  "id": "provider-resource-id",
  "model": "customer-model-name",
  "reference": "asset://provider-reference-id",
  "status": "ready"
}
```

未知客户模型返回 `404 model_not_found`；只有已配置并确认不支持素材库的模型或操作才返回
`422 unsupported_asset_operation`。Provider 拒绝、凭据或服务异常不会伪装成“不支持”，而返回脱敏的
`asset_upstream_error` 或 `asset_upstream_unavailable`。

## 查询、更新和删除

```text
GET    /v1/assets/{asset_id}?model={customer_model}
PATCH  /v1/assets/{asset_id}  body: {"model":"...","name":"..."}
DELETE /v1/assets/{asset_id}?model={customer_model}
```

中转站只调用该客户模型选定的唯一上游。ID 来自另一个 Provider 时，不探测真实来源、不换渠道、
不 fallback，直接返回当前上游的脱敏结果。

## 素材组与真人认证

```text
POST   /v1/asset-groups
GET    /v1/asset-groups/{group_id}?model={customer_model}
```

中转站不提供素材组删除接口，也不会主动删除 Provider 素材组。确需清理时，应由 Provider 管理员在确认
素材组归属、组内素材和级联影响后执行。

真人认证创建响应中的 `id` 是上游会话 ID，并包含 `verification_url`。查询认证结果时使用：

```text
GET /v1/asset-groups/{session_id}?model={customer_model}&verification_session=true
```

认证完成后响应的 `group_id` 才用于创建真人素材。中转站不采集人脸、证件或活体数据。

## 在视频请求中引用

```json
{
  "type": "image_url",
  "image_url": {"url": "asset://provider-reference-id"},
  "role": "reference_image"
}
```

中转站不查询素材、不校验所有权、ready 状态、创建模型、Channel 或 Provider 作用域。Provider 在当次
生成中判断素材存在性、权限、模型兼容性与内容审核；单项查询成功也不保证生成成功。
