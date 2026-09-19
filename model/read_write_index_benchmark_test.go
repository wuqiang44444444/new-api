package model

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// BenchmarkReadWriteIndexes compares complete index sets on disposable DBs.
// Each operation inserts a log and updates a task; every tenth creates a task.
// Mixed also performs a time-ordered log page and scoped task evidence read
// every eighth operation. It measures storage/GORM cost, not provider latency
// or the full funds transaction. No timing assertions or business data are used.
// Run with -cpu=4 -benchtime=2000x -count=1 (three alternating-order rounds); server opt-in uses the same isolated
// database fixture as the migration tests, never the configured database itself.
func BenchmarkReadWriteIndexes(b *testing.B) {
	for round := 0; round < 3; round++ {
		versions := []string{"before", "optimized"}
		if round%2 == 1 {
			slices.Reverse(versions)
		}
		for _, workload := range []string{"writes", "mixed"} {
			for _, version := range versions {
				b.Run(fmt.Sprintf("%s/round%d/%s", workload, round+1, version), func(b *testing.B) {
					var db *gorm.DB
					if dialect := os.Getenv("TEST_BILLING_STATEMENT_DIALECT"); dialect != "" {
						db = setupBillingStatementServerDB(b, dialect)
					} else {
						var err error
						db, err = gorm.Open(sqlite.Open(filepath.Join(b.TempDir(), "indexes.db")+"?_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)&_pragma=busy_timeout(30000)&_txlock=immediate"), &gorm.Config{})
						require.NoError(b, err)
					}
					db = db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
					conn, err := db.DB()
					require.NoError(b, err)
					b.Cleanup(func() { _ = conn.Close() })
					conn.SetMaxOpenConns(8)
					require.NoError(b, db.AutoMigrate(&Log{}, &Task{}))
					require.NoError(b, migrateBillingStatementLogIndex(db))
					require.NoError(b, migrateTaskUsageCheckIndex(db))
					if version == "before" {
						require.NoError(b, db.Migrator().DropIndex(&Log{}, "idx_created_at_id"))
						for _, sql := range []string{
							"CREATE INDEX idx_created_at_id ON logs(id,created_at)",
							"CREATE INDEX idx_logs_user_id ON logs(user_id)",
							"CREATE INDEX idx_logs_model_name ON logs(model_name)",
							"CREATE INDEX idx_tasks_task_id ON tasks(task_id)",
						} {
							require.NoError(b, db.Exec(sql).Error)
						}
					} else {
						require.NoError(b, migrateBillingStatementTaskIndex(db))
					}
					payload, err := common.Marshal(map[string]string{"fixture": strings.Repeat("x", 768)})
					require.NoError(b, err)
					logs := make([]Log, 10000)
					for i := range logs {
						logs[i] = Log{UserId: i%20 + 1, TokenId: i%40 + 1, CreatedAt: int64(i/10 + 1), Type: LogTypeConsume,
							ModelName: fmt.Sprintf("model-%d", i%8), Username: fmt.Sprintf("user-%d", i%20), Other: string(payload)}
					}
					require.NoError(b, db.CreateInBatches(logs, 100).Error)
					tasks := make([]Task, 500)
					for i := range tasks {
						tasks[i] = Task{TaskID: fmt.Sprintf("task-%d", i), UserId: i%20 + 1, AppID: i%20 + 1,
							Status: TaskStatusInProgress, Properties: Properties{OriginModelName: "model-0"},
							PrivateData: TaskPrivateData{TokenId: i%20 + 1}, Data: []byte(`{"fixture":"` + strings.Repeat("x", 4400) + `"}`)}
					}
					require.NoError(b, db.CreateInBatches(tasks, 50).Error)
					var sequence atomic.Int64
					latencies := make([]int64, b.N)
					stats := conn.Stats()
					b.ResetTimer()
					b.RunParallel(func(pb *testing.PB) {
						for pb.Next() {
							n := int(sequence.Add(1)) - 1
							start := time.Now()
							log := logs[n%len(logs)]
							log.Id, log.CreatedAt = 0, int64(2000+n/10)
							if err := db.Create(&log).Error; err != nil {
								b.Error(err)
								return
							}
							task := tasks[n%len(tasks)]
							// Includes identity columns like the native full-field update path.
							if err := db.Model(&Task{}).Where("id = ?", task.ID).Updates(map[string]any{
								"status": TaskStatusInProgress, "progress": fmt.Sprintf("%d%%", n%100),
								"user_id": task.UserId, "app_id": task.AppID, "task_id": task.TaskID,
							}).Error; err != nil {
								b.Error(err)
								return
							}
							if n%10 == 0 {
								task.ID, task.TaskID = 0, fmt.Sprintf("new-%d", n)
								if err := db.Create(&task).Error; err != nil {
									b.Error(err)
									return
								}
							}
							latencies[n] = time.Since(start).Nanoseconds()
							if workload == "mixed" && n%8 == 0 {
								var page []Log
								if err := db.Where("created_at >= ? AND created_at <= ?", 100, 1000).Order("created_at DESC, id DESC").Limit(20).Find(&page).Error; err != nil {
									b.Error(err)
									return
								}
								var evidence []Task
								if err := db.Select("user_id,app_id,task_id,channel_id,properties,private_data").Where("user_id = ? AND app_id = ? AND task_id IN ?", 1, 1, []string{"task-0", "task-20", "task-40"}).Find(&evidence).Error; err != nil {
									b.Error(err)
									return
								}
							}
						}
					})
					b.StopTimer()
					slices.Sort(latencies)
					b.ReportMetric(float64(latencies[(len(latencies)-1)*95/100])/1000, "write-p95-us")
					b.ReportMetric(float64(latencies[(len(latencies)-1)*99/100])/1000, "write-p99-us")
					b.ReportMetric(float64(conn.Stats().WaitCount-stats.WaitCount)/float64(b.N), "pool-waits/op")
				})
			}
		}
	}
}
