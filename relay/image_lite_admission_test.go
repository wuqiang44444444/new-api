package relay

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLiteAsyncAdmissionUsesMappedSizePolicy(t *testing.T) {
	for _, apiType := range []int{constant.APITypeGemini, constant.APITypeVertexAi} {
		info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesGenerations,
			ChannelMeta: &relaycommon.ChannelMeta{ApiType: apiType, UpstreamModelName: "gemini-3.1-flash-lite-image"}}
		for _, size := range []string{"auto", "1024x1024", "1920x1080", "3840x2160"} {
			err := validateImageAsyncFamilyContract(nil, info, &dto.ImageRequest{Model: "customer-alias", Prompt: "cup", Size: size, ResponseFormat: "url"})
			if size == "auto" || size == "1024x1024" {
				require.Nil(t, err)
			} else {
				require.NotNil(t, err)
				assert.Equal(t, 400, err.StatusCode)
			}
		}
	}
}
