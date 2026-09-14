package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContractDiscountConsumersRejectInvalidSnapshots(t *testing.T) {
	for _, tc := range []struct {
		name  string
		units []int64
	}{
		{"negative", []int64{-1}},
		{"zero", []int64{0}},
		{"above maximum", []int64{100_000_001}},
		{"negative before valid", []int64{-1, 80_000_000}},
		{"negative after valid", []int64{80_000_000, -1}},
		{"conflicting discounts", []int64{80_000_000, 90_000_000}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := &model.ContractEntitySnapshot{Id: 901, UserId: 902, Version: 1, Enabled: true}
			for i, units := range tc.units {
				snapshot.Rules = append(snapshot.Rules, model.ContractEntityRule{PublicModel: "demo", ChannelId: i + 1, RatioUnits: units})
			}
			customerContractEntityCache.Store(snapshot.Id, cachedContractEntity{authVersion: 1, snapshot: snapshot})
			t.Cleanup(func() { InvalidateContractEntityCache(snapshot.Id) })
			fact, err := ResolveContractEntityRule(snapshot.UserId, 1, snapshot.Id, "demo")
			require.ErrorIs(t, err, ErrCustomerContractUnavailable)
			assert.Nil(t, fact)
			views, err := BuildContractEntityUserViews([]model.ContractEntitySnapshot{*snapshot})
			require.ErrorIs(t, err, ErrCustomerContractUnavailable)
			assert.Nil(t, views)
			prices, err := ApplyContractDiscountOverlay([]model.Pricing{{ModelName: "demo"}}, nil, "default", snapshot)
			require.ErrorIs(t, err, ErrCustomerContractUnavailable)
			assert.Nil(t, prices)
		})
	}
}

func TestContractDiscountConsumersPreserveUnlistedAndDeduplicatedModels(t *testing.T) {
	snapshot := &model.ContractEntitySnapshot{Id: 903, UserId: 904, Version: 2, Enabled: true, Rules: []model.ContractEntityRule{
		{PublicModel: "demo", ChannelId: 1, RatioUnits: 80_000_000},
		{PublicModel: "demo", ChannelId: 2, RatioUnits: 80_000_000},
	}}
	customerContractEntityCache.Store(snapshot.Id, cachedContractEntity{authVersion: 1, snapshot: snapshot})
	t.Cleanup(func() { InvalidateContractEntityCache(snapshot.Id) })
	fact, err := ResolveContractEntityRule(snapshot.UserId, 1, snapshot.Id, "demo")
	require.NoError(t, err)
	require.NotNil(t, fact)
	assert.Equal(t, int64(80_000_000), fact.RatioUnits)
	fact, err = ResolveContractEntityRule(snapshot.UserId, 1, snapshot.Id, "unlisted")
	require.NoError(t, err)
	assert.Nil(t, fact)
	views, err := BuildContractEntityUserViews([]model.ContractEntitySnapshot{*snapshot})
	require.NoError(t, err)
	require.Len(t, views, 1)
	assert.Equal(t, []CustomerContractUserRuleView{{Model: "demo", Discount: "0.8"}}, views[0].Models)
}
