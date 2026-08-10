package model

import (
	"errors"
	"strings"
	"unicode"

	"github.com/QuantumNous/new-api/dto"
	"gorm.io/gorm"
)

const TaskClientProtocolOpenAIImages = "openai_images"

type TaskMediaImagePrivateData struct {
	Protocol            string     `json:"protocol,omitempty"`
	QueryBaseURL        string     `json:"query_base_url,omitempty"`
	QueryPathTemplate   string     `json:"query_path_template,omitempty"`
	Proxy               string     `json:"proxy,omitempty"`
	AuthType            string     `json:"auth_type,omitempty"`
	AuthName            string     `json:"auth_name,omitempty"`
	AuthValueTemplate   string     `json:"auth_value_template,omitempty"`
	ResponseFormat      string     `json:"response_format,omitempty"`
	RequestedImageCount uint       `json:"requested_image_count,omitempty"`
	ResultURLs          []string   `json:"result_urls,omitempty"`
	CreateRequestID     string     `json:"create_request_id,omitempty"`
	LastPollRequestID   string     `json:"last_poll_request_id,omitempty"`
	PollAttempts        int        `json:"poll_attempts,omitempty"`
	Usage               *dto.Usage `json:"usage,omitempty"`
	UsePrice            bool       `json:"use_price,omitempty"`
	UsageBillingEnabled bool       `json:"usage_billing_enabled,omitempty"`
}

func GetTaskForProtocol(userID int, taskID, protocol string, includeDeleted bool) (*Task, bool, error) {
	if userID <= 0 || strings.TrimSpace(taskID) == "" || strings.TrimSpace(protocol) == "" {
		return nil, false, nil
	}
	query := DB.Where("user_id = ? AND task_id = ? AND client_protocol = ?", userID, strings.TrimSpace(taskID), protocol)
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

func ProjectOpenAIImageTask(task *Task) *dto.ImageTask {
	projected := &dto.ImageTask{Object: dto.ImageTaskObject, Status: dto.ImageTaskStatusUnknown}
	if task == nil {
		projected.Error = &dto.ImageTaskError{Code: "internal_error", Message: "Image task state is unavailable"}
		return projected
	}

	projected.ID = task.TaskID
	projected.CreatedAt = task.CreatedAt
	projected.Model = task.Properties.OriginModelName
	switch task.Status {
	case TaskStatusNotStart, TaskStatusSubmitted, TaskStatusQueued:
		projected.Status = dto.ImageTaskStatusQueued
	case TaskStatusInProgress:
		projected.Status = dto.ImageTaskStatusInProgress
	case TaskStatusSuccess:
		projected.Status = dto.ImageTaskStatusCompleted
	case TaskStatusFailure, TaskStatusProviderContractFailure, TaskStatusCancelled, TaskStatusExpired:
		projected.Status = dto.ImageTaskStatusFailed
	default:
		projected.Status = dto.ImageTaskStatusUnknown
	}

	if task.Status.IsTerminal() {
		projected.CompletedAt = task.FinishTime
		if projected.CompletedAt == 0 {
			projected.CompletedAt = task.UpdatedAt
		}
	}
	if task.Status == TaskStatusSuccess && task.PrivateData.MediaImage != nil {
		data := make([]dto.ImageData, 0, len(task.PrivateData.MediaImage.ResultURLs))
		for _, resultURL := range task.PrivateData.MediaImage.ResultURLs {
			data = append(data, dto.ImageData{Url: resultURL})
		}
		projected.Result = &dto.ImageTaskResult{Created: projected.CompletedAt, Data: data}
	}
	switch task.Status {
	case TaskStatusProviderContractFailure:
		projected.Error = &dto.ImageTaskError{Code: "provider_contract_failure", Message: "The provider result could not be delivered safely"}
	case TaskStatusFailure:
		projected.Error = &dto.ImageTaskError{Code: "image_generation_failed", Message: publicImageTaskFailure(task.FailReason)}
	case TaskStatusCancelled:
		projected.Error = &dto.ImageTaskError{Code: "cancelled", Message: "Image generation was cancelled"}
	case TaskStatusExpired:
		projected.Error = &dto.ImageTaskError{Code: "expired", Message: "Image generation expired"}
	}
	return projected
}

func publicImageTaskFailure(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return "Image generation failed"
	}
	lower := strings.ToLower(reason)
	for _, sensitive := range []string{"http://", "https://", "bearer ", "api_key", "api-key", "authorization", "cookie"} {
		if strings.Contains(lower, sensitive) {
			return "Image generation failed"
		}
	}
	runes := []rune(reason)
	if len(runes) > 512 {
		runes = runes[:512]
	}
	for i, value := range runes {
		if unicode.IsControl(value) {
			runes[i] = ' '
		}
	}
	return strings.TrimSpace(string(runes))
}
