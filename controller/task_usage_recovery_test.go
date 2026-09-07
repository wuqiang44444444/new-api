package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskUsageReviewRequiresVerifiedStatementAndMatchingProof(t *testing.T) {
	require.True(t, isAllowedSecurityProofScope(securityProofScopeTaskUsageReview))
	for _, tc := range []struct {
		name, scope, body string
		status            int
	}{
		{"unverified", securityProofScopeTaskUsageReview, `{"completion_tokens":0,"reference":"statement"}`, 400},
		{"wrong scope", securityProofScopeTaskContractAttemptRecover, `{"completion_tokens":100,"provider_verified":true,"reference":"statement"}`, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			identity := service.AuthIdentity{UserID: 9, SessionID: "review-session", UserAuthVersion: 1, SessionVersion: 1}
			proof, _, err := service.IssueSecurityProof(identity, "2fa", []string{tc.scope})
			require.NoError(t, err)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/task-contract/usage-recovery/task-1/review", strings.NewReader(tc.body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Request.Header.Set("X-Security-Proof", proof)
			c.Set("id", 9)
			c.Set("auth_identity", identity)
			ReviewTaskUsage(c)
			assert.Equal(t, tc.status, rec.Code)
		})
	}
}

func TestTaskUsageRecoveryListRejectsInvalidPagination(t *testing.T) {
	for _, q := range []string{"limit=0", "limit=101", "offset=-1", "limit=invalid"} {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/task-contract/usage-recovery?"+q, nil)
		ListTaskUsageRecovery(c)
		assert.Equal(t, 400, rec.Code)
	}
}
