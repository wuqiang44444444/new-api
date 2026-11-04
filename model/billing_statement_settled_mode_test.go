package model

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatementGroupsUseSettledPricingBranchAndFrozenTaskMeter(t *testing.T) {
	for _, tc := range []struct {
		name, expression, unit, mode string
		units                        map[string]string
	}{
		{"fixed branch", `p > 10 ? tier("call", fixed(0.01)) : tier("tokens", p * 2)`, "request", "per_call", nil},
		{"token branch", `p > 10 ? tier("call", fixed(0.01)) : tier("tokens", p * 2)`, "token", "token", nil},
		{"missing settled branch", `tier("call", fixed(0))`, "", "unknown", nil},
		{"seconds stay seconds", `tier("seconds", u("seconds") * 0.4)`, "token", "per_second", map[string]string{"seconds": "second"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupBillingReconciliationTestDB(t)
			facts, err := common.Marshal(map[string]any{
				"contract_applicable": false, "group_ratio": 1,
				"expr_b64":     base64.StdEncoding.EncodeToString([]byte(tc.expression)),
				"billing_unit": tc.unit, "usage_units": tc.units,
			})
			require.NoError(t, err)
			require.NoError(t, db.Create(&Log{UserId: 7, ChannelId: 21, TokenId: 11, CreatedAt: 1100, Type: LogTypeConsume, ModelName: "customer", Quota: 100, Other: string(facts)}).Error)
			statement, err := GetBillingCustomerStatement(context.Background(), 7, 1000, 1500, "api_key", 0, "", "")
			require.NoError(t, err)
			require.Len(t, statement.Groups, 1)
			require.Len(t, statement.Groups[0].Models, 1)
			assert.Equal(t, tc.mode, statement.Groups[0].Models[0].BillingMode)
		})
	}
}
