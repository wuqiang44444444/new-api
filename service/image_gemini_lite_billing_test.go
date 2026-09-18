package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeminiLiteImageBillingSeparatesTextAndImageOutput(t *testing.T) {
	const expression = `tier("base", p * 0.25 + cr * 0.025 + c * 1.5 + img_o * 30)`
	var metadata dto.GeminiUsageMetadata
	require.NoError(t, common.Unmarshal([]byte(`{"promptTokenCount":1200,"cachedContentTokenCount":40,"candidatesTokenCount":1140,"thoughtsTokenCount":10,"totalTokenCount":2350,"promptTokensDetails":[{"modality":"IMAGE","tokenCount":1120},{"modality":"TEXT","tokenCount":80}],"candidatesTokensDetails":[{"modality":"IMAGE","tokenCount":1120},{"modality":"TEXT","tokenCount":20}]}`), &metadata))
	usage := relayconvert.UsageFromGeminiMetadata(&metadata, 0)
	params := BuildTieredTokenParams(usage, false, billingexpr.UsedVars(expression))
	assert.Equal(t, float64(1160), params.P)
	assert.Equal(t, float64(40), params.CR)
	assert.Equal(t, float64(30), params.C)
	assert.Equal(t, float64(1120), params.ImgO)
	snapshot := tieredTestSnapshot(expression, 20000)
	result, err := billingexpr.ComputeTieredQuotaWithRequest(snapshot, params, billingexpr.RequestInput{})
	require.NoError(t, err)
	// USD: (1160*.25 + 40*.025 + 30*1.5 + 1120*30) / 1M = .033936.
	assert.InDelta(t, 16968.0, result.ActualQuotaBeforeGroup, 0.000001)
	// PostTextConsumeQuota first reconstructs canonical provider billing usage.
	// Prove that this synchronous path and the frozen async calculation agree.
	syncParams := BuildTieredTokenParams(effectiveBillingUsage(usage), false, billingexpr.UsedVars(expression))
	applied, syncQuota, syncResult := TryTieredSettle(&relaycommon.RelayInfo{TieredBillingSnapshot: snapshot}, syncParams)
	require.True(t, applied)
	require.NotNil(t, syncResult)
	assert.Equal(t, 16968, syncQuota)
	require.NoError(t, ValidateGeminiImageUsage("gemini-3.1-flash-lite-image", usage))
	task := newWorkerImageTask(9917, 20000)
	task.PrivateData.ImageTask.UpstreamModel = "gemini-3.1-flash-lite-image"
	task.PrivateData.BillingContext = &model.TaskBillingContext{TieredSnapshot: snapshot}
	quota, clamp, err := imageTaskTargetQuota(t.Context(), task, usage)
	require.NoError(t, err)
	assert.Nil(t, clamp)
	assert.Equal(t, 16968, quota, "async uses the frozen expression and same modality normalization")
}

func TestGeminiLiteTrustedUsageSettlesWalletAndTokenOnce(t *testing.T) {
	truncate(t)
	const userID = 9933
	seedImageTaskUser(t, userID, 100000)
	token := &model.Token{UserId: userID, Key: common.GetUUID(), RemainQuota: 100000}
	require.NoError(t, model.DB.Create(token).Error)
	task := newWorkerImageTask(userID, 20000)
	task.PrivateData.ImageTask.UpstreamModel = "gemini-3.1-flash-lite-image"
	task.PrivateData.TokenId = token.Id
	task.PrivateData.AsyncBilling = &model.TaskAsyncBillingContext{State: model.TaskBillingStatePending}
	task.PrivateData.BillingContext = &model.TaskBillingContext{OriginModelName: "lite", TieredSnapshot: tieredTestSnapshot(`tier("base", p * 0.25 + cr * 0.025 + c * 1.5 + img_o * 30)`, 20000)}
	require.NoError(t, model.InsertImageTask(model.ImageTaskInsertParams{Task: task, GlobalScope: model.ImageTaskAdmissionScopeGlobal(), AppScope: model.ImageTaskAdmissionScopeApp(userID, 7)}))
	var metadata dto.GeminiUsageMetadata
	require.NoError(t, common.Unmarshal([]byte(`{"promptTokenCount":1200,"cachedContentTokenCount":40,"candidatesTokenCount":1140,"thoughtsTokenCount":10,"totalTokenCount":2350,"candidatesTokensDetails":[{"modality":"IMAGE","tokenCount":1120},{"modality":"TEXT","tokenCount":20}]}`), &metadata))
	usage := relayconvert.UsageFromGeminiMetadata(&metadata, 0)
	won, err := model.FinishImageTaskSuccess(task, []model.TaskImageArtifact{{ObjectKey: "fixture/result.png"}}, usage)
	require.NoError(t, err)
	require.True(t, won)
	settleImageTaskBilling(t.Context(), task)
	ReconcileTaskBilling(t.Context(), 100)
	stored := reloadTask(t, task.ID)
	assert.Equal(t, model.TaskBillingStateSettled, stored.PrivateData.AsyncBilling.State)
	assert.Equal(t, 16968, stored.Quota)
	assert.Equal(t, 83032, getUserQuota(t, userID))
	var savedToken model.Token
	require.NoError(t, model.DB.First(&savedToken, token.Id).Error)
	assert.Equal(t, 83032, savedToken.RemainQuota)
	assert.Equal(t, 16968, savedToken.UsedQuota)
}
