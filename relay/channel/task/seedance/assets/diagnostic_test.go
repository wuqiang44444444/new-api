package assets

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
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
			wantDiagnostic: "status=429 provider_code=RateLimit.Exceeded",
			wantOK:         true,
		},
		{
			name:           "unsafe provider code omitted",
			err:            &upstreamHTTPError{StatusCode: http.StatusBadGateway, ProviderCode: "secret value"},
			wantDiagnostic: "status=502",
			wantOK:         true,
		},
		{
			name:           "oversized provider code omitted",
			err:            &upstreamHTTPError{StatusCode: http.StatusBadRequest, ProviderCode: strings.Repeat("a", 129)},
			wantDiagnostic: "status=400",
			wantOK:         true,
		},
		{
			name:           "numeric application code",
			err:            &upstreamApplicationError{provider: "provider", code: 40001},
			wantDiagnostic: "provider_code=40001",
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
