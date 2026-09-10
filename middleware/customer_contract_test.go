package middleware

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupCustomerContractMiddlewareDB(t *testing.T) (*gorm.DB, model.User, model.ContractEntitySnapshot) {
	t.Helper()
	require.NoError(t, i18n.Init())
	previousDB := model.DB
	previousType := common.MainDatabaseType()
	previousRedis := common.RedisEnabled
	previousMemoryCache := common.MemoryCacheEnabled
	previousRatios := ratio_setting.GroupRatio2JSONString()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.User{}, &model.Channel{}, &model.Ability{}, &model.Token{},
		&model.CustomerModelContract{},
		&model.CustomerContract{}, &model.CustomerContractEntityRule{}, &model.CustomerContractEntityAudit{},
	))
	model.DB = db
	common.RedisEnabled = false
	common.MemoryCacheEnabled = true
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"contract-route":0.87}`))
	user := model.User{
		Username: "contract-middleware-user", AffCode: "contract-middleware-aff", Group: "default",
		AuthVersion: 2,
	}
	require.NoError(t, db.Create(&user).Error)
	priority := int64(0)
	channel := model.Channel{Name: "contract-route-channel", Group: "contract-route", Models: "Model-A", Key: "test-key", Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group: "contract-route", Model: "Model-A", ChannelId: channel.Id, Enabled: true, Priority: &priority,
	}).Error)

	// The user's key binds an enabled contract entity that pins the channel.
	snapshot, err := model.CreateCustomerContractEntity(model.CreateCustomerContractParams{
		UserId: user.Id, AdminUserId: user.Id, Name: "Entity Contract", Enabled: true,
		Reason: "middleware fixture",
		Rules: []model.CustomerContractEntityRuleInput{{
			PublicModel: "Model-A", ChannelId: channel.Id, RouteGroup: "contract-route", RatioUnits: 80_000_000,
		}},
	})
	require.NoError(t, err)

	model.InitChannelCache()
	service.ResetContractEntityCacheForTest()

	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		service.ResetContractEntityCacheForTest()
		model.DB = previousDB
		common.RedisEnabled = previousRedis
		common.MemoryCacheEnabled = previousMemoryCache
		common.SetMainDatabaseType(previousType)
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previousRatios))
		_ = sqlDB.Close()
		if previousMemoryCache && previousDB != nil {
			model.InitChannelCache()
		}
	})
	return db, user, *snapshot
}

func customerContractGinContext(user model.User, contractId int) *gin.Context {
	c, _ := gin.CreateTestContext(nil)
	common.SetContextKey(c, constant.ContextKeyUserId, user.Id)
	common.SetContextKey(c, constant.ContextKeyAuthVersion, user.AuthVersion)
	common.SetContextKey(c, constant.ContextKeyTokenContractId, contractId)
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "stale-token-group")
	common.SetContextKey(c, constant.ContextKeyTokenGroup, "auto")
	common.SetContextKey(c, constant.ContextKeyTokenCrossGroupRetry, true)
	return c
}

func TestCustomerContractGuardLeavesUnboundKeyUntouched(t *testing.T) {
	c, _ := gin.CreateTestContext(nil)
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "native-group")

	fact, err := applyCustomerContractRequest(c, "any-model")
	require.NoError(t, err)
	assert.Nil(t, fact, "a key without a contract binding never enters the contract path")
	assert.Equal(t, "native-group", common.GetContextKeyString(c, constant.ContextKeyUsingGroup))
	_, exists := common.GetContextKey(c, constant.ContextKeyContractFact)
	assert.False(t, exists)
}

func TestCustomerContractGuardUsesExactModelLocksRouteAndPinsChannel(t *testing.T) {
	_, user, contract := setupCustomerContractMiddlewareDB(t)
	c := customerContractGinContext(user, contract.Id)

	fact, err := applyCustomerContractRequest(c, "Model-A")
	require.NoError(t, err)
	require.NotNil(t, fact)
	assert.Equal(t, contract.Id, fact.ContractId)
	assert.Equal(t, contract.Rules[0].ChannelId, fact.ChannelId)
	assert.Equal(t, "contract-route", fact.RouteGroup)
	assert.EqualValues(t, 80_000_000, fact.RatioUnits)
	assert.Equal(t, "contract-route", common.GetContextKeyString(c, constant.ContextKeyUsingGroup))
	assert.Equal(t, "contract-route", common.GetContextKeyString(c, constant.ContextKeyTokenGroup))
	assert.False(t, common.GetContextKeyBool(c, constant.ContextKeyTokenCrossGroupRetry))
	stored, ok := common.GetContextKeyType[*hosttypes.ContractBillingFact](c, constant.ContextKeyContractFact)
	assert.True(t, ok)
	assert.Equal(t, fact, stored)
	pin, found, _ := service.GetChannelConstraints(c).ResolvedPin()
	assert.True(t, found, "the contract pins its explicit channel")
	assert.Equal(t, contract.Rules[0].ChannelId, pin.ChannelId)
	assert.Equal(t, dto.PinSourceContract, pin.Source)

	_, err = applyCustomerContractRequest(customerContractGinContext(user, contract.Id), "model-a")
	require.ErrorIs(t, err, service.ErrCustomerContractModelDenied)
	_, err = applyCustomerContractRequest(customerContractGinContext(user, contract.Id), " Model-A ")
	require.ErrorIs(t, err, service.ErrCustomerContractModelDenied)
}

func TestCustomerContractGuardFallsBackToNativeWhenContractDisabledAndFailsClosedWhenMissing(t *testing.T) {
	db, user, contract := setupCustomerContractMiddlewareDB(t)

	disabled := false
	_, err := model.ReplaceCustomerContractEntity(model.ReplaceCustomerContractEntityParams{
		ContractId: contract.Id, AdminUserId: user.Id, ExpectedVersion: contract.Version, Name: contract.Name,
		Enabled: &disabled, Reason: "disable for fallback test",
		Rules: []model.CustomerContractEntityRuleInput{{
			PublicModel: "Model-A", ChannelId: contract.Rules[0].ChannelId,
			RouteGroup: "contract-route", RatioUnits: 80_000_000,
		}},
	})
	require.NoError(t, err)
	service.ResetContractEntityCacheForTest()
	fact, err := applyCustomerContractRequest(customerContractGinContext(user, contract.Id), "Model-A")
	require.NoError(t, err)
	assert.Nil(t, fact, "a disabled contract restores the key's native logic")

	require.NoError(t, db.Where("id = ?", contract.Id).Delete(&model.CustomerContract{}).Error)
	service.ResetContractEntityCacheForTest()
	_, err = applyCustomerContractRequest(customerContractGinContext(user, contract.Id), "Model-A")
	require.ErrorIs(t, err, service.ErrCustomerContractUnavailable,
		"an abnormal binding is a hard failure, never a native fallback")
}

func TestSpecificChannelMustBelongToContractGroupModelAndPinnedChannel(t *testing.T) {
	db, _, contract := setupCustomerContractMiddlewareDB(t)
	priority := int64(0)
	var allowed model.Channel
	require.NoError(t, db.Where("name = ?", "contract-route-channel").First(&allowed).Error)
	outside := model.Channel{Name: "outside", Group: "contract-route", Models: "Model-A", Key: "key", Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(&outside).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "contract-route", Model: "Model-A", ChannelId: outside.Id, Enabled: true, Priority: &priority}).Error)
	model.InitChannelCache()

	fact := &hosttypes.ContractBillingFact{
		PublicModel: "Model-A", RouteGroup: "contract-route", RatioUnits: 80_000_000,
		ChannelId: contract.Rules[0].ChannelId,
	}
	assert.Equal(t, allowed.Id, fact.ChannelId)
	assert.True(t, channelSatisfiesCustomerContract(&allowed, fact), "the contract's own channel satisfies the fact")
	assert.False(t, channelSatisfiesCustomerContract(&outside, fact), "any other channel fails the pinned contract")
	allowed.Status = common.ChannelStatusManuallyDisabled
	assert.False(t, channelSatisfiesCustomerContract(&allowed, fact))
}

func TestResolvedTaskModelUsesContractTokenIntersectionAndLockedChannel(t *testing.T) {
	db, user, contract := setupCustomerContractMiddlewareDB(t)
	var allowed model.Channel
	require.NoError(t, db.Where("name = ?", "contract-route-channel").First(&allowed).Error)
	c := customerContractGinContext(user, contract.Id)
	common.SetContextKey(c, constant.ContextKeyTokenModelLimitEnabled, true)
	common.SetContextKey(c, constant.ContextKeyTokenModelLimit, map[string]bool{"Model-A": true})

	fact, err := ApplyCustomerContractResolvedModel(c, "Model-A", &allowed)
	require.NoError(t, err)
	require.NotNil(t, fact)
	assert.Equal(t, "contract-route", fact.RouteGroup)

	outside := model.Channel{Name: "resolved-outside", Group: "default", Models: "Model-A", Key: "key", Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(&outside).Error)
	_, err = ApplyCustomerContractResolvedModel(customerContractGinContext(user, contract.Id), "Model-A", &outside)
	require.Error(t, err)

	denied := customerContractGinContext(user, contract.Id)
	common.SetContextKey(denied, constant.ContextKeyTokenModelLimitEnabled, true)
	common.SetContextKey(denied, constant.ContextKeyTokenModelLimit, map[string]bool{})
	_, err = ApplyCustomerContractResolvedModel(denied, "Model-A", &allowed)
	require.Error(t, err)
}

func TestSpecificChannelStillEnforcesContractTokenModelIntersection(t *testing.T) {
	_, user, contract := setupCustomerContractMiddlewareDB(t)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"Model-A"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(c, constant.ContextKeyUserId, user.Id)
	common.SetContextKey(c, constant.ContextKeyAuthVersion, user.AuthVersion)
	common.SetContextKey(c, constant.ContextKeyTokenContractId, contract.Id)
	service.GetChannelConstraints(c).AddPin(dto.ChannelPin{ChannelId: 1, Source: dto.PinSourceToken, Rank: dto.PinRankToken, RetryMode: dto.PinRetrySingleAttempt})
	common.SetContextKey(c, constant.ContextKeyTokenModelLimitEnabled, true)
	common.SetContextKey(c, constant.ContextKeyTokenModelLimit, map[string]bool{})

	Distribute()(c)

	assert.True(t, c.IsAborted())
	assert.Equal(t, 403, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "no access to model Model-A")
	assert.NotContains(t, recorder.Body.String(), "contract-route")
}

func TestCustomerContractChannelFailureDoesNotExposeRouteGroup(t *testing.T) {
	db, user, contract := setupCustomerContractMiddlewareDB(t)
	require.NoError(t, db.Model(&model.Channel{}).
		Where("name = ?", "contract-route-channel").
		Update("status", common.ChannelStatusManuallyDisabled).Error)
	model.InitChannelCache()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"Model-A"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(c, constant.ContextKeyUserId, user.Id)
	common.SetContextKey(c, constant.ContextKeyAuthVersion, user.AuthVersion)
	common.SetContextKey(c, constant.ContextKeyTokenContractId, contract.Id)

	Distribute()(c)

	assert.True(t, c.IsAborted())
	// The pinned contract channel is disabled, so the pinned distribution path
	// fails closed with an explicit error and never leaks the contract group.
	assert.Equal(t, 403, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "channel has been disabled")
	assert.NotContains(t, recorder.Body.String(), "contract-route")
	assert.NotContains(t, recorder.Body.String(), "No available channel")
}

func TestContractTokenGroupExemptionRequiresOwnedEnabledBinding(t *testing.T) {
	db, user, contract := setupCustomerContractMiddlewareDB(t)
	token := &model.Token{UserId: user.Id, ContractId: contract.Id, Group: "removed-native-group"}
	active, err := activeTokenContract(token, user.AuthVersion)
	require.NoError(t, err)
	assert.True(t, active)
	token.UserId = user.Id + 1000
	_, err = activeTokenContract(token, user.AuthVersion)
	require.Error(t, err)
	token.UserId = user.Id
	require.NoError(t, db.Model(&model.CustomerContract{}).Where("id = ?", contract.Id).Update("enabled", false).Error)
	service.ResetContractEntityCacheForTest()
	active, err = activeTokenContract(token, user.AuthVersion)
	require.NoError(t, err)
	assert.False(t, active)
	token.ContractId = 0
	active, err = activeTokenContract(token, user.AuthVersion)
	require.NoError(t, err)
	assert.False(t, active)
}
