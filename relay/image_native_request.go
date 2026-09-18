package relay

import (
	"bytes"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
)

type nativeImageConnection struct {
	URL      string              `json:"url"`
	Headers  http.Header         `json:"headers"`
	Settings dto.ChannelSettings `json:"settings"`
}

// Freeze the same body/header precedence as ImageHelper + DoApi/FormRequest.
// Keeping this preparation local avoids changing the native adapter interface.
func freezeNativeImageRequest(c *gin.Context, info *relaycommon.RelayInfo, taskID string, request *dto.ImageRequest) (_ *model.TaskNativeImageRequest, err error) {
	stage := "request_body"
	defer func() {
		if err != nil {
			// The logger carries the request ID. Never log the wrapped error:
			// adapter, override and storage errors may contain customer secrets.
			logger.LogWarn(c, "native image preparation failed: stage=%s", stage)
		}
	}()
	adaptor := GetAdaptor(info.ApiType)
	adaptor.Init(info)
	var body io.Reader
	if model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled {
		storage, err := common.GetBodyStorage(c)
		if err != nil {
			return nil, err
		}
		body = common.NewReplayableBodyReader(storage)
	} else {
		stage = "conversion"
		converted, err := adaptor.ConvertImageRequest(c, info, *request)
		if err != nil {
			return nil, err
		}
		stage = "input_preparation"
		if err := service.PrepareImageUpstreamRequest(c.Request.Context(), converted); err != nil {
			return nil, err
		}
		relaycommon.AppendRequestConversionFromRequest(info, converted)
		if buffer, ok := converted.(*bytes.Buffer); ok {
			body = buffer
		} else {
			stage = "request_encoding"
			encoded, err := common.Marshal(converted)
			if err != nil {
				return nil, err
			}
			if len(info.ParamOverride) > 0 {
				stage = "parameter_override"
				encoded, err = relaycommon.ApplyParamOverrideWithRelayInfo(encoded, info)
				if err != nil {
					return nil, err
				}
			}
			body = bytes.NewReader(encoded)
		}
	}
	stage = "response_format"
	formattedBody, formatCloser, err := prepareNativeImageFormatBody(c, info, body)
	if err != nil {
		return nil, err
	}
	if formatCloser != nil {
		defer formatCloser.Close()
	}
	body = formattedBody

	stage = "request_url"
	url, err := adaptor.GetRequestURL(info)
	if err != nil {
		return nil, err
	}
	connection := nativeImageConnection{URL: url, Headers: make(http.Header), Settings: info.ChannelSetting}
	stage = "authentication_headers"
	if err := adaptor.SetupRequestHeader(c, &connection.Headers, info); err != nil {
		return nil, err
	}
	stage = "header_override"
	overrides, err := channel.ResolveHeaderOverride(info, c)
	if err != nil {
		return nil, err
	}
	for key, value := range overrides {
		connection.Headers.Set(key, value)
	}
	stage = "connection_encryption"
	encoded, err := common.Marshal(connection)
	if err != nil {
		return nil, err
	}
	ciphertext, err := common.EncryptShortLivedSecretForScope("image-native:"+taskID, string(encoded))
	if err != nil {
		return nil, err
	}
	stage = "request_storage"
	refs, err := service.StoreImageTaskPayload(c.Request.Context(), taskID, "request", body)
	if err != nil {
		return nil, err
	}
	return &model.TaskNativeImageRequest{ConnectionCiphertext: ciphertext, Body: refs}, nil
}
