package service

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

func billingImageTotalsCSVHeader(language string) []string {
	if language == "zh" {
		return []string{"本站计费张数", "计费张数缺失笔数", "渠道测试计价张数", "测试张数缺失笔数"}
	}
	return []string{"Billable images", "Missing billable image rows", "Channel test billable images", "Missing test billable image rows"}
}

func billingImageTotalsCSVCells(images model.BillingImageTotals) []string {
	return []string{
		positiveBillingMeterCSV(images.BillableImages),
		positiveBillingMeterCSV(images.BillableImageMissingRows),
		positiveBillingMeterCSV(images.TestBillableImages),
		positiveBillingMeterCSV(images.TestBillableImageMissingRows),
	}
}

func positiveBillingMeterCSV(value int64) string {
	if value == 0 {
		return ""
	}
	return strconv.FormatInt(value, 10)
}

func frozenBillingDetailCSVHeader(language string) []string {
	if language == "zh" {
		return []string{"计费张数", "计费张数缺失", "实际出图张数", "冻结命中档位", "复算折前 quota", "复算应收 quota", "实际入账 quota", "入账差额 quota", "复算缺项原因", "冻结计费 Token 输入"}
	}
	return []string{"Billable images", "Billable images missing", "Completed images", "Frozen matched tier", "Replayed original quota", "Replayed expected quota", "Booked quota", "Settlement delta quota", "Frozen replay reason", "Frozen billable token inputs"}
}

func frozenBillingDetailCSVCells(row model.UpstreamBillingDetailItem) []string {
	cell := func(value *int64) string {
		if value == nil {
			return ""
		}
		return strconv.FormatInt(*value, 10)
	}
	missing := ""
	if row.BillableImageMissing {
		missing = "1"
	}
	cells := []string{cell(row.BillableImageCount), missing, cell(row.CompletedImageCount)}
	if replay := row.FrozenReplay; replay != nil {
		tokenInputs := ""
		if len(replay.TokenInputs) != 0 {
			raw, _ := common.Marshal(replay.TokenInputs)
			tokenInputs = string(raw)
		}
		return append(cells, exportCsvCellGuard(replay.MatchedTier), cell(replay.OriginalQuota), cell(replay.ExpectedQuota), cell(replay.BookedQuota), cell(replay.DeltaQuota), replay.Reason, tokenInputs)
	}
	return append(cells, "", "", "", "", "", "", "")
}
