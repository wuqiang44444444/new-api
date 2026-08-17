# Seedance 2.0 素材库使用说明

Seedance 2.0 支持两类可信素材库，上传成功后接口返回 `assetUrl`（格式 `asset://<assetId>`），在 Seedance 2.0 / Seedance 2.0 Fast 生成视频时把该 `assetUrl` 作为素材 URL 传入即可：

- **真人素材**：需先完成真人认证，再上传素材文件。
- **虚拟素材**：用于虚拟人像，先创建素材组，再向组内上传素材，**无需真人认证**。

**Base URL**: `https://mm-internal-cn.leonecloud.com`

私有部署或 IP 访问时，Base URL 使用当前 open-chat 所在 host，例如 `http://8.153.104.252`。

所有接口都需要请求头：

```http
Authorization: Bearer {YOUR_API_KEY}
```

> 通用说明：
>
> - 上传素材为异步处理，返回的 `assetStatus` 为 `Active` 时素材方可用于生成；`Processing` 表示仍在处理，可稍后在素材列表中查看；`Failed` 表示处理失败。
> - 生成视频时使用 `assetUrl`，不要使用 `fileUrl`。
> - 单个上传文件最大 `100MB`。

---

# 一、真人素材

真人素材需要先完成真人认证，再上传素材文件。

## 操作流程

1. 创建真人认证二维码。
2. 用户扫码完成真人认证。
3. 上传真人素材文件。
4. 从上传结果或素材列表中拿到 `assetUrl`。
5. 调用 Seedance 2.0 生成视频时，把 `assetUrl` 作为素材 URL 传入。

## 1. 创建真人认证二维码

**POST** `/api/v2/open/material/person/validate/session`

请求：

```bash
curl -X POST "{BASE_URL}/api/v2/open/material/person/validate/session" \
  -H "Authorization: Bearer {YOUR_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "callbackUrl": "https://example.com/#/open-chat/seedance2-0"
  }'
```

返回：

```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "bytedToken": "20260515143000xxxxxxxx",
    "h5Link": "https://h5-v2.example.com?..."
  }
}
```

前端把 `h5Link` 渲染成二维码，让用户扫码完成认证。`bytedToken` 用于下一步上传素材。

## 2. 上传真人素材

**POST** `/api/v2/open/material/upload`

`bytedToken` 必须传第 1 步返回的值。

```bash
curl -X POST "{BASE_URL}/api/v2/open/material/upload" \
  -H "Authorization: Bearer {YOUR_API_KEY}" \
  -F "file=@person.jpg" \
  -F "personName=张三" \
  -F "bytedToken=20260515143000xxxxxxxx"
```

返回：

```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "materialId": "mat_1778822935_bcd0f95059bd",
    "materialName": "person.jpg",
    "materialType": 1,
    "materialCategory": 1,
    "fileUrl": "https://fc-gw-sh.oss-accelerate.aliyuncs.com/materials/20260515/143000_abcd.jpg",
    "assetUrl": "asset://asset-20260515143020-xxxxx",
    "fileSize": 1281678,
    "personName": "张三",
    "isAsset": true,
    "assetStatus": "Active"
  }
}
```

---

# 二、虚拟素材

虚拟素材用于虚拟人像形象，**无需真人认证**。先创建一个素材组，再向组内上传素材文件。同一虚拟人物的多张素材建议放入同一组，便于在生成视频时保持形象一致。

## 操作流程

1. 创建素材组，拿到 `groupId`。
2. 向素材组中上传虚拟素材文件。
3. 从上传结果或素材列表中拿到 `assetUrl`。
4. 调用 Seedance 2.0 生成视频时，把 `assetUrl` 作为素材 URL 传入。

## 1. 创建素材组

**POST** `/api/v2/open/material/group/create`

```bash
curl -X POST "{BASE_URL}/api/v2/open/material/group/create" \
  -H "Authorization: Bearer {YOUR_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "groupName": "虚拟人A",
    "description": "可选，组的文字描述"
  }'
```

返回：

```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "groupId": "group-20260318033332-xxxxx",
    "groupName": "虚拟人A",
    "description": "可选，组的文字描述",
    "materialCount": 0,
    "createdTime": "2026-03-18T03:33:32Z"
  }
}
```

## 2. 上传虚拟素材

**POST** `/api/v2/open/material/virtual/upload`

`groupId` 必须传第 1 步返回的素材组 ID。

```bash
curl -X POST "{BASE_URL}/api/v2/open/material/virtual/upload" \
  -H "Authorization: Bearer {YOUR_API_KEY}" \
  -F "file=@figure.jpg" \
  -F "groupId=group-20260318033332-xxxxx" \
  -F "materialName=全身正面图"
```

返回：

```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "materialId": "mat_1778822935_bcd0f95059bd",
    "materialName": "全身正面图",
    "materialType": 1,
    "materialCategory": 2,
    "groupId": "group-20260318033332-xxxxx",
    "fileUrl": "https://fc-gw-sh.oss-accelerate.aliyuncs.com/materials/20260318/035710_abcd.jpg",
    "assetUrl": "asset://asset-20260318035710-xxxxx",
    "fileSize": 1281678,
    "isAsset": true,
    "assetStatus": "Active"
  }
}
```

## 3. 素材组管理

查询素材组列表：

**GET** `/api/v2/open/material/group/list?page=1&pageSize=20`

```bash
curl "{BASE_URL}/api/v2/open/material/group/list?page=1&pageSize=20" \
  -H "Authorization: Bearer {YOUR_API_KEY}"
```

更新素材组（组名 / 描述）：

**POST** `/api/v2/open/material/group/update`

```bash
curl -X POST "{BASE_URL}/api/v2/open/material/group/update" \
  -H "Authorization: Bearer {YOUR_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{ "groupId": "group-20260318033332-xxxxx", "groupName": "虚拟人A-改名" }'
```

删除素材组（**会同时删除组内全部素材**）：

**POST** `/api/v2/open/material/group/delete?groupId=group-20260318033332-xxxxx`

```bash
curl -X POST "{BASE_URL}/api/v2/open/material/group/delete?groupId=group-20260318033332-xxxxx" \
  -H "Authorization: Bearer {YOUR_API_KEY}"
```

---

# 三、查询素材列表

**GET** `/api/v2/open/material/list?page=1&pageSize=12`

支持筛选参数：

| 参数               | 说明                                                 |
| ------------------ | ---------------------------------------------------- |
| `materialCategory` | 素材分类：`1`-真人，`2`-虚拟（不传为全部）           |
| `groupId`          | 按素材组筛选（虚拟素材）                             |
| `materialType`     | 素材类型：`1`-图片，`2`-视频，`3`-音频（不传为全部） |
| `keyword`          | 按素材名称模糊搜索                                   |

```bash
# 查询某个虚拟素材组下的素材
curl "{BASE_URL}/api/v2/open/material/list?page=1&pageSize=12&materialCategory=2&groupId=group-20260318033332-xxxxx" \
  -H "Authorization: Bearer {YOUR_API_KEY}"
```

列表中的每一项都会返回 `fileUrl` 和 `assetUrl`。前端预览用 `fileUrl`，提交生成任务用 `assetUrl`。

---

# 四、在 Seedance 2.0 中使用

把素材的 `assetUrl` 放到 `content` 里的素材 URL 字段（真人素材与虚拟素材用法一致）：

```bash
curl -X POST "{BASE_URL}/api/v2/open/aigc/seedance2-0" \
  -H "Authorization: Bearer {YOUR_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "content": [
      {
        "type": "text",
        "text": "图片1中的人物面带笑容，向镜头介绍产品"
      },
      {
        "type": "image_url",
        "image_url": {
          "url": "asset://asset-20260515143020-xxxxx"
        },
        "role": "reference_image"
      }
    ],
    "ratio": "16:9",
    "duration": 5,
    "resolution": "720p"
  }'
```

> Seedance 2.0 Fast（`/api/v2/open/aigc/seedance2-0-fast`）用法相同，同样支持真人素材与虚拟素材的 `assetUrl`。

## 注意事项

- 真人认证二维码有效期较短，过期后重新创建。
- 上传素材可能需要等待处理，前端应显示 loading。
- 生成视频时使用 `assetUrl`，不要使用 `fileUrl`。
- 在提示词中通过「图片1」「视频1」「音频1」按素材在请求体中的顺序引用，不要直接使用 Asset ID。
- 单个上传文件最大 `100MB`。
