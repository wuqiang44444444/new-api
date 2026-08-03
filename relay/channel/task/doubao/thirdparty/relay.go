package thirdparty

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

type relayFacadeMedia struct {
	URL string `json:"url"`
}

type relayFacadeContent struct {
	Type     string            `json:"type"`
	Text     string            `json:"text"`
	ImageURL *relayFacadeMedia `json:"image_url"`
	VideoURL *relayFacadeMedia `json:"video_url"`
	AudioURL *relayFacadeMedia `json:"audio_url"`
	Role     string            `json:"role"`
}

type relayFacadeRequest struct {
	Model         string               `json:"model"`
	Content       []relayFacadeContent `json:"content"`
	GenerateAudio *bool                `json:"generate_audio"`
	Resolution    string               `json:"resolution"`
	Ratio         string               `json:"ratio"`
	Duration      *int                 `json:"duration"`
	Seed          *int                 `json:"seed"`
	CameraFixed   *bool                `json:"camera_fixed"`
	Watermark     *bool                `json:"watermark"`
}

type relayCreateRequest struct {
	Model           string   `json:"model"`
	Capability      string   `json:"capability"`
	InputMode       string   `json:"input_mode"`
	ControlMode     string   `json:"control_mode"`
	Prompt          string   `json:"prompt,omitempty"`
	Image           string   `json:"image,omitempty"`
	EndImage        string   `json:"end_image,omitempty"`
	ReferenceImages []string `json:"reference_images,omitempty"`
	DurationSeconds *int     `json:"duration_seconds,omitempty"`
	WithAudio       *bool    `json:"with_audio,omitempty"`
	Resolution      string   `json:"resolution,omitempty"`
	AspectRatio     string   `json:"aspect_ratio,omitempty"`
	Seed            *int     `json:"seed,omitempty"`
	CameraFixed     *bool    `json:"camera_fixed,omitempty"`
	Watermark       *bool    `json:"watermark,omitempty"`
}

// RelayCreateRequest 把现有 DoubaoVideo 请求转换为第三方中转的统一媒体任务合同。
// 不支持的媒体类型或冲突的首帧、尾帧、参考图组合 fail closed（方案 §3.2）。
func RelayCreateRequest(body []byte) ([]byte, error) {
	var input relayFacadeRequest
	if err := common.Unmarshal(body, &input); err != nil {
		return nil, fmt.Errorf("invalid JSON request")
	}
	if strings.TrimSpace(input.Model) == "" {
		return nil, fmt.Errorf("model is required")
	}

	output := relayCreateRequest{
		Model:           strings.TrimSpace(input.Model),
		Capability:      "video_generation",
		InputMode:       "text",
		ControlMode:     "none",
		DurationSeconds: input.Duration,
		WithAudio:       input.GenerateAudio,
		Resolution:      input.Resolution,
		AspectRatio:     input.Ratio,
		Seed:            input.Seed,
		CameraFixed:     input.CameraFixed,
		Watermark:       input.Watermark,
	}
	var firstFrameImages []string
	for _, item := range input.Content {
		switch strings.ToLower(strings.TrimSpace(item.Type)) {
		case "", "text":
			if text := strings.TrimSpace(item.Text); text != "" {
				if output.Prompt != "" {
					output.Prompt += "\n"
				}
				output.Prompt += text
			}
		case "image_url":
			if item.ImageURL == nil || strings.TrimSpace(item.ImageURL.URL) == "" {
				return nil, fmt.Errorf("image_url.url is required")
			}
			mediaURL := strings.TrimSpace(item.ImageURL.URL)
			switch strings.ToLower(strings.TrimSpace(item.Role)) {
			case "", "first_frame":
				firstFrameImages = append(firstFrameImages, mediaURL)
			case "last_frame":
				if output.EndImage != "" {
					return nil, fmt.Errorf("only one last_frame image is supported")
				}
				output.EndImage = mediaURL
			case "reference_image":
				output.ReferenceImages = append(output.ReferenceImages, mediaURL)
			default:
				return nil, fmt.Errorf("unsupported image role %q", item.Role)
			}
		case "video_url", "audio_url":
			return nil, fmt.Errorf("third-party relay does not accept %s content through this adaptor", item.Type)
		default:
			return nil, fmt.Errorf("unsupported content type %q", item.Type)
		}
	}

	if len(firstFrameImages) > 1 {
		return nil, fmt.Errorf("multiple first-frame images require role=reference_image")
	}
	if len(firstFrameImages) == 1 {
		output.Image = firstFrameImages[0]
	}
	if output.EndImage != "" && output.Image == "" {
		return nil, fmt.Errorf("last_frame requires a first_frame image")
	}
	if output.EndImage != "" && len(output.ReferenceImages) > 0 {
		return nil, fmt.Errorf("end-frame and reference-image controls cannot be combined")
	}

	switch {
	case len(output.ReferenceImages) > 0:
		output.InputMode = "multi_image"
		output.ControlMode = "reference"
		if output.Image != "" {
			output.ReferenceImages = append([]string{output.Image}, output.ReferenceImages...)
			output.Image = ""
		}
	case output.EndImage != "":
		output.InputMode = "multi_image"
		output.ControlMode = "end_frame"
	case output.Image != "":
		output.InputMode = "single_image"
	}
	if output.Prompt == "" && output.Image == "" && len(output.ReferenceImages) == 0 {
		return nil, fmt.Errorf("prompt or image content is required")
	}
	return common.Marshal(output)
}

// RelayCreateResponse 归一化第三方中转创建响应的 task_id 到 DoubaoVideo adaptor 内部 {"id": ...} 合同。
func RelayCreateResponse(body []byte) ([]byte, error) {
	root, err := object(body)
	if err != nil {
		return nil, err
	}
	data := unwrapData(root)
	taskID := firstString(data, "task_id", "id")
	if taskID == "" {
		taskID = firstString(root, "task_id", "id")
	}
	if taskID == "" {
		return nil, fmt.Errorf("upstream create response has no task id")
	}
	return common.Marshal(map[string]any{"id": taskID})
}

// RelayTaskResponse 归一化第三方中转任务状态与结果字段到现有 DoubaoVideo 轮询合同。
// usage 透传 completion_tokens/total_tokens（与反代协议 reverse_proxy.go 一致），用于按 token 结算；
// 上游终态真实返回 usage 已由实测验证（方案 §10.2②），不再刻意丢弃。
func RelayTaskResponse(body []byte) ([]byte, error) {
	root, err := object(body)
	if err != nil {
		return nil, err
	}
	data := unwrapData(root)
	status, err := normalizeRelayStatus(firstString(data, "status", "state"))
	if err != nil {
		return nil, err
	}
	result := map[string]any{
		"id":     firstString(data, "task_id", "id"),
		"status": status,
	}
	if model := firstString(data, "model"); model != "" {
		result["model"] = model
	}

	mediaResult := mapValue(data["result"])
	videoURL := firstString(mediaResult, "primary_url")
	if videoURL == "" {
		if urls, ok := mediaResult["urls"].([]any); ok {
			for _, value := range urls {
				if candidate, ok := value.(string); ok && strings.TrimSpace(candidate) != "" {
					videoURL = candidate
					break
				}
			}
		}
	}
	if status == "succeeded" && videoURL == "" {
		return nil, fmt.Errorf("upstream succeeded response has no result URL")
	}
	if videoURL != "" {
		result["content"] = map[string]any{"video_url": videoURL}
	}
	// usage 透传 completion_tokens/total_tokens（与反代协议一致），用于按 token 结算。
	// 上游终态真实返回 usage 已实测验证（方案 §10.2②），不再刻意丢弃。
	if usage := mapValue(data["usage"]); usage != nil {
		actual := map[string]any{}
		for _, field := range []string{"completion_tokens", "total_tokens"} {
			if value, exists := usage[field]; exists {
				actual[field] = value
			}
		}
		if len(actual) > 0 {
			result["usage"] = actual
		}
	}
	if status == "failed" {
		message := firstString(data, "error_message")
		if message == "" {
			message = findString(data, []string{"error", "message"}, []string{"message"})
		}
		if message == "" {
			message = "upstream task failed"
		}
		result["error"] = map[string]any{"message": sanitizeMessage(message)}
	}
	return common.Marshal(result)
}

// normalizeRelayStatus 归一化中转四态状态，未识别状态报错（方案 §3.2 不静默降级）。
func normalizeRelayStatus(status string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "queued", "pending", "submitted":
		return "queued", nil
	case "running", "processing":
		return "running", nil
	case "succeeded", "success", "completed":
		return "succeeded", nil
	case "failed", "failure", "error", "cancelled", "canceled":
		return "failed", nil
	default:
		return "", fmt.Errorf("upstream response has unsupported status %q", status)
	}
}
