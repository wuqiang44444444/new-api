package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Link 视频结果托管登记（本地扩展）。每个结果部件（主视频/尾帧）一行，
// 持久记录待转存、对象位置、MIME、大小与交付进度/重试事实；它是唯一交付
// 事实源，不冒用 ImageTask 登记，也不建立第二套 Provider 结果事实——上游
// 结果 URL 仍只保存在 Task.PrivateData.ResultURL。

const (
	TaskVideoResultPartVideo     = "video"
	TaskVideoResultPartLastFrame = "last_frame"

	// TaskVideoResultStatusPending 已登记待转存；Transferring 已被 worker
	// 领取（ClaimExpiresAt 内有效）；Ready 对象已登记可签发；Unavailable
	// 上游结果失效或转存永久失败，如实表达交付不可用。
	TaskVideoResultStatusPending      = "pending"
	TaskVideoResultStatusTransferring = "transferring"
	TaskVideoResultStatusReady        = "ready"
	TaskVideoResultStatusUnavailable  = "unavailable"

	// VideoResultPresignTTLSeconds 是结果下载预签名 GET URL 的固定有效期：
	// 从每次签发时刻起 24 小时（86400 秒），与任务创建或生成完成时间无关。
	VideoResultPresignTTLSeconds = 86400
)

// VideoResultStorageLocation 记录对象归属的存储实例。对象位置由平台生成，
// 凭据不落库；当前配置与登记位置不一致时失败关闭，不换桶解释。
type VideoResultStorageLocation struct {
	Backend  string `json:"backend" gorm:"type:varchar(31)"`
	Endpoint string `json:"endpoint" gorm:"type:varchar(512)"`
	Bucket   string `json:"bucket" gorm:"type:varchar(255)"`
	Prefix   string `json:"prefix" gorm:"type:varchar(512)"`
}

type TaskVideoResultObject struct {
	ID     int64  `json:"id" gorm:"primary_key"`
	TaskID string `json:"task_id" gorm:"type:varchar(64);uniqueIndex:uix_video_result_task_part,priority:1;index"`
	Part   string `json:"part" gorm:"type:varchar(31);uniqueIndex:uix_video_result_task_part,priority:2"`
	UserID int    `json:"user_id" gorm:"index;not null;autoIncrement:false"`
	AppID  int    `json:"app_id" gorm:"index"`
	Status string `json:"status" gorm:"type:varchar(31);not null;index"`
	// Location 冻结上传完成时的存储实例；签名前必须与当前配置完全一致。
	Location  VideoResultStorageLocation `json:"location" gorm:"embedded;embeddedPrefix:storage_"`
	ObjectKey string                     `json:"object_key" gorm:"type:varchar(512)"`
	MimeType  string                     `json:"mime_type" gorm:"type:varchar(127)"`
	SizeBytes int64                      `json:"size_bytes" gorm:"bigint"`
	// Attempts 累计领取次数；NextAttemptAt 是下次允许领取的时间（重试退避）。
	Attempts      int    `json:"attempts" gorm:"not null;default:0"`
	NextAttemptAt int64  `json:"next_attempt_at" gorm:"bigint;index"`
	LastError     string `json:"last_error" gorm:"type:varchar(512)"`
	// ClaimedBy / ClaimExpiresAt 是行级领取租约；过期后其他 worker 可重新领取。
	ClaimedBy      string `json:"claimed_by" gorm:"type:varchar(128)"`
	ClaimExpiresAt int64  `json:"claim_expires_at" gorm:"bigint"`
	CreatedAt      int64  `json:"created_at" gorm:"bigint;not null"`
	UpdatedAt      int64  `json:"updated_at" gorm:"bigint;not null"`
}

func (TaskVideoResultObject) TableName() string { return "task_video_result_objects" }

// VideoTaskResultParts 返回该任务需要托管的结果部件。主视频固定登记；
// 尾帧只有上游响应实际携带 content.last_frame_url 时才登记。
func VideoTaskResultParts(t *Task) []string {
	parts := []string{TaskVideoResultPartVideo}
	var upstream struct {
		Content struct {
			LastFrameURL string `json:"last_frame_url"`
		} `json:"content"`
	}
	if t != nil && len(t.Data) > 0 {
		_ = common.Unmarshal(t.Data, &upstream)
	}
	if strings.TrimSpace(upstream.Content.LastFrameURL) != "" {
		parts = append(parts, TaskVideoResultPartLastFrame)
	}
	return parts
}

// RegisterVideoResultObjects 幂等登记待转存结果：仅补建缺失的 (task, part)
// 行，已存在的行不做任何修改，重复调用与并发调用都安全。
func RegisterVideoResultObjects(task *Task, parts []string) error {
	if task == nil || strings.TrimSpace(task.TaskID) == "" {
		return errors.New("video result registration requires a task")
	}
	valid := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == TaskVideoResultPartVideo || part == TaskVideoResultPartLastFrame {
			valid = append(valid, part)
		}
	}
	if len(valid) == 0 {
		return errors.New("video result registration requires at least one part")
	}
	now := common.GetTimestamp()
	rows := make([]TaskVideoResultObject, 0, len(valid))
	known := make(map[string]bool, len(valid))
	for _, part := range valid {
		if known[part] {
			continue
		}
		known[part] = true
		rows = append(rows, TaskVideoResultObject{
			TaskID: task.TaskID, Part: part, UserID: task.UserId, AppID: task.AppID,
			Status: TaskVideoResultStatusPending, NextAttemptAt: now, CreatedAt: now, UpdatedAt: now,
		})
	}
	// 全部部件原子提交；冲突只保留既有登记，不在失败事务中再查询。
	return DB.Transaction(func(tx *gorm.DB) error {
		return tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "task_id"}, {Name: "part"}}, DoNothing: true,
		}).Create(&rows).Error
	})
}

// ListVideoResultObjects 返回任务的全部结果对象登记（调用方已验证任务归属）。
func ListVideoResultObjects(taskID string) ([]TaskVideoResultObject, error) {
	if strings.TrimSpace(taskID) == "" {
		return nil, nil
	}
	var rows []TaskVideoResultObject
	err := DB.Where("task_id = ?", strings.TrimSpace(taskID)).Order("id asc").Find(&rows).Error
	return rows, err
}

// ClaimVideoResultObjects 领取最多 limit 个可执行行：pending、领取租约过期
// 的 transferring，且到达 NextAttemptAt。行级 CAS UPDATE 保证多节点并发领取
// 时每行只被一个 worker 取得。
func ClaimVideoResultObjects(limit int, runnerID string, now int64, claimTTLSeconds int64) ([]TaskVideoResultObject, error) {
	if limit <= 0 {
		return nil, nil
	}
	var candidates []TaskVideoResultObject
	err := DB.Where(
		"next_attempt_at <= ? AND (status = ? OR (status = ? AND claim_expires_at < ?))",
		now, TaskVideoResultStatusPending, TaskVideoResultStatusTransferring, now,
	).Order("id asc").Limit(limit).Find(&candidates).Error
	if err != nil {
		return nil, err
	}
	claimed := make([]TaskVideoResultObject, 0, len(candidates))
	for i := range candidates {
		candidate := candidates[i]
		result := DB.Model(&TaskVideoResultObject{}).
			Where(
				"id = ? AND next_attempt_at <= ? AND (status = ? OR (status = ? AND claim_expires_at < ?))",
				candidate.ID, now, TaskVideoResultStatusPending, TaskVideoResultStatusTransferring, now,
			).
			Updates(map[string]any{
				"status":           TaskVideoResultStatusTransferring,
				"claimed_by":       runnerID,
				"claim_expires_at": now + claimTTLSeconds,
				"attempts":         gorm.Expr("attempts + 1"),
				"updated_at":       now,
			})
		if result.Error != nil {
			return claimed, result.Error
		}
		if result.RowsAffected != 1 {
			continue
		}
		var row TaskVideoResultObject
		if err := DB.Where("id = ?", candidate.ID).First(&row).Error; err != nil {
			return claimed, err
		}
		claimed = append(claimed, row)
	}
	return claimed, nil
}

func videoResultCompletionScope(objectID int64, runnerID string) *gorm.DB {
	return DB.Model(&TaskVideoResultObject{}).
		Where("id = ? AND status = ? AND claimed_by = ?", objectID, TaskVideoResultStatusTransferring, runnerID)
}

// FinishVideoResultObjectReady 在领取租约内提交对象登记事实：CAS 保证只有
// 领取者能写入，重复提交（上传成功但提交结果未知后重试）是幂等覆盖。
func FinishVideoResultObjectReady(objectID int64, runnerID string, location VideoResultStorageLocation, objectKey, mimeType string, sizeBytes int64, now int64) (bool, error) {
	result := videoResultCompletionScope(objectID, runnerID).Updates(map[string]any{
		"status":           TaskVideoResultStatusReady,
		"storage_backend":  location.Backend,
		"storage_endpoint": location.Endpoint,
		"storage_bucket":   location.Bucket,
		"storage_prefix":   location.Prefix,
		"object_key":       objectKey,
		"mime_type":        mimeType,
		"size_bytes":       sizeBytes,
		"last_error":       "",
		"claimed_by":       "",
		"updated_at":       now,
		"next_attempt_at":  int64(0),
	})
	return result.RowsAffected == 1, result.Error
}

// RescheduleVideoResultObjectRetry 把可重试失败退回 pending 并按退避时间再领
// 取；重试只针对同一任务同一结果的读取或上传。
func RescheduleVideoResultObjectRetry(objectID int64, runnerID string, nextAttemptAt int64, sanitizedError string, now int64) (bool, error) {
	result := videoResultCompletionScope(objectID, runnerID).Updates(map[string]any{
		"status":           TaskVideoResultStatusPending,
		"claimed_by":       "",
		"claim_expires_at": int64(0),
		"next_attempt_at":  nextAttemptAt,
		"last_error":       sanitizedError,
		"updated_at":       now,
	})
	return result.RowsAffected == 1, result.Error
}

// FinishVideoResultObjectUnavailable 如实记录交付不可用（上游失效、超出重试
// 上限或任务已删除），不改任务状态、资金事实，也不回退旧代理。
func FinishVideoResultObjectUnavailable(objectID int64, runnerID string, sanitizedError string, now int64) (bool, error) {
	result := videoResultCompletionScope(objectID, runnerID).Updates(map[string]any{
		"status":           TaskVideoResultStatusUnavailable,
		"claimed_by":       "",
		"claim_expires_at": int64(0),
		"last_error":       sanitizedError,
		"updated_at":       now,
	})
	return result.RowsAffected == 1, result.Error
}

// CountVideoResultTransferWork 统计待执行行数（pending 或租约过期的
// transferring），供调度器判断是否有工作。
func CountVideoResultTransferWork(now int64) (int64, error) {
	var count int64
	err := DB.Model(&TaskVideoResultObject{}).
		Where(
			"next_attempt_at <= ? AND (status = ? OR (status = ? AND claim_expires_at < ?))",
			now, TaskVideoResultStatusPending, TaskVideoResultStatusTransferring, now,
		).Count(&count).Error
	return count, err
}

// FindVideoResultRegistrationGaps 返回完成窗口内成功且缺少任一实际结果部件的
// 范围内 Link 视频任务，用于补登记（进程崩溃或登记提交失败后的持久恢复）。
// 窗口之外的存量任务不自动迁移，其范围由部署方上线前盘点决定。
func FindVideoResultRegistrationGaps(platforms []constant.TaskPlatform, windowStart int64, limit int) ([]Task, error) {
	if len(platforms) == 0 || limit <= 0 {
		return nil, nil
	}
	var gaps []Task
	cursor := int64(0)
	for len(gaps) < limit {
		var candidates []Task
		err := DB.Where(
			"status = ? AND client_protocol = ? AND client_deleted_at = ? AND finish_time >= ? AND platform IN ?",
			TaskStatusSuccess, TaskClientProtocolModelArkV3, 0, windowStart, platforms,
		).Where("tasks.id > ?", cursor).
			Where("NOT EXISTS (SELECT 1 FROM task_video_result_objects v WHERE v.task_id = tasks.task_id AND v.part = ?) OR NOT EXISTS (SELECT 1 FROM task_video_result_objects v WHERE v.task_id = tasks.task_id AND v.part = ?)", TaskVideoResultPartVideo, TaskVideoResultPartLastFrame).
			Order("tasks.id asc").Limit(200).Find(&candidates).Error
		if err != nil {
			return nil, err
		}
		if len(candidates) == 0 {
			break
		}
		taskIDs := make([]string, 0, len(candidates))
		for i := range candidates {
			taskIDs = append(taskIDs, candidates[i].TaskID)
		}
		var registered []TaskVideoResultObject
		if err := DB.Select("task_id", "part").Where("task_id IN ?", taskIDs).Find(&registered).Error; err != nil {
			return nil, err
		}
		parts := make(map[string]map[string]bool, len(candidates))
		for _, row := range registered {
			if parts[row.TaskID] == nil {
				parts[row.TaskID] = map[string]bool{}
			}
			parts[row.TaskID][row.Part] = true
		}
		for i := range candidates {
			task := &candidates[i]
			for _, part := range VideoTaskResultParts(task) {
				if !parts[task.TaskID][part] {
					gaps = append(gaps, *task)
					break
				}
			}
			if len(gaps) == limit {
				return gaps, nil
			}
		}
		cursor = candidates[len(candidates)-1].ID
	}
	return gaps, nil
}

// VideoResultReadyObject 返回任务指定部件的 ready 登记；其余状态返回 nil。
func VideoResultReadyObject(rows []TaskVideoResultObject, part string) *TaskVideoResultObject {
	for i := range rows {
		if rows[i].Part == part && rows[i].Status == TaskVideoResultStatusReady {
			return &rows[i]
		}
	}
	return nil
}
