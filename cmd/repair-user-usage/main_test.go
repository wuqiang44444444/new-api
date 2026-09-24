package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestUsageRepairCommandProcess(t *testing.T) {
	if os.Getenv("USAGE_REPAIR_TEST_PROCESS") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			os.Args = append([]string{os.Args[0]}, os.Args[i+1:]...)
			break
		}
	}
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	main()
	os.Exit(0)
}

func runRepairCommand(t *testing.T, args ...string) (string, error) {
	t.Helper()
	command := exec.Command(os.Args[0], append([]string{"-test.run=^TestUsageRepairCommandProcess$", "--"}, args...)...)
	command.Env = append(os.Environ(), "USAGE_REPAIR_TEST_PROCESS=1")
	output, err := command.CombinedOutput()
	return string(output), err
}

func TestUsageRepairCommandWorkflow(t *testing.T) {
	t.Run("SQLite", func(t *testing.T) { testUsageRepairCommandWorkflow(t, "") })
	for _, engine := range []string{"MYSQL", "POSTGRES"} {
		t.Run(engine, func(t *testing.T) {
			dsn := os.Getenv("USAGE_REPAIR_TEST_" + engine + "_DSN")
			if dsn == "" {
				t.Skip("disposable database DSN not configured")
			}
			require.Equal(t, "1", os.Getenv("USAGE_REPAIR_TEST_ALLOW_RESET"))
			require.Contains(t, dsn, "usage_repair_tests")
			testUsageRepairCommandWorkflow(t, dsn)
		})
	}
}

func testUsageRepairCommandWorkflow(t *testing.T, externalDSN string) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.db")
	var driver gorm.Dialector = sqlite.Open(source)
	if externalDSN != "" {
		source = externalDSN
		var parseErr error
		driver, _, _, parseErr = parseDatabase(source)
		require.NoError(t, parseErr)
	}
	db, err := gorm.Open(driver, &gorm.Config{Logger: logger.Discard})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	models := []any{&model.User{}, &model.Log{}, &model.Task{}, &model.TaskCreateAttempt{}, &model.TaskBillingDelivery{}, &model.AuditLog{}, &model.QuotaData{}, &model.Channel{}}
	if externalDSN != "" {
		require.NoError(t, db.Migrator().DropTable(models...))
	}
	require.NoError(t, db.AutoMigrate(models...))
	require.NoError(t, db.Create(&model.User{Id: 1, Username: "root", AffCode: "root", Role: common.RoleRootUser, Status: 1}).Error)
	require.NoError(t, db.Create(&model.User{Id: 91, Username: "customer", AffCode: "customer", UsedQuota: 100, Quota: 200, RequestCount: 1}).Error)
	require.NoError(t, db.Create(&model.Log{UserId: 91, Type: model.LogTypeConsume, Quota: 100, RequestId: "consume", Other: "{}"}).Error)
	require.NoError(t, db.Create(&model.Log{UserId: 91, Type: model.LogTypeRefund, Quota: 40, RequestId: "legacy-refund", Other: "{}"}).Error)
	base := []string{"-dsn", source, "-user-id", "91"}
	reviewPath := filepath.Join(dir, "review.json")
	output, err := runRepairCommand(t, append(base, "-write-review", reviewPath)...)
	require.NoError(t, err, output)
	assert.Contains(t, output, "status=not_correctable")
	manifestPath := filepath.Join(dir, "manifest.json")
	generate := append(append([]string{}, base...), "-review", reviewPath, "-write-manifest", manifestPath, "-operator-id", "1")
	output, err = runRepairCommand(t, generate...)
	require.Error(t, err)
	assert.Contains(t, output, "no correction manifest")
	raw, err := os.ReadFile(reviewPath)
	require.NoError(t, err)
	var review model.UsageRepairReview
	require.NoError(t, common.Unmarshal(raw, &review))
	review.ReviewedByUserID = 1
	review.LogHistoryComplete = true
	review.LogHistoryEvidence = "test fixture has complete history"
	review.PriorAdjustmentsEvidence = "test fixture has no prior adjustment"
	require.Len(t, review.Refunds, 1)
	review.Refunds[0].Decision = "missed_usage_decrement"
	review.Refunds[0].EvidenceReference = "fixture: historical writer did not decrement cumulative usage"
	raw, err = common.Marshal(review)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(reviewPath, raw, 0o600))
	output, err = runRepairCommand(t, generate...)
	require.NoError(t, err, output)
	raw, err = os.ReadFile(manifestPath)
	require.NoError(t, err)
	sum := sha256.Sum256(raw)
	operation := []string{"-dsn", source, "-user-id", "91", "-manifest", manifestPath, "-manifest-sha256", hex.EncodeToString(sum[:]), "-operator-id", "1", "-confirm-stop-write"}
	if externalDSN != "" {
		operation = append(operation, "-engine-backup-confirmed")
	}
	wrongUser := append([]string{}, operation...)
	wrongUser[3] = "1"
	output, err = runRepairCommand(t, append(wrongUser, "-apply", "-backup", filepath.Join(dir, "wrong.db"))...)
	require.Error(t, err)
	assert.Contains(t, output, "does not match -user-id")
	_, err = os.Stat(filepath.Join(dir, "wrong.db"))
	assert.True(t, os.IsNotExist(err))
	output, err = runRepairCommand(t, append(operation, "-apply", "-backup", filepath.Join(dir, "before.db"))...)
	require.NoError(t, err, output)
	assert.Contains(t, output, "applied=true")
	assert.Contains(t, output, "post_apply status=zero_pending used_quota=60")
	var user model.User
	require.NoError(t, db.Take(&user, 91).Error)
	assert.Equal(t, 60, user.UsedQuota)
	assert.Equal(t, 200, user.Quota)
	assert.Equal(t, 1, user.RequestCount)
	output, err = runRepairCommand(t, append(operation, "-apply", "-backup", filepath.Join(dir, "before.db"))...)
	require.NoError(t, err, output)
	assert.Contains(t, output, "applied=already_done")
	output, err = runRepairCommand(t, append(operation, "-rollback", "-backup", filepath.Join(dir, "rollback.db"))...)
	require.NoError(t, err, output)
	assert.Contains(t, output, "applied=true")
	output, err = runRepairCommand(t, append(operation, "-rollback")...)
	require.NoError(t, err, output)
	assert.Contains(t, output, "applied=already_done")
	require.NoError(t, db.Take(&user, 91).Error)
	assert.Equal(t, 100, user.UsedQuota)
	assert.Equal(t, 200, user.Quota)
	var count int64
	require.NoError(t, db.Model(&model.AuditLog{}).Count(&count).Error)
	assert.Equal(t, int64(2), count)
	// Reset only this disposable fixture's old maintenance history, then exercise
	// startup correction and its command rollback without a manifest file.
	require.NoError(t, db.Where("action = ?", model.UsageRepairAction).Delete(&model.AuditLog{}).Error)
	report, err := model.RepairRecordedUserUsage(db, db)
	require.NoError(t, err)
	assert.Equal(t, 1, report.Corrected)
	autoRollback := []string{"-dsn", source, "-user-id", "91", "-operator-id", "1", "-confirm-stop-write", "-rollback-recorded"}
	if externalDSN != "" {
		autoRollback = append(autoRollback, "-engine-backup-confirmed")
	}
	output, err = runRepairCommand(t, append(autoRollback, "-backup", filepath.Join(dir, "auto-rollback.db"))...)
	require.NoError(t, err, output)
	assert.Contains(t, output, "applied=true")
	output, err = runRepairCommand(t, autoRollback...)
	require.NoError(t, err, output)
	assert.Contains(t, output, "applied=already_done")
	require.NoError(t, db.Take(&user, 91).Error)
	assert.Equal(t, 100, user.UsedQuota)
	assert.Equal(t, 200, user.Quota)
	if externalDSN != "" {
		return
	}
	// Backup is an independently readable pre-apply snapshot.
	backup, err := openMainDB(filepath.Join(dir, "before.db"), false)
	require.NoError(t, err)
	backupSQL, err := backup.DB()
	require.NoError(t, err)
	defer backupSQL.Close()
	require.NoError(t, backup.Take(&user, 91).Error)
	assert.Equal(t, 100, user.UsedQuota)
}

func TestUsageRepairDatabaseIdentity(t *testing.T) {
	cases := []struct {
		dsn      string
		kind     common.DatabaseType
		identity string
	}{
		{"alice:secret@tcp(localhost:3306)/billing?charset=utf8mb4", common.DatabaseTypeMySQL, "mysql:tcp:localhost:3306/billing"},
		{"bob:other@tcp(localhost:3306)/billing?parseTime=true", common.DatabaseTypeMySQL, "mysql:tcp:localhost:3306/billing"},
		{"postgres://alice:secret@localhost:5432/billing?sslmode=disable", common.DatabaseTypePostgreSQL, "postgres:localhost:5432/billing"},
		{"host=localhost port=5432 user=alice password=secret dbname=billing sslmode=disable", common.DatabaseTypePostgreSQL, "postgres:localhost:5432/billing"},
	}
	for _, tc := range cases {
		t.Run(string(tc.kind)+tc.identity, func(t *testing.T) {
			_, kind, identity, err := parseDatabase(tc.dsn)
			require.NoError(t, err)
			assert.Equal(t, tc.kind, kind)
			assert.Equal(t, tc.identity, identity)
			assert.NotContains(t, identity, "secret")

		})
	}
	_, _, _, err := parseDatabase("clickhouse://user:secret@localhost/logs")
	require.Error(t, err)
	assert.False(t, strings.Contains(err.Error(), "secret"))
}
