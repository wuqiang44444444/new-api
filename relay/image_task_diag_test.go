package relay

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func nativeImageDiagResponse(status int, headers map[string]string, body string) *http.Response {
	resp := &http.Response{StatusCode: status, Header: http.Header{}}
	for key, value := range headers {
		resp.Header.Set(key, value)
	}
	resp.Body = io.NopCloser(strings.NewReader(body))
	return resp
}

func TestInspectNativeImageRejectionExtractsBoundedFacts(t *testing.T) {
	body := `{"error":{"code":"invalid_value","message":"` + "Failed check: SAFETY_CHECK_TYPE" + ` ... long provider text"}}`
	resp := nativeImageDiagResponse(400, map[string]string{"X-Request-Id": "req-abc-123"}, body)
	evidence := inspectNativeImageRejection(resp)
	assert.Equal(t, 400, evidence.StatusCode)
	assert.Equal(t, "req-abc-123", evidence.ProviderRequestID)
	assert.True(t, evidence.ViolationMarker)
}

func TestInspectNativeImageRejectionHeaderWhitelistAndPriority(t *testing.T) {
	resp := nativeImageDiagResponse(502, map[string]string{
		"X-Ms-Request-Id": "azure-id",
		"X-Request-Id":    "openai-id",
	}, `{}`)
	evidence := inspectNativeImageRejection(resp)
	assert.Equal(t, 502, evidence.StatusCode)
	assert.Equal(t, "openai-id", evidence.ProviderRequestID)
	assert.False(t, evidence.ViolationMarker)
}

func TestInspectNativeImageRejectionSanitizesExternalHeader(t *testing.T) {
	resp := nativeImageDiagResponse(429, map[string]string{
		"Apim-Request-Id": "line1\nline2\ttab\x00null " + strings.Repeat("x", 200),
	}, "")
	evidence := inspectNativeImageRejection(resp)
	assert.NotContains(t, evidence.ProviderRequestID, "\n")
	assert.NotContains(t, evidence.ProviderRequestID, "\t")
	assert.NotContains(t, evidence.ProviderRequestID, "\x00")
	assert.NotContains(t, evidence.ProviderRequestID, " ")
	assert.LessOrEqual(t, len(evidence.ProviderRequestID), 64)
}

func TestInspectNativeImageRejectionReadFailureKeepsFacts(t *testing.T) {
	resp := nativeImageDiagResponse(400, map[string]string{"X-Request-Id": "kept"}, "{}")
	resp.Body = io.NopCloser(errReader{})
	evidence := inspectNativeImageRejection(resp)
	assert.Equal(t, "kept", evidence.ProviderRequestID)
	assert.False(t, evidence.ViolationMarker)
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestInspectNativeImageRejectionUsesParsedPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       bool
	}{
		{"escaped_message", `{"error":{"message":"Content\u0020violates usage guidelines"}}`, true},
		{"unrelated_field", `{"error":{"message":"invalid quality"},"note":"Content violates usage guidelines"}`, false},
		{"policy_code", `{"error":{"code":"violation_fee.grok.csam","message":"rejected"}}`, true},
		{"truncated", `{"error":{"message":"Content violates usage guidelines"},"padding":"` + strings.Repeat("x", nativeImageRejectionBodyBudget) + `"}`, false},
		{"invalid_json", `Content violates usage guidelines`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			evidence := inspectNativeImageRejection(nativeImageDiagResponse(400, nil, tc.body))
			assert.Equal(t, tc.want, evidence.ViolationMarker)
		})
	}
}
