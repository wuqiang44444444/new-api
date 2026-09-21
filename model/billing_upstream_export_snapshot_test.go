package model

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpstreamExportMetadataSnapshotAcrossBatches(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.AutoMigrate(&ProviderURLGroupDisplayName{}))
	require.NoError(t, db.Create(&User{Id: 1, Username: "snapshot-admin", Role: common.RoleAdminUser, Status: common.UserStatusEnabled}).Error)
	period := time.Date(2026, 9, 1, 0, 0, 0, 0, time.FixedZone("Shanghai", 8*3600)).Unix()
	ids := make([]int, 101)
	for i := range ids {
		ids[i] = i + 1
		require.NoError(t, db.Create(&Channel{Id: i + 1, Name: fmt.Sprintf("channel-%d", i+1)}).Error)
	}
	require.NoError(t, db.Create(&[]ProviderChannelBillingDiscount{
		{PeriodStart: previousBillingPeriodStart(period), ChannelId: 101, Discount: decimal.RequireFromString("0.8"), Version: 1},
		{PeriodStart: period, ChannelId: 202, Discount: decimal.RequireFromString("0.5"), Version: 2},
		{PeriodStart: period, ChannelId: 203, Discount: decimal.NewFromInt(1), PendingReason: "conflict", Version: 1},
	}).Error)
	for _, all := range []bool{false, true} {
		t.Run(fmt.Sprint(all), func(t *testing.T) {
			selected := ids
			if all {
				selected = nil
			}
			scope, err := FreezeUpstreamExportScope(context.Background(), 1, period, selected, "", all)
			require.NoError(t, err)
			channel := scope.Channel(101, period)
			assert.Equal(t, "channel-101", channel.Name)
			require.NotNil(t, channel.Discount)
			assert.Equal(t, "0.8", channel.Discount.Value.String())
			assert.Zero(t, channel.Discount.Version)
			if all {
				assert.Equal(t, "0.5", scope.Channel(202, period).Discount.Value.String(), "deleted channel coefficients remain frozen")
				assert.Nil(t, scope.Channel(203, period).Discount, "pending records remain unknown")
				assert.Equal(t, "1", scope.Channel(204, period).Discount.Value.String(), "absent configuration uses the frozen default")
			}
		})
	}
}

func TestUpstreamGlobalExportAuthorizationRequiresExplicitEvidenceScope(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&User{Id: 1, Username: "scope-admin", Role: common.RoleAdminUser, Status: common.UserStatusEnabled}).Error)
	for _, tc := range []struct {
		name, jobType string
		scope         UpstreamExportScope
		allowed       bool
	}{
		{"global evidence", CustomerExportJobTypeUpstreamDetails, UpstreamExportScope{AllChannels: true, EvidenceFilter: "incomplete"}, true},
		{"unscoped", CustomerExportJobTypeUpstreamDetails, UpstreamExportScope{AllChannels: true}, false},
		{"mixed group", CustomerExportJobTypeUpstreamDetails, UpstreamExportScope{AllChannels: true, EvidenceFilter: "incomplete", URLKey: "https://scope.example"}, false},
		{"summary", CustomerExportJobTypeUpstreamSummary, UpstreamExportScope{AllChannels: true, EvidenceFilter: "incomplete"}, false},
		{"root-only", CustomerExportJobTypeUpstreamDetails, UpstreamExportScope{AllChannels: true, EvidenceFilter: "incomplete", IncludeRootFields: true}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			filters, err := common.Marshal(CustomerExportFilters{Upstream: &tc.scope})
			require.NoError(t, err)
			job := CustomerExportJob{UserId: 1, JobType: tc.jobType, Filters: string(filters)}
			err = AuthorizeCustomerExportJob(context.Background(), &job)
			if tc.allowed {
				require.NoError(t, err)
			} else {
				assert.ErrorIs(t, err, ErrCustomerExportNotFound)
			}
		})
	}
}
