package clienterrlog

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestBackendEventDropsUnapprovedDetailsAndSanitizesFailure(t *testing.T) {
	event := buildBackendEvent(BackendEvent{EventType: EventTaskFailure, Detail: map[string]string{
		"platform": "suno", "fail_reason": "failed https://example.com/image?signature=fixture-secret api_key=fixture-key", "provider_body": "private-response", "Authorization": "Bearer fixture-token",
	}})
	assert.Equal(t, "suno", event.Detail["platform"])
	assert.NotContains(t, event.Detail, "provider_body")
	assert.NotContains(t, event.Detail, "Authorization")
	assert.NotContains(t, event.Detail["fail_reason"], "fixture-secret")
	assert.NotContains(t, event.Detail["fail_reason"], "fixture-key")
	assert.NotContains(t, event.Detail["fail_reason"], "https://")
}

func TestBackendEventPreservesCompleteDiagnostics(t *testing.T) {
	detail := map[string]string{
		"platform": "openai", "action": "probe", "test_mode": "auto",
		"endpoint_type": "chat", "probe_stream": "true", "upstream_status": "401",
		"threshold_kind": "response_time", "check_scope": "generation_probe",
		"config_check": "passed", "generation_evidence": "not_verified",
		"upstream_request": "response_received", "config_reason": "protocol_invalid",
		"config_entry": "channel_edit", "config_summary": "Check the channel protocol configuration.",
		"readonly_check": "unsupported", "billing_model": "customer-model",
		"check_result": "failed", "check_reason": "test_upstream_rejected",
		"check_code": "bad_response_status_code", "connection_result": "auth_error",
		"probe_media": "image", "upstream_cost_status": "pending",
	}
	event := buildBackendEvent(BackendEvent{EventType: EventChannelTest, Detail: detail})
	require.NotNil(t, event.Detail)
	// Existing sanitization replaces spaces; completeness must not bypass it.
	detail["config_summary"] = "Check_the_channel_protocol_configuration."
	assert.Equal(t, detail, event.Detail, "all approved diagnostic fields must survive together")
}
