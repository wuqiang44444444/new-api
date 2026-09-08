---
page-id: model-parameters
kind: guide
last-verified: 2026-09-09
operations: []
---

# 模型与参数

先选公开模型和操作，再根据该模型的参数表构造请求。本文解释模型目录中的机器可读字段；每种接口的
完整请求与响应仍以对应 API Reference 为准。

## 1. 确认当前模型可调用

调用 `GET /v1/models` 或 `GET /v1/models/{model}`，使用相同 API Key。模型名放进 URL 路径时应进行
路径编码，请求 JSON 中则保留原始 ID。目录可能列出不可用模型：返回 `available` 时必须为 `true`，
否则查看 `availability`，不要仅凭列表中存在就提交生成。

| 字段 | 如何使用 |
| --- | --- |
| `id` | 请求中使用的客户模型 ID，不自行改名或追加后缀 |
| `supported_endpoint_types` | 已公开的接口类型，不能据此猜测另一协议的可用性 |
| `api.image.creation` | 图片生成方法、路径、内容类型和参数 |
| `api.image.edit` | 图片编辑输入；不能用生成参数表代替 |
| `api.image.async` | 图片显式异步请求头、查询路径和流式优先规则 |
| `api.video.protocol` / `creation` | 视频协议及创建合同 |
| `api.assets` | 素材操作、媒体类型、管理模式和复用域 |
| `operations[]` | 找到目标 `operation`，确认 `supported=true`，使用对应方法和路径 |

缺少某项元数据不等于自动支持该能力，也不要把文本模型没有 `api.image` 解释为文本不可用。

## 2. 读取参数约束

| 参数合同字段 | 含义与处理 |
| --- | --- |
| `required_fields` | 请求的必填字段；有点号时为嵌套字段 |
| `required_one_of` | 多个输入字段中恰好选择一个，例如 JSON 编辑的 `image` 与 `images` |
| `parameters[].required` | 对应参数是否必填 |
| `type` / `item_type` | JSON 类型与数组元素类型；表单中的数值和布尔值按文档使用文本 |
| `fixed_value` | 字段被固定；显式发送时必须使用该值 |
| `default_value` | 省略时采用的值；不是所有模型共享的默认值 |
| `enum` | 允许的值集合，大小写与类型都应保持一致 |
| `minimum` / `maximum` | 数值范围 |
| `special_values` | 明确允许的特殊值；不能由范围自行推导，例如视频的自动时长值 |
| `min_length` / `max_length` | 字符串长度约束 |
| `min_items` / `max_items` | 数组长度约束；参考图数量与输出图片数量分开计算 |
| `additional_properties=false` | 不发送参数表未发布的字段 |

`fixed_value=false`、`default_value=0` 都是有效值。程序应判断字段是否存在，不用“是否为真”决定是否读取。
`false`、`0`、空字符串与省略不等价；可选字段没有值时省略，不统一发送 `null`。

带点的参数名表示嵌套位置。若模型发布 `extra_fields.resolution`，JSON 应写成
`extra_fields` 对象中的 `resolution` 字段，不能发送名为 `"extra_fields.resolution"` 的顶层键。
`extra_fields` 也不是任意字段透传入口，只填写模型公开的子字段。

## 3. 区分通用上限与模型规格

公开 API 的总参数表表示各模型支持字段的范围，不能证明某个模型支持整张表。
例如图片数量有公共安全上限，某个模型仍可能固定 `n=1`；参考图最多 14 张的输入规范也可能被模型收紧。

第一次调用优先只填必需字段，再逐项加入尺寸、质量、数量等选项。示例中的 `720p`、`16:9`、时长和数量
都需要核对当前模型，不应当作所有模型的通用组合。

部分视频字段带生效条件，例如是否返回末帧、是否输出音频或使用哪种参考图角色。字段已发布也不表示
每种输入组合都产生相同效果；请同时阅读对应视频参数表中的条件，客户无需按内部服务改变请求形状。

## 4. 保存调用上下文

每个任务保存客户模型、协议、任务 ID、Key 的内部标识和创建时选定的参数。不要保存 Key 明文到任务日志。
配置或价格变化后仍查询原任务，不重新选择模型来解释已经发生的请求。

素材另外保存 `model + id + reference`。判断跨模型可尝试复用时比较完整、非空的 `reuse_scope`，
不能比较界面短标签、模型名或资源前缀。详情见[素材与素材组](api-reference/assets)。
