package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

type customerContractRatioImpactRequest struct {
	GroupRatio      string `json:"group_ratio"`
	GroupGroupRatio string `json:"group_group_ratio"`
}

func PreviewCustomerContractRatioImpact(c *gin.Context) {
	var request customerContractRatioImpactRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request body"})
		return
	}
	impact, err := service.PreviewCustomerContractRatioImpact(request.GroupRatio, request.GroupGroupRatio)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	common.ApiSuccess(c, impact)
}
