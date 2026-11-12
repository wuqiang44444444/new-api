package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

// Only the existing precharge is booked if the final funding adjustment fails.
func settleSynchronousUsage(ctx *gin.Context, info *relaycommon.RelayInfo, expectedQuota int, hasUsage bool) (bookedQuota int, funded bool) {
	bookedQuota = info.FinalPreConsumedQuota
	if info.Billing != nil {
		bookedQuota = info.Billing.GetPreConsumedQuota()
	}
	funded, err := settleBillingWithFunding(ctx, info, expectedQuota)
	if err != nil {
		logger.LogError(ctx, common.LogText("error settling billing: %s", err.Error()))
	}
	if funded {
		bookedQuota = expectedQuota
	}
	if hasUsage {
		model.UpdateUserUsedQuotaAndRequestCount(info.UserId, bookedQuota)
		model.UpdateChannelUsedQuota(info.ChannelId, bookedQuota)
	} else if bookedQuota > 0 {
		model.UpdateUserUsedQuota(info.UserId, bookedQuota)
		model.UpdateChannelUsedQuota(info.ChannelId, bookedQuota)
	}
	return bookedQuota, funded
}

func markUnsettledUsage(other *model.LogOther, funded bool) {
	if !funded {
		other.SetAdmin(model.StatementFundingAdjustmentFailedKey, true)
	}
}
