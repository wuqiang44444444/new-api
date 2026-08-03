package controller

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func TestChannelAssetAction(c *gin.Context) {
	testChannelConnectivity(c, service.CheckAssetChannelConnectivity)
}

func TestChannelVideoAPI(c *gin.Context) {
	testChannelConnectivity(c, service.CheckVideoChannelConnectivity)
}

func testChannelConnectivity(c *gin.Context, check func(context.Context, *model.Channel) error) {
	channelID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	channel, err := model.GetChannelById(channelID, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	startedAt := time.Now()
	if err := check(c.Request.Context(), channel); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success":    false,
			"message":    err.Error(),
			"error_code": service.ChannelConnectivityErrorCode(err),
			"time":       time.Since(startedAt).Seconds(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"time":    time.Since(startedAt).Seconds(),
	})
}
