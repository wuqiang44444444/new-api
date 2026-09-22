package controller

import (
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVideoRefundRequiresSessionBoundProof(t *testing.T) {
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Params = gin.Params{{Key: "kind", Value: "task"}, {Key: "id", Value: "42"}}
	c.Request = httptest.NewRequest("POST", "/api/video-funds/task/42/refund", strings.NewReader(`{"kind":"task","id":42,"version":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","note":"missing result"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	RefundVideoFunds(c)
	assert.Equal(t, 403, response.Code)
	assert.Contains(t, response.Body.String(), "SECURITY_PROOF_INVALID")
}
