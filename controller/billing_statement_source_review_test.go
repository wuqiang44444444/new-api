package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestBillingSourceReviewPermissionsAndLegacyRequest(t *testing.T) {
	for _, tc := range []struct {
		name    string
		role    int
		handler gin.HandlerFunc
		body    string
		status  int
	}{
		{"admin_cannot_attest", common.RoleAdminUser, PostAdminBillingStatementSourceVerification, `{}`, http.StatusForbidden},
		{"admin_cannot_accept", common.RoleAdminUser, PostAdminBillingSourceReview, `{}`, http.StatusForbidden},
		{"legacy_accept_cannot_decide", common.RoleRootUser, PostAdminBillingSourceReview, `{"accept":true,"note":"trust me","fingerprint":"old","issue_id":"log:1"}`, http.StatusConflict},
		{"free_text_cannot_attest", common.RoleRootUser, PostAdminBillingStatementSourceVerification, `{"status":"intact","evidence":"trust me"}`, http.StatusConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Set("role", tc.role)
			c.Set("id", 1)
			c.Request = httptest.NewRequest("POST", "/?user_id=91&start_timestamp=1785513600&end_timestamp=1788191999", strings.NewReader(tc.body))
			c.Request.Header.Set("Content-Type", "application/json")
			tc.handler(c)
			assert.Equal(t, tc.status, w.Code)
		})
	}
}
