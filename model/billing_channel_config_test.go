package model

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestManualChannelBillingDatabaseMatrix(t *testing.T) {
	for _, dialect := range []common.DatabaseType{common.DatabaseTypeSQLite, common.DatabaseTypeMySQL, common.DatabaseTypePostgreSQL} {
		t.Run(string(dialect), func(t *testing.T) {
			var driver gorm.Dialector = sqlite.Open(":memory:")
			if dialect != common.DatabaseTypeSQLite {
				env := "TEST_BILLING_MYSQL_DSN"
				if dialect == common.DatabaseTypePostgreSQL {
					env = "TEST_BILLING_POSTGRES_DSN"
				}
				dsn := os.Getenv(env)
				if dsn == "" {
					t.Skip(env + " is not configured")
				}
				if dialect == common.DatabaseTypeMySQL {
					driver = mysql.Open(dsn)
				} else {
					driver = postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
				}
			}
			db, err := gorm.Open(driver, &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			if dialect != common.DatabaseTypeSQLite {
				sqlDB.SetMaxOpenConns(4)
			}
			beforeDB, beforeLog, beforeMainType, beforeLogType := DB, LOG_DB, common.MainDatabaseType(), common.LogDatabaseType()
			DB, LOG_DB = db, db
			common.SetDatabaseTypes(dialect, dialect)
			t.Cleanup(func() {
				DB, LOG_DB = beforeDB, beforeLog
				common.SetDatabaseTypes(beforeMainType, beforeLogType)
				require.NoError(t, sqlDB.Close())
			})
			require.NoError(t, db.AutoMigrate(&User{}, &Channel{}, &Log{}, &Task{}, &ErrorEvent{}))
			require.NoError(t, MigrateDailyReports(db))
			require.NoError(t, migrateBillingReconciliationDB())
			require.NoError(t, migrateBillingReconciliationDB(), "a second startup must preserve the reporting configuration")
			t.Cleanup(func() {
				for _, table := range []any{&Channel{}, &Log{}, &ProviderChannelBillingConfig{}, &ProviderChannelBillingDiscount{}} {
					column := "channel_id"
					if _, ok := table.(*Channel); ok {
						column = "id"
					}
					require.NoError(t, db.Where(column+" IN ?", []int{900001, 900002}).Delete(table).Error)
				}
				require.NoError(t, db.Where("url_key = ?", "https://same.example").Delete(&ProviderURLGroupDisplayName{}).Error)
				require.NoError(t, db.Where("request_id = ?", "manual-channel-matrix").Delete(&ErrorEvent{}).Error)
			})
			ctx := context.Background()
			start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.FixedZone("Shanghai", 8*3600)).Unix()
			url := "https://same.example"
			require.NoError(t, db.Create(&[]Channel{{Id: 900001, Name: "A", BaseURL: &url}, {Id: 900002, Name: "B", BaseURL: &url}}).Error)
			require.NoError(t, SaveProviderURLGroupName(url, "Old URL label", 1))
			view, err := GetProviderChannelBillingConfig(ctx, 900001, start)
			require.NoError(t, err)
			assert.Empty(t, view.UpstreamName, "old URL labels must not be copied to a channel")
			for _, id := range []int{900001, 900002} {
				require.NoError(t, SaveProviderChannelBillingName(ctx, id, "Same supplier", 1))
				require.NoError(t, db.Create(&Log{ChannelId: id, CreatedAt: start + 1, Type: LogTypeConsume, ModelName: "image", TokenName: "模型测试", Content: "模型测试", Quota: 100, Other: `{"test_pricing":{"version":1,"mode":"ratio","status":"settled","original_quota":100}}`}).Error)
			}
			filter := UpstreamSummaryPageFilter{Level: "channels", Page: 1, PageSize: 10}
			page, err := GetUpstreamSummaryPage(ctx, start, start+100, start, filter)
			require.NoError(t, err)
			require.Len(t, page.Channels, 2, "same URL and same name never merge channels")
			assert.Equal(t, 2, page.Total)
			require.NoError(t, db.Model(&Channel{}).Where("id = ?", 900001).Update("base_url", "https://changed.example").Error)
			changed, err := GetUpstreamSummaryPage(ctx, start, start+100, start, filter)
			require.NoError(t, err)
			assert.Equal(t, page.Channels, changed.Channels, "URL changes must not alter historical channel totals")
			require.NoError(t, db.Delete(&Channel{}, 900002).Error)
			view, err = GetProviderChannelBillingConfig(ctx, 900002, start)
			require.NoError(t, err)
			assert.True(t, view.Deleted)
			discount := ProviderChannelBillingDiscount{PeriodStart: start, ChannelId: 900002, Discount: decimal.RequireFromString("0.8"), Reason: ReasonChannelDiscountManualUpdate}
			require.NoError(t, SaveProviderChannelBillingDiscount(&discount, 0, 1))
			require.ErrorIs(t, SaveProviderChannelBillingDiscount(&discount, 0, 1), ErrBillingReconciliationVersionConflict)
			view, err = GetProviderChannelBillingConfig(ctx, 900002, start)
			require.NoError(t, err)
			require.NotNil(t, view.Discount)
			assert.Equal(t, "0.8", view.Discount.Value.String())
			assert.EqualValues(t, 1, view.ExpectedVersion)
			next := time.Unix(start, 0).In(time.FixedZone("Shanghai", 8*3600)).AddDate(0, 1, 0).Unix()
			inherited, err := GetProviderChannelBillingConfig(ctx, 900002, next)
			require.NoError(t, err)
			require.NotNil(t, inherited.Discount)
			assert.Equal(t, "0.8", inherited.Discount.Value.String())
			assert.Zero(t, inherited.ExpectedVersion)
			outcomes, err := InitializeProviderChannelBillingDiscounts(next, []int{900002}, 1)
			require.NoError(t, err)
			require.Len(t, outcomes, 1)
			assert.Equal(t, "created", outcomes[0].Outcome)
			// Two first writes for a deleted channel must not both win with version 0.
			// External databases use separate connections, exercising the row lock.
			concurrentPeriod := time.Unix(next, 0).In(time.FixedZone("Shanghai", 8*3600)).AddDate(0, 1, 0).Unix()
			results := make(chan error, 2)
			startWrites := make(chan struct{})
			for _, coefficient := range []string{"0.6", "0.7"} {
				go func(value string) {
					<-startWrites
					record := ProviderChannelBillingDiscount{PeriodStart: concurrentPeriod, ChannelId: 900002, Discount: decimal.RequireFromString(value), Reason: ReasonChannelDiscountManualUpdate}
					results <- SaveProviderChannelBillingDiscount(&record, 0, 1)
				}(coefficient)
			}
			close(startWrites)
			wins, conflicts := 0, 0
			for range 2 {
				if err := <-results; err == nil {
					wins++
				} else {
					require.ErrorIs(t, err, ErrBillingReconciliationVersionConflict)
					conflicts++
				}
			}
			assert.Equal(t, 1, wins)
			assert.Equal(t, 1, conflicts)
			require.Error(t, SaveProviderChannelBillingName(ctx, 999999, "invalid", 1))
			changed, err = GetUpstreamSummaryPage(ctx, start, start+100, start, filter)
			require.NoError(t, err)
			require.Len(t, changed.Channels, 2)
			assert.True(t, changed.Channels[1].Deleted)
			require.NotNil(t, changed.Channels[1].ReferenceAmount)
			assert.EqualValues(t, 80, *changed.Channels[1].ReferenceAmount)
			// Diagnostic cleanup uses the log dialect and never deletes billing facts.
			require.NoError(t, db.Create(&[]ErrorEvent{{RequestId: "manual-channel-matrix", CreatedAt: 1}, {RequestId: "manual-channel-matrix", CreatedAt: 100}, {RequestId: "manual-channel-matrix", CreatedAt: 101}}).Error)
			deleted, err := DeleteOldErrorEventBatch(ctx, 100, 1)
			require.NoError(t, err)
			assert.EqualValues(t, 1, deleted)
			var count int64
			require.NoError(t, db.Model(&ErrorEvent{}).Where("request_id = ?", "manual-channel-matrix").Count(&count).Error)
			assert.EqualValues(t, 2, count)
			require.NoError(t, db.Model(&Log{}).Where("channel_id IN ?", []int{900001, 900002}).Count(&count).Error)
			assert.EqualValues(t, 2, count)
		})
	}
}
