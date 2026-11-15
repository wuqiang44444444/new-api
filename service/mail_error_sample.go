package service

import (
	"context"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

type mailErrorDetails struct {
	checked, unavailable, incomplete bool
	rows                             []model.MailErrorSample
	accounts                         map[int]model.MailAccount
}

func loadMailErrorDetails(ctx context.Context, scope model.MailErrorScope, extraIDs []int, incomplete bool) mailErrorDetails {
	ctx, cancel := context.WithTimeout(ctx, model.DailyReportReadBudget)
	defer cancel()
	detail := mailErrorDetails{checked: true, incomplete: incomplete}
	rows, err := model.ReadMailErrorSamples(ctx, scope)
	if err != nil {
		detail.unavailable = true
		common.SysError("email error samples could not be read")
	}
	detail.rows = rows
	ids := append([]int(nil), extraIDs...)
	for _, row := range rows {
		if row.UserID > 0 {
			ids = append(ids, row.UserID)
		}
	}
	if len(ids) > 0 {
		detail.accounts, err = model.ReadMailAccounts(model.DB.WithContext(ctx), ids)
		if err != nil {
			common.SysError("email account labels could not be read")
		}
	}
	return detail
}

var mailAddressInField = regexp.MustCompile(`(?i)[a-z0-9.!#$%&'*+/=?^_{}|~-]+@[a-z0-9.-]+\.[a-z]{2,}`)

func safeMailRequestID(s string) string {
	if len(s) > 96 {
		return ""
	}
	for _, ch := range s {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '-' || ch == '_' || ch == '.') {
			return ""
		}
	}
	return s
}

func safeMailText(s string, limit int) string {
	s = mailAddressInField.ReplaceAllString(common.SanitizeTaskDiagnostic(s), "[redacted]")
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	runes := []rune(strings.TrimSpace(s))
	if len(runes) > limit {
		return string(runes[:limit]) + "…"
	}
	return string(runes)
}

func mailAccountLabel(id int, account model.MailAccount, snapshot string) string {
	for _, name := range []string{account.DisplayName, account.Username, snapshot} {
		if safe := safeMailText(name, 48); safe != "" {
			return fmt.Sprintf("%s (#%d)", safe, id)
		}
	}
	return fmt.Sprintf("名称不可用 / Name unavailable (#%d)", id)
}

func mailErrorReason(row model.MailErrorSample) string {
	switch row.Reason {
	case clienterrlog.WalletQuotaInsufficient:
		return "钱包余额不足 / Insufficient wallet balance"
	case clienterrlog.SubscriptionQuotaInsufficient:
		return "订阅不可用或额度不足 / Subscription unavailable or quota insufficient"
	case clienterrlog.APIKeyQuotaInsufficient:
		return "API Key 额度不足 / Insufficient API key quota"
	}
	switch row.EventType {
	case clienterrlog.EventTaskFailure:
		return "任务失败或到期 / Task failed or expired"
	case clienterrlog.EventChannelTest:
		return "渠道测试失败 / Channel test failed"
	case clienterrlog.EventStreamError:
		switch row.Reason {
		case "client_disconnected", "upstream_response_cancelled":
			return "流式连接取消或断开 / Stream cancelled or disconnected"
		case "stream_soft_errors", "stream_stopped_with_error":
			return "流式异常 / Stream warning"
		default:
			return "流式响应中断 / Stream interrupted"
		}
	}
	switch {
	case row.Status == 429:
		return "请求受到限流 / Request rate limited"
	case row.Status >= 500:
		return "服务错误 / Service error"
	case row.Status >= 400:
		return "请求被拒绝 / Request rejected"
	default:
		return "已记录错误 / Recorded error"
	}
}

func writeMailErrorDetails(body *strings.Builder, detail mailErrorDetails) {
	if !detail.checked {
		return
	}
	body.WriteString("<h3>最近错误（最多 5 条） / Latest errors (up to 5)</h3>")
	if detail.unavailable {
		body.WriteString("<p>明细暂不可用。 / Error details are temporarily unavailable.</p>")
	} else if len(detail.rows) == 0 {
		body.WriteString("<p>未找到匹配的错误记录。 / No matching errors were found.</p>")
	}
	if detail.incomplete {
		body.WriteString("<p>以下内容仅覆盖可读取的日志。 / Details cover the available logs only.</p>")
	}
	for _, row := range detail.rows {
		label := time.Unix(row.CreatedAt, 0).In(errorReportLocation()).Format("01-02 15:04:05")
		if row.UserID > 0 {
			label += " · " + mailAccountLabel(row.UserID, detail.accounts[row.UserID], row.Username)
		}
		parts := make([]string, 0, 4)
		if route := safeMailText(strings.SplitN(row.Route, "?", 2)[0], 96); route != "" {
			parts = append(parts, strings.TrimSpace(safeMailText(row.Method, 12)+" "+route))
		}
		if modelName := safeMailText(row.ModelName, 72); modelName != "" {
			parts = append(parts, modelName)
		}
		if row.Status >= 400 {
			parts = append(parts, "HTTP "+strconv.Itoa(row.Status))
		}
		if request := safeMailRequestID(row.RequestID); request != "" {
			parts = append(parts, "请求 / Request "+request)
		}
		writeMailEvidenceCard(body, label, mailErrorReason(row), strings.Join(parts, " · "))
	}
}

func writeMailEvidenceCard(body *strings.Builder, title, reason, details string) {
	fmt.Fprintf(body, `<div style="margin:8px 0;padding:10px 12px;border:1px solid #dce4ee;overflow-wrap:anywhere"><strong>%s</strong><br>%s`, html.EscapeString(title), html.EscapeString(reason))
	if details != "" {
		fmt.Fprintf(body, `<br><span style="font-size:12px;color:#49627a">%s</span>`, html.EscapeString(details))
	}
	body.WriteString("</div>")
}
