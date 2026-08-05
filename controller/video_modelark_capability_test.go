package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestModelArkVideoCapabilitiesListsRegisteredModelsAndTokenAvailability(t *testing.T) {
	initModelListColumnNames(t)
	previousDB := model.DB
	previousType := common.MainDatabaseType()
	previousSelfUse := operation_setting.SelfUseModeEnabled
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	operation_setting.SelfUseModeEnabled = true
	require.NoError(t, db.AutoMigrate(
		&model.Channel{}, &model.Ability{}, &model.LinkModelPublication{}, &model.LinkModelPublicationAudit{},
	))
	t.Cleanup(func() {
		model.DB = previousDB
		common.SetMainDatabaseType(previousType)
		operation_setting.SelfUseModeEnabled = previousSelfUse
	})

	channel := &model.Channel{
		Type: constant.ChannelTypeDoubaoVideo, Models: model.VideoSKUSeedanceBytePlus,
		Group: "default", Status: common.ChannelStatusEnabled,
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		VideoUpstreamProfile: dto.VideoUpstreamProfileOfficial,
		AssetUpstreamProfile: dto.AssetUpstreamProfileOfficial,
		LinkImplementation: dto.LinkImplementationRef{
			ID: model.LinkImplementationBytePlusSeedanceArk, Version: model.LinkImplementationVersionV1,
		},
	})
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group: "default", Model: model.VideoSKUSeedanceBytePlus, ChannelId: channel.Id, Enabled: true,
	}).Error)
	require.NoError(t, model.EnsureChannelLinkModelPublications(db, channel, 1))
	aliasChannel := &model.Channel{
		Type: constant.ChannelTypeDoubaoVideo, Models: "customer-seedance",
		Group: "default", Status: common.ChannelStatusEnabled,
		ModelMapping: common.GetPointer(`{"customer-seedance":"seedance-byteplus"}`),
	}
	aliasChannel.SetOtherSettings(dto.ChannelOtherSettings{
		VideoUpstreamProfile: dto.VideoUpstreamProfileOfficial,
		AssetUpstreamProfile: dto.AssetUpstreamProfileOfficial,
		LinkImplementation: dto.LinkImplementationRef{
			ID: model.LinkImplementationBytePlusSeedanceArk, Version: model.LinkImplementationVersionV1,
		},
	})
	require.NoError(t, db.Create(aliasChannel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group: "default", Model: "customer-seedance", ChannelId: aliasChannel.Id, Enabled: true,
	}).Error)
	require.NoError(t, model.EnsureChannelLinkModelPublications(db, aliasChannel, 1))

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/v3/contents/generations/models", nil)
	common.SetContextKey(context, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(context, constant.ContextKeyTokenGroup, "default")

	ModelArkVideoCapabilities(context)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response modelArkVideoCapabilityCatalog
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, "list", response.Object)
	require.Len(t, response.Data, 16)
	items := make(map[string]modelArkVideoCapabilityCatalogItem, len(response.Data))
	for _, item := range response.Data {
		items[item.ID] = item
	}
	bytePlus := items[model.VideoSKUSeedanceBytePlus]
	assert.True(t, bytePlus.Published)
	assert.True(t, bytePlus.VisibleInV1Models)
	assert.True(t, bytePlus.Available)
	assert.Equal(t, model.VideoSKUCapabilityVersionModelArkV2, bytePlus.Capability.Version)
	alias := items["customer-seedance"]
	assert.True(t, alias.Published)
	assert.True(t, alias.VisibleInV1Models)
	assert.True(t, alias.Available)
	assert.Equal(t, "customer-seedance", alias.Capability.PublicModel)
	assert.Equal(t, bytePlus.Capability.ContentHash, alias.Capability.ContentHash)
	assert.False(t, items[model.VideoSKUSeedance20Value4K].Published)
	assert.False(t, items[model.VideoSKUSeedance20Value4K].Available)

	aliasRecorder := httptest.NewRecorder()
	aliasContext, _ := gin.CreateTestContext(aliasRecorder)
	aliasContext.Request = httptest.NewRequest(http.MethodGet, "/api/v3/contents/generations/models?model=customer-seedance", nil)
	common.SetContextKey(aliasContext, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(aliasContext, constant.ContextKeyTokenGroup, "default")

	ModelArkVideoCapabilities(aliasContext)

	require.Equal(t, http.StatusOK, aliasRecorder.Code)
	var aliasResponse modelArkVideoCapabilityCatalog
	require.NoError(t, common.Unmarshal(aliasRecorder.Body.Bytes(), &aliasResponse))
	require.Len(t, aliasResponse.Data, 1)
	assert.Equal(t, "customer-seedance", aliasResponse.Data[0].ID)
	assert.Equal(t, "customer-seedance", aliasResponse.Data[0].Capability.PublicModel)
}
