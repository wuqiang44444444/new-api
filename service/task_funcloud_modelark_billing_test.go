package service

import (
	"context"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestSecondsExpressionPreservesTokenEvidenceWithoutChangingCharge(t *testing.T) {
	truncate(t)
	const userID = 8119
	seedUser(t, userID, 1000)
	task := persistedAsyncTask(t, userID, 200000, model.TaskStatusSuccess)
	task.PrivateData.AsyncBilling.TieredSnapshot = tieredTestSnapshot(`tier("seconds", param("_task.duration_seconds") * 100000)`, 200000)
	task.PrivateData.AsyncBilling.BillingProbe = &billingexpr.RequestInput{Body: []byte(`{"_task":{"duration_seconds":4}}`)}
	prepareTerminalTaskBilling(task, &relaycommon.TaskInfo{CompletionTokens: 38800, CompletionTokensReported: true, UsageReported: true, UsageSource: "usage.completion_tokens", UsageEvidence: map[string]int{"usage.completion_tokens": 38800}})
	require.NoError(t, model.DB.Save(task).Error)
	require.True(t, settleTaskTieredSnapshot(context.Background(), task, 38800))
	stored := reloadTask(t, task.ID)
	assert.Equal(t, 200000, stored.Quota)
	assert.Equal(t, 38800, stored.PrivateData.AsyncBilling.ActualTokens)
	assert.Equal(t, "usage.completion_tokens", stored.PrivateData.AsyncBilling.ActualUsageSource)
	assert.Equal(t, map[string]int{"usage.completion_tokens": 38800}, stored.PrivateData.AsyncBilling.ActualUsageEvidence)
	assert.Equal(t, 1000, getUserQuota(t, userID))
	assert.Equal(t, model.TaskBillingStateSettled, stored.PrivateData.AsyncBilling.State)
}

type funCloudUsagePollingAdaptor struct {
	taskPollingFetchAdaptor
	usage    *int
	fetchErr error
}

func (a *funCloudUsagePollingAdaptor) FetchTask(string, string, *model.Task, string) (*http.Response, error) {
	if a.fetchErr != nil {
		return nil, a.fetchErr
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"status":"succeeded"}`))}, nil
}

func (a *funCloudUsagePollingAdaptor) ParseTaskResult(*model.Task, *http.Response, []byte) (*relaycommon.TaskInfo, error) {
	result := &relaycommon.TaskInfo{Status: model.TaskStatusSuccess, Url: "https://example.com/video.mp4"}
	if a.usage != nil {
		result.CompletionTokens = *a.usage
		result.CompletionTokensReported = true
		result.UsageReported = true
		result.UsageSource = "usage.completion_tokens"
	}
	return result, nil
}

func TestFunCloudModelArkPollingCollectsLateUsage(t *testing.T) {
	truncate(t)
	seedUser(t, 8123, 1000)
	task := persistedAsyncTask(t, 8123, 700, model.TaskStatusInProgress)
	task.PrivateData.VideoUpstreamProfile = dto.VideoUpstreamProfileThirdPartyFunCloudModelArkV3
	task.PrivateData.VideoUpstreamProtocol = dto.VideoUpstreamProtocolFunCloudModelArkV3
	task.PrivateData.SouthboundAdapterVersion = relaycommon.CurrentVideoSouthboundAdapterVersion(constant.ChannelTypeSeedanceLink, task.PrivateData.VideoUpstreamProfile)
	task.PrivateData.AsyncBilling.TieredSnapshot = tieredTestSnapshot(`tier("tokens", c * 2)`, 700)
	require.NoError(t, model.DB.Save(task).Error)
	channel := &model.Channel{Type: constant.ChannelTypeSeedanceLink}
	adaptor := &funCloudUsagePollingAdaptor{}
	require.NoError(t, updateVideoSingleTask(context.Background(), adaptor, channel, task.TaskID, map[string]*model.Task{task.TaskID: task}))
	waiting := reloadTask(t, task.ID)
	assert.Equal(t, model.TaskBillingStateAwaitingUsage, waiting.PrivateData.AsyncBilling.State)
	assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), waiting.Status)
	previousLimit := constant.TaskPollMaxFailures
	constant.TaskPollMaxFailures = 1
	t.Cleanup(func() { constant.TaskPollMaxFailures = previousLimit })
	adaptor.fetchErr = assert.AnError
	require.Error(t, updateVideoSingleTask(context.Background(), adaptor, channel, task.TaskID, map[string]*model.Task{task.TaskID: waiting}))
	unchanged := reloadTask(t, task.ID)
	assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), unchanged.Status)
	assert.Equal(t, model.TaskBillingStateAwaitingUsage, unchanged.PrivateData.AsyncBilling.State)
	assert.Equal(t, 1000, getUserQuota(t, 8123))
	adaptor.fetchErr = nil
	usage := 100
	adaptor.usage = &usage
	require.NoError(t, updateVideoSingleTask(context.Background(), adaptor, channel, task.TaskID, map[string]*model.Task{task.TaskID: waiting}))
	settled := reloadTask(t, task.ID)
	assert.Equal(t, model.TaskBillingStateSettled, settled.PrivateData.AsyncBilling.State)
	assert.Equal(t, 100, settled.Quota)
	assert.Equal(t, 1600, getUserQuota(t, 8123))
	adaptor.fetchErr = assert.AnError
	require.Error(t, updateVideoSingleTask(context.Background(), adaptor, channel, task.TaskID, map[string]*model.Task{task.TaskID: settled}))
	assert.Equal(t, model.TaskBillingStateSettled, reloadTask(t, task.ID).PrivateData.AsyncBilling.State)
	assert.Equal(t, 1600, getUserQuota(t, 8123))
}

func TestFunCloudModelArkMissingUsageWaitsThenSettlesOnce(t *testing.T) {
	truncate(t)
	const userID = 8120
	seedUser(t, userID, 1000)
	task := persistedAsyncTask(t, userID, 700, model.TaskStatusSuccess)
	task.PrivateData.VideoUpstreamProfile = dto.VideoUpstreamProfileThirdPartyFunCloudModelArkV3
	task.PrivateData.VideoUpstreamProtocol = dto.VideoUpstreamProtocolFunCloudModelArkV3
	task.PrivateData.AsyncBilling.TieredSnapshot = tieredTestSnapshot(`tier("tokens", c * 2)`, 700)
	require.NoError(t, model.DB.Save(task).Error)
	require.True(t, settleTaskTieredSnapshot(context.Background(), task, 0))
	waiting := reloadTask(t, task.ID)
	assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), waiting.Status)
	assert.Equal(t, model.TaskBillingStateAwaitingUsage, waiting.PrivateData.AsyncBilling.State)
	assert.Nil(t, waiting.PrivateData.AsyncBilling.TargetQuota)
	assert.Equal(t, 700, waiting.Quota)
	assert.Zero(t, ReconcileTaskBilling(context.Background(), 10).Scanned)
	assert.Equal(t, 1000, getUserQuota(t, userID))
	prepareTerminalTaskBilling(waiting, &relaycommon.TaskInfo{CompletionTokens: 100, CompletionTokensReported: true, UsageReported: true, UsageSource: "usage.completion_tokens"})
	require.NoError(t, waiting.UpdateBilling())
	require.True(t, settleTaskTieredSnapshot(context.Background(), waiting, 100))
	settled := reloadTask(t, task.ID)
	assert.Equal(t, model.TaskBillingStateSettled, settled.PrivateData.AsyncBilling.State)
	assert.Equal(t, 100, settled.Quota)
	assert.Equal(t, 1600, getUserQuota(t, userID))
	prepareTerminalTaskBilling(settled, &relaycommon.TaskInfo{CompletionTokens: 200, UsageReported: true})
	require.True(t, settleTaskTieredSnapshot(context.Background(), settled, 200))
	assert.Equal(t, 100, settled.PrivateData.AsyncBilling.ActualTokens)
	assert.Equal(t, 1600, getUserQuota(t, userID))
}

func TestFunCloudModelArkFixedPriceAndReportedZeroDoNotWait(t *testing.T) {
	for _, expr := range []string{`tier("fixed", 400)`, `tier("tokens", c * 2)`} {
		t.Run(expr, func(t *testing.T) {
			truncate(t)
			seedUser(t, 8121, 1000)
			task := persistedAsyncTask(t, 8121, 200, model.TaskStatusSuccess)
			task.PrivateData.VideoUpstreamProfile = dto.VideoUpstreamProfileThirdPartyFunCloudModelArkV3
			task.PrivateData.VideoUpstreamProtocol = dto.VideoUpstreamProtocolFunCloudModelArkV3
			task.PrivateData.AsyncBilling.TieredSnapshot = tieredTestSnapshot(expr, 200)
			task.PrivateData.AsyncBilling.ActualUsageReported = expr == `tier("tokens", c * 2)`
			require.NoError(t, model.DB.Save(task).Error)
			require.True(t, settleTaskTieredSnapshot(context.Background(), task, 0))
			stored := reloadTask(t, task.ID)
			assert.Equal(t, model.TaskBillingStateSettled, stored.PrivateData.AsyncBilling.State)
			if stored.PrivateData.AsyncBilling.ActualUsageReported {
				assert.Zero(t, stored.Quota)
			} else {
				assert.Equal(t, 200, stored.Quota)
			}
		})
	}
}

func TestTieredNegativeSettlementDoesNotPersistTarget(t *testing.T) {
	truncate(t)
	seedUser(t, 8122, 1000)
	task := persistedAsyncTask(t, 8122, 200, model.TaskStatusSuccess)
	task.PrivateData.AsyncBilling.TieredSnapshot = tieredTestSnapshot(`tier("negative", 100 - c * 2)`, 200)
	require.NoError(t, model.DB.Save(task).Error)
	require.True(t, settleTaskTieredSnapshot(context.Background(), task, 100))
	stored := reloadTask(t, task.ID)
	assert.Equal(t, model.TaskBillingStateFailed, stored.PrivateData.AsyncBilling.State)
	assert.Nil(t, stored.PrivateData.AsyncBilling.TargetQuota)
	assert.Equal(t, 200, stored.Quota)
	assert.Equal(t, 1000, getUserQuota(t, 8122))
}
