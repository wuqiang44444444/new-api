package service

import (
	"context"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

func init() {
	RegisterSystemTaskHandler(newErrorReportHandler(common.SendEmailContext))
}

func (h *errorReportHandler) generateDailyReport(ctx context.Context) (int, []string, error) {
	recipients, err := model.GetOperationsMailRecipients(ctx)
	if err != nil {
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
	if report.Summary != "" {
		if err := common.UnmarshalJsonStr(report.Summary, &summary); err != nil {
			return 0, nil, err
		}
	}
	through, total := report.AggregatedThrough, int64(report.TotalEvents)
	if summary.Signals == nil {
		// A pending report from the old format has no triage totals. Re-read
		// its window instead of presenting partial signal counts as complete.
		summary.Signals = make(map[string]int64)
		through, total = start, 0
	}
	initialThrough, initialGap := through, summary.RetentionGapThrough
	cleanupCutoff, err := model.ReadErrorEventRetentionCutoff(ctx)
	if err != nil {
		return 0, []string{"daily error retention coverage"}, err
	}
	cutoff := cleanupCutoff
	// Deployments upgrading from before the durable watermark may already
	// have deleted rows. Treat the currently configured horizon as uncertain.
	if days := errorEventRetentionDays(); days > 0 {
		cutoff = max(cutoff, now.Unix()-int64(days)*86400)
	}
	if cutoff > start {
		summary.RetentionGapThrough = max(initialGap, min(cutoff, report.WindowEnd))
		through = max(through, summary.RetentionGapThrough)
	}
	groups := make(map[model.DailyReportErrorKey]model.DailyReportErrorGroup)
	for _, group := range summary.Groups {
		groups[group.DailyReportErrorKey] = group
	}
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
		for signal, count := range slice.Signals {
			summary.Signals[signal] += count
		}
		for _, group := range slice.Groups {
			previous := groups[group.DailyReportErrorKey]
			group.Count += previous.Count
			group.LastAt = max(group.LastAt, previous.LastAt)
			groups[group.DailyReportErrorKey] = group
		}
		summary.Approximate = summary.Approximate || slice.GroupsCapped
		through, total = end, total+slice.Total
	}
	if through != initialThrough || summary.RetentionGapThrough != initialGap {
		// Retention can run against a separate log store during these reads.
		// Its watermark commits before deleting, so discard a batch when the
		// newly excluded range intersects the portion being aggregated.
		latestCutoff, err := model.ReadErrorEventRetentionCutoff(ctx)
		if err != nil {
			return 0, []string{"daily error retention coverage"}, err
		}
		if latestCutoff > cleanupCutoff && latestCutoff > max(initialThrough, summary.RetentionGapThrough) {
			return 0, nil, nil
		}
		if err := summary.retainGroups(groups); err != nil {
			return 0, nil, err
		}
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
	if through < report.WindowEnd || now.Unix() < report.WindowEnd+6*3600 {
		return 0, nil, nil
	}

	readCtx, cancel := context.WithTimeout(ctx, model.DailyReportReadBudget)
	tasks, resources, err := model.ReadDailyOperationsCounts(readCtx, start, report.WindowEnd)
	cancel()
	if err != nil {
		return 0, []string{"daily operations aggregation"}, err
	}
	readCtx, cancel = context.WithTimeout(ctx, model.DailyReportReadBudget)
	incidents, incidentCount, err := model.ReadDailyReportChannelIncidents(readCtx, start, report.WindowEnd)
	cancel()
	if err != nil {
		return 0, []string{"daily channel incidents"}, err
	}
	ids := make([]int, 0, len(summary.Groups)+len(incidents))
	for _, group := range summary.Groups {
		if group.ChannelID > 0 {
			ids = append(ids, group.ChannelID)
		}
	}
	for _, incident := range incidents {
		ids = append(ids, incident.ChannelID)
	}
	readCtx, cancel = context.WithTimeout(ctx, model.DailyReportReadBudget)
	channelNames, err := model.ReadDailyReportChannelNames(readCtx, ids)
	cancel()
	if err != nil {
		common.SysError("daily email channel labels could not be read")
	}
	recipients, err = model.GetOperationsMailRecipients(ctx)
	if err != nil || len(recipients) == 0 {
		return 0, nil, err
	}
	var quotaAccounts map[string]int64
	if summary.Signals[clienterrlog.WalletQuotaInsufficient]+summary.Signals[clienterrlog.SubscriptionQuotaInsufficient]+summary.Signals[clienterrlog.APIKeyQuotaInsufficient] > 0 {
		readCtx, cancel = context.WithTimeout(ctx, model.DailyReportReadBudget)
		quotaAccounts, err = model.ReadDailyQuotaRejectionAccounts(readCtx, max(start, summary.RetentionGapThrough), report.WindowEnd)
		cancel()
		if err != nil {
			common.SysError("daily email quota rejection account totals could not be read")
		}
	}
	details := loadMailErrorDetails(ctx, model.MailErrorScope{Start: max(start, summary.RetentionGapThrough), End: report.WindowEnd}, nil, summary.RetentionGapThrough > start)
	day := time.Unix(start, 0).In(now.Location()).Format("2006-01-02")
	part := renderDailyReport(report, day, summary, dailyReportOperations{tasks: tasks, resources: resources, incidents: incidents, incidentCount: incidentCount, channelNames: channelNames, asOf: now, quotaAccounts: quotaAccounts, details: details})
	dataNote := "closed five-minute buckets; error groups may be truncated"
	if summary.RetentionGapThrough > start {
		dataNote = "error log coverage incomplete due to retention; counts cover available records"
	}
	if err := model.PublishDailyReport(ctx, part, recipients, report.TotalEvents, report.Summary, dataNote); err != nil {
		return 0, nil, err
	}
	return 1, nil, model.AdvanceDailyReportCursor(ctx, start)
}
