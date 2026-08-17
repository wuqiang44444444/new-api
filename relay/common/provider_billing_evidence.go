package common

const VideoTaskBillingContextKey = "video_task_billing_context"

// VideoTaskBillingContext carries only task-create-time facts into a Provider
// polling adapter. It is passed through the shared FetchTask map as one typed
// value so protocol code cannot silently mix independent string keys.
type VideoTaskBillingContext struct {
	ProviderModel    string
	BillingProbeBody []byte
	EstimatedTokens  int
}

// ProviderBillingEvidence is a private, durable explanation of Provider usage
// and cost measurements. It must never be returned in a customer task response.
type ProviderBillingEvidence struct {
	Provider        string `json:"provider"`
	TokenSource     string `json:"token_source,omitempty"`
	ReportedTokens  int    `json:"reported_tokens,omitempty"`
	RawConsumption  string `json:"raw_consumption,omitempty"`
	ConsumptionUnit string `json:"consumption_unit,omitempty"`
	ProviderModel   string `json:"provider_model,omitempty"`
	Resolution      string `json:"resolution,omitempty"`
	HasVideoInput   bool   `json:"has_video_input"`
}
