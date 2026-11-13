---
page-id: overview
kind: guide
last-verified: 2026-09-09
operations: []
---

# API 文档概述

{{SYSTEM_NAME}} 提供统一的 AI API 入口。你可以按现有客户端所使用的协议接入文本、图片和视频模型，也可以通过素材 API 管理可复用的媒体资源。

## 从这里开始

第一次接入建议依次阅读：

1. [快速开始](quickstart)
2. [鉴权](authentication)
3. [Base URL](base-url)
4. [模型与参数](concepts/model-parameters)
5. [错误与重试](concepts/errors)
6. [图片与视频调用实战](guides/media-workflow)

示例使用当前站点的 Base URL。调用前请替换 API Key 和模型 ID 占位符。

## 能力范围

| 能力     | 推荐入口                                                | 说明                                           |
| -------- | ------------------------------------------------------- | ---------------------------------------------- |
| 模型发现 | `GET /v1/models`                                        | 查询模型入口、可用状态与逐模型参数合同         |
| 文本     | `/v1/chat/completions`、`/v1/responses`、`/v1/messages` | 选择与你的 SDK 相同的协议                      |
| 图片     | `/v1/images/generations`、`/v1/images/edits`            | 默认同步；支持时显式选择异步任务或流式         |
| 视频     | ModelArk V3、OpenAI Videos、Kling、即梦                 | 不同协议的字段不能混用                         |
| 素材     | `/v1/assets`                                            | 账号级托管库接收图片、MP3 和 MP4。调用方保存 ID 与引用 |

## 示例约定

- `{{API_KEY_PLACEHOLDER}}` 表示你自行创建的 API Key。
- `{{MODEL_ID_PLACEHOLDER}}` 表示模型列表返回且当前 Key 有权访问的模型 ID。
- 示例域名、素材 ID 和任务 ID 都是占位值。
- 示例默认使用 JSON；图片编辑等页面会明确标注 multipart。
