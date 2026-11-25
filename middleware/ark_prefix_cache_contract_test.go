package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfiguredArkPilotUsesContractAdmission(t *testing.T) {
	db, user, _ := setupCustomerContractMiddlewareDB(t)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.SystemTask{}, &model.ArkPrefixCache{}))
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
		t.Cleanup(func() {
			common.OptionMapRWMutex.Lock()
			common.OptionMap = nil
			common.OptionMapRWMutex.Unlock()
		})
	}
	common.OptionMapRWMutex.Unlock()
	previous := strconv.FormatBool(model.APIKeyContractRequired())
	t.Cleanup(func() {
		require.NoError(t, model.UpdateRequestPolicyOptions(map[string]string{model.RequireAPIKeyContractOption: previous}))
	})
	token := model.Token{UserId: user.Id, Key: "arkcontractpilot", Group: "default", Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 10}
	require.NoError(t, db.Create(&token).Error)
	const replayKey = "ark-contract-replay"
	require.NoError(t, db.Create(&model.ArkPrefixCache{
		ID: model.ArkPrefixCacheIDPrefix + "fixture", UserID: user.Id, TokenID: token.Id,
		Model: "seed-lite", KeyDigest: model.ArkPrefixCacheKeyDigest(replayKey), State: model.ArkCacheSending,
	}).Error)
	pilot := &service.ArkPrefixCachePilot{UserID: user.Id, TokenID: token.Id, Model: "seed-lite", Mode: "observe"}
	engine := gin.New()
	auth := ModelTokenAuthWith(arkPrefixCacheTokenAuth(func() *service.ArkPrefixCachePilot { return pilot }))
	engine.POST("/v1/responses", BodyStorageCleanup(), auth, func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	for _, tt := range []struct {
		name, body, idempotencyKey string
		quota                      int
	}{
		{"create", `{"model":"seed-lite","caching":{"type":"enabled","prefix":true}}`, "new-cache", 10},
		{"continue", `{"model":"seed-lite","previous_response_id":"resp_ark_prefix_fixture","caching":{"type":"enabled"}}`, "", 10},
		{"exhausted replay", `{"model":"seed-lite","caching":{"type":"enabled","prefix":true}}`, replayKey, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, db.Model(&token).Update("remain_quota", tt.quota).Error)
			for _, setting := range []struct {
				value  string
				status int
			}{{"true", http.StatusForbidden}, {"false", http.StatusNoContent}} {
				require.NoError(t, model.UpdateRequestPolicyOptions(map[string]string{model.RequireAPIKeyContractOption: setting.value}))
				request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(tt.body))
				request.Header.Set("Authorization", "Bearer "+token.Key)
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set("Idempotency-Key", tt.idempotencyKey)
				response := httptest.NewRecorder()
				engine.ServeHTTP(response, request)
				assert.Equal(t, setting.status, response.Code, response.Body.String())
				if setting.value == "true" {
					assert.Contains(t, response.Body.String(), constant.ErrorCodeContractRequired)
				}
			}
		})
	}
}
