package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomerContractScopeCrossGroupPriorityAndRetry(t *testing.T) {
	for _, cached := range []bool{false, true} {
		for _, cross := range []bool{false, true} {
			t.Run(fmt.Sprintf("cache=%t/cross=%t", cached, cross), func(t *testing.T) {
				db, user, contract := setupCustomerContractMiddlewareDB(t)
				common.MemoryCacheEnabled = cached
				priority := int64(10)
				winner := model.Channel{Name: "allowed-high", Group: "default", Models: "Model-A", Status: common.ChannelStatusEnabled, Key: "test-key", Priority: &priority}
				require.NoError(t, db.Create(&winner).Error)
				require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "Model-A", ChannelId: winner.Id, Enabled: true, Priority: &priority}).Error)
				require.NoError(t, db.Create(&model.CustomerContractEntityRule{ContractId: contract.Id, PublicModel: "Model-A", ChannelId: winner.Id, RouteGroup: "default", RatioUnits: 80_000_000}).Error)
				model.InitChannelCache()
				c := customerContractGinContext(user, contract.Id)
				common.SetContextKey(c, constant.ContextKeyUserGroup, user.Group)
				common.SetContextKey(c, constant.ContextKeyTokenCrossGroupRetry, cross)
				_, err := applyCustomerContractRequest(c, "Model-A")
				require.NoError(t, err)
				param := &service.RetryParam{Ctx: c, ModelName: "Model-A", TokenGroup: "removed-key-group"}
				channel, group, err := service.SelectCustomerContractChannel(param, true)
				require.NoError(t, err)
				assert.Equal(t, winner.Id, channel.Id)
				assert.Equal(t, "default", group)
				param.SetRetry(1)
				channel, group, err = service.CacheGetRandomSatisfiedChannel(param)
				require.NoError(t, err)
				if cross {
					assert.Equal(t, contract.Rules[0].ChannelId, channel.Id)
					assert.Equal(t, "contract-route", group)
				} else {
					assert.Equal(t, winner.Id, channel.Id)
					assert.Equal(t, "default", group)
				}
				// A new origin-task operation locks its first actual channel,
				// even if distribution provisionally selected another group.
				fact, err := service.ResolveCustomerContractLockedChannel(c, "Model-A", contract.Rules[0].ChannelId)
				require.NoError(t, err)
				assert.EqualValues(t, 80_000_000, fact.RatioUnits)
				assert.Equal(t, "contract-route", common.GetContextKeyString(c, constant.ContextKeyUsingGroup))
				// No active candidate may escape to another native channel/group.
				require.NoError(t, db.Model(&winner).Update("status", common.ChannelStatusManuallyDisabled).Error)
				require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", contract.Rules[0].ChannelId).Update("status", common.ChannelStatusManuallyDisabled).Error)
				channel, _, err = service.CacheGetRandomSatisfiedChannel(param)
				require.ErrorIs(t, err, service.ErrCustomerContractScope)
				assert.Nil(t, channel)
			})
		}
	}
}

func TestCustomerContractScopePermissionsPinsAndFrozenRequest(t *testing.T) {
	db, user, contract := setupCustomerContractMiddlewareDB(t)
	c := customerContractGinContext(user, contract.Id)
	common.SetContextKey(c, constant.ContextKeyUserGroup, user.Group)
	fact, err := applyCustomerContractRequest(c, "Model-A")
	require.NoError(t, err)
	service.GetChannelConstraints(c).AddPin(dto.ChannelPin{ChannelId: 999, Source: dto.PinSourceToken, Rank: dto.PinRankToken})
	_, _, err = service.SelectCustomerContractChannel(&service.RetryParam{Ctx: c, ModelName: "Model-A"}, true)
	require.ErrorIs(t, err, service.ErrCustomerContractScope)
	service.GetChannelConstraints(c).Pins = nil
	common.SetContextKey(c, constant.ContextKeyChannelId, contract.Rules[0].ChannelId)
	resolved, err := ApplyCustomerContractResolvedModel(c, "Model-A")
	require.NoError(t, err)
	assert.Equal(t, fact, resolved)
	assert.Equal(t, "contract-route", common.GetContextKeyString(c, constant.ContextKeyUsingGroup))
	common.SetContextKey(c, constant.ContextKeyChannelId, 999)
	_, err = ApplyCustomerContractResolvedModel(c, "Model-A")
	require.ErrorIs(t, err, service.ErrCustomerContractScope)

	require.NoError(t, db.Model(&model.CustomerContract{}).Where("id = ?", contract.Id).Update("enabled", false).Error)
	service.ResetContractEntityCacheForTest()
	common.SetContextKey(c, constant.ContextKeyTokenContractId, 0)
	_, _, err = service.SelectCustomerContractChannel(&service.RetryParam{Ctx: c, ModelName: "Model-A"}, false)
	require.NoError(t, err, "in-flight request retains the accepted contract snapshot")
	next := customerContractGinContext(user, contract.Id)
	nextFact, err := applyCustomerContractRequest(next, "not-listed")
	require.NoError(t, err)
	assert.Nil(t, nextFact, "a new request sees disabled state and follows native handling")

	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default"}`))
	_, _, err = service.SelectCustomerContractChannel(&service.RetryParam{Ctx: c, ModelName: "Model-A"}, false)
	require.NoError(t, err, "user group visibility cannot revoke an accepted contract route")
}

func TestCustomerContractScopeAssetAndHistory(t *testing.T) {
	db, user, contract := setupCustomerContractMiddlewareDB(t)
	c := customerContractGinContext(user, contract.Id)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/old-task", nil)
	require.NoError(t, db.Delete(&model.CustomerContract{}, contract.Id).Error)
	active, err := customerContractAuthGate(c, &model.Token{UserId: user.Id, ContractId: contract.Id})
	require.NoError(t, err, "historical task reads do not reload the current contract")
	assert.True(t, active, "history bypasses current route-group checks")
	c, _ = gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(c, constant.ContextKeyAuthVersion, user.AuthVersion)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos/old-task/remix", nil)
	_, err = customerContractAuthGate(c, &model.Token{UserId: user.Id, ContractId: contract.Id})
	require.Error(t, err, "new remix must authorize current binding")
	for _, path := range []string{"/v1/realtime", "/v1/models", "/api/pricing", "/v1/assets/opaque"} {
		next, _ := gin.CreateTestContext(httptest.NewRecorder())
		common.SetContextKey(next, constant.ContextKeyAuthVersion, user.AuthVersion)
		next.Request = httptest.NewRequest(http.MethodGet, path, nil)
		_, err := customerContractAuthGate(next, &model.Token{UserId: user.Id, ContractId: contract.Id})
		require.Error(t, err, "%s must authorize the current binding", path)
	}

}

func TestCustomerContractAssetAccessUsesTypedScopeWithoutAbility(t *testing.T) {
	db, user, contract := setupCustomerContractMiddlewareDB(t)
	channelID := contract.Rules[0].ChannelId
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", channelID).Update("type", constant.ChannelTypeSeedanceLink).Error)
	require.NoError(t, db.Where("channel_id = ?", channelID).Delete(&model.Ability{}).Error)
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyUserId, user.Id)
		common.SetContextKey(c, constant.ContextKeyUserGroup, user.Group)
		common.SetContextKey(c, constant.ContextKeyTokenContractId, contract.Id)
		common.SetContextKey(c, constant.ContextKeyAuthVersion, user.AuthVersion)
		_, err := service.CustomerContractForRequest(c, user.Id, user.AuthVersion, contract.Id)
		require.NoError(t, err)
	}, CustomerContractAssetAccess())
	engine.GET("/v1/assets/:id", func(c *gin.Context) {
		c.String(http.StatusOK, common.GetContextKeyString(c, constant.ContextKeyUsingGroup))
	})
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/assets/opaque?model=Model-A", nil))
	assert.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "contract-route", response.Body.String())
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", channelID).Update("status", common.ChannelStatusManuallyDisabled).Error)
	require.NoError(t, db.Create(&model.Channel{Name: "replacement", Type: constant.ChannelTypeSeedanceLink, Group: "contract-route", Models: "Model-A", Status: common.ChannelStatusEnabled}).Error)
	response = httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/assets/opaque?model=Model-A", nil))
	assert.Equal(t, http.StatusForbidden, response.Code, "an available replacement outside the contract cannot authorize asset calls")
}
