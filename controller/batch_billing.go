package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// Dashboard sessions may inspect their own apps; API-key resource operations
// retain the stricter user+app scope in the dedicated /v1 routes.
func BatchBillingDetails(c *gin.Context) {
	job, err := model.GetDueBatchJobById(c.Param("id"))
	if err != nil || (job.UserId != c.GetInt("id") && c.GetInt("role") < common.RoleAdminUser) {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Batch job not found"})
		return
	}
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if offset < 0 {
		offset = 0
	}
	lines, err := model.GetBatchBillingPage(job, offset, 101)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "Batch billing is unavailable"})
		return
	}
	hasMore := len(lines) > 100
	if hasMore {
		lines = lines[:100]
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"id": job.Id, "status": job.PublicStatus, "delivery_state": job.DeliveryState, "settle_state": job.SettleState,
		"request_count": job.LineCount, "completed": job.CountCompleted, "failed": job.CountFailed,
		"estimated_quota": job.EstimateQuota, "target_quota": job.TargetQuota, "lines": lines, "offset": offset, "has_more": hasMore,
	}})
}
