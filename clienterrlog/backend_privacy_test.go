package clienterrlog

import (
	"github.com/stretchr/testify/assert"
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
