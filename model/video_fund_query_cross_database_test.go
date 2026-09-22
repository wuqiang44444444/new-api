package model

import (
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// External DSNs must point to an empty, disposable database: raw projection SQL
// intentionally uses the production table names. Existing tables are rejected.
func TestVideoFundMissingResultsAcrossDatabases(t *testing.T) {
	for _, backend := range []common.DatabaseType{common.DatabaseTypeSQLite, common.DatabaseTypeMySQL, common.DatabaseTypePostgreSQL} {
		t.Run(string(backend), func(t *testing.T) {
			var dialector gorm.Dialector
			switch backend {
			case common.DatabaseTypeMySQL:
				dsn := os.Getenv("TEST_VIDEO_FUND_MYSQL_DSN")
				if dsn == "" {
					t.Skip("TEST_VIDEO_FUND_MYSQL_DSN is not configured")
				}
				dialector = mysql.Open(dsn)
			case common.DatabaseTypePostgreSQL:
				dsn := os.Getenv("TEST_VIDEO_FUND_POSTGRES_DSN")
				if dsn == "" {
					t.Skip("TEST_VIDEO_FUND_POSTGRES_DSN is not configured")
				}
				dialector = postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
			default:
				dialector = sqlite.Open(":memory:")
			}
			db, err := gorm.Open(dialector, &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { assert.NoError(t, sqlDB.Close()) })
			sqlDB.SetMaxOpenConns(1)
			for _, table := range []any{&Task{}, &TaskCreateAttempt{}, &TaskBillingDelivery{}} {
				require.False(t, db.Migrator().HasTable(table), "test requires an empty database")
			}
			oldDB, oldType := DB, common.MainDatabaseType()
			DB = db
			common.SetMainDatabaseType(backend)
			t.Cleanup(func() {
				DB = oldDB
				common.SetMainDatabaseType(oldType)
				assert.NoError(t, db.Migrator().DropTable(&TaskBillingDelivery{}, &TaskCreateAttempt{}, &Task{}))
			})
			require.NoError(t, db.AutoMigrate(&Task{}, &TaskCreateAttempt{}, &TaskBillingDelivery{}))
			assertVideoFundMissingResults(t)
		})
	}
}

func assertVideoFundMissingResults(t *testing.T) {
	t.Helper()
	cases := []struct {
		id             string
		result, legacy string
		quota          int
		app            int
		abnormal       bool
	}{
		{"missing-a", "", "", 20, 11, true},
		{"missing-b", "", "", 30, 11, true},
		{"available", "https://example.test/result", "", 40, 11, false},
		{"legacy", "", "https://example.test/legacy", 50, 11, false},
		{"refunded", "", "", 0, 11, false},
		{"other-app", "", "", 60, 12, true},
	}
	var firstID int64
	for i, tc := range cases {
		task := Task{TaskID: tc.id, UserId: 1800, AppID: tc.app, ChannelId: 21, Platform: "video", ClientProtocol: TaskClientProtocolModelArkV3,
			Status: TaskStatusSuccess, BillingState: TaskBillingStateSettled, Quota: tc.quota, CreatedAt: int64(100 + i), FailReason: tc.legacy, PrivateData: TaskPrivateData{ResultURL: tc.result}}
		require.NoError(t, DB.Create(&task).Error)
		if i == 0 {
			firstID = task.ID
		}
		entry, err := GetVideoFundLog("task", task.ID)
		require.NoError(t, err)
		if tc.result != "" || tc.legacy != "" {
			assert.NotEqual(t, "result_unavailable", entry.Delivery)
		}
	}
	filters := VideoFundFilters{UserID: 1800, AppID: 11, ChannelID: 21, State: "abnormal", Limit: 1}
	rows, total, err := ListVideoFundLogs(filters)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.EqualValues(t, 2, total)
	assert.Equal(t, "missing-b", rows[0].TaskID)
	filters.Offset = 1
	rows, total, err = ListVideoFundLogs(filters)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.EqualValues(t, 2, total)
	assert.Equal(t, "missing-a", rows[0].TaskID)
	summary, err := SummarizeVideoFunds(filters)
	require.NoError(t, err)
	assert.EqualValues(t, 2, summary.AbnormalCount)
	assert.EqualValues(t, 50, summary.AbnormalQuota)
	// A late result must remove the anomaly immediately, without backfill or a new state.
	require.NoError(t, DB.Model(&Task{}).Where("id = ?", firstID).Update("private_data", TaskPrivateData{ResultURL: "https://example.test/recovered"}).Error)
	filters.State, filters.Offset = "", 0
	rows, _, err = ListVideoFundLogs(filters)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "missing-b", rows[0].TaskID, "actual anomaly precedes newer normal rows")
	summary, err = SummarizeVideoFunds(filters)
	require.NoError(t, err)
	assert.EqualValues(t, 1, summary.AbnormalCount)
	assert.EqualValues(t, 30, summary.AbnormalQuota)
}
