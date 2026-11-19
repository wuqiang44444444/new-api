package controller

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

func respondLogCursorPage(c *gin.Context, admin bool) bool {
	return respondUsageLogPage(c, admin, "")
}

func respondUsageLogPage(c *gin.Context, admin bool, mode model.UsageLogTotalMode) bool {
	cursor := c.Query("page_cursor")
	ref := c.Query("log_ref")
	snapshot := c.Query("page_snapshot")
	if cursor == "" && ref == "" && snapshot == "" {
		return false
	}
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		if ref != "" {
			common.ApiErrorT(c, "Invalid log reference")
			return true
		}
		return false
	}
	userID, maxRange, ok := logReadScope(c, admin)
	if !ok {
		return true
	}
	role := c.GetInt("role")
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	if ref != "" {
		log, err := model.GetLogCursorDetail(ctx, userID, ref)
		if err != nil {
			common.ApiError(c, err)
			return true
		}
		if admin {
			if role < common.RoleRootUser {
				model.FormatAdminLogs([]*model.Log{log})
			} else {
				model.FormatRootLogs([]*model.Log{log})
			}
		}
		service.AttachLogsBillingDisplay([]*model.Log{log})
		common.ApiSuccess(c, log)
		return true
	}
	page := common.GetPageQuery(c)
	filter := logFilterFromQuery(c, admin)
	if snapshot != "" {
		data, err := model.GetUsageLogPage(ctx, userID, filter, snapshot, page.GetPage(), page.GetPageSize(), maxRange, role, mode)
		if errors.Is(err, model.ErrUsageLogSnapshotExpired) {
			common.ApiErrorStatus(c, http.StatusOK, common.NewMessage("Invalid log pagination"), gin.H{"code": "log_snapshot_expired"})
		} else if err != nil {
			common.ApiError(c, err)
		} else {
			common.ApiSuccess(c, data)
		}
		return true
	}
	data, err := model.GetLogCursorPage(ctx, userID, filter, cursor, page.GetPage(), page.GetPageSize(), maxRange, role)
	if err != nil {
		common.ApiError(c, err)
		return true
	}
	common.ApiSuccess(c, data)
	return true
}

func logReadScope(c *gin.Context, admin bool) (int, int64, bool) {
	if admin {
		return 0, 0, true
	}
	userID := c.GetInt("id")
	if userID <= 0 {
		common.ApiErrorMsg(c, "Invalid user")
		return 0, 0, false
	}
	if c.GetInt("role") < common.RoleAdminUser {
		return userID, operation_setting.GetLogSetting().MaxRangeSeconds(), true
	}
	return userID, 0, true
}

func respondLogRangeStatistics(c *gin.Context, admin bool) bool {
	return respondUsageLogRangeStatistics(c, admin, false)
}

func respondUsageLogRangeStatistics(c *gin.Context, admin bool, resolvedRange bool) bool {
	if c.Query("range_count") != "true" {
		return false
	}
	userID, maxRange, ok := logReadScope(c, admin)
	if !ok {
		return true
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	result, err := model.GetLogRangeStatistics(ctx, userID, logFilterFromQuery(c, admin), maxRange, resolvedRange)
	if err != nil {
		common.ApiError(c, err)
		return true
	}
	common.ApiSuccess(c, result)
	return true
}
