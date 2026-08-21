package seedance

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

func (a *TaskAdaptor) applyCMCCRequestHeaders(c *gin.Context, request *http.Request) error {
	if a.protocol != dto.VideoUpstreamProtocolModelArkV3CMCC {
		return nil
	}
	payload, typed, err := a.modelArkContractPayload(c)
	if err != nil {
		return err
	}
	if !typed {
		return relaycommon.NewVideoContractError("invalid_video_contract", "CMCC Seedance requires the ModelArk V3 request contract")
	}
	for _, item := range payload.Content {
		if item.Type == "video_url" && item.VideoURL != nil && strings.TrimSpace(item.VideoURL.URL) != "" {
			request.Header.Set("Input-Has-Video", "true")
			break
		}
	}
	return nil
}
