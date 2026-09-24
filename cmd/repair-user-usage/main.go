// repair-user-usage performs the one-off users.used_quota statistical
// correction documented in docs/80-dev/2026-09-23-用户历史用量虚高分析与修复方案.md.
// Preview is the default and only reads. Apply requires an operator-reviewed
// manifest, a stop-write window and a consistent backup; it updates
// users.used_quota and writes one audit record, nothing else.
package main

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"flag"
	"fmt"
	mysqlDSN "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type options struct {
	rollbackRecorded      bool
	reviewPath            string
	writeReview           string
	dsn                   string
	logDSN                string
	userID                int
	manifestPath          string
	manifestSHA256        string
	writeManifest         string
	apply                 bool
	rollback              bool
	operatorID            int
	confirmStopWrite      bool
	engineBackupConfirmed bool
	backupPath            string
	manualRefundsVerified bool
}

func main() {
	opts := &options{}
	flag.StringVar(&opts.dsn, "dsn", "", "main database DSN or SQLite file path (required)")
	flag.StringVar(&opts.logDSN, "log-dsn", "", "separate log database DSN; preview only, apply refuses split databases")
	flag.IntVar(&opts.userID, "user-id", 0, "target customer id (required)")
	flag.StringVar(&opts.writeManifest, "write-manifest", "", "write the correction manifest JSON to this path after preview")
	flag.StringVar(&opts.manifestPath, "manifest", "", "manifest JSON path for -apply or -rollback")
	flag.StringVar(&opts.manifestSHA256, "manifest-sha256", "", "SHA-256 hex of the manifest file, required with -apply and -rollback")
	flag.BoolVar(&opts.apply, "apply", false, "apply the reviewed manifest")
	flag.BoolVar(&opts.rollback, "rollback", false, "build and apply the reverse operation of the given manifest")
	flag.IntVar(&opts.operatorID, "operator-id", 0, "root operator user id")
	flag.BoolVar(&opts.confirmStopWrite, "confirm-stop-write", false, "attest that all writers are stopped in a maintenance window")
	flag.BoolVar(&opts.engineBackupConfirmed, "engine-backup-confirmed", false, "attest a consistent engine-level backup exists (non-SQLite apply)")
	flag.StringVar(&opts.backupPath, "backup", "", "new SQLite backup path created with VACUUM INTO before apply")
	flag.BoolVar(&opts.manualRefundsVerified, "manual-refunds-verified", false, "attest the prefix-bucket manual refunds were reviewed offline")
	flag.StringVar(&opts.reviewPath, "review", "", "offline reviewed evidence JSON required to generate a manifest")
	flag.StringVar(&opts.writeReview, "write-review", "", "write an unapproved per-refund review template during preview")
	flag.BoolVar(&opts.rollbackRecorded, "rollback-recorded", false, "reverse the selected user startup correction from its durable audit")
	flag.Parse()
	if opts.rollbackRecorded {
		if opts.apply || opts.rollback || opts.manifestPath != "" {
			fail("-rollback-recorded cannot be combined with -apply, -rollback or -manifest")
		}
		opts.rollback = true
	}

	if opts.dsn == "" || opts.userID <= 0 {
		fail("explicit -dsn and -user-id are required")
	}
	if opts.apply && opts.rollback {
		fail("-apply and -rollback are mutually exclusive")
	}
	writeOnly := opts.writeManifest != "" && !opts.apply && !opts.rollback
	if (opts.apply || opts.rollback) && ((!opts.rollbackRecorded && (opts.manifestPath == "" || opts.manifestSHA256 == "")) || opts.operatorID <= 0 || !opts.confirmStopWrite) {
		fail("writes require -operator-id and -confirm-stop-write; -apply/-rollback also require -manifest and -manifest-sha256")
	}
	if opts.logDSN != "" && (opts.apply || opts.rollback || opts.writeManifest != "" || opts.writeReview != "") {
		fail("split log databases are preview-only; apply requires logs and audit in the main database")
	}

	switch {
	case opts.apply:
		runApply(opts)
	case opts.rollback:
		runRollback(opts)
	case writeOnly:
		runWriteManifest(opts)
	default:
		runPreview(opts)
	}
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}

func printAppliedResult(applied bool, manifest *model.UsageRepairManifest) {
	if applied {
		fmt.Printf("applied=true operation_id=%s after=%d\n", manifest.OperationID, manifest.ExpectedAfter)
		return
	}
	fmt.Printf("applied=already_done operation_id=%s after=%d\n", manifest.OperationID, manifest.ExpectedAfter)
}

// Database identity excludes credentials and connection tuning parameters.
func parseDatabase(dsn string) (gorm.Dialector, common.DatabaseType, string, error) {
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") || strings.Contains(dsn, "host=") || strings.Contains(dsn, "dbname=") {
		config, err := pgconn.ParseConfig(dsn)
		if err != nil {
			return nil, "", "", fmt.Errorf("invalid PostgreSQL DSN")
		}
		identity := fmt.Sprintf("postgres:%s:%d/%s", config.Host, config.Port, config.Database)
		return postgres.Open(dsn), common.DatabaseTypePostgreSQL, identity, nil
	}
	if strings.Contains(dsn, "@tcp(") || strings.Contains(dsn, "@unix(") {
		config, err := mysqlDSN.ParseDSN(dsn)
		if err != nil {
			return nil, "", "", fmt.Errorf("invalid MySQL DSN")
		}
		return mysql.Open(dsn), common.DatabaseTypeMySQL, "mysql:" + config.Net + ":" + config.Addr + "/" + config.DBName, nil
	}
	if strings.Contains(dsn, "://") || strings.Contains(dsn, "?") || strings.HasPrefix(dsn, "file:") || dsn == ":memory:" || dsn == "" {
		return nil, "", "", fmt.Errorf("unsupported DSN; SQLite requires a plain filesystem path")
	}
	absolute, err := filepath.Abs(dsn)
	if err != nil {
		return nil, "", "", fmt.Errorf("invalid SQLite path")
	}
	// Resolve aliases so a manifest cannot accidentally refer to a different file.
	absolute, err = filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, "", "", fmt.Errorf("SQLite source must already exist")
	}
	return sqlite.Open(absolute), common.DatabaseTypeSQLite, "sqlite:" + absolute, nil
}

func openMainDB(dsn string, writeMode bool) (*gorm.DB, error) {
	driver, kind, identity, err := parseDatabase(dsn)
	if err != nil {
		return nil, err
	}
	if kind == common.DatabaseTypeSQLite {
		mode := "ro"
		if writeMode {
			mode = "rw"
		}
		uri := url.URL{Scheme: "file", Path: strings.TrimPrefix(identity, "sqlite:")}
		params := url.Values{"mode": {mode}, "_pragma": {"busy_timeout(5000)"}}
		if writeMode {
			params.Set("_txlock", "immediate")
		}
		uri.RawQuery = params.Encode()
		driver = sqlite.Open(uri.String())
	}
	db, err := gorm.Open(driver, &gorm.Config{Logger: logger.Discard})
	if err != nil {
		return nil, fmt.Errorf("cannot open %s database", kind)
	}
	common.SetMainDatabaseType(kind)
	return db, nil
}

func openLogDB(dsn string) (*gorm.DB, error) {
	mainKind := common.MainDatabaseType()
	db, err := openMainDB(dsn, false)
	if err == nil {
		common.SetLogDatabaseType(common.MainDatabaseType())
	}
	common.SetMainDatabaseType(mainKind)
	return db, err
}

func isSQLitePath(dsn string) bool {
	_, kind, _, err := parseDatabase(dsn)
	return err == nil && kind == common.DatabaseTypeSQLite
}

func sourceDBIdentity(dsn string) string {
	_, _, identity, _ := parseDatabase(dsn)
	return identity
}

func collectAssessment(db *gorm.DB, logDB *gorm.DB, userID int, review *model.UsageRepairReview) (*model.UsageRepairEvidence, *model.UsageRepairAssessment, error) {
	var evidence *model.UsageRepairEvidence
	var err error
	if db != logDB {
		evidence, err = model.CollectUserUsageRepairEvidence(db, logDB, userID)
	} else {
		isolation := sql.LevelRepeatableRead
		if db.Dialector.Name() == "sqlite" {
			isolation = sql.LevelDefault
		}
		err = db.Transaction(func(tx *gorm.DB) error {
			evidence, err = model.CollectUserUsageRepairEvidence(tx, tx, userID)
			return err
		}, &sql.TxOptions{Isolation: isolation, ReadOnly: true})
	}
	if err != nil {
		return nil, nil, err
	}
	assessment := model.DeriveUsageRepairAssessment(evidence, evidence.PriorOps, review)
	if db != logDB {
		assessment = &model.UsageRepairAssessment{Status: model.UsageRepairStatusNotCorrectable, Issues: []string{"split databases are diagnostic-only; no consistent cross-database snapshot"}}
	}
	return evidence, assessment, nil
}

func loadReview(opts *options) (*model.UsageRepairReview, error) {
	if opts.reviewPath == "" {
		return nil, nil
	}
	raw, err := os.ReadFile(opts.reviewPath)
	if err != nil {
		return nil, err
	}
	var review model.UsageRepairReview
	if err := common.Unmarshal(raw, &review); err != nil {
		return nil, err
	}
	if review.UserID != opts.userID || review.SourceDBIdentity != sourceDBIdentity(opts.dsn) {
		return nil, fmt.Errorf("review target or source database mismatch")
	}
	return &review, nil
}

func newOperationID() (string, error) {
	raw := make([]byte, 8)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return "usage-repair-" + hex.EncodeToString(raw), nil
}

func loadManifest(opts *options) (*model.UsageRepairManifest, error) {
	raw, err := os.ReadFile(opts.manifestPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read manifest: %w", err)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != strings.ToLower(opts.manifestSHA256) {
		return nil, fmt.Errorf("manifest sha256 mismatch")
	}
	manifest := &model.UsageRepairManifest{}
	if err := common.UnmarshalJsonStr(string(raw), manifest); err != nil {
		return nil, fmt.Errorf("cannot decode manifest: %w", err)
	}
	if manifest.UserID != opts.userID {
		return nil, fmt.Errorf("manifest user %d does not match -user-id %d", manifest.UserID, opts.userID)
	}
	if manifest.SourceDBIdentity != sourceDBIdentity(opts.dsn) {
		return nil, fmt.Errorf("manifest source database mismatch")
	}
	if err := model.ValidateUsageRepairManifest(manifest); err != nil {
		return nil, err
	}
	return manifest, nil
}

func runPreview(opts *options) {
	db, err := openMainDB(opts.dsn, false)
	if err != nil {
		fail(err.Error())
	}
	logDB := db
	if opts.logDSN != "" {
		logDB, err = openLogDB(opts.logDSN)
		if err != nil {
			fail(err.Error())
		}
	}
	review, err := loadReview(opts)
	if err != nil {
		fail(err.Error())
	}
	evidence, assessment, err := collectAssessment(db, logDB, opts.userID, review)
	if err != nil {
		fail(err.Error())
	}
	fingerprint, ferr := evidence.Fingerprint()
	if ferr != nil {
		fail(ferr.Error())
	}
	printEvidence(evidence)
	printAssessment(assessment)
	fmt.Printf("evidence_fingerprint=%s source=%s\n", fingerprint, sourceDBIdentity(opts.dsn))
	if opts.writeReview != "" {
		draft, err := model.NewUsageRepairReview(evidence, sourceDBIdentity(opts.dsn))
		if err != nil {
			fail(err.Error())
		}
		encoded, err := common.Marshal(draft)
		if err != nil {
			fail(err.Error())
		}
		if err := writeNewReviewArtifact(opts.writeReview, encoded); err != nil {
			fail(err.Error())
		}
		fmt.Println("unapproved review template written; unresolved decisions cannot be applied")
	}
}

// formatUSD renders the display-only quota/500000 conversion with six
// fraction digits; it never feeds back into quota arithmetic. One quota is
// 0.000002 USD, so the remainder is doubled to obtain micro-dollars.
func formatUSD(quota int64) string {
	sign := ""
	value := uint64(quota)
	if quota < 0 {
		sign = "-"
		value = uint64(-(quota + 1)) + 1
	}
	return fmt.Sprintf("%s%d.%06d", sign, value/500000, (value%500000)*2)
}

func printEvidence(e *model.UsageRepairEvidence) {
	fmt.Printf("user=%d username=%s used_quota=%d (usd %s)\n", e.UserID, e.Username, e.UsedQuota, formatUSD(e.UsedQuota))
	fmt.Printf("wallet_quota=%d request_count=%d (invariant fields, never written)\n", e.WalletQuota, e.RequestCount)
	fmt.Printf("consume logs: count=%d quota=%d (usd %s)\n", e.ConsumeCount, e.ConsumeQuota, formatUSD(e.ConsumeQuota))
	fmt.Printf("refund logs: count=%d quota=%d (usd %s)\n", e.RefundCount, e.RefundQuota, formatUSD(e.RefundQuota))
	fmt.Printf("log window: min_created_at=%d max_created_at=%d max_log_id=%d\n", e.MinLogCreatedAt, e.MaxLogCreatedAt, e.MaxLogID)
	fmt.Printf("duplicates=%d undelivered_events=%d delivery_events=%d quota_data_sum=%d\n", e.DuplicateRequestIDs, e.UndeliveredEvents, e.DeliveryCount, e.QuotaDataSum)
	for _, bucket := range e.Buckets {
		fmt.Printf("bucket %s: count=%d quota=%d (usd %s)\n", bucket.Kind, bucket.Count, bucket.Quota, formatUSD(bucket.Quota))
		for _, item := range bucket.Items {
			if bucket.Kind == model.UsageRepairBucketOtherUnmatched {
				continue
			} // Full list is in the review template.
			fmt.Printf("  item log_id=%d quota=%d request_id=%s manual=%t task_row=%d task_exists=%t delivery_events=%s preauth_log=%d preauth_exists=%t preauth_quota=%d operator=%s\n",
				item.LogID, item.Quota, item.RequestID, item.ManualRefund, item.TaskRowID, item.TaskExists,
				strings.Join(item.TaskDeliveryEvents, ","), item.OriginalPreauthLogID, item.PreauthExists, item.PreauthQuota, item.Operator)
		}
	}
	for _, channel := range e.ChannelUsages {
		fmt.Printf("channel %d used_quota=%d (informational, not corrected here)\n", channel.ChannelID, channel.UsedQuota)
	}
}

func printAssessment(a *model.UsageRepairAssessment) {
	fmt.Printf("assessment status=%s\n", a.Status)
	if a.Candidate != nil {
		fmt.Printf("candidate before=%d delta=%d after=%d (usd %s)\n",
			a.Candidate.ExpectedBefore, a.Candidate.Delta, a.Candidate.ExpectedAfter, formatUSD(a.Candidate.Delta))
	}
	for _, issue := range a.Issues {
		fmt.Printf("issue: %s\n", issue)
	}
}

func runWriteManifest(opts *options) {
	db, err := openMainDB(opts.dsn, false)
	if err != nil {
		fail(err.Error())
	}
	logDB := db
	if opts.logDSN != "" {
		logDB, err = openLogDB(opts.logDSN)
		if err != nil {
			fail(err.Error())
		}
	}
	if opts.operatorID <= 0 {
		fail("-operator-id is required with -write-manifest")
	}
	review, err := loadReview(opts)
	if err != nil {
		fail(err.Error())
	}
	evidence, assessment, err := collectAssessment(db, logDB, opts.userID, review)
	if err != nil {
		fail(err.Error())
	}
	if assessment.Status != model.UsageRepairStatusCorrectable || assessment.Candidate == nil {
		fail(fmt.Sprintf("current status is %s; no correction manifest can be written", assessment.Status))
	}
	for _, issue := range assessment.Issues {
		fmt.Printf("issue: %s\n", issue)
	}
	fingerprint, err := evidence.Fingerprint()
	if err != nil {
		fail(err.Error())
	}
	operationID, err := newOperationID()
	if err != nil {
		fail(err.Error())
	}
	offset := time.Now().Format("-07:00")
	manifest := &model.UsageRepairManifest{
		Version:                    2,
		Review:                     review,
		OperationID:                operationID,
		UserID:                     evidence.UserID,
		Username:                   evidence.Username,
		ExpectedBefore:             assessment.Candidate.ExpectedBefore,
		Delta:                      assessment.Candidate.Delta,
		ExpectedAfter:              assessment.Candidate.ExpectedAfter,
		ManualRefundBucketVerified: opts.manualRefundsVerified,
		EvidenceFingerprint:        fingerprint,
		Evidence:                   evidence,
		SourceDBIdentity:           sourceDBIdentity(opts.dsn),
		CollectedAtUnix:            time.Now().Unix(),
		CollectedAtUTCOffset:       offset,
		PreparedByUserID:           opts.operatorID,
	}
	if err := model.ValidateUsageRepairManifest(manifest); err != nil {
		fail(err.Error())
	}
	encoded, err := common.Marshal(manifest)
	if err != nil {
		fail(err.Error())
	}
	if err := writeNewReviewArtifact(opts.writeManifest, encoded); err != nil {
		fail(fmt.Sprintf("cannot write manifest: %v", err))
	}
	sum := sha256.Sum256(encoded)
	fmt.Printf("manifest written=%s operation_id=%s sha256=%s\n", opts.writeManifest, manifest.OperationID, hex.EncodeToString(sum[:]))
	fmt.Printf("verify before apply with: -manifest-sha256 %s\n", hex.EncodeToString(sum[:]))
}

func runApply(opts *options) {
	manifest, err := loadManifest(opts)
	if err != nil {
		fail(err.Error())
	}
	db, err := openMainDB(opts.dsn, true)
	if err != nil {
		fail(err.Error())
	}
	already, err := model.UsageRepairAlreadyApplied(db, manifest)
	if err != nil {
		fail(err.Error())
	}
	if already {
		printAppliedResult(false, manifest)
		return
	}
	backupSQLite(opts)
	applied, err := model.ApplyUserUsageRepair(db, manifest, opts.operatorID)
	if err != nil {
		fail(err.Error())
	}
	printAppliedResult(applied, manifest)
	evidence, assessment, err := collectAssessment(db, db, opts.userID, nil)
	if err != nil {
		fail(err.Error())
	}
	fmt.Printf("post_apply status=%s used_quota=%d\n", assessment.Status, evidence.UsedQuota)
}

func runRollback(opts *options) {
	db, err := openMainDB(opts.dsn, false)
	if err != nil {
		fail(err.Error())
	}
	var reverse *model.UsageRepairManifest
	var already bool
	if opts.rollbackRecorded {
		reverse, already, err = model.PrepareRecordedUsageRepairRollback(db, opts.userID, sourceDBIdentity(opts.dsn))
	} else {
		var original *model.UsageRepairManifest
		original, err = loadManifest(opts)
		if err == nil {
			reverse, already, err = model.PrepareUsageRepairRollback(db, original)
		}
	}
	if err != nil {
		fail(err.Error())
	}
	if already {
		printAppliedResult(false, reverse)
		return
	}
	reverse.PreparedByUserID = opts.operatorID
	writeDB, err := openMainDB(opts.dsn, true)
	if err != nil {
		fail(err.Error())
	}
	backupSQLite(opts)
	applied, err := model.ApplyUserUsageRepair(writeDB, reverse, opts.operatorID)
	if err != nil {
		fail(err.Error())
	}
	printAppliedResult(applied, reverse)
}

func backupSQLite(opts *options) {
	isSQLite := isSQLitePath(opts.dsn)
	if isSQLite {
		if opts.backupPath == "" {
			fail("a consistent SQLite backup is required: pass a new -backup path (VACUUM INTO)")
		}
		db, err := openMainDB(opts.dsn, true)
		if err != nil {
			fail(err.Error())
		}
		file, err := os.OpenFile(opts.backupPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			fail("backup path must be new and writable")
		}
		_ = file.Close()
		if db.Exec("VACUUM INTO ?", opts.backupPath).Error != nil {
			fail("create consistent SQLite backup")
		}
		return
	}
	if !opts.engineBackupConfirmed {
		fail("confirm an engine-level consistent backup exists with -engine-backup-confirmed before apply")
	}
}

// Never overwrite a database, an approved manifest, or another existing file.
func writeNewReviewArtifact(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}
