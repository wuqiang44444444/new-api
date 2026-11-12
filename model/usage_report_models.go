package model

import (
	"context"
	"errors"
	"sort"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// -1 is the publication marker, 0 is the upstream payload, and positive IDs are customers.
const usageReportModelScope = -2

type usageReportModelTotal struct {
	Model   string
	Metrics usageReportMetrics
}

type PublishedUsageModelTotal struct {
	ModelName string                `json:"model_name"`
	Total     UsageAnalyticsMetrics `json:"total"`
}

func writeUsageReportModels(ctx context.Context, db *gorm.DB, generation string, totals map[string]*usageMetricsAcc) error {
	models := make([]usageReportModelTotal, 0, len(totals))
	for model, acc := range totals {
		models = append(models, usageReportModelTotal{Model: model, Metrics: makeUsageReportMetrics(acc)})
	}
	sort.Slice(models, func(i, j int) bool { return models[i].Model < models[j].Model })
	raw, err := common.Marshal(models)
	if err != nil {
		return err
	}
	if err := usageReportBeforeBatch(ctx); err != nil {
		return err
	}
	payload, err := writeDailyReportPayload(ctx, db, usageReportBeforeBatch, generation, reportUsagePayload, usageReportModelScope, raw)
	if err != nil {
		return err
	}
	return db.Create(&usageReportDayRow{Generation: generation, UserID: usageReportModelScope, Payload: payload}).Error
}

func mergeUsageReportModel(totals map[string]*usageMetricsAcc, model string, acc *usageMetricsAcc) {
	if totals[model] == nil {
		totals[model] = &usageMetricsAcc{}
	}
	totals[model].merge(acc)
}

// GetPublishedUsageModelTotals reads exactly the same published days as the
// customer overview. Older generations are folded from their frozen customer
// payloads, with no reads from the request log or current billing configuration.
func GetPublishedUsageModelTotals(ctx context.Context, period UsageAnalyticsPeriod) ([]PublishedUsageModelTotal, error) {
	requested, _ := ctx.Value(usageReportReferencesKey{}).([]UsageReportDayReference)
	refs, err := FreezeUsageReportReferences(ctx, period, requested)
	if err != nil {
		return nil, err
	}
	totals := make(map[string]*usageMetricsAcc)
	for _, ref := range refs {
		if index := usageDayIndex(period, ref.Start); index < 0 || period.Days[index].Future {
			continue
		}
		if err := mergePublishedUsageDayModels(ctx, ref.Generation, totals); err != nil {
			return nil, err
		}
	}
	// A retired generation can be pruned between the initial lookup and the
	// legacy reads. Recheck availability before returning a possibly partial fold.
	if _, err := FreezeUsageReportReferences(ctx, period, refs); err != nil {
		return nil, err
	}
	models := make([]PublishedUsageModelTotal, 0, len(totals))
	for model, acc := range totals {
		models = append(models, PublishedUsageModelTotal{ModelName: model, Total: acc.finalize()})
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ModelName < models[j].ModelName })
	return models, nil
}

func mergePublishedUsageDayModels(ctx context.Context, generation string, totals map[string]*usageMetricsAcc) error {
	db := DB.WithContext(ctx)
	var projection usageReportDayRow
	err := db.Where("generation = ? AND user_id = ?", generation, usageReportModelScope).Take(&projection).Error
	if err == nil {
		raw, err := dailyReportPayload(ctx, db, generation, reportUsagePayload, usageReportModelScope, projection.Payload)
		if err != nil {
			return err
		}
		var models []usageReportModelTotal
		if err := common.UnmarshalJsonStr(raw, &models); err != nil {
			return err
		}
		for _, model := range models {
			mergeUsageReportModel(totals, model.Model, model.Metrics.accumulator())
		}
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	lastUserID := 0
	for {
		var rows []usageReportDayRow
		if err := db.Select("user_id, payload").Where("generation = ? AND user_id > ? AND overview <> ?", generation, lastUserID, "").
			Order("user_id").Limit(50).Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			raw, err := dailyReportPayload(ctx, db, generation, reportUsagePayload, row.UserID, row.Payload)
			if err != nil {
				return err
			}
			var data usageReportDayData
			if err := common.UnmarshalJsonStr(raw, &data); err != nil {
				return err
			}
			for _, leaf := range data.Customers {
				mergeUsageReportModel(totals, leaf.Model, leaf.Metrics.accumulator())
			}
			lastUserID = row.UserID
		}
		if len(rows) < 50 {
			return nil
		}
	}
}
