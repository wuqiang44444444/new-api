package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/relaykit/dto"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskCalculationRoundsAfterFrozenContractDiscount(t *testing.T) {
	for _, image := range []bool{false, true} {
		for _, test := range []struct {
			name     string
			contract *hosttypes.ContractBillingFact
			quota    int
			amount   string
		}{
			{"without_contract", nil, 3, "2.6"},
			{"with_contract", &hosttypes.ContractBillingFact{RatioUnits: 50000000}, 1, "1.3"},
		} {
			kind := "video"
			if image {
				kind = "image"
			}
			t.Run(kind+"/"+test.name, func(t *testing.T) {
				const expression = `tier("base", (p+c)*0.13)`
				snapshot := &billingexpr.BillingSnapshot{
					ExprString: expression, ExprHash: billingexpr.ExprHashString(expression),
					GroupRatio: 1, QuotaPerUnit: 1000000,
				}
				task := &model.Task{PrivateData: model.TaskPrivateData{
					BillingContext: &model.TaskBillingContext{TieredSnapshot: snapshot, ContractFact: test.contract},
					AsyncBilling:   &model.TaskAsyncBillingContext{TieredSnapshot: snapshot, ActualTokens: 20, ActualUsageReported: true},
				}}
				var calculation *billingexpr.Calculation
				if image {
					task.PrivateData.ImageTask = &model.TaskImageExecutionData{}
					quota, _, err := imageTaskTargetQuota(t.Context(), task, &dto.Usage{PromptTokens: 20, TotalTokens: 20})
					require.NoError(t, err)
					assert.Equal(t, test.quota, quota)
					calculation = task.PrivateData.AsyncBilling.Calculation
				} else {
					result, _, err := ComputeTaskTieredBilling(task)
					require.NoError(t, err)
					assert.Equal(t, test.quota, result.ActualQuotaAfterGroup)
					calculation = result.Calculation
				}
				require.NotNil(t, calculation)
				assert.Equal(t, test.quota, calculation.Quota)
				require.GreaterOrEqual(t, len(calculation.Steps), 2)
				var rounds int
				for _, step := range calculation.Steps {
					if step.Op == "round" {
						rounds++
					}
				}
				assert.Equal(t, 1, rounds, "the trace must not suggest rounding before applying the contract")
				last := calculation.Steps[len(calculation.Steps)-1]
				assert.Equal(t, "round", last.Op)
				assert.Equal(t, []string{test.amount}, last.Inputs)
				assert.Equal(t, billingexpr.CalculationNumber(test.quota), last.Result)
				if test.contract != nil {
					discount := calculation.Steps[len(calculation.Steps)-2]
					assert.Equal(t, "contract_ratio", discount.Op)
					assert.Equal(t, []string{"2.6", "0.5"}, discount.Inputs)
					assert.Equal(t, "1.3", discount.Result)
				}
			})
		}
	}
}
