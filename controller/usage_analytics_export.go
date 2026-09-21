package controller

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

type usageAnalyticsExportBody struct {
	SourceJobID string `json:"source_job_id"`
	Period      string `json:"period"`
	Date        string `json:"date"`
	View        string `json:"view"`
	UserId      *int   `json:"user_id"`
	Language    string `json:"language"`
	Search      string `json:"search"`
}

// CreateUsageSelfExport accepts a usage summary export of the caller themself.
func CreateUsageSelfExport(c *gin.Context) {
	var body usageAnalyticsExportBody
	if err := c.ShouldBindJSON(&body); err != nil {
		common.ApiErrorMsg(c, "invalid usage export request")
		return
	}
	if body.SourceJobID != "" {
		job, err := service.ResubmitUsageAnalyticsExport(c.GetInt("id"), body.SourceJobID, true)
		if err != nil {
			respondUsageAnalyticsExportError(c, err)
			return
		}
		common.ApiSuccess(c, job.ToView())
		return
	}
	job, err := service.SubmitUsageAnalyticsExport(c.GetInt("id"), service.UsageAnalyticsExportRequest{
		Period:   body.Period,
		Date:     body.Date,
		View:     "self",
		UserId:   body.UserId,
		Language: body.Language,
		Search:   body.Search,
	})
	if err != nil {
		respondUsageAnalyticsExportError(c, err)
		return
	}
	common.ApiSuccess(c, job.ToView())
}

// CreateUsageAdminExport accepts customer/upstream usage exports. The target
// user is explicit for the customer view; upstream exports are admin-only by
// route middleware and re-checked at every read.
func CreateUsageAdminExport(c *gin.Context) {
	var body usageAnalyticsExportBody
	if err := c.ShouldBindJSON(&body); err != nil {
		common.ApiErrorMsg(c, "invalid usage export request")
		return
	}
	if body.SourceJobID != "" {
		job, err := service.ResubmitUsageAnalyticsExport(c.GetInt("id"), body.SourceJobID, false)
		if err != nil {
			respondUsageAnalyticsExportError(c, err)
			return
		}
		common.ApiSuccess(c, job.ToView())
		return
	}
	job, err := service.SubmitUsageAnalyticsExport(c.GetInt("id"), service.UsageAnalyticsExportRequest{
		Period:   body.Period,
		Date:     body.Date,
		View:     body.View,
		UserId:   body.UserId,
		Language: body.Language,
		Search:   body.Search,
	})
	if err != nil {
		respondUsageAnalyticsExportError(c, err)
		return
	}
	common.ApiSuccess(c, job.ToView())
}

func respondUsageAnalyticsExportError(c *gin.Context, err error) {
	if errors.Is(err, service.ErrCustomerExportInvalidRequest) {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	common.ApiError(c, err)
}
