package controller

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/samber/lo"
)

func buildChannelTestImageRequest(model string) *dto.ImageRequest {
	size := "1024x1024"
	switch strings.ToLower(strings.TrimSpace(model)) {
	case "nano-banana-2", "gemini-3.1-flash-image-preview-usage":
		size = "1K"
	case "nano-banana-2-lite":
		size = ""
	case "doubao-seedream-4-5-251128":
		size = "2048x2048"
	case "seedream-5", "seedream-5-moxing", "seedream-5-qihang", "seedream-5-0-260128", "seedream-5.0-lite":
		size = "2K"
	case "seedream-5.0-pro":
		size = "1K"
	}
	return &dto.ImageRequest{
		Model:  model,
		Prompt: "a cute cat",
		N:      lo.ToPtr(uint(1)),
		Size:   size,
	}
}

func buildChannelTestImageRequestForChannel(channel *model.Channel, customerModel string) *dto.ImageRequest {
	request := buildChannelTestImageRequest(customerModel)
	if channel == nil || channel.Type != constant.ChannelTypeAsyncImage {
		return request
	}
	var mapping map[string]string
	modelMapping := strings.TrimSpace(channel.GetModelMapping())
	if modelMapping != "" && modelMapping != "{}" {
		if err := common.UnmarshalJsonStr(modelMapping, &mapping); err != nil {
			return request
		}
	}
	providerModel, _, err := model.ResolveModelMapping(customerModel, mapping)
	if err != nil {
		return request
	}
	size, supported := constant.ImageRelayTestSize(channel.GetOtherSettings().ImageUpstreamProtocol, providerModel)
	if supported {
		request.Size = size
	}
	return request
}
