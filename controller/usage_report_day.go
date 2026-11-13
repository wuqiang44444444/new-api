package controller

import (
	"context"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func respondPublishedUsage(c *gin.Context, period model.UsageAnalyticsPeriod, scope string, userID int, search string) {
	if !model.DailyReportSourceSupported() {
		common.ApiErrorMsg(c, "Billing report source is unsupported")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 4*time.Second)
	defer cancel()
	if raw := c.Query("report_days"); raw != "" {
		var refs []model.UsageReportDayReference
		if len(raw) > max(1024, len(period.Days)*96) || common.UnmarshalJsonStr(raw, &refs) != nil || len(refs) == 0 || len(refs) > len(period.Days) {
			common.ApiErrorMsg(c, "Invalid usage report versions")
			return
		}
		ctx = model.WithUsageReportReferences(ctx, refs)
	}
	if scope == "customer-models" {
		if c.Query("rank_metric") != "" {
			respondPublishedUsageRanking(c, ctx, period, "model", "")
			return
		}
		models, err := model.GetPublishedUsageModelTotals(ctx, period)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		common.ApiSuccess(c, models)
		return
	}
	var result any
	var meta model.PublishedUsageReport
	var err error
	filters := gin.H{"scope": scope}
	switch scope {
	case "upstream":
		upstream := model.UsageUpstreamScope{URLKey: c.Query("url_key")}
		if raw, present := c.GetQuery("channel_id"); present {
			id, parseErr := strconv.Atoi(raw)
			if parseErr != nil {
				common.ApiErrorT(c, "Invalid upstream usage scope")
				return
			}
			upstream.ChannelID = &id
		}
		if !upstream.Valid() {
			common.ApiErrorT(c, "Invalid upstream usage scope")
			return
		}
		if c.Query("rank_metric") != "" {
			if upstream.ChannelID != nil {
				common.ApiErrorT(c, "Invalid upstream usage scope")
				return
			}
			respondPublishedUsageRanking(c, ctx, period, "channel", upstream.URLKey)
			return
		}
		filters["url_key"], filters["channel_id"] = upstream.URLKey, upstream.ChannelID
		var view model.UsageUpstreamView
		view, meta, err = model.GetPublishedUsageUpstream(ctx, period, upstream)
		if c.Query("compact") == "true" {
			for i := range view.UrlGroups {
				view.UrlGroups[i].Models = nil
				view.UrlGroups[i].Days = nil
			}
		}
		result = view
	case "customers":
		filters["search"] = search
		var overview model.UsageCustomersOverview
		overview, meta, err = model.GetPublishedUsageOverview(ctx, period, search)
		if c.Query("compact") == "true" {
			if c.Query("include_days") != "true" {
				overview.DayTotals = nil
			}
			for i := range overview.Customers {
				overview.Customers[i].Days = nil
			}
		}
		result = overview
	default:
		if userID <= 0 {
			common.ApiErrorMsg(c, "Invalid user")
			return
		}
		if scope == "customer" {
			filters["user_id"] = userID
			result, meta, err = model.GetPublishedAdminUsageCustomer(ctx, period, userID)
		} else {
			result, meta, err = model.GetPublishedUsageCustomer(ctx, period, userID, "")
		}
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"period": period, "filters": filters, "result": result, "generated_at": meta.AsOf,
		"data_source": "published_daily_usage", "published_days": meta.PublishedDays, "incomplete_days": meta.IncompleteDays, "pending_refunds": meta.PendingRefunds, "report_days": meta.References,
		"rebuilding_days": meta.RebuildingDays, "source_unavailable_days": meta.SourceUnavailableDays,
		"coverage_note": usageAnalyticsCoverageNote(),
	})
}

func respondPublishedUsageRanking(c *gin.Context, ctx context.Context, period model.UsageAnalyticsPeriod, kind, url string) {
	page, err := strconv.Atoi(c.DefaultQuery("rank_page", "1"))
	if err != nil {
		common.ApiErrorMsg(c, "Invalid pagination")
		return
	}
	size, err := strconv.Atoi(c.DefaultQuery("rank_page_size", "10"))
	if err != nil {
		common.ApiErrorMsg(c, "Invalid pagination")
		return
	}
	result, err := model.GetPublishedUsageRanks(ctx, period, model.UsageReportRankQuery{
		Kind: kind, Metric: c.Query("rank_metric"), URLKey: url, Page: page, Size: size,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}
