package kling

// Payload types re-homed from the upstream-removed platform adaptor
// (rc.31 moved native kling/jimeng southbound to JS plugins). They persist
// only as the Link northbound contract shapes used by video_contract.go.

type TrajectoryPoint struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type DynamicMask struct {
	Mask         string            `json:"mask,omitempty"`
	Trajectories []TrajectoryPoint `json:"trajectories,omitempty"`
}

type CameraConfig struct {
	Horizontal float64 `json:"horizontal,omitempty"`
	Vertical   float64 `json:"vertical,omitempty"`
	Pan        float64 `json:"pan,omitempty"`
	Tilt       float64 `json:"tilt,omitempty"`
	Roll       float64 `json:"roll,omitempty"`
	Zoom       float64 `json:"zoom,omitempty"`
}

type CameraControl struct {
	Type   string        `json:"type,omitempty"`
	Config *CameraConfig `json:"config,omitempty"`
}

type requestPayload struct {
	Prompt         string         `json:"prompt,omitempty"`
	Image          string         `json:"image,omitempty"`
	ImageTail      string         `json:"image_tail,omitempty"`
	NegativePrompt string         `json:"negative_prompt,omitempty"`
	Mode           string         `json:"mode,omitempty"`
	Duration       string         `json:"duration,omitempty"`
	AspectRatio    string         `json:"aspect_ratio,omitempty"`
	ModelName      string         `json:"model_name,omitempty"`
	Model          string         `json:"model,omitempty"` // Compatible with upstreams that only recognize "model"
	CfgScale       float64        `json:"cfg_scale,omitempty"`
	StaticMask     string         `json:"static_mask,omitempty"`
	DynamicMasks   []DynamicMask  `json:"dynamic_masks,omitempty"`
	CameraControl  *CameraControl `json:"camera_control,omitempty"`
	CallbackUrl    string         `json:"callback_url,omitempty"`
	ExternalTaskId string         `json:"external_task_id,omitempty"`
}
