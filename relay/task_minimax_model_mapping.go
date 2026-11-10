package relay

import (
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	taskminimax "github.com/QuantumNous/new-api/relay/channel/task/minimax"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/gin-gonic/gin"
)

func resolveTaskModelMapping(c *gin.Context, info *relaycommon.RelayInfo, platform constant.TaskPlatform) error {
	if platform != taskminimax.Platform {
		return helper.ModelMappedHelper(c, info, nil)
	}
	upstreamModel, err := model.ResolveMiniMaxChannelModelMapping(info.OriginModelName, c.GetString("model_mapping"))
	if err != nil {
		return err
	}
	info.UpstreamModelName = upstreamModel
	info.IsModelMapped = upstreamModel != info.OriginModelName
	return nil
}
