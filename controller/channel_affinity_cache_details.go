package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func GetChannelAffinityCacheDetails(c *gin.Context) {
	var cursor uint64
	if raw := c.Query("cursor"); raw != "" {
		var err error
		cursor, err = strconv.ParseUint(raw, 10, 64)
		if err != nil {
			common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Invalid parameters"))
			return
		}
	}
	page, err := service.GetChannelAffinityCacheDetails(c.Request.Context(), c.Query("layer"), cursor)
	if err != nil {
		if message, ok := err.(*common.Message); ok {
			common.ApiErrorStatus(c, http.StatusBadRequest, message)
			return
		}
		common.ApiErrorStatus(c, http.StatusServiceUnavailable, common.NewMessage("Operation failed"))
		return
	}
	common.ApiSuccess(c, page)
}
