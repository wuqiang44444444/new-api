package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/QuantumNous/new-api/common"
)

const (
	moxingOverseaModel             = "seedance-2-0-oversea"
	tokenSaveDoubaoModel           = "doubao-seedance-2-0-260128"
	moxingPriceCNYPerMillionTokens = 49.0
	moxingBillingFPS               = 24.0
	verificationCostSafetyFactor   = 1.25
)

type createRequest struct {
	Model           string   `json:"model"`
	Capability      string   `json:"capability"`
	InputMode       string   `json:"input_mode"`
	ControlMode     string   `json:"control_mode"`
	Prompt          string   `json:"prompt"`
	DurationSeconds int      `json:"duration_seconds"`
	WithAudio       bool     `json:"with_audio"`
	Resolution      string   `json:"resolution"`
	AspectRatio     string   `json:"aspect_ratio"`
	Image           string   `json:"image,omitempty"`
	EndImage        string   `json:"end_image,omitempty"`
	ReferenceImages []string `json:"reference_images,omitempty"`
}

type verificationReport struct {
	StartedAt                string  `json:"started_at"`
	FinishedAt               string  `json:"finished_at"`
	BaseURL                  string  `json:"base_url"`
	ProviderModel            string  `json:"provider_model"`
	RequestedDuration        int     `json:"requested_duration"`
	RequestedResolution      string  `json:"requested_resolution"`
	RequestedRatio           string  `json:"requested_ratio"`
	RequestedGenerateAudio   bool    `json:"requested_generate_audio"`
	EstimatedSpendUpperCNY   float64 `json:"estimated_spend_upper_cny"`
	AuthPreflightHTTPStatus  int     `json:"auth_preflight_http_status"`
	CreateHTTPStatus         int     `json:"create_http_status,omitempty"`
	CreateRequestIDCaptured  bool    `json:"create_request_id_captured"`
	ProviderTaskID           string  `json:"-"`
	PollCount                int     `json:"poll_count"`
	TerminalStatus           string  `json:"terminal_status,omitempty"`
	ResultURL                string  `json:"-"`
	ResultType               string  `json:"result_type,omitempty"`
	UsageType                string  `json:"usage_type,omitempty"`
	UsageHasCompletionTokens bool    `json:"usage_has_completion_tokens"`
	UsageHasTotalTokens      bool    `json:"usage_has_total_tokens"`
	ContentHTTPStatus        int     `json:"content_http_status,omitempty"`
	ContentType              string  `json:"content_type,omitempty"`
	ContentLength            int64   `json:"content_length,omitempty"`
	RangeHTTPStatus          int     `json:"range_http_status,omitempty"`
	VideoWidth               int     `json:"video_width,omitempty"`
	VideoHeight              int     `json:"video_height,omitempty"`
	AudioStreamCount         int     `json:"audio_stream_count"`
	Passed                   bool    `json:"passed"`
	Error                    string  `json:"error,omitempty"`
}

type providerErrorEnvelope struct {
	Error struct {
		Type string `json:"type"`
		Code string `json:"code"`
	} `json:"error"`
}

var reportFilePath string

func main() {
	modelFlag := flag.String("model", moxingOverseaModel, "Moxing Seedance model ID")
	baseURLFlag := flag.String("base-url", "", "provider HTTPS origin URL; defaults from -model")
	usdCNY := flag.Float64("usd-cny", 7, "USD/CNY rate for TokenSave spend guard")
	duration := flag.Int("duration", 4, "output duration: 4..15 or -1")
	resolution := flag.String("resolution", "480p", "output resolution: 480p or 720p")
	ratio := flag.String("ratio", "16:9", "output aspect ratio")
	generateAudio := flag.Bool("generate-audio", false, "request generated audio")
	confirmSpend := flag.Bool("confirm-spend", false, "confirm that the live create may incur provider charges")
	maxSpend := flag.Float64("max-spend-cny", 0, "operator-approved CNY limit for this invocation")
	pollInterval := flag.Duration("poll-interval", 10*time.Second, "task polling interval")
	pollTimeout := flag.Duration("poll-timeout", 30*time.Minute, "overall task timeout")
	reportFile := flag.String("report-file", "", "optional path for the sanitized JSON report")
	flag.Parse()
	reportFilePath = strings.TrimSpace(*reportFile)

	providerModel := strings.TrimSpace(*modelFlag)
	defaultBaseURL, err := defaultBaseURLForModel(providerModel)
	if err != nil {
		fatalf("invalid model: %v", err)
	}
	if strings.TrimSpace(*baseURLFlag) == "" {
		*baseURLFlag = defaultBaseURL
	}
	baseURL, err := validateBaseURL(*baseURLFlag)
	if err != nil {
		fatalf("invalid base URL: %v", err)
	}
	if err := validateRequestDimensions(providerModel, *duration, *resolution, *ratio); err != nil {
		fatalf("invalid verification request: %v", err)
	}
	estimatedSpend := estimatedSpendUpperCNY(providerModel, *duration, *resolution, *usdCNY)
	if !*confirmSpend || *maxSpend < estimatedSpend {
		fatalf("live verification requires -confirm-spend and -max-spend-cny >= %.2f", estimatedSpend)
	}
	apiKey := strings.TrimSpace(os.Getenv("MOXING_API_KEY"))
	if apiKey == "" {
		fatalf("MOXING_API_KEY is required")
	}
	ffprobePath, err := exec.LookPath("ffprobe")
	if err != nil {
		fatalf("ffprobe is required")
	}

	report := verificationReport{
		StartedAt:              time.Now().UTC().Format(time.RFC3339),
		BaseURL:                baseURL.String(),
		ProviderModel:          providerModel,
		RequestedDuration:      *duration,
		RequestedResolution:    *resolution,
		RequestedRatio:         *ratio,
		RequestedGenerateAudio: *generateAudio,
		EstimatedSpendUpperCNY: estimatedSpend,
	}
	client := &http.Client{Timeout: 30 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), *pollTimeout)
	defer cancel()

	report.AuthPreflightHTTPStatus = authPreflight(ctx, client, baseURL, apiKey)
	if report.AuthPreflightHTTPStatus != http.StatusBadRequest {
		report.Error = fmt.Sprintf("auth preflight returned HTTP %d instead of documented HTTP 400", report.AuthPreflightHTTPStatus)
		finish(report, false)
	}

	requestBody, err := common.Marshal(createRequest{
		Model: providerModel, Capability: "video_generation", InputMode: "text", ControlMode: "none",
		Prompt:          "A calm sunrise over a quiet lake, slow cinematic camera movement",
		DurationSeconds: *duration, WithAudio: *generateAudio, Resolution: *resolution, AspectRatio: *ratio,
	})
	if err != nil {
		report.Error = "encode create request"
		finish(report, false)
	}
	status, headers, payload, err := providerRequest(
		ctx, client, http.MethodPost, endpoint(baseURL, "/v1/media/generations"), apiKey, requestBody, "",
	)
	report.CreateHTTPStatus = status
	report.CreateRequestIDCaptured = strings.TrimSpace(headers.Get("X-Oneapi-Request-Id")) != ""
	if err != nil {
		report.Error = "provider create request failed"
		finish(report, false)
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		report.Error = fmt.Sprintf("create HTTP %d: %s", status, providerErrorCode(payload))
		finish(report, false)
	}
	report.ProviderTaskID, err = extractTaskID(payload)
	if err != nil {
		report.Error = "create response has no trustworthy task id"
		finish(report, false)
	}

	ticker := time.NewTicker(*pollInterval)
	defer ticker.Stop()
	for {
		pollStatus, _, pollPayload, pollErr := providerRequest(
			ctx,
			client,
			http.MethodGet,
			endpoint(baseURL, "/v1/media/tasks/"+url.PathEscape(report.ProviderTaskID)),
			apiKey,
			nil,
			"",
		)
		report.PollCount++
		if pollErr != nil || pollStatus != http.StatusOK {
			report.Error = "provider polling failed"
			finish(report, false)
		}
		terminal, pollErr := consumeTaskResponse(pollPayload, &report)
		if pollErr != nil {
			report.Error = pollErr.Error()
			finish(report, false)
		}
		if terminal {
			break
		}
		select {
		case <-ctx.Done():
			report.Error = "poll timeout"
			finish(report, false)
		case <-ticker.C:
		}
	}

	if report.TerminalStatus != "succeeded" {
		report.Error = "provider task failed"
		finish(report, false)
	}
	report.ContentHTTPStatus, report.ContentType, report.ContentLength, report.VideoWidth, report.VideoHeight, report.AudioStreamCount, err =
		inspectVideo(ctx, client, report.ResultURL, ffprobePath)
	if err != nil {
		report.Error = "video content inspection failed"
		finish(report, false)
	}
	report.RangeHTTPStatus = inspectRange(ctx, client, report.ResultURL)
	report.Passed = (report.ContentHTTPStatus == http.StatusOK || report.ContentHTTPStatus == http.StatusPartialContent) &&
		report.RangeHTTPStatus == http.StatusPartialContent && report.VideoWidth > 0 && report.VideoHeight > 0
	if *generateAudio {
		report.Passed = report.Passed && report.AudioStreamCount > 0
	}
	finish(report, report.Passed)
}

func authPreflight(ctx context.Context, client *http.Client, baseURL *url.URL, apiKey string) int {
	status, _, _, _ := providerRequest(
		ctx, client, http.MethodPost, endpoint(baseURL, "/v1/media/generations"), apiKey, []byte(`{}`), "",
	)
	return status
}

func validateBaseURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("base URL must be an HTTPS origin")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed, nil
}

func defaultBaseURLForModel(model string) (string, error) {
	switch model {
	case moxingOverseaModel:
		return "https://www.moxing.pro", nil
	case tokenSaveDoubaoModel:
		return "https://tokensave.pro", nil
	default:
		return "", errors.New("model is not one of the two registered Moxing Seedance models")
	}
}

func validateRequestDimensions(model string, duration int, resolution, ratio string) error {
	if duration != -1 && (duration < 4 || duration > 15) {
		return errors.New("duration must be 4..15 or -1")
	}
	allowedResolution := resolution == "480p" || resolution == "720p"
	if model == tokenSaveDoubaoModel {
		allowedResolution = allowedResolution || resolution == "1080p"
	}
	if !allowedResolution {
		return errors.New("resolution is not supported by the selected model")
	}
	allowedRatios := map[string]struct{}{
		"16:9": {}, "4:3": {}, "1:1": {}, "3:4": {}, "9:16": {}, "21:9": {}, "adaptive": {},
	}
	if _, ok := allowedRatios[ratio]; !ok {
		return errors.New("ratio is not supported")
	}
	return nil
}

func estimatedSpendUpperCNY(model string, duration int, resolution string, usdCNY float64) float64 {
	if usdCNY <= 0 || math.IsNaN(usdCNY) || math.IsInf(usdCNY, 0) {
		return math.Inf(1)
	}
	if duration == -1 {
		duration = 15
	}
	if model == tokenSaveDoubaoModel {
		priceUSDPerSecond := 0.0679
		switch resolution {
		case "720p":
			priceUSDPerSecond = 0.1462
		case "1080p":
			priceUSDPerSecond = 0.3647
		}
		estimate := float64(duration) * priceUSDPerSecond * usdCNY * verificationCostSafetyFactor
		return float64(int64(estimate*100+0.999999)) / 100
	}
	width, height := 854.0, 480.0
	if resolution == "720p" {
		width, height = 1280, 720
	}
	tokens := float64(duration) * width * height * moxingBillingFPS / 1024
	estimate := tokens * moxingPriceCNYPerMillionTokens / 1_000_000 * verificationCostSafetyFactor
	return float64(int64(estimate*100+0.999999)) / 100
}

func extractTaskID(payload []byte) (string, error) {
	root, err := decodeObject(payload)
	if err != nil {
		return "", err
	}
	data := objectValue(root["data"])
	if data == nil {
		data = root
	}
	for _, field := range []string{"task_id", "id"} {
		if candidate, ok := data[field].(string); ok {
			candidate = strings.TrimSpace(candidate)
			if candidate != "" && len(candidate) <= 191 && !strings.ContainsFunc(candidate, unicode.IsControl) {
				return candidate, nil
			}
		}
	}
	return "", errors.New("invalid task id")
}

func consumeTaskResponse(payload []byte, report *verificationReport) (bool, error) {
	root, err := decodeObject(payload)
	if err != nil {
		return false, errors.New("invalid JSON task response")
	}
	data := objectValue(root["data"])
	if data == nil {
		data = root
	}
	taskID, err := extractTaskID(payload)
	if err != nil || taskID != report.ProviderTaskID {
		return false, errors.New("task response identity mismatch")
	}
	status, _ := data["status"].(string)
	status = strings.ToLower(strings.TrimSpace(status))
	switch status {
	case "queued", "running":
		return false, nil
	case "failed":
		report.TerminalStatus = "failed"
		return true, nil
	case "succeeded":
		report.TerminalStatus = "succeeded"
	default:
		return false, errors.New("unsupported provider task status")
	}

	report.ResultType = valueType(data["result"])
	report.UsageType = valueType(data["usage"])
	if usage := objectValue(data["usage"]); usage != nil {
		_, report.UsageHasCompletionTokens = usage["completion_tokens"]
		_, report.UsageHasTotalTokens = usage["total_tokens"]
	}
	report.ResultURL, err = resultURL(data["result"])
	if err != nil {
		return false, errors.New("succeeded task has no trustworthy result URL")
	}
	return true, nil
}

func resultURL(result any) (string, error) {
	if text, ok := result.(string); ok {
		text = strings.TrimSpace(text)
		if parsed, err := validateResultURL(text); err == nil {
			return parsed, nil
		}
		var object map[string]any
		if common.UnmarshalJsonStr(text, &object) == nil {
			result = object
		}
	}
	object := objectValue(result)
	if object == nil {
		return "", errors.New("result is not an object")
	}
	for _, field := range []string{"primary_url", "url"} {
		if candidate, ok := object[field].(string); ok {
			if validated, err := validateResultURL(candidate); err == nil {
				return validated, nil
			}
		}
	}
	if urls, ok := object["urls"].([]any); ok {
		for _, item := range urls {
			if candidate, ok := item.(string); ok {
				if validated, err := validateResultURL(candidate); err == nil {
					return validated, nil
				}
			}
		}
	}
	return "", errors.New("result has no HTTPS URL")
}

func validateResultURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || !parsed.IsAbs() {
		return "", errors.New("result URL must be absolute HTTPS")
	}
	return raw, nil
}

func inspectVideo(ctx context.Context, client *http.Client, videoURL, ffprobePath string) (int, string, int64, int, int, int, error) {
	status, headers, payload, err := providerRequest(ctx, client, http.MethodGet, videoURL, "", nil, "")
	if err != nil || (status != http.StatusOK && status != http.StatusPartialContent) {
		return status, headers.Get("Content-Type"), int64(len(payload)), 0, 0, 0, errors.New("content request failed")
	}
	temporary, err := os.CreateTemp("", "moxing-seedance-*.mp4")
	if err != nil {
		return status, headers.Get("Content-Type"), int64(len(payload)), 0, 0, 0, err
	}
	path := temporary.Name()
	defer os.Remove(path)
	written, writeErr := temporary.Write(payload)
	closeErr := temporary.Close()
	if writeErr != nil || closeErr != nil {
		return status, headers.Get("Content-Type"), int64(written), 0, 0, 0, errors.New("store temporary content")
	}
	command := exec.CommandContext(
		ctx, ffprobePath, "-v", "error", "-select_streams", "v:0", "-show_entries", "stream=width,height", "-of", "csv=s=x:p=0", path,
	)
	output, err := command.Output()
	if err != nil {
		return status, headers.Get("Content-Type"), int64(written), 0, 0, 0, errors.New("ffprobe failed")
	}
	dimensions := strings.Split(strings.TrimSpace(string(output)), "x")
	if len(dimensions) != 2 {
		return status, headers.Get("Content-Type"), int64(written), 0, 0, 0, errors.New("ffprobe returned invalid dimensions")
	}
	width, widthErr := strconv.Atoi(dimensions[0])
	height, heightErr := strconv.Atoi(dimensions[1])
	if widthErr != nil || heightErr != nil || width <= 0 || height <= 0 {
		return status, headers.Get("Content-Type"), int64(written), 0, 0, 0, errors.New("ffprobe returned invalid dimensions")
	}
	audioCommand := exec.CommandContext(
		ctx, ffprobePath, "-v", "error", "-select_streams", "a", "-show_entries", "stream=index", "-of", "csv=p=0", path,
	)
	audioOutput, err := audioCommand.Output()
	if err != nil {
		return status, headers.Get("Content-Type"), int64(written), 0, 0, 0, errors.New("ffprobe audio inspection failed")
	}
	return status, headers.Get("Content-Type"), int64(written), width, height, len(strings.Fields(string(audioOutput))), nil
}

func inspectRange(ctx context.Context, client *http.Client, videoURL string) int {
	status, _, _, _ := providerRequest(ctx, client, http.MethodGet, videoURL, "", nil, "bytes=0-1023")
	return status
}

func providerRequest(
	ctx context.Context,
	client *http.Client,
	method string,
	requestURL string,
	apiKey string,
	body []byte,
	rangeHeader string,
) (int, http.Header, []byte, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, requestURL, reader)
	if err != nil {
		return 0, nil, nil, err
	}
	if apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+apiKey)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if rangeHeader != "" {
		request.Header.Set("Range", rangeHeader)
	}
	response, err := client.Do(request)
	if err != nil {
		return 0, nil, nil, err
	}
	defer response.Body.Close()
	payload, readErr := io.ReadAll(io.LimitReader(response.Body, 256*1024*1024))
	return response.StatusCode, response.Header.Clone(), payload, readErr
}

func decodeObject(payload []byte) (map[string]any, error) {
	var object map[string]any
	if err := common.Unmarshal(payload, &object); err != nil || object == nil {
		return nil, errors.New("response is not a JSON object")
	}
	return object, nil
}

func objectValue(value any) map[string]any {
	object, _ := value.(map[string]any)
	return object
}

func valueType(value any) string {
	switch value.(type) {
	case nil:
		return "null"
	case string:
		return "string"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	default:
		return "scalar"
	}
}

func providerErrorCode(payload []byte) string {
	var envelope providerErrorEnvelope
	if common.Unmarshal(payload, &envelope) == nil {
		if code := strings.TrimSpace(envelope.Error.Code); code != "" {
			return code
		}
		if errorType := strings.TrimSpace(envelope.Error.Type); errorType != "" {
			return errorType
		}
	}
	return "provider_error_response"
}

func endpoint(baseURL *url.URL, path string) string {
	return strings.TrimRight(baseURL.String(), "/") + path
}

func finish(report verificationReport, success bool) {
	report.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	report.Passed = success
	payload, err := common.Marshal(report)
	if err != nil {
		fatalf("encode report: %v", err)
	}
	if reportFilePath != "" {
		if err := os.WriteFile(reportFilePath, append(payload, '\n'), 0o600); err != nil {
			fatalf("write sanitized report: %v", err)
		}
	}
	fmt.Println(string(payload))
	if !success {
		os.Exit(1)
	}
	os.Exit(0)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(2)
}
