package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"html"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/model"
)

const walletRejectionWindow = 10 * time.Minute

type walletNoticeHandler struct{ recoveryCursor atomic.Int64 }

func init() { RegisterSystemTaskHandler(&walletNoticeHandler{}) }

func (*walletNoticeHandler) Type() string            { return "wallet_notice_mail" }
func (*walletNoticeHandler) Enabled() bool           { return true }
func (*walletNoticeHandler) Interval() time.Duration { return time.Minute }
func (*walletNoticeHandler) NewPayload() any         { return nil }

func (h *walletNoticeHandler) Run(ctx context.Context, task *model.SystemTask, runner string) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	now := time.Now()
	digestErr := queueWalletRejectionDigest(ctx, now)
	recoveryErr := h.recoverWalletNotices(ctx, now)
	operationErr := queueWalletOperationMails(ctx)
	local := now.In(errorReportLocation())
	cutoff := time.Date(local.Year(), local.Month(), local.Day()-1, 0, 0, 0, 0, local.Location())
	pruneErr := model.PruneWalletDigestAccounts(ctx, cutoff.Unix())
	queueResourceAlertDeliveries()
	if digestErr != nil || recoveryErr != nil || operationErr != nil || pruneErr != nil {
		failSystemTask(task, runner, errors.New("could not process wallet notices"))
		return
	}
	if err := model.FinishSystemTask(task.TaskID, runner, model.SystemTaskStatusSucceeded, nil, ""); err != nil {
		logSystemTaskLockError(ctx, task, err)
	}
}

func queueWalletRejectionDigest(ctx context.Context, now time.Time) error {
	prefs, err := model.LoadMailPreferences(ctx)
	if err != nil || !prefs.Enabled(model.MailWalletRejection) {
		return err
	}
	recipients, err := model.GetOperationsMailRecipients(ctx)
	if err != nil || len(recipients) == 0 {
		return err
	}
	// Leave a minute for the diagnostic writer before closing a window.
	end := now.Add(-time.Minute).Truncate(walletRejectionWindow)
	start := end.Add(-walletRejectionWindow)
	local := start.In(errorReportLocation())
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
	changedAt := prefs.Types[model.MailWalletRejection].ChangedAt
	enabledAt := time.Unix(0, changedAt)
	// Error timestamps are stored at second precision. Skip the partial second
	// during which the switch was changed.
	if enabledAt.After(start) {
		start = enabledAt.Truncate(time.Second).Add(time.Second)
	}
	if !start.Before(end) {
		return nil
	}
	key := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s:%d", model.CustomerWalletRejectionMail, end.Unix()))))
	var exists int64
	if err := model.DB.WithContext(ctx).Model(&model.ResourceAlertEpisode{}).Where("open_key = ?", key).Count(&exists).Error; err != nil || exists > 0 {
		return err
	}
	readCtx, cancel := context.WithTimeout(ctx, model.DailyReportReadBudget)
	counts, err := model.ReadWalletRejectionCounts(readCtx, start.Unix(), end.Unix())
	cancel()
	if err != nil || len(counts) == 0 {
		return err
	}
	prepared, err := model.SelectWalletDigestAccounts(model.DB.WithContext(ctx), day.Unix(), counts)
	if err != nil || len(prepared) == 0 {
		return err
	}
	ids := make([]int, 0, len(prepared))
	for _, row := range prepared {
		ids = append(ids, row.UserID)
	}
	details := loadMailErrorDetails(ctx, model.MailErrorScope{
		Start: start.Unix(), End: end.Unix(), UserIDs: ids, Reason: clienterrlog.WalletQuotaInsufficient,
	}, ids[:min(10, len(ids))], false)
	episode := renderWalletRejectionDigest(key, start, end, prepared, details)
	return model.PublishWalletRejectionDigest(ctx, day.Unix(), end, changedAt, recipients, counts, prepared, episode)
}

func renderWalletRejectionDigest(key string, start, end time.Time, accounts []model.WalletRejectionCount, details mailErrorDetails) model.ResourceAlertEpisode {
	title := "客户调用因钱包余额不足被拒 / Requests rejected due to insufficient wallet balance"
	var body strings.Builder
	var total int64
	for _, row := range accounts {
		total += row.Rejections
	}
	writeOperationsMailRows(&body, [][2]string{
		{"时间窗（北京时间） / Window (Beijing time)", start.In(errorReportLocation()).Format("2006-01-02 15:04:05") + " — " + end.In(errorReportLocation()).Format("15:04:05")},
		{"当日首次纳入摘要的账户 / Accounts first reported today", strconv.Itoa(len(accounts))},
		{"这些账户的已入库拒绝次数 / Rejections recorded at aggregation", strconv.FormatInt(total, 10)},
	})
	body.WriteString("<h3>受影响账户（最多 10 个） / Affected accounts (up to 10)</h3>")
	snapshots := make(map[int]string)
	for _, row := range details.rows {
		if snapshots[row.UserID] == "" {
			snapshots[row.UserID] = row.Username
		}
	}
	rows := make([][2]string, 0, min(10, len(accounts)))
	for _, row := range accounts[:min(10, len(accounts))] {
		rows = append(rows, [2]string{mailAccountLabel(row.UserID, details.accounts[row.UserID], snapshots[row.UserID]), strconv.FormatInt(row.Rejections, 10)})
	}
	writeOperationsMailRows(&body, rows)
	writeMailErrorDetails(&body, details)
	top := accounts[0]
	label := html.EscapeString(mailAccountLabel(top.UserID, details.accounts[top.UserID], snapshots[top.UserID]))
	fmt.Fprintf(&body, "<p><strong>排查建议 / What to check</strong><br>%s 的拒绝次数居首（%d/%d）。先核对钱包余额和本次预扣所需额度，再查看是否有重复重试。<br>%s had the most rejections (%d of %d). Check wallet balance and the required precharge, then look for repeated retries.</p>", label, top.Rejections, total, label, top.Rejections, total)
	if address := consoleMailLink("/usage-logs/error"); address != "" {
		if parsed, err := url.Parse(address); err == nil {
			body.WriteString(dailyReportErrorLink(*parsed, &model.ErrorReport{WindowStart: start.Unix(), WindowEnd: end.Unix()},
				url.Values{"event_type": {clienterrlog.EventAPIError}, "reason": {strconv.Quote(clienterrlog.WalletQuotaInsufficient)}},
				"查看余额拒绝记录 / View wallet rejections", true))
		}
	}
	return model.ResourceAlertEpisode{
		OpenKey: &key, Resource: model.CustomerWalletRejectionMail, OpenedAt: end.Unix(),
		Subject: title, BodyHTML: RenderMailTemplate(title, body.String()),
	}
}
