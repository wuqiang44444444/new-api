package model

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

type billingReportCustomerLine struct {
	Export        CustomerExportRow
	UseTime       int
	IsStream      bool
	RecordedInput int64
}

// All consumers of a frozen customer line use the same price evidence.
func (row billingReportLine) customerDetail() (billingReportCustomerLine, error) {
	var detail billingReportCustomerLine
	if err := common.UnmarshalJsonStr(string(row.Customer), &detail); err != nil {
		return detail, err
	}
	export := &detail.Export
	if export.RecordedOriginal == "" && export.CountsTowardBill {
		// Never use the old estimate. An explicit unavailable result must stay unavailable.
		blocked := export.QualityStatus == "unrecorded" || export.HasAuxiliaryCharge ||
			slices.Contains(export.EstimateReasons, BillingEstimateInvalidFacts)
		if quota, ok := bookedStatementOriginalQuota(export.Quota, export.EventType == "refund", export.GroupRatio,
			export.ContractApplicable, export.ContractRatio, blocked); ok {
			export.RecordedOriginal = strconv.FormatInt(quota, 10)
		}
	}
	export.OriginalEstimate = export.RecordedOriginal
	export.OriginalExact = export.RecordedOriginal != ""
	if export.OriginalExact {
		export.QualityStatus = "exact"
		export.EstimateReasons = nil
	} else {
		export.QualityStatus = "unrecorded"
	}
	return detail, nil
}

func (report *BillingReportManifest) customerLines(ctx context.Context, filter BillingStatementLogFilter) *gorm.DB {
	query := report.lines(ctx).Where("user_id = ? AND fund_group <> ? AND created_at BETWEEN ? AND ?", filter.UserId, "", filter.Start, filter.End)
	if filter.TokenId != nil {
		query = query.Where("token_id = ?", *filter.TokenId)
	}
	if filter.ChannelId != nil {
		query = query.Where("channel_id = ?", *filter.ChannelId)
	}
	if filter.LogType != 0 {
		query = query.Where("log_type = ?", filter.LogType)
	}
	return billingReportLineFilters(query, map[string]string{
		"token_name": filter.TokenName, "username": filter.Username, "group_name": filter.Group, "request_id": filter.RequestId,
		"upstream_request_id": filter.UpstreamRequestId, "customer_model": filter.ModelName, "billing_mode": filter.BillingMode,
		"fund_group": filter.FundGroup,
	})
}

func billingReportLineFilters(query *gorm.DB, filters map[string]string) *gorm.DB {
	for key, value := range filters {
		if value == "" {
			continue
		}
		// The original parser compares these identities byte-for-byte. MySQL's
		// default collation is case-insensitive, including URL path segments.
		if common.UsingMainDatabase(common.DatabaseTypeMySQL) && (key == "customer_model" || key == "provider_model" || key == "url_key") {
			query = query.Where("BINARY "+key+" = BINARY ?", value)
		} else {
			query = query.Where(map[string]any{key: value})
		}
	}
	return query
}

func (report *BillingReportManifest) customerDetails(ctx context.Context, filter BillingStatementLogFilter, result BillingStatementLogs, role int) (BillingStatementLogs, error) {
	query := report.customerLines(ctx, filter)
	var totals struct{ Quota, Requests, Tokens, RowCount int64 }
	if err := query.Session(&gorm.Session{}).Select("COALESCE(SUM(customer_charge),0) AS quota, COALESCE(SUM(billable_requests),0) AS requests, COALESCE(SUM(input_tokens),0) + COALESCE(SUM(output_tokens),0) AS tokens, COUNT(*) AS row_count").Scan(&totals).Error; err != nil {
		return result, err
	}
	result.Quota = totals.Quota
	minutes := float64(filter.End-filter.Start+1) / 60
	if minutes > 0 {
		result.RPM, result.TPM = float64(totals.Requests)/minutes, float64(totals.Tokens)/minutes
	}
	offset := (result.Page - 1) * result.PageSize
	if filter.GroupFunds {
		if err := query.Session(&gorm.Session{}).Distinct("fund_group").Count(&result.Total).Error; err != nil {
			return result, err
		}
		groups := []BillingStatementFundGroup{}
		if int64(offset) < result.Total {
			if err := query.Select("fund_group AS fund_key, MIN(customer_model) AS model_name, MAX(request_id) AS request_id, COUNT(*) AS event_count, SUM(customer_charge) AS net_quota").
				Group("fund_group").Order("MAX(source_id) DESC").Offset(offset).Limit(result.PageSize).
				Scan(&groups).Error; err != nil {
				return result, err
			}
			for i := range groups {
				groups[i].Date, _, _ = strings.Cut(groups[i].Key, ".")
			}
		}
		result.FundGroups = &groups
		return result, nil
	}
	result.Total = totals.RowCount
	var rows []billingReportLine
	if err := query.Omit("upstream", "root_task_ref").Order("source_id DESC").Offset(offset).Limit(result.PageSize).Find(&rows).Error; err != nil {
		return result, err
	}
	for _, row := range rows {
		record, err := row.customerDetail()
		if err != nil {
			return result, err
		}
		raw, err := common.Marshal(map[string]any{"billing_facts": record.Export, "cache_tokens": record.Export.CacheReadTokens, "cache_creation_tokens": record.Export.CacheWriteTokens})
		if err != nil {
			return result, err
		}
		log := &Log{Id: int(row.SourceID), UserId: row.UserID, CreatedAt: row.CreatedAt, Type: row.LogType, Username: row.Username, TokenName: row.TokenName,
			TokenId: row.TokenID, ChannelId: row.ChannelID, ModelName: row.CustomerModel, Group: record.Export.GroupName, Quota: int(record.Export.Quota),
			PromptTokens: int(record.RecordedInput), CompletionTokens: int(record.Export.OutputTokens), UseTime: record.UseTime, IsStream: record.IsStream,
			RequestId: row.RequestID, Other: string(raw)}
		if role >= common.RoleAdminUser {
			log.UpstreamRequestId = row.UpstreamRequestID
		}
		result.Items = append(result.Items, log)
	}
	if role < common.RoleAdminUser {
		formatUserLogs(result.Items, offset)
	}
	return result, nil
}

func (report *BillingReportManifest) upstreamLines(ctx context.Context, filter UpstreamBillingDetailFilter) *gorm.DB {
	query := report.lines(ctx).Where("created_at BETWEEN ? AND ?", filter.Start, filter.End)
	if len(filter.ChannelIds) > 0 {
		query = query.Where("channel_id IN ?", filter.ChannelIds)
	}
	if filter.ProviderModelFallback != nil {
		query = query.Where("model_fallback = ?", *filter.ProviderModelFallback)
	}
	if filter.EvidenceFilter != "" {
		query = query.Where("evidence_tags LIKE ? ESCAPE '!'", "%|"+strings.ReplaceAll(filter.EvidenceFilter, "_", "!_")+"|%")
	}
	return billingReportLineFilters(upstreamRequestIDCandidates(query, filter.RequestId), map[string]string{
		"provider_model": filter.ProviderModel, "billing_mode": filter.BillingMode,
		"upstream_request_id": filter.UpstreamRequestId, "url_key": filter.URLKey,
	})
}

func (report *BillingReportManifest) upstreamDetails(ctx context.Context, filter UpstreamBillingDetailFilter, result UpstreamBillingDetails, root bool) (UpstreamBillingDetails, error) {
	if err := ValidateUpstreamEvidenceFilter(filter.EvidenceFilter); err != nil {
		return result, err
	}
	if filter.RequestId != "" {
		skip := int64(result.Page-1) * int64(result.PageSize)
		err := report.scanUpstreamDetails(ctx, filter, root, BillingStatementReadPolicy{}, func(item UpstreamBillingDetailItem) error {
			if result.Total >= skip && len(result.Items) < result.PageSize {
				result.Items = append(result.Items, item)
			}
			result.Total++
			return nil
		})
		return result, err
	}
	query := report.upstreamLines(ctx, filter)
	if err := query.Session(&gorm.Session{}).Count(&result.Total).Error; err != nil {
		return result, err
	}
	var rows []billingReportLine
	if err := query.Select("user_id, upstream, original_exact, root_task_ref").Order("source_id ASC").Offset((result.Page - 1) * result.PageSize).Limit(result.PageSize).Find(&rows).Error; err != nil {
		return result, err
	}
	items, err := upstreamReportDetailItems(ctx, rows, root)
	if err != nil {
		return result, err
	}
	result.Items = items
	return result, nil
}

func upstreamReportDetailItems(ctx context.Context, rows []billingReportLine, root bool) ([]UpstreamBillingDetailItem, error) {
	items := make([]UpstreamBillingDetailItem, 0, len(rows))
	for _, row := range rows {
		item, err := row.upstreamDetail(root)
		if err != nil {
			return nil, err
		}
		item.requestUserID = row.UserID
		items = append(items, item)
	}
	if err := resolveUpstreamDetailRequestIDs(ctx, items); err != nil {
		return nil, err
	}
	return items, nil
}

func (row billingReportLine) upstreamDetail(root bool) (UpstreamBillingDetailItem, error) {
	var item UpstreamBillingDetailItem
	if err := common.UnmarshalJsonStr(string(row.Upstream), &item); err != nil {
		return item, err
	}
	item.OriginalExactQuota = row.OriginalExact
	if item.OriginalAmountEstimated == nil && item.OriginalAmount != nil && item.TestPricing == nil {
		// Older published rows only used the reverse-calculated basis.
		estimated := *item.OriginalAmount != 0
		item.OriginalAmountEstimated = &estimated
	}
	if root {
		if err := row.setUpstreamTaskID(&item); err != nil {
			return item, err
		}
	}
	return item, nil
}

func (row billingReportLine) setUpstreamTaskID(item *UpstreamBillingDetailItem) error {
	if row.RootTaskRef == "" {
		return nil
	}
	var err error
	item.UpstreamTaskId, err = common.DecryptShortLivedSecretForScope("billing-report-upstream-task", row.RootTaskRef)
	return err
}

func (report *BillingReportManifest) scanUpstreamDetails(ctx context.Context, filter UpstreamBillingDetailFilter, root bool, policy BillingStatementReadPolicy, consume func(UpstreamBillingDetailItem) error) error {
	if err := ValidateUpstreamEvidenceFilter(filter.EvidenceFilter); err != nil {
		return err
	}
	var cursor int64
	for {
		if policy.BeforeBatch != nil {
			if err := policy.BeforeBatch(ctx); err != nil {
				return err
			}
		}
		timeout := policy.BatchTimeout
		if timeout <= 0 {
			timeout = 5 * time.Second
		}
		batchCtx, cancel := context.WithTimeout(ctx, timeout)
		var rows []billingReportLine
		err := report.upstreamLines(batchCtx, filter).Select("source_id, user_id, upstream, original_exact, root_task_ref").
			Where("source_id > ?", cursor).Order("source_id ASC").Limit(500).Find(&rows).Error
		if err != nil {
			cancel()
			return err
		}
		items, err := upstreamReportDetailItems(batchCtx, rows, false)
		cancel()
		if err != nil {
			return err
		}
		for i, item := range items {
			cursor = rows[i].SourceID
			if filter.RequestId != "" && filter.RequestId != item.RequestId {
				continue
			}
			if root {
				if err := rows[i].setUpstreamTaskID(&item); err != nil {
					return err
				}
			}
			if err := consume(item); err != nil {
				return err
			}
		}
		if policy.AfterBatch != nil {
			if err := policy.AfterBatch(len(rows)); err != nil {
				return err
			}
		}
		if len(rows) < 500 {
			return nil
		}
	}
}

func (report *BillingReportManifest) customerExportBatch(ctx context.Context, userID int, filters CustomerExportFilters, cursor int64, limit int) ([]billingReportLine, error) {
	query := report.customerLines(ctx, BillingStatementLogFilter{
		UserId: userID, Start: filters.StartTimestamp, End: filters.EndTimestamp - 1, TokenId: filters.TokenId,
		ChannelId: filters.ChannelId, ModelName: filters.ModelName, BillingMode: filters.BillingMode,
		TokenName: filters.TokenName, Group: filters.Group, Username: filters.Username, RequestId: filters.RequestId,
		UpstreamRequestId: filters.UpstreamRequestId,
	})
	if len(filters.LogTypes) > 0 {
		query = query.Where("log_type IN ?", filters.LogTypes)
	}
	var rows []billingReportLine
	err := query.Select("source_id, customer").Where("source_id > ?", cursor).Order("source_id ASC").Limit(limit).Find(&rows).Error
	return rows, err
}

func ReadBillingReportCustomerExportBatch(ctx context.Context, userID int, filters CustomerExportFilters, cursor int64, limit int) ([]CustomerExportRow, int64, error) {
	rows, err := BillingReportFromContext(ctx).customerExportBatch(ctx, userID, filters, cursor, limit)
	if err != nil {
		return nil, cursor, err
	}
	result := make([]CustomerExportRow, 0, len(rows))
	for _, row := range rows {
		detail, err := row.customerDetail()
		if err != nil {
			return nil, cursor, err
		}
		result = append(result, detail.Export)
		cursor = row.SourceID
	}
	return result, cursor, nil
}
