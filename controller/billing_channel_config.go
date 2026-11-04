package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func GetAdminChannelBillingConfig(c *gin.Context) {
	var request struct {
		ChannelID   int   `form:"channel_id"`
		PeriodStart int64 `form:"period_start"`
	}
	if err := c.ShouldBindQuery(&request); err != nil || !validProviderBillingPeriod(request.PeriodStart) || request.ChannelID <= 0 {
		common.ApiErrorMsg(c, "invalid provider discount")
		return
	}
	view, err := model.GetProviderChannelBillingConfig(c.Request.Context(), request.ChannelID, request.PeriodStart)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, view)
}

func PutAdminChannelBillingName(c *gin.Context) {
	var request struct {
		ChannelID int    `json:"channel_id"`
		Name      string `json:"name"`
	}
	if err := c.ShouldBindJSON(&request); err != nil || request.ChannelID <= 0 {
		common.ApiErrorMsg(c, "invalid upstream name")
		return
	}
	if err := model.SaveProviderChannelBillingName(c.Request.Context(), request.ChannelID, request.Name, c.GetInt("id")); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}
