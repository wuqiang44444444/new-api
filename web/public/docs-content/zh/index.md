---
page-id: overview
kind: guide
last-verified: 2026-07-29
operations: []
---

# API 文档概述

{{SYSTEM_NAME}} 提供统一的 AI API 入口。你可以按现有客户端所使用的协议接入文本、图片和视频模型，也可以通过素材 API 管理可复用的媒体资源。

## 从这里开始

第一次接入建议依次阅读：

1. [快速开始](quickstart)
2. [鉴权](authentication)
3. [Base URL](base-url)
4. [错误与重试](concepts/errors)

文档中的 Base URL 会根据当前部署动态显示。API Key 和模型 ID 始终是固定占位符，不会读取你的登录信息或自动填充真实凭据。

## 能力范围

| 能力     | 推荐入口                                                | 说明                      |
| -------- | ------------------------------------------------------- | ------------------------- |
| 模型发现 | `GET /v1/models`                                        | 查询当前 Key 可访问的模型 |
| 文本     | `/v1/chat/completions`、`/v1/responses`、`/v1/messages` | 选择与你的 SDK 相同的协议 |
| 图片     | `/v1/images/generations`、`/v1/images/edits`            | 支持同步响应和异步任务    |
| 视频     | ModelArk、Kling、即梦北向合同                           | 不同协议的字段不能混用    |
| 素材     | `/v1/assets`                                            | 管理可复用素材及其绑定    |

## 兼容性边界

不同供应商协议只在明确说明的入口上兼容。不要把某个渠道的私有字段加入通用请求，也不要依赖未在本文档公开的管理接口、内部任务字段或历史兼容入口。

## 示例约定

- `{{API_KEY_PLACEHOLDER}}` 表示你自行创建的 API Key。
- `{{MODEL_ID_PLACEHOLDER}}` 表示模型列表返回且当前 Key 有权访问的模型 ID。
- 示例域名、素材 ID 和任务 ID 都是占位值。
- 示例默认使用 JSON；图片编辑等页面会明确标注 multipart。
