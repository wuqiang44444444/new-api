package model

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"gorm.io/gorm"
)

const (
	TaskClientProtocolOpenAIVideos  = "openai_videos"
	TaskClientProtocolModelArkV3    = "modelark_v3"
	TaskClientProtocolKlingV1       = "kling_v1"
	TaskClientProtocolJimeng        = "jimeng_official"
	TaskClientProtocolPlatformVideo = "platform_video"

	TaskCancellationStateRequested = "requested"
	TaskCancellationStateUnknown   = "unknown"
	TaskCancellationStateConfirmed = "confirmed"
	TaskCancellationStateRejected  = "rejected"
)

type TaskClientRequestSnapshot struct {
	Prompt             string `json:"prompt,omitempty"`
	Seconds            string `json:"seconds,omitempty"`
	Size               string `json:"size,omitempty"`
	RemixedFromVideoID string `json:"remixed_from_video_id,omitempty"`
	ServiceTier        string `json:"service_tier,omitempty"`
}

func (s TaskStatus) IsActive() bool {
	switch s {
	case TaskStatusNotStart, TaskStatusSubmitted, TaskStatusQueued, TaskStatusInProgress, TaskStatusUnknown:
		return true
	default:
		return false
	}
}

func (s TaskStatus) IsTerminal() bool {
	switch s {
	case TaskStatusSuccess, TaskStatusFailure, TaskStatusCancelled, TaskStatusExpired:
		return true
	default:
		return false
	}
}

func (s TaskStatus) CanRequestCancellation() bool {
	return s == TaskStatusNotStart || s == TaskStatusSubmitted || s == TaskStatusQueued
}

func (s TaskStatus) ShouldRefundOnTerminal() bool {
	return s == TaskStatusFailure || s == TaskStatusCancelled || s == TaskStatusExpired
}

func TerminalTaskStatuses() []TaskStatus {
	return []TaskStatus{TaskStatusSuccess, TaskStatusFailure, TaskStatusCancelled, TaskStatusExpired}
}

func ProjectOpenAIVideo(task *Task) *dto.OpenAIVideo {
	video := dto.NewOpenAIVideo()
	if task == nil {
		video.Status = dto.VideoStatusFailed
		video.Error = &dto.OpenAIVideoError{Code: "internal_error", Message: "Video state is unavailable"}
		return video
	}

	video.ID = task.TaskID
	video.Model = task.Properties.OriginModelName
	video.Status = task.Status.ToVideoStatus()
	video.SetProgressStr(task.Progress)
	video.CreatedAt = task.CreatedAt
	video.Prompt = task.PrivateData.ClientRequest.Prompt
	video.Seconds = task.PrivateData.ClientRequest.Seconds
	video.Size = task.PrivateData.ClientRequest.Size
	video.RemixedFromVideoID = task.PrivateData.ClientRequest.RemixedFromVideoID
	if task.Status.IsTerminal() {
		video.CompletedAt = task.FinishTime
		if video.CompletedAt == 0 {
			video.CompletedAt = task.UpdatedAt
		}
	}

	var upstream struct {
		ExpiresAt int64  `json:"expires_at"`
		Seconds   any    `json:"seconds"`
		Size      string `json:"size"`
		Duration  any    `json:"duration"`
	}
	if len(task.Data) > 0 && common.Unmarshal(task.Data, &upstream) == nil {
		video.ExpiresAt = upstream.ExpiresAt
		if video.Size == "" {
			video.Size = upstream.Size
		}
		if video.Seconds == "" {
			video.Seconds = taskVideoSeconds(upstream.Seconds, upstream.Duration)
		}
	}

	switch task.Status {
	case TaskStatusCancelled:
		video.Error = &dto.OpenAIVideoError{Code: "cancelled", Message: "Video generation was cancelled"}
	case TaskStatusExpired:
		video.Error = &dto.OpenAIVideoError{Code: "expired", Message: "Video generation expired"}
	case TaskStatusFailure, TaskStatusUnknown:
		video.Error = &dto.OpenAIVideoError{Code: "video_generation_failed", Message: "Video generation failed"}
	}
	return video
}

func taskVideoSeconds(values ...any) string {
	for _, value := range values {
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				return typed
			}
		case float64:
			return strconv.FormatFloat(typed, 'f', -1, 64)
		case int:
			return strconv.Itoa(typed)
		}
	}
	return ""
}

func GetVideoTaskForProtocol(userID int, taskID, protocol string, includeDeleted bool) (*Task, bool, error) {
	if userID <= 0 || strings.TrimSpace(taskID) == "" {
		return nil, false, nil
	}
	query := DB.Where("user_id = ? AND task_id = ?", userID, strings.TrimSpace(taskID))
	if protocol != "" {
		query = query.Where("client_protocol = ?", protocol)
	}
	if !includeDeleted {
		query = query.Where("client_deleted_at = ?", 0)
	}
	var task Task
	err := query.First(&task).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &task, true, nil
}

func GetVisibleVideoTask(userID int, taskID string, protocols ...string) (*Task, bool, error) {
	if userID <= 0 || strings.TrimSpace(taskID) == "" || len(protocols) == 0 {
		return nil, false, nil
	}
	var task Task
	err := DB.Where(
		"user_id = ? AND task_id = ? AND client_protocol IN ? AND client_deleted_at = ?",
		userID,
		strings.TrimSpace(taskID),
		protocols,
		0,
	).First(&task).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &task, true, nil
}

func ListOpenAIVideoTasks(userID int, after string, limit int, order string) ([]Task, bool, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if order != "asc" {
		order = "desc"
	}
	query := DB.Where("user_id = ? AND client_protocol = ? AND client_deleted_at = ?", userID, TaskClientProtocolOpenAIVideos, 0)
	if after != "" {
		var cursor Task
		err := DB.Select("id", "created_at").
			Where("user_id = ? AND task_id = ? AND client_protocol = ? AND client_deleted_at = ?", userID, after, TaskClientProtocolOpenAIVideos, 0).
			First(&cursor).Error
		if err != nil {
			return nil, false, err
		}
		if order == "asc" {
			query = query.Where("created_at > ? OR (created_at = ? AND id > ?)", cursor.CreatedAt, cursor.CreatedAt, cursor.ID)
		} else {
			query = query.Where("created_at < ? OR (created_at = ? AND id < ?)", cursor.CreatedAt, cursor.CreatedAt, cursor.ID)
		}
	}
	direction := "created_at desc, id desc"
	if order == "asc" {
		direction = "created_at asc, id asc"
	}
	var tasks []Task
	if err := query.Order(direction).Limit(limit + 1).Find(&tasks).Error; err != nil {
		return nil, false, err
	}
	hasMore := len(tasks) > limit
	if hasMore {
		tasks = tasks[:limit]
	}
	return tasks, hasMore, nil
}

func MarkVideoTaskClientDeleted(userID int, taskID, protocol string) (bool, error) {
	now := common.GetTimestamp()
	result := DB.Model(&Task{}).
		Where("user_id = ? AND task_id = ? AND client_protocol = ? AND client_deleted_at = ?", userID, taskID, protocol, 0).
		Update("client_deleted_at", now)
	return result.RowsAffected == 1, result.Error
}

type TaskCancellationBeginResult struct {
	Task           *Task
	ShouldCall     bool
	AlreadyPending bool
}

func BeginTaskCancellation(userID int, taskID, protocol string) (*TaskCancellationBeginResult, error) {
	result := &TaskCancellationBeginResult{}
	err := DB.Transaction(func(tx *gorm.DB) error {
		var task Task
		query := lockForUpdate(tx).Where("user_id = ? AND task_id = ? AND client_protocol = ? AND client_deleted_at = ?", userID, taskID, protocol, 0)
		if err := query.First(&task).Error; err != nil {
			return err
		}
		result.Task = &task
		if !task.Status.CanRequestCancellation() {
			return nil
		}
		if task.CancellationState == TaskCancellationStateRequested || task.CancellationState == TaskCancellationStateUnknown {
			result.AlreadyPending = true
			return nil
		}
		now := common.GetTimestamp()
		if err := tx.Model(&task).Updates(map[string]any{
			"cancellation_state":        TaskCancellationStateRequested,
			"cancellation_requested_at": now,
			"cancellation_error":        "",
			"updated_at":                now,
		}).Error; err != nil {
			return err
		}
		task.CancellationState = TaskCancellationStateRequested
		task.CancellationRequestedAt = now
		task.CancellationError = ""
		result.ShouldCall = true
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, gorm.ErrRecordNotFound
	}
	return result, err
}

func CompleteTaskCancellation(taskID int64, confirmed bool, rejected bool, operationErr string) (*Task, bool, error) {
	var saved Task
	wonTerminal := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where("id = ?", taskID).First(&saved).Error; err != nil {
			return err
		}
		now := common.GetTimestamp()
		state := TaskCancellationStateUnknown
		if confirmed && saved.Status.IsTerminal() && saved.Status != TaskStatusCancelled {
			confirmed = false
			rejected = true
			if operationErr == "" {
				operationErr = "task already reached a different terminal state"
			}
		}
		if confirmed {
			state = TaskCancellationStateConfirmed
		} else if rejected {
			state = TaskCancellationStateRejected
		}
		updates := map[string]any{
			"cancellation_state": state,
			"cancellation_error": operationErr,
			"updated_at":         now,
		}
		if confirmed && saved.Status.CanRequestCancellation() {
			updates["status"] = TaskStatusCancelled
			updates["progress"] = "100%"
			updates["finish_time"] = now
			updates["cancellation_completed_at"] = now
			saved.Status = TaskStatusCancelled
			saved.Progress = "100%"
			saved.FinishTime = now
			saved.CancellationCompletedAt = now
			wonTerminal = true
		}
		if err := tx.Model(&saved).Updates(updates).Error; err != nil {
			return err
		}
		saved.CancellationState = state
		saved.CancellationError = operationErr
		saved.UpdatedAt = now
		return nil
	})
	return &saved, wonTerminal, err
}

func ValidateTaskProtocol(protocol string) error {
	switch protocol {
	case TaskClientProtocolOpenAIVideos, TaskClientProtocolModelArkV3, TaskClientProtocolKlingV1, TaskClientProtocolJimeng, TaskClientProtocolPlatformVideo, TaskClientProtocolOpenAIImages:
		return nil
	default:
		return fmt.Errorf("unsupported task client protocol %q", protocol)
	}
}
