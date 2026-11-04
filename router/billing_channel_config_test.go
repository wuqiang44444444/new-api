package router

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelBillingRoutesRequireAdministratorAndRetireURLNameWrites(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, model.DB.Create(&model.Channel{Id: 1, Name: "test channel"}).Error)
	router := gin.New()
	registerBillingReconciliationRoutes(router.Group("/api"))
	for _, role := range []int{0, common.RoleCommonUser, common.RoleAdminUser, common.RoleRootUser} {
		pat := ""
		if role > 0 {
			pat = "billing-config-fixture-" + strconv.Itoa(role)
			require.NoError(t, model.DB.Create(&model.User{Username: pat, AffCode: pat, Status: common.UserStatusEnabled, Role: role, Group: "default", AccessToken: &pat, AuthVersion: 1}).Error)
		}
		for _, method := range []string{http.MethodGet, http.MethodPut} {
			path, body := "/api/billing/admin/channel-config?channel_id=1&period_start=1788192000", ""
			if method == http.MethodPut {
				path, body = "/api/billing/admin/channel-name", `{"channel_id":1,"name":"Manual supplier"}`
			}
			request := httptest.NewRequest(method, path, strings.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			if pat != "" {
				request.Header.Set("Authorization", "Bearer "+pat)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			expected := http.StatusOK
			if role == 0 {
				expected = http.StatusUnauthorized
			} else if role < common.RoleAdminUser {
				expected = http.StatusForbidden
			}
			assert.Equal(t, expected, response.Code)
			if role >= common.RoleAdminUser {
				assert.Contains(t, response.Body.String(), `"success":true`)
			}
		}
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPut, "/api/billing/admin/upstream-url-names", strings.NewReader(`{}`)))
	assert.Equal(t, http.StatusNotFound, response.Code)
}
