package channel

import (
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
)

func TestImageRelayHTTPClientUsesFixedDefaultAndShorterGlobalLimit(t *testing.T) {
	originalRelayTimeout := common.RelayTimeout
	t.Cleanup(func() { common.RelayTimeout = originalRelayTimeout })

	base := &http.Client{}
	common.RelayTimeout = 0
	fixed := ImageRelayHTTPClient(base, time.Now())
	assert.Zero(t, base.Timeout)
	assert.Greater(t, fixed.Timeout, 9*time.Minute)
	assert.LessOrEqual(t, fixed.Timeout, 10*time.Minute)

	common.RelayTimeout = 30
	shorter := ImageRelayHTTPClient(base, time.Now())
	assert.Greater(t, shorter.Timeout, 29*time.Second)
	assert.LessOrEqual(t, shorter.Timeout, 30*time.Second)
}
