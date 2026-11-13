package service

import (
	"context"
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestImageTaskSettlementPublishesTheChargedBranch(t *testing.T) {
	const expression = `p > 30 ? tier("token", p * 2 + c * 4) : (param("quality") == "low" ? tier("shared", fixed(0.01667)) * image_count : tier("shared", fixed(0.05484)))`
	for _, tc := range []struct {
		quality, mode string
		input, quota  int
		images        int64
	}{
		{"low", model.BillingReconciliationModePerImage, 20, 16670, 2},
		{"medium", model.BillingReconciliationModePerCall, 20, 27420, 0},
		{"low", model.BillingReconciliationModeToken, 40, 80, 0},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			truncate(t)
			seedUser(t, 41, 100000)
			task := newWorkerImageTask(41, 100)
			task.Properties.OriginModelName = "customer-image"
			task.PrivateData.ImageTask.ChannelType = constant.ChannelTypeOpenAI
			task.Status = model.TaskStatusSuccess
			task.PrivateData.ImageTask.ImageCount = 1
			task.PrivateData.ImageTask.Usage = &dto.Usage{PromptTokens: tc.input, CompletionTokens: 20, TotalTokens: tc.input + 20}
			snap := tieredTestSnapshot(expression, 100)
			task.PrivateData.BillingContext = &model.TaskBillingContext{TieredSnapshot: snap}
			task.PrivateData.AsyncBilling = &model.TaskAsyncBillingContext{State: model.TaskBillingStatePending, TieredSnapshot: snap}
			info := &relaycommon.RelayInfo{
				TieredBillingSnapshot: snap,
				BillingRequestInput:   &billingexpr.RequestInput{ImageCount: common.GetPointer(2)},
			}
			require.NoError(t, FreezeImageTaskBilling(t.Context(), task, info, &dto.ImageRequest{Quality: tc.quality}))
			quota, _, err := imageTaskTargetQuota(t.Context(), task, task.PrivateData.ImageTask.Usage)
			require.NoError(t, err)
			require.Equal(t, tc.quota, quota)
			persisted, err := common.DeepCopy(task)
			require.NoError(t, err)
			log, err := BuildTaskBillingDeliveryLog(persisted, model.TaskBillingDelivery{Event: "complete", AfterQuota: quota})
			require.NoError(t, err)
			log.CreatedAt, log.RequestId = 1100, "task-billing:41:complete"
			require.NoError(t, model.LOG_DB.Create(log).Error)
			statement, err := model.GetBillingCustomerStatement(t.Context(), 41, 1000, 1200, "api_key", 0, "", "")
			require.NoError(t, err)
			require.Len(t, statement.Groups, 1)
			require.Len(t, statement.Groups[0].Models, 1)
			line := statement.Groups[0].Models[0]
			assert.Equal(t, tc.mode, line.BillingMode)
			assert.Equal(t, tc.images, line.Usage.BillableImages)
			assert.EqualValues(t, tc.quota, line.Usage.NetQuota)
		})
	}
}

func TestImageBillingFreezesEncryptedRequestProbe(t *testing.T) {
	task := newWorkerImageTask(9905, 100)
	snapshot := tieredTestSnapshot(`tier("base", (p + c) * (param("model") == "customer-image" ? 2 : 1) * (header("x-factor") == "double" ? 2 : 1))`, 100)
	task.PrivateData.BillingContext = &model.TaskBillingContext{TieredSnapshot: snapshot}
	task.PrivateData.AsyncBilling = &model.TaskAsyncBillingContext{State: model.TaskBillingStatePending}
	info := &relaycommon.RelayInfo{
		OriginModelName: "customer-image", TieredBillingSnapshot: snapshot,
		BillingRequestInput: &billingexpr.RequestInput{Headers: map[string]string{"X-Factor": "double", "Authorization": "test-secret-not-for-persistence"}},
	}
	request := &dto.ImageRequest{Model: "gemini-3.1-flash-image", Prompt: "p", Size: "1024x1024"}
	require.NoError(t, FreezeImageTaskBilling(t.Context(), task, info, request))
	encoded, err := common.Marshal(task.PrivateData)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "test-secret-not-for-persistence")
	restored, err := common.DeepCopy(task)
	require.NoError(t, err)
	info.BillingRequestInput.Headers["X-Factor"] = "changed"
	quota, _, err := imageTaskTargetQuota(t.Context(), restored, &dto.Usage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150})
	require.NoError(t, err)
	assert.Equal(t, 300, quota, "customer model and headers must retain admission-time meaning")
}

func TestImageQuotaUsesActualTokensAndFrozenPrices(t *testing.T) {
	task := newWorkerImageTask(9901, 100)
	task.PrivateData.BillingContext = &model.TaskBillingContext{
		OriginModelName: "image-test-frozen", TieredSnapshot: tieredTestSnapshot(`tier("base", p * 2 + c * 4)`, 100),
	}
	usage := &dto.Usage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150}
	quota, _, err := imageTaskTargetQuota(t.Context(), task, usage)
	require.NoError(t, err)
	assert.Equal(t, 200, quota, "p and c must be actual usage, not empty TokenParams")
	task.PrivateData.BillingContext.TieredSnapshot = nil
	task.PrivateData.ImageTask.Price = &hosttypes.PriceData{
		ModelRatio: 2, CompletionRatio: 4, GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1},
	}
	previous := ratio_setting.GetModelRatioCopy()
	t.Cleanup(func() {
		encoded, err := common.Marshal(previous)
		require.NoError(t, err)
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(encoded)))
	})
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"image-test-frozen":999}`))
	quota, _, err = imageTaskTargetQuota(t.Context(), task, usage)
	require.NoError(t, err)
	assert.Equal(t, 600, quota, "settlement must not use the administrator's edited ratio")
	quota, _, err = imageTaskTargetQuota(t.Context(), task, nil)
	require.NoError(t, err)
	assert.Equal(t, 100, quota, "absent usage preserves the frozen hold")
	quota, _, err = imageTaskTargetQuota(t.Context(), task, &dto.Usage{})
	require.NoError(t, err)
	assert.Zero(t, quota, "explicit zero usage is not fabricated")
}

func TestImageSettlementRollbackAndRestartReconcile(t *testing.T) {
	truncate(t)
	seedImageTaskUser(t, 9902, 1000)
	seedToken(t, 9903, 9902, "image-settlement-token", 1000)
	seedChannel(t, 42)
	task := newWorkerImageTask(9902, 100)
	task.PrivateData.TokenId = 9903
	task.PrivateData.AsyncBilling = &model.TaskAsyncBillingContext{State: model.TaskBillingStatePending}
	task.PrivateData.BillingContext = &model.TaskBillingContext{
		OriginModelName: "image-test", TieredSnapshot: tieredTestSnapshot(`tier("base", p * 2 + c * 4)`, 100),
	}
	require.NoError(t, model.InsertImageTask(model.ImageTaskInsertParams{Task: task,
		GlobalScope: model.ImageTaskAdmissionScopeGlobal(), AppScope: model.ImageTaskAdmissionScopeApp(task.UserId, task.AppID)}))
	won, err := model.FinishImageTaskSuccess(task, []model.TaskImageArtifact{{ObjectKey: "images/tasks/test/result-0"}}, &dto.Usage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150, InputImages: common.GetPointer(2)})
	require.NoError(t, err)
	require.True(t, won)
	const callback = "image-settlement-fault"
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "tasks" {
			tx.AddError(errors.New("injected settlement write failure"))
		}
	}))
	t.Cleanup(func() { model.DB.Callback().Update().Remove(callback) })
	settleImageTaskBilling(context.Background(), task)
	assert.Equal(t, 900, getUserQuota(t, 9902), "failed task write must roll back wallet delta")
	assert.Equal(t, 900, getTokenRemainQuota(t, 9903))
	assert.Equal(t, model.TaskBillingStatePending, reloadTask(t, task.ID).PrivateData.AsyncBilling.State)
	require.NoError(t, model.DB.Callback().Update().Remove(callback))
	ReconcileTaskBilling(context.Background(), 100)
	ReconcileTaskBilling(context.Background(), 100)
	assert.Equal(t, 800, getUserQuota(t, 9902), "restart must apply exactly one 100-quota delta")
	assert.Equal(t, 800, getTokenRemainQuota(t, 9903))
	assert.Equal(t, 200, getTokenUsedQuota(t, 9903))
	persisted := reloadTask(t, task.ID)
	require.Equal(t, common.GetPointer(2), persisted.PrivateData.ImageTask.Usage.InputImages)
	assert.Equal(t, 200, persisted.Quota)
	assert.Equal(t, model.TaskBillingStateSettled, persisted.PrivateData.AsyncBilling.State)
	require.NoError(t, model.RebuildImageTaskSlots())
	var slot model.ImageTaskSlot
	require.NoError(t, model.DB.First(&slot, "scope = ?", model.ImageTaskAdmissionScopeGlobal()).Error)
	assert.Zero(t, slot.Count, "settlement and capacity release must commit together")
	var user model.User
	require.NoError(t, model.DB.First(&user, 9902).Error)
	assert.Equal(t, 200, user.UsedQuota)
	assert.Equal(t, 1, user.RequestCount)
}

func TestImageRelayEditBillingFreezesReferenceCount(t *testing.T) {
	task := newWorkerImageTask(9991, 44286)
	snapshot := tieredTestSnapshot(`tier("2K", (0.60 + (param("input_image_count") == nil ? 0 : max(param("input_image_count") - 1, 0) * 0.02)) / 7 * 1000000)`, 44286)
	task.PrivateData.BillingContext = &model.TaskBillingContext{TieredSnapshot: snapshot}
	task.PrivateData.AsyncBilling = &model.TaskAsyncBillingContext{State: model.TaskBillingStatePending}
	task.PrivateData.ImageTask.Inputs = []model.TaskImageInputRef{{URL: "https://example.com/a"}, {URL: "https://example.com/b"}}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeAsyncImage}, OriginModelName: "local-pro", TieredBillingSnapshot: snapshot}
	request := &dto.ImageRequest{Model: "mapped-pro", Prompt: "edit", Images: []byte(`["https://example.com/a","https://example.com/b"]`)}
	require.NoError(t, FreezeImageTaskBilling(t.Context(), task, info, request))
	quota, clamp, err := imageTaskTargetQuota(t.Context(), task, &dto.Usage{CompletionTokens: 16384, TotalTokens: 16384, InputImages: common.GetPointer(2)})
	require.NoError(t, err)
	assert.Nil(t, clamp)
	assert.Equal(t, 44286, quota)
	assert.Empty(t, task.PrivateData.ImageTask.Parameters.Images)
}
