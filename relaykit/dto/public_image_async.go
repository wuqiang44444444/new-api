package dto

// PublicImageAsync describes the opt-in gateway task lifecycle.
type PublicImageAsync struct {
	RequestHeader  string `json:"request_header"`
	RequestValue   string `json:"request_value"`
	QueryPath      string `json:"query_path"`
	StreamPriority bool   `json:"stream_priority"`
}
