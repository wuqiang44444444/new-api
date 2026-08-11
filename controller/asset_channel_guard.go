package controller

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func rejectAssetChannelFenceError(c *gin.Context, err error) bool {
	if !errors.Is(err, model.ErrChannelHasActiveAssetResources) {
		return false
	}
	c.JSON(http.StatusConflict, gin.H{
		"success":    false,
		"message":    "channel cannot be deleted or have its account credentials changed while active asset resources exist",
		"error_code": "asset_resources_active",
	})
	return true
}

func rejectAssetChannelDeletion(c *gin.Context, channelIDs []int) bool {
	channelID, active, err := model.FirstChannelWithActiveAssetResources(channelIDs)
	if err != nil {
		common.ApiError(c, err)
		return true
	}
	if !active {
		return false
	}
	c.JSON(http.StatusConflict, gin.H{"success": false, "message": fmt.Sprintf("channel %d still owns active asset resources", channelID)})
	return true
}

func rejectDisabledAssetChannelDeletion(c *gin.Context) bool {
	ids, err := model.GetDisabledChannelIDs()
	if err != nil {
		common.ApiError(c, err)
		return true
	}
	return rejectAssetChannelDeletion(c, ids)
}
