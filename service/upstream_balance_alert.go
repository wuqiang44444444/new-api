package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"slices"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"
)

type upstreamBalanceAlertHandler struct {
	send  func(context.Context, string, string, string) error
	query func(context.Context, *model.Channel, int, string) UpstreamBalanceResult
	now   func() time.Time
}

func init() {
	RegisterSystemTaskHandler(&upstreamBalanceAlertHandler{send: common.SendEmailContext, query: QueryUpstreamBalance, now: time.Now})
}

func (*upstreamBalanceAlertHandler) Type() string { return model.SystemTaskTypeUpstreamBalanceAlert }
func (*upstreamBalanceAlertHandler) Enabled() bool {
	return common.SMTPServer != "" && len(operation_setting.ParseErrorReportRecipients(operation_setting.GetErrorReportSetting().Recipients)) > 0
}
func (*upstreamBalanceAlertHandler) Interval() time.Duration { return 10 * time.Minute }
func (*upstreamBalanceAlertHandler) NewPayload() any         { return nil }

type upstreamBalanceAlertSummary struct {
	Checked int `json:"checked"`
	Low     int `json:"low"`
	Sent    int `json:"sent"`
	Skipped int `json:"skipped"`
	Failed  int `json:"failed"`
}

func (h *upstreamBalanceAlertHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	result, err := h.checkAndNotify(ctx, task.TaskID)
	status, message := model.SystemTaskStatusSucceeded, ""
	if err != nil || result.Failed > 0 {
		status, message = model.SystemTaskStatusFailed, "上游余额检查或邮件投递失败，将在下次检查重试 / Balance check or email delivery failed; retry on next check"
	}
	if err := model.FinishSystemTask(task.TaskID, runnerID, status, result, message); err != nil {
		logSystemTaskLockError(ctx, task, err)
	}
}

// classifyUpstreamBalanceAlert compares in the source currency, avoiding division
// rounding at the threshold. Unknown amounts, credits and missing rates do not
// imply recovery or low funds. Display amounts remain in the upstream currency.
func classifyUpstreamBalanceAlert(result UpstreamBalanceResult, cnyPerUSD decimal.Decimal) (known, low bool) {
	if result.Status != "ok" || len(result.Amounts) != 1 {
		return false, false
	}
	amount, err := decimal.NewFromString(result.Amounts[0].Amount)
	if err != nil {
		return false, false
	}
	threshold := decimal.NewFromInt(100)
	switch result.Amounts[0].Unit {
	case "USD":
	case "CNY":
		if !cnyPerUSD.IsPositive() {
			return false, false
		}
		threshold = threshold.Mul(cnyPerUSD)
	default:
		return false, false
	}
	return true, amount.LessThan(threshold)
}

func (h *upstreamBalanceAlertHandler) checkAndNotify(ctx context.Context, owner string) (upstreamBalanceAlertSummary, error) {
	result := upstreamBalanceAlertSummary{}
	recipients, err := model.GetUpstreamBalanceAlertRecipients(ctx)
	if err != nil {
		return result, errors.New("unable to read report recipients")
	}
	if len(recipients) == 0 {
		return result, nil
	}
	if common.SMTPServer == "" {
		return result, errors.New("SMTP not configured")
	}
	channels, err := model.GetUpstreamBalanceChannels(ctx)
	if err != nil {
		return result, errors.New("unable to read upstream connections")
	}
	byID := make(map[int]*model.Channel, len(channels))
	for _, channel := range channels {
		byID[channel.Id] = channel
	}
	rate := decimal.Zero
	if configured, err := operation_setting.CurrentUsdExchangeRateContext(); err == nil {
		rate = decimal.NewFromFloat(configured.Rate)
	}
	for _, connection := range UpstreamBalanceInventory(channels) {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		if !connection.Queryable {
			result.Skipped++
			continue
		}
		balance := h.query(ctx, byID[connection.ChannelID], connection.KeyIndex, connection.ID)
		result.Checked++
		if balance.Status == "error" {
			result.Failed++
			continue
		}
		known, low := classifyUpstreamBalanceAlert(balance, rate)
		if !known {
			result.Skipped++
			continue
		}
		if !low {
			if err := model.ResetRecoveredUpstreamBalanceAlert(ctx, connection.ID, h.now().Unix()); err != nil {
				return result, errors.New("unable to reset recovered balance alert")
			}
			continue
		}
		result.Low++
		body := renderUpstreamBalanceAlert(connection, balance, rate)
		for _, recipient := range recipients {
			if ctx.Err() != nil {
				return result, ctx.Err()
			}
			// Removing a recipient while upstream requests are in flight must stop
			// that recipient's upcoming sends, independently of option-cache sync.
			current, err := model.GetUpstreamBalanceAlertRecipients(ctx)
			if err != nil {
				return result, errors.New("unable to recheck report recipients")
			}
			if !slices.Contains(current, recipient) {
				continue
			}
			identity, _ := common.Marshal([]string{connection.ID, recipient})
			sum := sha256.Sum256(identity)
			id := hex.EncodeToString(sum[:])
			claimed, err := model.ClaimUpstreamBalanceAlert(ctx, connection.ID, id, owner, h.now().Unix(), int64((24 * time.Hour).Seconds()))
			if err != nil {
				return result, errors.New("unable to claim balance alert")
			}
			if !claimed {
				continue
			}
			sendErr := h.send(ctx, "上游余额低于 100 美元 / Upstream balance below USD 100", recipient, body)
			sentAt := int64(0)
			if sendErr == nil {
				sentAt = h.now().Unix()
				result.Sent++
			} else {
				result.Failed++
			}
			// A successful SMTP DATA acknowledgement must be recorded even if the
			// scheduler context was just cancelled. CAS still fences a new owner.
			finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			err = model.FinishUpstreamBalanceAlert(finishCtx, id, owner, sentAt)
			cancel()
			if err != nil {
				return result, errors.New("unable to record balance alert delivery")
			}
		}
	}
	return result, nil
}

func renderUpstreamBalanceAlert(connection UpstreamBalanceConnection, result UpstreamBalanceResult, rate decimal.Decimal) string {
	amount := result.Amounts[0]
	var channels strings.Builder
	for _, channel := range connection.Channels {
		fmt.Fprintf(&channels, "<li>#%d %s</li>", channel.ID, html.EscapeString(channel.Name))
	}
	rateNote := ""
	if amount.Unit == "CNY" {
		rateNote = fmt.Sprintf("<p>系统美元汇率 / System exchange rate: 1 USD = %s CNY</p>", rate.String())
	}
	scopeNote := ""
	if result.Scope == "unconfirmed" {
		scopeNote = "<p>账户 / Key 口径待确认；请核对上游后台。 / Account or key scope is unconfirmed; verify in the upstream console.</p>"
	}
	return fmt.Sprintf(`<h2>上游低余额提醒 / Low upstream balance</h2>
<p>检测到余额低于 100 USD。 / The reported balance is below USD 100.</p>
<p>上游 / Upstream: %s<br>Key: %s<br>上游返回余额 / Reported balance: <strong>%s %s</strong><br>查询时间 / Checked at (UTC): %s</p>
%s%s<ul>%s</ul>
<p>多个 Key 可能共用同一账户，请勿合计。 / Keys may share an account; do not add these balances together.</p>
<p>系统每 10 分钟检查；持续低余额每 24 小时再次提醒，恢复后再次跌破会重新提醒。 / Checked every 10 minutes; persistent low balances are reminded every 24 hours, or upon a new drop after recovery.</p>`, html.EscapeString(connection.Origin), html.EscapeString(connection.KeyLabel), html.EscapeString(amount.Amount), html.EscapeString(amount.Unit), time.Unix(result.CheckedAt, 0).UTC().Format(time.RFC3339), rateNote, scopeNote, channels.String())
}
