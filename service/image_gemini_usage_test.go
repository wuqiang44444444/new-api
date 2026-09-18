package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeminiLiteUsageRequiresClassifiedOutput(t *testing.T) {
	for _, tc := range []struct {
		name, metadata string
		valid          bool
	}{
		{"image only", `{"promptTokenCount":10,"candidatesTokenCount":1120,"totalTokenCount":1130,"candidatesTokensDetails":[{"modality":"IMAGE","tokenCount":1120}]}`, true},
		{"unclassified", `{"promptTokenCount":10,"candidatesTokenCount":1120,"totalTokenCount":1130}`, false},
		{"missing output category", `{"promptTokenCount":10,"candidatesTokenCount":1140,"totalTokenCount":1150,"candidatesTokensDetails":[{"modality":"IMAGE","tokenCount":1120}]}`, false},
		{"wrong total", `{"promptTokenCount":10,"candidatesTokenCount":1120,"totalTokenCount":1120,"candidatesTokensDetails":[{"modality":"IMAGE","tokenCount":1120}]}`, false},
		{"cache exceeds input", `{"promptTokenCount":10,"cachedContentTokenCount":11,"candidatesTokenCount":1120,"totalTokenCount":1130,"candidatesTokensDetails":[{"modality":"IMAGE","tokenCount":1120}]}`, false},
		{"negative", `{"promptTokenCount":-10,"candidatesTokenCount":1120,"totalTokenCount":1110,"candidatesTokensDetails":[{"modality":"IMAGE","tokenCount":1120}]}`, false},
		{"oversized", `{"promptTokenCount":2147483648,"candidatesTokenCount":1120,"totalTokenCount":2147484768,"candidatesTokensDetails":[{"modality":"IMAGE","tokenCount":1120}]}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var metadata dto.GeminiUsageMetadata
			require.NoError(t, common.Unmarshal([]byte(tc.metadata), &metadata))
			err := ValidateGeminiImageUsage("gemini-3.1-flash-lite-image", relayconvert.UsageFromGeminiMetadata(&metadata, 0))
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
	require.Error(t, ValidateGeminiImageUsage("gemini-3.1-flash-lite-image", nil))
	require.NoError(t, ValidateGeminiImageUsage("gemini-3.1-flash-image", nil), "other models retain their existing contract")
}

func TestGeminiLiteIncompleteUsageKeepsBillingPending(t *testing.T) {
	for _, absent := range []bool{true, false} {
		t.Run(map[bool]string{true: "absent", false: "unclassified"}[absent], func(t *testing.T) {
			truncate(t)
			const userID = 9932
			seedImageTaskUser(t, userID, 100000)
			task := newWorkerImageTask(userID, 20000)
			task.PrivateData.ImageTask.UpstreamModel = "gemini-3.1-flash-lite-image"
			task.PrivateData.AsyncBilling = &model.TaskAsyncBillingContext{State: model.TaskBillingStatePending}
			task.PrivateData.BillingContext = &model.TaskBillingContext{OriginModelName: "lite", TieredSnapshot: tieredTestSnapshot(`tier("base", p * 0.25 + cr * 0.025 + c * 1.5 + img_o * 30)`, 20000)}
			require.NoError(t, model.InsertImageTask(model.ImageTaskInsertParams{Task: task, GlobalScope: model.ImageTaskAdmissionScopeGlobal(), AppScope: model.ImageTaskAdmissionScopeApp(userID, 7)}))
			var usage *dto.Usage
			if !absent {
				usage = &dto.Usage{PromptTokens: 10, CompletionTokens: 1120, TotalTokens: 1130}
			}
			won, err := model.FinishImageTaskSuccess(task, []model.TaskImageArtifact{{ObjectKey: "fixture/result.png"}}, usage)
			require.NoError(t, err)
			require.True(t, won)
			settleImageTaskBilling(t.Context(), task)
			ReconcileTaskBilling(t.Context(), 100)
			stored := reloadTask(t, task.ID)
			assert.Equal(t, model.TaskBillingStatePending, stored.PrivateData.AsyncBilling.State)
			assert.Nil(t, stored.PrivateData.AsyncBilling.TargetQuota)
			assert.Equal(t, 80000, getUserQuota(t, userID), "hold stays intact; no estimate or text-only settlement")
		})
	}
}
