package service

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/gin-gonic/gin"
)

// Media scope describes evidence eligibility only; it never selects a channel
// or infers Link identity. Generic multimodal chat is outside this contract.
func mediaEvidenceRequestKind(method, path string) string {
	switch {
	case method == http.MethodGet && strings.HasPrefix(path, "/v1/videos/") && strings.HasSuffix(path, "/content"):
		return "" // Dedicated delivery recorder stores statistics, never a video copy.
	case method == http.MethodGet && strings.HasPrefix(path, "/v1/tasks/"):
		return "task_query" // Persist only after authenticated frozen Task classification.
	case method == http.MethodPost && strings.HasPrefix(path, "/v1/tasks/") && !strings.Contains(strings.TrimPrefix(path, "/v1/tasks/"), "/"):
		return "task_submit" // Candidate only; the pinned plugin must declare media.
	case method == http.MethodPost && (path == "/v1/images/generations" || path == "/v1/images/edits" || path == "/v1/images/variations" || path == "/v1/edits"):
		return "image_request"
	case method == http.MethodPost && path == "/v1/audio/speech":
		return model.TaskRequestEvidenceKindAudioSpeech
	case method == http.MethodPost && path == "/v1/audio/transcriptions":
		return model.TaskRequestEvidenceKindAudioTranscribe
	case method == http.MethodPost && path == "/v1/audio/translations":
		return model.TaskRequestEvidenceKindAudioTranslate
	case method == http.MethodGet && path == "/v1/realtime":
		return model.TaskRequestEvidenceKindAudioTask
	case (path == "/v1/video/generations" || path == "/v1/videos") && method == http.MethodPost:
		return model.TaskRequestEvidenceKindVideoTask
	case strings.HasPrefix(path, "/v1/video/generations/") || strings.HasPrefix(path, "/v1/videos/"):
		if method == http.MethodGet || method == http.MethodDelete || method == http.MethodPost {
			return model.TaskRequestEvidenceKindVideoTask
		}
	case path == "/api/v3/contents/generations/tasks" || strings.HasPrefix(path, "/api/v3/contents/generations/tasks/"):
		if method == http.MethodPost || method == http.MethodGet || method == http.MethodDelete {
			return model.TaskRequestEvidenceKindVideoTask
		}
	case strings.HasPrefix(path, "/kling/v1/videos/") || path == "/jimeng/":
		if method == http.MethodPost || method == http.MethodGet {
			return model.TaskRequestEvidenceKindVideoTask
		}
	case path == "/v1/assets" || strings.HasPrefix(path, "/v1/assets/") || path == "/v1/asset-groups" || strings.HasPrefix(path, "/v1/asset-groups/"):
		if method == http.MethodPost || method == http.MethodGet || method == http.MethodPatch || method == http.MethodDelete {
			return "media_asset"
		}
	case midjourneyEvidenceOperation(method, path) != "":
		return "image_request"
	case method == http.MethodPost && strings.HasPrefix(path, "/suno/submit/"):
		if strings.EqualFold(strings.TrimPrefix(path, "/suno/submit/"), "music") {
			return model.TaskRequestEvidenceKindAudioTask
		}
	case (method == http.MethodGet && strings.HasPrefix(path, "/suno/fetch/")) || (method == http.MethodPost && path == "/suno/fetch"):
		return "task_query"
	}
	return ""
}

// Match registered MJ operations, including the /:mode/mj routes. Dashboard
// lists and unknown suffixes never gain eligibility by containing /mj/.
func midjourneyEvidenceOperation(method, path string) string {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) > 1 && parts[0] != "mj" && parts[1] == "mj" {
		parts = parts[1:]
	}
	if len(parts) < 3 || parts[0] != "mj" {
		return ""
	}
	if method == http.MethodGet && len(parts) == 4 && parts[1] == "task" && parts[2] != "" && (parts[3] == "fetch" || parts[3] == "image-seed") {
		return "query"
	}
	if method == http.MethodGet && len(parts) == 3 && parts[1] == "image" && parts[2] != "" {
		return "delivery"
	}
	if method != http.MethodPost || len(parts) != 3 {
		return ""
	}
	switch parts[1] + "/" + parts[2] {
	case "task/list-by-condition":
		return "query"
	case "submit/action", "submit/shorten", "submit/modal", "submit/imagine", "submit/change", "submit/simple-change", "submit/describe", "submit/blend", "submit/edits", "submit/video", "insight-face/swap", "submit/upload-discord-images":
		return "create"
	}
	return ""
}

// Generic task paths alone say nothing about media. Use the already pinned
// code protocol declarations, without changing native routing or billing.
func mediaEvidenceContextKind(c *gin.Context) string {
	kind := mediaEvidenceRequestKind(c.Request.Method, c.Request.URL.Path)
	if kind != "task_submit" {
		return kind
	}
	value, _ := c.Get(pluginruntime.ContextKeyPinnedPlugin)
	pinned, ok := value.(pluginruntime.PinnedPlugin)
	if !ok || pinned.Plugin == nil || pinned.Plugin.Meta.Key != c.Param("key") {
		return ""
	}
	var image, video bool
	for _, protocol := range pinned.Plugin.Meta.Protocols {
		image = image || protocol.Name == pluginruntime.ProtocolOpenAIImage
		video = video || protocol.Name == "openai_video"
	}
	if image && video {
		return "media_task"
	}
	if image {
		return "image_request"
	}
	if video {
		return model.TaskRequestEvidenceKindVideoTask
	}
	if pinned.Plugin.Meta.Key == "sunoapi" && strings.EqualFold(c.GetString("task_action"), "music") {
		return model.TaskRequestEvidenceKindAudioTask
	}
	return ""
}

func mediaEvidenceTaskKind(task *model.Task) string {
	if task == nil || task.Platform == constant.TaskPlatformAzureBatch || task.ClientProtocol == "openai_responses" {
		return ""
	}
	if task.ClientProtocol == model.TaskClientProtocolImageOpenAIV1 || task.ClientProtocol == "openai_image" || task.PrivateData.ImageTask != nil {
		return "image_request"
	}
	if task.Platform == constant.TaskPlatformSuno || task.Platform == "sunoapi" {
		if strings.EqualFold(task.Action, "lyrics") {
			return ""
		}
		return model.TaskRequestEvidenceKindAudioTask
	}
	if task.Platform == constant.TaskPlatformMidjourney {
		return "image_request"
	}
	switch task.ClientProtocol {
	case model.TaskClientProtocolModelArkV3, model.TaskClientProtocolKlingV1, model.TaskClientProtocolJimeng, "openai_video":
		return model.TaskRequestEvidenceKindVideoTask
	}
	if task.PrivateData.VideoUpstreamProtocol != "" || task.PrivateData.VideoUpstreamProfile != "" {
		return model.TaskRequestEvidenceKindVideoTask
	}
	if task.PrivateData.Execution != nil {
		return mediaEvidenceRequestKind(http.MethodPost, task.PrivateData.Execution.RequestPath)
	}

	return ""
}

// Reuse the earliest durable index and Task creation time. Deleting an index
// never gives an old task a fresh seven-day window. Missing dates are not now.
func taskMediaEvidenceIndex(task *model.Task) (*model.TaskRequestEvidence, error) {
	if !evidenceEnabled() || task == nil {
		return nil, nil
	}
	kind := mediaEvidenceTaskKind(task)
	if kind == "task_submit" {
		// The creation recorder froze media eligibility on its durable index.
		// Never consult today's plugin registry to reinterpret an old Task.
		index, err := model.FindTaskMediaEvidence(task.TaskID, task.UserId, task.AppID)
		if err != nil || index == nil {
			return nil, err
		}
		kind = index.Kind
	}
	if kind == "" {
		return nil, nil
	}
	index, err := model.GetOrCreateTaskMediaEvidence(task, kind, common.GetTimestamp(), EvidenceRetentionSeconds())
	if err == model.ErrTaskRequestEvidenceCreationTime {
		common.SysError("evidence task creation time unavailable; capture skipped")
		return nil, nil
	}
	if err == model.ErrTaskRequestEvidenceBodyClosed {
		return nil, nil
	}
	return index, err
}
