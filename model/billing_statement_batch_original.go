package model

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// BatchStatementOriginalQuota sums the verified per-line originals in the
// same rounding order as settlement. Older traces carry the input to the
// group conversion, which is stronger evidence than dividing the settled fee.
func BatchStatementOriginalQuota(job *BatchJob, lines []BatchJobLine, booked int) (int64, bool) {
	if int64(len(lines)) != job.LineCount || len(lines) == 0 {
		return 0, false
	}
	var frozen BatchFrozenSnapshot
	var frozenLoaded bool
	var original, net int64
	for _, line := range lines {
		if line.JobId != job.Id || line.FinalQuota < 0 || line.QuotaClamp != "" || int64(line.FinalQuota) > math.MaxInt64-net {
			return 0, false
		}
		net += int64(line.FinalQuota)
		if line.Status != "completed" {
			if line.FinalQuota != 0 {
				return 0, false
			}
			continue
		}
		var calc billingexpr.Calculation
		if line.CalculationVersion != 1 || common.UnmarshalJsonStr(string(line.Calculation), &calc) != nil || calc.Quota != line.FinalQuota {
			return 0, false
		}
		amount := calc.OriginalQuota
		if amount == nil {
			if !frozenLoaded {
				if common.UnmarshalJsonStr(string(job.FrozenSnapshot), &frozen) != nil ||
					frozen.QuotaPerUnit <= 0 || frozen.GroupRatio <= 0 {
					return 0, false
				}
				frozenLoaded = true
			}
			amount = originalFromFrozenBatchTrace(&frozen, &calc, line)
		}
		if amount == nil || *amount < 0 || int64(*amount) > math.MaxInt64-original {
			return 0, false
		}
		original += int64(*amount)
	}
	return original, net == int64(booked)
}

func originalFromFrozenBatchTrace(frozen *BatchFrozenSnapshot, calc *billingexpr.Calculation, line BatchJobLine) *int {
	var amount decimal.Decimal
	var found bool
	for _, step := range calc.Steps {
		if step.Op != "batch_conversion" {
			continue
		}
		if found || len(step.Inputs) != 4 || step.Inputs[1] != "1000000" {
			return nil
		}
		output, outputErr := decimal.NewFromString(step.Inputs[0])
		qpu, qpuErr := decimal.NewFromString(step.Inputs[2])
		group, groupErr := decimal.NewFromString(step.Inputs[3])
		applied, appliedErr := decimal.NewFromString(step.Result)
		if outputErr != nil || qpuErr != nil || groupErr != nil || appliedErr != nil ||
			output.IsNegative() || !qpu.Equal(decimal.NewFromFloat(frozen.QuotaPerUnit)) ||
			!group.Equal(decimal.NewFromFloat(frozen.GroupRatio)) {
			return nil
		}
		amount = output.Div(decimal.NewFromInt(1_000_000)).Mul(qpu)
		if !amount.Mul(group).Equal(applied) {
			return nil
		}
		groupQuota, err := common.QuotaFromDecimalStrict(applied)
		if err != nil || groupQuota != line.ModelQuota {
			return nil
		}
		net := applied
		if frozen.ContractFact != nil {
			ratio := frozen.ContractFact.RatioDecimal()
			if ratio.IsZero() {
				return nil
			}
			net = net.Mul(ratio)
		}
		netQuota, err := common.QuotaFromDecimalStrict(net)
		if err != nil || netQuota != line.FinalQuota {
			return nil
		}
		found = true
	}
	if !found {
		return nil
	}
	result, err := common.QuotaFromDecimalStrict(amount)
	if err != nil {
		return nil
	}
	return &result
}

func recoverBatchCustomerOriginal(ctx context.Context, log billingReconciliationLog, parsed *parsedBillingReconciliationLog) error {
	if log.Type != LogTypeConsume || !strings.HasPrefix(log.RequestId, "batch_") || log.UserId <= 0 || log.TokenId <= 0 ||
		parsed.replayedOriginalQuota != nil || parsed.recordedOriginalQuota != nil || parsed.unavailable ||
		billingBreakdownString(loadCustomerExportFacts(log.Other).other["billing_mode"]) != "azure_batch" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	db := usageReportMainDB(ctx).WithContext(ctx)
	var job BatchJob
	err := db.Where("id = ? AND user_id = ? AND app_id = ? AND token_id = ? AND channel_id = ? AND public_model = ? AND settle_state = ?",
		log.RequestId, log.UserId, log.TokenId, log.TokenId, log.ChannelId, parsed.customerModel, BatchSettleSettled).Take(&job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if job.TargetQuota == nil || *job.TargetQuota != log.Quota {
		return nil
	}
	var lines []BatchJobLine
	if err := db.Where("job_id = ?", job.Id).Find(&lines).Error; err != nil {
		return err
	}
	if original, ok := BatchStatementOriginalQuota(&job, lines, log.Quota); ok {
		parsed.replayedOriginalQuota = &original
		parsed.originalReplayReason = ""
	}
	return nil
}
