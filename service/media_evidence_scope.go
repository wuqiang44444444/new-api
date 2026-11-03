package service

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// Business route scope only; this never selects a channel or infers Link identity.
func mediaEvidenceRequestKind(method, path string) string {
	switch {
	case method == http.MethodGet && strings.HasPrefix(path, "/v1/videos/") && strings.HasSuffix(path, "/content"):
		return "video_delivery" // Body is excluded independently of request eligibility.
	case method == http.MethodGet && strings.HasPrefix(path, "/v1/tasks/"):
		return "task_query"
	case method == http.MethodPost && strings.HasPrefix(path, "/v1/tasks/") && !strings.Contains(strings.TrimPrefix(path, "/v1/tasks/"), "/"):
		return "task_submit"
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
	for _, prefix := range []string{"/v1/", "/v1beta/", "/v1alpha/", "/v1beta1/", "/pg/", "/suno/", "/api/v3/contents/generations/"} {
		if strings.HasPrefix(path, prefix) {
			return "api_request"
		}
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

// Eligibility comes from the business route, never a model name or parsed body.
func mediaEvidenceContextKind(c *gin.Context) string {
	return mediaEvidenceRequestKind(c.Request.Method, c.Request.URL.Path)
}
