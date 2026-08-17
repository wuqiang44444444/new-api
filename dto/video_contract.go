package dto

type VideoContractID string

const (
	VideoContractModelArkV3 VideoContractID = "modelark.contents.generations.v3"
	VideoContractKlingV1    VideoContractID = "kling.v1.videos"
	VideoContractJimeng     VideoContractID = "jimeng.cv.async.2022-08-31"
)

type VideoContractRequest struct {
	ContractID VideoContractID
	ModelArk   *ModelArkVideoCreateRequest
	Kling      *KlingVideoCreateRequest
	Jimeng     *JimengVideoCreateRequest
}

type VideoMediaURL struct {
	URL string `json:"url"`
}

type ModelArkVideoContent struct {
	Type     string         `json:"type"`
	Text     *string        `json:"text,omitempty"`
	Role     *string        `json:"role,omitempty"`
	ImageURL *VideoMediaURL `json:"image_url,omitempty"`
	VideoURL *VideoMediaURL `json:"video_url,omitempty"`
	AudioURL *VideoMediaURL `json:"audio_url,omitempty"`
}

type ModelArkVideoCreateRequest struct {
	Model                 string                 `json:"model"`
	Content               []ModelArkVideoContent `json:"content"`
	CallbackURL           *string                `json:"callback_url,omitempty"`
	Duration              *int                   `json:"duration,omitempty"`
	Resolution            *string                `json:"resolution,omitempty"`
	Ratio                 *string                `json:"ratio,omitempty"`
	OutputFormat          *string                `json:"output_format,omitempty"`
	ServiceTier           *string                `json:"service_tier,omitempty"`
	GenerateAudio         *bool                  `json:"generate_audio,omitempty"`
	Watermark             *bool                  `json:"watermark,omitempty"`
	ReturnLastFrame       *bool                  `json:"return_last_frame,omitempty"`
	ExecutionExpiresAfter *int                   `json:"execution_expires_after,omitempty"`
	Draft                 *bool                  `json:"draft,omitempty"`
	Tools                 *[]ModelArkVideoTool   `json:"tools,omitempty"`
	SafetyIdentifier      *string                `json:"safety_identifier,omitempty"`
	Priority              *int                   `json:"priority,omitempty"`
	Frames                *int                   `json:"frames,omitempty"`
	Seed                  *int                   `json:"seed,omitempty"`
	CameraFixed           *bool                  `json:"camera_fixed,omitempty"`
}

type ModelArkVideoTool struct {
	Type *string `json:"type,omitempty"`
}

type KlingTrajectoryPoint struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type KlingDynamicMask struct {
	Mask         *string                `json:"mask,omitempty"`
	Trajectories []KlingTrajectoryPoint `json:"trajectories,omitempty"`
}

type KlingCameraConfig struct {
	Horizontal *float64 `json:"horizontal,omitempty"`
	Vertical   *float64 `json:"vertical,omitempty"`
	Pan        *float64 `json:"pan,omitempty"`
	Tilt       *float64 `json:"tilt,omitempty"`
	Roll       *float64 `json:"roll,omitempty"`
	Zoom       *float64 `json:"zoom,omitempty"`
}

type KlingCameraControl struct {
	Type   *string            `json:"type,omitempty"`
	Config *KlingCameraConfig `json:"config,omitempty"`
}

type KlingVideoCreateRequest struct {
	Prompt         *string             `json:"prompt,omitempty"`
	Image          *string             `json:"image,omitempty"`
	ImageTail      *string             `json:"image_tail,omitempty"`
	NegativePrompt *string             `json:"negative_prompt,omitempty"`
	Mode           *string             `json:"mode,omitempty"`
	Duration       *string             `json:"duration,omitempty"`
	AspectRatio    *string             `json:"aspect_ratio,omitempty"`
	ModelName      *string             `json:"model_name,omitempty"`
	CfgScale       *float64            `json:"cfg_scale,omitempty"`
	StaticMask     *string             `json:"static_mask,omitempty"`
	DynamicMasks   []KlingDynamicMask  `json:"dynamic_masks,omitempty"`
	CameraControl  *KlingCameraControl `json:"camera_control,omitempty"`
	CallbackURL    *string             `json:"callback_url,omitempty"`
	ExternalTaskID *string             `json:"external_task_id,omitempty"`
}

type JimengVideoCreateRequest struct {
	ReqKey           string   `json:"req_key"`
	BinaryDataBase64 []string `json:"binary_data_base64,omitempty"`
	ImageURLs        []string `json:"image_urls,omitempty"`
	Prompt           *string  `json:"prompt,omitempty"`
	Seed             *int64   `json:"seed,omitempty"`
	AspectRatio      *string  `json:"aspect_ratio,omitempty"`
	Frames           *int     `json:"frames,omitempty"`
}
