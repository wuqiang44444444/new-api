package mediaimage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

const (
	ProtocolMediaImageTaskV1 = "media-image-task/v1"
	maxResponseBytes         = 1 << 20
)

type CreateDisposition string

const (
	CreatePassthrough CreateDisposition = "passthrough"
	CreateCompleted   CreateDisposition = "completed"
	CreateAccepted    CreateDisposition = "accepted"
	CreateRejected    CreateDisposition = "rejected"
)

type State string

const (
	StateQueued     State = "queued"
	StateInProgress State = "in_progress"
	StateCompleted  State = "completed"
	StateFailed     State = "failed"
	StateUnknown    State = "unknown"
)

type Result struct {
	PrimaryURL string
	URLs       []string
	Usage      json.RawMessage
}

type CreateObservation struct {
	Disposition CreateDisposition
	TaskID      string
	RequestID   string
	Result      Result
	Failure     string
}

type PollObservation struct {
	State       State
	TaskID      string
	RequestID   string
	Result      Result
	Failure     string
	Trustworthy bool
}

type QuerySpec struct {
	Protocol          string
	BaseURL           string
	PathTemplate      string
	TaskID            string
	APIKey            string
	AuthType          string
	AuthName          string
	AuthValueTemplate string
	Headers           http.Header
}

type WaitOptions struct {
	InitialDelay time.Duration
	MaxDelay     time.Duration
	SkipSleep    bool
}

type WaitResult struct {
	Observation PollObservation
	Attempts    int
	Elapsed     time.Duration
}

type DoRequest func(*http.Request) (*http.Response, error)

type taskEnvelope struct {
	TaskID       string          `json:"task_id"`
	ID           string          `json:"id"`
	RequestID    string          `json:"request_id"`
	Status       string          `json:"status"`
	State        string          `json:"state"`
	Error        json.RawMessage `json:"error"`
	ErrorMessage string          `json:"error_message"`
	Message      string          `json:"message"`
	Result       *taskResult     `json:"result"`
	Usage        json.RawMessage `json:"usage"`
	Data         json.RawMessage `json:"data"`
}

type taskResult struct {
	PrimaryURL string          `json:"primary_url"`
	URLs       []string        `json:"urls"`
	Usage      json.RawMessage `json:"usage"`
}

func ValidateProtocol(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("media image task protocol is required")
	}
	if value != ProtocolMediaImageTaskV1 {
		return "", fmt.Errorf("unsupported media image protocol %q", value)
	}
	return value, nil
}

func InspectCreateResponse(protocol string, response *http.Response) (CreateObservation, error) {
	if _, err := ValidateProtocol(protocol); err != nil {
		return CreateObservation{}, err
	}
	if response == nil || response.Body == nil {
		return CreateObservation{}, errors.New("upstream media image create response body is empty")
	}
	body, err := readAndRestoreResponse(response)
	if err != nil {
		return CreateObservation{}, err
	}
	return inspectCreate(response.StatusCode, response.Header, body)
}

func Query(ctx context.Context, do DoRequest, spec QuerySpec) (PollObservation, error) {
	if do == nil {
		return PollObservation{}, errors.New("media image query transport is required")
	}
	target, err := BuildQueryURL(spec)
	if err != nil {
		return PollObservation{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, http.NoBody)
	if err != nil {
		return PollObservation{}, fmt.Errorf("create media image task query: %w", err)
	}
	if err := applyAuth(request, spec); err != nil {
		return PollObservation{}, err
	}
	for name, values := range spec.Headers {
		request.Header.Del(name)
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}
	request.Header.Del("Content-Type")
	request.Header.Set("Accept", "application/json")
	if host := strings.TrimSpace(request.Header.Get("Host")); host != "" {
		request.Host = host
		request.Header.Del("Host")
	}

	response, err := do(request)
	if err != nil {
		return PollObservation{}, fmt.Errorf("query upstream media image task: %w", err)
	}
	if response == nil {
		return PollObservation{}, errors.New("upstream media image task query returned no response")
	}
	if response.Body == nil {
		if response.StatusCode == http.StatusOK {
			return PollObservation{State: StateUnknown}, nil
		}
		return PollObservation{}, fmt.Errorf("upstream media image task query returned status %d", response.StatusCode)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
		return PollObservation{}, fmt.Errorf("upstream media image task query returned status %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return PollObservation{}, errors.New("read upstream media image task response")
	}
	if len(body) > maxResponseBytes {
		return PollObservation{State: StateUnknown}, nil
	}
	return parsePoll(response.Header, body, spec.TaskID)
}

func Wait(ctx context.Context, do DoRequest, spec QuerySpec, options WaitOptions) (WaitResult, error) {
	if options.InitialDelay <= 0 {
		options.InitialDelay = time.Second
	}
	if options.MaxDelay <= 0 {
		options.MaxDelay = 5 * time.Second
	}
	if options.MaxDelay < options.InitialDelay {
		options.MaxDelay = options.InitialDelay
	}
	startedAt := time.Now()
	delay := options.InitialDelay
	result := WaitResult{}
	for {
		if !options.SkipSleep {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				result.Elapsed = time.Since(startedAt)
				return result, ctx.Err()
			case <-timer.C:
			}
		} else if err := ctx.Err(); err != nil {
			result.Elapsed = time.Since(startedAt)
			return result, err
		}

		observation, err := Query(ctx, do, spec)
		result.Attempts++
		result.Observation = observation
		result.Elapsed = time.Since(startedAt)
		if err != nil {
			return result, err
		}
		switch observation.State {
		case StateQueued, StateInProgress:
		case StateCompleted, StateFailed, StateUnknown:
			return result, nil
		default:
			return result, fmt.Errorf("unsupported media image task state %q", observation.State)
		}
		if delay < options.MaxDelay {
			delay *= 2
			if delay > options.MaxDelay {
				delay = options.MaxDelay
			}
		}
	}
}

func BuildQueryURL(spec QuerySpec) (string, error) {
	if _, err := ValidateProtocol(spec.Protocol); err != nil {
		return "", err
	}
	base, err := url.Parse(strings.TrimSpace(spec.BaseURL))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return "", errors.New("media image task query base URL is invalid")
	}
	if base.Scheme != "http" && base.Scheme != "https" {
		return "", errors.New("media image task query base URL must use http or https")
	}
	template := strings.TrimSpace(spec.PathTemplate)
	if template == "" {
		return "", errors.New("media image task query path is required")
	}
	if !strings.HasPrefix(template, "/") || strings.HasPrefix(template, "//") || strings.Contains(template, "://") || !strings.Contains(template, "{task_id}") {
		return "", errors.New("media image task query path is invalid")
	}
	taskID, err := validateTaskID(spec.TaskID)
	if err != nil {
		return "", err
	}
	rawPath := strings.TrimRight(base.EscapedPath(), "/") + strings.ReplaceAll(template, "{task_id}", url.PathEscape(taskID))
	path, err := url.PathUnescape(rawPath)
	if err != nil {
		return "", errors.New("media image task query path is invalid")
	}
	base.Path = path
	base.RawPath = rawPath
	base.RawQuery = ""
	base.Fragment = ""
	if strings.TrimSpace(spec.AuthType) == dto.AdvancedCustomAuthTypeQuery {
		name := strings.TrimSpace(spec.AuthName)
		if name == "" {
			return "", errors.New("media image task query auth name is missing")
		}
		query := base.Query()
		query.Set(name, applyAuthTemplate(spec.AuthValueTemplate, spec.APIKey))
		base.RawQuery = query.Encode()
	}
	return base.String(), nil
}

func NormalizeResultURLs(result Result, maximum int) ([]string, error) {
	if maximum <= 0 {
		return nil, errors.New("media image result URL limit is invalid")
	}
	urls := make([]string, 0, len(result.URLs)+1)
	seen := make(map[string]struct{}, len(result.URLs)+1)
	add := func(value string) error {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil
		}
		parsed, err := url.Parse(value)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return errors.New("upstream media image task returned an invalid result URL")
		}
		if _, ok := seen[value]; ok {
			return nil
		}
		if len(urls) >= maximum {
			return fmt.Errorf("upstream media image task returned more than %d images", maximum)
		}
		seen[value] = struct{}{}
		urls = append(urls, value)
		return nil
	}
	for _, value := range result.URLs {
		if err := add(value); err != nil {
			return nil, err
		}
	}
	if err := add(result.PrimaryURL); err != nil {
		return nil, err
	}
	if len(urls) == 0 {
		return nil, errors.New("upstream media image task returned no result URL")
	}
	return urls, nil
}

func SanitizeRequestID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsFunc(value, unicode.IsControl) {
		return ""
	}
	runes := []rune(value)
	if len(runes) > 512 {
		return string(runes[:512])
	}
	return value
}

func SanitizeFailure(message string) string {
	message = strings.TrimSpace(message)
	lower := strings.ToLower(message)
	for _, sensitive := range []string{"http://", "https://", "bearer ", "api_key", "api-key", "authorization", "cookie"} {
		if strings.Contains(lower, sensitive) {
			return "upstream media image task failed"
		}
	}
	if message == "" {
		return "upstream media image task failed"
	}
	runes := []rune(message)
	if len(runes) > 512 {
		return string(runes[:512])
	}
	return message
}

func inspectCreate(statusCode int, header http.Header, body []byte) (CreateObservation, error) {
	envelope, payload, err := decodeEnvelope(body)
	if err != nil {
		return CreateObservation{}, errors.New("decode media image task create response")
	}
	status := normalizedState(payload)
	taskID := firstNonEmpty(payload.TaskID, payload.ID, envelope.TaskID, envelope.ID)
	requestID := requestIDFromEnvelope(envelope, payload, header)
	if statusCode == http.StatusOK && status == StateUnknown && strings.TrimSpace(taskID) == "" && payload.Result == nil {
		return CreateObservation{Disposition: CreatePassthrough}, nil
	}
	switch status {
	case StateQueued, StateInProgress:
		taskID, err = validateTaskID(taskID)
		if err != nil {
			return CreateObservation{}, err
		}
		return CreateObservation{Disposition: CreateAccepted, TaskID: taskID, RequestID: requestID}, nil
	case StateCompleted:
		return CreateObservation{Disposition: CreateCompleted, TaskID: strings.TrimSpace(taskID), RequestID: requestID, Result: resultFromEnvelope(payload)}, nil
	case StateFailed:
		return CreateObservation{Disposition: CreateRejected, TaskID: strings.TrimSpace(taskID), RequestID: requestID, Failure: SanitizeFailure(payload.failureMessage())}, nil
	case StateUnknown:
		if statusCode == http.StatusAccepted {
			taskID, err = validateTaskID(taskID)
			if err != nil {
				return CreateObservation{}, err
			}
			return CreateObservation{Disposition: CreateAccepted, TaskID: taskID, RequestID: requestID}, nil
		}
		return CreateObservation{}, fmt.Errorf("upstream media image task returned unsupported status %q", rawState(payload))
	default:
		return CreateObservation{}, fmt.Errorf("upstream media image task returned unsupported status %q", status)
	}
}

func parsePoll(header http.Header, body []byte, expectedTaskID string) (PollObservation, error) {
	envelope, payload, err := decodeEnvelope(body)
	if err != nil {
		return PollObservation{State: StateUnknown}, nil
	}
	returnedTaskID := firstNonEmpty(payload.TaskID, payload.ID)
	if returnedTaskID != "" && returnedTaskID != strings.TrimSpace(expectedTaskID) {
		return PollObservation{State: StateUnknown}, nil
	}
	state := normalizedState(payload)
	observation := PollObservation{
		State:       state,
		TaskID:      returnedTaskID,
		RequestID:   requestIDFromEnvelope(envelope, payload, header),
		Result:      resultFromEnvelope(payload),
		Failure:     SanitizeFailure(payload.failureMessage()),
		Trustworthy: true,
	}
	return observation, nil
}

func decodeEnvelope(body []byte) (*taskEnvelope, *taskEnvelope, error) {
	var envelope taskEnvelope
	if err := common.Unmarshal(body, &envelope); err != nil {
		return nil, nil, err
	}
	payload := &envelope
	data := bytes.TrimSpace(envelope.Data)
	if len(data) > 0 && data[0] == '{' {
		var nested taskEnvelope
		if err := common.Unmarshal(data, &nested); err != nil {
			return nil, nil, err
		}
		payload = &nested
	}
	return &envelope, payload, nil
}

func normalizedState(envelope *taskEnvelope) State {
	switch strings.ToLower(rawState(envelope)) {
	case "queued":
		return StateQueued
	case "running", "in_progress", "processing":
		return StateInProgress
	case "succeeded", "success", "completed":
		return StateCompleted
	case "failed", "failure", "cancelled", "canceled", "expired":
		return StateFailed
	default:
		return StateUnknown
	}
}

func rawState(envelope *taskEnvelope) string {
	if envelope == nil {
		return ""
	}
	if status := strings.TrimSpace(envelope.Status); status != "" {
		return status
	}
	return strings.TrimSpace(envelope.State)
}

func resultFromEnvelope(envelope *taskEnvelope) Result {
	if envelope == nil || envelope.Result == nil {
		return Result{}
	}
	usage := envelope.Usage
	if len(envelope.Result.Usage) > 0 {
		usage = envelope.Result.Usage
	}
	return Result{
		PrimaryURL: envelope.Result.PrimaryURL,
		URLs:       append([]string(nil), envelope.Result.URLs...),
		Usage:      append(json.RawMessage(nil), usage...),
	}
}

func requestIDFromEnvelope(envelope, payload *taskEnvelope, header http.Header) string {
	if payload != nil {
		if requestID := SanitizeRequestID(payload.RequestID); requestID != "" {
			return requestID
		}
	}
	if envelope != nil && envelope != payload {
		if requestID := SanitizeRequestID(envelope.RequestID); requestID != "" {
			return requestID
		}
	}
	for _, name := range []string{common.RequestIdKey, "X-Request-Id", "Request-Id", "X-Trace-Id"} {
		if requestID := SanitizeRequestID(header.Get(name)); requestID != "" {
			return requestID
		}
	}
	return ""
}

func (e *taskEnvelope) failureMessage() string {
	if e == nil {
		return "upstream media image task failed"
	}
	if message := strings.TrimSpace(e.ErrorMessage); message != "" {
		return message
	}
	if message := strings.TrimSpace(e.Message); message != "" {
		return message
	}
	if len(e.Error) > 0 && string(e.Error) != "null" {
		var message string
		if common.Unmarshal(e.Error, &message) == nil && strings.TrimSpace(message) != "" {
			return message
		}
		var object struct {
			Message string `json:"message"`
		}
		if common.Unmarshal(e.Error, &object) == nil && strings.TrimSpace(object.Message) != "" {
			return object.Message
		}
	}
	return "upstream media image task failed"
}

func validateTaskID(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("upstream media image task response has no task_id")
	}
	if len(value) > 191 {
		return "", errors.New("upstream media image task id is too long")
	}
	if strings.ContainsFunc(value, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' ||
			r >= 'A' && r <= 'Z' ||
			r >= '0' && r <= '9' ||
			strings.ContainsRune("-._~:", r))
	}) {
		return "", errors.New("upstream media image task id contains unsafe characters")
	}
	return value, nil
}

func applyAuth(request *http.Request, spec QuerySpec) error {
	switch strings.TrimSpace(spec.AuthType) {
	case "":
		request.Header.Set("Authorization", "Bearer "+spec.APIKey)
	case dto.AdvancedCustomAuthTypeNone, dto.AdvancedCustomAuthTypeQuery:
	case dto.AdvancedCustomAuthTypeHeader:
		name := strings.TrimSpace(spec.AuthName)
		if name == "" {
			return errors.New("media image task header auth name is missing")
		}
		request.Header.Set(name, applyAuthTemplate(spec.AuthValueTemplate, spec.APIKey))
	default:
		return errors.New("media image task auth snapshot is invalid")
	}
	return nil
}

func applyAuthTemplate(template, apiKey string) string {
	return strings.ReplaceAll(template, "{api_key}", apiKey)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func readAndRestoreResponse(response *http.Response) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	_ = response.Body.Close()
	if err != nil {
		return nil, errors.New("read upstream media image task response")
	}
	if len(body) > maxResponseBytes {
		return nil, errors.New("upstream media image task response is too large")
	}
	response.Body = io.NopCloser(bytes.NewReader(body))
	response.ContentLength = int64(len(body))
	return body, nil
}
