package controller

import (
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseBillingReconciliationPeriodRequiresShanghaiNaturalMonth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	start := time.Date(2026, time.August, 1, 0, 0, 0, 0, location).Unix()
	end := time.Date(2026, time.September, 1, 0, 0, 0, 0, location).Unix() - 1

	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest("GET", fmt.Sprintf("/?start_timestamp=%d&end_timestamp=%d", start, end), nil)
	period, ok := parseBillingReconciliationPeriod(context)
	require.True(t, ok)
	assert.Equal(t, start, period.StartTimestamp)
	assert.Equal(t, end, period.EndTimestamp)
	assert.Equal(t, "Asia/Shanghai", period.Timezone)

	invalidContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	invalidContext.Request = httptest.NewRequest("GET", fmt.Sprintf("/?start_timestamp=%d&end_timestamp=%d", start+24*60*60, end), nil)
	_, ok = parseBillingReconciliationPeriod(invalidContext)
	assert.False(t, ok)
}

func TestValidCopiedProviderBillingPeriodOnlyAllowsPreviousNaturalMonth(t *testing.T) {
	august := time.Date(2026, time.August, 1, 0, 0, 0, 0, billingSettlementLocation).Unix()
	july := time.Date(2026, time.July, 1, 0, 0, 0, 0, billingSettlementLocation).Unix()
	june := time.Date(2026, time.June, 1, 0, 0, 0, 0, billingSettlementLocation).Unix()
	assert.True(t, validCopiedProviderBillingPeriod(august, 0))
	assert.True(t, validCopiedProviderBillingPeriod(august, july))
	assert.False(t, validCopiedProviderBillingPeriod(august, june))
}
