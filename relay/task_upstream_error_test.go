package relay

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseTaskUpstreamHTTPErrorExtractsAllowlistedDetails(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantCode    string
		wantMessage string
		wantError   string
	}{
		{
			name:        "nested provider error",
			body:        `{"error":{"code":"InvalidParameter","message":"service_tier is not supported"},"debug":"secret"}`,
			wantCode:    "InvalidParameter",
			wantMessage: "service_tier is not supported",
			wantError:   "upstream task request returned HTTP 400 (InvalidParameter): service_tier is not supported",
		},
		{
			name:        "direct provider error",
			body:        `{"code":"BadRequest","message":"invalid duration"}`,
			wantCode:    "BadRequest",
			wantMessage: "invalid duration",
			wantError:   "upstream task request returned HTTP 400 (BadRequest): invalid duration",
		},
		{
			name:      "unsafe message is not surfaced",
			body:      `{"error":{"code":"BadRequest","message":"inspect https://provider.example/private?token=secret"}}`,
			wantCode:  "BadRequest",
			wantError: "upstream task request returned HTTP 400 (BadRequest)",
		},
		{
			name:        "invalid provider code is not surfaced",
			body:        `{"error":{"code":"bad code\ninjected","message":"request rejected"}}`,
			wantError:   "upstream task request returned HTTP 400: request rejected",
			wantMessage: "request rejected",
		},
		{
			name:      "malformed response stays generic",
			body:      `<html>gateway error</html>`,
			wantError: "upstream task request returned HTTP 400",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := parseTaskUpstreamHTTPError(400, []byte(test.body))

			assert.Equal(t, test.wantCode, result.providerCode)
			assert.Equal(t, test.wantMessage, result.providerMessage)
			assert.Equal(t, test.wantError, result.Error())
		})
	}
}

func TestParseTaskUpstreamHTTPErrorBoundsProviderMessage(t *testing.T) {
	result := parseTaskUpstreamHTTPError(502, []byte(`{"message":"`+strings.Repeat("猫", 400)+`"}`))

	assert.NotEmpty(t, result.providerMessage)
	assert.True(t, strings.HasSuffix(result.providerMessage, "…"))
	assert.LessOrEqual(t, len(result.providerMessage), 515)
	assert.NotContains(t, result.Error(), strings.Repeat("猫", 400))
}
