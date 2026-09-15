// recover-synlink-failure is a single-task, offline SQLite maintenance tool.
// It never queries a provider, loads active plugins or resends a generation.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	sqlitedriver "github.com/glebarez/go-sqlite"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, output io.Writer) error {
	flags := flag.NewFlagSet("recover-synlink-failure", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var scope model.SynlinkFailureRecovery
	database := flags.String("database", "", "existing local SQLite file containing main and audit/log tables")
	backup := flags.String("backup", "", "new consistent backup file, required for apply")
	apply := flags.Bool("apply", false, "record verified failure and refund; default is read-only preview")
	verified := flags.Bool("verified-failed", false, "operator verified HTTP 200, matching frozen task ID, failed status and no output")
	offline := flags.Bool("offline-no-redis", false, "all application workers stopped and deployment does not use Redis")
	flags.StringVar(&scope.TaskID, "task-id", "", "one public Task ID")
	flags.IntVar(&scope.UserID, "user-id", 0, "task owner ID")
	flags.IntVar(&scope.AppID, "app-id", -1, "task application ID (explicit 0 allowed)")
	flags.IntVar(&scope.ChannelID, "channel-id", 0, "frozen channel ID")
	flags.IntVar(&scope.OperatorID, "operator-id", 0, "enabled Root operator ID for attribution")
	flags.IntVar(&scope.ExpectedQuota, "expected-quota", 0, "verified held quota; a precondition, not a credit instruction")
	flags.StringVar(&scope.EvidenceRef, "evidence-ref", "", "protected evidence reference: letters, digits, underscore or hyphen; no payloads")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			flags.SetOutput(output)
			flags.PrintDefaults()
			return nil
		}
		return errors.New("stage=arguments code=invalid_input: invalid flags; use -help for accepted arguments")
	}
	if flags.NArg() != 0 {
		return errors.New("stage=arguments code=invalid_input: unexpected positional arguments")
	}
	if *database == "" || (*apply && (!*verified || !*offline || *backup == "")) {
		return errors.New("stage=arguments code=invalid_input: database required; apply also requires verified-failed, offline-no-redis and a new backup path")
	}
	path, err := filepath.Abs(*database)
	if err != nil {
		return errors.New("stage=database_open code=invalid_input: invalid database path")
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("stage=database_open code=database_unavailable: database must be an existing regular SQLite file")
	}
	mode := "ro"
	if *apply {
		mode = "rw"
	}
	uri := url.URL{Scheme: "file", Path: path}
	query := url.Values{"mode": {mode}, "_pragma": {"busy_timeout(5000)"}}
	if *apply {
		query.Set("_txlock", "immediate")
	}
	uri.RawQuery = query.Encode()
	db, err := gorm.Open(sqlite.Open(uri.String()), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		return recoveryDiagnostic("database_open", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return recoveryDiagnostic("database_open", err)
	}
	defer sqlDB.Close()
	sqlDB.SetMaxOpenConns(1)
	// Deliberately do not initialize the application: no migrations, schedulers,
	// current channel settings, Redis or background batch writes belong here.
	previousDB, previousLogDB := model.DB, model.LOG_DB
	model.DB, model.LOG_DB = db, db
	defer func() { model.DB, model.LOG_DB = previousDB, previousLogDB }()
	if os.Getenv("REDIS_CONN_STRING") != "" || common.RDB != nil || common.MemoryCacheEnabled || common.BatchUpdateEnabled {
		return errors.New("stage=preflight code=environment_mismatch: maintenance process must not use caches or batch writes")
	}
	common.RedisEnabled = false // The package default is true until normal server initialization.
	task, err := model.RecoverSynlinkFailedTask(scope, false)
	if err != nil {
		return recoveryDiagnostic("preflight", err)
	}
	fmt.Fprintf(output, "task=%s status=%s billing=%s held_quota=%d target_quota=0 apply=%t\n", task.TaskID, task.Status, task.BillingState, task.Quota, *apply)
	if !*apply {
		return nil
	}
	file, err := os.OpenFile(*backup, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return errors.New("stage=backup code=backup_unavailable: backup path must be new and writable")
	}
	if err := file.Close(); err != nil {
		return errors.New("stage=backup code=backup_unavailable: cannot close backup file")
	}
	if err := db.Exec("VACUUM INTO ?", *backup).Error; err != nil {
		return fmt.Errorf("%w; cannot create consistent backup; no task changes applied", recoveryDiagnostic("backup", err))
	}
	task, err = model.RecoverSynlinkFailedTask(scope, true)
	if err != nil {
		return fmt.Errorf("%w; terminal decision rejected or rolled back; no refund attempted", recoveryDiagnostic("terminal_decision", err))
	}
	// Existing transaction reads the locked task's held amount, not the CLI amount.
	if _, _, err := model.ApplyTaskBillingTarget(task, 0); err != nil {
		return fmt.Errorf("%w; failure decision audited; refund pending, resolve the cause and repeat the same scope with a new backup", recoveryDiagnostic("refund", err))
	}
	var events []model.TaskBillingDelivery
	if err := db.Where("task_row_id = ? AND delivered_at = 0", task.ID).Order("id").Find(&events).Error; err != nil {
		return fmt.Errorf("%w; funding settled; log delivery verification pending", recoveryDiagnostic("log_delivery", err))
	}
	for _, event := range events {
		if err := model.DeliverTaskBillingLog(context.Background(), event.ID, service.BuildTaskBillingDeliveryLog); err != nil {
			return fmt.Errorf("%w; funding settled; log delivery pending, repeat the same scope with a new backup", recoveryDiagnostic("log_delivery", err))
		}
	}
	fmt.Fprintf(output, "task=%s status=%s billing=settled held_quota=%d refund_logs_delivered=true\n", task.TaskID, task.Status, task.Quota)
	return nil
}

// recoveryDiagnostic exposes only known static categories, never driver messages,
// SQL, paths or wrapped values. Unknown causes stay private even when wrapped.
func recoveryDiagnostic(stage string, err error) error {
	for _, rejection := range []error{
		model.ErrSynlinkRecoveryEnvironmentMismatch, model.ErrSynlinkRecoveryInvalidInput,
		model.ErrSynlinkRecoveryOperatorInvalid, model.ErrSynlinkRecoveryTaskScopeMismatch,
		model.ErrSynlinkRecoveryFrozenContractMismatch, model.ErrSynlinkRecoveryFrozenFundingInvalid,
		model.ErrSynlinkRecoveryEvidenceConflict, model.ErrSynlinkRecoveryQuotaMismatch,
		model.ErrSynlinkRecoveryTaskStateConflict, model.ErrSynlinkRecoveryTaskChanged,
	} {
		if errors.Is(err, rejection) {
			return fmt.Errorf("stage=%s code=%s", stage, rejection.Error())
		}
	}
	var databaseError *sqlitedriver.Error
	switch {
	case errors.Is(err, model.ErrTaskBillingInsufficientFunding):
		return fmt.Errorf("stage=%s code=funding_conflict: recheck frozen funding and current balances", stage)
	case errors.Is(err, gorm.ErrRecordNotFound):
		return fmt.Errorf("stage=%s code=frozen_record_missing: recheck the frozen task and funding records", stage)
	case errors.As(err, &databaseError):
		return fmt.Errorf("stage=%s code=database_error: check database availability, schema and constraints", stage)
	default:
		return fmt.Errorf("stage=%s code=operation_failed: inspect the maintenance environment and protected evidence", stage)
	}
}
