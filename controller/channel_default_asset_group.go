package controller

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func GetChannelDefaultAssetGroup(c *gin.Context) {
	channel, ok := defaultAssetGroupChannel(c)
	if !ok {
		return
	}
	status, err := service.GetChannelDefaultAssetGroupStatus(channel)
	if err != nil {
		writeDefaultAssetGroupError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": status})
}

func CreateOrReuseChannelDefaultAssetGroup(c *gin.Context) {
	channel, ok := defaultAssetGroupChannel(c)
	if !ok {
		return
	}
	result, err := service.CreateOrReuseChannelDefaultAssetGroup(c.Request.Context(), channel)
	if err != nil {
		writeDefaultAssetGroupError(c, err)
		return
	}
	recordManageAudit(c, "channel.default_asset_group.create_or_reuse", map[string]interface{}{
		"id":     channel.Id,
		"result": result.Action,
	})
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": result})
}

func defaultAssetGroupChannel(c *gin.Context) (*model.Channel, bool) {
	channelID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return nil, false
	}
	channel, err := model.GetChannelById(channelID, true)
	if err != nil {
		common.ApiError(c, err)
		return nil, false
	}
	return channel, true
}

func writeDefaultAssetGroupError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrUnsupportedAssetOperation), errors.Is(err, service.ErrAssetLibraryUnsupported):
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"success": false, "message": "asset group operation is not supported by this channel", "error_code": "unsupported_asset_operation",
		})
	case errors.Is(err, service.ErrAssetLibraryUnavailable), errors.Is(err, service.ErrAssetUpstreamUnavailable):
		c.JSON(http.StatusConflict, gin.H{
			"success": false, "message": "asset channel configuration is unavailable", "error_code": "asset_channel_unavailable",
		})
	case errors.Is(err, service.ErrAssetUpstreamError):
		c.JSON(http.StatusBadGateway, gin.H{
			"success": false, "message": "asset upstream operation failed", "error_code": "asset_upstream_error",
		})
	default:
		common.ApiError(c, err)
	}
}
