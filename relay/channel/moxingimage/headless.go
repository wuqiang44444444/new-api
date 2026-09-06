package moxingimage

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
)

// HeadlessGenerate executes the single synchronous Moxing POST without a
// client context（异步图片 worker 专用），返回全部有效结果 URL。
func HeadlessGenerate(ctx context.Context, info *relaycommon.RelayInfo, headers map[string]string, requestBody io.Reader) ([]string, *types.NewAPIError) {
	if info == nil {
		return nil, upstreamError("missing relay info")
	}
	adaptor := &Adaptor{}
	requestURL, err := adaptor.GetRequestURL(info)
	if err != nil {
		return nil, upstreamError("failed to build image request URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, requestBody)
	if err != nil {
		return nil, upstreamError("failed to build image request")
	}
	header := http.Header{}
	if err := adaptor.SetupRequestHeader(nil, &header, info); err != nil {
		return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
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
		return nil, upstreamError("failed to initialize image provider client")
	}
	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, timeoutError(err)
		}
		return nil, upstreamError("image request failed")
	}
	defer service.CloseResponseBodyGracefully(resp)
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, upstreamError("failed to read image response")
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, providerHTTPError(resp.StatusCode, body)
	}
	var provider providerResponse
	if err := common.Unmarshal(body, &provider); err != nil {
		return nil, upstreamError("invalid image response")
	}
	if hasProviderError(provider.Error) {
		return nil, providerApplicationError(provider.Error)
	}
	if name := trimSpaces(provider.Model); name != "" && name != info.UpstreamModelName {
		return nil, upstreamError("image response model does not match the request")
	}
	if len(provider.Data) == 0 {
		return nil, upstreamError("image provider returned no image")
	}
	urls := make([]string, 0, len(provider.Data))
	for _, image := range provider.Data {
		if candidate := trimSpaces(image.URL); isHTTPURL(candidate) && trimSpaces(image.B64JSON) == "" {
			urls = append(urls, candidate)
		}
	}
	if len(urls) == 0 {
		return nil, upstreamError("image provider result must contain HTTP(S) URLs")
	}
	return urls, nil
}

func trimSpaces(value string) string {
	for len(value) > 0 && (value[0] == ' ' || value[0] == '\t' || value[0] == '\n' || value[0] == '\r') {
		value = value[1:]
	}
	for len(value) > 0 {
		last := value[len(value)-1]
		if last == ' ' || last == '\t' || last == '\n' || last == '\r' {
			value = value[:len(value)-1]
			continue
		}
		break
	}
	return value
}
