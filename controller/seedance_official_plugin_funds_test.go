package controller

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOfficialPluginFundsUsesFrozenExpressionAndValidUsage(t *testing.T) {
	for _, tc := range []struct {
		name, usage string
		charge      int
		awaiting    bool
	}{
		{"completion", `{"completion_tokens":10000,"total_tokens":12000}`, 50000, false},
		{"explicit-zero", `{"completion_tokens":0}`, 0, false},
		{"missing-completion", `{"total_tokens":12000}`, seedanceFundsHold, true},
		{"overflow", `{"completion_tokens":18446744073709551615}`, seedanceFundsHold, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := newSeedanceFundsFixture(t)
			var channel model.Channel
			require.NoError(t, fx.db.First(&channel, fx.channelID).Error)
			channel.ModelMapping = common.GetPointer(`{"customer-video":"ep-reviewed-deployment"}`)
			channel.SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolModelArkV3Volcengine, AssetUpstreamProtocol: dto.AssetUpstreamProtocolNone})
			require.NoError(t, fx.db.Save(&channel).Error)
			require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"billing_setting.billing_expr": `{"customer-video":"c * 10"}`}))
			fx.assertAttemptAtPost.Store(true)
			publicID := decodeSeedanceFundsCreateID(t, fx.submitCreate())
			require.Equal(t, seedanceFundsInitialQuota-seedanceFundsHold, fx.userQuota())
			task := fx.loadTask(publicID)
			require.NotNil(t, task.PrivateData.Execution)
			require.NotNil(t, task.PrivateData.Execution.TaskPlugin)
			require.NotNil(t, task.PrivateData.AsyncBilling.TieredSnapshot)
			assert.False(t, task.PrivateData.AsyncBilling.TieredSnapshot.TaskUsageBilling)
			// An administrator's later price change cannot reprice the frozen task.
			require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"billing_setting.billing_expr": `{"customer-video":"c * 1000"}`}))
			fx.queryBodyRes.Store(fmt.Sprintf(`{"id":"prov-task-1","status":"succeeded","content":{"video_url":%q},"usage":%s}`, fx.server.URL+"/video.mp4", tc.usage))
			fx.pollOnce(publicID)
			assert.Equal(t, seedanceFundsInitialQuota-tc.charge, fx.userQuota())
			task = fx.loadTask(publicID)
			assert.EqualValues(t, model.TaskStatusSuccess, task.Status)
			if tc.awaiting {
				assert.Equal(t, model.TaskBillingStateAwaitingUsage, task.PrivateData.AsyncBilling.State)
			}
			fx.pollOnce(publicID)
			assert.Equal(t, seedanceFundsInitialQuota-tc.charge, fx.userQuota(), "repeated observation must not settle or refund twice")
			remain, _ := fx.tokenQuota()
			assert.Equal(t, seedanceFundsInitialQuota-tc.charge, remain)
		})
	}
}
