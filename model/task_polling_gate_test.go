package model

import (
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestTaskPollingGateAcrossDatabases(t *testing.T) {
	for _, backend := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			var dialector gorm.Dialector
			databaseType := common.DatabaseTypeSQLite
			switch backend {
			case "mysql":
				dsn := os.Getenv("TEST_TASK_POLL_MYSQL_DSN")
				if dsn == "" {
					t.Skip("TEST_TASK_POLL_MYSQL_DSN is not configured")
				}
				dialector = mysql.Open(dsn)
				databaseType = common.DatabaseTypeMySQL
			case "postgres":
				dsn := os.Getenv("TEST_TASK_POLL_POSTGRES_DSN")
				if dsn == "" {
					t.Skip("TEST_TASK_POLL_POSTGRES_DSN is not configured")
				}
				dialector = postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
				databaseType = common.DatabaseTypePostgreSQL
			default:
				dialector = sqlite.Open(":memory:")
			}
			db, err := gorm.Open(dialector, &gorm.Config{NamingStrategy: schema.NamingStrategy{
				TablePrefix: "pollgate_" + common.GetUUID()[:12] + "_",
			}})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			previousDB, previousType := DB, common.MainDatabaseType()
			DB = db
			common.SetMainDatabaseType(databaseType)
			t.Cleanup(func() {
				DB = previousDB
				common.SetMainDatabaseType(previousType)
				assert.NoError(t, db.Migrator().DropTable(&TaskBillingDelivery{}, &TaskCreateAttempt{}, &Task{}))
				assert.NoError(t, sqlDB.Close())
			})
			require.NoError(t, db.AutoMigrate(&Task{}, &TaskCreateAttempt{}, &TaskBillingDelivery{}))
			assert.False(t, HasUnfinishedSyncTasks())
			assert.False(t, HasTaskPollingWork())

			// Each fixture is the only task, so a different eligible row cannot mask a miss.
			cases := []struct {
				name   string
				fields map[string]any
				want   bool
			}{
				{"native suno", nil, true},
				{"native video", map[string]any{"platform": "4"}, true},
				{"not started", map[string]any{"status": TaskStatusNotStart}, true},
				{"submitted", map[string]any{"status": TaskStatusSubmitted}, true},
				{"in progress", map[string]any{"status": TaskStatusInProgress}, true},
				{"unknown", map[string]any{"status": TaskStatusUnknown}, true},
				{"reconciliation", map[string]any{"status": TaskStatusReconciliationRequired}, true},
				{"future status", map[string]any{"status": "FUTURE_STATUS"}, true},
				{"empty status", map[string]any{"status": ""}, true},
				{"null status", map[string]any{"status": nil}, false},
				{"success", map[string]any{"status": TaskStatusSuccess}, false},
				{"failure", map[string]any{"status": TaskStatusFailure}, false},
				{"cancelled", map[string]any{"status": TaskStatusCancelled}, false},
				{"expired", map[string]any{"status": TaskStatusExpired}, false},
				{"provider contract failure", map[string]any{"status": TaskStatusProviderContractFailure}, false},
				{"complete progress", map[string]any{"progress": "100%"}, false},
				{"progress above pivot", map[string]any{"progress": "99%"}, true},
				{"decimal progress", map[string]any{"progress": "100.0%"}, true},
				{"nonstandard progress", map[string]any{"progress": "pending"}, true},
				{"empty progress", map[string]any{"progress": ""}, true},
				{"null progress", map[string]any{"progress": nil}, false},
				{"legacy null protocol", map[string]any{"client_protocol": nil}, true},
				{"link video", map[string]any{"client_protocol": TaskClientProtocolModelArkV3}, true},
				{"image excluded", map[string]any{"client_protocol": TaskClientProtocolImageOpenAIV1}, false},
				{"batch excluded", map[string]any{"platform": constant.TaskPlatformAzureBatch}, false},
				{"null platform", map[string]any{"platform": nil}, false},
				{"empty platform", map[string]any{"platform": ""}, true},
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					row := map[string]any{
						"status": TaskStatusQueued, "progress": "0%", "client_protocol": "",
						"platform": "suno", "submit_time": int64(1),
					}
					for key, value := range tc.fields {
						row[key] = value
					}
					require.NoError(t, db.Model(&Task{}).Create(row).Error)
					t.Cleanup(func() { assert.NoError(t, db.Where("1 = 1").Delete(&Task{}).Error) })
					assert.Equal(t, tc.want, HasUnfinishedSyncTasks())
					assert.Equal(t, tc.want, len(GetAllUnFinishSyncTasks(1)) > 0, "gate must match polling selection")
					assert.Equal(t, tc.want, len(GetTimedOutUnfinishedTasks(2, 1)) > 0, "gate must match timeout selection")
				})
			}

			// Trailing-space equality differs by database collation; preserve the old
			// predicate's result instead of imposing a new normalization rule.
			t.Run("collation preserves original predicate", func(t *testing.T) {
				for _, progress := range []string{"100% ", "100%\t", " 100%", "１００％"} {
					require.NoError(t, db.Model(&Task{}).Create(map[string]any{
						"progress": progress, "status": TaskStatusQueued, "platform": "suno", "client_protocol": "",
					}).Error)
					var ids []int64
					require.NoError(t, db.Model(&Task{}).Where("progress != ?", "100%").
						Where("status NOT IN ?", TerminalTaskStatuses()).
						Where("client_protocol IS NULL OR client_protocol <> ?", TaskClientProtocolImageOpenAIV1).
						Where("platform <> ?", constant.TaskPlatformAzureBatch).Limit(1).Pluck("id", &ids).Error)
					assert.Equal(t, len(ids) > 0, HasUnfinishedSyncTasks(), "progress %q", progress)
					require.NoError(t, db.Where("1 = 1").Delete(&Task{}).Error)
				}
			})

			t.Run("usage probe migration preserves due work", func(t *testing.T) {
				now := GetDBTimestamp()
				cases := []struct {
					name         string
					review, next any
					state        TaskBillingState
					status       TaskStatus
					want         bool
				}{
					{"null scheduling", nil, nil, TaskBillingStateAwaitingUsage, TaskStatusSuccess, true},
					{"zero scheduling", int64(0), int64(0), TaskBillingStateAwaitingUsage, TaskStatusSuccess, true},
					{"due boundary", int64(0), now, TaskBillingStateAwaitingUsage, TaskStatusSuccess, true},
					{"future", int64(0), now + 3600, TaskBillingStateAwaitingUsage, TaskStatusSuccess, false},
					{"reviewed", now, int64(0), TaskBillingStateAwaitingUsage, TaskStatusSuccess, false},
					{"not successful", int64(0), int64(0), TaskBillingStateAwaitingUsage, TaskStatusFailure, false},
					{"settled", int64(0), int64(0), TaskBillingStateSettled, TaskStatusSuccess, false},
				}
				for _, tc := range cases {
					t.Run(tc.name, func(t *testing.T) {
						stmt := &gorm.Statement{DB: db}
						require.NoError(t, stmt.Parse(&Task{}))
						migrator := db.Table(stmt.Schema.Table).Migrator()
						if migrator.HasIndex(&taskUsageCheckIndex{}, "idx_task_usage_poll") {
							require.NoError(t, migrator.DropIndex(&taskUsageCheckIndex{}, "idx_task_usage_poll"))
						}
						task := &Task{Status: tc.status, Progress: "100%", Platform: "suno", BillingState: tc.state}
						require.NoError(t, db.Create(task).Error)
						require.NoError(t, db.Model(task).Updates(map[string]any{"usage_review_at": tc.review, "usage_check_next_at": tc.next}).Error)
						t.Cleanup(func() { assert.NoError(t, db.Delete(task).Error) })
						for _, migrated := range []bool{false, true} {
							if migrated {
								require.NoError(t, migrateTaskUsageCheckIndex(db))
								require.NoError(t, migrateTaskUsageCheckIndex(db))
							}
							assert.Equal(t, tc.want, HasDueTaskUsageChecks(now))
							ids, err := DueTaskUsageCheckIDs(now, 10)
							require.NoError(t, err)
							if tc.want {
								assert.Equal(t, []int64{task.ID}, ids)
								assert.True(t, HasTaskPollingWork())
							} else {
								assert.Empty(t, ids)
							}
						}
					})
				}
			})

			t.Run("terminal refund still wakes polling", func(t *testing.T) {
				task := &Task{Status: TaskStatusFailure, Progress: "100%", Platform: "suno", Quota: 100,
					SubmitTime: TaskRefundLegacyCutoff, UpdatedAt: 1}
				require.NoError(t, db.Create(task).Error)
				t.Cleanup(func() { assert.NoError(t, db.Delete(task).Error) })
				assert.False(t, HasUnfinishedSyncTasks())
				assert.True(t, HasTaskPollingWork())
				require.NoError(t, db.Model(task).Update("quota", 0).Error)
				assert.False(t, HasTaskPollingWork())
			})

			t.Run("terminal settlement still wakes polling", func(t *testing.T) {
				task := &Task{Status: TaskStatusSuccess, Progress: "100%", Platform: "suno", BillingState: TaskBillingStatePending,
					PrivateData: TaskPrivateData{AsyncBilling: &TaskAsyncBillingContext{State: TaskBillingStatePending}}}
				require.NoError(t, db.Create(task).Error)
				t.Cleanup(func() { assert.NoError(t, db.Delete(task).Error) })
				assert.False(t, HasUnfinishedSyncTasks())
				assert.False(t, HasDueTaskUsageChecks(GetDBTimestamp()))
				assert.True(t, HasTaskPollingWork())
				task.PrivateData.AsyncBilling.NextRetryAt = GetDBTimestamp() + 3600
				require.NoError(t, db.Model(task).Update("private_data", task.PrivateData).Error)
				assert.False(t, HasTaskPollingWork())
			})
			t.Run("log delivery without active task still wakes polling", func(t *testing.T) {
				event := &TaskBillingDelivery{TaskRowID: 1, Event: "create", NextRetryAt: GetDBTimestamp() - 1}
				require.NoError(t, db.Create(event).Error)
				t.Cleanup(func() { assert.NoError(t, db.Delete(event).Error) })
				assert.False(t, HasUnfinishedSyncTasks())
				assert.True(t, HasTaskPollingWork())
				require.NoError(t, db.Model(event).Update("next_retry_at", GetDBTimestamp()+3600).Error)
				assert.False(t, HasTaskPollingWork())
			})

			t.Run("due attempt without task still wakes polling", func(t *testing.T) {
				attempt := &TaskCreateAttempt{PublicTaskID: "pollgate-attempt", Status: TaskCreateAttemptUnknown,
					NextAttemptAt: GetDBTimestamp() - 1}
				require.NoError(t, db.Create(attempt).Error)
				t.Cleanup(func() { assert.NoError(t, db.Delete(attempt).Error) })
				assert.False(t, HasUnfinishedSyncTasks())
				assert.True(t, HasTaskPollingWork())
				require.NoError(t, db.Model(attempt).Update("next_attempt_at", GetDBTimestamp()+3600).Error)
				assert.False(t, HasTaskPollingWork())
			})
		})
	}
}
