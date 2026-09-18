package service

import (
	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/model"
)

// 错误事件责任分类与状态用语（开发方案第 3 节）。分类只复用采集层已结构化的
// 事件类型与原因码，不按 HTTP 状态或错误文本关键词猜测根因：上游账号失效、
// 模型权限或渠道额度不足也可能返回 4xx，缺少证据时保留未知。

const (
	errorReportClassClient      = "client"
	errorReportClassPlatform    = "platform"
	errorReportClassUnknown     = "unknown"
	errorReportClassStream      = "stream"
	errorReportClassChannelTest = "channel_test"
)

// errorReportClientReasons 是已核对的客户侧原因码白名单（来自 middleware/
// asset service/relay adapter 的采集点）。新增客户侧原因码时应同步本表。
var errorReportClientReasons = map[string]bool{
	"token_model_forbidden":       true,
	"unsafe_url":                  true,
	"content_type_mismatch":       true,
	"source_too_large":            true,
	"invalid_image":               true,
	"invalid_request":             true,
	"unsupported_asset_operation": true,
	"resource_not_found":          true,
	"client_disconnected":         true,
}

// classifyErrorEvent 返回事件的责任分类。
func classifyErrorEvent(event *model.ErrorEvent) string {
	switch event.EventType {
	case clienterrlog.EventChannelTest:
		return errorReportClassChannelTest
	case clienterrlog.EventStreamError:
		// Event type remains available in by_type; responsibility follows
		// captured reason codes so the summary and responsibility counts agree.
		if errorReportClientReasons[event.Reason] {
			return errorReportClassClient
		}
		if event.Reason == "upstream_response_failed" || event.Reason == "stream_handler_panic" {
			return errorReportClassPlatform
		}
		return errorReportClassStream
	case clienterrlog.EventTaskFailure:
		if errorReportClientReasons[event.Reason] {
			return errorReportClassClient
		}
		if event.Status >= 500 {
			return errorReportClassPlatform
		}
		return errorReportClassUnknown
	default:
		// api_error 及空类型（历史行按 api_error 归类）。
		if errorReportClientReasons[event.Reason] {
			return errorReportClassClient
		}
		if event.Status >= 500 {
			return errorReportClassPlatform
		}
		return errorReportClassUnknown
	}
}

// errorReportStatusTexts 是运行摘要的可解释状态用语，不建任意阈值健康分数。
type errorReportStatusTexts struct {
	// Headline 进入邮件标题的短句；Body 是摘要正文说明。
	Headline string
	Body     string
}

// resolveErrorReportStatus 按已确认规则产出状态用语：
//   - 平台/上游证据优先；只有客户侧错误时明确说「未在本次数据中发现」；
//   - 原因不明时保留待分类，不归责任何一方；
//   - 无错误时区分「有活动证据」与「数据不足」，绝不显示绿色「系统正常」。
func resolveErrorReportStatus(total int, counts map[string]int, perfHasActivity bool, perfNote string) errorReportStatusTexts {
	switch {
	case counts[errorReportClassPlatform] > 0:
		return errorReportStatusTexts{
			Headline: "发现平台／上游相关异常 / Platform / upstream errors detected",
			Body:     "本次窗口内已入库错误事件中包含明确的平台／上游服务错误（HTTP 5xx 或已登记的流式失败原因），影响范围与次数见下方分类汇总，逐条明细见正文。 / Persisted events in this window include explicit platform or upstream errors (HTTP 5xx or registered stream failure reasons). See the summaries for scope and counts, and the full details below.",
		}
	case counts[errorReportClassUnknown] > 0:
		return errorReportStatusTexts{
			Headline: "存在待分类错误，需排查 / Unclassified errors require investigation",
			Body: "本次窗口内存在原因不明的已入库错误，未默认归责客户或平台／上游，" +
				"需要人工排查分类，逐条明细见正文。 / Persisted errors with unknown causes require manual classification. Responsibility is not assigned to clients, the platform or upstream providers by default. Full details follow.",
		}
	case counts[errorReportClassStream]+counts[errorReportClassChannelTest] > 0:
		return errorReportStatusTexts{
			Headline: "发现流式或渠道测试异常 / Stream or channel test errors detected",
			Body:     "本窗口存在流式异常或渠道测试失败；事件类型本身不能确定责任归属，请结合原因码与逐条明细排查。 / Stream or channel test errors occurred in this window. Event type alone does not establish responsibility; investigate the reason codes and individual details.",
		}
	case counts[errorReportClassClient] > 0:
		return errorReportStatusTexts{
			Headline: "已记录客户侧错误 / Client-side errors recorded",
			Body: "本次窗口内已入库错误均为已识别的客户侧原因，" +
				"未在本次数据中发现明确的平台／上游故障；明细仍完整展示。 / All persisted errors in this window have identified client-side reasons. No explicit platform or upstream failure was found in this data. Full details are included.",
		}
	case total == 0 && perfHasActivity:
		return errorReportStatusTexts{
			Headline: "本次已入库记录中未发现错误 / No errors found in persisted records",
			Body: "截至生成时刻，本窗口内已入库错误事件为零；模型统计覆盖范围见下方说明，" +
				"该结论仅限已采集口径，不代表未接入路径。 / No errors were persisted for this window as of generation time. The conclusion applies only to collected data, not uncovered paths. Model sampling coverage is described below.",
		}
	default:
		return errorReportStatusTexts{
			Headline: "数据不足，无法判断 / Insufficient evidence to assess status",
			Body: "本窗口内缺少可靠的活动/采集证据（无已入库错误事件，且模型统计未覆盖或无数据）；" +
				"不能把缺少记录等同于系统正常。 / There is insufficient activity or collection evidence: no persisted errors, and performance statistics are unavailable or empty. Missing records do not prove that the system is healthy.",
		}
	}
}
