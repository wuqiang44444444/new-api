package controller

import (
	"context"
	"errors"
	"fmt"
	"github.com/QuantumNous/new-api/dto"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// 媒体自动检查只作已登记的只读连接检查或本地诊断，不退回生成测试。
// 文本和手工测试保留原路径。媒体名称信号只用于阻止生成，不赋予 Link 身份。

// channelAutoCheckModelName 解析自动检查将使用的测试模型，与 testChannel 的
// 解析顺序保持一致（指定测试模型 → 渠道模型清单首个 → 内置兜底）。
func channelAutoCheckModelName(channel *model.Channel) string {
	if channel == nil {
		return ""
	}
	if channel.TestModel != nil && *channel.TestModel != "" {
		return strings.TrimSpace(*channel.TestModel)
	}
	models := channel.GetModels()
	if len(models) > 0 && strings.TrimSpace(models[0]) != "" {
		return strings.TrimSpace(models[0])
	}
	return "gpt-4o-mini"
}

// Use the existing endpoint resolver; an image declaration with an ambiguous
// target must never be treated as evidence from a Chat probe.
func channelAutoCheckScopeForChannel(channel *model.Channel) string {
	modelName := channelAutoCheckModelName(channel)
	if seedanceLinkChannel(channel) || channel.Type == constant.ChannelTypeAzureBatch {
		return "readonly_probe"
	}
	if normalizeChannelTestEndpoint(channel, modelName, "") == string(constant.EndpointTypeImageGeneration) {
		return "config_only"
	}
	if channel.Type == constant.ChannelTypeAdvancedCustom {
		config := channel.GetOtherSettings().AdvancedCustom
		if config != nil {
			for _, endpoint := range config.SupportedEndpointTypesForModel(modelName) {
				if endpoint == constant.EndpointTypeImageGeneration {
					return "target_unresolved"
				}
			}
		}
	}
	// This is the existing native test's image signal, used only to stop a probe.
	// It does not establish Link identity or grant generation eligibility.
	if channel.Type == constant.ChannelTypeVolcEngine && strings.Contains(modelName, "seedream") {
		return "target_unresolved"
	}
	if kind := channelAutoCheckMediaKind(channel); kind != "" {
		if channel.Type == constant.ChannelTypeOpenAI || kind == "video" || channel.Type == constant.ChannelTypeMidjourney || channel.Type == constant.ChannelTypeMidjourneyPlus {
			return "readonly_probe"
		}
		return "config_only"
	}
	return "generation_probe"
}

var errChannelAutoCheckTarget = errors.New("automatic channel test target needs confirmation")

// Check raw JSON before calling native getters: those getters repair malformed
// settings by saving the channel. Diagnostics must preserve the original facts.
func validateChannelAutoCheckSettings(channel *model.Channel) *types.NewAPIError {
	var setting dto.ChannelSettings
	var other dto.ChannelOtherSettings
	if channel.Setting != nil && *channel.Setting != "" {
		if err := common.UnmarshalJsonStr(*channel.Setting, &setting); err != nil {
			return types.NewError(errors.New("channel settings contain invalid JSON"), types.ErrorCodeInvalidApiType)
		}
	}
	if channel.OtherSettings != "" {
		if err := common.UnmarshalJsonStr(channel.OtherSettings, &other); err != nil {
			return types.NewError(errors.New("channel other settings contain invalid JSON"), types.ErrorCodeInvalidApiType)
		}
	}
	return nil
}

func runAutomaticChannelCheck(ctx context.Context, channel *model.Channel, testUserID int) (testResult, string) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateChannelAutoCheckSettings(channel); err != nil {
		return testResult{localErr: err, newAPIError: err}, "config_only"
	}
	scope := channelAutoCheckScopeForChannel(channel)
	if seedanceLinkChannel(channel) {
		return seedanceLinkChannelHealthResult(ctx, channel), scope
	}
	if scope == "readonly_probe" && channel.Type == constant.ChannelTypeOpenAI {
		return runChannelConnectionProbe(ctx, channel), scope
	}
	if scope == "generation_probe" || channel.Type == constant.ChannelTypeAzureBatch {
		return testChannel(ctx, channel, testUserID, "", "", shouldUseStreamForAutomaticChannelTest(channel)), scope
	}
	if scope == "readonly_probe" {
		// No registered read-only operation. Do not run Chat preparation/pricing
		// against native task adapters and misreport it as broken configuration.
		return testResult{testedModel: channelAutoCheckModelName(channel), checkUnsupported: true}, scope
	}
	result := runChannelConfigOnlyCheck(ctx, channel, testUserID)
	result.checkUnsupported = channelAutoCheckMediaKind(channel) != "" && result.localErr == nil && result.newAPIError == nil
	return result, scope
}

// runChannelConfigOnlyCheck 对生成类目标渠道执行无网络请求的本地配置检查：
// 复用正式测试路径的前置校验（上下文、协议注册、模型映射、测试样例、计费配置），
// 在计费校验通过后停止。不发送上游请求、不预扣、不创建 Task/attempt，错误形状
// 与 testChannel 一致，因此事件分类与受控诊断复用同一合同。
func runChannelConfigOnlyCheck(ctx context.Context, channel *model.Channel, testUserID int) (result testResult) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateChannelAutoCheckSettings(channel); err != nil {
		return testResult{localErr: err, newAPIError: err}
	}
	// Reuse native enabled-key selection on a private projection. Random selection
	// only reads key availability; polling would advance and persist a cursor even
	// though this check sends no request. No key-specific availability is claimed.
	snapshot := *channel
	if snapshot.ChannelInfo.IsMultiKey && snapshot.ChannelInfo.MultiKeyMode == constant.MultiKeyModePolling {
		snapshot.ChannelInfo.MultiKeyMode = constant.MultiKeyModeRandom
	}
	channel = &snapshot
	testModel := channelAutoCheckModelName(channel)
	defer func() {
		// 与 testChannel 相同：冻结实际测试模型供错误事件使用。
		result.testedModel = testModel
	}()

	if channelAutoCheckScopeForChannel(channel) == "target_unresolved" {
		return testResult{localErr: errChannelAutoCheckTarget, newAPIError: types.NewError(errChannelAutoCheckTarget, types.ErrorCodeInvalidApiType)}
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	endpointType := normalizeChannelTestEndpoint(channel, testModel, "")
	requestPath := "/v1/chat/completions"
	if endpointType != "" {
		if endpointInfo, ok := common.GetDefaultEndpointInfo(constant.EndpointType(endpointType)); ok {
			requestPath = endpointInfo.Path
		}
	}
	c.Request = httptest.NewRequestWithContext(ctx, http.MethodPost, requestPath, nil)
	clienterrlog.InstallHTTPExchange(c)

	cache, err := model.GetUserCache(testUserID)
	if err != nil {
		return testResult{localErr: err}
	}
	cache.WriteContext(c)
	c.Set("id", testUserID)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("channel", channel.Type)
	c.Set("base_url", channel.GetBaseURL())
	group, _ := model.GetUserGroup(testUserID, false)
	c.Set("group", group)

	if channel.Type == constant.ChannelTypeAdvancedCustom {
		if config := channel.GetOtherSettings().AdvancedCustom; config != nil {
			if err := config.Validate(); err != nil {
				return testResult{context: c, localErr: err, newAPIError: types.NewError(err, types.ErrorCodeInvalidApiType)}
			}
		}
	}
	newAPIError := middleware.SetupContextForSelectedChannel(c, channel, testModel)
	if newAPIError != nil {
		return testResult{context: c, localErr: newAPIError, newAPIError: newAPIError}
	}

	// 与 testChannel 的端点判定一致：图片端点为图片格式，其余按 OpenAI 格式
	// 构造本地样例；生成请求构造与发送不在配置检查范围内。
	var relayFormat types.RelayFormat
	switch constant.EndpointType(endpointType) {
	case constant.EndpointTypeImageGeneration:
		relayFormat = types.RelayFormatOpenAIImage
	default:
		relayFormat = types.RelayFormatOpenAI
	}

	request := buildTestRequest(testModel, endpointType, channel, false)
	service.ObserveChannelTestRequest(c, request, string(relayFormat))

	info, err := relaycommon.GenRelayInfo(c, relayFormat, request, nil)
	if err != nil {
		return testResult{context: c, localErr: err, newAPIError: types.NewError(err, types.ErrorCodeGenRelayInfoFailed)}
	}
	info.IsChannelTest = true
	info.InitChannelMeta(c)

	if err := attachTestBillingRequestInput(info, request); err != nil {
		return testResult{context: c, localErr: err, newAPIError: types.NewError(err, types.ErrorCodeJsonMarshalFailed)}
	}
	if err := helper.ModelMappedHelper(c, info, request); err != nil {
		return testResult{context: c, localErr: err, newAPIError: types.NewError(err, types.ErrorCodeChannelModelMappedError)}
	}
	if err := helper.ApplyReasoningModelSuffix(c, info, request); err != nil {
		return testResult{
			context:     c,
			localErr:    err,
			newAPIError: types.NewErrorWithStatusCode(err, types.ErrorCodeConvertRequestFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry()),
		}
	}
	testModel = info.UpstreamModelName
	request.SetModelName(testModel)

	apiType, _ := common.ChannelType2APIType(channel.Type)
	if relay.GetAdaptor(apiType) == nil {
		err := fmt.Errorf("invalid api type: %d, adaptor is nil", apiType)
		return testResult{context: c, localErr: err, newAPIError: types.NewError(err, types.ErrorCodeInvalidApiType)}
	}

	_, err = helper.ModelPriceHelper(c, info, 0, request.GetTokenCountMeta())
	c.Set("channel_test_billing_model", info.GetBillingModelName())
	if err != nil {
		return testResult{
			context:     c,
			localErr:    err,
			newAPIError: types.NewError(err, types.ErrorCodeModelPriceError, types.ErrOptionWithStatusCode(http.StatusBadRequest)),
		}
	}
	return testResult{context: c}
}

// Each completed automatic check is stored in the existing system-task result.
// It is historical evidence for this run, never another channel status authority.
func channelAutoCheckResult(channel *model.Channel, result testResult, scope string) service.ChannelAutoCheckResult {
	detail := map[string]string{"check_scope": scope, "upstream_request": "not_sent", "config_check": "passed", "check_result": "passed"}
	if kind := channelAutoCheckMediaKind(channel); kind != "" {
		detail["probe_media"] = kind
		detail["connection_result"] = channelConnectionObservation(result)
	}
	if result.checkUnsupported {
		detail["check_result"] = "unsupported"
	} else if result.localErr != nil || result.newAPIError != nil {
		detail["check_result"] = "failed"
	}
	if !result.checkUnsupported && (result.localErr != nil || result.newAPIError != nil) {
		_, reason, code := service.ClassifyChannelTestFailure(result.localErr, result.newAPIError, "")
		detail["check_reason"], detail["check_code"] = reason, code
	}
	status := result.upstreamStatus
	if status == 0 && result.context != nil {
		status = result.context.GetInt("channel_test_upstream_status")
	}
	if status > 0 {
		detail["upstream_status"] = strconv.Itoa(status)
	}
	if result.upstreamAttempted {
		detail["upstream_request"] = "attempted"
	}
	if result.upstreamResponded {
		detail["upstream_request"] = "response_received"
	}
	if !result.upstreamAttempted && (result.localErr != nil || result.newAPIError != nil) {
		detail["config_check"] = "not_checked"
	}
	if scope == "config_only" || scope == "target_unresolved" {
		detail["generation_evidence"] = "not_verified"
		detail["readonly_check"] = "unsupported"
		if result.localErr != nil || result.newAPIError != nil {
			detail["config_check"] = "failed"
		}
	}
	if scope == "readonly_probe" {
		detail["config_check"] = "not_checked"
		detail["generation_evidence"] = "not_verified"
		detail["readonly_check"] = "passed"
		if result.localErr != nil {
			detail["readonly_check"] = "failed"
		}
		if result.checkUnsupported {
			detail["readonly_check"] = "unsupported"
		}
		code := service.ChannelConnectivityErrorCode(result.localErr)
		switch code {
		case service.ChannelConnectivityAssetNotConfigured, service.ChannelConnectivityAssetInvalidConfig:
			detail["config_check"], detail["readonly_check"] = "failed", "not_checked"
			detail["config_reason"], detail["config_entry"] = "protocol_invalid", "channel_edit"
			detail["config_summary"] = "Check the channel protocol configuration."
		}
	}
	if result.context != nil {
		if billingModel := result.context.GetString("channel_test_billing_model"); billingModel != "" {
			detail["billing_model"] = billingModel
		}
	}
	if diagnostic := service.DiagnoseChannelTestConfigFailure(result.localErr, result.newAPIError); diagnostic != nil {
		detail["config_check"] = "failed"
		detail["config_reason"], detail["config_entry"], detail["config_summary"] = diagnostic.Reason, diagnostic.Entry, diagnostic.Summary
		if diagnostic.BillingModel != "" {
			detail["billing_model"] = diagnostic.BillingModel
		}
	}
	if errors.Is(result.localErr, errChannelAutoCheckTarget) {
		detail["config_reason"], detail["config_entry"] = "target_ambiguous", "channel_edit"
		detail["config_summary"] = "Confirm the automatic check target in channel settings."
	}
	return service.ChannelAutoCheckResult{ChannelID: channel.Id, Model: channelAutoCheckModelName(channel), CheckedAt: time.Now().Unix(), Detail: detail}
}
