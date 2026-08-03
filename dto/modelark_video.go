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
	Usage            *ModelArkVideoTaskUsage   `json:"usage,omitempty"`
	Error            *ModelArkVideoTaskError   `json:"error,omitempty"`
	CreatedAt        int64                     `json:"created_at"`
	UpdatedAt        int64                     `json:"updated_at"`
}

type ModelArkVideoTaskContent struct {
	VideoURL     string `json:"video_url,omitempty"`
	LastFrameURL string `json:"last_frame_url,omitempty"`
}

type ModelArkVideoTaskUsage struct {
	CompletionTokens int                     `json:"completion_tokens,omitempty"`
	TotalTokens      int                     `json:"total_tokens,omitempty"`
	ToolUsage        *ModelArkVideoToolUsage `json:"tool_usage,omitempty"`
}

type ModelArkVideoToolUsage struct {
	WebSearch int `json:"web_search,omitempty"`
}

type ModelArkVideoTaskError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ModelArkVideoTaskList struct {
	Total int64                `json:"total"`
	Items []*ModelArkVideoTask `json:"items"`
}
