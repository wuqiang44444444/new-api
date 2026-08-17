package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfiguredSeedanceModelAvailabilityUsesCallerAccess(t *testing.T) {
	withSelfUseModeEnabled(t)
	db := setupModelListControllerTestDB(t)

	enabled := &model.Channel{
		Type: constant.ChannelTypeSeedanceLink, Status: common.ChannelStatusEnabled,
		Name: "enabled Seedance", Key: "unused", Group: "default", Models: "seedance-enabled",
	}
	enabled.SetOtherSettings(dto.ChannelOtherSettings{
		VideoUpstreamProtocol: dto.VideoUpstreamProtocolModelArkV3Volcengine,
		AssetUpstreamProtocol: dto.AssetUpstreamProtocolVolcengineAction,
	})
	disabled := &model.Channel{
		Type: constant.ChannelTypeSeedanceLink, Status: common.ChannelStatusManuallyDisabled,
		Name: "disabled Seedance", Key: "unused", Group: "default", Models: "seedance-disabled",
	}
	disabled.SetOtherSettings(dto.ChannelOtherSettings{
		VideoUpstreamProtocol: dto.VideoUpstreamProtocolModelArkV3Volcengine,
		AssetUpstreamProtocol: dto.AssetUpstreamProtocolVolcengineAction,
	})
	require.NoError(t, db.Create(&[]*model.Channel{enabled, disabled}).Error)

	retrieve := func(modelName, userGroup string, modelLimit map[string]bool) dto.OpenAIModels {
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Request = httptest.NewRequest(http.MethodGet, "/v1/models/"+modelName, nil)
		context.Params = gin.Params{{Key: "model", Value: modelName}}
		common.SetContextKey(context, constant.ContextKeyUserGroup, userGroup)
		if modelLimit != nil {
			common.SetContextKey(context, constant.ContextKeyTokenModelLimitEnabled, true)
			common.SetContextKey(context, constant.ContextKeyTokenModelLimit, modelLimit)
		}
		RetrieveModel(context, constant.ChannelTypeOpenAI)
		require.Equal(t, http.StatusOK, recorder.Code)
		var response dto.OpenAIModels
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		return response
	}

	allowed := retrieve("seedance-enabled", "default", nil)
	require.NotNil(t, allowed.Available)
	assert.True(t, *allowed.Available)
	assert.Equal(t, "available", allowed.Availability)

	restricted := retrieve("seedance-enabled", "default", map[string]bool{"another-model": true})
	require.NotNil(t, restricted.Available)
	assert.False(t, *restricted.Available)
	assert.Equal(t, "restricted", restricted.Availability)

	wrongGroup := retrieve("seedance-enabled", "vip", nil)
	require.NotNil(t, wrongGroup.Available)
	assert.False(t, *wrongGroup.Available)
	assert.Equal(t, "restricted", wrongGroup.Availability)

	disabledModel := retrieve("seedance-disabled", "default", nil)
	require.NotNil(t, disabledModel.Available)
	assert.False(t, *disabledModel.Available)
	assert.Equal(t, "disabled", disabledModel.Availability)
}
