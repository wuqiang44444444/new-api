package billingexpr

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageExpressionKeepsActualMetersForIndependentLogRecalculation(t *testing.T) {
	expression := `tier("image", p * 2 + img_o * 8)`
	result, err := ComputeTieredQuota(&BillingSnapshot{ExprString: expression, ExprHash: ExprHashString(expression), QuotaPerUnit: 500000, GroupRatio: 1}, TokenParams{P: 100, ImgO: 200})
	require.NoError(t, err)
	require.NotNil(t, result.BillingTokens)
	assert.Equal(t, float64(100), result.BillingTokens.P)
	assert.Equal(t, float64(200), result.BillingTokens.ImgO)
	assert.Equal(t, 900, result.ActualQuotaAfterGroup)
}
