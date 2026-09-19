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
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupCustomerContractControllerDB(t *testing.T) (model.User, model.User, model.ContractEntitySnapshot) {
	t.Helper()
	previousUsable := setting.UserUsableGroups2JSONString()
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default"}`))
	t.Cleanup(func() { require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(previousUsable)) })
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
		&model.Token{}, &model.CustomerModelContract{},
		&model.CustomerContract{}, &model.CustomerContractEntityRule{}, &model.CustomerContractEntityAudit{},
		&model.Log{},
	))
	model.DB = db
	model.LOG_DB = db
	common.RedisEnabled = false
	common.MemoryCacheEnabled = true
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	// Native model discovery filters abilities with the shared quoted group
	// column, which startup normally initializes. Bootstrap it here so these
	// tests also pass when run standalone.
	t.Setenv("LOG_SQL_DSN", "")
	require.NoError(t, model.InitLogDB())
	model.LOG_DB = db
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"contract-api":0.87}`))
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"contract-model":1,"default-model":1,"outside-model":1}`))
	model.InvalidatePricingCache()
	service.ResetContractEntityCacheForTest()

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
	defaultChannel := model.Channel{Name: "default-contract-channel", Group: "default", Models: "default-model", Key: "key", Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(&defaultChannel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group: "default", Model: "default-model", ChannelId: defaultChannel.Id, Enabled: true, Priority: &priority,
	}).Error)
	snapshot, err := model.CreateCustomerContractEntity(model.CreateCustomerContractParams{
		UserId: user.Id, AdminUserId: admin.Id, Name: "API Contract", Enabled: true,
		Reason: "activate test contract",
		Rules: []model.CustomerContractEntityRuleInput{{
			PublicModel: "contract-model", ChannelId: channel.Id, RouteGroup: "contract-api", RatioUnits: 80_000_000,
		}, {
			PublicModel: "default-model", ChannelId: defaultChannel.Id, RouteGroup: "default", RatioUnits: 90_000_000,
		}},
	})
	require.NoError(t, err)
	model.InitChannelCache()

	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		service.ResetContractEntityCacheForTest()
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
	return admin, user, *snapshot
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

func TestCustomerContractAdminAPICreatesEntityAndAppliesNativeRatioBeforeDiscount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	admin, user, contract := setupCustomerContractControllerDB(t)
	c, recorder := customerContractAdminContext(http.MethodPost, fmt.Sprintf("/api/user/%d/contract", user.Id), `{
		"enabled":true,"name":"Second Contract","reason":"signed contract",
		"rules":[{"model":"contract-model","channel_id":1,"route_group":"contract-api","discount":"8折"}]
	}`, admin, user)

	PostCustomerContractEntity(c)

	assert.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	assert.Contains(t, body, `"discount":"0.8"`)
	assert.Contains(t, body, `"native_group_ratio":"0.87"`)
	assert.Contains(t, body, `"effective_multiplier":"0.696"`)
	list, err := model.ListContractEntitiesForUser(user.Id, false)
	require.NoError(t, err)
	require.Len(t, list, 2, "a user can hold multiple contracts")
	assert.Equal(t, contract.Id, list[0].Id)
	assert.Equal(t, "Second Contract", list[1].Name)
}

func TestCustomerContractAdminAPIRejectsStaleExpectedVersion(t *testing.T) {
	admin, user, contract := setupCustomerContractControllerDB(t)
	channelId := contract.Rules[0].ChannelId
	c, recorder := customerContractAdminContext(http.MethodPut, fmt.Sprintf("/api/contract/%d", contract.Id), fmt.Sprintf(`{
		"expected_version":0,"enabled":true,"name":"API Contract","reason":"stale edit",
		"rules":[{"model":"contract-model","channel_id":%d,"route_group":"contract-api","discount":"50%%"}]
	}`, channelId), admin, user)
	c.Params = append(c.Params, gin.Param{Key: "contract_id", Value: fmt.Sprintf("%d", contract.Id)})

	PutCustomerContractEntity(c)

	assert.Equal(t, http.StatusConflict, recorder.Code, recorder.Body.String())
	current, err := model.GetContractEntitySnapshot(contract.Id, false)
	require.NoError(t, err)
	assert.EqualValues(t, 1, current.Version)
	assert.EqualValues(t, 80_000_000, current.Rules[0].RatioUnits)
}

func TestSelfCustomerContractResponseHidesInternalRouteAndProviderFacts(t *testing.T) {
	_, user, _ := setupCustomerContractControllerDB(t)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/user/self/contract", nil)
	c.Set("id", user.Id)

	GetSelfCustomerContract(c)

	assert.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	assert.Contains(t, body, `"enabled":true`)
	assert.Contains(t, body, `"discount":"0.8"`)
	assert.NotContains(t, body, "route_group")
	assert.NotContains(t, body, "contract-api")
	assert.NotContains(t, body, "channel_id")
	assert.NotContains(t, body, "provider")
}

func TestCustomerContractAdminAPIEnforcesTargetRoleBoundary(t *testing.T) {
	admin, user, _ := setupCustomerContractControllerDB(t)
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

func customerContractTokenContext(method string, path string, user model.User, contractId int) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(method, path, nil)
	c.Set("id", user.Id)
	common.SetContextKey(c, constant.ContextKeyUserGroup, user.Group)
	common.SetContextKey(c, constant.ContextKeyAuthVersion, user.AuthVersion)
	common.SetContextKey(c, constant.ContextKeyTokenContractId, contractId)
	return c, recorder
}

func TestModelDiscoveryUsesContractScopeAcrossGroups(t *testing.T) {
	_, user, contract := setupCustomerContractControllerDB(t)
	outside := model.Channel{Name: "outside", Group: "default", Models: "outside-model", Key: "key", Status: common.ChannelStatusEnabled}
	require.NoError(t, model.DB.Create(&outside).Error)
	priority := int64(0)
	require.NoError(t, model.DB.Create(&model.Ability{Group: "default", Model: "outside-model", ChannelId: outside.Id, Enabled: true, Priority: &priority}).Error)
	model.InitChannelCache()

	// A bound key discovers effective contract models outside its saved group.
	c, recorder := customerContractTokenContext(http.MethodGet, "/v1/models", user, contract.Id)
	ListModels(c, constant.ChannelTypeOpenAI)

	assert.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	assert.NotContains(t, body, `"id":"outside-model"`)
	assert.Contains(t, body, `"id":"contract-model"`)
	assert.NotContains(t, body, "contract-api")

	// An unbound key retains the native projection.
	native, nativeRecorder := customerContractTokenContext(http.MethodGet, "/v1/models", user, 0)
	ListModels(native, constant.ChannelTypeOpenAI)
	assert.Equal(t, http.StatusOK, nativeRecorder.Code)
	assert.Contains(t, nativeRecorder.Body.String(), `"id":"outside-model"`)
	assert.NotContains(t, nativeRecorder.Body.String(), `"id":"contract-model"`, "contract access does not grant native access to the internal group")

	limited, limitedRecorder := customerContractTokenContext(http.MethodGet, "/v1/models", user, contract.Id)
	common.SetContextKey(limited, constant.ContextKeyTokenModelLimitEnabled, true)
	common.SetContextKey(limited, constant.ContextKeyTokenModelLimit, map[string]bool{})
	ListModels(limited, constant.ChannelTypeOpenAI)
	assert.NotContains(t, limitedRecorder.Body.String(), "outside-model")
}

func TestRetrieveModelUsesContractScopeAcrossGroups(t *testing.T) {
	_, user, contract := setupCustomerContractControllerDB(t)
	outside := model.Channel{Name: "outside", Group: "default", Models: "outside-model", Key: "key", Status: common.ChannelStatusEnabled}
	require.NoError(t, model.DB.Create(&outside).Error)
	priority := int64(0)
	require.NoError(t, model.DB.Create(&model.Ability{Group: "default", Model: "outside-model", ChannelId: outside.Id, Enabled: true, Priority: &priority}).Error)
	model.InitChannelCache()
	t.Cleanup(func() { model.InvalidatePricingCache() })

	c, recorder := customerContractTokenContext(http.MethodGet, "/v1/models/outside-model", user, contract.Id)
	c.Params = gin.Params{{Key: "model", Value: "outside-model"}}
	RetrieveModel(c, constant.ChannelTypeOpenAI)
	assert.Contains(t, recorder.Body.String(), `"code":"model_not_found"`)

	// The contract authorizes an internal group unavailable to the user and saved Key.
	denied, deniedRecorder := customerContractTokenContext(http.MethodGet, "/v1/models/contract-model", user, contract.Id)
	denied.Params = gin.Params{{Key: "model", Value: "contract-model"}}
	RetrieveModel(denied, constant.ChannelTypeOpenAI)
	assert.Contains(t, deniedRecorder.Body.String(), `"id":"contract-model"`)
}

func TestContractPricingOverlaysDiscountOnNativeProjection(t *testing.T) {
	_, user, contract := setupCustomerContractControllerDB(t)

	c, recorder := customerContractTokenContext(http.MethodGet, "/api/pricing", user, contract.Id)
	GetPricing(c)

	assert.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	// default-model is natively reachable in the key's own group: the overlay
	// applies the contract discount on top of the native group ratio.
	assert.Contains(t, body, `"model_name":"default-model"`)
	assert.Contains(t, body, `"contract_discount":"0.9"`)
	assert.Contains(t, body, `"group_ratio":{"default":0.9}`, "per-group effective ratio = native group ratio × contract discount")
	// Contract models outside the saved Key group use their actual group price.
	assert.Contains(t, body, "contract-model")
	assert.Contains(t, body, `"group_ratio":{"contract-api":0.696}`, "only the contract group contributes to this quote")
	assert.NotContains(t, body, "channel_id")
}

func TestCustomerContractAdminPreviewUsesNativeSpecialGroupRatioBeforeDiscount(t *testing.T) {
	admin, user, _ := setupCustomerContractControllerDB(t)
	previous := ratio_setting.GroupGroupRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{"default":{"contract-api":0.9}}`))
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(previous))
	})
	service.ResetContractEntityCacheForTest()

	c, recorder := customerContractAdminContext(http.MethodGet, "/api/user/1/contract", "", admin, user)
	GetCustomerContract(c)

	body := recorder.Body.String()
	assert.Contains(t, body, `"native_group_ratio":"0.9"`)
	assert.Contains(t, body, `"effective_multiplier":"0.72"`)
	assert.Contains(t, body, `"special_group_ratio":true`)
}

func TestCustomerContractAdminOptionsChannelsAndAuditAreOperationalAndSafe(t *testing.T) {
	admin, user, contract := setupCustomerContractControllerDB(t)

	optionsContext, optionsRecorder := customerContractAdminContext(http.MethodGet, "/api/user/1/contract/options", "", admin, user)
	GetCustomerContractOptions(optionsContext)
	assert.Contains(t, optionsRecorder.Body.String(), `"group":"contract-api"`)
	assert.Contains(t, optionsRecorder.Body.String(), `"contract-model"`)
	assert.Contains(t, optionsRecorder.Body.String(), `"current_discounted_price":"0.87"`)
	assert.NotContains(t, optionsRecorder.Body.String(), `"group":"auto"`)

	channelsContext, channelsRecorder := customerContractAdminContext(http.MethodGet, "/api/user/1/contract/channels", "", admin, user)
	GetCustomerContractChannelOptions(channelsContext)
	assert.Contains(t, channelsRecorder.Body.String(), `"group":"contract-api"`)

	auditContext, auditRecorder := customerContractAdminContext(http.MethodGet, fmt.Sprintf("/api/contract/%d/audits", contract.Id), "", admin, user)
	auditContext.Params = append(auditContext.Params, gin.Param{Key: "contract_id", Value: fmt.Sprintf("%d", contract.Id)})
	GetCustomerContractEntityAudits(auditContext)
	auditBody := auditRecorder.Body.String()
	assert.Contains(t, auditBody, `"admin_username":"contract-api-admin"`)
	assert.Contains(t, auditBody, `"operation":"create"`)
	assert.NotContains(t, auditBody, "before_state")
	assert.NotContains(t, auditBody, "after_state")
}

func TestTokenBindingRejectsContractsTheUserDoesNotOwn(t *testing.T) {
	admin, user, _ := setupCustomerContractControllerDB(t)
	otherSnapshot, err := model.CreateCustomerContractEntity(model.CreateCustomerContractParams{
		UserId: admin.Id, AdminUserId: admin.Id, Name: "Admin Contract", Enabled: true,
		Reason: "owner boundary fixture",
		Rules: []model.CustomerContractEntityRuleInput{{
			PublicModel: "contract-model", ChannelId: 1, RouteGroup: "contract-api", RatioUnits: 80_000_000,
		}},
	})
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/token/", strings.NewReader(fmt.Sprintf(
		`{"name":"bound-key","expired_time":-1,"remain_quota":1000,"unlimited_quota":true,"contract_id":%d}`, otherSnapshot.Id)))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("id", user.Id)
	AddToken(c)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "not owned by the user")
}

func TestContractSessionPricingRequiresExplicitOwnedSelection(t *testing.T) {
	_, user, contract := setupCustomerContractControllerDB(t)
	c, w := gin.CreateTestContext(httptest.NewRecorder())
	_ = w
	c.Request = httptest.NewRequest("GET", "/api/pricing", nil)
	c.Set("id", user.Id)
	assert.False(t, respondCustomerContractPricing(c), "no implicit default contract")
	response := httptest.NewRecorder()
	c, _ = gin.CreateTestContext(response)
	c.Request = httptest.NewRequest("GET", fmt.Sprintf("/api/pricing?contract_id=%d", contract.Id), nil)
	c.Set("id", user.Id)
	common.SetContextKey(c, constant.ContextKeyUserGroup, user.Group)
	assert.True(t, respondCustomerContractPricing(c))
	assert.Equal(t, http.StatusOK, response.Code)
	response = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(response)
	c.Request = httptest.NewRequest("GET", fmt.Sprintf("/api/pricing?contract_id=%d", contract.Id), nil)
	c.Set("id", user.Id+1000)
	assert.True(t, respondCustomerContractPricing(c))
	assert.NotEqual(t, http.StatusOK, response.Code)
}

func TestCustomerContractSeedanceCatalogDoesNotAdvertiseOtherSources(t *testing.T) {
	_, user, contract := setupCustomerContractControllerDB(t)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("id", user.Id)
	common.SetContextKey(c, constant.ContextKeyTokenContractId, contract.Id)
	common.SetContextKey(c, constant.ContextKeyAuthVersion, user.AuthVersion)
	common.SetContextKey(c, constant.ContextKeyUserGroup, user.Group)
	catalog := []model.SeedancePublicModel{{ModelName: "contract-model", Enabled: true}, {ModelName: "outside-model", Enabled: true}}
	filtered, err := customerContractSeedanceCatalog(c, catalog)
	require.NoError(t, err)
	assert.Empty(t, filtered, "ordinary sources do not advertise same-name Seedance capabilities")
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", contract.Rules[0].ChannelId).Update("type", constant.ChannelTypeSeedanceLink).Error)
	filtered, err = customerContractSeedanceCatalog(c, catalog)
	require.NoError(t, err)
	require.Len(t, filtered, 1)
	assert.Equal(t, "contract-model", filtered[0].ModelName)
}
