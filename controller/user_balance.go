package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func GetUserBalance(c *gin.Context) {
	quota, err := model.GetUserQuota(c.GetInt("id"), true)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"balance":  float64(max(quota, 0)) / common.QuotaPerUnit,
			"currency": "USD",
		},
	})
}
