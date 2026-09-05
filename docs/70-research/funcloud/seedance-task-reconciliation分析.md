# Seedance 任务卡死分析：RECONCILIATION_REQUIRED

> 分析时间：2026-09-04 18:06
> 问题任务：`task_LwIzuFcoHjdb4rW6KOKZ5QW1KAqzjtxx`
> 状态：RECONCILIATION_REQUIRED（持续 30+ 分钟，客户端仍在轮询）

---

## 一、任务基本信息

| 字段 | 值 |
|---|---|
| 任务ID | `task_LwIzuFcoHjdb4rW6KOKZ5QW1KAqzjtxx` |
| 提交时间 | 2026-09-04 17:34:33 |
| 用户 | userId=91（randy 组，token: yingxiaojiang） |
| 渠道 | **#72** FUNCLOUD-SD2-海外-素材共享 |
| 模型 | `seedance-2-f` → 上游实际模型 `seedance-2` |
| 上游地址 | `https://mm-internal-cn.leonecloud.com`（Funcloud 内网） |
| 操作类型 | `referenceGenerate`（素材参考生成） |
| 分辨率 | 1080p |
| 计费金额 | 1,741,740 quota（per_call 按次计费，matched_tier="1080p"） |
| 当前状态 | **RECONCILIATION_REQUIRED**（已持续 30+ 分钟） |

---

## 二、完整事件时间线

```
17:34:30  用户提交素材上传 (POST /v1/assets) → 201 成功，耗时 8.2s
17:34:32  第二个素材上传 (POST /v1/assets) → 201 成功，耗时 10.6s
17:34:33  视频生成任务提交 (POST /api/v3/contents/generations/tasks) → 200 成功，耗时 528ms
          记录扣费: quota=1,741,740，渠道#72，模型 seedance-2-f
17:34:38  客户端第一次轮询任务状态 (GET /api/v3/contents/generations/tasks/task_LwIzu...)
          ↓
18:06:32  客户端最后一次轮询（日志截断处），仍在轮询中
          ↓
          共计 405+ 次轮询，全部返回 HTTP 200，但任务状态始终为 RECONCILIATION_REQUIRED

18:03:27  尝试拉取视频素材 (POST /v1/assets) → 502，耗时 28.2s
18:03:29  Seedance asset upstream operation failed × 2
18:03:29  POST /v1/assets → 502，耗时 29.7s
18:03:29  POST /v1/assets → 502，耗时 29.8s
18:03:59  GET /v1/assets/219658?model=seedance-2-0-m → 502，耗时 30.0s
```

---

## 三、根因分析（三个问题叠加）

### 问题 1：上游 Funcloud 视频任务失败，进入"账单对账"状态

`RECONCILIATION_REQUIRED` 是 **火山引擎/Volcengine** 体系中的错误状态码，含义是：

> 上游视频生成任务**未成功产出视频**，但已扣费或已占资源，需要进行账单对账（reconciliation）。该任务**已不可恢复**，视频永远不会生成。

触发此状态的常见原因：
- **内容安全审核未通过**（视频素材或 prompt 触发了风控）
- **上游算力资源不足**（GPU 集群排队超时或资源回收）
- **素材格式/分辨率不兼容**（referenceGenerate 对素材有特定要求）

从日志中可以看到，任务提交本身是成功的（上游返回了 task ID），但后续轮询时上游一直返回 `RECONCILIATION_REQUIRED`，说明上游在处理过程中失败了。

### 问题 2：渠道 #72 的 API Key 权限已损坏

每次 newapi 对渠道 #72 进行健康检查时，都返回 **403**：

```
channel_id=72 name=FUNCLOUD-SD2-海外-素材共享
status=403
message: "该 API Key 尚未开通 LLM 能力，请在控制台重新创建 API Key 完成开通"
type: "permission_error"
```

这条错误在整个日志中**反复出现**（每次健康检查周期都会触发）：

| 时间 | 状态 |
|---|---|
| 08/29 12:25:19 | 403 — API Key 尚未开通 LLM 能力 |
| 09/04 15:44:18 | 403 — 同上 |
| 09/04 16:17:19 | 403 — 同上 |
| 09/04 16:52:02 | 403 — 同上 |
| 09/04 17:25:28 | 403 — 同上 |
| 09/04 17:59:26 | 403 — 同上 |

**关键点**：虽然 API Key 的 LLM 能力权限有问题，但**视频任务提交接口（/v1/video/generations）仍然能成功**。这说明 Funcloud 的视频 API 和 LLM API 使用不同的权限体系，视频权限尚在，但 LLM 权限已失效。然而，这可能导致上游在某些内部调用链路中失败。

### 问题 3：素材拉取全部 502 超时

当 newapi 尝试从上游拉取生成的视频素材时：

```
18:03:27  POST /v1/assets → 502 (28.2s)
18:03:29  POST /v1/assets → 502 (29.7s)
18:03:29  POST /v1/assets → 502 (29.8s)
18:03:59  GET /v1/assets/219658 → 502 (30.0s)
```

502 Bad Gateway 说明 **Funcloud 上游根本没有生成视频文件**，所以无法提供素材下载。这与 `RECONCILIATION_REQUIRED` 状态一致——任务失败了，没有产出物。

---

## 四、为什么 newapi 一直轮询不停？

newapi（yuan-new-api）的任务调度器逻辑如下：

1. 每 **~15 秒** 执行一次"任务进度轮询"
2. 查询数据库中所有**未完成**的视频任务
3. 对渠道 #72，始终发现 `pending video tasks: 1`
4. 向上游轮询任务状态，上游返回 `RECONCILIATION_REQUIRED`
5. **newapi 不认为这是一个终态**，继续保留任务为"进行中"
6. 客户端也每 5-10 秒主动轮询一次 GET `/api/v3/contents/generations/tasks/task_LwIzu...`

从 17:34:38 到 18:06:32，**32 分钟内轮询了 405+ 次**，全部返回 HTTP 200，但任务永远无法完成。

这是一个 **newapi 的 bug 或设计缺陷**：它没有将 `RECONCILIATION_REQUIRED` 视为终态（如 `failed` 或 `cancelled`），导致死循环轮询。

---


## 六、建议处理方案

### 立即处理

1. **联系 Funcloud 客服**，说明渠道 #72 的 API Key 需要重新开通 LLM 能力权限，同时确认该视频任务的上游失败原因
2. **该任务已不可恢复**，需要重新提交一次视频生成请求（建议切换到渠道 #97 火山官方或 #81 火山官方，如果 API Key 已修复的话）

### 长期优化

3. 在 newapi 的 Seedance 任务轮询逻辑中，**增加 `RECONCILIATION_REQUIRED` 作为终态处理**，避免无限轮询
4. 增加任务超时机制（如超过 30 分钟未完成则自动标记为 failed）
5. 定期检查各 Seedance 渠道的 API Key 有效性，及时发现权限问题
