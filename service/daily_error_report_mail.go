package service

import (
	"cmp"
	"fmt"
	"html"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

type dailyReportSummary struct {
	Signals             map[string]int64              `json:"signals"`
	Groups              []model.DailyReportErrorGroup `json:"groups"`
	RetentionGapThrough int64                         `json:"retention_gap_through,omitempty"`
	Approximate         bool                          `json:"approximate,omitempty"`
}

func (summary *dailyReportSummary) retainGroups(counts map[model.DailyReportErrorKey]model.DailyReportErrorGroup) error {
	// MySQL TEXT holds 65,535 bytes. Leave room for the fixed signal
	// counts, JSON framing and future summary fields.
	const groupBudget = 48 << 10
	groups := make([]model.DailyReportErrorGroup, 0, len(counts))
	for _, group := range counts {
		groups = append(groups, group)
	}
	slices.SortFunc(groups, func(a, b model.DailyReportErrorGroup) int {
		return cmp.Or(cmp.Compare(b.Count, a.Count), cmp.Compare(a.ChannelID, b.ChannelID),
			cmp.Compare(a.EventType, b.EventType), cmp.Compare(a.Status, b.Status), cmp.Compare(a.ModelName, b.ModelName), cmp.Compare(a.Reason, b.Reason))
	})
	summary.Groups = nil
	retained := make(map[string]int)
	bytesUsed := 0
	for _, group := range groups {
		signal := group.Signal()
		if retained[signal] >= 12 {
			summary.Approximate = true
			continue
		}
		encoded, err := common.Marshal(group)
		if err != nil {
			return err
		}
		if bytesUsed+len(encoded)+1 > groupBudget {
			summary.Approximate = true
			continue
		}
		summary.Groups = append(summary.Groups, group)
		bytesUsed += len(encoded) + 1
		retained[signal]++
	}
	return nil
}

type dailyReportOperations struct {
	tasks, resources []model.DailyReportCount
	incidents        []model.ChannelAlertEpisode
	incidentCount    int64
	channelNames     map[int]string
	quotaAccounts    map[string]int64
	asOf             time.Time
	details          mailErrorDetails
}

func renderDailyReport(report *model.ErrorReport, day string, summary dailyReportSummary, ops dailyReportOperations) *model.ErrorReportPart {
	const title = "运维日报 / Daily operations report"
	var body strings.Builder
	fmt.Fprintf(&body, `<p style="margin:0 0 10px;color:#49627a">%s · 00:00–24:00 · 北京时间 / Beijing time</p>`, html.EscapeString(day))
	triageTitle, triageBG, triageAccent := "记录摘要 / Recorded activity", "#eef4f8", "#446079"
	analysis := "窗口内未记录错误或已确认渠道事件。<br>No errors or confirmed channel incidents were recorded for this window."
	incomplete := summary.RetentionGapThrough > report.WindowStart
	if incomplete {
		gapEnd := time.Unix(summary.RetentionGapThrough, 0).In(errorReportLocation()).Format("01-02 15:04:05")
		fmt.Fprintf(&body, `<p style="padding:10px 12px;background:#fff4df;color:#713d09"><strong>错误日志覆盖不完整 / Incomplete error log coverage</strong><br>保留期影响窗口截至 %s 的记录，错误总量未知。<br>Retention affects the window through %s (Beijing time). The total error count is unknown.</p>`,
			gapEnd, gapEnd)
		triageTitle = "错误状况待核实 / Error activity unknown"
		analysis = "现有日志不足以判断此窗口是否发生过错误。<br>Available logs cannot establish whether errors occurred in this window."
	}
	switch {
	case ops.incidentCount > 0:
		triageTitle, triageBG, triageAccent = "有已确认渠道异常 / Confirmed channel incidents", "#fff1ee", "#9b392c"
		analysis = "已有渠道异常达到确认条件。先核对仍未关闭的记录，再查同渠道的客户调用。<br>Channel incidents met confirmation criteria. Review open incidents first, then customer calls on those channels."
	case summary.Signals[model.DailyErrorAPI5xx]+summary.Signals[clienterrlog.EventStreamError]+summary.Signals[clienterrlog.EventTaskFailure] > 0:
		triageTitle, triageBG, triageAccent = "优先核对客户失败 / Review customer failures", "#fff4df", "#9a5b1b"
		analysis = "已有客户 API 5xx、需排查的流式异常或任务失败的记录。优先核对对应渠道和原因，确认影响范围。<br>Review recorded API 5xx, stream errors requiring investigation or failed tasks first to establish customer impact."
	case summary.Signals[clienterrlog.WalletQuotaInsufficient]+summary.Signals[clienterrlog.SubscriptionQuotaInsufficient]+summary.Signals[clienterrlog.APIKeyQuotaInsufficient] > 0:
		triageTitle = "核对额度不足 / Review quota rejections"
		analysis = "请求被本站的资金来源或 API Key 限额阻断。请按下方分类处理。<br>Requests were blocked by account funding or API key limits. Review the breakdown below."
	case summary.Signals[model.DailyErrorAPI429]+summary.Signals[model.DailyErrorAPI4xx] > 0:
		triageTitle, triageBG, triageAccent = "核对限流与请求拒绝 / Review throttling and rejections", "#fff4df", "#9a5b1b"
		analysis = "已记录客户请求被限流或拒绝。检查 429 的限流来源及其他 4xx 的鉴权、参数或策略原因。<br>Review the source of throttling for 429s and authentication, input or policy causes for other 4xx responses."
	case summary.Signals[clienterrlog.EventChannelTest] > 0:
		triageTitle = "核对渠道测试 / Review channel tests"
		analysis = "本窗口记录到巡检失败。检查是否只影响测试，或能在同渠道找到实际客户失败。<br>Check whether probe failures affected testing only or coincide with actual customer failures on the same channel."
	case summary.Signals[model.DailyErrorStreamDisconnected]+summary.Signals[model.DailyErrorStreamWarning] > 0:
		analysis = "已记录流式取消、断开或软错误。核对流式结束原因和响应详情后再判断影响。<br>Review stream cancellations, disconnections or warnings and their response details before judging impact."
	case report.TotalEvents > 0:
		analysis = "存在未归类的错误事件，请查看后台记录。<br>Review unclassified events in the error log."
	}
	if len(ops.tasks)+len(ops.resources) > 0 {
		analysis += "<br>另有运维任务失败或资源超限，见下方。 / Review the operations failures and resource alerts below."
	}
	fmt.Fprintf(&body, `<table role="presentation" width="100%%" cellspacing="0" cellpadding="0" style="margin:0 0 16px"><tr><td style="padding:13px 15px;background:%s;border-left:4px solid %s">
<h3 style="margin:0 0 6px;font-size:16px;color:#14283f">%s</h3><p style="margin:0">%s</p>
</td></tr></table>`, triageBG, triageAccent, triageTitle, analysis)

	body.WriteString(`<table width="100%" cellspacing="0" cellpadding="0" style="table-layout:fixed;background:#f5f8fb;border:1px solid #dce4ee;margin:0 0 14px;word-break:break-word">`)
	for _, metric := range []struct {
		zh, en string
		value  string
	}{
		{"确认渠道异常", "Confirmed incidents", strconv.FormatInt(ops.incidentCount, 10)},
		{"客户 API 5xx", "Customer API 5xx", dailyReportErrorCount(summary.Signals[model.DailyErrorAPI5xx], incomplete)},
		{"已记录错误", "Recorded errors", dailyReportErrorCount(int64(report.TotalEvents), incomplete)},
	} {
		fmt.Fprintf(&body, `<tr><th scope="row" align="left" style="padding:6px 12px;font-size:13px;border-bottom:1px solid #e5edf3;color:#26394f">%s <span style="font-weight:400;color:#49627a">/ %s</span></th><td width="21%%" align="right" style="padding:6px 12px;font-size:17px;border-bottom:1px solid #e5edf3;color:#14283f"><strong>%s</strong></td></tr>`, metric.zh, metric.en, metric.value)
	}
	body.WriteString("</table>")

	address, err := url.Parse(strings.TrimSpace(system_setting.ServerAddress))
	if err != nil || (address.Scheme != "https" && address.Scheme != "http") || address.Host == "" || address.User != nil {
		address = nil
	} else {
		address.Path = strings.TrimRight(address.Path, "/") + "/usage-logs/error"
		address.RawPath, address.RawQuery, address.Fragment, address.ForceQuery = "", "", "", false
	}
	if address != nil {
		linkLabel := "打开当日错误日志 / Open all errors for this day"
		if incomplete {
			linkLabel = "查看仍保留的错误日志 / Open retained error records"
		}
		fmt.Fprintf(&body, `<p style="margin:0 0 22px;word-break:break-word">%s</p>`, dailyReportErrorLink(*address, report, nil, linkLabel, true))
	} else {
		body.WriteString("<p>管理后台 → 日志 → 错误日志，选择上方日期。 / In the admin console, open Error Logs and select the date above.</p>")
	}

	if ops.incidentCount > 0 {
		fmt.Fprintf(&body, "<h3>已确认渠道异常 / Confirmed channel incidents · %d</h3>", ops.incidentCount)
		fmt.Fprintf(&body, "<p>状态截至 %s（北京时间） · Status as of report generation</p>", ops.asOf.In(errorReportLocation()).Format("2006-01-02 15:04"))
		for _, incident := range ops.incidents {
			state, stateColor := "仍未关闭 / Still open", "#9b392c"
			if incident.ClosedAt > 0 {
				switch incident.CloseReason {
				case model.ChannelAlertClosedRecovered:
					state, stateColor = "已观察到恢复 / Recovery observed", "#186453"
				default:
					state, stateColor = "记录已关闭 / Incident closed", "#446079"
				}
				state += " · " + time.UnixMilli(incident.ClosedAt).In(errorReportLocation()).Format("01-02 15:04")
			}
			body.WriteString(dailyReportCardStart)
			fmt.Fprintf(&body, `<strong style="color:%s">%s</strong><br><strong>%s</strong> · %s<br>%s · %s · %d 条确认样本 / observations`,
				stateColor, state, html.EscapeString(dailyReportChannelLabel(incident.ChannelID, ops.channelNames)),
				html.EscapeString(common.SanitizeTaskDiagnostic(incident.Model)), html.EscapeString(incident.Operation), html.EscapeString(incident.Kind), incident.EvidenceCount)

			fmt.Fprintf(&body, "<br>确认时间 / Confirmed %s", time.UnixMilli(incident.ConfirmedAt).In(errorReportLocation()).Format("01-02 15:04"))
			if incident.StatusCode > 0 {
				fmt.Fprintf(&body, " · 上游 / Upstream HTTP %d", incident.StatusCode)
			}
			if address != nil {
				fmt.Fprintf(&body, "<br>%s", dailyReportErrorLink(*address, report, url.Values{"channel": {strconv.Itoa(incident.ChannelID)}}, "查看当日渠道记录 / View channel errors", false))
			}
			body.WriteString(dailyReportCardEnd)
		}
		if ops.incidentCount > int64(len(ops.incidents)) {
			fmt.Fprintf(&body, "<p>正文展示 %d 起 / Showing %d incidents</p>", len(ops.incidents), len(ops.incidents))
		}
	}

	sections := []struct{ signal, label string }{
		{model.DailyErrorAPI5xx, "客户 API 5xx / Customer API 5xx"},
		{clienterrlog.EventStreamError, "需排查的流式异常 / Stream errors to investigate"},
		{clienterrlog.EventTaskFailure, "异步任务失败或过期 / Failed or expired tasks"},
		{model.DailyErrorAPI429, "客户 API 429 / Customer API 429"},
		{clienterrlog.WalletQuotaInsufficient, "钱包余额不足 / Insufficient wallet balance"},
		{clienterrlog.SubscriptionQuotaInsufficient, "订阅不可用或额度不足 / Subscription unavailable or insufficient quota"},
		{clienterrlog.APIKeyQuotaInsufficient, "API Key 额度不足 / Insufficient API key quota"},
		{model.DailyErrorAPI4xx, "客户 API 其他 4xx / Other customer API 4xx"},
		{clienterrlog.EventChannelTest, "巡检与手工测试失败 / Probe and manual test failures"},
		{model.DailyErrorStreamDisconnected, "流式取消或客户端断开 / Stream cancellations or client disconnects"},
		{model.DailyErrorStreamWarning, "流式软错误 / Stream warnings"},
		{model.DailyErrorOther, "其他错误事件 / Other recorded errors"},
	}
	summaryRows := make([][2]string, 0, len(sections))
	for _, section := range sections {
		value := dailyReportErrorCount(summary.Signals[section.signal], incomplete)
		switch section.signal {
		case clienterrlog.WalletQuotaInsufficient, clienterrlog.SubscriptionQuotaInsufficient, clienterrlog.APIKeyQuotaInsufficient:
			if accounts := ops.quotaAccounts[section.signal]; accounts > 0 {
				value += fmt.Sprintf(" · 日志中 %d 个账户 / %d accounts in retained logs", accounts, accounts)
			}
		}
		summaryRows = append(summaryRows, [2]string{section.label, value})
	}
	writeMailErrorDetails(&body, ops.details)
	if len(summary.Groups) > 0 {
		body.WriteString("<h3>重点错误组 / Error groups to review</h3>")
		body.WriteString("<p>每类展示最多 3 组，记录按渠道、模型、HTTP 状态和原因归并。<br>Up to three groups per category, grouped by channel, model, HTTP status and reason.</p>")
	}
	customerFailures := make(map[int]map[string]bool)
	for _, group := range summary.Groups {
		switch group.Signal() {
		case model.DailyErrorAPI5xx, clienterrlog.EventStreamError, clienterrlog.EventTaskFailure:
			if group.ChannelID > 0 && group.ModelName != "" {
				if customerFailures[group.ChannelID] == nil {
					customerFailures[group.ChannelID] = make(map[string]bool)
				}
				customerFailures[group.ChannelID][group.ModelName] = true
			}
		}
	}
	for _, section := range sections {
		shown := 0
		for _, group := range summary.Groups {
			if group.Signal() != section.signal {
				continue
			}
			matchingImpact := group.EventType == clienterrlog.EventChannelTest && customerFailures[group.ChannelID][group.ModelName]
			var card strings.Builder
			writeDailyReportErrorGroup(&card, address, report, group, ops.channelNames, summary.Approximate || incomplete, matchingImpact)
			// URL encoding and HTML escaping can expand an otherwise small
			// summary. Reserve space for the counts and fixed report footer.
			if body.Len()+card.Len() > 48<<10 {
				summary.Approximate = true
				continue
			}
			if shown == 0 {
				fmt.Fprintf(&body, `<h4 style="margin:16px 0 6px;color:#26394f;font-size:14px">%s</h4>`, section.label)
			}
			body.WriteString(card.String())
			shown++
			if shown == 3 {
				break
			}
		}
	}

	body.WriteString("<h3>错误分布 / Recorded error breakdown</h3>")
	writeOperationsMailRows(&body, summaryRows)

	for _, section := range []struct {
		title string
		items []model.DailyReportCount
	}{
		{"失败运维任务 / Failed operations tasks", ops.tasks},
		{"资源告警 / Resource alerts", ops.resources},
	} {
		fmt.Fprintf(&body, "<h3>%s</h3>", section.title)
		if len(section.items) == 0 {
			body.WriteString("<p>无记录 / No records</p>")
			continue
		}
		rows := make([][2]string, 0, min(10, len(section.items)))
		for _, item := range section.items[:min(10, len(section.items))] {
			rows = append(rows, [2]string{item.Key, strconv.FormatInt(item.Count, 10)})
		}
		writeOperationsMailRows(&body, rows)
	}

	if summary.Approximate {
		body.WriteString("<p>高基数切片只保留部分候选组。≥ 为已计入的组内下限，分组次序可能偏差。<br>Some groups were dropped from the ranking. ≥ marks a lower bound and group order may differ from the full log.</p>")
	}
	body.WriteString(`<p style="padding-top:12px;border-top:1px solid #dce4ee;color:#49627a;font-size:12px">错误量基于已入库事件，可能遗漏迟到记录。邮件没有成功请求总量，不能推断失败率或全站可用性。登录管理员账号可查看保留期内的脱敏请求和响应。<br>Counts cover recorded events and may miss late arrivals. No request denominator is available here to infer failure rate or site health. Sign in as an administrator to view retained, redacted request and response details.</p>`)
	return &model.ErrorReportPart{ReportID: report.ReportID, PartNo: 1,
		Subject: title + " — " + day, BodyHTML: RenderMailTemplate(title, body.String())}
}

func dailyReportErrorCount(count int64, incomplete bool) string {
	if !incomplete {
		return strconv.FormatInt(count, 10)
	}
	if count == 0 {
		return "未知 / Unknown"
	}
	return "≥" + strconv.FormatInt(count, 10)
}

const dailyReportCardStart = `<table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="table-layout:fixed;margin:0 0 8px"><tr><td style="padding:9px 12px;background:#f5f8fb;border:1px solid #dce4ee;word-break:break-word">`
const dailyReportCardEnd = "</td></tr></table>"

func writeDailyReportErrorGroup(body *strings.Builder, address *url.URL, report *model.ErrorReport, group model.DailyReportErrorGroup, names map[int]string, approximate, matchingImpact bool) {
	reason := common.SanitizeTaskDiagnostic(group.Reason)
	if reason == "" {
		reason = "未记录原因 / No recorded reason"
	}
	status := ""
	isAPI := group.EventType == clienterrlog.EventAPIError || group.EventType == clienterrlog.EventStreamError
	if isAPI && group.Status >= 100 && group.Status < 600 {
		status = fmt.Sprintf(" · HTTP %d", group.Status)
	}
	countPrefix := ""
	if approximate {
		countPrefix = "≥"
	}
	modelName := common.SanitizeTaskDiagnostic(group.ModelName)
	label := dailyReportChannelLabel(group.ChannelID, names)
	if modelName != "" {
		label += " · " + modelName
	}
	body.WriteString(dailyReportCardStart)
	fmt.Fprintf(body, `<strong>%s</strong> · %s%d%s<br><span style="font-size:12px;color:#49627a">%s · 最近 / Last %s</span>`,
		html.EscapeString(label), countPrefix, group.Count, status, html.EscapeString(reason),
		time.Unix(group.LastAt, 0).In(errorReportLocation()).Format("15:04:05"))
	if matchingImpact {
		body.WriteString("<br>同渠道、同模型也有客户失败记录，请核对原因。 / Customer failures were also recorded for this channel and model.")
	}
	if address != nil {
		filters := url.Values{}
		if clienterrlog.ValidEventType(group.EventType) && group.EventType != "" {
			filters.Set("event_type", group.EventType)
		}
		if group.ChannelID > 0 {
			filters.Set("channel", strconv.Itoa(group.ChannelID))
		}
		if modelName == group.ModelName && group.ModelName != "" {
			if encoded, err := common.Marshal(group.ModelName); err == nil {
				filters.Set("model_name", string(encoded))
			}
		}
		if status != "" {
			filters.Set("status", strconv.Itoa(group.Status))
		}
		if reason == group.Reason && group.Reason != "" {
			// The router parses query values as JSON when possible. Quote this
			// string so numeric and boolean-looking reasons remain strings.
			if encoded, err := common.Marshal(group.Reason); err == nil {
				filters.Set("reason", string(encoded))
			}
		}
		fmt.Fprintf(body, "<br>%s", dailyReportErrorLink(*address, report, filters, "查看相关错误 / View related errors", false))
	}
	body.WriteString(dailyReportCardEnd)
}

func dailyReportChannelLabel(id int, names map[int]string) string {
	if id <= 0 {
		return "未关联渠道 / No linked channel"
	}
	label := fmt.Sprintf("#%d", id)
	if name := common.SanitizeTaskDiagnostic(names[id]); name != "" {
		label += " " + name
	}
	return label
}

func dailyReportErrorLink(address url.URL, report *model.ErrorReport, filters url.Values, label string, primary bool) string {
	if filters == nil {
		filters = url.Values{}
	}
	filters.Set("start_timestamp", strconv.FormatInt(report.WindowStart, 10))
	filters.Set("end_timestamp", strconv.FormatInt(report.WindowEnd-1, 10))
	address.RawQuery = filters.Encode()
	href := html.EscapeString(address.String())
	if primary {
		return fmt.Sprintf(`<a href="%s" style="display:inline-block;box-sizing:border-box;max-width:100%%;white-space:normal;padding:9px 13px;background:#075b68;color:#ffffff;font-weight:600;text-decoration:none;border:1px solid #075b68">%s</a>`, href, label)
	}
	return fmt.Sprintf(`<a href="%s" style="color:#075b68;font-weight:600">%s</a>`, href, label)
}
