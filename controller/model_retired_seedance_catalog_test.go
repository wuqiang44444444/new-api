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

func TestRetiredSeedanceChannelsDoNotBlockModelAndPriceCatalogs(t *testing.T) {
	withSelfUseModeEnabled(t)
	db := setupModelListControllerTestDB(t)
	withTieredBillingConfig(t, map[string]string{"native-chat": "tiered_expr", "current-video": "tiered_expr"},
		map[string]string{"native-chat": "p + c", "current-video": "c * 5"})
	for _, tc := range []struct {
		name     string
		protocol dto.VideoUpstreamProtocol
	}{
		{"retired-funcloud", dto.VideoUpstreamProtocolFunCloudSeedance},
		{"retired-moxing", dto.VideoUpstreamProtocolMoxingMediaTaskV1},
		{"current-video", dto.VideoUpstreamProtocolModelArkV3Volcengine},
	} {
		channel := model.Channel{
			Type: constant.ChannelTypeSeedanceLink, Status: common.ChannelStatusManuallyDisabled,
			Name: tc.name, Models: tc.name, Group: "default", Key: "test-key",
		}
		mapping, err := common.Marshal(map[string]string{tc.name: "doubao-seedance-2-0-260128"})
		require.NoError(t, err)
		channel.ModelMapping = common.GetPointer(string(mapping))
		channel.SetOtherSettings(dto.ChannelOtherSettings{
			VideoUpstreamProtocol: tc.protocol, AssetUpstreamProtocol: dto.AssetUpstreamProtocolNone,
		})
		require.NoError(t, db.Create(&channel).Error)
	}
	native := model.Channel{Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled,
		Name: "native", Models: "native-chat", Group: "default", Key: "test-key"}
	require.NoError(t, native.Insert())
	model.InvalidatePricingCache()
	prices := pricingByModelName(model.GetPricing())
	assert.Contains(t, prices, "native-chat")
	assert.Contains(t, prices, "current-video")
	assert.NotContains(t, prices, "retired-funcloud")
	assert.NotContains(t, prices, "retired-moxing")

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	ListModels(c, constant.ChannelTypeOpenAI)
	ids := decodeListModelsResponse(t, recorder)
	assert.Contains(t, ids, "native-chat")
	assert.Contains(t, ids, "current-video")
	assert.NotContains(t, ids, "retired-funcloud")
	assert.NotContains(t, ids, "retired-moxing")
	var count int64
	require.NoError(t, db.Model(&model.Channel{}).Where("status = ?", common.ChannelStatusManuallyDisabled).Count(&count).Error)
	assert.EqualValues(t, 3, count, "catalog reads do not migrate or delete historical channels")
}
