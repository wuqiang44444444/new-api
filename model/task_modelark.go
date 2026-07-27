package model

import (
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

const ModelArkTaskListWindowSeconds int64 = 7 * 24 * 60 * 60

type ModelArkTaskListFilter struct {
	Statuses    []TaskStatus
	TaskIDs     []string
	Model       string
	ServiceTier string
	PageNum     int
	PageSize    int
	Now         int64
}

func ListModelArkVideoTasks(userID int, filter ModelArkTaskListFilter) ([]Task, int64, error) {
	if filter.PageNum <= 0 {
		filter.PageNum = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 10
	}
	if filter.Now <= 0 {
		filter.Now = common.GetTimestamp()
	}
	query := DB.Where(
		"user_id = ? AND client_protocol = ? AND client_deleted_at = ? AND created_at >= ? AND created_at < ?",
		userID,
		TaskClientProtocolModelArkV3,
		0,
		filter.Now-ModelArkTaskListWindowSeconds,
		filter.Now,
	)
	if len(filter.Statuses) > 0 {
		query = query.Where("status IN ?", filter.Statuses)
	}
	if len(filter.TaskIDs) > 0 {
		query = query.Where("task_id IN ?", filter.TaskIDs)
	}
	var candidates []Task
	if err := query.Order("created_at desc, id desc").Find(&candidates).Error; err != nil {
		return nil, 0, err
	}
	modelName := strings.TrimSpace(filter.Model)
	serviceTier := strings.TrimSpace(filter.ServiceTier)
	filtered := make([]Task, 0, len(candidates))
	for i := range candidates {
		task := candidates[i]
		if modelName != "" && task.Properties.OriginModelName != modelName {
			continue
		}
		taskServiceTier := strings.TrimSpace(task.PrivateData.ClientRequest.ServiceTier)
		if taskServiceTier == "" {
			taskServiceTier = "default"
		}
		if serviceTier != "" && taskServiceTier != serviceTier {
			continue
		}
		filtered = append(filtered, task)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].CreatedAt == filtered[j].CreatedAt {
			return filtered[i].ID > filtered[j].ID
		}
		return filtered[i].CreatedAt > filtered[j].CreatedAt
	})
	total := int64(len(filtered))
	start := (filter.PageNum - 1) * filter.PageSize
	if start >= len(filtered) {
		return []Task{}, total, nil
	}
	end := min(start+filter.PageSize, len(filtered))
	return filtered[start:end], total, nil
}

func (t *Task) ToModelArkVideoTask() *dto.ModelArkVideoTask {
	result := &dto.ModelArkVideoTask{
		ID:          t.TaskID,
		Model:       t.Properties.OriginModelName,
		Status:      t.Status.ToModelArkVideoStatus(),
		ServiceTier: t.PrivateData.ClientRequest.ServiceTier,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
	}
	var upstream struct {
		Seed             int64  `json:"seed"`
		Resolution       string `json:"resolution"`
		Duration         int    `json:"duration"`
		Frames           int    `json:"frames"`
		Ratio            string `json:"ratio"`
		FramesPerSecond  int    `json:"framespersecond"`
		GenerateAudio    *bool  `json:"generate_audio"`
		Draft            *bool  `json:"draft"`
		DraftTaskID      string `json:"draft_task_id"`
		SafetyIdentifier string `json:"safety_identifier"`
		Priority         *int   `json:"priority"`
		ServiceTier      string `json:"service_tier"`
		Content          struct {
			LastFrameURL string `json:"last_frame_url"`
		} `json:"content"`
		Usage struct {
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
			ToolUsage        *struct {
				WebSearch int `json:"web_search"`
			} `json:"tool_usage"`
		} `json:"usage"`
	}
	if len(t.Data) > 0 {
		_ = common.Unmarshal(t.Data, &upstream)
	}
	result.Seed = upstream.Seed
	result.Resolution = upstream.Resolution
	result.Duration = upstream.Duration
	result.Frames = upstream.Frames
	result.Ratio = upstream.Ratio
	result.FramesPerSecond = upstream.FramesPerSecond
	result.GenerateAudio = upstream.GenerateAudio
	result.Draft = upstream.Draft
	result.DraftTaskID = upstream.DraftTaskID
	result.SafetyIdentifier = upstream.SafetyIdentifier
	result.Priority = upstream.Priority
	if result.ServiceTier == "" {
		result.ServiceTier = upstream.ServiceTier
	}
	if result.ServiceTier == "" {
		result.ServiceTier = "default"
	}
	if upstream.Usage.CompletionTokens != 0 || upstream.Usage.TotalTokens != 0 || upstream.Usage.ToolUsage != nil {
		result.Usage = &dto.ModelArkVideoTaskUsage{
			CompletionTokens: upstream.Usage.CompletionTokens,
			TotalTokens:      upstream.Usage.TotalTokens,
		}
		if upstream.Usage.ToolUsage != nil {
			result.Usage.ToolUsage = &dto.ModelArkVideoToolUsage{WebSearch: upstream.Usage.ToolUsage.WebSearch}
		}
	}
	switch t.Status {
	case TaskStatusSuccess:
		result.Content = &dto.ModelArkVideoTaskContent{VideoURL: "/v1/videos/" + t.TaskID + "/content"}
		if strings.TrimSpace(upstream.Content.LastFrameURL) != "" {
			result.Content.LastFrameURL = "/v1/videos/" + t.TaskID + "/content?part=last_frame"
		}
	case TaskStatusFailure:
		result.Error = &dto.ModelArkVideoTaskError{Code: "generation_failed", Message: "Video generation failed"}
	case TaskStatusCancelled:
		result.Error = &dto.ModelArkVideoTaskError{Code: "cancelled", Message: "Video generation was cancelled"}
	case TaskStatusExpired:
		result.Error = &dto.ModelArkVideoTaskError{Code: "expired", Message: "Video generation expired"}
	}
	return result
}

func (s TaskStatus) ToModelArkVideoStatus() string {
	switch s {
	case TaskStatusInProgress:
		return "running"
	case TaskStatusSuccess:
		return "succeeded"
	case TaskStatusFailure:
		return "failed"
	case TaskStatusCancelled:
		return "cancelled"
	case TaskStatusExpired:
		return "expired"
	default:
		return "queued"
	}
}

func ModelArkTaskStatuses(status string) ([]TaskStatus, bool) {
	switch strings.TrimSpace(status) {
	case "":
		return nil, true
	case "queued":
		return []TaskStatus{TaskStatusNotStart, TaskStatusSubmitted, TaskStatusQueued, TaskStatusUnknown}, true
	case "running":
		return []TaskStatus{TaskStatusInProgress}, true
	case "succeeded":
		return []TaskStatus{TaskStatusSuccess}, true
	case "failed":
		return []TaskStatus{TaskStatusFailure}, true
	case "cancelled":
		return []TaskStatus{TaskStatusCancelled}, true
	case "expired":
		return []TaskStatus{TaskStatusExpired}, true
	default:
		return nil, false
	}
}
