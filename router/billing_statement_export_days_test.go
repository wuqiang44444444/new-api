package router

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomerStatementExportDaysRequiresAdministratorAndBillingRead(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.UserAccessToken{}, &model.AuditLog{}))
	router := gin.New()
	registerBillingReconciliationRoutes(router.Group("/api"))
	const path = "/api/billing/admin/customer-statement-export-days?start_timestamp=1788192000&end_timestamp=1790783999"

	for _, tc := range []struct {
		name, scope, rejection string
		role                   int
		detailPath             string
	}{
		{name: "administrator with billing read", scope: "billing:read", role: common.RoleAdminUser},
		{name: "administrator missing billing read", scope: "profile:read", role: common.RoleAdminUser, rejection: "ACCESS_TOKEN_SCOPE_DENIED"},
		{name: "customer with billing read", scope: "billing:read", role: common.RoleCommonUser, rejection: "AUTH_INSUFFICIENT_PRIVILEGE"},
		{name: "self detail dates", scope: "usage:read", role: common.RoleCommonUser, detailPath: "/statement/self/detail-export-days"},
		{name: "self detail dates without scope", scope: "profile:read", role: common.RoleCommonUser, detailPath: "/statement/self/detail-export-days", rejection: "ACCESS_TOKEN_SCOPE_DENIED"},
		{name: "admin detail dates", scope: "billing:read", role: common.RoleAdminUser, detailPath: "/admin/customer-detail-export-days"},
		{name: "customer denied admin detail dates", scope: "billing:read", role: common.RoleCommonUser, detailPath: "/admin/customer-detail-export-days", rejection: "AUTH_INSUFFICIENT_PRIVILEGE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			suffix, err := common.GenerateRandomCharsKey(43)
			require.NoError(t, err)
			raw := model.AccessTokenPrefix + suffix
			user := model.User{Username: tc.name, AffCode: tc.name, Status: common.UserStatusEnabled, Role: tc.role}
			require.NoError(t, model.DB.Create(&user).Error)
			token := model.UserAccessToken{Name: "billing export dates", TokenHash: model.AccessTokenFingerprint(raw), TokenHint: model.AccessTokenHint(raw)}
			require.NoError(t, token.SetScopes([]string{tc.scope}))
			require.NoError(t, model.CreateUserAccessToken(user.Id, &token, service.AccessTokenMaxPerUser))
			url := path
			if tc.detailPath != "" {
				url = "/api/billing" + tc.detailPath + "?start_timestamp=1788192000&end_timestamp=1790783999&report_version=-&user_id=" + strconv.Itoa(user.Id)
			}
			request := httptest.NewRequest(http.MethodGet, url, nil)
			request.Header.Set("Authorization", "Bearer "+raw)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if tc.rejection == "" {
				assert.Equal(t, http.StatusOK, response.Code)
				assert.Contains(t, response.Body.String(), `"success":true`)
				if tc.detailPath != "" {
					assert.Contains(t, response.Body.String(), `"days":[]`)
				}
				return
			}
			assert.Equal(t, http.StatusForbidden, response.Code)
			assert.Contains(t, response.Body.String(), tc.rejection)
		})
	}
}
