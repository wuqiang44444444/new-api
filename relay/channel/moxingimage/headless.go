package moxingimage

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
)

// HeadlessGenerate executes the single synchronous Moxing POST without a
// client context（异步图片 worker 专用），返回全部有效结果 URL。
func HeadlessGenerate(ctx context.Context, info *relaycommon.RelayInfo, headers map[string]string, requestBody io.Reader) ([]string, *dto.Usage, *types.NewAPIError) {
	if info == nil {
		return nil, nil, upstreamError("missing relay info")
	}
	adaptor := &Adaptor{}
	requestURL, err := adaptor.GetRequestURL(info)
	if err != nil {
		return nil, nil, upstreamError("failed to build image request URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, requestBody)
	if err != nil {
		return nil, nil, upstreamError("failed to build image request")
	}
	header := http.Header{}
	if err := adaptor.SetupRequestHeader(nil, &header, info); err != nil {
		return nil, nil, types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	req.Header = header
	for key, value := range headers {
		req.Header.Set(key, value)
		if strings.EqualFold(key, "Host") {
			req.Host = value
		}
	}
	client, err := adaptor.httpClient(info)
	if err != nil {
		return nil, nil, upstreamError("failed to initialize image provider client")
	}
	resp, err := client.Do(req)
	service.ObserveImageHTTPExchange(ctx, req, resp, err, "upstream_response")
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, nil, timeoutError(err)
		}
		return nil, nil, upstreamError("image request failed")
	}
	defer service.CloseResponseBodyGracefully(resp)
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, nil, upstreamError("failed to read image response")
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, nil, providerHTTPError(resp.StatusCode, body)
	}
	var provider providerResponse
	if err := common.Unmarshal(body, &provider); err != nil {
		return nil, nil, upstreamError("invalid image response")
	}
	return normalizeResult(provider, info.UpstreamModelName)
}
