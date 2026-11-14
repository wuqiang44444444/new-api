package service

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/model"
)

func customerStatusReportMailBody(day string, meta model.PublishedUsageReport, report model.CustomerStatusReport, observedAt time.Time) renderedUsageReportMail {
	const title = "客户情况日报 / Daily customer activity report"
	var body strings.Builder
	writeUsageMailPeriod(&body, day, meta)
	body.WriteString(`<h3 style="margin:18px 0 0;font-size:15px">新增与使用 / Acquisition and usage</h3>`)
	count := func(n int64) string { return strconv.FormatInt(n, 10) }
	writeOperationsMailRows(&body, [][2]string{
		{"新增客户账户 / New customer accounts", count(report.NewAccounts)},
		{"有已结束调用的客户 / Customers with completed calls", count(report.AccountsWithCalls)},
		{"当日有已结束调用的新客户 / New customers with completed calls", count(report.NewAccountsWithCalls)},
		{"有失败调用的客户 / Customers with failed calls", count(report.AccountsWithFailures)},
		{"已结束调用全部失败的客户 / Customers whose completed calls all failed", count(report.AccountsWithOnlyFailures)},
	})
	if address := consoleMailLink("/admin/usage?date=" + day + "&period=day&section=customers"); address != "" {
		writeReportMailLink(&body, address, "打开当日客户用量 / View customer usage for this day")
	}
	body.WriteString(`<h3 style="margin:18px 0 0;font-size:15px">账户状态 / Account status</h3>`)
	fmt.Fprintf(&body, `<p style="margin:4px 0;color:#49627a">统计于 / Counted at %s（北京时间 / Beijing time）</p>`,
		observedAt.In(model.UsageAnalyticsZone()).Format("2006-01-02 15:04:05"))
	writeOperationsMailRows(&body, [][2]string{
		{"启用客户账户 / Enabled customer accounts", count(report.EnabledAccounts)},
		{"停用客户账户 / Disabled customer accounts", count(report.DisabledAccounts)},
		{"钱包余额小于等于零的启用客户 / Enabled customers with zero or negative wallet balance", count(report.NonPositiveWallets)},
	})
	body.WriteString("<p>统计范围为普通客户账户。钱包余额按统计时点读取，使用订阅的客户也可能出现在该项。<br>Counts cover customer accounts. Wallet balances are measured at collection time and can include customers using subscriptions.</p>")
	if address := consoleMailLink("/users"); address != "" {
		writeReportMailLink(&body, address, "打开用户管理 / Open user management")
	}
	return renderedUsageReportMail{subject: title + " — " + day, body: renderOperationsMail(title, body.String())}
}
