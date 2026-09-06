package controller

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/model_setting"
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
	if channel == nil {
		return request
	}
	if channel.Type != constant.ChannelTypeAsyncImage &&
		channel.Type != constant.ChannelTypeGemini &&
		channel.Type != constant.ChannelTypeVertexAi {
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
	if channel.Type == constant.ChannelTypeGemini || channel.Type == constant.ChannelTypeVertexAi {
		// gemini_image 族测试默认 1024x1024（含明确 imageConfig 档位）。
		return request
	}
	size, supported := constant.ImageRelayTestSize(channel.GetOtherSettings().ImageUpstreamProtocol, providerModel)
	if supported {
		request.Size = size
	}
	return request
}

// channelTestUsesGeminiImageContract reports whether the customer model maps
// onto an imagine-registered provider model for the admin channel test.
func channelTestUsesGeminiImageContract(channel *model.Channel, customerModel string) bool {
	if channel == nil {
		return false
	}
	providerModel := customerModel
	if mapping, err := decodeChannelModelMapping(channel); err == nil {
		if resolved, _, err := model.ResolveModelMapping(customerModel, mapping); err == nil {
			providerModel = resolved
		}
	}
	return model_setting.IsGeminiModelSupportImagine(providerModel)
}

func decodeChannelModelMapping(channel *model.Channel) (map[string]string, error) {
	mapping := make(map[string]string)
	modelMapping := strings.TrimSpace(channel.GetModelMapping())
	if modelMapping == "" || modelMapping == "{}" {
		return mapping, nil
	}
	if err := common.UnmarshalJsonStr(modelMapping, &mapping); err != nil {
		return nil, err
	}
	return mapping, nil
}
