package router

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelAffinityCacheDetailsRequiresRoot(t *testing.T) {
	setupRelayRouterTestDB(t)
	engine := gin.New()
	SetApiRouter(engine)
	const path = "/api/option/channel_affinity_cache/entries"
	for _, tc := range []struct {
		role int
		want int
	}{
		{0, http.StatusUnauthorized},
		{common.RoleCommonUser, http.StatusForbidden},
		{common.RoleAdminUser, http.StatusForbidden},
		{common.RoleRootUser, http.StatusOK},
	} {
		request := httptest.NewRequest(http.MethodGet, path+"?layer=session", nil)
		if tc.role != 0 {
			token := "affinity-inspection-" + strconv.Itoa(tc.role)
			require.NoError(t, model.DB.Create(&model.User{
				Username: token, AffCode: token, Group: "default", Status: common.UserStatusEnabled,
				Role: tc.role, AccessToken: &token, AuthVersion: 1,
			}).Error)
			request.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		require.Equal(t, tc.want, response.Code, response.Body.String())
		if tc.role == common.RoleRootUser {
			assert.Contains(t, response.Body.String(), `"entries":[`)
			assert.Contains(t, response.Header().Get("Cache-Control"), "no-store")
			for _, query := range []string{"?layer=invalid", "?layer=session&cursor=bad"} {
				invalid := httptest.NewRequest(http.MethodGet, path+query, nil)
				invalid.Header.Set("Authorization", request.Header.Get("Authorization"))
				result := httptest.NewRecorder()
				engine.ServeHTTP(result, invalid)
				assert.Equal(t, http.StatusBadRequest, result.Code)
			}
		}
	}
}
