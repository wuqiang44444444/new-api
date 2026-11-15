package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"html"
	"slices"

	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
)

type usageReportMailHandler struct{}

func init() { RegisterSystemTaskHandler(usageReportMailHandler{}) }

func (usageReportMailHandler) Type() string            { return "usage_report_mail" }
func (usageReportMailHandler) Enabled() bool           { return model.DailyReportSourceSupported() }
func (usageReportMailHandler) Interval() time.Duration { return time.Minute }
func (usageReportMailHandler) NewPayload() any         { return nil }
func (usageReportMailHandler) Run(ctx context.Context, task *model.SystemTask, runner string) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	queued, err := queueUsageReportEmails(ctx, time.Now())
	if queued {
		queueResourceAlertDeliveries()
	}
	if err != nil {
		failSystemTask(task, runner, errors.New("could not prepare usage report emails"))
		return
	}
	if err := model.FinishSystemTask(task.TaskID, runner, model.SystemTaskStatusSucceeded, nil, ""); err != nil {
		logSystemTaskLockError(ctx, task, err)
	}
}

func queueUsageReportEmails(ctx context.Context, now time.Time) (bool, error) {
	if !model.DailyReportSourceSupported() {
		return false, nil
	}
	transport, err := common.LoadMailTransport(ctx)
	if err != nil {
		return false, err
	}
	if transport.Validate() != nil {
		return false, nil
	}
	day := now.In(model.UsageAnalyticsZone()).Add(-6*time.Hour).AddDate(0, 0, -1).Format(time.DateOnly)
	period, err := model.ResolveUsageAnalyticsPeriod(model.UsageAnalyticsPeriodDay, day, now.Unix())
	if err != nil {
		return false, err
	}
	meta, err := model.PublishedUsageMailDay(ctx, period)
	if err != nil || len(meta.References) == 0 {
		// A not-yet-published day is expected when the background build is late.
		return false, err
	}
	ref := meta.References[0]
	readCtx, cancel := context.WithTimeout(ctx, model.DailyReportReadBudget)
	cutoff, cutoffErr := model.ReadErrorEventRetentionCutoff(readCtx)
	cancel()
	if cutoffErr != nil {
		common.SysError("usage mail error-log coverage could not be read")
	}
	platform, platformErr := queueAdminUsageReportEmails(ctx, period, meta, ref, now, cutoff, cutoffErr != nil)
	prefs, err := model.LoadMailPreferences(ctx)
	if err != nil {
		return platform, errors.Join(platformErr, err)
	}
	if !prefs.Allows(model.MailPersonalUsage, time.Unix(ref.Start+30*3600, 0)) {
		return platform, platformErr
	}
	accounts, err := model.PublishedUsageMailAccounts(ctx, ref, 100)
	if err != nil {
		return platform, errors.Join(platformErr, err)
	}
	queued := platform
	for _, account := range accounts {
		var to []string
		if common.IsBareMailAddress(account.Email) {
			to = []string{account.Email}
		}
		if len(to) == 0 {
			continue
		}
		var details mailErrorDetails
		if account.Total.FailureCalls > 0 {
			details = loadUsageMailErrors(ctx, ref.Start, period.EndTimestamp, cutoff, cutoffErr != nil, []int{account.UserID}, false)
		}
		opened, err := openUsageReportMail(ctx, model.PersonalUsageReportMail, ref.Start, account.UserID, usageReportMailBody(day, meta, account.Total, nil, details), to)
		if err != nil {
			return queued, errors.Join(platformErr, err)
		}
		queued = queued || (opened && len(to) > 0)
	}
	return queued, platformErr
}

func queueAdminUsageReportEmails(ctx context.Context, period model.UsageAnalyticsPeriod, meta model.PublishedUsageReport, ref model.UsageReportDayReference, now time.Time, cutoff int64, coverageUnknown bool) (bool, error) {
	recipients, err := model.GetOperationsMailRecipients(ctx)
	if err != nil || len(recipients) == 0 {
		return false, err
	}
	prefs, err := model.LoadMailPreferences(ctx)
	if err != nil {
		return false, err
	}
	eligible := time.Unix(ref.Start+30*3600, 0)
	var existing []string
	if err := model.DB.WithContext(ctx).Model(&model.ResourceAlertEpisode{}).Where("open_key IN ?", []string{
		usageReportMailKey(model.PlatformUsageReportMail, ref.Start, 0), usageReportMailKey(model.CustomerStatusReportMail, ref.Start, 0),
	}).Pluck("resource", &existing).Error; err != nil {
		return false, err
	}
	var platformQueued, customersQueued bool
	var platformErr, customerErr error
	if prefs.Allows(model.MailPlatformUsage, eligible) && !slices.Contains(existing, model.PlatformUsageReportMail) {
		var total model.UsageAnalyticsMetrics
		var active int64
		total, active, platformErr = model.PublishedUsageMailTotals(ctx, ref)
		if platformErr == nil {
			var details mailErrorDetails
			if total.FailureCalls > 0 {
				details = loadUsageMailErrors(ctx, ref.Start, period.EndTimestamp, cutoff, coverageUnknown, nil, false)
			}
			platformQueued, platformErr = openUsageReportMail(ctx, model.PlatformUsageReportMail, ref.Start, 0, usageReportMailBody(period.Date, meta, total, &active, details), recipients)
		}
	}
	if prefs.Allows(model.MailCustomerStatus, eligible) && !slices.Contains(existing, model.CustomerStatusReportMail) {
		var customers model.CustomerStatusReport
		customers, customerErr = model.ReadCustomerStatusReport(ctx, ref, period.EndTimestamp)
		if customerErr == nil {
			var details mailErrorDetails
			if customers.AccountsWithFailures > 0 {
				details = loadUsageMailErrors(ctx, ref.Start, period.EndTimestamp, cutoff, coverageUnknown, nil, true)
			}
			customersQueued, customerErr = openUsageReportMail(ctx, model.CustomerStatusReportMail, ref.Start, 0, customerStatusReportMailBody(period.Date, meta, customers, now, details), recipients)
		}
	}
	return platformQueued || customersQueued, errors.Join(platformErr, customerErr)
}

func usageReportMailKey(kind string, start int64, userID int) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%d", kind, start, userID))))
}

type renderedUsageReportMail struct{ subject, body string }

func openUsageReportMail(ctx context.Context, kind string, start int64, userID int, mail renderedUsageReportMail, recipients []string) (bool, error) {
	key := usageReportMailKey(kind, start, userID)
	return model.OpenResourceAlert(ctx, model.ResourceAlertEpisode{
		OpenKey: &key, Resource: kind, MailUserID: userID, OpenedAt: start, Subject: mail.subject, BodyHTML: mail.body,
	}, recipients)
}

func loadUsageMailErrors(ctx context.Context, start, end, cutoff int64, coverageUnknown bool, accounts []int, customerOnly bool) mailErrorDetails {
	return loadMailErrorDetails(ctx, model.MailErrorScope{
		Start: max(start, cutoff), End: end, UserIDs: accounts, CustomerOnly: customerOnly, FailuresOnly: true,
	}, nil, coverageUnknown || cutoff > start)
}

func usageReportMailBody(day string, meta model.PublishedUsageReport, usage model.UsageAnalyticsMetrics, active *int64, details mailErrorDetails) renderedUsageReportMail {
	title := "个人使用日报 / Your daily usage report"
	path, action := "/usage", "查看个人用量明细 / View my usage details"
	if active != nil {
		title = "平台用量日报 / Platform daily usage report"
		path, action = "/admin/usage", "查看全客户用量明细 / View customer usage details"
	}
	var body strings.Builder
	writeUsageMailPeriod(&body, day, meta)
	count := func(value int64) string { return strconv.FormatInt(value, 10) }
	rows := make([][2]string, 0, 7)
	if active != nil {
		rows = append(rows, [2]string{"有用量记录的账户 / Accounts with usage", count(*active)})
	}
	rows = append(rows,
		[2]string{"已结束调用 / Completed calls", count(usage.TotalCalls)},
		[2]string{"成功 / 失败 / 取消 · Succeeded / Failed / Cancelled", fmt.Sprintf("%d / %d / %d", usage.SuccessCalls, usage.FailureCalls, usage.CancelledCalls)},
		[2]string{"客户净用量费用 / Net customer usage charges", logger.FormatQuota(int(usage.NetQuota))},
		[2]string{"已关联退款 / Linked refunds", logger.FormatQuota(int(usage.RefundQuota))},
	)
	if usage.OtherResultCalls > 0 {
		rows = append(rows, [2]string{"其他结果 / Other outcomes", count(usage.OtherResultCalls)})
	}
	if usage.UnlinkedRefundQuota != 0 {
		rows = append(rows, [2]string{"未关联退款 / Unlinked refunds", logger.FormatQuota(int(usage.UnlinkedRefundQuota))})
	}
	writeOperationsMailRows(&body, rows)
	rows = nil
	if usage.BillingTokensApplicable || usage.InputTokens != 0 || usage.OutputTokens != 0 || usage.RowsMissingTokens != 0 {
		rows = append(rows,
			[2]string{"输入 Token / Input tokens", usageMailMeter(count(usage.InputTokens), usage.RowsMissingTokens)},
			[2]string{"输出 Token / Output tokens", usageMailMeter(count(usage.OutputTokens), usage.RowsMissingTokens)},
		)
	}
	if usage.BillableImages != 0 || usage.BillableImageMissingRows != 0 {
		rows = append(rows, [2]string{"计费张数 / Billable images", usageMailMeter(count(usage.BillableImages), usage.BillableImageMissingRows)})
	}
	if usage.Seconds != nil || usage.SecondsMissingRows != 0 {
		seconds := "0"
		if usage.Seconds != nil {
			seconds = usage.Seconds.String()
		}
		rows = append(rows, [2]string{"秒数 / Seconds", usageMailMeter(seconds, usage.SecondsMissingRows)})
	}
	if len(rows) > 0 {
		body.WriteString(`<h3 style="margin:18px 0 0;font-size:15px">计量 / Usage meters</h3>`)
		writeOperationsMailRows(&body, rows)
	}
	if usage.BillingMeterUnknownRows > 0 || usage.RowsMoneyPending > 0 || usage.RowsMissingMoney > 0 {
		body.WriteString("<p>部分用量的计费单位或金额待核对，详见控制台。<br>Some usage units or charges require review. See the console for details.</p>")
	}
	if details.checked {
		body.WriteString("<p>以下是同日的诊断日志样本，日志记录与用量失败统计采用各自口径。<br>These diagnostic samples are from the same day. Usage failure totals follow the usage report’s counting rules.</p>")
		writeMailErrorDetails(&body, details)
		fmt.Fprintf(&body, "<p>该日用量共记录 %d 次失败。请在用量页核对对应用量结果。<br>The usage report records %d failed calls. Check the usage page for their outcomes.</p>", usage.FailureCalls, usage.FailureCalls)
	}
	if address := consoleMailLink(path + "?date=" + day + "&period=day"); address != "" {
		writeReportMailLink(&body, address, action)
	} else {
		body.WriteString("<p>请登录控制台并在用量页面选择上述日期。<br>Sign in to the console and select this date on the usage page.</p>")
	}
	return renderedUsageReportMail{subject: title + " — " + day, body: RenderMailTemplate(title, body.String())}
}

func writeUsageMailPeriod(body *strings.Builder, day string, meta model.PublishedUsageReport) {
	fmt.Fprintf(body, `<p style="margin:0 0 14px;color:#49627a">%s · 00:00–24:00 · 北京时间 / Beijing time<br>用量数据截至 / Usage data as of %s</p>`,
		html.EscapeString(day), time.Unix(meta.AsOf, 0).In(model.UsageAnalyticsZone()).Format("2006-01-02 15:04:05"))
	if len(meta.IncompleteDays)+len(meta.RebuildingDays)+len(meta.SourceUnavailableDays) > 0 {
		body.WriteString(`<p style="background:#fff4df;color:#713d09;padding:10px 12px"><strong>部分数据或待更新 / Partial or awaiting refresh</strong><br>调用与用量指标基于当前已记录数据。请在控制台核对最新发布状态。<br>Call and usage figures reflect recorded data. Check the console for the latest publication status.</p>`)
	}
}

func usageMailMeter(value string, missing int64) string {
	if missing == 0 {
		return value
	}
	if value == "0" {
		return "未记录 / Unavailable"
	}
	return value + "（已知小计 / Known subtotal）"
}
