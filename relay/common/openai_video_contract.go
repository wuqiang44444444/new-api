package common

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/dto"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

func validateOpenAIVideoCreateContract(c *gin.Context, req *TaskSubmitReq) *dto.TaskError {
	if c == nil || req == nil || c.Request == nil || c.Request.URL == nil || c.Request.URL.Path != "/v1/videos" {
		return nil
	}
	if len([]rune(strings.TrimSpace(req.Prompt))) > 32000 {
		return createTaskError(fmt.Errorf("prompt must not exceed 32000 characters"), "invalid_prompt", 400, true)
	}
	if strings.TrimSpace(req.Model) == "" {
		req.Model = "sora-2"
	}
	if req.Seconds == "" && req.Duration == 0 {
		req.Seconds = "4"
	}
	seconds := req.Seconds
	if seconds == "" && req.Duration > 0 {
		seconds = fmt.Sprintf("%d", req.Duration)
	}
	if !lo.Contains([]string{"4", "8", "12"}, seconds) {
		return createTaskError(fmt.Errorf("seconds must be one of 4, 8, or 12"), "invalid_seconds", 400, true)
	}
	if req.Size == "" {
		req.Size = "720x1280"
	}
	if !lo.Contains([]string{"720x1280", "1280x720", "1024x1792", "1792x1024"}, req.Size) {
		return createTaskError(fmt.Errorf("size is not supported"), "invalid_size", 400, true)
	}
	if req.InputReference != "" && strings.HasPrefix(c.GetHeader("Content-Type"), "application/json") && !req.InputReferenceObject {
		return createTaskError(fmt.Errorf("input_reference must be an object with exactly one of file_id or image_url"), "invalid_input_reference", 400, true)
	}
	if req.InputReferenceImageURL != "" {
		if len(req.InputReferenceImageURL) > 20*1024*1024 {
			return createTaskError(fmt.Errorf("input_reference.image_url is too long"), "invalid_input_reference", 400, true)
		}
		if err := validateOpenAIVideoImageReference(req.InputReferenceImageURL); err != nil {
			return createTaskError(err, "invalid_input_reference", 400, true)
		}
	}
	return nil
}

func validateOpenAIVideoImageReference(value string) error {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "data:") || strings.HasPrefix(value, "asset://") {
		return nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil {
		return fmt.Errorf("input_reference.image_url must be an http(s), data, or asset URL")
	}
	return nil
}
