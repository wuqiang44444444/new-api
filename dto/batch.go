package dto

// Azure Batch north-bound protocol DTOs. The north shape follows the OpenAI
// Batch contract that customers already integrate against; the south Azure
// differences are absorbed by the azurebatch adapter.

const (
	BatchEndpointChatCompletions = "/v1/chat/completions"
	// BatchCompletionWindow24h is the only accepted completion window. The
	// value is a processing target, never a local deadline for refunds.
	BatchCompletionWindow24h = "24h"
)

// Batch limits confirmed in the implementation plan: 10,000 requests per file
// and 20 MiB per file. Both bounds are enforced together.
const (
	MaxBatchRequestsPerFile = 10_000
	MaxBatchFileBytes       = 20_971_520
	// MaxBatchLineBytes bounds one JSONL line while parsing so a single
	// oversized line cannot buffer unboundedly; the file-level byte cap still
	// applies to the whole stream.
	MaxBatchLineBytes = MaxBatchFileBytes
)

type BatchCreateRequest struct {
	InputFileId      string            `json:"input_file_id"`
	Endpoint         string            `json:"endpoint"`
	CompletionWindow string            `json:"completion_window"`
	Metadata         map[string]string `json:"metadata,omitempty"`
}

func (r *BatchCreateRequest) Validate() *BatchValidateError {
	if r.InputFileId == "" {
		return &BatchValidateError{Message: "input_file_id is required"}
	}
	if r.Endpoint != BatchEndpointChatCompletions {
		return &BatchValidateError{Message: "endpoint must be /v1/chat/completions for now"}
	}
	if r.CompletionWindow != BatchCompletionWindow24h {
		return &BatchValidateError{Message: "completion_window is required and must be \"24h\""}
	}
	if len(r.Metadata) > 16 {
		return &BatchValidateError{Message: "metadata supports at most 16 entries"}
	}
	for key, value := range r.Metadata {
		if len([]rune(key)) > 64 || len([]rune(value)) > 512 {
			return &BatchValidateError{Message: "metadata keys and values exceed the supported length"}
		}
	}
	return nil
}

// BatchValidateError carries a sanitized north-bound rejection reason. Line
// errors include the 1-based line number and never echo request bodies.
type BatchValidateError struct {
	Line    int    `json:"-"`
	Message string `json:"message"`
}

func (e *BatchValidateError) Error() string { return e.Message }

type BatchLineError struct {
	Line    int    `json:"line"`
	Message string `json:"message"`
}

type BatchFileObject struct {
	Id        string            `json:"id"`
	Object    string            `json:"object"`
	Bytes     int64             `json:"bytes"`
	CreatedAt int64             `json:"created_at"`
	Filename  string            `json:"filename"`
	Purpose   string            `json:"purpose"`
	Status    string            `json:"status,omitempty"`
	Details   map[string]string `json:"details,omitempty"`
}

type BatchErrorRef struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Param    string `json:"param,omitempty"`
	Line     *int   `json:"line,omitempty"`
	Severity string `json:"severity,omitempty"`
}

type BatchErrors struct {
	Object string          `json:"object"`
	Data   []BatchErrorRef `json:"data"`
}

type BatchJobCounts struct {
	Total     int64 `json:"total"`
	Completed int64 `json:"completed"`
	Failed    int64 `json:"failed"`
	Expired   int64 `json:"expired"`
	Errored   int64 `json:"errored,omitempty"`
	Cancelled int64 `json:"cancelled,omitempty"`
}

type BatchJobUsage struct {
	InputTokens  int64 `json:"input_tokens"`
	InputCached  int64 `json:"input_cached,omitempty"`
	OutputTokens int64 `json:"output_tokens"`
	TotalTokens  int64 `json:"total_tokens"`
}

type BatchJobObject struct {
	Id               string            `json:"id"`
	Object           string            `json:"object"`
	Endpoint         string            `json:"endpoint"`
	Model            string            `json:"model"`
	Status           string            `json:"status"`
	CompletionWindow string            `json:"completion_window"`
	CreatedAt        int64             `json:"created_at"`
	ExpiresAt        int64             `json:"expires_at,omitempty"`
	CancellingAt     int64             `json:"cancelling_at,omitempty"`
	CancelledAt      int64             `json:"cancelled_at,omitempty"`
	FailedAt         int64             `json:"failed_at,omitempty"`
	CompletedAt      int64             `json:"completed_at,omitempty"`
	Metadata         map[string]string `json:"metadata,omitempty"`
	InputFileId      string            `json:"input_file_id"`
	ErrorFileId      string            `json:"error_file_id,omitempty"`
	OutputFileId     string            `json:"output_file_id,omitempty"`
	RequestCounts    *BatchJobCounts   `json:"request_counts,omitempty"`
	Usage            *BatchJobUsage    `json:"usage,omitempty"`
	Errors           *BatchErrors      `json:"errors,omitempty"`
}
