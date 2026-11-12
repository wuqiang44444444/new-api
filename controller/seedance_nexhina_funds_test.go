package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNexhinaSeedanceQueryRecoveryAndLifecycle(t *testing.T) {
	fx := newSeedanceFundsFixture(t)
	require.NoError(t, fx.db.AutoMigrate(&model.TaskVideoResultObject{}))
	fx.engine.DELETE("/api/v3/contents/generations/tasks/:task_id", middleware.TokenAuth(), ModelArkVideoDelete)
	var channel model.Channel
	require.NoError(t, fx.db.First(&channel, fx.channelID).Error)
	channel.ModelMapping = common.GetPointer(`{"customer-video":"doubao-seedance-2-0-mini-260615"}`)
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		VideoUpstreamProtocol: dto.VideoUpstreamProtocolNexhinaModelArkV3,
		AssetUpstreamProtocol: dto.AssetUpstreamProtocolNone,
	})
	require.NoError(t, fx.db.Save(&channel).Error)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_expr": `{"customer-video":"tier(\"base\", u(\"tokens\") * 10 / 1000000)"}`,
	}))

	publicID := decodeSeedanceFundsCreateID(t, fx.submitCreateBody(`{"model":"customer-video","content":[{"type":"text","text":"A landscape"}],"duration":4,"resolution":"480p","ratio":"16:9"}`))
	task := fx.loadTask(publicID)
	require.Equal(t, dto.VideoUpstreamProtocolNexhinaModelArkV3, task.PrivateData.VideoUpstreamProtocol)
	require.Equal(t, seedanceFundsInitialQuota-seedanceFundsHold, fx.userQuota())

	// Official tasks still support lifecycle calls even though Nexhina shares their transport.
	official := task
	official.PrivateData.VideoUpstreamProtocol = dto.VideoUpstreamProtocolModelArkV3Volcengine
	caps := relay.TaskLifecycleCapabilities(&official)
	assert.True(t, caps.SupportsCancelQueued)
	assert.True(t, caps.SupportsDeleteTerminal)

	assertDeleteRejected := func(code string) {
		t.Helper()
		before := fx.queryCalls.Load()
		request := httptest.NewRequest(http.MethodDelete, "/api/v3/contents/generations/tasks/"+publicID, nil)
		request.Header.Set("Authorization", "Bearer sk-"+strings.Repeat("f", 32))
		response := httptest.NewRecorder()
		fx.engine.ServeHTTP(response, request)
		require.Equal(t, http.StatusConflict, response.Code)
		assert.Contains(t, response.Body.String(), code)
		assert.Equal(t, before, fx.queryCalls.Load(), "rejected DELETE must not contact the provider")
	}
	assertDeleteRejected("cancellation_unsupported")

	for _, status := range []int{http.StatusNotFound, http.StatusGone} {
		fx.queryStatus.Store(int32(status))
		fx.queryBodyRes.Store("proxy response with private diagnostic")
		fx.pollOnce(publicID)
		task = fx.loadTask(publicID)
		require.Equal(t, model.TaskStatusReconciliationRequired, task.Status)
		assert.Contains(t, task.FailReason, fmt.Sprint(status))
		assert.NotContains(t, task.FailReason, "private diagnostic")
		assert.Equal(t, model.TaskBillingStatePending, task.BillingState)
		assert.Equal(t, seedanceFundsHold, task.Quota)
		assert.Equal(t, seedanceFundsInitialQuota-seedanceFundsHold, fx.userQuota())
		remain, used := fx.tokenQuota()
		assert.Equal(t, seedanceFundsInitialQuota-seedanceFundsHold, remain)
		assert.Equal(t, seedanceFundsHold, used)
		assert.Empty(t, fx.logsByType()[model.LogTypeRefund])
	}

	fx.queryStatus.Store(http.StatusOK)
	fx.queryBodyRes.Store(`{"id":"prov-task-1","status":"succeeded","content":{"video_url":"https://result.example/video.mp4"},"usage":{"completion_tokens":10000,"total_tokens":10000}}`)
	fx.pollOnce(publicID)
	task = fx.loadTask(publicID)
	require.EqualValues(t, model.TaskStatusSuccess, task.Status)
	assert.Empty(t, task.FailReason)
	assert.Equal(t, model.TaskBillingStateSettled, task.BillingState)
	assert.Equal(t, 50_000, task.Quota)
	assertDeleteRejected("delete_unsupported")

	fx.pollOnce(publicID)
	assert.EqualValues(t, 1, fx.createCalls.Load(), "reconciliation must not resend creation")
	assert.Equal(t, seedanceFundsInitialQuota-50_000, fx.userQuota())
	remain, used := fx.tokenQuota()
	assert.Equal(t, seedanceFundsInitialQuota-50_000, remain)
	assert.Equal(t, 50_000, used)
	require.Len(t, fx.logsByType()[model.LogTypeRefund], 1)
}
