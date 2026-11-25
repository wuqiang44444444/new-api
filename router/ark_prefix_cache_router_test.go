package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArkPilotRouterKeepsManagedIDsOutOfDistributionWhenDisabled(t *testing.T) {
	t.Setenv("ARK_PREFIX_CACHE_PILOT", "")
	setupRelayRouterTestDB(t)
	user := model.User{Username: "ark-disabled-router", Group: "default", Status: common.UserStatusEnabled, Quota: 100}
	require.NoError(t, model.DB.Create(&user).Error)
	token := model.Token{UserId: user.Id, Key: "arkdisabledroutertest", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}
	require.NoError(t, model.DB.Create(&token).Error)
	engine := gin.New()
	setTaskPluginProtocolRouterWithArkPilot(engine)
	request := httptest.NewRequest(http.MethodPost, "/v1/responses",
		strings.NewReader(`{"model":"native-model","previous_response_id":"`+model.ArkSessionCacheIDPrefix+`saved"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token.Key)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	assert.Equal(t, http.StatusNotFound, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), "cache_not_found")
}
