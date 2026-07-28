package relay

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreparePersistentImageTaskRequestMarksOnlyMediaTaskRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/images/generations", nil)
	common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeAdvancedCustom)
	common.SetContextKey(c, constant.ContextKeyChannelOtherSetting, dto.ChannelOtherSettings{
		AdvancedCustom: &dto.AdvancedCustomConfig{Routes: []dto.AdvancedCustomRoute{{
			IncomingPath: "/v1/images/generations",
			UpstreamPath: "/v1/images/generations",
			Converter:    dto.AdvancedCustomConverterMediaTaskImageBlocking,
			Models:       []string{"seedream-5-0-260128"},
		}}},
	})
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeImagesGenerations,
		OriginModelName: "seedream-5-0-260128",
	}

	PreparePersistentImageTaskRequest(c, info)

	assert.True(t, info.ForcePreConsume)
	require.NotNil(t, info.TaskRelayInfo)
	assert.Equal(t, model.TaskClientProtocolOpenAIImages, info.TaskRelayInfo.ClientProtocol)
	assert.True(t, common.GetContextKeyBool(c, constant.ContextKeyTaskPersistenceEnabled))
}
