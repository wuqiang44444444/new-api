package dto

type ModelArkVideoTask struct {
	ID               string                    `json:"id"`
	Model            string                    `json:"model"`
	Status           string                    `json:"status"`
	Content          *ModelArkVideoTaskContent `json:"content,omitempty"`
	Seed             int64                     `json:"seed,omitempty"`
	Resolution       string                    `json:"resolution,omitempty"`
	Duration         int                       `json:"duration,omitempty"`
	Frames           int                       `json:"frames,omitempty"`
	Ratio            string                    `json:"ratio,omitempty"`
	FramesPerSecond  int                       `json:"framespersecond,omitempty"`
	GenerateAudio    *bool                     `json:"generate_audio,omitempty"`
	Draft            *bool                     `json:"draft,omitempty"`
	DraftTaskID      string                    `json:"draft_task_id,omitempty"`
	SafetyIdentifier string                    `json:"safety_identifier,omitempty"`
	Priority         *int                      `json:"priority,omitempty"`
	ServiceTier      string                    `json:"service_tier,omitempty"`
	Usage            ModelArkVideoTaskUsage    `json:"usage"`
	Error            *ModelArkVideoTaskError   `json:"error,omitempty"`
	CreatedAt        int64                     `json:"created_at"`
	UpdatedAt        int64                     `json:"updated_at"`
}

type ModelArkVideoTaskContent struct {
	VideoURL     string `json:"video_url,omitempty"`
	LastFrameURL string `json:"last_frame_url,omitempty"`
	// DeliveryStatus 是 Link 视频结果托管的公共交付合同（唯一表达）：ready
	// 时 video_url/last_frame_url 为本站 OSS 预签名地址；preparing 表示转存
	// 进行中（URL 暂缺）；unavailable 表示交付不可用（URL 暂缺，如实失败）。
	DeliveryStatus string `json:"delivery_status,omitempty"`
}

type ModelArkVideoTaskUsage struct {
	TotalSeconds      float64                `json:"total_seconds"`
	InputSeconds      float64                `json:"input_seconds"`
	OutputSeconds     float64                `json:"output_seconds"`
	InputAudioSeconds float64                `json:"input_audio_seconds"`
	InputImageCount   int                    `json:"input_image_count"`
	PromptTokens      int                    `json:"prompt_tokens"`
	CompletionTokens  int                    `json:"completion_tokens"`
	TotalTokens       int                    `json:"total_tokens"`
	ToolUsage         ModelArkVideoToolUsage `json:"tool_usage"`
}

type ModelArkVideoToolUsage struct {
	WebSearch int `json:"web_search"`
}

type ModelArkVideoTaskError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ModelArkVideoTaskList struct {
	Total int64                `json:"total"`
	Items []*ModelArkVideoTask `json:"items"`
}
