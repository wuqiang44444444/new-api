// Package mediaarrays implements the Seedance media-arrays protocol.
package mediaarrays

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

type createResponse struct {
	ID string `json:"id"`
}

func CreateResponse(body []byte) ([]byte, error) {
	var response createResponse
	if err := common.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("upstream create response is invalid JSON")
	}
	response.ID = strings.TrimSpace(response.ID)
	if response.ID == "" {
		return nil, fmt.Errorf("upstream create response has no id")
	}
	if len(response.ID) > 191 || strings.ContainsFunc(response.ID, unicode.IsControl) {
		return nil, fmt.Errorf("upstream create response has an invalid id")
	}
	return common.Marshal(map[string]any{"id": response.ID})
}

type taskResponse struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	VideoURL string `json:"video_url,omitempty"`
	Error    struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type TaskResponseContext struct {
	BaseURL string
}

func TaskResponse(body []byte, expectedTaskID string, responseContext TaskResponseContext) ([]byte, error) {
	var response taskResponse
	if err := common.Unmarshal(body, &response); err != nil {
		return nil, &relaycommon.UpstreamContractViolation{Reason: "invalid JSON task response"}
	}
	response.ID = strings.TrimSpace(response.ID)
	if response.ID == "" || response.ID != expectedTaskID {
		return nil, &relaycommon.UpstreamContractViolation{Reason: "task id mismatch"}
	}
	result := map[string]any{"id": response.ID}
	switch response.Status {
	case "queued":
		result["status"] = "queued"
	case "processing", "in_progress":
		result["status"] = "running"
	case "completed":
		videoURL, err := relaycommon.ValidateSameOriginVideoResultURL(response.VideoURL, responseContext.BaseURL)
		if err != nil {
			return nil, &relaycommon.UpstreamContractViolation{Reason: "invalid completed video url"}
		}
		result["status"] = "succeeded"
		result["content"] = map[string]any{"video_url": videoURL}
	case "failed":
		result["status"] = "failed"
		result["error"] = map[string]any{"code": sanitize(response.Error.Code, 64), "message": sanitize(response.Error.Message, 500)}
	default:
		return nil, &relaycommon.UpstreamContractViolation{Reason: "unsupported task status"}
	}
	return common.Marshal(result)
}

func sanitize(value string, limit int) string {
	value = strings.TrimSpace(value)
	lower := strings.ToLower(value)
	for _, sensitive := range []string{"http://", "https://", "bearer ", "authorization", "api_key", "api-key", "cookie"} {
		if strings.Contains(lower, sensitive) {
			return "upstream task failed"
		}
	}
	runes := []rune(value)
	if len(runes) > limit {
		runes = runes[:limit]
	}
	for index, character := range runes {
		if unicode.IsControl(character) {
			runes[index] = ' '
		}
	}
	return strings.TrimSpace(string(runes))
}
