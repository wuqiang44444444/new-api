package assets

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"syscall"
)

const (
	// 素材代理错误的脱敏诊断阶段：区分"Provider 明确拒绝"与"请求可能已写出但结果不明"。
	AssetStageUploadBody     = "upload_body"
	AssetStageWaitResponse   = "wait_response"
	AssetStageDecodeResponse = "decode_response"

	// 素材代理错误的脱敏诊断类别。
	AssetClassTimeout          = "timeout"
	AssetClassConnect          = "connect"
	AssetClassReset            = "reset"
	AssetClassUpstreamHTTP     = "upstream_http"
	AssetClassApplicationError = "application_error"
	AssetClassInvalidResponse  = "invalid_response"
	AssetClassTransport        = "transport"
)

// upstreamTransportError 保留 transport 错误的发生阶段与类别。此前 transport 错误
// 只产出无结构 error，诊断日志丢弃了分类，无法区分超时、连接失败与响应等待阶段。
type upstreamTransportError struct {
	Stage string
	Class string
	Cause error
}

func (e *upstreamTransportError) Error() string {
	return fmt.Sprintf("asset upstream transport error at %s (%s)", e.Stage, e.Class)
}

func (e *upstreamTransportError) Unwrap() error {
	return e.Cause
}

func invalidUpstreamResponse(err error) error {
	return &upstreamTransportError{Stage: AssetStageDecodeResponse, Class: AssetClassInvalidResponse, Cause: err}
}

// classifyTransportError 把 provider 调用的 transport 失败包装为带阶段与类别的结构化
// 错误。分类近似：客户端整体超时归为 timeout；连接建立失败归为 connect；连接被对端
// 重置归为 reset；其余归为 transport。context.Canceled 不视为 provider 故障。
func classifyTransportError(stage string, err error) error {
	if err == nil {
		return nil
	}
	class := AssetClassTransport
	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) {
		class = AssetClassTimeout
	} else if errors.As(err, &netErr) && netErr.Timeout() {
		class = AssetClassTimeout
	} else if errors.Is(err, context.Canceled) {
		class = AssetClassTransport
	} else if errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE) {
		class = AssetClassReset
	} else {
		var opErr *net.OpError
		if errors.As(err, &opErr) && opErr.Op == "dial" {
			class = AssetClassConnect
		}
	}
	return &upstreamTransportError{Stage: stage, Class: class, Cause: err}
}

// UpstreamDiagnostic carries the bounded, non-sensitive fields already
// produced by the asset adapter error types so text logs and the unified
// error event share one source. Callers must not log the original error or
// response body.
type UpstreamDiagnostic struct {
	Stage        string
	Class        string
	StatusCode   int
	ProviderCode string
}

// UpstreamDiagnosticFields extracts the structured diagnostic from known
// asset adapter errors.
func UpstreamDiagnosticFields(err error) (UpstreamDiagnostic, bool) {
	var transportErr *upstreamTransportError
	if errors.As(err, &transportErr) {
		return UpstreamDiagnostic{Stage: transportErr.Stage, Class: transportErr.Class}, true
	}
	var statusErr *upstreamHTTPError
	if errors.As(err, &statusErr) {
		return UpstreamDiagnostic{
			Stage:        AssetStageWaitResponse,
			Class:        AssetClassUpstreamHTTP,
			StatusCode:   statusErr.StatusCode,
			ProviderCode: sanitizeProviderCodeForDiagnostic(statusErr.ProviderCode),
		}, true
	}
	var applicationErr *upstreamApplicationError
	if errors.As(err, &applicationErr) {
		return UpstreamDiagnostic{
			Stage:        AssetStageDecodeResponse,
			Class:        AssetClassApplicationError,
			ProviderCode: strconv.Itoa(applicationErr.code),
		}, true
	}
	var stringApplicationErr *upstreamStringApplicationError
	if errors.As(err, &stringApplicationErr) {
		return UpstreamDiagnostic{
			Stage:        AssetStageDecodeResponse,
			Class:        AssetClassApplicationError,
			ProviderCode: sanitizeProviderCodeForDiagnostic(stringApplicationErr.code),
		}, true
	}
	return UpstreamDiagnostic{}, false
}

// SafeUpstreamDiagnostic returns only bounded, non-sensitive fields from known
// asset adapter errors. Callers must not log the original error or response body.
func SafeUpstreamDiagnostic(err error) (string, bool) {
	fields, ok := UpstreamDiagnosticFields(err)
	if !ok {
		return "", false
	}
	return formatUpstreamDiagnostic(fields), true
}

func formatUpstreamDiagnostic(diag UpstreamDiagnostic) string {
	if diag.StatusCode > 0 && diag.ProviderCode != "" {
		return fmt.Sprintf("stage=%s class=%s status=%d provider_code=%s", diag.Stage, diag.Class, diag.StatusCode, diag.ProviderCode)
	}
	if diag.StatusCode > 0 {
		return fmt.Sprintf("stage=%s class=%s status=%d", diag.Stage, diag.Class, diag.StatusCode)
	}
	if diag.ProviderCode != "" {
		return fmt.Sprintf("stage=%s class=%s provider_code=%s", diag.Stage, diag.Class, diag.ProviderCode)
	}
	return fmt.Sprintf("stage=%s class=%s", diag.Stage, diag.Class)
}

func sanitizeProviderCodeForDiagnostic(providerCode string) string {
	if len(providerCode) > 128 {
		return ""
	}
	for _, character := range providerCode {
		if character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' ||
			character == '.' || character == '_' || character == '-' {
			continue
		}
		return ""
	}
	return providerCode
}
