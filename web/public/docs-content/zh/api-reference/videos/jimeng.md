---
page-id: videos-jimeng
kind: api-reference
last-verified: 2026-07-31
operations:
  - createJimengVideo
---

# 即梦视频

即梦通过同一个 `POST /jimeng/` 入口和查询参数 `Action` 区分提交与查询。
合同 operation 已发布不表示所有账户都能使用任意 `req_key`；调用前应以 `GET /v1/models` 和
当前账户权限为准。当前即梦 Link 合同只发布 submit/query Action，不提供列表或删除合同。
当前内置适配器登记的公开 `req_key` 为 `jimeng_vgfm_t2v_l20`。

## 提交任务

```bash
curl "{{SITE_BASE_URL}}/jimeng/?Action=CVSync2AsyncSubmitTask&Version=2022-08-31" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  -H "Content-Type: application/json" \
  -d '{
    "req_key": "jimeng_vgfm_t2v_l20",
    "prompt": "阳光穿过窗帘，房间里的植物轻轻摇曳",
    "aspect_ratio": "16:9"
  }'
```

`req_key` 必填且当前只接受 `jimeng_vgfm_t2v_l20`。创建字段为 `req_key`、
`binary_data_base64`、`image_urls`、`prompt`、`seed`、`aspect_ratio` 和 `frames`。按模型能力
可使用 `image_urls` 或 `binary_data_base64` 输入图片；不要同时发送与模型不兼容的媒体字段。

## 查询结果

```bash
curl "{{SITE_BASE_URL}}/jimeng/?Action=CVSync2AsyncGetResult&Version=2022-08-31" \
  -H "Authorization: Bearer {{API_KEY_PLACEHOLDER}}" \
  -H "Content-Type: application/json" \
  -d '{"task_id": "video-task-placeholder"}'
```

`Action` 和 `Version` 都是合同的一部分。提交与查询虽然共用 operation，但请求字段和结果阶段不同。
当前合同不发布列表、平台内容代理、取消或删除操作。

## 响应与重试

检查 HTTP 状态后，再读取响应中的公开 `code`、`message`、`request_id` 与任务数据。任务未到终态时使用退避轮询；业务失败后停止轮询并保留请求 ID。
