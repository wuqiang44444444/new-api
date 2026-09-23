package assets

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSafeUpstreamDiagnosticExposesOnlySanitizedFields(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		wantDiagnostic string
		wantOK         bool
	}{
		{
			name:           "HTTP status and safe provider code",
			err:            fmt.Errorf("wrapped: %w", &upstreamHTTPError{StatusCode: http.StatusTooManyRequests, ProviderCode: "RateLimit.Exceeded"}),
			wantDiagnostic: "stage=wait_response class=upstream_http status=429 provider_code=RateLimit.Exceeded",
			wantOK:         true,
		},
		{
			name:           "unsafe provider code omitted",
			err:            &upstreamHTTPError{StatusCode: http.StatusBadGateway, ProviderCode: "secret value"},
			wantDiagnostic: "stage=wait_response class=upstream_http status=502",
			wantOK:         true,
		},
		{
			name:           "oversized provider code omitted",
			err:            &upstreamHTTPError{StatusCode: http.StatusBadRequest, ProviderCode: strings.Repeat("a", 129)},
			wantDiagnostic: "stage=wait_response class=upstream_http status=400",
			wantOK:         true,
		},
		{
			name:           "numeric application code",
			err:            &upstreamApplicationError{provider: "provider", code: 40001},
			wantDiagnostic: "stage=decode_response class=application_error provider_code=40001",
			wantOK:         true,
		},
		{
			name:   "unknown error rejected",
			err:    errors.New("request URL with sensitive query"),
			wantOK: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			diagnostic, ok := SafeUpstreamDiagnostic(test.err)
			assert.Equal(t, test.wantDiagnostic, diagnostic)
			assert.Equal(t, test.wantOK, ok)
		})
	}
}

func TestClassifyTransportErrorUsesStableDiagnosticTaxonomy(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		wantDiagnostic string
	}{
		{name: "deadline", err: context.DeadlineExceeded, wantDiagnostic: "stage=wait_response class=timeout"},
		{name: "connect", err: &net.OpError{Op: "dial", Err: errors.New("unreachable")}, wantDiagnostic: "stage=wait_response class=connect"},
		{name: "reset", err: fmt.Errorf("wrapped: %w", syscall.ECONNRESET), wantDiagnostic: "stage=wait_response class=reset"},
		{name: "canceled", err: context.Canceled, wantDiagnostic: "stage=wait_response class=transport"},
		{name: "other", err: errors.New("transport failed"), wantDiagnostic: "stage=wait_response class=transport"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			diagnostic, ok := SafeUpstreamDiagnostic(fmt.Errorf("outer: %w", classifyTransportError(AssetStageWaitResponse, test.err)))
			assert.True(t, ok)
			assert.Equal(t, test.wantDiagnostic, diagnostic)
		})
	}
}

// UpstreamDiagnosticFields 是统一事件消费的结构化契约：阶段、类别、状态与已
// 脱敏 Provider code 必须与字符串版诊断一致，未知错误保持不可解构。
func TestUpstreamDiagnosticFieldsMatchStringContract(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantOK    bool
		wantStage string
		wantClass string
		wantCode  string
	}{
		{name: "http status", err: &upstreamHTTPError{StatusCode: 429, ProviderCode: "RateLimit.Exceeded"}, wantOK: true, wantStage: AssetStageWaitResponse, wantClass: AssetClassUpstreamHTTP, wantCode: "RateLimit.Exceeded"},
		{name: "http unsafe code dropped", err: &upstreamHTTPError{StatusCode: 502, ProviderCode: "secret value"}, wantOK: true, wantStage: AssetStageWaitResponse, wantClass: AssetClassUpstreamHTTP},
		{name: "numeric business error", err: &upstreamApplicationError{provider: "provider", code: 40001}, wantOK: true, wantStage: AssetStageDecodeResponse, wantClass: AssetClassApplicationError, wantCode: "40001"},
		{name: "string business error", err: &upstreamStringApplicationError{provider: "provider", code: "InvalidParameter"}, wantOK: true, wantStage: AssetStageDecodeResponse, wantClass: AssetClassApplicationError, wantCode: "InvalidParameter"},
		{name: "timeout transport", err: classifyTransportError(AssetStageUploadBody, context.DeadlineExceeded), wantOK: true, wantStage: AssetStageUploadBody, wantClass: AssetClassTimeout},
		{name: "unknown error", err: errors.New("opaque"), wantOK: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fields, ok := UpstreamDiagnosticFields(test.err)
			assert.Equal(t, test.wantOK, ok)
			assert.Equal(t, test.wantStage, fields.Stage)
			assert.Equal(t, test.wantClass, fields.Class)
			assert.Equal(t, test.wantCode, fields.ProviderCode)
		})
	}
}
