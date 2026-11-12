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
	Model       string
	BillingMode string
	Metrics     usageReportMetrics
}

type PublishedUsageModelTotal struct {
	ModelName   string                `json:"model_name"`
	BillingMode string                `json:"billing_mode,omitempty"`
	Total       UsageAnalyticsMetrics `json:"total"`
}

type usageReportModelKey struct {
	name string
	mode string
}

func writeUsageReportModels(ctx context.Context, db *gorm.DB, generation string, totals map[usageReportModelKey]*usageMetricsAcc) error {
	models := make([]usageReportModelTotal, 0, len(totals))
	for key, acc := range totals {
		models = append(models, usageReportModelTotal{Model: key.name, BillingMode: key.mode, Metrics: makeUsageReportMetrics(acc)})
	}
	sort.Slice(models, func(i, j int) bool {
		if models[i].Model == models[j].Model {
			return models[i].BillingMode < models[j].BillingMode
		}
		return models[i].Model < models[j].Model
	})
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

func mergeUsageReportModel(totals map[usageReportModelKey]*usageMetricsAcc, key usageReportModelKey, acc *usageMetricsAcc) {
	if key.mode == "" {
		key.mode = BillingReconciliationModeUnknown
	}
	if totals[key] == nil {
		totals[key] = &usageMetricsAcc{}
	}
	totals[key].merge(acc)
}

// GetPublishedUsageModelTotals reads exactly the same published days as the
// customer overview. Older generations are folded from their frozen customer
// payloads, with no reads from the request log or current billing configuration.
func GetPublishedUsageModelTotals(ctx context.Context, period UsageAnalyticsPeriod) ([]PublishedUsageModelTotal, error) {
	return getPublishedUsageModelTotals(ctx, period, false)
}

func getPublishedUsageModelTotals(ctx context.Context, period UsageAnalyticsPeriod, splitBillingMode bool) ([]PublishedUsageModelTotal, error) {
	requested, _ := ctx.Value(usageReportReferencesKey{}).([]UsageReportDayReference)
	refs, err := FreezeUsageReportReferences(ctx, period, requested)
	if err != nil {
		return nil, err
	}
	totals := make(map[usageReportModelKey]*usageMetricsAcc)
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
	if !splitBillingMode {
		collapsed := make(map[usageReportModelKey]*usageMetricsAcc)
		for key, acc := range totals {
			key.mode = ""
			if collapsed[key] == nil {
				collapsed[key] = &usageMetricsAcc{}
			}
			collapsed[key].merge(acc)
		}
		totals = collapsed
	}
	models := make([]PublishedUsageModelTotal, 0, len(totals))
	for key, acc := range totals {
		models = append(models, PublishedUsageModelTotal{ModelName: key.name, BillingMode: key.mode, Total: acc.finalize()})
	}
	sort.Slice(models, func(i, j int) bool {
		if models[i].ModelName == models[j].ModelName {
			return models[i].BillingMode < models[j].BillingMode
		}
		return models[i].ModelName < models[j].ModelName
	})
	return models, nil
}

func mergePublishedUsageDayModels(ctx context.Context, generation string, totals map[usageReportModelKey]*usageMetricsAcc) error {
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
			mergeUsageReportModel(totals, usageReportModelKey{model.Model, model.BillingMode}, model.Metrics.accumulator())
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
				mergeUsageReportModel(totals, usageReportModelKey{leaf.Model, leaf.BillingMode}, leaf.Metrics.accumulator())
			}
			lastUserID = row.UserID
		}
		if len(rows) < 50 {
			return nil
		}
	}
}
