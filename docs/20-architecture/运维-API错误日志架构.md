---
status: current
owner: Dev Team
last-reviewed: 2026-09-19
---

# API 错误日志架构

## 范围与边界

本专题覆盖两类服务端错误可观测出口：

1. **已鉴权 4xx 统一诊断**：凡正式鉴权成功放行后最终返回 4xx 的 API 请求，恰好产生一条可按
   `request_id` 查询的统一诊断（WARN 日志，类型 `authenticated_api_client_error`）。仅观察、
   非阻断、失败隔离，不改变业务响应。
2. **error_events 持久化与管理员查询**：relay/asset 已鉴权请求最终 4xx/5xx 的持久化事件，另含
   三类扩展采集（渠道测试失败、SSE 流式异常、异步任务失败）与独立管理菜单。

不承担：小时级系统运行与错误邮件报告（见[系统错误报告架构](运维-系统错误报告架构.md)）、音视频
请求证据（见[音视频请求证据架构](音视频请求证据架构.md)）、客户账单与对账投影。

## 职责与事实源

- `clienterrlog/`：请求级鉴权放行标记（`MarkAuthPassed`，接线 TokenAuth 与 Session/PAT/
  TryUserAuth/capability 放行点）、统一出口判定、有界队列、WARN 与持久化双出口、健康计数。
- `model/error_event*`：事件持久化、`event_type`/`task_id` 列、四方言 JSON 状态过滤表达式、
  启动回填；ClickHouse 为手写 DDL。
- `controller/error_log*` 与前端错误日志独立菜单：`AdminAuth` 查询入口。
- 异步任务失败经 `model/task_error_event` 安全投影；渠道测试经 `service/channel_test_error_event`
  合成 method/route/protocol。

排除场景：匿名请求（无已鉴权身份）不产生诊断；部分有意排除（如非法指定渠道 400）在放行点即不打标。

## 数据与控制流

- 放行点打标 → 请求结束统一出口判定（已鉴权 + 最终 4xx）→ 入队（1024 有界，满载丢弃并计数，
  不排空等待）→ WARN 出口与持久化出口分别处理；两出口各自恢复 panic 并维护独立计数，WARN 失败
  不阻断持久化，持久化失败不影响业务。Root 健康端点暴露双出口计数。
- relay 最终错误在 defer 中单次 Attach；SSE 流式异常单行 defer + 五类分类，4xx 合并不重复记录；
  异步任务失败窄接线 + CAS 防重，`fail_reason` 走 `PublicTaskErrorMessage` 安全投影；渠道测试
  后台与手工测试共用提交入口，method/纯路径/协议在读取上游响应处冻结。
- 查询：`GET /api/error_log/`（AdminAuth），状态过滤在数据库层用四方言表达式先过滤后分页。

## 约束与取舍

- 隐私边界：持久化详情走白名单键与键数上限；`error_events.detail.http_exchange` 保存有界脱敏
  JSON 快照，与普通文本日志、邮件正文隔离（见[系统错误报告架构](运维-系统错误报告架构.md)）。
  快照范围与"不保存用户提示词"既有承诺的边界正在重新裁决（见
  [80 目录未完成事项汇总](../50-planning/2026-09-19-80目录未完成事项汇总.md)），本文不预断结果。
- 原生文件最小侵入：NEWAPI 原生路径仅保留单行或窄分支接线，主体位于 `clienterrlog/` 与账单/
  错误日志本地扩展文件。
- 已知边界：error_events 当前无保留 TTL 与清理任务；WARN 输出与持久化在同一 worker 内串行，
  WARN 出口阻塞会拖慢持久化出口；启动回填对大表有随存量线性增长的开销。对应跟进项见
  [80 目录未完成事项汇总](../50-planning/2026-09-19-80目录未完成事项汇总.md)。
