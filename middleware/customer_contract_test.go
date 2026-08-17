package middleware

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

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

func setupCustomerContractMiddlewareDB(t *testing.T) (*gorm.DB, model.User) {
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
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Channel{}, &model.Ability{}, &model.CustomerModelContract{}))
	model.DB = db
	common.RedisEnabled = false
	common.MemoryCacheEnabled = true
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"contract-route":0.87}`))
	user := model.User{
		Username: "contract-middleware-user", AffCode: "contract-middleware-aff", Group: "default",
		AuthVersion: 2, ContractMode: true, ContractVersion: 7,
	}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&model.CustomerModelContract{
		UserId: user.Id, PublicModel: "Model-A", RouteGroup: "contract-route", RatioUnits: 80_000_000,
	}).Error)
	priority := int64(0)
	channel := model.Channel{Name: "contract-route-channel", Group: "contract-route", Models: "Model-A", Key: "test-key", Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group: "contract-route", Model: "Model-A", ChannelId: channel.Id, Enabled: true, Priority: &priority,
	}).Error)
	model.InitChannelCache()
	service.ResetCustomerContractCacheForTest()

	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		service.ResetCustomerContractCacheForTest()
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
	return db, user
}

func customerContractGinContext(user model.User) *gin.Context {
	c, _ := gin.CreateTestContext(nil)
	common.SetContextKey(c, constant.ContextKeyUserId, user.Id)
	common.SetContextKey(c, constant.ContextKeyContractMode, user.ContractMode)
	common.SetContextKey(c, constant.ContextKeyContractVersion, user.ContractVersion)
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "stale-token-group")
	common.SetContextKey(c, constant.ContextKeyTokenGroup, "auto")
	common.SetContextKey(c, constant.ContextKeyTokenCrossGroupRetry, true)
	return c
}

func TestCustomerContractGuardLeavesNativeRequestUntouched(t *testing.T) {
	c, _ := gin.CreateTestContext(nil)
	common.SetContextKey(c, constant.ContextKeyContractMode, false)
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "native-group")

	fact, err := applyCustomerContractRequest(c, "any-model")
	require.NoError(t, err)
	assert.Nil(t, fact)
	assert.Equal(t, "native-group", common.GetContextKeyString(c, constant.ContextKeyUsingGroup))
	_, exists := common.GetContextKey(c, constant.ContextKeyContractFact)
	assert.False(t, exists)
}

func TestCustomerContractGuardUsesExactModelAndLocksRoute(t *testing.T) {
	_, user := setupCustomerContractMiddlewareDB(t)
	c := customerContractGinContext(user)

	fact, err := applyCustomerContractRequest(c, "Model-A")
	require.NoError(t, err)
	require.NotNil(t, fact)
	assert.Equal(t, "contract-route", fact.RouteGroup)
	assert.EqualValues(t, 80_000_000, fact.RatioUnits)
	assert.Equal(t, "contract-route", common.GetContextKeyString(c, constant.ContextKeyUsingGroup))
	assert.Equal(t, "contract-route", common.GetContextKeyString(c, constant.ContextKeyTokenGroup))
	assert.False(t, common.GetContextKeyBool(c, constant.ContextKeyTokenCrossGroupRetry))
	stored, ok := common.GetContextKeyType[*hosttypes.ContractBillingFact](c, constant.ContextKeyContractFact)
	assert.True(t, ok)
	assert.Equal(t, fact, stored)

	_, err = applyCustomerContractRequest(customerContractGinContext(user), "model-a")
	require.ErrorIs(t, err, service.ErrCustomerContractModelDenied)
	_, err = applyCustomerContractRequest(customerContractGinContext(user), " Model-A ")
	require.ErrorIs(t, err, service.ErrCustomerContractModelDenied)
}

func TestCustomerContractGuardFailsClosedOnVersionMismatchAndEmptyContract(t *testing.T) {
	db, user := setupCustomerContractMiddlewareDB(t)
	stale := customerContractGinContext(user)
	common.SetContextKey(stale, constant.ContextKeyContractVersion, int64(6))
	_, err := applyCustomerContractRequest(stale, "Model-A")
	require.ErrorIs(t, err, service.ErrCustomerContractUnavailable)

	require.NoError(t, db.Where("user_id = ?", user.Id).Delete(&model.CustomerModelContract{}).Error)
	service.ResetCustomerContractCacheForTest()
	_, err = applyCustomerContractRequest(customerContractGinContext(user), "Model-A")
	require.ErrorIs(t, err, service.ErrCustomerContractModelDenied)
}

func TestSpecificChannelMustBelongToContractGroupAndModel(t *testing.T) {
	db, _ := setupCustomerContractMiddlewareDB(t)
	priority := int64(0)
	allowed := model.Channel{Name: "allowed", Group: "contract-route", Models: "Model-A", Key: "key", Status: common.ChannelStatusEnabled}
	outside := model.Channel{Name: "outside", Group: "default", Models: "Model-A", Key: "key", Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(&allowed).Error)
	require.NoError(t, db.Create(&outside).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "contract-route", Model: "Model-A", ChannelId: allowed.Id, Enabled: true, Priority: &priority}).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "Model-A", ChannelId: outside.Id, Enabled: true, Priority: &priority}).Error)
	model.InitChannelCache()
	fact := &hosttypes.ContractBillingFact{PublicModel: "Model-A", RouteGroup: "contract-route", RatioUnits: 80_000_000}

	assert.True(t, channelSatisfiesCustomerContract(&allowed, fact))
	assert.False(t, channelSatisfiesCustomerContract(&outside, fact))
	outModel := *fact
	outModel.PublicModel = "Model-B"
	assert.False(t, channelSatisfiesCustomerContract(&allowed, &outModel))
	allowed.Status = common.ChannelStatusManuallyDisabled
	assert.False(t, channelSatisfiesCustomerContract(&allowed, fact))
}

func TestResolvedTaskModelUsesContractTokenIntersectionAndLockedChannel(t *testing.T) {
	db, user := setupCustomerContractMiddlewareDB(t)
	var allowed model.Channel
	require.NoError(t, db.Where("name = ?", "contract-route-channel").First(&allowed).Error)
	c := customerContractGinContext(user)
	common.SetContextKey(c, constant.ContextKeyTokenModelLimitEnabled, true)
	common.SetContextKey(c, constant.ContextKeyTokenModelLimit, map[string]bool{"Model-A": true})

	fact, err := ApplyCustomerContractResolvedModel(c, "Model-A", &allowed)
	require.NoError(t, err)
	require.NotNil(t, fact)
	assert.Equal(t, "contract-route", fact.RouteGroup)

	outside := model.Channel{Name: "resolved-outside", Group: "default", Models: "Model-A", Key: "key", Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(&outside).Error)
	_, err = ApplyCustomerContractResolvedModel(customerContractGinContext(user), "Model-A", &outside)
	require.Error(t, err)

	denied := customerContractGinContext(user)
	common.SetContextKey(denied, constant.ContextKeyTokenModelLimitEnabled, true)
	common.SetContextKey(denied, constant.ContextKeyTokenModelLimit, map[string]bool{})
	_, err = ApplyCustomerContractResolvedModel(denied, "Model-A", &allowed)
	require.Error(t, err)
}

func TestSpecificChannelStillEnforcesContractTokenModelIntersection(t *testing.T) {
	_, user := setupCustomerContractMiddlewareDB(t)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"Model-A"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(c, constant.ContextKeyUserId, user.Id)
	common.SetContextKey(c, constant.ContextKeyContractMode, true)
	common.SetContextKey(c, constant.ContextKeyContractVersion, user.ContractVersion)
	common.SetContextKey(c, constant.ContextKeyTokenSpecificChannelId, "1")
	common.SetContextKey(c, constant.ContextKeyTokenModelLimitEnabled, true)
	common.SetContextKey(c, constant.ContextKeyTokenModelLimit, map[string]bool{})

	Distribute()(c)

	assert.True(t, c.IsAborted())
	assert.Equal(t, 403, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "no access to model Model-A")
	assert.NotContains(t, recorder.Body.String(), "contract-route")
}

func TestCustomerContractChannelFailureDoesNotExposeRouteGroup(t *testing.T) {
	db, user := setupCustomerContractMiddlewareDB(t)
	require.NoError(t, db.Model(&model.Channel{}).
		Where("name = ?", "contract-route-channel").
		Update("status", common.ChannelStatusManuallyDisabled).Error)
	model.InitChannelCache()
	_, err := service.ResolveCustomerContractRule(user.Id, user.ContractVersion, "Model-A")
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"Model-A"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(c, constant.ContextKeyUserId, user.Id)
	common.SetContextKey(c, constant.ContextKeyContractMode, true)
	common.SetContextKey(c, constant.ContextKeyContractVersion, user.ContractVersion)

	Distribute()(c)

	assert.True(t, c.IsAborted())
	assert.Equal(t, 503, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "model_not_found")
	assert.NotContains(t, recorder.Body.String(), "contract-route")
	assert.NotContains(t, recorder.Body.String(), "No available channel")
}
