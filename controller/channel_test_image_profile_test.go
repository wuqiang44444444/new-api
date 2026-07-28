package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildChannelTestImageRequestUsesModelCompatibleSize(t *testing.T) {
	tests := []struct {
		name  string
		model string
		size  string
	}{
		{name: "Gemini usage price", model: "gemini-3.1-flash-image-preview-usage", size: "1K"},
		{name: "Nano Banana 2 public SKU", model: "nano-banana-2", size: "1K"},
		{name: "Seedream 4.5", model: "doubao-seedream-4-5-251128", size: "2048x2048"},
		{name: "Seedream 5.0", model: "seedream-5-0-260128", size: "2K"},
		{name: "Moxing Seedream public SKU", model: "seedream-5-moxing", size: "2K"},
		{name: "Qihang Seedream public SKU", model: "seedream-5-qihang", size: "2K"},
		{name: "Default", model: "other-image-model", size: "1024x1024"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := buildChannelTestImageRequest(tt.model)

			require.NotNil(t, request.N)
			assert.Equal(t, uint(1), *request.N)
			assert.Equal(t, tt.size, request.Size)
		})
	}
}

func TestNormalizeChannelTestEndpointUsesAdvancedCustomModelRoute(t *testing.T) {
	channel := &model.Channel{Type: constant.ChannelTypeAdvancedCustom}
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		AdvancedCustom: &dto.AdvancedCustomConfig{
			Routes: []dto.AdvancedCustomRoute{
				{
					IncomingPath: "/v1/images/generations",
					UpstreamPath: "/v1/images/generations",
					Converter:    dto.AdvancedCustomConverterMediaTaskImageBlocking,
					Models:       []string{"seedream-5-moxing"},
				},
			},
		},
	})

	assert.Equal(
		t,
		string(constant.EndpointTypeImageGeneration),
		normalizeChannelTestEndpoint(channel, "seedream-5-moxing", ""),
	)
}

func TestNormalizeChannelTestEndpointDoesNotGuessAmbiguousAdvancedCustomRoute(t *testing.T) {
	channel := &model.Channel{Type: constant.ChannelTypeAdvancedCustom}
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		AdvancedCustom: &dto.AdvancedCustomConfig{
			Routes: []dto.AdvancedCustomRoute{
				{
					IncomingPath: "/v1/images/generations",
					UpstreamPath: "/v1/images/generations",
					Converter:    dto.AdvancedCustomConverterMediaTaskImageBlocking,
					Models:       []string{"shared-model"},
				},
				{
					IncomingPath: "/v1/chat/completions",
					UpstreamPath: "/v1/chat/completions",
					Converter:    "none",
					Models:       []string{"shared-model"},
				},
			},
		},
	})

	assert.Empty(t, normalizeChannelTestEndpoint(channel, "shared-model", ""))
}
