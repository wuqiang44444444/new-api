---
status: reference
owner: Dev Team
last-reviewed: 2026-09-23
retrieved-at: 2026-09-23T15:42:30+00:00
source-url: https://docs.jdcloud.com/cn/jdaip/hailuo-text-to-video
source-content-sha256: 19bd33b200e9393a62cef96f40d63e46f007d9793188a988291212d84bd30db8
---

# 京东云海螺文生视频API原文

## 来源与保存边界

- 官方来源：[京东云 JoyBuilder 模型开发平台 2.0](https://docs.jdcloud.com/cn/jdaip/hailuo-text-to-video)。
- 保存方式：从官方页面内嵌的文档 `content` 提取 Markdown 正文，仅解码网页中的字符串转义；
  下方原文未修订标题、表格、拼写、HTTP 示例地址、示例模型或参数。
- 原文中的 Key 和任务 ID 是官方文档示例，不是本次测试凭据或真实任务标识。
- 页面目前以 `MiniMax-Hailuo-2.3` 为例，不能把其中的枚举、时长和计费描述直接视为 H3 合同。
- 本地实测与适配解释另见[MiniMax 标准北向与京东云南向适配方案](../../80-dev/2026-09-23-MiniMax标准北向与京东云南向适配方案.md)，不回写原文。
- `source-content-sha256` 对下方原文字串的 UTF-8 字节计算；不包含本文件 frontmatter 和来源说明。

## 官方原文开始

## 文生视频

> **注意⚠️：**
> **所有任务，提交结束后，建议至少 15分钟来查询一次任务状态；**
> **任务一旦成功，结果信息只保存 90分钟；**

### 创建任务

| **请求地址** | **请求方法** | **请求格式** | **响应格式** |
| ---- | ---- | ---- | ---- |
| http://modelservice.jdcloud.com/v1/task/submit | POST | application/json | application/json |

#### 请求头

| **字段** | **值** | **描述** |
| --- | --- | --- |
| Content-Type | application/json | 数据交换格式 |
| Authorization | 鉴权信息，参考接口鉴权 | 鉴权信息，参考接口鉴权 |

#### 请求体

| **字段** | **二级字段** | **三级** | **类型** | **必填** | **含义** |
| --- | ---- | --- | --- | --- | --- |
| model | - |  | string | 是 | 模型调用值，可选值：MiniMax-Hailuo-2.3 |
| content |  |  | array[object] | 是 |  |
| @rows=2: | type |  | string | 是 | 内容类型<br>text:相当于提示词文本类型 |
| text |  | string | 否 | 提示词文本 |
| parameters |  |  | object | 否 |  |
| @rows=4: | duration |  | int | 否 | 时长秒数，仅支持 6，10两种<br>默认 6s |
| prompt\_optimizer |  | bool | 否 | 是否自动优化 `prompt`，默认为 `true`。设为 `false` 可进行更精确的控制 |
| resolution |  | string | 否 | 视频分辨率<br>默认768P<br>MiniMax-Hailuo-2.3:<br>6秒可用分辨率为768P和1080P<br>10秒可用分辨率为768P |
| watermark |  | bool | 否 | 是否在生成的视频中添加水印，默认为 `false` |

#### 调用示例

```
curl --location '{domain}/v1/task/submit' \
--header 'Content-Type: application/json' \
--header 'Trace-id: xxxxxx' \
--header 'Authorization: Bearer pk-*********' \
--data '{
    "model": "MiniMax-Hailuo-2.3",
    "content": [
        {
            "type": "text",
            "text": "生成一女孩跟狐狸跳舞的视频"
        }
    ],
    "parameters": {
        "duration": 6,
        "prompt_optimizer":true,
        "resolution":"768P",
        "watermark":true
    }
}'
```

### 提交任务返回结果

```
{
    "error": null,
    "requestId": "fghfghdfs234234523-SElrd",
    "result": {
        "message": "任务提交成功",
        "status": "pending",
        "task_id": "task-97399k5h92r4kxv"
    }
}
```

### 查询任务

#### 请求体

| **字段** | **二级字段** | **三级** | **类型** | **含义** |
| --- | ---- | --- | --- | --- |
| task\_status |  |  | string | 任务状态：<br>pending：待运行<br>running：运行中<br>success：成功<br>failed：失败<br>cancelled：被取消 |
| content |  |  | array[object] |  |
| @rows=7: | id |  | string | 生成物id，用来标识不同的生成物 |
| @rows=2:watermarked\_url |  | object | 水印类型 |
| url | string | 带水印的生成物url，90分钟有效期 |
| @rows=2:cover\_url |  | object | 图片类型 |
| url | string | 生成物封面，90分钟有效期 |
| @rows=2:video\_url |  |  | 视频类型 |
| url | string | 生成物URL， 90分钟有效期 |
| @rows=2:usage |  |  | object |  |
| video\_output |  | int | 消耗积分 |
| error |  |  | object | 异常信息 |
| @rows=3: | code |  | int | 错误码 |
| type |  | string | 错误类型 |
| message |  | string | 错误信息 |

#### 调用示例

```
curl --location --request GET '{domain}/v1/task/task-daxxx9' \
--header 'Content-Type: application/json' \
--header 'Trace-id: *******' \
--header 'Authorization: Bearer pk-********'
```

### 取消任务

#### 请求头

| **字段** | **值** | **描述** |
| --- | --- | --- |
| Content-Type | application/json | 数据交换格式 |
| Authorization | 鉴权信息，参考接口鉴权 | 鉴权信息，参考接口鉴权 |

#### 调用示例

```
curl --location --request DELETE '{domain}/v1/task/task-byaxxxxxj' \
--header 'Content-Type: application/json' \
--header 'Trace-id: xxxxxx' \
--header 'Authorization: Bearer pk-*****'
```

#### 响应体

```
# 示例1
{
    "error": {
        "cause": "",
        "code": 400,
        "message": "任务已处于终态，无法取消",
        "status": "FAILED_PRECONDITION"
    },
    "requestId": "wefwefwe43543-OVRwS",
    "result": null
}

# 示例2
{
    "error": null,
    "requestId": "wefwefwe43543-SENMa",
    "result": {
        "task_id": "task-850mlryn0m",
        "task_status": "cancelled",
        "message": "任务取消成功",
    }
}
```