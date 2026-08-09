package controller

import (
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

func GetAdminCustomerBillingStatements(c *gin.Context) {
	period, ok := parseBillingReconciliationPeriod(c)
	if !ok {
		return
	}

	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page <= 0 {
		common.ApiErrorMsg(c, "invalid page")
		return
	}
	pageSize, err := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if err != nil || (pageSize != 10 && pageSize != 20 && pageSize != 50 && pageSize != 100) {
		common.ApiErrorMsg(c, "invalid page_size")
		return
	}

	search := strings.TrimSpace(c.Query("search"))
	if len(search) > 100 {
		common.ApiErrorMsg(c, "invalid search")
		return
	}
	qualityStatus := strings.TrimSpace(c.Query("quality_status"))
	if qualityStatus != "" && qualityStatus != "complete" && qualityStatus != "partial" {
		common.ApiErrorMsg(c, "invalid quality_status")
		return
	}
	sortBy := strings.TrimSpace(c.DefaultQuery("sort_by", "net_quota"))
	if sortBy != "net_quota" && sortBy != "requests" && sortBy != "original_quota" && sortBy != "username" {
		common.ApiErrorMsg(c, "invalid sort_by")
		return
	}
	sortOrder := strings.TrimSpace(c.DefaultQuery("sort_order", "desc"))
	if sortOrder != "asc" && sortOrder != "desc" {
		common.ApiErrorMsg(c, "invalid sort_order")
		return
	}

	result, err := model.GetBillingCustomerStatementList(
		period.StartTimestamp,
		period.EndTimestamp,
		search,
		qualityStatus,
		sortBy,
		sortOrder,
		page,
		pageSize,
	)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	respondBillingReconciliation(c, period, gin.H{
		"search": search, "quality_status": qualityStatus,
		"sort_by": sortBy, "sort_order": sortOrder,
		"page": page, "page_size": pageSize,
	}, result, "main_database+log_database")
}
