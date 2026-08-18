package advancedcustom

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/stretchr/testify/require"
)

func TestNoneConverterPreservesOpenAIImageRequest(t *testing.T) {
	request := dto.ImageRequest{Model: "gpt-image-2", Prompt: "a cat"}
	info := advancedCustomRelayInfo(&dto.AdvancedCustomConfig{Routes: []dto.AdvancedCustomRoute{{
		IncomingPath: "/v1/images/generations",
		UpstreamPath: "/v1/images/generations",
		Converter:    relayconvert.ConverterNone,
		Models:       []string{"gpt-image-2"},
	}}})
	info.RelayMode = relayconstant.RelayModeImagesGenerations
	info.RequestURLPath = "/v1/images/generations"
	info.OriginModelName = "gpt-image-2"
	info.UpstreamModelName = "gpt-image-2"

	converted, err := (&Adaptor{}).ConvertImageRequest(advancedCustomGinContext("/v1/images/generations"), info, request)
	require.NoError(t, err)
	require.Equal(t, request, converted)
}
