package service

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDailyUsageEmailsFromPublishedReportReachLocalInbox(t *testing.T) {
	db := setupVersionServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.ResourceAlertEpisode{}, &model.ResourceAlertDelivery{}, &model.SystemTask{}, &model.SystemTaskLock{}, &model.TaskBillingDelivery{}))
	prepareDailyReportExportTest(t)
	inbox := newLocalMailInbox(t)
	t.Setenv("PERSONAL_USAGE_REPORT_MAIL_ENABLED", "true")
	oldAddress := system_setting.ServerAddress
	system_setting.ServerAddress = "https://gateway.example.com"
	t.Cleanup(func() { system_setting.ServerAddress = oldAddress })
	accounts := []model.User{
		{Id: 7, Username: "first", AffCode: "um07", Email: "first@example.com", Status: common.UserStatusEnabled},
		{Id: 8, Username: "second", AffCode: "um08", Email: "second@example.com", Status: common.UserStatusEnabled, Role: common.RoleAdminUser},
		{Id: 9, Username: "disabled", AffCode: "um09", Email: "disabled@example.com", Status: common.UserStatusDisabled},
		{Id: 10, Username: "no-activity", AffCode: "um10", Email: "idle@example.com", Status: common.UserStatusEnabled},
		{Id: 11, Username: "changed", AffCode: "um11", Email: "stale@example.com", Status: common.UserStatusEnabled},
		{Id: 12, Username: "failed", AffCode: "um12", Status: common.UserStatusEnabled, Quota: 100},
	}
	require.NoError(t, db.Create(&accounts).Error)
	ctx := context.Background()
	zone := model.UsageAnalyticsZone()
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, zone)
	require.NoError(t, db.Model(&model.User{}).Where("id IN ?", []int{7, 8, 12}).Update("created_at", start.Unix()).Error)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", 7).Update("quota", 100).Error)
	tick := start.Add(30 * time.Hour)
	queue := func(at time.Time) bool {
		t.Helper()
		queued, err := queueUsageReportEmails(ctx, at)
		require.NoError(t, err)
		return queued
	}
	require.False(t, queue(tick), "an unpublished day cannot generate a zero-usage message")
	logs := []model.Log{
		{UserId: 7, CreatedAt: start.Unix() + 10, Type: model.LogTypeConsume, ModelName: "first-model", Quota: 135001, RequestId: "first-private-request", TokenName: "first-private-key"},
		{UserId: 8, CreatedAt: start.Unix() + 20, Type: model.LogTypeConsume, ModelName: "second-model", Quota: 245002, RequestId: "second-private-request", TokenName: "second-private-key"},
		{UserId: 9, CreatedAt: start.Unix() + 30, Type: model.LogTypeConsume, ModelName: "disabled-model", Quota: 352003, RequestId: "disabled-request"},
		{UserId: 11, CreatedAt: start.Unix() + 40, Type: model.LogTypeConsume, ModelName: "stale-model", Quota: 412004, RequestId: "stale-request"},
		{UserId: 12, CreatedAt: start.Unix() + 50, Type: model.LogTypeError, ModelName: "failed-model", RequestId: "failed-request", ChannelId: 1},
	}
	require.NoError(t, db.Create(&logs).Error)
	require.NoError(t, model.BuildUsageReportDay(ctx, start.Format(time.DateOnly), "export-fixture", "runner", nil))
	loadTransport := common.LoadMailTransport
	func() {
		common.LoadMailTransport = func(context.Context) (common.MailTransportConfig, error) {
			return common.MailTransportConfig{}, nil
		}
		defer func() { common.LoadMailTransport = loadTransport }()
		require.False(t, queue(tick))
		var deliveries int64
		require.NoError(t, db.Model(&model.ResourceAlertDelivery{}).Count(&deliveries).Error)
		assert.Zero(t, deliveries)
	}()
	require.False(t, queue(tick.Add(-time.Minute)))
	require.True(t, queue(tick), "personal reports work with no operations recipient configured")
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", 11).Update("email", "current@example.com").Error)
	require.NoError(t, deliverResourceAlerts(time.Now, common.SendEmailContext))
	first := requireLocalMail(t, inbox, "Your daily usage report", "first@example.com")
	second := requireLocalMail(t, inbox, "Your daily usage report", "second@example.com")
	for _, message := range []inboxMail{first, second} {
		assert.Contains(t, message.body, "个人使用日报")
		assert.Contains(t, message.body, "/usage?date=2026-10-01&amp;period=day")
		assert.NotContains(t, message.body, "private-")
		assert.NotContains(t, message.body, "Accounts with usage")
	}
	assert.Contains(t, first.body, logger.FormatQuota(135001))
	assert.NotContains(t, first.body, logger.FormatQuota(245002))
	assert.Contains(t, second.body, logger.FormatQuota(245002))
	assert.NotContains(t, second.body, logger.FormatQuota(135001))

	require.NoError(t, db.Create(&model.Option{Key: "error_report_setting.recipients", Value: "ops@example.com"}).Error)
	require.True(t, queue(tick))
	require.NoError(t, deliverResourceAlerts(time.Now, common.SendEmailContext))
	ops := requireLocalMail(t, inbox, "Platform daily usage report", "ops@example.com")
	assert.Contains(t, ops.body, "/admin/usage?date=2026-10-01&amp;period=day")
	assert.Contains(t, ops.body, logger.FormatQuota(1144010))
	assert.Contains(t, ops.body, "Accounts with usage")
	assert.NotContains(t, ops.body, "@example.com")
	assert.NotContains(t, ops.body, "private-")
	customers := requireLocalMail(t, inbox, "Daily customer activity report", "ops@example.com")
	assert.Contains(t, customers.body, "客户情况日报")
	assert.Contains(t, customers.body, "/admin/usage?date=2026-10-01&amp;period=day&amp;section=customers")
	assert.Contains(t, customers.body, `href="https://gateway.example.com/users"`)
	for label, value := range map[string]string{
		"New customer accounts": "2", "Customers with completed calls": "4", "New customers with completed calls": "2",
		"Customers with failed calls": "1", "Customers whose completed calls all failed": "1",
		"Enabled customers with zero or negative wallet balance": "2",
	} {
		assert.Regexp(t, regexp.QuoteMeta(label)+`</th><td[^>]*>`+value+`</td>`, customers.body)
	}
	assert.NotContains(t, customers.body, "@example.com")
	assert.NotContains(t, customers.body, "first-private")
	assert.False(t, queue(tick.Add(time.Minute)))
	var deliveries []model.ResourceAlertDelivery
	require.NoError(t, db.Order("id").Find(&deliveries).Error)
	require.Len(t, deliveries, 5)
	assert.Equal(t, model.MailDeliveryStopped, deliveries[2].Status)
	assert.Equal(t, "stale@example.com", deliveries[2].Recipient)
	_, resources, err := model.ReadDailyOperationsCounts(ctx, start.Unix(), start.AddDate(0, 0, 1).Unix())
	require.NoError(t, err)
	assert.Empty(t, resources)

	// A published empty day is distinct from the unpublished day above.
	empty := start.AddDate(0, 0, 1)
	require.NoError(t, model.BuildUsageReportDay(ctx, empty.Format(time.DateOnly), "export-fixture", "runner", nil))
	t.Setenv("PERSONAL_USAGE_REPORT_MAIL_ENABLED", "false")
	require.True(t, queue(tick.AddDate(0, 0, 1)))
	require.NoError(t, deliverResourceAlerts(time.Now, common.SendEmailContext))
	emptyMail := requireLocalMail(t, inbox, "Platform daily usage report", "ops@example.com")
	assert.Contains(t, emptyMail.body, "Completed calls</th>")
	assert.Contains(t, emptyMail.body, ">0</td>")
	assert.NotContains(t, emptyMail.body, "Partial or awaiting refresh")
	requireLocalMail(t, inbox, "Daily customer activity report", "ops@example.com")
}

func TestUsageMailTemplateMarksIncompleteMeters(t *testing.T) {
	mail := usageReportMailBody("2026-10-01", model.PublishedUsageReport{IncompleteDays: []string{"2026-10-01"}}, model.UsageAnalyticsMetrics{
		InputTokens: 123, RowsMissingTokens: 1, SecondsMissingRows: 1, RowsMoneyPending: 1,
	}, nil)
	assert.Contains(t, mail.body, "Partial or awaiting refresh")
	assert.Contains(t, mail.body, "123（已知小计 / Known subtotal）")
	assert.Contains(t, mail.body, "未记录 / Unavailable")
	assert.Contains(t, mail.body, "Some usage units or charges require review")
}
