package service

import (
	"context"
	"fmt"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedImageTaskUser(t *testing.T, userID, quota int) {
	t.Helper()
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "img-user", Quota: quota, Status: common.UserStatusEnabled}).Error)
}

func newWorkerImageTask(userID, quota int) *model.Task {
	execution := &model.TaskImageExecutionData{
		Operation: "generations", Prompt: "p", N: 1,
		ChannelType: 24, ChannelBaseUrl: "https://g.example.com", ChannelKey: "k",
		UpstreamModel: "gemini-3.1-flash-image", FundsHeld: true, HeldQuota: quota,
	}
	return &model.Task{
		TaskID: model.GenerateTaskID(), Platform: model.ImageTaskPlatform(24),
		UserId: userID, AppID: 7, Group: "default", ChannelId: 42,
		Action: model.TaskActionImageGeneration, Status: model.TaskStatusQueued,
		ClientProtocol: model.TaskClientProtocolImageOpenAIV1,
		SubmitTime:     common.GetTimestamp(), Progress: "0%", Quota: quota,
		PrivateData: model.TaskPrivateData{ImageTask: execution, AppID: 7},
	}
}

// 有真实预扣的失败任务：退款恰好一次、金额恰好为预扣额。
func TestFundedImageTaskFailureRefundsExactlyOnce(t *testing.T) {
	truncate(t)
	userID := 9102
	seedImageTaskUser(t, userID, 200)

	task := newWorkerImageTask(userID, 150)
	task.PrivateData.AsyncBilling = &model.TaskAsyncBillingContext{State: model.TaskBillingStatePending}
	task.PrivateData.BillingContext = &model.TaskBillingContext{OriginModelName: "m"}
	task.PrivateData.TokenId = 0
	require.NoError(t, model.InsertImageTask(model.ImageTaskInsertParams{
		Task: task, GlobalScope: model.ImageTaskAdmissionScopeGlobal(), AppScope: model.ImageTaskAdmissionScopeApp(userID, 7),
	}))

	won, err := model.FinishImageTaskFailure(task, model.TaskStatusFailure, "queue_expired")
	require.NoError(t, err)
	require.True(t, won)

	// 终态触发退款（worker 语义）。
	settleImageTaskBilling(context.Background(), task)
	ReconcileTaskBilling(context.Background(), 100) // 二次扫描不得重复退款

	var user model.User
	require.NoError(t, model.DB.Where("id = ?", userID).First(&user).Error)
	assert.Equal(t, 200, user.Quota, "pre-consumed quota must be refunded exactly once")
}

func TestImageRecoveryRotatesPastInconclusiveBatch(t *testing.T) {
	truncate(t)
	seedImageTaskUser(t, 9920, 1000)
	var tasks []*model.Task
	for i := 0; i < 9; i++ {
		task := newWorkerImageTask(9920, 10)
		task.ChannelId = i + 1
		task.Status = model.TaskStatusReconciliationRequired
		task.PrivateData.ImageTask.GenerationComplete = true
		task.PrivateData.ImageTask.ProviderTaskID = ""
		task.UpdatedAt = 1
		require.NoError(t, model.DB.Create(task).Error)
		tasks = append(tasks, task)
	}
	previous := ImageTaskResumePollFunc
	t.Cleanup(func() { ImageTaskResumePollFunc = previous })
	ImageTaskResumePollFunc = func(ctx context.Context, task *model.Task) ImageTaskExecution {
		if task.TaskID == tasks[8].TaskID {
			return ImageTaskExecution{Outcome: ImageTaskOutcomeSuccess, Images: []model.TaskImageArtifact{{ObjectKey: fmt.Sprintf("images/tasks/%s/result-0", task.TaskID)}}}
		}
		return ImageTaskExecution{Outcome: ImageTaskOutcomeUnknown, FailureCode: "no_provider_task_id"}
	}
	config := system_setting.LoadImageTaskConfig()
	config.WorkerBatch, config.ExecConcurrency, config.ChannelExec = 8, 8, 1
	resumeImageTaskUnknowns(context.Background(), config)
	assert.Equal(t, model.TaskStatusReconciliationRequired, reloadTask(t, tasks[8].ID).Status)
	resumeImageTaskUnknowns(context.Background(), config)
	assert.EqualValues(t, model.TaskStatusSuccess, reloadTask(t, tasks[8].ID).Status)
	for _, task := range tasks[:8] {
		stored := reloadTask(t, task.ID)
		assert.Equal(t, model.TaskStatusReconciliationRequired, stored.Status)
		assert.True(t, stored.PrivateData.ImageTask.FundsHeld)
	}
	assert.Equal(t, 1000, getUserQuota(t, 9920), "recovery does not invent a refund")
}

func TestImageUnknownExecutionKeepsWalletAndTokenHolds(t *testing.T) {
	truncate(t)
	seedImageTaskUser(t, 9921, 1000)
	token := &model.Token{UserId: 9921, Key: common.GetUUID(), RemainQuota: 1000}
	require.NoError(t, model.DB.Create(token).Error)
	task := newWorkerImageTask(9921, 100)
	task.PrivateData.TokenId = token.Id
	task.PrivateData.AsyncBilling = &model.TaskAsyncBillingContext{State: model.TaskBillingStatePending}
	require.NoError(t, model.InsertImageTask(model.ImageTaskInsertParams{Task: task, GlobalScope: model.ImageTaskAdmissionScopeGlobal(), AppScope: model.ImageTaskAdmissionScopeApp(task.UserId, task.AppID)}))
	claimed, won, err := model.ClaimImageTask(task.TaskID, model.ImageTaskExecutionScopeGlobal(), 1, model.ImageTaskExecutionScopeChannel(task.ChannelId), 1)
	require.NoError(t, err)
	require.True(t, won)
	previous := ImageTaskExecuteFunc
	t.Cleanup(func() { ImageTaskExecuteFunc = previous })
	ImageTaskExecuteFunc = func(context.Context, *model.Task) ImageTaskExecution {
		return ImageTaskExecution{Outcome: ImageTaskOutcomeUnknown, ProviderTaskID: "accepted-id", FailureCode: "poll_inconclusive"}
	}
	executeImageTask(context.Background(), claimed, system_setting.LoadImageTaskConfig())
	ReconcileTaskBilling(context.Background(), 100)
	stored := reloadTask(t, task.ID)
	assert.Equal(t, model.TaskStatusReconciliationRequired, stored.Status)
	assert.True(t, stored.PrivateData.ImageTask.FundsHeld)
	assert.Equal(t, model.TaskBillingStatePending, stored.PrivateData.AsyncBilling.State)
	assert.Equal(t, 900, getUserQuota(t, 9921))
	var savedToken model.Token
	require.NoError(t, model.DB.First(&savedToken, token.Id).Error)
	assert.Equal(t, 900, savedToken.RemainQuota)
	assert.Equal(t, 100, savedToken.UsedQuota)
}
