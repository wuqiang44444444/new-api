package imagerelay

import (
	"errors"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/asyncimage"
	"github.com/QuantumNous/new-api/relay/channel/moxingimage"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

const ChannelName = "Image Relay"

type Adaptor struct {
	delegate channel.Adaptor
	protocol dto.ImageUpstreamProtocol
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
	_, err := a.adaptorForInfo(info)
	if err != nil {
		a.delegate = nil
		a.protocol = ""
		return
	}
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	delegate, err := a.adaptorForInfo(info)
	if err != nil {
		return "", err
	}
	return delegate.GetRequestURL(info)
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, header *http.Header, info *relaycommon.RelayInfo) error {
	delegate, err := a.adaptorForInfo(info)
	if err != nil {
		return err
	}
	return delegate.SetupRequestHeader(c, header, info)
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	delegate, err := a.adaptorForInfo(info)
	if err != nil {
		return nil, err
	}
	return delegate.ConvertOpenAIRequest(c, info, request)
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	delegate, err := a.adaptorForInfo(info)
	if err != nil {
		return nil, err
	}
	return delegate.ConvertClaudeRequest(c, info, request)
}

func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	delegate, err := a.adaptorForInfo(info)
	if err != nil {
		return nil, err
	}
	return delegate.ConvertGeminiRequest(c, info, request)
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	if a == nil || a.delegate == nil {
		return nil, errors.New("image_upstream_protocol is required")
	}
	return a.delegate.ConvertRerankRequest(c, relayMode, request)
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	delegate, err := a.adaptorForInfo(info)
	if err != nil {
		return nil, err
	}
	return delegate.ConvertEmbeddingRequest(c, info, request)
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	delegate, err := a.adaptorForInfo(info)
	if err != nil {
		return nil, err
	}
	return delegate.ConvertAudioRequest(c, info, request)
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	delegate, err := a.adaptorForInfo(info)
	if err != nil {
		return nil, err
	}
	return delegate.ConvertImageRequest(c, info, request)
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	delegate, err := a.adaptorForInfo(info)
	if err != nil {
		return nil, err
	}
	return delegate.ConvertOpenAIResponsesRequest(c, info, request)
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	delegate, err := a.adaptorForInfo(info)
	if err != nil {
		return nil, err
	}
	return delegate.DoRequest(c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, response *http.Response, info *relaycommon.RelayInfo) (any, *types.NewAPIError) {
	delegate, err := a.adaptorForInfo(info)
	if err != nil {
		return nil, types.NewErrorWithStatusCode(
			err,
			types.ErrorCodeInvalidRequest,
			http.StatusServiceUnavailable,
			types.ErrOptionWithSkipRetry(),
		)
	}
	return delegate.DoResponse(c, response, info)
}

func (*Adaptor) GetModelList() []string {
	models := constant.ImageRelayProviderModels(dto.ImageUpstreamProtocolFunCloudAIGCV2)
	return append(models, constant.ImageRelayProviderModels(dto.ImageUpstreamProtocolMoxingImagesV1)...)
}

func (*Adaptor) GetChannelName() string { return ChannelName }

func (a *Adaptor) adaptorForInfo(info *relaycommon.RelayInfo) (channel.Adaptor, error) {
	if info == nil || info.ChannelMeta == nil {
		return nil, errors.New("image relay channel metadata is required")
	}
	protocol := info.ChannelOtherSettings.ImageUpstreamProtocol
	if a != nil && a.delegate != nil && a.protocol == protocol {
		return a.delegate, nil
	}
	var delegate channel.Adaptor
	switch protocol {
	case dto.ImageUpstreamProtocolFunCloudAIGCV2:
		delegate = &asyncimage.Adaptor{}
	case dto.ImageUpstreamProtocolMoxingImagesV1:
		delegate = &moxingimage.Adaptor{}
	default:
		return nil, errors.New("image_upstream_protocol is missing or unsupported")
	}
	delegate.Init(info)
	if a != nil {
		a.delegate = delegate
		a.protocol = protocol
	}
	return delegate, nil
}
