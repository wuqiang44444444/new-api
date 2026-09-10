// Package azurebatch implements the Azure OpenAI Batch south protocol: file
// upload, batch create/retrieve/cancel and file download. It absorbs the
// Azure path, api-version and endpoint-field differences so the north batch
// contract stays provider neutral.
package azurebatch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const (
	// Status values reported by Azure batch jobs.
	StatusValidating = "validating"
	StatusInProgress = "in_progress"
	StatusFinalizing = "finalizing"
	StatusCancelling = "cancelling"
	StatusCompleted  = "completed"
	StatusFailed     = "failed"
	StatusExpired    = "expired"
	StatusCancelled  = "cancelled"
)

// AdapterVersion identifies the frozen south protocol version recorded on
// attempts, tasks and snapshots.
const AdapterVersion = "azure_batch_v1"

// azureBatchEndpoint is the endpoint field value Azure expects in batch
// create requests (no /v1 prefix, unlike the north contract).
const azureBatchEndpoint = "/chat/completions"

var ErrUpstreamRejected = errors.New("azure batch request was rejected")

type Client struct {
	BaseURL    string
	APIKey     string
	APIVersion string
	HTTPClient *http.Client
}

func NewClient(baseURL, apiKey, apiVersion string) *Client {
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		APIKey:     apiKey,
		APIVersion: apiVersion,
		HTTPClient: &http.Client{Timeout: 120 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}
}

func (c *Client) url(path string) string {
	u, err := url.Parse(c.BaseURL + "/openai/" + path)
	if err != nil {
		return ""
	}
	q := u.Query()
	q.Set("api-version", c.APIVersion)
	u.RawQuery = q.Encode()
	return u.String()
}

func (c *Client) do(ctx context.Context, method, url string, contentType string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("api-key", c.APIKey)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
		switch resp.StatusCode {
		case 400, 401, 403, 404, 405, 413, 415, 422, 429:
			return nil, fmt.Errorf("%w: HTTP %d", ErrUpstreamRejected, resp.StatusCode)
		default:
			return nil, fmt.Errorf("azure batch acceptance is uncertain: HTTP %d", resp.StatusCode)
		}
	}
	return resp, nil
}

// UploadBatchFile uploads one converted JSONL file with purpose=batch and
// returns the upstream file id. Uploading a file is free; it does not create
// any billed inference job.
func (c *Client) UploadBatchFile(ctx context.Context, filename string, content []byte) (string, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return "", err
	}
	if _, err := part.Write(content); err != nil {
		return "", err
	}
	if err := writer.WriteField("purpose", "batch"); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}
	resp, err := c.do(ctx, http.MethodPost, c.url("files"), writer.FormDataContentType(), body.Bytes())
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var payload struct {
		Id string `json:"id"`
	}
	if err := common.DecodeJson(resp.Body, &payload); err != nil {
		return "", fmt.Errorf("azure batch file upload returned an unreadable response")
	}
	if strings.TrimSpace(payload.Id) == "" {
		return "", errors.New("azure batch file upload returned no file id")
	}
	return payload.Id, nil
}

// BatchStatus is the normalized upstream batch observation.
type BatchStatus struct {
	ValidationFailed bool
	Id               string
	Status           string
	InputFileId      string
	OutputFileId     string
	ErrorFileId      string
	CountsPresent    bool
	CountTotal       int64
	CountCompleted   int64
	CountFailed      int64
	CountExpired     int64
	CountErrored     int64
	CountCancelled   int64
	InputTokens      int64
	InputCached      int64
	OutputTokens     int64
	TotalTokens      int64
	ExpiresAt        int64
	CancelledAt      int64
	FailedAt         int64
	CompletedAt      int64
	CancelRequested  bool
	SanitizeError    string
}

func (s *BatchStatus) Terminal() bool {
	switch s.Status {
	case StatusCompleted, StatusFailed, StatusExpired, StatusCancelled:
		return true
	}
	return false
}

// Terminal reports whether the upstream status is final.

func decodeBatchStatus(body io.Reader) (*BatchStatus, error) {
	var payload struct {
		Id            string `json:"id"`
		Status        string `json:"status"`
		InputFileId   string `json:"input_file_id"`
		OutputFileId  string `json:"output_file_id"`
		ErrorFileId   string `json:"error_file_id"`
		ExpiresAt     int64  `json:"expires_at"`
		CancelledAt   int64  `json:"cancelled_at"`
		FailedAt      int64  `json:"failed_at"`
		CompletedAt   int64  `json:"completed_at"`
		RequestCounts *struct {
			Total     *int64 `json:"total"`
			Completed *int64 `json:"completed"`
			Failed    *int64 `json:"failed"`
			Expired   int64  `json:"expired"`
			Errored   *int64 `json:"errored"`
			Cancelled *int64 `json:"cancelled"`
		} `json:"request_counts"`
		Usage *struct {
			InputTokens        int64 `json:"input_tokens"`
			InputCachedDetails *struct {
				CachedTokens int64 `json:"cached_tokens"`
			} `json:"input_token_details"`
			OutputTokens int64 `json:"output_tokens"`
			TotalTokens  int64 `json:"total_tokens"`
		} `json:"usage"`
		Error *struct {
			Message string `json:"message"`
			Code    string `json:"code"`
		} `json:"error"`
		Errors *struct {
			Data []struct {
				Code string `json:"code"`
			} `json:"data"`
		} `json:"errors"`
	}
	if err := common.DecodeJson(body, &payload); err != nil {
		return nil, errors.New("azure batch response was unreadable")
	}
	if strings.TrimSpace(payload.Id) == "" {
		return nil, errors.New("azure batch response has no identity")
	}
	switch payload.Status {
	case StatusValidating, StatusInProgress, StatusFinalizing, StatusCancelling, StatusCompleted, StatusFailed, StatusExpired, StatusCancelled:
	default:
		return nil, errors.New("azure batch response has an unknown status")
	}
	status := &BatchStatus{
		Id: payload.Id, Status: payload.Status,
		InputFileId: payload.InputFileId, OutputFileId: payload.OutputFileId, ErrorFileId: payload.ErrorFileId,
		ExpiresAt: payload.ExpiresAt, CancelledAt: payload.CancelledAt,
		FailedAt: payload.FailedAt, CompletedAt: payload.CompletedAt,
	}
	status.CancelRequested = payload.Status == StatusCancelling
	if payload.RequestCounts != nil {
		if payload.RequestCounts.Total == nil || payload.RequestCounts.Completed == nil || payload.RequestCounts.Failed == nil {
			return nil, errors.New("azure batch request counters are incomplete")
		}
		status.CountsPresent = true
		status.CountTotal = *payload.RequestCounts.Total
		status.CountCompleted = *payload.RequestCounts.Completed
		status.CountFailed = *payload.RequestCounts.Failed
		status.CountExpired = payload.RequestCounts.Expired
		if payload.RequestCounts.Errored != nil {
			status.CountErrored = *payload.RequestCounts.Errored
		}
		if payload.RequestCounts.Cancelled != nil {
			status.CountCancelled = *payload.RequestCounts.Cancelled
		}
	}
	if payload.Usage != nil {
		status.InputTokens = payload.Usage.InputTokens
		status.OutputTokens = payload.Usage.OutputTokens
		status.TotalTokens = payload.Usage.TotalTokens
		if payload.Usage.InputCachedDetails != nil {
			status.InputCached = payload.Usage.InputCachedDetails.CachedTokens
		}
	}
	if payload.Error != nil {
		status.SanitizeError = "upstream batch processing failed"
	}
	if payload.Errors != nil && len(payload.Errors.Data) > 0 {
		status.SanitizeError = "upstream batch validation failed"
		status.ValidationFailed = status.Status == StatusFailed && status.CountCompleted == 0 && status.OutputFileId == ""
	}
	if status.CountTotal < 0 || status.CountTotal > 10000 || status.CountCompleted < 0 || status.CountFailed < 0 || status.CountExpired < 0 || status.CountCancelled < 0 || status.CountCompleted > status.CountTotal || status.CountFailed > status.CountTotal {
		return nil, errors.New("azure batch counters are invalid")
	}
	return status, nil
}

// CreateBatch submits one uploaded input file for processing. Azure expects
// the bare /chat/completions endpoint; the adapter absorbs that difference.
func (c *Client) CreateBatch(ctx context.Context, inputFileId, completionWindow string) (*BatchStatus, error) {
	payload, err := common.Marshal(map[string]string{
		"input_file_id":     inputFileId,
		"endpoint":          azureBatchEndpoint,
		"completion_window": completionWindow,
	})
	if err != nil {
		return nil, err
	}
	resp, err := c.do(ctx, http.MethodPost, c.url("batches"), "application/json", payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return decodeBatchStatus(resp.Body)
}

// RetrieveBatch reads one batch's current status and counters.
func (c *Client) RetrieveBatch(ctx context.Context, batchId string) (*BatchStatus, error) {
	resp, err := c.do(ctx, http.MethodGet, c.url("batches/"+url.PathEscape(batchId)), "", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return decodeBatchStatus(resp.Body)
}

// CancelBatch requests cancellation. Azure keeps charging for work already
// done; the returned status is an observation, not a settlement fact.
func (c *Client) CancelBatch(ctx context.Context, batchId string) (*BatchStatus, error) {
	resp, err := c.do(ctx, http.MethodPost, c.url("batches/"+url.PathEscape(batchId)+"/cancel"), "", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return decodeBatchStatus(resp.Body)
}

// VerifyConnection performs a read-only files list call so channel testing
// never creates a billed batch job.
func (c *Client) VerifyConnection(ctx context.Context) error {
	resp, err := c.do(ctx, http.MethodGet, c.url("files?purpose=batch"), "", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<10))
	return nil
}

// OpenFile streams result bytes without buffering the whole Batch output.
func (c *Client) OpenFile(ctx context.Context, id string) (io.ReadCloser, error) {
	resp, err := c.do(ctx, http.MethodGet, c.url("files/"+url.PathEscape(id)+"/content"), "", nil)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}
