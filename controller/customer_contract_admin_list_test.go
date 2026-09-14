package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestCustomerContractAdminListAPIProvidesSafeAggregateView(t *testing.T) {
	gin.SetMode(gin.TestMode)
	admin, user, contract := setupCustomerContractControllerDB(t)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/customer-contracts?keyword=contract-model&p=1&page_size=20", nil)
	c.Set("role", admin.Role)

	GetCustomerContracts(c)

	assert.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	assert.Contains(t, body, fmt.Sprintf(`"user_id":%d`, user.Id))
	assert.Contains(t, body, fmt.Sprintf(`"contract_id":%d`, contract.Id))
	assert.Contains(t, body, `"contract_status":"active"`)
	assert.Contains(t, body, `"rule_count":2`, "rule_count counts channel rules")
	assert.NotContains(t, body, "zero_access")
	assert.Contains(t, body, `"active":1`)
	assert.NotContains(t, body, `"route_group"`)
	assert.NotContains(t, body, "test-key")
}

func TestCustomerContractAdminListAPIRejectsUnknownStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	admin, _, _ := setupCustomerContractControllerDB(t)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/customer-contracts?status=unknown", nil)
	c.Set("role", admin.Role)

	GetCustomerContracts(c)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "invalid customer contract status")
}

func TestCustomerContractAdminListAPIRejectsOversizedSearch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	admin, _, _ := setupCustomerContractControllerDB(t)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/customer-contracts?keyword="+strings.Repeat("a", 256), nil)
	c.Set("role", admin.Role)

	GetCustomerContracts(c)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "customer contract search keyword is too long")
}
