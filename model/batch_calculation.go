package model

import (
	"sort"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"gorm.io/gorm"
)

type BatchLineCalculation struct {
	Initial       *billingexpr.Calculation
	Settlement    *billingexpr.Calculation
	Quota         int
	ResultPresent bool
	// NotCharged is a zero settlement target supported by complete result
	// collection, not a fabricated provider result or a committed refund.
	NotCharged bool
}

func GetBatchLineCalculation(job *BatchJob, customID string) (*BatchLineCalculation, error) {
	var line BatchJobLine
	err := DB.Select("calculation", "calculation_version", "final_quota").Where("job_id = ? AND custom_id = ?", job.Id, customID).First(&line).Error
	missing := err == gorm.ErrRecordNotFound
	if err != nil && !missing {
		return nil, err
	}
	var frozen BatchFrozenSnapshot
	if err = common.UnmarshalJsonStr(string(job.FrozenSnapshot), &frozen); err != nil {
		return nil, err
	}
	result := &BatchLineCalculation{Initial: frozen.InitialCalculations[customID], Quota: line.FinalQuota, ResultPresent: !missing}
	if missing {
		_, knownInput := frozen.LineInputs[customID]
		if !knownInput && result.Initial == nil {
			return nil, nil
		}
		result.NotCharged = job.DeliveryState == BatchDeliveryReady
		if !result.NotCharged && result.Initial != nil {
			result.Quota = result.Initial.Quota
		}
		return result, nil
	}
	if line.CalculationVersion > 0 && line.Calculation == "" {
		result.Settlement = &billingexpr.Calculation{}
	}
	if line.Calculation != "" {
		if err = common.UnmarshalJsonStr(string(line.Calculation), &result.Settlement); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// GetBatchBillingPage includes accepted input IDs absent from provider results.
// Sorting the union before pagination prevents absent rows from disappearing or
// duplicating across pages. Only compact result columns are loaded, never traces.
func GetBatchBillingPage(job *BatchJob, offset, limit int) ([]BatchBillingLineView, error) {
	var frozen BatchFrozenSnapshot
	if err := common.UnmarshalJsonStr(string(job.FrozenSnapshot), &frozen); err != nil {
		return nil, err
	}
	results, err := GetBatchBillingLines(job.Id, 0, -1)
	if err != nil {
		return nil, err
	}
	rows := make(map[string]BatchBillingLineView, len(frozen.LineInputs))
	for key := range frozen.LineInputs {
		rows[key] = BatchBillingLineView{CustomId: key, ChargeUnknown: true}
	}
	for key, initial := range frozen.InitialCalculations {
		if initial != nil {
			rows[key] = BatchBillingLineView{CustomId: key, FinalQuota: initial.Quota}
		}
	}
	for key, row := range rows {
		row.UsageUnavailable = true
		row.Status, row.Estimated = "estimated", true
		if job.DeliveryState == BatchDeliveryReady {
			row.Status, row.Estimated = "not_charged", false
			row.FinalQuota, row.ChargeUnknown = 0, false
		}
		rows[key] = row
	}
	for _, row := range results {
		rows[row.CustomId] = row
	}
	keys := make([]string, 0, len(rows))
	for key := range rows {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	page := make([]BatchBillingLineView, 0, min(limit, len(keys)))
	for i := max(offset, 0); i < len(keys) && len(page) < limit; i++ {
		page = append(page, rows[keys[i]])
	}
	return page, nil
}
