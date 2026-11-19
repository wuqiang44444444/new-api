package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func GetAdminUsageLogConsolePage(c *gin.Context) { getUsageLogConsolePage(c, true) }
func GetSelfUsageLogConsolePage(c *gin.Context)  { getUsageLogConsolePage(c, false) }

func getUsageLogConsolePage(c *gin.Context, admin bool) {
	mode := model.UsageLogTotalMode(c.Query("page_total"))
	if c.Query("log_ref") == "" && (c.Query("page_snapshot") == "" ||
		(mode != model.UsageLogTotalDeferred && mode != model.UsageLogTotalOnly && mode != model.UsageLogLastPage)) {
		common.ApiErrorT(c, "Invalid log pagination")
		return
	}
	if !respondUsageLogPage(c, admin, mode) {
		common.ApiErrorStatus(c, http.StatusNotFound, common.NewMessage("Invalid log pagination"))
	}
}

func GetAdminUsageLogConsoleStatistics(c *gin.Context) { getUsageLogConsoleStatistics(c, true) }
func GetSelfUsageLogConsoleStatistics(c *gin.Context)  { getUsageLogConsoleStatistics(c, false) }

func getUsageLogConsoleStatistics(c *gin.Context, admin bool) {
	start, startErr := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	end, endErr := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	if c.Query("resolved_log_range") != "true" || startErr != nil || endErr != nil ||
		start < 0 || end <= 0 || !respondUsageLogRangeStatistics(c, admin, true) {
		common.ApiErrorT(c, "Invalid time range")
	}
}
