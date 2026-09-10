package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestTaskContractWriteOperationsRequireSessionBoundSecurityProof(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name    string
		path    string
		handler gin.HandlerFunc
	}{
		{name: "usage review", path: "/api/task-contract/usage-recovery/task_1/review", handler: ReviewTaskUsage},
		{
			name:    "attempt recovery",
			path:    "/api/task-contract/attempts/attempt_1/recover",
			handler: RecoverTaskCreateAttempt,
		},
		{
			name:    "attempt rejection",
			path:    "/api/task-contract/attempts/attempt_1/reject",
			handler: RejectTaskCreateAttempt,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest(
				http.MethodPost,
				test.path,
				strings.NewReader(`{}`),
			)

			test.handler(context)

			assert.Equal(t, http.StatusForbidden, recorder.Code)
			assert.Contains(t, recorder.Body.String(), "SECURITY_PROOF_INVALID")
		})
	}
}

func TestTaskContractAttemptRejectSecurityProofScopeIsAllowed(t *testing.T) {
	_, err := service.BindVerificationOperation(service.VerificationOperation{Scope: securityProofScopeTaskContractAttemptReject})
	assert.NoError(t, err)
}
