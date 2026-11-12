package model

import (
	"context"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

type usageReportRank struct {
	ID              int64  `gorm:"primaryKey"`
	Generation      string `gorm:"size:32;index:idx_usage_report_rank_scope,priority:1"`
	Kind            string `gorm:"size:8;index:idx_usage_report_rank_scope,priority:2"`
	URLHash         string `gorm:"size:64;index:idx_usage_report_rank_scope,priority:3"`
	Subject         string `gorm:"size:64"`
	NonTokenPayload string
	UsageReportRankItem
}

type UsageReportRankItem struct {
	Name          string               `json:"name" gorm:"size:255"`
	ChannelID     int                  `json:"channel_id,omitempty"`
	Calls         int64                `json:"calls"`
	Tokens        int64                `json:"tokens"`
	MissingTokens int64                `json:"missing_tokens"`
	Amount        *int64               `json:"amount"`
	PendingAmount int64                `json:"pending_amount"`
	NonToken      *UsageReportNonToken `json:"non_token" gorm:"-"`
}

type UsageReportRankPage struct {
	Items   []UsageReportRankItem `json:"items"`
	HasMore bool                  `json:"has_more"`
}

type UsageReportRankQuery struct {
	Kind, Metric, URLKey string
	Page, Size           int
}

func usageRankSubject(name string, channelID int) string {
	if channelID != 0 {
		return fmt.Sprint(channelID)
	}
	return hex.EncodeToString(common.Sha256Raw([]byte(name)))
}

func usageReportRankItem(name string, channel int, metrics UsageAnalyticsMetrics, upstream bool) UsageReportRankItem {
	item := UsageReportRankItem{Name: name, ChannelID: channel, Calls: metrics.TotalCalls,
		Tokens: metrics.InputTokens + metrics.OutputTokens, MissingTokens: metrics.RowsMissingTokens,
		NonToken: usageReportNonToken(metrics)}
	if upstream {
		item.Amount = metrics.ReferenceAmount
		if item.Amount == nil {
			item.Amount = metrics.KnownReferenceAmount
		}
		if metrics.ReferencePending {
			item.PendingAmount = 1
		}
	} else {
		item.Amount = &metrics.NetQuota
		item.PendingAmount = metrics.RowsMoneyPending + metrics.RowsMissingMoney
	}
	return item
}

func writeUsageReportRanks(ctx context.Context, db *gorm.DB, generation string, models map[string]*usageMetricsAcc, upstream []usageReportUpstreamLeaf) error {
	rows := make([]usageReportRank, 0, len(models))
	for name, acc := range models {
		rows = append(rows, usageReportRank{Generation: generation, Kind: "model",
			Subject:             usageRankSubject(name, 0),
			UsageReportRankItem: usageReportRankItem(name, 0, acc.finalize(), false)})
	}
	type channelScope struct {
		id  int
		url string
	}
	channels := make(map[channelScope]*usageMetricsAcc)
	names := make(map[channelScope]string)
	for _, leaf := range upstream {
		key := channelScope{leaf.Channel, leaf.URLKey}
		if channels[key] == nil {
			channels[key] = &usageMetricsAcc{}
			names[key] = strings.TrimSpace(leaf.Name)
			if names[key] == "" {
				names[key] = fmt.Sprintf("Channel #%d", leaf.Channel)
			}
		}
		channels[key].merge(leaf.Metrics.accumulator())
	}
	for key, acc := range channels {
		rows = append(rows, usageReportRank{Generation: generation, Kind: "channel",
			URLHash: providerURLGroupHash(key.url), Subject: usageRankSubject("", key.id),
			UsageReportRankItem: usageReportRankItem(names[key], key.id, acc.finalizeUpstream(), true)})
	}
	for start := 0; start < len(rows); start += 100 {
		if err := usageReportBeforeBatch(ctx); err != nil {
			return err
		}
		batch := rows[start:min(start+100, len(rows))]
		for i := range batch {
			raw, err := common.Marshal(batch[i].NonToken)
			if err != nil {
				return err
			}
			batch[i].NonTokenPayload = string(raw)
		}
		writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := db.WithContext(writeCtx).Create(batch).Error
		cancel()
		if err != nil {
			return err
		}
	}
	return nil
}

func GetPublishedUsageRanks(ctx context.Context, period UsageAnalyticsPeriod, filter UsageReportRankQuery) (UsageReportRankPage, error) {
	page := UsageReportRankPage{Items: []UsageReportRankItem{}}
	if (filter.Kind != "model" && filter.Kind != "channel") ||
		(filter.Metric != "calls" && filter.Metric != "tokens" && filter.Metric != "amount") ||
		filter.Page < 1 || filter.Page > 10000 || filter.Size < 1 || filter.Size > 100 {
		return page, common.NewMessage("Invalid pagination")
	}
	requested, _ := ctx.Value(usageReportReferencesKey{}).([]UsageReportDayReference)
	heads, err := reportDayHeads(ctx, period, requested)
	if err != nil {
		return page, err
	}
	now := common.GetTimestamp()
	ids := make([]string, 0, len(heads))
	refs := make([]UsageReportDayReference, 0, len(heads))
	legacy := false
	for _, head := range heads {
		index := usageDayIndex(period, head.Start)
		if index < 0 || period.Days[index].Future || (len(requested) == 0 && period.Days[index].End <= now && head.AsOf < period.Days[index].End+3*3600) {
			continue
		}
		ids = append(ids, head.Generation)
		refs = append(refs, UsageReportDayReference{Start: head.Start, Generation: head.Generation})
		legacy = legacy || !head.RankReady
	}
	if len(ids) == 0 {
		return page, nil
	}
	offset := (filter.Page - 1) * filter.Size
	if legacy {
		page.Items, err = readLegacyUsageRanks(WithUsageReportReferences(ctx, refs), period, filter)
		if err != nil {
			return page, err
		}
		page.Items = page.Items[min(offset, len(page.Items)):min(offset+filter.Size+1, len(page.Items))]
	} else {
		db := DB.WithContext(ctx)
		scope := db.Model(&usageReportRank{}).Where("generation IN ? AND kind = ?", ids, filter.Kind)
		if filter.URLKey != "" {
			scope = scope.Where("url_hash = ?", providerURLGroupHash(filter.URLKey))
		}
		groups := scope.Session(&gorm.Session{})
		groups = groups.Select("MIN(name) AS name, MAX(channel_id) AS channel_id, SUM(calls) AS calls, SUM(tokens) AS tokens, SUM(missing_tokens) AS missing_tokens, SUM(amount) AS amount, SUM(pending_amount) AS pending_amount, subject").Group("subject")
		query := db.Table("(?) AS ranks", groups)
		if filter.Metric == "amount" {
			query = query.Order("CASE WHEN amount IS NULL THEN 1 ELSE 0 END")
		}
		var ranked []usageReportRank
		err = query.Order(filter.Metric + " DESC").Order("subject ASC").Offset(offset).Limit(filter.Size + 1).Scan(&ranked).Error
		if err != nil {
			return page, err
		}
		page.HasMore = len(ranked) > filter.Size
		ranked = ranked[:min(len(ranked), filter.Size)]
		if err := attachUsageRankNonToken(ctx, scope, period, filter, refs, ranked); err != nil {
			return page, err
		}
		for _, row := range ranked {
			page.Items = append(page.Items, row.UsageReportRankItem)
		}
	}
	if _, err := reportDayHeads(ctx, period, refs); err != nil {
		return page, err
	}
	page.HasMore = page.HasMore || len(page.Items) > filter.Size
	page.Items = page.Items[:min(len(page.Items), filter.Size)]
	return page, nil
}

// Legacy generations carry the same facts in frozen payloads.
func readLegacyUsageRanks(ctx context.Context, period UsageAnalyticsPeriod, filter UsageReportRankQuery) ([]UsageReportRankItem, error) {
	rows := []UsageReportRankItem{}
	if filter.Kind == "model" {
		models, err := GetPublishedUsageModelTotals(ctx, period)
		if err != nil {
			return nil, err
		}
		for _, model := range models {
			rows = append(rows, usageReportRankItem(model.ModelName, 0, model.Total, false))
		}
	} else {
		view, _, err := GetPublishedUsageUpstream(ctx, period, UsageUpstreamScope{URLKey: filter.URLKey})
		if err != nil {
			return nil, err
		}
		for _, channel := range view.Channels {
			rows = append(rows, usageReportRankItem(channel.ChannelName, channel.ChannelId, channel.Total, true))
		}
	}
	value := func(row UsageReportRankItem) *int64 {
		switch filter.Metric {
		case "calls":
			return &row.Calls
		case "tokens":
			return &row.Tokens
		default:
			return row.Amount
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		a, b := value(rows[i]), value(rows[j])
		if a == nil || b == nil {
			if a != b {
				return a != nil
			}
		} else if *a != *b {
			return *a > *b
		}
		if filter.Kind == "channel" {
			return fmt.Sprint(rows[i].ChannelID) < fmt.Sprint(rows[j].ChannelID)
		}
		return strings.Compare(string(common.Sha256Raw([]byte(rows[i].Name))), string(common.Sha256Raw([]byte(rows[j].Name)))) < 0
	})
	return rows, nil
}
