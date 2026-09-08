package relay

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

func recordLinkTaskErrorContext(c *gin.Context, info *relaycommon.RelayInfo) {
	if info.TaskRelayInfo != nil && model.IsLinkVideoTaskClientProtocol(info.TaskRelayInfo.ClientProtocol) {
		common.SetContextKey(c, constant.ContextKeyTaskErrorRelayInfo, info)
	}
}
