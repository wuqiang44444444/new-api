package model

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/bytedance/gopkg/util/gopool"

	"gorm.io/gorm"
)

// 显式图片执行协议（§3.7/硬约束 §4）。图片 Task 允许先于 Provider task ID
// 创建；Task 自身状态机承载发送许可与“可能已发送”事实。

const TaskClientProtocolImageOpenAIV1 = "image_openai_v1"

// 图片任务的 Action 值（NormalizeTaskAction 对未知值原样透传）。
const (
	TaskActionImageGeneration = "image_generation"
	TaskActionImageEdit       = "image_edit"
)

// IsImageTaskClientProtocol reports whether a task belongs to the explicit
// image execution protocol.
func IsImageTaskClientProtocol(protocol string) bool {
	return protocol == TaskClientProtocolImageOpenAIV1
}

// IsImageTask reports whether the row is an explicit image execution task.
func IsImageTask(task *Task) bool {
	return task != nil && IsImageTaskClientProtocol(task.ClientProtocol)
}

// TaskImageExecutionData 是图片任务的受保护执行快照（PrivateData，绝不
// 进入客户响应）。渠道编辑、凭据轮换或价格变化不影响已受理任务。
type TaskImageExecutionData struct {
	Revision       int64  `json:"revision"`
	Operation      string `json:"operation"` // generations|edits
	RequestHash    string `json:"request_hash,omitempty"`
	Prompt         string `json:"prompt,omitempty"`
	Size           string `json:"size,omitempty"`
	ResponseFormat string `json:"response_format,omitempty"`
	N              uint   `json:"n,omitempty"`

	// 输入：二进制输入已落私有 OSS（对象引用）；直传 URL 原样冻结。
	Parameters               *dto.ImageRequest    `json:"parameters,omitempty"` // scalar request only; input bytes stay in OSS
	Price                    *hosttypes.PriceData `json:"price,omitempty"`
	BillingRequestCiphertext string               `json:"billing_request_ciphertext,omitempty"`
	Usage                    *dto.Usage           `json:"usage,omitempty"`
	GenerationComplete       bool                 `json:"generation_complete,omitempty"`
	ExpectedImages           int                  `json:"expected_images,omitempty"`
	ResultManifest           []TaskImageArtifact  `json:"result_manifest,omitempty"`
	Inputs                   []TaskImageInputRef  `json:"inputs,omitempty"`

	FundsHeld bool `json:"funds_held"`
	HeldQuota int  `json:"held_quota,omitempty"`
	FreeModel bool `json:"free_model,omitempty"`

	HeadersCiphertext string                   `json:"headers_ciphertext"`
	ChannelType       int                      `json:"channel_type"`
	ChannelBaseUrl    string                   `json:"channel_base_url"`
	ChannelKey        string                   `json:"channel_key,omitempty"`
	ChannelProxy      string                   `json:"channel_proxy,omitempty"`
	ChannelSettings   dto.ChannelSettings      `json:"channel_settings,omitempty"`
	ChannelOther      dto.ChannelOtherSettings `json:"channel_other,omitempty"`
	UpstreamModel     string                   `json:"upstream_model,omitempty"`
	ApiVersion        string                   `json:"api_version,omitempty"`

	QueueDeadlineAt int64 `json:"queue_deadline_at,omitempty"`
	SentAt          int64 `json:"sent_at,omitempty"` // 发送许可：持久提交后才能写请求字节
	LeaseAt         int64 `json:"lease_at,omitempty"`

	ProviderTaskID string              `json:"provider_task_id,omitempty"` // 异步上游真实 ID（FunCloud）
	Artifacts      []TaskImageArtifact `json:"artifacts,omitempty"`

	ImageCount  int    `json:"image_count,omitempty"`
	FailureCode string `json:"failure_code,omitempty"`
}

type TaskImageInputRef struct {
	ObjectKey string `json:"object_key,omitempty"`
	URL       string `json:"url,omitempty"`
	MimeType  string `json:"mime_type,omitempty"`
}

func (r TaskImageInputRef) IsURL() bool { return r.URL != "" }

type TaskImageArtifact struct {
	ObjectKey string `json:"object_key"`
	MimeType  string `json:"mime_type,omitempty"`
	Size      int64  `json:"size,omitempty"`
}

// BuildImageTaskChannelMeta reconstructs the frozen southbound connection.
func (d *TaskImageExecutionData) BuildImageTaskChannelMeta(channelID int) *relaycommon.ChannelMeta {
	return &relaycommon.ChannelMeta{
		ChannelType:          d.ChannelType,
		ChannelId:            channelID,
		ChannelBaseUrl:       d.ChannelBaseUrl,
		ApiKey:               d.ChannelKey,
		ApiVersion:           d.ApiVersion,
		ChannelSetting:       d.ChannelSettings,
		ChannelOtherSettings: d.ChannelOther,
		UpstreamModelName:    d.UpstreamModel,
	}
}

// ImageTaskInsertParams carries everything the acceptance transaction needs.
type ImageTaskInsertParams struct {
	Task          *Task
	IdempotencyID int64
	GlobalScope   string
	GlobalLimit   int
	AppScope      string
	AppLimit      int
}

// InsertImageTask atomically reserves admission capacity, creates the task,
// and completes the idempotency binding（§3.8.3：禁止无锁 count 后 insert）。
func InsertImageTask(params ImageTaskInsertParams) error {
	if params.Task == nil {
		return errors.New("image task is required")
	}
	task := params.Task
	if !IsImageTask(task) || task.PrivateData.ImageTask == nil || task.Quota < 0 || task.Status != TaskStatusQueued {
		return errors.New("invalid image acceptance")
	}
	var tokenKey string
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockImageTaskSlotsTx(tx); err != nil {
			return err
		}
		if err := reserveImageSlotsTx(tx, params.GlobalScope, params.GlobalLimit, params.AppScope, params.AppLimit); err != nil {
			return err
		}
		if task.Quota > 0 {
			wallet := tx.Model(&User{}).Where("id = ? AND quota >= ?", task.UserId, task.Quota).
				Update("quota", gorm.Expr("quota - ?", task.Quota))
			if wallet.Error != nil {
				return wallet.Error
			}
			if wallet.RowsAffected != 1 {
				return ErrTaskAttemptInsufficientQuota
			}
			if task.PrivateData.TokenId > 0 && !task.PrivateData.SkipTokenQuota {
				var token Token
				if err := lockForUpdate(tx).Where("id = ? AND user_id = ?", task.PrivateData.TokenId, task.UserId).First(&token).Error; err != nil {
					return err
				}
				if !token.UnlimitedQuota && token.RemainQuota < task.Quota {
					return ErrTaskAttemptInsufficientQuota
				}
				if err := tx.Model(&token).Updates(map[string]any{
					"remain_quota":  gorm.Expr("remain_quota - ?", task.Quota),
					"used_quota":    gorm.Expr("used_quota + ?", task.Quota),
					"accessed_time": common.GetTimestamp(),
				}).Error; err != nil {
					return err
				}
				tokenKey = token.Key
			}
		}
		task.PrivateData.ImageTask.FundsHeld = true
		task.PrivateData.ImageTask.HeldQuota = task.Quota
		task.BillingState = deriveBillingState(task.PrivateData)

		if err := tx.Create(params.Task).Error; err != nil {
			return err
		}
		if params.IdempotencyID != 0 {
			result := tx.Model(&TaskCreateIdempotency{}).
				Where("id = ? AND status = ?", params.IdempotencyID, TaskCreateIdempotencyCreating).
				Updates(map[string]any{
					"status":     TaskCreateIdempotencyComplete,
					"task_id":    params.Task.TaskID,
					"channel_id": params.Task.ChannelId,
					"updated_at": common.GetTimestamp(),
				})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return errors.New("image idempotency claim is no longer active")
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if task.Quota > 0 {
		userID, heldQuota := task.UserId, task.Quota
		gopool.Go(func() {
			if err := cacheDecrUserQuota(userID, int64(heldQuota)); err != nil {
				common.SysError("image wallet cache update failed: " + err.Error())
			}
			if tokenKey != "" && common.RedisEnabled && common.RDB != nil {
				if err := cacheDecrTokenQuota(tokenKey, int64(heldQuota)); err != nil {
					common.SysError("image token cache update failed: " + err.Error())
				}
			}
		})
	}
	return nil
}

// MarkImageTaskSending durably commits the send permit before any provider
// bytes are written (§3.8.4/R7)。
func MarkImageTaskSending(task *Task) (bool, error) {
	if !IsImageTask(task) || task.PrivateData.ImageTask == nil || task.Status != TaskStatusInProgress || task.PrivateData.ImageTask.SentAt != 0 {
		return false, errors.New("image send permit is unavailable")
	}
	return transitionImageTask(task, TaskStatusInProgress, func(data *TaskImageExecutionData) {
		data.SentAt = common.GetTimestamp()
		data.LeaseAt = common.GetTimestamp()
	})
}

// MarkImageTaskProviderTaskID persists a trusted async upstream task id.
func MarkImageTaskProviderTaskID(task *Task, providerTaskID string) (bool, error) {
	return transitionImageTask(task, task.Status, func(data *TaskImageExecutionData) {
		data.ProviderTaskID = providerTaskID
	})
}

// RecordImageTaskGeneration persists usage and planned object keys BEFORE result upload.
// It is evidence of generation, not evidence of successful delivery.
func RecordImageTaskGeneration(task *Task, manifest []TaskImageArtifact, usage *dto.Usage) (bool, error) {
	if len(manifest) == 0 {
		return false, errors.New("image result manifest is empty")
	}
	return transitionImageTask(task, task.Status, func(data *TaskImageExecutionData) {
		data.GenerationComplete = true
		data.ExpectedImages = len(manifest)
		data.ResultManifest = manifest
		data.Usage = usage
	})
}

// FinishImageTaskSuccess CAS-commits the terminal success state together with
// the artifact references and usage evidence.
func FinishImageTaskSuccess(task *Task, artifacts []TaskImageArtifact, usage *dto.Usage) (bool, error) {
	return transitionImageTask(task, TaskStatusSuccess, func(data *TaskImageExecutionData) {
		data.Artifacts = artifacts
		data.ImageCount = len(artifacts)
		data.Usage = usage
		data.GenerationComplete = true
		data.ExpectedImages = len(artifacts)
	})
}

// FinishImageTaskFailure CAS-commits a definite provider rejection.
func FinishImageTaskFailure(task *Task, status TaskStatus, code string) (bool, error) {
	if status != TaskStatusFailure && status != TaskStatusExpired && status != TaskStatusReconciliationRequired {
		return false, errors.New("invalid image task failure status")
	}
	return transitionImageTask(task, status, func(data *TaskImageExecutionData) {
		data.FailureCode = code
	})
}

// AppendImageTaskArtifact 逐图持久化结果对象关联（评审 S6）：每张上传
// 成功后立即登记，崩溃后按确定性对象键补登记/续传，不再只在终态一次性
// 提交。幂等：重复登记同一键不追加。
func AppendImageTaskArtifact(task *Task, artifact TaskImageArtifact) (bool, error) {
	if strings.TrimSpace(artifact.ObjectKey) == "" {
		return false, errors.New("image artifact object key is required")
	}
	return transitionImageTask(task, task.Status, func(data *TaskImageExecutionData) {
		for _, existing := range data.Artifacts {
			if existing.ObjectKey == artifact.ObjectKey {
				return
			}
		}
		data.Artifacts = append(data.Artifacts, artifact)
		if len(data.ResultManifest) > 0 {
			order := make(map[string]int, len(data.ResultManifest))
			for index, planned := range data.ResultManifest {
				order[planned.ObjectKey] = index
			}
			sort.SliceStable(data.Artifacts, func(i, j int) bool {
				return order[data.Artifacts[i].ObjectKey] < order[data.Artifacts[j].ObjectKey]
			})
		}
		data.ImageCount = len(data.Artifacts)
	})
}

// FindImageTaskArtifact 按旧 artifact 内容路由的 key（result-N）解析图片
// 任务的持久化结果引用；无持久化事实时返回 nil（评审 S5：Resolve 不得
// 为从未入库的对象签发引用）。
func FindImageTaskArtifact(task *Task, artifactKey string) *TaskImageArtifact {
	if !IsImageTask(task) || task.PrivateData.ImageTask == nil {
		return nil
	}
	for index := range task.PrivateData.ImageTask.Artifacts {
		artifact := &task.PrivateData.ImageTask.Artifacts[index]
		if artifact.ObjectKey == buildImageTaskObjectKey(task, artifactKey) ||
			strings.HasSuffix(artifact.ObjectKey, "/"+artifactKey) {
			return artifact
		}
	}
	return nil
}

// ImageTaskArtifactObjectKey 暴露图片任务结果对象的确定性命名规则，
// 供存储层 Resolve 与执行器共用。
func ImageTaskArtifactObjectKey(taskID, artifactKey string) string {
	return fmt.Sprintf("images/tasks/%s/%s", strings.TrimSpace(taskID), strings.TrimSpace(artifactKey))
}

func buildImageTaskObjectKey(task *Task, artifactKey string) string {
	return ImageTaskArtifactObjectKey(task.TaskID, artifactKey)
}

// ImageTaskRequiresIdempotencyRetention 判断图片幂等 claim 到期后是否仍
// 不得重置：任务未终态（含待核实）期间必须保留绑定（方案 §3.7，评审 S7）。
func ImageTaskRequiresIdempotencyRetention(userID int, taskID string) bool {
	if strings.TrimSpace(taskID) == "" {
		return false
	}
	task, exists, err := GetTaskForProtocol(userID, taskID, TaskClientProtocolImageOpenAIV1, true)
	if err != nil || !exists || task == nil {
		return true
	}
	return imageAdmissionOccupied(task)
}

func transitionImageTask(task *Task, toStatus TaskStatus, mutate func(*TaskImageExecutionData)) (bool, error) {
	if !IsImageTask(task) || task.PrivateData.ImageTask == nil {
		return false, errors.New("task is not an image execution task")
	}
	var next *Task
	won := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockImageTaskSlotsTx(tx); err != nil {
			return err
		}
		var current Task
		if err := lockForUpdate(tx).Where("id = ?", task.ID).First(&current).Error; err != nil {
			return err
		}
		if current.Status != task.Status || current.PrivateData.ImageTask.Revision != task.PrivateData.ImageTask.Revision {
			return nil
		}
		if current.Status.IsTerminal() {
			return nil
		}
		var err error
		next, err = common.DeepCopy(&current)
		if err != nil {
			return err
		}
		mutate(next.PrivateData.ImageTask)
		next.Status = toStatus
		next.PrivateData.ImageTask.Revision++
		next.BillingState = deriveBillingState(next.PrivateData)
		if toStatus.IsTerminal() {
			next.Progress = "100%"
			if next.FinishTime == 0 {
				next.FinishTime = common.GetTimestamp()
			}
		}
		if err := updateImageTaskSlotsTx(tx, &current, next); err != nil {
			return err
		}
		if err := tx.Model(next).Updates(map[string]any{
			"status": next.Status, "private_data": next.PrivateData,
			"billing_state": next.BillingState, "progress": next.Progress, "finish_time": next.FinishTime,
		}).Error; err != nil {
			return err
		}
		won = true
		return nil
	})
	if err == nil && won {
		*task = *next
	}
	return won && err == nil, err
}

// ─── 查询与领取 ─────────────────────────────────────────────────────────

// GetQueuedImageTasks returns fund-held queued image tasks in FIFO order.
func GetQueuedImageTasks(limit int) []*Task {
	if limit <= 0 {
		return nil
	}
	var tasks []*Task
	err := DB.Where("client_protocol = ? AND status = ? AND progress != ?",
		TaskClientProtocolImageOpenAIV1, TaskStatusQueued, "100%").
		Order("id").
		Limit(limit).
		Find(&tasks).Error
	if err != nil {
		return nil
	}
	return tasks
}

// ClaimImageTask CAS-claims one queued task for execution, reserving global
// and per-channel execution slots in the same transaction.
func ClaimImageTask(taskID string, globalScope string, globalLimit int, channelScope string, channelLimit int) (*Task, bool, error) {
	return claimImageTask(taskID, TaskStatusQueued, globalScope, globalLimit, channelScope, channelLimit)
}

// ClaimImageTaskRecovery reserves the same execution capacity; recovery can only query or store.
func ClaimImageTaskRecovery(taskID string, globalScope string, globalLimit int, channelScope string, channelLimit int) (*Task, bool, error) {
	return claimImageTask(taskID, TaskStatusReconciliationRequired, globalScope, globalLimit, channelScope, channelLimit)
}

func claimImageTask(taskID string, from TaskStatus, globalScope string, globalLimit int, channelScope string, channelLimit int) (*Task, bool, error) {
	var claimed *Task
	wonClaim := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockImageTaskSlotsTx(tx); err != nil {
			return err
		}
		var task Task
		if err := lockForUpdate(tx).Where("task_id = ? AND client_protocol = ?", taskID, TaskClientProtocolImageOpenAIV1).First(&task).Error; err != nil {
			return err
		}
		if task.Status != from || task.PrivateData.ImageTask == nil || !task.PrivateData.ImageTask.FundsHeld {
			claimed = &task
			return nil
		}
		if err := reserveImageSlotsTx(tx, globalScope, globalLimit, channelScope, channelLimit); err != nil {
			return err
		}
		task.Status = TaskStatusInProgress
		task.PrivateData.ImageTask.Revision++
		task.PrivateData.ImageTask.LeaseAt = common.GetTimestamp()
		result := tx.Model(&Task{}).Where("id = ? AND status = ?", task.ID, from).
			Updates(map[string]any{"status": task.Status, "private_data": task.PrivateData})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			// CAS 失败：返回错误回滚事务，避免已保留的执行槽位计数泄漏。
			return errImageClaimLost
		}
		claimed = &task
		wonClaim = true
		return nil
	})
	if err != nil || claimed == nil {
		return nil, false, err
	}
	return claimed, wonClaim, nil
}

var errImageClaimLost = errors.New("image task claim lost the race")

// IsImageTaskClaimLost reports a benign lost race in ClaimImageTask.
func IsImageTaskClaimLost(err error) bool { return errors.Is(err, errImageClaimLost) }

// RequeueImageTask releases a never-sent claim back to the queue.
func RequeueImageTask(task *Task) (bool, error) {
	if task == nil || task.PrivateData.ImageTask == nil || task.PrivateData.ImageTask.SentAt != 0 {
		return false, errors.New("sent image task cannot be requeued")
	}
	return transitionImageTask(task, TaskStatusQueued, func(data *TaskImageExecutionData) { data.LeaseAt = 0 })
}

// GetStalledImageTasks returns in-progress image tasks whose lease expired.
func GetStalledImageTasks(leaseDeadline int64, limit int) []*Task {
	var tasks []*Task
	err := DB.Where("client_protocol = ? AND status = ?",
		TaskClientProtocolImageOpenAIV1, TaskStatusInProgress).
		Where("updated_at < ?", leaseDeadline).
		Order("id").Limit(limit).Find(&tasks).Error
	if err != nil {
		return nil
	}
	filtered := make([]*Task, 0, len(tasks))
	for _, task := range tasks {
		if task.PrivateData.ImageTask != nil && task.PrivateData.ImageTask.LeaseAt > 0 && task.PrivateData.ImageTask.LeaseAt < leaseDeadline {
			filtered = append(filtered, task)
		}
	}
	return filtered
}

// GetExpiredQueuedImageTasks returns queued image tasks past their queue
// deadline（已证实从未发送，可安全失败并退款）。
func GetExpiredQueuedImageTasks(now int64, limit int) []*Task {
	var tasks []*Task
	err := DB.Where("client_protocol = ? AND status = ?",
		TaskClientProtocolImageOpenAIV1, TaskStatusQueued).
		Order("id").Limit(limit).Find(&tasks).Error
	if err != nil {
		return nil
	}
	filtered := make([]*Task, 0, len(tasks))
	for _, task := range tasks {
		if data := task.PrivateData.ImageTask; data != nil && data.FundsHeld && data.QueueDeadlineAt > 0 && data.QueueDeadlineAt < now {
			filtered = append(filtered, task)
		}
	}
	return filtered
}

// GetReconcilableImageTasks includes stored-result evidence without a Provider ID.
// Page past non-recoverable unknown tasks instead of starving later recoverable rows.
func GetReconcilableImageTasks(limit int) []*Task {
	if limit <= 0 {
		return nil
	}
	filtered := make([]*Task, 0, limit)
	var cursor, updatedAt int64
	for len(filtered) < limit {
		var tasks []*Task
		query := DB.Where("client_protocol = ? AND status = ?", TaskClientProtocolImageOpenAIV1, TaskStatusReconciliationRequired)
		if cursor != 0 {
			query = query.Where("updated_at > ? OR (updated_at = ? AND id > ?)", updatedAt, updatedAt, cursor)
		}
		// Each recovery updates the task. Oldest observations go first so an
		// inconclusive batch cannot monopolize subsequent worker passes.
		err := query.Order("updated_at").Order("id").Limit(limit).Find(&tasks).Error
		if err != nil {
			return nil
		}
		for _, task := range tasks {
			cursor, updatedAt = task.ID, task.UpdatedAt
			if data := task.PrivateData.ImageTask; data != nil && (strings.TrimSpace(data.ProviderTaskID) != "" || data.GenerationComplete) {
				filtered = append(filtered, task)
				if len(filtered) == limit {
					break
				}
			}
		}
		if len(tasks) < limit {
			break
		}
	}
	return filtered
}

// HasImageTaskWork reports whether the image worker should schedule a pass.
func HasImageTaskWork() bool {
	var id int64
	err := DB.Model(&Task{}).
		Where("client_protocol = ? AND status IN ?",
			TaskClientProtocolImageOpenAIV1,
			[]TaskStatus{TaskStatusQueued, TaskStatusInProgress, TaskStatusReconciliationRequired}).
		Limit(1).
		Pluck("id", &id).Error
	return err == nil && id != 0
}

// ImageTaskImagePlatform derives the platform label（与视频任务一致使用
// 数值渠道类型字符串）。
func ImageTaskPlatform(channelType int) constant.TaskPlatform {
	return constant.TaskPlatform(strconv.Itoa(channelType))
}

// ImageTaskQueueDeadline computes the admission-time queue deadline.
func ImageTaskQueueDeadline(waitSeconds int64) int64 {
	if waitSeconds <= 0 {
		waitSeconds = int64((30 * time.Minute).Seconds())
	}
	return common.GetTimestamp() + waitSeconds
}

// DescribeImageTaskScopeKey builds deterministic admission scope keys.
func ImageTaskAdmissionScopeGlobal() string { return "accept:global" }

func ImageTaskAdmissionScopeApp(userID, appID int) string {
	return fmt.Sprintf("accept:app:%d:%d", userID, appID)
}

func ImageTaskExecutionScopeGlobal() string { return "exec:global" }

func ImageTaskExecutionScopeChannel(channelID int) string {
	return fmt.Sprintf("exec:channel:%d", channelID)
}

// SanitizeImageTaskForClientError keeps provider/internal detail out of the
// customer-facing failure message (V4/R5)。
func SanitizeImageTaskForClientError(failureCode string) string {
	code := strings.TrimSpace(failureCode)
	if code == "" {
		return "image task failed"
	}
	if len(code) > 64 {
		code = code[:64]
	}
	return "image task failed: " + code
}
