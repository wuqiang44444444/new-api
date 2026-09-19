package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBatchContractTokenAuthChecksSavedGroupOnlyInNativeMode(t *testing.T) {
	// The HTTP auth path uses the database dialect's initialized key quoting.
	// Bootstrap it against a private in-memory database, never the workspace DB.
	previousDB, previousPath, previousMaster := model.DB, common.SQLitePath, common.IsMasterNode
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	t.Setenv("SQL_DSN", "local")
	t.Setenv("LOG_SQL_DSN", "")
	common.SQLitePath, common.IsMasterNode = "file:batch_auth_init?mode=memory&cache=shared", true
	require.NoError(t, model.InitDB())
	bootstrap, err := model.DB.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = bootstrap.Close()
		model.DB, common.SQLitePath, common.IsMasterNode = previousDB, previousPath, previousMaster
		common.SetMainDatabaseType(previousMainType)
		common.SetLogDatabaseType(previousLogType)
	})
	db, user, contract := setupCustomerContractMiddlewareDB(t)
	require.NoError(t, db.Model(&user).Update("status", common.UserStatusEnabled).Error)
	token := model.Token{UserId: user.Id, Key: "batchauthregression", Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 1000, Group: "default", ContractId: contract.Id}
	require.NoError(t, db.Create(&token).Error)
	// An active contract replaces the saved Key route group; a disabled
	// contract restores the original saved-group permission check.
	stale := model.Token{UserId: user.Id, Key: "batchauthstalegroup", Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 1000, Group: "removed-native-group", ContractId: contract.Id}
	require.NoError(t, db.Create(&stale).Error)
	internal := model.Token{UserId: user.Id, Key: "batchauthinternalgroup", Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 1000, Group: "contract-route", ContractId: contract.Id}
	require.NoError(t, db.Create(&internal).Error)
	router := gin.New()
	router.POST("/v1/batches", TokenAuth(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	for _, path := range []string{"/v1/batches/:id", "/v1/files/:id/content", "/v1/videos/:id", "/v1/tasks/:id"} {
		router.GET(path, TokenAuth(), func(c *gin.Context) {
			assert.Nil(t, service.ActiveCustomerContract(c), "history must not load the current contract")
			c.Status(http.StatusNoContent)
		})
	}
	router.POST("/v1/batches/:id/cancel", TokenAuth(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	for _, tc := range []struct {
		enabled bool
		status  int
	}{{true, 204}, {false, 204}} {
		require.NoError(t, db.Model(&model.CustomerContract{}).Where("id = ?", contract.Id).Update("enabled", tc.enabled).Error)
		service.ResetContractEntityCacheForTest()
		request := httptest.NewRequest("POST", "/v1/batches", nil)
		request.Header.Set("Authorization", "Bearer sk-"+token.Key)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		assert.Equal(t, tc.status, recorder.Code, recorder.Body.String())

		staleRequest := httptest.NewRequest("POST", "/v1/batches", nil)
		staleRequest.Header.Set("Authorization", "Bearer sk-"+stale.Key)
		staleRecorder := httptest.NewRecorder()
		router.ServeHTTP(staleRecorder, staleRequest)
		expected := 403
		if tc.enabled {
			expected = 204
		}
		assert.Equal(t, expected, staleRecorder.Code, staleRecorder.Body.String())
		internalRequest := httptest.NewRequest("POST", "/v1/batches", nil)
		internalRequest.Header.Set("Authorization", "Bearer sk-"+internal.Key)
		internalRecorder := httptest.NewRecorder()
		router.ServeHTTP(internalRecorder, internalRequest)
		assert.Equal(t, expected, internalRecorder.Code, "disabled contracts restore native group permission checks")
		for _, path := range []string{"/v1/batches/owned", "/v1/files/owned/content", "/v1/videos/owned", "/v1/tasks/owned", "/v1/batches/owned/cancel"} {
			method := http.MethodGet
			if strings.HasSuffix(path, "/cancel") {
				method = http.MethodPost
			}
			request := httptest.NewRequest(method, path, nil)
			request.Header.Set("Authorization", "Bearer sk-"+stale.Key)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			assert.Equal(t, http.StatusNoContent, recorder.Code, "%s enabled=%t", path, tc.enabled)
		}

	}
}
