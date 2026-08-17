package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupCustomerContractControllerDB(t *testing.T) (model.User, model.User) {
	t.Helper()
	previousDB := model.DB
	previousLogDB := model.LOG_DB
	previousType := common.MainDatabaseType()
	previousRedis := common.RedisEnabled
	previousMemoryCache := common.MemoryCacheEnabled
	previousRatios := ratio_setting.GroupRatio2JSONString()
	previousModelRatios := ratio_setting.ModelRatio2JSONString()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.User{}, &model.Channel{}, &model.Ability{}, &model.Model{}, &model.Vendor{},
		&model.CustomerModelContract{}, &model.CustomerContractAudit{}, &model.Log{},
	))
	model.DB = db
	model.LOG_DB = db
	common.RedisEnabled = false
	common.MemoryCacheEnabled = true
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"contract-api":0.87}`))
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"contract-model":1}`))
	model.InvalidatePricingCache()
	service.ResetCustomerContractCacheForTest()

	admin := model.User{Username: "contract-api-admin", AffCode: "contract-api-admin-aff", Role: common.RoleAdminUser, AuthVersion: 1}
	user := model.User{Username: "contract-api-user", AffCode: "contract-api-user-aff", Role: common.RoleCommonUser, Group: "default", AuthVersion: 1}
	require.NoError(t, db.Create(&admin).Error)
	require.NoError(t, db.Create(&user).Error)
	channel := model.Channel{Name: "contract-api-channel", Group: "contract-api", Models: "contract-model", Key: "key", Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(&channel).Error)
	priority := int64(0)
	require.NoError(t, db.Create(&model.Ability{
		Group: "contract-api", Model: "contract-model", ChannelId: channel.Id, Enabled: true, Priority: &priority,
	}).Error)
	model.InitChannelCache()

	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		service.ResetCustomerContractCacheForTest()
		model.InvalidatePricingCache()
		model.DB = previousDB
		model.LOG_DB = previousLogDB
		common.RedisEnabled = previousRedis
		common.MemoryCacheEnabled = previousMemoryCache
		common.SetMainDatabaseType(previousType)
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previousRatios))
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(previousModelRatios))
		_ = sqlDB.Close()
		if previousMemoryCache && previousDB != nil {
			model.InitChannelCache()
		}
	})
	return admin, user
}

func customerContractAdminContext(method string, path string, body string, admin model.User, target model.User) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", target.Id)}}
	c.Set("id", admin.Id)
	c.Set("role", admin.Role)
	c.Set("username", admin.Username)
	return c, recorder
}

func TestCustomerContractAdminAPIAtomicallyCreatesAndAutoEnablesFirstRules(t *testing.T) {
	gin.SetMode(gin.TestMode)
	admin, user := setupCustomerContractControllerDB(t)
	c, recorder := customerContractAdminContext(http.MethodPut, "/api/user/1/contract", `{
		"expected_version":0,"enabled":false,"reason":"signed contract",
		"rules":[{"model":"contract-model","route_group":"contract-api","discount":"8折"}]
	}`, admin, user)

	PutCustomerContract(c)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"contract_mode":true`)
	assert.Contains(t, recorder.Body.String(), `"discount":"0.8"`)
	assert.Contains(t, recorder.Body.String(), `"native_group_ratio":"0.87"`)
	assert.Contains(t, recorder.Body.String(), `"effective_multiplier":"0.696"`)
	snapshot, err := model.GetCustomerContractSnapshot(user.Id)
	require.NoError(t, err)
	assert.True(t, snapshot.Enabled)
	assert.EqualValues(t, 1, snapshot.Version)
	require.Len(t, snapshot.Rules, 1)
}

func TestCustomerContractAdminAPIRejectsStaleExpectedVersion(t *testing.T) {
	admin, user := setupCustomerContractControllerDB(t)
	_, err := model.ReplaceCustomerContract(model.ReplaceCustomerContractParams{
		UserId: user.Id, AdminUserId: admin.Id, ExpectedVersion: 0, Enabled: true, Reason: "first version",
		Rules: []model.CustomerContractRule{{PublicModel: "contract-model", RouteGroup: "contract-api", RatioUnits: 80_000_000}},
	})
	require.NoError(t, err)
	c, recorder := customerContractAdminContext(http.MethodPut, "/api/user/1/contract", `{
		"expected_version":0,"enabled":true,"reason":"stale edit",
		"rules":[{"model":"contract-model","route_group":"contract-api","discount":"50%"}]
	}`, admin, user)

	PutCustomerContract(c)

	assert.Equal(t, http.StatusConflict, recorder.Code)
	current, err := model.GetCustomerContractSnapshot(user.Id)
	require.NoError(t, err)
	assert.EqualValues(t, 1, current.Version)
	assert.EqualValues(t, 80_000_000, current.Rules[0].RatioUnits)
}

func TestSelfCustomerContractResponseHidesInternalRouteAndProviderFacts(t *testing.T) {
	admin, user := setupCustomerContractControllerDB(t)
	_, err := model.ReplaceCustomerContract(model.ReplaceCustomerContractParams{
		UserId: user.Id, AdminUserId: admin.Id, ExpectedVersion: 0, Enabled: true, Reason: "customer-visible contract",
		Rules: []model.CustomerContractRule{{PublicModel: "contract-model", RouteGroup: "contract-api", RatioUnits: 60_000_000}},
	})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/user/self/contract", nil)
	c.Set("id", user.Id)

	GetSelfCustomerContract(c)

	assert.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	assert.Contains(t, body, `"contract_mode":true`)
	assert.Contains(t, body, `"discount":"0.6"`)
	assert.NotContains(t, body, "route_group")
	assert.NotContains(t, body, "contract-api")
	assert.NotContains(t, body, "channel_id")
	assert.NotContains(t, body, "provider")
}

func TestCustomerContractAdminAPIEnforcesTargetRoleBoundary(t *testing.T) {
	admin, user := setupCustomerContractControllerDB(t)
	peer := user
	peer.Id = 0
	peer.Username = "peer-admin"
	peer.AffCode = "peer-admin-aff"
	peer.Role = common.RoleAdminUser
	require.NoError(t, model.DB.Create(&peer).Error)
	c, recorder := customerContractAdminContext(http.MethodGet, "/api/user/1/contract", "", admin, peer)

	GetCustomerContract(c)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":false`)
}

func createEnabledCustomerContract(t *testing.T, admin model.User, user model.User) {
	t.Helper()
	_, err := model.ReplaceCustomerContract(model.ReplaceCustomerContractParams{
		UserId: user.Id, AdminUserId: admin.Id, ExpectedVersion: 0, Enabled: true, Reason: "activate test contract",
		Rules: []model.CustomerContractRule{{PublicModel: "contract-model", RouteGroup: "contract-api", RatioUnits: 80_000_000}},
	})
	require.NoError(t, err)
}

func customerContractUserContext(method string, path string, user model.User) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(method, path, nil)
	c.Set("id", user.Id)
	common.SetContextKey(c, constant.ContextKeyUserGroup, user.Group)
	common.SetContextKey(c, constant.ContextKeyContractMode, true)
	common.SetContextKey(c, constant.ContextKeyContractVersion, int64(1))
	return c, recorder
}

func TestCustomerContractModelDiscoveryReturnsOnlyExactContractModels(t *testing.T) {
	admin, user := setupCustomerContractControllerDB(t)
	createEnabledCustomerContract(t, admin, user)
	outside := model.Channel{Name: "outside", Group: "default", Models: "outside-model", Key: "key", Status: common.ChannelStatusEnabled}
	require.NoError(t, model.DB.Create(&outside).Error)
	priority := int64(0)
	require.NoError(t, model.DB.Create(&model.Ability{Group: "default", Model: "outside-model", ChannelId: outside.Id, Enabled: true, Priority: &priority}).Error)

	c, recorder := customerContractUserContext(http.MethodGet, "/v1/models", user)
	ListModels(c, constant.ChannelTypeOpenAI)

	assert.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	assert.Contains(t, body, `"id":"contract-model"`)
	assert.Contains(t, body, `"owned_by":"new-api"`)
	assert.NotContains(t, body, "outside-model")
	assert.NotContains(t, body, "contract-api")

	limited, limitedRecorder := customerContractUserContext(http.MethodGet, "/v1/models", user)
	common.SetContextKey(limited, constant.ContextKeyTokenModelLimitEnabled, true)
	common.SetContextKey(limited, constant.ContextKeyTokenModelLimit, map[string]bool{})
	ListModels(limited, constant.ChannelTypeOpenAI)
	assert.NotContains(t, limitedRecorder.Body.String(), "contract-model")
}

func TestCustomerContractRetrieveModelUsesExactCase(t *testing.T) {
	admin, user := setupCustomerContractControllerDB(t)
	createEnabledCustomerContract(t, admin, user)

	c, recorder := customerContractUserContext(http.MethodGet, "/v1/models/contract-model", user)
	c.Params = gin.Params{{Key: "model", Value: "contract-model"}}
	RetrieveModel(c, constant.ChannelTypeOpenAI)
	assert.Contains(t, recorder.Body.String(), `"id":"contract-model"`)

	wrongCase, wrongCaseRecorder := customerContractUserContext(http.MethodGet, "/v1/models/Contract-Model", user)
	wrongCase.Params = gin.Params{{Key: "model", Value: "Contract-Model"}}
	RetrieveModel(wrongCase, constant.ChannelTypeOpenAI)
	assert.Contains(t, wrongCaseRecorder.Body.String(), `"code":"model_not_found"`)
}

func TestCustomerContractPricingUsesPerModelEffectiveMultiplierWithoutRouteLeak(t *testing.T) {
	admin, user := setupCustomerContractControllerDB(t)
	createEnabledCustomerContract(t, admin, user)

	c, recorder := customerContractUserContext(http.MethodGet, "/api/pricing", user)
	GetPricing(c)

	assert.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	assert.Contains(t, body, `"model_name":"contract-model"`)
	assert.Contains(t, body, `"group_ratio":{"contract":0.696}`)
	assert.Contains(t, body, `"owner_by":"new-api"`)
	assert.NotContains(t, body, "contract-api")
}

func TestCustomerContractAdminPreviewUsesNativeSpecialGroupRatioBeforeDiscount(t *testing.T) {
	admin, user := setupCustomerContractControllerDB(t)
	previous := ratio_setting.GroupGroupRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{"default":{"contract-api":0.9}}`))
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(previous))
	})
	createEnabledCustomerContract(t, admin, user)

	c, recorder := customerContractAdminContext(http.MethodGet, "/api/user/1/contract", "", admin, user)
	GetCustomerContract(c)

	body := recorder.Body.String()
	assert.Contains(t, body, `"native_group_ratio":"0.9"`)
	assert.Contains(t, body, `"effective_multiplier":"0.72"`)
	assert.Contains(t, body, `"special_group_ratio":true`)
}

func TestCustomerContractAdminOptionsAndAuditAreOperationalAndSafe(t *testing.T) {
	admin, user := setupCustomerContractControllerDB(t)
	createEnabledCustomerContract(t, admin, user)

	optionsContext, optionsRecorder := customerContractAdminContext(http.MethodGet, "/api/user/1/contract/options", "", admin, user)
	GetCustomerContractOptions(optionsContext)
	assert.Contains(t, optionsRecorder.Body.String(), `"group":"contract-api"`)
	assert.Contains(t, optionsRecorder.Body.String(), `"contract-model"`)
	assert.Contains(t, optionsRecorder.Body.String(), `"current_discounted_price":"0.87"`)
	assert.NotContains(t, optionsRecorder.Body.String(), `"group":"auto"`)

	auditContext, auditRecorder := customerContractAdminContext(http.MethodGet, "/api/user/1/contract/audits", "", admin, user)
	GetCustomerContractAudits(auditContext)
	auditBody := auditRecorder.Body.String()
	assert.Contains(t, auditBody, `"admin_username":"contract-api-admin"`)
	assert.Contains(t, auditBody, `"before_rule_count":0`)
	assert.Contains(t, auditBody, `"after_rule_count":1`)
	assert.NotContains(t, auditBody, "before_state")
	assert.NotContains(t, auditBody, "after_state")
}
