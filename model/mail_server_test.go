package model

import (
	"context"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMailSettingsAndDailyMigration(t *testing.T) {
	var db *gorm.DB
	dialect := os.Getenv("TEST_BILLING_STATEMENT_DIALECT")
	kind := common.DatabaseTypeSQLite
	if dialect != "" {
		db = setupBillingStatementServerDB(t, dialect)
		if dialect == "postgres" {
			kind = common.DatabaseTypePostgreSQL
		} else {
			kind = common.DatabaseTypeMySQL
		}
	} else {
		var err error
		db, err = gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		require.NoError(t, err)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		sqlDB.SetMaxOpenConns(1)
		t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	}
	previousDB, previousLogDB, previousOptions := DB, LOG_DB, common.OptionMap
	previousType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	DB, LOG_DB, common.OptionMap = db, db, map[string]string{}
	common.SetDatabaseTypes(kind, kind)
	t.Cleanup(func() {
		DB, LOG_DB, common.OptionMap = previousDB, previousLogDB, previousOptions
		common.SetDatabaseTypes(previousType, previousLogType)
	})
	// Exercise adding the coverage watermark to an existing schedule without
	// moving its cursor or treating prior cleanup as newly verified.
	require.NoError(t, db.Migrator().CreateTable(&legacyErrorReportSchedule{}))
	require.NoError(t, db.AutoMigrate(&Option{}, &ErrorReport{}, &ErrorReportDelivery{}, &ResourceAlertEpisode{}, &ResourceAlertDelivery{}, &ErrorEvent{}, &SystemTask{}, &WalletOperation{}, &WalletOperationMailPrepared{}))
	require.NoError(t, db.Exec("CREATE UNIQUE INDEX idx_error_reports_window_start ON error_reports (window_start)").Error)
	const start int64 = 1762617600
	require.NoError(t, db.Table("error_report_schedules").Create(map[string]any{
		"scope": ErrorReportScheduleScopeGlobal, "daily_window_start": start,
	}).Error)
	require.NoError(t, db.AutoMigrate(&ErrorReportSchedule{}))
	hour := ErrorReport{ReportID: "legacy-hour", WindowStart: start, WindowEnd: start + 3600, Status: ErrorReportStatusReady}
	require.NoError(t, db.Create(&hour).Error)
	require.NoError(t, db.Create(&Option{Key: retiredErrorReportEnabledOption, Value: "false"}).Error)
	require.NoError(t, db.Create(&ErrorReportDelivery{ReportID: hour.ReportID, PartID: 1, Recipient: "ops@example.com", Status: MailDeliveryPending}).Error)

	require.NoError(t, db.Create(&ErrorReportDelivery{ReportID: hour.ReportID, PartID: 1, Recipient: "history@example.com", Status: "smtp_accepted"}).Error)

	ctx := context.Background()
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return RecordWalletOperation(tx, WalletOperation{UserID: 11, ActorID: 12, Kind: "add", Before: 25, After: 125, CreatedAt: start})
	}))
	wallet, err := ReadWalletOperationsSummary(ctx, start, start+1)
	require.NoError(t, err)
	require.Len(t, wallet, 1)
	assert.EqualValues(t, 1, wallet[0].Count)
	assert.Equal(t, "100", wallet[0].Credited.String())
	assert.Equal(t, "0", wallet[0].Debited.String())
	require.NoError(t, db.Create(&WalletOperation{UserID: 12, ActorID: 12, Kind: "add", Before: 1, After: 2, CreatedAt: start - 1}).Error)
	pending, err := PendingWalletOperationMails(ctx)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	assert.Equal(t, 11, pending[0].UserID)
	require.NoError(t, MarkWalletOperationMailPrepared(ctx, pending[0].ID))
	require.NoError(t, MarkWalletOperationMailPrepared(ctx, pending[0].ID))
	pending, err = PendingWalletOperationMails(ctx)
	require.NoError(t, err)
	assert.Empty(t, pending)
	cutoff, err := ReadErrorEventRetentionCutoff(ctx)
	require.NoError(t, err)
	require.Zero(t, cutoff)
	require.NoError(t, RecordErrorEventRetentionCutoff(ctx, start-10))
	require.NoError(t, RecordErrorEventRetentionCutoff(ctx, start-20))
	cutoff, err = ReadErrorEventRetentionCutoff(ctx)
	require.NoError(t, err)
	require.Equal(t, start-10, cutoff)
	require.NoError(t, migrateDailyReportWindowIndex())
	require.NoError(t, migrateDailyReportWindowIndex())
	require.NoError(t, migrateDailyErrorReportSchedule())
	var retiredOptions int64
	require.NoError(t, db.Model(&Option{}).Where(&Option{Key: retiredErrorReportEnabledOption}).Count(&retiredOptions).Error)
	assert.Zero(t, retiredOptions)
	cursor, err := DailyReportCursor(ctx, start)
	require.NoError(t, err)
	require.Equal(t, start, cursor)
	daily, err := LoadDailyReport(ctx, start, "daily")
	require.NoError(t, err)
	assert.Equal(t, start+DailyReportWindowSeconds, daily.WindowEnd)
	var oldDelivery ErrorReportDelivery
	require.NoError(t, db.Where("report_id = ? AND recipient = ?", hour.ReportID, "ops@example.com").First(&oldDelivery).Error)
	assert.Equal(t, MailDeliveryStopped, oldDelivery.Status)

	assert.Equal(t, "daily_report_cutover", oldDelivery.StopReason)
	var historic ErrorReportDelivery
	require.NoError(t, db.Where("report_id = ? AND recipient = ?", hour.ReportID, "history@example.com").First(&historic).Error)
	assert.Equal(t, "smtp_accepted", historic.Status)
	require.NoError(t, migrateDailyErrorReportSchedule())
	cursor, err = DailyReportCursor(ctx, start+DailyReportWindowSeconds)
	require.NoError(t, err)
	assert.Equal(t, start, cursor)

	require.NoError(t, db.Create(&ErrorEvent{CreatedAt: start, EventType: "fixture", ChannelId: 2}).Error)
	slice, err := ReadDailyReportSlice(ctx, start, start+300)
	require.NoError(t, err)
	assert.EqualValues(t, 1, slice.Total)
	assert.EqualValues(t, 1, slice.Signals[DailyErrorOther])
	require.Len(t, slice.Groups, 1)
	assert.Equal(t, 2, slice.Groups[0].ChannelID)

	key := "resource-fixture"
	recipients := []string{"ops@example.com"}
	opened, err := OpenResourceAlert(ctx, ResourceAlertEpisode{OpenKey: &key, Resource: "cpu", OpenedAt: start}, recipients)
	require.NoError(t, err)
	assert.True(t, opened)
	opened, err = OpenResourceAlert(ctx, ResourceAlertEpisode{OpenKey: &key, Resource: "cpu", OpenedAt: start + 1}, recipients)
	require.NoError(t, err)
	assert.False(t, opened)
	require.NoError(t, CloseResourceAlert(ctx, "resource-fixture", start+2))
	opened, err = OpenResourceAlert(ctx, ResourceAlertEpisode{OpenKey: &key, Resource: "cpu", OpenedAt: start + 3}, recipients)
	require.NoError(t, err)
	assert.True(t, opened)
	require.NoError(t, db.Create(&Option{Key: "error_report_setting.recipients", Value: recipients[0]}).Error)
	require.NoError(t, RetryIncidentMail(ctx, false))
	var pendingCount int64
	require.NoError(t, db.Model(&ResourceAlertDelivery{}).Where("status = ? AND recipient = ?", MailDeliveryPending, "ops@example.com").Count(&pendingCount).Error)
	assert.EqualValues(t, 2, pendingCount)

	_, resources, err := ReadDailyOperationsCounts(ctx, start, start+300)
	require.NoError(t, err)
	assert.Equal(t, []DailyReportCount{{Key: "cpu", Count: 2}}, resources)

	smtp := common.MailTransportConfig{Mode: common.MailTransportSMTP, SMTP: &common.SMTPMailConfig{
		Server: "smtp.example.com", Port: 587, Account: "sender@example.com", Token: "synthetic-smtp-token", StartTLS: true,
	}}
	require.NoError(t, UpdateMailTransport(ctx, smtp))
	smtp.SMTP.Token, smtp.SMTP.Port = "", 2525
	require.NoError(t, UpdateMailTransport(ctx, smtp))
	require.Equal(t, "synthetic-smtp-token", common.CurrentMailTransport().SMTP.Token)
	graph := common.MailTransportConfig{Mode: common.MailTransportGraph, Graph: &common.GraphMailConfig{
		TenantID: "00000000-0000-4000-8000-000000000001", ClientID: "00000000-0000-4000-8000-000000000002",
		Sender: "sender@example.com", ClientSecret: "synthetic-graph-token",
	}}
	require.NoError(t, UpdateMailTransport(ctx, graph))
	effective := common.CurrentMailTransport()
	assert.Nil(t, effective.SMTP)
	require.NotNil(t, effective.Graph)
	assert.Equal(t, graph.Graph.ClientSecret, effective.Graph.ClientSecret)
	graph.Graph.ClientSecret, graph.Graph.ClientID = "", "00000000-0000-4000-8000-000000000003"
	assert.Error(t, UpdateMailTransport(ctx, graph))
	common.OptionMap = map[string]string{} // Simulate a node that has not synchronized options.
	effective, err = LoadMailTransport(ctx)
	require.NoError(t, err)
	require.NotNil(t, effective.Graph)
	assert.Equal(t, "00000000-0000-4000-8000-000000000002", effective.Graph.ClientID)
}

type legacyErrorReportSchedule struct {
	ID               int64  `gorm:"primaryKey"`
	Scope            string `gorm:"type:varchar(32);uniqueIndex"`
	DailyWindowStart int64  `gorm:"bigint"`
	UpdatedAt        int64  `gorm:"bigint"`
	DeliveryCursor   string `gorm:"type:varchar(255)"`
}

func (legacyErrorReportSchedule) TableName() string { return "error_report_schedules" }
