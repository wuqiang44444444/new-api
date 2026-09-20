package controller

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// Cross-customer exports have a separate admin entry. Customer endpoints retain
// their single-customer request contract and cannot select this job type.
func CreateAdminUpstreamExport(c *gin.Context) {
	var request struct {
		customerExportHttpRequest
		URLKey      string `json:"url_key"`
		SourceJobID string `json:"source_job_id"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiErrorMsg(c, "invalid export request")
		return
	}
	if request.SourceJobID != "" {
		job, err := service.ResubmitUpstreamExportJob(c.GetInt("id"), request.SourceJobID)
		if err != nil {
			respondCustomerExportError(c, err)
			return
		}
		c.JSON(http.StatusAccepted, gin.H{"success": true, "data": job.ToView()})
		return
	}
	request.URLKey = strings.TrimSpace(request.URLKey)
	if len(request.URLKey) > maxBillingURLKeyFilterLength {
		common.ApiErrorMsg(c, "invalid url_key")
		return
	}
	var ids []int
	if request.ChannelId != nil && *request.ChannelId > 0 {
		ids = []int{*request.ChannelId}
	} else if request.URLKey != "" {
		var err error
		ids, err = resolveUpstreamURLChannels(request.URLKey)
		if err != nil {
			common.ApiError(c, err)
			return
		}
	}
	if len(ids) == 0 {
		common.ApiErrorMsg(c, "upstream details require a URL or channel filter")
		return
	}
	job, err := service.SubmitUpstreamExportJob(c.GetInt("id"), service.CustomerExportRequest{StartTimestamp: request.StartTimestamp, EndTimestamp: request.EndTimestamp, RequestId: strings.TrimSpace(request.RequestId), UpstreamRequestId: strings.TrimSpace(request.UpstreamRequestId), ModelName: request.ModelName, BillingMode: request.BillingMode, Language: request.Language}, ids, request.URLKey)
	if err != nil {
		respondCustomerExportError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"success": true, "message": "", "data": job.ToView()})
}
