package controller

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const seedanceDraftRequest = `{"model":"customer-video","content":[{"type":"text","text":"A landscape"}],"duration":4,"draft":true}`

func seedanceDraftFixture(t *testing.T, protocol dto.VideoUpstreamProtocol, providerModel string) *seedanceFundsFixture {
	t.Helper()
	fx := newSeedanceFundsFixtureOnDatabase(t, openTaskDialectDatabase, &model.ProviderCostExposure{}, &model.TaskVideoResultObject{}, &model.ErrorEvent{})
	var channel model.Channel
	require.NoError(t, fx.db.First(&channel, fx.channelID).Error)
	channel.ModelMapping = common.GetPointer(fmt.Sprintf(`{"customer-video":%q}`, providerModel))
	channel.SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamProtocol: protocol})
	require.NoError(t, fx.db.Save(&channel).Error)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_expr": `{"customer-video":"tier(\"fixture\", u(\"tokens\") / 1000000)"}`,
	}))
	return fx
}

func seedanceDraftResult(id, resolution, marker string, tokens int) string {
	return fmt.Sprintf(`{"id":%q,"status":"succeeded","created_at":1795012800,"duration":4,"resolution":%q,"content":{"video_url":"https://fixture.example/result.mp4"},"usage":{"completion_tokens":%d,"total_tokens":%d}%s}`,
		id, resolution, tokens, tokens, marker)
}

func TestSeedanceDraftCreatesIndependentFinalOnPinnedProvider(t *testing.T) {
	for _, tc := range []struct {
		protocol dto.VideoUpstreamProtocol
		model    string
		prefix   string
	}{
		{dto.VideoUpstreamProtocolViduModelArkV3, "viduq3.1-drama-std", "/api/v3/contents/generations/tasks"},
		{dto.VideoUpstreamProtocolFunCloudModelArkV3, "seedance-2-5", "/api/v3/contents/generations/tasks"},
		{dto.VideoUpstreamProtocolModelArkV3Volcengine, "ep-reviewed-deployment", "/api/v3/contents/generations/tasks"},
		{dto.VideoUpstreamProtocolModelArkV3BytePlus, "seedance-reviewed-deployment", "/api/v3/contents/generations/tasks"},
		{dto.VideoUpstreamProtocolNexhinaModelArkV3, "doubao-seedance-2-0-mini-260615", "/seedance/api/v3/contents/generations/tasks"},
		{dto.VideoUpstreamProtocolArkMediaV1, "ep-reviewed", "/v1/ark/media/generations"},
	} {
		t.Run(string(tc.protocol), func(t *testing.T) {
			fx := seedanceDraftFixture(t, tc.protocol, tc.model)
			sourceID := decodeSeedanceFundsCreateID(t, fx.submitCreateBody(seedanceDraftRequest))
			assert.Contains(t, string(fx.createRequestBody()), `"draft":true`)
			assert.Contains(t, string(fx.createRequestBody()), `"resolution":"480p"`)
			require.Equal(t, seedanceFundsInitialQuota-50000, fx.userQuota())

			finalRequest := fmt.Sprintf(`{"model":"customer-video","content":[{"type":"draft_task","draft_task":{"id":%q}}],"resolution":"1080p","draft":false}`, sourceID)
			assert.Contains(t, fx.submitCreateBody(finalRequest), `"invalid_draft_task"`)
			assert.EqualValues(t, 1, fx.createCalls.Load())

			fx.queryBodyRes.Store(seedanceDraftResult("prov-task-1", "480p", "", 38830))
			fx.pollOnce(sourceID)
			assert.Equal(t, seedanceFundsInitialQuota-50000, fx.userQuota())
			fx.queryBodyRes.Store(seedanceDraftResult("prov-task-1", "480p", `,"draft":true`, 38830))
			fx.pollOnce(sourceID)
			source := fx.loadTask(sourceID)
			require.EqualValues(t, model.TaskStatusSuccess, source.Status)
			require.Equal(t, 19415, source.Quota)
			assert.Equal(t, tc.prefix, source.PrivateData.VideoDraftCreatePath)
			assert.Equal(t, true, *source.ToModelArkVideoTask().Draft)

			require.NoError(t, fx.db.Model(&source).Update("app_id", fx.userID+1).Error)
			assert.Contains(t, fx.submitCreateBody(finalRequest), `"invalid_draft_task"`)
			require.NoError(t, fx.db.Model(&source).Update("app_id", fx.userID).Error)
			require.NoError(t, fx.db.Model(&model.Channel{}).Where("id = ?", fx.channelID).Update("key", "rotated-fixture-key").Error)
			assert.Contains(t, fx.submitCreateBody(finalRequest), `"invalid_draft_task"`)
			require.NoError(t, fx.db.Model(&model.Channel{}).Where("id = ?", fx.channelID).Update("key", source.PrivateData.Key).Error)
			assert.EqualValues(t, 1, fx.createCalls.Load())
			assert.Equal(t, seedanceFundsInitialQuota-source.Quota, fx.userQuota())

			fx.createBodyRes.Store(`{"id":"prov-final-1"}`)
			finalID := decodeSeedanceFundsCreateID(t, fx.submitCreateBody(finalRequest))
			require.NotEqual(t, sourceID, finalID)
			expected := fmt.Sprintf(`{"model":%q,"content":[{"type":"draft_task","draft_task":{"id":"prov-task-1"}}],"resolution":"1080p"}`, tc.model)
			if tc.protocol == dto.VideoUpstreamProtocolFunCloudModelArkV3 {
				expected = expected[:len(expected)-1] + `,"real_person_mode":true}`
			}
			assert.JSONEq(t, expected, string(fx.createRequestBody()))
			assert.Equal(t, seedanceFundsInitialQuota-source.Quota-50000, fx.userQuota())

			fx.queryBodyRes.Store(seedanceDraftResult("prov-final-1", "1080p", `,"draft":false,"draft_task_id":"different-provider-id"`, 196425))
			fx.pollOnce(finalID)
			final := fx.loadTask(finalID)
			require.EqualValues(t, model.TaskStatusSuccess, final.Status)
			assert.Equal(t, sourceID, final.PrivateData.VideoDraftSourceID)
			assert.Equal(t, 196425, final.PrivateData.AsyncBilling.ActualTokens)
			assert.Equal(t, 98213, final.Quota)
			assert.Equal(t, seedanceFundsInitialQuota-source.Quota-final.Quota, fx.userQuota())
			assert.Equal(t, sourceID, final.ToModelArkVideoTask().DraftTaskID)
			assert.EqualValues(t, 2, fx.createCalls.Load())
			var attempts int64
			require.NoError(t, fx.db.Model(&model.TaskCreateAttempt{}).Count(&attempts).Error)
			assert.EqualValues(t, 2, attempts)

			// A conflicting ID cannot be treated as a definitive rejection.
			fx.createStatus.Store(http.StatusBadRequest)
			fx.createBodyRes.Store(`{"id":"contradictory-id","error":{"code":"invalid_request_error","message":"bad request"}}`)
			assert.Contains(t, fx.submitCreateBody(seedanceDraftRequest), `"create_outcome_unknown"`)
			assert.EqualValues(t, 3, fx.createCalls.Load())
			var unknown model.TaskCreateAttempt
			require.NoError(t, fx.db.Where("status = ?", model.TaskCreateAttemptUnknown).First(&unknown).Error)
			assert.Equal(t, model.TaskCreateAttemptBillingHeld, unknown.BillingHoldState)
			assert.Equal(t, seedanceFundsInitialQuota-source.Quota-final.Quota-50000, fx.userQuota())
		})
	}
}

func TestSeedanceDraftRequiresConfirmedResultAndRefundsOrdinaryResult(t *testing.T) {
	fx := seedanceDraftFixture(t, dto.VideoUpstreamProtocolViduModelArkV3, "viduq3.1-drama-std")
	pollLimit := constant.TaskPollMaxFailures
	constant.TaskPollMaxFailures = 1
	t.Cleanup(func() { constant.TaskPollMaxFailures = pollLimit })
	id := decodeSeedanceFundsCreateID(t, fx.submitCreateBody(seedanceDraftRequest))
	heldBalance := fx.userQuota()
	require.Less(t, heldBalance, seedanceFundsInitialQuota)

	fx.queryBodyRes.Store(seedanceDraftResult("prov-task-1", "480p", "", 38830))
	fx.pollOnce(id)
	task := fx.loadTask(id)
	assert.Equal(t, model.TaskStatusReconciliationRequired, task.Status)
	assert.Empty(t, task.PrivateData.ResultURL)
	assert.Equal(t, heldBalance, fx.userQuota())

	fx.queryBodyRes.Store(seedanceDraftResult("prov-task-1", "480p", `,"draft":false`, 38830))
	fx.pollOnce(id)
	task = fx.loadTask(id)
	assert.Equal(t, model.TaskStatusProviderContractFailure, task.Status)
	assert.Empty(t, task.PrivateData.ResultURL)
	assert.NotContains(t, string(task.Data), "fixture.example")
	assert.Equal(t, seedanceFundsInitialQuota, fx.userQuota())
	var exposure model.ProviderCostExposure
	require.NoError(t, fx.db.Where("source_id = ?", id).First(&exposure).Error)
	assert.Nil(t, exposure.ProviderAmount)
	assert.Equal(t, seedanceFundsInitialQuota-heldBalance, exposure.CustomerQuotaReleased)

	fx.pollOnce(id)
	assert.Equal(t, seedanceFundsInitialQuota, fx.userQuota())
	remain, used := fx.tokenQuota()
	assert.Equal(t, seedanceFundsInitialQuota, remain)
	assert.Zero(t, used)
	assert.EqualValues(t, 1, fx.createCalls.Load())
}

func TestSeedanceDraftMoxingRejectionReleasesAttempt(t *testing.T) {
	fx := seedanceDraftFixture(t, dto.VideoUpstreamProtocolMoxingModelArkV1, "doubao-seedance-2-5-260628")
	fx.createStatus.Store(http.StatusBadRequest)
	fx.createBodyRes.Store(`{"error":{"code":400,"message":"请求参数不合法，请检查后重试。"}}`)
	assert.Contains(t, fx.submitCreateBody(seedanceDraftRequest), `"error"`)
	assert.EqualValues(t, 1, fx.createCalls.Load())
	assert.Contains(t, string(fx.createRequestBody()), `"draft":true`)
	assert.Equal(t, seedanceFundsInitialQuota, fx.userQuota())
	var count int64
	require.NoError(t, fx.db.Model(&model.Task{}).Count(&count).Error)
	assert.Zero(t, count)
	var attempt model.TaskCreateAttempt
	require.NoError(t, fx.db.First(&attempt).Error)
	assert.Equal(t, model.TaskCreateAttemptBillingReleased, attempt.BillingHoldState)
}

func TestSeedanceDraftRejectsNonTokenPricingBeforeAttempt(t *testing.T) {
	fx := seedanceDraftFixture(t, dto.VideoUpstreamProtocolViduModelArkV3, "viduq3.1-drama-std")
	for _, expression := range []string{
		`tier("seconds", u("duration_seconds") * 0.01)`,
		`tier("seconds", u("tokens") >= 0 ? u("duration_seconds") * 0.01 : 0)`,
		`u("tokens") >= 100000 ? tier("tokens", u("tokens") / 1000000) : tier("flat", 1)`,
	} {
		pricing, err := common.Marshal(map[string]string{"customer-video": expression})
		require.NoError(t, err)
		require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"billing_setting.billing_expr": string(pricing)}))
		assert.Contains(t, fx.submitCreateBody(seedanceDraftRequest), `"error"`, expression)
	}
	assertNoAdditionalDraftAttempt(t, fx, 0, seedanceFundsInitialQuota)

	decodeSeedanceFundsCreateID(t, fx.submitCreateBody(`{"model":"customer-video","content":[{"type":"text","text":"A landscape"}],"duration":4,"draft":false}`))
	assert.NotContains(t, string(fx.createRequestBody()), `"draft"`)
}

func assertNoAdditionalDraftAttempt(t *testing.T, fx *seedanceFundsFixture, existing int, balance int) {
	t.Helper()
	assert.EqualValues(t, existing, fx.createCalls.Load())
	assert.Equal(t, balance, fx.userQuota())
	var attempts int64
	require.NoError(t, fx.db.Model(&model.TaskCreateAttempt{}).Count(&attempts).Error)
	assert.EqualValues(t, existing, attempts)
}

func TestSeedanceDraftUnconfirmedResultSurvivesMixedPollFailures(t *testing.T) {
	fx := seedanceDraftFixture(t, dto.VideoUpstreamProtocolFunCloudModelArkV3, "seedance-2-5")
	oldLimit := constant.TaskPollMaxFailures
	constant.TaskPollMaxFailures = 2
	t.Cleanup(func() { constant.TaskPollMaxFailures = oldLimit })

	id := decodeSeedanceFundsCreateID(t, fx.submitCreateBody(seedanceDraftRequest))
	balance := fx.userQuota()
	fx.queryBodyRes.Store(seedanceDraftResult("prov-task-1", "480p", "", 38830))
	fx.pollOnce(id)
	fx.queryStatus.Store(http.StatusBadGateway)
	fx.pollOnce(id)
	assert.Equal(t, model.TaskStatusReconciliationRequired, fx.loadTask(id).Status)
	fx.queryStatus.Store(http.StatusNotFound)
	fx.pollOnce(id)
	assert.Equal(t, model.TaskStatusReconciliationRequired, fx.loadTask(id).Status)
	assertNoAdditionalDraftAttempt(t, fx, 1, balance)

	fx.queryStatus.Store(http.StatusOK)
	fx.queryBodyRes.Store(seedanceDraftResult("prov-task-1", "480p", `,"draft":true`, 38830))
	fx.pollOnce(id)
	task := fx.loadTask(id)
	assert.EqualValues(t, model.TaskStatusSuccess, task.Status)
	assert.Equal(t, 19415, task.Quota)
	assert.Equal(t, seedanceFundsInitialQuota-task.Quota, fx.userQuota())
}

func TestSeedanceDraftUsesOnlyTheTypedPollObservation(t *testing.T) {
	fx := seedanceDraftFixture(t, dto.VideoUpstreamProtocolModelArkV3Volcengine, "ep-reviewed-deployment")
	id := decodeSeedanceFundsCreateID(t, fx.submitCreateBody(seedanceDraftRequest))
	heldBalance := fx.userQuota()
	fx.queryBodyRes.Store(`{"id":"prov-task-1","status":"running","draft":true,"code":"success","data":{"task_id":"other-task","status":"SUCCESS","result_url":"https://fixture.example/unrelated.mp4"}}`)
	fx.pollOnce(id)
	task := fx.loadTask(id)
	assert.EqualValues(t, model.TaskStatusInProgress, task.Status)
	assert.Empty(t, task.PrivateData.ResultURL)
	assert.Equal(t, heldBalance, fx.userQuota())
}

func TestSeedanceDraftLateUsagePreservesTheAcceptedResult(t *testing.T) {
	fx := seedanceDraftFixture(t, dto.VideoUpstreamProtocolViduModelArkV3, "viduq3.1-drama-std")
	id := decodeSeedanceFundsCreateID(t, fx.submitCreateBody(seedanceDraftRequest))
	fx.queryBodyRes.Store(`{"id":"prov-task-1","status":"succeeded","draft":true,"created_at":1795012800,"duration":4,"resolution":"480p","content":{"video_url":"https://fixture.example/accepted.mp4"}}`)
	fx.pollOnce(id)
	accepted := fx.loadTask(id)
	require.EqualValues(t, model.TaskStatusSuccess, accepted.Status)
	require.Equal(t, model.TaskBillingStateAwaitingUsage, accepted.BillingState)

	for _, marker := range []string{"", `,"draft":true`} {
		fx.queryBodyRes.Store(fmt.Sprintf(`{"id":"prov-task-1","status":"succeeded","resolution":"480p","content":{"video_url":"https://fixture.example/refreshed.mp4"},"usage":{"completion_tokens":38830,"total_tokens":38830}%s}`, marker))
		current := fx.loadTask(id)
		require.NoError(t, service.RefreshVideoTask(context.Background(), &current))
		current = fx.loadTask(id)
		assert.Equal(t, model.TaskBillingStateSettled, current.BillingState)
		assert.Equal(t, 19415, current.Quota)
		assert.Equal(t, accepted.PrivateData.ResultURL, current.PrivateData.ResultURL)
		var result struct {
			CreatedAt int64 `json:"created_at"`
		}
		require.NoError(t, common.Unmarshal(current.Data, &result))
		assert.EqualValues(t, 1795012800, result.CreatedAt)
		assert.Equal(t, model.ModelArkConfirmedDraft, model.ModelArkDraftMarker(current.Data))
		assert.Equal(t, seedanceFundsInitialQuota-19415, fx.userQuota())
	}
}

func TestSeedanceDraftRejectsBlankResolutionBeforeHold(t *testing.T) {
	fx := seedanceDraftFixture(t, dto.VideoUpstreamProtocolModelArkV3Volcengine, "ep-reviewed-deployment")
	for _, resolution := range []string{"", "  "} {
		body := strings.TrimSuffix(seedanceDraftRequest, "}") + fmt.Sprintf(`,"resolution":%q}`, resolution)
		assert.Contains(t, fx.submitCreateBody(body), `"invalid_video_parameter"`)
		assertNoAdditionalDraftAttempt(t, fx, 0, seedanceFundsInitialQuota)
	}
}

func TestSeedanceDraftFinalReturnedAsSampleIsRefunded(t *testing.T) {
	for _, rate := range []int{0, 1} {
		t.Run(fmt.Sprint(rate), func(t *testing.T) {
			fx := seedanceDraftFixture(t, dto.VideoUpstreamProtocolViduModelArkV3, "viduq3.1-drama-std")
			source := completedDraftFixture(t, fx)
			balance := fx.userQuota()
			price, err := common.Marshal(map[string]string{"customer-video": fmt.Sprintf(`tier("final", u("tokens") * %d / 1000000)`, rate)})
			require.NoError(t, err)
			require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"billing_setting.billing_expr": string(price)}))
			fx.createBodyRes.Store(`{"id":"prov-final-1"}`)
			finalID := decodeSeedanceFundsCreateID(t, fx.submitCreateBody(finalDraftRequest(source.TaskID)))
			held := balance - fx.userQuota()
			require.Equal(t, rate*50000, held)

			fx.queryBodyRes.Store(seedanceDraftResult("prov-final-1", "480p", `,"draft":true`, 38830))
			fx.pollOnce(finalID)
			final := fx.loadTask(finalID)
			assert.Equal(t, model.TaskStatusProviderContractFailure, final.Status)
			assert.Nil(t, final.ToModelArkVideoTask().Content)
			assert.Empty(t, final.PrivateData.ResultURL)
			assert.Equal(t, balance, fx.userQuota())
			assert.Equal(t, source.Quota, fx.loadTask(source.TaskID).Quota)
			var exposure model.ProviderCostExposure
			require.NoError(t, fx.db.Where("source_id = ?", finalID).First(&exposure).Error)
			assert.Equal(t, held, exposure.CustomerQuotaReleased)
			assert.Nil(t, exposure.ProviderAmount)
			fx.pollOnce(finalID)
			assert.Equal(t, balance, fx.userQuota())
		})
	}
}

func completedDraftFixture(t *testing.T, fx *seedanceFundsFixture) model.Task {
	t.Helper()
	id := decodeSeedanceFundsCreateID(t, fx.submitCreateBody(seedanceDraftRequest))
	fx.queryBodyRes.Store(seedanceDraftResult("prov-task-1", "480p", `,"draft":true`, 38830))
	fx.pollOnce(id)
	task := fx.loadTask(id)
	require.EqualValues(t, model.TaskStatusSuccess, task.Status)
	return task
}

func finalDraftRequest(id string) string {
	return fmt.Sprintf(`{"model":"customer-video","content":[{"type":"draft_task","draft_task":{"id":%q}}],"resolution":"1080p"}`, id)
}

func TestSeedanceDraftRollbackStopsNewOperationsAndPreservesInFlight(t *testing.T) {
	fx := seedanceDraftFixture(t, dto.VideoUpstreamProtocolViduModelArkV3, "viduq3.1-drama-std")
	source := completedDraftFixture(t, fx)
	fx.createBodyRes.Store(`{"id":"prov-pending-1"}`)
	pendingID := decodeSeedanceFundsCreateID(t, fx.submitCreateBody(seedanceDraftRequest))
	balance := fx.userQuota()

	old := seedanceGoodExtensionRows()[0]
	current := old.Version
	old.Version = "1.6.3"
	olderSource := strings.Replace(string(old.Source), `version: "`+current+`"`, `version: "`+old.Version+`"`, 1)
	olderSource = strings.Replace(olderSource, "export const modelArkDraftV3 = true;", "", 1)
	old.Source = model.LongText(olderSource)
	old.SourceHash = fmt.Sprintf("%x", common.Sha256Raw([]byte(olderSource)))
	fx.seedExtension([]model.TaskPlugin{old})

	assert.Contains(t, fx.submitCreateBody(seedanceDraftRequest), `"unsupported_parameter"`)
	assert.Contains(t, fx.submitCreateBody(finalDraftRequest(source.TaskID)), `"unsupported_parameter"`)
	assertNoAdditionalDraftAttempt(t, fx, 2, balance)

	fx.queryBodyRes.Store(seedanceDraftResult("prov-pending-1", "480p", `,"draft":true`, 38830))
	fx.pollOnce(pendingID)
	pending := fx.loadTask(pendingID)
	assert.Equal(t, current, pending.PrivateData.Execution.TaskPlugin.Version)
	assert.EqualValues(t, model.TaskStatusSuccess, pending.Status)
	assert.Equal(t, seedanceFundsInitialQuota-source.Quota-pending.Quota, fx.userQuota())
}

func TestSeedanceDraftSourceRejectsChangedPathsBeforeAttempt(t *testing.T) {
	fx := seedanceDraftFixture(t, dto.VideoUpstreamProtocolViduModelArkV3, "viduq3.1-drama-std")
	source := completedDraftFixture(t, fx)
	balance := fx.userQuota()
	frozen := source.PrivateData
	for _, paths := range []struct{ create, query string }{
		{"/unrelated/create", frozen.VideoUpstreamQueryPathTemplate},
		{frozen.VideoDraftCreatePath, "/unrelated/query/{task_id}"},
	} {
		private := frozen
		private.VideoDraftCreatePath = paths.create
		private.VideoUpstreamQueryPathTemplate = paths.query
		require.NoError(t, fx.db.Model(&source).Update("private_data", private).Error)
		assert.Contains(t, fx.submitCreateBody(finalDraftRequest(source.TaskID)), `"invalid_draft_task"`)
		assertNoAdditionalDraftAttempt(t, fx, 1, balance)
	}
	legacy := frozen
	legacy.VideoDraftCreatePath = ""
	require.NoError(t, fx.db.Model(&source).Update("private_data", legacy).Error)
	fx.createBodyRes.Store(`{"id":"prov-final-1"}`)
	finalID := decodeSeedanceFundsCreateID(t, fx.submitCreateBody(finalDraftRequest(source.TaskID)))
	assert.Equal(t, source.TaskID, fx.loadTask(finalID).PrivateData.VideoDraftSourceID)
}

func TestSeedanceDraftRejectsInvalidSourceBeforeAttempt(t *testing.T) {
	for _, invalid := range []string{"model rebound", "deleted", "ordinary video", "expired domestic draft"} {
		t.Run(invalid, func(t *testing.T) {
			fx := seedanceDraftFixture(t, dto.VideoUpstreamProtocolViduModelArkV3, "viduq3.1-drama-std")
			source := completedDraftFixture(t, fx)
			switch invalid {
			case "model rebound":
				var channel model.Channel
				require.NoError(t, fx.db.First(&channel, fx.channelID).Error)
				require.NoError(t, fx.db.Model(&channel).Update("models", "other-model").Error)
				channel.Id = fx.channelID + 1
				require.NoError(t, fx.db.Create(&channel).Error)
			case "deleted":
				require.NoError(t, fx.db.Model(&source).Update("client_deleted_at", 1).Error)
			case "ordinary video":
				source.PrivateData.VideoDraftRequested = false
				source.Data = []byte(seedanceDraftResult("prov-task-1", "480p", `,"draft":false`, 38830))
				require.NoError(t, fx.db.Model(&source).Select("Data", "PrivateData").Updates(&source).Error)
			case "expired domestic draft":
				source.PrivateData.VideoUpstreamQueryBaseURL = "https://API.VIDU.CN:443/ent"
				source.Data = []byte(`{"draft":true,"created_at":1}`)
				require.NoError(t, fx.db.Model(&source).Select("Data", "PrivateData").Updates(&source).Error)
				require.NoError(t, fx.db.Model(&model.Channel{}).Where("id = ?", fx.channelID).
					Update("base_url", source.PrivateData.VideoUpstreamQueryBaseURL).Error)
				fx.onInterpose(func(c *gin.Context) {
					t.Error("expired draft reached relay instead of being rejected during routing")
					c.AbortWithStatus(http.StatusInternalServerError)
				})
			}
			balance := fx.userQuota()
			assert.Contains(t, fx.submitCreateBody(finalDraftRequest(source.TaskID)), `"invalid_draft_task"`)
			assertNoAdditionalDraftAttempt(t, fx, 1, balance)
		})
	}
}

func TestSeedanceDraftRejectsInvalidRequestShape(t *testing.T) {
	fx := seedanceDraftFixture(t, dto.VideoUpstreamProtocolViduModelArkV3, "viduq3.1-drama-std")
	for _, body := range []string{
		`{"model":"customer-video","content":[{"type":"draft_task","draft_task":{"id":"sample"}},{"type":"text","text":"extra"}],"resolution":"1080p"}`,
		`{"model":"customer-video","content":[{"type":"draft_task","draft_task":{"id":"sample"}}],"duration":4,"resolution":"1080p"}`,
		`{"model":"customer-video","content":[{"type":"draft_task","draft_task":{"id":"sample"}}]}`,
		`{"model":"customer-video","content":[{"type":"draft_task","draft_task":{"id":"sample"}}],"resolution":"1080p","draft":true}`,
		`{"model":"customer-video","content":[{"type":"text","text":"fixture"}],"draft":"true"}`,
		`{"model":"customer-video","content":[{"type":"text","text":"fixture"}],"draft":null}`,
	} {
		assert.Contains(t, fx.submitCreateBody(body), `"invalid_request"`, body)
	}
	assertNoAdditionalDraftAttempt(t, fx, 0, seedanceFundsInitialQuota)
}

func TestSeedanceDraftMiniMaxCannotExpressOperations(t *testing.T) {
	fx := newMiniMaxFundsFixture(t)
	assert.Contains(t, fx.submitCreateBody(seedanceDraftRequest), `"unsupported_parameter"`)
	assert.Contains(t, fx.submitCreateBody(finalDraftRequest("sample")), `"invalid_draft_task"`)
	assertNoAdditionalDraftAttempt(t, fx, 0, seedanceFundsInitialQuota)
}
