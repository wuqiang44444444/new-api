package model

import (
	"encoding/base64"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFrozenTaskImageMeterUsesFinalCalculation(t *testing.T) {
	expression := base64.StdEncoding.EncodeToString([]byte(`tier("image", u("image_count") * 0.04)`))
	for _, tc := range []struct {
		name, event, id string
		kind            int
		facts           map[string]any
		wantApplicable  bool
		wantCount       int64
	}{
		{"create reservation", "create", "task-billing:4:create", LogTypeConsume, map[string]any{"image_count": 8}, false, 0},
		{"final adjustment", "adjustment", "task-billing:4:adjustment", LogTypeConsume, map[string]any{
			"image_count": 8, "billing_calculation": map[string]any{"matched_tier": "image", "usage_facts": map[string]int{"image_count": 3}},
		}, true, 3},
		{"downward final adjustment", "adjustment", "task-billing:4:adjustment", LogTypeRefund, map[string]any{
			"image_count": 8, "billing_calculation": map[string]any{"matched_tier": "image", "usage_facts": map[string]int{"image_count": 2}},
		}, true, 2},
		{"ordinary refund", "refund", "task-billing:4:refund", LogTypeRefund, map[string]any{"image_count": 8}, false, 0},
		{"completion without frozen charge quantity", "create", "task-billing:4:complete", LogTypeConsume, map[string]any{"image_count": 8, "completed_image_count": 8}, true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.facts["expr_b64"] = expression
			tc.facts["usage_units"] = map[string]string{"image_count": "count"}
			tc.facts["matched_tier"] = "image"
			tc.facts["task_id"] = "task-4"
			tc.facts["task_billing_event"] = tc.event
			raw, err := common.Marshal(tc.facts)
			require.NoError(t, err)
			log := billingReconciliationLog{Type: tc.kind, RequestId: tc.id, Other: string(raw)}
			parsed := parseBillingReconciliationLog(log)
			assert.Equal(t, tc.wantApplicable, parsed.billableImageApplicable)
			if tc.wantCount == 0 {
				assert.Nil(t, parsed.billableImageCount)
			} else {
				require.NotNil(t, parsed.billableImageCount)
				assert.Equal(t, tc.wantCount, *parsed.billableImageCount)
			}
		})
	}
}
