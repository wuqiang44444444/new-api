package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func PostBillingReportRollback(c *gin.Context) {
	var body struct {
		Date    string `json:"date"`
		Current string `json:"current_generation"`
		Restore string `json:"restore_generation"`
	}
	if c.ShouldBindJSON(&body) != nil {
		common.ApiErrorMsg(c, "Invalid billing report rollback")
		return
	}
	if err := model.RestoreBillingReportGeneration(c.Request.Context(), body.Date, body.Current, body.Restore); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "billing.report_rollback", map[string]any{"date": body.Date, "current_generation": body.Current, "restore_generation": body.Restore})
	common.ApiSuccess(c, nil)
}
