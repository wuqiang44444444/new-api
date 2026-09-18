package controller

import (
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaykittypes "github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/assert"
)

func TestTaskQuotaErrorKeepsFundingDistinctFromPermission(t *testing.T) {
	for _, tc := range []struct {
		name, code, wantCode, wantType string
		local                          bool
		status                         int
	}{
		{"local funding", string(relaykittypes.ErrorCodeInsufficientUserQuota), "insufficient_quota", "insufficient_quota", true, http.StatusForbidden},
		{"real permission", "model_not_allowed", "permission_denied", "permission_error", true, http.StatusForbidden},
		{"unclassified token failure", string(relaykittypes.ErrorCodePreConsumeTokenQuotaFailed), "permission_denied", "permission_error", true, http.StatusForbidden},
		{"provider funding code", string(relaykittypes.ErrorCodeInsufficientUserQuota), string(relaykittypes.ErrorCodeInsufficientUserQuota), "invalid_request_error", false, http.StatusForbidden},
		{"local server failure", string(relaykittypes.ErrorCodeInsufficientUserQuota), "internal_error", "server_error", true, http.StatusInternalServerError},
		{"existing payment required", "insufficient_quota", "insufficient_quota", "insufficient_quota", true, http.StatusPaymentRequired},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := &dto.TaskError{Code: tc.code, StatusCode: tc.status, LocalError: tc.local}
			if tc.local {
				input = service.TaskErrorFromAPIError(relaykittypes.NewErrorWithStatusCode(
					errors.New("private funding diagnostic"), relaykittypes.ErrorCode(tc.code), tc.status,
				))
			}
			status, code, errorType, message := taskProtocolErrorFields(input, nil)
			assert.Equal(t, tc.status, status)
			assert.Equal(t, tc.wantCode, code)
			assert.Equal(t, tc.wantType, errorType)
			assert.NotContains(t, message, "private funding diagnostic")
			if tc.wantCode == "insufficient_quota" {
				assert.Equal(t, "Insufficient quota", message)
			}
		})
	}
}
