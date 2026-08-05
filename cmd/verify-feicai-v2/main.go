package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const estimatedFullMatrixSpendCNY = 71.81

var providerTaskIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,255}$`)

type modelSpec struct {
	ProviderModel  string
	CredentialName string
	Credential     string
	Duration       int
	ReferenceImage string
	EstimatedCNY   float64
}

type createRequest struct {
	Model    string   `json:"model"`
	Prompt   string   `json:"prompt"`
	Duration int      `json:"duration"`
	Size     string   `json:"size"`
	Images   []string `json:"images,omitempty"`
}

type createResponse struct {
	ID     string `json:"id"`
	TaskID string `json:"task_id"`
	Status string `json:"status"`
}

type taskResponse struct {
	ID         string `json:"id"`
	TaskID     string `json:"task_id"`
	Status     string `json:"status"`
	VideoURL   string `json:"video_url"`
	FailReason string `json:"fail_reason"`
}

type taskListResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		Items []struct {
			TaskID     string `json:"task_id"`
			Status     string `json:"status"`
			Quota      int64  `json:"quota"`
			VideoURL   string `json:"video_url"`
			FailReason string `json:"fail_reason"`
		} `json:"items"`
	} `json:"data"`
}

type modelListResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

type usageResponse struct {
	TotalUsage float64 `json:"total_usage"`
}

type modelResult struct {
	ProviderModel         string  `json:"provider_model"`
	CredentialName        string  `json:"credential_name"`
	Duration              int     `json:"duration"`
	Size                  string  `json:"size"`
	CreateHTTPStatus      int     `json:"create_http_status"`
	ProviderID            string  `json:"-"`
	CreateStatus          string  `json:"create_status,omitempty"`
	ObservedTaskID        string  `json:"-"`
	PollCount             int     `json:"poll_count"`
	TerminalStatus        string  `json:"terminal_status,omitempty"`
	VideoURL              string  `json:"-"`
	TaskListStatus        string  `json:"task_list_status,omitempty"`
	TaskListHTTPStatus    int     `json:"task_list_http_status,omitempty"`
	TaskQuota             int64   `json:"task_quota,omitempty"`
	ContentHTTPStatus     int     `json:"content_http_status,omitempty"`
	ContentType           string  `json:"content_type,omitempty"`
	ContentLength         int64   `json:"content_length,omitempty"`
	VideoWidth            int     `json:"video_width,omitempty"`
	VideoHeight           int     `json:"video_height,omitempty"`
	VideoDurationSeconds  float64 `json:"video_duration_seconds,omitempty"`
	IdentityStable        bool    `json:"identity_stable"`
	SameOriginHTTPSVideo  bool    `json:"same_origin_https_video"`
	Passed                bool    `json:"passed"`
	Error                 string  `json:"error,omitempty"`
	consecutivePollErrors int
}

type report struct {
	StartedAt                string             `json:"started_at"`
	FinishedAt               string             `json:"finished_at"`
	BaseURL                  string             `json:"base_url"`
	SizeCandidate            string             `json:"size_candidate"`
	EstimatedMinimumSpendCNY float64            `json:"estimated_minimum_spend_cny"`
	ReconciliationAvailable  bool               `json:"reconciliation_available"`
	UsageBeforeCents         map[string]float64 `json:"usage_before_cents"`
	UsageAfterCents          map[string]float64 `json:"usage_after_cents"`
	Models                   []modelResult      `json:"models"`
	Passed                   bool               `json:"passed"`
}

type providerErrorEnvelope struct {
	Message string `json:"message"`
	Error   struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	} `json:"error"`
}

func main() {
	baseURLFlag := flag.String("base-url", "https://feicai123.top", "Feicai HTTPS base URL")
	size := flag.String("size", "1280x720", "single candidate size tested for every exact provider model")
	confirmSpend := flag.Bool("confirm-spend", false, "confirm that the live calls may incur provider charges")
	maxSpend := flag.Float64("max-spend-cny", 0, "operator-approved maximum CNY spend")
	selectedModels := flag.String("models", "", "optional comma-separated exact provider models to verify")
	pollInterval := flag.Duration("poll-interval", 10*time.Second, "poll interval")
	pollTimeout := flag.Duration("poll-timeout", 30*time.Minute, "overall poll timeout")
	flag.Parse()

	if !*confirmSpend {
		fatalf("live verification requires -confirm-spend")
	}
	baseURL, err := validatedBaseURL(*baseURLFlag)
	if err != nil {
		fatalf("invalid base URL: %v", err)
	}
	if strings.TrimSpace(*size) == "" {
		fatalf("size is required")
	}
	ffprobePath, err := exec.LookPath("ffprobe")
	if err != nil {
		fatalf("ffprobe is required to verify generated video dimensions")
	}
	vipKey := strings.TrimSpace(os.Getenv("FEICAI_VIP_API_KEY"))
	valueKey := strings.TrimSpace(os.Getenv("FEICAI_VALUE_API_KEY"))
	referenceImage := strings.TrimSpace(os.Getenv("FEICAI_REFERENCE_IMAGE_URL"))
	if vipKey == "" || valueKey == "" {
		fatalf("FEICAI_VIP_API_KEY and FEICAI_VALUE_API_KEY are required")
	}
	if err := validateReferenceImage(referenceImage); err != nil {
		fatalf("FEICAI_REFERENCE_IMAGE_URL is invalid: %v", err)
	}

	specs, err := selectVerificationModelSpecs(
		verificationModelSpecs(vipKey, valueKey, referenceImage),
		*selectedModels,
	)
	if err != nil {
		fatalf("invalid model selection: %v", err)
	}
	estimatedSpend := estimatedSpendCNY(specs)
	if *maxSpend < estimatedSpend {
		fatalf("selected live verification requires -max-spend-cny >= %.2f", estimatedSpend)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), *pollTimeout)
	defer cancel()

	credentialSpecs := map[string]modelSpec{
		"vip":   {CredentialName: "vip", Credential: vipKey},
		"value": {CredentialName: "value", Credential: valueKey},
	}
	for name, credential := range credentialSpecs {
		expectedModels := specsForCredential(specs, name)
		if len(expectedModels) == 0 {
			continue
		}
		if err := verifyModelVisibility(ctx, client, baseURL, credential.Credential, expectedModels); err != nil {
			fatalf("%s model preflight failed: %v", name, err)
		}
	}

	verification := report{
		StartedAt:                time.Now().UTC().Format(time.RFC3339),
		BaseURL:                  baseURL.String(),
		SizeCandidate:            *size,
		EstimatedMinimumSpendCNY: estimatedSpend,
		UsageBeforeCents:         make(map[string]float64, len(credentialSpecs)),
		UsageAfterCents:          make(map[string]float64, len(credentialSpecs)),
		Models:                   make([]modelResult, len(specs)),
	}
	for name, credential := range credentialSpecs {
		if len(specsForCredential(specs, name)) == 0 {
			continue
		}
		usage, err := readUsage(ctx, client, baseURL, credential.Credential)
		if err != nil {
			fatalf("%s usage preflight failed: %v", name, err)
		}
		verification.UsageBeforeCents[name] = usage
	}

	for index, spec := range specs {
		verification.Models[index] = createTask(ctx, client, baseURL, spec, *size)
	}
	pollTasks(ctx, client, baseURL, specs, verification.Models, *pollInterval)
	for index := range verification.Models {
		completeEvidence(ctx, client, baseURL, specs[index], &verification.Models[index], ffprobePath)
	}
	for name, credential := range credentialSpecs {
		if len(specsForCredential(specs, name)) == 0 {
			continue
		}
		usage, err := readUsage(ctx, client, baseURL, credential.Credential)
		if err != nil {
			verification.UsageAfterCents[name] = -1
			continue
		}
		verification.UsageAfterCents[name] = usage
	}

	verification.Passed = true
	verification.ReconciliationAvailable = true
	for index := range verification.Models {
		result := &verification.Models[index]
		result.Passed = result.Error == "" && result.TerminalStatus == "completed" && result.IdentityStable &&
			result.SameOriginHTTPSVideo &&
			(result.ContentHTTPStatus == http.StatusOK || result.ContentHTTPStatus == http.StatusPartialContent) &&
			result.VideoWidth > 0 && result.VideoHeight > 0 && result.VideoDurationSeconds > 0
		verification.Passed = verification.Passed && result.Passed
		verification.ReconciliationAvailable = verification.ReconciliationAvailable &&
			result.TaskListHTTPStatus == http.StatusOK && result.TaskListStatus == "SUCCESS" && result.TaskQuota > 0
	}
	for name, before := range verification.UsageBeforeCents {
		after := verification.UsageAfterCents[name]
		if after < before {
			verification.Passed = false
		}
	}
	verification.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	payload, err := common.Marshal(verification)
	if err != nil {
		fatalf("marshal report: %v", err)
	}
	fmt.Println(string(payload))
	if !verification.Passed {
		os.Exit(1)
	}
}

func verificationModelSpecs(vipKey, valueKey, referenceImage string) []modelSpec {
	return []modelSpec{
		{ProviderModel: "seedance-2.0-vip-720p-mini-azhw-feicai", CredentialName: "vip", Credential: vipKey, Duration: 4, EstimatedCNY: 2.20},
		{ProviderModel: "seedance2.0-sd2-feicai", CredentialName: "value", Credential: valueKey, Duration: 11, ReferenceImage: referenceImage, EstimatedCNY: 6.60},
		{ProviderModel: "seedance-2.0-vip-720p-fast-azhw-feicai", CredentialName: "vip", Credential: vipKey, Duration: 4, EstimatedCNY: 2.40},
		{ProviderModel: "seedance-2.0-933-720p-azhw-feicai", CredentialName: "value", Credential: valueKey, Duration: 4, EstimatedCNY: 2.48},
		{ProviderModel: "seedance-2.0-vip-720p-azhw-feicai", CredentialName: "vip", Credential: vipKey, Duration: 4, EstimatedCNY: 2.96},
		{ProviderModel: "seedance-2.0-933-1080p-azhw-feicai", CredentialName: "value", Credential: valueKey, Duration: 4, EstimatedCNY: 5.92},
		{ProviderModel: "seedance-2.0-vip-1080p-azhw-feicai", CredentialName: "vip", Credential: vipKey, Duration: 4, EstimatedCNY: 6.96},
		{ProviderModel: "seedance-2.0-933-4k-azhw-feicai", CredentialName: "value", Credential: valueKey, Duration: 4, EstimatedCNY: 14.32},
		{ProviderModel: "seedance-2.0-vip-4k-azhw-feicai", CredentialName: "vip", Credential: vipKey, Duration: 4, EstimatedCNY: 16.92},
		{ProviderModel: "seedance-933-pro-pi-feicai", CredentialName: "value", Credential: valueKey, Duration: 15, EstimatedCNY: 11.05},
	}
}

func selectVerificationModelSpecs(specs []modelSpec, rawSelection string) ([]modelSpec, error) {
	if strings.TrimSpace(rawSelection) == "" {
		return specs, nil
	}
	available := make(map[string]modelSpec, len(specs))
	for _, spec := range specs {
		available[spec.ProviderModel] = spec
	}
	selected := make([]modelSpec, 0)
	seen := make(map[string]struct{})
	for _, rawModel := range strings.Split(rawSelection, ",") {
		providerModel := strings.TrimSpace(rawModel)
		spec, ok := available[providerModel]
		if !ok {
			return nil, fmt.Errorf("unknown provider model %q", providerModel)
		}
		if _, duplicate := seen[providerModel]; duplicate {
			return nil, fmt.Errorf("duplicate provider model %q", providerModel)
		}
		seen[providerModel] = struct{}{}
		selected = append(selected, spec)
	}
	if len(selected) == 0 {
		return nil, errors.New("at least one provider model is required")
	}
	return selected, nil
}

func estimatedSpendCNY(specs []modelSpec) float64 {
	total := 0.0
	for _, spec := range specs {
		total += spec.EstimatedCNY
	}
	return float64(int64(total*100+0.5)) / 100
}

func validatedBaseURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("base URL must be an origin HTTPS URL")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed, nil
}

func validateReferenceImage(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return errors.New("a public HTTPS image URL is required for SD2")
	}
	return nil
}

func specsForCredential(specs []modelSpec, credentialName string) []string {
	models := make([]string, 0, len(specs))
	for _, spec := range specs {
		if spec.CredentialName == credentialName {
			models = append(models, spec.ProviderModel)
		}
	}
	return models
}

func verifyModelVisibility(
	ctx context.Context,
	client *http.Client,
	baseURL *url.URL,
	credential string,
	expected []string,
) error {
	status, payload, err := doProviderRequest(ctx, client, http.MethodGet, endpoint(baseURL, "/v1/models"), credential, nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", status, providerErrorCode(payload))
	}
	var response modelListResponse
	if err := common.Unmarshal(payload, &response); err != nil {
		return fmt.Errorf("decode model list: %w", err)
	}
	visible := make([]string, 0, len(response.Data))
	for _, item := range response.Data {
		visible = append(visible, strings.TrimSpace(item.ID))
	}
	for _, providerModel := range expected {
		if !slices.Contains(visible, providerModel) {
			return fmt.Errorf("provider model %q is not visible", providerModel)
		}
	}
	return nil
}

func readUsage(ctx context.Context, client *http.Client, baseURL *url.URL, credential string) (float64, error) {
	status, payload, err := doProviderRequest(
		ctx,
		client,
		http.MethodGet,
		endpoint(baseURL, "/v1/dashboard/billing/usage"),
		credential,
		nil,
	)
	if err != nil {
		return 0, err
	}
	if status != http.StatusOK {
		return 0, fmt.Errorf("HTTP %d: %s", status, providerErrorCode(payload))
	}
	var response usageResponse
	if err := common.Unmarshal(payload, &response); err != nil {
		return 0, fmt.Errorf("decode usage: %w", err)
	}
	if response.TotalUsage < 0 {
		return 0, errors.New("negative total_usage")
	}
	return response.TotalUsage, nil
}

func createTask(
	ctx context.Context,
	client *http.Client,
	baseURL *url.URL,
	spec modelSpec,
	size string,
) modelResult {
	result := modelResult{
		ProviderModel: spec.ProviderModel, CredentialName: spec.CredentialName, Duration: spec.Duration, Size: size,
	}
	request := createRequest{
		Model: spec.ProviderModel, Prompt: "A calm landscape with a slow cinematic camera movement",
		Duration: spec.Duration, Size: size,
	}
	if spec.ReferenceImage != "" {
		request.Images = []string{spec.ReferenceImage}
	}
	body, err := common.Marshal(request)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	status, payload, err := doProviderRequest(ctx, client, http.MethodPost, endpoint(baseURL, "/v1/videos"), spec.Credential, body)
	result.CreateHTTPStatus = status
	if err != nil {
		result.Error = "provider create request failed"
		return result
	}
	if status < 200 || status >= 300 {
		result.Error = fmt.Sprintf("create HTTP %d: %s", status, providerErrorCode(payload))
		return result
	}
	var response createResponse
	if err := common.Unmarshal(payload, &response); err != nil {
		result.Error = "decode create response: " + err.Error()
		return result
	}
	response.ID = strings.TrimSpace(response.ID)
	if !providerTaskIDPattern.MatchString(response.ID) {
		result.Error = "create response has no valid top-level id"
		return result
	}
	result.ProviderID = response.ID
	result.CreateStatus = strings.ToLower(strings.TrimSpace(response.Status))
	result.ObservedTaskID = strings.TrimSpace(response.TaskID)
	result.IdentityStable = true
	return result
}

func pollTasks(
	ctx context.Context,
	client *http.Client,
	baseURL *url.URL,
	specs []modelSpec,
	results []modelResult,
	interval time.Duration,
) {
	for {
		pending := 0
		for index := range results {
			result := &results[index]
			if result.ProviderID == "" || result.TerminalStatus != "" || result.Error != "" {
				continue
			}
			pending++
			pollTask(ctx, client, baseURL, specs[index], result)
		}
		if pending == 0 {
			return
		}
		select {
		case <-ctx.Done():
			for index := range results {
				if results[index].ProviderID != "" && results[index].TerminalStatus == "" && results[index].Error == "" {
					results[index].Error = "poll timeout: " + ctx.Err().Error()
				}
			}
			return
		case <-time.After(interval):
		}
	}
}

func pollTask(
	ctx context.Context,
	client *http.Client,
	baseURL *url.URL,
	spec modelSpec,
	result *modelResult,
) {
	status, payload, err := doProviderRequest(
		ctx,
		client,
		http.MethodGet,
		endpoint(baseURL, "/v1/videos/"+url.PathEscape(result.ProviderID)),
		spec.Credential,
		nil,
	)
	result.PollCount++
	if err != nil || status != http.StatusOK {
		result.consecutivePollErrors++
		if result.consecutivePollErrors >= 3 {
			if err != nil {
				result.Error = "provider poll request failed"
			} else {
				result.Error = fmt.Sprintf("poll HTTP %d: %s", status, providerErrorCode(payload))
			}
		}
		return
	}
	result.consecutivePollErrors = 0
	var response taskResponse
	if err := common.Unmarshal(payload, &response); err != nil {
		result.Error = "decode poll response: " + err.Error()
		return
	}
	if strings.TrimSpace(response.ID) != result.ProviderID {
		result.IdentityStable = false
		result.Error = "poll response top-level id changed"
		return
	}
	result.ObservedTaskID = strings.TrimSpace(response.TaskID)
	switch strings.ToLower(strings.TrimSpace(response.Status)) {
	case "queued", "processing", "in_progress":
		return
	case "completed":
		result.TerminalStatus = "completed"
		result.VideoURL = strings.TrimSpace(response.VideoURL)
		result.SameOriginHTTPSVideo = sameOriginHTTPS(baseURL, result.VideoURL)
		if !result.SameOriginHTTPSVideo {
			result.Error = "completed response has no same-origin HTTPS video_url"
		}
	case "failed":
		result.TerminalStatus = "failed"
		result.Error = "provider task reported failure"
	default:
		result.Error = fmt.Sprintf("unknown provider task status %q", response.Status)
	}
}

func completeEvidence(
	ctx context.Context,
	client *http.Client,
	baseURL *url.URL,
	spec modelSpec,
	result *modelResult,
	ffprobePath string,
) {
	if result.ProviderID == "" || result.TerminalStatus == "" {
		return
	}
	for attempt := 0; attempt < 3; attempt++ {
		status, payload, err := doProviderRequest(
			ctx,
			client,
			http.MethodGet,
			endpoint(baseURL, "/v1/tasks?task_id="+url.QueryEscape(result.ProviderID)+"&page_size=5"),
			spec.Credential,
			nil,
		)
		result.TaskListHTTPStatus = status
		if err == nil && status == http.StatusOK {
			var response taskListResponse
			if common.Unmarshal(payload, &response) == nil && response.Success {
				for _, item := range response.Data.Items {
					if strings.TrimSpace(item.TaskID) == result.ProviderID {
						result.TaskListStatus = strings.ToUpper(strings.TrimSpace(item.Status))
						result.TaskQuota = item.Quota
						break
					}
				}
			}
		}
		if result.TaskListStatus != "" {
			break
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
	if result.TerminalStatus == "completed" && result.SameOriginHTTPSVideo {
		contentStatus, contentType, contentLength, width, height, videoDuration, err := inspectContent(
			ctx,
			client,
			result.VideoURL,
			spec.Credential,
			ffprobePath,
		)
		if err != nil {
			if result.Error == "" {
				result.Error = "content inspection request failed"
			}
			return
		}
		result.ContentHTTPStatus = contentStatus
		result.ContentType = contentType
		result.ContentLength = contentLength
		result.VideoWidth = width
		result.VideoHeight = height
		result.VideoDurationSeconds = videoDuration
	}
}

func inspectContent(
	ctx context.Context,
	client *http.Client,
	contentURL string,
	credential string,
	ffprobePath string,
) (int, string, int64, int, int, float64, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, contentURL, nil)
	if err != nil {
		return 0, "", 0, 0, 0, 0, err
	}
	request.Header.Set("Authorization", "Bearer "+credential)
	response, err := client.Do(request)
	if err != nil {
		return 0, "", 0, 0, 0, 0, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusPartialContent {
		return response.StatusCode, response.Header.Get("Content-Type"), response.ContentLength, 0, 0, 0,
			fmt.Errorf("content HTTP %d", response.StatusCode)
	}
	temporaryVideo, err := os.CreateTemp("", "feicai-v2-video-*.mp4")
	if err != nil {
		return response.StatusCode, response.Header.Get("Content-Type"), response.ContentLength, 0, 0, 0, err
	}
	temporaryPath := temporaryVideo.Name()
	defer os.Remove(temporaryPath)
	written, copyErr := io.Copy(temporaryVideo, response.Body)
	closeErr := temporaryVideo.Close()
	if copyErr != nil {
		return response.StatusCode, response.Header.Get("Content-Type"), written, 0, 0, 0, copyErr
	}
	if closeErr != nil {
		return response.StatusCode, response.Header.Get("Content-Type"), written, 0, 0, 0, closeErr
	}
	width, height, videoDuration, err := probeVideoMetadata(ctx, ffprobePath, temporaryPath)
	if err != nil {
		return response.StatusCode, response.Header.Get("Content-Type"), written, 0, 0, 0, err
	}
	return response.StatusCode, response.Header.Get("Content-Type"), written, width, height, videoDuration, nil
}

func probeVideoMetadata(ctx context.Context, ffprobePath, videoPath string) (int, int, float64, error) {
	command := exec.CommandContext(
		ctx,
		ffprobePath,
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height",
		"-of", "csv=s=x:p=0",
		videoPath,
	)
	output, err := command.Output()
	if err != nil {
		return 0, 0, 0, fmt.Errorf("ffprobe video dimensions: %w", err)
	}
	dimensions := strings.Split(strings.TrimSpace(string(output)), "x")
	if len(dimensions) != 2 {
		return 0, 0, 0, errors.New("ffprobe returned no video dimensions")
	}
	width, widthErr := strconv.Atoi(dimensions[0])
	height, heightErr := strconv.Atoi(dimensions[1])
	if widthErr != nil || heightErr != nil || width <= 0 || height <= 0 {
		return 0, 0, 0, errors.New("ffprobe returned invalid video dimensions")
	}
	durationCommand := exec.CommandContext(
		ctx,
		ffprobePath,
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		videoPath,
	)
	durationOutput, err := durationCommand.Output()
	if err != nil {
		return 0, 0, 0, fmt.Errorf("ffprobe video duration: %w", err)
	}
	videoDuration, err := strconv.ParseFloat(strings.TrimSpace(string(durationOutput)), 64)
	if err != nil || videoDuration <= 0 {
		return 0, 0, 0, errors.New("ffprobe returned invalid video duration")
	}
	return width, height, videoDuration, nil
}

func sameOriginHTTPS(baseURL *url.URL, raw string) bool {
	parsed, err := url.Parse(raw)
	return err == nil && parsed.Scheme == "https" && strings.EqualFold(parsed.Host, baseURL.Host) && parsed.User == nil
}

func endpoint(baseURL *url.URL, path string) string {
	return strings.TrimRight(baseURL.String(), "/") + path
}

func doProviderRequest(
	ctx context.Context,
	client *http.Client,
	method string,
	requestURL string,
	credential string,
	body []byte,
) (int, []byte, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, requestURL, reader)
	if err != nil {
		return 0, nil, err
	}
	request.Header.Set("Authorization", "Bearer "+credential)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(request)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, 2*1024*1024))
	return response.StatusCode, payload, err
}

func providerErrorCode(payload []byte) string {
	var envelope providerErrorEnvelope
	if common.Unmarshal(payload, &envelope) == nil {
		if code := strings.TrimSpace(envelope.Error.Code); code != "" {
			return code
		}
	}
	return "provider_error_response"
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(2)
}
