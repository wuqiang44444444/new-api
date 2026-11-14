package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"html"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// QueueWalletOperationMail adds the receipt and any finance alert to the
// existing mail queue in the same transaction as the wallet operation.
func QueueWalletOperationMail(tx *gorm.DB, operation model.WalletOperation) error {
	prefs, err := model.MailPreferencesInTransaction(tx)
	if err != nil {
		return err
	}
	if prefs.Enabled(model.MailWalletReceipt) {
		email, err := model.MailUserEmail(tx, operation.UserID)
		if err != nil {
			return err
		}
		if common.IsBareMailAddress(email) {
			const title = "钱包余额变更回执 / Wallet balance receipt"
			if _, err := model.OpenResourceAlertInTx(tx, model.ResourceAlertEpisode{
				Resource: model.WalletReceiptMail, MailUserID: operation.UserID, OpenedAt: operation.CreatedAt,
				Subject: title, BodyHTML: walletOperationMailBody(title, operation, false),
			}, []string{email}); err != nil {
				return err
			}
		}
	}
	delta := operation.After - operation.Before
	limit, _ := common.WalletQuotaFromDecimalStrict(decimal.NewFromFloat(common.QuotaPerUnit).Mul(decimal.NewFromInt(100)))
	if configured, err := strconv.Atoi(os.Getenv("FINANCE_WALLET_ALERT_THRESHOLD")); err == nil && configured > 0 && configured <= common.MaxWalletQuota {
		limit = configured
	}
	if !(delta < 0 && operation.After <= 0) && !(limit > 0 && (delta >= limit || delta <= -limit)) {
		return nil
	}
	var finance []string
	if prefs.Enabled(model.MailWalletRisk) {
		finance, err = model.FinanceMailRecipients(tx)
		if err != nil {
			return err
		}
	}

	const title = "钱包高风险变更 / High-risk wallet adjustment"
	_, err = model.OpenResourceAlertInTx(tx, model.ResourceAlertEpisode{
		Resource: model.WalletRiskMail, OpenedAt: operation.CreatedAt,
		Subject: title, BodyHTML: walletOperationMailBody(title, operation, true),
	}, finance)
	return err
}

func WakeWalletMailDelivery() { queueResourceAlertDeliveries() }

func walletOperationLabel(kind string) string {
	switch kind {
	case "add":
		return "人工加额 / Manual credit"
	case "subtract":
		return "人工扣额 / Manual debit"
	case "override":
		return "覆盖额度 / Manual override"
	case "manual_topup":
		return "管理员补单 / Manual top-up completion"
	default:
		return kind
	}
}

func walletOperationMailBody(title string, operation model.WalletOperation, finance bool) string {
	var body strings.Builder
	rows := [][2]string{
		{"业务流水 / Operation", strconv.FormatInt(operation.ID, 10)},
		{"变更 / Change", walletOperationLabel(operation.Kind)},
		{"变更前 / Before", logger.FormatQuota(operation.Before)},
		{"变更金额 / Change amount", logger.FormatQuota(operation.After - operation.Before)},
		{"变更后 / After", logger.FormatQuota(operation.After)},
		{"入账时间 / Posted at (UTC+8)", time.Unix(operation.CreatedAt, 0).In(errorReportLocation()).Format("2006-01-02 15:04:05")},
	}
	if finance {
		rows = append(rows, [2]string{"用户编号 / Account ID", strconv.Itoa(operation.UserID)},
			[2]string{"操作人编号 / Operator ID", strconv.Itoa(operation.ActorID)})
	}
	writeOperationsMailRows(&body, rows)
	path, label := "/wallet", "打开钱包 / Open wallet"
	if finance {
		path, label = "/usage-logs/audit", "查看审计日志 / Open audit logs"
	}
	if address := consoleMailLink(path); address != "" {
		fmt.Fprintf(&body, `<p><a href="%s">%s</a></p>`, html.EscapeString(address), label)
	}
	return renderOperationsMail(title, body.String())
}

type walletSummaryHandler struct{}

func init() { RegisterSystemTaskHandler(walletSummaryHandler{}) }

func (walletSummaryHandler) Type() string { return "wallet_finance_summary" }
func (walletSummaryHandler) Enabled() bool {
	recipients, err := model.FinanceMailRecipients(model.DB)
	return err == nil && len(recipients) > 0
}
func (walletSummaryHandler) Interval() time.Duration { return 15 * time.Minute }
func (walletSummaryHandler) NewPayload() any         { return nil }
func (walletSummaryHandler) Run(ctx context.Context, task *model.SystemTask, runner string) {
	if err := queueWalletFinanceSummary(ctx, time.Now()); err != nil {
		failSystemTask(task, runner, errors.New("could not queue daily wallet finance summary"))
		return
	}
	queueResourceAlertDeliveries()
	if err := model.FinishSystemTask(task.TaskID, runner, model.SystemTaskStatusSucceeded, nil, ""); err != nil {
		logSystemTaskLockError(ctx, task, err)
	}
}

func queueWalletFinanceSummary(ctx context.Context, now time.Time) error {
	now = now.In(errorReportLocation())
	if now.Hour() < 6 {
		return nil
	}
	recipients, err := model.FinanceMailRecipients(model.DB.WithContext(ctx))
	if err != nil || len(recipients) == 0 {
		return err
	}
	end := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	start := end.AddDate(0, 0, -1)
	prefs, err := model.LoadMailPreferences(ctx)
	if err != nil || !prefs.Allows(model.MailWalletSummary, end.Add(6*time.Hour)) {
		return err
	}
	key := fmt.Sprintf("%x", sha256.Sum256([]byte(model.WalletSummaryMail+start.Format("2006-01-02"))))
	var existing int64
	if err := model.DB.WithContext(ctx).Model(&model.ResourceAlertEpisode{}).Where("open_key = ?", key).Count(&existing).Error; err != nil || existing > 0 {
		return err
	}
	summaries, err := model.ReadWalletOperationsSummary(ctx, start.Unix(), end.Unix())
	if err != nil || len(summaries) == 0 {
		return err
	}
	day := start.Format("2006-01-02")
	title := "钱包人工操作日报 / Manual wallet operations — " + day
	var body strings.Builder
	body.WriteString("<p>北京时间前一自然日已提交的人工改额与补单。逐笔记录请查看后台审计和钱包日志。<br>Committed manual wallet adjustments and top-up completions for the previous Beijing calendar day. See the audit and wallet logs for details.</p>")
	rows := make([][2]string, 0, len(summaries)*3)
	for _, summary := range summaries {
		rows = append(rows,
			[2]string{walletOperationLabel(summary.Kind), fmt.Sprintf("%d 笔 / %d operations", summary.Count, summary.Count)},
			[2]string{"增加额度合计 / Total credited (quota units)", summary.Credited.String()},
			[2]string{"扣减额度合计 / Total debited (quota units)", summary.Debited.String()},
		)
	}
	writeOperationsMailRows(&body, rows)
	if address := consoleMailLink("/usage-logs/audit"); address != "" {
		fmt.Fprintf(&body, `<p><a href="%s">查看审计日志 / Open audit logs</a></p>`, html.EscapeString(address))
	}
	_, err = model.OpenResourceAlert(ctx, model.ResourceAlertEpisode{
		OpenKey: &key, Resource: model.WalletSummaryMail, OpenedAt: now.Unix(), Subject: title, BodyHTML: renderOperationsMail(title, body.String()),
	}, recipients)
	return err
}
