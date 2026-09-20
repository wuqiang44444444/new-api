package service

import (
	"bufio"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"
)

// 客户导出调度器：注册为独立 SystemTask 类型，复用现有租约设施保证全站
// 同一时刻只有一个源数据读取执行器。ExportJob 行是申请与交付的操作事实；
// SystemTask 只负责唤醒、租约与调度，不为每个客户发明 task type。

const (
	// SystemTask 唤醒任务类型；见 model.SystemTaskTypeCustomerExport。
	SystemTaskTypeCustomerExport = model.SystemTaskTypeCustomerExport

	// 计划 7.4 建议起点（待验证参数，不是 SLA）。
	customerExportBatchSize          = 1000
	customerExportBatchTimeout       = 5 * time.Second
	customerExportMaxRatePerSecond   = 2000
	customerExportJobBudget          = 10 * time.Minute
	customerExportQueueWaitBudget    = 30 * time.Minute
	customerExportRunSlice           = 8 * time.Minute
	customerExportFileRetention      = 24 * time.Hour
	customerExportRecordRetention    = 7 * 24 * time.Hour
	customerExportDownloadURLTTL     = 5 * time.Minute
	customerExportProgressInterval   = 2 * time.Second
	customerExportTotalBytes         = 512 << 20
	customerExportCleanupLimitPerRun = 5
)

var ErrCustomerExportInvalidRequest = errors.New("invalid export request")

type customerExportHandler struct{}

func (customerExportHandler) Type() string { return model.SystemTaskTypeCustomerExport }

func (customerExportHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	resume := runCustomerExportScheduler(ctx, runnerID+"-"+task.TaskID)
	status := model.SystemTaskStatusSucceeded
	if ctx.Err() != nil {
		status = model.SystemTaskStatusFailed
	}
	if err := model.FinishSystemTask(task.TaskID, runnerID, status, nil, ""); err != nil {
		logger.LogWarn(ctx, "customer export scheduler could not finish its task")
		return
	}
	if resume && ctx.Err() == nil {
		if queued, err := model.CountQueuedCustomerExportJobs(); err == nil && queued > 0 {
			_, _, _ = EnqueueSystemTask(SystemTaskTypeCustomerExport, nil)
		}
	}
}

// Periodic discovery repairs missed notifications and retries object cleanup.
func (customerExportHandler) Enabled() bool           { return true }
func (customerExportHandler) Interval() time.Duration { return time.Minute }
func (customerExportHandler) NewPayload() any         { return nil }

func init() {
	RegisterSystemTaskHandler(customerExportHandler{})
}

// SubmitCustomerExportJob 校验并规范化提交范围，原子受理后唤醒调度器。
// 鉴权、目标客户与角色由调用方（controller）先行校验。
func SubmitCustomerExportJob(initiatorId int, targetUserId int, request CustomerExportRequest) (*model.CustomerExportJob, error) {
	if err := model.AuthorizeCustomerExport(context.Background(), initiatorId, targetUserId); err != nil {
		return nil, err
	}
	store, err := currentExportObjectStore()
	if err != nil {
		return nil, err
	}
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		return nil, model.ErrCustomerExportBackendUnsupported
	}
	filters, err := normalizeCustomerExportFilters(request.JobType, request)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrCustomerExportInvalidRequest, err)
	}
	revisionCtx, cancel := context.WithTimeout(context.Background(), customerExportBatchTimeout)
	defer cancel()
	sourceVersion, err := model.CustomerStatementExportSourceVersion(revisionCtx, targetUserId, request.JobType, filters)
	if err != nil {
		return nil, err
	}
	job, created, err := model.CreateCustomerExportJob(initiatorId, targetUserId, request.JobType, filters, model.CustomerExportReuse{SourceVersion: sourceVersion, StoreIdentity: store.ExportIdentity()})
	if err != nil {
		return nil, err
	}
	if created {
		if _, _, err := EnqueueSystemTask(SystemTaskTypeCustomerExport, nil); err != nil {
			logger.LogWarn(context.Background(), fmt.Sprintf("customer export scheduler wake failed: %v", err))
		}
	}
	return job, nil
}

// CustomerExportRequest 是控制台提交的原始范围；规范化后冻结进任务行。
type CustomerExportRequest struct {
	JobType           string
	StartTimestamp    int64
	EndTimestamp      int64
	LogTypes          []int
	TokenId           *int
	ChannelId         *int   `json:"channel_id,omitempty"`
	TokenName         string `json:"token_name,omitempty"`
	Group             string `json:"group,omitempty"`
	RequestId         string `json:"request_id,omitempty"`
	UpstreamRequestId string `json:"upstream_request_id,omitempty"`
	Username          string `json:"username,omitempty"`
	ModelName         string
	BillingMode       string
	Language          string
}

// normalizeCustomerExportFilters 冻结提交时的规范化范围。使用记录导出可跨月
// 但不超过 31 天；账单类固定自然月（Asia/Shanghai），左右边界按左闭右开。
// customerExportLanguages 是导出产物支持的展示语言；语言作为冻结条件的一
// 部分（7.1），不同语言的同范围申请是不同任务，产物表头按该语言固化。
var customerExportLanguages = map[string]bool{
	"en": true, "zh": true, "zh-TW": true, "fr": true, "ru": true, "ja": true, "vi": true,
}

func normalizeCustomerExportLanguage(language string) string {
	trimmed := strings.ToLower(strings.TrimSpace(language))
	switch trimmed {
	case "zhcn":
		return "zh"
	case "zhtw":
		return "zh-TW"
	}
	if customerExportLanguages[trimmed] {
		return trimmed
	}
	// Accept common BCP-47 forms such as zh-CN / zh_TW / en-US by their
	// primary subtag when the region variant is not separately supported.
	if idx := strings.IndexAny(trimmed, "-_"); idx > 0 {
		base := trimmed[:idx]
		if base == "zh" {
			// zh-CN/zh-SG map to zh; zh-HK/zh-MO/zh-TW map to zh-TW.
			region := strings.ToLower(trimmed[idx+1:])
			if region == "tw" || region == "hk" || region == "mo" || region == "hant" {
				return "zh-TW"
			}
			return "zh"
		}
		if customerExportLanguages[base] {
			return base
		}
	}
	return "en"
}

func normalizeCustomerExportFilters(jobType string, request CustomerExportRequest) (model.CustomerExportFilters, error) {
	filters := model.CustomerExportFilters{
		FieldVersion: customerExportFieldVersion,
		QuotaPerUnit: common.QuotaPerUnit, Currency: operation_setting.GetQuotaDisplayType(),
		CurrencyRate:      operation_setting.GetUsdToCurrencyRate(operation_setting.USDExchangeRate),
		StartTimestamp:    request.StartTimestamp,
		EndTimestamp:      request.EndTimestamp,
		TokenId:           request.TokenId,
		ChannelId:         request.ChannelId,
		TokenName:         request.TokenName,
		Group:             request.Group,
		RequestId:         request.RequestId,
		UpstreamRequestId: request.UpstreamRequestId,
		Username:          request.Username,
		ModelName:         strings.TrimSpace(request.ModelName),
		BillingMode:       strings.TrimSpace(request.BillingMode),
		Timezone:          "Asia/Shanghai",
		Language:          normalizeCustomerExportLanguage(request.Language),
	}
	if filters.QuotaPerUnit <= 0 || math.IsNaN(filters.QuotaPerUnit) || math.IsInf(filters.QuotaPerUnit, 0) || filters.CurrencyRate <= 0 || math.IsNaN(filters.CurrencyRate) || math.IsInf(filters.CurrencyRate, 0) {
		return filters, errors.New("invalid export currency settings")
	}
	if jobType == model.CustomerExportJobTypeStatementSummary {
		filters.TokenId = nil
		filters.ChannelId = nil
		filters.TokenName, filters.Group, filters.RequestId, filters.UpstreamRequestId, filters.Username = "", "", "", "", ""
		filters.ModelName = ""
		filters.BillingMode = ""
	}
	if (filters.TokenId != nil && *filters.TokenId < 0) || (filters.ChannelId != nil && *filters.ChannelId < 0) {
		return filters, errors.New("invalid export identity filter")
	}
	for _, value := range []string{filters.TokenName, filters.Group, filters.RequestId, filters.UpstreamRequestId, filters.Username} {
		if len(value) > 255 {
			return filters, errors.New("invalid export text filter")
		}
	}
	if filters.StartTimestamp <= 0 || filters.EndTimestamp <= filters.StartTimestamp {
		return filters, errors.New("invalid export time range")
	}
	if jobType != model.CustomerExportJobTypeStatementSummary {
		if len(request.LogTypes) > 7 {
			return filters, errors.New("too many log type filters")
		}
		if len(request.LogTypes) > 0 {
			for _, logType := range request.LogTypes {
				switch logType {
				case model.LogTypeConsume, model.LogTypeRefund, model.LogTypeError, model.LogTypeTopup, model.LogTypeSystem, model.LogTypeManage, model.LogTypeLogin:
				default:
					return filters, errors.New("invalid log type filter")
				}
			}
			seen := make(map[int]bool, len(request.LogTypes))
			for _, logType := range request.LogTypes {
				if !seen[logType] {
					filters.LogTypes = append(filters.LogTypes, logType)
					seen[logType] = true
				}
			}
		}
	}
	switch jobType {
	case model.CustomerExportJobTypeUsageLogs:
		if filters.EndTimestamp-filters.StartTimestamp > int64(31*24*time.Hour/time.Second) {
			return filters, errors.New("usage log export range cannot exceed 31 days")
		}
	case model.CustomerExportJobTypeStatementSummary, model.CustomerExportJobTypeStatementDetails:
		period := billingExportTimezone()
		start := time.Unix(filters.StartTimestamp, 0).In(period)
		nextMonth := time.Date(start.Year(), start.Month()+1, 1, 0, 0, 0, 0, period)
		if filters.StartTimestamp != time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, period).Unix() ||
			filters.EndTimestamp != nextMonth.Unix() {
			return filters, errors.New("billing export must cover one natural month in Asia/Shanghai")
		}

	default:
		return filters, errors.New("invalid export job type")
	}
	if filters.BillingMode != "" && filters.BillingMode != model.BillingReconciliationModeToken &&
		filters.BillingMode != model.BillingReconciliationModePerCall &&
		filters.BillingMode != model.BillingReconciliationModePerSecond &&
		filters.BillingMode != model.BillingReconciliationModeUnknown {
		return filters, errors.New("invalid billing mode filter")
	}
	if filters.ModelName != "" && len(filters.ModelName) > 255 {
		return filters, errors.New("invalid model_name filter")
	}
	return filters, nil
}

// runCustomerExportScheduler 是 SystemTask 处理器主循环：先做小预算恢复与
// 清理，然后按预算逐个认领并执行排队任务；预算结束后若仍有排队任务则重新
// 唤醒自己，不允许永不退出的循环占据调度设施。
func runCustomerExportScheduler(ctx context.Context, runnerID string) bool {
	executor := "customer-export-" + runnerID
	if _, err := model.RecoverInterruptedCustomerExportJobs(common.GetTimestamp()); err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("customer export recovery failed: %v", err))
	}
	maintenanceCtx, stopMaintenance := context.WithTimeout(ctx, 30*time.Second)
	cleanupCustomerExportTemporaryFiles(maintenanceCtx)
	customerExportCleanupPass(maintenanceCtx)
	stopMaintenance()
	deadline := time.Now().Add(customerExportRunSlice)
	var pressure customerExportPressureTracker
	for ctx.Err() == nil && time.Now().Before(deadline) {
		pressure.step(model.CustomerExportLogDatabasePressure())
		if pressure.degraded {
			// 压力窗口内不领取新任务；排队任务由末尾重新唤醒在冷却后再处理。
			logger.LogWarn(ctx, "customer export scheduler paused by database pressure")
			return false
		}
		job, err := model.ClaimNextQueuedCustomerExportJob(executor, customerExportLeaseUntil(), int64(customerExportQueueWaitBudget.Seconds()))
		if err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("customer export claim failed: %v", err))
			break
		}
		if job == nil {
			break
		}
		runCustomerExportJob(ctx, job, executor, &pressure)
	}
	return ctx.Err() == nil
}

func customerExportLeaseUntil() int64 {
	return common.GetTimestamp() + int64(systemTaskLockTTL.Seconds())*2
}

// customerExportCleanupPass 是每次调度运行的小预算维护：应用层到期失效、
// 有界数量的物理删除与任务记录保留期清理。
func customerExportCleanupPass(ctx context.Context) {
	ctx, stop := context.WithTimeout(ctx, 30*time.Second)
	defer stop()
	now := common.GetTimestamp()
	_, err := model.ExpireCustomerExportArtifacts(now)
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("customer export expire pass failed: %v", err))
		return
	}
	store, storeErr := currentExportObjectStore()
	if storeErr != nil {
		return
	}
	pending, err := model.FindExpiredCustomerExportArtifacts(customerExportCleanupLimitPerRun)
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("customer export cleanup list failed: %v", err))
		return
	}
	for _, job := range pending {
		if ctx.Err() != nil {
			return
		}
		if err := model.RecordCustomerExportCleanupAttempt(ctx, job.JobID, now); err != nil {
			return
		}
		artifact := job.DecodeArtifact()
		if artifact == nil {
			logger.LogWarn(ctx, "customer export artifact metadata is unreadable; retaining reference")
			continue
		}
		if artifact.StoreIdentity == "" || artifact.StoreIdentity != store.ExportIdentity() {
			continue
		}
		cleaned := true
		for _, file := range artifact.Files {
			if ctx.Err() != nil {
				return
			}
			deleteCtx, cancel := context.WithTimeout(ctx, time.Minute)
			if err := store.ExportDeleteObject(deleteCtx, file.ObjectKey); err != nil {
				cleaned = false
			}
			cancel()
		}
		if cleaned {
			if err := model.DeleteCustomerExportArtifactRecord(job.JobID); err != nil {
				logger.LogWarn(ctx, fmt.Sprintf("customer export cleanup record failed: job=%s err=%v", job.JobID, err))
			}
		}
	}
	if _, err := model.CleanupCustomerExportJobRecords(now, int64(customerExportRecordRetention.Seconds())); err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("customer export record cleanup failed: %v", err))
	}
}

// runCustomerExportJob 执行单个任务：执行前再次校验存储能力；失败路径删除
// 未发布产物并进入明确终态，不发布部分文件。
func runCustomerExportJob(ctx context.Context, job *model.CustomerExportJob, executor string, pressure *customerExportPressureTracker) {
	ctx = model.WithBillingStatementRefundReferenceCache(ctx)
	if err := model.CheckCustomerExportExecution(ctx, job.JobID, executor, common.GetTimestamp()); err != nil {
		status := model.CustomerExportJobStatusFailed
		if errors.Is(err, model.ErrCustomerExportCancelled) {
			status = model.CustomerExportJobStatusCancelled
		}
		_ = model.FinishCustomerExportJob(job.JobID, executor, status, "authorization_or_lease", "export no longer authorized or active", nil)
		return
	}
	ctx, stop := monitorCustomerExport(ctx, job, executor)
	filters, err := job.DecodeFilters()
	if err == nil && filters.FieldVersion != customerExportFieldVersion {
		err = errors.New("export format changed; submit a new export")
	}
	var sourceVersion string
	if err == nil {
		sourceVersion, err = model.CustomerStatementExportSourceVersion(ctx, job.TargetUserId, job.JobType, filters)
	}
	var artifact *model.CustomerExportArtifact
	workDir := filepath.Join(customerExportTemporaryRoot(), job.JobID)
	defer os.RemoveAll(workDir)
	if err == nil {
		switch job.JobType {
		case model.CustomerExportJobTypeUpstreamDetails:
			artifact, err = executeUpstreamExport(ctx, job, filters, workDir, pressure)
		case model.CustomerExportJobTypeStatementVersion:
			err = runBillingStatementVersionGeneration(ctx, job, pressure)
		case model.CustomerExportJobTypeStatementSummary:
			artifact, err = executeCustomerExportSummary(ctx, job, filters, workDir)
		case model.CustomerExportJobTypeStatementDetails, model.CustomerExportJobTypeUsageLogs:
			artifact, err = executeCustomerExportLogScan(ctx, job, filters, workDir, executor, pressure)
		default:
			err = errors.New("unknown export job type")
		}
	}
	if err == nil && artifact != nil && sourceVersion != "" {
		currentVersion, revisionErr := model.CustomerStatementExportSourceVersion(ctx, job.TargetUserId, job.JobType, filters)
		if revisionErr == nil && currentVersion == sourceVersion {
			artifact.SourceVersion = sourceVersion
			// Empty exports have no uploaded object but still have a delivery identity.
			if artifact.StoreIdentity == "" {
				if store, storeErr := currentExportObjectStore(); storeErr == nil {
					artifact.StoreIdentity = store.ExportIdentity()
				}
			}
		}
	}
	if cause := context.Cause(ctx); cause != nil {
		err = cause
	}
	if err == nil {
		err = model.ValidateBillingStatementRefundReferenceCache(ctx)
	}
	if err == nil {
		err = model.CheckCustomerExportExecution(ctx, job.JobID, executor, common.GetTimestamp())
	}
	stop()
	status, code := model.CustomerExportJobStatusSucceeded, ""
	if err != nil {
		status, code = model.CustomerExportJobStatusFailed, "generation_failed"
		artifact = nil // Pending references stay durable for cleanup.
		if errors.Is(err, model.ErrCustomerExportCancelled) || errors.Is(err, errCustomerExportCancelled) {
			status, code = model.CustomerExportJobStatusCancelled, "cancelled"
		} else if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, errCustomerExportBudgetExceeded) {
			code = "budget_exceeded"
		}
	}
	if finishErr := model.FinishCustomerExportJob(job.JobID, executor, status, code, "", artifact); finishErr != nil {
		// A cancellation may race the final gate. Never publish after it.
		if cancelled, readErr := model.IsCustomerExportCancelRequested(job.JobID); readErr == nil && cancelled {
			_ = model.FinishCustomerExportJob(job.JobID, executor, model.CustomerExportJobStatusCancelled, "cancelled", "", nil)
		}
		if job.JobType == model.CustomerExportJobTypeStatementVersion {
			_ = model.FinishCustomerExportJob(job.JobID, executor, model.CustomerExportJobStatusFailed, "publication_rejected", "", nil)
		}
		logger.LogWarn(context.Background(), fmt.Sprintf("customer export finish rejected: job=%s", job.JobID))
	}
}

var (
	errCustomerExportCancelled      = errors.New("customer export cancelled")
	errCustomerExportBudgetExceeded = errors.New("customer export budget exceeded")
	errCustomerExportInterrupted    = errors.New("customer export executor interrupted")
)

// executeCustomerExportLogScan 按冻结条件分批 keyset 扫描并写 CSV；批次间
// 检查取消与租约，节流时不持有数据库连接或事务（7.3.1/7.3.2）。
func executeCustomerExportLogScan(ctx context.Context, job *model.CustomerExportJob, filters model.CustomerExportFilters, workDir string, executor string, pressure *customerExportPressureTracker) (*model.CustomerExportArtifact, error) {
	ctx = model.WithBillingStatementRefundReferenceCache(ctx)
	if pressure == nil {
		pressure = &customerExportPressureTracker{}
	}
	writer, err := newCustomerExportCsvWriter(workDir, "export", customerExportCsvShardBytes)
	if err != nil {
		return nil, err
	}
	defer writer.Cleanup()
	scope := customerExportScopeColumns{
		Language: filters.Language,
		JobID:    job.JobID, ExportType: job.JobType,
		PeriodStart: filters.StartTimestamp, PeriodEnd: filters.EndTimestamp,
		QuotaPerUnit: filters.QuotaPerUnit, Currency: filters.Currency, CurrencyRate: filters.CurrencyRate,
		Timezone: filters.Timezone, CustomerId: job.TargetUserId, GeneratedAt: common.GetTimestamp(),
	}
	if username, err := model.GetUsernameById(job.TargetUserId, false); err == nil && strings.TrimSpace(username) != "" {
		scope.CustomerName = username
	} else {
		scope.CustomerName = fmt.Sprintf("user-%d", job.TargetUserId)
	}

	statementScope := job.JobType == model.CustomerExportJobTypeStatementDetails
	if statementScope {
		writer.useStatementDetails(filters.Language)
		scope.PeriodEnd = filters.EndTimestamp - 1
	}
	batchParams := model.CustomerExportBatchParams{
		ModelName:      filters.ModelName,
		UserId:         job.TargetUserId,
		StartTimestamp: filters.StartTimestamp,
		EndTimestamp:   filters.EndTimestamp,
		LogTypes:       filters.LogTypes,
		TokenId:        filters.TokenId,
		ChannelId:      filters.ChannelId, TokenName: filters.TokenName, Group: filters.Group,
		RequestId: filters.RequestId, UpstreamRequestId: filters.UpstreamRequestId, Username: filters.Username,
		Limit:          customerExportBatchSize,
		StatementScope: statementScope,
	}
	if statementScope && len(batchParams.LogTypes) == 0 {
		batchParams.LogTypes = []int{model.LogTypeConsume, model.LogTypeRefund}
	}
	upperCtx, stopUpper := context.WithTimeout(ctx, customerExportBatchTimeout)
	upper, upperErr := model.CustomerExportLogUpperBound(upperCtx)
	stopUpper()
	if upperErr != nil {
		return nil, upperErr
	}
	if upper == 0 {
		return &model.CustomerExportArtifact{Files: []model.CustomerExportArtifactFile{}, GeneratedAt: scope.GeneratedAt}, nil
	}
	batchParams.UpperId = upper
	progress := model.CustomerExportProgress{}
	var lastProgressWrite time.Time
	deadline := time.Now().Add(customerExportJobBudget)
	cursor := int64(0)
	for {
		pressure.step(model.CustomerExportLogDatabasePressure())
		if err := pressure.publishWaiting(ctx, job.JobID, executor, &progress); err != nil {
			return nil, err
		}
		if pressure.degraded {
			// 批次间退让：不持有数据库连接地等待一个冷却窗口，预算与取消
			// 仍然生效（7.3：核心请求延迟越过预算时不启动下一批查询）。
			select {
			case <-time.After(customerExportPressurePause):
			case <-ctx.Done():
				return nil, errCustomerExportInterrupted
			}
			continue
		}
		stop, err := customerExportBatchGate(ctx, job.JobID, executor, &deadline, &progress, &lastProgressWrite)
		if err != nil {
			return nil, err
		}
		if stop {
			return nil, errCustomerExportCancelled
		}
		batchStart := time.Now()
		batchCtx, cancelBatch := context.WithTimeout(ctx, customerExportBatchTimeout)
		rows, scanErr := model.NextCustomerExportLogBatch(batchCtx, withCustomerExportCursor(batchParams, cursor))
		var exportRows []model.CustomerExportRow
		var buildErr error
		if scanErr == nil && len(rows) > 0 {
			// 7.3.13: the batch context also bounds the serial Task-fact
			// classification so auxiliary queries stay inside the budget.
			modelFilter := filters.ModelName
			if !statementScope {
				modelFilter = "" // Native model filtering has already run in SQL.
			}
			exportRows, buildErr = model.BuildCustomerExportRows(batchCtx, rows, modelFilter, filters.BillingMode, func(log *model.Log, row *model.CustomerExportRow) {
				attachCustomerExportBillingExplanation(log, row, filters.QuotaPerUnit)
			})
		}
		cancelBatch()
		if scanErr != nil {
			if ctx.Err() != nil || errors.Is(scanErr, context.DeadlineExceeded) {
				return nil, fmt.Errorf("%w: batch query timed out", errCustomerExportBudgetExceeded)
			}
			return nil, scanErr
		}
		if buildErr != nil {
			if ctx.Err() != nil || errors.Is(buildErr, context.DeadlineExceeded) {
				return nil, fmt.Errorf("%w: batch classification timed out", errCustomerExportBudgetExceeded)
			}
			return nil, buildErr
		}
		if len(rows) == 0 {
			break
		}
		for _, exportRow := range exportRows {
			if err := writer.AppendRow(exportRow, scope); err != nil {
				return nil, err
			}
		}
		progress.Scanned += int64(len(rows))
		progress.Matched += int64(len(exportRows))
		progress.Written = progress.Matched
		progress.Files = int64(writer.shardIndex) + 1
		if len(rows) < batchParams.Limit {
			break
		}
		cursor = int64(rows[len(rows)-1].Id)
		if sleep := customerExportRateThrottle(len(rows), time.Since(batchStart)); sleep > 0 {
			select {
			case <-time.After(sleep):
			case <-ctx.Done():
				return nil, errCustomerExportInterrupted
			}
		}
	}
	files, paths, lines, _, err := writer.Finish()
	if err != nil {
		if errors.Is(err, errCustomerExportNoFiles) {
			// 空结果不是失败：发布零文件清单，界面据此提示无可导出记录。
			return &model.CustomerExportArtifact{Files: []model.CustomerExportArtifactFile{}, GeneratedAt: common.GetTimestamp()}, nil
		}
		return nil, err
	}
	artifact, err := uploadCustomerExportArtifact(ctx, job.JobID, files, paths, lines)
	if err == nil {
		artifact.GeneratedAt = scope.GeneratedAt
	}
	return artifact, err
}

func withCustomerExportCursor(params model.CustomerExportBatchParams, cursor int64) model.CustomerExportBatchParams {
	params.CursorId = cursor
	return params
}

// customerExportBatchGate 在每个批次前检查中断、预算、取消与租约，并按节流
// 频率持久化进度。返回 stop=true 表示用户已取消（任务进入取消中状态）。
func customerExportBatchGate(ctx context.Context, jobID string, executor string, deadline *time.Time, progress *model.CustomerExportProgress, lastProgressWrite *time.Time) (bool, error) {
	if ctx.Err() != nil {
		return false, errCustomerExportInterrupted
	}
	if time.Now().After(*deadline) {
		return false, fmt.Errorf("%w: job budget exhausted", errCustomerExportBudgetExceeded)
	}
	if err := model.CheckCustomerExportExecution(ctx, jobID, executor, common.GetTimestamp()); err != nil {
		if errors.Is(err, model.ErrCustomerExportCancelled) {
			return true, nil
		}
		return false, err
	}
	if time.Since(*lastProgressWrite) >= customerExportProgressInterval {
		*lastProgressWrite = time.Now()
		if err := model.UpdateCustomerExportProgress(ctx, jobID, executor, *progress); err != nil && !errors.Is(err, model.ErrCustomerExportStateConflict) {
			logger.LogWarn(ctx, fmt.Sprintf("customer export progress write failed: job=%s err=%v", jobID, err))
		}
	}
	return false, nil
}

const (
	// 7.3 数据库压力退让的保守固定阈值：目标部署给出基线前不提速，也不把
	// 退让当成永久暂停；监控不可用时的保守行为按计划执行。
	customerExportPressureWaitSeconds    = 0.25
	customerExportPressureRecoverWindows = 3
	customerExportPressurePause          = 5 * time.Second
)

// customerExportPressureTracker converts log-database pool wait samples into
// a degraded/healthy decision. Degraded means: stop claiming new jobs and
// pause between batches without holding a database connection.
type customerExportPressureTracker struct {
	initialized     bool
	lastWaitCount   int64
	lastWaitSeconds float64
	degraded        bool
	healthyWindows  int
}

func (t *customerExportPressureTracker) step(sampleWaitCount int64, sampleWaitSeconds float64) {
	if !t.initialized {
		t.initialized = true
		t.lastWaitCount = sampleWaitCount
		t.lastWaitSeconds = sampleWaitSeconds
		return
	}
	// DBStats 的 WaitCount/WaitDuration 都是进程累计值：均值必须用窗口增量。
	newWaits := sampleWaitCount - t.lastWaitCount
	if newWaits < 0 {
		newWaits = 0
	}
	newWaitSeconds := sampleWaitSeconds - t.lastWaitSeconds
	if newWaitSeconds < 0 {
		newWaitSeconds = 0
	}
	t.lastWaitCount = sampleWaitCount
	t.lastWaitSeconds = sampleWaitSeconds
	avgWait := 0.0
	if newWaits > 0 {
		avgWait = newWaitSeconds / float64(newWaits)
	}
	if newWaits > 0 && avgWait > customerExportPressureWaitSeconds {
		t.degraded = true
		t.healthyWindows = 0
		return
	}
	if t.degraded {
		t.healthyWindows++
		if t.healthyWindows >= customerExportPressureRecoverWindows {
			t.degraded = false
			t.healthyWindows = 0
		}
	}
}

// customerExportRateThrottle 把读取速率约束在预算内；批次耗时已超过配额时
// 不额外睡眠。
func customerExportRateThrottle(rows int, elapsed time.Duration) time.Duration {
	if rows <= 0 || customerExportMaxRatePerSecond <= 0 {
		return 0
	}
	minDuration := time.Duration(float64(rows) / float64(customerExportMaxRatePerSecond) * float64(time.Second))
	if elapsed >= minDuration {
		return 0
	}
	return minDuration - elapsed
}

// executeCustomerExportSummary 从既有账单聚合输出 API Key × 模型叶子行。
// 空月份发布零文件清单，不伪造零金额账单。
func executeCustomerExportSummary(ctx context.Context, job *model.CustomerExportJob, filters model.CustomerExportFilters, workDir string) (*model.CustomerExportArtifact, error) {
	summaryCtx, cancelSummary := context.WithTimeout(ctx, customerExportJobBudget)
	defer cancelSummary()
	// Cancellation stays responsive even though the aggregation is one
	// request-scoped query: the read is bounded by the same 10-minute job
	// budget as the scan path and fails honestly when it cannot finish.
	statement, err := model.GetBillingCustomerStatement(
		summaryCtx, job.TargetUserId, filters.StartTimestamp, filters.EndTimestamp-1, "api_key",
		0, "", "",
		model.BillingStatementReadPolicy{
			BatchTimeout: customerExportBatchTimeout, MaxGroups: 10000,
			BeforeBatch: customerExportSummaryGate(&customerExportPressureTracker{}, job, nil),
		},
	)
	if err != nil {
		if summaryCtx.Err() != nil {
			return nil, fmt.Errorf("%w: month aggregation timed out", errCustomerExportBudgetExceeded)
		}
		return nil, err
	}
	username, err := model.GetUsernameById(job.TargetUserId, false)
	if err != nil || strings.TrimSpace(username) == "" {
		username = fmt.Sprintf("user-%d", job.TargetUserId)
	}
	scope := customerExportScopeColumns{
		Language: filters.Language,
		JobID:    job.JobID, ExportType: job.JobType,
		PeriodStart: filters.StartTimestamp, PeriodEnd: filters.EndTimestamp - 1,
		QuotaPerUnit: filters.QuotaPerUnit, Currency: filters.Currency, CurrencyRate: filters.CurrencyRate,
		Timezone: filters.Timezone, CustomerId: job.TargetUserId, GeneratedAt: common.GetTimestamp(), CustomerName: username,
	}
	files, paths, lines, err := writeCustomerExportSummaryCsv(workDir, scope, filters.Language, statement)
	if err != nil {
		return nil, err
	}
	if lines == 0 {
		return &model.CustomerExportArtifact{Files: []model.CustomerExportArtifactFile{}, GeneratedAt: common.GetTimestamp()}, nil
	}
	artifact, err := uploadCustomerExportArtifact(ctx, job.JobID, files, paths, lines)
	if err == nil {
		artifact.GeneratedAt = scope.GeneratedAt
	}
	return artifact, err
}

func customerExportSummaryHeader(language string) []string {
	header := make([]string, 0, len(customerExportSummaryColumnOrder))
	for _, key := range customerExportSummaryColumnOrder {
		header = append(header, customerExportSummaryHeaderLabel(language, key))
	}
	return header
}

// Only model leaf rows are exported, so customers can sum amount columns
// directly without counting parent totals or discount combinations again.
func writeCustomerExportSummaryCsv(workDir string, scope customerExportScopeColumns, language string, statement model.BillingCustomerStatement) ([]model.CustomerExportArtifactFile, []string, int64, error) {
	if len(statement.Groups) == 0 {
		return nil, nil, 0, nil
	}
	if err := os.MkdirAll(workDir, 0o700); err != nil {
		return nil, nil, 0, err
	}
	file, err := os.CreateTemp(workDir, "summary-*")
	if err != nil {
		return nil, nil, 0, err
	}
	path := file.Name()
	defer func() { _ = file.Close() }()
	buffered := bufio.NewWriter(file)
	if _, err := buffered.WriteString(customerExportCsvBOM); err != nil {
		return nil, nil, 0, err
	}
	writer := csv.NewWriter(buffered)
	if err := writer.Write(customerExportSummaryHeader(language)); err != nil {
		return nil, nil, 0, err
	}
	lineCount := int64(0)
	writeRow := func(row map[string]string) {
		record := make([]string, 0, len(customerExportSummaryColumnOrder))
		for _, key := range customerExportSummaryColumnOrder {
			record = append(record, row[key])
		}
		if writer.Write(record) == nil {
			lineCount++
		}
	}
	baseRow := func() map[string]string {
		row := make(map[string]string, len(customerExportSummaryColumnOrder))
		row["generated_at"] = formatExportTimestamp(scope.GeneratedAt)
		row["currency"] = scope.Currency
		row["period_start"] = formatExportTimestamp(scope.PeriodStart)
		row["period_end"] = formatExportTimestamp(scope.PeriodEnd)
		row["timezone"] = scope.Timezone
		row["customer_username"] = exportCsvCellGuard(scope.CustomerName)
		return row
	}
	writeAmounts := func(row map[string]string, usage model.BillingReconciliationUsage, original *int64, discount *int64) {
		row["requests"] = strconv.FormatInt(usage.Requests, 10)
		row["input_tokens"] = strconv.FormatInt(usage.InputTokens, 10)
		row["cache_read_tokens"] = strconv.FormatInt(usage.CacheReadTokens, 10)
		row["cache_write_tokens"] = strconv.FormatInt(usage.CacheWriteTokens, 10)
		row["output_tokens"] = strconv.FormatInt(usage.OutputTokens, 10)
		row["billable_calls"] = strconv.FormatInt(usage.BillableCalls, 10)
		row["refunded_calls"] = strconv.FormatInt(usage.RefundedCalls, 10)
		row["original_quota"] = exportQuotaAmount(original)
		row["discount_quota"] = exportQuotaAmount(discount)
		row["gross_quota"] = strconv.FormatInt(usage.GrossQuota, 10)
		row["refund_quota"] = strconv.FormatInt(usage.RefundQuota, 10)
		row["net_quota"] = strconv.FormatInt(usage.NetQuota, 10)
		for _, key := range []string{"original", "discount", "gross", "refund", "net"} {
			row[key+"_amount"] = exportCurrencyAmount(row[key+"_quota"], scope)
		}
	}
	qualityLabel := func(quality *model.BillingReconciliationDataQuality) string {
		if quality == nil || quality.Status == "complete" {
			return "complete"
		}
		return "partial"
	}

	for _, group := range statement.Groups {
		for _, modelSummary := range group.Models {
			modelRow := baseRow()
			modelRow["api_key_id"] = strconv.FormatInt(group.Id, 10)
			modelRow["api_key_name"] = exportCsvCellGuard(group.Name)
			modelRow["model"] = exportCsvCellGuard(modelSummary.ModelName)
			modelRow["billing_mode"] = customerStatementExportValue(language, "billing_mode", modelSummary.BillingMode)
			writeAmounts(modelRow, modelSummary.Usage, modelSummary.OriginalQuota, model.BillingStatementEstimatedSavings(modelSummary.OriginalQuota, modelSummary.Usage.NetQuota))
			modelRow["estimate_reasons"] = customerExportEstimateReasons(language, modelSummary.EstimateReasons)
			modelRow["data_quality"] = customerStatementExportValue(language, "quality_status", qualityLabel(modelSummary.DataQuality))
			if modelSummary.DataQuality != nil && modelSummary.DataQuality.InputTokensUnavailableRequests > 0 {
				modelRow["input_tokens"] = ""
			}
			writeRow(modelRow)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, nil, 0, err
	}
	if err := buffered.Flush(); err != nil {
		return nil, nil, 0, err
	}
	if lineCount == 0 {
		return nil, nil, 0, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, nil, 0, err
	}
	digest, err := streamFileSha256(path)
	if err != nil {
		return nil, nil, 0, err
	}
	return []model.CustomerExportArtifactFile{{
		ObjectKey: "", FileName: "statement-summary.csv", SizeBytes: info.Size(),
		LineCount: lineCount, Sha256: digest,
	}}, []string{path}, lineCount, nil
}

func exportQuotaAmount(value *int64) string {
	if value == nil {
		return ""
	}
	return strconv.FormatInt(*value, 10)
}

// uploadCustomerExportArtifact 把全部分片流式上传到私有导出命名空间；全部
// 成功才发布完整清单；失败保留 staged 引用交由定期清理，不发布部分文件。
func uploadCustomerExportArtifact(ctx context.Context, jobID string, files []model.CustomerExportArtifactFile, paths []string, lines int64) (*model.CustomerExportArtifact, error) {
	store, err := currentExportObjectStore()
	if err != nil {
		return nil, err
	}
	now := common.GetTimestamp()
	artifact := &model.CustomerExportArtifact{
		StoreIdentity: store.ExportIdentity(),
		Files:         make([]model.CustomerExportArtifactFile, 0, len(files)),
		LineCount:     lines,
		GeneratedAt:   now,
		ExpiresAt:     now + int64(customerExportFileRetention.Seconds()),
	}
	for i := range files {
		files[i].ObjectKey = CustomerExportObjectNamespace + "/" + jobID + "/" + files[i].FileName
		artifact.SizeBytes += files[i].SizeBytes
	}
	artifact.Files = files
	if artifact.SizeBytes > customerExportTotalBytes {
		return nil, errCustomerExportBudgetExceeded
	}
	if err := model.StageCustomerExportArtifact(ctx, jobID, artifact); err != nil {
		return nil, err
	}
	for i, file := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := store.ExportPutFile(ctx, file.ObjectKey, "text/csv; charset=utf-8", paths[i], file.SizeBytes); err != nil {
			return nil, err
		}
		if err := os.Remove(paths[i]); err != nil {
			return nil, err
		}
	}
	return artifact, nil
}

// CSV money is a display conversion of settled quota, never a repricing.
func exportCurrencyAmount(raw string, scope customerExportScopeColumns) string {
	if raw == "" || scope.QuotaPerUnit <= 0 || scope.CurrencyRate <= 0 {
		return ""
	}
	value, err := decimal.NewFromString(raw)
	if err != nil {
		return ""
	}
	if scope.Currency == "TOKENS" {
		return value.String()
	}
	return value.Div(decimal.NewFromFloat(scope.QuotaPerUnit)).Mul(decimal.NewFromFloat(scope.CurrencyRate)).StringFixed(8)
}
