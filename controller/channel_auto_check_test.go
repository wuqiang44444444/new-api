package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// 自动检查合同：图片/视频目标渠道不产生生成副作用；文本渠道本地终止的失败
// 不进入上游故障禁用判断、不覆盖渠道延迟；配置检查通过不恢复被禁渠道。
// 仅上游与数据库为夹具，走真实测试与事件持久化路径。

func setupAutoCheckTest(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousRedis, previousMemory := common.RedisEnabled, common.MemoryCacheEnabled
	previousDisable, previousEnable := common.AutomaticDisableChannelEnabled, common.AutomaticEnableChannelEnabled
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.RedisEnabled, common.MemoryCacheEnabled = previousRedis, previousMemory
		common.AutomaticDisableChannelEnabled, common.AutomaticEnableChannelEnabled = previousDisable, previousEnable
		common.SetDatabaseTypes(previousMainType, previousLogType)
	})
	db := setupModelListControllerTestDB(t)
	previousGroupRatios := ratio_setting.GroupRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previousGroupRatios)) })
	previousRatios, err := common.Marshal(ratio_setting.GetModelRatioCopy())
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-4o":1}`))
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(previousRatios))) })
	common.MemoryCacheEnabled = false
	common.AutomaticDisableChannelEnabled, common.AutomaticEnableChannelEnabled = true, true
	require.NoError(t, model.MigrateErrorEvents())
	return db
}

func registerAutoCheckEventWait(t *testing.T, db *gorm.DB) <-chan int {
	t.Helper()
	persisted := make(chan int, 1)
	require.NoError(t, db.Callback().Create().After("gorm:commit_or_rollback_transaction").Register("test:auto_check_event_committed", func(tx *gorm.DB) {
		if row, ok := tx.Statement.Dest.(*model.ErrorEvent); ok && tx.Error == nil {
			persisted <- row.Id
		}
	}))
	return persisted
}

func waitPersistedAutoCheckEvent(t *testing.T, persisted <-chan int) {
	t.Helper()
	select {
	case <-persisted:
	case <-time.After(5 * time.Second):
		t.Fatal("channel test failure was not persisted")
	}
}

func loadAutoCheckEventDetail(t *testing.T, db *gorm.DB, channelId int) (*model.ErrorEvent, map[string]string) {
	t.Helper()
	rows, total, err := model.GetErrorEvents(model.ErrorEventFilter{ChannelId: channelId}, 0, 10)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	var detail map[string]string
	require.NoError(t, common.UnmarshalJsonStr(rows[0].Detail, &detail))
	return rows[0], detail
}

func assertAutoCheckChannelUnchanged(t *testing.T, db *gorm.DB, id int, status int, responseTime int) {
	t.Helper()
	var updated model.Channel
	require.NoError(t, db.First(&updated, id).Error)
	assert.Equal(t, status, updated.Status, "channel status must stay unchanged")
	assert.Equal(t, responseTime, updated.ResponseTime, "response time must stay unchanged")
}

func mustNotReachProviderServer(t *testing.T) *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("auto check must not send any request to the provider: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	return server
}

func newAutoCheckUser(t *testing.T, db *gorm.DB) *model.User {
	user := &model.User{Username: "auto-check-operator", Group: "default", Status: common.UserStatusEnabled, Quota: 1000000}
	require.NoError(t, db.Create(user).Error)
	return user
}

const autoCheckImageSettings = `{"advanced_custom":{"advanced_routes":[{"models":["img-model"],"incoming_path":"/v1/images/generations","upstream_path":"/v1/images/generations"}]}}`

// 图片/视频目标渠道的自动检查只做本地配置检查：不请求 Provider、不因配置失败
// 禁用渠道；事件携带配置检查分项事实与受控价格诊断。
func TestAutoCheckSkipsGenerationSideEffectsForImageChannels(t *testing.T) {
	db := setupAutoCheckTest(t)
	server := mustNotReachProviderServer(t)
	user := newAutoCheckUser(t, db)
	channel := &model.Channel{
		Type:          constant.ChannelTypeAdvancedCustom,
		Name:          "image fixture",
		Key:           "fixture-channel-secret",
		Models:        "img-model",
		Group:         "default",
		Status:        common.ChannelStatusEnabled,
		BaseURL:       common.GetPointer(server.URL),
		OtherSettings: autoCheckImageSettings,
	}
	require.NoError(t, db.Create(channel).Error)
	persisted := registerAutoCheckEventWait(t, db)

	summary := testChannelForHealthCheck(context.Background(), channel, user.Id, true, 0, true)
	assert.Equal(t, 1, summary.Tested)
	assert.Equal(t, 1, summary.Failed)
	assert.Zero(t, summary.Succeeded)
	waitPersistedAutoCheckEvent(t, persisted)

	row, detail := loadAutoCheckEventDetail(t, db, channel.Id)
	assert.Equal(t, "auto", detail["test_mode"])
	assert.Equal(t, "config_only", detail["check_scope"])
	assert.Equal(t, "failed", detail["config_check"])
	assert.Equal(t, "not_verified", detail["generation_evidence"])
	assert.Equal(t, "not_sent", detail["upstream_request"])
	assert.Equal(t, "test_config_error", row.Reason)
	assert.Equal(t, "model_price_error", row.PublicCode)
	assert.Equal(t, "price_not_configured", detail["config_reason"])
	assert.Equal(t, "model_pricing", detail["config_entry"])
	assert.Equal(t, "img-model", detail["billing_model"])
	assert.Equal(t, "Configure_a_price_for_the_billing_model.", detail["config_summary"])

	assertAutoCheckChannelUnchanged(t, db, channel.Id, common.ChannelStatusEnabled, 0)
}

// 配置检查通过不具备恢复证明力：被自动禁用的图片渠道不得因配置检查通过而恢复，
// 延迟指标也不被本地检查耗时覆盖。
func TestAutoCheckConfigPassDoesNotEnableDisabledImageChannel(t *testing.T) {
	db := setupAutoCheckTest(t)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-4o":1,"img-model":1}`))
	server := mustNotReachProviderServer(t)
	user := newAutoCheckUser(t, db)
	channel := &model.Channel{
		Type:          constant.ChannelTypeAdvancedCustom,
		Name:          "image fixture pass",
		Models:        "img-model",
		Group:         "default",
		Status:        common.ChannelStatusAutoDisabled,
		ResponseTime:  1234,
		BaseURL:       common.GetPointer(server.URL),
		OtherSettings: autoCheckImageSettings,
	}
	require.NoError(t, db.Create(channel).Error)

	summary := testChannelForHealthCheck(context.Background(), channel, user.Id, true, 0, true)
	assert.Equal(t, 1, summary.Unsupported)
	assert.Zero(t, summary.Succeeded)
	assert.Zero(t, summary.Failed)
	assert.Zero(t, summary.Disabled)
	assert.Zero(t, summary.Enabled)
	assertAutoCheckChannelUnchanged(t, db, channel.Id, common.ChannelStatusAutoDisabled, 1234)
}

// 文本渠道本地终止的失败（价格未配置）不进入上游故障自动禁用，也不覆盖渠道延迟；
// 事件明确标记"未发送上游请求"并携带受控价格诊断。
func TestAutoCheckLocalFailureIsolatedFromDisableAndLatency(t *testing.T) {
	db := setupAutoCheckTest(t)
	previousRanges := operation_setting.AutomaticDisableStatusCodesToString()
	require.NoError(t, operation_setting.AutomaticDisableStatusCodesFromString("400,401"))
	t.Cleanup(func() { require.NoError(t, operation_setting.AutomaticDisableStatusCodesFromString(previousRanges)) })
	server := mustNotReachProviderServer(t)
	user := newAutoCheckUser(t, db)
	channel := &model.Channel{
		Type:         constant.ChannelTypeOpenAI,
		Name:         "text fixture",
		Key:          "fixture-channel-secret",
		Models:       "txt-unpriced-model",
		Group:        "default",
		Status:       common.ChannelStatusEnabled,
		ResponseTime: 1234,
		AutoBan:      common.GetPointer(1),
		BaseURL:      common.GetPointer(server.URL),
	}
	require.NoError(t, db.Create(channel).Error)
	persisted := registerAutoCheckEventWait(t, db)

	summary := testChannelForHealthCheck(context.Background(), channel, user.Id, true, 0, true)
	assert.Equal(t, 1, summary.Failed)
	assert.Zero(t, summary.Disabled)
	waitPersistedAutoCheckEvent(t, persisted)

	row, detail := loadAutoCheckEventDetail(t, db, channel.Id)
	assert.Equal(t, "test_config_error", row.Reason)
	assert.Equal(t, "not_sent", detail["upstream_request"])
	assert.Equal(t, "price_not_configured", detail["config_reason"])
	assert.Equal(t, "generation_probe", detail["check_scope"])

	assertAutoCheckChannelUnchanged(t, db, channel.Id, common.ChannelStatusEnabled, 1234)
}

func TestManualBatchStillCallsImageProviderWhileScheduledRunDoesNot(t *testing.T) {
	db := setupAutoCheckTest(t)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"img-model":1}`))
	user := newAutoCheckUser(t, db)
	require.NoError(t, db.Model(user).Update("role", common.RoleRootUser).Error)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		assert.Equal(t, "/v1/images/generations", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"fixture rejection","type":"invalid_request_error"}}`))
	}))
	defer server.Close()
	channel := &model.Channel{Type: constant.ChannelTypeAdvancedCustom, Name: "batch image", Models: "img-model", Group: "default", Status: common.ChannelStatusEnabled, BaseURL: &server.URL, OtherSettings: autoCheckImageSettings}
	require.NoError(t, db.Create(channel).Error)
	automaticResult, err := runChannelTestTask(context.Background(), operation_setting.ChannelTestModeScheduledAll, false, nil)
	require.NoError(t, err)
	assert.Zero(t, calls.Load())
	require.Len(t, automaticResult.Checks, 1)
	assert.Equal(t, "passed", automaticResult.Checks[0].Detail["config_check"])
	require.NoError(t, db.AutoMigrate(&model.SystemTask{}, &model.SystemTaskLock{}))
	task, err := model.CreateSystemTask(model.SystemTaskTypeChannelTest, nil, nil)
	require.NoError(t, err)
	claimed, ok, err := model.ClaimSystemTask(task.ID, task.Type, "check-fixture", common.GetTimestamp()+60)
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, model.FinishSystemTask(claimed.TaskID, "check-fixture", model.SystemTaskStatusSucceeded, automaticResult, ""))
	stored, err := model.GetSystemTaskByTaskID(task.TaskID)
	require.NoError(t, err)
	var decoded channelTestSummary
	require.NoError(t, common.UnmarshalJsonStr(stored.Result, &decoded))
	assert.Equal(t, automaticResult.Checks, decoded.Checks)
	persisted := registerAutoCheckEventWait(t, db)
	// notify=true is the existing manual TestAllChannels payload; preserve its semantics.
	// Stop the fixture after its completed progress report: this exercises the
	// real manual dispatch but avoids starting the unrelated notification daemon.
	manualContext, cancelManual := context.WithCancel(context.Background())
	defer cancelManual()
	manualResult, err := runChannelTestTask(manualContext, operation_setting.ChannelTestModeScheduledAll, true, func(processed, total int) {
		if total > 0 && processed == total {
			cancelManual()
		}
	})
	require.NoError(t, err)
	assert.Equal(t, 1, manualResult.Failed)
	assert.Empty(t, manualResult.Checks)
	assert.EqualValues(t, 1, calls.Load())
	if calls.Load() == 0 {
		return
	}
	waitPersistedAutoCheckEvent(t, persisted)
	_, detail := loadAutoCheckEventDetail(t, db, channel.Id)
	assert.NotContains(t, detail, "check_scope")
	assert.NotContains(t, detail, "config_reason")
}

func TestUnsupportedSeedanceProbeDoesNotClaimUpstreamCall(t *testing.T) {
	db := setupAutoCheckTest(t)
	seedPublishedSeedanceControllerArtifact(t)
	server := mustNotReachProviderServer(t)
	channel := &model.Channel{Type: constant.ChannelTypeSeedanceLink, Models: "video-model", Status: common.ChannelStatusAutoDisabled, ResponseTime: 1234, BaseURL: &server.URL, Key: "fixture-secret"}
	channel.SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolTokenSaveMediaTaskV1, AssetUpstreamProtocol: dto.AssetUpstreamProtocolTokenSaveAssetsV1})
	require.NoError(t, db.Create(channel).Error)
	summary := testChannelForHealthCheck(context.Background(), channel, 0, true, 0, true)
	assert.Equal(t, 1, summary.Unsupported)
	assert.Zero(t, summary.Failed)
	assert.Zero(t, summary.Succeeded)
	require.Len(t, summary.Checks, 1)
	assert.Equal(t, "unsupported", summary.Checks[0].Detail["readonly_check"])
	assert.Equal(t, "not_sent", summary.Checks[0].Detail["upstream_request"])
	assert.Equal(t, "not_verified", summary.Checks[0].Detail["generation_evidence"])
	assertAutoCheckChannelUnchanged(t, db, channel.Id, common.ChannelStatusAutoDisabled, 1234)
}

func TestAutoCheckImageTargetCoverageAndSafeAmbiguity(t *testing.T) {
	db := setupAutoCheckTest(t)
	user := newAutoCheckUser(t, db)
	server := mustNotReachProviderServer(t)
	prices, err := common.Marshal(ratio_setting.GetModelPriceCopy())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(string(prices))) })
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"img-model":0.1}`))
	for _, tc := range []struct {
		name                            string
		kind                            int
		mapping, settings, model, scope string
	}{
		{"AsyncImage", constant.ChannelTypeAsyncImage, `{"img-model":"doubao-seedream-5-0-260128"}`, `{"image_upstream_protocol":"moxing_images_v1"}`, "img-model", "config_only"},
		{"Gemini", constant.ChannelTypeGemini, `{"img-model":"gemini-3.1-flash-image"}`, "", "img-model", "config_only"},
		{"Vertex", constant.ChannelTypeVertexAi, `{"img-model":"gemini-3.1-flash-image"}`, "", "img-model", "config_only"},
		{"AdvancedCustom", constant.ChannelTypeAdvancedCustom, "", autoCheckImageSettings, "img-model", "config_only"},
		{"AdvancedCustom ambiguous", constant.ChannelTypeAdvancedCustom, "", `{"advanced_custom":{"advanced_routes":[{"models":["img-model"],"incoming_path":"/v1/images/generations","upstream_path":"/images"},{"models":["img-model"],"incoming_path":"/v1/chat/completions","upstream_path":"/chat"}]}}`, "img-model", "target_unresolved"},
		{"VolcEngine native signal", constant.ChannelTypeVolcEngine, "", "", "seedream-5.0", "target_unresolved"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			channel := &model.Channel{Type: tc.kind, Models: tc.model, Group: "default", Status: common.ChannelStatusEnabled, BaseURL: &server.URL, OtherSettings: tc.settings}
			if tc.mapping != "" {
				channel.ModelMapping = &tc.mapping
			}
			result := runChannelConfigOnlyCheck(context.Background(), channel, user.Id)
			check := channelAutoCheckResult(channel, result, channelAutoCheckScopeForChannel(channel))
			assert.Equal(t, tc.scope, check.Detail["check_scope"])
			assert.Equal(t, tc.model, check.Model)
			assert.Equal(t, "not_sent", check.Detail["upstream_request"])
			if tc.scope == "config_only" {
				require.NoError(t, result.localErr)
				assert.Equal(t, "passed", check.Detail["config_check"])
				assert.Equal(t, "img-model", check.Detail["billing_model"])
			} else {
				require.ErrorIs(t, result.localErr, errChannelAutoCheckTarget)
				assert.Equal(t, "target_ambiguous", check.Detail["config_reason"])
			}
		})
	}
	var unchanged model.User
	require.NoError(t, db.First(&unchanged, user.Id).Error)
	assert.Equal(t, user.Quota, unchanged.Quota)
}

func TestAutoCheckConnectionFailurePreservesLatency(t *testing.T) {
	db := setupAutoCheckTest(t)
	user := newAutoCheckUser(t, db)
	server := httptest.NewServer(http.NotFoundHandler())
	server.Close()
	channel := &model.Channel{Type: constant.ChannelTypeOpenAI, Models: "gpt-4o", Group: "default", Status: common.ChannelStatusEnabled, ResponseTime: 1234, BaseURL: &server.URL}
	require.NoError(t, db.Create(channel).Error)
	persisted := registerAutoCheckEventWait(t, db)
	summary := testChannelForHealthCheck(context.Background(), channel, user.Id, false, 0, true)
	assert.Equal(t, 1, summary.Failed)
	require.Len(t, summary.Checks, 1)
	assert.Equal(t, "attempted", summary.Checks[0].Detail["upstream_request"])
	assert.Equal(t, "failed", summary.Checks[0].Detail["check_result"])
	assert.Equal(t, "test_upstream_unreachable", summary.Checks[0].Detail["check_reason"])
	assert.Equal(t, "passed", summary.Checks[0].Detail["config_check"])
	waitPersistedAutoCheckEvent(t, persisted)
	_, detail := loadAutoCheckEventDetail(t, db, channel.Id)
	assert.Equal(t, "attempted", detail["upstream_request"])
	assertAutoCheckChannelUnchanged(t, db, channel.Id, common.ChannelStatusEnabled, 1234)
}

func TestAutoCheckRealBillingErrorsHaveSafeTypedDiagnostics(t *testing.T) {
	db := setupAutoCheckTest(t)
	user := newAutoCheckUser(t, db)
	server := mustNotReachProviderServer(t)
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(k, v string) error { saved[k] = v; return nil }))
	t.Cleanup(func() { require.NoError(t, config.GlobalConfig.LoadFromDB(saved)) })
	channel := &model.Channel{Type: constant.ChannelTypeAdvancedCustom, Models: "img-model", Group: "default", BaseURL: &server.URL, OtherSettings: autoCheckImageSettings}
	for _, tc := range []struct{ name, mode, expr, reason string }{
		{"missing expression", `{"img-model":"tiered_expr"}`, `{}`, "billing_expr_missing"},
		{"invalid expression", `{"img-model":"tiered_expr"}`, `{"img-model":"fixture_secret_invalid_expression("}`, "billing_expr_failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"billing_setting.billing_mode": tc.mode, "billing_setting.billing_expr": tc.expr}))
			result := runChannelConfigOnlyCheck(context.Background(), channel, user.Id)
			require.Error(t, result.localErr)
			check := channelAutoCheckResult(channel, result, "config_only")
			assert.Equal(t, tc.reason, check.Detail["config_reason"])
			assert.Equal(t, "img-model", check.Detail["billing_model"])
			assert.NotContains(t, check.Detail["config_summary"], "fixture_secret")
		})
	}
}

func TestAutoCheckReadOnlySuccessDoesNotRestoreGenerationChannel(t *testing.T) {
	db := setupAutoCheckTest(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/openai/files", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()
	channel := &model.Channel{Type: constant.ChannelTypeAzureBatch, Models: "gpt-4o", BaseURL: &server.URL, Status: common.ChannelStatusAutoDisabled, ResponseTime: 1234}
	require.NoError(t, db.Create(channel).Error)
	summary := testChannelForHealthCheck(context.Background(), channel, 0, true, 0, true)
	assert.Equal(t, 1, summary.Succeeded)
	assert.EqualValues(t, 1, calls.Load())
	require.Len(t, summary.Checks, 1)
	assert.Equal(t, "response_received", summary.Checks[0].Detail["upstream_request"])
	assert.Equal(t, "passed", summary.Checks[0].Detail["readonly_check"])
	assert.Equal(t, "not_verified", summary.Checks[0].Detail["generation_evidence"])
	assertAutoCheckChannelUnchanged(t, db, channel.Id, common.ChannelStatusAutoDisabled, 1234)
}

func TestAutoCheckPreservesChannelConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name           string
		kind           int
		setting, other string
		multi, failed  bool
	}{
		{"polling keys", constant.ChannelTypeAdvancedCustom, "", autoCheckImageSettings, true, false},
		{"invalid other settings", constant.ChannelTypeAdvancedCustom, "", `{"broken":`, false, true},
		{"invalid settings", constant.ChannelTypeAdvancedCustom, `{"broken":`, autoCheckImageSettings, false, true},
		{"invalid text settings", constant.ChannelTypeOpenAI, "", `{"broken":`, false, true},
		{"invalid video settings", constant.ChannelTypeSeedanceLink, "", `{"broken":`, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupAutoCheckTest(t)
			user := newAutoCheckUser(t, db)
			require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"img-model":1}`))
			server := mustNotReachProviderServer(t)
			channel := &model.Channel{Type: tc.kind, Models: "img-model", Group: "default", Status: common.ChannelStatusEnabled, BaseURL: &server.URL, OtherSettings: tc.other}
			if tc.setting != "" {
				channel.Setting = &tc.setting
			}
			if tc.multi {
				channel.Key = "fixture-disabled\nfixture-enabled"
				channel.ChannelInfo = model.ChannelInfo{IsMultiKey: true, MultiKeySize: 2, MultiKeyMode: constant.MultiKeyModePolling, MultiKeyPollingIndex: 1, MultiKeyStatusList: map[int]int{0: common.ChannelStatusAutoDisabled}}
			}
			require.NoError(t, db.Create(channel).Error)
			before, err := common.Marshal(channel)
			require.NoError(t, err)
			var channelWrites atomic.Int32
			require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:no_channel_check_updates", func(tx *gorm.DB) {
				if tx.Statement.Table == "channels" {
					channelWrites.Add(1)
				}
			}))
			persisted := registerAutoCheckEventWait(t, db)
			summary := testChannelForHealthCheck(context.Background(), channel, user.Id, true, 0, true)
			require.Len(t, summary.Checks, 1)
			if tc.failed {
				assert.Equal(t, 1, summary.Failed)
				assert.Equal(t, "protocol_invalid", summary.Checks[0].Detail["config_reason"])
				waitPersistedAutoCheckEvent(t, persisted)
			} else {
				assert.Equal(t, 1, summary.Unsupported)
			}
			assert.Zero(t, channelWrites.Load())
			after, err := common.Marshal(channel)
			require.NoError(t, err)
			assert.JSONEq(t, string(before), string(after))
			var stored model.Channel
			require.NoError(t, db.First(&stored, channel.Id).Error)
			assert.Equal(t, channel.OtherSettings, stored.OtherSettings)
			assert.Equal(t, channel.Setting, stored.Setting)
			assert.Equal(t, channel.ChannelInfo, stored.ChannelInfo)
		})
	}
}

func TestAutoCheckPersistsConnectionOutcomeWithoutGenerationOrHealthWrites(t *testing.T) {
	for _, tc := range []struct {
		status      int
		observation string
		unsupported bool
	}{
		{200, "response_received", false}, {400, "response_received", false},
		{401, "auth_error", false}, {403, "auth_error", false}, {429, "rate_limited", false},
		{500, "service_error", false}, {503, "service_error", false}, {504, "service_error", false},
		{404, "probe_unsupported", true}, {405, "probe_unsupported", true}, {302, "probe_unsupported", true},
	} {
		t.Run(strconv.Itoa(tc.status), func(t *testing.T) {
			db := setupAutoCheckTest(t)
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, "/v1/models", r.URL.Path)
				assert.EqualValues(t, 0, r.ContentLength)
				assert.Equal(t, "Bearer usable-key", r.Header.Get("Authorization"))
				if tc.status == 302 {
					w.Header().Set("Location", "/v1/images/generations")
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(`{"api_key":"must-not-be-persisted"}`))
			}))
			defer server.Close()
			ch := &model.Channel{Type: constant.ChannelTypeOpenAI, Models: "gpt-image-2.5-flare", Key: "disabled-key\nusable-key", Status: common.ChannelStatusEnabled, BaseURL: &server.URL, ResponseTime: 1234, TestTime: 123, AutoBan: common.GetPointer(1)}
			ch.ChannelInfo = model.ChannelInfo{IsMultiKey: true, MultiKeyMode: constant.MultiKeyModePolling, MultiKeyPollingIndex: 0, MultiKeyStatusList: map[int]int{0: common.ChannelStatusAutoDisabled}}
			require.NoError(t, db.Create(ch).Error)
			before, err := common.Marshal(ch)
			require.NoError(t, err)
			persisted := registerAutoCheckEventWait(t, db)
			// No user or pricing is needed for a read-only connection check. A negative
			// generation-time threshold must not disable this channel.
			summary := testChannelForHealthCheck(context.Background(), ch, 0, true, -1, true)
			require.Len(t, summary.Checks, 1)
			d := summary.Checks[0].Detail
			assert.Equal(t, "readonly_probe", d["check_scope"])
			assert.Equal(t, "not_checked", d["config_check"])
			assert.NotContains(t, d, "billing_model")
			assert.Equal(t, "image", d["probe_media"])
			assert.Equal(t, "response_received", d["upstream_request"])
			assert.Equal(t, strconv.Itoa(tc.status), d["upstream_status"])
			assert.Equal(t, tc.observation, d["connection_result"])
			assert.EqualValues(t, 1, calls.Load())
			assert.Zero(t, summary.Disabled)
			assert.Zero(t, summary.Enabled)
			var stored model.Channel
			require.NoError(t, db.First(&stored, ch.Id).Error)
			after, err := common.Marshal(&stored)
			require.NoError(t, err)
			assert.JSONEq(t, string(before), string(after))
			if tc.unsupported {
				assert.Equal(t, 1, summary.Unsupported)
				assert.Zero(t, summary.Failed)
			} else if tc.status >= 400 {
				assert.Equal(t, 1, summary.Failed)
				waitPersistedAutoCheckEvent(t, persisted)
				event, detail := loadAutoCheckEventDetail(t, db, ch.Id)
				assert.Equal(t, "GET", event.Method)
				assert.Equal(t, "/v1/models", event.Route)
				assert.Equal(t, tc.observation, detail["connection_result"])
				assert.Equal(t, "image", detail["probe_media"])
				assert.NotContains(t, event.Detail, "must-not-be-persisted")
			} else {
				assert.Equal(t, 1, summary.Succeeded)
			}
			encoded, err := common.Marshal(summary)
			require.NoError(t, err)
			var decoded channelTestSummary
			require.NoError(t, common.Unmarshal(encoded, &decoded))
			assert.Equal(t, summary.Checks, decoded.Checks)
		})
	}
}

func TestAutoCheckUnknownAndNativeMediaNeverGenerate(t *testing.T) {
	for _, tc := range []struct {
		kind     int
		model    string
		readonly bool
	}{
		{constant.ChannelTypeAzure, "gpt-image-2", false},
		{constant.ChannelTypeOpenAI, "Seedance2.0", true},
		{constant.ChannelTypeOpenAI, "gpt-image-2", true},
		{constant.ChannelTypeAzure, "gpt-image-2.5-sunburst", false},
		{constant.ChannelTypeSora, "opaque-video-model", false},
	} {
		t.Run(strconv.Itoa(tc.kind)+tc.model, func(t *testing.T) {
			db := setupAutoCheckTest(t)
			user := newAutoCheckUser(t, db)
			require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"`+tc.model+`":0.1}`))
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				assert.True(t, tc.readonly)
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, "/v1/models", r.URL.Path)
				w.WriteHeader(200)
			}))
			defer server.Close()
			ch := &model.Channel{Type: tc.kind, Models: tc.model, Key: "fixture", Group: "default", BaseURL: &server.URL}
			result, scope := runAutomaticChannelCheck(context.Background(), ch, user.Id)
			assert.NotEqual(t, "generation_probe", scope)
			if tc.readonly {
				assert.EqualValues(t, 1, calls.Load())
				require.NoError(t, result.localErr)
			} else {
				assert.Zero(t, calls.Load())
			}
		})
	}
}

// Text probes keep their original request, pricing, failure and latency rules.
func TestAutoCheckPersistsFinalTextProbeOutcome(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		threshold int64
		reason    string
	}{
		{"unauthorized", 401, 100000, "test_upstream_rejected"},
		{"server error", 500, 100000, "test_upstream_rejected"},
		{"threshold exceeded", 200, -1, "response_time_exceeded"},
		{"successful probe", 200, 100000, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupAutoCheckTest(t)
			require.NoError(t, db.AutoMigrate(&model.Log{}))
			user := newAutoCheckUser(t, db)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "/v1/chat/completions", r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				if tc.status != 200 {
					_, _ = w.Write([]byte(`{"error":{"message":"fixture rejection","type":"invalid_request_error"}}`))
					return
				}
				_, _ = w.Write([]byte(`{"id":"fixture","object":"chat.completion","model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
			}))
			defer server.Close()
			ch := &model.Channel{Type: constant.ChannelTypeOpenAI, Models: "gpt-4o", Status: common.ChannelStatusEnabled, BaseURL: &server.URL, Group: "default"}
			require.NoError(t, db.Create(ch).Error)
			persisted := registerAutoCheckEventWait(t, db)
			summary := testChannelForHealthCheck(context.Background(), ch, user.Id, false, tc.threshold, true)
			require.Len(t, summary.Checks, 1)
			d := summary.Checks[0].Detail
			assert.Equal(t, "generation_probe", d["check_scope"])
			assert.Equal(t, "passed", d["config_check"])
			assert.Equal(t, "response_received", d["upstream_request"])
			assert.Equal(t, strconv.Itoa(tc.status), d["upstream_status"])
			assert.NotContains(t, d, "probe_media")
			assert.NotContains(t, d, "connection_result")
			if tc.reason != "" {
				assert.Equal(t, 1, summary.Failed)
				assert.Equal(t, tc.reason, d["check_reason"])
				waitPersistedAutoCheckEvent(t, persisted)
				_, stored := loadAutoCheckEventDetail(t, db, ch.Id)
				assert.Equal(t, d["check_reason"], stored["check_reason"])
			} else {
				assert.Equal(t, 1, summary.Succeeded)
			}
		})
	}
}

func TestAutoCheckUsesMappedMediaSignalWithoutChangingNativeRouting(t *testing.T) {
	setupAutoCheckTest(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/v1/models", r.URL.Path)
		w.WriteHeader(200)
	}))
	defer server.Close()
	ch := &model.Channel{Type: constant.ChannelTypeOpenAI, Models: "customer-alias", ModelMapping: common.GetPointer(`{"customer-alias":"gpt-image-2"}`), BaseURL: &server.URL}
	result, scope := runAutomaticChannelCheck(context.Background(), ch, 0)
	require.NoError(t, result.localErr)
	assert.Equal(t, "readonly_probe", scope)
	assert.Equal(t, "image", channelAutoCheckResult(ch, result, scope).Detail["probe_media"])
	assert.Equal(t, "", normalizeChannelTestEndpoint(ch, "customer-alias", ""), "manual/native resolver stays unchanged")
}

func TestAutoCheckMediaConnectionFailureIsNotGenerationFailure(t *testing.T) {
	db := setupAutoCheckTest(t)
	server := httptest.NewServer(http.NotFoundHandler())
	server.Close()
	ch := &model.Channel{Type: constant.ChannelTypeOpenAI, Models: "gpt-image-2", BaseURL: &server.URL, Status: common.ChannelStatusEnabled, AutoBan: common.GetPointer(1), ResponseTime: 1234, TestTime: 123}
	require.NoError(t, db.Create(ch).Error)
	persisted := registerAutoCheckEventWait(t, db)
	summary := testChannelForHealthCheck(context.Background(), ch, 0, true, 0, true)
	require.Equal(t, 1, summary.Failed)
	assert.Zero(t, summary.Disabled)
	waitPersistedAutoCheckEvent(t, persisted)
	event, d := loadAutoCheckEventDetail(t, db, ch.Id)
	assert.Equal(t, "connection_error", d["connection_result"])
	assert.Equal(t, "attempted", d["upstream_request"])
	assert.Equal(t, "image", d["probe_media"])
	assert.NotContains(t, d, "upstream_status")
	assert.Equal(t, "test_upstream_unreachable", event.Reason)
	var stored model.Channel
	require.NoError(t, db.First(&stored, ch.Id).Error)
	assert.Equal(t, 1234, stored.ResponseTime)
	assert.EqualValues(t, 123, stored.TestTime)
	assert.Equal(t, common.ChannelStatusEnabled, stored.Status)
}
