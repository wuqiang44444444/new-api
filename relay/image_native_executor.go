package relay

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
)

// Unlike the older 64 MiB reader, this budget detects overflow explicitly.
// It accommodates multiple b64 images while bounding each worker's response.
const maxNativeImageResponseBytes = 512 << 20

var errNativeImageResponseTooLarge = errors.New("native image response exceeds the read budget")

func readNativeImageResponse(reader io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errNativeImageResponseTooLarge
	}
	return data, nil
}

func executeNativeImageTask(ctx context.Context, task *model.Task) service.ImageTaskExecution {
	data := task.PrivateData.ImageTask
	unknown := service.ImageTaskExecution{Outcome: service.ImageTaskOutcomeUnknown, FailureCode: "native_image_outcome_unknown"}
	// The worker commits the send permit first. Direct callers cannot bypass it.
	if data.SentAt == 0 {
		return unknown
	}
	encoded, err := common.DecryptShortLivedSecretForScope("image-native:"+task.TaskID, data.NativeRequest.ConnectionCiphertext)
	if err != nil {
		unknown.FailureCode = "connection_snapshot_unreadable"
		return unknown
	}
	var connection nativeImageConnection
	if err := common.Unmarshal([]byte(encoded), &connection); err != nil {
		unknown.FailureCode = "connection_snapshot_unreadable"
		return unknown
	}
	body, err := service.RestoreImageTaskPayload(ctx, data.NativeRequest.Body)
	if err != nil {
		return service.ImageTaskExecution{Outcome: service.ImageTaskOutcomeFailure, FailureCode: "request_storage_unavailable"}
	}
	defer os.Remove(body.Name())
	defer body.Close()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, connection.URL, body)
	if err != nil {
		return service.ImageTaskExecution{Outcome: service.ImageTaskOutcomeFailure, FailureCode: "request_build_failed"}
	}
	for _, ref := range data.NativeRequest.Body {
		req.ContentLength += ref.Size
	}
	req.Header = connection.Headers
	req.Host = connection.Headers.Get("Host")
	// No GetBody: even a forwarded Idempotency-Key cannot authorize a transport
	// replay. Redirects must not send the frozen credentials to another endpoint.
	client, err := service.GetHttpClientWithProxySettings(connection.Settings.Proxy, connection.Settings)
	if err != nil {
		return service.ImageTaskExecution{Outcome: service.ImageTaskOutcomeFailure, FailureCode: "connection_unavailable"}
	}
	oneShot := *client
	oneShot.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := oneShot.Do(req)
	if err != nil {
		return unknown
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != http.StatusRequestTimeout && resp.StatusCode != http.StatusTooManyRequests {
		return service.ImageTaskExecution{Outcome: service.ImageTaskOutcomeFailure, FailureCode: "provider_rejected"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return unknown
	}
	raw, err := readNativeImageResponse(resp.Body, maxNativeImageResponseBytes)
	if err != nil {
		unknown.FailureCode = "response_read_failed"
		if errors.Is(err, errNativeImageResponseTooLarge) {
			unknown.FailureCode = "response_size_limit"
		}
		return unknown
	}
	var result struct {
		Data  []dto.ImageData `json:"data"`
		Error any             `json:"error"`
	}
	var usage *dto.Usage
	info := buildFrozenImageRelayInfo(task, data)
	if strings.HasPrefix(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		result.Data, usage, err = openai.ImageTaskStreamResponse(info, raw)
	} else {
		err = common.Unmarshal(raw, &result)
		if err == nil {
			usage, err = openai.ImageTaskUsage(info, raw)
		}
	}
	if err != nil || result.Error != nil || len(result.Data) == 0 || len(result.Data) > int(dto.MaxImageN) {
		unknown.FailureCode = "invalid_provider_response"
		return unknown
	}
	raw = nil
	artifacts, err := storeImageResults(ctx, task, len(result.Data), usage, func(index int) ([]byte, string, error) {
		item := result.Data[index]
		if item.B64Json != "" {
			if base64.StdEncoding.DecodedLen(len(item.B64Json)) > maxImageResultBytes {
				return nil, "", errors.New("image result exceeds storage budget")
			}
			decoded, err := base64.StdEncoding.DecodeString(item.B64Json)
			return decoded, "", err
		}
		if strings.TrimSpace(item.Url) != "" {
			return downloadImageURL(ctx, item.Url)
		}
		return nil, "", errors.New("image result is missing")
	})
	if err != nil {
		return unknown
	}
	return service.ImageTaskExecution{Outcome: service.ImageTaskOutcomeSuccess, Images: artifacts, Usage: usage}
}
