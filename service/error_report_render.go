package service

import (
	"context"
	"fmt"
	"html"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/perf_metrics_setting"
)

// 每小时报告的冻结正文渲染（开发方案第 4、5 节）。
// 全部动态字段经 html.EscapeString 转义，正文在发布后冻结：重试与补发不
// 因当前参数、渠道改名或模板升级而重新分页。分卷按 UTF-8 HTML 编码后的
// 字节预算控制，预算是工程初值，不代表平台容量事实。

const (
	errorReportPartBudgetBytes = 48 * 1024
	// errorReportPartSkeletonReserve 为邮件骨架（样式、报告标识、分卷编号）预留的字节。
	errorReportPartSkeletonReserve = 2048
)

// errorReportPartDraft 是发布前的分卷草稿；主题在分卷总数确定后回填。
type errorReportPartDraft struct {
	Subject    string
	BodyHTML   string
	DetailRows int
}

// errorReportUnit 是可独立渲染的展示单元：非表格单元用 Text，表格单元用
// Title/Header/Rows。表格行按预算拆到多个单元时重复表头，不截断统计。
type errorReportUnit struct {
	Title  string
	Header string
	Rows   []string
	Text   string
	// Detail 为 true 表示本单元是错误明细表，其行数计入分卷明细行数；
	// 汇总表的行数不计入，避免与事件总数混淆。
	Detail bool
}

// errorReportWindowView 是一个窗口报告的完整渲染视图。
type errorReportWindowView struct {
	ReportID    string
	WindowStart time.Time
	WindowEnd   time.Time
	GeneratedAt time.Time
	DataNote    string
	Status      errorReportStatusTexts
	PerfNote    string
	TotalEvents int
	TotalParts  int
	SummaryJSON string
}

// renderErrorReportSummary renders only the bounded summary dimensions.
func (h *errorReportHandler) renderErrorReportSummary(ctx context.Context, windowStart, windowEnd int64, total int, counts, byType, byReason, byChannel map[string]int) (*errorReportWindowView, []errorReportUnit, error) {
	loc := errorReportLocation()
	now := h.now().In(loc)
	perfNote, perfModels, perfHasActivity := h.errorReportPerfSection(ctx, windowStart, windowEnd)
	view := &errorReportWindowView{
		ReportID:    errorReportID(windowStart),
		WindowStart: time.Unix(windowStart, 0).In(loc),
		WindowEnd:   time.Unix(windowEnd, 0).In(loc),
		GeneratedAt: now,
		DataNote: "本报告是截至生成时刻已入库错误事件的固定快照；生成之后才入库的" +
			"同小时事件不包含在本报告中，错误采集层未接入的事件也不会因邮件机制可见。 / This is a fixed snapshot of error events persisted by generation time. Events arriving later, even for this hour, and events outside collection coverage are not included.",
		TotalEvents: total,
	}
	view.Status = resolveErrorReportStatus(total, counts, perfHasActivity, perfNote)
	view.PerfNote = perfNote

	units := []errorReportUnit{
		{Title: "报告标识与数据说明 / Report identity and data scope", Text: h.renderReportMeta(view)},
		{Title: "运行摘要 / Operational summary", Text: h.renderStatusSection(view)},
		{Title: "错误分类汇总 / Error classification summary", Text: renderClassSummary(counts)},
		{Title: "按事件类型汇总 / Summary by event type", Header: tableHeader("事件类型 / Event type", "数量 / Count"), Rows: countRows(byType)},
		{Title: "按原因码汇总 / Summary by reason code", Header: tableHeader("原因码 / Reason code", "数量 / Count"), Rows: countRows(byReason)},
		{Title: "按渠道汇总 / Summary by channel", Header: tableHeader("渠道 / Channel", "数量 / Count"), Rows: countRows(byChannel)},
	}
	if len(perfModels) > 0 {
		units = append(units, errorReportUnit{
			Title:  "模型统计（性能采样口径） / Model statistics (performance samples)",
			Header: tableHeader("模型 / Model", "采样请求数 / Sampled requests", "采样成功数 / Sampled successes"),
			Rows:   perfModelRows(perfModels),
			Text:   "",
		})
	} else {
		units = append(units, errorReportUnit{Title: "模型统计（性能采样口径） / Model statistics (performance samples)", Text: "<p>" + html.EscapeString(perfNote) + "</p>"})
	}
	summaryView := map[string]any{
		"status_headline": view.Status.Headline,
		"total_events":    view.TotalEvents,
		"by_class":        counts,
		"by_type":         byType,
		"perf_note":       perfNote,
	}
	summaryJSON, err := common.Marshal(summaryView)
	if err != nil {
		return nil, nil, err
	}
	view.SummaryJSON = string(summaryJSON)
	return view, units, nil
}

// errorReportPerfSection 汇总窗口内已落库性能统计。只展示可核实的采样口径，
// 不推算全站或渠道成功率，也不与错误事件相加。
func (h *errorReportHandler) errorReportPerfSection(ctx context.Context, windowStart, windowEnd int64) (string, []model.PerfMetricSummary, bool) {
	if !perf_metrics_setting.GetSetting().Enabled {
		return "性能统计未启用，模型统计数据不足，无法据此判断调用活动。 / Performance statistics are disabled; there is insufficient evidence to determine request activity.", nil, false
	}
	summaries, err := model.GetErrorReportPerfSummaries(ctx, windowStart, windowEnd)
	if err != nil {
		return "性能统计读取失败，模型统计数据不足，无法据此判断调用活动。 / Performance statistics could not be read; there is insufficient evidence to determine request activity.", nil, false
	}
	var activity bool
	for _, summary := range summaries {
		if summary.RequestCount > 0 {
			activity = true
			break
		}
	}
	note := "以下模型统计来自已落库的性能采样（按内存时间桶定期落库，整点生成时最近桶" +
		"可能尚未持久化），不是全站或渠道请求总量，不可与错误事件相加计算成功率。 / These are persisted performance samples; the latest in-memory time bucket may not yet be saved. They are not site-wide or channel request totals and must not be combined with error events to calculate a success rate."
	if !activity {
		note = "本窗口内无已落库性能统计采样；不能把缺少记录等同于没有流量。 / No persisted performance samples exist for this window. Missing records do not prove there was no traffic. " + note
	}
	return note, summaries, activity
}

func tableHeader(cols ...string) string {
	var b strings.Builder
	b.WriteString("<tr>")
	for _, col := range cols {
		b.WriteString("<th>" + html.EscapeString(col) + "</th>")
	}
	b.WriteString("</tr>")
	return b.String()
}

func countRows(counts map[string]int) []string {
	type pair struct {
		key   string
		count int
	}
	pairs := make([]pair, 0, len(counts))
	for key, count := range counts {
		pairs = append(pairs, pair{key, count})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].count != pairs[j].count {
			return pairs[i].count > pairs[j].count
		}
		return pairs[i].key < pairs[j].key
	})
	rows := make([]string, 0, len(pairs))
	for _, p := range pairs {
		rows = append(rows, "<tr><td>"+html.EscapeString(p.key)+"</td><td>"+fmt.Sprint(p.count)+"</td></tr>")
	}
	return rows
}

func perfModelRows(summaries []model.PerfMetricSummary) []string {
	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].RequestCount != summaries[j].RequestCount {
			return summaries[i].RequestCount > summaries[j].RequestCount
		}
		return summaries[i].ModelName < summaries[j].ModelName
	})
	rows := make([]string, 0, len(summaries))
	for _, summary := range summaries {
		rows = append(rows, "<tr><td>"+html.EscapeString(summary.ModelName)+"</td><td>"+
			fmt.Sprint(summary.RequestCount)+"</td><td>"+fmt.Sprint(summary.SuccessCount)+"</td></tr>")
	}
	return rows
}

// renderClassSummary 产出责任分类的文字摘要，逐类说明口径。
func renderClassSummary(counts map[string]int) string {
	lines := []struct {
		class string
		label string
		note  string
	}{
		{errorReportClassPlatform, "平台／上游相关 / Platform / upstream", "明确的平台或上游服务错误（HTTP 5xx 或已登记的流式失败原因） / Explicit platform or upstream errors (HTTP 5xx or registered stream failure reasons)"},
		{errorReportClassClient, "客户侧 / Client-side", "已核对的客户参数、权限或客户端断开原因码（不按 HTTP 4xx 猜测） / Verified client parameter, permission or disconnection reasons; HTTP 4xx alone does not establish responsibility"},
		{errorReportClassStream, "流式观测 / Stream observations", "HTTP 已 2xx/3xx 提交后观测到的流处理异常 / Stream processing errors observed after an HTTP 2xx/3xx response was committed"},
		{errorReportClassChannelTest, "渠道测试 / Channel tests", "渠道测试失败，不等于实际客户调用失败 / Failed channel tests do not imply failed customer requests"},
		{errorReportClassUnknown, "待分类 / Unclassified", "原因不明，需要人工排查 / Unknown cause; manual investigation required"},
	}
	var b strings.Builder
	b.WriteString("<table><tr><th>分类 / Class</th><th>数量 / Count</th><th>口径说明 / Definition</th></tr>")
	for _, line := range lines {
		b.WriteString("<tr><td>" + line.label + "</td><td>" + fmt.Sprint(counts[line.class]) +
			"</td><td>" + line.note + "</td></tr>")
	}
	b.WriteString("</table>")
	return b.String()
}

// renderReportMeta 渲染报告标识与数据说明块。
func (h *errorReportHandler) renderReportMeta(view *errorReportWindowView) string {
	format := "2006-01-02 15:04:05"
	return "<p>报告标识 / Report ID: " + html.EscapeString(view.ReportID) + "<br>" +
		"报告时段 / Window: " + html.EscapeString(view.WindowStart.Format(format)) + " 至 / to " +
		html.EscapeString(view.WindowEnd.Format(format)) + "（北京时间，左闭右开 / UTC+08:00, start inclusive, end exclusive）<br>" +
		"生成时间 / Generated at: " + html.EscapeString(view.GeneratedAt.Format(format)) + "<br>" +
		"事件总数 / Total events: " + fmt.Sprint(view.TotalEvents) + "<br>" +
		"数据说明 / Data scope: " + html.EscapeString(view.DataNote) + "</p>"
}

// renderStatusSection 渲染运行摘要块。
func (h *errorReportHandler) renderStatusSection(view *errorReportWindowView) string {
	return "<p><strong>" + html.EscapeString(view.Status.Headline) + "</strong><br>" +
		html.EscapeString(view.Status.Body) + "</p>" +
		"<p>模型统计口径 / Model statistics scope: " + html.EscapeString(view.PerfNote) + "</p>"
}

// renderDetailUnits 把全部错误事件渲染为明细表格单元（不抽样、不截断）。
// 单条记录超过预算时按带关联编号的续段拆分，不删除该记录。
func (h *errorReportHandler) renderDetailUnits(events []*model.ErrorEvent, loc *time.Location, offset int) []errorReportUnit {
	if len(events) == 0 {
		return []errorReportUnit{{Title: "错误明细 / Error details", Text: "<p>本窗口已入库记录中未发现错误。 / No errors found in persisted records for this window. </p>"}}
	}
	header := errorReportDetailHeader()
	units := make([]errorReportUnit, 0, 8)
	current := errorReportUnit{Title: "错误明细 / Error details", Header: header, Detail: true}
	for index, event := range events {
		current.Rows = append(current.Rows, h.renderDetailRow(offset+index+1, event, loc)...)
		// 每个表格单元控制在预算内，由 splitErrorReportUnits 续表。
		if len(current.Rows) >= 200 {
			units = append(units, current)
			current = errorReportUnit{Title: "错误明细（续） / Error details (continued)", Header: header, Detail: true}
		}
	}
	units = append(units, current)
	return units
}

func errorReportDetailHeader() string {
	return tableHeader("序号 / No.", "时间 / Time", "类型 / Type", "模块 / Module", "HTTP 状态 / HTTP status", "用户 / User", "令牌 / API key", "模型 / Model", "渠道 / Channel", "阶段 / Stage", "原因码 / Reason code", "公开错误码 / Public error code", "请求 ID / Request ID", "上游请求 ID / Upstream request ID", "任务 ID / Task ID", "耗时(ms) / Duration (ms)", "详情 / Details")
}

// renderDetailRow 渲染一条事件的 1..N 个 <tr>；超长详情拆分为带关联编号的续段。
func (h *errorReportHandler) renderDetailRow(seq int, event *model.ErrorEvent, loc *time.Location) []string {
	cells := []string{
		fmt.Sprint(seq),
		time.Unix(event.CreatedAt, 0).In(loc).Format("2006-01-02 15:04:05"),
		emptyDash(event.EventType),
		emptyDash(event.Module),
		httpStatusText(event.Status),
		emptyDash(event.Username),
		emptyDash(event.TokenName),
		emptyDash(event.ModelName),
		emptyDash(event.ChannelName),
		emptyDash(event.Stage),
		emptyDash(event.Reason),
		emptyDash(event.PublicCode),
		emptyDash(event.RequestId),
		emptyDash(event.UpstreamRequestId),
		emptyDash(event.TaskId),
		fmt.Sprint(event.ElapsedMs),
	}
	fixed := "<tr><td>" + strings.Join(escapeAll(cells), "</td><td>") + "</td><td>"
	// Every continued row is wrapped in a table with the full bilingual header.
	// Reserve that actual header plus title/continuation markup before splitting.
	detailCell := renderDetailCell(event.Detail, errorReportPartBudgetBytes-errorReportPartSkeletonReserve-len(fixed)-len(errorReportDetailHeader())-len("<table></table>")-256)
	if len(detailCell) == 1 {
		return []string{fixed + detailCell[0] + "</td></tr>"}
	}
	rows := make([]string, 0, len(detailCell))
	rows = append(rows, fixed+detailCell[0]+"</td></tr>")
	for i := 1; i < len(detailCell); i++ {
		rows = append(rows, fmt.Sprintf("<tr><td>明细 / Detail #%d（续 / continued %d/%d）</td><td colspan=\"16\">%s</td></tr>",
			seq, i, len(detailCell)-1, detailCell[i]))
	}
	return rows
}

// renderDetailCell 把事件的受控详情 JSON 渲染为 1..N 个已转义的单元格片段。
// 超长值在原始字符串的字符边界分段后分别转义，不会切破 HTML 实体。
func renderDetailCell(detail string, budget int) []string {
	const detailLabel = "—"
	if strings.TrimSpace(detail) == "" {
		return []string{detailLabel}
	}
	var kv map[string]string
	if err := common.UnmarshalJsonStr(detail, &kv); err != nil || len(kv) == 0 {
		return escapeChunked(detail, maxInt(budget, 256))
	}
	keys := make([]string, 0, len(kv))
	for key := range kv {
		if key == clienterrlog.HTTPExchangeDetailKey {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	// pieceBudget 按转义后长度计量（escapeChunked 内部以转义后字节累计），
	// 预留实体余量，保证单个片段不会撑破分卷预算。
	pieceBudget := maxInt(budget-64, 256)
	var pieces []string
	for _, key := range keys {
		value := kv[key]
		if key == "fail_reason" {
			value = common.PublicTaskErrorMessage(value)
		}
		escapedKey := html.EscapeString(key)
		if value == "" {
			pieces = append(pieces, escapedKey+"="+detailLabel+"<br>")
			continue
		}
		for _, chunk := range escapeChunked(value, pieceBudget) {
			pieces = append(pieces, escapedKey+"="+chunk+"<br>")
		}
	}
	return packDetailPieces(pieces, budget)
}

// packDetailPieces 把详情片段贪心打包为不超过预算的单元格片段。
func packDetailPieces(pieces []string, budget int) []string {
	if budget < 256 {
		budget = 256
	}
	groups := make([]string, 0, len(pieces))
	var current strings.Builder
	for _, piece := range pieces {
		if current.Len() > 0 && current.Len()+len(piece) > budget {
			groups = append(groups, current.String())
			current.Reset()
		}
		current.WriteString(piece)
	}
	if current.Len() > 0 {
		groups = append(groups, current.String())
	}
	if len(groups) == 0 {
		return []string{"—"}
	}
	return groups
}

// escapeChunked 把原始字符串按字符边界分段，每段单独转义，保证转义实体
// 不被切破；size 按每字符的转义后长度累计，因此转义扩张不会撑破预算。
func escapeChunked(value string, maxBytes int) []string {
	if maxBytes < 16 {
		maxBytes = 16
	}
	var chunks []string
	var raw strings.Builder
	size := 0
	flush := func() {
		if raw.Len() > 0 {
			chunks = append(chunks, html.EscapeString(raw.String()))
			raw.Reset()
			size = 0
		}
	}
	for _, r := range value {
		rs := string(r)
		escapedLen := len(html.EscapeString(rs))
		if size > 0 && size+escapedLen > maxBytes {
			flush()
		}
		raw.WriteString(rs)
		size += escapedLen
	}
	flush()
	if len(chunks) == 0 {
		return []string{"—"}
	}
	return chunks
}

func escapeAll(values []string) []string {
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = html.EscapeString(value)
	}
	return out
}

func emptyDash(value string) string {
	if value == "" {
		return "—"
	}
	return value
}

func httpStatusText(status int) string {
	if status <= 0 {
		return "—"
	}
	return fmt.Sprint(status)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// errorReportFragment 是渲染管线中的最小打包单位：一个完整的展示块
// （文本段或带表头的完整表格），已按预算从更大单元拆出。
type errorReportFragment struct {
	html       string
	detailRows int
}

// splitErrorReportUnits 把展示单元拆为不超过预算的完整片段：表格单元按
// 行续表（重复标题与表头），不截断任何一行统计。
func splitErrorReportUnits(units []errorReportUnit) []errorReportFragment {
	budget := errorReportPartBudgetBytes - errorReportPartSkeletonReserve
	fragments := make([]errorReportFragment, 0, len(units))
	for _, unit := range units {
		if unit.Text != "" {
			text := unit.Text
			if unit.Title != "" {
				text = "<h2>" + html.EscapeString(unit.Title) + "</h2>" + text
			}
			fragments = append(fragments, errorReportFragment{html: text, detailRows: 0})
			continue
		}
		title := ""
		if unit.Title != "" {
			title = "<h2>" + html.EscapeString(unit.Title) + "</h2>"
		}
		if len(unit.Rows) == 0 {
			fragments = append(fragments, errorReportFragment{html: title + "<p>无数据。 / No data. </p>"})
			continue
		}
		wrapperOverhead := len(title) + len(unit.Header) + len("<table></table>") + 64
		var current []string
		currentSize := 0
		continuation := 0
		flush := func() {
			if len(current) == 0 {
				return
			}
			heading := title
			if continuation > 0 {
				heading = "<h2>" + html.EscapeString(unit.Title) + fmt.Sprintf("（续 / continued %d）", continuation) + "</h2>"
			}
			detailRows := 0
			if unit.Detail {
				detailRows = len(current)
			}
			fragments = append(fragments, errorReportFragment{
				html:       heading + "<table>" + unit.Header + strings.Join(current, "") + "</table>",
				detailRows: detailRows,
			})
			continuation++
			current = nil
			currentSize = 0
		}
		for _, row := range unit.Rows {
			if len(current) > 0 && currentSize+len(row)+wrapperOverhead > budget {
				flush()
			}
			current = append(current, row)
			currentSize += len(row)
		}
		flush()
	}
	return fragments
}

// errorReportSubject 生成冻结主题；分卷多于一封时标注「第 i/N 封」。
// 补发标识不在此处冻结：积压恢复发送时由投递循环按发送时刻追加。
func errorReportSubject(view *errorReportWindowView, partNo, totalParts int) string {
	window := view.WindowStart.Format("2006-01-02 15:04") + "–" + view.WindowEnd.Format("15:04")
	subject := fmt.Sprintf("[系统运行与错误报告 / Operations and errors] %s 北京时间 / UTC+08:00｜%s", window, view.Status.Headline)
	if totalParts > 1 {
		subject += fmt.Sprintf("｜分卷 / Part %d/%d", partNo, totalParts)
	}
	return subject
}

// wrapErrorReportPart 生成完整 HTML 正文骨架（内联样式，无外部资源）。
func wrapErrorReportPart(view *errorReportWindowView, partNo, totalParts int, content string) string {
	var header strings.Builder
	header.WriteString("<p style=\"color:#555;font-size:12px;\">")
	header.WriteString(html.EscapeString(view.ReportID))
	if totalParts > 1 {
		header.WriteString(fmt.Sprintf("｜分卷 / Part %d/%d", partNo, totalParts))
	}
	header.WriteString("<br>报告时段 / Window: " + html.EscapeString(view.WindowStart.Format("2006-01-02 15:04")) +
		"–" + html.EscapeString(view.WindowEnd.Format("15:04")) + "（北京时间 / UTC+08:00）")
	header.WriteString("<br>" + html.EscapeString(view.DataNote) + "</p>")
	return "<!DOCTYPE html><html><head><meta charset=\"UTF-8\"></head><body style=\"font-family:sans-serif;font-size:13px;\">" +
		header.String() + content +
		"<hr><p style=\"color:#999;font-size:12px;\">本邮件由系统定时任务发送，仅包含已脱敏的安全字段。 / Sent by a scheduled system task. Only sanitized, safe fields are included. </p></body></html>"
}
