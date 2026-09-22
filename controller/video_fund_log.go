package controller

import (
	"errors"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
)

func ListVideoFundLogs(c *gin.Context) {
	user, _ := strconv.Atoi(c.Query("user_id"))
	channel, _ := strconv.Atoi(c.Query("channel_id"))
	app, _ := strconv.Atoi(c.Query("app_id"))
	from, _ := strconv.ParseInt(c.Query("refund_from"), 10, 64)
	to, _ := strconv.ParseInt(c.Query("refund_to"), 10, 64)
	page, _ := strconv.Atoi(c.Query("p"))
	size, _ := strconv.Atoi(c.Query("page_size"))
	if page < 1 {
		page = 1
	}
	if size <= 0 || size > 100 {
		size = 20
	}
	if page > 1000000 {
		page = 1000000
	}
	filters := model.VideoFundFilters{UserID: user, AppID: app, ChannelID: channel, TaskID: c.Query("task_id"), State: c.Query("state"), RefundFrom: from, RefundTo: to, Offset: (page - 1) * size, Limit: size}
	items, total, err := model.ListVideoFundLogs(filters)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "message": "Failed to load video fund records"})
		return
	}
	summary, err := model.SummarizeVideoFunds(filters)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "message": "Failed to load video fund records"})
		return
	}
	c.JSON(200, gin.H{"success": true, "data": gin.H{"items": items, "total": total, "summary": summary}})
}

func GetVideoFundLog(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(400, gin.H{"success": false, "message": "Invalid request"})
		return
	}
	item, err := model.GetVideoFundLog(c.Param("kind"), id)
	if err != nil {
		c.JSON(404, gin.H{"success": false, "message": "Video fund record not found"})
		return
	}
	timeline, err := model.VideoFundTimeline(c.Param("kind"), id)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "message": "Failed to load video fund records"})
		return
	}
	c.JSON(200, gin.H{"success": true, "data": item, "timeline": timeline})
}

func RefundVideoFunds(c *gin.Context) {
	var req service.VideoRefundContext
	if c.ShouldBindJSON(&req) != nil || req.Kind != c.Param("kind") || strconv.FormatInt(req.ID, 10) != c.Param("id") {
		c.JSON(400, gin.H{"success": false, "message": "Invalid request"})
		return
	}
	context, err := common.Marshal(req)
	if err != nil {
		c.JSON(400, gin.H{"success": false, "message": "Invalid request"})
		return
	}
	if middleware.RequireSecurityProof(c, service.VerificationOperation{Scope: service.VerificationScopeVideoRefund, Context: context}) == nil {
		return
	}
	if err := model.RequestVideoRefund(req.Kind, req.ID, req.Version, c.GetInt("id"), req.Note); err != nil {
		status, message := http.StatusServiceUnavailable, "Refund request could not be saved; please retry"
		if errors.Is(err, model.ErrVideoRefundConflict) {
			status, message = http.StatusConflict, "Video funding changed; refresh the refund preview"
		}
		if errors.Is(err, model.ErrTaskCreateAttemptFundBlocked) {
			status, message = http.StatusConflict, "Funding evidence is required before refund"
		}
		c.JSON(status, gin.H{"success": false, "message": message})
		return
	}
	// Acceptance is durable. Processing errors remain visible as pending, never a
	// misleading failed request that encourages a second financial instruction.
	_ = service.ProcessVideoRefund(c.Request.Context(), req.Kind, req.ID)
	recordManageAudit(c, "video.funds.refund", map[string]interface{}{"kind": req.Kind, "id": req.ID})
	item, err := model.GetVideoFundLog(req.Kind, req.ID)
	if err != nil {
		c.JSON(202, gin.H{"success": true, "data": gin.H{"fund_state": "pending"}})
		return
	}
	c.JSON(200, gin.H{"success": true, "data": item})
}
