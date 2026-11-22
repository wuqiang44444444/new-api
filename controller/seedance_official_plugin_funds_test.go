package controller

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOfficialPluginFundsUsesFrozenExpressionAndValidUsage(t *testing.T) {
	for _, tc := range []struct {
		name, usage string
		charge      int
		awaiting    bool
		prompt      int
		webSearch   int
	}{
		{"completion", `{"prompt_tokens":2000,"completion_tokens":10000,"total_tokens":12000,"tool_usage":{"web_search":2}}`, 50000, false, 2000, 2},
		{"explicit-zero", `{"completion_tokens":0}`, 0, false, 0, 0},
		{"missing-completion", `{"prompt_tokens":2000,"total_tokens":12000}`, seedanceFundsHold, true, 2000, 0},
		{"overflow", `{"completion_tokens":18446744073709551615}`, seedanceFundsHold, true, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := newSeedanceFundsFixture(t)
			require.NoError(t, fx.db.AutoMigrate(&model.TaskVideoResultObject{}))
			fx.engine.GET("/api/v3/contents/generations/tasks/:task_id", middleware.VideoTaskReadTokenAuth(), ModelArkVideoGet)
			var channel model.Channel
			require.NoError(t, fx.db.First(&channel, fx.channelID).Error)
			channel.ModelMapping = common.GetPointer(`{"customer-video":"ep-reviewed-deployment"}`)
			channel.SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolModelArkV3Volcengine, AssetUpstreamProtocol: dto.AssetUpstreamProtocolNone})
			require.NoError(t, fx.db.Save(&channel).Error)
			require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"billing_setting.billing_expr": `{"customer-video":"tier(\"base\", u(\"tokens\") * 10 / 1000000)"}`}))
			fx.assertAttemptAtPost.Store(true)
			publicID := decodeSeedanceFundsCreateID(t, fx.submitCreate())
			require.Equal(t, seedanceFundsInitialQuota-seedanceFundsHold, fx.userQuota())
			task := fx.loadTask(publicID)
			require.NotNil(t, task.PrivateData.Execution)
			require.NotNil(t, task.PrivateData.Execution.TaskPlugin)
			require.NotNil(t, task.PrivateData.AsyncBilling.TieredSnapshot)
			assert.True(t, task.PrivateData.AsyncBilling.TieredSnapshot.TaskUsageBilling)
			// An administrator's later price change cannot reprice the frozen task.
			require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"billing_setting.billing_expr": `{"customer-video":"tier(\"base\", u(\"tokens\") * 1000 / 1000000)"}`}))
			fx.queryBodyRes.Store(fmt.Sprintf(`{"id":"prov-task-1","status":"succeeded","content":{"video_url":%q},"usage":%s}`, fx.server.URL+"/video.mp4", tc.usage))
			fx.pollOnce(publicID)
			assert.Equal(t, seedanceFundsInitialQuota-tc.charge, fx.userQuota())
			task = fx.loadTask(publicID)
			assert.EqualValues(t, model.TaskStatusSuccess, task.Status)
			if tc.awaiting {
				assert.Equal(t, model.TaskBillingStateAwaitingUsage, task.PrivateData.AsyncBilling.State)
			}
			w := minimaxGet(t, fx, "/api/v3/contents/generations/tasks/"+publicID)
			require.Equal(t, http.StatusOK, w.Code)
			var public dto.ModelArkVideoTask
			require.NoError(t, common.Unmarshal(w.Body.Bytes(), &public))
			assert.Equal(t, tc.prompt, public.Usage.PromptTokens)
			assert.Equal(t, tc.webSearch, public.Usage.ToolUsage.WebSearch)
			fx.pollOnce(publicID)
			assert.Equal(t, seedanceFundsInitialQuota-tc.charge, fx.userQuota(), "repeated observation must not settle or refund twice")
			remain, _ := fx.tokenQuota()
			assert.Equal(t, seedanceFundsInitialQuota-tc.charge, remain)
		})
	}
}

func TestOfficialSeedanceShortVideoHoldsWithinWalletAndSettlesActualUsage(t *testing.T) {
	fx := newSeedanceFundsFixture(t)
	require.NoError(t, fx.db.AutoMigrate(&model.TaskVideoResultObject{}))
	var channel model.Channel
	require.NoError(t, fx.db.First(&channel, fx.channelID).Error)
	channel.ModelMapping = common.GetPointer(`{"customer-video":"doubao-seedance-2-0-260128"}`)
	channel.SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolModelArkV3Volcengine})
	require.NoError(t, fx.db.Save(&channel).Error)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_expr":           `{"customer-video":"u(\"has_video_input\") ? tier(\"reference_video\", u(\"tokens\") * 4.116 / 1000000) : tier(\"no_video\", u(\"tokens\") * 6.762 / 1000000)"}`,
		"task_billing_setting.preconsume_tokens": `{"customer-video":7000000}`,
	}))

	const initialQuota, heldQuota, finalQuota = 500_000, 459_312, 365_148
	require.NoError(t, fx.db.Model(&model.User{}).Where("id = ?", fx.userID).Update("quota", initialQuota).Error)
	require.NoError(t, fx.db.Model(&model.Token{}).Where("id = ?", fx.userID).Update("remain_quota", initialQuota).Error)
	publicID := decodeSeedanceFundsCreateID(t, fx.submitCreateBody(
		`{"model":"customer-video","content":[{"type":"text","text":"A landscape"}],"duration":5,"resolution":"720p","ratio":"16:9"}`,
	))
	task := fx.loadTask(publicID)
	require.NotNil(t, task.PrivateData.AsyncBilling.TieredSnapshot)
	assert.Equal(t, 135_851, task.PrivateData.AsyncBilling.TieredSnapshot.EstimatedCompletionTokens)
	assert.Equal(t, initialQuota-heldQuota, fx.userQuota())
	remaining, _ := fx.tokenQuota()
	assert.Equal(t, initialQuota-heldQuota, remaining)

	fx.queryBodyRes.Store(fmt.Sprintf(`{"id":"prov-task-1","status":"succeeded","content":{"video_url":%q},"usage":{"completion_tokens":108000}}`, fx.server.URL+"/video.mp4"))
	fx.pollOnce(publicID)
	fx.pollOnce(publicID)
	assert.Equal(t, initialQuota-finalQuota, fx.userQuota())
	remaining, _ = fx.tokenQuota()
	assert.Equal(t, initialQuota-finalQuota, remaining)
	task = fx.loadTask(publicID)
	assert.Equal(t, model.TaskBillingStateSettled, task.PrivateData.AsyncBilling.State)
}
