package middleware

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
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
	user := model.User{Username: "contract-middleware-user", AffCode: "contract-middleware-aff", Group: "default", AuthVersion: 2}
	require.NoError(t, db.Create(&user).Error)
	priority := int64(0)
	channel := model.Channel{Name: "contract-route-channel", Group: "contract-route", Models: "Model-A", Key: "test-key", Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "contract-route", Model: "Model-A", ChannelId: channel.Id, Enabled: true, Priority: &priority}).Error)

	// The user's key binds an enabled contract entity.
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
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
	common.SetContextKey(c, constant.ContextKeyTokenGroup, "default")
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

func TestCustomerContractGuardResolvesDiscountWithoutRouteCoupling(t *testing.T) {
	_, user, contract := setupCustomerContractMiddlewareDB(t)
	c := customerContractGinContext(user, contract.Id)

	fact, err := applyCustomerContractRequest(c, "Model-A")
	require.NoError(t, err)
	require.NotNil(t, fact)
	assert.Equal(t, contract.Id, fact.ContractId)
	assert.EqualValues(t, 80_000_000, fact.RatioUnits)
	assert.Equal(t, "default", common.GetContextKeyString(c, constant.ContextKeyUsingGroup), "UsingGroup stays native")
	assert.Equal(t, "default", common.GetContextKeyString(c, constant.ContextKeyTokenGroup), "TokenGroup stays native")
	assert.True(t, common.GetContextKeyBool(c, constant.ContextKeyTokenCrossGroupRetry), "native cross-group retry is untouched")
	stored, ok := common.GetContextKeyType[*hosttypes.ContractBillingFact](c, constant.ContextKeyContractFact)
	assert.True(t, ok)
	assert.Equal(t, fact, stored)
	if _, found, _ := service.GetChannelConstraints(c).ResolvedPin(); found {
		t.Fatal("the contract never pins a channel")
	}

	// A model the contract does not list falls back to native handling.
	unlisted := customerContractGinContext(user, contract.Id)
	unlistedFact, unlistedErr := applyCustomerContractRequest(unlisted, "Other-Model")
	require.NoError(t, unlistedErr)
	assert.Nil(t, unlistedFact, "an unlisted model keeps native pricing")
	_, exists := common.GetContextKey(unlisted, constant.ContextKeyContractFact)
	assert.False(t, exists) // Case-insensitive model names are rejected at save time, so a lookup
	// uses the exact public model only.
	caseFact, caseErr := applyCustomerContractRequest(customerContractGinContext(user, contract.Id), "model-a")
	require.NoError(t, caseErr)
	assert.Nil(t, caseFact, "matching uses the exact public model name")
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

func TestDistributeKeepsNativeTokenModelLimitAuthorityForContractKeys(t *testing.T) {
	_, user, contract := setupCustomerContractMiddlewareDB(t)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"Model-A"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(c, constant.ContextKeyUserId, user.Id)
	common.SetContextKey(c, constant.ContextKeyAuthVersion, user.AuthVersion)
	common.SetContextKey(c, constant.ContextKeyTokenContractId, contract.Id)
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
	common.SetContextKey(c, constant.ContextKeyTokenGroup, "default")
	common.SetContextKey(c, constant.ContextKeyTokenModelLimitEnabled, true)
	common.SetContextKey(c, constant.ContextKeyTokenModelLimit, map[string]bool{})

	Distribute()(c)

	assert.True(t, c.IsAborted())
	assert.Equal(t, 403, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "no access to model Model-A")
}

func TestDistributeIgnoresContractChannelListWhenSelectingChannel(t *testing.T) {
	for _, cached := range []bool{false, true} {
		t.Run(fmt.Sprintf("cache=%t", cached), func(t *testing.T) {
			db, user, contract := setupCustomerContractMiddlewareDB(t)
			common.MemoryCacheEnabled = cached
			priority := int64(10)
			other := model.Channel{Name: "native-priority-winner", Group: "default", Models: "Model-A", Key: "test-key", Status: common.ChannelStatusEnabled, Priority: &priority}
			require.NoError(t, db.Create(&other).Error)
			require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "Model-A", ChannelId: other.Id, Enabled: true, Priority: &priority}).Error)
			require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", contract.Rules[0].ChannelId).Update("group", "default").Error)
			require.NoError(t, db.Model(&model.Ability{}).Where("channel_id = ?", contract.Rules[0].ChannelId).Update("group", "default").Error)
			model.InitChannelCache()
			for _, bound := range []int{0, contract.Id} {
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Request = httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"Model-A"}`))
				c.Request.Header.Set("Content-Type", "application/json")
				common.SetContextKey(c, constant.ContextKeyUserId, user.Id)
				common.SetContextKey(c, constant.ContextKeyAuthVersion, user.AuthVersion)
				common.SetContextKey(c, constant.ContextKeyTokenContractId, bound)
				common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
				common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
				common.SetContextKey(c, constant.ContextKeyTokenGroup, "default")
				common.SetContextKey(c, constant.ContextKeyRequestStartTime, time.Now())
				Distribute()(c)
				require.False(t, c.IsAborted(), recorder.Body.String())
				require.NotNil(t, channelOf(c))
				assert.Equal(t, other.Id, channelOf(c).Id, "binding must preserve the native priority winner")
				assert.Equal(t, "default", common.GetContextKeyString(c, constant.ContextKeyUsingGroup))
				stored, ok := common.GetContextKeyType[*hosttypes.ContractBillingFact](c, constant.ContextKeyContractFact)
				if bound != 0 {
					require.True(t, ok)
					assert.EqualValues(t, 80_000_000, stored.RatioUnits)
				} else {
					assert.False(t, ok)
				}
			}
		})
	}
}

func channelOf(c *gin.Context) *model.Channel {
	id, ok := common.GetContextKeyType[int](c, constant.ContextKeyChannelId)
	if !ok || id == 0 {
		return nil
	}
	ch, err := model.CacheGetChannel(id)
	if err != nil {
		return nil
	}
	return ch
}
