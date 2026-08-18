package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomerContractAdminListAPIProvidesSafeAggregateView(t *testing.T) {
	gin.SetMode(gin.TestMode)
	admin, user := setupCustomerContractControllerDB(t)
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", user.Id).Updates(map[string]any{
		"contract_mode": true, "contract_version": 1,
	}).Error)
	require.NoError(t, model.DB.Create(&model.CustomerModelContract{
		UserId: user.Id, PublicModel: "contract-model", RouteGroup: "contract-api", RatioUnits: 80_000_000,
	}).Error)
	require.NoError(t, model.DB.Create(&model.CustomerContractAudit{
		UserId: user.Id, ContractVersion: 1, AdminUserId: admin.Id,
		Operation: "create", Reason: "signed contract", CreatedAt: 100,
	}).Error)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/customer-contracts?keyword=contract-model&p=1&page_size=20", nil)
	c.Set("role", admin.Role)

	GetCustomerContracts(c)

	assert.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	assert.Contains(t, body, fmt.Sprintf(`"user_id":%d`, user.Id))
	assert.Contains(t, body, `"contract_status":"active"`)
	assert.Contains(t, body, `"rule_count":1`)
	assert.Contains(t, body, `"active":1`)
	assert.NotContains(t, body, `"route_group"`)
	assert.NotContains(t, body, "test-key")
}

func TestCustomerContractAdminListAPIRejectsUnknownStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	admin, _ := setupCustomerContractControllerDB(t)
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
	admin, _ := setupCustomerContractControllerDB(t)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/customer-contracts?keyword="+strings.Repeat("a", 256), nil)
	c.Set("role", admin.Role)

	GetCustomerContracts(c)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "customer contract search keyword is too long")
}
