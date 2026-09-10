package seedance

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel/task/seedance/thirdparty/feicai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildFeicaiVideoCreateRequestUsesMappedProviderModelWithoutClientNameRules(t *testing.T) {
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", nil)
	pinSeedanceExtensionForTest(t, context)
	duration, resolution, ratio := 4, "720p", "21:9"
	relaycommon.SetVideoContractRequest(context, dto.VideoContractRequest{
		ContractID: dto.VideoContractModelArkV3,
		ModelArk: &dto.ModelArkVideoCreateRequest{
			Model:      "administrator-defined-client-name",
			Duration:   &duration,
			Resolution: &resolution,
			Ratio:      &ratio,
			Content: []dto.ModelArkVideoContent{{
				Type: "text",
				Text: common.GetPointer("A paper boat crosses a quiet lake"),
			}},
		},
	})

	adaptor := &TaskAdaptor{
		protocol: dto.VideoUpstreamProtocolFeicaiVideosV1,
		profile:  dto.VideoUpstreamProfileThirdPartyFeicaiVideos,
	}
	body, handled, err := adaptor.buildSeedancePluginCreateRequestBody(
		context,
		&relaycommon.RelayInfo{
			ChannelMeta: &relaycommon.ChannelMeta{
				IsModelMapped:     true,
				UpstreamModelName: feicai.ProviderModelSeedance20Mini720P,
			},
		},
		dto.VideoUpstreamProfileThirdPartyFeicaiVideos,
	)
	require.NoError(t, err)
	assert.True(t, handled)

	var upstream map[string]any
	require.NoError(t, common.Unmarshal(body, &upstream))
	assert.Equal(t, feicai.ProviderModelSeedance20Mini720P, upstream["model"])
	assert.Equal(t, "21:9", upstream["ratio"])
	assert.NotContains(t, upstream, "size")
}
