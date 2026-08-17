# FunCloud 海外渠道执行记录

## 1. `seedance-2-funcloud`

### 1.1. 环境与请求

- 验证环境：本地运行实例，SQLite 主数据库 `one-api.db`
- Provider 线路：FunCloud 海外；渠道 ID `61`
- 渠道名称：`SD FunCloud Seedance 2 Standard`
- 北向合同：ModelArk V3（`newapi_modelark_video_v3`）
- 南向协议：`funcloud_seedance`
- 客户模型：`seedance-2-funcloud`
- Provider 请求模型：`seedance-2`
- 平台 Task ID：`task_k5MKVh6TRIRsovARow2Yi8GXoCTqmDNb`
- 平台请求时间：`2026-08-14 23:38:01`
- FunCloud Task ID：`task_20260814233804_3bnirqcu`

本次请求为纯文本、4 秒、480p、4:3，不生成音频：

```json
{
  "model": "seedance-2-funcloud",
  "content": [
    {
      "type": "text",
      "text": "美丽的大自然"
    }
  ],
  "duration": 4,
  "ratio": "4:3",
  "resolution": "480p"
}
```

### 1.2. 运行结果

| 核验项 | 本次任务事实 |
| --- | --- |
| 运行状态 | `SUCCESS / 100%` |
| Worker 开始时间 | `2026-08-14 23:38:04` |
| FunCloud 完成时间 | `2026-08-14 23:40:07` |
| 平台完成时间 | `2026-08-14 23:40:10` |
| FunCloud 耗时 | `123` 秒 |
| 计费方式 | 按 Token |
| 预估 Token | `38,430` |
| 实际 Token | `39,891` |
| 计费状态 | `settled` |
| 运行结果 | 视频生成成功，结果引用已写入 Task。 |

本次保存 FunCloud 控制台的实际结果，以及平台适配器转换后写入 Task 的归一化结果。媒体 URL 不写入对账文档。

#### 1.2.1. FunCloud 上游实际输出

| 字段 | 实际值 |
| --- | --- |
| 任务 ID | `task_20260814233804_3bnirqcu` |
| 模型 | `seedance2` |
| 类型 | 视频 |
| 意图 | `t2v` |
| 分辨率 | `480p` |
| 时长 | `4s` |
| 计费方式 | 按 Token |
| 预估 Token | `38,430` |
| 实际 Token | `39,891` |
| 状态 | 成功 |
| 创建时间 | `2026-08-14 23:38:04` |

南向查询结果同时返回：

```json
{
  "taskId": "task_20260814233804_3bnirqcu",
  "status": "success",
  "progress": 100,
  "completionTokens": 39891,
  "pointConsume": "0.228700",
  "updatedAt": "2026-08-14 23:40:07",
  "costTime": 123,
  "resultCount": 1
}
```

#### 1.2.2. 平台归一化 Task 结果

```json
{
  "id": "task_20260814233804_3bnirqcu",
  "status": "succeeded",
  "content": {
    "video_url": "[媒体结果已写入 Task，不在对账文档保存]"
  },
  "usage": {
    "completion_tokens": 39891,
    "total_tokens": 39891
  }
}
```

平台直接采用上游 `completionTokens = 39,891` 作为实际 Token 用量，不再根据 `pointConsume` 反推。

### 1.3. 请求分类与账单

“我方单价”取平台为渠道 61 设置的 BytePlus 官方无折扣价格；“我方总价”按上游实际返回的
`39,891 completionTokens`计算；“上游总价”取南向查询返回的 `pointConsume`。人民币按
`1 USD = 6.82 RMB`换算。

| 生成模式 | 我方单价 | 我方总价 | 上游单价 | 上游总价 | 官方单价 | 相对官方比例 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 文生视频<br>480p<br>4 秒<br>不生成音频 | USD：`$7.00/1M tokens`<br>RMB：`¥47.74/1M tokens` | Tokens：`39,891`<br>USD：`$0.279237`<br>RMB：`¥1.904396`<br>Quota：`139619` | USD：`$5.733123/1M tokens`<br>RMB：`¥39.099897/1M tokens` | Tokens：`39,891`<br>USD：`$0.228700`<br>RMB：`¥1.559734` | USD：`$7.00/1M tokens`<br>RMB：`¥47.74/1M tokens` | `81.9018%` |

上游单价按本次实际 Token 与实际总价折算：

```text
$0.228700 ÷ 39,891 × 1,000,000
= $5.733123 / 1M tokens
```

平台金额满足：

```text
39,891 × $7.00 ÷ 1,000,000
= $0.279237
```

平台按 `500,000 quota/USD`换算并取整，实际结算为 `139619 quota`。

### 1.4. FunCloud 上游账单

| 计费方式 | 预估 Token | 实际 Token | 实际扣费 | 折算实际单价 |
| --- | ---: | ---: | ---: | ---: |
| 按 Token | `38,430` | `39,891` | `$0.228700`<br>`¥1.559734` | `$5.733123/1M tokens`<br>`¥39.099897/1M tokens` |

上游金额按本次任务数据可以对上：

```text
39,891 × $5.733123 ÷ 1,000,000
≈ $0.228700
```

实际 Token 比预估 Token 多 `1,461`，高 `3.8017%`；最终结算使用实际 Token，不使用预估 Token。

### 1.5. 阶段性结论

1. 渠道 61 的 480p、4 秒文生视频请求成功完成，Task 已进入 `settled`。
2. 上游实际返回 `completionTokens = 39,891`，平台归一化结果与最终结算均使用 `39,891 tokens`。
3. 我方按 BytePlus 官方价 `$7.00/1M tokens`结算，金额为 `$0.279237 / 139619 quota`。
4. **账单可以对上：**上游 Token、平台归一化 Token 和平台计费 Token 完全一致；平台金额也符合官方单价计算结果。
