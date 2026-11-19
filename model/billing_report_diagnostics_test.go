package model

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHistoricalImageDiagnosticsReadPersistedMessagesWithoutChangingCharges(t *testing.T) {
	db, start, _ := setupBillingReportTest(t)
	ctx := context.Background()
	const legacy = `(
	  (param("size") == "1K" || param("size") == "1024x1024") ?
	    (param("quality") == "low" ? tier("1K_low",fixed(0.1)) : param("quality") == "high" ? tier("1K_high",fixed(0.3)) : tier("1K_medium",fixed(0.2))) :
	  param("size") == "2K" ?
	    (param("quality") == "low" ? tier("2K_low",fixed(0.2)) : param("quality") == "high" ? tier("2K_high",fixed(0.4)) : tier("2K_medium",fixed(0.3))) :
	  param("size") == "4K" ?
	    (param("quality") == "low" ? tier("4K_low",fixed(0.3)) : param("quality") == "high" ? tier("4K_high",fixed(0.5)) : tier("4K_medium",fixed(0.4))) :
	  tier("1K_medium",fixed(0.2))) * image_count`
	// These literal keys are the stored format, independent of current display text.
	for _, key := range []string{"Size {{size}}", "Unknown historical message"} {
		raw, err := common.Marshal(map[string]any{
			"billing_mode": "tiered_expr", "billing_unit": "request", "request_path": "/v1/images/generations", "image_count": 1,
			"expr_b64": base64.StdEncoding.EncodeToString([]byte(legacy)), "fixed_price": 0.2, "matched_tier": "1K_medium",
			"content_parts": []*common.Message{common.NewMessage(key, map[string]any{"size": "3840x2160"}), common.NewMessage("Quality {{quality}}", map[string]any{"quality": "high"})},
		})
		require.NoError(t, err)
		require.NoError(t, db.Create(&Log{UserId: 7, Type: LogTypeConsume, CreatedAt: start + 1, ModelName: "sample", Quota: 100000, Other: string(raw)}).Error)
	}
	require.NoError(t, BuildBillingReportDay(ctx, start, "billing-test", "runner", nil, nil))
	lines, err := BillingReportDiagnosticLines(ctx, start, 0)
	require.NoError(t, err)
	require.Len(t, lines, 2)
	assert.Equal(t, "image_price_tier_mismatch", lines[0].PriceIssue)
	assert.Equal(t, "image_price_inputs_missing", lines[1].PriceIssue)
	for _, line := range lines {
		assert.EqualValues(t, 100000, line.CustomerCharge)
	}
}
