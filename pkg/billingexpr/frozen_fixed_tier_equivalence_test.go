package billingexpr

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFrozenFixedTierEquivalentRequestBranches(t *testing.T) {
	for _, test := range []struct {
		name, expression string
		price            float64
		ambiguous        bool
	}{
		{"2.5", `(param("size") == "1K" ? (param("quality") == "high" ? tier("1K_high", fixed(0.05)) : tier("1K_medium", fixed(0.02263))) : tier("1K_medium", fixed(0.02263))) * image_count`, 0.02263, false},
		{"2-r", `((param("size") == "1K" || param("size") == "1024x1024") ? tier("1K_medium", fixed(0.05484)) : tier("1K_medium", fixed(0.05484))) * image_count`, 0.05484, false},
		{"token condition", `((param("size") == "1K" || p > 1) ? tier("1K_medium", fixed(0.02263)) : tier("1K_medium", fixed(0.02263))) * image_count`, 0.02263, true},
		{"count condition", `((param("size") == "1K" || image_count > 1) ? tier("1K_medium", fixed(0.02263)) : tier("1K_medium", fixed(0.02263))) * image_count`, 0.02263, true},
		{"recorded rule guard", `((param("size") == "1K" && (param("quality") == "high" ? 2 : 1) > 1) ? tier("1K_medium", fixed(0.02263)) : tier("1K_medium", fixed(0.02263))) * image_count`, 0.02263, true},
		{"unverified frozen price", `(param("size") == "1K" ? tier("1K_medium", fixed(0.05)) : tier("1K_medium", fixed(0.02263))) * image_count`, 0.035, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			one := 1
			request := RequestInput{Body: []byte(`{"size":"1K","quality":"high"}`), ImageCount: &one}
			_, trace, err := RunExprWithRequest(test.expression, TokenParams{}, request)
			require.NoError(t, err)
			frozen, _, err := WithFrozenRequestRules(test.expression, RequestInput{ImageCount: &one}, trace.RequestRules, "1K_medium", &test.price)
			if test.ambiguous {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			cost, _, err := RunExprWithRequest(test.expression, TokenParams{}, frozen)
			require.NoError(t, err)
			assert.InDelta(t, test.price*500_000, cost/2, 1e-6)
		})
	}
}
