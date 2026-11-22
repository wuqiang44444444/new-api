package service

import (
	"context"
	"errors"
	"math"
	"os"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shirou/gopsutil/disk"
	"gorm.io/gorm"
)

var (
	errCustomerStatementExportPressure    = errors.New("statement export waiting for resources")
	errCustomerStatementExportYield       = errors.New("statement export execution slice ended")
	errCustomerStatementExportPartFull    = errors.New("statement export shard is full")
	errCustomerStatementExportCalibration = errors.New("statement export measuring foreground baseline")
)

var reportArtifactUploads atomic.Int64

const (
	statementExportMinRowsPerSecond = 1_000
	statementExportMaxRowsPerSecond = 16_000
	statementExportMaxPageRows      = 1_000
	statementExportPartRows         = 500_000
	statementExportPartBytes        = 300_000_000
	statementExportCommitReserve    = 2 * time.Minute
	statementExportUploadStage      = "customer_statement_export"
)

type customerStatementExportPolicy struct {
	beforeBatch func(context.Context) error
	currentRate func() int
	partRows    int
}

func newCustomerStatementExportPolicy() customerStatementExportPolicy {
	var speed adaptiveReportRate
	var sample func(context.Context) ([]model.DailyReportDatabaseLoad, error)
	var lastDiskCheck time.Time
	var diskAvailable bool
	return customerStatementExportPolicy{
		partRows:    statementExportPartRows,
		currentRate: func() int { return speed.rate },
		beforeBatch: func(ctx context.Context) error {
			if sample == nil {
				var err error
				sample, err = model.DailyReportDatabaseSampler(ctx)
				if err != nil {
					return errCustomerStatementExportPressure
				}
			}
			decision := checkReportAdmission(ctx, sample)
			if time.Since(lastDiskCheck) >= 3*time.Second {
				lastDiskCheck = time.Now()
				usage, err := disk.UsageWithContext(ctx, os.TempDir())
				diskAvailable = err == nil && usage.Free >= 256<<20
			}
			if !diskAvailable {
				decision.pressure = reportPause
				decision.calibrating = false
			}
			if decision.calibrating {
				speed.last = decision.at
				return errCustomerStatementExportCalibration
			}
			speed.advance(decision, statementExportMinRowsPerSecond, statementExportMaxRowsPerSecond)
			if decision.pressure == reportPause {
				return errCustomerStatementExportPressure
			}
			return ctx.Err()
		},
	}
}

func (p customerStatementExportPolicy) waitForAdmission(ctx context.Context) error {
	for {
		err := p.beforeBatch(ctx)
		if !errors.Is(err, errCustomerStatementExportCalibration) {
			return err
		}
		if err = waitDailyReportBudget(ctx, reportObservationInterval); err != nil {
			return err
		}
	}
}

func executeCustomerStatementExport(ctx context.Context, job *model.CustomerExportJob, filters model.CustomerExportFilters, workDir string, policy customerStatementExportPolicy) (*model.CustomerExportArtifact, error) {
	slice, stop := context.WithTimeout(ctx, 5*time.Minute)
	defer stop()
	deadline, _ := slice.Deadline()
	readWindow, stopReading := context.WithDeadlineCause(slice, deadline.Add(-statementExportCommitReserve), errCustomerStatementExportYield)
	defer stopReading()
	if err := policy.waitForAdmission(slice); err != nil {
		return nil, err
	}
	store, err := currentExportObjectStore()
	if err != nil {
		return nil, err
	}
	reportDB, closeSource, err := model.OpenCustomerStatementExportSource()
	if err != nil {
		return nil, errCustomerStatementExportPressure
	}
	defer closeSource()
	if err := os.MkdirAll(workDir, 0o700); err != nil {
		return nil, err
	}
	workDir, err = os.MkdirTemp(workDir, "lease-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(workDir)
	checkCtx, cancel := context.WithTimeout(slice, customerExportBatchTimeout)
	err = model.CheckCustomerStatementExportSource(checkCtx, reportDB)
	cancel()
	if err != nil {
		return nil, errCustomerStatementExportPressure
	}

	artifact := job.DecodeArtifact()
	if artifact == nil {
		if job.Artifact != "" {
			return nil, errors.New("invalid statement export checkpoint")
		}
		artifact = &model.CustomerExportArtifact{
			StoreIdentity: store.ExportIdentity(), GeneratedAt: common.GetTimestamp(),
			Files: []model.CustomerExportArtifactFile{}, Checkpoint: &model.CustomerStatementExportCheckpoint{},
			StatementTotals: &model.CustomerStatementExportTotals{},
		}
	}
	if artifact.StoreIdentity != store.ExportIdentity() {
		return nil, ErrCustomerExportStorageUnavailable
	}
	if artifact.Checkpoint == nil || artifact.StatementTotals == nil {
		return nil, errors.New("invalid statement export checkpoint")
	}

	var pending *model.CustomerExportArtifactFile
	if artifact.Checkpoint.PendingFile {
		if len(artifact.Files) == 0 {
			return nil, errors.New("missing pending statement export file")
		}
		file := artifact.Files[len(artifact.Files)-1]
		pending = &file
		artifact.Files = artifact.Files[:len(artifact.Files)-1]
	}

	scope := customerExportScopeColumns{
		Language: filters.Language, PeriodStart: filters.StartTimestamp, PeriodEnd: filters.EndTimestamp - 1,
		Currency: filters.Currency, CurrencyRate: filters.CurrencyRate, QuotaPerUnit: filters.QuotaPerUnit,
		Timezone: filters.Timezone, GeneratedAt: artifact.GeneratedAt,
	}
	for {
		readCtx := readWindow
		if pending != nil {
			// Reconstruct a previously staged shard to its recorded boundary.
			readCtx = slice
		}
		// Only reads use the shorter window. A staged shard can upload and commit
		// against the remaining lease-bound job budget.
		done, err := writeCustomerStatementExportPart(ctx, readCtx, reportDB, store, job, filters, artifact, pending, scope, workDir, policy)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) && errors.Is(slice.Err(), context.DeadlineExceeded) && ctx.Err() == nil {
				return nil, errCustomerStatementExportYield
			}
			return nil, err
		}
		if done {
			artifact.Checkpoint = nil
			artifact.MissingDates = model.CustomerStatementExportMissingDates(ctx, filters)
			artifact.ExpiresAt = common.GetTimestamp() + int64(customerExportFileRetention.Seconds())
			return artifact, nil
		}
		pending = nil
		if readWindow.Err() != nil {
			return nil, errCustomerStatementExportYield
		}
	}
}

func writeCustomerStatementExportPart(ctx, readCtx context.Context, reportDB *gorm.DB, store exportObjectStore, job *model.CustomerExportJob, filters model.CustomerExportFilters,
	artifact *model.CustomerExportArtifact, pending *model.CustomerExportArtifactFile, scope customerExportScopeColumns, workDir string, policy customerStatementExportPolicy) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	writer, err := newCustomerExportCsvWriter(workDir, "statement-details", statementExportPartBytes)
	if err != nil {
		return false, err
	}
	defer writer.Cleanup()
	writer.useStatementDetails(filters.Language)
	writer.setBillingCoverage(filters)
	writer.statementPart = true
	writer.shardIndex = len(artifact.Files)

	cursor := artifact.Checkpoint.Cursor
	totals := *artifact.StatementTotals
	partRows := int64(policy.partRows)
	if pending != nil {
		partRows = pending.LineCount
	}
	var scanned int64
	var eof, full bool
	for writer.totalLines < partRows && (pending != nil || scanned < partRows) && !eof && !full {
		started := time.Now()
		var page model.CustomerStatementExportPage
		for {
			if err = policy.waitForAdmission(readCtx); err != nil {
				break
			}
			err = executeSharedReportBatch(readCtx, policy.beforeBatch, func(batchCtx context.Context) error {
				pageRows := statementExportMaxPageRows
				if policy.currentRate != nil {
					// Keep low-speed batches short enough to yield quickly.
					pageRows = min(pageRows, max(1, policy.currentRate()/8))
				}
				if pending == nil {
					pageRows = int(min(int64(pageRows), partRows-scanned))
				}
				batchCtx, cancel := context.WithTimeout(batchCtx, 2*time.Second)
				defer cancel()
				var readErr error
				page, readErr = model.ReadCustomerStatementExportEvents(batchCtx, reportDB, job.TargetUserId, filters, cursor, pageRows)
				return readErr
			})
			if !errors.Is(err, errReportBatchDeferred) && !errors.Is(err, errCustomerStatementExportCalibration) {
				break
			}
			if err = waitDailyReportBudget(readCtx, 100*time.Millisecond); err != nil {
				break
			}
		}
		if err != nil && errors.Is(context.Cause(readCtx), errCustomerStatementExportYield) && ctx.Err() == nil {
			// Seal the accumulated rows while there is still time to upload and checkpoint.
			break
		}
		var pgErr *pgconn.PgError
		if readCtx.Err() == nil && (errors.Is(err, context.DeadlineExceeded) || pgconn.SafeToRetry(err) ||
			(errors.As(err, &pgErr) && (pgErr.Code == "40001" || pgErr.Code == "57014" || pgErr.Code == "57P01" || pgErr.Code == "57P03"))) {
			return false, errCustomerStatementExportPressure
		}
		if err != nil {
			return false, err
		}
		eof = page.Done
		scanned += int64(page.Scanned)
		for _, event := range page.Events {
			if writer.totalLines >= partRows {
				full, eof = true, false
				break
			}
			err = writer.AppendRow(event.Row, scope)
			if errors.Is(err, errCustomerStatementExportPartFull) {
				full, eof = true, false
				break
			}
			if err != nil {
				return false, err
			}
			cursor = event.SourceID
			if event.Charge < 0 {
				if -event.Charge > math.MaxInt64-totals.Refunds {
					return false, errors.New("statement export refund total exceeds supported range")
				}
				totals.Refunds -= event.Charge
			} else {
				if event.Charge > math.MaxInt64-totals.Charges {
					return false, errors.New("statement export charge total exceeds supported range")
				}
				totals.Charges += event.Charge
			}
		}
		if !full {
			cursor = page.ScannedThrough
		}
		if policy.currentRate != nil {
			interval := time.Duration(page.Scanned) * time.Second / time.Duration(max(1, policy.currentRate()))
			if sleep := interval - time.Since(started); sleep > 0 {
				if err := waitDailyReportBudget(readCtx, sleep); err != nil {
					if errors.Is(context.Cause(readCtx), errCustomerStatementExportYield) && ctx.Err() == nil {
						break
					}
					return false, err
				}
			}
		}
	}

	if err := policy.waitForAdmission(ctx); err != nil {
		return false, err
	}
	files, paths, lines, _, err := writer.Finish()
	if errors.Is(err, errCustomerExportNoFiles) && pending == nil {
		if eof {
			return true, nil
		}
		artifact.Checkpoint.Cursor = cursor
		return false, model.SaveCustomerStatementExport(ctx, job.JobID, job.Executor, artifact, false)
	}
	if err != nil {
		return false, err
	}
	file := files[0]
	file.ObjectKey = CustomerExportObjectNamespace + "/" + job.JobID + "/" + file.FileName
	if pending != nil && (file.ObjectKey != pending.ObjectKey || file.Sha256 != pending.Sha256 || file.LineCount != pending.LineCount || file.SizeBytes != pending.SizeBytes) {
		return false, errors.New("regenerated statement export part differs from its checkpoint")
	}
	if err := policy.waitForAdmission(ctx); err != nil {
		return false, err
	}
	artifact.Files = append(artifact.Files, file)
	artifact.Checkpoint.PendingFile = true
	if err := model.SaveCustomerStatementExport(ctx, job.JobID, job.Executor, artifact, false); err != nil {
		return false, err
	}
	if err := uploadCustomerStatementExportPart(ctx, store, file, paths[0], policy.beforeBatch); err != nil {
		return false, err
	}

	artifact.LineCount += lines
	artifact.SizeBytes += file.SizeBytes
	totals.Net = totals.Charges - totals.Refunds
	artifact.StatementTotals = &totals
	artifact.Checkpoint.Cursor = cursor
	artifact.Checkpoint.PendingFile = false
	if err := model.SaveCustomerStatementExport(ctx, job.JobID, job.Executor, artifact, true); err != nil {
		return false, err
	}
	return eof, nil
}

func uploadCustomerStatementExportPart(ctx context.Context, store exportObjectStore, file model.CustomerExportArtifactFile, path string, gate func(context.Context) error) error {
	reportArtifactUploads.Add(1)
	defer reportArtifactUploads.Add(-1)
	ctx, cancel := context.WithCancelCause(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := gate(ctx); err != nil && !errors.Is(err, errCustomerStatementExportCalibration) {
					cancel(err)
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	const contentType = "text/csv; charset=utf-8"
	var err error
	if azure, ok := store.(*azureBlobArtifactStore); ok {
		err = azure.putObjectFile(ctx, file.ObjectKey, contentType, path, file.SizeBytes, statementExportUploadStage)
	} else {
		err = store.ExportPutFile(ctx, file.ObjectKey, contentType, path, file.SizeBytes)
	}
	if err != nil {
		if cause := context.Cause(ctx); cause != nil {
			err = cause
		} else {
			err = errors.Join(ErrCustomerExportStorageUnavailable, err)
		}
	}
	cancel(nil)
	<-done
	return err
}
