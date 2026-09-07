package service

import (
	"context"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var recoveryProtocols = []dto.VideoUpstreamProtocol{
	dto.VideoUpstreamProtocolModelArkV3Volcengine, dto.VideoUpstreamProtocolModelArkV3BytePlus,
	dto.VideoUpstreamProtocolModelArkV3CMCC, dto.VideoUpstreamProtocolMoxingModelArkV1,
	dto.VideoUpstreamProtocolFunCloudSeedance, dto.VideoUpstreamProtocolFunCloudModelArkV3,
	dto.VideoUpstreamProtocolArkMediaV1, dto.VideoUpstreamProtocolTokenSaveMediaTaskV1,
	dto.VideoUpstreamProtocolMoxingMediaTaskV1, dto.VideoUpstreamProtocolFeicaiVideosV1,
}

func persistedSeedanceBillingTask(t *testing.T, protocol dto.VideoUpstreamProtocol, wallet int, expr string) *model.Task {
	t.Helper()
	truncate(t)
	seedUser(t, 8160, wallet)
	task := persistedAsyncTask(t, 8160, 700, model.TaskStatusInProgress)
	task.ClientProtocol = model.TaskClientProtocolModelArkV3
	task.Platform = constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSeedanceLink))
	task.PrivateData.VideoUpstreamProtocol = protocol
	task.PrivateData.VideoUpstreamProfile = protocol.TransportProfile()
	task.PrivateData.SouthboundAdapterVersion = relaycommon.CurrentVideoSouthboundAdapterVersion(constant.ChannelTypeSeedanceLink, protocol.TransportProfile())
	task.PrivateData.Key = "fixture-key"
	task.PrivateData.VideoUpstreamQueryBaseURL = "https://frozen.example"
	_, task.PrivateData.VideoUpstreamQueryPathTemplate = protocol.TransportPaths("seedance-2-fast")
	task.PrivateData.AsyncBilling.TieredSnapshot = tieredTestSnapshot(expr, 700)
	task.PrivateData.AsyncBilling.BillingProbe = &billingexpr.RequestInput{Body: []byte(`{"_task":{"duration_seconds":4}}`)}
	require.NoError(t, model.DB.Save(task).Error)
	return task
}

func TestSeedanceProtocolsRecoverUsageWithoutClient(t *testing.T) {
	for _, protocol := range recoveryProtocols {
		t.Run(string(protocol), func(t *testing.T) {
			task := persistedSeedanceBillingTask(t, protocol, 1000, `tier("tokens", c * 2)`)
			adaptor := &funCloudUsagePollingAdaptor{}
			old := GetTaskAdaptorFunc
			GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor { return adaptor }
			t.Cleanup(func() { GetTaskAdaptorFunc = old })
			require.NoError(t, RefreshVideoTask(context.Background(), task))
			waiting := reloadTask(t, task.ID)
			require.Equal(t, model.TaskStatus(model.TaskStatusSuccess), waiting.Status)
			require.Equal(t, model.TaskBillingStateAwaitingUsage, waiting.BillingState)
			assert.Nil(t, waiting.PrivateData.AsyncBilling.TargetQuota)
			assert.Equal(t, 700, waiting.Quota)
			_, _, err := model.ApplyTaskBillingTarget(waiting, 0)
			require.Error(t, err, "missing evidence must not allow a funding shortcut")
			ReconcileTaskUsage(context.Background())
			stillWaiting := reloadTask(t, task.ID)
			assert.Equal(t, 1, stillWaiting.UsageCheckAttempts)
			assert.Equal(t, model.TaskBillingStateAwaitingUsage, stillWaiting.BillingState)
			usage := 100
			adaptor.usage = &usage
			require.NoError(t, model.DB.Model(task).Update("usage_check_next_at", 0).Error)
			ReconcileTaskUsage(context.Background())
			settled := reloadTask(t, task.ID)
			assert.Equal(t, model.TaskBillingStateSettled, settled.BillingState)
			assert.Equal(t, 100, settled.Quota)
			assert.Equal(t, 1600, getUserQuota(t, task.UserId))
			// A stale GET and stale billing write cannot restore the hold or accepted usage.
			different := 200
			adaptor.usage = &different
			require.NoError(t, RefreshVideoTask(context.Background(), waiting))
			require.NoError(t, stillWaiting.UpdateBilling())
			settled = reloadTask(t, task.ID)
			assert.Equal(t, 100, settled.PrivateData.AsyncBilling.ActualTokens)
			assert.Equal(t, 100, settled.ToModelArkVideoTask().Usage.CompletionTokens)
			assert.Equal(t, model.TaskBillingStateSettled, settled.BillingState)
			assert.Equal(t, 100, settled.Quota)
			adaptor.fetchErr = assert.AnError
			require.Error(t, RefreshVideoTask(context.Background(), settled))
			assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), reloadTask(t, task.ID).Status)
			assert.Equal(t, 1600, getUserQuota(t, task.UserId))
		})
	}
}

func TestSeedanceProtocolsSettleFrozenSecondsWithoutUsage(t *testing.T) {
	for _, protocol := range recoveryProtocols {
		t.Run(string(protocol), func(t *testing.T) {
			task := persistedSeedanceBillingTask(t, protocol, 1000, `tier("seconds", param("_task.duration_seconds") * 100)`)
			old := GetTaskAdaptorFunc
			GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor { return &funCloudUsagePollingAdaptor{} }
			t.Cleanup(func() { GetTaskAdaptorFunc = old })
			require.NoError(t, RefreshVideoTask(context.Background(), task))
			saved := reloadTask(t, task.ID)
			assert.Equal(t, model.TaskBillingStateSettled, saved.BillingState)
			assert.Equal(t, 200, saved.Quota, "evaluate frozen expression rather than treating the 700 hold as the bill")
			assert.Equal(t, 1500, getUserQuota(t, task.UserId))
			assert.False(t, model.HasDueTaskUsageChecks(model.GetDBTimestamp()))
		})
	}
}

func TestSeedanceProtocolsDebtAndOperatorRecovery(t *testing.T) {
	for _, protocol := range recoveryProtocols {
		t.Run(string(protocol), func(t *testing.T) {
			task := persistedSeedanceBillingTask(t, protocol, 0, `tier("tokens", c * 2)`)
			task.Status = model.TaskStatusSuccess
			require.NoError(t, model.DB.Save(task).Error)
			require.True(t, settleTaskTieredSnapshot(context.Background(), task, 0))
			_, review, err := model.ClaimTaskUsageCheck(task.ID, model.GetDBTimestamp())
			require.NoError(t, err)
			assert.False(t, review)
			claim := reloadTask(t, task.ID)
			_, review, err = model.ClaimTaskUsageCheck(task.ID, claim.UsageCheckStartedAt+model.TaskUsageCheckWindowSeconds)
			require.NoError(t, err)
			assert.True(t, review)
			items, err := model.ListTaskUsageRecovery(true, 0, 50)
			require.NoError(t, err)
			require.Len(t, items, 1)
			usage := 1000
			stale := reloadTask(t, task.ID)
			_, err = model.ReviewTaskUsage(task.TaskID, 9, "verified-statement", &usage)
			require.NoError(t, err)
			require.Equal(t, 1, ReconcileTaskBilling(context.Background(), 10).Scanned)
			debt := reloadTask(t, task.ID)
			require.Equal(t, model.TaskBillingStateDebt, debt.BillingState)
			require.Equal(t, 1000, *debt.PrivateData.AsyncBilling.TargetQuota)
			require.NoError(t, stale.UpdateBilling())
			unchanged := reloadTask(t, task.ID)
			assert.Equal(t, model.TaskBillingStateDebt, unchanged.BillingState)
			assert.Equal(t, 1000, *unchanged.PrivateData.AsyncBilling.TargetQuota)
			require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", task.UserId).Update("quota", 500).Error)
			unchanged.PrivateData.AsyncBilling.NextRetryAt = 0
			require.NoError(t, unchanged.UpdateBilling())
			require.Equal(t, 1, ReconcileTaskBilling(context.Background(), 10).Scanned)
			assert.Equal(t, 200, getUserQuota(t, task.UserId))
			assert.Zero(t, ReconcileTaskBilling(context.Background(), 10).Scanned)
			assert.Equal(t, 1000, reloadTask(t, task.ID).Quota)
		})
	}
}

func TestNativeTaskDoesNotEnterSeedanceUsageRecovery(t *testing.T) {
	truncate(t)
	seedUser(t, 8161, 1000)
	task := persistedAsyncTask(t, 8161, 700, model.TaskStatusSuccess)
	task.PrivateData.AsyncBilling.TieredSnapshot = tieredTestSnapshot(`tier("tokens", c * 2)`, 700)
	// A familiar transport profile or customer model name does not grant Link identity.
	task.PrivateData.VideoUpstreamProfile = dto.VideoUpstreamProfileOfficial
	task.Properties.OriginModelName = "seedance-2"
	require.NoError(t, model.DB.Save(task).Error)
	require.True(t, settleTaskTieredSnapshot(context.Background(), task, 0))
	assert.Equal(t, model.TaskBillingStateSettled, reloadTask(t, task.ID).BillingState)
	assert.False(t, model.HasDueTaskUsageChecks(model.GetDBTimestamp()))
}

func TestSeedanceOperatorZeroRespectsProtocolContract(t *testing.T) {
	for _, protocol := range recoveryProtocols {
		t.Run(string(protocol), func(t *testing.T) {
			task := persistedSeedanceBillingTask(t, protocol, 1000, `tier("tokens",c*2)`)
			task.Status = model.TaskStatusSuccess
			require.NoError(t, model.DB.Save(task).Error)
			require.True(t, settleTaskTieredSnapshot(context.Background(), task, 0))
			zero := 0
			_, err := model.ReviewTaskUsage(task.TaskID, 9, "statement-zero", &zero)
			if protocol == dto.VideoUpstreamProtocolFunCloudSeedance {
				require.Error(t, err)
				assert.Equal(t, model.TaskBillingStateAwaitingUsage, reloadTask(t, task.ID).BillingState)
				assert.Equal(t, 1000, getUserQuota(t, task.UserId))
				return
			}
			require.NoError(t, err)
			require.Equal(t, 1, ReconcileTaskBilling(context.Background(), 10).Scanned)
			assert.Zero(t, reloadTask(t, task.ID).Quota)
			assert.Equal(t, 1700, getUserQuota(t, task.UserId))
			assert.Zero(t, ReconcileTaskBilling(context.Background(), 10).Scanned)
		})
	}
}
