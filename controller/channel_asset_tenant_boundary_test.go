package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAssetTenantControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB := model.DB
	previousLogDB := model.LOG_DB
	previousMainType := common.MainDatabaseType()
	previousLogType := common.LogDatabaseType()
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	previousRedisEnabled := common.RedisEnabled

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.Channel{},
		&model.Ability{},
		&model.ChannelAssetCredential{},
		&model.ChannelAssetScopeIdentity{},
		&model.Log{},
	))
	model.DB = db
	model.LOG_DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.MemoryCacheEnabled = false
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.DB = previousDB
		model.LOG_DB = previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		common.RedisEnabled = previousRedisEnabled
		sqlDB, closeErr := db.DB()
		if closeErr == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestAssetTenantMutationErrorsExposeStableCodes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name string
		err  error
		code string
	}{
		{name: "immutable boundary", err: model.ErrAssetTenantBoundaryImmutable, code: assetTenantBoundaryImmutableCode},
		{
			name: "unconfirmed replacement",
			err:  &model.AssetTenantReplacementRequiredError{ChangedFields: []string{"asset_provider_project"}},
			code: assetTenantReplacementUnconfirmedCode,
		},
		{name: "unconfirmed rotation", err: model.ErrAssetTenantRotationUnconfirmed, code: assetTenantRotationUnconfirmedCode},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			assert.True(t, respondAssetTenantMutationError(context, tt.err))
			assert.Equal(t, 409, recorder.Code)

			var payload map[string]any
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
			assert.Equal(t, tt.code, payload["error_code"])
			assert.Equal(t, false, payload["success"])
			if tt.code == assetTenantReplacementUnconfirmedCode {
				assert.Contains(t, payload["changed_fields"], "asset_provider_project")
			}
		})
	}
}

func TestConfirmedAssetTenantCredentialRotationIsAuditedWithoutSecrets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAssetTenantControllerTestDB(t)

	const (
		rootUserID     = 900
		oldVideoSecret = "old-video-secret"
		newVideoSecret = "rotated-video-secret"
		customerModel  = "customer-fast"
	)
	require.NoError(t, db.Create(&model.User{
		Id: rootUserID, Username: "root", Password: "not-used-in-test", Role: common.RoleRootUser,
		Status: common.UserStatusEnabled, Group: "default",
	}).Error)

	channel := &model.Channel{
		Type: constant.ChannelTypeSeedanceLink, Status: common.ChannelStatusManuallyDisabled,
		Name: "audit-rotation", Key: oldVideoSecret, Models: customerModel, Group: "default",
		BaseURL:      common.GetPointer("https://assets.example.com"),
		ModelMapping: common.GetPointer(`{"customer-fast":"doubao-seedance-2-0-fast-260128"}`),
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		VideoUpstreamProtocol: dto.VideoUpstreamProtocolMoxingModelArkV1,
		AssetUpstreamProtocol: dto.AssetUpstreamProtocolMoxingVolcAssetsV1,
		AssetMinURLTTLSeconds: 3600,
	})
	require.NoError(t, channel.ValidateSettings())
	require.NoError(t, channel.Insert())

	update := func(confirm bool) *httptest.ResponseRecorder {
		t.Helper()
		payload := map[string]any{
			"id":            channel.Id,
			"type":          channel.Type,
			"name":          channel.Name,
			"key":           newVideoSecret,
			"models":        channel.Models,
			"group":         channel.Group,
			"base_url":      channel.BaseURL,
			"model_mapping": channel.ModelMapping,
			"settings":      channel.OtherSettings,
		}
		if confirm {
			payload["confirm_asset_tenant_unchanged"] = true
		}
		body, marshalErr := common.Marshal(payload)
		require.NoError(t, marshalErr)

		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Set("id", rootUserID)
		context.Set("role", common.RoleRootUser)
		context.Set("username", "root")
		context.Request = httptest.NewRequest(http.MethodPut, "/api/channel/", bytes.NewReader(body))
		context.Request.Header.Set("Content-Type", "application/json")
		UpdateChannel(context)
		return recorder
	}

	unconfirmed := update(false)
	assert.Equal(t, http.StatusConflict, unconfirmed.Code)
	var rejected map[string]any
	require.NoError(t, common.Unmarshal(unconfirmed.Body.Bytes(), &rejected))
	assert.Equal(t, assetTenantRotationUnconfirmedCode, rejected["error_code"])

	confirmed := update(true)
	assert.Equal(t, http.StatusOK, confirmed.Code)
	var accepted map[string]any
	require.NoError(t, common.Unmarshal(confirmed.Body.Bytes(), &accepted))
	assert.Equal(t, true, accepted["success"])

	stored, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.Equal(t, newVideoSecret, stored.Key)

	var logs []model.Log
	require.NoError(t, db.Where("type = ?", model.LogTypeManage).Find(&logs).Error)
	require.Len(t, logs, 1)
	var other map[string]any
	require.NoError(t, common.UnmarshalJsonStr(logs[0].Other, &other))
	op, ok := other["op"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "channel.update", op["action"])
	params, ok := op["params"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, params["asset_tenant_unchanged_confirmed"])
	assert.Contains(t, params["changed_fields"], "key")

	auditRecord := logs[0].Content + logs[0].Other
	assert.NotContains(t, auditRecord, oldVideoSecret)
	assert.NotContains(t, auditRecord, newVideoSecret)
}

func TestConfirmedAssetTenantReplacementKeepsChannelAndModelsAndRotatesScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAssetTenantControllerTestDB(t)

	const (
		rootUserID    = 901
		customerModel = "customer-seedance"
	)
	require.NoError(t, db.Create(&model.User{
		Id: rootUserID, Username: "root-replacement", Password: "not-used-in-test",
		Role: common.RoleRootUser, Status: common.UserStatusEnabled, Group: "default",
	}).Error)

	channel := &model.Channel{
		Type: constant.ChannelTypeSeedanceLink, Status: common.ChannelStatusManuallyDisabled,
		Name: "tenant-replacement", Key: "video-secret", Models: customerModel, Group: "default",
		BaseURL:      common.GetPointer("https://ark.cn-beijing.volces.com"),
		ModelMapping: common.GetPointer(`{"customer-seedance":"doubao-seedance-2-0-fast-260128"}`),
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		VideoUpstreamProtocol: dto.VideoUpstreamProtocolModelArkV3Volcengine,
		AssetUpstreamProtocol: dto.AssetUpstreamProtocolVolcengineAction,
		AssetMinURLTTLSeconds: 3600,
		AssetProviderProject:  "default",
		AssetRegion:           model.VolcengineAssetActionRegion,
	})
	require.NoError(t, channel.ValidateSettings())
	require.NoError(t, model.InsertChannelWithAssetCredentialActor(
		channel,
		&dto.ChannelAssetCredentialInput{AccessKeyID: "asset-access", SecretAccessKey: "asset-secret"},
		rootUserID,
	))
	originalScope, err := model.ChannelAssetReuseScope(channel.Id)
	require.NoError(t, err)

	replacementSettings := channel.GetOtherSettings()
	replacementSettings.AssetProviderProject = "lumen-test"
	channel.SetOtherSettings(replacementSettings)

	update := func(confirm bool) *httptest.ResponseRecorder {
		t.Helper()
		payload := map[string]any{
			"id": channel.Id, "type": channel.Type, "name": channel.Name,
			"models": channel.Models, "group": channel.Group, "base_url": channel.BaseURL,
			"model_mapping": channel.ModelMapping, "settings": channel.OtherSettings,
		}
		if confirm {
			payload["confirm_asset_tenant_replacement"] = true
		}
		body, marshalErr := common.Marshal(payload)
		require.NoError(t, marshalErr)
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Set("id", rootUserID)
		context.Set("role", common.RoleRootUser)
		context.Set("username", "root-replacement")
		context.Request = httptest.NewRequest(http.MethodPut, "/api/channel/", bytes.NewReader(body))
		context.Request.Header.Set("Content-Type", "application/json")
		UpdateChannel(context)
		return recorder
	}

	unconfirmed := update(false)
	assert.Equal(t, http.StatusConflict, unconfirmed.Code)
	var rejected map[string]any
	require.NoError(t, common.Unmarshal(unconfirmed.Body.Bytes(), &rejected))
	assert.Equal(t, assetTenantReplacementUnconfirmedCode, rejected["error_code"])
	assert.Contains(t, rejected["changed_fields"], "asset_provider_project")

	storedBefore, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.Equal(t, "default", storedBefore.GetOtherSettings().AssetProviderProject)
	scopeBefore, err := model.ChannelAssetReuseScope(channel.Id)
	require.NoError(t, err)
	assert.Equal(t, originalScope, scopeBefore)

	confirmed := update(true)
	assert.Equal(t, http.StatusOK, confirmed.Code)
	var accepted map[string]any
	require.NoError(t, common.Unmarshal(confirmed.Body.Bytes(), &accepted))
	assert.Equal(t, true, accepted["success"])

	storedAfter, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.Equal(t, channel.Id, storedAfter.Id)
	assert.Equal(t, customerModel, storedAfter.Models)
	assert.Equal(t, "lumen-test", storedAfter.GetOtherSettings().AssetProviderProject)
	replacedScope, err := model.ChannelAssetReuseScope(channel.Id)
	require.NoError(t, err)
	assert.NotEqual(t, originalScope, replacedScope)

	var logs []model.Log
	require.NoError(t, db.Where("type = ?", model.LogTypeManage).Find(&logs).Error)
	require.Len(t, logs, 1)
	var other map[string]any
	require.NoError(t, common.UnmarshalJsonStr(logs[0].Other, &other))
	op, ok := other["op"].(map[string]any)
	require.True(t, ok)
	params, ok := op["params"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, params["asset_tenant_replaced"])
	assert.Contains(t, params["asset_tenant_boundary_changed_fields"], "asset_provider_project")
	assert.NotContains(t, logs[0].Content+logs[0].Other, "asset-secret")
}

func TestPartialChannelUpdateWithoutPersistedBoundaryChangeDoesNotAuditReplacement(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAssetTenantControllerTestDB(t)

	const (
		rootUserID    = 902
		customerModel = "customer-seedance"
	)
	require.NoError(t, db.Create(&model.User{
		Id: rootUserID, Username: "root-partial-update", Password: "not-used-in-test",
		Role: common.RoleRootUser, Status: common.UserStatusEnabled, Group: "default",
	}).Error)

	channel := &model.Channel{
		Type: constant.ChannelTypeSeedanceLink, Status: common.ChannelStatusManuallyDisabled,
		Name: "partial-update", Key: "video-secret", Models: customerModel, Group: "default",
		BaseURL:      common.GetPointer("https://ark.cn-beijing.volces.com"),
		ModelMapping: common.GetPointer(`{"customer-seedance":"doubao-seedance-2-0-fast-260128"}`),
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		VideoUpstreamProtocol: dto.VideoUpstreamProtocolModelArkV3Volcengine,
		AssetUpstreamProtocol: dto.AssetUpstreamProtocolVolcengineAction,
		AssetMinURLTTLSeconds: 3600,
		AssetProviderProject:  "default",
		AssetRegion:           model.VolcengineAssetActionRegion,
	})
	require.NoError(t, channel.ValidateSettings())
	require.NoError(t, model.InsertChannelWithAssetCredentialActor(
		channel,
		&dto.ChannelAssetCredentialInput{AccessKeyID: "asset-access", SecretAccessKey: "asset-secret"},
		rootUserID,
	))
	originalScope, err := model.ChannelAssetReuseScope(channel.Id)
	require.NoError(t, err)

	// 请求省略 base_url：零值不会落库，即使携带替换确认也不构成实际边界变化。
	payload := map[string]any{
		"id": channel.Id, "type": channel.Type, "name": channel.Name,
		"models": channel.Models, "group": channel.Group,
		"settings":                         channel.OtherSettings,
		"confirm_asset_tenant_replacement": true,
	}
	body, marshalErr := common.Marshal(payload)
	require.NoError(t, marshalErr)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Set("id", rootUserID)
	context.Set("role", common.RoleRootUser)
	context.Set("username", "root-partial-update")
	context.Request = httptest.NewRequest(http.MethodPut, "/api/channel/", bytes.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")
	UpdateChannel(context)
	assert.Equal(t, http.StatusOK, recorder.Code)
	var response map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, true, response["success"], "update rejected: %v", response["message"])

	stored, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.Equal(t, "default", stored.GetOtherSettings().AssetProviderProject)
	scopeAfter, err := model.ChannelAssetReuseScope(channel.Id)
	require.NoError(t, err)
	assert.Equal(t, originalScope, scopeAfter)

	var logs []model.Log
	require.NoError(t, db.Where("type = ?", model.LogTypeManage).Find(&logs).Error)
	require.Len(t, logs, 1)
	var other map[string]any
	require.NoError(t, common.UnmarshalJsonStr(logs[0].Other, &other))
	op, ok := other["op"].(map[string]any)
	require.True(t, ok)
	params, ok := op["params"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, false, params["asset_tenant_replaced"])
	assert.Empty(t, params["asset_tenant_boundary_changed_fields"])
	assert.Empty(t, params["changed_fields"])
}
