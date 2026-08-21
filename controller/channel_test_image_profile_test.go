package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
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
		{name: "Nano Banana 2 Lite", model: "nano-banana-2-lite", size: ""},
		{name: "Seedream 4.5", model: "doubao-seedream-4-5-251128", size: "2048x2048"},
		{name: "Seedream 5.0", model: "seedream-5-0-260128", size: "2K"},
		{name: "Moxing Seedream public SKU", model: "seedream-5-moxing", size: "2K"},
		{name: "Qihang Seedream public SKU", model: "seedream-5-qihang", size: "2K"},
		{name: "Seedream 5 Lite", model: "seedream-5.0-lite", size: "2K"},
		{name: "Seedream 5 Pro", model: "seedream-5.0-pro", size: "1K"},
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

func TestNormalizeChannelTestEndpointUsesAsyncImageEndpoint(t *testing.T) {
	channel := &model.Channel{Type: constant.ChannelTypeAsyncImage}
	assert.Equal(t, string(constant.EndpointTypeImageGeneration), normalizeChannelTestEndpoint(channel, "nano-banana-2", ""))
}

func TestBuildChannelTestImageRequestForMoxingUsesMappedProviderProfile(t *testing.T) {
	channel := &model.Channel{
		Type:   constant.ChannelTypeMoxingImage,
		Models: "lite-customer-model,pro-customer-model",
		ModelMapping: common.GetPointer(`{
			"lite-customer-model":"doubao-seedream-5-0-260128",
			"pro-customer-model":"doubao-seedream-5-0-pro-260628"
		}`),
	}
	tests := []struct {
		name          string
		customerModel string
		wantSize      string
	}{
		{name: "lite", customerModel: "lite-customer-model", wantSize: constant.MoxingImageSeedream5LiteSize},
		{name: "pro", customerModel: "pro-customer-model", wantSize: constant.MoxingImageSeedream5ProSize},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := buildChannelTestImageRequestForChannel(channel, test.customerModel)

			require.NotNil(t, request.N)
			assert.Equal(t, uint(1), *request.N)
			assert.Equal(t, test.wantSize, request.Size)
		})
	}
}

func TestNormalizeChannelTestEndpointUsesMoxingImageEndpoint(t *testing.T) {
	channel := &model.Channel{Type: constant.ChannelTypeMoxingImage}
	assert.Equal(t, string(constant.EndpointTypeImageGeneration), normalizeChannelTestEndpoint(channel, "customer-image-model", ""))
}
