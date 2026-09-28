package controller

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestViduFundsSettlesObservedUsageWithFrozenPriceAndContract(t *testing.T) {
	for _, tc := range []struct {
		provider     string
		video        bool
		hold, charge int
	}{
		{"viduq3-drama-std", false, 2525917, 296414},
		{"viduq3-drama-ab-std", false, 2525917, 296414},
		{"viduq3-drama-std", true, 1537515, 180426},
		{"viduq3-drama-ab-std", true, 1537515, 180426},
	} {
		t.Run(fmt.Sprintf("%s/video=%t", tc.provider, tc.video), func(t *testing.T) {
			fx := newSeedanceFundsFixture(t)
			var channel model.Channel
			require.NoError(t, fx.db.First(&channel, fx.channelID).Error)
			channel.ModelMapping = common.GetPointer(fmt.Sprintf(`{"customer-video":%q}`, tc.provider))
			channel.SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolViduModelArkV3, AssetUpstreamProtocol: dto.AssetUpstreamProtocolNone})
			require.NoError(t, fx.db.Save(&channel).Error)
			previousRate := operation_setting.USDExchangeRate
			t.Cleanup(func() { require.NoError(t, operation_setting.SetUSDExchangeRate(fmt.Sprint(previousRate))) })
			require.NoError(t, operation_setting.SetUSDExchangeRate("6.76"))
			require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
				"billing_setting.billing_expr":           `{"customer-video":"tier(\"official\", u(\"tokens\") * (u(\"has_video_input\") ? 28 : 46) / usd_exchange_rate() / 1000000)"}`,
				"task_billing_setting.preconsume_tokens": `{"customer-video":928000}`,
			}))
			require.NoError(t, fx.db.AutoMigrate(&model.Ability{}, &model.CustomerContract{}, &model.CustomerContractEntityRule{}, &model.CustomerContractEntityAudit{}))
			require.NoError(t, fx.db.Model(&model.User{}).Where("id = ?", fx.userID).Update("auth_version", 1).Error)
			service.ResetContractEntityCacheForTest()
			t.Cleanup(service.ResetContractEntityCacheForTest)
			contract, err := model.CreateCustomerContractEntity(model.CreateCustomerContractParams{
				UserId: fx.userID, AdminUserId: fx.userID, Name: "Fixture", Enabled: true, Reason: "fixture",
				Rules: []model.CustomerContractEntityRuleInput{{PublicModel: "customer-video", ChannelId: fx.channelID, RouteGroup: "default", RatioUnits: 80_000_000}},
			})
			require.NoError(t, err)
			require.NoError(t, fx.db.Model(&model.Token{}).Where("id = ?", fx.userID).Update("contract_id", contract.Id).Error)
			videoContent := ""
			if tc.video {
				videoContent = `,{"type":"video_url","role":"reference_video","video_url":{"url":"https://fixture.example/input.mp4"}}`
			}
			publicID := decodeSeedanceFundsCreateID(t, fx.submitCreateBody(`{"model":"customer-video","content":[{"type":"text","text":"A landscape"},{"type":"image_url","role":"reference_image","image_url":{"url":"https://fixture.example/image.png"}},{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://fixture.example/audio.mp3"}}`+videoContent+`],"duration":5,"resolution":"720p","ratio":"16:9","generate_audio":true,"return_last_frame":true}`))
			// 928000 × official CNY price / 1e6 / 6.76 × 500000 × 80%, rounded once.
			hold := tc.hold
			require.Equal(t, seedanceFundsInitialQuota-hold, fx.userQuota())
			task := fx.loadTask(publicID)
			require.NotNil(t, task.PrivateData.BillingContext.ContractFact)
			assert.EqualValues(t, 80_000_000, task.PrivateData.BillingContext.ContractFact.RatioUnits)
			assert.Contains(t, string(fx.createRequestBody()), `"generate_audio":true`)
			assert.Contains(t, string(fx.createRequestBody()), `"audio_url"`)
			// Query errors and invalid usage must never silently refund or charge zero.
			fx.queryStatus.Store(404)
			fx.pollOnce(publicID)
			assert.Equal(t, seedanceFundsInitialQuota-hold, fx.userQuota())
			fx.queryStatus.Store(200)
			fx.queryBodyRes.Store(`{"id":"prov-task-1","status":"succeeded","content":{"video_url":"https://fixture.example/result.mp4"},"usage":{"total_tokens":"108900"}}`)
			fx.pollOnce(publicID)
			task = fx.loadTask(publicID)
			assert.Equal(t, model.TaskBillingStateAwaitingUsage, task.PrivateData.AsyncBilling.State)
			assert.Equal(t, seedanceFundsInitialQuota-hold, fx.userQuota())
			// Changes after submission do not reprice this task.
			require.NoError(t, operation_setting.SetUSDExchangeRate("10"))
			require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"billing_setting.billing_expr": `{"customer-video":"tier(\"changed\", u(\"tokens\") * 1000 / 1000000)"}`}))
			fx.queryBodyRes.Store(`{"id":"prov-task-1","status":"succeeded","seed":"0","duration":"5","created_at":"1790563514","updated_at":"1790563750","frames_per_second":"24","resolution":"720p","ratio":"16:9","content":{"video_url":"https://fixture.example/result.mp4","last_frame_url":"https://fixture.example/frame.jpg"},"usage":{"prompt_tokens":"0","completion_tokens":"108900","total_tokens":"108900"}}`)
			fx.pollOnce(publicID)
			// The frozen media tier selects ¥46 (image/audio) or ¥28 (video) per million.
			charge := tc.charge
			assert.Equal(t, seedanceFundsInitialQuota-charge, fx.userQuota())
			task = fx.loadTask(publicID)
			assert.EqualValues(t, model.TaskStatusSuccess, task.Status)
			assert.Equal(t, model.TaskBillingStateSettled, task.PrivateData.AsyncBilling.State)
			assert.Equal(t, 108900, task.PrivateData.AsyncBilling.ActualTokens)
			assert.Equal(t, charge, task.Quota)
			fx.pollOnce(publicID)
			assert.Equal(t, seedanceFundsInitialQuota-charge, fx.userQuota())
			remain, used := fx.tokenQuota()
			assert.Equal(t, seedanceFundsInitialQuota-charge, remain)
			assert.Equal(t, charge, used)
			logs := fx.logsByType()
			net := 0
			for _, entry := range logs[model.LogTypeConsume] {
				net += entry.Quota
			}
			for _, entry := range logs[model.LogTypeRefund] {
				net -= entry.Quota
			}
			assert.Equal(t, charge, net, "ledger equals wallet, token and task settlement")
		})
	}
}

func TestViduZeroUsageAndTrustedFailuresReleaseFundsOnce(t *testing.T) {
	for _, tc := range []struct{ name, response string }{
		{"zero", `{"id":"prov-task-1","status":"succeeded","content":{"video_url":"https://fixture.example/result.mp4"},"usage":{"completion_tokens":"0"}}`},
		{"failed", `{"id":"prov-task-1","status":"failed","error":{"code":"GenerationFailed","message":"fixture failure"}}`},
		{"expired", `{"id":"prov-task-1","status":"expired","content":null,"usage":null,"error":null}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := newSeedanceFundsFixture(t)
			require.NoError(t, fx.db.AutoMigrate(&model.ErrorEvent{}))
			var channel model.Channel
			require.NoError(t, fx.db.First(&channel, fx.channelID).Error)
			channel.ModelMapping = common.GetPointer(`{"customer-video":"viduq3-drama-std"}`)
			channel.SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolViduModelArkV3, AssetUpstreamProtocol: dto.AssetUpstreamProtocolNone})
			require.NoError(t, fx.db.Save(&channel).Error)
			require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"billing_setting.billing_expr": `{"customer-video":"tier(\"fixture\",u(\"tokens\")/1000000)"}`}))
			id := decodeSeedanceFundsCreateID(t, fx.submitCreate())
			require.Less(t, fx.userQuota(), seedanceFundsInitialQuota)
			fx.queryBodyRes.Store(tc.response)
			fx.pollOnce(id)
			task := fx.loadTask(id)
			if tc.name == "zero" {
				assert.Equal(t, model.TaskBillingStateSettled, task.PrivateData.AsyncBilling.State)
			} else if tc.name == "expired" {
				assert.EqualValues(t, model.TaskStatusExpired, task.Status)
				assert.NotEmpty(t, task.FailReason)
			} else {
				assert.EqualValues(t, model.TaskStatusFailure, task.Status)
			}
			assert.Equal(t, seedanceFundsInitialQuota, fx.userQuota())
			fx.pollOnce(id)
			assert.Equal(t, seedanceFundsInitialQuota, fx.userQuota())
			remain, used := fx.tokenQuota()
			assert.Equal(t, seedanceFundsInitialQuota, remain)
			assert.Zero(t, used)
		})
	}
}

func TestViduInvalidResultURLsCannotSettleHeldFunds(t *testing.T) {
	for _, content := range []string{
		`{"video_url":"not-a-url"}`,
		`{"video_url":"http://fixture.example/result.mp4"}`,
		`{"video_url":"https://user:password@fixture.example/result.mp4"}`,
		`{"video_url":"https://fixture.example/result.mp4","last_frame_url":"not-a-url"}`,
	} {
		t.Run(content, func(t *testing.T) {
			fx := newSeedanceFundsFixture(t)
			var channel model.Channel
			require.NoError(t, fx.db.First(&channel, fx.channelID).Error)
			channel.ModelMapping = common.GetPointer(`{"customer-video":"viduq3-drama-std"}`)
			channel.SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolViduModelArkV3, AssetUpstreamProtocol: dto.AssetUpstreamProtocolNone})
			require.NoError(t, fx.db.Save(&channel).Error)
			require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"billing_setting.billing_expr": `{"customer-video":"tier(\"fixture\",u(\"tokens\")/1000000)"}`}))
			id := decodeSeedanceFundsCreateID(t, fx.submitCreate())
			heldBalance := fx.userQuota()
			require.Less(t, heldBalance, seedanceFundsInitialQuota)
			fx.queryBodyRes.Store(`{"id":"prov-task-1","status":"succeeded","content":` + content + `,"usage":{"completion_tokens":"100"}}`)
			fx.pollOnce(id)
			task := fx.loadTask(id)
			assert.Equal(t, model.TaskStatusReconciliationRequired, task.Status)
			assert.Equal(t, model.TaskBillingStatePending, task.PrivateData.AsyncBilling.State)
			assert.False(t, task.PrivateData.AsyncBilling.ActualUsageReported)
			assert.Nil(t, task.PrivateData.AsyncBilling.TargetQuota)
			assert.Empty(t, task.PrivateData.ResultURL)
			assert.Equal(t, heldBalance, fx.userQuota())
			var adjustments int64
			require.NoError(t, fx.db.Model(&model.TaskBillingDelivery{}).Where("task_row_id = ? AND event = ?", task.ID, "adjustment").Count(&adjustments).Error)
			assert.Zero(t, adjustments)
			fx.queryBodyRes.Store(`{"id":"prov-task-1","status":"succeeded","content":{"video_url":"https://fixture.example/result.mp4"},"usage":{"completion_tokens":"100"}}`)
			fx.pollOnce(id)
			assert.Equal(t, model.TaskBillingStateSettled, fx.loadTask(id).PrivateData.AsyncBilling.State)
			assert.EqualValues(t, 1, fx.createCalls.Load())
		})
	}
}
