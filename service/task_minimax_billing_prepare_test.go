package service

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func minimaxBillingTask(t *testing.T) *model.Task {
	t.Helper()
	task := &model.Task{TaskID: "task_minimax_billing", Platform: constant.TaskPlatform(model.MiniMaxLinkTaskPlatform()),
		PrivateData: model.TaskPrivateData{VideoUpstreamProtocol: "jdcloud_video_task_v1"}}
	model.AttachAsyncTaskBilling(&task.PrivateData, &relaycommon.RelayInfo{
		ChannelMeta:           &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeMiniMaxLink},
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{ExprString: `tier("fixed", 6)`, TaskUsageBilling: true},
	}, 0)
	require.NotNil(t, task.PrivateData.AsyncBilling)
	require.True(t, task.HasTaskUsageBilling())
	return task
}

// A MiniMax terminal observation must keep its credit usage evidence and its
// refundable-terminal operation through the terminal billing preparation:
// the frozen typed identity has usage billing without Seedance facts, which
// previously made the preparation a no-op and silently dropped both.
func TestPrepareTerminalTaskBillingKeepsMinimaxCreditEvidence(t *testing.T) {
	task := minimaxBillingTask(t)
	task.Status = model.TaskStatusSuccess
	result := &relaycommon.TaskInfo{
		Status:        "SUCCESS",
		UsageSource:   "credit:video_output",
		UsageEvidence: map[string]int{"video_output": 6},
	}
	prepareTerminalTaskBilling(task, result)
	async := task.PrivateData.AsyncBilling
	require.NotNil(t, async)
	assert.Equal(t, "credit:video_output", async.ActualUsageSource)
	assert.Equal(t, map[string]int{"video_output": 6}, async.ActualUsageEvidence)
	assert.Empty(t, async.Operation, "success keeps the settle direction")

	task = minimaxBillingTask(t)
	task.Status = model.TaskStatusFailure
	task.FailReason = "content policy"
	result = &relaycommon.TaskInfo{Status: "FAILURE", Reason: "content policy"}
	prepareTerminalTaskBilling(task, result)
	async = task.PrivateData.AsyncBilling
	require.NotNil(t, async)
	assert.Equal(t, "refund", async.Operation)
	assert.NotEmpty(t, async.Reason)
}
