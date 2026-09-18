package model

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatementClassifiesFrozenUsageUnits(t *testing.T) {
	for _, tc := range []struct {
		name, expression, mode string
		units                  map[string]string
	}{
		{"tokens", `tier("base", u("meter") * 7 / 1000000)`, "token", map[string]string{"meter": "token"}},
		{"enum token tier", `u("mode") == "pro" ? tier("pro", u("meter") * 9 / 1000000) : tier("base", u("meter") * 7 / 1000000)`, "token", map[string]string{"meter": "token", "mode": "enum"}},
		{"boolean token tier", `u("audio") ? tier("audio", u("meter") * 9 / 1000000) : tier("base", u("meter") * 7 / 1000000)`, "token", map[string]string{"meter": "token", "audio": "boolean"}},
		{"name is not a unit", `tier("base", u("tokens") * 7)`, "per_second", map[string]string{"tokens": "second"}},
		{"credits", `tier("base", u("meter") * 7)`, "unknown", map[string]string{"meter": "credit"}},
		{"mixed meters", `tier("base", u("meter") * 7 / 1000000 + u("seconds"))`, "unknown", map[string]string{"meter": "token", "seconds": "second"}},
		{"historical missing unit", `tier("base", u("tokens") * 7 / 1000000)`, "unknown", nil},
		{"dynamic key", `tier("base", u(param("unit")) + u("meter"))`, "unknown", map[string]string{"meter": "token"}},
		{"aliased meter", `let meter = u; tier("base", meter("seconds") + u("tokens"))`, "unknown", map[string]string{"tokens": "token"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupBillingReconciliationTestDB(t)
			require.NoError(t, db.Create(&User{Id: 7, Username: "customer"}).Error)
			other, err := common.Marshal(map[string]any{"is_task": true, "task_id": "task", "expr_b64": base64.StdEncoding.EncodeToString([]byte(tc.expression)), "group_ratio": 1,
				"admin_info": map[string]any{"statement_snapshot": map[string]any{"billing_mode": "per_call", "usage_units": tc.units}}})
			require.NoError(t, err)
			require.NoError(t, db.Create(&Log{UserId: 7, TokenId: 4, ModelName: "video", Type: LogTypeConsume, CreatedAt: 1100, Quota: 80, Other: string(other)}).Error)
			s, err := GetBillingCustomerStatement(context.Background(), 7, 1000, 1200, "api_key", 4, "", "")
			require.NoError(t, err)
			require.Len(t, s.Groups, 1)
			require.Len(t, s.Groups[0].Models, 1)
			assert.Equal(t, tc.mode, s.Groups[0].Models[0].BillingMode)
			assert.EqualValues(t, 80, s.Summary.NetQuota)
			assert.EqualValues(t, 1, s.Summary.Requests)
		})
	}
}
