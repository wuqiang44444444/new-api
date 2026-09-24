package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupUpstreamBalanceAlertTest(t *testing.T) *model.Channel {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Option{}, &model.UpstreamBalanceAlertDelivery{}))
	oldDB, oldSMTP, oldRate := model.DB, common.SMTPServer, operation_setting.USDExchangeRate
	model.DB, common.SMTPServer = db, "fixture.invalid"
	require.NoError(t, operation_setting.SetUSDExchangeRate("7"))
	t.Cleanup(func() {
		model.DB, common.SMTPServer = oldDB, oldSMTP
		require.NoError(t, operation_setting.SetUSDExchangeRate(fmt.Sprint(oldRate)))
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.Create(&model.Option{Key: "error_report_setting.recipients", Value: "one@example.com;two@example.com"}).Error)
	base := "https://mm-internal-cn.leonecloud.com"
	channel := &model.Channel{Name: "<channel>", Key: "secret-private-key-1234", Type: 1, BaseURL: &base, Status: 2, Balance: 12.34, BalanceUpdatedTime: 100}
	require.NoError(t, db.Create(channel).Error)
	return channel
}

func TestUpstreamBalanceAlertThreshold(t *testing.T) {
	for _, test := range []struct {
		amount, unit, rate, status string
		known, low                 bool
	}{
		{"99.999999999999999999", "USD", "7", "ok", true, true},
		{"100", "USD", "7", "ok", true, false},
		{"0", "USD", "7", "ok", true, true},
		{"-10", "USD", "7", "ok", true, true},
		{"679.999999999999999999", "CNY", "6.8", "ok", true, true},
		{"680", "CNY", "6.8", "ok", true, false},
		{"5", "CNY", "0", "ok", false, false},
		{"5", "credits", "7", "ok", false, false},
		{"5", "", "7", "ok", false, false},
		{"5", "USD", "7", "error", false, false},
		{"100000000", "USD", "7", "unlimited", false, false},
		{"", "USD", "7", "ok", false, false},
	} {
		t.Run(test.amount+test.unit+test.status, func(t *testing.T) {
			rate, err := decimal.NewFromString(test.rate)
			require.NoError(t, err)
			known, low := classifyUpstreamBalanceAlert(UpstreamBalanceResult{Status: test.status, Amounts: []UpstreamBalanceAmount{{Amount: test.amount, Unit: test.unit}}}, rate)
			assert.Equal(t, test.known, known)
			assert.Equal(t, test.low, low)
		})
	}
}

func TestUpstreamBalanceAlertRemindersAndRecovery(t *testing.T) {
	channel := setupUpstreamBalanceAlertTest(t)
	now := time.Unix(1700000000, 0)
	amount := "99"
	status := "ok"
	query := func(context.Context, *model.Channel, int, string) UpstreamBalanceResult {
		return UpstreamBalanceResult{Status: status, Amounts: []UpstreamBalanceAmount{{Amount: amount, Unit: "USD"}}, CheckedAt: now.Unix()}
	}
	var receivers []string
	send := func(_ context.Context, subject, receiver, body string) error {
		receivers = append(receivers, receiver)
		assert.Contains(t, subject, "100")
		assert.Contains(t, body, "&lt;channel&gt;")
		assert.Contains(t, body, "••••1234")
		assert.NotContains(t, body, channel.Key)
		assert.Contains(t, body, "Reported balance")
		assert.Contains(t, body, "上游返回余额")
		return nil
	}
	h := &upstreamBalanceAlertHandler{query: query, send: send, now: func() time.Time { return now }}
	result, err := h.checkAndNotify(context.Background(), "first-run")
	require.NoError(t, err)
	assert.Equal(t, 2, result.Sent)
	assert.ElementsMatch(t, []string{"one@example.com", "two@example.com"}, receivers)
	// New process/handler still sees the persisted recipient cooldowns.
	h = &upstreamBalanceAlertHandler{query: query, send: send, now: func() time.Time { return now }}
	now = now.Add(10 * time.Minute)
	result, err = h.checkAndNotify(context.Background(), "next-run")
	require.NoError(t, err)
	assert.Zero(t, result.Sent)
	// An unavailable result cannot reset the low episode.
	status = "unavailable"
	_, err = h.checkAndNotify(context.Background(), "unknown")
	require.NoError(t, err)
	status = "ok"
	result, err = h.checkAndNotify(context.Background(), "still-low")
	require.NoError(t, err)
	assert.Zero(t, result.Sent)
	now = now.Add(24 * time.Hour)
	result, err = h.checkAndNotify(context.Background(), "daily")
	require.NoError(t, err)
	assert.Equal(t, 2, result.Sent)
	amount = "100"
	result, err = h.checkAndNotify(context.Background(), "recovered")
	require.NoError(t, err)
	assert.Zero(t, result.Sent)
	amount = "99"
	result, err = h.checkAndNotify(context.Background(), "dropped-again")
	require.NoError(t, err)
	assert.Equal(t, 2, result.Sent)
	var unchanged model.Channel
	require.NoError(t, model.DB.First(&unchanged, channel.Id).Error)
	assert.Equal(t, channel.Balance, unchanged.Balance)
	assert.Equal(t, channel.Status, unchanged.Status)
	assert.Equal(t, channel.BalanceUpdatedTime, unchanged.BalanceUpdatedTime)
}

func TestUpstreamBalanceAlertRetriesOnlyFailedRecipients(t *testing.T) {
	setupUpstreamBalanceAlertTest(t)
	now := time.Unix(1700000000, 0)
	var receivers []string
	fail := true
	h := &upstreamBalanceAlertHandler{
		now: func() time.Time { return now },
		query: func(context.Context, *model.Channel, int, string) UpstreamBalanceResult {
			return UpstreamBalanceResult{Status: "ok", Amounts: []UpstreamBalanceAmount{{Amount: "1", Unit: "USD"}}, CheckedAt: now.Unix()}
		},
		send: func(_ context.Context, _, recipient, _ string) error {
			receivers = append(receivers, recipient)
			if recipient == "one@example.com" && fail {
				return errors.New("SMTP refused private-detail")
			}
			return nil
		},
	}
	result, err := h.checkAndNotify(context.Background(), "first")
	require.NoError(t, err)
	assert.Equal(t, 1, result.Sent)
	assert.Equal(t, 1, result.Failed)
	fail = false
	now = now.Add(10 * time.Minute)
	result, err = h.checkAndNotify(context.Background(), "retry")
	require.NoError(t, err)
	assert.Equal(t, 1, result.Sent)
	assert.Equal(t, []string{"one@example.com", "two@example.com", "one@example.com"}, receivers)
}

func TestUpstreamBalanceAlertRechecksRecipientsAndSkipsInvalidCurrency(t *testing.T) {
	setupUpstreamBalanceAlertTest(t)
	sent := 0
	h := &upstreamBalanceAlertHandler{
		now: time.Now,
		query: func(context.Context, *model.Channel, int, string) UpstreamBalanceResult {
			require.NoError(t, model.DB.Model(&model.Option{}).Where("key = ?", "error_report_setting.recipients").Update("value", "").Error)
			return UpstreamBalanceResult{Status: "ok", Amounts: []UpstreamBalanceAmount{{Amount: "1", Unit: "USD"}}}
		},
		send: func(context.Context, string, string, string) error { sent++; return nil },
	}
	_, err := h.checkAndNotify(context.Background(), "removed-during-query")
	require.NoError(t, err)
	assert.Zero(t, sent)
	require.NoError(t, model.DB.Model(&model.Option{}).Where("key = ?", "error_report_setting.recipients").Update("value", "one@example.com").Error)
	require.Error(t, operation_setting.SetUSDExchangeRate("0"))
	h.query = func(context.Context, *model.Channel, int, string) UpstreamBalanceResult {
		return UpstreamBalanceResult{Status: "ok", Amounts: []UpstreamBalanceAmount{{Amount: "1", Unit: "CNY"}}}
	}
	result, err := h.checkAndNotify(context.Background(), "invalid-rate")
	require.NoError(t, err)
	assert.Equal(t, 1, result.Skipped)
	assert.Zero(t, sent)
}

func TestUpstreamBalanceAlertLeaseFencesStaleSenders(t *testing.T) {
	setupUpstreamBalanceAlertTest(t)
	ctx := context.Background()
	claimed, err := model.ClaimUpstreamBalanceAlert(ctx, "connection", "delivery", "first", 1000, 86400)
	require.NoError(t, err)
	require.True(t, claimed)
	claimed, err = model.ClaimUpstreamBalanceAlert(ctx, "connection", "delivery", "second", 1001, 86400)
	require.NoError(t, err)
	assert.False(t, claimed)
	claimed, err = model.ClaimUpstreamBalanceAlert(ctx, "connection", "delivery", "second", 1121, 86400)
	require.NoError(t, err)
	require.True(t, claimed)
	assert.ErrorIs(t, model.FinishUpstreamBalanceAlert(ctx, "delivery", "first", 1122), model.ErrSystemTaskLockLost)
	require.NoError(t, model.FinishUpstreamBalanceAlert(ctx, "delivery", "second", 1122))
	claimed, err = model.ClaimUpstreamBalanceAlert(ctx, "connection", "delivery", "third", 1123, 86400)
	require.NoError(t, err)
	assert.False(t, claimed)
}
