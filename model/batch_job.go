package model

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	hosttypes "github.com/QuantumNous/new-api/types"
	"gorm.io/gorm"
)

var ErrBatchJobNotFound = errors.New("batch job not found")

// Batch job delivery states: result/error file collection is tracked
// independently of upstream execution and of money.
const (
	BatchDeliveryPending    = "pending"
	BatchDeliveryProcessing = "processing"
	BatchDeliveryReady      = "ready"
	BatchDeliveryFailed     = "failed"
)

// Batch job settlement states.
const (
	BatchSettlePending = "pending"
	BatchSettleSettled = "settled"
	BatchSettleFailed  = "failed"
	BatchSettleDebt    = "debt"
)

// BatchJob is the durable execution/delivery fact of one batch job. The Task
// row (platform azure_batch) remains the single business and funds fact;
// this table holds the frozen south-bound snapshot, upstream observation and
// file-delivery state that never enters customer responses.
type BatchJob struct {
	Id              string `json:"id" gorm:"type:varchar(64);primaryKey"`
	TaskRowId       int64  `json:"-" gorm:"index"`
	UserId          int    `json:"-" gorm:"index"`
	AppID           int    `json:"-" gorm:"index"`
	TokenId         int    `json:"-"`
	ChannelId       int    `json:"-"`
	PublicModel     string `json:"model" gorm:"type:varchar(255)"`
	Deployment      string `json:"-" gorm:"type:varchar(255)"`
	UpstreamBatchId string `json:"-" gorm:"type:varchar(191);index"`

	InputFileId      string `json:"-"`
	Endpoint         string `json:"-"`
	CompletionWindow string `json:"-" gorm:"type:varchar(16)"`
	Metadata         string `json:"-" gorm:"type:text"`

	UpstreamInputFileId string `json:"-" gorm:"type:varchar(191)"`
	UpstreamStatus      string `json:"-" gorm:"type:varchar(32);index"`
	PublicStatus        string `json:"status" gorm:"type:varchar(32);index"`
	DeliveryState       string `json:"-" gorm:"type:varchar(20);index"`
	SettleState         string `json:"-" gorm:"type:varchar(20);index"`

	OutputObjectKey string `json:"-" gorm:"type:varchar(512)"`
	ErrorObjectKey  string `json:"-" gorm:"type:varchar(512)"`
	OutputFileId    string `json:"-" gorm:"type:varchar(64)"`
	ErrorFileId     string `json:"-" gorm:"type:varchar(64)"`

	LineCount      int64 `json:"-"`
	CountTotal     int64 `json:"-"`
	CountCompleted int64 `json:"-"`
	CountFailed    int64 `json:"-"`
	CountExpired   int64 `json:"-"`
	CountErrored   int64 `json:"-"`
	CountCancelled int64 `json:"-"`

	UsageInput  int64 `json:"-"`
	UsageCached int64 `json:"-"`
	UsageOutput int64 `json:"-"`
	UsageTotal  int64 `json:"-"`

	EstimateQuota int  `json:"-"`
	TargetQuota   *int `json:"-"`
	CurrencyQuota int  `json:"-"`

	FrozenSnapshot BillingText `json:"-"`
	SanitizeError  string      `json:"-" gorm:"type:text"`

	PollAfter         int64 `json:"-" gorm:"bigint;index"`
	PollVersion       int64 `json:"-"`
	PollFailures      int   `json:"-"`
	ExpiresAt         int64 `json:"-"`
	CancelRequestedAt int64 `json:"-"`
	CancelledAt       int64 `json:"-"`
	FailedAt          int64 `json:"-"`
	CompletedAt       int64 `json:"-"`
	CreatedAt         int64 `json:"created_at" gorm:"bigint;index"`
	UpdatedAt         int64 `json:"-" gorm:"bigint"`
}

// BatchFrozenSnapshot is the frozen pricing and execution context captured at
// job creation. Settlement reads only this snapshot plus trusted upstream
// usage; later config, channel or price changes cannot re-interpret the job.
type BatchConnection struct {
	BaseURL    string `json:"base_url"`
	Key        string `json:"key"`
	APIVersion string `json:"api_version"`
}

type BatchFrozenSnapshot struct {
	InitialCalculations billingexpr.CalculationArchive `json:"initial_calculations,omitempty"`

	AdapterVersion   string                         `json:"adapter_version"`
	LineParams       map[string]json.RawMessage     `json:"line_params,omitempty"`
	Connection       BatchConnection                `json:"connection"`
	Expr             string                         `json:"expr"`
	ExprHash         string                         `json:"expr_hash"`
	PricingTime      int64                          `json:"pricing_time"` // unix UTC
	QuotaPerUnit     float64                        `json:"quota_per_unit"`
	GroupRatio       float64                        `json:"group_ratio"`
	PublicModel      string                         `json:"public_model"`
	Deployment       string                         `json:"deployment"`
	ChannelId        int                            `json:"channel_id"`
	Endpoint         string                         `json:"endpoint"`
	CompletionWindow string                         `json:"completion_window"`
	ContractFact     *hosttypes.ContractBillingFact `json:"contract_fact,omitempty"`
	// UsdExchangeRate freezes the CNY/USD rate for expressions calling
	// usd_exchange_rate(). One rate per accepted job runs through every line
	// estimate, settlement and recovery; nil when the expression is
	// rate-free.
	UsdExchangeRate *billingexpr.ExchangeRateContext `json:"usd_exchange_rate,omitempty"`
	EstimateQuota   int                              `json:"estimate_quota"`
	LineInputs      map[string]int                   `json:"line_inputs"` // custom_id -> estimated input tokens
}

// NewBatchJobPublicId generates the north-facing batch job id.
func NewBatchJobPublicId() (string, error) {
	return newBatchJobId()
}

func newBatchJobId() (string, error) {
	key, err := common.GenerateRandomCharsKey(28)
	if err != nil {
		return "", err
	}
	return "batch_" + strings.ToLower(key), nil
}

type CreateBatchJobParams struct {
	PublicId         string
	TaskRowId        int64
	UserId           int
	AppID            int
	TokenId          int
	ChannelId        int
	PublicModel      string
	Deployment       string
	InputFileId      string
	Endpoint         string
	CompletionWindow string
	Metadata         string
	Frozen           BatchFrozenSnapshot
	EstimateQuota    int
	LineCount        int64
}

// CreatePendingBatchJob records the job row inside the caller's transaction
// (the same transaction that creates the Task and transfers the attempt
// hold). The job starts with no upstream id; the trusted upstream batch id is
// attached by CommitBatchJobUpstreamId in the same transaction.
func CreatePendingBatchJobTx(tx *gorm.DB, params CreateBatchJobParams) (*BatchJob, error) {
	id := params.PublicId
	if id == "" {
		return nil, errors.New("batch public identity is required")
	}
	frozenJSON, err := common.Marshal(params.Frozen)
	if err != nil {
		return nil, err
	}
	job := &BatchJob{
		Id: id, TaskRowId: params.TaskRowId, UserId: params.UserId, AppID: params.AppID,
		TokenId: params.TokenId, ChannelId: params.ChannelId,
		PublicModel: params.PublicModel, Deployment: params.Deployment,
		InputFileId: params.InputFileId, Endpoint: params.Endpoint,
		CompletionWindow: params.CompletionWindow, Metadata: params.Metadata,
		PublicStatus: "validating", DeliveryState: BatchDeliveryPending,
		SettleState:    BatchSettlePending,
		FrozenSnapshot: BillingText(frozenJSON),
		EstimateQuota:  params.EstimateQuota, LineCount: params.LineCount,
		CreatedAt: common.GetTimestamp(), UpdatedAt: common.GetTimestamp(),
	}
	if err := tx.Create(job).Error; err != nil {
		return nil, err
	}
	return job, nil
}

// AttachBatchJobUpstreamIdTx commits the trusted upstream batch id together
// with the caller's transaction (Task creation + attempt hold transfer). The
// conditional update keeps a restarted creator from double-attaching.
func AttachBatchJobUpstreamIdTx(tx *gorm.DB, jobId string, upstreamBatchId string, publicStatus string) error {
	updates := map[string]any{
		"upstream_batch_id": upstreamBatchId,
		"public_status":     publicStatus,
		"updated_at":        common.GetTimestamp(),
	}
	result := tx.Model(&BatchJob{}).
		Where("id = ? AND upstream_batch_id = ''", jobId).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("batch job already attached to an upstream id")
	}
	return nil
}

func GetBatchJobOwned(jobId string, userId int, appID int) (*BatchJob, error) {
	if jobId == "" || userId <= 0 || appID <= 0 {
		return nil, ErrBatchJobNotFound
	}
	var job BatchJob
	if err := DB.Where("id = ? AND user_id = ? AND app_id = ?", jobId, userId, appID).First(&job).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrBatchJobNotFound
		}
		return nil, err
	}
	return &job, nil
}

type BatchJobListPage struct {
	Items   []BatchJob `json:"items"`
	Total   int64      `json:"total"`
	HasMore bool       `json:"has_more"`
}

func ListBatchJobsOwned(userId int, appID int, limit int, after string) (*BatchJobListPage, error) {
	if userId <= 0 || appID <= 0 {
		return nil, ErrBatchJobNotFound
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var total int64
	if err := DB.Model(&BatchJob{}).Where("user_id = ? AND app_id = ?", userId, appID).Count(&total).Error; err != nil {
		return nil, err
	}
	page := &BatchJobListPage{Items: []BatchJob{}}
	query := DB.Where("user_id = ? AND app_id = ?", userId, appID)
	if after != "" {
		cursor, err := GetBatchJobOwned(after, userId, appID)
		if err != nil {
			return nil, err
		}
		query = query.Where("created_at < ? OR (created_at = ? AND id < ?)", cursor.CreatedAt, cursor.CreatedAt, cursor.Id)
	}
	err := query.
		Order("created_at DESC, id DESC").
		Limit(limit + 1).
		Find(&page.Items).Error
	if err != nil {
		return nil, err
	}
	page.Total = total
	page.HasMore = len(page.Items) > limit
	if page.HasMore {
		page.Items = page.Items[:limit]
	}
	return page, nil
}

var batchTerminalPublicStatuses = []string{"completed", "failed", "expired", "cancelled"}

// BatchJobFullyDone reports whether a job reached its terminal public status
// with settlement applied and results delivered.
func BatchJobFullyDone(job *BatchJob) bool {
	if job == nil {
		return false
	}
	terminal := false
	for _, status := range batchTerminalPublicStatuses {
		if job.PublicStatus == status {
			terminal = true
			break
		}
	}
	return terminal && job.SettleState == BatchSettleSettled && job.DeliveryState == BatchDeliveryReady
}

// GetDueBatchJobs returns jobs whose next progression step is due: a job is
// due until it is terminal with settlement applied and results delivered.
// Terminal jobs with pending settlement or delivery stay due so collection
// and settlement recover independently of the upstream observation.
func GetDueBatchJobs(now int64, limit int) ([]*BatchJob, error) {
	if limit <= 0 {
		return nil, nil
	}
	var jobs []*BatchJob
	err := DB.
		Where("upstream_batch_id <> ? AND poll_after <= ? AND NOT (public_status IN ? AND settle_state = ? AND delivery_state = ?)",
			"", now, batchTerminalPublicStatuses, BatchSettleSettled, BatchDeliveryReady).
		Order("poll_after, created_at").
		Limit(limit).
		Find(&jobs).Error
	if err != nil {
		return nil, err
	}
	return jobs, nil
}

// ClaimBatchJobPoll takes row ownership of one poll round so multiple workers
// or a stale worker cannot double-collect. The claim matches the previous
// poll_after and failure count; a job rescheduled meanwhile is skipped.
func ClaimBatchJobPoll(jobId string, previousPollAfter int64, previousFailures int, previousVersion int64, nextPollAfter int64) (bool, error) {
	result := DB.Model(&BatchJob{}).
		Where("id = ? AND poll_after = ? AND poll_failures = ? AND poll_version = ?",
			jobId, previousPollAfter, previousFailures, previousVersion).
		Updates(map[string]any{
			"poll_after":    nextPollAfter,
			"poll_failures": previousFailures + 1,
			"poll_version":  previousVersion + 1,
			"updated_at":    common.GetTimestamp(),
		})
	return result.RowsAffected == 1, result.Error
}

// SaveBatchJobObservation persists one trusted upstream observation with a
// conditional update matching the claimed poll version, so a stale worker
// cannot overwrite a newer result.
func SaveBatchJobObservation(job *BatchJob, claimedVersion int64) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&BatchJob{}).
			Where("id = ? AND poll_version = ?", job.Id, claimedVersion).
			Updates(map[string]any{
				"upstream_status":   job.UpstreamStatus,
				"public_status":     job.PublicStatus,
				"delivery_state":    job.DeliveryState,
				"settle_state":      job.SettleState,
				"count_total":       job.CountTotal,
				"count_completed":   job.CountCompleted,
				"count_failed":      job.CountFailed,
				"count_expired":     job.CountExpired,
				"count_errored":     job.CountErrored,
				"count_cancelled":   job.CountCancelled,
				"usage_input":       job.UsageInput,
				"usage_cached":      job.UsageCached,
				"usage_output":      job.UsageOutput,
				"usage_total":       job.UsageTotal,
				"output_object_key": job.OutputObjectKey,
				"error_object_key":  job.ErrorObjectKey,
				"output_file_id":    job.OutputFileId,
				"error_file_id":     job.ErrorFileId,
				"target_quota":      job.TargetQuota,
				"expires_at":        job.ExpiresAt,
				"cancelled_at":      job.CancelledAt,
				"failed_at":         job.FailedAt,
				"completed_at":      job.CompletedAt,
				"sanitize_error":    job.SanitizeError,
				"poll_failures":     0,
				"updated_at":        common.GetTimestamp(),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("batch job observation lost the poll claim")
		}
		return nil
	})
}

// BatchJobLine is one request line's execution and billing evidence. Rows are
// written once per trusted observation of that custom_id; repeats of the same
// content are no-ops, conflicting content keeps the first row and marks the
// discrepancy for audit.
type BatchJobLine struct {
	CalculationVersion int         `json:"-"`
	Calculation        BillingText `json:"-"`

	Id           int64  `json:"-" gorm:"primaryKey"`
	JobId        string `json:"-" gorm:"type:varchar(64);uniqueIndex:idx_batch_lines_job_custom,priority:1"`
	CustomId     string `json:"-" gorm:"type:varchar(255);uniqueIndex:idx_batch_lines_job_custom"`
	QuotaClamp   string `json:"-" gorm:"type:text"`
	Status       string `json:"-" gorm:"type:varchar(32)"`
	InputTokens  int64  `json:"-"`
	OutputTokens int64  `json:"-"`
	CachedTokens int64  `json:"-"`
	TotalTokens  int64  `json:"-"`
	ModelQuota   int    `json:"-"`
	FinalQuota   int    `json:"-"`
	ErrorCode    string `json:"-" gorm:"type:varchar(64)"`
	CreatedAt    int64  `json:"-" gorm:"bigint"`
}

// MarkBatchJobCancelRequested records that cancellation was accepted
// upstream; the job keeps polling until Azure reports the final state.
func MarkBatchJobCancelRequested(jobId string) error {
	now := common.GetTimestamp()
	return DB.Model(&BatchJob{}).Where("id = ? AND public_status NOT IN ?", jobId, []string{"completed", "failed", "expired", "cancelled", "finalizing"}).
		Updates(map[string]any{
			"public_status":       "cancelling",
			"cancel_requested_at": now,
			"updated_at":          now,
		}).Error
}

// ListBatchJobLines returns every recorded line result of a job.
func ListBatchJobLines(jobId string) ([]BatchJobLine, error) {
	var lines []BatchJobLine
	err := DB.Where("job_id = ?", jobId).Order("id ASC").Find(&lines).Error
	return lines, err
}

// MarkBatchJobSettleState advances only the settlement state.
func MarkBatchJobSettleState(job *BatchJob, state string) error {
	return DB.Model(&BatchJob{}).Where("id = ? AND poll_version = ? AND settle_state <> ?", job.Id, job.PollVersion, BatchSettleSettled).
		Updates(map[string]any{"settle_state": state, "updated_at": common.GetTimestamp()}).Error
}

// GetDueBatchJobById reloads one job row by id.
func GetDueBatchJobById(jobId string) (*BatchJob, error) {
	if jobId == "" {
		return nil, ErrBatchJobNotFound
	}
	var job BatchJob
	if err := DB.Where("id = ?", jobId).First(&job).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrBatchJobNotFound
		}
		return nil, err
	}
	return &job, nil
}
