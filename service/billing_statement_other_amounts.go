package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
)

// FreezeMidjourneyStatementOriginal preserves the original fixed price for the
// charge and a later full refund. Ratio-based reservations are not list prices.
func FreezeMidjourneyStatementOriginal(task *model.Midjourney, price types.PriceData, prepared bool) {
	if !prepared || !price.UsePrice || price.ModelPrice < 0 {
		return
	}
	if original, err := common.QuotaFromFloatStrict(price.ModelPrice * common.QuotaPerUnit); err == nil {
		task.OriginalQuota = &original
	}
}

func AppendMidjourneyStatementOriginal(other *model.LogOther, task *model.Midjourney) {
	if task.OriginalQuota != nil {
		other.SetAdmin(model.StatementOriginalQuotaLogKey, *task.OriginalQuota)
	}
}
