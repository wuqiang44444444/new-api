package service

import (
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskBillingDeliveryPreservesFrozenExchangeRate(t *testing.T) {
	oldRate := operation_setting.USDExchangeRate
	t.Cleanup(func() { require.NoError(t, operation_setting.SetUSDExchangeRate(fmt.Sprint(oldRate))) })
	require.NoError(t, operation_setting.SetUSDExchangeRate("8.8"))

	frozenAt := time.Date(2026, 9, 28, 7, 40, 0, 0, time.UTC)
	nativeRate, err := billingexpr.NewExchangeRateContext(7, frozenAt.Add(-time.Hour))
	require.NoError(t, err)
	asyncRate, err := billingexpr.NewExchangeRateContext(6.7, frozenAt)
	require.NoError(t, err)
	const expression = `tier("no_video", u("tokens") * 23 / usd_exchange_rate() / 1000000)`

	for _, tc := range []struct {
		name     string
		async    bool
		rate     *billingexpr.ExchangeRateContext
		expected *billingexpr.ExchangeRateContext
	}{
		{name: "billing context snapshot", expected: nativeRate},
		{name: "async snapshot owns expression and rate", async: true, rate: asyncRate, expected: asyncRate},
		{name: "missing async rate never falls back", async: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			task := &model.Task{Quota: 69676, PrivateData: model.TaskPrivateData{
				BillingContext: &model.TaskBillingContext{TieredSnapshot: &billingexpr.BillingSnapshot{
					ExprString: expression, UsdExchangeRate: nativeRate,
				}},
			}}
			if tc.async {
				task.PrivateData.AsyncBilling = &model.TaskAsyncBillingContext{TieredSnapshot: &billingexpr.BillingSnapshot{
					ExprString: expression, UsdExchangeRate: tc.rate, TaskUsageBilling: true,
					UsageUnits: map[string]string{"tokens": "token"},
				}}
			}
			before, err := common.Marshal(task)
			require.NoError(t, err)
			for _, event := range []struct {
				name   string
				before int
				after  int
				quota  int
				typ    int
			}{
				{"create", 0, 583582, 583582, model.LogTypeConsume},
				{"adjustment", 583582, 69676, 513906, model.LogTypeRefund},
				{"refund", 583582, 0, 583582, model.LogTypeRefund},
			} {
				t.Run(event.name, func(t *testing.T) {
					log, err := BuildTaskBillingDeliveryLog(task, model.TaskBillingDelivery{
						Event: event.name, BeforeQuota: event.before, AfterQuota: event.after,
					})
					require.NoError(t, err)
					AttachLogsBillingDisplay([]*model.Log{log})
					var payload struct {
						Rate    *billingexpr.ExchangeRateContext `json:"usd_exchange_rate"`
						Display *billingexpr.DisplayProjection   `json:"billing_display"`
					}
					require.NoError(t, common.UnmarshalJsonStr(log.Other, &payload))
					assert.Equal(t, tc.expected, payload.Rate)
					require.NotNil(t, payload.Display)
					assert.Equal(t, tc.expected, payload.Display.AppliedExchangeRate)
					if tc.expected == nil {
						assert.Equal(t, billingexpr.DisplayReasonExchangeRateUnresolved, payload.Display.Reason)
					} else {
						assert.NotEqual(t, billingexpr.DisplayReasonExchangeRateUnresolved, payload.Display.Reason)
					}
					assert.Equal(t, event.quota, log.Quota)
					assert.Equal(t, event.typ, log.Type)
				})
			}
			after, err := common.Marshal(task)
			require.NoError(t, err)
			assert.JSONEq(t, string(before), string(after), "log projection must not modify frozen task or funding facts")
		})
	}
}
