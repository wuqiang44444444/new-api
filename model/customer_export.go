package model

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

// 客户自助/管理员代客异步导出（使用记录、账单汇总、账单明细）。
// ExportJob 行是用户申请与交付的操作事实；调度复用 SystemTask 的独立类型
// 租约，全站同一时刻只有一个源数据读取执行器。容量由固定槽位表约束。

const (
	CustomerExportJobTypeUsageLogs        = "usage_logs"
	CustomerExportJobTypeStatementDetails = "statement_details"
	CustomerExportJobTypeStatementSummary = "statement_summary"

	// SystemTaskTypeCustomerExport 是导出调度器的独立 SystemTask 类型；
	// 同类型租约约束全站只有一个源数据读取执行器。
	SystemTaskTypeCustomerExport = "customer_export"

	// CustomerExportSlotCount 是全站源数据读取队列容量（含执行中）。
	// 计划建议起点 1 执行 + 2 等待；扩容时必须同步重算等待预算。
	CustomerExportSlotCount = 3
)

type CustomerExportJobStatus string

const (
	CustomerExportJobStatusQueued     CustomerExportJobStatus = "queued"
	CustomerExportJobStatusRunning    CustomerExportJobStatus = "running"
	CustomerExportJobStatusSucceeded  CustomerExportJobStatus = "succeeded"
	CustomerExportJobStatusFailed     CustomerExportJobStatus = "failed"
	CustomerExportJobStatusCancelled  CustomerExportJobStatus = "cancelled"
	CustomerExportJobStatusExpired    CustomerExportJobStatus = "expired"
	CustomerExportJobStatusCancelWait CustomerExportJobStatus = "cancelling"
)

var (
	errCustomerExportSlotMissing    = errors.New("export slot for job not found")
	ErrCustomerExportUserBusy       = errors.New("user already has an active source-reading export job")
	ErrCustomerExportQueueBusy      = errors.New("export queue is full")
	ErrCustomerExportNotFound       = errors.New("export job not found")
	ErrCustomerExportNotCancellable = errors.New("export job is not cancellable")
	errCustomerExportLostSlot       = errors.New("queued export job lost its slot")
	ErrCustomerExportStateConflict  = errors.New("export job state conflict")
)

// CustomerExportFilters is the normalized, versioned submission scope. It is
// frozen at acceptance and never re-resolved from current settings.
type CustomerExportFilters struct {
	UsageView         string                          `json:"usage_view,omitempty"`
	UsageSearch       string                          `json:"usage_search,omitempty"`
	UsageDiscounts    *UsageAnalyticsDiscountSnapshot `json:"usage_discounts,omitempty"`
	Upstream          *UpstreamExportScope            `json:"upstream,omitempty"`
	StatementDraftId  string                          `json:"statement_draft_id,omitempty"`
	QuotaPerUnit      float64                         `json:"quota_per_unit"`
	Currency          string                          `json:"currency"`
	CurrencyRate      float64                         `json:"currency_rate"`
	FieldVersion      int                             `json:"field_version"`
	StartTimestamp    int64                           `json:"start_timestamp"` // 含起点
	EndTimestamp      int64                           `json:"end_timestamp"`   // 不含终点（左闭右开）
	LogTypes          []int                           `json:"log_types,omitempty"`
	TokenId           *int                            `json:"token_id,omitempty"`
	ChannelId         *int                            `json:"channel_id,omitempty"`
	TokenName         string                          `json:"token_name,omitempty"`
	Group             string                          `json:"group,omitempty"`
	RequestId         string                          `json:"request_id,omitempty"`
	UpstreamRequestId string                          `json:"upstream_request_id,omitempty"`
	Username          string                          `json:"username,omitempty"`
	ModelName         string                          `json:"model_name,omitempty"`
	BillingMode       string                          `json:"billing_mode,omitempty"`
	Timezone          string                          `json:"timezone"`
	Language          string                          `json:"language,omitempty"`
}

// CustomerExportProgress is throttled persisted scan state. Counts are
// candidate/matched/written facts, never a percentage of an unknown total.
type CustomerExportProgress struct {
	Scanned  int64 `json:"scanned"`
	Matched  int64 `json:"matched"`
	Written  int64 `json:"written"`
	Files    int64 `json:"files"`
	WaitingE bool  `json:"waiting_for_resources,omitempty"`
}

// CustomerExportArtifact describes published deliverables. Objects stay in the
// private export namespace; the database stores references, never signed URLs.
type CustomerExportArtifact struct {
	SourceVersion  string                       `json:"source_version,omitempty"`
	StoreIdentity  string                       `json:"store_identity,omitempty"`
	Files          []CustomerExportArtifactFile `json:"files"`
	LineCount      int64                        `json:"line_count"`
	SizeBytes      int64                        `json:"size_bytes"`
	GeneratedAt    int64                        `json:"generated_at"`
	ExpiresAt      int64                        `json:"expires_at"`
	AutoReuseMatch bool                         `json:"-"`
}

type CustomerExportArtifactFile struct {
	ObjectKey  string `json:"object_key"`
	FileName   string `json:"file_name"`
	SizeBytes  int64  `json:"size_bytes"`
	LineCount  int64  `json:"line_count"`
	Sha256     string `json:"sha256"`
	Downloaded int64  `json:"downloaded_count,omitempty"`
}

type CustomerExportJob struct {
	ID               int64                   `json:"id"`
	JobID            string                  `json:"job_id" gorm:"type:varchar(64);uniqueIndex"`
	UserId           int                     `json:"user_id" gorm:"index;index:idx_customer_export_user_active,priority:1"`
	TargetUserId     int                     `json:"target_user_id" gorm:"index"`
	JobType          string                  `json:"job_type" gorm:"type:varchar(32);index"`
	Status           CustomerExportJobStatus `json:"status" gorm:"type:varchar(32);index"`
	ActiveKey        *string                 `json:"active_key,omitempty" gorm:"type:varchar(64);uniqueIndex"`
	SlotId           int64                   `json:"slot_id" gorm:"bigint;index;default:0"`
	Filters          string                  `json:"filters" gorm:"type:text"`
	Progress         string                  `json:"progress" gorm:"type:text"`
	Artifact         string                  `json:"artifact" gorm:"type:text"`
	ErrorCode        string                  `json:"error_code" gorm:"type:varchar(64);default:''"`
	Error            string                  `json:"error" gorm:"type:text"`
	Executor         string                  `json:"executor" gorm:"type:varchar(128);default:''"`
	LeaseUntil       int64                   `json:"lease_until" gorm:"bigint;default:0"`
	CancelRequested  bool                    `json:"cancel_requested"`
	CancelWait       bool                    `json:"cancel_wait"`
	CreatedAt        int64                   `json:"created_at" gorm:"bigint;index"`
	StartedAt        int64                   `json:"started_at" gorm:"bigint;default:0"`
	FinishedAt       int64                   `json:"finished_at" gorm:"bigint;default:0"`
	ExpiresAt        int64                   `json:"expires_at" gorm:"bigint;default:0"`
	CleanupAttemptAt int64                   `json:"-" gorm:"bigint;default:0"`
	UpdatedAt        int64                   `json:"updated_at" gorm:"bigint;index"`
}

// CustomerExportSlot 是固定容量槽位。主键 1..N 幂等初始化；占槽/释放都用
// 条件更新并检查影响行数，不依赖数据库专属的部分索引。
type CustomerExportSlot struct {
	ID        int64  `json:"id" gorm:"primary_key"`
	JobID     string `json:"job_id" gorm:"type:varchar(64);index;default:''"`
	UpdatedAt int64  `json:"updated_at" gorm:"bigint;default:0"`
}

func (job *CustomerExportJob) BeforeCreate(_ *gorm.DB) error {
	now := common.GetTimestamp()
	if job.CreatedAt == 0 {
		job.CreatedAt = now
	}
	if job.UpdatedAt == 0 {
		job.UpdatedAt = now
	}
	return nil
}

func GenerateCustomerExportJobID() (string, error) {
	key, err := common.GenerateRandomCharsKey(32)
	if err != nil {
		return "", err
	}
	return "cex_" + key, nil
}

func migrateCustomerExportDB() error {
	if err := DB.AutoMigrate(&CustomerExportJob{}, &CustomerExportSlot{}); err != nil {
		return err
	}
	// 槽位按固定主键幂等初始化；缺失才插入，不更新既有行。
	for id := int64(1); id <= CustomerExportSlotCount; id++ {
		var count int64
		if err := DB.Model(&CustomerExportSlot{}).Where("id = ?", id).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			continue
		}
		if err := DB.Create(&CustomerExportSlot{ID: id, JobID: ""}).Error; err != nil {
			return err
		}
	}
	return nil
}

func decodeCustomerExportFilters(raw string) (CustomerExportFilters, error) {
	var filters CustomerExportFilters
	if strings.TrimSpace(raw) == "" {
		return filters, errors.New("export filters are required")
	}
	if err := common.UnmarshalJsonStr(raw, &filters); err != nil {
		return filters, err
	}
	return filters, nil
}

func (job *CustomerExportJob) DecodeFilters() (CustomerExportFilters, error) {
	return decodeCustomerExportFilters(job.Filters)
}

func (job *CustomerExportJob) DecodeProgress() CustomerExportProgress {
	var progress CustomerExportProgress
	if job.Progress != "" {
		_ = common.UnmarshalJsonStr(job.Progress, &progress)
	}
	return progress
}

func (job *CustomerExportJob) DecodeArtifact() *CustomerExportArtifact {
	if job.Artifact == "" {
		return nil
	}
	var artifact CustomerExportArtifact
	if err := common.UnmarshalJsonStr(job.Artifact, &artifact); err != nil {
		return nil
	}
	return &artifact
}

type CustomerExportJobView struct {
	JobID           string                  `json:"job_id"`
	UserId          int                     `json:"user_id"`
	TargetUserId    int                     `json:"target_user_id"`
	JobType         string                  `json:"job_type"`
	Status          CustomerExportJobStatus `json:"status"`
	Filters         CustomerExportFilters   `json:"filters"`
	Progress        CustomerExportProgress  `json:"progress"`
	CancelRequested bool                    `json:"cancel_requested"`
	ErrorCode       string                  `json:"error_code,omitempty"`
	Error           string                  `json:"error,omitempty"`
	Artifact        *CustomerExportArtifact `json:"artifact,omitempty"`
	CreatedAt       int64                   `json:"created_at"`
	StartedAt       int64                   `json:"started_at,omitempty"`
	FinishedAt      int64                   `json:"finished_at,omitempty"`
	ExpiresAt       int64                   `json:"expires_at,omitempty"`
}

// ToView 是客户安全的任务投影：不暴露执行器、槽位或内部状态列。
func (job *CustomerExportJob) ToView() CustomerExportJobView {
	view := CustomerExportJobView{
		JobID:           job.JobID,
		UserId:          job.UserId,
		TargetUserId:    job.TargetUserId,
		JobType:         job.JobType,
		Status:          job.Status,
		Progress:        job.DecodeProgress(),
		CancelRequested: job.CancelRequested,
		ErrorCode:       job.ErrorCode,
		Error:           job.Error,
		CreatedAt:       job.CreatedAt,
		StartedAt:       job.StartedAt,
		FinishedAt:      job.FinishedAt,
		ExpiresAt:       job.ExpiresAt,
	}
	if filters, err := job.DecodeFilters(); err == nil {
		view.Filters = filters
	}
	if artifact := job.DecodeArtifact(); artifact != nil && job.Status == CustomerExportJobStatusSucceeded {
		for i := range artifact.Files {
			artifact.Files[i].ObjectKey = ""
		}
		artifact.StoreIdentity = ""
		artifact.SourceVersion = ""
		view.Artifact = artifact
	}
	return view
}

// customerExportActiveKey 是按发起人计的配额键；值不包含目标客户或应用，
// 管理员不能通过更换目标客户绕过个人限额。
func customerExportActiveKey(userId int) string {
	return "cexa:" + strconv.Itoa(userId)
}

// CreateCustomerExportJob 原子受理一个源数据读取申请：个人配额（可空唯一键）
// 与固定槽位占用在同一短事务内提交；唯一冲突或无空槽整体回滚。相同申请
// （同类型、同目标、同规范化条件）返回既有任务，不重复扫描；不同申请返回忙碌。
func CreateCustomerExportJob(userId int, targetUserId int, jobType string, filters CustomerExportFilters, reuseOptions ...CustomerExportReuse) (*CustomerExportJob, bool, error) {
	filtersText, err := common.Marshal(filters)
	if err != nil {
		return nil, false, err
	}
	var reuse CustomerExportReuse
	if len(reuseOptions) > 0 {
		reuse = reuseOptions[0]
	}
	activeKey := customerExportActiveKey(userId)
	for attempt := 0; attempt < 2; attempt++ {
		job, created, err := createCustomerExportJobOnce(userId, targetUserId, jobType, string(filtersText), activeKey, attempt > 0, reuse)
		if err == nil || !errors.Is(err, ErrCustomerExportStateConflict) {
			return job, created, err
		}
		// 唯一冲突：并发提交下重读权威状态再判定，不能一概当作成功。
	}
	return nil, false, ErrCustomerExportUserBusy
}

func createCustomerExportJobOnce(userId int, targetUserId int, jobType string, filtersText string, activeKey string, lastAttempt bool, reuse CustomerExportReuse) (*CustomerExportJob, bool, error) {
	var created *CustomerExportJob
	err := DB.Transaction(func(tx *gorm.DB) error {
		if job, err := findReusableCustomerExport(tx, userId, targetUserId, jobType, filtersText, reuse); err != nil {
			return err
		} else if job != nil {
			return errCustomerExportDuplicate(job.JobID)
		}
		var existing CustomerExportJob
		err := tx.Where("active_key = ?", activeKey).First(&existing).Error
		if err == nil {
			// 本人相同申请返回原记录；不同申请返回忙碌，不重复扫描。
			if existing.JobType == jobType && existing.TargetUserId == targetUserId && (!reuse.RequireExactActiveFilters || existing.Filters == filtersText) && sameActiveCustomerExportFilters(jobType, existing.Filters, filtersText) {
				return errCustomerExportDuplicate(existing.JobID)
			}
			return ErrCustomerExportUserBusy
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		jobID, err := GenerateCustomerExportJobID()
		if err != nil {
			return err
		}
		claimedSlot := int64(0)
		for slotId := int64(1); slotId <= CustomerExportSlotCount; slotId++ {
			result := tx.Model(&CustomerExportSlot{}).
				Where("id = ? AND job_id = ?", slotId, "").
				Updates(map[string]any{"job_id": jobID, "updated_at": common.GetTimestamp()})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected > 0 {
				claimedSlot = slotId
				break
			}
		}
		if claimedSlot == 0 {
			return ErrCustomerExportQueueBusy
		}

		job := &CustomerExportJob{
			JobID:        jobID,
			UserId:       userId,
			TargetUserId: targetUserId,
			JobType:      jobType,
			Status:       CustomerExportJobStatusQueued,
			ActiveKey:    &activeKey,
			SlotId:       claimedSlot,
			Filters:      filtersText,
		}
		if err := tx.Create(job).Error; err != nil {
			if lastAttempt {
				// 重试后仍失败：不是并发唯一冲突，原样返回数据库错误。
				return err
			}
			return ErrCustomerExportStateConflict
		}
		created = job
		return nil
	})
	if err == nil {
		return created, true, nil
	}
	var duplicateErr errCustomerExportDuplicateJobID
	if errors.As(err, &duplicateErr) {
		job, getErr := GetCustomerExportJob(duplicateErr.JobID)
		if getErr != nil {
			return nil, false, getErr
		}
		return job, false, nil
	}
	return nil, false, err
}

type errCustomerExportDuplicateJobID struct{ JobID string }

func (e errCustomerExportDuplicateJobID) Error() string {
	return "duplicate active export job: " + e.JobID
}

func errCustomerExportDuplicate(jobID string) error {
	return errCustomerExportDuplicateJobID{JobID: jobID}
}

func GetCustomerExportJob(jobID string) (*CustomerExportJob, error) {
	ctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	var job CustomerExportJob
	if err := DB.WithContext(ctx).Where("job_id = ?", jobID).First(&job).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCustomerExportNotFound
		}
		return nil, err
	}
	return &job, nil
}

// GetCustomerExportJobForOwner 返回属于指定发起人的任务；任务 ID 不携带权限。
func GetCustomerExportJobForOwner(jobID string, userId int) (*CustomerExportJob, error) {
	job, err := GetCustomerExportJob(jobID)
	if err != nil {
		return nil, err
	}
	if job.UserId != userId {
		return nil, ErrCustomerExportNotFound
	}
	if err := AuthorizeCustomerExportJob(context.Background(), job); err != nil {
		return nil, err
	}
	return job, nil
}

func ListCustomerExportJobs(ctx context.Context, userId, page, pageSize int) ([]*CustomerExportJob, int64, error) {
	if page < 1 || page > 1000000 || pageSize < 1 || pageSize > 100 {
		return nil, 0, errors.New("invalid pagination")
	}
	if err := AuthorizeCustomerExport(ctx, userId, userId); err != nil {
		return nil, 0, err
	}
	var actor User
	if err := DB.WithContext(ctx).Select("role").First(&actor, userId).Error; err != nil {
		return nil, 0, err
	}
	query := DB.WithContext(ctx).Where("user_id = ? AND job_type <> ?", userId, CustomerExportJobTypeStatementVersion)
	if actor.Role < common.RoleAdminUser {
		query = query.Where("target_user_id = ?", userId)
	}
	visible := make([]*CustomerExportJob, 0, pageSize)
	var total, cursor int64
	skip := int64(page-1) * int64(pageSize)
	for {
		batchQuery := query.Session(&gorm.Session{})
		if cursor > 0 {
			batchQuery = batchQuery.Where("id < ?", cursor)
		}
		var jobs []*CustomerExportJob
		if err := batchQuery.Order("id desc").Limit(100).Find(&jobs).Error; err != nil {
			return nil, 0, err
		}
		for _, job := range jobs {
			jobAuthorizable := job.JobType == CustomerExportJobTypeUpstreamDetails || job.JobType == CustomerExportJobTypeUpstreamSummary || job.JobType == CustomerExportJobTypeUsageSummary
			if jobAuthorizable && AuthorizeCustomerExportJob(ctx, job) != nil {
				continue
			}
			if total >= skip && len(visible) < pageSize {
				visible = append(visible, job)
			}
			total++
		}
		if len(jobs) < 100 {
			break
		}
		cursor = jobs[len(jobs)-1].ID
	}
	return visible, total, nil
}

// CancelCustomerExportJob 取消一个排队中的申请并原子释放配额与槽位；
// 运行中的申请只登记取消请求，由执行器在批次结束时确认停止。
func CancelCustomerExportJob(jobID string, userId int) (*CustomerExportJob, error) {
	job, err := GetCustomerExportJobForOwner(jobID, userId)
	if err != nil {
		return nil, err
	}
	switch job.Status {
	case CustomerExportJobStatusQueued:
		err = DB.Transaction(func(tx *gorm.DB) error {
			result := tx.Model(&CustomerExportJob{}).
				Where("job_id = ? AND status = ?", jobID, CustomerExportJobStatusQueued).
				Updates(map[string]any{
					"status":      CustomerExportJobStatusCancelled,
					"active_key":  nil,
					"error_code":  "cancelled",
					"finished_at": common.GetTimestamp(),
					"updated_at":  common.GetTimestamp(),
				})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return ErrCustomerExportStateConflict
			}
			if err := finishBillingStatementGenerationTx(tx, jobID, CustomerExportJobStatusCancelled); err != nil {
				return err
			}
			return freeCustomerExportSlot(tx, jobID)
		})
		if errors.Is(err, ErrCustomerExportStateConflict) {
			return CancelCustomerExportJob(jobID, userId)
		}
		if err != nil {
			return nil, err
		}
	case CustomerExportJobStatusRunning:
		result := DB.Model(&CustomerExportJob{}).
			Where("job_id = ? AND status = ?", jobID, CustomerExportJobStatusRunning).
			Updates(map[string]any{"status": CustomerExportJobStatusCancelWait, "cancel_wait": true, "cancel_requested": true, "updated_at": common.GetTimestamp()})
		if result.Error != nil {
			return nil, result.Error
		}
		if result.RowsAffected == 0 {
			return CancelCustomerExportJob(jobID, userId)
		}
	case CustomerExportJobStatusCancelWait:
		// 已在取消中；幂等返回当前状态。
	default:
		return nil, ErrCustomerExportNotCancellable
	}
	return GetCustomerExportJob(jobID)
}

func freeCustomerExportSlot(tx *gorm.DB, jobID string) error {
	result := tx.Model(&CustomerExportSlot{}).
		Where("job_id = ?", jobID).
		Updates(map[string]any{"job_id": "", "updated_at": common.GetTimestamp()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errCustomerExportSlotMissing
	}
	return nil
}

// ClaimNextQueuedCustomerExportJob 以条件更新认领最早的排队任务。执行器是
// 全站唯一的源数据读取器（SystemTask 类型租约），认领同时校验槽位仍然在握。
func ClaimNextQueuedCustomerExportJob(executor string, leaseUntil int64, maxQueueWaitSeconds int64) (*CustomerExportJob, error) {
	var jobs []*CustomerExportJob
	if err := DB.Where("status = ?", CustomerExportJobStatusQueued).Order("id asc").Limit(5).Find(&jobs).Error; err != nil {
		return nil, err
	}
	now := common.GetTimestamp()
	for _, job := range jobs {
		if maxQueueWaitSeconds > 0 && now-job.CreatedAt > maxQueueWaitSeconds {
			// 排队超时明确失败，不误报仍在运行；槽位与配额同事务释放。
			if _, err := FailCustomerExportJobById(job.JobID, "queue_timeout", "export queue wait budget exceeded"); err != nil {
				return nil, err
			}
			continue
		}
		err := DB.Transaction(func(tx *gorm.DB) error {
			if job.SlotId <= 0 {
				return errCustomerExportLostSlot
			}
			result := tx.Model(&CustomerExportJob{}).
				Where("job_id = ? AND status = ? AND slot_id > 0", job.JobID, CustomerExportJobStatusQueued).
				Updates(map[string]any{
					"status":      CustomerExportJobStatusRunning,
					"executor":    executor,
					"lease_until": leaseUntil,
					"started_at":  now,
					"updated_at":  now,
				})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return ErrCustomerExportStateConflict
			}
			var slots int64
			if err := tx.Model(&CustomerExportSlot{}).Where("id = ? AND job_id = ?", job.SlotId, job.JobID).Count(&slots).Error; err != nil {
				return err
			}
			if slots == 0 {
				return errCustomerExportLostSlot
			}
			return nil
		})
		if errors.Is(err, errCustomerExportLostSlot) {
			if _, failErr := FailCustomerExportJobById(job.JobID, "slot_lost", "export queue reservation lost; submit again"); failErr != nil {
				return nil, failErr
			}
			continue
		}
		if errors.Is(err, ErrCustomerExportStateConflict) {
			continue
		}
		if err != nil {
			return nil, err
		}
		return GetCustomerExportJob(job.JobID)
	}
	return nil, nil
}

// RenewCustomerExportLease 条件续租；租约丢失返回错误，执行器必须立即停止。
func RenewCustomerExportLease(jobID string, executor string, leaseUntil int64) error {
	result := DB.Model(&CustomerExportJob{}).
		Where("job_id = ? AND status = ? AND executor = ? AND lease_until > ?", jobID, CustomerExportJobStatusRunning, executor, common.GetTimestamp()).
		Updates(map[string]any{"lease_until": leaseUntil, "updated_at": common.GetTimestamp()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrCustomerExportStateConflict
	}
	return nil
}

// UpdateCustomerExportProgress 按调用方节流频率持久化扫描进度。
func UpdateCustomerExportProgress(ctx context.Context, jobID string, executor string, progress CustomerExportProgress) error {
	progressText, err := common.Marshal(progress)
	if err != nil {
		return err
	}
	result := DB.WithContext(ctx).Model(&CustomerExportJob{}).
		Where("job_id = ? AND status = ? AND executor = ?", jobID, CustomerExportJobStatusRunning, executor).
		Updates(map[string]any{"progress": string(progressText), "updated_at": common.GetTimestamp()})
	return result.Error
}

// CountQueuedCustomerExportJobs 返回仍需执行的任务数，供调度器决定是否
// 重新唤醒下一轮。
func CountQueuedCustomerExportJobs() (int64, error) {
	var count int64
	err := DB.Model(&CustomerExportJob{}).Where("status = ?", CustomerExportJobStatusQueued).Count(&count).Error
	return count, err
}

// IsCustomerExportCancelRequested 读取最新的取消标记；运行中取消只解除本人
// 等待，不删除其他有效申请引用的产物。
func IsCustomerExportCancelRequested(jobID string) (bool, error) {
	var job CustomerExportJob
	if err := DB.Select("cancel_requested", "status").Where("job_id = ?", jobID).First(&job).Error; err != nil {
		return false, err
	}
	return job.CancelRequested, nil
}

// FinishCustomerExportJob 在同一主库事务中结束工作并释放容量槽与个人配额。
// 条件更新保证幂等：旧执行器或重复完成不能覆盖终态。
func FinishCustomerExportJob(jobID string, executor string, status CustomerExportJobStatus, errorCode string, errorMessage string, artifact *CustomerExportArtifact) error {
	ctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		updates := map[string]any{
			"status":      status,
			"active_key":  nil,
			"error_code":  errorCode,
			"error":       errorMessage,
			"finished_at": common.GetTimestamp(),
			"updated_at":  common.GetTimestamp(),
		}
		if artifact != nil {
			if artifact.ExpiresAt == 0 {
				artifact.ExpiresAt = common.GetTimestamp() + 86400
			}
			artifactText, err := common.Marshal(artifact)
			if err != nil {
				return err
			}
			updates["artifact"] = string(artifactText)
			updates["expires_at"] = artifact.ExpiresAt
		}
		query := tx.Model(&CustomerExportJob{}).Where("job_id = ? AND status IN ?", jobID, []CustomerExportJobStatus{CustomerExportJobStatusRunning, CustomerExportJobStatusCancelWait})
		if executor != "" {
			query = query.Where("executor = ?", executor)
		}
		if status == CustomerExportJobStatusSucceeded {
			query = query.Where("status = ? AND cancel_requested = ? AND lease_until > ?", CustomerExportJobStatusRunning, false, common.GetTimestamp())
		}
		result := query.Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrCustomerExportStateConflict
		}
		if err := finishBillingStatementGenerationTx(tx, jobID, status); err != nil {
			return err
		}
		return freeCustomerExportSlot(tx, jobID)
	})
}

// FailCustomerExportJobById 供调度器在无执行器上下文时失败一个任务
// （排队超时、进程恢复）。
func FailCustomerExportJobById(jobID string, errorCode string, errorMessage string) (bool, error) {
	var failed bool
	err := DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&CustomerExportJob{}).
			Where("job_id = ? AND (status = ? OR (status IN ? AND lease_until <= ?))", jobID, CustomerExportJobStatusQueued, []CustomerExportJobStatus{CustomerExportJobStatusRunning, CustomerExportJobStatusCancelWait}, common.GetTimestamp()).
			Updates(map[string]any{
				"status":      CustomerExportJobStatusFailed,
				"active_key":  nil,
				"error_code":  errorCode,
				"error":       errorMessage,
				"finished_at": common.GetTimestamp(),
				"updated_at":  common.GetTimestamp(),
			})
		if result.Error != nil {
			return result.Error
		}
		failed = result.RowsAffected > 0
		if !failed {
			return nil
		}
		if err := finishBillingStatementGenerationTx(tx, jobID, CustomerExportJobStatusFailed); err != nil {
			return err
		}
		err := freeCustomerExportSlot(tx, jobID)
		if errorCode == "slot_lost" && errors.Is(err, errCustomerExportSlotMissing) {
			return nil
		}
		return err
	})
	return failed, err
}

// RecoverInterruptedCustomerExportJobs 是进程恢复路径：租约过期的 running
// 任务在第一版直接标为失败，允许用户重新生成；不做断点续传。
func RecoverInterruptedCustomerExportJobs(now int64) (int64, error) {
	var expired []*CustomerExportJob
	if err := DB.Where("status IN ? AND lease_until > 0 AND lease_until < ?", []CustomerExportJobStatus{CustomerExportJobStatusRunning, CustomerExportJobStatusCancelWait}, now).Limit(100).Find(&expired).Error; err != nil {
		return 0, err
	}
	var recovered int64
	for _, job := range expired {
		failed, err := FailCustomerExportJobById(job.JobID, "lease_expired", "export executor lease expired")
		if err != nil {
			return recovered, err
		}
		if failed {
			recovered++
		}
	}
	return recovered, nil
}

// ExpireCustomerExportArtifacts 把到达保留期的产物在应用层立即失效；
// 物理删除由清理流程执行，失效不依赖删除成功。
func ExpireCustomerExportArtifacts(now int64) ([]*CustomerExportJob, error) {
	var jobs []*CustomerExportJob
	if err := DB.Where("status = ? AND expires_at > 0 AND expires_at <= ?", CustomerExportJobStatusSucceeded, now).Limit(100).Find(&jobs).Error; err != nil {
		return nil, err
	}
	for _, job := range jobs {
		// 过期即应用层失效（下载资格消失）；产物引用保留给物理清理，
		// 清理成功后才由 DeleteCustomerExportArtifactRecord 清空。
		result := DB.Model(&CustomerExportJob{}).
			Where("job_id = ? AND status = ?", job.JobID, CustomerExportJobStatusSucceeded).
			Updates(map[string]any{
				"status":     CustomerExportJobStatusExpired,
				"updated_at": common.GetTimestamp(),
			})
		if result.Error != nil {
			return jobs, result.Error
		}
	}
	return jobs, nil
}

// FindExpiredCustomerExportArtifacts 返回已过期但对象可能仍存在的任务，
// 供物理清理重试；清理失败不会恢复下载资格。
func FindExpiredCustomerExportArtifacts(limit int) ([]*CustomerExportJob, error) {
	if limit <= 0 {
		limit = 20
	}
	var jobs []*CustomerExportJob
	err := DB.Where("status IN ? AND artifact <> ''", []CustomerExportJobStatus{CustomerExportJobStatusExpired, CustomerExportJobStatusFailed, CustomerExportJobStatusCancelled}).Order("cleanup_attempt_at asc, id asc").Limit(limit).Find(&jobs).Error
	return jobs, err
}

// Rotate attempted jobs even when their storage location is unavailable.
// The durable timestamp lets later jobs progress across scheduler restarts.
func RecordCustomerExportCleanupAttempt(ctx context.Context, jobID string, now int64) error {
	return DB.WithContext(ctx).Model(&CustomerExportJob{}).Where("job_id = ?", jobID).
		Update("cleanup_attempt_at", now).Error
}

// DeleteCustomerExportArtifactRecord 在对象删除成功（或确认不存在）后清空
// 引用；行本身按任务记录保留期另行清理。
func DeleteCustomerExportArtifactRecord(jobID string) error {
	result := DB.Model(&CustomerExportJob{}).
		Where("job_id = ? AND status IN ?", jobID, []CustomerExportJobStatus{CustomerExportJobStatusExpired, CustomerExportJobStatusFailed, CustomerExportJobStatusCancelled}).
		Updates(map[string]any{"artifact": "", "updated_at": common.GetTimestamp()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrCustomerExportStateConflict
	}
	return nil
}

// CleanupCustomerExportJobRecords 删除超过保留期的终态任务行；仅限终态。
func CleanupCustomerExportJobRecords(now int64, retentionSeconds int64) (int64, error) {
	result := DB.Where("artifact = '' AND status IN ? AND finished_at > 0 AND finished_at < ?", []CustomerExportJobStatus{
		CustomerExportJobStatusSucceeded, CustomerExportJobStatusFailed,
		CustomerExportJobStatusCancelled, CustomerExportJobStatusExpired,
	}, now-retentionSeconds).Delete(&CustomerExportJob{})
	return result.RowsAffected, result.Error
}
