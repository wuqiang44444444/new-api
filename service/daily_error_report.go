package service

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

func init() {
	RegisterSystemTaskHandler(newErrorReportHandler(common.SendEmailContext))
}

type dailyReportSummary struct {
	Errors   map[string]int64 `json:"errors"`
	Channels map[int]int64    `json:"channels"`
}

func (h *errorReportHandler) generateDailyReport(ctx context.Context) (int, []string, error) {
	recipients, err := model.GetOperationsMailRecipients(ctx)
	if err != nil || len(recipients) == 0 {
		return 0, nil, err
	}
	now := h.now().In(errorReportLocation())
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	start, err := model.DailyReportCursor(ctx, today.AddDate(0, 0, -1).Unix())
	if err != nil || start == 0 || start > today.Unix() {
		return 0, nil, err
	}
	report, err := model.LoadDailyReport(ctx, start, fmt.Sprintf("daily-%d", start))
	if err != nil {
		return 0, nil, err
	}
	if report.Status == model.ErrorReportStatusReady {
		return 0, nil, model.AdvanceDailyReportCursor(ctx, start)
	}
	// Read only closed intervals, with a short grace period for queued error logs.
	lastClosedSlice := now.Add(-5 * time.Minute).Truncate(5 * time.Minute).Unix()
	var summary dailyReportSummary
	if report.Summary == "" {
		summary = dailyReportSummary{Errors: map[string]int64{}, Channels: map[int]int64{}}
	} else if err := common.UnmarshalJsonStr(report.Summary, &summary); err != nil {
		return 0, nil, err
	}
	through := report.AggregatedThrough
	total := int64(report.TotalEvents)
	var readErr error
	for range 6 {
		end := min(through+300, report.WindowEnd)
		if through >= report.WindowEnd || end > lastClosedSlice {
			break
		}
		readCtx, cancel := context.WithTimeout(ctx, model.DailyReportReadBudget)
		slice, err := model.ReadDailyReportSlice(readCtx, through, end)
		cancel()
		if err != nil {
			readErr = err
			break
		}
		for _, count := range slice.Errors {
			summary.Errors[count.Key] += count.Count
		}
		for _, count := range slice.Channels {
			summary.Channels[count.ChannelID] += count.Count
		}
		retainDailyLeaders(summary.Errors)
		retainDailyLeaders(summary.Channels)
		through, total = end, total+slice.Total
	}
	if through != report.AggregatedThrough {
		// Commit a bounded batch once, instead of contending with model
		// traffic for a separate main-database write on every slice.
		data, err := common.Marshal(summary)
		if err != nil {
			return 0, nil, err
		}
		if err := model.SaveDailyReportProgress(ctx, report, through, string(data), total); err != nil {
			return 0, nil, err
		}
		report.AggregatedThrough, report.Summary, report.TotalEvents = through, string(data), int(total)
	}
	if readErr != nil {
		return 0, []string{"daily error aggregation"}, readErr
	}
	if report.AggregatedThrough < report.WindowEnd || now.Unix() < report.WindowEnd+6*3600 {
		return 0, nil, nil
	}

	readCtx, cancel := context.WithTimeout(ctx, model.DailyReportReadBudget)
	tasks, resources, err := model.ReadDailyOperationsCounts(readCtx, start, report.WindowEnd)
	cancel()
	if err != nil {
		return 0, []string{"daily operations aggregation"}, err
	}
	recipients, err = model.GetOperationsMailRecipients(ctx)
	if err != nil || len(recipients) == 0 {
		return 0, nil, err
	}
	day := time.Unix(start, 0).In(now.Location()).Format("2006-01-02")
	part := renderDailyReport(report, day, summary, tasks, resources)
	if err := model.PublishDailyReport(ctx, part, recipients, report.TotalEvents, report.Summary, "closed five-minute buckets; top groups are approximate"); err != nil {
		return 0, nil, err
	}
	return 1, nil, model.AdvanceDailyReportCursor(ctx, start)
}

func renderDailyReport(report *model.ErrorReport, day string, summary dailyReportSummary, tasks, resources []model.DailyReportCount) *model.ErrorReportPart {
	errors := make([]model.DailyReportCount, 0, len(summary.Errors))
	channels := make([]model.DailyReportCount, 0, len(summary.Channels))
	for key, count := range summary.Errors {
		if key == "" {
			key = "未分类 / Unclassified"
		}
		errors = append(errors, model.DailyReportCount{Key: key, Count: count})
	}
	for id, count := range summary.Channels {
		channels = append(channels, model.DailyReportCount{Key: fmt.Sprintf("#%d", id), Count: count})
	}
	var body strings.Builder
	fmt.Fprintf(&body, `<p style="margin:0 0 14px;color:#49627a">%s · 北京时间 / Beijing time</p><p style="margin:0 0 20px;padding:12px;background:#e8f5f2;color:#143f38;font-size:17px">已采集错误 / Recorded errors · <strong>%d</strong></p>`, day, report.TotalEvents)
	for _, section := range []struct {
		title string
		items []model.DailyReportCount
	}{
		{"主要错误类型 / Top error types", errors},
		{"受影响渠道 / Affected channels", channels},
		{"失败运维任务 / Failed operations tasks", tasks},
		{"资源告警 / Resource alerts", resources},
	} {
		fmt.Fprintf(&body, `<h3 style="margin:18px 0 6px;font-size:15px;color:#14283f">%s</h3>`, section.title)
		sort.Slice(section.items, func(i, j int) bool {
			if section.items[i].Count == section.items[j].Count {
				return section.items[i].Key < section.items[j].Key
			}
			return section.items[i].Count > section.items[j].Count
		})
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
	body.WriteString("<p>数据来自已落库事件，排行按时段聚合，可能遗漏迟到记录。完整详情请在后台查看。</p><p>Counts use recorded events. Rankings are based on time-slice aggregates and late events may be missing. See the console for details.</p>")
	return &model.ErrorReportPart{ReportID: report.ReportID, PartNo: 1,
		Subject: "运维日报 / Daily operations report — " + day, BodyHTML: renderOperationsMail("运维日报 / Daily operations report", body.String())}
}

func retainDailyLeaders[K cmp.Ordered](counts map[K]int64) {
	const limit = 96
	if len(counts) <= limit {
		return
	}
	keys := slices.SortedFunc(maps.Keys(counts), func(a, b K) int {
		if counts[a] != counts[b] {
			return cmp.Compare(counts[b], counts[a])
		}
		return cmp.Compare(a, b)
	})
	for _, key := range keys[limit:] {
		delete(counts, key)
	}
}
