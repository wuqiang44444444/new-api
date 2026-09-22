package controller

import (
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"strconv"
)

func ListSelfVideoFundProgress(c *gin.Context) { listVideoFundProgress(c, 0) }
func ListTokenVideoFundProgress(c *gin.Context) {
	appID := c.GetInt("token_id")
	if appID <= 0 {
		c.JSON(403, gin.H{"success": false, "message": "Forbidden"})
		return
	}
	listVideoFundProgress(c, appID)
}
func listVideoFundProgress(c *gin.Context, appID int) {
	page, _ := strconv.Atoi(c.Query("p"))
	if page < 1 {
		page = 1
	}
	if page > 1000000 {
		page = 1000000
	}
	rows, total, err := model.ListVideoFundProgress(c.GetInt("id"), appID, c.Query("task_id"), (page-1)*20, 20)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "message": "Failed to load video fund records"})
		return
	}
	c.JSON(200, gin.H{"success": true, "data": gin.H{"items": rows, "total": total}})
}
