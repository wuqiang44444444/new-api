package controller

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// Cross-customer exports have a separate admin entry. Customer endpoints retain
// their single-customer request contract and cannot select this job type.
func CreateAdminUpstreamExport(c *gin.Context) {
	var request struct {
		customerExportHttpRequest
		EvidenceFilter        string `json:"evidence_filter"`
		URLKey                string `json:"url_key"`
		ProviderModelFallback *bool  `json:"provider_model_fallback"`
		SourceJobID           string `json:"source_job_id"`
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
	request.EvidenceFilter = strings.TrimSpace(request.EvidenceFilter)
	if err := model.ValidateUpstreamEvidenceFilter(request.EvidenceFilter); err != nil {
		common.ApiError(c, err)
		return
	}
	request.URLKey = strings.TrimSpace(request.URLKey)
	if len(request.URLKey) > maxBillingURLKeyFilterLength {
		common.ApiErrorMsg(c, "invalid url_key")
		return
	}
	if request.JobType == model.CustomerExportJobTypeUpstreamSummary && (request.URLKey == "" || request.ChannelId != nil) {
		common.ApiErrorMsg(c, "upstream summary requires one URL group")
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
	if len(ids) == 0 && (request.URLKey != "" || request.ChannelId != nil || request.EvidenceFilter == "") {
		common.ApiErrorMsg(c, "upstream details require a URL or channel filter")
		return
	}
	job, err := service.SubmitUpstreamExportJob(c.Request.Context(), c.GetInt("id"), service.CustomerExportRequest{JobType: request.JobType, ChannelId: request.ChannelId, UpstreamEvidenceFilter: request.EvidenceFilter, StartTimestamp: request.StartTimestamp, EndTimestamp: request.EndTimestamp, RequestId: strings.TrimSpace(request.RequestId), UpstreamRequestId: strings.TrimSpace(request.UpstreamRequestId), ModelName: request.ModelName, ProviderModelFallback: request.ProviderModelFallback, BillingMode: request.BillingMode, Language: request.Language}, ids, request.URLKey)
	if err != nil {
		respondCustomerExportError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"success": true, "message": "", "data": job.ToView()})
}
