package jimeng

// Payload types re-homed from the upstream-removed platform adaptor
// (rc.31 moved native kling/jimeng southbound to JS plugins). They persist
// only as the Link northbound contract shapes used by video_contract.go.

type requestPayload struct {
	ReqKey           string   `json:"req_key"`
	BinaryDataBase64 []string `json:"binary_data_base64,omitempty"`
	ImageUrls        []string `json:"image_urls,omitempty"`
	Prompt           string   `json:"prompt,omitempty"`
	Seed             int64    `json:"seed"`
	AspectRatio      string   `json:"aspect_ratio"`
	Frames           int      `json:"frames,omitempty"`
}

type responsePayload struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	RequestId string `json:"request_id"`
	Data      struct {
		TaskID string `json:"task_id"`
	} `json:"data"`
}
