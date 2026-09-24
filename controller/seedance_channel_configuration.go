package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func GetSeedanceChannelConfiguration(c *gin.Context) {
	configuration, err := model.GetSeedancePluginConfiguration()
	if err != nil {
		common.ApiErrorMsg(c, "Seedance plugin configuration is unavailable")
		return
	}
	common.ApiSuccess(c, configuration)
}

// GetMinimaxChannelConfiguration serves the active minimax-link declaration
// snapshot for the MiniMax Link channel form.
func GetMinimaxChannelConfiguration(c *gin.Context) {
	configuration, err := model.GetMinimaxPluginConfiguration()
	if err != nil {
		common.ApiErrorMsg(c, "MiniMax plugin configuration is unavailable")
		return
	}
	common.ApiSuccess(c, configuration)
}
