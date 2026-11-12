package service

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLegacyTaskCompletionUsesFrozenContractBeforeAdjustingHold(t *testing.T) {
	truncate(t)
	const userID, wallet, hold = 45, 300000, 146000
	seedUser(t, userID, wallet)
	task := makeTask(userID, 0, hold, 0, BillingSourceWallet, 0)
	const expression = `tier("base", u("seconds") * 0.08)`
	task.PrivateData.BillingContext.TieredSnapshot = &billingexpr.BillingSnapshot{
		ExprString: expression, ExprHash: billingexpr.ExprHashString(expression),
		GroupRatio: 1, QuotaPerUnit: 500000, ExprVersion: 1, TaskUsageBilling: true,
	}
	task.PrivateData.BillingContext.ContractFact = &types.ContractBillingFact{RatioUnits: 73_000_000}
	persistTaskCalculationFixture(t, task)

	require.True(t, settleTaskBillingOnComplete(context.Background(), &mockAdaptor{}, task, &relaycommon.TaskInfo{
		Status: model.TaskStatusSuccess, UsageFacts: map[string]any{"seconds": float64(5)},
	}))
	assert.Equal(t, hold, task.Quota)
	assert.Equal(t, wallet, getUserQuota(t, userID))
	assert.Zero(t, countLogs(t))
	calculation := task.PrivateData.BillingContext.SettlementCalculation
	require.NotNil(t, calculation)
	require.NotNil(t, calculation.OriginalQuota)
	assert.Equal(t, 200000, *calculation.OriginalQuota)
	assert.Equal(t, hold, calculation.Quota)
}
