package service

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWalletOperationMailReceiptsFinanceAlertsAndDailySummary(t *testing.T) {
	db := withErrorReportTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}, &model.Log{}, &model.WalletOperation{}, &model.WalletOperationMailPrepared{}))
	t.Setenv("FINANCE_MAIL_RECIPIENTS", "finance@example.com")
	t.Setenv("FINANCE_WALLET_ALERT_THRESHOLD", "1000")
	inbox := newLocalMailInbox(t)
	ctx := context.Background()
	user := model.User{Id: 42, Username: "account", DisplayName: "Finance Studio", Email: "account@example.com", Quota: 500, Status: common.UserStatusEnabled}
	user.SetSetting(dto.UserSetting{NotifyType: dto.NotifyTypeWebhook, NotificationEmail: "other@example.com"})
	require.NoError(t, db.Create(&user).Error)
	admin := model.User{Id: 43, Username: "wallet-admin", AffCode: "wa43", Role: common.RoleAdminUser, Email: "admin@example.com", Quota: 500, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&admin).Error)
	_, err := model.AdjustUserQuotaByAdmin(user.Id, common.RoleRootUser, "add", 10, 7)
	require.NoError(t, err)
	_, err = model.AdjustUserQuotaByAdmin(user.Id, common.RoleRootUser, "subtract", 510, 7)
	require.NoError(t, err)
	_, err = model.AdjustUserQuotaByAdmin(user.Id, common.RoleRootUser, "override", 0, 7)
	require.NoError(t, err)

	order := model.TopUp{UserId: user.Id, Amount: 1, TradeNo: "fixture-order", Status: common.TopUpStatusPending, PaymentProvider: model.PaymentProviderEpay}
	require.NoError(t, db.Create(&order).Error)
	require.NoError(t, model.ManualCompleteTopUp(order.TradeNo, "", 7))
	require.NoError(t, model.ManualCompleteTopUp(order.TradeNo, "", 7))
	var operations int64
	require.NoError(t, db.Model(&model.WalletOperation{}).Count(&operations).Error)
	assert.EqualValues(t, 3, operations)

	require.NoError(t, queueWalletOperationMails(ctx))
	require.NoError(t, queueWalletOperationMails(ctx))
	var deliveries []model.ResourceAlertDelivery
	require.NoError(t, db.Order("id").Find(&deliveries).Error)
	require.Len(t, deliveries, 5)
	deliver := func() { require.NoError(t, deliverResourceAlerts(time.Now, common.SendEmailContext)) }
	deliver()
	deliver()
	requireLocalMail(t, inbox, "Wallet balance receipt", user.Email)
	debit := requireLocalMail(t, inbox, "Wallet balance receipt", user.Email)
	assert.NotContains(t, debit.body, "Operator ID")
	assert.Contains(t, debit.body, "Finance Studio (#42)")
	requireLocalMail(t, inbox, "High-risk wallet adjustment", "finance@example.com")
	receipt := requireLocalMail(t, inbox, "Wallet balance receipt", user.Email)
	assert.Contains(t, receipt.body, "Manual top-up completion")
	assert.NotContains(t, receipt.body, order.TradeNo)
	requireLocalMail(t, inbox, "High-risk wallet adjustment", "finance@example.com")
	_, err = model.AdjustUserQuotaByAdmin(admin.Id, common.RoleRootUser, "override", 550, 7)
	require.NoError(t, err)
	require.NoError(t, queueWalletOperationMails(ctx))
	deliver()
	adminMail := requireLocalMail(t, inbox, "Wallet balance receipt", admin.Email)
	assert.Contains(t, adminMail.body, "Manual override")

	// The finance summary has its own recipients even without an operations list.
	now := time.Now().In(errorReportLocation())
	tomorrowMorning := time.Date(now.Year(), now.Month(), now.Day()+1, 6, 0, 0, 0, now.Location())
	require.NoError(t, queueWalletFinanceSummary(ctx, tomorrowMorning))
	deliver()
	daily := requireLocalMail(t, inbox, "Manual wallet operations", "finance@example.com")
	assert.Contains(t, daily.body, "Manual top-up completion")
	assert.Contains(t, daily.body, "Manual debit")
	assert.Contains(t, daily.body, "Total credited (quota units)")
	assert.Contains(t, daily.body, "Latest manual operations (up to 5)")
	assert.Contains(t, daily.body, "Finance Studio (#42)")
	assert.Contains(t, daily.body, "Total debited (quota units)")
	assert.Contains(t, daily.body, ">510</td>")
	require.NoError(t, queueWalletFinanceSummary(ctx, tomorrowMorning))
	var summaries int64
	require.NoError(t, db.Model(&model.ResourceAlertEpisode{}).Where("resource = ?", model.WalletSummaryMail).Count(&summaries).Error)
	assert.EqualValues(t, 1, summaries)
}
