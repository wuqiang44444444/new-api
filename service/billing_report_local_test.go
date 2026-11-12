package service

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFastBillingReportsRequireLocalDevelopmentDatabases(t *testing.T) {
	const local = "postgres://dev@127.0.0.1:54330/dev?sslmode=disable"
	const remote = "postgres://dev@db.example.com/dev?sslmode=disable"
	for _, tc := range []struct {
		name, setting, main, logs string
		want                      bool
	}{
		{name: "default uses the resource gate", main: local},
		{name: "local development bypasses the resource gate", setting: "true", main: local, want: true},
		{name: "local IPv6 log endpoint", setting: "true", main: local, logs: "postgres://dev@[::1]/dev?sslmode=disable", want: true},
		{name: "remote primary", setting: "true", main: remote},
		{name: "remote log endpoint", setting: "true", main: local, logs: remote},
		{name: "remote failover host", setting: "true", main: "postgres://dev@localhost,db.example.com/dev?sslmode=disable"},
		{name: "remote address overrides local hostname", setting: "true", main: local + "&host=db.example.com"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("LOCAL_DEV_FAST_BILLING_REPORTS", tc.setting)
			t.Setenv("SQL_DSN", tc.main)
			t.Setenv("LOG_SQL_DSN", tc.logs)
			assert.Equal(t, tc.want, localFastBillingReports())
		})
	}
}

func TestFastBillingReportsPublishMultipleHistoricalDatesUnderOneLease(t *testing.T) {
	db := setupVersionServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.TaskBillingDelivery{}, &model.SystemTask{}, &model.SystemTaskLock{}))
	prepareDailyReportExportTest(t)
	ctx := context.Background()
	now := time.Now().In(model.UsageAnalyticsZone())
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	first := today.AddDate(0, 0, -121).Unix()
	second := today.AddDate(0, 0, -120).Unix()
	require.NoError(t, db.Create(&[]model.Log{
		{UserId: 7, Type: model.LogTypeConsume, CreatedAt: first + 1, Quota: 30, ModelName: "first"},
		{UserId: 7, Type: model.LogTypeConsume, CreatedAt: second + 1, Quota: 40, ModelName: "second"},
	}).Error)

	require.NoError(t, runDailyReport(ctx, &model.SystemTask{TaskID: "export-fixture"}, "runner", func(ctx context.Context) error {
		return ctx.Err()
	}, true))

	_, report, err := model.ResolveBillingReport(ctx, first, second+86399, "")
	require.NoError(t, err)
	assert.Equal(t, 2, report.PublishedDays)
}
