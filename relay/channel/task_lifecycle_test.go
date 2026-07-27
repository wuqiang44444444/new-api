package channel

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsDefinitiveTaskLifecycleRejectionExcludesRetryableClientStatuses(t *testing.T) {
	for _, status := range []int{http.StatusRequestTimeout, http.StatusTooEarly, http.StatusTooManyRequests} {
		assert.False(t, IsDefinitiveTaskLifecycleRejection(&TaskLifecycleError{StatusCode: status}))
	}
	for _, status := range []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusConflict} {
		assert.True(t, IsDefinitiveTaskLifecycleRejection(&TaskLifecycleError{StatusCode: status}))
	}
	assert.False(t, IsDefinitiveTaskLifecycleRejection(&TaskLifecycleError{StatusCode: http.StatusBadGateway}))
}
