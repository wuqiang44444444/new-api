---
page-id: assets-lifecycle
kind: api-reference
last-verified: 2026-08-10
operations:
  - createAsset
  - listAssets
  - getAsset
  - updateAsset
  - deleteAsset
  - createAssetGroup
  - listAssetGroups
  - getAssetGroup
  - deleteAssetGroup
---

# 素材与素材组

素材 API 把远程媒体直接创建到指定 Seedance 模型的固定渠道。平台只返回 `ast_*`、`astgrp_*` 和
`asset://ast_*`，不会暴露 Provider 资源 ID、渠道凭据或完整源地址，也不会把素材迁移或复制到其它渠道。

## 创建普通素材

`POST /v1/assets` · `application/json`

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

成功返回 `201`。远程 URL 必须保持到渠道要求的最短抓取时间；Provider 创建完成后，平台不保存该源 URL。
创建或删除失败按普通失败返回，客户端应保存 `request_id` 交给技术人员排查，不要自动换模型或渠道重试。

## 素材组与真人认证

普通组和真人组都通过 `POST /v1/asset-groups` 创建。真人认证由 Provider 官网完成，平台不采集人脸或
自建认证表单：

```bash
curl "{{OPENAI_BASE_URL}}/asset-groups" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "已认证演员",
    "group_kind": "real_person",
    "model": "{{MODEL_ID_PLACEHOLDER}}",
    "redirect_url": "https://example.com/verification-complete"
  }'
```

响应中的 `verification_url` 是上游认证地址，应直接交给本人打开。使用
`GET /v1/asset-groups/{group_id}` 查询上游认证结果；达到 `ready` 后，创建真人素材时传入该
`asset_group_id`。一个素材组和其素材永远固定在创建时的模型、渠道、账号和区域。

## 查询与删除

```text
GET    /v1/assets
GET    /v1/assets/{asset_id}
PATCH  /v1/assets/{asset_id}
DELETE /v1/assets/{asset_id}

GET    /v1/asset-groups
GET    /v1/asset-groups/{group_id}
DELETE /v1/asset-groups/{group_id}
```

删除非空素材组会失败。系统不提供素材 binding、迁移、自动物化、真人授权撤回或重试接口。

## 在视频请求中引用

素材达到 `ready` 后，在 ModelArk V3 的对应媒体 URL 中使用：

```text
asset://ast_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

网关只解析当前 API Key 所属应用、当前客户模型及其唯一 Seedance 渠道创建的素材。普通 HTTP/HTTPS
URL 和 Data URL 仍可按官方请求结构直接使用，但不会自动获得素材库语义。
