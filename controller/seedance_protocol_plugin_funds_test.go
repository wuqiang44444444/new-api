package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/plugins"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
)

// Every newly migrated family crosses the same durable barrier and retains
// the original wallet/token refund and unknown-create semantics.
func TestSeedanceMigratedProtocolsPreserveFundsBarrier(t *testing.T) {
	for _, protocol := range []struct {
		video   dto.VideoUpstreamProtocol
		model   string
		failure string
	}{
		{dto.VideoUpstreamProtocolModelArkV3BytePlus, "ep-reviewed", `{"id":"prov-task-1","status":"failed","error":{"message":"content policy rejection"}}`},
		{dto.VideoUpstreamProtocolModelArkV3CMCC, "doubao-seedance-2.0", `{"id":"prov-task-1","status":"failed","error":{"message":"content policy rejection"}}`},
		{dto.VideoUpstreamProtocolArkMediaV1, "ep-reviewed", `{"id":"prov-task-1","status":"failed","error":{"message":"content policy rejection"}}`},
		{dto.VideoUpstreamProtocolTokenSaveMediaTaskV1, "doubao-seedance-2-0-260128", `{"task_id":"prov-task-1","status":"failed","error_message":"content policy rejection"}`},
		{dto.VideoUpstreamProtocolMoxingModelArkV1, "doubao-seedance-2-0-260128-0818", `{"task_id":"prov-task-1","status":"failed","error_message":"content policy rejection"}`},
	} {
		for _, outcome := range []string{"accepted", "failed", "unknown"} {
			t.Run(string(protocol.video)+"/"+outcome, func(t *testing.T) {
				fx := newSeedanceFundsFixture(t)
				var channel model.Channel
				require.NoError(t, fx.db.First(&channel, fx.channelID).Error)
				mapping, err := common.Marshal(map[string]string{"customer-video": protocol.model})
				require.NoError(t, err)
				channel.ModelMapping = common.GetPointer(string(mapping))
				channel.SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamProtocol: protocol.video, AssetUpstreamProtocol: dto.AssetUpstreamProtocolNone})
				require.NoError(t, fx.db.Save(&channel).Error)
				require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"billing_setting.billing_expr": `{"customer-video":"tier(\"fixed\", 1)"}`}))
				fx.assertAttemptAtPost.Store(true)
				if outcome == "unknown" {
					fx.createStatus.Store(http.StatusInternalServerError)
					fx.createBodyRes.Store(`{"error":{"message":"uncertain upstream result"}}`)
				}
				response := fx.submitCreateBody(`{"model":"customer-video","content":[{"type":"text","text":"A landscape"}],"duration":4,"resolution":"720p","ratio":"16:9"}`)
				assert.EqualValues(t, 1, fx.createCalls.Load())
				assert.Equal(t, seedanceFundsInitialQuota-seedanceFundsHold, fx.userQuota())
				if outcome == "unknown" {
					var count int64
					require.NoError(t, fx.db.Model(&model.Task{}).Count(&count).Error)
					assert.Zero(t, count)
					return
				}
				publicID := decodeSeedanceFundsCreateID(t, response)
				task := fx.loadTask(publicID)
				require.NotNil(t, task.PrivateData.Execution.TaskPlugin)
				assert.Equal(t, plugins.SeedanceVersion(), task.PrivateData.Execution.TaskPlugin.Version)
				if outcome == "failed" {
					fx.queryBodyRes.Store(protocol.failure)
					fx.pollOnce(publicID)
					fx.pollOnce(publicID)
					assert.Equal(t, seedanceFundsInitialQuota, fx.userQuota())
					quota, _ := fx.tokenQuota()
					assert.Equal(t, seedanceFundsInitialQuota, quota)
				}
			})
		}
	}
}
