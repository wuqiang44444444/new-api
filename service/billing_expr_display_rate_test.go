package service

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFrozenLogExchangeRate(t *testing.T) {
	// 汇率事实按日志注入形状往返：rate + frozen_at。
	frozen, err := billingexpr.NewExchangeRateContext(6.76, time.Date(2026, 9, 23, 8, 0, 0, 0, time.UTC))
	require.NoError(t, err)

	other := map[string]any{
		"usd_exchange_rate": map[string]any{
			"source_key": billingexpr.UsdExchangeRateSourceKey,
			"rate":       6.76,
			"frozen_at":  frozen.FrozenAt.Format(time.RFC3339),
		},
	}
	rate := frozenLogExchangeRate(other)
	require.NotNil(t, rate)
	assert.Equal(t, 6.76, rate.Rate)
	assert.Equal(t, billingexpr.UsdExchangeRateSourceKey, rate.SourceKey)
	assert.True(t, rate.FrozenAt.Equal(frozen.FrozenAt))
	assert.NoError(t, rate.Validate())

	// 旧日志、缺字段与非法值都明确不可证明，不回退当前设置。
	for name, other := range map[string]map[string]any{
		"no fact":       {},
		"missing rate":  {"usd_exchange_rate": map[string]any{"source_key": "USDExchangeRate"}},
		"zero rate":     {"usd_exchange_rate": map[string]any{"rate": 0.0}},
		"negative rate": {"usd_exchange_rate": map[string]any{"rate": -6.76}},
		"non-number":    {"usd_exchange_rate": map[string]any{"rate": "6.76"}},
		"wrong mystery": {"usd_exchange_rate": "6.76"},
	} {
		assert.Nil(t, frozenLogExchangeRate(other), "case %s must stay unprovable", name)
	}
}

func TestAttachLogsBillingDisplayKeepsPerLogFrozenInstant(t *testing.T) {
	const expression = `tier("log-provenance", p * 7 / usd_exchange_rate())`
	start := time.Date(2026, 9, 23, 8, 0, 0, 0, time.UTC)
	logs := make([]*model.Log, 2)
	for i := range logs {
		rate, err := billingexpr.NewExchangeRateContext(7, start.Add(time.Duration(i)*time.Hour))
		require.NoError(t, err)
		other, err := common.Marshal(map[string]any{
			"billing_mode": "tiered_expr", "expr_b64": base64.StdEncoding.EncodeToString([]byte(expression)), "usd_exchange_rate": rate,
		})
		require.NoError(t, err)
		logs[i] = &model.Log{Type: model.LogTypeConsume, Other: string(other)}
	}
	AttachLogsBillingDisplay(logs)
	for i, log := range logs {
		var payload struct {
			Rate    *billingexpr.ExchangeRateContext `json:"usd_exchange_rate"`
			Display *billingexpr.DisplayProjection   `json:"billing_display"`
		}
		require.NoError(t, common.UnmarshalJsonStr(log.Other, &payload))
		require.NotNil(t, payload.Display)
		require.NotNil(t, payload.Display.AppliedExchangeRate)
		assert.Equal(t, *payload.Rate, *payload.Display.AppliedExchangeRate)
		assert.Equal(t, start.Add(time.Duration(i)*time.Hour), payload.Display.AppliedExchangeRate.FrozenAt)
	}
}
