package service

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSynchronousBillingStatementUsesBookedFunds(t *testing.T) {
	for _, tc := range []struct {
		name, failedTable  string
		precharged, booked int
		wantOriginal       bool
	}{
		{name: "wallet update fails", failedTable: "users", precharged: 100, booked: 100},
		{name: "key update fails after wallet commits", failedTable: "tokens", precharged: 100, booked: 150, wantOriginal: true},
		{name: "zero delta uses the settled precharge", precharged: 150, booked: 150, wantOriginal: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncate(t)
			const id, wallet = 62, 10_000
			const key = "sync-billing-test"
			seedUser(t, id, wallet)
			seedChannel(t, id)
			seedToken(t, id, id, key, wallet)
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
			info := &relaycommon.RelayInfo{
				UserId: id, TokenId: id, TokenKey: key, OriginModelName: "sync-billing-model", StartTime: time.Now(),
				UsingGroup: "default", ForcePreConsume: true, UserSetting: dto.UserSetting{BillingPreference: "wallet_only"},
				ChannelMeta: &relaycommon.ChannelMeta{ChannelId: id},
				PriceData:   types.PriceData{ModelRatio: 1, CompletionRatio: 1, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 0.5}},
			}
			require.Nil(t, PreConsumeBilling(ctx, tc.precharged, info))
			if tc.failedTable != "" {
				column := "quota"
				if tc.failedTable == "tokens" {
					column = "remain_quota"
				}
				require.NoError(t, model.DB.Exec("CREATE TRIGGER reject_sync_settlement BEFORE UPDATE OF "+column+" ON "+tc.failedTable+" BEGIN SELECT RAISE(ABORT, 'test settlement failure'); END").Error)
				t.Cleanup(func() { require.NoError(t, model.DB.Exec("DROP TRIGGER reject_sync_settlement").Error) })
			}

			PostTextConsumeQuota(ctx, info, &dto.Usage{PromptTokens: 300, TotalTokens: 300}, nil)

			balance, err := model.GetUserQuota(id, true)
			require.NoError(t, err)
			assert.Equal(t, wallet-tc.booked, balance)
			log := getLastLog(t)
			require.NotNil(t, log)
			assert.Equal(t, tc.booked, log.Quota)
			var other struct {
				Admin map[string]any `json:"admin_info"`
			}
			require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
			statement, err := model.GetBillingCustomerStatement(context.Background(), id, log.CreatedAt-1, log.CreatedAt+1, "api_key", 0, "", "")
			require.NoError(t, err)
			assert.EqualValues(t, tc.booked, statement.Summary.NetQuota)
			if tc.wantOriginal {
				assert.EqualValues(t, 300, other.Admin[model.StatementOriginalQuotaLogKey])
				require.NotNil(t, statement.OriginalQuota)
				assert.EqualValues(t, 300, *statement.OriginalQuota)
			} else {
				assert.Equal(t, true, other.Admin[model.StatementFundingAdjustmentFailedKey])
				assert.Nil(t, statement.OriginalQuota)
			}
		})
	}
}

func TestSynchronousWalletAdjustmentCommitsWithBatchUpdatesEnabled(t *testing.T) {
	truncate(t)
	const id = 64
	seedUser(t, id, 900)
	original := common.BatchUpdateEnabled
	common.BatchUpdateEnabled = true
	t.Cleanup(func() { common.BatchUpdateEnabled = original })
	require.NoError(t, (&WalletFunding{userId: id}).Settle(50))
	balance, err := model.GetUserQuota(id, true)
	require.NoError(t, err)
	assert.Equal(t, 850, balance)
}
