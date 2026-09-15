package model

import (
	"github.com/QuantumNous/new-api/common"
)

// BillingStatementLogFilter preserves the statement's stable customer/key
// identities. A nil TokenId means all keys; a pointer to zero means playground.
type BillingStatementLogFilter struct {
	UserId                                                   int
	Start, End                                               int64
	TokenId                                                  *int
	ChannelId                                                *int
	ModelName, BillingMode                                   string
	TokenName, Username, Group, RequestId, UpstreamRequestId string
	LogType                                                  int
}

type BillingStatementLogs struct {
	Items    []*Log  `json:"items"`
	Total    int64   `json:"total"`
	Page     int     `json:"page"`
	PageSize int     `json:"page_size"`
	Quota    int64   `json:"quota"`
	RPM      float64 `json:"rpm"`
	TPM      float64 `json:"tpm"`
}

func GetBillingStatementLogs(filter BillingStatementLogFilter, page, pageSize, role int) (BillingStatementLogs, error) {
	result := BillingStatementLogs{Items: make([]*Log, 0), Page: page, PageSize: pageSize}
	query := LOG_DB.Model(&Log{}).Scopes(customerSettlementLogs).
		Where("user_id = ? AND type IN ? AND created_at >= ? AND created_at <= ?", filter.UserId, []int{LogTypeConsume, LogTypeRefund}, filter.Start, filter.End)
	if filter.TokenId != nil {
		query = query.Where("token_id = ?", *filter.TokenId)
	}
	if filter.ChannelId != nil {
		query = query.Where("channel_id = ?", *filter.ChannelId)
	}
	for column, value := range map[string]string{
		"token_name": filter.TokenName, "username": filter.Username,
		"group": filter.Group, "request_id": filter.RequestId, "upstream_request_id": filter.UpstreamRequestId,
	} {
		if value != "" {
			query = query.Where(map[string]any{column: value})
		}
	}
	if filter.LogType != 0 {
		query = query.Where("type = ?", filter.LogType)
	}
	order := "id desc"
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		order = clickHouseLogOrder("")
	}
	rows, err := query.Order(order).Rows()
	if err != nil {
		return result, err
	}
	defer rows.Close()
	var usage BillingReconciliationUsage
	start := int64(page-1) * int64(pageSize)
	for rows.Next() {
		var log Log
		if err := LOG_DB.ScanRows(rows, &log); err != nil {
			return result, err
		}
		fact := billingReconciliationLog{UserId: log.UserId, TokenId: log.TokenId, TokenName: log.TokenName, ChannelId: log.ChannelId, ModelName: log.ModelName, Type: log.Type, CreatedAt: log.CreatedAt, PromptTokens: log.PromptTokens, CompletionTokens: log.CompletionTokens, Quota: log.Quota, Other: log.Other}
		parsed := parseBillingReconciliationLog(fact)
		if filter.ModelName != "" && parsed.customerModel != filter.ModelName {
			continue
		}
		log.ModelName = parsed.customerModel
		log.PromptTokens = int(parsed.recordedInputTokens)
		log.CompletionTokens = int(parsed.outputTokens)
		if filter.BillingMode != "" && parsed.billingMode != filter.BillingMode {
			continue
		}
		accumulateBillingReconciliationLog(&usage, fact, parsed)
		if result.Total >= start && len(result.Items) < pageSize {
			result.Items = append(result.Items, &log)
		}
		result.Total++
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	finalizeBillingReconciliationUsage(&usage)
	result.Quota = usage.NetQuota
	minutes := float64(filter.End-filter.Start+1) / 60
	if minutes > 0 {
		result.RPM = float64(usage.Requests) / minutes
		result.TPM = float64(usage.InputTokens+usage.OutputTokens) / minutes
	}
	if role < common.RoleAdminUser {
		formatUserLogs(result.Items, int(start))
	} else {
		ids := make([]int, 0, len(result.Items))
		for _, log := range result.Items {
			ids = append(ids, log.ChannelId)
		}
		var channels []struct {
			Id   int
			Name string
		}
		if len(ids) > 0 {
			if err := DB.Model(&Channel{}).Select("id, name").Where("id IN ?", ids).Find(&channels).Error; err != nil {
				return result, err
			}
		}
		names := make(map[int]string, len(channels))
		for _, channel := range channels {
			names[channel.Id] = channel.Name
		}
		for _, log := range result.Items {
			log.ChannelName = names[log.ChannelId]
		}
		if role < common.RoleRootUser {
			FormatAdminLogs(result.Items)
		} else {
			FormatRootLogs(result.Items)
		}
	}
	return result, nil
}
